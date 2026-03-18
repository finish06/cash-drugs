package handler_test

import (
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/fetchlock"
	"github.com/finish06/cash-drugs/internal/handler"
	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/finish06/cash-drugs/internal/model"
	"github.com/finish06/cash-drugs/internal/upstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// --- Mock Fetcher for warmup orchestrator tests ---

type warmupMockFetcher struct {
	mu         sync.Mutex
	fetchCount int32
	delay      time.Duration
	failSlugs  map[string]bool
}

func (f *warmupMockFetcher) Fetch(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error) {
	atomic.AddInt32(&f.fetchCount, 1)
	if f.delay > 0 {
		time.Sleep(f.delay)
	}
	if f.failSlugs != nil {
		f.mu.Lock()
		fail := f.failSlugs[ep.Slug]
		f.mu.Unlock()
		if fail {
			return nil, fmt.Errorf("mock fetch error for %s", ep.Slug)
		}
	}
	cacheKey := cache.BuildCacheKey(ep.Slug, params)
	return &model.CachedResponse{
		Slug:       ep.Slug,
		Params:     params,
		CacheKey:   cacheKey,
		Data:       []interface{}{},
		FetchedAt:  time.Now(),
		HTTPStatus: 200,
		PageCount:  1,
		CreatedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}, nil
}

// --- Mock Repository for warmup orchestrator tests ---

type warmupMockRepo struct {
	mu       sync.Mutex
	store    map[string]*model.CachedResponse
	failNext bool
}

func newWarmupMockRepo() *warmupMockRepo {
	return &warmupMockRepo{store: make(map[string]*model.CachedResponse)}
}

func (r *warmupMockRepo) Get(cacheKey string) (*model.CachedResponse, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	resp, ok := r.store[cacheKey]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return resp, nil
}

func (r *warmupMockRepo) Upsert(resp *model.CachedResponse) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.failNext {
		r.failNext = false
		return fmt.Errorf("mock upsert error")
	}
	r.store[resp.CacheKey] = resp
	return nil
}

func (r *warmupMockRepo) FetchedAt(cacheKey string) (time.Time, bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	resp, ok := r.store[cacheKey]
	if !ok {
		return time.Time{}, false, nil
	}
	return resp.FetchedAt, true, nil
}

func (r *warmupMockRepo) upsertedKeys() []string {
	r.mu.Lock()
	defer r.mu.Unlock()
	keys := make([]string, 0, len(r.store))
	for k := range r.store {
		keys = append(keys, k)
	}
	return keys
}

