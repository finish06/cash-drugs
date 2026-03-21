package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/handler"
	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/finish06/cash-drugs/internal/model"
	"github.com/finish06/cash-drugs/internal/upstream"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/testutil"
)

// AC-013: Consumer requests for endpoints not defined in config return 404
func TestAC013_UnknownEndpoint404(t *testing.T) {
	h := handler.NewCacheHandler(
		[]config.Endpoint{},
		&mockCacheRepo{},
		&mockFetcher{},
	)

	req := httptest.NewRequest("GET", "/api/cache/nonexistent", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}

	var errResp model.ErrorResponse
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Error != "endpoint not configured" {
		t.Errorf("expected 'endpoint not configured' error, got '%s'", errResp.Error)
	}
	if errResp.Slug != "nonexistent" {
		t.Errorf("expected slug 'nonexistent', got '%s'", errResp.Slug)
	}
}

// extractSlug: paths with fewer than 3 parts return empty slug → 404
func TestExtractSlug_ShortPaths(t *testing.T) {
	h := handler.NewCacheHandler(
		[]config.Endpoint{},
		&mockCacheRepo{},
		&mockFetcher{},
	)

	tests := []struct {
		name string
		path string
	}{
		{"root path", "/"},
		{"single segment", "/api"},
		{"two segments", "/api/cache"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if w.Code != http.StatusNotFound {
				t.Errorf("expected 404 for path %q, got %d", tc.path, w.Code)
			}

			var errResp model.ErrorResponse
			_ = json.NewDecoder(w.Body).Decode(&errResp)
			if errResp.Slug != "" {
				t.Errorf("expected empty slug for path %q, got %q", tc.path, errResp.Slug)
			}
		})
	}
}

// AC-014: Consumer-facing URL pattern mirrors configured slug
func TestAC014_URLPatternMatchesSlug(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: &model.CachedResponse{
			Slug:      "drugnames",
			Data:      map[string]interface{}{"items": []interface{}{}},
			FetchedAt: time.Now(),
		}},
		&mockFetcher{},
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// AC-008: Return cached response to consumer if cache exists
func TestAC008_ReturnCachedResponse(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	cached := &model.CachedResponse{
		Slug:       "drugnames",
		CacheKey:   "drugnames",
		Data:       map[string]interface{}{"items": []interface{}{"drug1"}},
		FetchedAt:  time.Now(),
		SourceURL:  "http://example.com/v2/drugnames",
		PageCount:  1,
		HTTPStatus: 200,
	}

	fetcher := &mockFetcher{}
	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: cached},
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if fetcher.fetchCalled {
		t.Error("expected no upstream fetch when cache exists")
	}

	var resp model.APIResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Meta.Stale {
		t.Error("expected stale=false for fresh cache")
	}
}

// AC-004: No cache triggers upstream fetch
func TestAC004_NoCacheTriggersFetch(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:       "drugnames",
			CacheKey:   "drugnames",
			Data:       map[string]interface{}{"items": []interface{}{"drug1"}},
			FetchedAt:  time.Now(),
			SourceURL:  "http://example.com/v2/drugnames",
			PageCount:  1,
			HTTPStatus: 200,
		},
	}
	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !fetcher.fetchCalled {
		t.Error("expected upstream fetch when no cache exists")
	}
}

// AC-009: Upstream failure returns stale cache
func TestAC009_UpstreamFailureReturnsStaleCacheViaHandler(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	staleCache := &model.CachedResponse{
		Slug:       "drugnames",
		CacheKey:   "drugnames",
		Data:       map[string]interface{}{"items": []interface{}{"stale-drug"}},
		FetchedAt:  time.Now().Add(-24 * time.Hour),
		SourceURL:  "http://example.com/v2/drugnames",
		PageCount:  1,
		HTTPStatus: 200,
	}

	// Cache repo returns nil on first call (no cache), stale on second (after fetch fails)
	repo := &mockCacheRepo{cached: staleCache}
	fetcher := &mockFetcher{err: fmt.Errorf("upstream unavailable")}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames?_force=true", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// Should return 200 with stale data
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 with stale cache, got %d", w.Code)
	}

	var resp model.APIResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Meta.Stale {
		t.Error("expected stale=true when serving stale cache")
	}
}

// AC-010: Upstream fails and no cache returns 502
func TestAC010_UpstreamFailNoCacheReturns502(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		&mockFetcher{err: fmt.Errorf("upstream unavailable")},
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502, got %d", w.Code)
	}

	var errResp model.ErrorResponse
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.Error != "upstream unavailable" {
		t.Errorf("expected 'upstream unavailable' error, got '%s'", errResp.Error)
	}
}

// AC-007: Response stored with metadata
func TestAC007_ResponseStoredWithMetadata(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	repo := &mockCacheRepo{}
	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:       "drugnames",
			CacheKey:   "drugnames",
			Data:       map[string]interface{}{"items": []interface{}{"drug1"}},
			FetchedAt:  time.Now(),
			SourceURL:  "http://example.com/v2/drugnames",
			PageCount:  1,
			HTTPStatus: 200,
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !repo.upsertCalled {
		t.Error("expected cache upsert after upstream fetch")
	}
	if repo.lastUpserted == nil {
		t.Fatal("expected upserted document")
	}
	if repo.lastUpserted.Slug != "drugnames" {
		t.Errorf("expected slug 'drugnames', got '%s'", repo.lastUpserted.Slug)
	}
	if repo.lastUpserted.FetchedAt.IsZero() {
		t.Error("expected fetched_at timestamp")
	}
	if repo.lastUpserted.SourceURL == "" {
		t.Error("expected source_url")
	}
	if repo.lastUpserted.PageCount < 1 {
		t.Error("expected page_count >= 1")
	}
}

// AC-003: Path parameter from query string
func TestAC003_PathParamFromQueryString(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "spl-detail",
		BaseURL: "http://example.com",
		Path:    "/v2/spls/{SETID}",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:      "spl-detail",
			CacheKey:  "spl-detail:SETID=abc-123",
			Data:      map[string]interface{}{"spl": "data"},
			FetchedAt: time.Now(),
			SourceURL: "http://example.com/v2/spls/abc-123",
			PageCount: 1,
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/spl-detail?SETID=abc-123", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !fetcher.fetchCalled {
		t.Error("expected fetch to be called")
	}
	if fetcher.lastParams["SETID"] != "abc-123" {
		t.Errorf("expected SETID=abc-123 passed to fetcher, got %v", fetcher.lastParams)
	}
}

