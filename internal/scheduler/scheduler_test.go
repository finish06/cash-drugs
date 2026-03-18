package scheduler_test

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/fetchlock"
	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/finish06/cash-drugs/internal/model"
	"github.com/finish06/cash-drugs/internal/scheduler"
	"github.com/finish06/cash-drugs/internal/upstream"
	"github.com/prometheus/client_golang/prometheus"
)

// AC-003: Scheduler starts background goroutines for scheduled endpoints
func TestAC003_SchedulerStartsForScheduledEndpoints(t *testing.T) {
	fetcher := &mockFetcher{}
	repo := &mockRepo{}

	endpoints := []config.Endpoint{
		{Slug: "drugnames", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "* * * * *"},
	}
	config.ApplyDefaults(&endpoints[0])

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	// Wait for warm-up fetch
	time.Sleep(200 * time.Millisecond)

	if fetcher.callCount.Load() < 1 {
		t.Error("expected at least 1 fetch call from scheduler")
	}
}

// AC-004: Immediate cache warming on startup
func TestAC004_CacheWarmingOnStartup(t *testing.T) {
	fetcher := &mockFetcher{}
	repo := &mockRepo{}

	endpoints := []config.Endpoint{
		{Slug: "drugnames", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "0 0 1 1 *"},
	}
	config.ApplyDefaults(&endpoints[0])

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	// Cron is set far in future (Jan 1 midnight), but warm-up should fire immediately
	time.Sleep(200 * time.Millisecond)

	if fetcher.callCount.Load() < 1 {
		t.Error("expected immediate warm-up fetch on startup")
	}
}

// AC-002: On-demand-only endpoints are not scheduled
func TestAC002_OnDemandEndpointsNotScheduled(t *testing.T) {
	fetcher := &mockFetcher{}
	repo := &mockRepo{}

	endpoints := []config.Endpoint{
		{Slug: "ondemand", BaseURL: "http://example.com", Path: "/api", Format: "json"},
	}
	config.ApplyDefaults(&endpoints[0])

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	time.Sleep(200 * time.Millisecond)

	if fetcher.callCount.Load() != 0 {
		t.Errorf("expected 0 fetch calls for on-demand endpoint, got %d", fetcher.callCount.Load())
	}
}

// AC-006: Parameterized endpoints excluded from scheduling
func TestAC006_ParameterizedEndpointsExcluded(t *testing.T) {
	fetcher := &mockFetcher{}
	repo := &mockRepo{}

	endpoints := []config.Endpoint{
		{Slug: "spl-detail", BaseURL: "http://example.com", Path: "/v2/spls/{SETID}", Format: "json", Refresh: "* * * * *"},
	}
	config.ApplyDefaults(&endpoints[0])

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	time.Sleep(200 * time.Millisecond)

	if fetcher.callCount.Load() != 0 {
		t.Errorf("expected 0 fetch calls for parameterized endpoint, got %d", fetcher.callCount.Load())
	}
}

// AC-005: Scheduled fetch always re-fetches and upserts
func TestAC005_ScheduledFetchAlwaysUpserts(t *testing.T) {
	fetcher := &mockFetcher{}
	repo := &mockRepo{}

	endpoints := []config.Endpoint{
		{Slug: "drugnames", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "* * * * *"},
	}
	config.ApplyDefaults(&endpoints[0])

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	time.Sleep(200 * time.Millisecond)

	if repo.upsertCount.Load() < 1 {
		t.Error("expected at least 1 upsert from scheduled fetch")
	}
}

// AC-007: Failed fetch preserves existing cache
func TestAC007_FailedFetchPreservesCache(t *testing.T) {
	fetcher := &mockFetcher{shouldFail: true}
	repo := &mockRepo{}

	endpoints := []config.Endpoint{
		{Slug: "drugnames", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "* * * * *"},
	}
	config.ApplyDefaults(&endpoints[0])

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	time.Sleep(200 * time.Millisecond)

	// Fetch was attempted but failed
	if fetcher.callCount.Load() < 1 {
		t.Error("expected fetch attempt even though it fails")
	}
	// Cache was NOT updated
	if repo.upsertCount.Load() != 0 {
		t.Error("expected 0 upserts when fetch fails")
	}
}

// AC-008: Graceful shutdown stops scheduler
func TestAC008_GracefulShutdown(t *testing.T) {
	fetcher := &mockFetcher{}
	repo := &mockRepo{}

	endpoints := []config.Endpoint{
		{Slug: "drugnames", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "* * * * *"},
	}
	config.ApplyDefaults(&endpoints[0])

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)

	// Wait for warm-up
	time.Sleep(200 * time.Millisecond)
	countBeforeStop := fetcher.callCount.Load()

	s.Stop()

	// After stop, no more fetches should happen
	time.Sleep(200 * time.Millisecond)
	countAfterStop := fetcher.callCount.Load()

	if countAfterStop > countBeforeStop+1 {
		t.Errorf("expected no new fetches after stop, before=%d after=%d", countBeforeStop, countAfterStop)
	}
}

