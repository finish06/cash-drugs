package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/middleware"
	"github.com/finish06/cash-drugs/internal/model"
	"github.com/finish06/cash-drugs/internal/upstream"
	gobreaker "github.com/sony/gobreaker/v2"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// SlugMeta is the response for GET /api/cache/{slug}/_meta.
type SlugMeta struct {
	Slug          string  `json:"slug"`
	LastRefreshed *string `json:"last_refreshed"`          // ISO 8601 or null
	TTLRemaining  string  `json:"ttl_remaining"`           // duration string, "0s" if stale
	IsStale       bool    `json:"is_stale"`                // true if past TTL or never cached
	PageCount     int     `json:"page_count"`              // from cached response metadata
	RecordCount   int     `json:"record_count"`            // total items across all pages
	CircuitState  string  `json:"circuit_state"`           // "closed", "open", "half-open"
	HasSchedule   bool    `json:"has_schedule"`            // true if refresh cron is configured
	Schedule      *string `json:"schedule,omitempty"`      // cron expression or null
	HasParams     bool    `json:"has_params"`              // true if endpoint has parameterized paths
}

// MetaHandler handles GET /api/cache/{slug}/_meta requests.
type MetaHandler struct {
	endpoints map[string]config.Endpoint
	repo      cache.Repository
	circuit   *upstream.CircuitRegistry
}

// MetaOption configures a MetaHandler.
type MetaOption func(*MetaHandler)

// WithMetaCircuit sets the circuit breaker registry for the MetaHandler.
func WithMetaCircuit(c *upstream.CircuitRegistry) MetaOption {
	return func(h *MetaHandler) {
		h.circuit = c
	}
}

// NewMetaHandler creates a new MetaHandler.
func NewMetaHandler(endpoints []config.Endpoint, repo cache.Repository, opts ...MetaOption) *MetaHandler {
	epMap := make(map[string]config.Endpoint, len(endpoints))
	for _, ep := range endpoints {
		epMap[ep.Slug] = ep
	}
	h := &MetaHandler{
		endpoints: epMap,
		repo:      repo,
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// ServeHTTP returns per-slug cache metadata without the data payload.
//
// @Summary      Per-slug cache metadata
// @Description  Returns lightweight cache state (staleness, TTL, record count, circuit state) for a single slug without transferring the data payload.
// @Tags         cache
// @Produce      json
// @Param        slug  path      string  true  "Endpoint slug from config"
// @Success      200   {object}  SlugMeta
// @Failure      404   {object}  model.ErrorResponse
// @Router       /api/cache/{slug}/_meta [get]
func (h *MetaHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	slug := extractMetaSlug(r.URL.Path)

	ep, ok := h.endpoints[slug]
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(model.ErrorResponse{
			Error:     "endpoint not configured",
			ErrorCode: model.ErrCodeEndpointNotFound,
			Slug:      slug,
			RequestID: middleware.RequestID(r.Context()),
		})
		return
	}

	meta := SlugMeta{
		Slug:         slug,
		TTLRemaining: "0s",
		IsStale:      true,
		CircuitState: "closed",
		HasSchedule:  ep.Refresh != "",
		HasParams:    len(config.ExtractAllParams(ep)) > 0,
	}

	if ep.Refresh != "" {
		sched := ep.Refresh
		meta.Schedule = &sched
	}

	// Circuit state
	if h.circuit != nil {
		meta.CircuitState = circuitStateString(h.circuit.State(slug))
	}

	// Cache state
	fetchedAt, found, err := h.repo.FetchedAt(slug)
	if err == nil && found {
		ts := fetchedAt.UTC().Format(time.RFC3339)
		meta.LastRefreshed = &ts
		meta.IsStale = config.IsStale(ep, fetchedAt)

		if !meta.IsStale && ep.TTLDuration > 0 {
			remaining := ep.TTLDuration - time.Since(fetchedAt)
			if remaining < 0 {
				remaining = 0
			}
			meta.TTLRemaining = remaining.Truncate(time.Second).String()
		}

		// Get cached response for page_count and record_count
		cached, getErr := h.repo.Get(slug)
		if getErr == nil && cached != nil {
			meta.PageCount = cached.PageCount
			meta.RecordCount = countRecords(cached.Data)
		}
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(meta)
}

// extractMetaSlug extracts the slug from a /_meta path.
// Path format: /api/cache/{slug}/_meta
func extractMetaSlug(path string) string {
	trimmed := strings.TrimPrefix(path, "/api/cache/")
	trimmed = strings.TrimSuffix(trimmed, "/_meta")
	// Guard against paths with extra slashes
	if idx := strings.Index(trimmed, "/"); idx >= 0 {
		trimmed = trimmed[:idx]
	}
	return trimmed
}

// circuitStateString converts a gobreaker.State to a human-readable string.
func circuitStateString(s gobreaker.State) string {
	switch s {
	case gobreaker.StateOpen:
		return "open"
	case gobreaker.StateHalfOpen:
		return "half-open"
	default:
		return "closed"
	}
}

// countRecords returns the number of items in the cached data.
func countRecords(data interface{}) int {
	switch d := data.(type) {
	case []interface{}:
		return len(d)
	case bson.A:
		return len(d)
	default:
		return 0
	}
}
