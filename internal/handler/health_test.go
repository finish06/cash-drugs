package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/finish06/drugs/internal/handler"
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
