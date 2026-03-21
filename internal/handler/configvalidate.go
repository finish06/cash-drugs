package handler

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/finish06/cash-drugs/internal/config"
	"github.com/finish06/cash-drugs/internal/middleware"
	"github.com/finish06/cash-drugs/internal/model"
	"github.com/robfig/cron/v3"
	"gopkg.in/yaml.v3"
)

// configValidateRequest is the JSON body accepted by POST /api/config/validate.
type configValidateRequest struct {
	YAML string `json:"yaml"`
}

// configValidateEndpoint describes a validated endpoint in the response.
type configValidateEndpoint struct {
	Slug          string   `json:"slug"`
	BaseURL       string   `json:"base_url"`
	Path          string   `json:"path"`
	Format        string   `json:"format"`
	Params        []string `json:"params"`
	HasSchedule   bool     `json:"has_schedule"`
	HasPagination bool     `json:"has_pagination"`
}

// configValidateResponse is returned on successful validation.
type configValidateResponse struct {
	Valid         bool                     `json:"valid"`
	Endpoints     []configValidateEndpoint `json:"endpoints"`
	EndpointCount int                      `json:"endpoint_count"`
	Warnings      []string                 `json:"warnings"`
	RequestID     string                   `json:"request_id"`
}

// configValidateErrorResponse is returned when validation fails.
type configValidateErrorResponse struct {
	Valid     bool   `json:"valid"`
	Error     string `json:"error"`
	ErrorCode string `json:"error_code"`
	RequestID string `json:"request_id"`
}

// ConfigValidateHandler handles POST /api/config/validate.
type ConfigValidateHandler struct{}

// NewConfigValidateHandler creates a new ConfigValidateHandler.
func NewConfigValidateHandler() *ConfigValidateHandler {
	return &ConfigValidateHandler{}
}

