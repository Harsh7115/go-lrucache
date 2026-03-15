// Package lrucache provides a thread-safe, generic LRU cache with optional
// per-entry TTL eviction. Both Get and Put run in O(1) time using a doubly
// linked list paired with a hash map.
//
// Example — basic usage:
//
//	c := lrucache.New[string, int](128, lrucache.WithTTL[string, int](5*time.Minute))
//	c.Put("answer", 42)
//	if v, ok := c.Get("answer"); ok {
//		fmt.Println(v) // 42
//	}
package lrucache

import (
	"container/list"
	"sync"
	"time"
)

// EvictCallback is called with the key and value of every evicted entry.
type EvictCallback[K comparable, V any] func(key K, value V)

// Option is a functional option for Cache.
type Option[K comparable, V any] func(*Cache[K, V])

// WithTTL sets a time-to-live for every entry. Expired entries are evicted
// lazily on the next Get or Put that touches them.
func WithTTL[K comparable, V any](ttl time.Duration) Option[K, V] {
	return func(c *Cache[K, V]) { c.ttl = ttl }
}

// WithEvictCallback registers a function that is called whenever an entry is
// removed from the cache (capacity eviction or TTL expiry).
func WithEvictCallback[K comparable, V any](fn EvictCallback[K, V]) Option[K, V] {
	return func(c *Cache[K, V]) { c.onEvict = fn }
}

// entry is the value stored in the list and looked up via the map.
type entry[K comparable, V any] struct {
	key       K
	value     V
	expiresAt time.Time // zero value means no TTL
}

// Cache is a thread-safe LRU cache with optional TTL eviction.
// K must be comparable; V can be any type.
type Cache[K comparable, V any] struct {
	mu      sync.Mutex
	cap     int
	ttl     time.Duration
	onEvict EvictCallback[K, V]
	list    *list.List
	items   map[K]*list.Element
}

// New creates a Cache with the given capacity and options.
// Panics if cap < 1.
func New[K comparable, V any](cap int, opts ...Option[K, V]) *Cache[K, V] {
	if cap < 1 {
		panic("lrucache: capacity must be >= 1")
	}
	c := &Cache[K, V]{
		cap:   cap,
		list:  list.New(),
		items: make(map[K]*list.Element, cap),
	}
	for _, o := range opts {
		o(c)
	}
	return c
}

// Get returns the value for key and true if it exists and has not expired.
// The entry is promoted to the front of the LRU list on a cache hit.
func (c *Cache[K, V]) Get(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}

	e := el.Value.(*entry[K, V])
	if c.isExpired(e) {
		c.removeElement(el)
		var zero V
		return zero, false
	}

	c.list.MoveToFront(el)
	return e.value, true
}

// Put inserts or updates key with the given value.
// If the cache is at capacity the least-recently-used entry is evicted first.
func (c *Cache[K, V]) Put(key K, value V) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if el, ok := c.items[key]; ok {
		c.list.MoveToFront(el)
		e := el.Value.(*entry[K, V])
		e.value = value
		e.expiresAt = c.expiry()
		return
	}

	if c.list.Len() >= c.cap {
		c.removeLRU()
	}

	e := &entry[K, V]{key: key, value: value, expiresAt: c.expiry()}
	el := c.list.PushFront(e)
	c.items[key] = el
}

// Delete removes key from the cache. Returns true if the key was present.
func (c *Cache[K, V]) Delete(key K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return false
	}
	c.removeElement(el)
	return true
}

// Contains reports whether key is present and not expired without updating the
// LRU order. Useful for existence checks that must not alter recency.
func (c *Cache[K, V]) Contains(key K) bool {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		return false
	}
	if c.isExpired(el.Value.(*entry[K, V])) {
		c.removeElement(el)
		return false
	}
	return true
}

// Peek returns the value for key without updating LRU order or expiry.
func (c *Cache[K, V]) Peek(key K) (V, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	el, ok := c.items[key]
	if !ok {
		var zero V
		return zero, false
	}
	e := el.Value.(*entry[K, V])
	if c.isExpired(e) {
		c.removeElement(el)
		var zero V
		return zero, false
	}
	return e.value, true
}

// Len returns the number of live (non-expired) entries.
func (c *Cache[K, V]) Len() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.list.Len()
}

// Purge removes all entries from the cache, calling the evict callback for
// each one if registered.
func (c *Cache[K, V]) Purge() {
	c.mu.Lock()
	defer c.mu.Unlock()

	for _, el := range c.items {
		c.removeElement(el)
	}
}

// Keys returns the keys in MRU → LRU order, skipping expired entries.
func (c *Cache[K, V]) Keys() []K {
	c.mu.Lock()
	defer c.mu.Unlock()

	keys := make([]K, 0, c.list.Len())
	var expired []*list.Element
	for el := c.list.Front(); el != nil; el = el.Next() {
		e := el.Value.(*entry[K, V])
		if c.isExpired(e) {
			expired = append(expired, el)
			continue
		}
		keys = append(keys, e.key)
	}
	for _, el := range expired {
		c.removeElement(el)
	}
	return keys
}

// Resize changes the cache capacity. If the new capacity is smaller than the
// current number of entries, the oldest entries are evicted until the cache
// fits. Returns the number of entries evicted.
func (c *Cache[K, V]) Resize(newCap int) int {
	if newCap < 1 {
		panic("lrucache: capacity must be >= 1")
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	c.cap = newCap
	evicted := 0
	for c.list.Len() > c.cap {
		c.removeLRU()
		evicted++
	}
	return evicted
}

// --- internal helpers (called with c.mu held) ---

func (c *Cache[K, V]) expiry() time.Time {
	if c.ttl <= 0 {
		return time.Time{}
	}
	return time.Now().Add(c.ttl)
}

func (c *Cache[K, V]) isExpired(e *entry[K, V]) bool {
	return !e.expiresAt.IsZero() && time.Now().After(e.expiresAt)
}

func (c *Cache[K, V]) removeElement(el *list.Element) {
	e := el.Value.(*entry[K, V])
	c.list.Remove(el)
	delete(c.items, e.key)
	if c.onEvict != nil {
		c.onEvict(e.key, e.value)
	}
}

func (c *Cache[K, V]) removeLRU() {
	if el := c.list.Back(); el != nil {
		c.removeElement(el)
	}
}