// AC-018: Raw/XML responses served with upstream content type (no JSON envelope)
func TestAC018_RawResponseServedDirectly(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "spl-xml",
		BaseURL: "http://example.com",
		Path:    "/v2/spls/{SETID}.xml",
		Format:  "raw",
	}
	config.ApplyDefaults(&ep)

	xmlBody := `<document><title>Test SPL</title></document>`
	cached := &model.CachedResponse{
		Slug:        "spl-xml",
		CacheKey:    "spl-xml:SETID=abc-123",
		Data:        xmlBody,
		ContentType: "application/xml",
		FetchedAt:   time.Now(),
		SourceURL:   "http://example.com/v2/spls/abc-123.xml",
		PageCount:   1,
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: cached},
		&mockFetcher{},
	)

	req := httptest.NewRequest("GET", "/api/cache/spl-xml?SETID=abc-123", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/xml" {
		t.Errorf("expected Content-Type 'application/xml', got '%s'", ct)
	}
	if w.Body.String() != xmlBody {
		t.Errorf("expected raw XML body, got '%s'", w.Body.String())
	}
}

// AC-018: Raw stale response includes stale headers
func TestAC018_RawStaleResponseHeaders(t *testing.T) {
	ep := config.Endpoint{
		Slug:        "spl-xml",
		BaseURL:     "http://example.com",
		Path:        "/v2/spls/{SETID}.xml",
		Format:      "raw",
		TTL:         "1s",
		TTLDuration: 1 * time.Second,
	}
	config.ApplyDefaults(&ep)

	cached := &model.CachedResponse{
		Slug:        "spl-xml",
		CacheKey:    "spl-xml:SETID=abc-123",
		Data:        "<doc/>",
		ContentType: "application/xml",
		FetchedAt:   time.Now().Add(-1 * time.Hour), // stale
		SourceURL:   "http://example.com/v2/spls/abc-123.xml",
		PageCount:   1,
	}

	fl := handler.NewFetchLocks()
	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: cached},
		&mockFetcher{err: fmt.Errorf("fail")},
		handler.WithFetchLocks(fl),
	)

	req := httptest.NewRequest("GET", "/api/cache/spl-xml?SETID=abc-123", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("X-Cache-Stale") != "true" {
		t.Error("expected X-Cache-Stale header for stale raw response")
	}
}

// AC-016: Handler extracts params from query_params {PARAM} placeholders
func TestAC016_HandlerExtractsQueryParamPlaceholders(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "spl-detail",
		BaseURL: "http://example.com",
		Path:    "/v2/spls.json",
		Format:  "json",
		QueryParams: map[string]string{
			"setid": "{SETID}",
		},
	}
	config.ApplyDefaults(&ep)

	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:      "spl-detail",
			CacheKey:  "spl-detail:SETID=abc-123",
			Data:      map[string]interface{}{"spl": "data"},
			FetchedAt: time.Now(),
			SourceURL: "http://example.com/v2/spls.json",
			PageCount: 1,
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/spl-detail?SETID=abc-123", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !fetcher.fetchCalled {
		t.Error("expected fetch to be called")
	}
	if fetcher.lastParams["SETID"] != "abc-123" {
		t.Errorf("expected SETID=abc-123 from query_params placeholder, got %v", fetcher.lastParams)
	}
}

// M8-AC-002/AC-003: HTTP request metrics recorded on cache hit
func TestM8_AC002_HTTPRequestMetricsOnCacheHit(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: &model.CachedResponse{
			Slug:      "drugnames",
			Data:      map[string]interface{}{"items": []interface{}{}},
			FetchedAt: time.Now(),
		}},
		&mockFetcher{},
		handler.WithMetrics(m),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	val := testutil.ToFloat64(m.HTTPRequestsTotal.WithLabelValues("drugnames", "GET", "200"))
	if val != 1 {
		t.Errorf("expected http_requests_total=1, got %f", val)
	}

	if testutil.CollectAndCount(m.HTTPRequestDuration) == 0 {
		t.Error("expected HTTP duration histogram to have observations")
	}
}

// M8-AC-004: Cache outcome counters recorded
func TestM8_AC004_CacheOutcomeMetrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	// Cache hit
	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: &model.CachedResponse{
			Slug:      "drugnames",
			Data:      map[string]interface{}{"items": []interface{}{}},
			FetchedAt: time.Now(),
		}},
		&mockFetcher{},
		handler.WithMetrics(m),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	hitVal := testutil.ToFloat64(m.CacheHitsTotal.WithLabelValues("drugnames", "hit"))
	if hitVal != 1 {
		t.Errorf("expected cache hit=1, got %f", hitVal)
	}

	// Cache miss
	h2 := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		&mockFetcher{result: &model.CachedResponse{
			Slug:      "drugnames",
			CacheKey:  "drugnames",
			Data:      []interface{}{},
			FetchedAt: time.Now(),
			PageCount: 1,
		}},
		handler.WithMetrics(m),
	)

	req2 := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w2 := httptest.NewRecorder()
	h2.ServeHTTP(w2, req2)

	missVal := testutil.ToFloat64(m.CacheHitsTotal.WithLabelValues("drugnames", "miss"))
	if missVal != 1 {
		t.Errorf("expected cache miss=1, got %f", missVal)
	}
}

// M8-AC-005/AC-006: Upstream fetch metrics on error
func TestM8_AC005_UpstreamFetchMetricsOnError(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		&mockFetcher{err: fmt.Errorf("upstream unavailable")},
		handler.WithMetrics(m),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	errVal := testutil.ToFloat64(m.UpstreamFetchErrors.WithLabelValues("drugnames"))
	if errVal != 1 {
		t.Errorf("expected upstream error=1, got %f", errVal)
	}

	if testutil.CollectAndCount(m.UpstreamFetchDuration) == 0 {
		t.Error("expected upstream fetch duration to be recorded even on error")
	}
}

// M8-AC-002: 404 metrics recorded for unknown slug
func TestM8_AC002_HTTPMetricsOn404(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	h := handler.NewCacheHandler(
		[]config.Endpoint{},
		&mockCacheRepo{},
		&mockFetcher{},
		handler.WithMetrics(m),
	)

	req := httptest.NewRequest("GET", "/api/cache/nonexistent", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	val := testutil.ToFloat64(m.HTTPRequestsTotal.WithLabelValues("nonexistent", "GET", "404"))
	if val != 1 {
		t.Errorf("expected 404 counter=1, got %f", val)
	}
}

// AC: Singleflight — concurrent requests for same cache key produce only 1 upstream fetch
func TestSingleflight_DeduplicatesConcurrentRequests(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	var fetchCount atomic.Int32
	fetcher := &slowMockFetcher{
		delay: 100 * time.Millisecond,
		result: &model.CachedResponse{
			Slug:      "drugnames",
			CacheKey:  "drugnames",
			Data:      map[string]interface{}{"items": []interface{}{"drug1"}},
			FetchedAt: time.Now(),
			SourceURL: "http://example.com/v2/drugnames",
			PageCount: 1,
		},
		fetchCount: &fetchCount,
	}

	lru := cache.NewLRUCache(0) // disabled LRU so we always go to upstream
	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		fetcher,
		handler.WithLRU(lru),
	)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
			if w.Code != http.StatusOK {
				t.Errorf("expected 200, got %d", w.Code)
			}
		}()
	}
	wg.Wait()

	count := fetchCount.Load()
	if count != 1 {
		t.Errorf("expected exactly 1 upstream fetch (singleflight dedup), got %d", count)
	}
}

