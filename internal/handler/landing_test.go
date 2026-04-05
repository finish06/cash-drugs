package handler_test

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/finish06/cash-drugs/internal/handler"
)

// AC-007: GET / returns 302 redirect when LANDING_URL is set
func TestAC007_RedirectWhenLandingURLSet(t *testing.T) {
	h := handler.NewLandingRedirectHandler("https://drug-cash.calebdunn.tech", http.NotFoundHandler())

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusFound {
		t.Errorf("expected 302, got %d", w.Code)
	}
	loc := w.Header().Get("Location")
	if loc != "https://drug-cash.calebdunn.tech" {
		t.Errorf("expected Location 'https://drug-cash.calebdunn.tech', got '%s'", loc)
	}
}

// AC-007: GET / falls through when LANDING_URL is unset
func TestAC007_NoRedirectWhenLandingURLUnset(t *testing.T) {
	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fallback"))
	})
	h := handler.NewLandingRedirectHandler("", fallback)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 from fallback, got %d", w.Code)
	}
	if w.Body.String() != "fallback" {
		t.Errorf("expected 'fallback' body, got '%s'", w.Body.String())
	}
}

// AC-007: Empty LANDING_URL treated as unset
func TestAC007_EmptyLandingURLNoRedirect(t *testing.T) {
	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fallback"))
	})
	h := handler.NewLandingRedirectHandler("   ", fallback)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 from fallback, got %d", w.Code)
	}
}

// AC-009: /api/endpoints not affected by redirect
func TestAC009_APIRoutesUnaffectedWithRedirect(t *testing.T) {
	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"endpoints":[]}`))
	})
	h := handler.NewLandingRedirectHandler("https://drug-cash.calebdunn.tech", fallback)

	req := httptest.NewRequest("GET", "/api/endpoints", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /api/endpoints, got %d", w.Code)
	}
	ct := w.Header().Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("expected application/json, got '%s'", ct)
	}
}

// AC-009: /health not affected by redirect
func TestAC009_HealthUnaffectedWithRedirect(t *testing.T) {
	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})
	h := handler.NewLandingRedirectHandler("https://drug-cash.calebdunn.tech", fallback)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for /health, got %d", w.Code)
	}
}

// AC-007: POST / does not redirect even when LANDING_URL is set
func TestAC007_PostMethodNoRedirect(t *testing.T) {
	fallback := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("fallback"))
	})
	h := handler.NewLandingRedirectHandler("https://drug-cash.calebdunn.tech", fallback)

	req := httptest.NewRequest("POST", "/", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code == http.StatusFound {
		t.Error("POST / should not redirect")
	}
}

// AC-010: landing/index.html is under 50KB
func TestAC010_LandingPageFileSize(t *testing.T) {
	path := filepath.Join("..", "..", "landing", "index.html")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("landing/index.html not found: %v", err)
	}
	maxSize := int64(50 * 1024) // 50KB
	if info.Size() > maxSize {
		t.Errorf("landing/index.html is %d bytes, exceeds 50KB limit", info.Size())
	}
}

// AC-002: landing page contains key content strings
func TestAC002_LandingPageContent(t *testing.T) {
	path := filepath.Join("..", "..", "landing", "index.html")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read landing/index.html: %v", err)
	}
	content := string(data)

	required := []string{"drug-cash", "DailyMed", "FDA", "RxNorm", "docker compose up"}
	for _, s := range required {
		if !strings.Contains(content, s) {
			t.Errorf("landing page missing required content: %q", s)
		}
	}
}
