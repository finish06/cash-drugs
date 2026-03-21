package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/finish06/cash-drugs/internal/model"
)

// dummyHandler is a simple handler that writes 200 OK.
var dummyHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
})

func TestAllowMethods_GETPassesThrough(t *testing.T) {
	handler := AllowMethods(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/cache/foo", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestAllowMethods_POSTToCacheReturns405(t *testing.T) {
	handler := AllowMethods(dummyHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/cache/foo", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}

	if allow := rr.Header().Get("Allow"); allow != "GET" {
		t.Fatalf("expected Allow: GET, got %q", allow)
	}

	var body model.ErrorResponse
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("failed to decode body: %v", err)
	}
	if body.ErrorCode != model.ErrCodeMethodNotAllowed {
		t.Fatalf("expected error code %s, got %s", model.ErrCodeMethodNotAllowed, body.ErrorCode)
	}
}

func TestAllowMethods_POSTToWarmupPassesThrough(t *testing.T) {
	handler := AllowMethods(dummyHandler)

	req := httptest.NewRequest(http.MethodPost, "/api/warmup", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}
}

func TestAllowMethods_DELETEReturns405(t *testing.T) {
	handler := AllowMethods(dummyHandler)

	req := httptest.NewRequest(http.MethodDelete, "/api/cache/foo", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}

	if allow := rr.Header().Get("Allow"); allow != "GET" {
		t.Fatalf("expected Allow: GET, got %q", allow)
	}
}

func TestAllowMethods_PUTReturns405(t *testing.T) {
	handler := AllowMethods(dummyHandler)

	req := httptest.NewRequest(http.MethodPut, "/api/endpoints", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}

	if allow := rr.Header().Get("Allow"); allow != "GET" {
		t.Fatalf("expected Allow: GET, got %q", allow)
	}
}

func TestAllowMethods_GETToWarmupReturns405(t *testing.T) {
	handler := AllowMethods(dummyHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/warmup", nil)
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rr.Code)
	}

	if allow := rr.Header().Get("Allow"); allow != "POST" {
		t.Fatalf("expected Allow: POST, got %q", allow)
	}
}
