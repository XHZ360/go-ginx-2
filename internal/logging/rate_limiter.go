package logging

import (
	"sync"
	"time"
)

type RateLimiter struct {
	mu       sync.Mutex
	interval time.Duration
	last     time.Time
}

func NewRateLimiter(interval time.Duration) *RateLimiter {
	return &RateLimiter{interval: interval}
}

func (limiter *RateLimiter) Allow() bool {
	limiter.mu.Lock()
	defer limiter.mu.Unlock()

	now := time.Now()
	if limiter.last.IsZero() || now.Sub(limiter.last) >= limiter.interval {
		limiter.last = now
		return true
	}
	return false
}
