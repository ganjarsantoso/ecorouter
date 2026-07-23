package auth

import (
	"strings"
	"testing"
	"time"
)

func TestGenerateAndVerify(t *testing.T) {
	plain, err := Generate()
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(plain, TokenPrefix) {
		t.Fatalf("missing prefix: %s", plain)
	}
	if len(plain) < len(TokenPrefix)+20 {
		t.Fatalf("token too short: %s", plain)
	}
	hr, err := Hash(plain)
	if err != nil {
		t.Fatal(err)
	}
	ok, err := Verify(plain, hr.Encoded)
	if err != nil || !ok {
		t.Fatalf("verify failed: ok=%v err=%v", ok, err)
	}
	ok, err = Verify(plain+"x", hr.Encoded)
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Fatal("expected mismatch")
	}
}

func TestParseDuration(t *testing.T) {
	exp, err := ParseDuration("90d")
	if err != nil || exp == nil {
		t.Fatalf("90d: %v %v", exp, err)
	}
	if exp.Before(time.Now().Add(89 * 24 * time.Hour)) {
		t.Fatal("expected ~90 days out")
	}
	exp, err = ParseDuration("")
	if err != nil || exp != nil {
		t.Fatalf("empty should be never: %v %v", exp, err)
	}
	exp, err = ParseDuration("1h")
	if err != nil || exp == nil {
		t.Fatal(err)
	}
}

func TestRedact(t *testing.T) {
	r := RedactToken("eco_live_abcdefghijklmnop")
	if !strings.HasPrefix(r, "eco_live_") || !strings.Contains(r, "…") {
		t.Fatalf("bad redact: %s", r)
	}
}
