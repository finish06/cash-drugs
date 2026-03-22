package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/middleware"
	"github.com/finish06/cash-drugs/internal/model"
	"go.mongodb.org/mongo-driver/v2/bson"
)

// SearchResult groups matching items from a single slug.
type SearchResult struct {
	Slug    string        `json:"slug"`
	Matches int           `json:"matches"`
	Items   []interface{} `json:"items"`
}

// SearchResponse is the envelope returned by GET /api/search.
type SearchResponse struct {
	Query        string         `json:"query"`
	TotalMatches int            `json:"total_matches"`
	Results      []SearchResult `json:"results"`
	DurationMS   float64        `json:"duration_ms"`
	RequestID    string         `json:"request_id"`
}

// SearchHandler handles GET /api/search?q=term&limit=50.
type SearchHandler struct {
	endpoints []config.Endpoint
	repo      cache.Repository
}

// NewSearchHandler creates a new SearchHandler.
func NewSearchHandler(endpoints []config.Endpoint, repo cache.Repository) *SearchHandler {
	return &SearchHandler{
		endpoints: endpoints,
		repo:      repo,
	}
}

// ServeHTTP handles search requests.
//
// @Summary      Search across all cached data
// @Description  Searches cached data across all slugs using case-insensitive string matching.
// @Tags         search
// @Produce      json
// @Param        q      query     string  true   "Search query (min 2 characters)"
// @Param        limit  query     int     false  "Max results per slug (default 50, max 200)"
// @Success      200  {object}  SearchResponse
// @Failure      400  {object}  model.ErrorResponse
// @Router       /api/search [get]
func (h *SearchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	requestID := middleware.RequestID(r.Context())

	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" || len(q) < 2 {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(model.ErrorResponse{
			Error:     "query parameter 'q' is required (min 2 characters)",
			ErrorCode: model.ErrCodeBadRequest,
			RequestID: requestID,
		})
		return
	}

	limit := 50
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if v, err := strconv.Atoi(limitStr); err == nil && v > 0 {
			limit = v
		}
	}
	if limit > 200 {
		limit = 200
	}

	qLower := strings.ToLower(q)
	var results []SearchResult
	totalMatches := 0

	for _, ep := range h.endpoints {
		// Only search slugs without required parameters (bulk cached data).
		// Parameterized endpoints have no cached data without specific params.
		if config.HasRequiredParams(ep) {
			continue
		}

		cached, err := h.repo.Get(ep.Slug)
		if err != nil || cached == nil {
			continue
		}

		items := extractDataItems(cached.Data)
		if len(items) == 0 {
			continue
		}

		var matched []interface{}
		for _, item := range items {
			if len(matched) >= limit {
				break
			}
			if itemMatchesQuery(item, qLower) {
				matched = append(matched, item)
			}
		}

		if len(matched) > 0 {
			results = append(results, SearchResult{
				Slug:    ep.Slug,
				Matches: len(matched),
				Items:   matched,
			})
			totalMatches += len(matched)
		}
	}

	if results == nil {
		results = []SearchResult{}
	}

	duration := time.Since(start)
	slog.Debug("search completed", "component", "search", "query", q, "total_matches", totalMatches, "duration", duration)

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(SearchResponse{
		Query:        q,
		TotalMatches: totalMatches,
		Results:      results,
		DurationMS:   float64(duration.Microseconds()) / 1000.0,
		RequestID:    requestID,
	})
}

// extractDataItems converts cached Data into a slice of items for iteration.
func extractDataItems(data interface{}) []interface{} {
	switch d := data.(type) {
	case []interface{}:
		return d
	case bson.A:
		return []interface{}(d)
	default:
		return nil
	}
}

// itemMatchesQuery converts an item to a JSON string and checks if it contains
// the query (case-insensitive). This is a pragmatic approach that searches all
// string values without knowing the schema.
func itemMatchesQuery(item interface{}, queryLower string) bool {
	b, err := json.Marshal(item)
	if err != nil {
		return false
	}
	return strings.Contains(strings.ToLower(string(b)), queryLower)
}

// extractNameFields pulls common name/title fields from a data item for autocomplete.
func extractNameFields(item interface{}) []string {
	m, ok := toStringMap(item)
	if !ok {
		return nil
	}

	// Common drug name field names across the various APIs
	nameKeys := []string{
		"drug_name", "drugname", "name", "title",
		"brand_name", "generic_name",
		"brand_name_base",
	}

	var names []string
	for _, key := range nameKeys {
		if val, ok := m[key]; ok {
			if s, ok := val.(string); ok && s != "" {
				names = append(names, s)
			}
		}
	}

	// Also check nested openfda object for FDA endpoints
	if openfda, ok := m["openfda"]; ok {
		if fdaMap, ok := toStringMap(openfda); ok {
			fdaNameKeys := []string{"brand_name", "generic_name"}
			for _, key := range fdaNameKeys {
				if val, ok := fdaMap[key]; ok {
					switch v := val.(type) {
					case []interface{}:
						for _, item := range v {
							if s, ok := item.(string); ok && s != "" {
								names = append(names, s)
							}
						}
					case string:
						if v != "" {
							names = append(names, v)
						}
					}
				}
			}
		}
	}

	return names
}

// toStringMap converts various map types to map[string]interface{}.
func toStringMap(v interface{}) (map[string]interface{}, bool) {
	switch m := v.(type) {
	case map[string]interface{}:
		return m, true
	case bson.M:
		return map[string]interface{}(m), true
	case bson.D:
		result := make(map[string]interface{}, len(m))
		for _, e := range m {
			result[e.Key] = e.Value
		}
		return result, true
	default:
		// Try to handle via JSON round-trip for other map-like types
		b, err := json.Marshal(v)
		if err != nil {
			return nil, false
		}
		var result map[string]interface{}
		if err := json.Unmarshal(b, &result); err != nil {
			return nil, false
		}
		if len(result) == 0 {
			return nil, false
		}
		return result, true
	}
}

// deduplicateStrings returns unique strings preserving first-seen order.
func deduplicateStrings(ss []string) []string {
	seen := make(map[string]struct{}, len(ss))
	result := make([]string, 0, len(ss))
	for _, s := range ss {
		if _, ok := seen[s]; !ok {
			seen[s] = struct{}{}
			result = append(result, s)
		}
	}
	return result
}

