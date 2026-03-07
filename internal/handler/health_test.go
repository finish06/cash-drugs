package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/finish06/cash-drugs/internal/handler"
)

// AC-008: Health check returns 200 when MongoDB is connected
func TestAC008_HealthCheckConnected(t *testing.T) {
	h := handler.NewHealthHandler(&mockPinger{healthy: true})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "ok" {
		t.Errorf("expected status 'ok', got '%s'", resp["status"])
	}
	if resp["db"] != "connected" {
		t.Errorf("expected db 'connected', got '%s'", resp["db"])
	}
}

// AC-008: Health check returns 503 when MongoDB is disconnected
func TestAC008_HealthCheckDisconnected(t *testing.T) {
	h := handler.NewHealthHandler(&mockPinger{healthy: false})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["status"] != "degraded" {
		t.Errorf("expected status 'degraded', got '%s'", resp["status"])
	}
	if resp["db"] != "disconnected" {
		t.Errorf("expected db 'disconnected', got '%s'", resp["db"])
	}
}

// AC-003 (docker-publish): Health check includes version field
func TestAC003_HealthCheckIncludesVersion(t *testing.T) {
	h := handler.NewHealthHandler(&mockPinger{healthy: true}, handler.WithVersion("v0.5.0"))

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["version"] != "v0.5.0" {
		t.Errorf("expected version 'v0.5.0', got '%s'", resp["version"])
	}
}

// AC-003 (docker-publish): Health check shows "dev" when no version set
func TestAC003_HealthCheckDefaultVersion(t *testing.T) {
	h := handler.NewHealthHandler(&mockPinger{healthy: true})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["version"] != "dev" {
		t.Errorf("expected version 'dev', got '%s'", resp["version"])
	}
}

// AC-003 (docker-publish): Degraded health check still includes version
func TestAC003_HealthCheckDegradedIncludesVersion(t *testing.T) {
	h := handler.NewHealthHandler(&mockPinger{healthy: false}, handler.WithVersion("v1.0.0"))

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["version"] != "v1.0.0" {
		t.Errorf("expected version 'v1.0.0', got '%s'", resp["version"])
	}
}

// --- Mock ---

type mockPinger struct {
	healthy bool
}

func (m *mockPinger) Ping() error {
	if m.healthy {
		return nil
	}
	return fmt.Errorf("connection refused")
}
