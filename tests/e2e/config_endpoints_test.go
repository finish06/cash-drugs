package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/config"
)

// envVarRefRegex matches ${VAR_NAME} references in header values.
var envVarRefRegex = regexp.MustCompile(`\$\{([A-Z_][A-Z0-9_]*)\}`)

// missingHeaderEnvVars returns the names of env vars referenced by ep.Headers
// that are currently unset or empty. Lets the E2E test skip endpoints whose
// upstream auth can't be satisfied in the current environment (e.g. the rx-dag
// slugs when RXDAG_API_KEY isn't exported).
func missingHeaderEnvVars(ep config.Endpoint) []string {
	var missing []string
	for _, value := range ep.Headers {
		for _, match := range envVarRefRegex.FindAllStringSubmatch(value, -1) {
			name := match[1]
			if os.Getenv(name) == "" {
				missing = append(missing, name)
			}
		}
	}
	return missing
}

// TestAC013_AllConfigEndpointsAgainstRealAPIs validates every endpoint in
// config.yaml against the real upstream API with minimal data (limit=1 or pagesize=1).
// This test requires network access and is skipped with -short.
func TestAC013_AllConfigEndpointsAgainstRealAPIs(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping E2E test in short mode")
	}

	endpoints, err := config.Load("../../config.yaml")
	if err != nil {
		t.Fatalf("failed to load config.yaml: %v", err)
	}

	client := &http.Client{Timeout: 30 * time.Second}

	for _, ep := range endpoints {
		t.Run(ep.Slug, func(t *testing.T) {
			// Skip endpoints whose upstream auth env vars aren't exported.
			// e.g. rx-dag slugs need RXDAG_API_KEY; without it the upstream
			// returns 401 and the test fails in a way that tells us nothing
			// about the code under test.
			if missing := missingHeaderEnvVars(ep); len(missing) > 0 {
				t.Skipf("skipping %s — missing env vars %v for upstream auth headers", ep.Slug, missing)
				return
			}

			// Skip endpoints that require path/query params we can't provide generically
			requiredParams := config.ExtractAllParams(ep)
			if len(requiredParams) > 0 {
				// For parameterized endpoints, provide test values
				testParams := getTestParams(ep.Slug)
				if testParams == nil {
					t.Skipf("skipping %s — requires params %v with no test values", ep.Slug, requiredParams)
					return
				}
				testParameterizedEndpoint(t, client, ep, testParams)
				return
			}

			testPrefetchEndpoint(t, client, ep)
		})
	}
}

func testPrefetchEndpoint(t *testing.T, client *http.Client, ep config.Endpoint) {
	t.Helper()

	reqURL := buildTestURL(ep, nil)
	t.Logf("GET %s", reqURL)

	resp, err := client.Get(reqURL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	if ep.Format == "json" {
		validateJSONResponse(t, resp, ep)
	}
}

func testParameterizedEndpoint(t *testing.T, client *http.Client, ep config.Endpoint, params map[string]string) {
	t.Helper()

	reqURL := buildTestURL(ep, params)
	t.Logf("GET %s", reqURL)

	resp, err := client.Get(reqURL)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, string(body))
	}

	if ep.Format == "json" {
		validateJSONResponse(t, resp, ep)
	}
}

func validateJSONResponse(t *testing.T, resp *http.Response, ep config.Endpoint) {
	t.Helper()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("failed to read response body: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	// Traverse dot-path data key to find the expected data
	dataKey := ep.DataKey
	if dataKey == "" {
		dataKey = "data"
	}

	var data interface{} = parsed
	for _, part := range strings.Split(dataKey, ".") {
		m, ok := data.(map[string]interface{})
		if !ok {
			t.Errorf("expected object at '%s' in path '%s', got %T", part, dataKey, data)
			return
		}
		data, ok = m[part]
		if !ok {
			t.Errorf("key '%s' not found in path '%s', available keys: %v", part, dataKey, mapKeys(m))
			return
		}
	}

	// Data can be an array, object, or string array — all are valid
	switch v := data.(type) {
	case []interface{}:
		if len(v) == 0 {
			t.Logf("warning: '%s' array is empty for %s", dataKey, ep.Slug)
		} else {
			t.Logf("ok: %s returned %d items", ep.Slug, len(v))
		}
	case map[string]interface{}:
		t.Logf("ok: %s returned object with %d fields", ep.Slug, len(v))
	case string:
		t.Logf("ok: %s returned string value", ep.Slug)
	case nil:
		t.Errorf("'%s' is null for %s", dataKey, ep.Slug)
	default:
		t.Logf("ok: %s returned %T", ep.Slug, data)
	}
}

func buildTestURL(ep config.Endpoint, params map[string]string) string {
	path := config.SubstitutePathParams(ep.Path, params)
	u := ep.BaseURL + path

	q := url.Values{}
	for k, v := range ep.QueryParams {
		q.Set(k, config.SubstitutePathParams(v, params))
	}

	// Request minimal data
	if ep.PaginationStyle == "offset" {
		q.Set("limit", "1")
	} else {
		maxPages, fetchAll := config.ParsePagination(ep)
		if fetchAll || maxPages > 1 {
			if ep.PagesizeParam != "" {
				q.Set(ep.PagesizeParam, "1")
			}
			if ep.PageParam != "" {
				q.Set(ep.PageParam, "1")
			}
		}
	}

	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}

func getTestParams(slug string) map[string]string {
	switch slug {
	case "spl-detail":
		return map[string]string{"SETID": "a33d860a-1e17-45b7-aede-51ab3223bebd"}
	case "spl-xml":
		return map[string]string{"SETID": "a33d860a-1e17-45b7-aede-51ab3223bebd"}
	case "spls-by-name":
		return map[string]string{"DRUGNAME": "aspirin"}
	case "spls-by-class":
		return map[string]string{"DRUG_CLASS": "Anti-Inflammatory"}
	case "fda-ndc-by-name":
		return map[string]string{"BRAND_NAME": "aspirin"}
	case "fda-drugsfda-by-name":
		return map[string]string{"BRAND_NAME": "aspirin"}
	case "fda-labels-by-name":
		return map[string]string{"DRUG_NAME": "aspirin"}
	case "fda-events-by-drug":
		return map[string]string{"DRUG_NAME": "aspirin"}
	case "fda-ndc":
		return map[string]string{"BRAND_NAME": "aspirin"}
	case "fda-label":
		return map[string]string{"BRAND_NAME": "aspirin"}
	case "rxnorm-find-drug":
		return map[string]string{"DRUG_NAME": "metformin"}
	case "rxnorm-approximate-match":
		return map[string]string{"DRUG_NAME": "ambienn"}
	case "rxnorm-spelling-suggestions":
		return map[string]string{"DRUG_NAME": "ambienn"}
	case "rxnorm-ndcs":
		return map[string]string{"RXCUI": "861004"}
	case "rxnorm-generic-product":
		return map[string]string{"RXCUI": "213269"}
	case "rxnorm-all-related":
		return map[string]string{"RXCUI": "161"}
	default:
		// Check if it has params we don't know about
		return nil
	}
}

func mapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
