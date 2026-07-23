package health

import (
	"testing"
	"time"
)

func TestCircuitBreak(t *testing.T) {
	tr := New(10, 0.5, 5, 1000)
	for i := 0; i < 5; i++ {
		tr.Record("m1", false, 10*time.Millisecond)
	}
	broken, reason := tr.IsBroken("m1")
	if !broken {
		t.Fatalf("expected broken, reason=%s", reason)
	}
	// wait cooldown
	time.Sleep(1100 * time.Millisecond)
	broken, _ = tr.IsBroken("m1")
	if broken {
		t.Fatal("expected recovered after cooldown")
	}
}
