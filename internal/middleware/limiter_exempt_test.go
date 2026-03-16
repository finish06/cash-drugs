package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/middleware"
)

// AC-003 (readiness-warmup): /ready is exempt from concurrency limiter
func TestAC003_ReadyExemptFromLimiter(t *testing.T) {
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

	// /ready should bypass the limiter
	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("/ready: expected 200, got %d (limiter should not block /ready)", rr.Code)
	}

	close(blocker)
	wg.Wait()
}
