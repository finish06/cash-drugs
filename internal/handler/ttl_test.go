package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/handler"
	"github.com/finish06/cash-drugs/internal/model"
)

// AC-004: Fresh cache within TTL returns stale=false
func TestTTL_AC004_FreshCacheWithinTTL(t *testing.T) {
	ep := config.Endpoint{
		Slug:        "drugnames",
		BaseURL:     "http://example.com",
		Path:        "/v2/drugnames",
		Format:      "json",
		TTL:         "6h",
		TTLDuration: 6 * time.Hour,
	}
	config.ApplyDefaults(&ep)

	cached := &model.CachedResponse{
		Slug:      "drugnames",
		CacheKey:  "drugnames",
		Data:      map[string]interface{}{"items": []interface{}{"drug1"}},
		FetchedAt: time.Now().Add(-2 * time.Hour), // 2h ago, within 6h TTL
		SourceURL: "http://example.com/v2/drugnames",
		PageCount: 1,
	}

	fetcher := &mockFetcher{}
	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: cached},
		fetcher,
		handler.WithFetchLocks(handler.NewFetchLocks()),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp model.APIResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Meta.Stale {
		t.Error("expected stale=false for cache within TTL")
	}
	if fetcher.fetchCalled {
		t.Error("expected no upstream fetch for fresh cache")
	}
}

// AC-005 + AC-007: Stale cache past TTL returns stale=true with reason "ttl_expired"
func TestTTL_AC005_StaleCachePastTTL(t *testing.T) {
	ep := config.Endpoint{
		Slug:        "drugnames",
		BaseURL:     "http://example.com",
		Path:        "/v2/drugnames",
		Format:      "json",
		TTL:         "6h",
		TTLDuration: 6 * time.Hour,
	}
	config.ApplyDefaults(&ep)

	cached := &model.CachedResponse{
		Slug:      "drugnames",
		CacheKey:  "drugnames",
		Data:      map[string]interface{}{"items": []interface{}{"stale-drug"}},
		FetchedAt: time.Now().Add(-8 * time.Hour), // 8h ago, past 6h TTL
		SourceURL: "http://example.com/v2/drugnames",
		PageCount: 1,
	}

	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:      "drugnames",
			CacheKey:  "drugnames",
			Data:      map[string]interface{}{"items": []interface{}{"fresh-drug"}},
			FetchedAt: time.Now(),
			SourceURL: "http://example.com/v2/drugnames",
			PageCount: 1,
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: cached},
		fetcher,
		handler.WithFetchLocks(handler.NewFetchLocks()),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp model.APIResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Meta.Stale {
		t.Error("expected stale=true for cache past TTL")
	}
	if resp.Meta.StaleReason != "ttl_expired" {
		t.Errorf("expected stale_reason='ttl_expired', got '%s'", resp.Meta.StaleReason)
	}
}

// AC-006: Background revalidation triggered for stale cache
func TestTTL_AC006_BackgroundRevalidationTriggered(t *testing.T) {
	ep := config.Endpoint{
		Slug:        "drugnames",
		BaseURL:     "http://example.com",
		Path:        "/v2/drugnames",
		Format:      "json",
		TTL:         "1m",
		TTLDuration: 1 * time.Minute,
	}
	config.ApplyDefaults(&ep)

	cached := &model.CachedResponse{
		Slug:      "drugnames",
		CacheKey:  "drugnames",
		Data:      map[string]interface{}{"items": []interface{}{"stale"}},
		FetchedAt: time.Now().Add(-5 * time.Minute), // 5m ago, past 1m TTL
		SourceURL: "http://example.com/v2/drugnames",
		PageCount: 1,
	}

	repo := &mockCacheRepo{cached: cached}
	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:      "drugnames",
			CacheKey:  "drugnames",
			Data:      map[string]interface{}{"items": []interface{}{"fresh"}},
			FetchedAt: time.Now(),
			SourceURL: "http://example.com/v2/drugnames",
			PageCount: 1,
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
		handler.WithFetchLocks(handler.NewFetchLocks()),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// Response should be immediate (stale data)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Wait for background goroutine to complete
	time.Sleep(200 * time.Millisecond)

	// Background fetch should have been triggered
	if !fetcher.fetchCalled {
		t.Error("expected background fetch to be triggered for stale cache")
	}
	// Background upsert should have happened
	if !repo.upsertCalled {
		t.Error("expected background upsert after revalidation")
	}
}

