package handler_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/handler"
	"github.com/finish06/cash-drugs/internal/model"
)

// mockRepo implements cache.Repository for testing.
type mockRepo struct {
	fetchedAtMap map[string]time.Time
	getMap       map[string]*model.CachedResponse
	err          error
}

func (m *mockRepo) Get(cacheKey string) (*model.CachedResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	resp, ok := m.getMap[cacheKey]
	if !ok {
		return nil, nil
	}
	return resp, nil
}

func (m *mockRepo) Upsert(resp *model.CachedResponse) error {
	return m.err
}

func (m *mockRepo) FetchedAt(cacheKey string) (time.Time, bool, error) {
	if m.err != nil {
		return time.Time{}, false, m.err
	}
	t, ok := m.fetchedAtMap[cacheKey]
	return t, ok, nil
}

// --- Backward compatibility tests (AC-007) ---

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

	repo := &mockRepo{fetchedAtMap: map[string]time.Time{}}
	h := handler.NewEndpointsHandler(endpoints, repo)

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

	// Check first endpoint — backward compat fields
	if result[0].Slug != "drugnames" {
		t.Errorf("expected slug 'drugnames', got '%s'", result[0].Slug)
	}
	if !result[0].Pagination {
		t.Error("expected pagination=true for drugnames")
	}
	if !result[0].Scheduled {
		t.Error("expected scheduled=true for drugnames")
	}

	// Check parameterized endpoint has params
	if len(result[1].Params) == 0 {
		t.Error("expected params for spl-detail")
	}
	if result[1].Scheduled {
		t.Error("expected scheduled=false for spl-detail")
	}
}

// M3: Empty endpoints returns empty array
func TestEndpoints_EmptyConfig(t *testing.T) {
	repo := &mockRepo{fetchedAtMap: map[string]time.Time{}}
	h := handler.NewEndpointsHandler([]config.Endpoint{}, repo)

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

// --- ParamInfo tests (AC-001, AC-002, AC-003) ---

func TestEndpoints_PathParamsMarkedRequired(t *testing.T) {
	endpoints := []config.Endpoint{
		{
			Slug:    "spl-xml",
			BaseURL: "http://example.com",
			Path:    "/REST/document/{SETID}/details.json",
			Format:  "json",
		},
	}
	for i := range endpoints {
		config.ApplyDefaults(&endpoints[i])
	}

	repo := &mockRepo{fetchedAtMap: map[string]time.Time{}}
	h := handler.NewEndpointsHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var result []handler.EndpointInfo
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	if len(result) != 1 {
		t.Fatalf("expected 1 endpoint, got %d", len(result))
	}

	params := result[0].Params
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	if params[0].Name != "SETID" {
		t.Errorf("expected param name 'SETID', got '%s'", params[0].Name)
	}
	if params[0].Type != "path" {
		t.Errorf("expected param type 'path', got '%s'", params[0].Type)
	}
	if !params[0].Required {
		t.Error("expected path param to be required")
	}
}

func TestEndpoints_SearchParamsMarkedOptional(t *testing.T) {
	endpoints := []config.Endpoint{
		{
			Slug:         "fda-ndc",
			BaseURL:      "https://api.fda.gov",
			Path:         "/drug/ndc.json",
			Format:       "json",
			SearchParams: []string{"BRAND_NAME", "GENERIC_NAME", "NDC", "PHARM_CLASS"},
		},
	}
	for i := range endpoints {
		config.ApplyDefaults(&endpoints[i])
	}

	repo := &mockRepo{fetchedAtMap: map[string]time.Time{}}
	h := handler.NewEndpointsHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var result []handler.EndpointInfo
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	params := result[0].Params
	if len(params) != 4 {
		t.Fatalf("expected 4 search params, got %d", len(params))
	}

	for _, p := range params {
		if p.Required {
			t.Errorf("expected search param '%s' to not be required", p.Name)
		}
		if p.Type != "search" {
			t.Errorf("expected param '%s' type 'search', got '%s'", p.Name, p.Type)
		}
	}
}

func TestEndpoints_QueryParamsFromTemplate(t *testing.T) {
	endpoints := []config.Endpoint{
		{
			Slug:        "spl-detail",
			BaseURL:     "http://example.com",
			Path:        "/v2/spls.json",
			Format:      "json",
			QueryParams: map[string]string{"search": "{SETID}"},
		},
	}
	for i := range endpoints {
		config.ApplyDefaults(&endpoints[i])
	}

	repo := &mockRepo{fetchedAtMap: map[string]time.Time{}}
	h := handler.NewEndpointsHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var result []handler.EndpointInfo
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	params := result[0].Params
	if len(params) != 1 {
		t.Fatalf("expected 1 param, got %d", len(params))
	}
	if params[0].Name != "SETID" {
		t.Errorf("expected param name 'SETID', got '%s'", params[0].Name)
	}
	if params[0].Type != "query" {
		t.Errorf("expected type 'query', got '%s'", params[0].Type)
	}
	if params[0].Required {
		t.Error("expected query param to not be required")
	}
}

func TestEndpoints_MixedPathAndSearchParams(t *testing.T) {
	endpoints := []config.Endpoint{
		{
			Slug:         "mixed",
			BaseURL:      "http://example.com",
			Path:         "/api/{ID}/data",
			Format:       "json",
			SearchParams: []string{"FILTER"},
		},
	}
	for i := range endpoints {
		config.ApplyDefaults(&endpoints[i])
	}

	repo := &mockRepo{fetchedAtMap: map[string]time.Time{}}
	h := handler.NewEndpointsHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var result []handler.EndpointInfo
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("decode error: %v", err)
	}

	params := result[0].Params
	if len(params) != 2 {
		t.Fatalf("expected 2 params, got %d", len(params))
	}

	// Path param first
	if params[0].Name != "ID" || params[0].Type != "path" || !params[0].Required {
		t.Errorf("unexpected path param: %+v", params[0])
	}
	// Search param second
	if params[1].Name != "FILTER" || params[1].Type != "search" || params[1].Required {
		t.Errorf("unexpected search param: %+v", params[1])
	}
}

