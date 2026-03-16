package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/finish06/cash-drugs/internal/model"
	"github.com/finish06/cash-drugs/internal/upstream"
	"golang.org/x/sync/singleflight"
)

// CacheHandler handles GET /api/cache/{slug} requests.
type CacheHandler struct {
	endpoints  map[string]config.Endpoint
	repo       cache.Repository
	fetcher    upstream.Fetcher
	fetchLocks *FetchLocks
	metrics    *metrics.Metrics
	lru        cache.LRUCache
	sfGroup    singleflight.Group
	circuit    *upstream.CircuitRegistry
	cooldown   *upstream.CooldownTracker
}

// Option configures a CacheHandler.
type Option func(*CacheHandler)

// WithFetchLocks sets the shared fetch locks for deduplication with the scheduler.
func WithFetchLocks(fl *FetchLocks) Option {
	return func(h *CacheHandler) {
		h.fetchLocks = fl
	}
}

// WithMetrics enables Prometheus metric instrumentation.
func WithMetrics(m *metrics.Metrics) Option {
	return func(h *CacheHandler) {
		h.metrics = m
	}
}

// WithLRU sets the in-memory LRU cache for the handler.
func WithLRU(lru cache.LRUCache) Option {
	return func(h *CacheHandler) {
		h.lru = lru
	}
}

// WithCircuit sets the circuit breaker registry.
func WithCircuit(c *upstream.CircuitRegistry) Option {
	return func(h *CacheHandler) {
		h.circuit = c
	}
}

// WithCooldown sets the force-refresh cooldown tracker.
func WithCooldown(cd *upstream.CooldownTracker) Option {
	return func(h *CacheHandler) {
		h.cooldown = cd
	}
}

