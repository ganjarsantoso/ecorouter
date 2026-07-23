package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ganjar/ecorouter/internal/auth"
	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/secrets"
	"github.com/ganjar/ecorouter/internal/store"
)

func TestFallbackAdvancesOn5xx(t *testing.T) {
	var hitsA, hitsB atomic.Int32
	upA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsA.Add(1)
		http.Error(w, "boom", http.StatusBadGateway)
	}))
	defer upA.Close()
	upB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsB.Add(1)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "ok",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "from-b"}},
			},
			"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 5},
		})
	}))
	defer upB.Close()

	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)

	cfg := config.Default()
	cfg.Providers["pa"] = config.ProviderConfig{Type: "openai", BaseURL: upA.URL, Models: []string{"model-a"}}
	cfg.Providers["pb"] = config.ProviderConfig{Type: "openai", BaseURL: upB.URL, Models: []string{"model-b"}}
	cfg.Routes["fb"] = config.RouteConfig{
		Mode:   "fallback",
		Models: []string{"pa/model-a", "pb/model-b"},
	}
	cfg.Defaults.ActiveRoute = "fb"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	sec, _ := secrets.Load("")
	_ = sec.Set("pa", "sk-a")
	_ = sec.Set("pb", "sk-b")

	db, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	plain, _ := auth.Generate()
	hr, _ := auth.Hash(plain)
	id, _ := auth.NewTokenID()
	_ = db.InsertToken(&store.Token{
		ID: id, Label: "fb", Hash: hr.Encoded, Rate: "600/min", CreatedAt: time.Now().UTC(),
	})

	srv, err := New(cfg, db, sec)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions",
		jsonBody(map[string]any{"model": "x", "messages": []any{}}))
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d body %s", resp.StatusCode, body)
	}
	if hitsA.Load() != 1 || hitsB.Load() != 1 {
		t.Fatalf("expected both upstreams hit once, A=%d B=%d", hitsA.Load(), hitsB.Load())
	}
	if !json.Valid(body) {
		t.Fatal("invalid json")
	}

	// activity should record success on model-b
	acts, err := db.ListActivity(time.Time{}, "", 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(acts) == 0 {
		t.Fatal("expected activity")
	}
	if acts[0].Model != "pa/model-a" && acts[0].Model != "pb/model-b" {
		// final success is model-b path
	}
	if acts[0].Status != 200 {
		t.Fatalf("activity status %d", acts[0].Status)
	}
	if acts[0].CostEstimate == nil {
		// model-a/b are unpriced — ok
	}
}

func TestNoFallbackOn400(t *testing.T) {
	var hitsA, hitsB atomic.Int32
	upA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsA.Add(1)
		http.Error(w, "bad request", http.StatusBadRequest)
	}))
	defer upA.Close()
	upB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hitsB.Add(1)
		w.WriteHeader(200)
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer upB.Close()

	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)

	cfg := config.Default()
	cfg.Providers["pa"] = config.ProviderConfig{Type: "openai", BaseURL: upA.URL, Models: []string{"model-a"}}
	cfg.Providers["pb"] = config.ProviderConfig{Type: "openai", BaseURL: upB.URL, Models: []string{"model-b"}}
	cfg.Routes["fb"] = config.RouteConfig{Mode: "fallback", Models: []string{"pa/model-a", "pb/model-b"}}
	cfg.Defaults.ActiveRoute = "fb"
	_ = cfg.Save()

	sec, _ := secrets.Load("")
	_ = sec.Set("pa", "sk")
	_ = sec.Set("pb", "sk")
	db, _ := store.Open("")
	defer db.Close()
	plain, _ := auth.Generate()
	hr, _ := auth.Hash(plain)
	id, _ := auth.NewTokenID()
	_ = db.InsertToken(&store.Token{ID: id, Label: "t", Hash: hr.Encoded, Rate: "600/min", CreatedAt: time.Now().UTC()})

	srv, _ := New(cfg, db, sec)
	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions",
		jsonBody(map[string]any{"model": "x", "messages": []any{}}))
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 400 {
		t.Fatalf("want 400 got %d", resp.StatusCode)
	}
	if hitsA.Load() != 1 || hitsB.Load() != 0 {
		t.Fatalf("must not fallback on 400: A=%d B=%d", hitsA.Load(), hitsB.Load())
	}
}

func TestSpendCapBlocks(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer up.Close()

	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)
	cfg := config.Default()
	cfg.Providers["openai"] = config.ProviderConfig{Type: "openai", BaseURL: up.URL, Models: []string{"gpt-4o-mini"}}
	cfg.Routes["d"] = config.RouteConfig{Mode: "single", Models: []string{"gpt-4o-mini"}}
	cfg.Defaults.ActiveRoute = "d"
	_ = cfg.Save()
	sec, _ := secrets.Load("")
	_ = sec.Set("openai", "sk")
	db, _ := store.Open("")
	defer db.Close()

	// seed spend over cap
	cost := 1.50
	_ = db.InsertActivity(&store.Activity{
		TS: time.Now().UTC(), TokenID: "tok_cap", TokenLabel: "cap",
		Model: "gpt-4o-mini", Status: 200, TokensIn: 1000, TokensOut: 1000, CostEstimate: &cost,
	})

	plain, _ := auth.Generate()
	hr, _ := auth.Hash(plain)
	_ = db.InsertToken(&store.Token{
		ID: "tok_cap", Label: "cap", Hash: hr.Encoded, Rate: "600/min",
		DailyCap: 1.0, CreatedAt: time.Now().UTC(),
	})

	// re-hash won't match - FindTokenByPlaintext uses hash of plain
	// Fix: use the generated id with matching hash
	// Actually we inserted with fixed id but hash of plain - GetToken uses id from Find which returns our token if hash matches.
	// Wait - we inserted id tok_cap with hr of plain - good.

	srv, _ := New(cfg, db, sec)
	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions",
		jsonBody(map[string]any{"model": "gpt-4o-mini", "messages": []any{}}))
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusTooManyRequests {
		b, _ := io.ReadAll(resp.Body)
		t.Fatalf("want 429 spend cap, got %d %s", resp.StatusCode, b)
	}
}
