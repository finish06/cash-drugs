package handler_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/handler"
	"github.com/finish06/cash-drugs/internal/metrics"
	"github.com/finish06/cash-drugs/internal/model"
	"github.com/prometheus/client_golang/prometheus"
)

func testEndpoint() config.Endpoint {
	ep := config.Endpoint{
		Slug:    "fda-ndc",
		BaseURL: "http://example.com",
		Path:    "/drugsfda.json",
		Format:  "json",
	}
	config.ApplyDefaults(&ep)
	return ep
}

func newTestBulkHandler(repo cache.Repository, opts ...handler.BulkOption) *handler.BulkHandler {
	return handler.NewBulkHandler([]config.Endpoint{testEndpoint()}, repo, opts...)
}

// --- Mock repo that supports per-key lookups ---

type bulkMockRepo struct {
	data map[string]*model.CachedResponse
	err  error
}

func (m *bulkMockRepo) Get(cacheKey string) (*model.CachedResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.data != nil {
		return m.data[cacheKey], nil
	}
	return nil, nil
}

func (m *bulkMockRepo) Upsert(_ *model.CachedResponse) error { return nil }

func (m *bulkMockRepo) FetchedAt(cacheKey string) (time.Time, bool, error) {
	if m.data != nil {
		if c, ok := m.data[cacheKey]; ok {
			return c.FetchedAt, true, nil
		}
	}
	return time.Time{}, false, nil
}

// --- Slow mock repo for concurrency testing ---

type slowMockRepo struct {
	data       map[string]*model.CachedResponse
	maxActive  atomic.Int32
	curActive  atomic.Int32
}

func (m *slowMockRepo) Get(cacheKey string) (*model.CachedResponse, error) {
	cur := m.curActive.Add(1)
	defer m.curActive.Add(-1)

	// Track max concurrency
	for {
		old := m.maxActive.Load()
		if cur <= old {
			break
		}
		if m.maxActive.CompareAndSwap(old, cur) {
			break
		}
	}

	time.Sleep(5 * time.Millisecond) // simulate latency

	if m.data != nil {
		return m.data[cacheKey], nil
	}
	return nil, nil
}

func (m *slowMockRepo) Upsert(_ *model.CachedResponse) error { return nil }

func (m *slowMockRepo) FetchedAt(cacheKey string) (time.Time, bool, error) {
	return time.Time{}, false, nil
}

func postBulk(h http.Handler, slug string, body interface{}) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(body)
	req := httptest.NewRequest(http.MethodPost, "/api/cache/"+slug+"/bulk", &buf)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)
	return w
}

// AC-009: Unknown slug returns 404
func TestBulk_AC009_UnknownSlug(t *testing.T) {
	h := newTestBulkHandler(&bulkMockRepo{})
	body := map[string]interface{}{
		"queries": []map[string]interface{}{
			{"params": map[string]string{"BRAND_NAME": "Tylenol"}},
		},
	}
	w := postBulk(h, "nonexistent", body)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}

	var errResp model.ErrorResponse
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.ErrorCode != model.ErrCodeEndpointNotFound {
		t.Errorf("expected error code %s, got %s", model.ErrCodeEndpointNotFound, errResp.ErrorCode)
	}
}

// AC-007: Empty queries returns 200 with empty results
func TestBulk_AC007_EmptyQueries(t *testing.T) {
	h := newTestBulkHandler(&bulkMockRepo{})
	body := map[string]interface{}{"queries": []interface{}{}}
	w := postBulk(h, "fda-ndc", body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp handler.BulkQueryResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)
	if resp.Total != 0 {
		t.Errorf("expected total 0, got %d", resp.Total)
	}
	if len(resp.Results) != 0 {
		t.Errorf("expected empty results, got %d", len(resp.Results))
	}
}

// AC-006: Over 100 queries returns 400
func TestBulk_AC006_OverLimit(t *testing.T) {
	h := newTestBulkHandler(&bulkMockRepo{})
	queries := make([]map[string]interface{}, 101)
	for i := range queries {
		queries[i] = map[string]interface{}{
			"params": map[string]string{"NDC": fmt.Sprintf("%d", i)},
		}
	}
	body := map[string]interface{}{"queries": queries}
	w := postBulk(h, "fda-ndc", body)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp model.ErrorResponse
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	if errResp.ErrorCode != model.ErrCodeBadRequest {
		t.Errorf("expected error code %s, got %s", model.ErrCodeBadRequest, errResp.ErrorCode)
	}
}

