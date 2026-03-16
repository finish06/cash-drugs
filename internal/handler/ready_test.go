package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/finish06/cash-drugs/internal/handler"
)

// --- Mock WarmupState ---

type mockWarmupState struct {
	ready     bool
	done      int
	total     int
	phase     string
}

func (m *mockWarmupState) IsReady() bool              { return m.ready }
func (m *mockWarmupState) Progress() (done, total int) { return m.done, m.total }
func (m *mockWarmupState) Phase() string               { return m.phase }

// AC-001: GET /ready returns 503 with warming status during warm-up
func TestAC001_ReadyReturns503WhenWarming(t *testing.T) {
	state := &mockWarmupState{ready: false, done: 0, total: 5}
	h := handler.NewReadyHandler(state)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "warming" {
		t.Errorf("expected status 'warming', got '%s'", resp["status"])
	}
	if resp["progress"] != "0/5" {
		t.Errorf("expected progress '0/5', got '%s'", resp["progress"])
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", ct)
	}
}

// AC-002: GET /ready returns 200 with ready status after warm-up complete
func TestAC002_ReadyReturns200WhenReady(t *testing.T) {
	state := &mockWarmupState{ready: true, done: 5, total: 5}
	h := handler.NewReadyHandler(state)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["status"] != "ready" {
		t.Errorf("expected status 'ready', got '%s'", resp["status"])
	}
	// Should not contain progress field when ready
	if _, ok := resp["progress"]; ok {
		t.Error("expected no 'progress' field when ready")
	}
}

// AC-008: Progress updates as endpoints warm
func TestAC008_ReadyProgressUpdates(t *testing.T) {
	state := &mockWarmupState{ready: false, done: 3, total: 5}
	h := handler.NewReadyHandler(state)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["progress"] != "3/5" {
		t.Errorf("expected progress '3/5', got '%s'", resp["progress"])
	}
}

// AC-007 (parameterized-warmup): Progress includes parameterized queries in total
func TestAC007_ReadyProgressIncludesQueries(t *testing.T) {
	// 5 scheduled + 196 queries = 201 total; 12 done
	state := &mockWarmupState{ready: false, done: 12, total: 201, phase: "queries"}
	h := handler.NewReadyHandler(state)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["progress"] != "12/201" {
		t.Errorf("expected progress '12/201', got '%s'", resp["progress"])
	}
	if resp["phase"] != "queries" {
		t.Errorf("expected phase 'queries', got '%s'", resp["phase"])
	}
}

// Phase field shows "scheduled" during scheduled warmup
func TestReadyPhaseScheduled(t *testing.T) {
	state := &mockWarmupState{ready: false, done: 2, total: 5, phase: "scheduled"}
	h := handler.NewReadyHandler(state)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp map[string]string
	if err := json.NewDecoder(w.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if resp["phase"] != "scheduled" {
		t.Errorf("expected phase 'scheduled', got '%s'", resp["phase"])
	}
}
