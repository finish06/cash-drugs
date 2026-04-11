package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"testing"

	"github.com/finish06/cash-drugs/internal/handler"
)

// AC-010, AC-011: /version returns all required build-time fields with
// `build_time` (not `build_date`).
func TestVersion_AC010_AC011_AllBuildFields(t *testing.T) {
	vh := handler.NewVersionHandler("v0.8.0", "abc123", "main", "2026-03-15T12:00:00Z")

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	vh.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp handler.VersionInfo
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode /version response: %v", err)
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
	if resp.BuildTime != "2026-03-15T12:00:00Z" {
		t.Errorf("expected build_time '2026-03-15T12:00:00Z', got '%s'", resp.BuildTime)
	}
	if resp.GoVersion == "" {
		t.Error("expected go_version to be non-empty")
	}
	if resp.OS != runtime.GOOS {
		t.Errorf("expected os '%s', got '%s'", runtime.GOOS, resp.OS)
	}
	if resp.Arch != runtime.GOARCH {
		t.Errorf("expected arch '%s', got '%s'", runtime.GOARCH, resp.Arch)
	}
}

// AC-011: JSON key is `build_time`, not `build_date`
func TestVersion_AC011_JSONKeyBuildTime(t *testing.T) {
	vh := handler.NewVersionHandler("v1.0.0", "deadbeef", "main", "2026-04-11T00:00:00Z")

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	vh.ServeHTTP(w, req)

	body := w.Body.String()
	if !strings.Contains(body, `"build_time"`) {
		t.Errorf("expected response to contain 'build_time' key, got: %s", body)
	}
	if strings.Contains(body, `"build_date"`) {
		t.Errorf("expected response to NOT contain legacy 'build_date' key, got: %s", body)
	}
}

// AC-012: /version does NOT contain runtime-varying fields
func TestVersion_AC012_NoRuntimeFields(t *testing.T) {
	vh := handler.NewVersionHandler("v1.0.0", "abc", "main", "2026-04-11T00:00:00Z")

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	vh.ServeHTTP(w, req)

	body := w.Body.String()
	forbidden := []string{
		`"uptime_seconds"`,
		`"start_time"`,
		`"endpoint_count"`,
		`"leader"`,
		`"hostname"`,
		`"gomaxprocs"`,
	}
	for _, f := range forbidden {
		if strings.Contains(body, f) {
			t.Errorf("/version should not contain runtime field %s, body: %s", f, body)
		}
	}
}

// AC-013: Build-time fields default to empty strings / "dev" when ldflags are absent
func TestVersion_AC013_DefaultsWhenLdflagsAbsent(t *testing.T) {
	vh := handler.NewVersionHandler("dev", "", "", "")

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	vh.ServeHTTP(w, req)

	var resp handler.VersionInfo
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if resp.Version != "dev" {
		t.Errorf("expected version 'dev', got '%s'", resp.Version)
	}
	if resp.GitCommit != "" {
		t.Errorf("expected empty git_commit, got '%s'", resp.GitCommit)
	}
	if resp.GitBranch != "" {
		t.Errorf("expected empty git_branch, got '%s'", resp.GitBranch)
	}
	if resp.BuildTime != "" {
		t.Errorf("expected empty build_time, got '%s'", resp.BuildTime)
	}
}

// AC-010: Content-Type is application/json
func TestVersion_AC010_ContentTypeJSON(t *testing.T) {
	vh := handler.NewVersionHandler("v1.0.0", "abc", "main", "2026-04-11T00:00:00Z")

	req := httptest.NewRequest("GET", "/version", nil)
	w := httptest.NewRecorder()
	vh.ServeHTTP(w, req)

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", ct)
	}
}

