package upstream_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/finish06/drugs/internal/config"
	"github.com/finish06/drugs/internal/upstream"
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
	var requests []url.Values
	totalItems := 25

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Query())

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

	expectedSkips := []string{"0", "10", "20"}
	for i, expected := range expectedSkips {
		got := requests[i].Get("skip")
		if got != expected {
			t.Errorf("request %d: expected skip=%s, got skip=%s", i, expected, got)
		}
		limit := requests[i].Get("limit")
		if limit != "10" {
			t.Errorf("request %d: expected limit=10, got limit=%s", i, limit)
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
	var requests []url.Values
	pageCount := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.URL.Query())
		pageCount++
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
	var receivedSkips []string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedSkips = append(receivedSkips, r.URL.Query().Get("skip"))

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

	expectedSkips := []string{"0", "25", "50"}
	for i, expected := range expectedSkips {
		if receivedSkips[i] != expected {
			t.Errorf("request %d: expected skip=%s, got skip=%s", i, expected, receivedSkips[i])
		}
	}
}