// M9-AC-017: Circuit open → scheduler skips fetch, logs warning
func TestM9_AC017_CircuitOpenSchedulerSkipsFetch(t *testing.T) {
	fetcher := &mockFetcher{}
	repo := &mockRepo{}

	endpoints := []config.Endpoint{
		{Slug: "drugnames", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "* * * * *"},
	}
	config.ApplyDefaults(&endpoints[0])

	circuit := upstream.NewCircuitRegistry(2, 30*time.Second)
	// Trip circuit for this slug
	for i := 0; i < 2; i++ {
		_, _ = circuit.Execute("drugnames", func() (interface{}, error) {
			return nil, &fetchError{msg: "fail"}
		})
	}

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New(), scheduler.WithCircuit(circuit))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	// Wait for warm-up attempt
	time.Sleep(200 * time.Millisecond)

	// Fetch should NOT have been called because circuit is open
	if fetcher.callCount.Load() > 0 {
		t.Errorf("expected 0 fetch calls when circuit is open, got %d", fetcher.callCount.Load())
	}
}

// M9-AC-018: Circuit closed → scheduler proceeds normally
func TestM9_AC018_CircuitClosedSchedulerProceeds(t *testing.T) {
	fetcher := &mockFetcher{}
	repo := &mockRepo{}

	endpoints := []config.Endpoint{
		{Slug: "drugnames", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "0 0 1 1 *"},
	}
	config.ApplyDefaults(&endpoints[0])

	circuit := upstream.NewCircuitRegistry(5, 30*time.Second)

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New(), scheduler.WithCircuit(circuit))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	time.Sleep(200 * time.Millisecond)

	if fetcher.callCount.Load() < 1 {
		t.Error("expected fetch to proceed when circuit is closed")
	}
}

// Test: WithMetrics option sets metrics on scheduler
func TestWithMetricsOption(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)
	fetcher := &mockFetcher{}
	repo := &mockRepo{}

	endpoints := []config.Endpoint{
		{Slug: "test", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "0 0 1 1 *"},
	}
	config.ApplyDefaults(&endpoints[0])

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New(), scheduler.WithMetrics(m))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	time.Sleep(200 * time.Millisecond)

	// Verify metrics were recorded (success path)
	if fetcher.callCount.Load() < 1 {
		t.Error("expected at least 1 fetch call")
	}
}

// Test: WithLRU option populates LRU cache after successful fetch
func TestWithLRUOption(t *testing.T) {
	fetcher := &mockFetcher{}
	repo := &mockRepo{}
	lru := &mockLRU{}

	endpoints := []config.Endpoint{
		{Slug: "test-lru", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "0 0 1 1 *"},
	}
	config.ApplyDefaults(&endpoints[0])

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New(), scheduler.WithLRU(lru))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	time.Sleep(200 * time.Millisecond)

	if lru.setCount.Load() < 1 {
		t.Errorf("expected LRU Set to be called at least once, got %d", lru.setCount.Load())
	}
}

// Test: fetchEndpoint records success metrics
func TestFetchEndpointSuccessMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)
	fetcher := &mockFetcher{}
	repo := &mockRepo{}

	endpoints := []config.Endpoint{
		{Slug: "metrics-ok", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "0 0 1 1 *"},
	}
	config.ApplyDefaults(&endpoints[0])

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New(), scheduler.WithMetrics(m))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	time.Sleep(200 * time.Millisecond)

	// Verify SchedulerRunsTotal success counter incremented
	gather, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	found := false
	for _, mf := range gather {
		if mf.GetName() == "cashdrugs_scheduler_runs_total" {
			for _, met := range mf.GetMetric() {
				for _, lp := range met.GetLabel() {
					if lp.GetName() == "result" && lp.GetValue() == "success" {
						found = true
					}
				}
			}
		}
	}
	if !found {
		t.Error("expected cashdrugs_scheduler_runs_total{result=success} metric")
	}
}

// Test: fetchEndpoint records error metrics on fetch failure
func TestFetchEndpointErrorMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)
	fetcher := &mockFetcher{shouldFail: true}
	repo := &mockRepo{}

	endpoints := []config.Endpoint{
		{Slug: "metrics-fail", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "0 0 1 1 *"},
	}
	config.ApplyDefaults(&endpoints[0])

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New(), scheduler.WithMetrics(m))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	time.Sleep(200 * time.Millisecond)

	gather, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	foundRunError := false
	foundFetchError := false
	for _, mf := range gather {
		if mf.GetName() == "cashdrugs_scheduler_runs_total" {
			for _, met := range mf.GetMetric() {
				for _, lp := range met.GetLabel() {
					if lp.GetName() == "result" && lp.GetValue() == "error" {
						foundRunError = true
					}
				}
			}
		}
		if mf.GetName() == "cashdrugs_upstream_fetch_errors_total" {
			foundFetchError = true
		}
	}
	if !foundRunError {
		t.Error("expected cashdrugs_scheduler_runs_total{result=error} metric")
	}
	if !foundFetchError {
		t.Error("expected cashdrugs_upstream_fetch_errors_total metric")
	}
}

