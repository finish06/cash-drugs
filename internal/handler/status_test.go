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

// mockStatusRepo implements cache.Repository for status handler tests.
// It allows per-slug control of FetchedAt results.
type mockStatusRepo struct {
	fetchedAtMap map[string]time.Time // slug -> fetched_at; absent key means not found
	fetchedAtErr error
}

func (m *mockStatusRepo) Get(cacheKey string) (*model.CachedResponse, error) {
	return nil, nil
}

func (m *mockStatusRepo) Upsert(resp *model.CachedResponse) error { return nil }

func (m *mockStatusRepo) FetchedAt(cacheKey string) (time.Time, bool, error) {
	if m.fetchedAtErr != nil {
		return time.Time{}, false, m.fetchedAtErr
	}
	t, ok := m.fetchedAtMap[cacheKey]
	return t, ok, nil
}

func TestStatusHandler_EmptyEndpoints(t *testing.T) {
	h := handler.NewStatusHandler(map[string]config.Endpoint{}, &mockStatusRepo{})

	req := httptest.NewRequest("GET", "/api/cache/status", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result handler.CacheStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.TotalSlugs != 0 {
		t.Errorf("expected total_slugs=0, got %d", result.TotalSlugs)
	}
	if result.HealthySlugs != 0 {
		t.Errorf("expected healthy_slugs=0, got %d", result.HealthySlugs)
	}
	if result.StaleSlugs != 0 {
		t.Errorf("expected stale_slugs=0, got %d", result.StaleSlugs)
	}
	if len(result.Slugs) != 0 {
		t.Errorf("expected empty slugs map, got %d entries", len(result.Slugs))
	}
	if result.GeneratedAt == "" {
		t.Error("expected non-empty generated_at")
	}
}

func TestStatusHandler_FreshEndpoint(t *testing.T) {
	now := time.Now()
	endpoints := map[string]config.Endpoint{
		"drugnames": {
			Slug:        "drugnames",
			TTL:         "4h",
			TTLDuration: 4 * time.Hour,
			Refresh:     "0 */4 * * *",
		},
	}
	repo := &mockStatusRepo{
		fetchedAtMap: map[string]time.Time{
			"drugnames": now.Add(-1 * time.Hour), // fetched 1h ago, TTL is 4h => fresh
		},
	}

	h := handler.NewStatusHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/cache/status", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result handler.CacheStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.TotalSlugs != 1 {
		t.Errorf("expected total_slugs=1, got %d", result.TotalSlugs)
	}
	if result.HealthySlugs != 1 {
		t.Errorf("expected healthy_slugs=1, got %d", result.HealthySlugs)
	}
	if result.StaleSlugs != 0 {
		t.Errorf("expected stale_slugs=0, got %d", result.StaleSlugs)
	}

	ss, ok := result.Slugs["drugnames"]
	if !ok {
		t.Fatal("expected drugnames in slugs map")
	}
	if ss.Health != 100 {
		t.Errorf("expected health=100, got %d", ss.Health)
	}
	if ss.IsStale {
		t.Error("expected is_stale=false")
	}
	if !ss.HasSchedule {
		t.Error("expected has_schedule=true")
	}
	if ss.Schedule != "0 */4 * * *" {
		t.Errorf("expected schedule='0 */4 * * *', got '%s'", ss.Schedule)
	}
	if ss.LastRefresh == "" {
		t.Error("expected non-empty last_refresh")
	}
	if ss.TTLRemaining == "0s" {
		t.Error("expected non-zero ttl_remaining for fresh entry")
	}
}

func TestStatusHandler_StaleEndpoint(t *testing.T) {
	now := time.Now()
	endpoints := map[string]config.Endpoint{
		"fda-ndc": {
			Slug:        "fda-ndc",
			TTL:         "24h",
			TTLDuration: 24 * time.Hour,
			Refresh:     "0 3 * * *",
		},
	}
	repo := &mockStatusRepo{
		fetchedAtMap: map[string]time.Time{
			"fda-ndc": now.Add(-48 * time.Hour), // fetched 48h ago, TTL is 24h => stale
		},
	}

	h := handler.NewStatusHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/cache/status", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result handler.CacheStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if result.HealthySlugs != 0 {
		t.Errorf("expected healthy_slugs=0, got %d", result.HealthySlugs)
	}
	if result.StaleSlugs != 1 {
		t.Errorf("expected stale_slugs=1, got %d", result.StaleSlugs)
	}

	ss := result.Slugs["fda-ndc"]
	if ss.Health != 50 {
		t.Errorf("expected health=50, got %d", ss.Health)
	}
	if !ss.IsStale {
		t.Error("expected is_stale=true")
	}
	if ss.TTLRemaining != "0s" {
		t.Errorf("expected ttl_remaining='0s', got '%s'", ss.TTLRemaining)
	}
}

func TestStatusHandler_NoCacheEntry(t *testing.T) {
	endpoints := map[string]config.Endpoint{
		"missing": {
			Slug:        "missing",
			TTL:         "1h",
			TTLDuration: 1 * time.Hour,
		},
	}
	// Empty fetchedAtMap => not found
	repo := &mockStatusRepo{fetchedAtMap: map[string]time.Time{}}

	h := handler.NewStatusHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/cache/status", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result handler.CacheStatusResponse
	if err := json.NewDecoder(w.Body).Decode(&result); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	ss := result.Slugs["missing"]
	if ss.Health != 0 {
		t.Errorf("expected health=0, got %d", ss.Health)
	}
	if !ss.IsStale {
		t.Error("expected is_stale=true for uncached endpoint")
	}
	if result.StaleSlugs != 1 {
		t.Errorf("expected stale_slugs=1, got %d", result.StaleSlugs)
	}
	if ss.LastRefresh != "" {
		t.Errorf("expected empty last_refresh, got '%s'", ss.LastRefresh)
	}
}

func TestStatusHandler_NoTTLConfigured(t *testing.T) {
	now := time.Now()
	endpoints := map[string]config.Endpoint{
		"no-ttl": {
			Slug: "no-ttl",
			// No TTL configured
		},
	}
	repo := &mockStatusRepo{
		fetchedAtMap: map[string]time.Time{
			"no-ttl": now.Add(-2 * time.Hour),
		},
	}

	h := handler.NewStatusHandler(endpoints, repo)

	req := httptest.NewRequest("GET", "/api/cache/status", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var result handler.CacheStatusResponse
	_ = json.NewDecoder(w.Body).Decode(&result)

	ss := result.Slugs["no-ttl"]
	if ss.Health != 75 {
		t.Errorf("expected health=75 (no TTL configured), got %d", ss.Health)
	}
	if ss.IsStale {
		t.Error("expected is_stale=false when no TTL configured")
	}
	if result.HealthySlugs != 1 {
		t.Errorf("expected healthy_slugs=1, got %d", result.HealthySlugs)
	}
}
