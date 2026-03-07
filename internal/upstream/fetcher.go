package upstream

import (
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/model"
)

// Fetcher defines the interface for upstream API fetching.
type Fetcher interface {
	Fetch(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error)
}

// HTTPFetcher fetches from upstream APIs using net/http.
type HTTPFetcher struct {
	Client *http.Client
}

// NewHTTPFetcher creates a new HTTPFetcher with sensible defaults.
func NewHTTPFetcher() *HTTPFetcher {
	return &HTTPFetcher{
		Client: &http.Client{Timeout: 30 * time.Second},
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

	var allPages []model.PageData
	pageCount := 0

	for page := 1; ; page++ {
		reqURL := buildURL(ep, path, page, params)

		data, parsed, err := f.fetchJSONPage(reqURL, ep.DataKey)
		if err != nil {
			// For offset pagination, gracefully stop on error and return partial data
			if isOffset && pageCount > 0 {
				slog.Warn("offset pagination stopped due to error, returning partial data",
					"slug", ep.Slug, "pages_fetched", pageCount, "error", err)
				break
			}
			return nil, err
		}

		pageCount++
		allPages = append(allPages, model.PageData{
			Page: pageCount,
			Data: data,
		})

		if !fetchAll && pageCount >= maxPages {
			break
		}

		if !hasMorePages(parsed, page, ep.TotalKey, isOffset, ep.Pagesize) {
			break
		}
	}

	cacheKey := cache.BuildCacheKey(ep.Slug, params)
	sourceURL := ep.BaseURL + path
	now := time.Now()

	// Combine all page data into a single slice for the response
	var allData []interface{}
	for _, p := range allPages {
		allData = append(allData, p.Data...)
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

func (f *HTTPFetcher) fetchRaw(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error) {
	path := config.SubstitutePathParams(ep.Path, params)
	reqURL := buildURL(ep, path, 1, params)

	resp, err := f.Client.Get(reqURL)
	if err != nil {
		return nil, fmt.Errorf("upstream request failed: %w", err)
	}
	defer resp.Body.Close()

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
	defer resp.Body.Close()

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
	if data, ok := parsed[dataKey]; ok {
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
		// Substitute {PARAM} placeholders in query param values
		q.Set(k, config.SubstitutePathParams(v, params))
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
