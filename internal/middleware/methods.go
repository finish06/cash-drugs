package middleware

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/finish06/cash-drugs/internal/model"
)

// AllowMethods enforces HTTP method restrictions per path prefix.
// /api/warmup allows POST only; all other paths allow GET only.
func AllowMethods(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		allowed := http.MethodGet
		if strings.HasPrefix(r.URL.Path, "/api/warmup") || strings.HasSuffix(r.URL.Path, "/bulk") || strings.HasPrefix(r.URL.Path, "/api/test-fetch") {
			allowed = http.MethodPost
		}

		if r.Method != allowed {
			w.Header().Set("Allow", allowed)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusMethodNotAllowed)
			_ = json.NewEncoder(w).Encode(model.ErrorResponse{
				Error:     "method not allowed",
				ErrorCode: model.ErrCodeMethodNotAllowed,
				Message:   "allowed: " + allowed,
				RequestID: RequestID(r.Context()),
			})
			return
		}

		next.ServeHTTP(w, r)
	})
}
