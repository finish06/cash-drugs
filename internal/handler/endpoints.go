package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
)

// ParamInfo describes a single parameter for an endpoint.
type ParamInfo struct {
	Name     string `json:"name"`
	Type     string `json:"type"`     // "path", "query", or "search"
	Required bool   `json:"required"`
	Example  string `json:"example,omitempty"`
}

// CacheStatusInfo describes the cache state for an endpoint.
type CacheStatusInfo struct {
	Cached        bool   `json:"cached"`
	LastRefreshed string `json:"last_refreshed,omitempty"`
	IsStale       bool   `json:"is_stale"`
}

// EndpointInfo describes a configured endpoint for the discovery API.
type EndpointInfo struct {
	Slug        string           `json:"slug"`
	Path        string           `json:"path"`
	Format      string           `json:"format"`
	Params      []ParamInfo      `json:"params,omitempty"`
	Pagination  bool             `json:"pagination"`
	Scheduled   bool             `json:"scheduled"`
	Schedule    string           `json:"schedule,omitempty"`
	TTL         string           `json:"ttl,omitempty"`
	ExampleURL  string           `json:"example_url"`
	CacheStatus *CacheStatusInfo `json:"cache_status,omitempty"`
}

// EndpointsHandler handles GET /api/endpoints requests.
type EndpointsHandler struct {
	configEps []config.Endpoint
	repo      cache.Repository
}

// NewEndpointsHandler creates a new EndpointsHandler from config endpoints.
func NewEndpointsHandler(endpoints []config.Endpoint, repo cache.Repository) *EndpointsHandler {
	return &EndpointsHandler{
		configEps: endpoints,
		repo:      repo,
	}
}

// ServeHTTP returns the list of configured endpoints.
//
// @Summary      List configured endpoints
// @Description  Returns all configured upstream API endpoints with their metadata.
// @Tags         system
// @Produce      json
// @Success      200  {array}  EndpointInfo
// @Router       /api/endpoints [get]
func (h *EndpointsHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	infos := make([]EndpointInfo, 0, len(h.configEps))
	for _, ep := range h.configEps {
		infos = append(infos, h.buildEndpointInfo(ep))
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(infos)
}

func (h *EndpointsHandler) buildEndpointInfo(ep config.Endpoint) EndpointInfo {
	_, fetchAll := config.ParsePagination(ep)
	maxPages, _ := config.ParsePagination(ep)

	params := buildParamInfos(ep)
	info := EndpointInfo{
		Slug:       ep.Slug,
		Path:       ep.BaseURL + ep.Path,
		Format:     ep.Format,
		Params:     params,
		Pagination: fetchAll || maxPages > 1,
		Scheduled:  ep.Refresh != "",
		Schedule:   ep.Refresh,
		TTL:        ep.TTL,
		ExampleURL: buildExampleURL(ep, params),
	}

	if h.repo != nil {
		info.CacheStatus = h.buildCacheStatus(ep)
	}

	return info
}

// buildParamInfos constructs ParamInfo slice from the endpoint config.
func buildParamInfos(ep config.Endpoint) []ParamInfo {
	var params []ParamInfo
	seen := make(map[string]bool)

	// Path params from ep.Path — required
	for _, p := range config.ExtractPathParams(ep.Path) {
		if !seen[p] {
			seen[p] = true
			params = append(params, ParamInfo{
				Name:     p,
				Type:     "path",
				Required: true,
				Example:  "example",
			})
		}
	}

	// Query params from ep.QueryParams values — extract {PARAM} placeholders
	for _, v := range ep.QueryParams {
		for _, p := range config.ExtractPathParams(v) {
			if !seen[p] {
				seen[p] = true
				params = append(params, ParamInfo{
					Name:     p,
					Type:     "query",
					Required: false,
					Example:  "example",
				})
			}
		}
	}

	// Search params
	for _, sp := range ep.SearchParams {
		// SearchParams may be raw names or {PARAM} templates
		for _, p := range config.ExtractPathParams(sp) {
			if !seen[p] {
				seen[p] = true
				params = append(params, ParamInfo{
					Name:     p,
					Type:     "search",
					Required: false,
					Example:  "example",
				})
			}
		}
		// If the search param is a plain name (no braces), use it directly
		if !strings.Contains(sp, "{") {
			if !seen[sp] {
				seen[sp] = true
				params = append(params, ParamInfo{
					Name:     sp,
					Type:     "search",
					Required: false,
					Example:  "example",
				})
			}
		}
	}

	return params
}

// buildExampleURL constructs an example URL for the cache proxy endpoint.
func buildExampleURL(ep config.Endpoint, params []ParamInfo) string {
	base := fmt.Sprintf("/api/cache/%s", ep.Slug)
	if len(params) == 0 {
		return base
	}

	// Only include non-path params as query params in the example URL.
	// Path params would be part of the slug routing, but our proxy uses
	// query params for all param types.
	var queryParts []string
	for _, p := range params {
		queryParts = append(queryParts, fmt.Sprintf("%s=example", p.Name))
	}

	if len(queryParts) == 0 {
		return base
	}
	return base + "?" + strings.Join(queryParts, "&")
}

// buildCacheStatus looks up the cache state for the endpoint's base slug.
func (h *EndpointsHandler) buildCacheStatus(ep config.Endpoint) *CacheStatusInfo {
	fetchedAt, found, err := h.repo.FetchedAt(ep.Slug)
	if err != nil || !found {
		return &CacheStatusInfo{
			Cached:  false,
			IsStale: true,
		}
	}

	status := &CacheStatusInfo{
		Cached:        true,
		LastRefreshed: fetchedAt.UTC().Format(time.RFC3339),
		IsStale:       config.IsStale(ep, fetchedAt),
	}
	return status
}
