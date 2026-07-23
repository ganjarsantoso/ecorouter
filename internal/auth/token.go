package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"math/big"
	"strings"
	"time"

	"golang.org/x/crypto/argon2"
)

const (
	TokenPrefix = "eco_live_"
	// Argon2id parameters (OWASP-ish, balanced for interactive create + fast verify).
	argonTime    = 1
	argonMemory  = 64 * 1024 // 64 MiB
	argonThreads = 4
	argonKeyLen  = 32
	saltLen      = 16
	// Opaque secret length in random bytes (encoded base62).
	secretBytes = 32
)

// HashResult is what we persist — never the plaintext token.
type HashResult struct {
	// Encoded form: $argon2id$v=19$m=...,t=...,p=...$salt$hash
	Encoded string
}

// Generate creates a new high-entropy Bearer token with eco_live_ prefix.
func Generate() (plaintext string, err error) {
	secret, err := randomBase62(secretBytes)
	if err != nil {
		return "", err
	}
	return TokenPrefix + secret, nil
}

// Hash computes an Argon2id hash of the full token string.
func Hash(plaintext string) (HashResult, error) {
	salt := make([]byte, saltLen)
	if _, err := rand.Read(salt); err != nil {
		return HashResult{}, err
	}
	hash := argon2.IDKey([]byte(plaintext), salt, argonTime, argonMemory, argonThreads, argonKeyLen)
	enc := fmt.Sprintf("$argon2id$v=19$m=%d,t=%d,p=%d$%s$%s",
		argonMemory, argonTime, argonThreads,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(hash),
	)
	return HashResult{Encoded: enc}, nil
}

// Verify checks plaintext against a stored encoded hash in constant time.
func Verify(plaintext, encoded string) (bool, error) {
	parts := strings.Split(encoded, "$")
	// "", "argon2id", "v=19", "m=...,t=...,p=...", salt, hash
	if len(parts) != 6 || parts[1] != "argon2id" {
		return false, fmt.Errorf("invalid hash encoding")
	}
	var memory uint32
	var timeCost uint32
	var threads uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &timeCost, &threads); err != nil {
		return false, fmt.Errorf("invalid argon params: %w", err)
	}
	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return false, err
	}
	want, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return false, err
	}
	got := argon2.IDKey([]byte(plaintext), salt, timeCost, memory, threads, uint32(len(want)))
	return subtle.ConstantTimeCompare(got, want) == 1, nil
}

// RedactToken shows only prefix + first 6 chars of secret for display.
func RedactToken(plaintext string) string {
	if !strings.HasPrefix(plaintext, TokenPrefix) {
		if len(plaintext) <= 8 {
			return "****"
		}
		return plaintext[:4] + "…"
	}
	rest := plaintext[len(TokenPrefix):]
	if len(rest) <= 6 {
		return TokenPrefix + rest
	}
	return TokenPrefix + rest[:6] + "…"
}

// ParseDuration accepts "90d", "24h", "30m", "never", or empty.
func ParseDuration(s string) (*time.Time, error) {
	s = strings.TrimSpace(strings.ToLower(s))
	if s == "" || s == "never" || s == "0" {
		return nil, nil
	}
	// Try Go duration first.
	if d, err := time.ParseDuration(s); err == nil {
		t := time.Now().UTC().Add(d)
		return &t, nil
	}
	// days
	if strings.HasSuffix(s, "d") {
		var n int
		if _, err := fmt.Sscanf(s, "%dd", &n); err != nil {
			return nil, fmt.Errorf("invalid duration %q", s)
		}
		t := time.Now().UTC().Add(time.Duration(n) * 24 * time.Hour)
		return &t, nil
	}
	return nil, fmt.Errorf("invalid duration %q (use 90d, 24h, 30m, or never)", s)
}

var base62Alphabet = []byte("0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz")

func randomBase62(nBytes int) (string, error) {
	// Produce ~1.3x chars from nBytes of entropy via base62.
	out := make([]byte, 0, nBytes*2)
	max := big.NewInt(int64(len(base62Alphabet)))
	// Enough symbols for nBytes of entropy: log62(256^n) ≈ n * 1.34
	count := int(float64(nBytes)*8/5.95) + 1
	for i := 0; i < count; i++ {
		v, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", err
		}
		out = append(out, base62Alphabet[v.Int64()])
	}
	return string(out), nil
}

// NewTokenID returns a short public id (not secret).
func NewTokenID() (string, error) {
	b := make([]byte, 6)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return fmt.Sprintf("tok_%s", base64.RawURLEncoding.EncodeToString(b)), nil
}
