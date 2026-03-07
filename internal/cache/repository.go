package cache

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/finish06/drugs/internal/model"
)

// Repository defines the interface for cache storage operations.
type Repository interface {
	Get(cacheKey string) (*model.CachedResponse, error)
	Upsert(resp *model.CachedResponse) error
	FetchedAt(cacheKey string) (time.Time, bool, error)
}

// BuildCacheKey constructs a deterministic cache key from slug and optional params.
func BuildCacheKey(slug string, params map[string]string) string {
	if len(params) == 0 {
		return slug
	}

	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := []string{slug}
	for _, k := range keys {
		parts = append(parts, fmt.Sprintf("%s=%s", k, params[k]))
	}
	return strings.Join(parts, ":")
}
