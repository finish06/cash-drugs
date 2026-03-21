package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/model"
)

// mockFetcher implements upstream.Fetcher for test purposes.
type mockFetcher struct {
	result *model.CachedResponse
	err    error
}

func (m *mockFetcher) Fetch(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.result, nil
}

func TestTestFetchHandler_MissingBaseURL(t *testing.T) {
	h := NewTestFetchHandler(WithTestFetcher(&mockFetcher{}))

	body := `{"path":"/foo","format":"json"}`
	req := httptest.NewRequest(http.MethodPost, "/api/test-fetch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	var resp testFetchErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Success {
		t.Fatal("expected success=false")
	}
	if resp.ErrorCode != model.ErrCodeBadRequest {
		t.Fatalf("expected error_code=%s, got %s", model.ErrCodeBadRequest, resp.ErrorCode)
	}
}

func TestTestFetchHandler_MissingPath(t *testing.T) {
	h := NewTestFetchHandler(WithTestFetcher(&mockFetcher{}))

	body := `{"base_url":"https://example.com","format":"json"}`
	req := httptest.NewRequest(http.MethodPost, "/api/test-fetch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	var resp testFetchErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Success {
		t.Fatal("expected success=false")
	}
}

func TestTestFetchHandler_MissingFormat(t *testing.T) {
	h := NewTestFetchHandler(WithTestFetcher(&mockFetcher{}))

	body := `{"base_url":"https://example.com","path":"/foo"}`
	req := httptest.NewRequest(http.MethodPost, "/api/test-fetch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	var resp testFetchErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Success {
		t.Fatal("expected success=false")
	}
}

func TestTestFetchHandler_InvalidJSON(t *testing.T) {
	h := NewTestFetchHandler(WithTestFetcher(&mockFetcher{}))

	req := httptest.NewRequest(http.MethodPost, "/api/test-fetch", bytes.NewBufferString("{not json"))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rr.Code)
	}

	var resp testFetchErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Success {
		t.Fatal("expected success=false")
	}
	if resp.ErrorCode != model.ErrCodeBadRequest {
		t.Fatalf("expected error_code=%s, got %s", model.ErrCodeBadRequest, resp.ErrorCode)
	}
}

func TestTestFetchHandler_SuccessWithDataPreview(t *testing.T) {
	// Create mock data with more than 5 items
	items := make([]interface{}, 10)
	for i := range items {
		items[i] = map[string]interface{}{"id": float64(i + 1)}
	}

	mf := &mockFetcher{
		result: &model.CachedResponse{
			Slug:        "test-fetch",
			Data:        items,
			ContentType: "application/json",
			HTTPStatus:  200,
			PageCount:   1,
			FetchedAt:   time.Now(),
		},
	}

	h := NewTestFetchHandler(WithTestFetcher(mf))

	body := `{"base_url":"https://api.example.com","path":"/data","format":"json","pagesize":10}`
	req := httptest.NewRequest(http.MethodPost, "/api/test-fetch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp testFetchResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Fatal("expected success=true")
	}
	if resp.StatusCode != 200 {
		t.Fatalf("expected status_code=200, got %d", resp.StatusCode)
	}
	if resp.ContentType != "application/json" {
		t.Fatalf("expected content_type=application/json, got %s", resp.ContentType)
	}
	if resp.TotalResults != 10 {
		t.Fatalf("expected total_results=10, got %d", resp.TotalResults)
	}

	// data_preview should be truncated to 5 items
	preview, ok := resp.DataPreview.([]interface{})
	if !ok {
		t.Fatalf("expected data_preview to be array, got %T", resp.DataPreview)
	}
	if len(preview) != 5 {
		t.Fatalf("expected data_preview to have 5 items, got %d", len(preview))
	}

	// page_count_estimate = ceil(10/10) = 1
	if resp.PageCountEstimate != 1 {
		t.Fatalf("expected page_count_estimate=1, got %d", resp.PageCountEstimate)
	}
}

func TestTestFetchHandler_UpstreamError(t *testing.T) {
	mf := &mockFetcher{
		err: &testUpstreamError{msg: "upstream returned 404"},
	}

	h := NewTestFetchHandler(WithTestFetcher(mf))

	body := `{"base_url":"https://api.example.com","path":"/data","format":"json"}`
	req := httptest.NewRequest(http.MethodPost, "/api/test-fetch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp testFetchErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Success {
		t.Fatal("expected success=false")
	}
	if resp.Error != "upstream returned 404" {
		t.Fatalf("expected error message 'upstream returned 404', got '%s'", resp.Error)
	}
}

// testUpstreamError is a simple error type for testing.
type testUpstreamError struct {
	msg string
}

func (e *testUpstreamError) Error() string { return e.msg }

func TestTestFetchHandler_SmallDataNoTruncation(t *testing.T) {
	items := []interface{}{
		map[string]interface{}{"name": "aspirin"},
		map[string]interface{}{"name": "ibuprofen"},
	}

	mf := &mockFetcher{
		result: &model.CachedResponse{
			Slug:        "test-fetch",
			Data:        items,
			ContentType: "application/json",
			HTTPStatus:  200,
			PageCount:   1,
			FetchedAt:   time.Now(),
		},
	}

	h := NewTestFetchHandler(WithTestFetcher(mf))

	body := `{"base_url":"https://api.example.com","path":"/data","format":"json","pagesize":10}`
	req := httptest.NewRequest(http.MethodPost, "/api/test-fetch", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	var resp testFetchResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Fatal("expected success=true")
	}
	if resp.TotalResults != 2 {
		t.Fatalf("expected total_results=2, got %d", resp.TotalResults)
	}

	preview, ok := resp.DataPreview.([]interface{})
	if !ok {
		t.Fatalf("expected data_preview to be array, got %T", resp.DataPreview)
	}
	if len(preview) != 2 {
		t.Fatalf("expected 2 items in preview, got %d", len(preview))
	}
}
