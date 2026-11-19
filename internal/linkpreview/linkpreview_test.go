package linkpreview_test

import (
	"net"
	"testing"

	"private-drop/internal/linkpreview"
)

func TestSSRFBlocking(t *testing.T) {
	blocked := []string{
		"http://127.0.0.1/",
		"http://localhost/",
		"http://192.168.1.1/",
		"http://10.0.0.1/",
		"http://172.16.0.1/",
		"http://169.254.169.254/latest/meta-data/",
		"http://[::1]/",
		"http://[fe80::1]/",
		"http://0.0.0.0/",
		"file:///etc/passwd",
		"ftp://example.com/file",
		"javascript:alert(1)",
		"http://user:pass@example.com/",
		"http://foo.local/",
		"http://metadata.google.internal/",
	}
	for _, u := range blocked {
		if err := linkpreview.ValidateURL(u); err == nil {
			t.Errorf("SSRF target %q was not blocked", u)
		}
	}
}

func TestIsBlockedIP(t *testing.T) {
	for _, s := range []string{"127.0.0.1", "10.1.2.3", "172.20.0.1", "192.168.0.1", "169.254.169.254", "100.64.0.1", "::1", "fc00::1", "fe80::1"} {
		if !linkpreview.IsBlockedIP(net.ParseIP(s)) {
			t.Errorf("IP %s should be blocked", s)
		}
	}
	for _, s := range []string{"8.8.8.8", "1.1.1.1", "2606:4700:4700::1111"} {
		if linkpreview.IsBlockedIP(net.ParseIP(s)) {
			t.Errorf("public IP %s should be allowed", s)
		}
	}
}
