package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/finish06/cash-drugs/internal/handler"
)

// AC-001: GET /openapi.json returns valid OpenAPI JSON
func TestAC001_OpenAPISpecReturnsJSON(t *testing.T) {
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	handler.ServeOpenAPISpec(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("expected application/json, got %s", ct)
	}

	// Should be valid JSON
	var spec map[string]interface{}
	if err := json.NewDecoder(w.Body).Decode(&spec); err != nil {
		t.Fatalf("response is not valid JSON: %v", err)
	}

	// Should contain swagger version
	if _, ok := spec["swagger"]; !ok {
		if _, ok := spec["openapi"]; !ok {
			t.Error("expected 'swagger' or 'openapi' field in spec")
		}
	}
}

// AC-007: Spec includes service metadata
func TestAC007_SpecIncludesMetadata(t *testing.T) {
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	handler.ServeOpenAPISpec(w, req)

	body := w.Body.String()
	if !strings.Contains(body, "drugs API") {
		t.Error("expected spec to contain 'drugs API' title")
	}
}

// AC-004: Cache endpoint is documented in spec
func TestAC004_CacheEndpointInSpec(t *testing.T) {
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	handler.ServeOpenAPISpec(w, req)

	var spec map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&spec)

	paths, ok := spec["paths"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'paths' in spec")
	}

	if _, ok := paths["/api/cache/{slug}"]; !ok {
		t.Error("expected /api/cache/{slug} in spec paths")
	}
}

// AC-005: Health endpoint is documented in spec
func TestAC005_HealthEndpointInSpec(t *testing.T) {
	req := httptest.NewRequest("GET", "/openapi.json", nil)
	w := httptest.NewRecorder()
	handler.ServeOpenAPISpec(w, req)

	var spec map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&spec)

	paths, ok := spec["paths"].(map[string]interface{})
	if !ok {
		t.Fatal("expected 'paths' in spec")
	}

	if _, ok := paths["/health"]; !ok {
		t.Error("expected /health in spec paths")
	}
}
