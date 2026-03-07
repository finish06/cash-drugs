package handler

import (
	"encoding/json"
	"net/http"

	"github.com/finish06/cash-drugs/internal/cache"
)

// HealthHandler handles GET /health requests.
type HealthHandler struct {
	pinger  cache.Pinger
	version string
}

// HealthOption configures a HealthHandler.
type HealthOption func(*HealthHandler)

// WithVersion sets the version reported by the health endpoint.
func WithVersion(v string) HealthOption {
	return func(h *HealthHandler) { h.version = v }
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(pinger cache.Pinger, opts ...HealthOption) *HealthHandler {
	h := &HealthHandler{pinger: pinger, version: "dev"}
	for _, o := range opts {
		o(h)
	}
	return h
}

// ServeHTTP handles health check requests.
//
// @Summary      Health check
// @Description  Returns service health status including database connectivity.
// @Tags         system
// @Produce      json
// @Success      200  {object}  map[string]string  "OK"
// @Failure      503  {object}  map[string]string  "Degraded"
// @Router       /health [get]
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	err := h.pinger.Ping()
	if err != nil {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{
			"status":  "degraded",
			"db":      "disconnected",
			"version": h.version,
		})
		return
	}

	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{
		"status":  "ok",
		"db":      "connected",
		"version": h.version,
	})
}