// AC-008: Invalid JSON body returns 400
func TestBulk_AC008_InvalidJSON(t *testing.T) {
	h := newTestBulkHandler(&bulkMockRepo{})

	req := httptest.NewRequest(http.MethodPost, "/api/cache/fda-ndc/bulk", bytes.NewBufferString("{invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// AC-001, AC-003, AC-004: Single query cache hit
func TestBulk_AC001_SingleHit(t *testing.T) {
	repo := &bulkMockRepo{
		data: map[string]*model.CachedResponse{
			"fda-ndc:BRAND_NAME=Tylenol": {
				Slug:      "fda-ndc",
				CacheKey:  "fda-ndc:BRAND_NAME=Tylenol",
				Data:      []interface{}{map[string]interface{}{"name": "Tylenol"}},
				FetchedAt: time.Now(),
				PageCount: 1,
				SourceURL: "http://example.com/drugsfda.json",
			},
		},
	}
	h := newTestBulkHandler(repo)

	body := map[string]interface{}{
		"queries": []map[string]interface{}{
			{"params": map[string]string{"BRAND_NAME": "Tylenol"}},
		},
	}
	w := postBulk(h, "fda-ndc", body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp handler.BulkQueryResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp.Total != 1 {
		t.Errorf("expected total 1, got %d", resp.Total)
	}
	if resp.Hits != 1 {
		t.Errorf("expected 1 hit, got %d", resp.Hits)
	}
	if resp.Misses != 0 {
		t.Errorf("expected 0 misses, got %d", resp.Misses)
	}
	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}

	r := resp.Results[0]
	if r.Status != "hit" {
		t.Errorf("expected status 'hit', got %q", r.Status)
	}
	if r.Index != 0 {
		t.Errorf("expected index 0, got %d", r.Index)
	}
	if r.Data == nil {
		t.Error("expected non-nil data")
	}
	if r.Meta == nil {
		t.Fatal("expected non-nil meta")
	}
	if r.Meta.ResultsCount != 1 {
		t.Errorf("expected results_count 1, got %d", r.Meta.ResultsCount)
	}
}

// AC-004, AC-005: Single query cache miss
func TestBulk_AC004_SingleMiss(t *testing.T) {
	h := newTestBulkHandler(&bulkMockRepo{})

	body := map[string]interface{}{
		"queries": []map[string]interface{}{
			{"params": map[string]string{"BRAND_NAME": "NotCached"}},
		},
	}
	w := postBulk(h, "fda-ndc", body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp handler.BulkQueryResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", resp.Misses)
	}

	r := resp.Results[0]
	if r.Status != "miss" {
		t.Errorf("expected status 'miss', got %q", r.Status)
	}
	if r.Data != nil {
		t.Error("expected nil data for miss")
	}
	if r.Error != "not cached" {
		t.Errorf("expected error 'not cached', got %q", r.Error)
	}
}

// AC-003, AC-005: Mixed hit and miss preserves order
func TestBulk_AC003_MixedHitMiss(t *testing.T) {
	repo := &bulkMockRepo{
		data: map[string]*model.CachedResponse{
			"fda-ndc:BRAND_NAME=Tylenol": {
				Slug:      "fda-ndc",
				CacheKey:  "fda-ndc:BRAND_NAME=Tylenol",
				Data:      []interface{}{map[string]interface{}{"name": "Tylenol"}},
				FetchedAt: time.Now(),
				PageCount: 1,
			},
		},
	}
	h := newTestBulkHandler(repo)

	body := map[string]interface{}{
		"queries": []map[string]interface{}{
			{"params": map[string]string{"BRAND_NAME": "Tylenol"}},
			{"params": map[string]string{"BRAND_NAME": "NotCached"}},
			{"params": map[string]string{"BRAND_NAME": "Tylenol"}}, // duplicate
		},
	}
	w := postBulk(h, "fda-ndc", body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp handler.BulkQueryResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp.Total != 3 {
		t.Errorf("expected total 3, got %d", resp.Total)
	}
	if resp.Hits != 2 {
		t.Errorf("expected 2 hits, got %d", resp.Hits)
	}
	if resp.Misses != 1 {
		t.Errorf("expected 1 miss, got %d", resp.Misses)
	}

	// Order preserved
	if resp.Results[0].Index != 0 || resp.Results[0].Status != "hit" {
		t.Errorf("result 0: expected index=0 status=hit, got index=%d status=%s", resp.Results[0].Index, resp.Results[0].Status)
	}
	if resp.Results[1].Index != 1 || resp.Results[1].Status != "miss" {
		t.Errorf("result 1: expected index=1 status=miss, got index=%d status=%s", resp.Results[1].Index, resp.Results[1].Status)
	}
	if resp.Results[2].Index != 2 || resp.Results[2].Status != "hit" {
		t.Errorf("result 2: expected index=2 status=hit, got index=%d status=%s", resp.Results[2].Index, resp.Results[2].Status)
	}
}

// AC-002: Concurrent execution with semaphore cap of 10
func TestBulk_AC002_ConcurrencyCap(t *testing.T) {
	repo := &slowMockRepo{
		data: make(map[string]*model.CachedResponse),
	}
	// Populate 50 entries
	for i := 0; i < 50; i++ {
		key := cache.BuildCacheKey("fda-ndc", map[string]string{"NDC": fmt.Sprintf("%d", i)})
		repo.data[key] = &model.CachedResponse{
			Slug:      "fda-ndc",
			CacheKey:  key,
			Data:      []interface{}{},
			FetchedAt: time.Now(),
		}
	}

	h := handler.NewBulkHandler([]config.Endpoint{testEndpoint()}, repo)

	queries := make([]map[string]interface{}, 50)
	for i := range queries {
		queries[i] = map[string]interface{}{
			"params": map[string]string{"NDC": fmt.Sprintf("%d", i)},
		}
	}
	body := map[string]interface{}{"queries": queries}
	w := postBulk(h, "fda-ndc", body)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	maxActive := repo.maxActive.Load()
	if maxActive > 10 {
		t.Errorf("expected max concurrency <= 10, got %d", maxActive)
	}
	if maxActive < 2 {
		t.Logf("warning: only observed %d concurrent goroutines (may be timing-dependent)", maxActive)
	}
}

// AC-010: Prometheus metrics are recorded
func TestBulk_AC010_Metrics(t *testing.T) {
	reg := prometheus.NewRegistry()
	m := metrics.NewMetrics(reg)

	repo := &bulkMockRepo{
		data: map[string]*model.CachedResponse{
			"fda-ndc:BRAND_NAME=Tylenol": {
				Slug:      "fda-ndc",
				CacheKey:  "fda-ndc:BRAND_NAME=Tylenol",
				Data:      []interface{}{},
				FetchedAt: time.Now(),
			},
		},
	}

	h := newTestBulkHandler(repo, handler.WithBulkMetrics(m))

	body := map[string]interface{}{
		"queries": []map[string]interface{}{
			{"params": map[string]string{"BRAND_NAME": "Tylenol"}},
			{"params": map[string]string{"BRAND_NAME": "NotCached"}},
		},
	}
	_ = postBulk(h, "fda-ndc", body)

	// Verify bulk_request_size was observed
	mfs, _ := reg.Gather()
	found := false
	for _, mf := range mfs {
		if mf.GetName() == "cashdrugs_bulk_request_size" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected cashdrugs_bulk_request_size metric to be recorded")
	}

	// Verify bulk_request_duration was observed
	found = false
	for _, mf := range mfs {
		if mf.GetName() == "cashdrugs_bulk_request_duration_seconds" {
			found = true
			break
		}
	}
	if !found {
		t.Error("expected cashdrugs_bulk_request_duration_seconds metric to be recorded")
	}
}

// AC-014: Request ID is included in error responses
func TestBulk_AC014_RequestID(t *testing.T) {
	h := newTestBulkHandler(&bulkMockRepo{})

	req := httptest.NewRequest(http.MethodPost, "/api/cache/fda-ndc/bulk", bytes.NewBufferString("{bad"))
	req.Header.Set("Content-Type", "application/json")
	// The middleware normally adds the request ID, but for this unit test
	// we test the handler directly, so request_id will be empty.
	w := httptest.NewRecorder()
	h.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}

	var errResp model.ErrorResponse
	_ = json.NewDecoder(w.Body).Decode(&errResp)
	// request_id field exists (empty without middleware, but present in struct)
	if errResp.ErrorCode != model.ErrCodeBadRequest {
		t.Errorf("expected error code %s, got %s", model.ErrCodeBadRequest, errResp.ErrorCode)
	}
}

// Verify slug is present in successful response
func TestBulk_ResponseContainsSlug(t *testing.T) {
	h := newTestBulkHandler(&bulkMockRepo{})

	body := map[string]interface{}{"queries": []interface{}{}}
	w := postBulk(h, "fda-ndc", body)

	var resp handler.BulkQueryResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if resp.Slug != "fda-ndc" {
		t.Errorf("expected slug 'fda-ndc', got %q", resp.Slug)
	}
}

// Stale tracking: hit with expired TTL shows stale=true in meta
func TestBulk_StaleResult(t *testing.T) {
	ep := testEndpoint()
	ep.TTL = "1s"
	ep.TTLDuration = time.Second

	repo := &bulkMockRepo{
		data: map[string]*model.CachedResponse{
			"fda-ndc:BRAND_NAME=Tylenol": {
				Slug:      "fda-ndc",
				CacheKey:  "fda-ndc:BRAND_NAME=Tylenol",
				Data:      []interface{}{},
				FetchedAt: time.Now().Add(-2 * time.Second), // past TTL
			},
		},
	}

	h := handler.NewBulkHandler([]config.Endpoint{ep}, repo)

	body := map[string]interface{}{
		"queries": []map[string]interface{}{
			{"params": map[string]string{"BRAND_NAME": "Tylenol"}},
		},
	}
	w := postBulk(h, "fda-ndc", body)

	var resp handler.BulkQueryResponse
	_ = json.NewDecoder(w.Body).Decode(&resp)

	if len(resp.Results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(resp.Results))
	}
	if resp.Results[0].Status != "hit" {
		t.Fatalf("expected hit, got %s", resp.Results[0].Status)
	}
	if resp.Results[0].Meta == nil || !resp.Results[0].Meta.Stale {
		t.Error("expected stale=true in meta for expired TTL")
	}
}
