package middleware

import (
	"context"
	"crypto/rand"
	"fmt"
	"net/http"
)

// contextKey is an unexported type to prevent collisions with context keys
// defined in other packages.
type contextKey struct{}

// requestIDKey is the context key for the request ID.
var requestIDKey = contextKey{}

// RequestIDMiddleware checks for an incoming X-Request-ID header and uses it
// if present. Otherwise it generates a new UUID v4. The ID is stored in the
// request context and set as the X-Request-ID response header.
func RequestIDMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Request-ID")
		if id == "" {
			id = newUUID()
		}

		ctx := context.WithValue(r.Context(), requestIDKey, id)
		w.Header().Set("X-Request-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequestID extracts the request ID from the context. Returns an empty string
// if no request ID is present.
func RequestID(ctx context.Context) string {
	if id, ok := ctx.Value(requestIDKey).(string); ok {
		return id
	}
	return ""
}

// newUUID generates a UUID v4 using crypto/rand.
func newUUID() string {
	var uuid [16]byte
	_, _ = rand.Read(uuid[:])
	uuid[6] = (uuid[6] & 0x0f) | 0x40 // version 4
	uuid[8] = (uuid[8] & 0x3f) | 0x80 // variant 10
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		uuid[0:4], uuid[4:6], uuid[6:8], uuid[8:10], uuid[10:16])
}
