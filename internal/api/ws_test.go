package api_test

import (
	"context"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"private-drop/internal/api"
	"private-drop/internal/testutil"
)

func TestWebSocketAuthAndBroadcast(t *testing.T) {
	d, cfg := testutil.NewDB(t)
	srv := api.New(d, cfg)
	httpSrv := httptest.NewServer(srv.Handler())
	defer httpSrv.Close()

	// Unauthenticated upgrade is rejected.
	wsURL := "ws" + strings.TrimPrefix(httpSrv.URL, "http") + "/api/ws"
	_, _, err := websocket.Dial(context.Background(), wsURL, nil)
	if err == nil {
		t.Fatal("unauthenticated websocket connection must be rejected")
	}

	token := initAdmin(t, srv.Handler())
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, wsURL+"?token="+token, nil)
	if err != nil {
		t.Fatalf("authenticated dial: %v", err)
	}
	defer conn.Close(websocket.StatusNormalClosure, "")

	// Dial may return before the server registers the client; wait for it.
	for srv.WSClientCount() == 0 && ctx.Err() == nil {
		time.Sleep(5 * time.Millisecond)
	}
	srv.Publish("notification", map[string]any{"message": "hello"})
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("read event: %v", err)
	}
	if !strings.Contains(string(data), `"type":"notification"`) {
		t.Fatalf("unexpected event frame: %s", data)
	}
}