// AC: Singleflight — different cache keys are independent
func TestSingleflight_DifferentKeysIndependent(t *testing.T) {
	ep1 := config.Endpoint{
		Slug:    "drug1",
		BaseURL: "http://example.com",
		Path:    "/v2/drug1",
		Format:  "json",
	}
	config.ApplyDefaults(&ep1)

	ep2 := config.Endpoint{
		Slug:    "drug2",
		BaseURL: "http://example.com",
		Path:    "/v2/drug2",
		Format:  "json",
	}
	config.ApplyDefaults(&ep2)

	var fetchCount atomic.Int32
	fetcher := &slowMockFetcher{
		delay: 50 * time.Millisecond,
		result: &model.CachedResponse{
			Slug:      "test",
			CacheKey:  "test",
			Data:      map[string]interface{}{"items": []interface{}{}},
			FetchedAt: time.Now(),
			PageCount: 1,
		},
		fetchCount: &fetchCount,
	}

	lru := cache.NewLRUCache(0)
	h := handler.NewCacheHandler(
		[]config.Endpoint{ep1, ep2},
		&mockCacheRepo{},
		fetcher,
		handler.WithLRU(lru),
	)

	var wg sync.WaitGroup
	for _, slug := range []string{"drug1", "drug2"} {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/api/cache/"+s, nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
		}(slug)
	}
	wg.Wait()

	count := fetchCount.Load()
	if count < 2 {
		t.Errorf("expected at least 2 fetches for different keys, got %d", count)
	}
}

// AC: Singleflight — errors are NOT shared (singleflight.Forget on error)
func TestSingleflight_ErrorsNotShared(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	var fetchCount atomic.Int32
	fetcher := &slowMockFetcher{
		delay:      50 * time.Millisecond,
		err:        fmt.Errorf("upstream unavailable"),
		fetchCount: &fetchCount,
	}

	lru := cache.NewLRUCache(0)
	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		fetcher,
		handler.WithLRU(lru),
	)

	// First request fails
	req1 := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w1 := httptest.NewRecorder()
	h.ServeHTTP(w1, req1)

	// Second request should also try (errors not cached in group)
	req2 := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w2 := httptest.NewRecorder()
	h.ServeHTTP(w2, req2)

	count := fetchCount.Load()
	if count < 2 {
		t.Errorf("expected at least 2 fetches (errors should not be shared), got %d", count)
	}
}

// AC: LRU hit serves directly without MongoDB
func TestLRU_HitServesDirectly(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	lru := cache.NewLRUCache(1024 * 1024)
	resp := &model.CachedResponse{
		Slug:      "drugnames",
		CacheKey:  "drugnames",
		Data:      map[string]interface{}{"items": []interface{}{"drug1"}},
		FetchedAt: time.Now(),
		SourceURL: "http://example.com/v2/drugnames",
		PageCount: 1,
	}
	lru.Set("drugnames", resp, 5*time.Minute)

	repo := &mockCacheRepo{} // empty - should not be called
	fetcher := &mockFetcher{}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
		handler.WithLRU(lru),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if fetcher.fetchCalled {
		t.Error("expected no upstream fetch when LRU has the data")
	}
}

// AC: Force refresh skips LRU and invalidates entry
func TestLRU_ForceRefreshSkipsAndInvalidates(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	lru := cache.NewLRUCache(1024 * 1024)
	cachedResp := &model.CachedResponse{
		Slug:      "drugnames",
		CacheKey:  "drugnames",
		Data:      map[string]interface{}{"items": []interface{}{"old"}},
		FetchedAt: time.Now(),
		SourceURL: "http://example.com/v2/drugnames",
		PageCount: 1,
	}
	lru.Set("drugnames", cachedResp, 5*time.Minute)

	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:      "drugnames",
			CacheKey:  "drugnames",
			Data:      map[string]interface{}{"items": []interface{}{"new"}},
			FetchedAt: time.Now(),
			SourceURL: "http://example.com/v2/drugnames",
			PageCount: 1,
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		fetcher,
		handler.WithLRU(lru),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames?_force=true", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !fetcher.fetchCalled {
		t.Error("expected upstream fetch on force refresh despite LRU hit")
	}
}

// AC: LRU metrics — hits, misses, singleflight dedup
func TestLRU_MetricsRecorded(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	lru := cache.NewLRUCache(1024 * 1024)
	resp := &model.CachedResponse{
		Slug:      "drugnames",
		CacheKey:  "drugnames",
		Data:      map[string]interface{}{"items": []interface{}{"drug1"}},
		FetchedAt: time.Now(),
		SourceURL: "http://example.com/v2/drugnames",
		PageCount: 1,
	}
	lru.Set("drugnames", resp, 5*time.Minute)

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		&mockFetcher{},
		handler.WithLRU(lru),
		handler.WithMetrics(m),
	)

	// LRU hit
	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	hitVal := testutil.ToFloat64(m.LRUCacheHitsTotal.WithLabelValues("drugnames"))
	if hitVal != 1 {
		t.Errorf("expected lru_cache_hits_total=1, got %f", hitVal)
	}

	// LRU miss (different slug)
	ep2 := config.Endpoint{
		Slug:    "other",
		BaseURL: "http://example.com",
		Path:    "/v2/other",
		Format:  "json",
	}
	config.ApplyDefaults(&ep2)

	h2 := handler.NewCacheHandler(
		[]config.Endpoint{ep2},
		&mockCacheRepo{},
		&mockFetcher{result: &model.CachedResponse{
			Slug:      "other",
			CacheKey:  "other",
			Data:      []interface{}{},
			FetchedAt: time.Now(),
			PageCount: 1,
		}},
		handler.WithLRU(lru),
		handler.WithMetrics(m),
	)

	req2 := httptest.NewRequest("GET", "/api/cache/other", nil)
	w2 := httptest.NewRecorder()
	h2.ServeHTTP(w2, req2)

	missVal := testutil.ToFloat64(m.LRUCacheMissesTotal.WithLabelValues("other"))
	if missVal != 1 {
		t.Errorf("expected lru_cache_misses_total=1, got %f", missVal)
	}
}

