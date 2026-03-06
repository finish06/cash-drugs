package upstream

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"github.com/finish06/drugs/internal/cache"
	"github.com/finish06/drugs/internal/config"
	"github.com/finish06/drugs/internal/model"
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
func (f *HTTPFetcher) Fetch(ep config.Endpoint, params map[string]string) (*model.CachedResponse, error) {
	path := config.SubstitutePathParams(ep.Path, params)
	maxPages, fetchAll := config.ParsePagination(ep)

	var allData []interface{}
	pageCount := 0

	for page := 1; ; page++ {
		reqURL := buildURL(ep, path, page)

		data, parsed, err := f.fetchPage(reqURL)
		if err != nil {
			return nil, err
		}

		pageCount++
		allData = append(allData, data...)

		if !fetchAll && pageCount >= maxPages {
			break
		}

		if !hasMorePages(parsed, page) {
			break
		}
	}

	cacheKey := cache.BuildCacheKey(ep.Slug, params)
	sourceURL := ep.BaseURL + path
	now := time.Now()

	return &model.CachedResponse{
		Slug:        ep.Slug,
		Params:      params,
		CacheKey:    cacheKey,
		Data:        allData,
		ContentType: "application/" + ep.Format,
		FetchedAt:   now,
		SourceURL:   sourceURL,
		HTTPStatus:  200,
		PageCount:   pageCount,
		CreatedAt:   now,
		UpdatedAt:   now,
	}, nil
}

func (f *HTTPFetcher) fetchPage(reqURL string) ([]interface{}, map[string]interface{}, error) {
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
	if data, ok := parsed["data"]; ok {
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

func hasMorePages(parsed map[string]interface{}, currentPage int) bool {
	metadata, ok := parsed["metadata"].(map[string]interface{})
	if !ok {
		return false
	}
	totalPages, _ := metadata["total_pages"].(float64)
	return currentPage < int(totalPages)
}

func buildURL(ep config.Endpoint, path string, page int) string {
	u := ep.BaseURL + path

	q := url.Values{}
	for k, v := range ep.QueryParams {
		q.Set(k, v)
	}

	maxPages, fetchAll := config.ParsePagination(ep)
	if fetchAll || maxPages > 1 {
		q.Set(ep.PageParam, fmt.Sprintf("%d", page))
		q.Set(ep.PagesizeParam, fmt.Sprintf("%d", ep.Pagesize))
	}

	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	return u
}
