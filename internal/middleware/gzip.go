package middleware

import (
	"bytes"
	"compress/gzip"
	"net/http"
	"strings"
)

const gzipMinSize = 1024 // 1KB minimum for compression

// gzipResponseWriter buffers the response to check size before compressing.
type gzipResponseWriter struct {
	http.ResponseWriter
	buf        bytes.Buffer
	statusCode int
}

func (w *gzipResponseWriter) WriteHeader(code int) {
	w.statusCode = code
}

func (w *gzipResponseWriter) Write(b []byte) (int, error) {
	return w.buf.Write(b)
}

// GzipMiddleware returns an http.Handler that compresses responses with gzip
// when the client sends Accept-Encoding: gzip and the response is large enough.
// Only compresses application/json, application/xml, and text/xml content types.
func GzipMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Always set Vary header so caches know responses differ by encoding
		w.Header().Set("Vary", "Accept-Encoding")

		// Check if client accepts gzip
		if !strings.Contains(r.Header.Get("Accept-Encoding"), "gzip") {
			next.ServeHTTP(w, r)
			return
		}

		// Buffer the response to check content type and size
		grw := &gzipResponseWriter{ResponseWriter: w}
		next.ServeHTTP(grw, r)

		statusCode := grw.statusCode
		if statusCode == 0 {
			statusCode = http.StatusOK
		}

		body := grw.buf.Bytes()

		// Check content type — only compress JSON and XML
		ct := grw.ResponseWriter.Header().Get("Content-Type")
		compressible := isCompressibleContentType(ct)

		// If body is too small or content type is not compressible, send uncompressed
		if len(body) < gzipMinSize || !compressible {
			w.WriteHeader(statusCode)
			w.Write(body)
			return
		}

		// Compress the response
		w.Header().Set("Content-Encoding", "gzip")
		w.Header().Del("Content-Length") // Length changes after compression
		w.WriteHeader(statusCode)

		gz := gzip.NewWriter(w)
		gz.Write(body)
		gz.Close()
	})
}

func isCompressibleContentType(ct string) bool {
	ct = strings.ToLower(ct)
	// Strip charset parameters (e.g., "application/json; charset=utf-8")
	if idx := strings.Index(ct, ";"); idx != -1 {
		ct = strings.TrimSpace(ct[:idx])
	}
	switch ct {
	case "application/json", "application/xml", "text/xml":
		return true
	}
	return false
}
