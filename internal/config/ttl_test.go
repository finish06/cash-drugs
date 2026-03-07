package config_test

import (
	"testing"
	"time"

	"github.com/finish06/drugs/internal/config"
)

// AC-001: Endpoints support optional TTL field with Go duration string
func TestTTL_AC001_ValidTTLParsed(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: drugnames
    base_url: http://example.com
    path: /api
    format: json
    ttl: "6h"
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if endpoints[0].TTL != "6h" {
		t.Errorf("expected TTL='6h', got '%s'", endpoints[0].TTL)
	}
	if endpoints[0].TTLDuration != 6*time.Hour {
		t.Errorf("expected TTLDuration=6h, got %v", endpoints[0].TTLDuration)
	}
}

// AC-001: Various valid TTL formats
func TestTTL_AC001_VariousDurations(t *testing.T) {
	tests := []struct {
		ttl      string
		expected time.Duration
	}{
		{"30m", 30 * time.Minute},
		{"24h", 24 * time.Hour},
		{"1h30m", 90 * time.Minute},
		{"0s", 0},
	}

	for _, tt := range tests {
		cfgPath := writeTestConfig(t, `
endpoints:
  - slug: test
    base_url: http://example.com
    path: /api
    format: json
    ttl: "`+tt.ttl+`"
`)
		endpoints, err := config.Load(cfgPath)
		if err != nil {
			t.Fatalf("ttl=%s: unexpected error: %v", tt.ttl, err)
		}
		if endpoints[0].TTLDuration != tt.expected {
			t.Errorf("ttl=%s: expected TTLDuration=%v, got %v", tt.ttl, tt.expected, endpoints[0].TTLDuration)
		}
	}
}

// AC-002: Invalid TTL prevents startup with clear error
func TestTTL_AC002_InvalidTTLPreventsStartup(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: drugnames
    base_url: http://example.com
    path: /api
    format: json
    ttl: "not-a-duration"
`)

	_, err := config.Load(cfgPath)
	if err == nil {
		t.Fatal("expected error for invalid TTL")
	}

	expected := "endpoint 'drugnames': invalid ttl"
	if !containsString(err.Error(), expected) {
		t.Errorf("expected error containing '%s', got '%s'", expected, err.Error())
	}
}

// AC-003: No TTL field means no expiry (TTLDuration is zero)
func TestTTL_AC003_NoTTLMeansNoExpiry(t *testing.T) {
	cfgPath := writeTestConfig(t, `
endpoints:
  - slug: drugnames
    base_url: http://example.com
    path: /api
    format: json
`)

	endpoints, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if endpoints[0].TTL != "" {
		t.Errorf("expected empty TTL, got '%s'", endpoints[0].TTL)
	}
	if endpoints[0].TTLDuration != 0 {
		t.Errorf("expected TTLDuration=0, got %v", endpoints[0].TTLDuration)
	}
}

// AC-010 + AC-003 + AC-004 + AC-005: IsStale checks fetched_at against TTLDuration
func TestTTL_AC010_IsStaleUsesFetchedAt(t *testing.T) {
	// No TTL: never stale
	ep := config.Endpoint{TTLDuration: 0}
	if config.IsStale(ep, time.Now().Add(-30*24*time.Hour)) {
		t.Error("expected not stale when TTLDuration=0")
	}

	// Within TTL: not stale
	ep = config.Endpoint{TTL: "6h", TTLDuration: 6 * time.Hour}
	if config.IsStale(ep, time.Now().Add(-2*time.Hour)) {
		t.Error("expected not stale when within TTL window")
	}

	// Past TTL: stale
	ep = config.Endpoint{TTL: "6h", TTLDuration: 6 * time.Hour}
	if !config.IsStale(ep, time.Now().Add(-8*time.Hour)) {
		t.Error("expected stale when past TTL window")
	}

	// TTL of 0s: always stale
	ep = config.Endpoint{TTLDuration: 0 * time.Second, TTL: "0s"}
	// With explicit TTL "0s", TTLDuration is 0 but TTL field is set — should be stale
	if !config.IsStale(ep, time.Now().Add(-1*time.Second)) {
		t.Error("expected stale when TTL='0s' (always stale)")
	}
}

func containsString(s, substr string) bool {
	return len(s) >= len(substr) && searchString(s, substr)
}

func searchString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