// Test: fetchEndpoint records error metrics on upsert failure
func TestFetchEndpointUpsertFailureMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)
	fetcher := &mockFetcher{}
	repo := &mockRepo{shouldFailUpsert: true}

	endpoints := []config.Endpoint{
		{Slug: "upsert-fail", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "0 0 1 1 *"},
	}
	config.ApplyDefaults(&endpoints[0])

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New(), scheduler.WithMetrics(m))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	time.Sleep(200 * time.Millisecond)

	if repo.upsertCount.Load() < 1 {
		t.Error("expected upsert to be called")
	}

	gather, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	foundRunError := false
	for _, mf := range gather {
		if mf.GetName() == "cashdrugs_scheduler_runs_total" {
			for _, met := range mf.GetMetric() {
				for _, lp := range met.GetLabel() {
					if lp.GetName() == "result" && lp.GetValue() == "error" {
						foundRunError = true
					}
				}
			}
		}
	}
	if !foundRunError {
		t.Error("expected cashdrugs_scheduler_runs_total{result=error} on upsert failure")
	}
}

// Test: fetchEndpoint dedup via fetchlock records metric
func TestFetchEndpointDedupMetric(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)
	// Use a slow fetcher so the lock is held during the second attempt
	fetcher := &mockFetcher{delay: 300 * time.Millisecond}
	repo := &mockRepo{}

	endpoints := []config.Endpoint{
		{Slug: "dedup-test", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "0 0 1 1 *"},
	}
	config.ApplyDefaults(&endpoints[0])

	locks := fetchlock.New()

	s := scheduler.New(endpoints, fetcher, repo, locks, scheduler.WithMetrics(m))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Pre-acquire the lock so the warm-up fetch is deduped
	mu := locks.Get("dedup-test")
	mu.Lock()

	s.Start(ctx)
	defer s.Stop()

	// Wait for warm-up to attempt and get deduped
	time.Sleep(200 * time.Millisecond)

	// Release the lock
	mu.Unlock()

	// Verify dedup metric was recorded
	gather, err := reg.Gather()
	if err != nil {
		t.Fatalf("failed to gather metrics: %v", err)
	}

	foundDedup := false
	for _, mf := range gather {
		if mf.GetName() == "cashdrugs_fetchlock_dedup_total" {
			foundDedup = true
		}
	}
	if !foundDedup {
		t.Error("expected cashdrugs_fetchlock_dedup_total metric when lock is held")
	}

	// Fetch should not have been called (lock was held)
	if fetcher.callCount.Load() > 0 {
		t.Error("expected 0 fetch calls when lock is held")
	}
}

// Test: warm-up skips endpoints with fresh cache
func TestCacheWarmSkipsFreshCache(t *testing.T) {
	fetcher := &mockFetcher{}
	repo := &mockRepo{
		fetchedAtTime:  time.Now(),
		fetchedAtFound: true,
	}

	endpoints := []config.Endpoint{
		{Slug: "fresh-cache", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "0 0 1 1 *", TTL: "1h"},
	}
	config.ApplyDefaults(&endpoints[0])
	endpoints[0].TTLDuration = 1 * time.Hour

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	time.Sleep(200 * time.Millisecond)

	if fetcher.callCount.Load() != 0 {
		t.Errorf("expected 0 fetch calls for fresh cache, got %d", fetcher.callCount.Load())
	}
}

// Test: warm-up fetches when FetchedAt returns error
func TestCacheWarmFetchesOnFetchedAtError(t *testing.T) {
	fetcher := &mockFetcher{}
	repo := &mockRepo{
		fetchedAtErr: fmt.Errorf("db error"),
	}

	endpoints := []config.Endpoint{
		{Slug: "err-cache", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "0 0 1 1 *"},
	}
	config.ApplyDefaults(&endpoints[0])

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	time.Sleep(200 * time.Millisecond)

	if fetcher.callCount.Load() < 1 {
		t.Error("expected fetch when FetchedAt returns error")
	}
}

