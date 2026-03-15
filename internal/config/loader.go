package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

// Endpoint represents a configured upstream API endpoint.
type Endpoint struct {
	Slug            string            `yaml:"slug"`
	BaseURL         string            `yaml:"base_url"`
	Path            string            `yaml:"path"`
	Format          string            `yaml:"format"`
	QueryParams     map[string]string `yaml:"query_params"`
	Pagination      interface{}       `yaml:"pagination"`
	PaginationStyle string            `yaml:"pagination_style"`
	PageParam       string            `yaml:"page_param"`
	PagesizeParam   string            `yaml:"pagesize_param"`
	Pagesize        int               `yaml:"pagesize"`
	SearchParams    []string          `yaml:"search_params"`
	DataKey         string            `yaml:"data_key"`
	TotalKey        string            `yaml:"total_key"`
	Refresh         string            `yaml:"refresh"`
	TTL             string            `yaml:"ttl"`
	TTLDuration     time.Duration     `yaml:"-"` // computed from TTL at load time
}

// AppConfig holds top-level application configuration beyond endpoints.
type AppConfig struct {
	LogLevel              string         `yaml:"log_level"`
	SystemMetricsInterval string         `yaml:"system_metrics_interval"`
	MaxConcurrentRequests int            `yaml:"max_concurrent_requests"`
	LRUCacheSizeMB        int            `yaml:"lru_cache_size_mb"`
	Endpoints             []Endpoint     `yaml:"endpoints"`
	Database              databaseConfig `yaml:"database"`
}

type configFile = AppConfig

type databaseConfig struct {
	URI string `yaml:"uri"`
}

// Load reads and validates a YAML config file, returning the parsed endpoints.
func Load(path string) ([]Endpoint, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config file not found at %s: %w", path, err)
	}

	var cfg configFile
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML in config file: %w", err)
	}

	if len(cfg.Endpoints) == 0 {
		return nil, fmt.Errorf("config file contains no endpoints")
	}

	slugsSeen := make(map[string]bool)
	for i, ep := range cfg.Endpoints {
		if ep.Slug == "" {
			return nil, fmt.Errorf("endpoint %d: missing required field 'slug'", i)
		}
		if ep.BaseURL == "" {
			return nil, fmt.Errorf("endpoint '%s': missing required field 'base_url'", ep.Slug)
		}
		if ep.Path == "" {
			return nil, fmt.Errorf("endpoint '%s': missing required field 'path'", ep.Slug)
		}
		if ep.Format == "" {
			return nil, fmt.Errorf("endpoint '%s': missing required field 'format'", ep.Slug)
		}
		if ep.Format != "json" && ep.Format != "xml" && ep.Format != "raw" {
			return nil, fmt.Errorf("endpoint '%s': invalid format '%s' (must be 'json', 'xml', or 'raw')", ep.Slug, ep.Format)
		}
		if slugsSeen[ep.Slug] {
			return nil, fmt.Errorf("duplicate slug '%s' in config", ep.Slug)
		}
		slugsSeen[ep.Slug] = true

		if ep.PaginationStyle != "" && ep.PaginationStyle != "page" && ep.PaginationStyle != "offset" {
			return nil, fmt.Errorf("endpoint '%s': invalid pagination_style '%s' (must be 'page' or 'offset')", ep.Slug, ep.PaginationStyle)
		}

		if ep.Refresh != "" {
			parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
			if _, err := parser.Parse(ep.Refresh); err != nil {
				return nil, fmt.Errorf("endpoint '%s': invalid cron expression '%s': %w", ep.Slug, ep.Refresh, err)
			}
		}

		if ep.TTL != "" {
			d, err := time.ParseDuration(ep.TTL)
			if err != nil {
				return nil, fmt.Errorf("endpoint '%s': invalid ttl '%s': %w", ep.Slug, ep.TTL, err)
			}
			cfg.Endpoints[i].TTLDuration = d
		}

		ApplyDefaults(&cfg.Endpoints[i])
	}

	return cfg.Endpoints, nil
}

