// Package benchmarks provides micro-benchmarks for the go-lrucache library.
//
// Run:
//   go test ./benchmarks/ -bench=. -benchmem -benchtime=5s
//
// Results are printed in the standard Go benchmark format so they can be
// compared across runs with benchstat.
package benchmarks

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	lru "github.com/Harsh7115/go-lrucache"
)

const (
	cacheSize  = 1_000
	keySpace   = 10_000 // larger than cache → many evictions
	smallSpace = 500    // smaller than cache → mostly hits
)

// --------------------------------------------------------------------
// Helpers
// --------------------------------------------------------------------

func newCache(tb testing.TB, cap int) *lru.Cache[int, int] {
	tb.Helper()
	c, err := lru.New[int, int](cap)
	if err != nil {
		tb.Fatalf("lru.New: %v", err)
	}
	return c
}

func newCacheWithTTL(tb testing.TB, cap int, ttl time.Duration) *lru.Cache[int, int] {
	tb.Helper()
	c, err := lru.NewWithTTL[int, int](cap, ttl)
	if err != nil {
		tb.Fatalf("lru.NewWithTTL: %v", err)
	}
	return c
}

func prefill(c *lru.Cache[int, int], n int) {
	for i := 0; i < n; i++ {
		c.Put(i, i*2)
	}
}

// --------------------------------------------------------------------
// Put benchmarks
// --------------------------------------------------------------------

// BenchmarkPut_NoEviction fills a cache that is larger than the number
// of keys written, so no entries are evicted.
func BenchmarkPut_NoEviction(b *testing.B) {
	c := newCache(b, b.N+1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put(i, i)
	}
}

// BenchmarkPut_WithEviction keeps the cache at capacity so every Put
// that exceeds capacity triggers an eviction.
func BenchmarkPut_WithEviction(b *testing.B) {
	c := newCache(b, cacheSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put(i%keySpace, i)
	}
}

// BenchmarkPut_Parallel exercises concurrent writers.
func BenchmarkPut_Parallel(b *testing.B) {
	c := newCache(b, cacheSize)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Put(i%keySpace, i)
			i++
		}
	})
}

// --------------------------------------------------------------------
// Get benchmarks
// --------------------------------------------------------------------

// BenchmarkGet_HitRate100 benchmarks Get when every key is present.
func BenchmarkGet_HitRate100(b *testing.B) {
	c := newCache(b, cacheSize)
	prefill(c, cacheSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(i % cacheSize)
	}
}

// BenchmarkGet_HitRate50 benchmarks Get with ~50 % hit rate.
func BenchmarkGet_HitRate50(b *testing.B) {
	c := newCache(b, cacheSize)
	prefill(c, cacheSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(i % (cacheSize * 2))
	}
}

// BenchmarkGet_HitRateLow benchmarks Get with a key space 10× the
// cache capacity, producing mostly misses.
func BenchmarkGet_HitRateLow(b *testing.B) {
	c := newCache(b, cacheSize)
	prefill(c, cacheSize)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(i % keySpace)
	}
}

// BenchmarkGet_Parallel exercises concurrent readers.
func BenchmarkGet_Parallel(b *testing.B) {
	c := newCache(b, cacheSize)
	prefill(c, cacheSize)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Get(i % cacheSize)
			i++
		}
	})
}

// --------------------------------------------------------------------
// Mixed read/write benchmarks
// --------------------------------------------------------------------

// BenchmarkMixed_80R20W simulates an 80 % read / 20 % write workload.
func BenchmarkMixed_80R20W(b *testing.B) {
	c := newCache(b, cacheSize)
	prefill(c, cacheSize)
	r := rand.New(rand.NewSource(42)) //nolint:gosec
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := r.Intn(keySpace)
		if r.Intn(10) < 8 {
			c.Get(key)
		} else {
			c.Put(key, key)
		}
	}
}

// BenchmarkMixed_Parallel_80R20W is the parallel variant.
func BenchmarkMixed_Parallel_80R20W(b *testing.B) {
	c := newCache(b, cacheSize)
	prefill(c, cacheSize)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		r := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec
		for pb.Next() {
			key := r.Intn(keySpace)
			if r.Intn(10) < 8 {
				c.Get(key)
			} else {
				c.Put(key, key)
			}
		}
	})
}

// --------------------------------------------------------------------
// TTL eviction benchmarks
// --------------------------------------------------------------------

// BenchmarkPut_TTL measures Put throughput when TTL eviction is active.
func BenchmarkPut_TTL(b *testing.B) {
	c := newCacheWithTTL(b, cacheSize, 100*time.Millisecond)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put(i%keySpace, i)
	}
}

// BenchmarkGet_TTL_AfterExpiry reads keys after their TTL has lapsed,
// expecting misses and lazy eviction.
func BenchmarkGet_TTL_AfterExpiry(b *testing.B) {
	const ttl = 50 * time.Millisecond
	c := newCacheWithTTL(b, cacheSize, ttl)
	prefill(c, cacheSize)
	time.Sleep(ttl + 10*time.Millisecond) // let entries expire
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(i % cacheSize)
	}
}

// --------------------------------------------------------------------
// Capacity scaling benchmarks
// --------------------------------------------------------------------

// BenchmarkPut_CapacityScaling shows how throughput changes as the
// cache grows from 100 to 1 000 000 entries.
func BenchmarkPut_CapacityScaling(b *testing.B) {
	for _, cap := range []int{100, 1_000, 10_000, 100_000, 1_000_000} {
		cap := cap
		b.Run(fmt.Sprintf("cap=%d", cap), func(b *testing.B) {
			c := newCache(b, cap)
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c.Put(i%cap, i)
			}
		})
	}
}
