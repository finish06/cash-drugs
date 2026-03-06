package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/finish06/drugs/internal/cache"
	"github.com/finish06/drugs/internal/config"
	"github.com/finish06/drugs/internal/model"
	"github.com/finish06/drugs/internal/upstream"
)

// CacheHandler handles GET /api/cache/{slug} requests.
type CacheHandler struct {
	endpoints map[string]config.Endpoint
	repo      cache.Repository
	fetcher   upstream.Fetcher
}

// NewCacheHandler creates a new CacheHandler.
func NewCacheHandler(endpoints []config.Endpoint, repo cache.Repository, fetcher upstream.Fetcher) *CacheHandler {
	epMap := make(map[string]config.Endpoint, len(endpoints))
	for _, ep := range endpoints {
		epMap[ep.Slug] = ep
	}
	return &CacheHandler{
		endpoints: epMap,
		repo:      repo,
		fetcher:   fetcher,
	}
}

// ServeHTTP handles incoming cache requests.
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
			respondWithCached(w, cached, false)
			return
		}
	}

	// Fetch from upstream
	result, fetchErr := h.fetcher.Fetch(ep, pathParams)
	if fetchErr != nil {
		// Try stale cache fallback
		cached, cacheErr := h.repo.Get(cacheKey)
		if cacheErr == nil && cached != nil {
			respondWithCached(w, cached, true)
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
	respondWithCached(w, result, false)
}

func respondWithCached(w http.ResponseWriter, cached *model.CachedResponse, stale bool) {
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
	if stale {
		resp.Meta.StaleReason = "upstream unavailable"
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
	paramNames := config.ExtractPathParams(ep.Path)
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
