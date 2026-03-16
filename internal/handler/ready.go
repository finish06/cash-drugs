package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
)

// WarmupState provides the current state of cache warm-up.
type WarmupState interface {
	// IsReady returns true when warm-up is complete.
	IsReady() bool
	// Progress returns the number of completed and total endpoints in the current warm-up.
	Progress() (done, total int)
}

// ReadyHandler handles GET /ready requests.
type ReadyHandler struct {
	state WarmupState
}

// NewReadyHandler creates a new ReadyHandler with the given warmup state.
func NewReadyHandler(state WarmupState) *ReadyHandler {
	return &ReadyHandler{state: state}
}

// ServeHTTP handles readiness probe requests.
//
// @Summary      Readiness probe
// @Description  Returns whether the service has finished warming its cache.
// @Tags         system
// @Produce      json
// @Success      200  {object}  map[string]string  "Ready"
// @Failure      503  {object}  map[string]string  "Warming"
// @Router       /ready [get]
func (h *ReadyHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	if h.state.IsReady() {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{
			"status": "ready",
		})
		return
	}

	done, total := h.state.Progress()
	w.WriteHeader(http.StatusServiceUnavailable)
	json.NewEncoder(w).Encode(map[string]string{
		"status":   "warming",
		"progress": fmt.Sprintf("%d/%d", done, total),
	})
}