// AC-008: Background revalidation deduplicates via shared mutex
func TestTTL_AC008_DeduplicatesWithScheduler(t *testing.T) {
	ep := config.Endpoint{
		Slug:        "drugnames",
		BaseURL:     "http://example.com",
		Path:        "/v2/drugnames",
		Format:      "json",
		TTL:         "1m",
		TTLDuration: 1 * time.Minute,
	}
	config.ApplyDefaults(&ep)

	cached := &model.CachedResponse{
		Slug:      "drugnames",
		CacheKey:  "drugnames",
		Data:      map[string]interface{}{"items": []interface{}{"stale"}},
		FetchedAt: time.Now().Add(-5 * time.Minute),
		SourceURL: "http://example.com/v2/drugnames",
		PageCount: 1,
	}

	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:      "drugnames",
			CacheKey:  "drugnames",
			Data:      map[string]interface{}{"items": []interface{}{"fresh"}},
			FetchedAt: time.Now(),
			SourceURL: "http://example.com/v2/drugnames",
			PageCount: 1,
		},
	}

	// Pre-lock the slug mutex (simulating scheduler already fetching)
	locks := handler.NewFetchLocks()
	mu := locks.Get("drugnames")
	mu.Lock() // scheduler holds this lock

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: cached},
		fetcher,
		handler.WithFetchLocks(locks),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// Should still get stale response immediately
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Wait a bit — background revalidation should have been skipped
	time.Sleep(200 * time.Millisecond)

	// Fetch should NOT have been called (lock was held by "scheduler")
	if fetcher.fetchCalled {
		t.Error("expected background fetch to be skipped when lock is held")
	}

	mu.Unlock() // release the simulated scheduler lock
}

// AC-009: Background revalidation failure preserves stale cache
func TestTTL_AC009_RevalidationFailurePreservesCache(t *testing.T) {
	ep := config.Endpoint{
		Slug:        "drugnames",
		BaseURL:     "http://example.com",
		Path:        "/v2/drugnames",
		Format:      "json",
		TTL:         "1m",
		TTLDuration: 1 * time.Minute,
	}
	config.ApplyDefaults(&ep)

	cached := &model.CachedResponse{
		Slug:      "drugnames",
		CacheKey:  "drugnames",
		Data:      map[string]interface{}{"items": []interface{}{"stale"}},
		FetchedAt: time.Now().Add(-5 * time.Minute),
		SourceURL: "http://example.com/v2/drugnames",
		PageCount: 1,
	}

	repo := &mockCacheRepo{cached: cached}
	fetcher := &mockFetcher{err: fmt.Errorf("upstream unavailable")}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
		handler.WithFetchLocks(handler.NewFetchLocks()),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// Should get stale response
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Wait for background goroutine
	time.Sleep(200 * time.Millisecond)

	// Fetch was attempted but failed
	if !fetcher.fetchCalled {
		t.Error("expected background fetch attempt")
	}
	// Cache was NOT overwritten (no upsert on failure)
	if repo.upsertCalled {
		t.Error("expected no upsert when background revalidation fails")
	}
}

// AC-003: No TTL means cache is never stale via handler
func TestTTL_AC003_NoTTLNeverStale(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
		// No TTL — cache never expires
	}
	config.ApplyDefaults(&ep)

	cached := &model.CachedResponse{
		Slug:      "drugnames",
		CacheKey:  "drugnames",
		Data:      map[string]interface{}{"items": []interface{}{"old-drug"}},
		FetchedAt: time.Now().Add(-30 * 24 * time.Hour), // 30 days ago
		SourceURL: "http://example.com/v2/drugnames",
		PageCount: 1,
	}

	fetcher := &mockFetcher{}
	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: cached},
		fetcher,
		handler.WithFetchLocks(handler.NewFetchLocks()),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp model.APIResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Meta.Stale {
		t.Error("expected stale=false when no TTL configured (never expires)")
	}
	if fetcher.fetchCalled {
		t.Error("expected no fetch when cache has no TTL")
	}
}

// --- FetchLocks mock helpers ---
// These test the NewFetchLocks/Get API that we'll need to implement

func TestFetchLocks_GetReturnsSameMutex(t *testing.T) {
	locks := handler.NewFetchLocks()
	mu1 := locks.Get("drugnames")
	mu2 := locks.Get("drugnames")

	if mu1 != mu2 {
		t.Error("expected same mutex for same slug")
	}

	mu3 := locks.Get("other-slug")
	if mu1 == mu3 {
		t.Error("expected different mutex for different slug")
	}
}

// Verify FetchLocks is goroutine-safe
func TestFetchLocks_ConcurrentAccess(t *testing.T) {
	locks := handler.NewFetchLocks()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			locks.Get("test-slug")
		}()
	}
	wg.Wait()
}
