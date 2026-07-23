package server

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/ganjar/ecorouter/internal/auth"
	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/health"
	"github.com/ganjar/ecorouter/internal/router"
	"github.com/ganjar/ecorouter/internal/secrets"
	"github.com/ganjar/ecorouter/internal/store"
)

type Server struct {
	cfg     *config.Config
	db      *store.Store
	secrets *secrets.Store
	engine  *router.Engine
	health  *health.Tracker
	rl      *RateLimiter
	lockout *Lockout
	access  *AccessControl
	conc    *Concurrency
	http    *http.Server
	logf    *log.Logger
	mu      sync.RWMutex
}

func New(cfg *config.Config, db *store.Store, sec *secrets.Store) (*Server, error) {
	h := health.New(cfg.Health.Window, cfg.Health.ErrorThreshold, cfg.Health.MinRequests, cfg.Health.CooldownMs)
	ac, err := NewAccess(cfg.Access)
	if err != nil {
		return nil, err
	}
	s := &Server{
		cfg:     cfg,
		db:      db,
		secrets: sec,
		engine:  router.New(h),
		health:  h,
		rl:      NewRateLimiter(cfg.Security.GlobalRate, "60/min"),
		lockout: NewLockout(cfg.Security.AuthFailLockout),
		access:  ac,
		conc:    NewConcurrency(),
		logf:    log.New(os.Stdout, "", log.LstdFlags),
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", s.handleHealthz)
	mux.HandleFunc("/v1/", s.handleAPI)
	mux.HandleFunc("/", s.handleRoot)
	s.http = &http.Server{
		Addr:              net.JoinHostPort(cfg.Server.Host, strconv.Itoa(cfg.Server.Port)),
		Handler:           s.middleware(mux),
		ReadHeaderTimeout: 10 * time.Second,
		// Read/Write timeouts left open for streaming; request context handles upstream.
		MaxHeaderBytes: 1 << 20,
	}
	return s, nil
}

func (s *Server) Engine() *router.Engine { return s.engine }
func (s *Server) Health() *health.Tracker { return s.health }

func (s *Server) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// security headers on every response path that we control early
		s.setSecurityHeaders(w)
		// body size is enforced in handlers; also limit via MaxBytesReader for safety
		if s.cfg.Security.MaxBodyBytes > 0 && r.Body != nil {
			r.Body = http.MaxBytesReader(w, r.Body, s.cfg.Security.MaxBodyBytes)
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) setSecurityHeaders(w http.ResponseWriter) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Eco-Version", "0.1.0")
	// HSTS is ideally set by Caddy; set here too for defense in depth when TLS is terminated elsewhere
	if s.cfg.Security.RequireTLS {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
	}
	// minimal CORS — no browser cross-origin by default
	w.Header().Set("Access-Control-Allow-Origin", "")
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

func (s *Server) handleRoot(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		// OpenAI-compatible paths without /v1
		if strings.HasPrefix(r.URL.Path, "/chat/") || strings.HasPrefix(r.URL.Path, "/models") ||
			strings.HasPrefix(r.URL.Path, "/completions") || strings.HasPrefix(r.URL.Path, "/embeddings") {
			s.handleAPI(w, r)
			return
		}
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	domain := s.cfg.Server.Domain
	if domain == "" {
		domain = "localhost"
	}
	_ = json.NewEncoder(w).Encode(map[string]string{
		"name":    "EcoRouter",
		"message": "LLM router — set base URL to this host and authenticate with Bearer token",
		"docs":    "https://github.com/ganjar/ecorouter",
		"domain":  domain,
	})
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	ip := ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))

	// IP access control
	if !s.access.Allowed(ip) {
		_ = s.db.InsertAudit("access_denied", "IP not allowed", ip, "")
		s.writeJSON(w, http.StatusForbidden, map[string]any{
			"error": map[string]string{"message": "forbidden", "type": "access_denied"},
		})
		return
	}

	// lockout
	if banned, rem := s.lockout.IsBanned(ip); banned {
		w.Header().Set("Retry-After", fmt.Sprintf("%d", int(rem.Seconds())+1))
		s.writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error": map[string]string{"message": "too many failed auth attempts", "type": "lockout"},
		})
		return
	}

	// authenticate
	tok, errMsg := s.authenticate(r)
	if tok == nil {
		banned, remaining := s.lockout.Fail(ip)
		_ = s.db.InsertAudit("auth_fail", errMsg, ip, "")
		if banned {
			_ = s.db.InsertAudit("lockout", "IP banned after repeated failures", ip, "")
			w.Header().Set("Retry-After", fmt.Sprintf("%d", int(s.lockout.banFor.Seconds())))
			s.writeJSON(w, http.StatusTooManyRequests, map[string]any{
				"error": map[string]string{"message": "too many failed auth attempts", "type": "lockout"},
			})
			return
		}
		// Client-facing auth errors are terse (PRD §13)
		_ = remaining
		s.writeJSON(w, http.StatusUnauthorized, map[string]any{
			"error": map[string]string{"message": "unauthorized", "type": "auth_error"},
		})
		return
	}
	s.lockout.Success(ip)
	// auth success is frequent; only audit failures + security events at info level would be noisy.
	// Keep a lightweight audit for first-use style events is optional; skip per-request auth_ok spam.

	// rate limit
	if !s.rl.Allow(tok.ID, tok.Rate) {
		w.Header().Set("Retry-After", "1")
		s.writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error": map[string]string{"message": "rate limit exceeded", "type": "rate_limit"},
		})
		_ = s.db.InsertAudit("rate_limit", "token rate exceeded", ip, tok.ID)
		return
	}

	// spend caps (daily USD)
	if blocked, code, msg := s.checkSpend(tok); blocked {
		w.Header().Set("Retry-After", "3600")
		s.writeJSON(w, code, map[string]any{
			"error": map[string]string{"message": msg, "type": "spend_cap"},
		})
		_ = s.db.InsertAudit("spend_cap", msg, ip, tok.ID)
		return
	}

	// concurrent request cap
	if !s.conc.Acquire(tok.ID, tok.MaxConcurrent) {
		w.Header().Set("Retry-After", "1")
		s.writeJSON(w, http.StatusTooManyRequests, map[string]any{
			"error": map[string]string{"message": "too many concurrent requests", "type": "concurrency_limit"},
		})
		_ = s.db.InsertAudit("concurrency_limit", "max concurrent exceeded", ip, tok.ID)
		return
	}
	defer s.conc.Release(tok.ID, tok.MaxConcurrent)

	// models list — local catalog
	if r.Method == http.MethodGet && (r.URL.Path == "/v1/models" || r.URL.Path == "/models") {
		s.handleModels(w, r, tok)
		return
	}

	s.handleProxy(w, r, tok)
}

