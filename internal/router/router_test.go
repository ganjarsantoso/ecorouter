package router

import (
	"testing"

	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/health"
)

func TestSingleAndFallback(t *testing.T) {
	cfg := config.Default()
	cfg.Routes["s"] = config.RouteConfig{Mode: "single", Models: []string{"m1"}}
	cfg.Routes["f"] = config.RouteConfig{Mode: "fallback", Models: []string{"a", "b", "c"}}
	cfg.Defaults.ActiveRoute = "s"
	h := health.New(20, 0.5, 5, 60000)
	e := New(h)

	d, err := e.Resolve(cfg, "s")
	if err != nil || d.Selected != "m1" {
		t.Fatalf("single: %+v %v", d, err)
	}
	d, err = e.Resolve(cfg, "f")
	if err != nil || d.Selected != "a" || len(d.Models) != 3 {
		t.Fatalf("fallback: %+v %v", d, err)
	}
}

func TestRoundRobin(t *testing.T) {
	cfg := config.Default()
	cfg.Routes["r"] = config.RouteConfig{Mode: "round", Models: []string{"a", "b", "c"}}
	e := New(health.New(20, 0.5, 5, 60000))
	seen := []string{}
	for i := 0; i < 6; i++ {
		d, err := e.Resolve(cfg, "r")
		if err != nil {
			t.Fatal(err)
		}
		seen = append(seen, d.Selected)
	}
	want := []string{"a", "b", "c", "a", "b", "c"}
	for i := range want {
		if seen[i] != want[i] {
			t.Fatalf("round order: got %v want %v", seen, want)
		}
	}
}

func TestRoundSkipsBroken(t *testing.T) {
	cfg := config.Default()
	cfg.Routes["r"] = config.RouteConfig{Mode: "round", Models: []string{"a", "b", "c"}}
	h := health.New(20, 0.5, 5, 60000)
	// force break a by recording failures
	for i := 0; i < 10; i++ {
		h.Record("a", false, 0)
	}
	e := New(h)
	d, err := e.Resolve(cfg, "r")
	if err != nil {
		t.Fatal(err)
	}
	if d.Selected == "a" {
		// may still select if min requests not met — check broken
		if broken, _ := h.IsBroken("a"); broken && d.Selected == "a" {
			t.Fatal("should skip broken a")
		}
	}
}

func TestResolveModelProvider(t *testing.T) {
	cfg := config.Default()
	cfg.Providers["openai"] = config.ProviderConfig{
		Type: "openai", BaseURL: "https://api.openai.com/v1", Models: []string{"gpt-4o"},
	}
	p, m, u, typ, err := ResolveModelProvider(cfg, "openai/gpt-4o")
	if err != nil || p != "openai" || m != "gpt-4o" || typ != "openai" || u == "" {
		t.Fatalf("%s %s %s %s %v", p, m, u, typ, err)
	}
	p, m, _, _, err = ResolveModelProvider(cfg, "gpt-4o")
	if err != nil || p != "openai" || m != "gpt-4o" {
		t.Fatalf("bare: %s %s %v", p, m, err)
	}
}
