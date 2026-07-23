package server

import (
	"net"
	"sync"
	"time"

	"github.com/ganjar/ecorouter/internal/config"
	"golang.org/x/time/rate"
)

// RateLimiter provides per-token and global token-bucket limits.
type RateLimiter struct {
	mu           sync.Mutex
	global       *rate.Limiter
	tokens       map[string]*rate.Limiter
	parse        func(string) (float64, int, error)
	defaultRate  string
}

func NewRateLimiter(globalRate, defaultTokenRate string) *RateLimiter {
	rl := &RateLimiter{
		tokens:      map[string]*rate.Limiter{},
		parse:       config.ParseRate,
		defaultRate: defaultTokenRate,
	}
	if lim, burst, err := config.ParseRate(globalRate); err == nil && lim > 0 {
		if burst < 1 {
			burst = 1
		}
		rl.global = rate.NewLimiter(rate.Limit(lim), burst)
	}
	return rl
}

func (rl *RateLimiter) Allow(tokenID, tokenRate string) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	if rl.global != nil && !rl.global.Allow() {
		return false
	}
	r := tokenRate
	if r == "" {
		r = rl.defaultRate
	}
	if r == "" {
		return true
	}
	lim, ok := rl.tokens[tokenID]
	if !ok {
		rateVal, burst, err := rl.parse(r)
		if err != nil || rateVal <= 0 {
			return true
		}
		if burst < 1 {
			burst = 1
		}
		lim = rate.NewLimiter(rate.Limit(rateVal), burst)
		rl.tokens[tokenID] = lim
	}
	return lim.Allow()
}

// Lockout tracks auth failures per IP with exponential-ish ban.
type Lockout struct {
	mu       sync.Mutex
	fails    map[string]*failState
	maxFails int
	window   time.Duration
	banFor   time.Duration
}

type failState struct {
	count    int
	first    time.Time
	bannedUntil time.Time
}

// ParseLockout parses "5/1m -> 15m".
func ParseLockout(s string) (maxFails int, window, ban time.Duration) {
	maxFails, window, ban = 5, time.Minute, 15*time.Minute
	// simple parse
	var n int
	var w, b string
	if _, err := parseLockoutParts(s, &n, &w, &b); err == nil {
		maxFails = n
		if d, err := time.ParseDuration(normalizeDur(w)); err == nil {
			window = d
		}
		if d, err := time.ParseDuration(normalizeDur(b)); err == nil {
			ban = d
		}
	}
	return
}

func parseLockoutParts(s string, n *int, w, b *string) (int, error) {
	// format: N/W -> B
	var left, right string
	parts := splitArrow(s)
	if len(parts) != 2 {
		return 0, errParse
	}
	left, right = parts[0], parts[1]
	slash := -1
	for i, c := range left {
		if c == '/' {
			slash = i
			break
		}
	}
	if slash < 0 {
		return 0, errParse
	}
	if _, err := parseInt(left[:slash], n); err != nil {
		return 0, err
	}
	*w = left[slash+1:]
	*b = right
	return 0, nil
}

var errParse = errString("parse")

type errString string

func (e errString) Error() string { return string(e) }

func splitArrow(s string) []string {
	for i := 0; i+1 < len(s); i++ {
		if s[i] == '-' && s[i+1] == '>' {
			return []string{trim(s[:i]), trim(s[i+2:])}
		}
	}
	return nil
}

func trim(s string) string {
	for len(s) > 0 && (s[0] == ' ' || s[0] == '\t') {
		s = s[1:]
	}
	for len(s) > 0 && (s[len(s)-1] == ' ' || s[len(s)-1] == '\t') {
		s = s[:len(s)-1]
	}
	return s
}

func parseInt(s string, n *int) (int, error) {
	v := 0
	if s == "" {
		return 0, errParse
	}
	for _, c := range s {
		if c < '0' || c > '9' {
			return 0, errParse
		}
		v = v*10 + int(c-'0')
	}
	*n = v
	return v, nil
}

func normalizeDur(s string) string {
	s = trim(s)
	// allow 1m, 15m, 1h
	if len(s) > 0 {
		return s
	}
	return "1m"
}

func NewLockout(spec string) *Lockout {
	max, win, ban := ParseLockout(spec)
	return &Lockout{
		fails:    map[string]*failState{},
		maxFails: max,
		window:   win,
		banFor:   ban,
	}
}

func (l *Lockout) IsBanned(ip string) (bool, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	s, ok := l.fails[ip]
	if !ok {
		return false, 0
	}
	if time.Now().Before(s.bannedUntil) {
		return true, time.Until(s.bannedUntil)
	}
	return false, 0
}

func (l *Lockout) Fail(ip string) (banned bool, remaining int) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	s, ok := l.fails[ip]
	if !ok || now.Sub(s.first) > l.window {
		s = &failState{count: 1, first: now}
		l.fails[ip] = s
		return false, l.maxFails - 1
	}
	s.count++
	if s.count >= l.maxFails {
		s.bannedUntil = now.Add(l.banFor)
		return true, 0
	}
	return false, l.maxFails - s.count
}

func (l *Lockout) Success(ip string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, ip)
}

// ClientIP extracts the real client IP (honors X-Forwarded-For from Caddy).
func ClientIP(remoteAddr, xff string) string {
	if xff != "" {
		// first hop
		for i := 0; i < len(xff); i++ {
			if xff[i] == ',' {
				return trim(xff[:i])
			}
		}
		return trim(xff)
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
