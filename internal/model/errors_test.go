package model

import "testing"

func TestErrorCodeConstants(t *testing.T) {
	codes := map[string]string{
		"ErrCodeEndpointNotFound":     ErrCodeEndpointNotFound,
		"ErrCodeUpstreamUnavailable":  ErrCodeUpstreamUnavailable,
		"ErrCodeUpstreamNotFound":     ErrCodeUpstreamNotFound,
		"ErrCodeCircuitOpen":          ErrCodeCircuitOpen,
		"ErrCodeServiceOverloaded":    ErrCodeServiceOverloaded,
		"ErrCodeForceRefreshCooldown": ErrCodeForceRefreshCooldown,
	}

	for name, code := range codes {
		if code == "" {
			t.Errorf("%s must not be empty", name)
		}
	}
}

func TestErrorCodePrefixes(t *testing.T) {
	tests := []struct {
		name           string
		code           string
		expectedPrefix string
	}{
		{"EndpointNotFound", ErrCodeEndpointNotFound, "CD-H"},
		{"ForceRefreshCooldown", ErrCodeForceRefreshCooldown, "CD-H"},
		{"UpstreamUnavailable", ErrCodeUpstreamUnavailable, "CD-U"},
		{"UpstreamNotFound", ErrCodeUpstreamNotFound, "CD-U"},
		{"CircuitOpen", ErrCodeCircuitOpen, "CD-U"},
		{"ServiceOverloaded", ErrCodeServiceOverloaded, "CD-S"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if len(tc.code) < len(tc.expectedPrefix) {
				t.Fatalf("code %q too short for prefix %q", tc.code, tc.expectedPrefix)
			}
			got := tc.code[:len(tc.expectedPrefix)]
			if got != tc.expectedPrefix {
				t.Errorf("expected prefix %q, got %q (code=%q)", tc.expectedPrefix, got, tc.code)
			}
		})
	}
}

func TestErrorCodeUniqueness(t *testing.T) {
	codes := []string{
		ErrCodeEndpointNotFound,
		ErrCodeUpstreamUnavailable,
		ErrCodeUpstreamNotFound,
		ErrCodeCircuitOpen,
		ErrCodeServiceOverloaded,
		ErrCodeForceRefreshCooldown,
	}

	seen := make(map[string]bool, len(codes))
	for _, code := range codes {
		if seen[code] {
			t.Errorf("duplicate error code: %s", code)
		}
		seen[code] = true
	}
}
