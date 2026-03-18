package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/handler"
)

// M3: Endpoint discovery returns all configured slugs
func TestEndpoints_ReturnsAllSlugs(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "drugnames", BaseURL: "http://example.com", Path: "/v2/drugnames", Format: "json", Pagination: "all", Refresh: "0 */6 * * *"},
		{Slug: "spl-detail", BaseURL: "http://example.com", Path: "/v2/spls.json", Format: "json", QueryParams: map[string]string{"setid": "{SETID}"}},
		{Slug: "spl-xml", BaseURL: "http://example.com", Path: "/v2/spls/{SETID}.xml", Format: "raw"},
	}
	for i := range endpoints {
		config.ApplyDefaults(&endpoints[i])
	}

	h := handler.NewEndpointsHandler(endpoints)

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	var result []handler.EndpointInfo
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if len(result) != 3 {
		t.Fatalf("expected 3 endpoints, got %d", len(result))
	}

	// Check first endpoint
	if result[0].Slug != "drugnames" {
		t.Errorf("expected slug 'drugnames', got '%s'", result[0].Slug)
	}
	if !result[0].Pagination {
		t.Error("expected pagination=true for drugnames")
	}
	if !result[0].Scheduled {
		t.Error("expected scheduled=true for drugnames")
	}

	// Check parameterized endpoint
	if len(result[1].Params) == 0 {
		t.Error("expected params for spl-detail")
	}
	if result[1].Scheduled {
		t.Error("expected scheduled=false for spl-detail")
	}
}

// M3: Empty endpoints returns empty array
func TestEndpoints_EmptyConfig(t *testing.T) {
	h := handler.NewEndpointsHandler([]config.Endpoint{})

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result []handler.EndpointInfo
	_ = json.NewDecoder(w.Body).Decode(&result)
	if len(result) != 0 {
		t.Errorf("expected empty array, got %d items", len(result))
	}
}
