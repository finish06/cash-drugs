package upstream_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sort"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/upstream"
)

// AC-004: On consumer request, fetch from upstream API
func TestAC004_FetchFromUpstream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data":     []string{"drug1", "drug2"},
			"metadata": map[string]interface{}{"total_pages": 1},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "test",
		BaseURL: server.URL,
		Path:    "/api",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	result, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Data == nil {
		t.Error("expected data in result")
	}
	if result.PageCount != 1 {
		t.Errorf("expected page_count=1, got %d", result.PageCount)
	}
}

// AC-005: Auto-paginate upstream responses
func TestAC005_AutoPaginate(t *testing.T) {
	pageCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		totalPages := 3
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []string{fmt.Sprintf("item-page-%d", pageCount)},
			"metadata": map[string]interface{}{
				"total_pages":  totalPages,
				"current_page": pageCount,
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:       "test",
		BaseURL:    server.URL,
		Path:       "/api",
		Format:     "json",
		Pagination: "all",
	}
	config.ApplyDefaults(&ep)

	result, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.PageCount != 3 {
		t.Errorf("expected 3 pages fetched, got %d", result.PageCount)
	}
}

// AC-006: Pagination limit is configurable — numeric cap
func TestAC006_PaginationNumericLimit(t *testing.T) {
	pageCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []string{fmt.Sprintf("item-%d", pageCount)},
			"metadata": map[string]interface{}{
				"total_pages":  10,
				"current_page": pageCount,
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:       "test",
		BaseURL:    server.URL,
		Path:       "/api",
		Format:     "json",
		Pagination: 3,
	}
	config.ApplyDefaults(&ep)

	result, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.PageCount != 3 {
		t.Errorf("expected 3 pages, got %d", result.PageCount)
	}
	if pageCount != 3 {
		t.Errorf("expected 3 upstream requests, got %d", pageCount)
	}
}

