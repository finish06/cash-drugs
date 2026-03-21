package handler

import (
	"encoding/json"
	"errors"
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
		h.recordErrorMetric(model.ErrCodeEndpointNotFound, slug)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(model.ErrorResponse{
			Error:     "endpoint not configured",
			ErrorCode: model.ErrCodeEndpointNotFound,
			Slug:      slug,
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
			// Check for negative cache entry (upstream 404)
			if lruResult.NotFound {
				slog.Debug("LRU negative cache hit", "component", "handler", "slug", slug)
				h.recordLRUHit(slug)
				h.respondNotFound(w, slug, pathParams, start, r.Method)
				return
			}
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
			// Check for negative cache entry (upstream 404)
			if cached.NotFound {
				// Check if negative cache has expired (10-min TTL)
				if time.Since(cached.FetchedAt) <= 10*time.Minute {
					slog.Debug("MongoDB negative cache hit", "component", "handler", "slug", slug)
					h.recordCacheOutcome(slug, "hit")
					h.respondNotFound(w, slug, pathParams, start, r.Method)
					return
				}
				slog.Debug("negative cache expired", "component", "handler", "slug", slug)
				// Fall through to upstream fetch
			} else {
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
		h.recordErrorMetric(model.ErrCodeCircuitOpen, slug)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_ = json.NewEncoder(w).Encode(model.ErrorResponse{
			Error:      "upstream circuit open",
			ErrorCode:  model.ErrCodeCircuitOpen,
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
		// Check for upstream 404 — return 404 to consumer, store negative cache entry
		var notFoundErr *upstream.ErrUpstreamNotFound
		if errors.As(sr.err, &notFoundErr) {
			// Store negative cache entry in MongoDB
			negEntry := &model.CachedResponse{
				Slug:      slug,
				Params:    pathParams,
				CacheKey:  cacheKey,
				NotFound:  true,
				FetchedAt: time.Now(),
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			_ = h.repo.Upsert(negEntry)
			// Store in LRU with 10-min TTL
			if h.lru != nil {
				h.lru.Set(cacheKey, negEntry, 10*time.Minute)
			}
			if h.metrics != nil {
				h.metrics.Upstream404Total.WithLabelValues(slug).Inc()
			}
			h.respondNotFound(w, slug, pathParams, start, r.Method)
			return
		}

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
		h.recordErrorMetric(model.ErrCodeUpstreamUnavailable, slug)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(model.ErrorResponse{
			Error:     "upstream unavailable",
			ErrorCode: model.ErrCodeUpstreamUnavailable,
			Slug:      slug,
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

func (h *CacheHandler) recordErrorMetric(code, slug string) {
	if h.metrics == nil {
		return
	}
	h.metrics.ErrorsTotal.WithLabelValues(code, slug).Inc()
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

// maxLRUPages is the page count threshold above which responses are too large
// for in-memory LRU caching. These are served directly from MongoDB instead.
const maxLRUPages = 10

func (h *CacheHandler) populateLRU(cacheKey string, resp *model.CachedResponse, ep config.Endpoint) {
	if h.lru == nil {
		return
	}
	// Skip LRU for large multi-page responses to prevent memory bloat.
	// Bulk endpoints (e.g., /spls with 200 pages) can be 50-100MB and would
	// fill the entire LRU cache with a single entry.
	if resp.PageCount > maxLRUPages {
		slog.Debug("skipping LRU — response too large", "component", "handler", "slug", resp.Slug, "pages", resp.PageCount)
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
// Skips bulk endpoints (scheduled refresh handles those) to avoid heavy memory usage.
func (h *CacheHandler) backgroundRevalidate(ep config.Endpoint, params map[string]string) {
	if h.fetchLocks == nil {
		return
	}

	// Skip background revalidation for scheduled endpoints without parameters —
	// the scheduler already handles refresh. Background revalidation of bulk
	// endpoints (200 pages, 50MB+) causes unnecessary memory spikes.
	if ep.Refresh != "" && len(params) == 0 {
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

// respondNotFound sends a 404 JSON response for upstream-not-found or negative-cache scenarios.
func (h *CacheHandler) respondNotFound(w http.ResponseWriter, slug string, params map[string]string, start time.Time, method string) {
	h.recordHTTPMetrics(slug, method, http.StatusNotFound, start)
	h.recordErrorMetric(model.ErrCodeUpstreamNotFound, slug)
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusNotFound)
	_ = json.NewEncoder(w).Encode(model.ErrorResponse{
		Error:     "not found",
		ErrorCode: model.ErrCodeUpstreamNotFound,
		Slug:      slug,
		Params:    params,
	})
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
			_, _ = fmt.Fprint(w, xmlStr)
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
	_ = json.NewEncoder(w).Encode(resp)
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
