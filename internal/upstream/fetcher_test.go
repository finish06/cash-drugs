package upstream_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
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
