package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/handler"
	"github.com/finish06/cash-drugs/internal/model"
)

// mockSearchRepo implements cache.Repository for search handler tests.
type mockSearchRepo struct {
	data map[string]*model.CachedResponse
}

func (m *mockSearchRepo) Get(cacheKey string) (*model.CachedResponse, error) {
	if resp, ok := m.data[cacheKey]; ok {
		return resp, nil
	}
	return nil, nil
}

func (m *mockSearchRepo) Upsert(resp *model.CachedResponse) error { return nil }

func (m *mockSearchRepo) FetchedAt(cacheKey string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func TestSearchHandler_EmptyQuery(t *testing.T) {
	h := handler.NewSearchHandler(nil, &mockSearchRepo{})

	req := httptest.NewRequest("GET", "/api/search", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp model.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}
	if errResp.ErrorCode != model.ErrCodeBadRequest {
		t.Errorf("expected error code %s, got %s", model.ErrCodeBadRequest, errResp.ErrorCode)
	}
}

func TestSearchHandler_QueryTooShort(t *testing.T) {
	h := handler.NewSearchHandler(nil, &mockSearchRepo{})

	req := httptest.NewRequest("GET", "/api/search?q=a", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestSearchHandler_ValidQueryWithMatches(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "drugnames"},
	}
	repo := &mockSearchRepo{
		data: map[string]*model.CachedResponse{
			"drugnames": {
				Slug: "drugnames",
				Data: []interface{}{
					map[string]interface{}{"drug_name": "Metformin", "type": "generic"},
					map[string]interface{}{"drug_name": "Aspirin", "type": "otc"},
					map[string]interface{}{"drug_name": "Metoprolol", "type": "generic"},
				},
				FetchedAt: time.Now(),
			},
		},
	}

	h := handler.NewSearchHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/search?q=metf", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp handler.SearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if resp.Query != "metf" {
		t.Errorf("expected query='metf', got '%s'", resp.Query)
	}
	if resp.TotalMatches != 1 {
		t.Errorf("expected total_matches=1, got %d", resp.TotalMatches)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result group, got %d", len(resp.Results))
	}
	if resp.Results[0].Slug != "drugnames" {
		t.Errorf("expected slug='drugnames', got '%s'", resp.Results[0].Slug)
	}
	if resp.Results[0].Matches != 1 {
		t.Errorf("expected matches=1, got %d", resp.Results[0].Matches)
	}
}

func TestSearchHandler_NoMatches(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "drugnames"},
	}
	repo := &mockSearchRepo{
		data: map[string]*model.CachedResponse{
			"drugnames": {
				Slug: "drugnames",
				Data: []interface{}{
					map[string]interface{}{"drug_name": "Aspirin"},
				},
				FetchedAt: time.Now(),
			},
		},
	}

	h := handler.NewSearchHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/search?q=zzzznotfound", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp handler.SearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if resp.TotalMatches != 0 {
		t.Errorf("expected total_matches=0, got %d", resp.TotalMatches)
	}
	if len(resp.Results) != 0 {
		t.Errorf("expected 0 result groups, got %d", len(resp.Results))
	}
}

func TestSearchHandler_LimitRespected(t *testing.T) {
	items := make([]interface{}, 100)
	for i := range items {
		items[i] = map[string]interface{}{"drug_name": "Metformin variant"}
	}

	endpoints := []config.Endpoint{
		{Slug: "drugnames"},
	}
	repo := &mockSearchRepo{
		data: map[string]*model.CachedResponse{
			"drugnames": {
				Slug:      "drugnames",
				Data:      items,
				FetchedAt: time.Now(),
			},
		},
	}

	h := handler.NewSearchHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/search?q=metf&limit=5", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp handler.SearchResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if resp.Results[0].Matches != 5 {
		t.Errorf("expected matches=5 (limit), got %d", resp.Results[0].Matches)
	}
}

func TestSearchHandler_SkipsParameterizedEndpoints(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "drugnames"}, // no params, should be searched
		{Slug: "spl-detail", Path: "/v2/spls.json", QueryParams: map[string]string{"setid": "{SETID}"}}, // has params, should be skipped
	}
	repo := &mockSearchRepo{
		data: map[string]*model.CachedResponse{
			"drugnames": {
				Slug: "drugnames",
				Data: []interface{}{
					map[string]interface{}{"drug_name": "Aspirin"},
				},
				FetchedAt: time.Now(),
			},
			"spl-detail": {
				Slug: "spl-detail",
				Data: []interface{}{
					map[string]interface{}{"title": "Aspirin Label"},
				},
				FetchedAt: time.Now(),
			},
		},
	}

	h := handler.NewSearchHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/search?q=aspirin", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp handler.SearchResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	// Should only have results from drugnames, not spl-detail
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result group (drugnames only), got %d", len(resp.Results))
	}
	if resp.Results[0].Slug != "drugnames" {
		t.Errorf("expected slug='drugnames', got '%s'", resp.Results[0].Slug)
	}
}

func TestSearchHandler_CaseInsensitive(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "drugnames"},
	}
	repo := &mockSearchRepo{
		data: map[string]*model.CachedResponse{
			"drugnames": {
				Slug: "drugnames",
				Data: []interface{}{
					map[string]interface{}{"drug_name": "METFORMIN HCL"},
				},
				FetchedAt: time.Now(),
			},
		},
	}

	h := handler.NewSearchHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/search?q=metformin", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp handler.SearchResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp.TotalMatches != 1 {
		t.Errorf("expected case-insensitive match, got total_matches=%d", resp.TotalMatches)
	}
}

func TestSearchHandler_DurationReturned(t *testing.T) {
	h := handler.NewSearchHandler([]config.Endpoint{{Slug: "drugnames"}}, &mockSearchRepo{data: map[string]*model.CachedResponse{}})

	req := httptest.NewRequest("GET", "/api/search?q=test", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp handler.SearchResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp.DurationMS < 0 {
		t.Errorf("expected non-negative duration_ms, got %f", resp.DurationMS)
	}
}

func TestSearchHandler_MaxLimitCapped(t *testing.T) {
	// Verify limit > 200 is capped at 200
	items := make([]interface{}, 250)
	for i := range items {
		items[i] = map[string]interface{}{"drug_name": "Metformin variant"}
	}

	endpoints := []config.Endpoint{{Slug: "drugnames"}}
	repo := &mockSearchRepo{
		data: map[string]*model.CachedResponse{
			"drugnames": {Slug: "drugnames", Data: items, FetchedAt: time.Now()},
		},
	}

	h := handler.NewSearchHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/search?q=metf&limit=999", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp handler.SearchResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp.Results[0].Matches > 200 {
		t.Errorf("expected matches capped at 200, got %d", resp.Results[0].Matches)
	}
}