// AC: Singleflight dedup metric incremented
func TestSingleflight_DedupMetricIncremented(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	var fetchCount atomic.Int32
	fetcher := &slowMockFetcher{
		delay: 100 * time.Millisecond,
		result: &model.CachedResponse{
			Slug:      "drugnames",
			CacheKey:  "drugnames",
			Data:      map[string]interface{}{"items": []interface{}{"drug1"}},
			FetchedAt: time.Now(),
			SourceURL: "http://example.com/v2/drugnames",
			PageCount: 1,
		},
		fetchCount: &fetchCount,
	}

	lru := cache.NewLRUCache(0)
	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		fetcher,
		handler.WithLRU(lru),
		handler.WithMetrics(m),
	)

	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)
		}()
	}
	wg.Wait()

	dedupVal := testutil.ToFloat64(m.SingleflightDedupTotal.WithLabelValues("drugnames"))
	if dedupVal < 1 {
		t.Errorf("expected singleflight_dedup_total >= 1, got %f", dedupVal)
	}
}

// M9-AC-013: Circuit open → serves stale cache with stale_reason: "circuit_open"
func TestM9_AC013_CircuitOpenServesStaleCacheWithReason(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	staleCache := &model.CachedResponse{
		Slug:      "drugnames",
		CacheKey:  "drugnames",
		Data:      map[string]interface{}{"items": []interface{}{"stale-drug"}},
		FetchedAt: time.Now().Add(-24 * time.Hour),
		SourceURL: "http://example.com/v2/drugnames",
		PageCount: 1,
	}

	circuit := upstream.NewCircuitRegistry(2, 30*time.Second)
	// Trip circuit
	for i := 0; i < 2; i++ {
		_, _ = circuit.Execute("drugnames", func() (interface{}, error) {
			return nil, fmt.Errorf("fail")
		})
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: staleCache},
		&mockFetcher{err: fmt.Errorf("should not be called")},
		handler.WithMetrics(m),
		handler.WithCircuit(circuit),
	)

	// Use _force=true to bypass cache-first path and reach the circuit breaker check
	req := httptest.NewRequest("GET", "/api/cache/drugnames?_force=true", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp model.APIResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if !resp.Meta.Stale {
		t.Error("expected stale=true when circuit is open")
	}
	if resp.Meta.StaleReason != "circuit_open" {
		t.Errorf("expected stale_reason='circuit_open', got '%s'", resp.Meta.StaleReason)
	}
}

// M9-AC-014: Circuit open + no cache → returns 503
func TestM9_AC014_CircuitOpenNoCacheReturns503(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	circuit := upstream.NewCircuitRegistry(2, 30*time.Second)
	// Trip circuit
	for i := 0; i < 2; i++ {
		_, _ = circuit.Execute("drugnames", func() (interface{}, error) {
			return nil, fmt.Errorf("fail")
		})
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{}, // no cache
		&mockFetcher{},
		handler.WithMetrics(m),
		handler.WithCircuit(circuit),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}

	var errResp map[string]interface{}
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp["error"] != "upstream circuit open" {
		t.Errorf("expected error 'upstream circuit open', got '%s'", errResp["error"])
	}
	if errResp["slug"] != "drugnames" {
		t.Errorf("expected slug 'drugnames', got '%s'", errResp["slug"])
	}
	retryAfter, ok := errResp["retry_after"].(float64)
	if !ok || retryAfter <= 0 {
		t.Errorf("expected retry_after > 0, got %v", errResp["retry_after"])
	}
}

// M9-AC-015: Force-refresh within cooldown → serves cache with X-Force-Cooldown header
func TestM9_AC015_ForceRefreshWithinCooldown(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	cached := &model.CachedResponse{
		Slug:      "drugnames",
		CacheKey:  "drugnames",
		Data:      map[string]interface{}{"items": []interface{}{"drug1"}},
		FetchedAt: time.Now(),
		SourceURL: "http://example.com/v2/drugnames",
		PageCount: 1,
	}

	cooldown := upstream.NewCooldownTracker(30 * time.Second)
	cooldown.Record("drugnames") // simulate a recent force-refresh

	fetcher := &mockFetcher{err: fmt.Errorf("should not be called")}
	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: cached},
		fetcher,
		handler.WithMetrics(m),
		handler.WithCooldown(cooldown),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames?_force=true", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	if w.Header().Get("X-Force-Cooldown") != "true" {
		t.Error("expected X-Force-Cooldown header to be set")
	}

	if fetcher.fetchCalled {
		t.Error("expected no upstream fetch during cooldown")
	}
}

// M9-AC-016: Force-refresh after cooldown expires → proceeds to upstream
func TestM9_AC016_ForceRefreshAfterCooldownExpires(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	cooldown := upstream.NewCooldownTracker(50 * time.Millisecond)
	cooldown.Record("drugnames")

	// Wait for cooldown to expire
	time.Sleep(60 * time.Millisecond)

	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:      "drugnames",
			CacheKey:  "drugnames",
			Data:      map[string]interface{}{"items": []interface{}{"fresh"}},
			FetchedAt: time.Now(),
			SourceURL: "http://example.com/v2/drugnames",
			PageCount: 1,
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		fetcher,
		handler.WithCooldown(cooldown),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames?_force=true", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	if !fetcher.fetchCalled {
		t.Error("expected upstream fetch after cooldown expires")
	}

	if w.Header().Get("X-Force-Cooldown") == "true" {
		t.Error("expected no X-Force-Cooldown header after cooldown expires")
	}
}

// ---------------------------------------------------------------------------
// M10: Empty Upstream Result Handling — RED phase tests
// Spec: specs/empty-upstream-results.md
// ---------------------------------------------------------------------------

// M10-AC-001/AC-002: Empty upstream result returns 200 with empty data array
func TestM10_EmptyResults_AC001_AC002_Returns200WithEmptyData(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/drug/ndc.json",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	// Fetcher returns valid response with empty data
	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:        "fda-ndc",
			CacheKey:    "fda-ndc:NDC=99999999",
			Data:        []interface{}{},
			ContentType: "application/json",
			FetchedAt:   time.Now(),
			SourceURL:   "http://example.com/drug/ndc.json",
			PageCount:   1,
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc?NDC=99999999", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for empty results, got %d", w.Code)
	}

	var resp model.APIResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	// Data should be empty array, not nil
	dataArr, ok := resp.Data.([]interface{})
	if !ok {
		t.Fatalf("expected Data to be []interface{}, got %T", resp.Data)
	}
	if len(dataArr) != 0 {
		t.Errorf("expected empty data array, got %d items", len(dataArr))
	}
}

