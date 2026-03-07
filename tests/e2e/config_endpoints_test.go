package e2e_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/finish06/drugs/internal/config"
)

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
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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

	// Check that the expected data key exists
	dataKey := ep.DataKey
	if dataKey == "" {
		dataKey = "data"
	}
	data, ok := parsed[dataKey]
	if !ok {
		t.Errorf("expected key '%s' in response, got keys: %v", dataKey, mapKeys(parsed))
		return
	}

	// Verify it's an array with at least one item
	arr, ok := data.([]interface{})
	if !ok {
		t.Errorf("expected '%s' to be an array, got %T", dataKey, data)
		return
	}
	if len(arr) == 0 {
		t.Logf("warning: '%s' array is empty for %s", dataKey, ep.Slug)
	} else {
		t.Logf("ok: %s returned %d items", ep.Slug, len(arr))
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

