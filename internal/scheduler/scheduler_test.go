package scheduler_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/finish06/drugs/internal/config"
	"github.com/finish06/drugs/internal/fetchlock"
	"github.com/finish06/drugs/internal/model"
	"github.com/finish06/drugs/internal/scheduler"
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

// --- Mocks ---

type mockFetcher struct {
	callCount  atomic.Int64
	shouldFail bool
	mu         sync.Mutex
	lastSlug   string
}

func (m *mockFetcher) Fetch(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error) {
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
	upsertCount atomic.Int64
}

func (m *mockRepo) Get(cacheKey string) (*model.CachedResponse, error) {
	return nil, nil
}

func (m *mockRepo) Upsert(resp *model.CachedResponse) error {
	m.upsertCount.Add(1)
	return nil
}
