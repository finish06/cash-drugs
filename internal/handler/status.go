package handler

import (
	"encoding/json"
	"net/http"
	"sort"
	"time"

	"github.com/finish06/cash-drugs/internal/cache"
	"github.com/finish06/cash-drugs/internal/config"
)

// SlugStatus describes the cache health of a single configured endpoint.
type SlugStatus struct {
	Slug         string `json:"slug"`
	Configured   bool   `json:"configured"`
	LastRefresh  string `json:"last_refresh,omitempty"`
	IsStale      bool   `json:"is_stale"`
	TTLRemaining string `json:"ttl_remaining"`
	HasSchedule  bool   `json:"has_schedule"`
	Schedule     string `json:"schedule,omitempty"`
	Health       int    `json:"health"`
}

// CacheStatusResponse is the JSON envelope returned by GET /api/cache/status.
type CacheStatusResponse struct {
	Slugs        map[string]SlugStatus `json:"slugs"`
	TotalSlugs   int                   `json:"total_slugs"`
	HealthySlugs int                   `json:"healthy_slugs"`
	StaleSlugs   int                   `json:"stale_slugs"`
	GeneratedAt  string                `json:"generated_at"`
}

// StatusHandler handles GET /api/cache/status requests.
type StatusHandler struct {
	endpoints map[string]config.Endpoint
	repo      cache.Repository
}

// NewStatusHandler creates a new StatusHandler.
func NewStatusHandler(endpoints map[string]config.Endpoint, repo cache.Repository) *StatusHandler {
	return &StatusHandler{
		endpoints: endpoints,
		repo:      repo,
	}
}

// ServeHTTP returns per-slug cache health.
//
// @Summary      Cache status overview
// @Description  Returns per-slug cache health including staleness, TTL remaining, and health score.
// @Tags         system
// @Produce      json
// @Success      200  {object}  CacheStatusResponse
// @Router       /api/cache/status [get]
func (h *StatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	now := time.Now()

	// Sort slugs for deterministic output
	slugs := make([]string, 0, len(h.endpoints))
	for slug := range h.endpoints {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	result := CacheStatusResponse{
		Slugs:       make(map[string]SlugStatus, len(h.endpoints)),
		TotalSlugs:  len(h.endpoints),
		GeneratedAt: now.Format(time.RFC3339),
	}

	for _, slug := range slugs {
		ep := h.endpoints[slug]
		ss := SlugStatus{
			Slug:        slug,
			Configured:  true,
			HasSchedule: ep.Refresh != "",
		}
		if ep.Refresh != "" {
			ss.Schedule = ep.Refresh
		}

		fetchedAt, found, err := h.repo.FetchedAt(slug)
		if err != nil || !found {
			// Not cached at all
			ss.Health = 0
			ss.TTLRemaining = "0s"
			ss.IsStale = true
			result.StaleSlugs++
			result.Slugs[slug] = ss
			continue
		}

		ss.LastRefresh = fetchedAt.Format(time.RFC3339)

		if ep.TTL == "" {
			// Cached but no TTL configured — unknown staleness
			ss.Health = 75
			ss.IsStale = false
			ss.TTLRemaining = "0s"
			result.HealthySlugs++
			result.Slugs[slug] = ss
			continue
		}

		stale := config.IsStale(ep, fetchedAt)
		ss.IsStale = stale

		if stale {
			ss.Health = 50
			ss.TTLRemaining = "0s"
			result.StaleSlugs++
		} else {
			ss.Health = 100
			remaining := ep.TTLDuration - now.Sub(fetchedAt)
			if remaining < 0 {
				remaining = 0
			}
			ss.TTLRemaining = remaining.Truncate(time.Second).String()
			result.HealthySlugs++
		}

		result.Slugs[slug] = ss
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(result)
}
