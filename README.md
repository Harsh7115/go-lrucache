# go-lrucache

[![Go Reference](https://pkg.go.dev/badge/github.com/Harsh7115/go-lrucache.svg)](https://pkg.go.dev/github.com/Harsh7115/go-lrucache)
[![License: MIT](https://img.shields.io/badge/License-MIT-green)](LICENSE)

Thread-safe LRU cache in Go with O(1) get/put and optional TTL eviction. Zero external dependencies.

## Install

```bash
go get github.com/Harsh7115/go-lrucache
```

## How It Works

Combines a **doubly linked list** (O(1) eviction order) with a **hash map** (O(1) key lookup):

- `Get` — look up by key, move to front (mark as most recently used)
- `Put` — insert at front; if over capacity, evict the tail (least recently used)

```
Head ←→ [A: most recent] ←→ [B] ←→ [C] ←→ [D: least recent] ←→ Tail
                ▲                                    ▲
            next insert                        next eviction
```

## Usage

```go
import "github.com/Harsh7115/go-lrucache"

// Basic LRU cache — evicts by capacity
cache := lrucache.New(128)

cache.Put("session:abc", sessionData)

val, ok := cache.Get("session:abc")
if ok {
    // cache hit — val is the stored value
}

cache.Delete("session:abc")
fmt.Println(cache.Len())  // current entry count
cache.Clear()             // evict everything
```

## TTL Eviction

Entries can carry an expiry — useful for session caches, API response caches, or anything with a natural freshness window.

```go
// NewWithTTL: entries expire after the given duration
cache := lrucache.NewWithTTL(128, 5*time.Minute)

cache.Put("token:xyz", tokenData)       // expires in 5 minutes
cache.PutTTL("token:abc", data, 30*time.Second) // custom per-entry TTL

val, ok := cache.Get("token:xyz")
// ok == false if the entry has expired, even if still in the cache
```

Expired entries are evicted lazily on access and proactively during `Put` when capacity is exceeded.

## Complexity

| Operation | Time | Space |
|---|---|---|
| Get | O(1) | O(1) |
| Put | O(1) | O(n) |
| Delete | O(1) | O(1) |
| Eviction | O(1) | O(1) |
| TTL expiry check | O(1) | O(1) |

## Benchmarks

```
BenchmarkGet_Hit-8        ~95 ns/op    0 B/op    0 allocs/op
BenchmarkGet_Miss-8       ~45 ns/op    0 B/op    0 allocs/op
BenchmarkPut_NoEvict-8   ~120 ns/op   48 B/op    1 allocs/op
BenchmarkPut_Evict-8     ~135 ns/op   48 B/op    1 allocs/op
```

Measured on Apple M-series, 128-entry cache, string keys, struct values.

## Design

- **Doubly linked list** (`container/list`) for O(1) node moves and tail eviction without scanning
- **`sync.RWMutex`** — multiple concurrent readers, exclusive writer — no lock contention on read-heavy workloads
- `interface{}` value type — store any type with no reflection overhead
- No goroutines or background sweepers — TTL is enforced on access, not via a ticker

## Real-World Use Cases

```go
// HTTP response cache
responseCache := lrucache.NewWithTTL(500, 10*time.Minute)

func cachedFetch(url string) ([]byte, error) {
    if val, ok := responseCache.Get(url); ok {
        return val.([]byte), nil
    }
    data, err := fetch(url)
    if err == nil {
        responseCache.Put(url, data)
    }
    return data, err
}

// Database query cache
queryCache := lrucache.New(1000)

func getUser(id string) (*User, error) {
    if val, ok := queryCache.Get(id); ok {
        return val.(*User), nil
    }
    user, err := db.QueryUser(id)
    if err == nil {
        queryCache.Put(id, user)
    }
    return user, err
}
```

## Tech Stack

Go · `container/list` · `sync`

---

Classic data structure implemented properly for production use — O(1) operations, TTL support, concurrent-safe.