// NewCacheHandler creates a new CacheHandler.
func NewCacheHandler(endpoints []config.Endpoint, repo cache.Repository, fetcher upstream.Fetcher, opts ...Option) *CacheHandler {
	epMap := make(map[string]config.Endpoint, len(endpoints))
	for _, ep := range endpoints {
		epMap[ep.Slug] = ep
	}
	h := &CacheHandler{
		endpoints: epMap,
		repo:      repo,
		fetcher:   fetcher,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// ServeHTTP handles incoming cache requests.
//
// @Summary      Get cached data for an endpoint
// @Description  Returns cached upstream API data. Fetches from upstream if not cached.
// @Description  The response data shape depends on the upstream API. For example, the
// @Description  `drugclasses` endpoint returns objects with fields: `name` (string),
// @Description  `type` (string), `code` (string), `codingSystem` (string).
// @Description  Available query parameters vary by endpoint — use `GET /api/endpoints`
// @Description  to discover each endpoint's supported params (e.g. BRAND_NAME, GENERIC_NAME,
// @Description  NDC, PHARM_CLASS for fda-ndc; SETID for spl-detail).
// @Tags         cache
// @Produce      json
// @Param        slug   path      string  true   "Endpoint slug from config"
// @Param        SETID  query     string  false  "Path parameter value (varies by endpoint)"
// @Param        _force query     string  false  "Force upstream refresh (true/false)"
// @Success      200  {object}  model.APIResponse
// @Failure      404  {object}  model.ErrorResponse
// @Failure      502  {object}  model.ErrorResponse
// @Failure      503  {object}  model.ErrorResponse  "Service overloaded or upstream circuit open"
// @Router       /api/cache/{slug} [get]
func (h *CacheHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	slug := extractSlug(r.URL.Path)

	ep, ok := h.endpoints[slug]
	if !ok {
		slog.Debug("request", "component", "handler", "method", r.Method, "path", r.URL.Path, "status", 404, "duration", time.Since(start))
		h.recordHTTPMetrics(slug, r.Method, http.StatusNotFound, start)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		json.NewEncoder(w).Encode(model.ErrorResponse{
			Error: "endpoint not configured",
			Slug:  slug,
		})
		return
	}

	// Extract path parameters from query string
	pathParams := extractPathParams(ep, r)

	// Build cache key
	cacheKey := cache.BuildCacheKey(slug, pathParams)

	// Check for _force param (used to simulate "no cache" scenario for stale tests)
	forceRefresh := r.URL.Query().Get("_force") == "true"

	// 1. Force-refresh cooldown check (BEFORE singleflight)
	if forceRefresh && h.cooldown != nil && h.cooldown.Check(cacheKey) {
		slog.Debug("force-refresh blocked by cooldown", "component", "handler", "slug", slug)
		if h.metrics != nil {
			h.metrics.ForceRefreshCooldownTotal.WithLabelValues(slug).Inc()
		}
		cached, err := h.repo.Get(cacheKey)
		if err == nil && cached != nil {
			w.Header().Set("X-Force-Cooldown", "true")
			respondWithCached(w, cached, false, "")
			h.recordHTTPMetrics(slug, r.Method, http.StatusOK, start)
			return
		}
		// No cache available — fall through to normal fetch path
		forceRefresh = false
	}

	// On force refresh, invalidate LRU entry
	if forceRefresh && h.lru != nil {
		h.lru.Invalidate(cacheKey)
	}

	// staleCache holds the first MongoDB lookup result for reuse in error fallback,
	// avoiding a duplicate query when the upstream fetch fails.
	var staleCache *model.CachedResponse

	// 2. Try LRU cache first (unless force refresh)
	if !forceRefresh && h.lru != nil {
		if lruResult, ok := h.lru.Get(cacheKey); ok {
			slog.Debug("LRU cache hit", "component", "handler", "slug", slug)
			h.recordLRUHit(slug)
			h.recordCacheOutcome(slug, "hit")
			respondWithCached(w, lruResult, false, "")
			slog.Debug("request", "component", "handler", "method", r.Method, "path", r.URL.Path, "slug", slug, "status", 200, "cache", "lru_hit", "duration", time.Since(start))
			h.recordHTTPMetrics(slug, r.Method, http.StatusOK, start)
			return
		}
		h.recordLRUMiss(slug)
	}

	// Try MongoDB cache (unless force refresh)
	if !forceRefresh {
		cached, err := h.repo.Get(cacheKey)
		if err == nil && cached != nil {
			// Check TTL staleness
			if config.IsStale(ep, cached.FetchedAt) {
				slog.Debug("cache hit (stale)", "component", "handler", "slug", slug, "reason", "ttl_expired")
				h.recordCacheOutcome(slug, "stale")
				respondWithCached(w, cached, true, "ttl_expired")
				h.backgroundRevalidate(ep, pathParams)
				slog.Debug("request", "component", "handler", "method", r.Method, "path", r.URL.Path, "slug", slug, "status", 200, "cache", "stale", "duration", time.Since(start))
				h.recordHTTPMetrics(slug, r.Method, http.StatusOK, start)
				return
			}
			slog.Debug("cache hit", "component", "handler", "slug", slug)
			h.recordCacheOutcome(slug, "hit")
			// Populate LRU from MongoDB hit
			h.populateLRU(cacheKey, cached, ep)
			respondWithCached(w, cached, false, "")
			slog.Debug("request", "component", "handler", "method", r.Method, "path", r.URL.Path, "slug", slug, "status", 200, "cache", "hit", "duration", time.Since(start))
			h.recordHTTPMetrics(slug, r.Method, http.StatusOK, start)
			return
		}
		// Save for potential reuse as stale fallback on upstream failure
		staleCache = cached
	}

	slog.Debug("cache miss", "component", "handler", "slug", slug)
	h.recordCacheOutcome(slug, "miss")

	// Circuit breaker check — if open, serve stale or return 503
	if h.circuit != nil && h.circuit.IsOpen(slug) {
		slog.Warn("circuit open — rejecting upstream fetch", "component", "handler", "slug", slug)
		if h.metrics != nil {
			h.metrics.CircuitRejectionsTotal.WithLabelValues(slug).Inc()
			h.metrics.CircuitState.WithLabelValues(slug).Set(2) // open
		}

		// Try stale cache fallback
		cached, cacheErr := h.repo.Get(cacheKey)
		if cacheErr == nil && cached != nil {
			h.recordCacheOutcome(slug, "stale")
			respondWithCached(w, cached, true, "circuit_open")
			h.recordHTTPMetrics(slug, r.Method, http.StatusOK, start)
			return
		}

		// No cache — return 503
		retryAfter := int(h.circuit.OpenDuration().Seconds())
		h.recordHTTPMetrics(slug, r.Method, http.StatusServiceUnavailable, start)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(struct {
			Error      string `json:"error"`
			Slug       string `json:"slug"`
			RetryAfter int    `json:"retry_after"`
		}{
			Error:      "upstream circuit open",
			Slug:       slug,
			RetryAfter: retryAfter,
		})
		return
	}

	// 3. Wrap upstream fetch in singleflight to deduplicate concurrent requests
	// Circuit breaker wraps the upstream fetch call inside singleflight
	type sfResult struct {
		resp *model.CachedResponse
		err  error
	}

	v, _, shared := h.sfGroup.Do(cacheKey, func() (interface{}, error) {
		slog.Info("fetch started", "component", "handler", "slug", slug)
		fetchStart := time.Now()
		result, fetchErr := h.fetcher.Fetch(ep, pathParams)
		fetchDuration := time.Since(fetchStart).Seconds()

		if fetchErr != nil {
			slog.Error("upstream fetch failed", "component", "handler", "slug", slug, "error", fetchErr)
			h.recordUpstreamMetrics(slug, fetchDuration, 0, true)
			// Forget on error so errors are not shared/cached
			h.sfGroup.Forget(cacheKey)
			return &sfResult{nil, fetchErr}, nil
		}

		slog.Info("fetch completed", "component", "handler", "slug", slug, "pages", result.PageCount)
		h.recordUpstreamMetrics(slug, fetchDuration, result.PageCount, false)

		// Store in MongoDB cache
		if err := h.repo.Upsert(result); err != nil {
			slog.Error("cache upsert failed", "component", "handler", "slug", slug, "error", err)
		}

		// 4. Populate LRU after successful response
		h.populateLRU(cacheKey, result, ep)

		return &sfResult{result, nil}, nil
	})

	// Record singleflight dedup metric
	if shared {
		h.recordSingleflightDedup(slug)
	}

	sr := v.(*sfResult)

	if sr.err != nil {
		// Try stale cache fallback. Reuse the earlier MongoDB lookup when available
		// (normal cache-miss path), otherwise query MongoDB (force-refresh path).
		fallback := staleCache
		if fallback == nil {
			fallback, _ = h.repo.Get(cacheKey)
		}
		if fallback != nil {
			slog.Warn("serving stale cache — upstream unavailable", "component", "handler", "slug", slug)
			h.recordCacheOutcome(slug, "stale")
			respondWithCached(w, fallback, true, "upstream unavailable")
			slog.Debug("request", "component", "handler", "method", r.Method, "path", r.URL.Path, "slug", slug, "status", 200, "cache", "stale", "duration", time.Since(start))
			h.recordHTTPMetrics(slug, r.Method, http.StatusOK, start)
			return
		}

		// No cache, return 502
		slog.Debug("request", "component", "handler", "method", r.Method, "path", r.URL.Path, "slug", slug, "status", 502, "cache", "miss", "duration", time.Since(start))
		h.recordHTTPMetrics(slug, r.Method, http.StatusBadGateway, start)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(model.ErrorResponse{
			Error: "upstream unavailable",
			Slug:  slug,
		})
		return
	}

	// Return fresh result
	respondWithCached(w, sr.resp, false, "")
	slog.Debug("request", "component", "handler", "method", r.Method, "path", r.URL.Path, "slug", slug, "status", 200, "cache", "miss", "duration", time.Since(start))
	h.recordHTTPMetrics(slug, r.Method, http.StatusOK, start)
}

func (h *CacheHandler) recordHTTPMetrics(slug, method string, statusCode int, start time.Time) {
	if h.metrics == nil {
		return
	}
	h.metrics.HTTPRequestsTotal.WithLabelValues(slug, method, strconv.Itoa(statusCode)).Inc()
	h.metrics.HTTPRequestDuration.WithLabelValues(slug, method).Observe(time.Since(start).Seconds())
}

func (h *CacheHandler) recordCacheOutcome(slug, outcome string) {
	if h.metrics == nil {
		return
	}
	h.metrics.CacheHitsTotal.WithLabelValues(slug, outcome).Inc()
}

func (h *CacheHandler) recordUpstreamMetrics(slug string, duration float64, pageCount int, isError bool) {
	if h.metrics == nil {
		return
	}
	h.metrics.UpstreamFetchDuration.WithLabelValues(slug).Observe(duration)
	if isError {
		h.metrics.UpstreamFetchErrors.WithLabelValues(slug).Inc()
	}
	if pageCount > 0 {
		h.metrics.UpstreamFetchPages.WithLabelValues(slug).Add(float64(pageCount))
	}
}

func (h *CacheHandler) populateLRU(cacheKey string, resp *model.CachedResponse, ep config.Endpoint) {
	if h.lru == nil {
		return
	}
	ttl := ep.TTLDuration
	if ttl == 0 {
		ttl = 5 * time.Minute // default LRU TTL
	}
	// Use shorter TTL for empty results to allow re-checking upstream sooner
	if isEmptyResult(resp) {
		ttl = 2 * time.Minute
	}
	h.lru.Set(cacheKey, resp, ttl)
	h.updateLRUSizeMetric()
}

// isEmptyResult returns true if the cached response has a valid but empty data array.
func isEmptyResult(resp *model.CachedResponse) bool {
	if resp == nil {
		return false
	}
	if dataArr, ok := resp.Data.([]interface{}); ok {
		return len(dataArr) == 0
	}
	return false
}

func (h *CacheHandler) updateLRUSizeMetric() {
	if h.metrics == nil || h.lru == nil {
		return
	}
	h.metrics.LRUCacheSizeBytes.Set(float64(h.lru.SizeBytes()))
}

func (h *CacheHandler) recordLRUHit(slug string) {
	if h.metrics == nil {
		return
	}
	h.metrics.LRUCacheHitsTotal.WithLabelValues(slug).Inc()
}

func (h *CacheHandler) recordLRUMiss(slug string) {
	if h.metrics == nil {
		return
	}
	h.metrics.LRUCacheMissesTotal.WithLabelValues(slug).Inc()
}

func (h *CacheHandler) recordSingleflightDedup(slug string) {
	if h.metrics == nil {
		return
	}
	h.metrics.SingleflightDedupTotal.WithLabelValues(slug).Inc()
}

// backgroundRevalidate spawns a goroutine to refresh the cache in the background.
func (h *CacheHandler) backgroundRevalidate(ep config.Endpoint, params map[string]string) {
	if h.fetchLocks == nil {
		return
	}

	go func() {
		mu := h.fetchLocks.Get(ep.Slug)
		if !mu.TryLock() {
			slog.Debug("skipping background revalidation — fetch already in progress", "component", "handler", "slug", ep.Slug)
			if h.metrics != nil {
				h.metrics.FetchLockDedupTotal.WithLabelValues(ep.Slug).Inc()
			}
			return
		}
		defer mu.Unlock()

		fetchStart := time.Now()
		result, err := h.fetcher.Fetch(ep, params)
		fetchDuration := time.Since(fetchStart).Seconds()

		if err != nil {
			slog.Error("background revalidation failed", "component", "handler", "slug", ep.Slug, "error", err)
			h.recordUpstreamMetrics(ep.Slug, fetchDuration, 0, true)
			return
		}

		h.recordUpstreamMetrics(ep.Slug, fetchDuration, result.PageCount, false)

		if err := h.repo.Upsert(result); err != nil {
			slog.Error("background upsert failed", "component", "handler", "slug", ep.Slug, "error", err)
		}
	}()
}

func respondWithCached(w http.ResponseWriter, cached *model.CachedResponse, stale bool, staleReason string) {
	// Non-JSON responses: serve raw body directly with original content type
	if cached.ContentType != "" && cached.ContentType != "application/json" {
		w.Header().Set("Content-Type", cached.ContentType)
		if stale {
			w.Header().Set("X-Cache-Stale", "true")
			if staleReason != "" {
				w.Header().Set("X-Cache-Stale-Reason", staleReason)
			}
		}
		w.WriteHeader(http.StatusOK)
		if xmlStr, ok := cached.Data.(string); ok {
			fmt.Fprint(w, xmlStr)
		}
		return
	}

	// JSON responses: wrap in APIResponse envelope
	// Count results for the meta field
	resultsCount := 0
	data := cached.Data
	if dataArr, ok := data.([]interface{}); ok {
		resultsCount = len(dataArr)
		// Ensure empty data is an empty array, not nil
		if dataArr == nil {
			data = []interface{}{}
		}
	}

	resp := model.APIResponse{
		Data: data,
		Meta: model.ResponseMeta{
			Slug:         cached.Slug,
			SourceURL:    cached.SourceURL,
			FetchedAt:    cached.FetchedAt.Format("2006-01-02T15:04:05Z"),
			PageCount:    cached.PageCount,
			ResultsCount: resultsCount,
			Stale:        stale,
		},
	}
	if stale && staleReason != "" {
		resp.Meta.StaleReason = staleReason
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

func extractSlug(path string) string {
	// Path format: /api/cache/{slug}
	parts := strings.Split(strings.TrimPrefix(path, "/"), "/")
	if len(parts) >= 3 {
		return parts[2]
	}
	return ""
}

func extractPathParams(ep config.Endpoint, r *http.Request) map[string]string {
	paramNames := config.ExtractAllParams(ep)
	if len(paramNames) == 0 {
		return nil
	}

	params := make(map[string]string)
	for _, name := range paramNames {
		val := r.URL.Query().Get(name)
		if val != "" {
			params[name] = val
		}
	}

	if len(params) == 0 {
		return nil
	}
	return params
}
