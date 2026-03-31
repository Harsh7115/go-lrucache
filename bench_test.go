package lrucache_test

import (
	"fmt"
	"sync"
	"testing"
	"time"

	lrucache "github.com/Harsh7115/go-lrucache"
)

// ----------------------------------------------------------------------------
// Put benchmarks
// ----------------------------------------------------------------------------

// BenchmarkPut measures single-goroutine Put throughput on a cache that never
// fills (capacity >> b.N) so we avoid measuring eviction overhead.
func BenchmarkPut(b *testing.B) {
	c := lrucache.New[int, int](b.N + 1)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put(i, i)
	}
}

// BenchmarkPutEvict measures Put throughput when the cache is always at
// capacity, forcing an LRU eviction on every insertion.
func BenchmarkPutEvict(b *testing.B) {
	const cap = 512
	c := lrucache.New[int, int](cap)
	for i := 0; i < cap; i++ {
		c.Put(i, i)
	}
	b.ResetTimer()
	for i := cap; i < cap+b.N; i++ {
		c.Put(i, i)
	}
}

// ----------------------------------------------------------------------------
// Get benchmarks
// ----------------------------------------------------------------------------

// BenchmarkGetHit measures Get throughput for a cache-hit workload (all keys
// present).
func BenchmarkGetHit(b *testing.B) {
	const cap = 1024
	c := lrucache.New[int, int](cap)
	for i := 0; i < cap; i++ {
		c.Put(i, i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(i % cap)
	}
}

// BenchmarkGetMiss measures Get throughput when all lookups miss (cold cache).
func BenchmarkGetMiss(b *testing.B) {
	c := lrucache.New[int, int](1024)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(i)
	}
}

// ----------------------------------------------------------------------------
// Mixed workload benchmarks
// ----------------------------------------------------------------------------

// BenchmarkMixed80Read20Write simulates a realistic read-heavy workload:
// 80 % Gets and 20 % Puts against a fixed key space.
func BenchmarkMixed80Read20Write(b *testing.B) {
	const cap = 512
	c := lrucache.New[int, int](cap)
	for i := 0; i < cap; i++ {
		c.Put(i, i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := i % cap
		if i%5 == 0 {
			c.Put(key, key*2)
		} else {
			c.Get(key)
		}
	}
}

// BenchmarkMixed50Read50Write simulates a write-heavy workload.
func BenchmarkMixed50Read50Write(b *testing.B) {
	const cap = 512
	c := lrucache.New[int, int](cap)
	for i := 0; i < cap; i++ {
		c.Put(i, i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		key := i % cap
		if i%2 == 0 {
			c.Put(key, key)
		} else {
			c.Get(key)
		}
	}
}

// ----------------------------------------------------------------------------
// TTL benchmarks
// ----------------------------------------------------------------------------

// BenchmarkPutWithTTL measures Put throughput when every entry carries a TTL.
func BenchmarkPutWithTTL(b *testing.B) {
	c := lrucache.New[int, int](
		b.N+1,
		lrucache.WithTTL[int, int](5*time.Minute),
	)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put(i, i)
	}
}

// BenchmarkGetExpired measures the overhead of lazy TTL eviction: every key
// was inserted with a 1 ns TTL so all Gets hit an expired entry.
func BenchmarkGetExpired(b *testing.B) {
	const cap = 1024
	c := lrucache.New[int, int](
		cap,
		lrucache.WithTTL[int, int](time.Nanosecond),
	)
	for i := 0; i < cap; i++ {
		c.Put(i, i)
	}
	// Give TTLs time to expire.
	time.Sleep(time.Millisecond)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(i % cap)
	}
}

// ----------------------------------------------------------------------------
// Concurrent benchmarks
// ----------------------------------------------------------------------------

// BenchmarkConcurrentGet measures Get throughput with GOMAXPROCS goroutines
// hammering the same hot-key set.
func BenchmarkConcurrentGet(b *testing.B) {
	const cap = 1024
	c := lrucache.New[int, int](cap)
	for i := 0; i < cap; i++ {
		c.Put(i, i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Get(i % cap)
			i++
		}
	})
}

// BenchmarkConcurrentPut measures Put throughput under high concurrency with
// eviction (cache capacity < total keys written).
func BenchmarkConcurrentPut(b *testing.B) {
	const cap = 256
	c := lrucache.New[int, int](cap)

	var mu sync.Mutex
	n := 0

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			mu.Lock()
			key := n
			n++
			mu.Unlock()
			c.Put(key, key)
		}
	})
}

// BenchmarkConcurrentMixed runs mixed Put/Get operations from multiple
// goroutines simultaneously — the most realistic concurrency scenario.
func BenchmarkConcurrentMixed(b *testing.B) {
	const cap = 512
	c := lrucache.New[int, int](cap)
	for i := 0; i < cap; i++ {
		c.Put(i, i)
	}

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			key := i % cap
			if i%4 == 0 {
				c.Put(key, key)
			} else {
				c.Get(key)
			}
			i++
		}
	})
}

// ----------------------------------------------------------------------------
// String key benchmarks
// ----------------------------------------------------------------------------

// BenchmarkStringKey measures performance with string keys (hashing cost).
func BenchmarkStringKey(b *testing.B) {
	const cap = 1024
	c := lrucache.New[string, int](cap)
	keys := make([]string, cap)
	for i := range keys {
		keys[i] = fmt.Sprintf("key-%04d", i)
		c.Put(keys[i], i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(keys[i%cap])
	}
}
