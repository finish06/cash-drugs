package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
)

// Dependency represents the health status of a single downstream dependency.
type Dependency struct {
	Name      string  `json:"name"`
	Status    string  `json:"status"`
	LatencyMs float64 `json:"latency_ms"`
	Error     string  `json:"error,omitempty"`
}

// HealthResponse is the JSON response for GET /health. It follows the
// stack-wide health contract shared across rx-dag, cash-drugs, drug-gate,
// and drugs-quiz BFF.
type HealthResponse struct {
	Status         string       `json:"status"`
	Version        string       `json:"version"`
	Uptime         string       `json:"uptime"`
	StartTime      string       `json:"start_time"`
	Dependencies   []Dependency `json:"dependencies"`
	CacheSlugCount int          `json:"cache_slug_count,omitempty"`
	Leader         bool         `json:"leader"`
}

// HealthHandler handles GET /health requests.
type HealthHandler struct {
	pinger         cache.Pinger
	version        string
	startTime      time.Time
	cacheSlugCount int
	leader         bool
}

// HealthOption configures a HealthHandler.
type HealthOption func(*HealthHandler)

// WithVersion sets the version reported by the health endpoint.
func WithVersion(v string) HealthOption {
	return func(h *HealthHandler) { h.version = v }
}

// WithHealthStartTime sets the process start time used to compute uptime.
func WithHealthStartTime(t time.Time) HealthOption {
	return func(h *HealthHandler) { h.startTime = t }
}

// WithCacheSlugCount sets the total configured slug count reported in /health.
func WithCacheSlugCount(n int) HealthOption {
	return func(h *HealthHandler) { h.cacheSlugCount = n }
}

// WithHealthLeader sets the scheduler leader flag reported in /health.
func WithHealthLeader(leader bool) HealthOption {
	return func(h *HealthHandler) { h.leader = leader }
}

// NewHealthHandler creates a new HealthHandler.
func NewHealthHandler(pinger cache.Pinger, opts ...HealthOption) *HealthHandler {
	h := &HealthHandler{
		pinger:    pinger,
		version:   "dev",
		startTime: time.Now(),
	}
	for _, o := range opts {
		o(h)
	}
	return h
}

// ServeHTTP handles health check requests.
//
// @Summary      Health check
// @Description  Returns structured service health per stack-wide contract: status, version, uptime, start_time, dependencies, and domain fields.
// @Tags         system
// @Produce      json
// @Success      200  {object}  HealthResponse  "OK"
// @Failure      503  {object}  HealthResponse  "Critical dependency unavailable"
// @Router       /health [get]
func (h *HealthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	latency, err := h.pinger.PingWithLatency()
	mongoDep := Dependency{
		Name:      "mongodb",
		LatencyMs: float64(latency.Microseconds()) / 1000.0,
	}

	status := "ok"
	httpStatus := http.StatusOK
	if err != nil {
		mongoDep.Status = "disconnected"
		mongoDep.Error = err.Error()
		mongoDep.LatencyMs = 0
		status = "error"
		httpStatus = http.StatusServiceUnavailable
	} else {
		mongoDep.Status = "connected"
	}

	resp := HealthResponse{
		Status:         status,
		Version:        h.version,
		Uptime:         time.Since(h.startTime).Truncate(time.Second).String(),
		StartTime:      h.startTime.UTC().Format(time.RFC3339),
		Dependencies:   []Dependency{mongoDep},
		CacheSlugCount: h.cacheSlugCount,
		Leader:         h.leader,
	}

	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(resp)
}
