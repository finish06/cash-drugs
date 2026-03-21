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
	"github.com/finish06/cash-drugs/internal/upstream"
)

// mockMetaRepo implements cache.Repository for meta handler tests.
type mockMetaRepo struct {
	fetchedAt    time.Time
	fetchedFound bool
	fetchedErr   error
	cached       *model.CachedResponse
	getErr       error
}

func (m *mockMetaRepo) Get(cacheKey string) (*model.CachedResponse, error) {
	return m.cached, m.getErr
}

func (m *mockMetaRepo) Upsert(resp *model.CachedResponse) error { return nil }

func (m *mockMetaRepo) FetchedAt(cacheKey string) (time.Time, bool, error) {
	return m.fetchedAt, m.fetchedFound, m.fetchedErr
}

// AC-010: Slug not found returns 404 with CD-H001
func TestMetaHandler_AC010_SlugNotFound(t *testing.T) {
	h := handler.NewMetaHandler(
		[]config.Endpoint{},
		&mockMetaRepo{},
	)

	req := httptest.NewRequest("GET", "/api/cache/nonexistent/_meta", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	var errResp model.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error response: %v", err)
	}
	if errResp.ErrorCode != "CD-H001" {
		t.Errorf("expected error_code CD-H001, got %q", errResp.ErrorCode)
	}
	if errResp.Slug != "nonexistent" {
		t.Errorf("expected slug 'nonexistent', got %q", errResp.Slug)
	}
}

// AC-001 through AC-009: Fresh cached endpoint returns all fields populated
func TestMetaHandler_FreshCachedEndpoint(t *testing.T) {
	now := time.Now()
	fetchedAt := now.Add(-2 * time.Hour)

	ep := config.Endpoint{
		Slug:        "fda-ndc",
		BaseURL:     "http://example.com",
		Path:        "/v2/ndc",
		Format:      "json",
		TTL:         "24h",
		TTLDuration: 24 * time.Hour,
		Refresh:     "0 */6 * * *",
	}
	config.ApplyDefaults(&ep)

	repo := &mockMetaRepo{
		fetchedAt:    fetchedAt,
		fetchedFound: true,
		cached: &model.CachedResponse{
			Slug:      "fda-ndc",
			PageCount: 3,
			Data:      make([]interface{}, 150),
			FetchedAt: fetchedAt,
		},
	}

	h := handler.NewMetaHandler([]config.Endpoint{ep}, repo)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc/_meta", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var meta handler.SlugMeta
	if err := json.NewDecoder(w.Body).Decode(&meta); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	// AC-008: slug is echoed
	if meta.Slug != "fda-ndc" {
		t.Errorf("expected slug 'fda-ndc', got %q", meta.Slug)
	}

	// AC-002: last_refreshed is populated
	if meta.LastRefreshed == nil {
		t.Fatal("expected non-nil last_refreshed")
	}

	// AC-004: is_stale is false for fresh entry
	if meta.IsStale {
		t.Error("expected is_stale=false for fresh entry")
	}

	// AC-003: ttl_remaining is non-zero
	if meta.TTLRemaining == "0s" {
		t.Error("expected non-zero ttl_remaining for fresh entry")
	}

	// AC-005: page_count
	if meta.PageCount != 3 {
		t.Errorf("expected page_count=3, got %d", meta.PageCount)
	}

	// AC-006: record_count
	if meta.RecordCount != 150 {
		t.Errorf("expected record_count=150, got %d", meta.RecordCount)
	}

	// AC-007: circuit_state defaults to closed
	if meta.CircuitState != "closed" {
		t.Errorf("expected circuit_state 'closed', got %q", meta.CircuitState)
	}

	// AC-009: has_schedule and schedule
	if !meta.HasSchedule {
		t.Error("expected has_schedule=true")
	}
	if meta.Schedule == nil || *meta.Schedule != "0 */6 * * *" {
		t.Errorf("expected schedule '0 */6 * * *', got %v", meta.Schedule)
	}
}

// AC-003, AC-004: Stale endpoint returns is_stale=true and ttl_remaining=0s
func TestMetaHandler_StaleEndpoint(t *testing.T) {
	fetchedAt := time.Now().Add(-48 * time.Hour)

	ep := config.Endpoint{
		Slug:        "fda-ndc",
		BaseURL:     "http://example.com",
		Path:        "/v2/ndc",
		Format:      "json",
		TTL:         "24h",
		TTLDuration: 24 * time.Hour,
	}
	config.ApplyDefaults(&ep)

	repo := &mockMetaRepo{
		fetchedAt:    fetchedAt,
		fetchedFound: true,
		cached: &model.CachedResponse{
			Slug:      "fda-ndc",
			PageCount: 3,
			Data:      make([]interface{}, 150),
			FetchedAt: fetchedAt,
		},
	}

	h := handler.NewMetaHandler([]config.Endpoint{ep}, repo)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc/_meta", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var meta handler.SlugMeta
	if err := json.NewDecoder(w.Body).Decode(&meta); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !meta.IsStale {
		t.Error("expected is_stale=true for stale entry")
	}
	if meta.TTLRemaining != "0s" {
		t.Errorf("expected ttl_remaining='0s', got %q", meta.TTLRemaining)
	}
}

