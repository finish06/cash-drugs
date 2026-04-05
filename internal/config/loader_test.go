package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/finish06/cash-drugs/internal/config"
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
		return
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

// ExtractAllParams includes SearchParams
func TestExtractAllParams_WithSearchParams(t *testing.T) {
	ep := config.Endpoint{
		Path:         "/v2/drugs",
		SearchParams: []string{"{DRUG_NAME}"},
	}

	params := config.ExtractAllParams(ep)
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d: %v", len(params), params)
	}
	if params[0] != "DRUG_NAME" {
		t.Errorf("expected DRUG_NAME, got %s", params[0])
	}
}

// ExtractAllParams with all three sources and dedup
func TestExtractAllParams_AllSources(t *testing.T) {
	ep := config.Endpoint{
		Path: "/v2/spls/{SETID}",
		QueryParams: map[string]string{
			"format": "{FMT}",
		},
		SearchParams: []string{"{NAME}", "{SETID}"},
	}

	params := config.ExtractAllParams(ep)
	if len(params) != 3 {
		t.Fatalf("expected 3 deduplicated params, got %d: %v", len(params), params)
	}

	found := map[string]bool{}
	for _, p := range params {
		found[p] = true
	}
	for _, expected := range []string{"SETID", "FMT", "NAME"} {
		if !found[expected] {
			t.Errorf("expected %s in params", expected)
		}
	}
}

