package handler_test

import (
	"sync"
	"testing"

	"github.com/finish06/cash-drugs/internal/handler"
)

func TestWarmupStateTracker_InitialState(t *testing.T) {
	s := handler.NewWarmupStateTracker()

	if s.IsReady() {
		t.Error("expected not ready initially")
	}
	done, total := s.Progress()
	if done != 0 || total != 0 {
		t.Errorf("expected 0/0, got %d/%d", done, total)
	}
	if s.Phase() != "" {
		t.Errorf("expected empty phase, got '%s'", s.Phase())
	}
}

func TestWarmupStateTracker_SetTotalAndIncrDone(t *testing.T) {
	s := handler.NewWarmupStateTracker()
	s.SetTotal(5)
	s.IncrDone()
	s.IncrDone()

	done, total := s.Progress()
	if done != 2 || total != 5 {
		t.Errorf("expected 2/5, got %d/%d", done, total)
	}
}

func TestWarmupStateTracker_PhaseTransitions(t *testing.T) {
	s := handler.NewWarmupStateTracker()

	s.SetPhase("scheduled")
	if s.Phase() != "scheduled" {
		t.Errorf("expected 'scheduled', got '%s'", s.Phase())
	}

	s.SetPhase("queries")
	if s.Phase() != "queries" {
		t.Errorf("expected 'queries', got '%s'", s.Phase())
	}

	s.MarkReady()
	if !s.IsReady() {
		t.Error("expected ready after MarkReady")
	}
	if s.Phase() != "ready" {
		t.Errorf("expected 'ready' phase, got '%s'", s.Phase())
	}
}

func TestWarmupStateTracker_Reset(t *testing.T) {
	s := handler.NewWarmupStateTracker()
	s.SetTotal(10)
	s.IncrDone()
	s.SetPhase("queries")
	s.MarkReady()

	s.Reset()

	if s.IsReady() {
		t.Error("expected not ready after reset")
	}
	done, total := s.Progress()
	if done != 0 || total != 0 {
		t.Errorf("expected 0/0 after reset, got %d/%d", done, total)
	}
	if s.Phase() != "" {
		t.Errorf("expected empty phase after reset, got '%s'", s.Phase())
	}
}

func TestWarmupStateTracker_ConcurrentAccess(t *testing.T) {
	s := handler.NewWarmupStateTracker()
	s.SetTotal(100)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			s.IncrDone()
			s.Progress()
			s.Phase()
			s.IsReady()
		}()
	}
	wg.Wait()

	done, total := s.Progress()
	if done != 100 {
		t.Errorf("expected 100 done after concurrent increments, got %d", done)
	}
	if total != 100 {
		t.Errorf("expected total 100, got %d", total)
	}
}
