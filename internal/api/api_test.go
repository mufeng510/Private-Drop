package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"private-drop/internal/api"
	"private-drop/internal/file"
	"private-drop/internal/testutil"
	"private-drop/internal/util"
)

func newServer(t *testing.T) (http.Handler, *api.Server) {
	t.Helper()
	d, cfg := testutil.NewDB(t)
	srv := api.New(d, cfg)
	return srv.Handler(), srv
}

func doReq(h http.Handler, method, path, body, token, remote string) *httptest.ResponseRecorder {
	var r *http.Request
	if body == "" {
		r = httptest.NewRequest(method, path, nil)
	} else {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	}
	if token != "" {
		r.Header.Set("Authorization", "Bearer "+token)
	}
	if remote != "" {
		r.RemoteAddr = remote
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func initAdmin(t *testing.T, h http.Handler) string {
	t.Helper()
	w := doReq(h, http.MethodPost, "/api/auth/init",
		`{"username":"admin","password":"strongpassword","deviceName":"Test PC"}`, "", "127.0.0.1:1234")
	if w.Code != 200 {
		t.Fatalf("init failed: %d %s", w.Code, w.Body.String())
	}
	var res struct {
		Token string `json:"token"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &res); err != nil || res.Token == "" {
		t.Fatalf("init response: %s err=%v", w.Body.String(), err)
	}
	return res.Token
}

func TestUnauthenticatedAccessBlocked(t *testing.T) {
	h, _ := newServer(t)
	for _, path := range []string{"/api/messages", "/api/files", "/api/devices", "/api/settings", "/api/notifications"} {
		w := doReq(h, http.MethodGet, path, "", "", "127.0.0.1:1234")
		if w.Code != http.StatusUnauthorized {
			t.Errorf("GET %s unauthenticated = %d, want 401", path, w.Code)
		}
	}
}

func TestInitProxyHeaderFromUntrustedPeerBlocked(t *testing.T) {
	h, _ := newServer(t)
	// A public client through Cloudflare Tunnel has a private immediate peer
	// but carries proxy headers. Without TRUSTED_PROXY_CIDRS we must fail closed.
	r := httptest.NewRequest(http.MethodPost, "/api/auth/init",
		strings.NewReader(`{"username":"admin","password":"strongpassword"}`))
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != http.StatusForbidden {
		t.Fatalf("proxied init from untrusted peer should be 403, got %d %s", w.Code, w.Body.String())
	}
	// A plain LAN request still works.
	if token := initAdmin(t, h); token == "" {
		t.Fatal("LAN init should still succeed")
	}
}

func TestInitLANOnlyAndOnce(t *testing.T) {
	h, _ := newServer(t)

	// Remote (public) client cannot initialize.
	w := doReq(h, http.MethodPost, "/api/auth/init",
		`{"username":"admin","password":"strongpassword"}`, "", "8.8.8.8:1234")
	if w.Code != http.StatusForbidden {
		t.Fatalf("remote init should be 403, got %d %s", w.Code, w.Body.String())
	}

	token := initAdmin(t, h)

	// Entry is permanently closed after initialization, even from LAN.
	w = doReq(h, http.MethodPost, "/api/auth/init",
		`{"username":"other","password":"strongpassword"}`, "", "127.0.0.1:1234")
	if w.Code != http.StatusConflict {
		t.Fatalf("second init should be 409, got %d %s", w.Code, w.Body.String())
	}

	w = doReq(h, http.MethodGet, "/api/messages", "", token, "127.0.0.1:1234")
	if w.Code != http.StatusOK {
		t.Fatalf("authenticated messages = %d %s", w.Code, w.Body.String())
	}
}

func TestDownloadRequiresAuth(t *testing.T) {
	h, _ := newServer(t)
	// The download route must reject unauthenticated callers before touching storage.
	w := doReq(h, http.MethodGet, "/api/files/nonexistent/download", "", "", "127.0.0.1:1234")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated download = %d, want 401", w.Code)
	}
}

func TestDownloadAuthenticatedServesBytes(t *testing.T) {
	d, cfg := testutil.NewDB(t)
	srv := api.New(d, cfg)
	h := srv.Handler()
	token := initAdmin(t, h)

	payload := []byte("secret file content")
	sha := util.SHA256Hex(payload)
	files := file.New(d, cfg)
	entity, err := files.StoreEntity(context.Background(), bytes.NewReader(payload), sha)
	if err != nil {
		t.Fatalf("store entity: %v", err)
	}
	att, err := files.CreateAttachment(context.Background(), entity, "secret.txt", "text/plain", sha, int64(len(payload)), 0, 0)
	if err != nil {
		t.Fatalf("create attachment: %v", err)
	}

	w := doReq(h, http.MethodGet, "/api/files/"+att.ID+"/download", "", token, "127.0.0.1:1234")
	if w.Code != http.StatusOK {
		t.Fatalf("authenticated download = %d %s", w.Code, w.Body.String())
	}
	if !bytes.Equal(w.Body.Bytes(), payload) {
		t.Fatalf("download body = %q", w.Body.String())
	}
}

func TestMFAGateOnSettings(t *testing.T) {
	h, _ := newServer(t)
	token := initAdmin(t, h)
	w := doReq(h, http.MethodGet, "/api/settings", "", token, "127.0.0.1:1234")
	if w.Code != http.StatusForbidden {
		t.Fatalf("settings without mfa = %d, want 403", w.Code)
	}
}
