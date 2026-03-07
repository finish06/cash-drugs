package config

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

// Endpoint represents a configured upstream API endpoint.
type Endpoint struct {
	Slug          string            `yaml:"slug"`
	BaseURL       string            `yaml:"base_url"`
	Path          string            `yaml:"path"`
	Format        string            `yaml:"format"`
	QueryParams   map[string]string `yaml:"query_params"`
	Pagination    interface{}       `yaml:"pagination"`
	PageParam     string            `yaml:"page_param"`
	PagesizeParam string            `yaml:"pagesize_param"`
	Pagesize      int               `yaml:"pagesize"`
	Refresh       string            `yaml:"refresh"`
}

type configFile struct {
	Endpoints []Endpoint     `yaml:"endpoints"`
	Database  databaseConfig `yaml:"database"`
}

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
		if ep.Format != "json" && ep.Format != "xml" {
			return nil, fmt.Errorf("endpoint '%s': invalid format '%s' (must be 'json' or 'xml')", ep.Slug, ep.Format)
		}
		if slugsSeen[ep.Slug] {
			return nil, fmt.Errorf("duplicate slug '%s' in config", ep.Slug)
		}
		slugsSeen[ep.Slug] = true

		if ep.Refresh != "" {
			parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
			if _, err := parser.Parse(ep.Refresh); err != nil {
				return nil, fmt.Errorf("endpoint '%s': invalid cron expression '%s': %w", ep.Slug, ep.Refresh, err)
			}
		}

		ApplyDefaults(&cfg.Endpoints[i])
	}

	return cfg.Endpoints, nil
}

// ApplyDefaults sets default values for optional fields.
func ApplyDefaults(ep *Endpoint) {
	if ep.PageParam == "" {
		ep.PageParam = "page"
	}
	if ep.PagesizeParam == "" {
		ep.PagesizeParam = "pagesize"
	}
	if ep.Pagesize == 0 {
		ep.Pagesize = 100
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

// SubstitutePathParams replaces {param} placeholders in a path with values.
func SubstitutePathParams(path string, params map[string]string) string {
	result := path
	for key, value := range params {
		result = strings.ReplaceAll(result, "{"+key+"}", value)
	}
	return result
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
