package cost

import "testing"

func TestEstimateKnown(t *testing.T) {
	c := Estimate("gpt-4o-mini", 1_000_000, 1_000_000)
	if c == nil {
		t.Fatal("expected price")
	}
	// 0.15 + 0.60 = 0.75
	if *c < 0.74 || *c > 0.76 {
		t.Fatalf("got %v", *c)
	}
}

func TestEstimateUnpriced(t *testing.T) {
	c := Estimate("my-local-llama", 100, 100)
	if c != nil {
		t.Fatal("expected unpriced")
	}
	if Format(nil) != "unpriced" {
		t.Fatal(Format(nil))
	}
}

func TestEstimateProviderPrefix(t *testing.T) {
	c := Estimate("openai/gpt-4o-mini", 0, 0)
	if c == nil {
		t.Fatal("expected match")
	}
}
