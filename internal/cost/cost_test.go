package cost

import (
	"testing"
)

func seedPrices(t *testing.T, m map[string]ModelPrice) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	ReloadPrices()
	if err := SavePrices(m); err != nil {
		t.Fatal(err)
	}
	ReloadPrices()
}

func TestEstimateKnown(t *testing.T) {
	seedPrices(t, map[string]ModelPrice{
		"gpt-4o-mini": {InputPer1M: 0.15, OutputPer1M: 0.60},
	})
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
	seedPrices(t, map[string]ModelPrice{})
	c := Estimate("my-local-llama", 100, 100)
	if c != nil {
		t.Fatal("expected unpriced")
	}
	if Format(nil) != "unpriced" {
		t.Fatal(Format(nil))
	}
}

func TestEstimateProviderPrefix(t *testing.T) {
	seedPrices(t, map[string]ModelPrice{
		"gpt-4o-mini": {InputPer1M: 0.15, OutputPer1M: 0.60},
	})
	c := Estimate("openai/gpt-4o-mini", 0, 0)
	if c == nil {
		t.Fatal("expected match")
	}
}

func TestEstimateExactProviderModel(t *testing.T) {
	seedPrices(t, map[string]ModelPrice{
		"openai/gpt-4o": {InputPer1M: 2.50, OutputPer1M: 10.00},
	})
	c := Estimate("openai/gpt-4o", 1_000_000, 0)
	if c == nil {
		t.Fatal("expected price")
	}
	if *c < 2.49 || *c > 2.51 {
		t.Fatalf("got %v", *c)
	}
}
