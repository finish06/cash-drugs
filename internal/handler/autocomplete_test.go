package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/handler"
	"github.com/finish06/cash-drugs/internal/model"
)

func TestAutocompleteHandler_EmptyQuery(t *testing.T) {
	h := handler.NewAutocompleteHandler(&mockSearchRepo{}, []string{"drugnames"})

	req := httptest.NewRequest("GET", "/api/autocomplete", nil)
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

func TestAutocompleteHandler_ValidPrefixWithMatches(t *testing.T) {
	repo := &mockSearchRepo{
		data: map[string]*model.CachedResponse{
			"drugnames": {
				Slug: "drugnames",
				Data: []interface{}{
					map[string]interface{}{"drug_name": "Metformin"},
					map[string]interface{}{"drug_name": "Metoprolol"},
					map[string]interface{}{"drug_name": "Aspirin"},
				},
				FetchedAt: time.Now(),
			},
		},
	}

	h := handler.NewAutocompleteHandler(repo, []string{"drugnames"})

	req := httptest.NewRequest("GET", "/api/autocomplete?q=met", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp handler.AutocompleteResponse
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if resp.Query != "met" {
		t.Errorf("expected query='met', got '%s'", resp.Query)
	}
	if len(resp.Suggestions) != 2 {
		t.Fatalf("expected 2 suggestions, got %d: %v", len(resp.Suggestions), resp.Suggestions)
	}
	// Sorted alphabetically
	if resp.Suggestions[0] != "Metformin" {
		t.Errorf("expected first suggestion='Metformin', got '%s'", resp.Suggestions[0])
	}
	if resp.Suggestions[1] != "Metoprolol" {
		t.Errorf("expected second suggestion='Metoprolol', got '%s'", resp.Suggestions[1])
	}
}

func TestAutocompleteHandler_NoMatches(t *testing.T) {
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

	h := handler.NewAutocompleteHandler(repo, []string{"drugnames"})

	req := httptest.NewRequest("GET", "/api/autocomplete?q=zzz", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp handler.AutocompleteResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Suggestions) != 0 {
		t.Errorf("expected 0 suggestions, got %d", len(resp.Suggestions))
	}
}

func TestAutocompleteHandler_LimitRespected(t *testing.T) {
	items := make([]interface{}, 20)
	for i := range items {
		items[i] = map[string]interface{}{"drug_name": "Metformin variant " + string(rune('A'+i))}
	}

	repo := &mockSearchRepo{
		data: map[string]*model.CachedResponse{
			"drugnames": {Slug: "drugnames", Data: items, FetchedAt: time.Now()},
		},
	}

	h := handler.NewAutocompleteHandler(repo, []string{"drugnames"})

	req := httptest.NewRequest("GET", "/api/autocomplete?q=met&limit=5", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp handler.AutocompleteResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Suggestions) != 5 {
		t.Errorf("expected 5 suggestions (limit), got %d", len(resp.Suggestions))
	}
}

func TestAutocompleteHandler_DeduplicatesResults(t *testing.T) {
	repo := &mockSearchRepo{
		data: map[string]*model.CachedResponse{
			"drugnames": {
				Slug: "drugnames",
				Data: []interface{}{
					map[string]interface{}{"drug_name": "Metformin"},
					map[string]interface{}{"drug_name": "Metformin"}, // duplicate
					map[string]interface{}{"drug_name": "Metformin"}, // duplicate
				},
				FetchedAt: time.Now(),
			},
		},
	}

	h := handler.NewAutocompleteHandler(repo, []string{"drugnames"})

	req := httptest.NewRequest("GET", "/api/autocomplete?q=met", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp handler.AutocompleteResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Suggestions) != 1 {
		t.Errorf("expected 1 suggestion (deduplicated), got %d: %v", len(resp.Suggestions), resp.Suggestions)
	}
}

func TestAutocompleteHandler_CaseInsensitivePrefix(t *testing.T) {
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

	h := handler.NewAutocompleteHandler(repo, []string{"drugnames"})

	req := httptest.NewRequest("GET", "/api/autocomplete?q=metf", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp handler.AutocompleteResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Suggestions) != 1 {
		t.Errorf("expected 1 suggestion (case insensitive), got %d", len(resp.Suggestions))
	}
}

func TestAutocompleteHandler_MultipleSlugs(t *testing.T) {
	repo := &mockSearchRepo{
		data: map[string]*model.CachedResponse{
			"drugnames": {
				Slug: "drugnames",
				Data: []interface{}{
					map[string]interface{}{"drug_name": "Aspirin"},
				},
				FetchedAt: time.Now(),
			},
			"fda-ndc": {
				Slug: "fda-ndc",
				Data: []interface{}{
					map[string]interface{}{"brand_name": "Aspirin EC"},
				},
				FetchedAt: time.Now(),
			},
		},
	}

	h := handler.NewAutocompleteHandler(repo, []string{"drugnames", "fda-ndc"})

	req := httptest.NewRequest("GET", "/api/autocomplete?q=asp", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp handler.AutocompleteResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Suggestions) != 2 {
		t.Errorf("expected 2 suggestions from 2 slugs, got %d: %v", len(resp.Suggestions), resp.Suggestions)
	}
}

func TestAutocompleteHandler_MaxLimitCapped(t *testing.T) {
	items := make([]interface{}, 60)
	for i := range items {
		items[i] = map[string]interface{}{"drug_name": "Metformin " + string(rune('A'+i%26)) + string(rune('A'+i/26))}
	}

	repo := &mockSearchRepo{
		data: map[string]*model.CachedResponse{
			"drugnames": {Slug: "drugnames", Data: items, FetchedAt: time.Now()},
		},
	}

	h := handler.NewAutocompleteHandler(repo, []string{"drugnames"})

	req := httptest.NewRequest("GET", "/api/autocomplete?q=met&limit=999", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp handler.AutocompleteResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Suggestions) > 50 {
		t.Errorf("expected suggestions capped at 50, got %d", len(resp.Suggestions))
	}
}