// ExtractAllParams with no params returns nil
func TestExtractAllParams_NoParams(t *testing.T) {
	ep := config.Endpoint{
		Path: "/v2/drugs",
	}

	params := config.ExtractAllParams(ep)
	if len(params) != 0 {
		t.Errorf("expected 0 params, got %d: %v", len(params), params)
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

// AC-004: log_level in config.yaml is parsed
func TestAC004_LogLevelFromConfig(t *testing.T) {
	cfgPath := writeTestConfig(t, `
log_level: debug
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
`)

	appCfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if appCfg.LogLevel != "debug" {
		t.Errorf("expected log_level 'debug', got '%s'", appCfg.LogLevel)
	}
}

// AC-004: LoadConfig with nonexistent file returns error
func TestAC004_LoadConfigFileNotFound(t *testing.T) {
	_, err := config.LoadConfig("/nonexistent/config.yaml")
	if err == nil {
		t.Fatal("expected error for missing config file")
	}
}

// AC-004: LoadConfig with invalid YAML returns error
func TestAC004_LoadConfigInvalidYAML(t *testing.T) {
	cfgPath := writeTestConfig(t, `{{{invalid yaml`)
	_, err := config.LoadConfig(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// AC-004: Missing log_level returns empty string (caller applies default)
func TestAC004_MissingLogLevelReturnsEmpty(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
`)

	appCfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if appCfg.LogLevel != "" {
		t.Errorf("expected empty log_level, got '%s'", appCfg.LogLevel)
	}
}

// FDA API Integration — New config fields (RED phase: these tests WILL FAIL)

// TestFDA_AC001_PaginationStyleField: pagination_style parses into PaginationStyle field
func TestFDA_AC001_PaginationStyleField(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: fda-drugs
    base_url: https://api.fda.gov
    path: /drug/label.json
    format: json
    pagination_style: offset
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if endpoints[0].PaginationStyle != "offset" {
		t.Errorf("expected PaginationStyle='offset', got '%s'", endpoints[0].PaginationStyle)
	}
}

// TestFDA_AC001_PaginationStyleDefault: missing pagination_style defaults to "page"
func TestFDA_AC001_PaginationStyleDefault(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: fda-drugs
    base_url: https://api.fda.gov
    path: /drug/label.json
    format: json
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if endpoints[0].PaginationStyle != "page" {
		t.Errorf("expected default PaginationStyle='page', got '%s'", endpoints[0].PaginationStyle)
	}
}

// TestFDA_AC001_PaginationStyleValidation: invalid pagination_style returns error
func TestFDA_AC001_PaginationStyleValidation(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: fda-drugs
    base_url: https://api.fda.gov
    path: /drug/label.json
    format: json
    pagination_style: invalid_value
`)

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid pagination_style 'invalid_value'")
	}
}

// TestFDA_AC003_DataKeyField: data_key parses into DataKey field
func TestFDA_AC003_DataKeyField(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: fda-drugs
    base_url: https://api.fda.gov
    path: /drug/label.json
    format: json
    data_key: results
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if endpoints[0].DataKey != "results" {
		t.Errorf("expected DataKey='results', got '%s'", endpoints[0].DataKey)
	}
}

// TestFDA_AC003_DataKeyDefault: missing data_key defaults to "data"
func TestFDA_AC003_DataKeyDefault(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: fda-drugs
    base_url: https://api.fda.gov
    path: /drug/label.json
    format: json
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if endpoints[0].DataKey != "data" {
		t.Errorf("expected default DataKey='data', got '%s'", endpoints[0].DataKey)
	}
}

// TestFDA_AC004_TotalKeyField: total_key parses into TotalKey field
func TestFDA_AC004_TotalKeyField(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: fda-drugs
    base_url: https://api.fda.gov
    path: /drug/label.json
    format: json
    total_key: meta.results.total
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if endpoints[0].TotalKey != "meta.results.total" {
		t.Errorf("expected TotalKey='meta.results.total', got '%s'", endpoints[0].TotalKey)
	}
}

// TestFDA_AC004_TotalKeyDefault: missing total_key defaults to "metadata.total_pages"
func TestFDA_AC004_TotalKeyDefault(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: fda-drugs
    base_url: https://api.fda.gov
    path: /drug/label.json
    format: json
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if endpoints[0].TotalKey != "metadata.total_pages" {
		t.Errorf("expected default TotalKey='metadata.total_pages', got '%s'", endpoints[0].TotalKey)
	}
}

// TestFDA_AC005_ExistingDailyMedEndpointsUnchanged: existing config backward compat
func TestFDA_AC005_ExistingDailyMedEndpointsUnchanged(t *testing.T) {
	cfgPath := writeTestConfig(t, validConfig)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, ep := range endpoints {
		if ep.PaginationStyle != "page" {
			t.Errorf("endpoint '%s': expected default PaginationStyle='page', got '%s'", ep.Slug, ep.PaginationStyle)
		}
		if ep.DataKey != "data" {
			t.Errorf("endpoint '%s': expected default DataKey='data', got '%s'", ep.Slug, ep.DataKey)
		}
		if ep.TotalKey != "metadata.total_pages" {
			t.Errorf("endpoint '%s': expected default TotalKey='metadata.total_pages', got '%s'", ep.Slug, ep.TotalKey)
		}
	}
}


// AC-CSM-012: SystemMetricsInterval field parsed from YAML
func TestAC_CSM012_SystemMetricsInterval(t *testing.T) {
	cfgPath := writeTestConfig(t, `
log_level: info
system_metrics_interval: "15s"
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
`)

	appCfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if appCfg.SystemMetricsInterval != "15s" {
		t.Errorf("expected SystemMetricsInterval='15s', got '%s'", appCfg.SystemMetricsInterval)
	}
}

// AC-CSM-012: Default SystemMetricsInterval when absent
func TestAC_CSM012_SystemMetricsIntervalDefault(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
`)

	appCfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if appCfg.SystemMetricsInterval != "" {
		t.Errorf("expected empty SystemMetricsInterval when absent, got '%s'", appCfg.SystemMetricsInterval)
	}
}

// AC-007 (connection-resilience): MaxConcurrentRequests field parsed from YAML
func TestAC007_MaxConcurrentRequestsParsed(t *testing.T) {
	cfgPath := writeTestConfig(t, `
max_concurrent_requests: 100
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
`)

	appCfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if appCfg.MaxConcurrentRequests != 100 {
		t.Errorf("expected MaxConcurrentRequests=100, got %d", appCfg.MaxConcurrentRequests)
	}
}

// AC-007 (connection-resilience): Default value (50) when field is absent
func TestAC007_MaxConcurrentRequestsDefault(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
`)

	appCfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resolved := appCfg.MaxConcurrentRequests
	if resolved == 0 {
		resolved = 50 // default applied at usage site
	}
	if resolved != 50 {
		t.Errorf("expected default MaxConcurrentRequests=50, got %d", resolved)
	}
}

// AC-007 (connection-resilience): Invalid values (0, negative) should use default
func TestAC007_MaxConcurrentRequestsInvalid(t *testing.T) {
	tests := []struct {
		name  string
		value string
	}{
		{"zero", "0"},
		{"negative", "-5"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfgPath := writeTestConfig(t, `
max_concurrent_requests: `+tc.value+`
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
`)

			appCfg, err := config.LoadConfig(cfgPath)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			resolved := appCfg.MaxConcurrentRequests
			if resolved <= 0 {
				resolved = 50 // default applied at usage site
			}
			if resolved != 50 {
				t.Errorf("expected resolved MaxConcurrentRequests=50, got %d", resolved)
			}
		})
	}
}

