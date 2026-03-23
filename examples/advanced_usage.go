// Package main shows advanced usage patterns for go-lrucache:
//   - Typed caches with struct values
//   - TTL expiry and lazy eviction
//   - Evict callbacks for instrumentation
//   - Concurrent access across goroutines
//   - Resize at runtime
//   - Using Peek / Contains without promoting entries
//
// Run with:
//
//	go run ./examples/advanced_usage.go
package main

import (
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	lrucache "github.com/Harsh7115/go-lrucache"
)

// ------------------------------------------------------------
// Example 1: Struct values with an evict callback
// ------------------------------------------------------------

type Session struct {
	UserID    int
	Token     string
	CreatedAt time.Time
}

func exampleStructValues() {
	fmt.Println("=== Example 1: Struct values + evict callback ===")

	var evicted int64
	cache := lrucache.New[string, Session](
		3,
		lrucache.WithEvictCallback[string, Session](func(key string, s Session) {
			atomic.AddInt64(&evicted, 1)
			fmt.Printf("  evicted session for user %d (token %s)\n", s.UserID, key)
		}),
	)

	for i := 1; i <= 5; i++ {
		token := fmt.Sprintf("tok-%04d", i)
		cache.Put(token, Session{
			UserID:    i,
			Token:     token,
			CreatedAt: time.Now(),
		})
	}

	fmt.Printf("  cache len=%d  evictions=%d\n\n", cache.Len(), evicted)
}

// ------------------------------------------------------------
// Example 2: TTL expiry
// ------------------------------------------------------------

func exampleTTL() {
	fmt.Println("=== Example 2: TTL expiry ===")

	cache := lrucache.New[string, string](
		100,
		lrucache.WithTTL[string, string](200*time.Millisecond),
	)

	cache.Put("short-lived", "hello")

	if v, ok := cache.Get("short-lived"); ok {
		fmt.Printf("  before expiry: %q\n", v)
	}

	time.Sleep(250 * time.Millisecond)

	if _, ok := cache.Get("short-lived"); !ok {
		fmt.Println("  after expiry: key correctly evicted (lazy)")
	}
	fmt.Println()
}

// ------------------------------------------------------------
// Example 3: Peek and Contains (non-promoting reads)
// ------------------------------------------------------------

func examplePeekContains() {
	fmt.Println("=== Example 3: Peek / Contains (non-promoting) ===")

	cache := lrucache.New[int, string](4)
	for i := 1; i <= 4; i++ {
		cache.Put(i, fmt.Sprintf("v%d", i))
	}

	before := cache.Keys()
	fmt.Printf("  order before Peek: %v\n", before)

	v, ok := cache.Peek(1)
	fmt.Printf("  Peek(1) = %q, found=%v\n", v, ok)

	after := cache.Keys()
	fmt.Printf("  order after  Peek: %v (unchanged)\n", after)

	fmt.Printf("  Contains(3) = %v\n", cache.Contains(3))
	fmt.Printf("  Contains(99) = %v\n\n", cache.Contains(99))
}

// ------------------------------------------------------------
// Example 4: Concurrent writes from multiple goroutines
// ------------------------------------------------------------

func exampleConcurrent() {
	fmt.Println("=== Example 4: Concurrent access ===")

	const (
		goroutines = 8
		ops        = 500
		cacheSize  = 64
	)

	cache := lrucache.New[int, int](cacheSize)
	var wg sync.WaitGroup
	var hits, misses int64

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(int64(id)))
			for i := 0; i < ops; i++ {
				key := rng.Intn(cacheSize * 2)
				if _, ok := cache.Get(key); ok {
					atomic.AddInt64(&hits, 1)
				} else {
					atomic.AddInt64(&misses, 1)
					cache.Put(key, key*key)
				}
			}
		}(g)
	}

	wg.Wait()
	total := hits + misses
	fmt.Printf("  goroutines=%d  ops=%d  hits=%d (%.1f%%)  misses=%d\n\n",
		goroutines, total, hits, float64(hits)/float64(total)*100, misses)
}

// ------------------------------------------------------------
// Example 5: Runtime resize
// ------------------------------------------------------------

func exampleResize() {
	fmt.Println("=== Example 5: Runtime resize ===")

	cache := lrucache.New[int, string](10)
	for i := 0; i < 10; i++ {
		cache.Put(i, fmt.Sprintf("item-%d", i))
	}
	fmt.Printf("  before resize: len=%d cap=10\n", cache.Len())

	evicted := cache.Resize(5)
	fmt.Printf("  after  resize: len=%d cap=5  evicted=%d\n\n", cache.Len(), evicted)
}

func main() {
	exampleStructValues()
	exampleTTL()
	examplePeekContains()
	exampleConcurrent()
	exampleResize()
	fmt.Println("All examples complete.")
}
