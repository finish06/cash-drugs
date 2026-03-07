package config

import "time"

// IsStale returns true if the cached data is past the endpoint's TTL.
// Returns false if no TTL is configured (TTL field empty and TTLDuration is zero).
func IsStale(ep Endpoint, fetchedAt time.Time) bool {
	// No TTL configured — never stale
	if ep.TTL == "" {
		return false
	}

	// TTL of "0s" means always stale
	if ep.TTLDuration == 0 {
		return true
	}

	return time.Since(fetchedAt) > ep.TTLDuration
}
