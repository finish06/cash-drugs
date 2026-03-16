package handler

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// LoadWarmupQueries reads and parses a warmup-queries.yaml file.
// Returns a map of slug -> []params. If the file doesn't exist, returns
// an empty map with no error (AC-011). Duplicate entries are deduplicated (AC-012).
func LoadWarmupQueries(path string) (map[string][]map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return make(map[string][]map[string]string), nil
		}
		return nil, fmt.Errorf("failed to read warmup queries file: %w", err)
	}

	var raw map[string][]map[string]string
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse warmup queries YAML: %w", err)
	}

	// Deduplicate entries per slug
	result := make(map[string][]map[string]string)
	for slug, entries := range raw {
		seen := make(map[string]bool)
		var unique []map[string]string
		for _, entry := range entries {
			key := paramsKey(entry)
			if !seen[key] {
				seen[key] = true
				unique = append(unique, entry)
			}
		}
		if len(unique) > 0 {
			result[slug] = unique
		}
	}

	return result, nil
}

// TotalQueryCount returns the total number of queries across all slugs.
func TotalQueryCount(queries map[string][]map[string]string) int {
	count := 0
	for _, entries := range queries {
		count += len(entries)
	}
	return count
}

// QueryCountForSlugs returns the total number of queries for the given slugs.
// If slugs is nil, returns the total across all slugs.
func QueryCountForSlugs(queries map[string][]map[string]string, slugs []string) int {
	if slugs == nil {
		return TotalQueryCount(queries)
	}
	count := 0
	for _, slug := range slugs {
		count += len(queries[slug])
	}
	return count
}

// paramsKey creates a deterministic string key for deduplication.
func paramsKey(params map[string]string) string {
	keys := make([]string, 0, len(params))
	for k := range params {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	parts := make([]string, 0, len(keys))
	for _, k := range keys {
		parts = append(parts, k+"="+params[k])
	}
	return strings.Join(parts, "&")
}