// AC-003: Path parameter substitution in upstream URL
func TestAC003_PathParameterSubstitution(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		json.NewEncoder(w).Encode(map[string]interface{}{"data": "ok"})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "spl-detail",
		BaseURL: server.URL,
		Path:    "/v2/spls/{SETID}",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	params := map[string]string{"SETID": "abc-123"}
	_, err := upstream.Fetch(ep, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if receivedPath != "/v2/spls/abc-123" {
		t.Errorf("expected path /v2/spls/abc-123, got %s", receivedPath)
	}
}

// AC-009/AC-010: Upstream failure
func TestAC009_UpstreamFailure(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "test",
		BaseURL: server.URL,
		Path:    "/api",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	_, err := upstream.Fetch(ep, nil)
	if err == nil {
		t.Fatal("expected error for upstream failure")
	}
}

// AC-011: Pagination failure midway discards partial data
func TestAC011_PaginationFailureMidway(t *testing.T) {
	pageCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		if pageCount == 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []string{fmt.Sprintf("item-%d", pageCount)},
			"metadata": map[string]interface{}{
				"total_pages":  5,
				"current_page": pageCount,
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:       "test",
		BaseURL:    server.URL,
		Path:       "/api",
		Format:     "json",
		Pagination: "all",
	}
	config.ApplyDefaults(&ep)

	_, err := upstream.Fetch(ep, nil)
	if err == nil {
		t.Fatal("expected error when pagination fails midway")
	}
}

// AC-017: Raw format fetches upstream response as-is
func TestAC017_RawFormatFetch(t *testing.T) {
	xmlBody := `<document><title>Test SPL</title></document>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(xmlBody))
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "spl-xml",
		BaseURL: server.URL,
		Path:    "/v2/spls/abc.xml",
		Format:  "raw",
	}
	config.ApplyDefaults(&ep)

	result, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.ContentType != "application/xml" {
		t.Errorf("expected content type 'application/xml', got '%s'", result.ContentType)
	}
	if result.Data != xmlBody {
		t.Errorf("expected raw body stored as string, got %v", result.Data)
	}
	if result.PageCount != 1 {
		t.Errorf("expected page_count=1, got %d", result.PageCount)
	}
}

// AC-017: Raw format with path param substitution
func TestAC017_RawFormatPathParamSubstitution(t *testing.T) {
	var receivedPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte("<doc/>"))
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "spl-xml",
		BaseURL: server.URL,
		Path:    "/v2/spls/{SETID}.xml",
		Format:  "raw",
	}
	config.ApplyDefaults(&ep)

	params := map[string]string{"SETID": "abc-123"}
	_, err := upstream.Fetch(ep, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if receivedPath != "/v2/spls/abc-123.xml" {
		t.Errorf("expected path /v2/spls/abc-123.xml, got %s", receivedPath)
	}
}

// AC-016: Query param {PARAM} substitution in upstream URL
func TestAC016_QueryParamSubstitution(t *testing.T) {
	var receivedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.Query().Get("setid")
		json.NewEncoder(w).Encode(map[string]interface{}{"data": "ok"})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "spl-detail",
		BaseURL: server.URL,
		Path:    "/v2/spls.json",
		Format:  "json",
		QueryParams: map[string]string{
			"setid": "{SETID}",
		},
	}
	config.ApplyDefaults(&ep)

	params := map[string]string{"SETID": "abc-123"}
	_, err := upstream.Fetch(ep, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if receivedQuery != "abc-123" {
		t.Errorf("expected query setid=abc-123, got '%s'", receivedQuery)
	}
}

// AC-019: Multi-page fetch populates Pages field
func TestAC019_MultiPageFetchPopulatesPages(t *testing.T) {
	pageCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []string{fmt.Sprintf("item-page-%d", pageCount)},
			"metadata": map[string]interface{}{
				"total_pages":  3,
				"current_page": pageCount,
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:       "test",
		BaseURL:    server.URL,
		Path:       "/api",
		Format:     "json",
		Pagination: "all",
	}
	config.ApplyDefaults(&ep)

	result, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(result.Pages) != 3 {
		t.Errorf("expected 3 pages, got %d", len(result.Pages))
	}
	for i, p := range result.Pages {
		if p.Page != i+1 {
			t.Errorf("expected page %d, got %d", i+1, p.Page)
		}
		if len(p.Data) == 0 {
			t.Errorf("expected data on page %d", i+1)
		}
	}
	// Combined Data should have all items
	allData, ok := result.Data.([]interface{})
	if !ok {
		t.Fatalf("expected Data to be []interface{}, got %T", result.Data)
	}
	if len(allData) != 3 {
		t.Errorf("expected 3 combined items, got %d", len(allData))
	}
}

// fetchRaw: empty Content-Type defaults to application/octet-stream
func TestFetchRaw_EmptyContentType_DefaultsToOctetStream(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Explicitly set Content-Type to empty to override Go's sniffing,
		// then delete it so the header is absent.
		w.Header()["Content-Type"] = nil
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("some binary data"))
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "test-raw",
		BaseURL: server.URL,
		Path:    "/data",
		Format:  "raw",
	}
	config.ApplyDefaults(&ep)

	result, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.ContentType != "application/octet-stream" {
		t.Errorf("expected content type 'application/octet-stream', got '%s'", result.ContentType)
	}
	if result.Data != "some binary data" {
		t.Errorf("expected raw body, got %v", result.Data)
	}
}

// fetchRaw: upstream returns 4xx/5xx error
func TestFetchRaw_UpstreamError(t *testing.T) {
	tests := []struct {
		name       string
		statusCode int
	}{
		{"400 Bad Request", http.StatusBadRequest},
		{"404 Not Found", http.StatusNotFound},
		{"500 Internal Server Error", http.StatusInternalServerError},
		{"503 Service Unavailable", http.StatusServiceUnavailable},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.statusCode)
			}))
			defer server.Close()

			ep := config.Endpoint{
				Slug:    "test-raw",
				BaseURL: server.URL,
				Path:    "/data",
				Format:  "raw",
			}
			config.ApplyDefaults(&ep)

			_, err := upstream.Fetch(ep, nil)
			if err == nil {
				t.Fatalf("expected error for status %d, got nil", tc.statusCode)
			}
		})
	}
}

// fetchJSONPage: response has no "data" key wraps entire response as single item
func TestFetchJSONPage_NoDataKey_WrapsResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []string{"a", "b"},
			"total":   2,
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "test-nodata",
		BaseURL: server.URL,
		Path:    "/api",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	result, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	allData, ok := result.Data.([]interface{})
	if !ok {
		t.Fatalf("expected Data to be []interface{}, got %T", result.Data)
	}
	// The entire response map should be wrapped as a single item
	if len(allData) != 1 {
		t.Errorf("expected 1 item (wrapped response), got %d", len(allData))
	}
	// The single item should be a map containing "results" and "total"
	item, ok := allData[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected item to be map[string]interface{}, got %T", allData[0])
	}
	if _, exists := item["results"]; !exists {
		t.Error("expected wrapped response to contain 'results' key")
	}
}

// hasMorePages: missing metadata should stop pagination
func TestHasMorePages_MissingMetadata_StopsPagination(t *testing.T) {
	pageCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		// Return data without any metadata field
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []string{fmt.Sprintf("item-%d", pageCount)},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:       "test-nometa",
		BaseURL:    server.URL,
		Path:       "/api",
		Format:     "json",
		Pagination: "all",
	}
	config.ApplyDefaults(&ep)

	result, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	// Without metadata, hasMorePages returns false, so only 1 page should be fetched
	if result.PageCount != 1 {
		t.Errorf("expected 1 page (no metadata to indicate more), got %d", result.PageCount)
	}
	if pageCount != 1 {
		t.Errorf("expected 1 upstream request, got %d", pageCount)
	}
}

// AC-004: URL construction with query params
func TestAC004_URLConstructionWithQueryParams(t *testing.T) {
	var receivedQuery string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.RawQuery
		json.NewEncoder(w).Encode(map[string]interface{}{"data": "ok"})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "test",
		BaseURL: server.URL,
		Path:    "/api",
		Format:  "json",
		QueryParams: map[string]string{
			"key": "value",
		},
	}
	config.ApplyDefaults(&ep)

	_, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should include the query param
	if receivedQuery == "" {
		t.Error("expected query parameters in request")
	}
}

// ---------------------------------------------------------------------------
// FDA API Integration — RED phase tests (new fields: PaginationStyle, DataKey, TotalKey)
// These tests reference struct fields and behaviors that DO NOT EXIST YET.
// They are expected to fail at compile time until the GREEN phase implements them.
// ---------------------------------------------------------------------------

// AC-002: Offset pagination sends skip & limit query params
func TestFDA_AC002_OffsetPaginationSendsSkipLimit(t *testing.T) {
	var mu sync.Mutex
	var requests []url.Values
	totalItems := 25

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.Query())
		mu.Unlock()

		// Generate items for this page based on skip/limit
		items := []interface{}{}
		for i := 0; i < 10 && len(items) < 10; i++ {
			items = append(items, map[string]interface{}{"id": i})
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": items,
			"meta": map[string]interface{}{
				"results": map[string]interface{}{
					"total": totalItems,
				},
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:            "fda-drugs",
		BaseURL:         server.URL,
		Path:            "/drug/label.json",
		Format:          "json",
		PaginationStyle: "offset",
		DataKey:         "results",
		TotalKey:        "meta.results.total",
		Pagination:      "all",
		Pagesize:        10,
	}
	config.ApplyDefaults(&ep)

	result, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should have made 3 requests: skip=0, skip=10, skip=20
	if len(requests) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(requests))
	}

	// First request is always sequential (skip=0)
	if requests[0].Get("skip") != "0" {
		t.Errorf("first request: expected skip=0, got skip=%s", requests[0].Get("skip"))
	}

	// Collect all skip values and verify the expected set is present
	var skips []string
	for _, q := range requests {
		skips = append(skips, q.Get("skip"))
		limit := q.Get("limit")
		if limit != "10" {
			t.Errorf("expected limit=10, got limit=%s", limit)
		}
	}
	sort.Strings(skips)
	expectedSkips := []string{"0", "10", "20"}
	sort.Strings(expectedSkips)
	for i, expected := range expectedSkips {
		if skips[i] != expected {
			t.Errorf("expected skip=%s in sorted requests, got skip=%s", expected, skips[i])
		}
	}

	if result.PageCount != 3 {
		t.Errorf("expected page_count=3, got %d", result.PageCount)
	}
}

// AC-003: Configurable data_key extracts items from the correct response key
func TestFDA_AC003_ConfigurableDataKey(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []interface{}{
				map[string]interface{}{"name": "aspirin"},
			},
			"meta": map[string]interface{}{
				"results": map[string]interface{}{
					"total": 1,
				},
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:     "fda-drugs",
		BaseURL:  server.URL,
		Path:     "/drug/label.json",
		Format:   "json",
		DataKey:  "results",
		TotalKey: "meta.results.total",
	}
	config.ApplyDefaults(&ep)

	result, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Data should come from "results" key, not "data"
	allData, ok := result.Data.([]interface{})
	if !ok {
		t.Fatalf("expected Data to be []interface{}, got %T", result.Data)
	}
	if len(allData) != 1 {
		t.Fatalf("expected 1 item, got %d", len(allData))
	}

	item, ok := allData[0].(map[string]interface{})
	if !ok {
		t.Fatalf("expected item to be map, got %T", allData[0])
	}
	if item["name"] != "aspirin" {
		t.Errorf("expected name=aspirin, got %v", item["name"])
	}
}

// AC-004/AC-016: Configurable total_key with dot-notation traversal for nested totals
func TestFDA_AC004_AC016_ConfigurableTotalKeyDotNotation(t *testing.T) {
	requestCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []interface{}{
				map[string]interface{}{"id": requestCount},
			},
			"meta": map[string]interface{}{
				"results": map[string]interface{}{
					"total": 3,
				},
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:            "fda-drugs",
		BaseURL:         server.URL,
		Path:            "/drug/label.json",
		Format:          "json",
		PaginationStyle: "offset",
		DataKey:         "results",
		TotalKey:        "meta.results.total",
		Pagination:      "all",
		Pagesize:        1,
	}
	config.ApplyDefaults(&ep)

	result, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// With total=3 and pagesize=1, fetcher should make exactly 3 requests
	if requestCount != 3 {
		t.Errorf("expected 3 upstream requests, got %d", requestCount)
	}
	if result.PageCount != 3 {
		t.Errorf("expected page_count=3, got %d", result.PageCount)
	}
}

// AC-005: Backward compatibility — page-based pagination still works when no PaginationStyle is set
func TestFDA_AC005_BackwardCompatPagePagination(t *testing.T) {
	var mu sync.Mutex
	var requests []url.Values
	pageCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requests = append(requests, r.URL.Query())
		pageCount++
		mu.Unlock()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []interface{}{
				map[string]interface{}{"item": pageCount},
			},
			"metadata": map[string]interface{}{
				"total_pages":  2,
				"current_page": pageCount,
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:       "dailymed-spls",
		BaseURL:    server.URL,
		Path:       "/api",
		Format:     "json",
		Pagination: "all",
		// No PaginationStyle, DataKey, or TotalKey set — should use defaults
	}
	config.ApplyDefaults(&ep)

	result, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Should use page/pagesize params (not skip/limit)
	for i, q := range requests {
		if q.Get("skip") != "" {
			t.Errorf("request %d: should NOT have skip param for page-based pagination", i)
		}
		if q.Get("limit") != "" {
			t.Errorf("request %d: should NOT have limit param for page-based pagination", i)
		}
		if q.Get("page") == "" {
			t.Errorf("request %d: expected page param for page-based pagination", i)
		}
	}

	// Should read data from "data" key (default)
	allData, ok := result.Data.([]interface{})
	if !ok {
		t.Fatalf("expected Data to be []interface{}, got %T", result.Data)
	}
	if len(allData) != 2 {
		t.Errorf("expected 2 combined items from 2 pages, got %d", len(allData))
	}

	if result.PageCount != 2 {
		t.Errorf("expected page_count=2, got %d", result.PageCount)
	}
}

// AC-012: Graceful skip/limit handling — fetcher stops on error and returns partial data
func TestFDA_AC012_GracefulSkipLimitHandling(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query()
		skip := q.Get("skip")
		// Simulate FDA's behavior: error when skip >= 100 (simulating 25K cap at small scale)
		if skip == "100" || skip == "150" {
			w.WriteHeader(http.StatusBadRequest)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error": map[string]interface{}{
					"code":    "BAD_REQUEST",
					"message": "Skip exceeds maximum allowed value",
				},
			})
			return
		}

		items := []interface{}{}
		for i := 0; i < 50; i++ {
			items = append(items, map[string]interface{}{"id": i})
		}

		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": items,
			"meta": map[string]interface{}{
				"results": map[string]interface{}{
					"total": 200,
				},
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:            "fda-drugs",
		BaseURL:         server.URL,
		Path:            "/drug/label.json",
		Format:          "json",
		PaginationStyle: "offset",
		DataKey:         "results",
		TotalKey:        "meta.results.total",
		Pagination:      "all",
		Pagesize:        50,
	}
	config.ApplyDefaults(&ep)

	result, err := upstream.Fetch(ep, nil)
	// Should NOT crash — should return partial data gracefully
	if err != nil {
		t.Fatalf("expected graceful handling (no error), got %v", err)
	}

	// Should have fetched pages for skip=0 and skip=50 successfully (2 pages)
	// skip=100 returns error, so pagination should stop
	if result.PageCount != 2 {
		t.Errorf("expected page_count=2 (partial data before error), got %d", result.PageCount)
	}

	// Should have data from the successful pages
	allData, ok := result.Data.([]interface{})
	if !ok {
		t.Fatalf("expected Data to be []interface{}, got %T", result.Data)
	}
	if len(allData) != 100 {
		t.Errorf("expected 100 items from 2 successful pages, got %d", len(allData))
	}
}

// AC-017: Offset skip calculation is (page-1) * pagesize
func TestFDA_AC017_OffsetSkipCalculation(t *testing.T) {
	var mu sync.Mutex
	var receivedSkips []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedSkips = append(receivedSkips, r.URL.Query().Get("skip"))
		mu.Unlock()

		items := []interface{}{map[string]interface{}{"id": 1}}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": items,
			"meta": map[string]interface{}{
				"results": map[string]interface{}{
					"total": 75,
				},
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:            "fda-drugs",
		BaseURL:         server.URL,
		Path:            "/drug/label.json",
		Format:          "json",
		PaginationStyle: "offset",
		DataKey:         "results",
		TotalKey:        "meta.results.total",
		Pagination:      3,
		Pagesize:        25,
	}
	config.ApplyDefaults(&ep)

	_, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if len(receivedSkips) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(receivedSkips))
	}

	// With parallel fetching, request order is non-deterministic after page 1.
	// Verify first request is skip=0 (sequential), then remaining contain 25 and 50.
	if receivedSkips[0] != "0" {
		t.Errorf("first request: expected skip=0, got skip=%s", receivedSkips[0])
	}
	expectedRemaining := map[string]bool{"25": false, "50": false}
	for _, skip := range receivedSkips[1:] {
		if _, ok := expectedRemaining[skip]; !ok {
			t.Errorf("unexpected skip value: %s", skip)
		}
		expectedRemaining[skip] = true
	}
	for skip, found := range expectedRemaining {
		if !found {
			t.Errorf("expected skip=%s in requests, not found", skip)
		}
	}
}

// Test: Optional query params — unresolved {PLACEHOLDER} params should be skipped
func TestOptionalQueryParams_UnresolvedPlaceholdersSkipped(t *testing.T) {
	var receivedQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.Query()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data":     []string{"result1"},
			"metadata": map[string]interface{}{"total_pages": 1},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "test-optional",
		BaseURL: server.URL,
		Path:    "/api/search",
		Format:  "json",
		QueryParams: map[string]string{
			"brand_name":   "{BRAND_NAME}",
			"generic_name": "{GENERIC_NAME}",
			"status":       "active", // static param, always sent
		},
	}
	config.ApplyDefaults(&ep)

	// Only provide BRAND_NAME, not GENERIC_NAME
	params := map[string]string{
		"BRAND_NAME": "Tylenol",
	}

	result, err := upstream.Fetch(ep, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// brand_name should be sent with the resolved value
	if receivedQuery.Get("brand_name") != "Tylenol" {
		t.Errorf("expected brand_name=Tylenol, got %q", receivedQuery.Get("brand_name"))
	}

	// static param should always be sent
	if receivedQuery.Get("status") != "active" {
		t.Errorf("expected status=active, got %q", receivedQuery.Get("status"))
	}

	// generic_name should NOT be sent (unresolved placeholder)
	if receivedQuery.Has("generic_name") {
		t.Errorf("expected generic_name to be omitted (unresolved placeholder), but got %q", receivedQuery.Get("generic_name"))
	}
}

func TestOptionalQueryParams_AllPlaceholdersResolved(t *testing.T) {
	var receivedQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.Query()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data":     []string{"result1"},
			"metadata": map[string]interface{}{"total_pages": 1},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "test-optional",
		BaseURL: server.URL,
		Path:    "/api/search",
		Format:  "json",
		QueryParams: map[string]string{
			"brand_name":   "{BRAND_NAME}",
			"generic_name": "{GENERIC_NAME}",
		},
	}
	config.ApplyDefaults(&ep)

	// Provide both params
	params := map[string]string{
		"BRAND_NAME":   "Tylenol",
		"GENERIC_NAME": "Acetaminophen",
	}

	_, err := upstream.Fetch(ep, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Both should be sent
	if receivedQuery.Get("brand_name") != "Tylenol" {
		t.Errorf("expected brand_name=Tylenol, got %q", receivedQuery.Get("brand_name"))
	}
	if receivedQuery.Get("generic_name") != "Acetaminophen" {
		t.Errorf("expected generic_name=Acetaminophen, got %q", receivedQuery.Get("generic_name"))
	}
}

func TestOptionalQueryParams_NoPlaceholdersProvided(t *testing.T) {
	var receivedQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.Query()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data":     []string{"result1"},
			"metadata": map[string]interface{}{"total_pages": 1},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "test-optional",
		BaseURL: server.URL,
		Path:    "/api/search",
		Format:  "json",
		QueryParams: map[string]string{
			"brand_name":   "{BRAND_NAME}",
			"generic_name": "{GENERIC_NAME}",
			"status":       "active",
		},
	}
	config.ApplyDefaults(&ep)

	// No placeholder params provided — only static params should be sent
	_, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if receivedQuery.Get("status") != "active" {
		t.Errorf("expected status=active, got %q", receivedQuery.Get("status"))
	}
	if receivedQuery.Has("brand_name") {
		t.Errorf("expected brand_name to be omitted, but got %q", receivedQuery.Get("brand_name"))
	}
	if receivedQuery.Has("generic_name") {
		t.Errorf("expected generic_name to be omitted, but got %q", receivedQuery.Get("generic_name"))
	}
}

// Test: search_params combines resolved clauses into a single "search" query param
func TestSearchParams_PartialResolution(t *testing.T) {
	var receivedQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.Query()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []interface{}{map[string]interface{}{"product_ndc": "12345"}},
			"meta":    map[string]interface{}{"results": map[string]interface{}{"total": 1}},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: server.URL,
		Path:    "/drug/ndc.json",
		Format:  "json",
		SearchParams: []string{
			"brand_name:\"{BRAND_NAME}\"",
			"generic_name:\"{GENERIC_NAME}\"",
			"product_ndc:\"{NDC}\"",
		},
		DataKey:         "results",
		TotalKey:        "meta.results.total",
		PaginationStyle: "offset",
		Pagesize:        100,
	}
	config.ApplyDefaults(&ep)

	// Only provide BRAND_NAME
	params := map[string]string{"BRAND_NAME": "Tylenol"}

	result, err := upstream.Fetch(ep, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result == nil {
		t.Fatal("expected result, got nil")
	}

	// search param should only contain the resolved brand_name clause
	search := receivedQuery.Get("search")
	if search != "brand_name:\"Tylenol\"" {
		t.Errorf("expected search='brand_name:\"Tylenol\"', got %q", search)
	}
}

func TestSearchParams_AllResolved(t *testing.T) {
	var receivedQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.Query()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []interface{}{map[string]interface{}{"product_ndc": "12345"}},
			"meta":    map[string]interface{}{"results": map[string]interface{}{"total": 1}},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: server.URL,
		Path:    "/drug/ndc.json",
		Format:  "json",
		SearchParams: []string{
			"brand_name:\"{BRAND_NAME}\"",
			"generic_name:\"{GENERIC_NAME}\"",
		},
		DataKey:         "results",
		TotalKey:        "meta.results.total",
		PaginationStyle: "offset",
		Pagesize:        100,
	}
	config.ApplyDefaults(&ep)

	params := map[string]string{
		"BRAND_NAME":   "Tylenol",
		"GENERIC_NAME": "Acetaminophen",
	}

	_, err := upstream.Fetch(ep, params)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	// Both clauses joined with +
	search := receivedQuery.Get("search")
	expected := "brand_name:\"Tylenol\"+generic_name:\"Acetaminophen\""
	if search != expected {
		t.Errorf("expected search=%q, got %q", expected, search)
	}
}

func TestSearchParams_NoneResolved(t *testing.T) {
	var receivedQuery url.Values
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedQuery = r.URL.Query()
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []interface{}{map[string]interface{}{"product_ndc": "12345"}},
			"meta":    map[string]interface{}{"results": map[string]interface{}{"total": 1}},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: server.URL,
		Path:    "/drug/ndc.json",
		Format:  "json",
		SearchParams: []string{
			"brand_name:\"{BRAND_NAME}\"",
			"generic_name:\"{GENERIC_NAME}\"",
		},
		DataKey:         "results",
		TotalKey:        "meta.results.total",
		PaginationStyle: "offset",
		Pagesize:        100,
	}
	config.ApplyDefaults(&ep)

	// No params provided — search param should not be sent
	_, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if receivedQuery.Has("search") {
		t.Errorf("expected no search param when all clauses unresolved, got %q", receivedQuery.Get("search"))
	}
}

// ---------------------------------------------------------------------------
// M10: Parallel Page Fetches — RED phase tests
// Spec: specs/parallel-page-fetches.md
// ---------------------------------------------------------------------------

// M10-AC-001/AC-002/AC-004: First page sequential, remaining concurrent, results in page order
func TestM10_AC001_AC002_AC004_ParallelPageFetchPageOrder(t *testing.T) {
	pageCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pageCount++
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []interface{}{fmt.Sprintf("item-page-%d", pageCount)},
			"metadata": map[string]interface{}{
				"total_pages":  6,
				"current_page": pageCount,
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:       "test-parallel",
		BaseURL:    server.URL,
		Path:       "/api",
		Format:     "json",
		Pagination: "all",
	}
	config.ApplyDefaults(&ep)

	fetcher := upstream.NewHTTPFetcher()
	fetcher.FetchConcurrency = 3

	result, err := fetcher.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.PageCount != 6 {
		t.Errorf("expected 6 pages, got %d", result.PageCount)
	}
	// Verify all pages present in order
	if len(result.Pages) != 6 {
		t.Fatalf("expected 6 pages in Pages slice, got %d", len(result.Pages))
	}
	for i, p := range result.Pages {
		if p.Page != i+1 {
			t.Errorf("expected page %d at index %d, got page %d", i+1, i, p.Page)
		}
	}
	// Combined data should have items from all 6 pages
	allData, ok := result.Data.([]interface{})
	if !ok {
		t.Fatalf("expected Data to be []interface{}, got %T", result.Data)
	}
	if len(allData) != 6 {
		t.Errorf("expected 6 combined items, got %d", len(allData))
	}
}

// M10-AC-003: Concurrency cap is respected
func TestM10_AC003_ConcurrencyCapRespected(t *testing.T) {
	var maxConcurrent atomic.Int32
	var currentConcurrent atomic.Int32

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cur := currentConcurrent.Add(1)
		// Track max concurrent
		for {
			old := maxConcurrent.Load()
			if cur <= old || maxConcurrent.CompareAndSwap(old, cur) {
				break
			}
		}
		time.Sleep(50 * time.Millisecond) // simulate work
		currentConcurrent.Add(-1)

		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []interface{}{"item"},
			"metadata": map[string]interface{}{
				"total_pages": 10,
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:       "test-cap",
		BaseURL:    server.URL,
		Path:       "/api",
		Format:     "json",
		Pagination: "all",
	}
	config.ApplyDefaults(&ep)

	fetcher := upstream.NewHTTPFetcher()
	fetcher.FetchConcurrency = 3

	result, err := fetcher.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.PageCount != 10 {
		t.Errorf("expected 10 pages, got %d", result.PageCount)
	}
	// Max concurrent should not exceed cap + 1 (the first sequential page may overlap briefly)
	// But after first page, at most 3 should be in-flight
	if maxConcurrent.Load() > 4 {
		t.Errorf("expected max concurrent <= 4 (1 sequential + 3 parallel), got %d", maxConcurrent.Load())
	}
}

// M10-AC-005: Error on any page fails entire fetch (page-style pagination)
func TestM10_AC005_ErrorFailsEntireFetch(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 3 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []interface{}{"item"},
			"metadata": map[string]interface{}{
				"total_pages": 4,
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:       "test-error",
		BaseURL:    server.URL,
		Path:       "/api",
		Format:     "json",
		Pagination: "all",
	}
	config.ApplyDefaults(&ep)

	fetcher := upstream.NewHTTPFetcher()
	fetcher.FetchConcurrency = 3

	_, err := fetcher.Fetch(ep, nil)
	if err == nil {
		t.Fatal("expected error when a page fails, got nil")
	}
}

// M10-AC-006: Offset pagination returns partial results on error
func TestM10_AC006_OffsetPartialResultsOnError(t *testing.T) {
	requestCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if requestCount == 3 {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []interface{}{map[string]interface{}{"id": requestCount}},
			"meta": map[string]interface{}{
				"results": map[string]interface{}{
					"total": 200,
				},
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:            "test-offset-partial",
		BaseURL:         server.URL,
		Path:            "/api",
		Format:          "json",
		PaginationStyle: "offset",
		DataKey:         "results",
		TotalKey:        "meta.results.total",
		Pagination:      "all",
		Pagesize:        50,
	}
	config.ApplyDefaults(&ep)

	fetcher := upstream.NewHTTPFetcher()
	fetcher.FetchConcurrency = 3

	result, err := fetcher.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error for offset partial results, got %v", err)
	}
	// Should have partial data (pages before the error)
	if result.PageCount < 1 {
		t.Errorf("expected at least 1 page of partial data, got %d", result.PageCount)
	}
}

// M10-AC-007: Single-page endpoints unaffected (no goroutines spawned)
func TestM10_AC007_SinglePageUnaffected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data":     []interface{}{"single-item"},
			"metadata": map[string]interface{}{"total_pages": 1},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "test-single",
		BaseURL: server.URL,
		Path:    "/api",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	fetcher := upstream.NewHTTPFetcher()
	fetcher.FetchConcurrency = 3

	result, err := fetcher.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.PageCount != 1 {
		t.Errorf("expected 1 page, got %d", result.PageCount)
	}
}

// M10-AC-013: Timing — 6 pages with concurrency=3 completes in ~2x single-page latency
func TestM10_AC013_TimingBenchmark(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond) // simulate latency
		json.NewEncoder(w).Encode(map[string]interface{}{
			"data": []interface{}{"item"},
			"metadata": map[string]interface{}{
				"total_pages": 6,
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:       "test-timing",
		BaseURL:    server.URL,
		Path:       "/api",
		Format:     "json",
		Pagination: "all",
	}
	config.ApplyDefaults(&ep)

	fetcher := upstream.NewHTTPFetcher()
	fetcher.FetchConcurrency = 3

	start := time.Now()
	result, err := fetcher.Fetch(ep, nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.PageCount != 6 {
		t.Errorf("expected 6 pages, got %d", result.PageCount)
	}

	// Sequential would be ~300ms (6 * 50ms)
	// Parallel should be ~150ms (1 sequential + ceil(5/3)*50ms = 1+2 batches = 150ms)
	// Allow generous margin for CI: should be under 250ms (not 300ms+)
	if elapsed > 250*time.Millisecond {
		t.Errorf("expected parallel fetch to complete under 250ms, took %v (sequential would be ~300ms)", elapsed)
	}
}

// M10-AC-008: Raw/XML format endpoints are unaffected
func TestM10_AC008_RawXMLUnaffected(t *testing.T) {
	xmlBody := `<document><title>Test</title></document>`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.Write([]byte(xmlBody))
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:    "test-raw",
		BaseURL: server.URL,
		Path:    "/doc.xml",
		Format:  "raw",
	}
	config.ApplyDefaults(&ep)

	fetcher := upstream.NewHTTPFetcher()
	fetcher.FetchConcurrency = 3

	result, err := fetcher.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.Data != xmlBody {
		t.Errorf("expected raw body, got %v", result.Data)
	}
}

// ---------------------------------------------------------------------------
// M10: Empty Upstream Results — fetcher-level tests
// Spec: specs/empty-upstream-results.md
// ---------------------------------------------------------------------------

// M10-AC-001: Upstream returns 200 with empty data array → valid CachedResponse
func TestM10_EmptyResults_AC001_FetcherReturnsEmptyData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []interface{}{},
			"meta": map[string]interface{}{
				"results": map[string]interface{}{
					"total": 0,
				},
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:            "fda-ndc",
		BaseURL:         server.URL,
		Path:            "/drug/ndc.json",
		Format:          "json",
		DataKey:         "results",
		TotalKey:        "meta.results.total",
		PaginationStyle: "offset",
		Pagesize:        100,
	}
	config.ApplyDefaults(&ep)

	result, err := upstream.Fetch(ep, nil)
	if err != nil {
		t.Fatalf("expected no error for empty results, got %v", err)
	}

	// Data should be an empty slice, not nil
	allData, ok := result.Data.([]interface{})
	if !ok {
		t.Fatalf("expected Data to be []interface{}, got %T", result.Data)
	}
	if allData == nil {
		t.Error("expected Data to be empty slice (not nil)")
	}
	if len(allData) != 0 {
		t.Errorf("expected 0 items, got %d", len(allData))
	}
}

// M10: Offset pagination also parallelized after first page
func TestM10_OffsetPaginationParallelized(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(50 * time.Millisecond)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"results": []interface{}{map[string]interface{}{"id": 1}},
			"meta": map[string]interface{}{
				"results": map[string]interface{}{
					"total": 150,
				},
			},
		})
	}))
	defer server.Close()

	ep := config.Endpoint{
		Slug:            "test-offset-parallel",
		BaseURL:         server.URL,
		Path:            "/api",
		Format:          "json",
		PaginationStyle: "offset",
		DataKey:         "results",
		TotalKey:        "meta.results.total",
		Pagination:      "all",
		Pagesize:        50,
	}
	config.ApplyDefaults(&ep)

	fetcher := upstream.NewHTTPFetcher()
	fetcher.FetchConcurrency = 3

	start := time.Now()
	result, err := fetcher.Fetch(ep, nil)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if result.PageCount != 3 {
		t.Errorf("expected 3 pages, got %d", result.PageCount)
	}
	// Sequential would be ~150ms (3 * 50ms)
	// Parallel: 1 sequential + 1 batch of 2 = ~100ms
	// Allow margin: should be under 140ms
	if elapsed > 140*time.Millisecond {
		t.Errorf("expected parallel offset fetch under 140ms, took %v", elapsed)
	}
}
