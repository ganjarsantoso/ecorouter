package server

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/ganjar/ecorouter/internal/auth"
	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/secrets"
	"github.com/ganjar/ecorouter/internal/store"
)

func TestProxyAuthAndRoute(t *testing.T) {
	// mock upstream
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer sk-test" {
			http.Error(w, "no key", 401)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id": "chatcmpl-1",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "hi"}},
			},
			"usage": map[string]int{"prompt_tokens": 3, "completion_tokens": 1},
		})
	}))
	defer upstream.Close()

	dir := t.TempDir()
	t.Setenv("ECO_HOME", dir)

	cfg := config.Default()
	cfg.Server.Host = "127.0.0.1"
	cfg.Server.Port = 0 // unused; we use httptest
	cfg.Providers["openai"] = config.ProviderConfig{
		Type: "openai", BaseURL: upstream.URL, Models: []string{"gpt-test"},
	}
	cfg.Routes["default"] = config.RouteConfig{Mode: "single", Models: []string{"gpt-test"}}
	cfg.Defaults.ActiveRoute = "default"
	if err := cfg.Save(); err != nil {
		t.Fatal(err)
	}

	sec, err := secrets.Load("")
	if err != nil {
		t.Fatal(err)
	}
	if err := sec.Set("openai", "sk-test"); err != nil {
		t.Fatal(err)
	}

	db, err := store.Open("")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	plain, _ := auth.Generate()
	hr, _ := auth.Hash(plain)
	id, _ := auth.NewTokenID()
	if err := db.InsertToken(&store.Token{
		ID: id, Label: "test", Hash: hr.Encoded, Rate: "600/min", CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatal(err)
	}

	srv, err := New(cfg, db, sec)
	if err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(srv.http.Handler)
	defer ts.Close()

	// no auth
	resp, err := http.Get(ts.URL + "/v1/chat/completions")
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 401 {
		t.Fatalf("want 401 got %d", resp.StatusCode)
	}
	_ = resp.Body.Close()

	// with auth
	req, _ := http.NewRequest(http.MethodPost, ts.URL+"/v1/chat/completions",
		jsonBody(map[string]any{"model": "ignored", "messages": []any{}}))
	req.Header.Set("Authorization", "Bearer "+plain)
	req.Header.Set("Content-Type", "application/json")
	resp, err = http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		t.Fatalf("want 200 got %d body=%s", resp.StatusCode, b)
	}
	if !json.Valid(b) {
		t.Fatal("invalid json")
	}
}

func jsonBody(v any) io.Reader {
	b, _ := json.Marshal(v)
	return &reader{b: b}
}

type reader struct {
	b []byte
	i int
}

func (r *reader) Read(p []byte) (int, error) {
	if r.i >= len(r.b) {
		return 0, io.EOF
	}
	n := copy(p, r.b[r.i:])
	r.i += n
	return n, nil
}