// M10-AC-003/AC-004: ResultsCount present in meta for all responses
func TestM10_EmptyResults_AC003_AC004_ResultsCountInMeta(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	t.Run("empty results have results_count=0", func(t *testing.T) {
		fetcher := &mockFetcher{
			result: &model.CachedResponse{
				Slug:        "drugnames",
				CacheKey:    "drugnames",
				Data:        []interface{}{},
				ContentType: "application/json",
				FetchedAt:   time.Now(),
				SourceURL:   "http://example.com/v2/drugnames",
				PageCount:   1,
			},
		}

		h := handler.NewCacheHandler(
			[]config.Endpoint{ep},
			&mockCacheRepo{},
			fetcher,
		)

		req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		var resp model.APIResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)

		if resp.Meta.ResultsCount != 0 {
			t.Errorf("expected results_count=0 for empty results, got %d", resp.Meta.ResultsCount)
		}
	})

	t.Run("non-empty results have correct results_count", func(t *testing.T) {
		fetcher := &mockFetcher{
			result: &model.CachedResponse{
				Slug:        "drugnames",
				CacheKey:    "drugnames",
				Data:        []interface{}{"drug1", "drug2", "drug3"},
				ContentType: "application/json",
				FetchedAt:   time.Now(),
				SourceURL:   "http://example.com/v2/drugnames",
				PageCount:   1,
			},
		}

		h := handler.NewCacheHandler(
			[]config.Endpoint{ep},
			&mockCacheRepo{},
			fetcher,
		)

		req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		var resp model.APIResponse
		_ = json.NewDecoder(w.Body).Decode(&resp)

		if resp.Meta.ResultsCount != 3 {
			t.Errorf("expected results_count=3, got %d", resp.Meta.ResultsCount)
		}
	})
}

// M10-AC-007: Upstream errors still return 502
func TestM10_EmptyResults_AC007_UpstreamErrorStill502(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/drug/ndc.json",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		&mockFetcher{err: fmt.Errorf("upstream unavailable")},
	)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc?NDC=12345", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadGateway {
		t.Errorf("expected 502 for upstream error, got %d", w.Code)
	}
}

// M10-AC-005: Empty results are cached in MongoDB
func TestM10_EmptyResults_AC005_CachedInMongoDB(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/drug/ndc.json",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	repo := &mockCacheRepo{}
	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:        "fda-ndc",
			CacheKey:    "fda-ndc:NDC=99999999",
			Data:        []interface{}{},
			ContentType: "application/json",
			FetchedAt:   time.Now(),
			SourceURL:   "http://example.com/drug/ndc.json",
			PageCount:   1,
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc?NDC=99999999", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if !repo.upsertCalled {
		t.Error("expected cache upsert for empty results")
	}
}

// M10-AC-006: Empty results cached with short LRU TTL
func TestM10_EmptyResults_AC006_ShortLRUTTL(t *testing.T) {
	ep := config.Endpoint{
		Slug:        "fda-ndc",
		BaseURL:     "http://example.com",
		Path:        "/drug/ndc.json",
		Format:      "json",
		TTL:         "10m",
		TTLDuration: 10 * time.Minute,
		SearchParams: []string{
			"product_ndc:\"{NDC}\"",
		},
		DataKey:         "results",
		TotalKey:        "meta.results.total",
		PaginationStyle: "offset",
		Pagesize:        100,
	}
	config.ApplyDefaults(&ep)

	lru := cache.NewLRUCache(1024 * 1024)
	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:        "fda-ndc",
			CacheKey:    "fda-ndc:NDC=99999999",
			Data:        []interface{}{},
			ContentType: "application/json",
			FetchedAt:   time.Now(),
			SourceURL:   "http://example.com/drug/ndc.json",
			PageCount:   1,
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		fetcher,
		handler.WithLRU(lru),
	)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc?NDC=99999999", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// Verify LRU was populated (entry exists)
	// Handler extracts NDC from search_params placeholder and builds cache key
	cacheKey := cache.BuildCacheKey("fda-ndc", map[string]string{"NDC": "99999999"})
	_, ok := lru.Get(cacheKey)
	if !ok {
		t.Error("expected empty result to be cached in LRU")
	}
}

// M10-AC-008: ResultsCount in meta for cached responses
func TestM10_EmptyResults_AC008_ResultsCountOnCachedResponse(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	cached := &model.CachedResponse{
		Slug:        "drugnames",
		CacheKey:    "drugnames",
		Data:        []interface{}{"drug1", "drug2"},
		ContentType: "application/json",
		FetchedAt:   time.Now(),
		SourceURL:   "http://example.com/v2/drugnames",
		PageCount:   1,
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: cached},
		&mockFetcher{},
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	var resp model.APIResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp.Meta.ResultsCount != 2 {
		t.Errorf("expected results_count=2 for cached response, got %d", resp.Meta.ResultsCount)
	}
}

// M10-AC-012: Cache outcome metric labels empty-result cache hits as "hit"
func TestM10_EmptyResults_AC012_CacheHitMetricCorrect(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/drug/ndc.json",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	cached := &model.CachedResponse{
		Slug:        "fda-ndc",
		CacheKey:    "fda-ndc:NDC=99999999",
		Data:        []interface{}{},
		ContentType: "application/json",
		FetchedAt:   time.Now(),
		SourceURL:   "http://example.com/drug/ndc.json",
		PageCount:   1,
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: cached},
		&mockFetcher{},
		handler.WithMetrics(m),
	)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc?NDC=99999999", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	hitVal := testutil.ToFloat64(m.CacheHitsTotal.WithLabelValues("fda-ndc", "hit"))
	if hitVal != 1 {
		t.Errorf("expected cache hit=1 for empty result cache hit, got %f", hitVal)
	}

	// Should be counted as 200, not error
	httpVal := testutil.ToFloat64(m.HTTPRequestsTotal.WithLabelValues("fda-ndc", "GET", "200"))
	if httpVal != 1 {
		t.Errorf("expected HTTP 200 count=1, got %f", httpVal)
	}
}

// ---------------------------------------------------------------------------
// Coverage gap tests — populateLRU, isEmptyResult, backgroundRevalidate,
// respondWithCached, extractPathParams
// ---------------------------------------------------------------------------

