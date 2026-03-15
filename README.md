# go-lrucache

A thread-safe, generic LRU (Least-Recently-Used) cache for Go 1.22+.

- **O(1)** `Get`, `Put`, and `Delete` via doubly linked list + hash map
- **Generics** — any comparable key type, any value type
- **Optional per-entry TTL** — expired entries are evicted lazily on access
- **Evict callback** — hook into every eviction (capacity or TTL)
- **Safe for concurrent use** — all exported methods are mutex-protected

## Installation

```bash
go get github.com/Harsh7115/go-lrucache
```

## Quick start

```go
package main

import (
    "fmt"
    "time"

    lrucache "github.com/Harsh7115/go-lrucache"
)

func main() {
    // 128-entry cache, entries expire after 5 minutes
    c := lrucache.New[string, int](128,
        lrucache.WithTTL[string, int](5*time.Minute),
        lrucache.WithEvictCallback[string, int](func(k string, v int) {
            fmt.Printf("evicted %s=%d\n", k, v)
        }),
    )

    c.Put("answer", 42)

    if v, ok := c.Get("answer"); ok {
        fmt.Println(v) // 42
    }

    fmt.Println(c.Len()) // 1
}
```

## API

| Method | Description |
|---|---|
| `New[K, V](cap, ...opts)` | Create a cache with the given capacity |
| `Put(key, value)` | Insert or update; evicts LRU entry when at capacity |
| `Get(key) (V, bool)` | Fetch and promote to MRU; returns false if missing or expired |
| `Delete(key) bool` | Remove a key; returns true if it was present |
| `Contains(key) bool` | Existence check without updating LRU order |
| `Peek(key) (V, bool)` | Fetch without updating LRU order or resetting TTL |
| `Len() int` | Number of live (non-expired) entries |
| `Keys() []K` | All live keys in MRU → LRU order |
| `Purge()` | Remove all entries (calls evict callback for each) |
| `Resize(newCap) int` | Change capacity; returns number of entries evicted |

### Options

```go
lrucache.WithTTL[K, V](d time.Duration)           // per-entry time-to-live
lrucache.WithEvictCallback[K, V](fn EvictCallback) // called on every eviction
```

## Design

```
  MRU ←─────────────────────────── LRU
  ┌───┐   ┌───┐   ┌───┐   ┌───┐
  │ c │ ↔ │ a │ ↔ │ b │ ↔ │ d │   doubly-linked list
  └───┘   └───┘   └───┘   └───┘
    ↑                         ↑
  Put/Get                  evicted first
  promotes here

  items map[K]*list.Element → O(1) lookup
```

TTL eviction is **lazy**: expired entries are detected and removed the next time they are accessed via `Get`, `Contains`, `Peek`, or `Keys`. No background goroutine is required.

## Running tests

```bash
go test -race ./...
```
