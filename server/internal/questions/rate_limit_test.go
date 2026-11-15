package questions

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestFixedWindowLimiterIsAtomic(t *testing.T) {
	limiter := newFixedWindowLimiter(25, time.Minute)
	now := time.Date(2026, time.November, 5, 12, 0, 0, 0, time.UTC)
	var allowed atomic.Int32
	var wg sync.WaitGroup
	for range 100 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if limiter.allow("203.0.113.80", now) {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if got := allowed.Load(); got != 25 {
		t.Fatalf("allowed requests = %d, want exactly 25", got)
	}
}
