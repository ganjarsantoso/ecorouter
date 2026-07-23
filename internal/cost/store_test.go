package cost

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSaveLoadRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	ReloadPrices()

	m := map[string]ModelPrice{
		"openai/gpt-4o": {InputPer1M: 2.5, OutputPer1M: 10},
		"local/llama":   {InputPer1M: 0, OutputPer1M: 0},
	}
	if err := SavePrices(m); err != nil {
		t.Fatal(err)
	}
	ReloadPrices()
	got := GetPrices()
	if len(got) != 2 {
		t.Fatalf("len=%d", len(got))
	}
	if got["openai/gpt-4o"].InputPer1M != 2.5 {
		t.Fatalf("in=%v", got["openai/gpt-4o"].InputPer1M)
	}
	if _, err := os.Stat(filepath.Join(dir, "pricing.toml")); err != nil {
		t.Fatal(err)
	}
}

func TestMissingFileEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	ReloadPrices()
	got := GetPrices()
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestMalformedFileEmpty(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	path := filepath.Join(dir, "pricing.toml")
	if err := os.WriteFile(path, []byte("not { valid toml [[["), 0o600); err != nil {
		t.Fatal(err)
	}
	ReloadPrices()
	got := GetPrices()
	if len(got) != 0 {
		t.Fatalf("expected empty on malformed, got %v", got)
	}
}
