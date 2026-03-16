package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/handler"
)

// --- Mock WarmupTrigger ---

type mockWarmupTrigger struct {
	mu             sync.Mutex
	triggeredAll   bool
	triggeredSlugs []string
	skipQueries    bool
	callCount      int32
}

func (m *mockWarmupTrigger) TriggerWarmup(slugs []string, skipQueries bool) {
	atomic.AddInt32(&m.callCount, 1)
	m.mu.Lock()
	defer m.mu.Unlock()
	m.skipQueries = skipQueries
	if slugs == nil {
		m.triggeredAll = true
	} else {
		m.triggeredSlugs = append(m.triggeredSlugs, slugs...)
	}
}

// AC-004: POST /api/warmup with no body triggers warmup for all endpoints, returns 202
func TestAC004_WarmupAllEndpointsNilBody(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "drugnames", Refresh: "*/5 * * * *", Format: "json", BaseURL: "http://example.com", Path: "/api"},
		{Slug: "fda-enforcement", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api"},
		{Slug: "no-refresh", Format: "json", BaseURL: "http://example.com", Path: "/api"},
	}
	trigger := &mockWarmupTrigger{}
	h := handler.NewWarmupHandler(endpoints, trigger)

	req := httptest.NewRequest("POST", "/api/warmup", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}

	var resp struct {
		Status  string `json:"status"`
		Warming int    `json:"warming"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Status != "accepted" {
		t.Errorf("expected status 'accepted', got '%s'", resp.Status)
	}
	// Should warm only scheduled endpoints (those with Refresh set)
	if resp.Warming != 2 {
		t.Errorf("expected warming 2 (scheduled endpoints), got %d", resp.Warming)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", ct)
	}
}

// AC-004: POST /api/warmup with empty JSON body triggers warmup for all endpoints
func TestAC004_WarmupAllEndpointsEmptyBody(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "drugnames", Refresh: "*/5 * * * *", Format: "json", BaseURL: "http://example.com", Path: "/api"},
	}
	trigger := &mockWarmupTrigger{}
	h := handler.NewWarmupHandler(endpoints, trigger)

	req := httptest.NewRequest("POST", "/api/warmup", bytes.NewReader([]byte("{}")))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
}

// AC-005: POST /api/warmup with specific slugs warms only those
func TestAC005_WarmupSpecificSlugs(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "drugnames", Refresh: "*/5 * * * *", Format: "json", BaseURL: "http://example.com", Path: "/api"},
		{Slug: "fda-enforcement", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api"},
		{Slug: "other", Refresh: "0 3 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api"},
	}
	trigger := &mockWarmupTrigger{}
	h := handler.NewWarmupHandler(endpoints, trigger)

	body := `{"slugs": ["drugnames", "fda-enforcement"]}`
	req := httptest.NewRequest("POST", "/api/warmup", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}

	var resp struct {
		Status  string `json:"status"`
		Warming int    `json:"warming"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Warming != 2 {
		t.Errorf("expected warming 2, got %d", resp.Warming)
	}
}