// checkSpend returns blocked=true when daily caps are exceeded.
// Default action: 429 (retry later). Cap of 0 disables.
func (s *Server) checkSpend(tok *store.Token) (blocked bool, code int, msg string) {
	if tok.DailyCap > 0 {
		spent, err := s.db.DailySpend(tok.ID)
		if err == nil && spent >= tok.DailyCap {
			return true, http.StatusTooManyRequests, "token daily spend cap exceeded"
		}
	}
	if s.cfg.Security.GlobalDailyCap > 0 {
		spent, err := s.db.DailySpend("")
		if err == nil && spent >= s.cfg.Security.GlobalDailyCap {
			return true, http.StatusTooManyRequests, "global daily spend cap exceeded"
		}
	}
	return false, 0, ""
}

func (s *Server) authenticate(r *http.Request) (*store.Token, string) {
	h := r.Header.Get("Authorization")
	if h == "" {
		return nil, "missing Authorization header"
	}
	const prefix = "Bearer "
	if !strings.HasPrefix(h, prefix) {
		return nil, "Authorization must be Bearer token"
	}
	plain := strings.TrimSpace(h[len(prefix):])
	if plain == "" {
		return nil, "empty token"
	}
	tok, err := s.db.FindTokenByPlaintext(plain, auth.Verify)
	if err != nil {
		return nil, err.Error()
	}
	if tok == nil {
		return nil, "invalid token"
	}
	if tok.Revoked {
		return nil, "token revoked"
	}
	return tok, ""
}

func (s *Server) handleModels(w http.ResponseWriter, r *http.Request, tok *store.Token) {
	cfg := s.cfg.Clone()
	type modelObj struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		OwnedBy string `json:"owned_by"`
	}
	var data []modelObj
	for name, p := range cfg.Providers {
		for _, m := range p.Models {
			id := m
			if len(tok.ScopeModels) > 0 {
				allowed := false
				for _, sm := range tok.ScopeModels {
					if sm == m || sm == name+"/"+m {
						allowed = true
						break
					}
				}
				if !allowed {
					continue
				}
			}
			data = append(data, modelObj{ID: id, Object: "model", OwnedBy: name})
		}
	}
	// also include route models not in catalog
	for _, rc := range cfg.Routes {
		for _, m := range rc.Models {
			found := false
			for _, d := range data {
				if d.ID == m || strings.HasSuffix(m, "/"+d.ID) {
					found = true
					break
				}
			}
			if !found {
				data = append(data, modelObj{ID: m, Object: "model", OwnedBy: "route"})
			}
		}
	}
	s.writeJSON(w, http.StatusOK, map[string]any{"object": "list", "data": data})
}

func (s *Server) writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	s.setSecurityHeaders(w)
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// Run starts the HTTP server (blocking until signal or Stop).
func (s *Server) Run(detached bool) error {
	addr := s.http.Addr
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		if isAddrInUse(err) {
			return fmt.Errorf("port in use: %w", err)
		}
		return err
	}

	// write PID
	if err := writePID(config.PIDPath()); err != nil {
		s.logf.Printf("warn: could not write pid file: %v", err)
	}

	s.logf.Printf("eco daemon listening on %s (loopback)", addr)
	if s.cfg.Server.Domain != "" {
		s.logf.Printf("public domain: %s (TLS via reverse proxy)", s.cfg.Server.Domain)
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.http.Serve(ln)
	}()

	if detached {
		// still block — process is the daemon; caller may background the process
	}

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, interruptSignals...)

	select {
	case sig := <-sigCh:
		s.logf.Printf("received %v, shutting down", sig)
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			_ = os.Remove(config.PIDPath())
			return err
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = s.http.Shutdown(ctx)
	_ = os.Remove(config.PIDPath())
	s.logf.Printf("stopped")
	return nil
}

func (s *Server) Stop() error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	err := s.http.Shutdown(ctx)
	_ = os.Remove(config.PIDPath())
	return err
}

func writePID(path string) error {
	return os.WriteFile(path, []byte(strconv.Itoa(os.Getpid())+"\n"), 0o644)
}

// ReloadAccess rebuilds access control from config.
func (s *Server) ReloadAccess(cfg *config.Config) error {
	ac, err := NewAccess(cfg.Access)
	if err != nil {
		return err
	}
	s.mu.Lock()
	s.cfg = cfg
	s.access = ac
	s.mu.Unlock()
	return nil
}
