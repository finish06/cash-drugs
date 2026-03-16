package handler

import (
	"log/slog"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/fetchlock"
	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/finish06/cash-drugs/internal/upstream"
)

// WarmupOrchestrator implements the WarmupTrigger interface.
// It orchestrates cache warming for scheduled endpoints and parameterized queries.
type WarmupOrchestrator struct {
	endpoints []config.Endpoint
	fetcher   upstream.Fetcher
	repo      cache.Repository
	locks     *fetchlock.Map
	circuit   *upstream.CircuitRegistry
	queries   map[string][]map[string]string
	state     *WarmupStateTracker
	metrics   *metrics.Metrics
	lru       cache.LRUCache
	semSize   int // concurrency cap for parameterized queries
}

// WarmupOrchestratorOption configures a WarmupOrchestrator.
type WarmupOrchestratorOption func(*WarmupOrchestrator)

// WithOrchestratorLRU sets the LRU cache for warmup results.
func WithOrchestratorLRU(lru cache.LRUCache) WarmupOrchestratorOption {
	return func(o *WarmupOrchestrator) {
		o.lru = lru
	}
}

// WithOrchestratorSemSize sets the concurrency cap for parameterized query fetches.
func WithOrchestratorSemSize(n int) WarmupOrchestratorOption {
	return func(o *WarmupOrchestrator) {
		if n > 0 {
			o.semSize = n
		}
	}
}

// NewWarmupOrchestrator creates a WarmupOrchestrator with the given dependencies.
func NewWarmupOrchestrator(
	endpoints []config.Endpoint,
	fetcher upstream.Fetcher,
	repo cache.Repository,
	locks *fetchlock.Map,
	circuit *upstream.CircuitRegistry,
	queries map[string][]map[string]string,
	state *WarmupStateTracker,
	m *metrics.Metrics,
	opts ...WarmupOrchestratorOption,
) *WarmupOrchestrator {
	o := &WarmupOrchestrator{
		endpoints: endpoints,
		fetcher:   fetcher,
		repo:      repo,
		locks:     locks,
		circuit:   circuit,
		queries:   queries,
		state:     state,
		metrics:   m,
		semSize:   5,
	}
	for _, opt := range opts {
		opt(o)
	}
	return o
}

// TriggerWarmup starts a background warm-up for the given slugs.
// If slugs is nil, all scheduled endpoints are warmed.
// If skipQueries is true, parameterized queries from warmup-queries.yaml are skipped.
func (o *WarmupOrchestrator) TriggerWarmup(slugs []string, skipQueries bool) {
	// Build slug filter set
	slugFilter := make(map[string]bool)
	if slugs != nil {
		for _, s := range slugs {
			slugFilter[s] = true
		}
	}

	// Determine scheduled endpoints to warm
	scheduled := o.scheduledEndpoints(slugFilter)

	// Count parameterized queries
	queryCount := 0
	if !skipQueries {
		queryCount = QueryCountForSlugs(o.queries, slugs)
	}

	total := len(scheduled) + queryCount
	o.state.Reset()
	o.state.SetTotal(total)
	o.state.SetPhase("scheduled")

	if o.metrics != nil {
		o.metrics.WarmupQueriesPending.Set(float64(queryCount))
	}

	go func() {
		// Phase 1: Warm scheduled endpoints
		o.warmScheduledEndpoints(scheduled)

		// Phase 2: Warm parameterized queries
		if !skipQueries && queryCount > 0 {
			o.state.SetPhase("queries")
			o.warmParameterizedQueries(slugFilter)
		}

		// Done
		o.state.MarkReady()
		if o.metrics != nil {
			o.metrics.WarmupQueriesPending.Set(0)
		}
		slog.Info("warmup complete", "component", "warmup", "total", total)
	}()
}

// scheduledEndpoints returns endpoints with a Refresh field, filtered by slugFilter.
func (o *WarmupOrchestrator) scheduledEndpoints(slugFilter map[string]bool) []config.Endpoint {
	var result []config.Endpoint
	for _, ep := range o.endpoints {
		if ep.Refresh == "" {
			continue
		}
		if len(slugFilter) > 0 && !slugFilter[ep.Slug] {
			continue
		}
		// Skip parameterized endpoints (they have no default params)
		if len(config.ExtractAllParams(ep)) > 0 {
			continue
		}
		result = append(result, ep)
	}
	return result
}

