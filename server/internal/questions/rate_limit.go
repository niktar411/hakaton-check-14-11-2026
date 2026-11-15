package questions

import (
	"sync"
	"time"
)

type rateWindow struct {
	startedAt time.Time
	count     int
}

type fixedWindowLimiter struct {
	mu          sync.Mutex
	windows     map[string]rateWindow
	limit       int
	window      time.Duration
	lastCleanup time.Time
}

func newFixedWindowLimiter(limit int, window time.Duration) *fixedWindowLimiter {
	return &fixedWindowLimiter{
		windows: make(map[string]rateWindow),
		limit:   limit,
		window:  window,
	}
}

func (l *fixedWindowLimiter) allow(ip string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	if l.lastCleanup.IsZero() || now.Before(l.lastCleanup) || now.Sub(l.lastCleanup) >= l.window {
		for key, candidate := range l.windows {
			if now.Before(candidate.startedAt) || now.Sub(candidate.startedAt) >= l.window {
				delete(l.windows, key)
			}
		}
		l.lastCleanup = now
	}

	current, ok := l.windows[ip]
	if !ok || now.Before(current.startedAt) || now.Sub(current.startedAt) >= l.window {
		current = rateWindow{startedAt: now}
	}
	if current.count >= l.limit {
		return false
	}
	current.count++
	l.windows[ip] = current
	return true
}
