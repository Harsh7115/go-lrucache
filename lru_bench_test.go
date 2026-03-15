package lrucache_test

import (
	"fmt"
	"strconv"
	"sync"
	"testing"

	lrucache "github.com/Harsh7115/go-lrucache"
)

const benchCap = 1024

// ── sequential ────────────────────────────────────────────────────────────────

func BenchmarkPut(b *testing.B) {
	c := lrucache.New[int, int](benchCap)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Put(i%benchCap, i)
	}
}

func BenchmarkGet_Hit(b *testing.B) {
	c := lrucache.New[int, int](benchCap)
	for i := 0; i < benchCap; i++ {
		c.Put(i, i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(i % benchCap)
	}
}

func BenchmarkGet_Miss(b *testing.B) {
	c := lrucache.New[int, int](benchCap)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(i % benchCap)
	}
}

func BenchmarkMixed(b *testing.B) {
	c := lrucache.New[int, int](benchCap)
	for i := 0; i < benchCap; i++ {
		c.Put(i, i)
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if i%3 == 0 {
			c.Put(i%benchCap, i)
		} else {
			c.Get(i % benchCap)
		}
	}
}

func BenchmarkStringKeys(b *testing.B) {
	c := lrucache.New[string, string](benchCap)
	keys := make([]string, benchCap)
	for i := range keys {
		keys[i] = "key:" + strconv.Itoa(i)
		c.Put(keys[i], fmt.Sprintf("value:%d", i))
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		c.Get(keys[i%benchCap])
	}
}

// ── parallel (goroutine-safe) ─────────────────────────────────────────────────

func BenchmarkPut_Parallel(b *testing.B) {
	c := lrucache.New[int, int](benchCap)
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Put(i%benchCap, i)
			i++
		}
	})
}

func BenchmarkGet_Parallel(b *testing.B) {
	c := lrucache.New[int, int](benchCap)
	for i := 0; i < benchCap; i++ {
		c.Put(i, i)
	}
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			c.Get(i % benchCap)
			i++
		}
	})
}

func BenchmarkMixed_Parallel(b *testing.B) {
	c := lrucache.New[int, int](benchCap)
	for i := 0; i < benchCap; i++ {
		c.Put(i, i)
	}
	b.RunParallel(func(pb *testing.PB) {
		i := 0
		for pb.Next() {
			if i%3 == 0 {
				c.Put(i%benchCap, i)
			} else {
				c.Get(i % benchCap)
			}
			i++
		}
	})
}

// ── scaling ───────────────────────────────────────────────────────────────────

func BenchmarkScale(b *testing.B) {
	sizes := []int{64, 256, 1024, 4096, 16384}
	for _, cap := range sizes {
		cap := cap
		b.Run(fmt.Sprintf("cap=%d", cap), func(b *testing.B) {
			c := lrucache.New[int, int](cap)
			for i := 0; i < cap; i++ {
				c.Put(i, i)
			}
			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				c.Get(i % cap)
			}
		})
	}
}

func BenchmarkContention(b *testing.B) {
	goroutines := []int{1, 2, 4, 8, 16, 32}
	for _, n := range goroutines {
		n := n
		b.Run(fmt.Sprintf("goroutines=%d", n), func(b *testing.B) {
			c := lrucache.New[int, int](benchCap)
			for i := 0; i < benchCap; i++ {
				c.Put(i, i)
			}
			b.ResetTimer()
			var wg sync.WaitGroup
			ops := b.N / n
			for g := 0; g < n; g++ {
				g := g
				wg.Add(1)
				go func() {
					defer wg.Done()
					for i := 0; i < ops; i++ {
						c.Get((g*ops + i) % benchCap)
					}
				}()
			}
			wg.Wait()
		})
	}
}
