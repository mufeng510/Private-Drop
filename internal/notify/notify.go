// Package notify implements the in-app notification center.
package notify

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"private-drop/internal/db"
	"private-drop/internal/events"
	"private-drop/internal/util"
)

type Service struct {
	db     *db.DB
	events events.Sink
}

func New(d *db.DB, sink events.Sink) *Service { return &Service{db: d, events: sink} }

// Enabled reports whether a notify_* setting permits a category of in-app
// notification. Missing settings default to enabled.
func (s *Service) Enabled(ctx context.Context, key string) bool {
	var v string
	if err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key=?`, key).Scan(&v); err != nil {
		return true
	}
	return !strings.EqualFold(strings.TrimSpace(v), "false") && v != "0"
}

type Notification struct {
	ID        string         `json:"id"`
	Type      string         `json:"type"`
	Payload   map[string]any `json:"payload"`
	Read      bool           `json:"read"`
	CreatedAt string         `json:"createdAt"`
}

// Create stores a notification and broadcasts it to connected clients.
func (s *Service) Create(ctx context.Context, typ string, payload map[string]any) (Notification, error) {
	if payload == nil {
		payload = map[string]any{}
	}
	n := Notification{
		ID: util.NewID(), Type: typ, Payload: payload, CreatedAt: util.Now(),
	}
	b, _ := json.Marshal(payload)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO notifications(id,type,payload,read,created_at) VALUES(?,?,?,0,?)`,
		n.ID, typ, string(b), n.CreatedAt)
	if err != nil {
		return Notification{}, err
	}
	s.events.Publish("notification", n)
	return n, nil
}

// List returns notifications newest-first together with the unread count.
func (s *Service) List(ctx context.Context) ([]Notification, int, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,type,payload,read,created_at FROM notifications ORDER BY created_at DESC, rowid DESC LIMIT 500`)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Notification
	for rows.Next() {
		var n Notification
		var payload string
		var read int
		if err := rows.Scan(&n.ID, &n.Type, &payload, &read, &n.CreatedAt); err != nil {
			return nil, 0, err
		}
		n.Read = db.Bool(int64(read))
		_ = json.Unmarshal([]byte(payload), &n.Payload)
		if n.Payload == nil {
			n.Payload = map[string]any{}
		}
		out = append(out, n)
	}
	var unread int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM notifications WHERE read=0`).Scan(&unread); err != nil {
		return nil, 0, err
	}
	if out == nil {
		out = []Notification{}
	}
	return out, unread, rows.Err()
}

func (s *Service) ReadAll(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `UPDATE notifications SET read=1 WHERE read=0`)
	return err
}

func (s *Service) Delete(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM notifications WHERE id=?`, id)
	return err
}

// Cleanup removes notifications older than the given duration.
func (s *Service) Cleanup(ctx context.Context, olderThan time.Duration) error {
	cutoff := time.Now().UTC().Add(-olderThan).Format(time.RFC3339)
	_, err := s.db.ExecContext(ctx, `DELETE FROM notifications WHERE created_at < ?`, cutoff)
	return err
}
