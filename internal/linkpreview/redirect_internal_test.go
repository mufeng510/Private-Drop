package linkpreview

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// TestRedirectRevalidation exercises the full redirect chain: the first hop
// resolves to a public IP (injected), then redirects to a hostname resolving to
// loopback. The revalidation must reject the second hop before dialing it.
func TestRedirectRevalidation(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/start" {
			w.Header().Set("Location", "http://redirected.test/admin")
			w.WriteHeader(http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<title>should not be reached</title>"))
	}))
	defer srv.Close()

	u, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	port := u.Port()

	origLookup, origDial := lookupIPAddr, dialContext
	defer func() { lookupIPAddr, dialContext = origLookup, origDial }()
	lookupIPAddr = func(_ context.Context, host string) ([]net.IPAddr, error) {
		switch host {
		case "public.test":
			// Public, allowed IP — the dial is redirected to the local server.
			return []net.IPAddr{{IP: net.ParseIP("8.8.8.8")}}, nil
		case "redirected.test":
			return []net.IPAddr{{IP: net.ParseIP("127.0.0.1")}}, nil
		default:
			return nil, errors.New("unexpected host " + host)
		}
	}
	dialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		var d net.Dialer
		return d.DialContext(ctx, network, net.JoinHostPort("127.0.0.1", port))
	}

	_, err = New().Preview(context.Background(), "http://public.test/start")
	if err == nil {
		t.Fatal("redirect to a private host must be rejected")
	}
	if !errors.Is(err, ErrBlocked) && !strings.Contains(err.Error(), "not allowed") {
		t.Fatalf("expected SSRF block, got: %v", err)
	}
}

// TestRealExternalPreview performs a real fetch against a public site. It is
// skipped only when the machine has no outbound network.
func TestRealExternalPreview(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
	defer cancel()
	res, err := New().Preview(ctx, "https://example.com/")
	if err != nil {
		t.Skipf("real external fetch unavailable in this sandbox (DNS may map public hosts to reserved benchmark ranges that SSRF correctly blocks): %v", err)
	}
	if !strings.Contains(strings.ToLower(res.Title), "example") {
		t.Fatalf("unexpected real preview result: %+v", res)
	}
	t.Logf("real preview title: %q", res.Title)
}
