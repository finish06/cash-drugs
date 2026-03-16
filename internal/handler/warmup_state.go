package handler

import "sync"

// WarmupStateTracker is a thread-safe implementation of the WarmupState interface.
// It tracks warmup progress through phases: "scheduled", "queries", "ready".
type WarmupStateTracker struct {
	mu    sync.Mutex
	done  int
	total int
	phase string
	ready bool
}

// NewWarmupStateTracker creates a new WarmupStateTracker in the initial state.
func NewWarmupStateTracker() *WarmupStateTracker {
	return &WarmupStateTracker{}
}

// IsReady returns true when warm-up is complete.
func (s *WarmupStateTracker) IsReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.ready
}

// Progress returns the number of completed and total items in the current warm-up.
func (s *WarmupStateTracker) Progress() (done, total int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.done, s.total
}

// Phase returns the current warmup phase.
func (s *WarmupStateTracker) Phase() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.phase
}

// SetTotal sets the total item count for progress tracking.
func (s *WarmupStateTracker) SetTotal(n int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.total = n
}

// IncrDone increments the completed item counter by one.
func (s *WarmupStateTracker) IncrDone() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done++
}

// SetPhase sets the current warmup phase (e.g., "scheduled", "queries", "ready").
func (s *WarmupStateTracker) SetPhase(phase string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.phase = phase
}

// MarkReady marks the warmup as complete.
func (s *WarmupStateTracker) MarkReady() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.ready = true
	s.phase = "ready"
}

// Reset resets the tracker for a new warmup cycle.
func (s *WarmupStateTracker) Reset() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.done = 0
	s.total = 0
	s.phase = ""
	s.ready = false
}
