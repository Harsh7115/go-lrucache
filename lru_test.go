package lrucache_test

import (
	"sync"
	"testing"
	"time"

	lrucache "github.com/Harsh7115/go-lrucache"
)

// ---- basic correctness ----

func TestPutAndGet(t *testing.T) {
	c := lrucache.New[string, int](3)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)

	for _, tt := range []struct {
		key  string
		want int
	}{{"a", 1}, {"b", 2}, {"c", 3}} {
		v, ok := c.Get(tt.key)
		if !ok || v != tt.want {
			t.Errorf("Get(%q) = %d, %v; want %d, true", tt.key, v, ok, tt.want)
		}
	}
}

func TestCapacityEviction(t *testing.T) {
	c := lrucache.New[string, int](2)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3) // "a" should be evicted (LRU)

	if _, ok := c.Get("a"); ok {
		t.Error("expected 'a' to be evicted")
	}
	if v, ok := c.Get("b"); !ok || v != 2 {
		t.Errorf("expected b=2, got %d, %v", v, ok)
	}
	if v, ok := c.Get("c"); !ok || v != 3 {
		t.Errorf("expected c=3, got %d, %v", v, ok)
	}
}

func TestLRUOrder(t *testing.T) {
	c := lrucache.New[string, int](3)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)

	// Access "a" to make it MRU; next eviction should remove "b"
	c.Get("a")
	c.Put("d", 4)

	if _, ok := c.Get("b"); ok {
		t.Error("expected 'b' to be evicted after promoting 'a'")
	}
	if _, ok := c.Get("a"); !ok {
		t.Error("expected 'a' to still be present")
	}
}

func TestUpdate(t *testing.T) {
	c := lrucache.New[string, int](2)
	c.Put("x", 10)
	c.Put("x", 99)

	if v, ok := c.Get("x"); !ok || v != 99 {
		t.Errorf("expected x=99, got %d, %v", v, ok)
	}
	if c.Len() != 1 {
		t.Errorf("expected len=1 after update, got %d", c.Len())
	}
}

func TestDelete(t *testing.T) {
	c := lrucache.New[string, int](3)
	c.Put("a", 1)
	c.Put("b", 2)

	if !c.Delete("a") {
		t.Error("Delete returned false for existing key")
	}
	if c.Delete("a") {
		t.Error("Delete returned true for already-deleted key")
	}
	if _, ok := c.Get("a"); ok {
		t.Error("key 'a' still accessible after delete")
	}
	if c.Len() != 1 {
		t.Errorf("expected len=1, got %d", c.Len())
	}
}

func TestContainsAndPeek(t *testing.T) {
	c := lrucache.New[string, int](2)
	c.Put("a", 42)

	if !c.Contains("a") {
		t.Error("Contains returned false for present key")
	}
	v, ok := c.Peek("a")
	if !ok || v != 42 {
		t.Errorf("Peek returned %d, %v; want 42, true", v, ok)
	}
	if c.Contains("missing") {
		t.Error("Contains returned true for missing key")
	}
}

func TestPurge(t *testing.T) {
	var evicted []string
	c := lrucache.New[string, int](4,
		lrucache.WithEvictCallback[string, int](func(k string, _ int) {
			evicted = append(evicted, k)
		}),
	)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Purge()

	if c.Len() != 0 {
		t.Errorf("Len after Purge = %d; want 0", c.Len())
	}
	if len(evicted) != 2 {
		t.Errorf("evict callback called %d times; want 2", len(evicted))
	}
}

func TestKeys(t *testing.T) {
	c := lrucache.New[string, int](3)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)
	c.Get("a") // promote a → MRU

	keys := c.Keys() // expected order: a, c, b
	if len(keys) != 3 {
		t.Fatalf("expected 3 keys, got %d", len(keys))
	}
	if keys[0] != "a" {
		t.Errorf("expected MRU key 'a', got %q", keys[0])
	}
}

func TestResize(t *testing.T) {
	c := lrucache.New[string, int](4)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3)

	evicted := c.Resize(2)
	if evicted != 1 {
		t.Errorf("Resize evicted %d entries; want 1", evicted)
	}
	if c.Len() != 2 {
		t.Errorf("Len after Resize = %d; want 2", c.Len())
	}
}

// ---- TTL eviction ----

func TestTTLExpiry(t *testing.T) {
	c := lrucache.New[string, int](4,
		lrucache.WithTTL[string, int](50*time.Millisecond),
	)
	c.Put("a", 1)

	if _, ok := c.Get("a"); !ok {
		t.Fatal("expected 'a' to be live immediately after Put")
	}

	time.Sleep(80 * time.Millisecond)

	if _, ok := c.Get("a"); ok {
		t.Error("expected 'a' to be expired after TTL")
	}
	if c.Len() != 0 {
		t.Errorf("Len after expiry = %d; want 0", c.Len())
	}
}

// ---- evict callback ----

func TestEvictCallback(t *testing.T) {
	var mu sync.Mutex
	evicted := map[string]int{}

	c := lrucache.New[string, int](2,
		lrucache.WithEvictCallback[string, int](func(k string, v int) {
			mu.Lock()
			evicted[k] = v
			mu.Unlock()
		}),
	)
	c.Put("a", 1)
	c.Put("b", 2)
	c.Put("c", 3) // evicts "a"

	mu.Lock()
	defer mu.Unlock()
	if evicted["a"] != 1 {
		t.Errorf("expected evicted['a']=1, got %d", evicted["a"])
	}
}

// ---- concurrency ----

func TestConcurrentAccess(t *testing.T) {
	c := lrucache.New[int, int](64)
	var wg sync.WaitGroup
	const goroutines = 50
	const ops = 200

	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < ops; j++ {
				key := (id*ops + j) % 100
				c.Put(key, key*2)
				c.Get(key)
				c.Contains(key)
			}
		}(i)
	}
	wg.Wait()
	// If we reach here without a race detector report, concurrency is fine.
}