// AC: LRUCacheSizeMB field parsed from YAML
func TestLRUCacheSizeMB_Parsed(t *testing.T) {
	cfgPath := writeTestConfig(t, `
lru_cache_size_mb: 512
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
`)

	appCfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if appCfg.LRUCacheSizeMB != 512 {
		t.Errorf("expected LRUCacheSizeMB=512, got %d", appCfg.LRUCacheSizeMB)
	}
}

// AC: LRUCacheSizeMB default when absent
func TestLRUCacheSizeMB_DefaultWhenAbsent(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
`)

	appCfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Default is 0 from YAML (caller applies 256 default)
	if appCfg.LRUCacheSizeMB != 0 {
		t.Errorf("expected LRUCacheSizeMB=0 when absent, got %d", appCfg.LRUCacheSizeMB)
	}
}

// AC-RN-001: Flatten bool field parsed from YAML (specs/response-normalization.md)
func TestAC_RN001_FlattenFieldParsed(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: rxnorm-all-related
    base_url: https://rxnav.nlm.nih.gov
    path: /REST/rxcui/{RXCUI}/allrelated.json
    format: json
    data_key: allRelatedGroup.conceptGroup
    flatten: true
    ttl: "336h"
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !endpoints[0].Flatten {
		t.Error("expected Flatten=true when flatten: true in config")
	}
}

// AC-RN-001: Flatten defaults to false when absent (specs/response-normalization.md)
func TestAC_RN001_FlattenDefaultFalse(t *testing.T) {
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

	if endpoints[0].Flatten {
		t.Error("expected Flatten=false when flatten is absent from config")
	}
}

// AC-MI-001: EnableScheduler *bool field parsed from YAML
func TestAC_MI001_EnableSchedulerParsed(t *testing.T) {
	cfgPath := writeTestConfig(t, `
enable_scheduler: false
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
`)

	appCfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if appCfg.EnableScheduler == nil {
		t.Fatal("expected EnableScheduler to be non-nil")
	}
	if *appCfg.EnableScheduler != false {
		t.Errorf("expected EnableScheduler=false, got %v", *appCfg.EnableScheduler)
	}
}

// AC-MI-001: EnableScheduler defaults to nil (caller treats nil as true)
func TestAC_MI001_EnableSchedulerDefaultNil(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
`)

	appCfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if appCfg.EnableScheduler != nil {
		t.Errorf("expected EnableScheduler=nil when absent, got %v", *appCfg.EnableScheduler)
	}
}

// AC-MI-001: EnableScheduler true is parsed correctly
func TestAC_MI001_EnableSchedulerTrue(t *testing.T) {
	cfgPath := writeTestConfig(t, `
enable_scheduler: true
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
`)

	appCfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if appCfg.EnableScheduler == nil {
		t.Fatal("expected EnableScheduler to be non-nil")
	}
	if *appCfg.EnableScheduler != true {
		t.Errorf("expected EnableScheduler=true, got %v", *appCfg.EnableScheduler)
	}
}

// Load: Invalid YAML content returns parse error
func TestLoad_InvalidYAML(t *testing.T) {
	cfgPath := writeTestConfig(t, `{{{invalid yaml`)
	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid YAML")
	}
}

// Load: Empty endpoints list returns error
func TestLoad_EmptyEndpoints(t *testing.T) {
	cfgPath := writeTestConfig(t, `endpoints: []`)
	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for empty endpoints list")
	}
}

// Load: Missing slug returns error
func TestLoad_MissingSlug(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - base_url: http://example.com
    path: /api
    format: json
`)
	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing slug")
	}
}

// Load: Missing path returns error
func TestLoad_MissingPath(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    format: json
`)
	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing path")
	}
}

// Load: Missing format returns error
func TestLoad_MissingFormat(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
`)
	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for missing format")
	}
}

// Load: Invalid cron expression returns error
func TestLoad_InvalidCronExpression(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
    refresh: "not-a-cron"
`)
	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid cron expression")
	}
}

// ParsePagination: non-"all" string defaults to 1 page
func TestParsePagination_NonAllString(t *testing.T) {
	ep := config.Endpoint{
		Pagination: "some-random-string",
	}
	maxPages, fetchAll := config.ParsePagination(ep)
	if fetchAll {
		t.Error("expected fetchAll=false for non-all string")
	}
	if maxPages != 1 {
		t.Errorf("expected maxPages=1 (default), got %d", maxPages)
	}
}

// rx-dag NDC Migration — Headers config (specs/rxdag-ndc-migration.md)

