// Package ws implements the WebSocket event hub. Authentication is performed
// via an injected validator so this package does not depend on auth or api.
package ws

import (
	"context"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"

	"private-drop/internal/util"
)

const historyLimit = 1000

// Event is the wire frame persisted in the durable event log and delivered to
// clients. Seq is monotonic; DeviceID identifies the originating device so a
// client may opt out of its own echoes.
type Event struct {
	Seq      int64  `json:"seq"`
	Type     string `json:"type"`
	Data     any    `json:"data"`
	TS       string `json:"ts"`
	DeviceID string `json:"deviceId,omitempty"`
}

// EventStore is the durable log used for reconnect catch-up. It is optional:
// without one the hub falls back to an in-memory ring buffer.
type EventStore interface {
	Append(typeName, deviceID string, data any) (Event, error)
	Since(cursor string, limit int) ([]Event, error)
	MaxSeq() int64
	Trim(ctx context.Context, olderThan time.Duration) error
}

type client struct {
	conn        *websocket.Conn
	send        chan []byte
	deviceID    string
	excludeSelf bool
}

// Hub broadcasts events and keeps both a durable log and a bounded in-memory
// history for reconnect catch-up.
type Hub struct {
	mu        sync.Mutex
	publishMu sync.Mutex
	clients   map[*client]struct{}
	history   []Event
	nextSeq   int64
	store     EventStore
	auth      func(token string) (string, bool)
}

func New(auth func(token string) (string, bool)) *Hub {
	return NewWithStore(auth, nil)
}

// NewWithStore creates a hub backed by a durable event store.
func NewWithStore(auth func(token string) (string, bool), store EventStore) *Hub {
	h := &Hub{clients: map[*client]struct{}{}, auth: auth, store: store}
	if store != nil {
		h.nextSeq = store.MaxSeq()
	}
	return h
}

// Store exposes the configured event store (used by the cleanup scheduler).
func (h *Hub) Store() EventStore { return h.store }

// Publish implements events.Sink.
func (h *Hub) Publish(eventType string, data any) { h.PublishFrom("", eventType, data) }

// PublishFrom publishes an event attributed to an originating device. When
// deviceID is empty it is inferred from a "deviceId" key in a map payload.
func (h *Hub) PublishFrom(deviceID, eventType string, data any) {
	if data == nil {
		data = map[string]any{}
	}
	if deviceID == "" {
		if m, ok := data.(map[string]any); ok {
			if v, ok := m["deviceId"].(string); ok {
				deviceID = v
			}
		}
	}

	// Serialize append + broadcast so delivery order matches sequence order.
	h.publishMu.Lock()
	defer h.publishMu.Unlock()

	var e Event
	if h.store != nil {
		if se, err := h.store.Append(eventType, deviceID, data); err == nil {
			e = se
		} else {
			log.Printf("ws persist %s: %v", eventType, err)
		}
	}
	if e.Seq == 0 {
		h.mu.Lock()
		h.nextSeq++
		e = Event{Seq: h.nextSeq, Type: eventType, Data: data, TS: util.Now(), DeviceID: deviceID}
		h.appendHistoryLocked(e)
		h.mu.Unlock()
	} else {
		h.mu.Lock()
		if e.Seq > h.nextSeq {
			h.nextSeq = e.Seq
		}
		h.appendHistoryLocked(e)
		h.mu.Unlock()
	}

	b, err := json.Marshal(e)
	if err != nil {
		return
	}
	h.mu.Lock()
	for c := range h.clients {
		if c.excludeSelf && e.DeviceID != "" && e.DeviceID == c.deviceID {
			continue
		}
		select {
		case c.send <- b:
		default:
			// Slow client: drop it rather than block all broadcasts.
			delete(h.clients, c)
			close(c.send)
			_ = c.conn.CloseNow()
		}
	}
	h.mu.Unlock()
}

