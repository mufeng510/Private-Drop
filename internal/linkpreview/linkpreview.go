// Package linkpreview fetches basic metadata for a URL with strict SSRF
// protection. It never accesses private, loopback, link-local, metadata or
// otherwise disallowed network targets, and revalidates every redirect.
package linkpreview

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

var (
	ErrBlocked = errors.New("url target is not allowed")
	ErrBadURL  = errors.New("invalid url")
	ErrScheme  = errors.New("unsupported url scheme")
)

const (
	fetchTimeout = 6 * time.Second
	dnsTimeout   = 2 * time.Second
	maxBodyBytes = 256 << 10
	maxRedirects = 3
)

var blockedNets = func() []*net.IPNet {
	cidrs := []string{
		"0.0.0.0/8", "10.0.0.0/8", "100.64.0.0/10", "127.0.0.0/8",
		"169.254.0.0/16", "172.16.0.0/12", "192.0.0.0/24", "192.168.0.0/16",
		"198.18.0.0/15", "224.0.0.0/4", "240.0.0.0/4",
		"::/128", "::1/128", "fc00::/7", "fe80::/10", "ff00::/8",
	}
	out := make([]*net.IPNet, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err == nil {
			out = append(out, n)
		}
	}
	return out
}()

// IsBlockedIP reports whether an IP is in a disallowed range.
func IsBlockedIP(ip net.IP) bool {
	if ip == nil {
		return true
	}
	if ip.IsUnspecified() || ip.IsLoopback() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsPrivate() || ip.IsMulticast() {
		return true
	}
	for _, n := range blockedNets {
		if n.Contains(ip) {
			return true
		}
	}
	return false
}

var blockedSuffixes = []string{
	".localhost", ".local", ".internal", ".home.arpa", ".localdomain",
}

// validate resolves the host and ensures every resolved address is public.
// It returns the host and port to dial (the validated IP is used directly so
// a subsequent DNS answer cannot rebind the connection).
func validate(ctx context.Context, raw string) (target string, u *url.URL, err error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return "", nil, ErrBadURL
	}
	scheme := strings.ToLower(parsed.Scheme)
	if scheme != "http" && scheme != "https" {
		return "", nil, ErrScheme
	}
	if parsed.User != nil {
		return "", nil, ErrBlocked // credentials can smuggle proxy tricks
	}
	host := parsed.Hostname()
	if host == "" {
		return "", nil, ErrBadURL
	}
	lower := strings.ToLower(host)
	for _, s := range blockedSuffixes {
		if lower == strings.TrimPrefix(s, ".") || strings.HasSuffix(lower, s) {
			return "", nil, ErrBlocked
		}
	}
	if strings.EqualFold(host, "localhost") {
		return "", nil, ErrBlocked
	}

	ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
	if err != nil {
		return "", nil, fmt.Errorf("%w: dns lookup failed", ErrBlocked)
	}
	if len(ips) == 0 {
		return "", nil, ErrBlocked
	}
	for _, ipa := range ips {
		if IsBlockedIP(ipa.IP) {
			return "", nil, ErrBlocked
		}
	}
	port := parsed.Port()
	if port == "" {
		if scheme == "https" {
			port = "443"
		} else {
			port = "80"
		}
	}
	// Dial the first validated IP directly. net/http still uses the original
	// hostname for TLS SNI and the Host header.
	return net.JoinHostPort(ips[0].IP.String(), port), parsed, nil
}

// ValidateURL validates a URL without fetching it. Exported for tests.
func ValidateURL(raw string) error {
	_, _, err := validateWithTimeout(raw)
	return err
}

// ValidateImageURL applies the same SSRF policy to an og:image URL. A blocked
// or malformed image URL must be dropped before it is persisted or returned.
func ValidateImageURL(raw string) error {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	_, _, err := validateWithTimeout(raw)
	return err
}

var reURL = regexp.MustCompile(`(?i)https?://[^\s<>"'\x60]+`)

// ExtractURLs returns unique http(s) URLs found in a block of text, preserving
// first-seen order and bounding the count to avoid excessive preview fetches.
func ExtractURLs(text string, max int) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, m := range reURL.FindAllString(text, -1) {
		u := strings.TrimRight(m, ".,;:!)]}")
		if seen[u] {
			continue
		}
		seen[u] = true
		out = append(out, u)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

func validateWithTimeout(raw string) (string, *url.URL, error) {
	ctx, cancel := context.WithTimeout(context.Background(), dnsTimeout)
	defer cancel()
	return validate(ctx, raw)
}

type Previewer struct{}

func New() *Previewer { return &Previewer{} }

// clientFor builds a client that always dials the pre-validated IP:port while
// leaving the request URL (and therefore TLS SNI and the Host header) intact.
// This prevents DNS rebinding between validation and connection.
func (p *Previewer) clientFor(target string) *http.Client {
	transport := &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(target)
			if err != nil || IsBlockedIP(net.ParseIP(host)) {
				return nil, ErrBlocked
			}
			d := &net.Dialer{Timeout: 4 * time.Second, KeepAlive: 30 * time.Second}
			return d.DialContext(ctx, network, target)
		},
		TLSClientConfig:     &tls.Config{MinVersion: tls.VersionTLS12},
		MaxIdleConns:        1,
		IdleConnTimeout:     10 * time.Second,
		TLSHandshakeTimeout: 4 * time.Second,
	}
	return &http.Client{
		Transport: transport,
		Timeout:   fetchTimeout,
		CheckRedirect: func(_ *http.Request, _ []*http.Request) error {
			return http.ErrUseLastResponse // redirects handled manually with revalidation
		},
	}
}

