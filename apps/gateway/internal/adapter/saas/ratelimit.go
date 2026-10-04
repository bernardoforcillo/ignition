package saas

import (
	"context"
	"sync"
	"time"
)

// memoryLimiter is a fixed-window counter satisfying identity's
// auth.RateLimiter. It is per process: with N replicas the effective
// budget is N times the configured one. That is the smallest mechanism
// that gives sign-up and login a brute-force ceiling with no new backing
// service; swap in a shared limiter (Redis) when replicas matter.
type memoryLimiter struct {
	now func() time.Time

	mu        sync.Mutex
	windows   map[string]*window
	nextSweep time.Time
}

type window struct {
	count int64
	ends  time.Time
}

const sweepEvery = time.Minute

func newMemoryLimiter(now func() time.Time) *memoryLimiter {
	return &memoryLimiter{now: now, windows: make(map[string]*window)}
}

// Allow counts one request under key and reports whether it fits in limit
// per window. It never fails.
func (l *memoryLimiter) Allow(_ context.Context, key string, limit int64, per time.Duration) (bool, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.sweep(now)

	w := l.windows[key]
	if w == nil || !now.Before(w.ends) {
		w = &window{ends: now.Add(per)}
		l.windows[key] = w
	}
	if w.count >= limit {
		return false, nil
	}
	w.count++
	return true, nil
}

// sweep drops expired windows so idle keys do not accumulate.
func (l *memoryLimiter) sweep(now time.Time) {
	if now.Before(l.nextSweep) {
		return
	}
	for k, w := range l.windows {
		if !now.Before(w.ends) {
			delete(l.windows, k)
		}
	}
	l.nextSweep = now.Add(sweepEvery)
}
