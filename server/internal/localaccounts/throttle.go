package localaccounts

import "time"

const (
	maxLoginFailures   = 5
	loginFailureWindow = 15 * time.Minute
	loginLockDuration  = 15 * time.Minute
	maxThrottleEntries = 10000
)

type loginFailures struct {
	count       int
	windowStart time.Time
	lockedUntil time.Time
}

func (s *Service) loginAllowed(key string) bool {
	now := s.now()
	s.throttleMu.Lock()
	defer s.throttleMu.Unlock()

	entry, ok := s.failures[key]
	if !ok {
		return true
	}
	if !entry.lockedUntil.IsZero() {
		if now.Before(entry.lockedUntil) {
			return false
		}
		delete(s.failures, key)
		return true
	}
	if now.Sub(entry.windowStart) >= loginFailureWindow {
		delete(s.failures, key)
	}
	return true
}

func (s *Service) recordLoginFailure(key string) {
	now := s.now()
	s.throttleMu.Lock()
	defer s.throttleMu.Unlock()
	s.pruneThrottleLocked(now)

	entry := s.failures[key]
	if entry.windowStart.IsZero() || now.Sub(entry.windowStart) >= loginFailureWindow || (!entry.lockedUntil.IsZero() && !now.Before(entry.lockedUntil)) {
		entry = loginFailures{windowStart: now}
	}
	entry.count++
	if entry.count >= maxLoginFailures {
		entry.lockedUntil = now.Add(loginLockDuration)
	}
	s.failures[key] = entry
}

func (s *Service) clearLoginFailures(key string) {
	s.throttleMu.Lock()
	delete(s.failures, key)
	s.throttleMu.Unlock()
}

func (s *Service) pruneThrottleLocked(now time.Time) {
	for key, entry := range s.failures {
		if (!entry.lockedUntil.IsZero() && !now.Before(entry.lockedUntil)) ||
			(entry.lockedUntil.IsZero() && now.Sub(entry.windowStart) >= loginFailureWindow) {
			delete(s.failures, key)
		}
	}
	if len(s.failures) < maxThrottleEntries {
		return
	}

	// Bound memory use by evicting the oldest active window. Existing blocked
	// keys are retained so a traffic spike cannot unlock all accounts.
	oldestKey := ""
	var oldest time.Time
	for key, entry := range s.failures {
		if !entry.lockedUntil.IsZero() {
			continue
		}
		if oldestKey == "" || entry.windowStart.Before(oldest) {
			oldestKey, oldest = key, entry.windowStart
		}
	}
	if oldestKey != "" {
		delete(s.failures, oldestKey)
	}
}
