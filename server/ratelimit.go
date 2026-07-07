package main

import (
	"sync"
	"time"
)

// rateLimiter is a small in-memory sliding-window limiter keyed by an arbitrary
// string (a user ID for action endpoints, a source IP for the webhook). It is
// per-plugin-node — good enough to blunt abuse and accidental floods without a
// shared store. Empty buckets are dropped as they age out, so memory stays
// bounded by the number of currently-active keys.
type rateLimiter struct {
	mu     sync.Mutex
	hits   map[string][]int64
	limit  int
	window time.Duration
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{
		hits:   make(map[string][]int64),
		limit:  limit,
		window: window,
	}
}

// allow records a hit for key and reports whether it is within the limit.
func (rl *rateLimiter) allow(key string) bool {
	if rl == nil {
		return true
	}
	now := time.Now().UnixNano()
	cutoff := now - rl.window.Nanoseconds()

	rl.mu.Lock()
	defer rl.mu.Unlock()

	recent := rl.hits[key][:0]
	for _, ts := range rl.hits[key] {
		if ts > cutoff {
			recent = append(recent, ts)
		}
	}
	if len(recent) >= rl.limit {
		rl.hits[key] = recent
		return false
	}
	rl.hits[key] = append(recent, now)
	return true
}