// ApplyDefaults sets default values for optional fields.
func ApplyDefaults(ep *Endpoint) {
	if ep.PaginationStyle == "" {
		ep.PaginationStyle = "page"
	}
	if ep.PageParam == "" {
		ep.PageParam = "page"
	}
	if ep.PagesizeParam == "" {
		ep.PagesizeParam = "pagesize"
	}
	if ep.Pagesize == 0 {
		ep.Pagesize = 100
	}
	if ep.DataKey == "" {
		ep.DataKey = "data"
	}
	if ep.TotalKey == "" {
		ep.TotalKey = "metadata.total_pages"
	}
}

// ParsePagination interprets the pagination field and returns (maxPages, fetchAll).
// If pagination is "all", fetchAll is true and maxPages is 0.
// If pagination is a number, maxPages is that number and fetchAll is false.
// If pagination is nil/unset, maxPages is 1 and fetchAll is false.
func ParsePagination(ep Endpoint) (maxPages int, fetchAll bool) {
	if ep.Pagination == nil {
		return 1, false
	}

	switch v := ep.Pagination.(type) {
	case string:
		if strings.EqualFold(v, "all") {
			return 0, true
		}
		return 1, false
	case int:
		if v <= 0 {
			return 1, false
		}
		return v, false
	case float64:
		iv := int(v)
		if iv <= 0 {
			return 1, false
		}
		return iv, false
	default:
		return 1, false
	}
}

var pathParamRegex = regexp.MustCompile(`\{(\w+)\}`)

// ExtractPathParams returns the names of path parameters in a path template.
func ExtractPathParams(path string) []string {
	matches := pathParamRegex.FindAllStringSubmatch(path, -1)
	params := make([]string, 0, len(matches))
	for _, m := range matches {
		params = append(params, m[1])
	}
	return params
}

// ExtractAllParams returns parameter names from both path and query_params values.
func ExtractAllParams(ep Endpoint) []string {
	seen := make(map[string]bool)
	var params []string

	for _, p := range ExtractPathParams(ep.Path) {
		if !seen[p] {
			seen[p] = true
			params = append(params, p)
		}
	}
	for _, v := range ep.QueryParams {
		for _, p := range ExtractPathParams(v) {
			if !seen[p] {
				seen[p] = true
				params = append(params, p)
			}
		}
	}
	for _, v := range ep.SearchParams {
		for _, p := range ExtractPathParams(v) {
			if !seen[p] {
				seen[p] = true
				params = append(params, p)
			}
		}
	}
	return params
}

// SubstitutePathParams replaces {param} placeholders in a path with values.
func SubstitutePathParams(path string, params map[string]string) string {
	result := path
	for key, value := range params {
		result = strings.ReplaceAll(result, "{"+key+"}", value)
	}
	return result
}

// LoadConfig reads the full application config including top-level settings.
func LoadConfig(path string) (*AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("config file not found at %s: %w", path, err)
	}

	var cfg AppConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("invalid YAML in config file: %w", err)
	}

	return &cfg, nil
}

// ResolveMongoURI determines the MongoDB connection URI.
// Priority: MONGO_URI env var > database.uri in config file.
// Returns error if neither is set.
func ResolveMongoURI(configPath string) (string, error) {
	if uri := os.Getenv("MONGO_URI"); uri != "" {
		return uri, nil
	}

	if configPath != "" {
		data, err := os.ReadFile(configPath)
		if err == nil {
			var cfg configFile
			if err := yaml.Unmarshal(data, &cfg); err == nil {
				if cfg.Database.URI != "" {
					return cfg.Database.URI, nil
				}
			}
		}
	}

	return "", fmt.Errorf("MongoDB URI not configured: set MONGO_URI or database.uri in config")
}