// Test: schedulableEndpoints with mixed endpoints
func TestSchedulableEndpointsMixed(t *testing.T) {
	fetcher := &mockFetcher{}
	repo := &mockRepo{}

	endpoints := []config.Endpoint{
		{Slug: "scheduled", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "0 0 1 1 *"},
		{Slug: "no-refresh", BaseURL: "http://example.com", Path: "/api", Format: "json"},
		{Slug: "parameterized", BaseURL: "http://example.com", Path: "/api/{ID}", Format: "json", Refresh: "0 0 1 1 *"},
	}
	for i := range endpoints {
		config.ApplyDefaults(&endpoints[i])
	}

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	time.Sleep(200 * time.Millisecond)

	// Only "scheduled" should have been fetched
	if fetcher.callCount.Load() != 1 {
		t.Errorf("expected exactly 1 fetch for schedulable endpoint, got %d", fetcher.callCount.Load())
	}
}

// Test: LRU uses TTLDuration from endpoint, defaults to 5m if zero
func TestLRUDefaultTTL(t *testing.T) {
	fetcher := &mockFetcher{}
	repo := &mockRepo{}
	lru := &mockLRU{}

	endpoints := []config.Endpoint{
		{Slug: "no-ttl", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "0 0 1 1 *"},
	}
	config.ApplyDefaults(&endpoints[0])
	// TTLDuration is zero (no TTL configured) — should default to 5m

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New(), scheduler.WithLRU(lru))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	time.Sleep(200 * time.Millisecond)

	if lru.setCount.Load() < 1 {
		t.Fatal("expected LRU Set to be called")
	}
	if lru.lastTTL != 5*time.Minute {
		t.Errorf("expected default TTL 5m, got %v", lru.lastTTL)
	}
}

// Test: LRU uses configured TTLDuration when set
func TestLRUConfiguredTTL(t *testing.T) {
	fetcher := &mockFetcher{}
	repo := &mockRepo{}
	lru := &mockLRU{}

	endpoints := []config.Endpoint{
		{Slug: "with-ttl", BaseURL: "http://example.com", Path: "/api", Format: "json", Refresh: "0 0 1 1 *", TTL: "10m"},
	}
	config.ApplyDefaults(&endpoints[0])
	endpoints[0].TTLDuration = 10 * time.Minute

	s := scheduler.New(endpoints, fetcher, repo, fetchlock.New(), scheduler.WithLRU(lru))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.Start(ctx)
	defer s.Stop()

	time.Sleep(200 * time.Millisecond)

	if lru.setCount.Load() < 1 {
		t.Fatal("expected LRU Set to be called")
	}
	if lru.lastTTL != 10*time.Minute {
		t.Errorf("expected TTL 10m, got %v", lru.lastTTL)
	}
}

// --- Mocks ---

type mockFetcher struct {
	callCount  atomic.Int64
	shouldFail bool
	delay      time.Duration
	mu         sync.Mutex
	lastSlug   string
}

func (m *mockFetcher) Fetch(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	m.callCount.Add(1)
	m.mu.Lock()
	m.lastSlug = ep.Slug
	m.mu.Unlock()

	if m.shouldFail {
		return nil, &fetchError{msg: "upstream unavailable"}
	}

	return &model.CachedResponse{
		Slug:      ep.Slug,
		CacheKey:  ep.Slug,
		Data:      map[string]interface{}{"items": []interface{}{"test"}},
		FetchedAt: time.Now(),
		SourceURL: ep.BaseURL + ep.Path,
		PageCount: 1,
	}, nil
}

type fetchError struct{ msg string }

func (e *fetchError) Error() string { return e.msg }

type mockRepo struct {
	upsertCount      atomic.Int64
	shouldFailUpsert bool
	fetchedAtTime    time.Time
	fetchedAtFound   bool
	fetchedAtErr     error
}

func (m *mockRepo) Get(cacheKey string) (*model.CachedResponse, error) {
	return nil, nil
}

func (m *mockRepo) Upsert(resp *model.CachedResponse) error {
	m.upsertCount.Add(1)
	if m.shouldFailUpsert {
		return fmt.Errorf("upsert failed")
	}
	return nil
}

func (m *mockRepo) FetchedAt(cacheKey string) (time.Time, bool, error) {
	if m.fetchedAtErr != nil {
		return time.Time{}, false, m.fetchedAtErr
	}
	return m.fetchedAtTime, m.fetchedAtFound, nil
}

type mockLRU struct {
	setCount atomic.Int64
	mu       sync.Mutex
	lastTTL  time.Duration
}

func (m *mockLRU) Get(key string) (*model.CachedResponse, bool) {
	return nil, false
}

func (m *mockLRU) Set(key string, resp *model.CachedResponse, ttl time.Duration) {
	m.setCount.Add(1)
	m.mu.Lock()
	m.lastTTL = ttl
	m.mu.Unlock()
}

func (m *mockLRU) Invalidate(key string) {}

func (m *mockLRU) SizeBytes() int64 { return 0 }
