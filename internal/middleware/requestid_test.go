package middleware

import (
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

// uuidV4Re matches a standard UUID v4 string.
var uuidV4Re = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestRequestIDMiddleware_GeneratesID(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := RequestID(r.Context())
		if id == "" {
			t.Fatal("expected non-empty request ID in context")
		}
		if !uuidV4Re.MatchString(id) {
			t.Fatalf("expected UUID v4 format, got %q", id)
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	respID := rec.Header().Get("X-Request-ID")
	if respID == "" {
		t.Fatal("expected X-Request-ID response header")
	}
	if !uuidV4Re.MatchString(respID) {
		t.Fatalf("response header not UUID v4 format: %q", respID)
	}
}

func TestRequestIDMiddleware_PreservesExisting(t *testing.T) {
	const existing = "my-custom-trace-id-123"

	var ctxID string
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxID = RequestID(r.Context())
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", existing)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if ctxID != existing {
		t.Fatalf("context ID = %q, want %q", ctxID, existing)
	}
	if got := rec.Header().Get("X-Request-ID"); got != existing {
		t.Fatalf("response header = %q, want %q", got, existing)
	}
}

func TestRequestIDMiddleware_SetsResponseHeader(t *testing.T) {
	handler := RequestIDMiddleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Header().Get("X-Request-ID") == "" {
		t.Fatal("expected X-Request-ID response header to be set")
	}
}

func TestRequestID_EmptyContext(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	if got := RequestID(req.Context()); got != "" {
		t.Fatalf("expected empty string for context without request ID, got %q", got)
	}
}

func TestNewUUID_Format(t *testing.T) {
	for i := 0; i < 100; i++ {
		id := newUUID()
		if !uuidV4Re.MatchString(id) {
			t.Fatalf("iteration %d: %q does not match UUID v4 format", i, id)
		}
	}
}

func TestNewUUID_Unique(t *testing.T) {
	seen := make(map[string]struct{}, 1000)
	for i := 0; i < 1000; i++ {
		id := newUUID()
		if _, ok := seen[id]; ok {
			t.Fatalf("duplicate UUID at iteration %d: %s", i, id)
		}
		seen[id] = struct{}{}
	}
}
