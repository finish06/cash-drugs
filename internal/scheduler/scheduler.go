package scheduler

import (
	"context"
	"log"
	"sync"
	"time"

	"github.com/finish06/drugs/internal/cache"
	"github.com/finish06/drugs/internal/config"
	"github.com/finish06/drugs/internal/fetchlock"
	"github.com/finish06/drugs/internal/upstream"
	"github.com/robfig/cron/v3"
)

// Scheduler manages background refresh of cached endpoints.
type Scheduler struct {
	endpoints []config.Endpoint
	fetcher   upstream.Fetcher
	repo      cache.Repository
	cron      *cron.Cron
	locks     *fetchlock.Map
}

// New creates a Scheduler for the given endpoints.
// Only endpoints with a non-empty Refresh field and no path parameters are scheduled.
func New(endpoints []config.Endpoint, fetcher upstream.Fetcher, repo cache.Repository, locks *fetchlock.Map) *Scheduler {
	return &Scheduler{
		endpoints: endpoints,
		fetcher:   fetcher,
		repo:      repo,
		cron:      cron.New(),
		locks:     locks,
	}
}

// Start warms the cache for all scheduled endpoints, then starts the cron scheduler.
func (s *Scheduler) Start(ctx context.Context) {
	scheduled := s.schedulableEndpoints()

	if len(scheduled) == 0 {
		log.Println("Scheduler: no endpoints to schedule")
		return
	}

	// Warm cache immediately
	var wg sync.WaitGroup
	for _, ep := range scheduled {
		wg.Add(1)
		go func(ep config.Endpoint) {
			defer wg.Done()
			s.fetchEndpoint(ep)
		}(ep)
	}
	wg.Wait()

	// Register cron jobs
	for _, ep := range scheduled {
		ep := ep // capture loop variable
		s.cron.AddFunc(ep.Refresh, func() {
			s.fetchEndpoint(ep)
		})
		log.Printf("Scheduler: registered %s with cron '%s'", ep.Slug, ep.Refresh)
	}

	s.cron.Start()
	log.Printf("Scheduler: started with %d endpoint(s)", len(scheduled))
}

// Stop gracefully shuts down the scheduler.
func (s *Scheduler) Stop() {
	ctx := s.cron.Stop()
	<-ctx.Done()
	log.Println("Scheduler stopped")
}

func (s *Scheduler) fetchEndpoint(ep config.Endpoint) {
	mu := s.locks.Get(ep.Slug)
	if !mu.TryLock() {
		log.Printf("Scheduler: skipping %s — previous fetch still running", ep.Slug)
		return
	}
	defer mu.Unlock()

	start := time.Now()
	result, err := s.fetcher.Fetch(ep, nil)
	duration := time.Since(start)

	if err != nil {
		log.Printf("Scheduler: fetch failed for %s (%v) — preserving existing cache", ep.Slug, duration)
		return
	}

	if err := s.repo.Upsert(result); err != nil {
		log.Printf("Scheduler: cache upsert failed for %s: %v", ep.Slug, err)
		return
	}

	log.Printf("Scheduler: refreshed %s (%v, %d pages)", ep.Slug, duration, result.PageCount)
}

func (s *Scheduler) schedulableEndpoints() []config.Endpoint {
	var result []config.Endpoint
	for _, ep := range s.endpoints {
		if ep.Refresh == "" {
			continue
		}
		if len(config.ExtractPathParams(ep.Path)) > 0 {
			log.Printf("Scheduler: warning — endpoint '%s' has path parameters and cannot be scheduled", ep.Slug)
			continue
		}
		result = append(result, ep)
	}
	return result
}
