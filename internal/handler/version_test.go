package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/handler"
	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// VersionResponse mirrors the expected JSON structure from GET /version.
type VersionResponse struct {
	Version        string  `json:"version"`
	GitCommit      string  `json:"git_commit"`
	GitBranch      string  `json:"git_branch"`
	BuildDate      string  `json:"build_date"`
	GoVersion      string  `json:"go_version"`
	OS             string  `json:"os"`
	Arch           string  `json:"arch"`
	Hostname       string  `json:"hostname"`
	GOMAXPROCS     int     `json:"gomaxprocs"`
	UptimeSeconds  float64 `json:"uptime_seconds"`
	EndpointCount  int     `json:"endpoint_count"`
	StartTime      string  `json:"start_time"`
}

// AC-001: GET /version returns HTTP 200 with JSON body containing all required fields
func TestAC001_VersionEndpointReturnsAllFields(t *testing.T) {
	vh := handler.NewVersionHandler("v0.8.0", "abc123", "main", "2026-03-15T12:00:00Z", 8)

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	vh.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp VersionResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resp.Version != "v0.8.0" {
		t.Errorf("expected version 'v0.8.0', got '%s'", resp.Version)
	}
	if resp.GitCommit != "abc123" {
		t.Errorf("expected git_commit 'abc123', got '%s'", resp.GitCommit)
	}
	if resp.GitBranch != "main" {
		t.Errorf("expected git_branch 'main', got '%s'", resp.GitBranch)
	}
	if resp.BuildDate != "2026-03-15T12:00:00Z" {
		t.Errorf("expected build_date '2026-03-15T12:00:00Z', got '%s'", resp.BuildDate)
	}
	if resp.GoVersion == "" {
		t.Error("expected go_version to be non-empty")
	}
	if resp.OS == "" {
		t.Error("expected os to be non-empty")
	}
	if resp.Arch == "" {
		t.Error("expected arch to be non-empty")
	}
	if resp.Hostname == "" {
		t.Error("expected hostname to be non-empty")
	}
	if resp.GOMAXPROCS <= 0 {
		t.Errorf("expected gomaxprocs > 0, got %d", resp.GOMAXPROCS)
	}
	if resp.UptimeSeconds < 0 {
		t.Errorf("expected uptime_seconds >= 0, got %f", resp.UptimeSeconds)
	}
	if resp.EndpointCount != 8 {
		t.Errorf("expected endpoint_count 8, got %d", resp.EndpointCount)
	}
	if resp.StartTime == "" {
		t.Error("expected start_time to be non-empty")
	}
}

// AC-002: Build vars default to dev/empty when not set via ldflags
func TestAC002_DevBuildShowsDefaults(t *testing.T) {
	vh := handler.NewVersionHandler("dev", "", "", "", 0)

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	vh.ServeHTTP(w, req)

	var resp VersionResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Version != "dev" {
		t.Errorf("expected version 'dev', got '%s'", resp.Version)
	}
	if resp.GitCommit != "" {
		t.Errorf("expected empty git_commit, got '%s'", resp.GitCommit)
	}
	if resp.GitBranch != "" {
		t.Errorf("expected empty git_branch, got '%s'", resp.GitBranch)
	}
	if resp.BuildDate != "" {
		t.Errorf("expected empty build_date, got '%s'", resp.BuildDate)
	}
	// Runtime fields should still be populated
	if resp.GoVersion == "" {
		t.Error("expected go_version populated even in dev build")
	}
	if resp.Hostname == "" {
		t.Error("expected hostname populated even in dev build")
	}
}

// AC-003: go_version populated from runtime.Version()
func TestAC003_GoVersionFromRuntime(t *testing.T) {
	vh := handler.NewVersionHandler("dev", "", "", "", 0)

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	vh.ServeHTTP(w, req)

	var resp VersionResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.GoVersion != runtime.Version() {
		t.Errorf("expected go_version '%s', got '%s'", runtime.Version(), resp.GoVersion)
	}
}

// AC-004: os and arch from runtime.GOOS/GOARCH
func TestAC004_OSArchFromRuntime(t *testing.T) {
	vh := handler.NewVersionHandler("dev", "", "", "", 0)

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	vh.ServeHTTP(w, req)

	var resp VersionResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.OS != runtime.GOOS {
		t.Errorf("expected os '%s', got '%s'", runtime.GOOS, resp.OS)
	}
	if resp.Arch != runtime.GOARCH {
		t.Errorf("expected arch '%s', got '%s'", runtime.GOARCH, resp.Arch)
	}
}

