package handler

import (
	"encoding/json"
	"net/http"

	"github.com/finish06/cash-drugs/internal/config"
)

// WarmupTrigger starts a background warm-up for the given slugs.
// If slugs is nil, all scheduled endpoints are warmed.
// If skipQueries is true, parameterized queries from warmup-queries.yaml are skipped.
type WarmupTrigger interface {
	TriggerWarmup(slugs []string, skipQueries bool)
}

// WarmupHandlerOption configures optional WarmupHandler behavior.
type WarmupHandlerOption func(*WarmupHandler)

// WithWarmupQueries sets the parameterized warmup queries for the handler.
func WithWarmupQueries(queries map[string][]map[string]string) WarmupHandlerOption {
	return func(h *WarmupHandler) {
		h.warmupQueries = queries
	}
}

// WarmupHandler handles POST /api/warmup requests.
type WarmupHandler struct {
	endpoints     []config.Endpoint
	trigger       WarmupTrigger
	slugSet       map[string]bool // valid slugs for validation
	warmupQueries map[string][]map[string]string
}

// NewWarmupHandler creates a new WarmupHandler.
func NewWarmupHandler(endpoints []config.Endpoint, trigger WarmupTrigger, opts ...WarmupHandlerOption) *WarmupHandler {
	slugSet := make(map[string]bool)
	for _, ep := range endpoints {
		slugSet[ep.Slug] = true
	}
	h := &WarmupHandler{
		endpoints:     endpoints,
		trigger:       trigger,
		slugSet:       slugSet,
		warmupQueries: make(map[string][]map[string]string),
	}
	for _, opt := range opts {
		opt(h)
	}
	return h
}

// ServeHTTP handles warmup trigger requests.
//
// @Summary      Trigger cache warmup
// @Description  Triggers background warm-up of cached endpoints. With no body, warms all scheduled endpoints. With {"slugs": [...]}, warms specific slugs.
// @Tags         system
// @Accept       json
// @Produce      json
// @Param        body  body  object  false  "Optional slug filter"
// @Success      202  {object}  map[string]interface{}  "Warmup started"
// @Failure      400  {object}  map[string]string       "Invalid slug"
// @Failure      405  {object}  map[string]string       "Method not allowed"
// @Router       /api/warmup [post]
func (h *WarmupHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_ = json.NewEncoder(w).Encode(map[string]string{
			"error": "method not allowed",
		})
		return
	}

	w.Header().Set("Content-Type", "application/json")

	// Parse optional request body
	var reqBody struct {
		Slugs       []string `json:"slugs"`
		SkipQueries bool     `json:"skip_queries"`
	}

	if r.Body != nil && r.ContentLength != 0 {
		if err := json.NewDecoder(r.Body).Decode(&reqBody); err != nil {
			// Treat decode errors as empty body (warm all)
			reqBody.Slugs = nil
		}
	}

	if len(reqBody.Slugs) > 0 {
		// Validate all slugs exist
		for _, slug := range reqBody.Slugs {
			if !h.slugSet[slug] {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{
					"error": "unknown slug",
					"slug":  slug,
				})
				return
			}
		}

		// Trigger warmup for specific slugs
		h.trigger.TriggerWarmup(reqBody.Slugs, reqBody.SkipQueries)
		queryCount := 0
		if !reqBody.SkipQueries {
			queryCount = QueryCountForSlugs(h.warmupQueries, reqBody.Slugs)
		}
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"status":          "accepted",
			"warming":         len(reqBody.Slugs),
			"warming_queries": queryCount,
		})
		return
	}

	// Warm all scheduled endpoints (those with a Refresh field)
	scheduled := h.scheduledSlugs()
	h.trigger.TriggerWarmup(nil, reqBody.SkipQueries)
	queryCount := 0
	if !reqBody.SkipQueries {
		queryCount = QueryCountForSlugs(h.warmupQueries, nil)
	}
	w.WriteHeader(http.StatusAccepted)
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"status":          "accepted",
		"warming":         len(scheduled),
		"warming_queries": queryCount,
	})
}

// scheduledSlugs returns slugs of endpoints that have a refresh schedule.
func (h *WarmupHandler) scheduledSlugs() []string {
	var slugs []string
	for _, ep := range h.endpoints {
		if ep.Refresh != "" {
			slugs = append(slugs, ep.Slug)
		}
	}
	return slugs
}