// ServeHTTP handles the config validation request.
//
// @Summary      Validate config YAML
// @Description  Validates a YAML config snippet for structure and required fields without fetching from upstream.
// @Tags         config
// @Accept       json
// @Produce      json
// @Param        body  body  configValidateRequest  true  "YAML config to validate"
// @Success      200  {object}  configValidateResponse
// @Failure      400  {object}  configValidateErrorResponse
// @Router       /api/config/validate [post]
func (h *ConfigValidateHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	requestID := middleware.RequestID(r.Context())

	// Read and parse request body
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20)) // 1MB limit
	if err != nil {
		respondConfigValidateError(w, http.StatusBadRequest, "failed to read request body", requestID)
		return
	}

	var req configValidateRequest
	if err := json.Unmarshal(body, &req); err != nil {
		respondConfigValidateError(w, http.StatusBadRequest, "invalid JSON body: "+err.Error(), requestID)
		return
	}

	if strings.TrimSpace(req.YAML) == "" {
		respondConfigValidateError(w, http.StatusBadRequest, "missing or empty 'yaml' field", requestID)
		return
	}

	// Parse YAML into config structure
	var cfg struct {
		Endpoints []config.Endpoint `yaml:"endpoints"`
	}
	if err := yaml.Unmarshal([]byte(req.YAML), &cfg); err != nil {
		respondConfigValidateError(w, http.StatusOK, "invalid YAML: "+err.Error(), requestID)
		return
	}

	if len(cfg.Endpoints) == 0 {
		respondConfigValidateError(w, http.StatusOK, "config contains no endpoints", requestID)
		return
	}

	// Validate each endpoint
	slugsSeen := make(map[string]bool)
	var warnings []string

	for i, ep := range cfg.Endpoints {
		if ep.Slug == "" {
			respondConfigValidateError(w, http.StatusOK, fmt.Sprintf("endpoint %d: missing required field 'slug'", i), requestID)
			return
		}
		if ep.BaseURL == "" {
			respondConfigValidateError(w, http.StatusOK, fmt.Sprintf("endpoint '%s': missing required field 'base_url'", ep.Slug), requestID)
			return
		}
		if ep.Path == "" {
			respondConfigValidateError(w, http.StatusOK, fmt.Sprintf("endpoint '%s': missing required field 'path'", ep.Slug), requestID)
			return
		}
		if ep.Format == "" {
			respondConfigValidateError(w, http.StatusOK, fmt.Sprintf("endpoint '%s': missing required field 'format'", ep.Slug), requestID)
			return
		}
		if ep.Format != "json" && ep.Format != "xml" && ep.Format != "raw" {
			respondConfigValidateError(w, http.StatusOK, fmt.Sprintf("endpoint '%s': invalid format '%s' (must be 'json', 'xml', or 'raw')", ep.Slug, ep.Format), requestID)
			return
		}
		if slugsSeen[ep.Slug] {
			respondConfigValidateError(w, http.StatusOK, fmt.Sprintf("duplicate slug '%s' in config", ep.Slug), requestID)
			return
		}
		slugsSeen[ep.Slug] = true

		if ep.Refresh != "" {
			parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
			if _, err := parser.Parse(ep.Refresh); err != nil {
				respondConfigValidateError(w, http.StatusOK, fmt.Sprintf("endpoint '%s': invalid cron expression '%s': %s", ep.Slug, ep.Refresh, err.Error()), requestID)
				return
			}
		}

		// Validate pagination if set
		if ep.Pagination != nil {
			switch v := ep.Pagination.(type) {
			case string:
				if !strings.EqualFold(v, "all") {
					respondConfigValidateError(w, http.StatusOK, fmt.Sprintf("endpoint '%s': invalid pagination value '%s' (must be 'all' or a positive integer)", ep.Slug, v), requestID)
					return
				}
			case int:
				if v <= 0 {
					respondConfigValidateError(w, http.StatusOK, fmt.Sprintf("endpoint '%s': pagination must be 'all' or a positive integer", ep.Slug), requestID)
					return
				}
			case float64:
				if v <= 0 || v != float64(int(v)) {
					respondConfigValidateError(w, http.StatusOK, fmt.Sprintf("endpoint '%s': pagination must be 'all' or a positive integer", ep.Slug), requestID)
					return
				}
			default:
				respondConfigValidateError(w, http.StatusOK, fmt.Sprintf("endpoint '%s': invalid pagination type", ep.Slug), requestID)
				return
			}
		}

		// Warnings
		if ep.Refresh != "" && ep.TTL == "" {
			warnings = append(warnings, fmt.Sprintf("endpoint '%s': scheduled refresh without TTL — cached data may never expire", ep.Slug))
		}
		if ep.Pagesize > 1000 {
			warnings = append(warnings, fmt.Sprintf("endpoint '%s': large pagesize (%d) may cause slow responses", ep.Slug, ep.Pagesize))
		}
	}

	// Build response
	endpoints := make([]configValidateEndpoint, len(cfg.Endpoints))
	for i, ep := range cfg.Endpoints {
		params := config.ExtractAllParams(ep)
		hasPagination := ep.Pagination != nil
		endpoints[i] = configValidateEndpoint{
			Slug:          ep.Slug,
			BaseURL:       ep.BaseURL,
			Path:          ep.Path,
			Format:        ep.Format,
			Params:        params,
			HasSchedule:   ep.Refresh != "",
			HasPagination: hasPagination,
		}
	}

	if warnings == nil {
		warnings = []string{}
	}

	resp := configValidateResponse{
		Valid:         true,
		Endpoints:     endpoints,
		EndpointCount: len(endpoints),
		Warnings:      warnings,
		RequestID:     requestID,
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(resp)
}

// respondConfigValidateError writes a JSON error response for config validation.
func respondConfigValidateError(w http.ResponseWriter, httpStatus int, errMsg, requestID string) {
	resp := configValidateErrorResponse{
		Valid:     false,
		Error:     errMsg,
		ErrorCode: model.ErrCodeBadRequest,
		RequestID: requestID,
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(httpStatus)
	_ = json.NewEncoder(w).Encode(resp)
}
