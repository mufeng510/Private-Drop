package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

// Argon2id parameters. 19 MiB memory, 2 passes, 1 lane — a reasonable
// balance for a low-power NAS while remaining resistant to GPU attacks.
const (
	argonMemory      = 19 * 1024
	argonIterations  = 2
	argonParallelism = 1
	argonKeyLen      = 32
	argonSaltLen     = 16
)

// HashPassword returns an encoded Argon2id hash:
//
//	$argon2id$v=19$m=<mem>,t=<iter>,p=<par>$<salt>$<key>
func HashPassword(password string) (string, error) {
	salt := make([]byte, argonSaltLen)
	if _, err := rand.Read(salt); err != nil {
		return "", err
	}
	key := argon2.IDKey([]byte(password), salt, argonIterations, argonMemory, argonParallelism, argonKeyLen)
	return fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonIterations, argonParallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(key)), nil
}

// VerifyPassword checks a password against an encoded hash.
func VerifyPassword(password, encoded string) bool {
	// $argon2id$v=19$params$salt$key -> split yields ["", "argon2id", "v=19", params, salt, key]
	parts := strings.Split(encoded, "$")
	if len(parts) != 6 || parts[1] != "argon2id" || parts[2] != "v=19" {
		return false
	}
	var mem, iter, par uint32
	for _, kv := range strings.Split(parts[3], ",") {
		kvp := strings.SplitN(kv, "=", 2)
		if len(kvp) != 2 {
			return false
		}
		n, err := strconv.ParseUint(kvp[1], 10, 32)
		if err != nil {
			return false
		}
		switch kvp[0] {
		case "m":
			mem = uint32(n)
		case "t":
			iter = uint32(n)
		case "p":
			par = uint32(n)
		}
	}
	if mem == 0 || iter == 0 || par == 0 {
		return false
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(want) == 0 {
		return false
	}
	got := argon2.IDKey([]byte(password), salt, iter, mem, byte(par), uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1
}
