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
)

// CacheHandler handles GET /api/cache/{slug} requests.
type CacheHandler struct {
	endpoints  map[string]config.Endpoint
	repo       cache.Repository
	fetcher    upstream.Fetcher
	fetchLocks *FetchLocks
	metrics    *metrics.Metrics
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
// @Tags         cache
// @Produce      json
// @Param        slug   path      string  true   "Endpoint slug from config"
// @Param        SETID  query     string  false  "Path parameter value (varies by endpoint)"
// @Param        _force query     string  false  "Force upstream refresh (true/false)"
// @Success      200  {object}  model.APIResponse
// @Failure      404  {object}  model.ErrorResponse
// @Failure      502  {object}  model.ErrorResponse
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

	// Try cache first (unless force refresh)
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
			respondWithCached(w, cached, false, "")
			slog.Debug("request", "component", "handler", "method", r.Method, "path", r.URL.Path, "slug", slug, "status", 200, "cache", "hit", "duration", time.Since(start))
			h.recordHTTPMetrics(slug, r.Method, http.StatusOK, start)
			return
		}
	}

	slog.Debug("cache miss", "component", "handler", "slug", slug)
	h.recordCacheOutcome(slug, "miss")

	// Fetch from upstream
	slog.Info("fetch started", "component", "handler", "slug", slug)
	fetchStart := time.Now()
	result, fetchErr := h.fetcher.Fetch(ep, pathParams)
	fetchDuration := time.Since(fetchStart).Seconds()

	if fetchErr != nil {
		slog.Error("upstream fetch failed", "component", "handler", "slug", slug, "error", fetchErr)
		h.recordUpstreamMetrics(slug, fetchDuration, 0, true)

		// Try stale cache fallback
		cached, cacheErr := h.repo.Get(cacheKey)
		if cacheErr == nil && cached != nil {
			slog.Warn("serving stale cache — upstream unavailable", "component", "handler", "slug", slug)
			h.recordCacheOutcome(slug, "stale")
			respondWithCached(w, cached, true, "upstream unavailable")
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

	slog.Info("fetch completed", "component", "handler", "slug", slug, "pages", result.PageCount)
	h.recordUpstreamMetrics(slug, fetchDuration, result.PageCount, false)

	// Store in cache
	if err := h.repo.Upsert(result); err != nil {
		slog.Error("cache upsert failed", "component", "handler", "slug", slug, "error", err)
	}

	// Return fresh result
	respondWithCached(w, result, false, "")
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
	resp := model.APIResponse{
		Data: cached.Data,
		Meta: model.ResponseMeta{
			Slug:      cached.Slug,
			SourceURL: cached.SourceURL,
			FetchedAt: cached.FetchedAt.Format("2006-01-02T15:04:05Z"),
			PageCount: cached.PageCount,
			Stale:     stale,
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
