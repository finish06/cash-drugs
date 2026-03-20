package upstream

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/model"
)

// ErrUpstreamNotFound is returned when the upstream API responds with HTTP 404.
type ErrUpstreamNotFound struct {
	StatusCode int
	URL        string
}

func (e *ErrUpstreamNotFound) Error() string {
	return fmt.Sprintf("upstream returned 404 (not found) for %s", e.URL)
}

// Fetcher defines the interface for upstream API fetching.
type Fetcher interface {
	Fetch(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error)
}

// HTTPFetcher fetches from upstream APIs using net/http.
type HTTPFetcher struct {
	Client           *http.Client
	FetchConcurrency int // max concurrent page fetches (default: 3)
}

// NewHTTPFetcher creates a new HTTPFetcher with sensible defaults.
func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{
		Client:           &http.Client{Timeout: 30 * time.Second},
		FetchConcurrency: 3,
	}
}

// Fetch is a convenience function that creates a default HTTPFetcher and fetches.
func Fetch(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error) {
	return NewHTTPFetcher().Fetch(ep, params)
}

// Fetch retrieves data from an upstream API endpoint, handling pagination.
// Returns a CachedResponse with Pages populated for multi-page results.
func (f *HTTPFetcher) Fetch(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error) {
	if ep.Format == "xml" || ep.Format == "raw" {
		return f.fetchRaw(ep, params)
	}
	return f.fetchJSON(ep, params)
}