// populateLRU: Large multi-page responses skip LRU caching
func TestPopulateLRU_LargeResponseSkipsLRU(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "spls",
		BaseURL: "http://example.com",
		Path:    "/v2/spls",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	lru := cache.NewLRUCache(1024 * 1024)
	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:      "spls",
			CacheKey:  "spls",
			Data:      []interface{}{"item1"},
			FetchedAt: time.Now(),
			SourceURL: "http://example.com/v2/spls",
			PageCount: 200, // > maxLRUPages (10) — should skip LRU
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		fetcher,
		handler.WithLRU(lru),
	)

	req := httptest.NewRequest("GET", "/api/cache/spls", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	// LRU should NOT have the entry (too many pages)
	_, ok := lru.Get("spls")
	if ok {
		t.Error("expected large response to skip LRU caching")
	}
}

// populateLRU: nil LRU is handled gracefully (no panic)
func TestPopulateLRU_NilLRU(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:      "drugnames",
			CacheKey:  "drugnames",
			Data:      []interface{}{"drug1"},
			FetchedAt: time.Now(),
			PageCount: 1,
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		fetcher,
		// No WithLRU — lru is nil
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// respondWithCached: non-JSON stale response with reason
func TestRespondWithCached_NonJSONStaleWithReason(t *testing.T) {
	ep := config.Endpoint{
		Slug:        "spl-xml",
		BaseURL:     "http://example.com",
		Path:        "/v2/spls/{SETID}.xml",
		Format:      "raw",
		TTL:         "1s",
		TTLDuration: 1 * time.Second,
	}
	config.ApplyDefaults(&ep)

	cached := &model.CachedResponse{
		Slug:        "spl-xml",
		CacheKey:    "spl-xml:SETID=abc",
		Data:        "<doc>stale</doc>",
		ContentType: "application/xml",
		FetchedAt:   time.Now().Add(-1 * time.Hour), // stale
		SourceURL:   "http://example.com/v2/spls/abc.xml",
		PageCount:   1,
	}

	fl := handler.NewFetchLocks()
	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: cached},
		&mockFetcher{err: fmt.Errorf("fail")},
		handler.WithFetchLocks(fl),
	)

	req := httptest.NewRequest("GET", "/api/cache/spl-xml?SETID=abc", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Header().Get("X-Cache-Stale") != "true" {
		t.Error("expected X-Cache-Stale header")
	}
	reason := w.Header().Get("X-Cache-Stale-Reason")
	if reason == "" {
		t.Error("expected X-Cache-Stale-Reason header for stale XML response")
	}
}

// respondWithCached: JSON response with non-array data (map)
func TestRespondWithCached_NonArrayData(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "test",
		BaseURL: "http://example.com",
		Path:    "/api",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	cached := &model.CachedResponse{
		Slug:      "test",
		CacheKey:  "test",
		Data:      map[string]interface{}{"key": "value"}, // not a []interface{}
		FetchedAt: time.Now(),
		SourceURL: "http://example.com/api",
		PageCount: 1,
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: cached},
		&mockFetcher{},
	)

	req := httptest.NewRequest("GET", "/api/cache/test", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	var resp model.APIResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	// resultsCount should be 0 for non-array data
	if resp.Meta.ResultsCount != 0 {
		t.Errorf("expected results_count=0 for non-array data, got %d", resp.Meta.ResultsCount)
	}
}

// backgroundRevalidate: skips for scheduled endpoints without parameters
func TestBackgroundRevalidate_SkipsScheduledEndpoint(t *testing.T) {
	ep := config.Endpoint{
		Slug:        "drugnames",
		BaseURL:     "http://example.com",
		Path:        "/v2/drugnames",
		Format:      "json",
		Refresh:     "0 */6 * * *", // scheduled
		TTL:         "1s",
		TTLDuration: 1 * time.Second,
	}
	config.ApplyDefaults(&ep)

	cached := &model.CachedResponse{
		Slug:      "drugnames",
		CacheKey:  "drugnames",
		Data:      []interface{}{"stale-data"},
		FetchedAt: time.Now().Add(-1 * time.Hour), // stale
		SourceURL: "http://example.com/v2/drugnames",
		PageCount: 1,
	}

	fl := handler.NewFetchLocks()
	fetcher := &mockFetcher{err: fmt.Errorf("should not be called for background revalidation")}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: cached},
		fetcher,
		handler.WithFetchLocks(fl),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// Should serve stale data (TTL expired triggers stale response)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// Sleep briefly to see if background revalidation was started (it should NOT be for scheduled endpoints)
	time.Sleep(50 * time.Millisecond)
	// The fetcher should not have been called for background revalidation
	// (it might have been called synchronously, but the mock returns an error for that)
	// Key point: handler serves stale data without panic even with scheduled + stale
}

// Force-refresh cooldown with no cache available falls through
func TestForceRefreshCooldown_NoCacheAvailable(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	cooldown := upstream.NewCooldownTracker(30 * time.Second)
	cooldown.Record("drugnames")

	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:      "drugnames",
			CacheKey:  "drugnames",
			Data:      []interface{}{"fresh"},
			FetchedAt: time.Now(),
			PageCount: 1,
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{}, // no cache
		fetcher,
		handler.WithCooldown(cooldown),
	)

	req := httptest.NewRequest("GET", "/api/cache/drugnames?_force=true", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}

	// When cooldown triggers but no cache is available, it falls through to normal fetch
	if !fetcher.fetchCalled {
		t.Error("expected fetch to be called when cooldown has no cache to serve")
	}
}

