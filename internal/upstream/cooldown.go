package upstream

import (
	"sync"
	"time"
)

// CooldownTracker tracks per-key cooldown windows for force-refresh rate limiting.
type CooldownTracker struct {
	timestamps sync.Map
	duration   time.Duration
}

// NewCooldownTracker creates a new CooldownTracker with the given cooldown duration.
func NewCooldownTracker(duration time.Duration) *CooldownTracker {
	return &CooldownTracker{
		duration: duration,
	}
}

// Check returns true if the key is within the cooldown window (blocked).
func (c *CooldownTracker) Check(key string) bool {
	val, ok := c.timestamps.Load(key)
	if !ok {
		return false
	}
	lastRefresh := val.(time.Time)
	return time.Since(lastRefresh) < c.duration
}

// Record records the current time as the last refresh for the given key.
func (c *CooldownTracker) Record(key string) {
	c.timestamps.Store(key, time.Now())
}
