package upstream

import (
	"sync"
	"time"

	gobreaker "github.com/sony/gobreaker/v2"
)

// Circuit breaker state constants for external use.
const (
	StateClosed   = gobreaker.StateClosed
	StateHalfOpen = gobreaker.StateHalfOpen
	StateOpen     = gobreaker.StateOpen
)

// CircuitRegistry manages per-slug circuit breakers.
type CircuitRegistry struct {
	breakers         sync.Map
	failureThreshold uint32
	openDuration     time.Duration
}

// NewCircuitRegistry creates a new CircuitRegistry with the given failure threshold and open duration.
func NewCircuitRegistry(failureThreshold uint32, openDuration time.Duration) *CircuitRegistry {
	return &CircuitRegistry{
		failureThreshold: failureThreshold,
		openDuration:     openDuration,
	}
}

// Execute runs fn within the circuit breaker for the given slug.
// If the circuit is open, it returns an error immediately without calling fn.
func (r *CircuitRegistry) Execute(slug string, fn func() (interface{}, error)) (interface{}, error) {
	cb := r.getOrCreate(slug)
	return cb.Execute(fn)
}

// State returns the current circuit breaker state for the given slug.
func (r *CircuitRegistry) State(slug string) gobreaker.State {
	cb := r.getOrCreate(slug)
	return cb.State()
}

// IsOpen returns true if the circuit breaker for the given slug is in the open state.
func (r *CircuitRegistry) IsOpen(slug string) bool {
	return r.State(slug) == gobreaker.StateOpen
}

// OpenDuration returns the configured open duration for the circuit breakers.
func (r *CircuitRegistry) OpenDuration() time.Duration {
	return r.openDuration
}

func (r *CircuitRegistry) getOrCreate(slug string) *gobreaker.CircuitBreaker[interface{}] {
	if val, ok := r.breakers.Load(slug); ok {
		return val.(*gobreaker.CircuitBreaker[interface{}])
	}

	threshold := r.failureThreshold
	cb := gobreaker.NewCircuitBreaker[interface{}](gobreaker.Settings{
		Name:        slug,
		MaxRequests: 1,
		Interval:    0,
		Timeout:     r.openDuration,
		ReadyToTrip: func(counts gobreaker.Counts) bool {
			return counts.ConsecutiveFailures >= threshold
		},
	})

	actual, _ := r.breakers.LoadOrStore(slug, cb)
	return actual.(*gobreaker.CircuitBreaker[interface{}])
}
