package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/handler"
)

// --- Mock ---

type mockPinger struct {
	healthy bool
	latency time.Duration
}

func (m *mockPinger) Ping() error {
	if m.healthy {
		return nil
	}
	return fmt.Errorf("connection refused")
}

func (m *mockPinger) PingWithLatency() (time.Duration, error) {
	if m.healthy {
		return m.latency, nil
	}
	return 0, fmt.Errorf("connection refused")
}

func decodeHealth(t *testing.T, body *httptest.ResponseRecorder) handler.HealthResponse {
	t.Helper()
	var resp handler.HealthResponse
	if err := json.NewDecoder(body.Body).Decode(&resp); err != nil {
		t.Fatalf("failed to decode /health response: %v", err)
	}
	return resp
}

// AC-001, AC-002, AC-005, AC-006: happy path — status ok, structured dependencies
func TestHealth_AC001_AC005_AC006_HappyPath(t *testing.T) {
	h := handler.NewHealthHandler(
		&mockPinger{healthy: true, latency: 2 * time.Millisecond},
		handler.WithVersion("v1.0.0"),
		handler.WithCacheSlugCount(20),
		handler.WithHealthLeader(true),
	)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	resp := decodeHealth(t, w)

	if resp.Status != "ok" {
		t.Errorf("expected status 'ok', got '%s'", resp.Status)
	}
	if resp.Version != "v1.0.0" {
		t.Errorf("expected version 'v1.0.0', got '%s'", resp.Version)
	}
	if len(resp.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(resp.Dependencies))
	}
	mongo := resp.Dependencies[0]
	if mongo.Name != "mongodb" {
		t.Errorf("expected dependency name 'mongodb', got '%s'", mongo.Name)
	}
	if mongo.Status != "connected" {
		t.Errorf("expected dependency status 'connected', got '%s'", mongo.Status)
	}
	if mongo.Error != "" {
		t.Errorf("expected empty error on healthy dep, got '%s'", mongo.Error)
	}
	if mongo.LatencyMs <= 0 {
		t.Errorf("expected positive latency_ms, got %f", mongo.LatencyMs)
	}
}

// AC-007: /health returns 503 and error status when MongoDB is down
func TestHealth_AC007_MongoDown(t *testing.T) {
	h := handler.NewHealthHandler(
		&mockPinger{healthy: false},
		handler.WithVersion("v1.0.0"),
	)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d", w.Code)
	}

	resp := decodeHealth(t, w)

	if resp.Status != "error" {
		t.Errorf("expected status 'error', got '%s'", resp.Status)
	}
	if len(resp.Dependencies) != 1 {
		t.Fatalf("expected 1 dependency, got %d", len(resp.Dependencies))
	}
	mongo := resp.Dependencies[0]
	if mongo.Status != "disconnected" {
		t.Errorf("expected dependency status 'disconnected', got '%s'", mongo.Status)
	}
	if mongo.Error == "" {
		t.Error("expected non-empty error on disconnected dep")
	}
	if mongo.LatencyMs != 0 {
		t.Errorf("expected latency_ms=0 on error, got %f", mongo.LatencyMs)
	}
}

// AC-003, AC-004: /health returns uptime (human-readable) and start_time (RFC3339)
func TestHealth_AC003_AC004_UptimeAndStartTime(t *testing.T) {
	start := time.Now().Add(-65 * time.Second) // "1m5s ago"
	h := handler.NewHealthHandler(
		&mockPinger{healthy: true},
		handler.WithHealthStartTime(start),
	)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	resp := decodeHealth(t, w)

	// Uptime should be a duration string (contains "m" or "s"), not a bare integer
	if resp.Uptime == "" {
		t.Error("expected non-empty uptime")
	}
	if _, err := time.ParseDuration(resp.Uptime); err != nil {
		t.Errorf("expected uptime to be a Go duration string, got '%s': %v", resp.Uptime, err)
	}

	// start_time should parse as RFC3339
	if _, err := time.Parse(time.RFC3339, resp.StartTime); err != nil {
		t.Errorf("expected start_time to be RFC3339, got '%s': %v", resp.StartTime, err)
	}
}

// AC-008, AC-009: /health includes domain-specific cache_slug_count and leader flag
func TestHealth_AC008_AC009_DomainFields(t *testing.T) {
	h := handler.NewHealthHandler(
		&mockPinger{healthy: true},
		handler.WithCacheSlugCount(42),
		handler.WithHealthLeader(true),
	)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	resp := decodeHealth(t, w)

	if resp.CacheSlugCount != 42 {
		t.Errorf("expected cache_slug_count=42, got %d", resp.CacheSlugCount)
	}
	if !resp.Leader {
		t.Error("expected leader=true")
	}
}

// AC-002: Default version is "dev"
func TestHealth_AC002_DefaultVersion(t *testing.T) {
	h := handler.NewHealthHandler(&mockPinger{healthy: true})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	resp := decodeHealth(t, w)
	if resp.Version != "dev" {
		t.Errorf("expected default version 'dev', got '%s'", resp.Version)
	}
}

// AC-014: Old flat `db` field is no longer emitted
func TestHealth_AC014_NoLegacyDbField(t *testing.T) {
	h := handler.NewHealthHandler(&mockPinger{healthy: true})

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	body := w.Body.String()
	if strings.Contains(body, `"db"`) {
		t.Errorf("expected no legacy 'db' field in /health response, got: %s", body)
	}
}
