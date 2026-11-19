// Package util holds small helpers shared across backend packages.
package util

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"time"

	"github.com/google/uuid"
)

// timeLayout is RFC3339 with nanosecond precision. Nanoseconds give every
// event/cursor a distinct ordering key, which is required for stable keyset
// pagination and gap-free WebSocket catch-up. time.Parse(time.RFC3339, ...)
// still accepts the extra fractional digits, so persisted second-precision
// values remain readable.
const timeLayout = time.RFC3339Nano

// Now returns the current UTC time formatted as RFC3339Nano.
func Now() string { return time.Now().UTC().Format(timeLayout) }

// Format renders a time as RFC3339Nano UTC.
func Format(t time.Time) string { return t.UTC().Format(timeLayout) }

// ParseTime parses an RFC3339 or RFC3339Nano timestamp.
func ParseTime(s string) (time.Time, error) { return time.Parse(time.RFC3339, s) }

// NewID returns a new UUID string.
func NewID() string { return uuid.NewString() }

// RandomToken returns a URL-safe random token with 256 bits of entropy.
func RandomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// HashToken returns the SHA-256 hex digest of a token. Tokens are stored
// hashed so a database leak cannot be replayed as a session.
func HashToken(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// SHA256Hex returns the SHA-256 hex digest of arbitrary bytes.
func SHA256Hex(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
