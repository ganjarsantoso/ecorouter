package server

import "sync"

// Concurrency tracks in-flight requests per token.
type Concurrency struct {
	mu    sync.Mutex
	inflight map[string]int
}

func NewConcurrency() *Concurrency {
	return &Concurrency{inflight: map[string]int{}}
}

// Acquire returns false if the token is at its concurrent cap (cap<=0 means unlimited).
func (c *Concurrency) Acquire(tokenID string, cap int) bool {
	if cap <= 0 {
		return true
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inflight[tokenID] >= cap {
		return false
	}
	c.inflight[tokenID]++
	return true
}

func (c *Concurrency) Release(tokenID string, cap int) {
	if cap <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.inflight[tokenID] > 0 {
		c.inflight[tokenID]--
	}
}
