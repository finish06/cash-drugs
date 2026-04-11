package handler

import (
	"encoding/json"
	"net/http"
	"runtime"
)

// VersionInfo represents the JSON response for GET /version. It carries
// build-time metadata only; runtime fields (uptime, start_time, leader,
// hostname, etc.) belong on /health per the stack-wide contract.
type VersionInfo struct {
	Version   string `json:"version"`
	GitCommit string `json:"git_commit"`
	GitBranch string `json:"git_branch"`
	GoVersion string `json:"go_version"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
	BuildTime string `json:"build_time"`
}

// VersionHandler handles GET /version requests.
type VersionHandler struct {
	version   string
	gitCommit string
	gitBranch string
	buildTime string
}

// NewVersionHandler creates a new VersionHandler with build-time variables
// typically injected via -ldflags at compile time.
func NewVersionHandler(version, gitCommit, gitBranch, buildTime string) *VersionHandler {
	return &VersionHandler{
		version:   version,
		gitCommit: gitCommit,
		gitBranch: gitBranch,
		buildTime: buildTime,
	}
}

// ServeHTTP handles version info requests.
//
// @Summary      Version and build info
// @Description  Returns compile-time build metadata (version, git commit/branch, Go version, target OS/arch, build time). Runtime fields are exposed via /health.
// @Tags         system
// @Produce      json
// @Success      200  {object}  VersionInfo  "OK"
// @Router       /version [get]
func (h *VersionHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	info := VersionInfo{
		Version:   h.version,
		GitCommit: h.gitCommit,
		GitBranch: h.gitBranch,
		GoVersion: runtime.Version(),
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
		BuildTime: h.buildTime,
	}

	_ = json.NewEncoder(w).Encode(info)
}
