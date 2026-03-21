# Spec: Go Client SDK

**Version:** 0.1.0
**Created:** 2026-03-20
**PRD Reference:** docs/prd.md (M15)
**Status:** Draft

## 1. Overview

Internal Go services consume cash-drugs via raw HTTP calls, manually constructing URLs, parsing JSON responses, and handling errors ad-hoc. This leads to duplicated boilerplate and inconsistent error handling across consumers. This feature provides a Go client SDK in `pkg/client/` with typed methods for all public endpoints, structured errors, auto-retry, and bulk query support.

### User Story

As an **internal Go service developer**, I want a typed client library for cash-drugs, so I can call `client.GetCache("fda-ndc", params)` instead of manually building HTTP requests, parsing JSON, and handling retries.

## 2. Acceptance Criteria

| ID | Criterion | Priority |
|----|-----------|----------|
| AC-001 | `pkg/client` package exports a `Client` struct created via `NewClient(baseURL string, opts ...ClientOption) *Client` | Must |
| AC-002 | `ClientOption` pattern supports: `WithHTTPClient(*http.Client)`, `WithTimeout(time.Duration)`, `WithRetry(maxRetries int)`, `WithUserAgent(string)` | Must |
| AC-003 | `Client.GetCache(ctx, slug, params)` returns `(*APIResponse, error)` with typed response | Must |
| AC-004 | `Client.GetEndpoints(ctx)` returns `([]EndpointInfo, error)` | Must |
| AC-005 | `Client.GetStatus(ctx)` returns `(*CacheStatusResponse, error)` | Must |
| AC-006 | `Client.GetVersion(ctx)` returns `(*VersionInfo, error)` | Must |
| AC-007 | `Client.BulkQuery(ctx, slug, queries)` returns `(*BulkQueryResponse, error)` | Must |
| AC-008 | `Client.GetMeta(ctx, slug)` returns `(*SlugMeta, error)` for the `/_meta` endpoint | Must |
| AC-009 | Typed sentinel errors: `ErrUpstreamDown` (502), `ErrCacheMiss` (cache miss in response), `ErrNotFound` (404), `ErrOverloaded` (503), `ErrBadRequest` (400) | Must |
| AC-010 | `APIError` type wraps HTTP errors with `StatusCode`, `ErrorCode`, `Message`, `RequestID`, `RetryAfter` fields | Must |
| AC-011 | Auto-retry on 503 responses, respecting the `Retry-After` header. Configurable max retries (default 3) with exponential backoff | Must |
| AC-012 | All methods accept `context.Context` and honor cancellation/timeout | Must |
| AC-013 | Test coverage of `pkg/client` is 80% or higher | Must |
| AC-014 | Client does not import any `internal/` packages — uses only public types defined in `pkg/client/` | Must |
| AC-015 | Package includes a `doc.go` with package-level documentation and usage example | Should |
| AC-016 | `Client.Health(ctx)` returns `(*HealthResponse, error)` for the `/health` endpoint | Should |

## 3. User Test Cases

### TC-001: Basic cache lookup

**Steps:**
1. Create client: `c := client.NewClient("http://localhost:8080")`
2. Call `resp, err := c.GetCache(ctx, "fda-ndc", map[string]string{"BRAND_NAME": "Tylenol"})`
**Expected Result:** `err` is nil, `resp.Data` is non-nil, `resp.Meta.Slug` is `"fda-ndc"`.
**Maps to:** AC-001, AC-003

### TC-002: Endpoint not found

**Steps:**
1. Call `c.GetCache(ctx, "nonexistent", nil)`
**Expected Result:** `err` wraps `ErrNotFound`. `errors.Is(err, client.ErrNotFound)` returns true. `err.(*APIError).StatusCode` is 404.
**Maps to:** AC-009, AC-010

### TC-003: Auto-retry on 503

**Steps:**
1. Server returns 503 with `Retry-After: 1` on first call, then 200 on second
2. Call `c.GetCache(ctx, "fda-ndc", params)` with retry enabled
**Expected Result:** Client retries after 1 second, returns successful response. No error.
**Maps to:** AC-011

### TC-004: Bulk query via client

**Steps:**
1. Call `c.BulkQuery(ctx, "fda-ndc", []client.BulkQueryItem{{Params: map[string]string{"BRAND_NAME": "Tylenol"}}})`
**Expected Result:** Returns `BulkQueryResponse` with `Total: 1`.
**Maps to:** AC-007

### TC-005: Context cancellation

**Steps:**
1. Create a context with immediate cancel
2. Call `c.GetCache(cancelledCtx, "fda-ndc", params)`
**Expected Result:** Returns `context.Canceled` error.
**Maps to:** AC-012

### TC-006: List endpoints

