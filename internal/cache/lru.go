package cache

import (
	"container/list"
	"encoding/json"
	"sync"
	"time"

	"github.com/finish06/cash-drugs/internal/model"
)

// LRUCache provides an in-memory cache with TTL and size-based eviction.
type LRUCache interface {
	Get(key string) (*model.CachedResponse, bool)
	Set(key string, resp *model.CachedResponse, ttl time.Duration)
	Invalidate(key string)
	SizeBytes() int64
}

type lruEntry struct {
	key       string
	resp      *model.CachedResponse
	expiresAt time.Time
	size      int64
}

type lruCache struct {
	mu       sync.Mutex
	maxBytes int64
	curBytes int64
	items    map[string]*list.Element
	order    *list.List // front = most recently used
}

// NewLRUCache creates a new LRU cache bounded by maxBytes.
// When maxBytes <= 0, returns a no-op implementation that always misses.
func NewLRUCache(maxBytes int64) LRUCache {
	if maxBytes <= 0 {
		return &noopLRU{}
	}
	return &lruCache{
		maxBytes: maxBytes,
		items:    make(map[string]*list.Element),
		order:    list.New(),
	}
}

func (c *lruCache) Get(key string) (*model.CachedResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	elem, ok := c.items[key]
	if !ok {
		return nil, false
	}

	entry := elem.Value.(*lruEntry)

	// Check TTL
	if time.Now().After(entry.expiresAt) {
		c.removeElement(elem)
		return nil, false
	}

	// Move to front (most recently used)
	c.order.MoveToFront(elem)
	return entry.resp, true
}

func (c *lruCache) Set(key string, resp *model.CachedResponse, ttl time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()

	size := estimateSize(resp)

	// If entry already exists, remove it first
	if elem, ok := c.items[key]; ok {
		c.removeElement(elem)
	}

	// Evict LRU entries until we have room
	for c.curBytes+size > c.maxBytes && c.order.Len() > 0 {
		c.removeLRU()
	}

	entry := &lruEntry{
		key:       key,
		resp:      resp,
		expiresAt: time.Now().Add(ttl),
		size:      size,
	}

	elem := c.order.PushFront(entry)
	c.items[key] = elem
	c.curBytes += size
}

func (c *lruCache) Invalidate(key string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if elem, ok := c.items[key]; ok {
		c.removeElement(elem)
	}
}

func (c *lruCache) SizeBytes() int64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.curBytes
}

func (c *lruCache) removeElement(elem *list.Element) {
	entry := elem.Value.(*lruEntry)
	c.order.Remove(elem)
	delete(c.items, entry.key)
	c.curBytes -= entry.size
}

func (c *lruCache) removeLRU() {
	back := c.order.Back()
	if back != nil {
		c.removeElement(back)
	}
}

// estimateSize returns an approximate byte size of a CachedResponse.
func estimateSize(resp *model.CachedResponse) int64 {
	// Base struct overhead
	size := int64(200)

	// Estimate data size by JSON marshaling
	if resp.Data != nil {
		data, err := json.Marshal(resp.Data)
		if err == nil {
			size += int64(len(data))
		}
	}

	size += int64(len(resp.Slug))
	size += int64(len(resp.CacheKey))
	size += int64(len(resp.SourceURL))
	size += int64(len(resp.ContentType))

	return size
}

// noopLRU is a no-op LRU cache that always misses.
type noopLRU struct{}

func (n *noopLRU) Get(key string) (*model.CachedResponse, bool) { return nil, false }
func (n *noopLRU) Set(key string, resp *model.CachedResponse, ttl time.Duration) {}
func (n *noopLRU) Invalidate(key string)                                          {}
func (n *noopLRU) SizeBytes() int64                                               { return 0 }