// TestOrchestratorTriggersScheduledAndParameterizedWarmup verifies that TriggerWarmup
// warms both scheduled endpoints and parameterized queries.
func TestOrchestratorTriggersScheduledAndParameterizedWarmup(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages"},
		{Slug: "no-refresh", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages"},
	}
	fetcher := &warmupMockFetcher{}
	repo := newWarmupMockRepo()
	locks := fetchlock.New()
	circuit := upstream.NewCircuitRegistry(5, 30*time.Second)
	queries := map[string][]map[string]string{
		"fda-ndc": {
			{"GENERIC_NAME": "METFORMIN"},
			{"GENERIC_NAME": "LISINOPRIL"},
		},
	}
	state := handler.NewWarmupStateTracker()

	orch := handler.NewWarmupOrchestrator(
		endpoints, fetcher, repo, locks, circuit,
		queries, state, nil,
	)

	orch.TriggerWarmup(nil, false)

	// Wait for warmup to complete
	deadline := time.After(5 * time.Second)
	for !state.IsReady() {
		select {
		case <-deadline:
			t.Fatal("warmup did not complete within 5 seconds")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Should have fetched: 1 scheduled endpoint + 2 parameterized queries = 3
	count := atomic.LoadInt32(&fetcher.fetchCount)
	if count != 3 {
		t.Errorf("expected 3 fetches (1 scheduled + 2 queries), got %d", count)
	}

	// Verify progress
	done, total := state.Progress()
	if done != 3 || total != 3 {
		t.Errorf("expected progress 3/3, got %d/%d", done, total)
	}

	if state.Phase() != "ready" {
		t.Errorf("expected phase 'ready', got '%s'", state.Phase())
	}

	// Verify all items are in repo
	keys := repo.upsertedKeys()
	if len(keys) != 3 {
		t.Errorf("expected 3 cached items, got %d: %v", len(keys), keys)
	}
}

// TestOrchestratorConcurrencyCapRespected verifies the semaphore limits concurrent queries.
func TestOrchestratorConcurrencyCapRespected(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages"},
	}

	var maxConcurrent int32
	var currentConcurrent int32

	fetcher := &concurrencyTrackingFetcher{
		maxConcurrent:     &maxConcurrent,
		currentConcurrent: &currentConcurrent,
		delay:             50 * time.Millisecond,
	}

	repo := newWarmupMockRepo()
	locks := fetchlock.New()
	circuit := upstream.NewCircuitRegistry(5, 30*time.Second)

	// Create 10 queries to exceed the concurrency cap of 3
	queries := map[string][]map[string]string{
		"fda-ndc": make([]map[string]string, 10),
	}
	for i := range queries["fda-ndc"] {
		queries["fda-ndc"][i] = map[string]string{"GENERIC_NAME": fmt.Sprintf("DRUG_%d", i)}
	}

	state := handler.NewWarmupStateTracker()
	orch := handler.NewWarmupOrchestrator(
		endpoints, fetcher, repo, locks, circuit,
		queries, state, nil,
		handler.WithOrchestratorSemSize(3),
	)

	orch.TriggerWarmup(nil, false)

	// Wait for completion
	deadline := time.After(10 * time.Second)
	for !state.IsReady() {
		select {
		case <-deadline:
			t.Fatal("warmup did not complete within 10 seconds")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	max := atomic.LoadInt32(&maxConcurrent)
	if max > 3 {
		t.Errorf("expected max concurrency <= 3, got %d", max)
	}
	if max < 1 {
		t.Errorf("expected at least 1 concurrent fetch, got %d", max)
	}
}

type concurrencyTrackingFetcher struct {
	maxConcurrent     *int32
	currentConcurrent *int32
	delay             time.Duration
}

func (f *concurrencyTrackingFetcher) Fetch(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error) {
	cur := atomic.AddInt32(f.currentConcurrent, 1)
	defer atomic.AddInt32(f.currentConcurrent, -1)

	// Track max
	for {
		old := atomic.LoadInt32(f.maxConcurrent)
		if cur <= old || atomic.CompareAndSwapInt32(f.maxConcurrent, old, cur) {
			break
		}
	}

	time.Sleep(f.delay)

	cacheKey := cache.BuildCacheKey(ep.Slug, params)
	return &model.CachedResponse{
		Slug:      ep.Slug,
		CacheKey:  cacheKey,
		Data:      []interface{}{},
		FetchedAt: time.Now(),
		HTTPStatus: 200,
		PageCount: 1,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}, nil
}

// TestOrchestratorCircuitBreakerSkipsWithWarning verifies that open circuits are skipped.
func TestOrchestratorCircuitBreakerSkipsWithWarning(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages"},
	}

	fetcher := &warmupMockFetcher{}
	repo := newWarmupMockRepo()
	locks := fetchlock.New()
	circuit := upstream.NewCircuitRegistry(1, 30*time.Second) // threshold=1 so we can trip it easily

	// Trip the circuit breaker for fda-ndc
	_, _ = circuit.Execute("fda-ndc", func() (interface{}, error) {
		return nil, fmt.Errorf("forced failure")
	})

	queries := map[string][]map[string]string{
		"fda-ndc": {{"GENERIC_NAME": "METFORMIN"}},
	}
	state := handler.NewWarmupStateTracker()

	orch := handler.NewWarmupOrchestrator(
		endpoints, fetcher, repo, locks, circuit,
		queries, state, nil,
	)

	orch.TriggerWarmup(nil, false)

	// Wait for completion
	deadline := time.After(5 * time.Second)
	for !state.IsReady() {
		select {
		case <-deadline:
			t.Fatal("warmup did not complete within 5 seconds")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Should have 0 fetches — both scheduled endpoint and query skipped due to open circuit
	count := atomic.LoadInt32(&fetcher.fetchCount)
	if count != 0 {
		t.Errorf("expected 0 fetches (circuit open), got %d", count)
	}

	// Progress should still show completion
	done, total := state.Progress()
	if done != total {
		t.Errorf("expected done==total, got %d/%d", done, total)
	}
}

// TestOrchestratorProgressTracking verifies state transitions through phases.
func TestOrchestratorProgressTracking(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages"},
	}
	fetcher := &warmupMockFetcher{delay: 20 * time.Millisecond}
	repo := newWarmupMockRepo()
	locks := fetchlock.New()
	circuit := upstream.NewCircuitRegistry(5, 30*time.Second)
	queries := map[string][]map[string]string{
		"fda-ndc": {{"GENERIC_NAME": "METFORMIN"}},
	}
	state := handler.NewWarmupStateTracker()

	orch := handler.NewWarmupOrchestrator(
		endpoints, fetcher, repo, locks, circuit,
		queries, state, nil,
	)

	// Before warmup
	if state.IsReady() {
		t.Error("expected not ready before warmup")
	}

	orch.TriggerWarmup(nil, false)

	// Wait for completion
	deadline := time.After(5 * time.Second)
	for !state.IsReady() {
		select {
		case <-deadline:
			t.Fatal("warmup did not complete within 5 seconds")
		default:
			time.Sleep(5 * time.Millisecond)
		}
	}

	// After warmup
	if !state.IsReady() {
		t.Error("expected ready after warmup")
	}
	if state.Phase() != "ready" {
		t.Errorf("expected phase 'ready', got '%s'", state.Phase())
	}
	done, total := state.Progress()
	if done != 2 || total != 2 {
		t.Errorf("expected 2/2 progress, got %d/%d", done, total)
	}
}

// TestOrchestratorSkipQueriesFlag verifies that skipQueries=true skips parameterized queries.
// Scheduled endpoint fetch fails → increments done, continues
func TestOrchestratorFetchErrorContinues(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages"},
		{Slug: "dailymed", Refresh: "0 3 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api2", DataKey: "data", TotalKey: "metadata.total_pages"},
	}
	fetcher := &warmupMockFetcher{failSlugs: map[string]bool{"fda-ndc": true}}
	repo := newWarmupMockRepo()
	locks := fetchlock.New()
	circuit := upstream.NewCircuitRegistry(5, 30*time.Second)
	state := handler.NewWarmupStateTracker()

	orch := handler.NewWarmupOrchestrator(
		endpoints, fetcher, repo, locks, circuit,
		nil, state, nil,
	)

	orch.TriggerWarmup(nil, true)

	deadline := time.After(5 * time.Second)
	for !state.IsReady() {
		select {
		case <-deadline:
			t.Fatal("warmup did not complete within 5 seconds")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// fda-ndc failed, dailymed succeeded — both counted as done
	done, total := state.Progress()
	if total != 2 {
		t.Errorf("expected total=2, got %d", total)
	}
	if done != 2 {
		t.Errorf("expected done=2, got %d", done)
	}

	// Only dailymed should be in the repo
	keys := repo.upsertedKeys()
	if len(keys) != 1 {
		t.Errorf("expected 1 upserted key, got %d: %v", len(keys), keys)
	}
}

// Upsert fails → increments done, continues
func TestOrchestratorUpsertErrorContinues(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages"},
	}
	fetcher := &warmupMockFetcher{}
	repo := newWarmupMockRepo()
	repo.failNext = true
	locks := fetchlock.New()
	circuit := upstream.NewCircuitRegistry(5, 30*time.Second)
	state := handler.NewWarmupStateTracker()

	orch := handler.NewWarmupOrchestrator(
		endpoints, fetcher, repo, locks, circuit,
		nil, state, nil,
	)

	orch.TriggerWarmup(nil, true)

	deadline := time.After(5 * time.Second)
	for !state.IsReady() {
		select {
		case <-deadline:
			t.Fatal("warmup did not complete within 5 seconds")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	done, total := state.Progress()
	if total != 1 {
		t.Errorf("expected total=1, got %d", total)
	}
	if done != 1 {
		t.Errorf("expected done=1 (even on upsert error), got %d", done)
	}
}

// Fresh cache skip — endpoint with recent data should not be refetched
func TestOrchestratorSkipsFreshCache(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages", TTL: "1h", TTLDuration: time.Hour},
	}

	fetcher := &warmupMockFetcher{}
	repo := newWarmupMockRepo()

	// Pre-populate cache with fresh data
	cacheKey := cache.BuildCacheKey("fda-ndc", nil)
	repo.store[cacheKey] = &model.CachedResponse{
		Slug:      "fda-ndc",
		CacheKey:  cacheKey,
		FetchedAt: time.Now(), // just fetched
		Data:      []interface{}{"existing"},
	}

	locks := fetchlock.New()
	circuit := upstream.NewCircuitRegistry(5, 30*time.Second)
	state := handler.NewWarmupStateTracker()

	orch := handler.NewWarmupOrchestrator(
		endpoints, fetcher, repo, locks, circuit,
		nil, state, nil,
	)

	orch.TriggerWarmup(nil, true)

	deadline := time.After(5 * time.Second)
	for !state.IsReady() {
		select {
		case <-deadline:
			t.Fatal("warmup did not complete within 5 seconds")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Fetcher should NOT have been called — cache is fresh
	count := atomic.LoadInt32(&fetcher.fetchCount)
	if count != 0 {
		t.Errorf("expected 0 fetches (cache is fresh), got %d", count)
	}
}

// Parameterized query upsert error → increments done, continues with remaining queries
func TestOrchestratorQueryUpsertErrorContinues(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages"},
	}
	fetcher := &warmupMockFetcher{}
	repo := newWarmupMockRepo()
	// Set failNext so the first upsert in parameterized queries fails
	repo.failNext = true
	locks := fetchlock.New()
	circuit := upstream.NewCircuitRegistry(5, 30*time.Second)
	queries := map[string][]map[string]string{
		"fda-ndc": {{"GENERIC_NAME": "METFORMIN"}, {"GENERIC_NAME": "LISINOPRIL"}},
	}
	state := handler.NewWarmupStateTracker()

	orch := handler.NewWarmupOrchestrator(
		endpoints, fetcher, repo, locks, circuit,
		queries, state, nil,
	)

	orch.TriggerWarmup(nil, false)

	deadline := time.After(5 * time.Second)
	for !state.IsReady() {
		select {
		case <-deadline:
			t.Fatal("warmup did not complete within 5 seconds")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// All items should be done despite the upsert error on one query
	done, total := state.Progress()
	if done != total {
		t.Errorf("expected all items done, got %d/%d", done, total)
	}

	// The scheduled endpoint upserts fine, one query fails, one succeeds
	// So we should have 2 keys: the scheduled endpoint + 1 successful query
	keys := repo.upsertedKeys()
	if len(keys) != 2 {
		t.Errorf("expected 2 upserted keys (1 scheduled + 1 query after upsert error on first), got %d: %v", len(keys), keys)
	}
}

// Parameterized query fetch error → continues with remaining queries
func TestOrchestratorQueryFetchErrorContinues(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages"},
	}
	fetcher := &warmupMockFetcher{failSlugs: map[string]bool{"fda-ndc": true}}
	repo := newWarmupMockRepo()
	locks := fetchlock.New()
	circuit := upstream.NewCircuitRegistry(5, 30*time.Second)
	queries := map[string][]map[string]string{
		"fda-ndc": {{"GENERIC_NAME": "METFORMIN"}, {"GENERIC_NAME": "LISINOPRIL"}},
	}
	state := handler.NewWarmupStateTracker()

	orch := handler.NewWarmupOrchestrator(
		endpoints, fetcher, repo, locks, circuit,
		queries, state, nil,
	)

	orch.TriggerWarmup(nil, false)

	deadline := time.After(5 * time.Second)
	for !state.IsReady() {
		select {
		case <-deadline:
			t.Fatal("warmup did not complete within 5 seconds")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// All items should be done despite errors
	done, total := state.Progress()
	if done != total {
		t.Errorf("expected all items done, got %d/%d", done, total)
	}
}

// Unknown slug in parameterized queries → skipped, continues with remaining queries
func TestOrchestratorQueryUnknownSlugSkipped(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages"},
	}
	fetcher := &warmupMockFetcher{}
	repo := newWarmupMockRepo()
	locks := fetchlock.New()
	circuit := upstream.NewCircuitRegistry(5, 30*time.Second)
	// Include a query for a slug that doesn't exist in endpoints
	queries := map[string][]map[string]string{
		"fda-ndc":       {{"GENERIC_NAME": "METFORMIN"}},
		"nonexistent":   {{"PARAM": "value"}},
	}
	state := handler.NewWarmupStateTracker()

	orch := handler.NewWarmupOrchestrator(
		endpoints, fetcher, repo, locks, circuit,
		queries, state, nil,
	)

	orch.TriggerWarmup(nil, false)

	deadline := time.After(5 * time.Second)
	for !state.IsReady() {
		select {
		case <-deadline:
			t.Fatal("warmup did not complete within 5 seconds")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// All items should complete (unknown slug is skipped, not stuck)
	done, total := state.Progress()
	if done != total {
		t.Errorf("expected all items done, got %d/%d", done, total)
	}

	// Only the valid query should be in repo (scheduled + fda-ndc query)
	keys := repo.upsertedKeys()
	if len(keys) != 2 {
		t.Errorf("expected 2 upserted keys (1 scheduled + 1 valid query), got %d: %v", len(keys), keys)
	}
}

// LRU populated during parameterized query warmup
func TestOrchestratorQueryPopulatesLRU(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages", TTL: "10m", TTLDuration: 10 * time.Minute},
	}
	fetcher := &warmupMockFetcher{}
	repo := newWarmupMockRepo()
	locks := fetchlock.New()
	circuit := upstream.NewCircuitRegistry(5, 30*time.Second)
	queries := map[string][]map[string]string{
		"fda-ndc": {{"GENERIC_NAME": "METFORMIN"}},
	}
	state := handler.NewWarmupStateTracker()
	lru := cache.NewLRUCache(10 * 1024 * 1024)

	orch := handler.NewWarmupOrchestrator(
		endpoints, fetcher, repo, locks, circuit,
		queries, state, nil,
		handler.WithOrchestratorLRU(lru),
	)

	orch.TriggerWarmup(nil, false)

	deadline := time.After(5 * time.Second)
	for !state.IsReady() {
		select {
		case <-deadline:
			t.Fatal("warmup did not complete within 5 seconds")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// LRU should have the query result
	cacheKey := cache.BuildCacheKey("fda-ndc", map[string]string{"GENERIC_NAME": "METFORMIN"})
	_, ok := lru.Get(cacheKey)
	if !ok {
		t.Error("expected LRU cache hit for parameterized query after warmup")
	}
}

// LRU populated during parameterized query warmup with zero TTL → uses default 5min
func TestOrchestratorQueryPopulatesLRU_ZeroTTL(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages"},
	}
	fetcher := &warmupMockFetcher{}
	repo := newWarmupMockRepo()
	locks := fetchlock.New()
	circuit := upstream.NewCircuitRegistry(5, 30*time.Second)
	queries := map[string][]map[string]string{
		"fda-ndc": {{"GENERIC_NAME": "METFORMIN"}},
	}
	state := handler.NewWarmupStateTracker()
	lru := cache.NewLRUCache(10 * 1024 * 1024)

	orch := handler.NewWarmupOrchestrator(
		endpoints, fetcher, repo, locks, circuit,
		queries, state, nil,
		handler.WithOrchestratorLRU(lru),
	)

	orch.TriggerWarmup(nil, false)

	deadline := time.After(5 * time.Second)
	for !state.IsReady() {
		select {
		case <-deadline:
			t.Fatal("warmup did not complete within 5 seconds")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// LRU should have the query result even with zero TTL (uses 5min default)
	cacheKey := cache.BuildCacheKey("fda-ndc", map[string]string{"GENERIC_NAME": "METFORMIN"})
	_, ok := lru.Get(cacheKey)
	if !ok {
		t.Error("expected LRU cache hit for parameterized query with zero TTL")
	}
}

// Parameterized queries with metrics enabled — covers all metrics branches
func TestOrchestratorQueryMetricsBranches(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages"},
	}

	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	t.Run("success path records metrics", func(t *testing.T) {
		fetcher := &warmupMockFetcher{}
		repo := newWarmupMockRepo()
		locks := fetchlock.New()
		circuit := upstream.NewCircuitRegistry(5, 30*time.Second)
		queries := map[string][]map[string]string{
			"fda-ndc": {{"GENERIC_NAME": "METFORMIN"}},
		}
		state := handler.NewWarmupStateTracker()

		orch := handler.NewWarmupOrchestrator(
			endpoints, fetcher, repo, locks, circuit,
			queries, state, m,
		)

		orch.TriggerWarmup(nil, false)

		deadline := time.After(5 * time.Second)
		for !state.IsReady() {
			select {
			case <-deadline:
				t.Fatal("warmup did not complete within 5 seconds")
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}

		val := testutil.ToFloat64(m.WarmupQueriesTotal.WithLabelValues("fda-ndc", "success"))
		if val < 1 {
			t.Errorf("expected warmup_queries_total success >= 1, got %f", val)
		}
	})

	t.Run("fetch error records error metrics", func(t *testing.T) {
		reg2 := prometheus.NewRegistry()
		m2 := metrics.NewMetrics(reg2)

		fetcher := &warmupMockFetcher{failSlugs: map[string]bool{"fda-ndc": true}}
		repo := newWarmupMockRepo()
		locks := fetchlock.New()
		circuit := upstream.NewCircuitRegistry(5, 30*time.Second)
		queries := map[string][]map[string]string{
			"fda-ndc": {{"GENERIC_NAME": "METFORMIN"}},
		}
		state := handler.NewWarmupStateTracker()

		orch := handler.NewWarmupOrchestrator(
			endpoints, fetcher, repo, locks, circuit,
			queries, state, m2,
		)

		orch.TriggerWarmup(nil, false)

		deadline := time.After(5 * time.Second)
		for !state.IsReady() {
			select {
			case <-deadline:
				t.Fatal("warmup did not complete within 5 seconds")
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}

		errVal := testutil.ToFloat64(m2.WarmupQueriesTotal.WithLabelValues("fda-ndc", "error"))
		if errVal < 1 {
			t.Errorf("expected warmup_queries_total error >= 1, got %f", errVal)
		}
		upstreamErrVal := testutil.ToFloat64(m2.UpstreamFetchErrors.WithLabelValues("fda-ndc"))
		if upstreamErrVal < 1 {
			t.Errorf("expected upstream_fetch_errors >= 1, got %f", upstreamErrVal)
		}
	})

	t.Run("unknown slug records skipped metrics", func(t *testing.T) {
		reg3 := prometheus.NewRegistry()
		m3 := metrics.NewMetrics(reg3)

		fetcher := &warmupMockFetcher{}
		repo := newWarmupMockRepo()
		locks := fetchlock.New()
		circuit := upstream.NewCircuitRegistry(5, 30*time.Second)
		queries := map[string][]map[string]string{
			"nonexistent": {{"PARAM": "value"}},
		}
		state := handler.NewWarmupStateTracker()

		orch := handler.NewWarmupOrchestrator(
			endpoints, fetcher, repo, locks, circuit,
			queries, state, m3,
		)

		orch.TriggerWarmup(nil, false)

		deadline := time.After(5 * time.Second)
		for !state.IsReady() {
			select {
			case <-deadline:
				t.Fatal("warmup did not complete within 5 seconds")
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}

		skipVal := testutil.ToFloat64(m3.WarmupQueriesTotal.WithLabelValues("nonexistent", "skipped"))
		if skipVal < 1 {
			t.Errorf("expected warmup_queries_total skipped >= 1, got %f", skipVal)
		}
	})

	t.Run("circuit open records circuit_open metrics", func(t *testing.T) {
		reg4 := prometheus.NewRegistry()
		m4 := metrics.NewMetrics(reg4)

		fetcher := &warmupMockFetcher{}
		repo := newWarmupMockRepo()
		locks := fetchlock.New()
		circuit := upstream.NewCircuitRegistry(1, 30*time.Second)

		// Trip the circuit breaker
		_, _ = circuit.Execute("fda-ndc", func() (interface{}, error) {
			return nil, fmt.Errorf("forced failure")
		})

		queries := map[string][]map[string]string{
			"fda-ndc": {{"GENERIC_NAME": "METFORMIN"}},
		}
		state := handler.NewWarmupStateTracker()

		orch := handler.NewWarmupOrchestrator(
			endpoints, fetcher, repo, locks, circuit,
			queries, state, m4,
		)

		orch.TriggerWarmup(nil, false)

		deadline := time.After(5 * time.Second)
		for !state.IsReady() {
			select {
			case <-deadline:
				t.Fatal("warmup did not complete within 5 seconds")
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}

		circuitVal := testutil.ToFloat64(m4.WarmupQueriesTotal.WithLabelValues("fda-ndc", "circuit_open"))
		if circuitVal < 1 {
			t.Errorf("expected warmup_queries_total circuit_open >= 1, got %f", circuitVal)
		}
	})

	t.Run("upsert error with metrics records error", func(t *testing.T) {
		reg5 := prometheus.NewRegistry()
		m5 := metrics.NewMetrics(reg5)

		fetcher := &warmupMockFetcher{}
		repo := newWarmupMockRepo()
		repo.failNext = true
		locks := fetchlock.New()
		circuit := upstream.NewCircuitRegistry(5, 30*time.Second)
		queries := map[string][]map[string]string{
			"fda-ndc": {{"GENERIC_NAME": "METFORMIN"}},
		}
		state := handler.NewWarmupStateTracker()

		// Use endpoint without Refresh to avoid scheduled warmup consuming failNext
		noRefreshEndpoints := []config.Endpoint{
			{Slug: "fda-ndc", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages"},
		}
		orch := handler.NewWarmupOrchestrator(
			noRefreshEndpoints, fetcher, repo, locks, circuit,
			queries, state, m5,
		)

		orch.TriggerWarmup(nil, false)

		deadline := time.After(5 * time.Second)
		for !state.IsReady() {
			select {
			case <-deadline:
				t.Fatal("warmup did not complete within 5 seconds")
			default:
				time.Sleep(10 * time.Millisecond)
			}
		}

		errVal := testutil.ToFloat64(m5.WarmupQueriesTotal.WithLabelValues("fda-ndc", "error"))
		if errVal < 1 {
			t.Errorf("expected warmup_queries_total error >= 1 for upsert failure, got %f", errVal)
		}
	})
}

func TestOrchestratorSkipQueriesFlag(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api", DataKey: "data", TotalKey: "metadata.total_pages"},
	}
	fetcher := &warmupMockFetcher{}
	repo := newWarmupMockRepo()
	locks := fetchlock.New()
	circuit := upstream.NewCircuitRegistry(5, 30*time.Second)
	queries := map[string][]map[string]string{
		"fda-ndc": {{"GENERIC_NAME": "METFORMIN"}, {"GENERIC_NAME": "LISINOPRIL"}},
	}
	state := handler.NewWarmupStateTracker()

	orch := handler.NewWarmupOrchestrator(
		endpoints, fetcher, repo, locks, circuit,
		queries, state, nil,
	)

	orch.TriggerWarmup(nil, true) // skipQueries=true

	deadline := time.After(5 * time.Second)
	for !state.IsReady() {
		select {
		case <-deadline:
			t.Fatal("warmup did not complete within 5 seconds")
		default:
			time.Sleep(10 * time.Millisecond)
		}
	}

	// Should only fetch the scheduled endpoint, not queries
	count := atomic.LoadInt32(&fetcher.fetchCount)
	if count != 1 {
		t.Errorf("expected 1 fetch (scheduled only), got %d", count)
	}

	done, total := state.Progress()
	if total != 1 {
		t.Errorf("expected total=1 (no queries), got %d", total)
	}
	if done != 1 {
		t.Errorf("expected done=1, got %d", done)
	}
}
