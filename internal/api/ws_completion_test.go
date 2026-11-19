package api_test

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"private-drop/internal/api"
	"private-drop/internal/testutil"
)

type wsFrame struct {
	Seq  int64           `json:"seq"`
	Type string          `json:"type"`
	Data json.RawMessage `json:"data"`
	TS   string          `json:"ts"`
}

func dialWS(t *testing.T, httpSrv *httptest.Server, token, extra string) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	url := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "/api/ws?token=" + token + extra
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
	return c
}

func readFrame(t *testing.T, ctx context.Context, c *websocket.Conn) wsFrame {
	t.Helper()
	_, data, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("read frame: %v", err)
	}
	var f wsFrame
	if err := json.Unmarshal(data, &f); err != nil {
		t.Fatalf("unmarshal frame %s: %v", data, err)
	}
	return f
}

// TestWebSocketTwoClientsBroadcastAndSinceReplay covers the reconnect
// contract: two live clients receive a message.created broadcast, and a third
// client connecting with ?since=<seq> replays only the missed event, exactly
// once, with no duplicates from the live broadcast path.
func TestWebSocketTwoClientsBroadcastAndSinceReplay(t *testing.T) {
	d, cfg := testutil.NewDB(t)
	srv := api.New(d, cfg)
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	token := initAdmin(t, srv.Handler())
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	c1 := dialWS(t, httpSrv, token, "")
	defer c1.Close(websocket.StatusNormalClosure, "")
	c2 := dialWS(t, httpSrv, token, "")
	defer c2.Close(websocket.StatusNormalClosure, "")
	for srv.WSClientCount() < 2 && ctx.Err() == nil {
		time.Sleep(5 * time.Millisecond)
	}

	// Create a message through the HTTP API -> message.created is broadcast.
	w := doReq(srv.Handler(), "POST", "/api/messages", `{"text":"ws broadcast"}`, token, "127.0.0.1:1234")
	if w.Code != 201 {
		t.Fatalf("create message: %d %s", w.Code, w.Body.String())
	}

	f1 := readFrame(t, ctx, c1)
	f2 := readFrame(t, ctx, c2)
	if f1.Type != "message.created" || f2.Type != "message.created" {
		t.Fatalf("expected message.created broadcast, got %q / %q", f1.Type, f2.Type)
	}
	if f1.Seq == 0 || f1.Seq != f2.Seq {
		t.Fatalf("both clients should see the same seq, got %d / %d", f1.Seq, f2.Seq)
	}
	if f1.TS == "" {
		t.Fatal("event should carry a timestamp")
	}

	// Disconnect both, publish another event, then reconnect with since=<seq>.
	c1.Close(websocket.StatusNormalClosure, "")
	c2.Close(websocket.StatusNormalClosure, "")
	time.Sleep(20 * time.Millisecond)

	w = doReq(srv.Handler(), "POST", "/api/messages", `{"text":"missed while offline"}`, token, "127.0.0.1:1234")
	if w.Code != 201 {
		t.Fatalf("create offline message: %d %s", w.Code, w.Body.String())
	}

	c3 := dialWS(t, httpSrv, token, "&since="+itoa64(f1.Seq))
	defer c3.Close(websocket.StatusNormalClosure, "")
	got := readFrame(t, ctx, c3)
	if got.Type != "message.created" || got.Seq <= f1.Seq {
		t.Fatalf("replay should deliver a newer message.created, got %+v", got)
	}
	var payload struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(got.Data, &payload)
	if payload.Text != "missed while offline" {
		t.Fatalf("replayed wrong payload: %s", got.Data)
	}

	// No duplicate: the very next frame must not be the same seq. A publish
	// after replay would have a strictly greater seq, and no live broadcast
	// duplicates the replayed seq (registration happens after replay).
	srv.Publish("notification", map[string]any{"m": "post-replay"})
	next := readFrame(t, ctx, c3)
	if next.Seq != got.Seq+1 {
		t.Fatalf("expected next seq %d, got %d (duplicate replay?)", got.Seq+1, next.Seq)
	}
}

func itoa64(n int64) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}