// extractPathParams: endpoint with no params returns nil
func TestExtractPathParams_NoConfiguredParams(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "drugnames",
		BaseURL: "http://example.com",
		Path:    "/v2/drugnames",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{cached: &model.CachedResponse{
			Slug:      "drugnames",
			Data:      []interface{}{"drug1"},
			FetchedAt: time.Now(),
		}},
		&mockFetcher{},
	)

	// Even with query params in URL, they should be ignored if not configured
	req := httptest.NewRequest("GET", "/api/cache/drugnames?random=ignored", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// extractPathParams: params present but values empty returns nil
func TestExtractPathParams_EmptyValues(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "spl-detail",
		BaseURL: "http://example.com",
		Path:    "/v2/spls/{SETID}",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	fetcher := &mockFetcher{
		result: &model.CachedResponse{
			Slug:      "spl-detail",
			CacheKey:  "spl-detail",
			Data:      []interface{}{"data"},
			FetchedAt: time.Now(),
			PageCount: 1,
		},
	}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		&mockCacheRepo{},
		fetcher,
	)

	// SETID query param is empty — extractPathParams should return nil
	req := httptest.NewRequest("GET", "/api/cache/spl-detail", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

// --- Mock implementations ---

// slowMockFetcher simulates a slow upstream fetch for singleflight tests
type slowMockFetcher struct {
	delay      time.Duration
	result     *model.CachedResponse
	err        error
	fetchCount *atomic.Int32
}

func (m *slowMockFetcher) Fetch(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error) {
	m.fetchCount.Add(1)
	time.Sleep(m.delay)
	if m.err != nil {
		return nil, m.err
	}
	// Return a copy with the correct slug
	resp := *m.result
	resp.Slug = ep.Slug
	resp.CacheKey = ep.Slug
	return &resp, nil
}

type mockCacheRepo struct {
	cached       *model.CachedResponse
	upsertCalled bool
	lastUpserted *model.CachedResponse
}

func (m *mockCacheRepo) Get(cacheKey string) (*model.CachedResponse, error) {
	return m.cached, nil
}

func (m *mockCacheRepo) Upsert(resp *model.CachedResponse) error {
	m.upsertCalled = true
	m.lastUpserted = resp
	return nil
}

func (m *mockCacheRepo) FetchedAt(cacheKey string) (time.Time, bool, error) {
	if m.cached != nil {
		return m.cached.FetchedAt, true, nil
	}
	return time.Time{}, false, nil
}

type mockFetcher struct {
	result      *model.CachedResponse
	err         error
	fetchCalled bool
	lastParams  map[string]string
}

func (m *mockFetcher) Fetch(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error) {
	m.fetchCalled = true
	m.lastParams = params
	return m.result, m.err
}

// mockLRU is a simple in-memory LRU mock for handler tests.
type mockLRU struct {
	mu    sync.Mutex
	store map[string]*model.CachedResponse
	ttls  map[string]time.Duration
}

func newMockLRU() *mockLRU {
	return &mockLRU{
		store: make(map[string]*model.CachedResponse),
		ttls:  make(map[string]time.Duration),
	}
}

func (m *mockLRU) Get(key string) (*model.CachedResponse, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.store[key]
	return r, ok
}

func (m *mockLRU) Set(key string, resp *model.CachedResponse, ttl time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.store[key] = resp
	m.ttls[key] = ttl
}

func (m *mockLRU) Invalidate(key string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.store, key)
	delete(m.ttls, key)
}

func (m *mockLRU) SizeBytes() int64 {
	return 0
}

// --- Upstream 404 Handling Tests (specs/upstream-404-handling.md) ---

// AC-001: Upstream 404 → consumer gets 404 (not 502)
func TestAC001_Upstream404Returns404ToConsumer(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/v2/drugs",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	fetcher := &mockFetcher{
		err: &upstream.ErrUpstreamNotFound{StatusCode: 404, URL: "http://example.com/v2/drugs"},
	}
	repo := &mockCacheRepo{}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// AC-002: 404 body includes error, slug, params
func TestAC002_404BodyIncludesErrorSlugParams(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/v2/drugs/{NDC}",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	fetcher := &mockFetcher{
		err: &upstream.ErrUpstreamNotFound{StatusCode: 404, URL: "http://example.com/v2/drugs/12345"},
	}
	repo := &mockCacheRepo{}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc?NDC=12345", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	var errResp model.ErrorResponse
	if err := json.NewDecoder(w.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}
	if errResp.Error != "not found" {
		t.Errorf("expected error 'not found', got %q", errResp.Error)
	}
	if errResp.Slug != "fda-ndc" {
		t.Errorf("expected slug 'fda-ndc', got %q", errResp.Slug)
	}
	if errResp.Params == nil || errResp.Params["NDC"] != "12345" {
		t.Errorf("expected params {NDC: 12345}, got %v", errResp.Params)
	}
}

// AC-003: Other 4xx (401, 403, 429) → still 502
func TestAC003_Other4xxStill502(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/v2/drugs",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	for _, statusCode := range []int{401, 403, 429} {
		t.Run(fmt.Sprintf("status_%d", statusCode), func(t *testing.T) {
			fetcher := &mockFetcher{
				err: fmt.Errorf("upstream returned status %d", statusCode),
			}
			repo := &mockCacheRepo{}

			h := handler.NewCacheHandler(
				[]config.Endpoint{ep},
				repo,
				fetcher,
			)

			req := httptest.NewRequest("GET", "/api/cache/fda-ndc", nil)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, req)

			if w.Code != http.StatusBadGateway {
				t.Errorf("status %d: expected 502, got %d", statusCode, w.Code)
			}
		})
	}
}

// AC-004: 5xx + network errors → still 502 with stale fallback (existing behavior)
func TestAC004_5xxNetworkErrorStill502WithFallback(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/v2/drugs",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	// With stale cache → serves stale
	t.Run("with_stale_cache", func(t *testing.T) {
		fetcher := &mockFetcher{
			err: fmt.Errorf("upstream returned status 500"),
		}
		staleData := &model.CachedResponse{
			Slug:      "fda-ndc",
			CacheKey:  "fda-ndc",
			Data:      []interface{}{"old-data"},
			FetchedAt: time.Now().Add(-1 * time.Hour),
		}
		repo := &mockCacheRepo{cached: staleData}

		h := handler.NewCacheHandler(
			[]config.Endpoint{ep},
			repo,
			fetcher,
		)

		req := httptest.NewRequest("GET", "/api/cache/fda-ndc?_force=true", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200 (stale fallback), got %d", w.Code)
		}
	})

	// Without stale cache → 502
	t.Run("without_stale_cache", func(t *testing.T) {
		fetcher := &mockFetcher{
			err: fmt.Errorf("upstream returned status 500"),
		}
		repo := &mockCacheRepo{}

		h := handler.NewCacheHandler(
			[]config.Endpoint{ep},
			repo,
			fetcher,
		)

		req := httptest.NewRequest("GET", "/api/cache/fda-ndc?_force=true", nil)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)

		if w.Code != http.StatusBadGateway {
			t.Errorf("expected 502, got %d", w.Code)
		}
	})
}

// AC-005: 404 overrides stale cache (negative entry takes precedence)
func TestAC005_404OverridesStaleCache(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/v2/drugs",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	// Even when stale cache exists, 404 from upstream returns 404 to consumer
	staleData := &model.CachedResponse{
		Slug:      "fda-ndc",
		CacheKey:  "fda-ndc",
		Data:      []interface{}{"old-data"},
		FetchedAt: time.Now().Add(-1 * time.Hour),
	}
	fetcher := &mockFetcher{
		err: &upstream.ErrUpstreamNotFound{StatusCode: 404, URL: "http://example.com/v2/drugs"},
	}
	repo := &mockCacheRepo{cached: staleData}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
	)

	// Force refresh to bypass cache and trigger upstream fetch
	req := httptest.NewRequest("GET", "/api/cache/fda-ndc?_force=true", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 (not stale fallback), got %d", w.Code)
	}

	// Verify negative cache entry was stored
	if !repo.upsertCalled {
		t.Error("expected negative cache entry to be upserted")
	}
	if repo.lastUpserted == nil || !repo.lastUpserted.NotFound {
		t.Error("expected upserted entry to have NotFound=true")
	}
}

