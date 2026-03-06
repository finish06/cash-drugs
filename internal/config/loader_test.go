package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/finish06/drugs/internal/config"
)

// AC-001: Service loads upstream API definitions from a YAML config file at startup
func TestAC001_LoadConfigFromYAML(t *testing.T) {
	cfgPath := writeTestConfig(t, validConfig)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(endpoints) != 2 {
		t.Fatalf("expected 2 endpoints, got %d", len(endpoints))
	}
}

// AC-002: Each config entry defines required fields
func TestAC002_ConfigEntryRequiredFields(t *testing.T) {
	cfgPath := writeTestConfig(t, validConfig)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	ep := endpoints[0]
	if ep.Slug == "" {
		t.Error("slug should not be empty")
	}
	if ep.BaseURL == "" {
		t.Error("base_url should not be empty")
	}
	if ep.Path == "" {
		t.Error("path should not be empty")
	}
	if ep.Format == "" {
		t.Error("format should not be empty")
	}
}

// AC-002: Pagination settings are parsed correctly
func TestAC002_PaginationSettings(t *testing.T) {
	cfgPath := writeTestConfig(t, validConfig)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// First endpoint should have pagination "all"
	ep := endpoints[0]
	maxPages, fetchAll := config.ParsePagination(ep)
	if !fetchAll {
		t.Error("expected fetchAll=true for pagination 'all'")
	}
	_ = maxPages
}

// AC-002: Numeric pagination
func TestAC002_NumericPagination(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
    pagination: 3
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	maxPages, fetchAll := config.ParsePagination(endpoints[0])
	if fetchAll {
		t.Error("expected fetchAll=false for numeric pagination")
	}
	if maxPages != 3 {
		t.Errorf("expected maxPages=3, got %d", maxPages)
	}
}

// AC-002: Default pagination (1 page, no pagination)
func TestAC002_DefaultPagination(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	maxPages, fetchAll := config.ParsePagination(endpoints[0])
	if fetchAll {
		t.Error("expected fetchAll=false for default pagination")
	}
	if maxPages != 1 {
		t.Errorf("expected maxPages=1, got %d", maxPages)
	}
}

// AC-003: Config entries support path parameters
func TestAC003_PathParameterSupport(t *testing.T) {
	cfgPath := writeTestConfig(t, validConfig)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Find the spl-detail endpoint
	var splEndpoint *config.Endpoint
	for i := range endpoints {
		if endpoints[i].Slug == "spl-detail" {
			splEndpoint = &endpoints[i]
			break
		}
	}
	if splEndpoint == nil {
		t.Fatal("expected spl-detail endpoint in config")
	}

	params := config.ExtractPathParams(splEndpoint.Path)
	if len(params) != 1 || params[0] != "SETID" {
		t.Errorf("expected path param [SETID], got %v", params)
	}
}

// AC-012: Invalid config file prevents startup with clear error
func TestAC012_InvalidConfigMissingRequiredField(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    path: /api
    format: json
`)

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing base_url")
	}
}

// AC-012: Config file not found
func TestAC012_ConfigFileNotFound(t *testing.T) {
	_, err := config.Load("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

// AC-012: Duplicate slugs
func TestAC012_DuplicateSlugs(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
  - slug: test
    base_url: http://example.com
    path: /api2
    format: json
`)

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for duplicate slugs")
	}
}

// AC-012: Invalid format value
func TestAC012_InvalidFormat(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: csv
`)

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid format 'csv'")
	}
}

// AC-002: Query params are parsed
func TestAC002_QueryParams(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
    query_params:
      key: value
      foo: bar
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if endpoints[0].QueryParams["key"] != "value" {
		t.Error("expected query_params[key]=value")
	}
	if endpoints[0].QueryParams["foo"] != "bar" {
		t.Error("expected query_params[foo]=bar")
	}
}

// AC-002: Page param defaults
func TestAC002_PageParamDefaults(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if endpoints[0].PageParam != "page" {
		t.Errorf("expected default page_param='page', got '%s'", endpoints[0].PageParam)
	}
	if endpoints[0].PagesizeParam != "pagesize" {
		t.Errorf("expected default pagesize_param='pagesize', got '%s'", endpoints[0].PagesizeParam)
	}
	if endpoints[0].Pagesize != 100 {
		t.Errorf("expected default pagesize=100, got %d", endpoints[0].Pagesize)
	}
}

// Helper functions

const validConfig = `
endpoints:
  - slug: drugnames
    base_url: https://dailymed.nlm.nih.gov/dailymed/services
    path: /v2/drugnames
    format: json
    pagination: all
    page_param: page
    pagesize_param: pagesize
    pagesize: 100
  - slug: spl-detail
    base_url: https://dailymed.nlm.nih.gov/dailymed/services
    path: /v2/spls/{SETID}
    format: json
`

func writeTestConfig(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("failed to write test config: %v", err)
	}
	return path
}
