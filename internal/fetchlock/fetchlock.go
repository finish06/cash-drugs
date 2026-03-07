package fetchlock

import "sync"

// Map provides per-slug mutexes for deduplicating concurrent fetches.
// Shared between the scheduler and cache handler.
type Map struct {
	mu    sync.Mutex
	locks map[string]*sync.Mutex
}

// New creates a new fetch lock map.
func New() *Map {
	return &Map{
		locks: make(map[string]*sync.Mutex),
	}
}

// Get returns the mutex for the given slug, creating it if needed.
func (m *Map) Get(slug string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()

	if mu, ok := m.locks[slug]; ok {
		return mu
	}
	mu := &sync.Mutex{}
	m.locks[slug] = mu
	return mu
}
