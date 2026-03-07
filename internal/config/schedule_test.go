package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/finish06/cash-drugs/internal/config"
)

// AC-001: Config supports optional refresh field with cron expression
func TestAC001_RefreshFieldParsed(t *testing.T) {
	cfgPath := writeScheduleConfig(t, `
endpoints:
  - slug: drugnames
    base_url: http://example.com
    path: /v2/drugnames
    format: json
    refresh: "0 */6 * * *"
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if endpoints[0].Refresh != "0 */6 * * *" {
		t.Errorf("expected refresh '0 */6 * * *', got '%s'", endpoints[0].Refresh)
	}
}

// AC-002: Endpoints without refresh remain on-demand only
func TestAC002_NoRefreshFieldMeansOnDemand(t *testing.T) {
	cfgPath := writeScheduleConfig(t, `
endpoints:
  - slug: drugnames
    base_url: http://example.com
    path: /v2/drugnames
    format: json
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if endpoints[0].Refresh != "" {
		t.Errorf("expected empty refresh for on-demand endpoint, got '%s'", endpoints[0].Refresh)
	}
}

// AC-006: Endpoints with path params and refresh field get a warning logged,
// but config loading succeeds. HasPathParams detects them.
func TestAC006_PathParamsDetected(t *testing.T) {
	params := config.ExtractPathParams("/v2/spls/{SETID}")
	if len(params) == 0 {
		t.Error("expected path params detected for /v2/spls/{SETID}")
	}
}

// AC-009: Invalid cron expression prevents startup
func TestAC009_InvalidCronPreventsStartup(t *testing.T) {
	cfgPath := writeScheduleConfig(t, `
endpoints:
  - slug: drugnames
    base_url: http://example.com
    path: /v2/drugnames
    format: json
    refresh: "not-a-cron"
`)

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid cron expression")
	}
	if !strings.Contains(err.Error(), "invalid cron") {
		t.Errorf("expected 'invalid cron' in error, got: %v", err)
	}
}

// AC-009: Valid 5-field cron accepted
func TestAC009_ValidCronAccepted(t *testing.T) {
	cfgPath := writeScheduleConfig(t, `
endpoints:
  - slug: drugnames
    base_url: http://example.com
    path: /v2/drugnames
    format: json
    refresh: "*/5 * * * *"
`)

	_, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error for valid cron: %v", err)
	}
}

func writeScheduleConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}
