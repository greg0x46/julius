package auth

import (
	"sync"
	"time"
)

const (
	limiterWindow  = time.Minute
	maxLimiterKeys = 1024
)

// Limiter is a fixed one-minute window with an overall budget and an optional
// per-key budget, in bounded memory. Blocked only checks and Record only
// counts, so a caller can spend budget on failed credential checks alone;
// Take checks and counts in one step for plain request limits.
type Limiter struct {
	// Now defaults to time.Now.
	Now           func() time.Time
	perKey, total int

	mu    sync.Mutex
	start time.Time
	count int
	keys  map[string]int
}

// NewLimiter returns a limiter with the given budgets per window; a value of
// zero or less disables that budget.
func NewLimiter(perKey, total int) *Limiter {
	return &Limiter{perKey: perKey, total: total}
}

// roll starts a new window once the current one has elapsed.
func (l *Limiter) roll() {
	now := time.Now
	if l.Now != nil {
		now = l.Now
	}
	if t := now(); l.start.IsZero() || t.Sub(l.start) >= limiterWindow {
		l.start, l.count = t, 0
		clear(l.keys)
	}
}

func (l *Limiter) blocked(key string) bool {
	return (l.total > 0 && l.count >= l.total) || (l.perKey > 0 && l.keys[key] >= l.perKey)
}

func (l *Limiter) record(key string) {
	l.count++
	if l.perKey <= 0 {
		return
	}
	if l.keys == nil {
		l.keys = make(map[string]int)
	}
	if _, seen := l.keys[key]; seen || len(l.keys) < maxLimiterKeys {
		l.keys[key]++
	}
}

// Blocked reports whether a budget for key is spent, without counting.
func (l *Limiter) Blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.roll()
	return l.blocked(key)
}

// Record counts one event for key.
func (l *Limiter) Record(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.roll()
	l.record(key)
}

// Take counts one event for key unless a budget is spent, and reports whether
// it fit.
func (l *Limiter) Take(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.roll()
	if l.blocked(key) {
		return false
	}
	l.record(key)
	return true
}
