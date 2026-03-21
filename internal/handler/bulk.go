package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/finish06/cash-drugs/internal/middleware"
	"github.com/finish06/cash-drugs/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

const (
	maxBulkQueries    = 100
	bulkSemaphoreCap  = 10
)

// BulkHandler handles POST /api/cache/{slug}/bulk requests.
type BulkHandler struct {
	endpoints map[string]config.Endpoint
	repo      cache.Repository
	metrics   *metrics.Metrics
	lru       cache.LRUCache
}

// BulkOption configures a BulkHandler.
type BulkOption func(*BulkHandler)

// WithBulkMetrics enables Prometheus metric instrumentation on the bulk handler.
func WithBulkMetrics(m *metrics.Metrics) BulkOption {
	return func(h *BulkHandler) {
		h.metrics = m
	}
}

// WithBulkLRU sets the in-memory LRU cache for the bulk handler.
func WithBulkLRU(lru cache.LRUCache) BulkOption {
	return func(h *BulkHandler) {
		h.lru = lru
	}
}

// NewBulkHandler creates a new BulkHandler.
func NewBulkHandler(endpoints []config.Endpoint, repo cache.Repository, opts ...BulkOption) *BulkHandler {
	epMap := make(map[string]config.Endpoint, len(endpoints))
	for _, ep := range endpoints {
		epMap[ep.Slug] = ep
	}
	h := &BulkHandler{
		endpoints: epMap,
		repo:      repo,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// bulkQueryRequest is the request body for POST /api/cache/{slug}/bulk.
type bulkQueryRequest struct {
	Queries []bulkQueryItem `json:"queries"`
}

type bulkQueryItem struct {
	Params map[string]string `json:"params"`
}

// BulkQueryResponse is the response envelope for bulk queries.
type BulkQueryResponse struct {
	Slug       string            `json:"slug"`
	Results    []BulkQueryResult `json:"results"`
	Total      int               `json:"total_queries"`
	Hits       int               `json:"hits"`
	Misses     int               `json:"misses"`
	Errors     int               `json:"errors"`
	DurationMs int64             `json:"duration_ms"`
	RequestID  string            `json:"request_id"`
}

// BulkQueryResult is a single result entry within a bulk response.
type BulkQueryResult struct {
	Index  int                `json:"index"`
	Status string             `json:"status"` // "hit", "miss", "error"
	Params map[string]string  `json:"params"`
	Data   interface{}        `json:"data"`
	Meta   *model.ResponseMeta `json:"meta"`
	Error  string             `json:"error,omitempty"`
}

// ServeHTTP handles incoming bulk cache requests.
//
// @Summary      Bulk cache lookup
// @Description  Looks up multiple cache keys in a single request. Cache-only — does not trigger upstream fetches.
// @Tags         cache
// @Accept       json
// @Produce      json
// @Param        slug  path  string  true  "Endpoint slug from config"
// @Param        body  body  bulkQueryRequest  true  "Batch of queries"
// @Success      200  {object}  BulkQueryResponse
// @Failure      400  {object}  model.ErrorResponse
// @Failure      404  {object}  model.ErrorResponse
// @Router       /api/cache/{slug}/bulk [post]
func (h *BulkHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	slug := extractBulkSlug(r.URL.Path)
	requestID := middleware.RequestID(r.Context())

	ep, ok := h.endpoints[slug]
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(model.ErrorResponse{
			Error:     "endpoint not configured",
			ErrorCode: model.ErrCodeEndpointNotFound,
			Slug:      slug,
			RequestID: requestID,
		})
		return
	}

	var req bulkQueryRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(model.ErrorResponse{
			Error:     "invalid request body",
			ErrorCode: model.ErrCodeBadRequest,
			RequestID: requestID,
		})
		return
	}

	if len(req.Queries) == 0 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(BulkQueryResponse{
			Slug:       slug,
			Results:    []BulkQueryResult{},
			Total:      0,
			Hits:       0,
			Misses:     0,
			Errors:     0,
			DurationMs: time.Since(start).Milliseconds(),
			RequestID:  requestID,
		})
		return
	}

	if len(req.Queries) > maxBulkQueries {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(model.ErrorResponse{
			Error:     "batch size exceeds limit of 100",
			ErrorCode: model.ErrCodeBadRequest,
			RequestID: requestID,
		})
		return
	}

	// Record batch size metric
	if h.metrics != nil {
		h.metrics.BulkRequestSize.WithLabelValues(slug).Observe(float64(len(req.Queries)))
	}

	// Concurrent cache lookups with semaphore
	results := make([]BulkQueryResult, len(req.Queries))
	sem := make(chan struct{}, bulkSemaphoreCap)
	var wg sync.WaitGroup

	for i, q := range req.Queries {
		wg.Add(1)
		go func(idx int, query bulkQueryItem) {
			defer wg.Done()
			sem <- struct{}{}        // acquire
			defer func() { <-sem }() // release

			result := h.lookupSingle(slug, ep, query.Params)
			result.Index = idx
			results[idx] = result
		}(i, q)
	}

	wg.Wait()

	// Tally hits/misses/errors
	var hits, misses, errors int
	for _, r := range results {
		switch r.Status {
		case "hit":
			hits++
		case "miss":
			misses++
		case "error":
			errors++
		}
	}

	duration := time.Since(start)

	// Record duration metric
	if h.metrics != nil {
		h.metrics.BulkRequestDuration.WithLabelValues(slug).Observe(duration.Seconds())
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(BulkQueryResponse{
		Slug:       slug,
		Results:    results,
		Total:      len(results),
		Hits:       hits,
		Misses:     misses,
		Errors:     errors,
		DurationMs: duration.Milliseconds(),
		RequestID:  requestID,
	})
}

