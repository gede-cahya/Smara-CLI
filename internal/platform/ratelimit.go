package platform

import (
	"sync"
	"time"
)

// RateLimiter implements a simple per-user rate limiter using a sliding window.
// No external dependencies — uses a map of timestamps per user.
type RateLimiter struct {
	mu              sync.Mutex
	maxPerMinute    int
	burstSize       int
	userTimestamps  map[string][]time.Time // userID → list of request timestamps
}

// NewRateLimiter creates a new rate limiter with the given configuration.
// If maxPerMinute is 0, rate limiting is disabled (all requests allowed).
func NewRateLimiter(cfg RateLimitConfig) *RateLimiter {
	maxPerMin := cfg.RequestsPerMinute
	if maxPerMin <= 0 {
		maxPerMin = 0 // disabled
	}
	burst := cfg.BurstSize
	if burst <= 0 {
		burst = maxPerMin // default burst = max per minute
	}
	return &RateLimiter{
		maxPerMinute:   maxPerMin,
		burstSize:      burst,
		userTimestamps: make(map[string][]time.Time),
	}
}

// Allow checks if a user is within their rate limit.
// Returns true if the request is allowed, false if rate limited.
func (r *RateLimiter) Allow(userID string) bool {
	if r.maxPerMinute == 0 {
		return true // rate limiting disabled
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now()
	windowStart := now.Add(-time.Minute)

	// Get existing timestamps and filter to current window
	timestamps := r.userTimestamps[userID]
	var active []time.Time
	for _, t := range timestamps {
		if t.After(windowStart) {
			active = append(active, t)
		}
	}

	// Check if over limit
	if len(active) >= r.maxPerMinute {
		r.userTimestamps[userID] = active
		return false
	}

	// Allow and record
	active = append(active, now)
	r.userTimestamps[userID] = active
	return true
}

// Reset clears rate limit data for a specific user.
func (r *RateLimiter) Reset(userID string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.userTimestamps, userID)
}

// Cleanup removes expired entries to prevent memory leaks.
// Should be called periodically (e.g., every 5 minutes).
func (r *RateLimiter) Cleanup() {
	r.mu.Lock()
	defer r.mu.Unlock()

	windowStart := time.Now().Add(-time.Minute)
	for userID, timestamps := range r.userTimestamps {
		var active []time.Time
		for _, t := range timestamps {
			if t.After(windowStart) {
				active = append(active, t)
			}
		}
		if len(active) == 0 {
			delete(r.userTimestamps, userID)
		} else {
			r.userTimestamps[userID] = active
		}
	}
}
