package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/finish06/drugs/internal/config"
	"github.com/finish06/drugs/internal/handler"
	"github.com/finish06/drugs/internal/model"
)

// AC-013: Consumer requests for endpoints not defined in config return 404
func TestAC013_UnknownEndpoint404(t *testing.T) {
	h := handler.NewCacheHandler(
		[]config.Endpoint{},
		&mockCacheRepo{},
		&mockFetcher{},
	)

	req := httptest.NewRequest("GET", "/api/cache/nonexistent", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var errResp model.ErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Error != "endpoint not configured" {
		t.Errorf("expected 'endpoint not configured' error, got '%s'", errResp.Error)
	}
	if errResp.Slug != "nonexistent" {
		t.Errorf("expected slug 'nonexistent', got '%s'", errResp.Slug)
	}
}

// AC-014: Consumer-facing URL pattern mirrors configured slug
func TestAC014_URLPatternMatchesSlug(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: &model.CachedResponse{
			Slug:      "drugnames",
			Data:      map[string]interface{}{"items": []interface{}{}},
			FetchedAt: time.Now(),
		}},
		&mockFetcher{},
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// AC-008: Return cached response to consumer if cache exists
func TestAC008_ReturnCachedResponse(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	cached := &model.CachedResponse{
		Slug:       "drugnames",
		CacheKey:   "drugnames",
		Data:       map[string]interface{}{"items": []interface{}{"drug1"}},
		FetchedAt:  time.Now(),
		SourceURL:  "http://example.com/v2/drugnames",
		PageCount:  1,
		HTTPStatus: 200,
	}

	fetcher := &mockFetcher{}
	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: cached},
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if fetcher.fetchCalled {
		t.Error("expected no upstream fetch when cache exists")
	}

	var resp model.APIResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if resp.Meta.Stale {
		t.Error("expected stale=false for fresh cache")
	}
}

// AC-004: No cache triggers upstream fetch
func TestAC004_NoCacheTriggersFetch(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:       "drugnames",
			CacheKey:   "drugnames",
			Data:       map[string]interface{}{"items": []interface{}{"drug1"}},
			FetchedAt:  time.Now(),
			SourceURL:  "http://example.com/v2/drugnames",
			PageCount:  1,
			HTTPStatus: 200,
		},
	}
	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !fetcher.fetchCalled {
		t.Error("expected upstream fetch when no cache exists")
	}
}

// AC-009: Upstream failure returns stale cache
func TestAC009_UpstreamFailureReturnsStaleCacheViaHandler(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	staleCache := &model.CachedResponse{
		Slug:       "drugnames",
		CacheKey:   "drugnames",
		Data:       map[string]interface{}{"items": []interface{}{"stale-drug"}},
		FetchedAt:  time.Now().Add(-24 * time.Hour),
		SourceURL:  "http://example.com/v2/drugnames",
		PageCount:  1,
		HTTPStatus: 200,
	}

	// Cache repo returns nil on first call (no cache), stale on second (after fetch fails)
	repo := &mockCacheRepo{cached: staleCache}
	fetcher := &mockFetcher{err: fmt.Errorf("upstream unavailable")}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames?_force=true", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// Should return 200 with stale data
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with stale cache, got %d", w.Code)
	}

	var resp model.APIResponse
	json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Meta.Stale {
		t.Error("expected stale=true when serving stale cache")
	}
}

// AC-010: Upstream fails and no cache returns 502
func TestAC010_UpstreamFailNoCacheReturns502(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		&mockFetcher{err: fmt.Errorf("upstream unavailable")},
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}

	var errResp model.ErrorResponse
	json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Error != "upstream unavailable" {
		t.Errorf("expected 'upstream unavailable' error, got '%s'", errResp.Error)
	}
}

// AC-007: Response stored with metadata
func TestAC007_ResponseStoredWithMetadata(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	repo := &mockCacheRepo{}
	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:       "drugnames",
			CacheKey:   "drugnames",
			Data:       map[string]interface{}{"items": []interface{}{"drug1"}},
			FetchedAt:  time.Now(),
			SourceURL:  "http://example.com/v2/drugnames",
			PageCount:  1,
			HTTPStatus: 200,
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !repo.upsertCalled {
		t.Error("expected cache upsert after upstream fetch")
	}
	if repo.lastUpserted == nil {
		t.Fatal("expected upserted document")
	}
	if repo.lastUpserted.Slug != "drugnames" {
		t.Errorf("expected slug 'drugnames', got '%s'", repo.lastUpserted.Slug)
	}
	if repo.lastUpserted.FetchedAt.IsZero() {
		t.Error("expected fetched_at timestamp")
	}
	if repo.lastUpserted.SourceURL == "" {
		t.Error("expected source_url")
	}
	if repo.lastUpserted.PageCount < 1 {
		t.Error("expected page_count >= 1")
	}
}

// AC-003: Path parameter from query string
func TestAC003_PathParamFromQueryString(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "spl-detail",
		BaseURL: "http://example.com",
		Path:    "/v2/spls/{SETID}",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:      "spl-detail",
			CacheKey:  "spl-detail:SETID=abc-123",
			Data:      map[string]interface{}{"spl": "data"},
			FetchedAt: time.Now(),
			SourceURL: "http://example.com/v2/spls/abc-123",
			PageCount: 1,
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/spl-detail?SETID=abc-123", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !fetcher.fetchCalled {
		t.Error("expected fetch to be called")
	}
	if fetcher.lastParams["SETID"] != "abc-123" {
		t.Errorf("expected SETID=abc-123 passed to fetcher, got %v", fetcher.lastParams)
	}
}

// --- Mock implementations ---

type mockCacheRepo struct {
	cached       *model.CachedResponse
	upsertCalled bool
	lastUpserted *model.CachedResponse
}

func (m *mockCacheRepo) Get(cacheKey string) (*model.CachedResponse, error) {
	return m.cached, nil
}

func (m *mockCacheRepo) Upsert(resp *model.CachedResponse) error {
	m.upsertCalled = true
	m.lastUpserted = resp
	return nil
}

type mockFetcher struct {
	result      *model.CachedResponse
	err         error
	fetchCalled bool
	lastParams  map[string]string
}

func (m *mockFetcher) Fetch(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error) {
	m.fetchCalled = true
	m.lastParams = params
	return m.result, m.err
}