**Steps:**
1. Call `c.GetEndpoints(ctx)`
**Expected Result:** Returns a slice of `EndpointInfo` with at least one entry.
**Maps to:** AC-004

### TC-007: Get metadata

**Steps:**
1. Call `c.GetMeta(ctx, "fda-ndc")`
**Expected Result:** Returns `SlugMeta` with `LastRefreshed`, `RecordCount`, etc.
**Maps to:** AC-008

### TC-008: Custom HTTP client

**Steps:**
1. Create client with `client.NewClient(url, client.WithHTTPClient(customHTTP), client.WithTimeout(5*time.Second))`
2. Make a request
**Expected Result:** Request uses the custom HTTP client and respects timeout.
**Maps to:** AC-002

## 4. Data Model

### Public Types (in `pkg/client/`)

```go
// Client is the cash-drugs API client.
type Client struct { /* unexported fields */ }

// ClientOption configures the Client.
type ClientOption func(*Client)

// APIResponse mirrors model.APIResponse for consumers.
type APIResponse struct {
    Data interface{}  `json:"data"`
    Meta ResponseMeta `json:"meta"`
}

type ResponseMeta struct {
    Slug         string `json:"slug"`
    SourceURL    string `json:"source_url"`
    FetchedAt    string `json:"fetched_at"`
    PageCount    int    `json:"page_count"`
    ResultsCount int    `json:"results_count"`
    Stale        bool   `json:"stale"`
    StaleReason  string `json:"stale_reason,omitempty"`
}

// APIError is returned for non-2xx responses.
type APIError struct {
    StatusCode int
    ErrorCode  string
    Message    string
    RequestID  string
    RetryAfter int
}

func (e *APIError) Error() string { ... }

// Sentinel errors
var (
    ErrNotFound     = errors.New("endpoint not found")
    ErrUpstreamDown = errors.New("upstream unavailable")
    ErrCacheMiss    = errors.New("cache miss")
    ErrOverloaded   = errors.New("service overloaded")
    ErrBadRequest   = errors.New("bad request")
)

// EndpointInfo mirrors handler.EndpointInfo.
type EndpointInfo struct { ... }

// VersionInfo mirrors handler.VersionInfo.
type VersionInfo struct { ... }

// CacheStatusResponse mirrors handler.CacheStatusResponse.
type CacheStatusResponse struct { ... }

// SlugMeta mirrors the /_meta response.
type SlugMeta struct { ... }

// BulkQueryItem is a single query in a bulk request.
type BulkQueryItem struct {
    Params map[string]string `json:"params"`
}

// BulkQueryResponse mirrors the bulk endpoint response.
type BulkQueryResponse struct { ... }

// HealthResponse mirrors the /health response.
type HealthResponse struct { ... }
```

## 5. API Contract

The client is a consumer of cash-drugs HTTP APIs. It does not introduce new endpoints. Method mapping:

| Client Method | HTTP | Path |
|---------------|------|------|
| `GetCache(ctx, slug, params)` | GET | `/api/cache/{slug}?params...` |
| `BulkQuery(ctx, slug, queries)` | POST | `/api/cache/{slug}/bulk` |
| `GetMeta(ctx, slug)` | GET | `/api/cache/{slug}/_meta` |
| `GetEndpoints(ctx)` | GET | `/api/endpoints` |
| `GetStatus(ctx)` | GET | `/api/cache/status` |
| `GetVersion(ctx)` | GET | `/version` |
| `Health(ctx)` | GET | `/health` |

## 6. Edge Cases

| Case | Expected Behavior |
|------|-------------------|
| Base URL has trailing slash | Client normalizes (strips trailing slash) |
| Empty params map passed to GetCache | Requests the base slug URL (no query string) |
| Server returns unexpected JSON shape | Returns error with raw body context |
| Network timeout | Returns `context.DeadlineExceeded` or wrapped timeout error |
| Retry exhausted (all attempts return 503) | Returns last `APIError` with `ErrOverloaded` |
| Retry-After header missing on 503 | Falls back to exponential backoff (1s, 2s, 4s) |
| Retry-After header has invalid value | Ignores header, uses exponential backoff |
| BulkQuery with empty queries slice | Sends request; server returns 200 with empty results |
| nil context passed | Panics (standard Go convention for nil ctx) |

## 7. Dependencies

- Go stdlib: `net/http`, `encoding/json`, `context`, `time`, `errors`, `fmt`, `net/url`
- No third-party dependencies (keep the client lightweight)
- Test dependencies: `net/http/httptest` for mock server

## 8. Revision History

| Date | Version | Author | Changes |
|------|---------|--------|---------|
| 2026-03-20 | 0.1.0 | calebdunn | Initial spec |
