package handler

import (
	"net/http"
	"strings"
)

// NewLandingRedirectHandler returns a handler that redirects exact GET /
// to landingURL when set. All other requests delegate to fallback.
// If landingURL is empty or whitespace, all requests go to fallback.
func NewLandingRedirectHandler(landingURL string, fallback http.Handler) http.Handler {
	url := strings.TrimSpace(landingURL)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if url != "" && r.URL.Path == "/" && r.Method == http.MethodGet {
			http.Redirect(w, r, url, http.StatusFound)
			return
		}
		fallback.ServeHTTP(w, r)
	})
}