// lookupSingle performs a single cache lookup for a bulk query item.
func (h *BulkHandler) lookupSingle(slug string, ep config.Endpoint, params map[string]string) BulkQueryResult {
	cacheKey := cache.BuildCacheKey(slug, params)

	// Try LRU first
	if h.lru != nil {
		if lruResult, ok := h.lru.Get(cacheKey); ok && !lruResult.NotFound {
			slog.Debug("bulk LRU hit", "component", "bulk-handler", "slug", slug, "key", cacheKey)
			if h.metrics != nil {
				h.metrics.CacheHitsTotal.WithLabelValues(slug, "hit").Inc()
			}
			return buildHitResult(params, lruResult, ep)
		}
	}

	// Try MongoDB
	cached, err := h.repo.Get(cacheKey)
	if err != nil {
		slog.Error("bulk cache lookup error", "component", "bulk-handler", "slug", slug, "key", cacheKey, "error", err)
		if h.metrics != nil {
			h.metrics.CacheHitsTotal.WithLabelValues(slug, "miss").Inc()
		}
		return BulkQueryResult{
			Status: "error",
			Params: params,
			Error:  "cache lookup failed",
		}
	}

	if cached == nil || cached.NotFound {
		if h.metrics != nil {
			h.metrics.CacheHitsTotal.WithLabelValues(slug, "miss").Inc()
		}
		return BulkQueryResult{
			Status: "miss",
			Params: params,
			Error:  "not cached",
		}
	}

	if h.metrics != nil {
		h.metrics.CacheHitsTotal.WithLabelValues(slug, "hit").Inc()
	}

	return buildHitResult(params, cached, ep)
}

// buildHitResult constructs a BulkQueryResult for a cache hit.
func buildHitResult(params map[string]string, cached *model.CachedResponse, ep config.Endpoint) BulkQueryResult {
	// Count results
	resultsCount := 0
	data := cached.Data
	switch d := data.(type) {
	case []interface{}:
		resultsCount = len(d)
		if d == nil {
			data = []interface{}{}
		}
	case bson.A:
		resultsCount = len(d)
		data = []interface{}(d)
	default:
		if data != nil {
			resultsCount = 1
		}
	}

	stale := config.IsStale(ep, cached.FetchedAt)

	meta := &model.ResponseMeta{
		Slug:         cached.Slug,
		SourceURL:    cached.SourceURL,
		FetchedAt:    cached.FetchedAt.Format("2006-01-02T15:04:05Z"),
		PageCount:    cached.PageCount,
		ResultsCount: resultsCount,
		Stale:        stale,
	}

	return BulkQueryResult{
		Status: "hit",
		Params: params,
		Data:   data,
		Meta:   meta,
	}
}

// extractBulkSlug extracts the slug from a bulk endpoint path.
// Path format: /api/cache/{slug}/bulk
func extractBulkSlug(path string) string {
	trimmed := strings.TrimPrefix(path, "/api/cache/")
	trimmed = strings.TrimSuffix(trimmed, "/bulk")
	// Handle any trailing slashes
	trimmed = strings.TrimSuffix(trimmed, "/")
	return trimmed
}
