package guard

import (
	"sync"

	"github.com/danielgtaylor/huma/v2"
)

// Limiter gives the SAM AI endpoints the same shape of protection as the main
// chat: one in-flight request per user and a small global ceiling.
type Limiter struct {
	mu     sync.Mutex
	active map[string]bool
	slots  chan struct{}
}

func NewLimiter(global, perUser int) *Limiter {
	if global < 1 {
		global = 1
	}
	if perUser < 1 {
		perUser = 1
	}
	return &Limiter{active: make(map[string]bool), slots: make(chan struct{}, global)}
}

func (l *Limiter) Acquire(username string) (func(), error) {
	if l == nil {
		return func() {}, nil
	}
	l.mu.Lock()
	if l.active[username] {
		l.mu.Unlock()
		return nil, huma.Error429TooManyRequests("chat.busy")
	}
	l.active[username] = true
	l.mu.Unlock()
	select {
	case l.slots <- struct{}{}:
		return func() {
			<-l.slots
			l.mu.Lock()
			delete(l.active, username)
			l.mu.Unlock()
		}, nil
	default:
		l.mu.Lock()
		delete(l.active, username)
		l.mu.Unlock()
		return nil, huma.Error429TooManyRequests("chat.busy")
	}
}
