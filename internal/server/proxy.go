package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/cost"
	"github.com/ganjar/ecorouter/internal/router"
	"github.com/ganjar/ecorouter/internal/secrets"
	"github.com/ganjar/ecorouter/internal/store"
)

// forwardResult captures one upstream attempt.
type forwardResult struct {
	status     int
	body       []byte
	header     http.Header
	model      string
	provider   string
	via        string
	latency    time.Duration
	tokensIn   int
	tokensOut  int
	err        error
	streamed   bool
}

func (s *Server) handleProxy(w http.ResponseWriter, r *http.Request, tok *store.Token) {
	start := time.Now()
	cfg := s.cfg.Clone()

	routeName := cfg.Defaults.ActiveRoute
	if tok.ScopeRoute != "" {
		routeName = tok.ScopeRoute
	}
	// Optional header override for multi-route operators
	if h := r.Header.Get("X-Eco-Route"); h != "" {
		if tok.ScopeRoute == "" || tok.ScopeRoute == h {
			routeName = h
		}
	}

	decision, err := s.engine.Resolve(cfg, routeName)
	if err != nil {
		s.writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": map[string]string{"message": err.Error(), "type": "route_error"},
		})
		_ = s.db.InsertActivity(&store.Activity{
			TS: start, TokenID: tok.ID, TokenLabel: tok.Label, SrcIP: ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For")),
			Route: routeName, Status: http.StatusBadGateway, Error: err.Error(), LatencyMs: time.Since(start).Milliseconds(),
		})
		return
	}

	// Scope models
	if len(tok.ScopeModels) > 0 {
		allowed := map[string]bool{}
		for _, m := range tok.ScopeModels {
			allowed[m] = true
		}
		filtered := []string{}
		for _, m := range decision.Models {
			base := m
			if i := strings.LastIndex(m, "/"); i >= 0 {
				base = m[i+1:]
			}
			if allowed[m] || allowed[base] {
				filtered = append(filtered, m)
			}
		}
		if len(filtered) == 0 {
			s.writeJSON(w, http.StatusForbidden, map[string]any{
				"error": map[string]string{"message": "token not scoped for any model on this route", "type": "scope_error"},
			})
			return
		}
		decision.Models = filtered
		decision.Selected = filtered[0]
	}

	body, err := io.ReadAll(io.LimitReader(r.Body, cfg.Security.MaxBodyBytes+1))
	if err != nil {
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}
	if int64(len(body)) > cfg.Security.MaxBodyBytes {
		http.Error(w, "request body too large", http.StatusRequestEntityTooLarge)
		return
	}
	_ = r.Body.Close()

	// Detect streaming intent
	stream := false
	var reqObj map[string]any
	if len(body) > 0 && json.Unmarshal(body, &reqObj) == nil {
		if v, ok := reqObj["stream"].(bool); ok && v {
			stream = true
		}
	}

	path := r.URL.Path
	// Strip optional /v1 prefix handling: we keep path as-is for openai-compat
	query := r.URL.RawQuery

	var last *forwardResult
	for _, model := range decision.Models {
		// rewrite model in body
		fwdBody := rewriteModel(body, model)

		res := s.tryUpstream(r.Context(), cfg, decision, model, path, query, r.Method, r.Header, fwdBody, stream, w)
		last = res

		if res.streamed {
			// already written to client — no fallback
			s.logActivity(tok, r, decision.Route, res, start)
			return
		}

		if res.err == nil && !shouldFallback(res.status) {
			// success or non-retryable client error
			copyHeader(w.Header(), res.header)
			s.setSecurityHeaders(w)
			w.WriteHeader(res.status)
			_, _ = w.Write(res.body)
			s.logActivity(tok, r, decision.Route, res, start)
			return
		}

		// record failure for circuit breaker
		s.health.Record(model, false, res.latency)

		// fallback only for fallback mode with more candidates
		if decision.Mode != "fallback" {
			break
		}
	}

	// all failed
	if last == nil {
		s.writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": map[string]string{"message": "no upstream models available", "type": "upstream_error"},
		})
		return
	}
	if last.err != nil {
		s.writeJSON(w, http.StatusBadGateway, map[string]any{
			"error": map[string]string{"message": last.err.Error(), "type": "upstream_error"},
		})
		s.logActivity(tok, r, decision.Route, last, start)
		return
	}
	copyHeader(w.Header(), last.header)
	s.setSecurityHeaders(w)
	w.WriteHeader(last.status)
	_, _ = w.Write(last.body)
	s.logActivity(tok, r, decision.Route, last, start)
}