// --- Example URL tests (AC-004) ---

func TestEndpoints_ExampleURLNoParams(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "drugnames", BaseURL: "http://example.com", Path: "/v2/drugnames", Format: "json"},
	}
	for i := range endpoints {
		config.ApplyDefaults(&endpoints[i])
	}

	repo := &mockRepo{fetchedAtMap: map[string]time.Time{}}
	h := handler.NewEndpointsHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var result []handler.EndpointInfo
	_ = json.NewDecoder(w.Body).Decode(&result)

	expected := "/api/cache/drugnames"
	if result[0].ExampleURL != expected {
		t.Errorf("expected example_url '%s', got '%s'", expected, result[0].ExampleURL)
	}
}

func TestEndpoints_ExampleURLWithParams(t *testing.T) {
	endpoints := []config.Endpoint{
		{
			Slug:         "fda-ndc",
			BaseURL:      "https://api.fda.gov",
			Path:         "/drug/ndc.json",
			Format:       "json",
			SearchParams: []string{"BRAND_NAME"},
		},
	}
	for i := range endpoints {
		config.ApplyDefaults(&endpoints[i])
	}

	repo := &mockRepo{fetchedAtMap: map[string]time.Time{}}
	h := handler.NewEndpointsHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var result []handler.EndpointInfo
	_ = json.NewDecoder(w.Body).Decode(&result)

	expected := "/api/cache/fda-ndc?BRAND_NAME=example"
	if result[0].ExampleURL != expected {
		t.Errorf("expected example_url '%s', got '%s'", expected, result[0].ExampleURL)
	}
}

// --- Cache status tests (AC-006) ---

func TestEndpoints_CacheStatusCached(t *testing.T) {
	fetchTime := time.Now().Add(-2 * time.Hour)
	endpoints := []config.Endpoint{
		{
			Slug:    "fda-ndc",
			BaseURL: "https://api.fda.gov",
			Path:    "/drug/ndc.json",
			Format:  "json",
			TTL:     "24h",
		},
	}
	for i := range endpoints {
		config.ApplyDefaults(&endpoints[i])
	}
	// Manually set TTLDuration since ApplyDefaults doesn't parse TTL
	endpoints[0].TTLDuration = 24 * time.Hour

	repo := &mockRepo{
		fetchedAtMap: map[string]time.Time{
			"fda-ndc": fetchTime,
		},
	}
	h := handler.NewEndpointsHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var result []handler.EndpointInfo
	_ = json.NewDecoder(w.Body).Decode(&result)

	cs := result[0].CacheStatus
	if cs == nil {
		t.Fatal("expected cache_status to be present")
	}
	if !cs.Cached {
		t.Error("expected cached=true")
	}
	if cs.IsStale {
		t.Error("expected is_stale=false (2h old with 24h TTL)")
	}
	if cs.LastRefreshed == "" {
		t.Error("expected last_refreshed to be set")
	}
}