// AC-005: hostname populated from os.Hostname()
func TestAC005_HostnamePopulated(t *testing.T) {
	vh := handler.NewVersionHandler("dev", "", "", "", 0)

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	vh.ServeHTTP(w, req)

	var resp VersionResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.Hostname == "" {
		t.Error("expected hostname to be non-empty")
	}
}

// AC-006: gomaxprocs from runtime.GOMAXPROCS(0)
func TestAC006_GOMAXPROCS(t *testing.T) {
	vh := handler.NewVersionHandler("dev", "", "", "", 0)

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	vh.ServeHTTP(w, req)

	var resp VersionResponse
	json.NewDecoder(w.Body).Decode(&resp)

	expected := runtime.GOMAXPROCS(0)
	if resp.GOMAXPROCS != expected {
		t.Errorf("expected gomaxprocs %d, got %d", expected, resp.GOMAXPROCS)
	}
}

// AC-007: uptime_seconds increases over time
func TestAC007_UptimeIncreases(t *testing.T) {
	vh := handler.NewVersionHandler("dev", "", "", "", 0)

	req1 := httptest.NewRequest("GET", "/version", nil)
	w1 := httptest.NewRecorder()
	vh.ServeHTTP(w1, req1)

	var resp1 VersionResponse
	json.NewDecoder(w1.Body).Decode(&resp1)

	time.Sleep(50 * time.Millisecond)

	req2 := httptest.NewRequest("GET", "/version", nil)
	w2 := httptest.NewRecorder()
	vh.ServeHTTP(w2, req2)

	var resp2 VersionResponse
	json.NewDecoder(w2.Body).Decode(&resp2)

	if resp2.UptimeSeconds <= resp1.UptimeSeconds {
		t.Errorf("expected uptime to increase: first=%f, second=%f", resp1.UptimeSeconds, resp2.UptimeSeconds)
	}
}

// AC-008: endpoint_count matches configured endpoints
func TestAC008_EndpointCount(t *testing.T) {
	vh := handler.NewVersionHandler("dev", "", "", "", 5)

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	vh.ServeHTTP(w, req)

	var resp VersionResponse
	json.NewDecoder(w.Body).Decode(&resp)

	if resp.EndpointCount != 5 {
		t.Errorf("expected endpoint_count 5, got %d", resp.EndpointCount)
	}
}

// AC-009: Prometheus gauge cashdrugs_build_info exists with correct labels and value 1
func TestAC009_PrometheusBuildInfoGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.BuildInfo.WithLabelValues("v0.8.0", "abc123", "go1.24", "2026-03-15T12:00:00Z").Set(1)

	expected := `
		# HELP cashdrugs_build_info Build metadata for version tracking.
		# TYPE cashdrugs_build_info gauge
		cashdrugs_build_info{build_date="2026-03-15T12:00:00Z",git_commit="abc123",go_version="go1.24",version="v0.8.0"} 1
	`
	if err := testutil.CollectAndCompare(m.BuildInfo, strings.NewReader(expected)); err != nil {
		t.Errorf("unexpected build_info metric: %v", err)
	}
}

// AC-010: Prometheus gauge cashdrugs_uptime_seconds exists and can be updated
func TestAC010_PrometheusUptimeSecondsGauge(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	m.UptimeSeconds.Set(100.5)

	val := testutil.ToFloat64(m.UptimeSeconds)
	if val != 100.5 {
		t.Errorf("expected uptime_seconds 100.5, got %f", val)
	}

	m.UptimeSeconds.Set(200.0)
	val = testutil.ToFloat64(m.UptimeSeconds)
	if val != 200.0 {
		t.Errorf("expected uptime_seconds 200.0, got %f", val)
	}
}

// AC-015: Content-Type is application/json
func TestAC015_ContentTypeJSON(t *testing.T) {
	vh := handler.NewVersionHandler("dev", "", "", "", 0)

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	vh.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", ct)
	}
}

// AC-013: Swagger annotations are present (verified by checking handler has doc comments)
// This is a compile-time / static check; we verify the handler file contains annotations
// in the VERIFY phase. Here we just ensure the handler responds correctly.
func TestAC013_SwaggerAnnotationsHandlerResponds(t *testing.T) {
	vh := handler.NewVersionHandler("v1.0.0", "def456", "release", "2026-03-15T00:00:00Z", 3)

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	vh.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