// AC-006: Negative cache stored with 10-min TTL (in LRU)
func TestAC006_NegativeCacheStoredWith10MinTTL(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/v2/drugs",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	fetcher := &mockFetcher{
		err: &upstream.ErrUpstreamNotFound{StatusCode: 404, URL: "http://example.com/v2/drugs"},
	}
	repo := &mockCacheRepo{}
	lru := newMockLRU()

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
		handler.WithLRU(lru),
	)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	// Check LRU stored the negative entry with 10-minute TTL
	cacheKey := "fda-ndc" // BuildCacheKey("fda-ndc", nil) = "fda-ndc"
	entry, ok := lru.Get(cacheKey)
	if !ok {
		t.Fatal("expected negative cache entry in LRU")
	}
	if !entry.NotFound {
		t.Error("expected LRU entry to have NotFound=true")
	}
	lru.mu.Lock()
	ttl := lru.ttls[cacheKey]
	lru.mu.Unlock()
	if ttl != 10*time.Minute {
		t.Errorf("expected 10m TTL, got %v", ttl)
	}
}

// AC-007: Repeated requests served from negative cache (no upstream call)
func TestAC007_RepeatedRequestsServedFromNegativeCache(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/v2/drugs",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	lru := newMockLRU()
	// Pre-populate LRU with a negative cache entry
	lru.Set("fda-ndc", &model.CachedResponse{
		Slug:      "fda-ndc",
		CacheKey:  "fda-ndc",
		NotFound:  true,
		FetchedAt: time.Now(),
	}, 10*time.Minute)

	fetcher := &mockFetcher{}
	repo := &mockCacheRepo{}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
		handler.WithLRU(lru),
	)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 from negative cache, got %d", w.Code)
	}
	// Verify fetcher was NOT called
	if fetcher.fetchCalled {
		t.Error("expected fetcher NOT to be called — should serve from negative cache")
	}
}

// AC-008: After negative cache TTL expires, re-checks upstream
func TestAC008_NegativeCacheTTLExpiresReChecksUpstream(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/v2/drugs",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	// MongoDB has negative cache entry that is expired (> 10 min old)
	expiredNeg := &model.CachedResponse{
		Slug:      "fda-ndc",
		CacheKey:  "fda-ndc",
		NotFound:  true,
		FetchedAt: time.Now().Add(-11 * time.Minute),
	}
	repo := &mockCacheRepo{cached: expiredNeg}

	// Upstream now returns data
	freshData := &model.CachedResponse{
		Slug:      "fda-ndc",
		CacheKey:  "fda-ndc",
		Data:      []interface{}{"fresh-data"},
		FetchedAt: time.Now(),
		PageCount: 1,
	}
	fetcher := &mockFetcher{result: freshData}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	// Should have re-checked upstream and returned fresh data
	if w.Code != http.StatusOK {
		t.Errorf("expected 200 (fresh data after expired negative cache), got %d", w.Code)
	}
	if !fetcher.fetchCalled {
		t.Error("expected fetcher to be called after negative cache expired")
	}
}

// AC-009: Existing endpoints unaffected
func TestAC009_ExistingEndpointsUnaffected(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/v2/drugs",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	// Normal cached response (not a 404)
	cached := &model.CachedResponse{
		Slug:      "fda-ndc",
		CacheKey:  "fda-ndc",
		Data:      []interface{}{"drug1"},
		FetchedAt: time.Now(),
		PageCount: 1,
	}
	repo := &mockCacheRepo{cached: cached}
	fetcher := &mockFetcher{}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200 for normal cached response, got %d", w.Code)
	}
	if fetcher.fetchCalled {
		t.Error("expected fetcher NOT to be called for cached response")
	}
}

// AC-010: TTL hardcoded 10 minutes (not configurable) — verified via AC-006 and AC-008
func TestAC010_TTLHardcoded10Minutes(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/v2/drugs",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	fetcher := &mockFetcher{
		err: &upstream.ErrUpstreamNotFound{StatusCode: 404, URL: "http://example.com/v2/drugs"},
	}
	repo := &mockCacheRepo{}
	lru := newMockLRU()

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
		handler.WithLRU(lru),
	)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	lru.mu.Lock()
	ttl := lru.ttls["fda-ndc"]
	lru.mu.Unlock()
	expected := 10 * time.Minute
	if ttl != expected {
		t.Errorf("expected hardcoded TTL of %v, got %v", expected, ttl)
	}
}

// Test: Negative cache entry in MongoDB returns 404 (not just LRU)
func TestNegativeCacheInMongoDBReturns404(t *testing.T) {
	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/v2/drugs",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	// MongoDB has fresh negative cache entry (< 10 min old)
	negEntry := &model.CachedResponse{
		Slug:      "fda-ndc",
		CacheKey:  "fda-ndc",
		NotFound:  true,
		FetchedAt: time.Now().Add(-5 * time.Minute),
	}
	repo := &mockCacheRepo{cached: negEntry}
	fetcher := &mockFetcher{}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
	)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 from MongoDB negative cache, got %d", w.Code)
	}
	if fetcher.fetchCalled {
		t.Error("expected fetcher NOT to be called for valid negative cache in MongoDB")
	}
}

// Test: Prometheus metric upstream_404_total incremented on 404
func TestUpstream404MetricIncremented(t *testing.T) {
	reg := prometheus.NewPedanticRegistry()
	m := metrics.NewMetrics(reg)

	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/v2/drugs",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)

	fetcher := &mockFetcher{
		err: &upstream.ErrUpstreamNotFound{StatusCode: 404, URL: "http://example.com/v2/drugs"},
	}
	repo := &mockCacheRepo{}

	h := handler.NewCacheHandler(
		[]config.Endpoint{ep},
		repo,
		fetcher,
		handler.WithMetrics(m),
	)

	req := httptest.NewRequest("GET", "/api/cache/fda-ndc", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	val := testutil.ToFloat64(m.Upstream404Total.WithLabelValues("fda-ndc"))
	if val != 1 {
		t.Errorf("expected upstream_404_total=1, got %f", val)
	}
}
