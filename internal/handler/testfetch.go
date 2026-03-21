package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/middleware"
	"github.com/finish06/cash-drugs/internal/model"
	"github.com/finish06/cash-drugs/internal/upstream"
)

// testFetchRequest is the JSON body accepted by POST /api/test-fetch.
type testFetchRequest struct {
	BaseURL      string            `json:"base_url"`
	Path         string            `json:"path"`
	Format       string            `json:"format"`
	QueryParams  map[string]string `json:"query_params"`
	SearchParams []string          `json:"search_params"`
	DataKey      string            `json:"data_key"`
	TotalKey     string            `json:"total_key"`
	Pagesize     int               `json:"pagesize"`
	Flatten      bool              `json:"flatten"`
}

// testFetchResponse is returned on a successful test fetch.
type testFetchResponse struct {
	Success           bool        `json:"success"`
	StatusCode        int         `json:"status_code"`
	ContentType       string      `json:"content_type"`
	DataPreview       interface{} `json:"data_preview"`
	TotalResults      int         `json:"total_results"`
	PageCountEstimate int         `json:"page_count_estimate"`
	FetchDurationMS   int64       `json:"fetch_duration_ms"`
	RequestID         string      `json:"request_id"`
}

// testFetchErrorResponse is returned when the test fetch fails.
type testFetchErrorResponse struct {
	Success         bool   `json:"success"`
	Error           string `json:"error"`
	ErrorCode       string `json:"error_code"`
	StatusCode      int    `json:"status_code,omitempty"`
	FetchDurationMS int64  `json:"fetch_duration_ms,omitempty"`
	RequestID       string `json:"request_id"`
}

// TestFetchHandler handles POST /api/test-fetch.
type TestFetchHandler struct {
	fetcher upstream.Fetcher
}

// TestFetchOption configures a TestFetchHandler.
type TestFetchOption func(*TestFetchHandler)

// WithTestFetcher overrides the default HTTP fetcher (used for testing).
func WithTestFetcher(f upstream.Fetcher) TestFetchOption {
	return func(h *TestFetchHandler) {
		h.fetcher = f
	}
}

// NewTestFetchHandler creates a new TestFetchHandler.
// If no fetcher is provided via options, a default upstream.HTTPFetcher is used.
func NewTestFetchHandler(opts ...TestFetchOption) *TestFetchHandler {
	h := &TestFetchHandler{}
	for _, opt := range opts {
		opt(h)
	}
	if h.fetcher == nil {
		h.fetcher = upstream.NewHTTPFetcher()
	}
	return h
}

// maxPreviewItems is the maximum number of items returned in data_preview.
const maxPreviewItems = 5

// ServeHTTP handles the test-fetch request.
//
// @Summary      Test-fetch dry run
// @Description  Fetches one page from an upstream API without caching. Used to validate endpoint configs.
// @Tags         test
// @Accept       json
// @Produce      json
// @Param        body  body  testFetchRequest  true  "Endpoint configuration to test"
// @Success      200  {object}  testFetchResponse
// @Failure      400  {object}  testFetchErrorResponse
// @Router       /api/test-fetch [post]
func (h *TestFetchHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestID(r.Context())

	// Read and parse request body
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		respondTestFetchError(w, http.StatusBadRequest, "failed to read request body", model.ErrCodeBadRequest, 0, requestID)
		return
	}

	var req testFetchRequest
	if err := json.Unmarshal(body, &req); err != nil {
		respondTestFetchError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error(), model.ErrCodeBadRequest, 0, requestID)
		return
	}

	// Validate required fields
	if req.BaseURL == "" {
		respondTestFetchError(w, http.StatusBadRequest, "missing required field: base_url", model.ErrCodeBadRequest, 0, requestID)
		return
	}
	if req.Path == "" {
		respondTestFetchError(w, http.StatusBadRequest, "missing required field: path", model.ErrCodeBadRequest, 0, requestID)
		return
	}
	if req.Format == "" {
		respondTestFetchError(w, http.StatusBadRequest, "missing required field: format", model.ErrCodeBadRequest, 0, requestID)
		return
	}

	// Build a temporary Endpoint from the request
	ep := config.Endpoint{
		Slug:         "test-fetch",
		BaseURL:      req.BaseURL,
		Path:         req.Path,
		Format:       req.Format,
		QueryParams:  req.QueryParams,
		SearchParams: req.SearchParams,
		DataKey:      req.DataKey,
		TotalKey:     req.TotalKey,
		Pagesize:     req.Pagesize,
		Flatten:      req.Flatten,
		Pagination:   1, // force single page
	}
	config.ApplyDefaults(&ep)

	// Fetch one page from upstream
	start := time.Now()
	result, fetchErr := h.fetcher.Fetch(ep, nil)
	durationMS := time.Since(start).Milliseconds()

	if fetchErr != nil {
		slog.Warn("test-fetch upstream error", "component", "handler", "error", fetchErr, "duration_ms", durationMS)

		// Try to extract status code from known error types
		statusCode := 0
		var notFoundErr *upstream.ErrUpstreamNotFound
		if isNotFound := isUpstreamNotFound(fetchErr, &notFoundErr); isNotFound {
			statusCode = 404
		}

		respondTestFetchError(w, http.StatusOK, fetchErr.Error(), model.ErrCodeUpstreamUnavailable, durationMS, requestID)
		_ = statusCode // included in the error message from upstream
		return
	}

	// Build success response
	dataPreview, totalResults := buildPreview(result.Data)
	pageCountEstimate := estimatePageCount(totalResults, ep.Pagesize)

	resp := testFetchResponse{
		Success:           true,
		StatusCode:        result.HTTPStatus,
		ContentType:       result.ContentType,
		DataPreview:       dataPreview,
		TotalResults:      totalResults,
		PageCountEstimate: pageCountEstimate,
		FetchDurationMS:   durationMS,
		RequestID:         requestID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// isUpstreamNotFound checks if the error is an ErrUpstreamNotFound.
func isUpstreamNotFound(err error, target **upstream.ErrUpstreamNotFound) bool {
	e, ok := err.(*upstream.ErrUpstreamNotFound)
	if ok {
		*target = e
		return true
	}
	return false
}

// buildPreview extracts a preview (first 5 items) and total count from fetch result data.
func buildPreview(data interface{}) (preview interface{}, total int) {
	switch d := data.(type) {
	case []interface{}:
		total = len(d)
		if total > maxPreviewItems {
			return d[:maxPreviewItems], total
		}
		return d, total
	default:
		// Non-array data (string for XML, single object, etc.)
		return data, 1
	}
}

// estimatePageCount estimates total pages given total results and page size.
func estimatePageCount(totalResults, pagesize int) int {
	if pagesize <= 0 || totalResults <= 0 {
		return 1
	}
	return int(math.Ceil(float64(totalResults) / float64(pagesize)))
}

// respondTestFetchError writes a JSON error response for test-fetch.
func respondTestFetchError(w http.ResponseWriter, httpStatus int, errMsg, errCode string, durationMS int64, requestID string) {
	resp := testFetchErrorResponse{
		Success:         false,
		Error:           errMsg,
		ErrorCode:       errCode,
		StatusCode:      httpStatus,
		FetchDurationMS: durationMS,
		RequestID:       requestID,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(resp)
}