// AC-011: Configured but never cached returns 200 with zero/null values
func TestMetaHandler_AC011_NeverCached(t *testing.T) {
	ep := config.Endpoint{
		Slug:        "rxnorm-drugs",
		BaseURL:     "http://example.com",
		Path:        "/v2/rxnorm",
		Format:      "json",
		TTL:         "12h",
		TTLDuration: 12 * time.Hour,
	}
	config.ApplyDefaults(&ep)

	repo := &mockMetaRepo{
		fetchedFound: false,
	}

	h := handler.NewMetaHandler([]config.Endpoint{ep}, repo)

	req := httptest.NewRequest("GET", "/api/cache/rxnorm-drugs/_meta", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var meta handler.SlugMeta
	if err := json.NewDecoder(w.Body).Decode(&meta); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if meta.LastRefreshed != nil {
		t.Errorf("expected null last_refreshed, got %v", meta.LastRefreshed)
	}
	if !meta.IsStale {
		t.Error("expected is_stale=true for never-cached endpoint")
	}
	if meta.TTLRemaining != "0s" {
		t.Errorf("expected ttl_remaining='0s', got %q", meta.TTLRemaining)
	}
	if meta.RecordCount != 0 {
		t.Errorf("expected record_count=0, got %d", meta.RecordCount)
	}
	if meta.PageCount != 0 {
		t.Errorf("expected page_count=0, got %d", meta.PageCount)
	}
}

// AC-007: Circuit state included when registry provided
func TestMetaHandler_AC007_CircuitStateOpen(t *testing.T) {
	ep := config.Endpoint{
		Slug:        "fda-ndc",
		BaseURL:     "http://example.com",
		Path:        "/v2/ndc",
		Format:      "json",
		TTL:         "24h",
		TTLDuration: 24 * time.Hour,
	}
	config.ApplyDefaults(&ep)

	repo := &mockMetaRepo{
		fetchedFound: false,
	}

	// Create a circuit registry and trip the breaker to open state
	circuitReg := upstream.NewCircuitRegistry(1, 60*time.Second)
	// Trip the circuit by executing a failing function
	_, _ = circuitReg.Execute("fda-ndc", func() (interface{}, error) {
		return nil, http.ErrAbortHandler
	})

	h := handler.NewMetaHandler(
		[]config.Endpoint{ep}, repo,
		handler.WithMetaCircuit(circuitReg),
	)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc/_meta", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var meta handler.SlugMeta
	if err := json.NewDecoder(w.Body).Decode(&meta); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if meta.CircuitState != "open" {
		t.Errorf("expected circuit_state 'open', got %q", meta.CircuitState)
	}
}

// Parameterized endpoint shows has_params=true
func TestMetaHandler_ParameterizedEndpoint(t *testing.T) {
	ep := config.Endpoint{
		Slug:        "spl-detail",
		BaseURL:     "http://example.com",
		Path:        "/v2/spl/{SETID}",
		Format:      "json",
		TTL:         "24h",
		TTLDuration: 24 * time.Hour,
	}
	config.ApplyDefaults(&ep)

	repo := &mockMetaRepo{
		fetchedFound: false,
	}

	h := handler.NewMetaHandler([]config.Endpoint{ep}, repo)

	req := httptest.NewRequest("GET", "/api/cache/spl-detail/_meta", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var meta handler.SlugMeta
	if err := json.NewDecoder(w.Body).Decode(&meta); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if !meta.HasParams {
		t.Error("expected has_params=true for parameterized endpoint")
	}
}

// Non-parameterized endpoint shows has_params=false
func TestMetaHandler_NonParameterizedEndpoint(t *testing.T) {
	ep := config.Endpoint{
		Slug:        "drugnames",
		BaseURL:     "http://example.com",
		Path:        "/v2/drugnames",
		Format:      "json",
	}
	config.ApplyDefaults(&ep)

	repo := &mockMetaRepo{
		fetchedFound: false,
	}

	h := handler.NewMetaHandler([]config.Endpoint{ep}, repo)

	req := httptest.NewRequest("GET", "/api/cache/drugnames/_meta", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var meta handler.SlugMeta
	if err := json.NewDecoder(w.Body).Decode(&meta); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if meta.HasParams {
		t.Error("expected has_params=false for non-parameterized endpoint")
	}
}
