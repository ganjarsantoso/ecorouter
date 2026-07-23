package health

import (
	"sync"
	"time"
)

// Tracker maintains per-model rolling windows and circuit-breaker state.
type Tracker struct {
	mu      sync.RWMutex
	models  map[string]*modelState
	window  int
	errRate float64
	minReq  int
	cool    time.Duration
}

type modelState struct {
	results   []bool // true = success
	latencies []time.Duration
	brokenUntil time.Time
	reason    string
}

type Snapshot struct {
	Model       string  `json:"model"`
	Healthy     bool    `json:"healthy"`
	Broken      bool    `json:"broken"`
	ErrorRate   float64 `json:"error_rate"`
	P50Latency  float64 `json:"p50_latency_ms"`
	Samples     int     `json:"samples"`
	BrokenUntil string  `json:"broken_until,omitempty"`
	Reason      string  `json:"reason,omitempty"`
}

func New(window int, errThreshold float64, minReq int, cooldownMs int) *Tracker {
	if window <= 0 {
		window = 20
	}
	if minReq <= 0 {
		minReq = 5
	}
	if cooldownMs <= 0 {
		cooldownMs = 60000
	}
	if errThreshold <= 0 {
		errThreshold = 0.5
	}
	return &Tracker{
		models:  map[string]*modelState{},
		window:  window,
		errRate: errThreshold,
		minReq:  minReq,
		cool:    time.Duration(cooldownMs) * time.Millisecond,
	}
}

func (t *Tracker) state(model string) *modelState {
	s, ok := t.models[model]
	if !ok {
		s = &modelState{}
		t.models[model] = s
	}
	return s
}

// Record notes a completed upstream attempt.
func (t *Tracker) Record(model string, success bool, latency time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	s := t.state(model)
	s.results = append(s.results, success)
	s.latencies = append(s.latencies, latency)
	if len(s.results) > t.window {
		s.results = s.results[len(s.results)-t.window:]
		s.latencies = s.latencies[len(s.latencies)-t.window:]
	}
	if len(s.results) >= t.minReq {
		fails := 0
		for _, ok := range s.results {
			if !ok {
				fails++
			}
		}
		rate := float64(fails) / float64(len(s.results))
		if rate >= t.errRate {
			s.brokenUntil = time.Now().Add(t.cool)
			s.reason = "error rate exceeded threshold"
		}
	}
	if success && time.Now().After(s.brokenUntil) {
		// half-open success clears break
		s.brokenUntil = time.Time{}
		s.reason = ""
	}
}

// IsBroken reports if the model is currently circuit-broken.
func (t *Tracker) IsBroken(model string) (bool, string) {
	t.mu.RLock()
	defer t.mu.RUnlock()
	s, ok := t.models[model]
	if !ok {
		return false, ""
	}
	if time.Now().Before(s.brokenUntil) {
		return true, s.reason
	}
	return false, ""
}

// Snapshots returns health for all known models.
func (t *Tracker) Snapshots() []Snapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Snapshot, 0, len(t.models))
	now := time.Now()
	for name, s := range t.models {
		fails := 0
		for _, ok := range s.results {
			if !ok {
				fails++
			}
		}
		n := len(s.results)
		rate := 0.0
		if n > 0 {
			rate = float64(fails) / float64(n)
		}
		broken := now.Before(s.brokenUntil)
		snap := Snapshot{
			Model:      name,
			Healthy:    !broken && rate < t.errRate,
			Broken:     broken,
			ErrorRate:  rate,
			P50Latency: p50(s.latencies),
			Samples:    n,
			Reason:     s.reason,
		}
		if broken {
			snap.BrokenUntil = s.brokenUntil.UTC().Format(time.RFC3339)
		}
		out = append(out, snap)
	}
	return out
}

func p50(ds []time.Duration) float64 {
	if len(ds) == 0 {
		return 0
	}
	// copy + simple insertion sort for small windows
	cp := append([]time.Duration{}, ds...)
	for i := 1; i < len(cp); i++ {
		j := i
		for j > 0 && cp[j] < cp[j-1] {
			cp[j], cp[j-1] = cp[j-1], cp[j]
			j--
		}
	}
	return float64(cp[len(cp)/2].Milliseconds())
}
