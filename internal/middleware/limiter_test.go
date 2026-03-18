package middleware_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/finish06/cash-drugs/internal/middleware"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

// AC-001: Request admitted when under limit
func TestAC001_RequestAdmittedUnderLimit(t *testing.T) {
	limiter := middleware.NewConcurrencyLimiter(10, nil)
	handler := limiter.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/cache/test", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rr.Code)
	}
}

// AC-002, AC-003, AC-004: Request rejected with 503 + Retry-After when at limit
func TestAC002_RequestRejectedAtLimit(t *testing.T) {
	limiter := middleware.NewConcurrencyLimiter(1, nil)

	blocker := make(chan struct{})
	handler := limiter.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocker // block until released
		w.WriteHeader(http.StatusOK)
	}))

	// Start a request that will hold the semaphore
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		req := httptest.NewRequest(http.MethodGet, "/api/cache/slow", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}()

	// Give the goroutine time to acquire the semaphore
	time.Sleep(50 * time.Millisecond)

	// This request should be rejected
	req := httptest.NewRequest(http.MethodGet, "/api/cache/rejected", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 503, got %d", rr.Code)
	}

	// AC-003: Retry-After header
	retryAfter := rr.Header().Get("Retry-After")
	if retryAfter != "1" {
		t.Errorf("expected Retry-After header '1', got '%s'", retryAfter)
	}

	// AC-004: JSON error body
	var errResp struct {
		Error      string `json:"error"`
		RetryAfter int    `json:"retry_after"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &errResp); err != nil {
		t.Fatalf("failed to parse response body: %v", err)
	}
	if errResp.Error != "service overloaded" {
		t.Errorf("expected error 'service overloaded', got '%s'", errResp.Error)
	}
	if errResp.RetryAfter != 1 {
		t.Errorf("expected retry_after 1, got %d", errResp.RetryAfter)
	}

	// Content-Type header
	ct := rr.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", ct)
	}

	// Release the blocking request
	close(blocker)
	wg.Wait()
}

// AC-001: Semaphore released after handler completes (including panics via defer)
func TestAC001_SemaphoreReleasedAfterCompletion(t *testing.T) {
	limiter := middleware.NewConcurrencyLimiter(1, nil)

	handler := limiter.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First request completes
	req := httptest.NewRequest(http.MethodGet, "/api/cache/first", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("first request: expected 200, got %d", rr.Code)
	}

	// Second request should also succeed (semaphore was released)
	req = httptest.NewRequest(http.MethodGet, "/api/cache/second", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("second request: expected 200, got %d", rr.Code)
	}
}

// AC-001: Semaphore released even on panic (defer safety)
func TestAC001_SemaphoreReleasedOnPanic(t *testing.T) {
	limiter := middleware.NewConcurrencyLimiter(1, nil)

	panicHandler := limiter.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	}))

	// Recover from the panic in the test
	func() {
		defer func() { _ = recover() }()
		req := httptest.NewRequest(http.MethodGet, "/api/cache/panic", nil)
		rr := httptest.NewRecorder()
		panicHandler.ServeHTTP(rr, req)
	}()

	// After panic, the semaphore should be released — next request should succeed
	normalHandler := limiter.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	req := httptest.NewRequest(http.MethodGet, "/api/cache/after-panic", nil)
	rr := httptest.NewRecorder()
	normalHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("after panic: expected 200, got %d", rr.Code)
	}
}

// AC-005: Exempt paths (/health, /metrics) bypass the limiter
func TestAC005_ExemptPathsBypassLimiter(t *testing.T) {
	limiter := middleware.NewConcurrencyLimiter(1, nil)

	blocker := make(chan struct{})
	handler := limiter.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/cache/slow" {
			<-blocker
		}
		w.WriteHeader(http.StatusOK)
	}))

	// Fill the semaphore with a blocking request
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		req := httptest.NewRequest(http.MethodGet, "/api/cache/slow", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}()
	time.Sleep(50 * time.Millisecond)

	// /health should bypass
	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("/health: expected 200, got %d", rr.Code)
	}

	// /metrics should bypass
	req = httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rr = httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("/metrics: expected 200, got %d", rr.Code)
	}

	close(blocker)
	wg.Wait()
}

// AC-008: Prometheus inflight gauge incremented/decremented
func TestAC008_InFlightGaugeUpdated(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)
	limiter := middleware.NewConcurrencyLimiter(10, m)

	blocker := make(chan struct{})
	handler := limiter.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocker
		w.WriteHeader(http.StatusOK)
	}))

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		req := httptest.NewRequest(http.MethodGet, "/api/cache/test", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}()
	time.Sleep(50 * time.Millisecond)

	// Check that the gauge is 1 while request is in-flight
	gauge := m.InFlightRequests
	var metric dto.Metric
	if err := gauge.Write(&metric); err != nil {
		t.Fatalf("failed to read gauge: %v", err)
	}
	if got := metric.GetGauge().GetValue(); got != 1 {
		t.Errorf("expected inflight gauge 1, got %f", got)
	}

	close(blocker)
	wg.Wait()

	// After completion, gauge should be back to 0
	var metric2 dto.Metric
	if err := gauge.Write(&metric2); err != nil {
		t.Fatalf("failed to read gauge: %v", err)
	}
	if got := metric2.GetGauge().GetValue(); got != 0 {
		t.Errorf("expected inflight gauge 0 after completion, got %f", got)
	}
}

// AC-009: Prometheus rejected counter incremented on rejection
func TestAC009_RejectedCounterIncremented(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)
	limiter := middleware.NewConcurrencyLimiter(1, m)

	blocker := make(chan struct{})
	handler := limiter.Wrap(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		<-blocker
		w.WriteHeader(http.StatusOK)
	}))

	// Fill the semaphore
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		req := httptest.NewRequest(http.MethodGet, "/api/cache/slow", nil)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
	}()
	time.Sleep(50 * time.Millisecond)

	// This request should be rejected
	req := httptest.NewRequest(http.MethodGet, "/api/cache/rejected", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", rr.Code)
	}

	// Check rejected counter
	counter := m.RejectedRequestsTotal
	var metric dto.Metric
	if err := counter.Write(&metric); err != nil {
		t.Fatalf("failed to read counter: %v", err)
	}
	if got := metric.GetCounter().GetValue(); got != 1 {
		t.Errorf("expected rejected counter 1, got %f", got)
	}

	close(blocker)
	wg.Wait()
}
