// Package config loads runtime configuration from the environment.
package config

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Config holds all runtime configuration for the server.
type Config struct {
	Port          string
	DataDir       string
	SessionTTL    time.Duration // default session lifetime when the client does not request a longer one
	MaxSessionTTL time.Duration // upper bound for "forever" style sessions
	LogLevel      string

	// WebAuthn relying-party configuration. When empty these are derived
	// from the incoming request host so the app works on a LAN IP as well
	// as behind a public domain.
	WebAuthnRPID     string
	WebAuthnOrigins  []string
	TrustedProxyCIDR []string // comma separated CIDRs allowed to set X-Forwarded-For
}

// Load reads configuration from environment variables, applying sane
// defaults suitable for a small NAS deployment.
func Load() Config {
	return Config{
		Port:             env("PORT", "8080"),
		DataDir:          env("DATA_DIR", "./data"),
		SessionTTL:       duration("SESSION_TTL", 30*24*time.Hour),
		MaxSessionTTL:    duration("MAX_SESSION_TTL", 10*365*24*time.Hour),
		LogLevel:         env("LOG_LEVEL", "info"),
		WebAuthnRPID:     os.Getenv("WEBAUTHN_RP_ID"),
		WebAuthnOrigins:  splitList(os.Getenv("WEBAUTHN_ORIGINS")),
		TrustedProxyCIDR: splitList(os.Getenv("TRUSTED_PROXY_CIDRS")),
	}
}

func (c Config) DBPath() string   { return filepath.Join(c.DataDir, "db.sqlite") }
func (c Config) FilesDir() string { return filepath.Join(c.DataDir, "files") }
func (c Config) TmpDir() string   { return filepath.Join(c.DataDir, "tmp") }

// EnsureDirs creates the data directory layout.
func (c Config) EnsureDirs() error {
	for _, d := range []string{c.DataDir, c.FilesDir(), c.TmpDir()} {
		if err := os.MkdirAll(d, 0o750); err != nil {
			return err
		}
	}
	return nil
}

func env(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func duration(key string, fallback time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return fallback
	}
	if d, err := time.ParseDuration(v); err == nil {
		return d
	}
	if n, err := strconv.Atoi(v); err == nil {
		return time.Duration(n) * 24 * time.Hour // treat bare integers as days
	}
	return fallback
}

func splitList(v string) []string {
	var out []string
	for _, p := range strings.Split(v, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
