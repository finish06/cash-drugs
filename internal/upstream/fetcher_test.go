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
