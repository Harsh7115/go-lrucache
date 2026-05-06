// examples/two_level_cache.go
//
// Two-Level LRU Cache (L1 / L2 hierarchy)
//
// Demonstrates a CPU-cache-inspired tiered cache built on top of go-lrucache.
// A small, fast L1 layer holds the hottest entries; a larger L2 layer catches
// the next tier.  On a Get:
//   1. L1 hit  - return immediately (fastest path)
//   2. L1 miss + L2 hit - promote entry to L1, return value
//   3. Both miss - call the user-supplied Fetch function, populate L2, return value
//
// On a Put the value is written to L1.  When L1 evicts an entry via its
// EvictCallback the evicted entry is demoted to L2 rather than discarded.

package main

import (
	"errors"
	"fmt"
	"sync"
	"time"

	lrucache "github.com/Harsh7115/go-lrucache"
)

// FetchFunc is called when a key misses both cache layers.
type FetchFunc[K comparable, V any] func(key K) (V, error)

// Stats holds hit/miss counters for observability.
type Stats struct {
	L1Hits    int64
	L2Hits    int64
	Misses    int64
	Fetches   int64
	Evictions int64
}

// TwoLevelCache is a generic, thread-safe two-level LRU cache.
type TwoLevelCache[K comparable, V any] struct {
	mu    sync.Mutex
	l1    *lrucache.Cache[K, V]
	l2    *lrucache.Cache[K, V]
	fetch FetchFunc[K, V]
	stats Stats
}

// ErrNotFound is returned by Get on a full miss with no FetchFunc.
var ErrNotFound = errors.New("lrucache: key not found")

// NewTwoLevelCache constructs a TwoLevelCache.
//
// l1Cap / l2Cap - entry capacities for each tier
// l1TTL / l2TTL - per-entry expiry (0 disables TTL for that layer)
// fetch         - cold-miss loader; may be nil
func NewTwoLevelCache[K comparable, V any](
	l1Cap, l2Cap int,
	l1TTL, l2TTL time.Duration,
	fetch FetchFunc[K, V],
) *TwoLevelCache[K, V] {
	c := &TwoLevelCache[K, V]{fetch: fetch}

	// L1 evict callback: demote evicted entry to L2 instead of discarding.
	l1Evict := func(key K, val V) {
		c.stats.Evictions++
		if c.l2 != nil {
			c.l2.Put(key, val)
		}
	}

	l1Opts := []lrucache.Option[K, V]{
		lrucache.WithEvictCallback[K, V](l1Evict),
	}
	if l1TTL > 0 {
		l1Opts = append(l1Opts, lrucache.WithTTL[K, V](l1TTL))
	}
	c.l1 = lrucache.New[K, V](l1Cap, l1Opts...)

	l2Opts := []lrucache.Option[K, V]{}
	if l2TTL > 0 {
		l2Opts = append(l2Opts, lrucache.WithTTL[K, V](l2TTL))
	}
	c.l2 = lrucache.New[K, V](l2Cap, l2Opts...)

	return c
}

// Put inserts or updates key in L1. Evicted L1 entries are demoted to L2.
func (c *TwoLevelCache[K, V]) Put(key K, val V) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.l1.Put(key, val)
}

// Get retrieves a value, consulting L1 then L2 then FetchFunc.
func (c *TwoLevelCache[K, V]) Get(key K) (V, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// 1. L1 hit
	if val, ok := c.l1.Get(key); ok {
		c.stats.L1Hits++
		return val, nil
	}

	// 2. L2 hit - promote to L1
	if val, ok := c.l2.Get(key); ok {
		c.stats.L2Hits++
		c.l1.Put(key, val)
		return val, nil
	}

	// 3. Cold miss
	c.stats.Misses++
	if c.fetch == nil {
		var zero V
		return zero, ErrNotFound
	}

	c.stats.Fetches++
	// Temporarily release the lock while doing I/O.
	c.mu.Unlock()
	val, err := c.fetch(key)
	c.mu.Lock()

	if err != nil {
		var zero V
		return zero, err
	}
	// Populate L2 on cold load; L1 is reserved for entries accessed repeatedly.
	c.l2.Put(key, val)
	return val, nil
}

// Delete removes key from both tiers.
func (c *TwoLevelCache[K, V]) Delete(key K) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.l1.Delete(key)
	c.l2.Delete(key)
}

// Stats returns a snapshot of hit/miss counters.
func (c *TwoLevelCache[K, V]) Stats() Stats {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.stats
}

// Len returns (l1Len, l2Len).
func (c *TwoLevelCache[K, V]) Len() (int, int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.l1.Len(), c.l2.Len()
}

// ---------------------------------------------------------------------------
// Demo
// ---------------------------------------------------------------------------

func main() {
	dbCalls := 0
	db := map[string]string{
		"user:1": "alice",
		"user:2": "bob",
		"user:3": "carol",
		"user:4": "dave",
		"user:5": "eve",
	}

	fetch := func(key string) (string, error) {
		dbCalls++
		val, ok := db[key]
		if !ok {
			return "", fmt.Errorf("key %q not in database", key)
		}
		fmt.Printf("  [DB] fetched %q -> %q\n", key, val)
		return val, nil
	}

	cache := NewTwoLevelCache[string, string](
		2,              // L1: only 2 slots to force early demotion
		10,             // L2: up to 10 entries
		30*time.Second, // L1 TTL
		5*time.Minute,  // L2 TTL
		fetch,
	)

	fmt.Println("=== Round 1: cold load five keys ===")
	keys := []string{"user:1", "user:2", "user:3", "user:4", "user:5"}
	for _, k := range keys {
		v, err := cache.Get(k)
		if err != nil {
			fmt.Printf("  GET %s -> error: %v\n", k, err)
		} else {
			fmt.Printf("  GET %s -> %q\n", k, v)
		}
	}

	fmt.Println()
	fmt.Println("=== Round 2: re-read (L1/L2 hits, zero DB calls) ===")
	dbBefore := dbCalls
	for _, k := range keys {
		v, _ := cache.Get(k)
		fmt.Printf("  GET %s -> %q\n", k, v)
	}
	fmt.Printf("  DB calls this round: %d (expected 0)\n", dbCalls-dbBefore)

	fmt.Println()
	fmt.Println("=== Round 3: explicit Put ===")
	cache.Put("user:99", "zara")
	v, _ := cache.Get("user:99")
	fmt.Printf("  PUT user:99=zara; GET user:99 -> %q\n", v)

	fmt.Println()
	s := cache.Stats()
	l1Len, l2Len := cache.Len()
	fmt.Printf("=== Stats ===\n")
	fmt.Printf("  L1 hits:    %d\n", s.L1Hits)
	fmt.Printf("  L2 hits:    %d\n", s.L2Hits)
	fmt.Printf("  Misses:     %d\n", s.Misses)
	fmt.Printf("  DB fetches: %d\n", s.Fetches)
	fmt.Printf("  Evictions:  %d (L1 -> L2 demotions)\n", s.Evictions)
	fmt.Printf("  L1 size:    %d / 2\n", l1Len)
	fmt.Printf("  L2 size:    %d / 10\n", l2Len)
}