type Result struct {
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
	ImageURL    string `json:"imageUrl,omitempty"`
}

// Preview fetches and parses metadata. Failures return an empty result and
// an error; callers should treat this as non-fatal for message sending.
func (p *Previewer) Preview(ctx context.Context, raw string) (Result, error) {
	target, u, err := validateWithTimeout(raw)
	if err != nil {
		return Result{}, err
	}
	current := u.String()
	for i := 0; i <= maxRedirects; i++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, current, nil)
		if err != nil {
			return Result{}, err
		}
		req.Header.Set("User-Agent", "PrivateDropLinkPreview/1.0")
		req.Header.Set("Accept", "text/html,application/xhtml+xml;q=0.9,*/*;q=0.5")
		client := p.clientFor(target)
		defer client.CloseIdleConnections()
		resp, err := client.Do(req)
		if err != nil {
			return Result{}, err
		}
		if resp.StatusCode >= 300 && resp.StatusCode < 400 {
			loc := resp.Header.Get("Location")
			resp.Body.Close()
			if loc == "" {
				return Result{}, errors.New("redirect without location")
			}
			next, err := url.Parse(loc)
			if err != nil {
				return Result{}, ErrBadURL
			}
			current = u.ResolveReference(next).String()
			// Revalidate the redirect target against the SSRF policy.
			target, u, err = validateWithTimeout(current)
			if err != nil {
				return Result{}, err
			}
			continue
		}
		defer resp.Body.Close()
		if resp.StatusCode < 200 || resp.StatusCode >= 300 {
			return Result{}, fmt.Errorf("unexpected status %d", resp.StatusCode)
		}
		ct := strings.ToLower(resp.Header.Get("Content-Type"))
		if !strings.Contains(ct, "html") && !strings.Contains(ct, "text/") {
			return Result{}, errors.New("non-html content")
		}
		b, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes))
		if err != nil {
			return Result{}, err
		}
		return parseHTML(string(b), current), nil
	}
	return Result{}, errors.New("too many redirects")
}

var (
	reTitle  = regexp.MustCompile(`(?is)<title[^>]*>\s*(.*?)\s*</title>`)
	reDesc   = regexp.MustCompile(`(?is)<meta[^>]+(?:name|property)=["'](?:description|og:description)["'][^>]+content=["'](.*?)["']`)
	reDesc2  = regexp.MustCompile(`(?is)<meta[^>]+content=["'](.*?)["'][^>]+(?:name|property)=["'](?:description|og:description)["']`)
	reImage  = regexp.MustCompile(`(?is)<meta[^>]+property=["']og:image["'][^>]+content=["'](.*?)["']`)
	reImage2 = regexp.MustCompile(`(?is)<meta[^>]+content=["'](.*?)["'][^>]+property=["']og:image["']`)
)

func parseHTML(body, base string) Result {
	clean := func(s string) string {
		s = regexp.MustCompile(`(?s)<[^>]+>`).ReplaceAllString(s, "")
		s = strings.ReplaceAll(s, "&amp;", "&")
		s = strings.ReplaceAll(s, "&lt;", "<")
		s = strings.ReplaceAll(s, "&gt;", ">")
		s = strings.ReplaceAll(s, "&quot;", "\"")
		s = strings.TrimSpace(s)
		if len(s) > 300 {
			s = s[:300]
		}
		return s
	}
	var r Result
	if m := reTitle.FindStringSubmatch(body); len(m) > 1 {
		r.Title = clean(m[1])
	}
	if m := reDesc.FindStringSubmatch(body); len(m) > 1 {
		r.Description = clean(m[1])
	} else if m := reDesc2.FindStringSubmatch(body); len(m) > 1 {
		r.Description = clean(m[1])
	}
	if m := reImage.FindStringSubmatch(body); len(m) > 1 {
		r.ImageURL = clean(m[1])
	} else if m := reImage2.FindStringSubmatch(body); len(m) > 1 {
		r.ImageURL = clean(m[1])
	}
	if r.ImageURL != "" {
		if iu, err := url.Parse(r.ImageURL); err == nil && !iu.IsAbs() {
			if bu, err := url.Parse(base); err == nil {
				r.ImageURL = bu.ResolveReference(iu).String()
			}
		}
	}
	return r
}