// warmScheduledEndpoints fetches and upserts each scheduled endpoint.
func (o *WarmupOrchestrator) warmScheduledEndpoints(endpoints []config.Endpoint) {
	for _, ep := range endpoints {
		// Circuit breaker check
		if o.circuit != nil && o.circuit.IsOpen(ep.Slug) {
			slog.Warn("warmup: skipping endpoint — circuit open", "component", "warmup", "slug", ep.Slug)
			o.state.IncrDone()
			continue
		}

		// Check freshness — skip if still fresh in MongoDB
		cacheKey := cache.BuildCacheKey(ep.Slug, nil)
		fetchedAt, found, err := o.repo.FetchedAt(cacheKey)
		if err == nil && found && !config.IsStale(ep, fetchedAt) {
			slog.Info("warmup: cache still fresh — skipping", "component", "warmup", "slug", ep.Slug)
			o.state.IncrDone()
			continue
		}

		start := time.Now()
		result, fetchErr := o.fetcher.Fetch(ep, nil)
		duration := time.Since(start)
		if fetchErr != nil {
			slog.Error("warmup: fetch failed", "component", "warmup", "slug", ep.Slug, "duration", duration, "error", fetchErr)
			if o.metrics != nil {
				o.metrics.UpstreamFetchErrors.WithLabelValues(ep.Slug).Inc()
			}
			o.state.IncrDone()
			continue
		}

		if upsertErr := o.repo.Upsert(result); upsertErr != nil {
			slog.Error("warmup: upsert failed", "component", "warmup", "slug", ep.Slug, "error", upsertErr)
			o.state.IncrDone()
			continue
		}

		// Populate LRU cache
		if o.lru != nil {
			ttl := ep.TTLDuration
			if ttl == 0 {
				ttl = 5 * time.Minute
			}
			o.lru.Set(cacheKey, result, ttl)
		}

		slog.Info("warmup: endpoint warmed", "component", "warmup", "slug", ep.Slug, "duration", duration)
		o.state.IncrDone()
	}
}

// warmParameterizedQueries fetches parameterized queries with a semaphore cap.
func (o *WarmupOrchestrator) warmParameterizedQueries(slugFilter map[string]bool) {
	// Build endpoint lookup
	epMap := make(map[string]config.Endpoint)
	for _, ep := range o.endpoints {
		epMap[ep.Slug] = ep
	}

	sem := make(chan struct{}, o.semSize)

	type workItem struct {
		slug   string
		params map[string]string
	}

	// Collect all work items
	var items []workItem
	for slug, paramsList := range o.queries {
		if len(slugFilter) > 0 && !slugFilter[slug] {
			continue
		}
		for _, params := range paramsList {
			items = append(items, workItem{slug: slug, params: params})
		}
	}

	// Process with semaphore
	done := make(chan struct{}, len(items))
	for _, item := range items {
		sem <- struct{}{} // acquire
		go func(wi workItem) {
			defer func() {
				<-sem // release
				done <- struct{}{}
			}()

			ep, ok := epMap[wi.slug]
			if !ok {
				slog.Warn("warmup: unknown slug in queries — skipping", "component", "warmup", "slug", wi.slug)
				o.state.IncrDone()
				if o.metrics != nil {
					o.metrics.WarmupQueriesTotal.WithLabelValues(wi.slug, "skipped").Inc()
					o.metrics.WarmupQueriesPending.Add(-1)
				}
				return
			}

			// Circuit breaker check
			if o.circuit != nil && o.circuit.IsOpen(wi.slug) {
				slog.Warn("warmup: skipping query — circuit open", "component", "warmup", "slug", wi.slug, "params", wi.params)
				o.state.IncrDone()
				if o.metrics != nil {
					o.metrics.WarmupQueriesTotal.WithLabelValues(wi.slug, "circuit_open").Inc()
					o.metrics.WarmupQueriesPending.Add(-1)
				}
				return
			}

			result, err := o.fetcher.Fetch(ep, wi.params)
			if err != nil {
				slog.Error("warmup: query fetch failed", "component", "warmup", "slug", wi.slug, "params", wi.params, "error", err)
				if o.metrics != nil {
					o.metrics.WarmupQueriesTotal.WithLabelValues(wi.slug, "error").Inc()
					o.metrics.UpstreamFetchErrors.WithLabelValues(wi.slug).Inc()
					o.metrics.WarmupQueriesPending.Add(-1)
				}
				o.state.IncrDone()
				return
			}

			if upsertErr := o.repo.Upsert(result); upsertErr != nil {
				slog.Error("warmup: query upsert failed", "component", "warmup", "slug", wi.slug, "params", wi.params, "error", upsertErr)
				if o.metrics != nil {
					o.metrics.WarmupQueriesTotal.WithLabelValues(wi.slug, "error").Inc()
					o.metrics.WarmupQueriesPending.Add(-1)
				}
				o.state.IncrDone()
				return
			}

			// Populate LRU cache
			if o.lru != nil {
				ttl := ep.TTLDuration
				if ttl == 0 {
					ttl = 5 * time.Minute
				}
				cacheKey := cache.BuildCacheKey(wi.slug, wi.params)
				o.lru.Set(cacheKey, result, ttl)
			}

			if o.metrics != nil {
				o.metrics.WarmupQueriesTotal.WithLabelValues(wi.slug, "success").Inc()
				o.metrics.WarmupQueriesPending.Add(-1)
			}
			o.state.IncrDone()
		}(item)
	}

	// Wait for all items to complete
	for range items {
		<-done
	}
}
