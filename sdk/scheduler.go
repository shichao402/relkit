package sdk

import (
	"context"
	"sync"
	"time"
)

// UpdateScheduler runs periodic CheckForce calls in the background.
type UpdateScheduler struct {
	Runtime  RuntimeConfig
	Check    func(ctx context.Context, force bool) CheckResult
	OnResult func(CheckResult)

	mu         sync.Mutex
	running    bool
	generation int
	timer      *time.Timer
	stopCh     chan struct{}
}

// Start begins the schedule. Safe to call once; subsequent calls are no-ops while running.
func (s *UpdateScheduler) Start() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.running {
		return
	}
	s.running = true
	s.stopCh = make(chan struct{})
	force := s.Runtime.ForceOnStart
	if s.Runtime.CheckOnStart || force {
		go s.tick(force)
	} else {
		s.armLocked(s.Runtime.Policy.normalizedSuccess())
	}
}

// Stop cancels the schedule and discards in-flight results.
func (s *UpdateScheduler) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running {
		return
	}
	s.running = false
	s.generation++
	if s.timer != nil {
		s.timer.Stop()
		s.timer = nil
	}
	if s.stopCh != nil {
		close(s.stopCh)
		s.stopCh = nil
	}
}

// IsRunning reports whether the scheduler is active.
func (s *UpdateScheduler) IsRunning() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.running
}

func (s *UpdateScheduler) tick(force bool) {
	s.mu.Lock()
	gen := s.generation
	check := s.Check
	onResult := s.OnResult
	policy := s.Runtime.Policy
	if policy.AfterSuccess <= 0 || policy.AfterFailure <= 0 {
		policy = DefaultPolicy()
		if s.Runtime.Policy.AfterSuccess > 0 {
			policy.AfterSuccess = s.Runtime.Policy.AfterSuccess
		}
		if s.Runtime.Policy.AfterFailure > 0 {
			policy.AfterFailure = s.Runtime.Policy.AfterFailure
		}
	}
	s.mu.Unlock()

	if check == nil {
		return
	}
	result := check(context.Background(), force)

	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.running || gen != s.generation {
		return
	}
	if result.Throttled {
		delay := time.Until(result.NextAllowedAt)
		if delay < time.Second {
			delay = time.Second
		}
		s.armLocked(delay)
		return
	}
	if onResult != nil {
		onResult(result)
	}
	switch {
	case result.Err != nil:
		s.armLocked(policy.normalizedFailure())
	default:
		s.armLocked(policy.normalizedSuccess())
	}
}

func (s *UpdateScheduler) armLocked(d time.Duration) {
	if s.timer != nil {
		s.timer.Stop()
	}
	gen := s.generation
	s.timer = time.AfterFunc(d, func() {
		s.mu.Lock()
		ok := s.running && s.generation == gen
		s.mu.Unlock()
		if ok {
			s.tick(false)
		}
	})
}
