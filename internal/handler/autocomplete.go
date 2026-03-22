package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/middleware"
	"github.com/finish06/cash-drugs/internal/model"
)

// AutocompleteResponse is the envelope returned by GET /api/autocomplete.
type AutocompleteResponse struct {
	Query       string   `json:"query"`
	Suggestions []string `json:"suggestions"`
	RequestID   string   `json:"request_id"`
}

// AutocompleteHandler handles GET /api/autocomplete?q=prefix&limit=10.
type AutocompleteHandler struct {
	repo  cache.Repository
	slugs []string
}

// NewAutocompleteHandler creates a new AutocompleteHandler.
// slugs specifies which cached slugs to search for drug name suggestions.
func NewAutocompleteHandler(repo cache.Repository, slugs []string) *AutocompleteHandler {
	return &AutocompleteHandler{
		repo:  repo,
		slugs: slugs,
	}
}

// ServeHTTP handles autocomplete requests.
//
// @Summary      Autocomplete drug names
// @Description  Returns prefix-matched drug name suggestions from cached data.
// @Tags         search
// @Produce      json
// @Param        q      query     string  true   "Search prefix (min 1 character)"
// @Param        limit  query     int     false  "Max suggestions (default 10, max 50)"
// @Success      200  {object}  AutocompleteResponse
// @Failure      400  {object}  model.ErrorResponse
// @Router       /api/autocomplete [get]
func (h *AutocompleteHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestID := middleware.RequestID(r.Context())

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(model.ErrorResponse{
			Error:     "query parameter 'q' is required",
			ErrorCode: model.ErrCodeBadRequest,
			RequestID: requestID,
		})
		return
	}

	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > 50 {
		limit = 50
	}

	qLower := strings.ToLower(q)
	var allNames []string

	for _, slug := range h.slugs {
		cached, err := h.repo.Get(slug)
		if err != nil || cached == nil {
			continue
		}

		items := extractDataItems(cached.Data)
		for _, item := range items {
			names := extractNameFields(item)
			for _, name := range names {
				if strings.HasPrefix(strings.ToLower(name), qLower) {
					allNames = append(allNames, name)
				}
			}
		}
	}

	// Deduplicate, sort, and cap at limit
	allNames = deduplicateStrings(allNames)
	sort.Strings(allNames)
	if len(allNames) > limit {
		allNames = allNames[:limit]
	}

	if allNames == nil {
		allNames = []string{}
	}

	duration := time.Since(start)
	slog.Debug("autocomplete completed", "component", "autocomplete", "query", q, "suggestions", len(allNames), "duration", duration)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(AutocompleteResponse{
		Query:       q,
		Suggestions: allNames,
		RequestID:   requestID,
	})
}
