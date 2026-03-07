package handler

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"

	"github.com/finish06/drugs/internal/cache"
	"github.com/finish06/drugs/internal/config"
	"github.com/finish06/drugs/internal/model"
	"github.com/finish06/drugs/internal/upstream"
)

// CacheHandler handles GET /api/cache/{slug} requests.
type CacheHandler struct {
	endpoints  map[string]config.Endpoint
	repo       cache.Repository
	fetcher    upstream.Fetcher
	fetchLocks *FetchLocks
}

// Option configures a CacheHandler.
type Option func(*CacheHandler)

// WithFetchLocks sets the shared fetch locks for deduplication with the scheduler.
func WithFetchLocks(fl *FetchLocks) Option {
	return func(h *CacheHandler) {
		h.fetchLocks = fl
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
	slug := extractSlug(r.URL.Path)

	ep, ok := h.endpoints[slug]
	if !ok {
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
				// Serve stale immediately, trigger background revalidation
				respondWithCached(w, cached, true, "ttl_expired")
				h.backgroundRevalidate(ep, pathParams)
				return
			}
			respondWithCached(w, cached, false, "")
			return
		}
	}

	// Fetch from upstream
	result, fetchErr := h.fetcher.Fetch(ep, pathParams)
	if fetchErr != nil {
		// Try stale cache fallback
		cached, cacheErr := h.repo.Get(cacheKey)
		if cacheErr == nil && cached != nil {
			respondWithCached(w, cached, true, "upstream unavailable")
			return
		}

		// No cache, return 502
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(model.ErrorResponse{
			Error: "upstream unavailable",
			Slug:  slug,
		})
		return
	}

	// Store in cache
	h.repo.Upsert(result)

	// Return fresh result
	respondWithCached(w, result, false, "")
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
			return
		}
		defer mu.Unlock()

		result, err := h.fetcher.Fetch(ep, params)
		if err != nil {
			slog.Error("background revalidation failed", "component", "handler", "slug", ep.Slug, "error", err)
			return
		}

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
