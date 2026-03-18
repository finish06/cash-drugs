package handler

import (
	"encoding/json"
	"net/http"

	"github.com/finish06/cash-drugs/internal/config"
)

// EndpointInfo describes a configured endpoint for the discovery API.
type EndpointInfo struct {
	Slug       string   `json:"slug"`
	Path       string   `json:"path"`
	Format     string   `json:"format"`
	Params     []string `json:"params,omitempty"`
	Pagination bool     `json:"pagination"`
	Scheduled  bool     `json:"scheduled"`
}

// EndpointsHandler handles GET /api/endpoints requests.
type EndpointsHandler struct {
	endpoints []EndpointInfo
}

// NewEndpointsHandler creates a new EndpointsHandler from config endpoints.
func NewEndpointsHandler(endpoints []config.Endpoint) *EndpointsHandler {
	infos := make([]EndpointInfo, 0, len(endpoints))
	for _, ep := range endpoints {
		_, fetchAll := config.ParsePagination(ep)
		maxPages, _ := config.ParsePagination(ep)
		infos = append(infos, EndpointInfo{
			Slug:       ep.Slug,
			Path:       ep.BaseURL + ep.Path,
			Format:     ep.Format,
			Params:     config.ExtractAllParams(ep),
			Pagination: fetchAll || maxPages > 1,
			Scheduled:  ep.Refresh != "",
		})
	}
	return &EndpointsHandler{endpoints: infos}
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
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(h.endpoints)
}
