package scheduler

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/fetchlock"
	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/finish06/cash-drugs/internal/upstream"
	"github.com/robfig/cron/v3"
)

// Scheduler manages background refresh of cached endpoints.
type Scheduler struct {
	endpoints []config.Endpoint
	fetcher   upstream.Fetcher
	repo      cache.Repository
	cron      *cron.Cron
	locks     *fetchlock.Map
	metrics   *metrics.Metrics
}

// Option configures a Scheduler.
type Option func(*Scheduler)

// WithMetrics enables Prometheus metric instrumentation for the scheduler.
func WithMetrics(m *metrics.Metrics) Option {
	return func(s *Scheduler) {
		s.metrics = m
	}
}

// New creates a Scheduler for the given endpoints.
// Only endpoints with a non-empty Refresh field and no path parameters are scheduled.
func New(endpoints []config.Endpoint, fetcher upstream.Fetcher, repo cache.Repository, locks *fetchlock.Map, opts ...Option) *Scheduler {
	s := &Scheduler{
		endpoints: endpoints,
		fetcher:   fetcher,
		repo:      repo,
		cron:      cron.New(),
		locks:     locks,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// Start registers cron jobs, starts the scheduler, and warms the cache in the background.
func (s *Scheduler) Start(ctx context.Context) {
	scheduled := s.schedulableEndpoints()

	if len(scheduled) == 0 {
		slog.Info("no endpoints to schedule", "component", "scheduler")
		return
	}

	// Register cron jobs
	for _, ep := range scheduled {
		ep := ep // capture loop variable
		s.cron.AddFunc(ep.Refresh, func() {
			s.fetchEndpoint(ep)
		})
		slog.Info("endpoint registered", "component", "scheduler", "slug", ep.Slug, "cron", ep.Refresh)
	}

	s.cron.Start()
	slog.Info("scheduler started", "component", "scheduler", "endpoints", len(scheduled))

	// Warm cache in background — skip endpoints with fresh data in MongoDB
	go func() {
		slog.Info("cache warm started", "component", "scheduler", "endpoints", len(scheduled))
		var wg sync.WaitGroup
		for _, ep := range scheduled {
			wg.Add(1)
			go func(ep config.Endpoint) {
				defer wg.Done()
				cacheKey := cache.BuildCacheKey(ep.Slug, nil)
				fetchedAt, found, err := s.repo.FetchedAt(cacheKey)
				if err == nil && found && !config.IsStale(ep, fetchedAt) {
					slog.Info("cache still fresh — skipping fetch", "component", "scheduler", "slug", ep.Slug, "fetched_at", fetchedAt)
					return
				}
				s.fetchEndpoint(ep)
			}(ep)
		}
		wg.Wait()
		slog.Info("cache warm complete", "component", "scheduler", "endpoints", len(scheduled))
	}()
}

// Stop gracefully shuts down the scheduler.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	slog.Info("scheduler stopped", "component", "scheduler")
}

func (s *Scheduler) fetchEndpoint(ep config.Endpoint) {
	mu := s.locks.Get(ep.Slug)
	if !mu.TryLock() {
		slog.Debug("skipping fetch — previous fetch still running", "component", "scheduler", "slug", ep.Slug)
		if s.metrics != nil {
			s.metrics.FetchLockDedupTotal.WithLabelValues(ep.Slug).Inc()
		}
		return
	}
	defer mu.Unlock()

	slog.Info("fetch started", "component", "scheduler", "slug", ep.Slug)
	start := time.Now()
	result, err := s.fetcher.Fetch(ep, nil)
	duration := time.Since(start)

	if err != nil {
		slog.Error("fetch failed — preserving existing cache", "component", "scheduler", "slug", ep.Slug, "duration", duration, "error", err)
		if s.metrics != nil {
			s.metrics.SchedulerRunsTotal.WithLabelValues(ep.Slug, "error").Inc()
			s.metrics.SchedulerRunDuration.WithLabelValues(ep.Slug).Observe(duration.Seconds())
			s.metrics.UpstreamFetchErrors.WithLabelValues(ep.Slug).Inc()
		}
		return
	}

	if err := s.repo.Upsert(result); err != nil {
		slog.Error("cache upsert failed", "component", "scheduler", "slug", ep.Slug, "error", err)
		if s.metrics != nil {
			s.metrics.SchedulerRunsTotal.WithLabelValues(ep.Slug, "error").Inc()
			s.metrics.SchedulerRunDuration.WithLabelValues(ep.Slug).Observe(duration.Seconds())
		}
		return
	}

	slog.Info("fetch completed", "component", "scheduler", "slug", ep.Slug, "duration", duration, "pages", result.PageCount)
	if s.metrics != nil {
		s.metrics.SchedulerRunsTotal.WithLabelValues(ep.Slug, "success").Inc()
		s.metrics.SchedulerRunDuration.WithLabelValues(ep.Slug).Observe(duration.Seconds())
		s.metrics.UpstreamFetchPages.WithLabelValues(ep.Slug).Add(float64(result.PageCount))
	}
}

func (s *Scheduler) schedulableEndpoints() []config.Endpoint {
	var result []config.Endpoint
	for _, ep := range s.endpoints {
		if ep.Refresh == "" {
			continue
		}
		if len(config.ExtractAllParams(ep)) > 0 {
			slog.Warn("endpoint has path parameters and cannot be scheduled", "component", "scheduler", "slug", ep.Slug)
			continue
		}
		result = append(result, ep)
	}
	return result
}
