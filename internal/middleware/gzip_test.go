package middleware_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/finish06/cash-drugs/internal/middleware"
)

// AC: Gzip compression when client sends Accept-Encoding: gzip with JSON content
func TestGzip_CompressesJSONWhenAcceptEncodingGzip(t *testing.T) {
	body := strings.Repeat(`{"key":"value"}`, 200) // > 1KB
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})

	handler := middleware.GzipMiddleware(inner)

	req := httptest.NewRequest("GET", "/api/cache/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Error("expected Content-Encoding: gzip header on compressed response")
	}

	// Verify the body is valid gzip and decompresses to original
	gr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer func() { _ = gr.Close() }()
	decompressed, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("failed to decompress: %v", err)
	}
	if string(decompressed) != body {
		t.Errorf("decompressed body does not match original")
	}
}

// AC: Gzip compression for XML content type
func TestGzip_CompressesXMLWhenAcceptEncodingGzip(t *testing.T) {
	body := strings.Repeat(`<doc><item>value</item></doc>`, 100) // > 1KB
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})

	handler := middleware.GzipMiddleware(inner)

	req := httptest.NewRequest("GET", "/api/cache/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Error("expected Content-Encoding: gzip for XML content")
	}

	gr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
	if err != nil {
		t.Fatalf("failed to create gzip reader: %v", err)
	}
	defer func() { _ = gr.Close() }()
	decompressed, err := io.ReadAll(gr)
	if err != nil {
		t.Fatalf("failed to decompress: %v", err)
	}
	if string(decompressed) != body {
		t.Errorf("decompressed XML body does not match original")
	}
}

// AC: Uncompressed response when no Accept-Encoding: gzip header (backward compatible)
func TestGzip_NoCompressionWithoutAcceptEncoding(t *testing.T) {
	body := strings.Repeat(`{"key":"value"}`, 200)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})

	handler := middleware.GzipMiddleware(inner)

	req := httptest.NewRequest("GET", "/api/cache/test", nil)
	// No Accept-Encoding header
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("expected no gzip encoding when client does not accept gzip")
	}
	if w.Body.String() != body {
		t.Error("expected uncompressed body to match original")
	}
}

// AC: Responses < 1 KB are NOT compressed (overhead exceeds benefit)
func TestGzip_SmallResponseNotCompressed(t *testing.T) {
	body := `{"small":"data"}` // < 1KB
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})

	handler := middleware.GzipMiddleware(inner)

	req := httptest.NewRequest("GET", "/api/cache/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("expected no gzip encoding for response < 1KB")
	}
	if w.Body.String() != body {
		t.Error("expected uncompressed body for small response")
	}
}

// AC: Vary: Accept-Encoding header set
func TestGzip_VaryHeaderSet(t *testing.T) {
	body := strings.Repeat(`{"key":"value"}`, 200)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})

	handler := middleware.GzipMiddleware(inner)

	req := httptest.NewRequest("GET", "/api/cache/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Vary") != "Accept-Encoding" {
		t.Errorf("expected Vary: Accept-Encoding, got '%s'", w.Header().Get("Vary"))
	}
}

// AC: text/xml content type also compressed
func TestGzip_CompressesTextXML(t *testing.T) {
	body := strings.Repeat(`<doc>data</doc>`, 200)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})

	handler := middleware.GzipMiddleware(inner)

	req := httptest.NewRequest("GET", "/api/cache/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Error("expected Content-Encoding: gzip for text/xml content")
	}
}

// Non-compressible content types should not be compressed even if large
func TestGzip_NonCompressibleContentTypeNotCompressed(t *testing.T) {
	body := strings.Repeat("plain text data here ", 200) // > 1KB
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})

	handler := middleware.GzipMiddleware(inner)

	req := httptest.NewRequest("GET", "/api/cache/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("expected no gzip encoding for non-compressible content type")
	}
	if w.Body.String() != body {
		t.Error("expected uncompressed body for non-compressible content type")
	}
}

// JSON with charset parameter should still be compressed
func TestGzip_CompressesJSONWithCharset(t *testing.T) {
	body := strings.Repeat(`{"key":"value"}`, 200)
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})

	handler := middleware.GzipMiddleware(inner)

	req := httptest.NewRequest("GET", "/api/cache/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") != "gzip" {
		t.Error("expected Content-Encoding: gzip for application/json with charset")
	}
}

// image/png content type should not be compressed
func TestGzip_ImageContentTypeNotCompressed(t *testing.T) {
	body := strings.Repeat("\x89PNG fake image data", 200) // > 1KB
	inner := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(body))
	})

	handler := middleware.GzipMiddleware(inner)

	req := httptest.NewRequest("GET", "/api/cache/test", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, req)

	if w.Header().Get("Content-Encoding") == "gzip" {
		t.Error("expected no gzip encoding for image/png content type")
	}
}