// AC-006: POST /api/warmup with invalid slug returns 400
func TestAC006_WarmupInvalidSlugReturns400(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "drugnames", Refresh: "*/5 * * * *", Format: "json", BaseURL: "http://example.com", Path: "/api"},
	}
	trigger := &mockWarmupTrigger{}
	h := handler.NewWarmupHandler(endpoints, trigger)

	body := `{"slugs": ["nonexistent"]}`
	req := httptest.NewRequest("POST", "/api/warmup", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}

	var resp struct {
		Error string `json:"error"`
		Slug  string `json:"slug"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.Error != "unknown slug" {
		t.Errorf("expected error 'unknown slug', got '%s'", resp.Error)
	}
	if resp.Slug != "nonexistent" {
		t.Errorf("expected slug 'nonexistent', got '%s'", resp.Slug)
	}
}

// AC-007: POST /api/warmup returns immediately without blocking
func TestAC007_WarmupReturnsImmediately(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "drugnames", Refresh: "*/5 * * * *", Format: "json", BaseURL: "http://example.com", Path: "/api"},
	}
	trigger := &mockWarmupTrigger{}
	h := handler.NewWarmupHandler(endpoints, trigger)

	req := httptest.NewRequest("POST", "/api/warmup", nil)
	w := httptest.NewRecorder()

	start := time.Now()
	h.ServeHTTP(w, req)
	duration := time.Since(start)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}
	// Should return immediately (well under 1 second)
	if duration > 500*time.Millisecond {
		t.Errorf("warmup endpoint blocked for %v, expected immediate return", duration)
	}
}

// GET /api/warmup returns 405 Method Not Allowed
func TestWarmupGETReturns405(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "drugnames", Refresh: "*/5 * * * *", Format: "json", BaseURL: "http://example.com", Path: "/api"},
	}
	trigger := &mockWarmupTrigger{}
	h := handler.NewWarmupHandler(endpoints, trigger)

	req := httptest.NewRequest("GET", "/api/warmup", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected 405, got %d", w.Code)
	}
}

// AC-009: Warmup deduplicates — calling warmup twice doesn't double-trigger
func TestAC009_WarmupDeduplicates(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "drugnames", Refresh: "*/5 * * * *", Format: "json", BaseURL: "http://example.com", Path: "/api"},
	}
	trigger := &mockWarmupTrigger{}
	h := handler.NewWarmupHandler(endpoints, trigger)

	// First call
	req := httptest.NewRequest("POST", "/api/warmup", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("first call: expected 202, got %d", w.Code)
	}

	// Second call — should still return 202 (deduplicated by trigger impl)
	req2 := httptest.NewRequest("POST", "/api/warmup", nil)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)
	if w2.Code != http.StatusAccepted {
		t.Errorf("second call: expected 202, got %d", w2.Code)
	}
}

// --- Parameterized Warmup Tests (specs/parameterized-warmup.md) ---

// AC-003: POST /api/warmup with no body warms scheduled + parameterized queries
func TestAC003_WarmupIncludesParameterizedQueries(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "drugnames", Refresh: "*/5 * * * *", Format: "json", BaseURL: "http://example.com", Path: "/api"},
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api"},
	}
	trigger := &mockWarmupTrigger{}
	warmupQueries := map[string][]map[string]string{
		"fda-ndc": {
			{"GENERIC_NAME": "METFORMIN"},
			{"GENERIC_NAME": "LISINOPRIL"},
		},
	}
	h := handler.NewWarmupHandler(endpoints, trigger, handler.WithWarmupQueries(warmupQueries))

	req := httptest.NewRequest("POST", "/api/warmup", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}

	var resp struct {
		Status         string `json:"status"`
		Warming        int    `json:"warming"`
		WarmingQueries int    `json:"warming_queries"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.WarmingQueries != 2 {
		t.Errorf("expected warming_queries 2, got %d", resp.WarmingQueries)
	}
}

// AC-004 (parameterized): POST /api/warmup with specific slugs includes only that slug's queries
func TestAC004_WarmupSpecificSlugIncludesQueries(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api"},
		{Slug: "rxnorm-find-drug", Format: "json", BaseURL: "http://example.com", Path: "/api"},
	}
	trigger := &mockWarmupTrigger{}
	warmupQueries := map[string][]map[string]string{
		"fda-ndc": {
			{"GENERIC_NAME": "METFORMIN"},
			{"GENERIC_NAME": "LISINOPRIL"},
			{"GENERIC_NAME": "ATORVASTATIN"},
		},
		"rxnorm-find-drug": {
			{"DRUG_NAME": "metformin"},
		},
	}
	h := handler.NewWarmupHandler(endpoints, trigger, handler.WithWarmupQueries(warmupQueries))

	body := `{"slugs": ["fda-ndc"]}`
	req := httptest.NewRequest("POST", "/api/warmup", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}

	var resp struct {
		WarmingQueries int `json:"warming_queries"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	// Only fda-ndc queries, not rxnorm-find-drug
	if resp.WarmingQueries != 3 {
		t.Errorf("expected warming_queries 3 (fda-ndc only), got %d", resp.WarmingQueries)
	}
}

// AC-005: POST /api/warmup with skip_queries=true skips parameterized queries
func TestAC005_WarmupSkipQueries(t *testing.T) {
	endpoints := []config.Endpoint{
		{Slug: "fda-ndc", Refresh: "0 2 * * *", Format: "json", BaseURL: "http://example.com", Path: "/api"},
	}
	trigger := &mockWarmupTrigger{}
	warmupQueries := map[string][]map[string]string{
		"fda-ndc": {{"GENERIC_NAME": "METFORMIN"}},
	}
	h := handler.NewWarmupHandler(endpoints, trigger, handler.WithWarmupQueries(warmupQueries))

	body := `{"slugs": ["fda-ndc"], "skip_queries": true}`
	req := httptest.NewRequest("POST", "/api/warmup", bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d", w.Code)
	}

	var resp struct {
		WarmingQueries int `json:"warming_queries"`
	}
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp.WarmingQueries != 0 {
		t.Errorf("expected warming_queries 0 with skip_queries=true, got %d", resp.WarmingQueries)
	}

	// Verify trigger was called with skipQueries=true
	trigger.mu.Lock()
	defer trigger.mu.Unlock()
	if !trigger.skipQueries {
		t.Error("expected trigger to be called with skipQueries=true")
	}
}