func (h *Hub) appendHistoryLocked(e Event) {
	h.history = append(h.history, e)
	if len(h.history) > historyLimit {
		h.history = append([]Event(nil), h.history[len(h.history)-historyLimit:]...)
	}
}

// DisconnectDevice closes every live WebSocket belonging to a device. Called
// after remote revocation so containment takes effect immediately instead of
// waiting for the client to disconnect on its own.
func (h *Hub) DisconnectDevice(deviceID string) {
	h.mu.Lock()
	for c := range h.clients {
		if c.deviceID == deviceID {
			delete(h.clients, c)
			_ = c.conn.CloseNow()
		}
	}
	h.mu.Unlock()
}

func (h *Hub) clientCount() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return len(h.clients)
}

// ServeHTTP upgrades a WebSocket connection after validating the token.
// Replay events are delivered before the socket joins the broadcast set so a
// reconnecting client never receives a missed event twice.
func (h *Hub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	token := r.URL.Query().Get("token")
	if token == "" {
		if a := r.Header.Get("Authorization"); strings.HasPrefix(a, "Bearer ") {
			token = strings.TrimPrefix(a, "Bearer ")
		}
	}
	if h.auth == nil {
		http.Error(w, "ws auth unavailable", http.StatusServiceUnavailable)
		return
	}
	deviceID, ok := h.auth(token)
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{})
	if err != nil {
		return
	}
	c := &client{
		conn: conn, send: make(chan []byte, 512), deviceID: deviceID,
		excludeSelf: r.URL.Query().Get("excludeSelf") == "true",
	}

	ctx, cancel := context.WithCancel(r.Context())
	defer func() {
		cancel()
		h.mu.Lock()
		delete(h.clients, c)
		h.mu.Unlock()
		_ = conn.CloseNow()
	}()

	// Start the writer first so catch-up frames drain even if they exceed the
	// channel buffer.
	go h.writeLoop(ctx, c)

	// Replay BEFORE registering: hold publishMu so no new event can be appended
	// or broadcast between loading the backlog and joining the client set.
	since := r.URL.Query().Get("since")
	h.publishMu.Lock()
	if since != "" && h.store != nil {
		backlog, err := h.store.Since(since, historyLimit)
		if err == nil {
			for _, e := range backlog {
				if c.excludeSelf && e.DeviceID != "" && e.DeviceID == deviceID {
					continue
				}
				if b, err := json.Marshal(e); err == nil {
					select {
					case c.send <- b:
					case <-ctx.Done():
					}
				}
			}
		}
	}
	h.mu.Lock()
	h.clients[c] = struct{}{}
	h.mu.Unlock()
	h.publishMu.Unlock()

	h.readLoop(ctx, c)
}

func (h *Hub) readLoop(ctx context.Context, c *client) {
	for {
		typ, data, err := c.conn.Read(ctx)
		if err != nil {
			return
		}
		if typ != websocket.MessageText {
			continue
		}
		var msg struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(data, &msg) == nil && msg.Type == "ping" {
			pong, _ := json.Marshal(map[string]any{"type": "pong", "ts": util.Now()})
			select {
			case c.send <- pong:
			default:
			}
		}
	}
}

func (h *Hub) writeLoop(ctx context.Context, c *client) {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case b := <-c.send:
			wctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := c.conn.Write(wctx, websocket.MessageText, b)
			cancel()
			if err != nil {
				return
			}
		case <-ticker.C:
			pctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			err := c.conn.Ping(pctx)
			cancel()
			if err != nil && !errors.Is(err, context.Canceled) {
				log.Printf("ws ping failed: %v", err)
				return
			}
		}
	}
}

// Stats is used by system diagnostics.
func (h *Hub) Stats() map[string]any {
	h.mu.Lock()
	defer h.mu.Unlock()
	return map[string]any{
		"clients": len(h.clients),
		"events":  len(h.history),
		"seq":     h.nextSeq,
	}
}
