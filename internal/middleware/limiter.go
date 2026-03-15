package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/finish06/cash-drugs/internal/metrics"
)

// ConcurrencyLimiter limits the number of in-flight requests using a channel-based semaphore.
type ConcurrencyLimiter struct {
	sem     chan struct{}
	metrics *metrics.Metrics
}

// NewConcurrencyLimiter creates a new ConcurrencyLimiter with the given limit.
// If m is non-nil, Prometheus metrics are recorded.
func NewConcurrencyLimiter(limit int, m *metrics.Metrics) *ConcurrencyLimiter {
	return &ConcurrencyLimiter{
		sem:     make(chan struct{}, limit),
		metrics: m,
	}
}

// Wrap returns an http.Handler that enforces the concurrency limit.
// Paths "/health" and "/metrics" are exempt from the limit.
func (cl *ConcurrencyLimiter) Wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Exempt paths bypass the limiter
		if isExempt(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}

		// Try to acquire the semaphore (non-blocking)
		select {
		case cl.sem <- struct{}{}:
			// Acquired — track inflight and ensure release via defer
			if cl.metrics != nil {
				cl.metrics.InFlightRequests.Inc()
			}
			defer func() {
				<-cl.sem
				if cl.metrics != nil {
					cl.metrics.InFlightRequests.Dec()
				}
			}()
			next.ServeHTTP(w, r)
		default:
			// Semaphore full — reject with 503
			if cl.metrics != nil {
				cl.metrics.RejectedRequestsTotal.Inc()
			}
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", "1")
			w.WriteHeader(http.StatusServiceUnavailable)
			json.NewEncoder(w).Encode(struct {
				Error      string `json:"error"`
				RetryAfter int    `json:"retry_after"`
			}{
				Error:      "service overloaded",
				RetryAfter: 1,
			})
		}
	})
}

// isExempt returns true for paths that should bypass the concurrency limiter.
func isExempt(path string) bool {
	return path == "/health" || path == "/metrics" || strings.HasPrefix(path, "/health/") || strings.HasPrefix(path, "/metrics/")
}
