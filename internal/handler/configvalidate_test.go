package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/finish06/cash-drugs/internal/model"
)

func TestConfigValidateHandler_ValidConfig(t *testing.T) {
	h := NewConfigValidateHandler()

	yamlStr := "endpoints:\n  - slug: test\n    base_url: https://example.com\n    path: /api\n    format: json\n"
	body, _ := json.Marshal(configValidateRequest{YAML: yamlStr})
	req := httptest.NewRequest(http.MethodPost, "/api/config/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp configValidateResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Valid {
		t.Fatal("expected valid=true")
	}
	if resp.EndpointCount != 1 {
		t.Fatalf("expected endpoint_count=1, got %d", resp.EndpointCount)
	}
	if resp.Endpoints[0].Slug != "test" {
		t.Fatalf("expected slug=test, got %s", resp.Endpoints[0].Slug)
	}
	if resp.Endpoints[0].BaseURL != "https://example.com" {
		t.Fatalf("expected base_url=https://example.com, got %s", resp.Endpoints[0].BaseURL)
	}
	if resp.Endpoints[0].Format != "json" {
		t.Fatalf("expected format=json, got %s", resp.Endpoints[0].Format)
	}
	if resp.Endpoints[0].HasSchedule {
		t.Fatal("expected has_schedule=false")
	}
	if resp.Endpoints[0].HasPagination {
		t.Fatal("expected has_pagination=false")
	}
	if len(resp.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", resp.Warnings)
	}
}

func TestConfigValidateHandler_MissingSlug(t *testing.T) {
	h := NewConfigValidateHandler()

	yamlStr := "endpoints:\n  - base_url: https://example.com\n    path: /api\n    format: json\n"
	body, _ := json.Marshal(configValidateRequest{YAML: yamlStr})
	req := httptest.NewRequest(http.MethodPost, "/api/config/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp configValidateErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false")
	}
	if resp.ErrorCode != model.ErrCodeBadRequest {
		t.Fatalf("expected error_code=%s, got %s", model.ErrCodeBadRequest, resp.ErrorCode)
	}
}

func TestConfigValidateHandler_MissingBaseURL(t *testing.T) {
	h := NewConfigValidateHandler()

	yamlStr := "endpoints:\n  - slug: test\n    path: /api\n    format: json\n"
	body, _ := json.Marshal(configValidateRequest{YAML: yamlStr})
	req := httptest.NewRequest(http.MethodPost, "/api/config/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	var resp configValidateErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false")
	}
}

func TestConfigValidateHandler_MissingFormat(t *testing.T) {
	h := NewConfigValidateHandler()

	yamlStr := "endpoints:\n  - slug: test\n    base_url: https://example.com\n    path: /api\n"
	body, _ := json.Marshal(configValidateRequest{YAML: yamlStr})
	req := httptest.NewRequest(http.MethodPost, "/api/config/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	var resp configValidateErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false")
	}
}

func TestConfigValidateHandler_InvalidFormatValue(t *testing.T) {
	h := NewConfigValidateHandler()

	yamlStr := "endpoints:\n  - slug: test\n    base_url: https://example.com\n    path: /api\n    format: csv\n"
	body, _ := json.Marshal(configValidateRequest{YAML: yamlStr})
	req := httptest.NewRequest(http.MethodPost, "/api/config/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	var resp configValidateErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false")
	}
	if resp.ErrorCode != model.ErrCodeBadRequest {
		t.Fatalf("expected error_code=%s, got %s", model.ErrCodeBadRequest, resp.ErrorCode)
	}
}

func TestConfigValidateHandler_DuplicateSlugs(t *testing.T) {
	h := NewConfigValidateHandler()

	yamlStr := "endpoints:\n  - slug: test\n    base_url: https://example.com\n    path: /api\n    format: json\n  - slug: test\n    base_url: https://other.com\n    path: /v2\n    format: json\n"
	body, _ := json.Marshal(configValidateRequest{YAML: yamlStr})
	req := httptest.NewRequest(http.MethodPost, "/api/config/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	var resp configValidateErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false")
	}
}

func TestConfigValidateHandler_InvalidCronExpression(t *testing.T) {
	h := NewConfigValidateHandler()

	yamlStr := "endpoints:\n  - slug: test\n    base_url: https://example.com\n    path: /api\n    format: json\n    refresh: \"not a cron\"\n"
	body, _ := json.Marshal(configValidateRequest{YAML: yamlStr})
	req := httptest.NewRequest(http.MethodPost, "/api/config/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	var resp configValidateErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false")
	}
}

func TestConfigValidateHandler_InvalidYAML(t *testing.T) {
	h := NewConfigValidateHandler()

	body, _ := json.Marshal(configValidateRequest{YAML: "{{invalid yaml"})
	req := httptest.NewRequest(http.MethodPost, "/api/config/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	var resp configValidateErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false")
	}
}

func TestConfigValidateHandler_EmptyBody(t *testing.T) {
	h := NewConfigValidateHandler()

	body, _ := json.Marshal(configValidateRequest{YAML: ""})
	req := httptest.NewRequest(http.MethodPost, "/api/config/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	var resp configValidateErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false")
	}
}

func TestConfigValidateHandler_MultipleEndpointsWithWarnings(t *testing.T) {
	h := NewConfigValidateHandler()

	yamlStr := `endpoints:
  - slug: scheduled
    base_url: https://example.com
    path: /api
    format: json
    refresh: "*/5 * * * *"
  - slug: bigpage
    base_url: https://other.com
    path: /v2
    format: xml
    pagesize: 2000
    pagination: all
`
	body, _ := json.Marshal(configValidateRequest{YAML: yamlStr})
	req := httptest.NewRequest(http.MethodPost, "/api/config/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp configValidateResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Valid {
		t.Fatal("expected valid=true")
	}
	if resp.EndpointCount != 2 {
		t.Fatalf("expected endpoint_count=2, got %d", resp.EndpointCount)
	}

	// First endpoint should have has_schedule=true
	if !resp.Endpoints[0].HasSchedule {
		t.Fatal("expected first endpoint has_schedule=true")
	}

	// Second endpoint should have has_pagination=true
	if !resp.Endpoints[1].HasPagination {
		t.Fatal("expected second endpoint has_pagination=true")
	}

	// Should have warnings: missing TTL on scheduled, large pagesize
	if len(resp.Warnings) != 2 {
		t.Fatalf("expected 2 warnings, got %d: %v", len(resp.Warnings), resp.Warnings)
	}
}

func TestConfigValidateHandler_InvalidJSON(t *testing.T) {
	h := NewConfigValidateHandler()

	req := httptest.NewRequest(http.MethodPost, "/api/config/validate", bytes.NewBufferString("{not json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	var resp configValidateErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Valid {
		t.Fatal("expected valid=false")
	}
}

func TestConfigValidateHandler_ParamsExtracted(t *testing.T) {
	h := NewConfigValidateHandler()

	yamlStr := "endpoints:\n  - slug: test\n    base_url: https://example.com\n    path: /api/{drug_name}\n    format: json\n"
	body, _ := json.Marshal(configValidateRequest{YAML: yamlStr})
	req := httptest.NewRequest(http.MethodPost, "/api/config/validate", bytes.NewBuffer(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	var resp configValidateResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if !resp.Valid {
		t.Fatal("expected valid=true")
	}
	if len(resp.Endpoints[0].Params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(resp.Endpoints[0].Params))
	}
	if resp.Endpoints[0].Params[0] != "drug_name" {
		t.Fatalf("expected param 'drug_name', got '%s'", resp.Endpoints[0].Params[0])
	}
}
