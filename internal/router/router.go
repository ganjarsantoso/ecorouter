package router

import (
	"fmt"
	"strings"
	"sync"

	"github.com/ganjar/ecorouter/internal/config"
	"github.com/ganjar/ecorouter/internal/health"
)

// Decision is the result of route selection for one request.
type Decision struct {
	Route    string   `json:"route"`
	Mode     string   `json:"mode"`
	Models   []string `json:"models"` // ordered candidates
	Selected string   `json:"selected"`
	Via      string   `json:"via,omitempty"`
	ViaReq   bool     `json:"via_required"`
	Reason   string   `json:"reason"`
}

// Engine picks models for routes.
type Engine struct {
	mu       sync.Mutex
	counters map[string]uint64 // route name -> round-robin counter
	health   *health.Tracker
}

func New(h *health.Tracker) *Engine {
	return &Engine{
		counters: map[string]uint64{},
		health:   h,
	}
}

// Resolve builds an ordered candidate list for a route.
// For single/round, Selected is the primary pick; for fallback, Models is the try-order.
func (e *Engine) Resolve(cfg *config.Config, routeName string) (*Decision, error) {
	if routeName == "" {
		routeName = cfg.Defaults.ActiveRoute
	}
	if routeName == "" {
		return nil, fmt.Errorf("no active route; run: eco use <route>")
	}
	rc, ok := cfg.Routes[routeName]
	if !ok {
		return nil, fmt.Errorf("route %q not found", routeName)
	}
	if len(rc.Models) == 0 {
		return nil, fmt.Errorf("route %q has no models", routeName)
	}

	via := ""
	viaReq := rc.ViaRequired
	if !rc.NoVia {
		if rc.Via != "" {
			via = rc.Via
		} else if cfg.Defaults.SaverDefault != "" {
			via = cfg.Defaults.SaverDefault
		}
	}

	d := &Decision{
		Route:  routeName,
		Mode:   rc.Mode,
		Via:    via,
		ViaReq: viaReq,
	}

	switch strings.ToLower(rc.Mode) {
	case "single":
		m := rc.Models[0]
		if broken, reason := e.health.IsBroken(m); broken {
			d.Models = []string{m}
			d.Selected = m
			d.Reason = fmt.Sprintf("single model %s is circuit-broken: %s", m, reason)
			return d, nil
		}
		d.Models = []string{m}
		d.Selected = m
		d.Reason = fmt.Sprintf("single mode → %s", m)
		return d, nil

	case "fallback":
		candidates := e.skipBroken(rc.Models)
		if len(candidates) == 0 {
			// all broken — still return full list so proxy can try / report
			candidates = append([]string{}, rc.Models...)
			d.Reason = "fallback: all models circuit-broken; trying in order"
		} else {
			d.Reason = fmt.Sprintf("fallback order: %s", strings.Join(candidates, " → "))
		}
		d.Models = candidates
		d.Selected = candidates[0]
		return d, nil

	case "round":
		e.mu.Lock()
		e.counters[routeName]++
		n := e.counters[routeName]
		e.mu.Unlock()

		// advance through list until healthy or wrap
		start := int((n - 1) % uint64(len(rc.Models)))
		chosen := ""
		for i := 0; i < len(rc.Models); i++ {
			idx := (start + i) % len(rc.Models)
			m := rc.Models[idx]
			if broken, _ := e.health.IsBroken(m); !broken {
				chosen = m
				break
			}
		}
		if chosen == "" {
			chosen = rc.Models[start]
			d.Reason = fmt.Sprintf("round-robin #%d → %s (all broken, no skip)", n, chosen)
		} else {
			d.Reason = fmt.Sprintf("round-robin #%d → %s", n, chosen)
		}
		d.Models = []string{chosen}
		d.Selected = chosen
		return d, nil

	default:
		return nil, fmt.Errorf("unknown route mode %q", rc.Mode)
	}
}

func (e *Engine) skipBroken(models []string) []string {
	var out []string
	for _, m := range models {
		if broken, _ := e.health.IsBroken(m); !broken {
			out = append(out, m)
		}
	}
	return out
}

// PeekRound does not advance the counter — used by route test.
func (e *Engine) PeekRound(routeName string, models []string) string {
	e.mu.Lock()
	n := e.counters[routeName]
	e.mu.Unlock()
	if len(models) == 0 {
		return ""
	}
	// next selection would use n+1
	idx := int(n % uint64(len(models)))
	for i := 0; i < len(models); i++ {
		m := models[(idx+i)%len(models)]
		if broken, _ := e.health.IsBroken(m); !broken {
			return m
		}
	}
	return models[idx]
}

// Counter returns current counter for tests/status.
func (e *Engine) Counter(route string) uint64 {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.counters[route]
}

// ResolveModelProvider finds which provider owns a model name.
// Convention: model may be "provider/model" or bare name matched against provider catalogs.
func ResolveModelProvider(cfg *config.Config, model string) (provider, modelName, baseURL, pType string, err error) {
	if strings.Contains(model, "/") {
		parts := strings.SplitN(model, "/", 2)
		p, ok := cfg.Providers[parts[0]]
		if !ok {
			return "", "", "", "", fmt.Errorf("provider %q not found for model %s", parts[0], model)
		}
		return parts[0], parts[1], p.BaseURL, p.Type, nil
	}
	// bare name: search catalogs
	for name, p := range cfg.Providers {
		for _, m := range p.Models {
			if m == model {
				return name, model, p.BaseURL, p.Type, nil
			}
		}
	}
	// fallback: if only one provider, use it
	if len(cfg.Providers) == 1 {
		for name, p := range cfg.Providers {
			return name, model, p.BaseURL, p.Type, nil
		}
	}
	// last resort: try openai-ish default
	if p, ok := cfg.Providers["openai"]; ok {
		return "openai", model, p.BaseURL, p.Type, nil
	}
	return "", "", "", "", fmt.Errorf("cannot resolve provider for model %q; use provider/model form", model)
}