func shouldFallback(status int) bool {
	if status == 0 {
		return true
	}
	if status == http.StatusTooManyRequests {
		return true
	}
	if status >= 500 {
		return true
	}
	return false
}

func (s *Server) tryUpstream(
	ctx context.Context,
	cfg *config.Config,
	decision *router.Decision,
	model, path, query, method string,
	inHeader http.Header,
	body []byte,
	stream bool,
	clientW http.ResponseWriter,
) *forwardResult {
	start := time.Now()
	res := &forwardResult{model: model}

	prov, modelName, baseURL, pType, err := router.ResolveModelProvider(cfg, model)
	if err != nil {
		res.err = err
		res.latency = time.Since(start)
		return res
	}
	res.provider = prov
	// ensure body uses bare model name for upstream
	body = rewriteModel(body, modelName)

	targetBase := baseURL
	viaName := decision.Via
	if viaName != "" {
		saver, ok := cfg.Savers[viaName]
		if !ok || saver.URL == "" {
			if decision.ViaReq {
				res.err = fmt.Errorf("saver %q not configured", viaName)
				res.latency = time.Since(start)
				return res
			}
			// bypass
			viaName = ""
		} else {
			// probe saver quickly; on failure bypass unless required
			if err := probeURL(saver.URL, 2*time.Second); err != nil {
				if decision.ViaReq {
					res.err = fmt.Errorf("saver %q unreachable: %w", viaName, err)
					res.latency = time.Since(start)
					return res
				}
				viaName = ""
			} else {
				targetBase = saver.URL
				res.via = viaName
			}
		}
	}

	u, err := url.Parse(strings.TrimRight(targetBase, "/"))
	if err != nil {
		res.err = err
		res.latency = time.Since(start)
		return res
	}
	// join path
	full := strings.TrimRight(u.String(), "/") + path
	if query != "" {
		full += "?" + query
	}

	timeout := time.Duration(cfg.Server.TimeoutMs) * time.Millisecond
	if timeout <= 0 {
		timeout = 30 * time.Second
	}
	reqCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	req, err := http.NewRequestWithContext(reqCtx, method, full, bytes.NewReader(body))
	if err != nil {
		res.err = err
		res.latency = time.Since(start)
		return res
	}

	// copy safe headers
	for k, vs := range inHeader {
		lk := strings.ToLower(k)
		if lk == "host" || lk == "authorization" || lk == "content-length" {
			continue
		}
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	req.Header.Set("Content-Type", "application/json")

	// Provider API key — when via saver, still pass provider key if saver expects it
	key, ok := s.secrets.Get(prov)
	if ok && key != "" {
		switch strings.ToLower(pType) {
		case "anthropic":
			req.Header.Set("x-api-key", key)
			if req.Header.Get("anthropic-version") == "" {
				req.Header.Set("anthropic-version", "2023-06-01")
			}
		default:
			req.Header.Set("Authorization", "Bearer "+key)
		}
	}

	// When routing via saver, set original base for savers that need it
	if res.via != "" {
		req.Header.Set("X-Eco-Upstream", baseURL)
		req.Header.Set("X-Eco-Provider", prov)
	}

	client := &http.Client{
		Transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{Timeout: 10 * time.Second}).DialContext,
			// don't follow redirects blindly for API
		},
		Timeout: 0, // use context
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			return http.ErrUseLastResponse
		},
	}

	resp, err := client.Do(req)
	res.latency = time.Since(start)
	if err != nil {
		res.err = err
		res.status = 0
		return res
	}
	defer resp.Body.Close()

	res.status = resp.StatusCode
	res.header = resp.Header.Clone()

	ct := resp.Header.Get("Content-Type")
	isSSE := strings.Contains(ct, "text/event-stream") || stream

	if isSSE {
		// stream to client; no fallback after first byte
		copyHeader(clientW.Header(), resp.Header)
		s.setSecurityHeaders(clientW)
		clientW.Header().Set("X-Eco-Model", model)
		clientW.Header().Set("X-Eco-Route", decision.Route)
		if res.via != "" {
			clientW.Header().Set("X-Eco-Via", res.via)
		}
		clientW.WriteHeader(resp.StatusCode)
		res.streamed = true

		flusher, _ := clientW.(http.Flusher)
		buf := make([]byte, 32*1024)
		first := true
		for {
			n, readErr := resp.Body.Read(buf)
			if n > 0 {
				if first {
					first = false
					// first byte committed
				}
				if _, werr := clientW.Write(buf[:n]); werr != nil {
					break
				}
				if flusher != nil {
					flusher.Flush()
				}
			}
			if readErr != nil {
				break
			}
		}
		okStatus := resp.StatusCode < 500 && resp.StatusCode != 429
		s.health.Record(model, okStatus, res.latency)
		return res
	}

	res.body, err = io.ReadAll(io.LimitReader(resp.Body, cfg.Security.MaxBodyBytes))
	if err != nil {
		res.err = err
		return res
	}

	// parse usage if present
	res.tokensIn, res.tokensOut = parseUsage(res.body)

	okStatus := resp.StatusCode < 400
	s.health.Record(model, okStatus || (resp.StatusCode < 500 && resp.StatusCode != 429), res.latency)
	return res
}