func TestEndpoints_CacheStatusNotCached(t *testing.T) {
	endpoints := []config.Endpoint{
		{
			Slug:    "rxnorm",
			BaseURL: "http://example.com",
			Path:    "/drugs",
			Format:  "json",
			TTL:     "24h",
		},
	}
	for i := range endpoints {
		config.ApplyDefaults(&endpoints[i])
	}
	endpoints[0].TTLDuration = 24 * time.Hour

	repo := &mockRepo{fetchedAtMap: map[string]time.Time{}}
	h := handler.NewEndpointsHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var result []handler.EndpointInfo
	_ = json.NewDecoder(w.Body).Decode(&result)

	cs := result[0].CacheStatus
	if cs == nil {
		t.Fatal("expected cache_status to be present")
	}
	if cs.Cached {
		t.Error("expected cached=false for never-fetched endpoint")
	}
	if !cs.IsStale {
		t.Error("expected is_stale=true for never-fetched endpoint")
	}
	if cs.LastRefreshed != "" {
		t.Errorf("expected empty last_refreshed, got '%s'", cs.LastRefreshed)
	}
}

func TestEndpoints_CacheStatusStale(t *testing.T) {
	fetchTime := time.Now().Add(-48 * time.Hour) // 48h ago
	endpoints := []config.Endpoint{
		{
			Slug:    "stale-ep",
			BaseURL: "http://example.com",
			Path:    "/data",
			Format:  "json",
			TTL:     "24h",
		},
	}
	for i := range endpoints {
		config.ApplyDefaults(&endpoints[i])
	}
	endpoints[0].TTLDuration = 24 * time.Hour

	repo := &mockRepo{
		fetchedAtMap: map[string]time.Time{
			"stale-ep": fetchTime,
		},
	}
	h := handler.NewEndpointsHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var result []handler.EndpointInfo
	_ = json.NewDecoder(w.Body).Decode(&result)

	cs := result[0].CacheStatus
	if cs == nil {
		t.Fatal("expected cache_status to be present")
	}
	if !cs.Cached {
		t.Error("expected cached=true")
	}
	if !cs.IsStale {
		t.Error("expected is_stale=true (48h old with 24h TTL)")
	}
}

func TestEndpoints_CacheStatusRepoError(t *testing.T) {
	endpoints := []config.Endpoint{
		{
			Slug:    "error-ep",
			BaseURL: "http://example.com",
			Path:    "/data",
			Format:  "json",
		},
	}
	for i := range endpoints {
		config.ApplyDefaults(&endpoints[i])
	}

	repo := &mockRepo{
		fetchedAtMap: map[string]time.Time{},
		err:          errors.New("db unavailable"),
	}
	h := handler.NewEndpointsHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// Should still return 200 with degraded cache status
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 even with repo error, got %d", w.Code)
	}

	var result []handler.EndpointInfo
	_ = json.NewDecoder(w.Body).Decode(&result)

	cs := result[0].CacheStatus
	if cs == nil {
		t.Fatal("expected cache_status to be present even on error")
	}
	if cs.Cached {
		t.Error("expected cached=false on repo error")
	}
}

func TestEndpoints_NilRepo(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "test", BaseURL: "http://example.com", Path: "/data", Format: "json"},
	}
	for i := range endpoints {
		config.ApplyDefaults(&endpoints[i])
	}

	h := handler.NewEndpointsHandler(endpoints, nil)

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var result []handler.EndpointInfo
	_ = json.NewDecoder(w.Body).Decode(&result)

	if result[0].CacheStatus != nil {
		t.Error("expected cache_status to be nil when repo is nil")
	}
}

// --- Schedule and TTL fields ---

func TestEndpoints_ScheduleAndTTLFields(t *testing.T) {
	endpoints := []config.Endpoint{
		{
			Slug:    "scheduled-ep",
			BaseURL: "http://example.com",
			Path:    "/data",
			Format:  "json",
			Refresh: "0 */6 * * *",
			TTL:     "24h",
		},
	}
	for i := range endpoints {
		config.ApplyDefaults(&endpoints[i])
	}

	repo := &mockRepo{fetchedAtMap: map[string]time.Time{}}
	h := handler.NewEndpointsHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var result []handler.EndpointInfo
	_ = json.NewDecoder(w.Body).Decode(&result)

	if result[0].Schedule != "0 */6 * * *" {
		t.Errorf("expected schedule '0 */6 * * *', got '%s'", result[0].Schedule)
	}
	if result[0].TTL != "24h" {
		t.Errorf("expected ttl '24h', got '%s'", result[0].TTL)
	}
}

// --- No params endpoint ---

func TestEndpoints_NoParams(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "simple", BaseURL: "http://example.com", Path: "/data", Format: "json"},
	}
	for i := range endpoints {
		config.ApplyDefaults(&endpoints[i])
	}

	repo := &mockRepo{fetchedAtMap: map[string]time.Time{}}
	h := handler.NewEndpointsHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var result []handler.EndpointInfo
	_ = json.NewDecoder(w.Body).Decode(&result)

	if result[0].Params != nil && len(result[0].Params) != 0 {
		t.Errorf("expected empty/nil params for no-param endpoint, got %d", len(result[0].Params))
	}
}
