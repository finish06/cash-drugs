package cache

import (
	"container/list"
	"hash/fnv"
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

// NewLRUCache creates a new sharded LRU cache bounded by maxBytes with 16 shards.
// When maxBytes <= 0, returns a no-op implementation that always misses.
func NewLRUCache(maxBytes int64) LRUCache {
	if maxBytes <= 0 {
		return &noopLRU{}
	}
	return NewShardedLRUCache(maxBytes, 16)
}

// minShardBytes is the minimum number of bytes a shard should hold to be
// effective. If maxBytes / shardCount would give less than this, the shard
// count is reduced so each shard is at least this size.
const minShardBytes = 32768 // 32KB

// NewShardedLRUCache creates a sharded LRU cache with the given shard count.
// Each shard has its own mutex, map, and list for independent locking.
// The total memory budget is divided evenly across shards.
func NewShardedLRUCache(maxBytes int64, shardCount int) LRUCache {
	// Clamp shard count to sensible bounds
	if shardCount <= 0 {
		shardCount = 1
	}
	if int64(shardCount) > maxBytes {
		shardCount = int(maxBytes)
		if shardCount <= 0 {
			shardCount = 1
		}
	}
	// Reduce shard count if per-shard budget would be too small
	if maxBytes/int64(shardCount) < minShardBytes && maxBytes > 0 {
		shardCount = int(maxBytes / minShardBytes)
		if shardCount <= 0 {
			shardCount = 1
		}
	}

	perShard := maxBytes / int64(shardCount)
	if perShard <= 0 {
		perShard = 1
	}

	shards := make([]*lruShard, shardCount)
	for i := range shards {
		shards[i] = &lruShard{
			maxBytes: perShard,
			items:    make(map[string]*list.Element),
			order:    list.New(),
		}
	}

	return &shardedLRUCache{
		shards:     shards,
		shardCount: shardCount,
	}
}

// shardedLRUCache distributes cache entries across multiple shards
// to reduce lock contention under concurrent access.
type shardedLRUCache struct {
	shards     []*lruShard
	shardCount int
}

// lruShard is a single shard with its own mutex and LRU data structures.
type lruShard struct {
	mu       sync.Mutex
	maxBytes int64
	curBytes int64
	items    map[string]*list.Element
	order    *list.List
}

// shardIndex returns the shard index for a given key using FNV-1a hash.
func shardIndex(key string, shardCount int) int {
	h := fnv.New32a()
	h.Write([]byte(key))
	return int(h.Sum32()) % shardCount
}

func (s *shardedLRUCache) getShard(key string) *lruShard {
	return s.shards[shardIndex(key, s.shardCount)]
}

func (s *shardedLRUCache) Get(key string) (*model.CachedResponse, bool) {
	shard := s.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	elem, ok := shard.items[key]
	if !ok {
		return nil, false
	}

	entry := elem.Value.(*lruEntry)

	// Check TTL
	if time.Now().After(entry.expiresAt) {
		shard.removeElement(elem)
		return nil, false
	}

	// Move to front (most recently used)
	shard.order.MoveToFront(elem)
	return entry.resp, true
}

func (s *shardedLRUCache) Set(key string, resp *model.CachedResponse, ttl time.Duration) {
	shard := s.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	size := estimateSize(resp)

	// If entry already exists, remove it first
	if elem, ok := shard.items[key]; ok {
		shard.removeElement(elem)
	}

	// Evict LRU entries until we have room
	for shard.curBytes+size > shard.maxBytes && shard.order.Len() > 0 {
		shard.removeLRU()
	}

	entry := &lruEntry{
		key:       key,
		resp:      resp,
		expiresAt: time.Now().Add(ttl),
		size:      size,
	}

	elem := shard.order.PushFront(entry)
	shard.items[key] = elem
	shard.curBytes += size
}

func (s *shardedLRUCache) Invalidate(key string) {
	shard := s.getShard(key)
	shard.mu.Lock()
	defer shard.mu.Unlock()

	if elem, ok := shard.items[key]; ok {
		shard.removeElement(elem)
	}
}

func (s *shardedLRUCache) SizeBytes() int64 {
	var total int64
	for _, shard := range s.shards {
		shard.mu.Lock()
		total += shard.curBytes
		shard.mu.Unlock()
	}
	return total
}

func (shard *lruShard) removeElement(elem *list.Element) {
	entry := elem.Value.(*lruEntry)
	shard.order.Remove(elem)
	delete(shard.items, entry.key)
	shard.curBytes -= entry.size
}

func (shard *lruShard) removeLRU() {
	back := shard.order.Back()
	if back != nil {
		shard.removeElement(back)
	}
}

// estimateSize returns an approximate byte size of a CachedResponse.
// Uses a PageCount-based heuristic to avoid expensive JSON marshaling on every Set.
func estimateSize(resp *model.CachedResponse) int64 {
	// Base struct overhead (slug, source URL, content type, timestamps, etc.)
	size := int64(200)

	size += int64(len(resp.Slug))
	size += int64(len(resp.CacheKey))
	size += int64(len(resp.SourceURL))
	size += int64(len(resp.ContentType))

	// Estimate data size from PageCount (~50KB per page average).
	// Falls back to 10KB default when PageCount is 0.
	if resp.PageCount > 0 {
		size += int64(resp.PageCount) * 50000
	} else if resp.Data != nil {
		size += 10000
	}

	return size
}

// noopLRU is a no-op LRU cache that always misses.
type noopLRU struct{}

func (n *noopLRU) Get(key string) (*model.CachedResponse, bool) { return nil, false }
func (n *noopLRU) Set(key string, resp *model.CachedResponse, ttl time.Duration) {}
func (n *noopLRU) Invalidate(key string)                                          {}
func (n *noopLRU) SizeBytes() int64                                               { return 0 }