func rewriteModel(body []byte, model string) []byte {
	if len(body) == 0 {
		return body
	}
	var obj map[string]any
	if err := json.Unmarshal(body, &obj); err != nil {
		return body
	}
	// strip provider/ prefix if present for upstream
	name := model
	if i := strings.LastIndex(model, "/"); i >= 0 {
		name = model[i+1:]
	}
	obj["model"] = name
	out, err := json.Marshal(obj)
	if err != nil {
		return body
	}
	return out
}

func parseUsage(body []byte) (in, out int) {
	var obj struct {
		Usage struct {
			PromptTokens     int `json:"prompt_tokens"`
			CompletionTokens int `json:"completion_tokens"`
			InputTokens      int `json:"input_tokens"`
			OutputTokens     int `json:"output_tokens"`
		} `json:"usage"`
	}
	if json.Unmarshal(body, &obj) != nil {
		return 0, 0
	}
	in = obj.Usage.PromptTokens
	out = obj.Usage.CompletionTokens
	if in == 0 {
		in = obj.Usage.InputTokens
	}
	if out == 0 {
		out = obj.Usage.OutputTokens
	}
	return in, out
}

func probeURL(raw string, timeout time.Duration) error {
	u, err := url.Parse(raw)
	if err != nil {
		return err
	}
	host := u.Host
	if host == "" {
		return fmt.Errorf("empty host")
	}
	// TCP dial only — savers may not have a health path
	d := net.Dialer{Timeout: timeout}
	conn, err := d.Dial("tcp", host)
	if err != nil {
		// try with default port
		if u.Port() == "" {
			port := "80"
			if u.Scheme == "https" {
				port = "443"
			}
			conn, err = d.Dial("tcp", net.JoinHostPort(u.Hostname(), port))
		}
		if err != nil {
			return err
		}
	}
	_ = conn.Close()
	return nil
}

func copyHeader(dst, src http.Header) {
	for k, vs := range src {
		// skip hop-by-hop
		lk := strings.ToLower(k)
		if lk == "connection" || lk == "transfer-encoding" || lk == "keep-alive" {
			continue
		}
		for _, v := range vs {
			dst.Add(k, v)
		}
	}
}

func (s *Server) logActivity(tok *store.Token, r *http.Request, route string, res *forwardResult, start time.Time) {
	ip := ClientIP(r.RemoteAddr, r.Header.Get("X-Forwarded-For"))
	errStr := ""
	if res.err != nil {
		errStr = res.err.Error()
	}
	est := cost.Estimate(res.model, res.tokensIn, res.tokensOut)
	_ = s.db.InsertActivity(&store.Activity{
		TS:           start,
		TokenID:      tok.ID,
		TokenLabel:   tok.Label,
		SrcIP:        ip,
		Route:        route,
		Model:        res.model,
		Provider:     res.provider,
		Via:          res.via,
		TokensIn:     res.tokensIn,
		TokensOut:    res.tokensOut,
		LatencyMs:    time.Since(start).Milliseconds(),
		Status:       res.status,
		CostEstimate: est,
		Error:        errStr,
	})
	_ = s.db.TouchToken(tok.ID, ip)
}

// Ensure secrets package is referenced for typing in Server
var _ = secrets.Store{}
