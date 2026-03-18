package upstream_test

import (
	"errors"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/upstream"
)

// AC-001: Per-slug circuit isolation — different slugs have independent circuits
func TestAC001_PerSlugCircuitIsolation(t *testing.T) {
	reg := upstream.NewCircuitRegistry(2, 30*time.Second)

	// Trip circuit for slug-a by causing 2 consecutive failures
	for i := 0; i < 2; i++ {
		_, _ = reg.Execute("slug-a", func() (interface{}, error) {
			return nil, errors.New("fail")
		})
	}

	// slug-a should be open
	if !reg.IsOpen("slug-a") {
		t.Error("expected slug-a circuit to be open after 2 failures")
	}

	// slug-b should still be closed (independent)
	if reg.IsOpen("slug-b") {
		t.Error("expected slug-b circuit to be closed — independent of slug-a")
	}

	// slug-b should still allow requests
	_, err := reg.Execute("slug-b", func() (interface{}, error) {
		return "ok", nil
	})
	if err != nil {
		t.Errorf("expected slug-b to execute successfully, got: %v", err)
	}
}

// AC-002: Closed → open after N consecutive failures (default: 5)
func TestAC002_ClosedToOpenAfterNFailures(t *testing.T) {
	threshold := uint32(5)
	reg := upstream.NewCircuitRegistry(threshold, 30*time.Second)

	// First 4 failures should keep circuit closed
	for i := uint32(0); i < threshold-1; i++ {
		_, _ = reg.Execute("test-slug", func() (interface{}, error) {
			return nil, errors.New("fail")
		})
	}
	if reg.IsOpen("test-slug") {
		t.Error("circuit should not be open before reaching threshold")
	}

	// 5th failure should trip the circuit
	_, _ = reg.Execute("test-slug", func() (interface{}, error) {
		return nil, errors.New("fail")
	})
	if !reg.IsOpen("test-slug") {
		t.Error("circuit should be open after reaching failure threshold")
	}
}

// AC-003: Open circuit rejects immediately (returns ErrCircuitOpen)
func TestAC003_OpenCircuitRejectsImmediately(t *testing.T) {
	reg := upstream.NewCircuitRegistry(2, 30*time.Second)

	// Trip circuit
	for i := 0; i < 2; i++ {
		_, _ = reg.Execute("test-slug", func() (interface{}, error) {
			return nil, errors.New("fail")
		})
	}

	// Should reject with ErrCircuitOpen
	called := false
	_, err := reg.Execute("test-slug", func() (interface{}, error) {
		called = true
		return "should not execute", nil
	})
	if err == nil {
		t.Error("expected error when circuit is open")
	}
	if called {
		t.Error("function should not be called when circuit is open")
	}
}

// AC-004: Open → half-open after timeout
func TestAC004_OpenToHalfOpenAfterTimeout(t *testing.T) {
	// Use short timeout for test
	reg := upstream.NewCircuitRegistry(2, 100*time.Millisecond)

	// Trip circuit
	for i := 0; i < 2; i++ {
		_, _ = reg.Execute("test-slug", func() (interface{}, error) {
			return nil, errors.New("fail")
		})
	}

	if !reg.IsOpen("test-slug") {
		t.Fatal("circuit should be open")
	}

	// Wait for timeout
	time.Sleep(150 * time.Millisecond)

	// After timeout, a request should be allowed through (half-open)
	result, err := reg.Execute("test-slug", func() (interface{}, error) {
		return "probe-ok", nil
	})
	if err != nil {
		t.Errorf("expected probe to succeed in half-open state, got: %v", err)
	}
	if result != "probe-ok" {
		t.Errorf("expected result 'probe-ok', got: %v", result)
	}
}

// AC-005: Half-open: successful probe → closed
func TestAC005_HalfOpenSuccessfulProbeToClosed(t *testing.T) {
	reg := upstream.NewCircuitRegistry(2, 100*time.Millisecond)

	// Trip circuit
	for i := 0; i < 2; i++ {
		_, _ = reg.Execute("test-slug", func() (interface{}, error) {
			return nil, errors.New("fail")
		})
	}

	// Wait for half-open
	time.Sleep(150 * time.Millisecond)

	// Successful probe
	_, _ = reg.Execute("test-slug", func() (interface{}, error) {
		return "ok", nil
	})

	// Circuit should now be closed — multiple requests should succeed
	for i := 0; i < 3; i++ {
		_, err := reg.Execute("test-slug", func() (interface{}, error) {
			return "ok", nil
		})
		if err != nil {
			t.Errorf("expected circuit to be closed after successful probe, call %d got: %v", i, err)
		}
	}
}

// AC-006: Half-open: failed probe → re-open
func TestAC006_HalfOpenFailedProbeToReopen(t *testing.T) {
	reg := upstream.NewCircuitRegistry(2, 100*time.Millisecond)

	// Trip circuit
	for i := 0; i < 2; i++ {
		_, _ = reg.Execute("test-slug", func() (interface{}, error) {
			return nil, errors.New("fail")
		})
	}

	// Wait for half-open
	time.Sleep(150 * time.Millisecond)

	// Failed probe
	_, _ = reg.Execute("test-slug", func() (interface{}, error) {
		return nil, errors.New("still failing")
	})

	// Circuit should be open again
	if !reg.IsOpen("test-slug") {
		t.Error("circuit should re-open after failed probe in half-open state")
	}
}

// AC-007: Configurable thresholds
func TestAC007_ConfigurableThresholds(t *testing.T) {
	// Test with threshold of 3
	reg := upstream.NewCircuitRegistry(3, 200*time.Millisecond)

	// 2 failures should not trip
	for i := 0; i < 2; i++ {
		_, _ = reg.Execute("test-slug", func() (interface{}, error) {
			return nil, errors.New("fail")
		})
	}
	if reg.IsOpen("test-slug") {
		t.Error("circuit should not be open before reaching threshold of 3")
	}

	// 3rd failure trips it
	_, _ = reg.Execute("test-slug", func() (interface{}, error) {
		return nil, errors.New("fail")
	})
	if !reg.IsOpen("test-slug") {
		t.Error("circuit should be open after 3 failures")
	}
}

// AC-008: State() returns current state per slug
func TestAC008_StateReturnsCurrentState(t *testing.T) {
	reg := upstream.NewCircuitRegistry(2, 100*time.Millisecond)

	// Initial state should be closed
	state := reg.State("test-slug")
	if state != upstream.StateClosed {
		t.Errorf("expected initial state to be closed, got: %v", state)
	}

	// Trip circuit
	for i := 0; i < 2; i++ {
		_, _ = reg.Execute("test-slug", func() (interface{}, error) {
			return nil, errors.New("fail")
		})
	}

	state = reg.State("test-slug")
	if state != upstream.StateOpen {
		t.Errorf("expected state to be open after failures, got: %v", state)
	}
}
