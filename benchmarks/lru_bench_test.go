package benchmarks

import (
	"fmt"
	"testing"

	lrucache "github.com/Harsh7115/go-lrucache"
)

// BenchmarkGet measures O(1) cache hit performance.
func BenchmarkGet(b *testing.B) {
	cache := lrucache.New(1024)
	for i := 0; i < 1024; i++ {
		cache.Put(fmt.Sprintf("key:%d", i), i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Get(fmt.Sprintf("key:%d", i%1024))
	}
}

// BenchmarkPut measures O(1) insert performance with eviction pressure.
func BenchmarkPut(b *testing.B) {
	cache := lrucache.New(256)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Put(fmt.Sprintf("key:%d", i), i)
	}
}

// BenchmarkMixed measures a realistic 80% read / 20% write workload.
func BenchmarkMixed(b *testing.B) {
	cache := lrucache.New(512)
	for i := 0; i < 512; i++ {
		cache.Put(fmt.Sprintf("key:%d", i), i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%5 == 0 {
			cache.Put(fmt.Sprintf("key:%d", i), i)
		} else {
			cache.Get(fmt.Sprintf("key:%d", i%512))
		}
	}
}

// BenchmarkEviction stress-tests LRU eviction by always inserting new keys
// into a full cache, forcing every Put to evict the least-recently-used entry.
func BenchmarkEviction(b *testing.B) {
	const capacity = 128
	cache := lrucache.New(capacity)
	for i := 0; i < capacity; i++ {
		cache.Put(fmt.Sprintf("seed:%d", i), i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Put(fmt.Sprintf("evict:%d", i), i)
	}
}

// BenchmarkDelete measures the cost of key removal.
func BenchmarkDelete(b *testing.B) {
	cache := lrucache.New(1024)
	for i := 0; i < 1024; i++ {
		cache.Put(fmt.Sprintf("key:%d", i), i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		k := fmt.Sprintf("key:%d", i%1024)
		cache.Delete(k)
		cache.Put(k, i)
	}
}

// BenchmarkGetParallel measures throughput under concurrent read access.
func BenchmarkGetParallel(b *testing.B) {
	cache := lrucache.New(1024)
	for i := 0; i < 1024; i++ {
		cache.Put(fmt.Sprintf("key:%d", i), i)
	}
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			cache.Get(fmt.Sprintf("key:%d", i%1024))
			i++
		}
	})
}

// BenchmarkPutParallel measures throughput under concurrent write access.
func BenchmarkPutParallel(b *testing.B) {
	cache := lrucache.New(512)
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			cache.Put(fmt.Sprintf("key:%d", i), i)
			i++
		}
	})
}

// BenchmarkSmallCache shows eviction rate when capacity is very tight.
func BenchmarkSmallCache(b *testing.B) {
	cache := lrucache.New(8)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.Put(fmt.Sprintf("k%d", i%16), i)
		cache.Get(fmt.Sprintf("k%d", i%16))
	}
}