// AC-002: Config supports a headers map field on any endpoint
func TestRxDAG_AC002_HeadersFieldParsed(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test-with-headers
    base_url: http://192.168.1.145:8081
    path: /api/ndc/search
    format: json
    headers:
      X-API-Key: "test-key-value"
      X-Custom: "custom-value"
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(endpoints[0].Headers) != 2 {
		t.Fatalf("expected 2 headers, got %d", len(endpoints[0].Headers))
	}
	if endpoints[0].Headers["X-API-Key"] != "test-key-value" {
		t.Errorf("expected X-API-Key='test-key-value', got '%s'", endpoints[0].Headers["X-API-Key"])
	}
	if endpoints[0].Headers["X-Custom"] != "custom-value" {
		t.Errorf("expected X-Custom='custom-value', got '%s'", endpoints[0].Headers["X-Custom"])
	}
}

// AC-014: Headers field is optional — endpoints without it behave unchanged
func TestRxDAG_AC014_HeadersFieldOptional(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test-no-headers
    base_url: http://example.com
    path: /api
    format: json
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if endpoints[0].Headers != nil {
		t.Errorf("expected nil headers when not specified, got %v", endpoints[0].Headers)
	}
	if endpoints[0].ResolvedHeaders != nil {
		t.Errorf("expected nil resolved headers when not specified, got %v", endpoints[0].ResolvedHeaders)
	}
}

// AC-003: Header values support ${ENV_VAR} interpolation
func TestRxDAG_AC003_HeaderEnvVarInterpolation(t *testing.T) {
	t.Setenv("TEST_API_KEY", "secret-key-123")

	headers := map[string]string{
		"X-API-Key":    "${TEST_API_KEY}",
		"X-Static":     "no-interpolation",
		"X-Mixed":      "prefix-${TEST_API_KEY}-suffix",
	}

	resolved := config.ResolveHeaders(headers)

	if resolved["X-API-Key"] != "secret-key-123" {
		t.Errorf("expected 'secret-key-123', got '%s'", resolved["X-API-Key"])
	}
	if resolved["X-Static"] != "no-interpolation" {
		t.Errorf("expected 'no-interpolation', got '%s'", resolved["X-Static"])
	}
	if resolved["X-Mixed"] != "prefix-secret-key-123-suffix" {
		t.Errorf("expected 'prefix-secret-key-123-suffix', got '%s'", resolved["X-Mixed"])
	}
}

// AC-013: Missing env var resolves to empty string
func TestRxDAG_AC013_MissingEnvVarResolvesToEmpty(t *testing.T) {
	// Ensure the var is unset
	t.Setenv("RXDAG_MISSING_VAR", "")

	headers := map[string]string{
		"X-API-Key": "${RXDAG_MISSING_VAR}",
	}

	resolved := config.ResolveHeaders(headers)

	if resolved["X-API-Key"] != "" {
		t.Errorf("expected empty string for missing env var, got '%s'", resolved["X-API-Key"])
	}
}

// AC-014: ResolveHeaders with nil map returns nil
func TestRxDAG_AC014_ResolveHeadersNilMap(t *testing.T) {
	resolved := config.ResolveHeaders(nil)
	if resolved != nil {
		t.Errorf("expected nil for nil input, got %v", resolved)
	}
}

// AC-014: ResolveHeaders with empty map returns nil
func TestRxDAG_AC014_ResolveHeadersEmptyMap(t *testing.T) {
	resolved := config.ResolveHeaders(map[string]string{})
	if resolved != nil {
		t.Errorf("expected nil for empty map, got %v", resolved)
	}
}

// AC-002: Headers are resolved during ApplyDefaults
func TestRxDAG_AC002_HeadersResolvedDuringApplyDefaults(t *testing.T) {
	t.Setenv("TEST_HDR_KEY", "resolved-value")

	ep := config.Endpoint{
		Slug:    "test",
		BaseURL: "http://example.com",
		Path:    "/api",
		Format:  "json",
		Headers: map[string]string{
			"X-Key": "${TEST_HDR_KEY}",
		},
	}
	config.ApplyDefaults(&ep)

	if ep.ResolvedHeaders == nil {
		t.Fatal("expected ResolvedHeaders to be populated after ApplyDefaults")
	}
	if ep.ResolvedHeaders["X-Key"] != "resolved-value" {
		t.Errorf("expected 'resolved-value', got '%s'", ep.ResolvedHeaders["X-Key"])
	}
}

// AC-012: Existing endpoints without headers still work after config extension
func TestRxDAG_AC012_ExistingEndpointsUnchanged(t *testing.T) {
	cfgPath := writeTestConfig(t, validConfig)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	for _, ep := range endpoints {
		if ep.Headers != nil {
			t.Errorf("endpoint '%s': expected nil headers for existing endpoints, got %v", ep.Slug, ep.Headers)
		}
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
