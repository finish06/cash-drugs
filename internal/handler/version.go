package handler

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"time"
)

// VersionInfo represents the JSON response for GET /version.
type VersionInfo struct {
	Version       string  `json:"version"`
	GitCommit     string  `json:"git_commit"`
	GitBranch     string  `json:"git_branch"`
	BuildDate     string  `json:"build_date"`
	GoVersion     string  `json:"go_version"`
	OS            string  `json:"os"`
	Arch          string  `json:"arch"`
	Hostname      string  `json:"hostname"`
	GOMAXPROCS    int     `json:"gomaxprocs"`
	UptimeSeconds float64 `json:"uptime_seconds"`
	EndpointCount int     `json:"endpoint_count"`
	StartTime     string  `json:"start_time"`
	Leader        bool    `json:"leader"`
}

// VersionHandler handles GET /version requests.
type VersionHandler struct {
	version       string
	gitCommit     string
	gitBranch     string
	buildDate     string
	hostname      string
	startTime     time.Time
	endpointCount int
	leader        bool
}

// VersionOption configures a VersionHandler.
type VersionOption func(*VersionHandler)

// WithLeader sets the leader flag on the version handler.
func WithLeader(leader bool) VersionOption {
	return func(h *VersionHandler) {
		h.leader = leader
	}
}

// NewVersionHandler creates a new VersionHandler with build-time variables.
func NewVersionHandler(version, gitCommit, gitBranch, buildDate string, endpointCount int, opts ...VersionOption) *VersionHandler {
	hostname, err := os.Hostname()
	if err != nil {
		slog.Warn("failed to get hostname, using 'unknown'", "component", "version", "error", err)
		hostname = "unknown"
	}

	h := &VersionHandler{
		version:       version,
		gitCommit:     gitCommit,
		gitBranch:     gitBranch,
		buildDate:     buildDate,
		hostname:      hostname,
		startTime:     time.Now(),
		endpointCount: endpointCount,
	}
	for _, o := range opts {
		o(h)
	}
	return h
}

// ServeHTTP handles version info requests.
//
// @Summary      Version and build info
// @Description  Returns build metadata, runtime info, and uptime for deployment verification.
// @Tags         system
// @Produce      json
// @Success      200  {object}  VersionInfo  "OK"
// @Router       /version [get]
func (h *VersionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	info := VersionInfo{
		Version:       h.version,
		GitCommit:     h.gitCommit,
		GitBranch:     h.gitBranch,
		BuildDate:     h.buildDate,
		GoVersion:     runtime.Version(),
		OS:            runtime.GOOS,
		Arch:          runtime.GOARCH,
		Hostname:      h.hostname,
		GOMAXPROCS:    runtime.GOMAXPROCS(0),
		UptimeSeconds: time.Since(h.startTime).Seconds(),
		EndpointCount: h.endpointCount,
		StartTime:     h.startTime.UTC().Format(time.RFC3339),
		Leader:        h.leader,
	}

	json.NewEncoder(w).Encode(info)
}