func (f *HTTPFetcher) fetchJSON(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error) {
	path := config.SubstitutePathParams(ep.Path, params)
	maxPages, fetchAll := config.ParsePagination(ep)
	isOffset := ep.PaginationStyle == "offset"

	// Step 1: Fetch first page sequentially to discover total pages
	firstURL := buildURL(ep, path, 1, params)
	firstData, firstParsed, err := f.fetchJSONPage(firstURL, ep.DataKey)
	if err != nil {
		return nil, err
	}

	allPages := []model.PageData{{Page: 1, Data: firstData}}
	pageCount := 1

	// Determine total pages from first page response
	totalPages := 1
	if fetchAll || maxPages > 1 {
		if hasMorePages(firstParsed, 1, ep.TotalKey, isOffset, ep.Pagesize) {
			totalPages = determineTotalPages(firstParsed, ep.TotalKey, isOffset, ep.Pagesize)
		}
	}

	// Apply maxPages cap
	if !fetchAll && totalPages > maxPages {
		totalPages = maxPages
	}

	// Step 2: If more than 1 page, fetch remaining pages concurrently
	if totalPages > 1 {
		remainingPages := totalPages - 1
		concurrency := f.FetchConcurrency
		if concurrency <= 0 {
			concurrency = 3
		}

		type pageResult struct {
			page int
			data []interface{}
			err  error
		}

		results := make([]pageResult, remainingPages)
		sem := make(chan struct{}, concurrency)
		var wg sync.WaitGroup

		for i := 0; i < remainingPages; i++ {
			pageNum := i + 2 // pages 2..N
			wg.Add(1)
			go func(idx, pNum int) {
				defer wg.Done()
				sem <- struct{}{}        // acquire
				defer func() { <-sem }() // release

				reqURL := buildURL(ep, path, pNum, params)
				data, _, fetchErr := f.fetchJSONPage(reqURL, ep.DataKey)
				results[idx] = pageResult{page: pNum, data: data, err: fetchErr}
			}(i, pageNum)
		}

		wg.Wait()

		// Collect results in page order
		for _, r := range results {
			if r.err != nil {
				if isOffset && pageCount > 0 {
					slog.Warn("offset pagination: page fetch failed, returning partial data",
						"slug", ep.Slug, "page", r.page, "pages_fetched", pageCount, "error", r.err)
					// Skip failed pages in offset mode, continue collecting successful ones
					continue
				}
				return nil, r.err
			}
			pageCount++
			allPages = append(allPages, model.PageData{
				Page: r.page,
				Data: r.data,
			})
		}
	}

	cacheKey := cache.BuildCacheKey(ep.Slug, params)
	sourceURL := ep.BaseURL + path
	now := time.Now()

	// Combine all page data into a single slice for the response.
	// Pre-allocate capacity to avoid repeated slice growth on large responses.
	totalItems := 0
	for _, p := range allPages {
		totalItems += len(p.Data)
	}
	allData := make([]interface{}, 0, totalItems)
	for _, p := range allPages {
		allData = append(allData, p.Data...)
	}

	// Apply flatten post-processing if configured (AC-002, AC-003, AC-006)
	if ep.Flatten {
		allData = flattenConceptGroups(allData)
	}

	return &model.CachedResponse{
		Slug:        ep.Slug,
		Params:      params,
		CacheKey:    cacheKey,
		Data:        allData,
		ContentType: "application/json",
		FetchedAt:   now,
		SourceURL:   sourceURL,
		HTTPStatus:  200,
		PageCount:   pageCount,
		Pages:       allPages,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

// determineTotalPages extracts the total page count from the parsed response.
func determineTotalPages(parsed map[string]interface{}, totalKey string, isOffset bool, pagesize int) int {
	val, ok := resolveByDotPath(parsed, totalKey)
	if !ok {
		return 1
	}
	total, ok := val.(float64)
	if !ok {
		return 1
	}
	if isOffset {
		// For offset pagination, total is item count — compute page count
		pages := int(total) / pagesize
		if int(total)%pagesize != 0 {
			pages++
		}
		return pages
	}
	return int(total)
}

func (f *HTTPFetcher) fetchRaw(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error) {
	path := config.SubstitutePathParams(ep.Path, params)
	reqURL := buildURL(ep, path, 1, params)

	resp, err := f.Client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 404 {
		return nil, &ErrUpstreamNotFound{StatusCode: 404, URL: reqURL}
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read upstream response: %w", err)
	}

	// Use upstream's content type, fall back to octet-stream
	contentType := resp.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	cacheKey := cache.BuildCacheKey(ep.Slug, params)
	sourceURL := ep.BaseURL + path
	now := time.Now()

	return &model.CachedResponse{
		Slug:        ep.Slug,
		Params:      params,
		CacheKey:    cacheKey,
		Data:        string(body),
		ContentType: contentType,
		FetchedAt:   now,
		SourceURL:   sourceURL,
		HTTPStatus:  200,
		PageCount:   1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (f *HTTPFetcher) fetchJSONPage(reqURL string, dataKey string) ([]interface{}, map[string]interface{}, error) {
	resp, err := f.Client.Get(reqURL)
	if err != nil {
		return nil, nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode == 404 {
		return nil, nil, &ErrUpstreamNotFound{StatusCode: 404, URL: reqURL}
	}
	if resp.StatusCode >= 400 {
		return nil, nil, fmt.Errorf("upstream returned status %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to read upstream response: %w", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, nil, fmt.Errorf("failed to parse upstream response: %w", err)
	}

	var items []interface{}
	// Support dot-path data keys (e.g., "allRelatedGroup.conceptGroup")
	if data, ok := resolveByDotPath(parsed, dataKey); ok {
		if arr, ok := data.([]interface{}); ok {
			items = arr
		} else {
			items = []interface{}{data}
		}
	} else if data, ok := parsed[dataKey]; ok {
		// Fallback to simple top-level lookup
		if arr, ok := data.([]interface{}); ok {
			items = arr
		} else {
			items = []interface{}{data}
		}
	} else {
		items = []interface{}{parsed}
	}

	return items, parsed, nil
}

// flattenConceptGroups flattens nested conceptGroup structures into a single array.
// For each item in data that has a "conceptProperties" array, it extracts those
// properties and copies the "tty" field from the parent group onto each child.
// Items without conceptProperties or that are already flat pass through unchanged.
func flattenConceptGroups(data []interface{}) []interface{} {
	result := make([]interface{}, 0)
	hasConceptGroups := false

	for _, item := range data {
		group, ok := item.(map[string]interface{})
		if !ok {
			// Not a map — include as-is (already flat)
			result = append(result, item)
			continue
		}

		cpRaw, hasCPs := group["conceptProperties"]
		if !hasCPs {
			// No conceptProperties — check if this looks like a conceptGroup
			// (has tty field). If so, skip it (empty group). Otherwise include as-is.
			if _, hasTTY := group["tty"]; hasTTY {
				hasConceptGroups = true
				continue // skip empty conceptGroup
			}
			result = append(result, item)
			continue
		}

		// conceptProperties exists but could be nil
		cpArr, ok := cpRaw.([]interface{})
		if !ok || len(cpArr) == 0 {
			hasConceptGroups = true
			continue // skip null or empty conceptProperties
		}

		hasConceptGroups = true
		tty := group["tty"]

		for _, cp := range cpArr {
			cpMap, ok := cp.(map[string]interface{})
			if !ok {
				result = append(result, cp)
				continue
			}
			// Copy tty from parent group onto each child
			if tty != nil {
				cpMap["tty"] = tty
			}
			result = append(result, cpMap)
		}
	}

	// If no conceptGroups were detected, return original data unchanged
	if !hasConceptGroups {
		return data
	}

	return result
}

func hasMorePages(parsed map[string]interface{}, currentPage int, totalKey string, isOffset bool, pagesize int) bool {
	val, ok := resolveByDotPath(parsed, totalKey)
	if !ok {
		return false
	}
	total, ok := val.(float64)
	if !ok {
		return false
	}
	if isOffset {
		// For offset pagination, check if skip + pagesize < total
		skip := (currentPage) * pagesize // currentPage is 1-based, next page skip
		return skip < int(total)
	}
	return currentPage < int(total)
}

func buildURL(ep config.Endpoint, path string, page int, params map[string]string) string {
	u := ep.BaseURL + path

	q := url.Values{}
	for k, v := range ep.QueryParams {
		substituted := config.SubstitutePathParams(v, params)
		// Skip query params with unresolved {PLACEHOLDER} values
		if strings.Contains(substituted, "{") && strings.Contains(substituted, "}") {
			continue
		}
		q.Set(k, substituted)
	}

	// Build search param from search_params: resolve each clause, skip unresolved, join with +
	if len(ep.SearchParams) > 0 {
		var clauses []string
		for _, clause := range ep.SearchParams {
			resolved := config.SubstitutePathParams(clause, params)
			if strings.Contains(resolved, "{") && strings.Contains(resolved, "}") {
				continue
			}
			clauses = append(clauses, resolved)
		}
		if len(clauses) > 0 {
			q.Set("search", strings.Join(clauses, "+"))
		}
	}

	maxPages, fetchAll := config.ParsePagination(ep)
	if fetchAll || maxPages > 1 {
		if ep.PaginationStyle == "offset" {
			skip := (page - 1) * ep.Pagesize
			q.Set("skip", fmt.Sprintf("%d", skip))
			q.Set("limit", fmt.Sprintf("%d", ep.Pagesize))
		} else {
			q.Set(ep.PageParam, fmt.Sprintf("%d", page))
			q.Set(ep.PagesizeParam, fmt.Sprintf("%d", ep.Pagesize))
		}
	}

	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}
