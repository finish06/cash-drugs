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

// AC-012: Raw format is valid
func TestAC012_RawFormatValid(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: spl-xml
    base_url: http://example.com
    path: /v2/spls/{SETID}.xml
    format: raw
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("expected no error for format 'raw', got %v", err)
	}
	if endpoints[0].Format != "raw" {
		t.Errorf("expected format 'raw', got '%s'", endpoints[0].Format)
	}
}

// AC-021: ExtractAllParams gets params from both path and query_params
func TestAC021_ExtractAllParams(t *testing.T) {
	ep := config.Endpoint{
		Path: "/v2/spls/{SETID}",
		QueryParams: map[string]string{
			"foo": "{BAR}",
		},
	}

	params := config.ExtractAllParams(ep)
	if len(params) != 2 {
		t.Fatalf("expected 2 params, got %d: %v", len(params), params)
	}

	found := map[string]bool{}
	for _, p := range params {
		found[p] = true
	}
	if !found["SETID"] {
		t.Error("expected SETID in params")
	}
	if !found["BAR"] {
		t.Error("expected BAR in params")
	}
}

// AC-021: ExtractAllParams deduplicates params appearing in both path and query
func TestAC021_ExtractAllParamsDedup(t *testing.T) {
	ep := config.Endpoint{
		Path: "/v2/spls/{SETID}",
		QueryParams: map[string]string{
			"setid": "{SETID}",
		},
	}

	params := config.ExtractAllParams(ep)
	if len(params) != 1 {
		t.Errorf("expected 1 deduplicated param, got %d: %v", len(params), params)
	}
}

// AC-016: Query param substitution
func TestAC016_SubstitutePathParamsInValues(t *testing.T) {
	result := config.SubstitutePathParams("{SETID}", map[string]string{"SETID": "abc-123"})
	if result != "abc-123" {
		t.Errorf("expected 'abc-123', got '%s'", result)
	}
}

// ParsePagination: float64 value (YAML parses numbers as float64)
func TestParsePagination_Float64(t *testing.T) {
	ep := config.Endpoint{
		Pagination: float64(5),
	}
	maxPages, fetchAll := config.ParsePagination(ep)
	if fetchAll {
		t.Error("expected fetchAll=false for float64 pagination")
	}
	if maxPages != 5 {
		t.Errorf("expected maxPages=5, got %d", maxPages)
	}
}

// ParsePagination: zero or negative number defaults to 1
func TestParsePagination_ZeroOrNegative(t *testing.T) {
	tests := []struct {
		name  string
		value interface{}
	}{
		{"zero int", int(0)},
		{"negative int", int(-3)},
		{"zero float64", float64(0)},
		{"negative float64", float64(-2.5)},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			ep := config.Endpoint{
				Pagination: tc.value,
			}
			maxPages, fetchAll := config.ParsePagination(ep)
			if fetchAll {
				t.Error("expected fetchAll=false for zero/negative pagination")
			}
			if maxPages != 1 {
				t.Errorf("expected maxPages=1 (default), got %d", maxPages)
			}
		})
	}
}

// ParsePagination: unknown type defaults to 1 page
func TestParsePagination_UnknownType(t *testing.T) {
	ep := config.Endpoint{
		Pagination: []string{"unexpected"},
	}
	maxPages, fetchAll := config.ParsePagination(ep)
	if fetchAll {
		t.Error("expected fetchAll=false for unknown type")
	}
	if maxPages != 1 {
		t.Errorf("expected maxPages=1 (default), got %d", maxPages)
	}
}

// Load: xml format should be accepted
func TestLoad_XmlFormatAccepted(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: xml-endpoint
    base_url: http://example.com
    path: /api/data
    format: xml
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("expected no error for format 'xml', got %v", err)
	}
	if endpoints[0].Format != "xml" {
		t.Errorf("expected format 'xml', got '%s'", endpoints[0].Format)
	}
}

// SubstitutePathParams: multiple params
func TestSubstitutePathParams_MultipleParams(t *testing.T) {
	path := "/v2/{TYPE}/items/{ID}/details/{FORMAT}"
	params := map[string]string{
		"TYPE":   "drugs",
		"ID":     "12345",
		"FORMAT": "json",
	}
	result := config.SubstitutePathParams(path, params)
	expected := "/v2/drugs/items/12345/details/json"
	if result != expected {
		t.Errorf("expected '%s', got '%s'", expected, result)
	}
}

// SubstitutePathParams: no matching params returns path unchanged
func TestSubstitutePathParams_NoMatchingParams(t *testing.T) {
	path := "/v2/spls/{SETID}"
	params := map[string]string{
		"OTHER": "value",
	}
	result := config.SubstitutePathParams(path, params)
	if result != path {
		t.Errorf("expected path unchanged '%s', got '%s'", path, result)
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
