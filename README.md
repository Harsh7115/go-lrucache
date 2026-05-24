# go-lrucache

A **thread-safe LRU (Least Recently Used) cache** in Go with O(1) get and put operations and zero external dependencies.

## How It Works

Combines a **doubly linked list** (for O(1) eviction order) with a **hash map** (for O(1) key lookup):

- `Get` — look up by key, move node to front (most recently used)
- `Put` — insert at front; if capacity exceeded, evict the tail (least recently used)

```
Head ←→ [most recent] ←→ ... ←→ [least recent] ←→ Tail
          ↑                                          ↑
       next Put                               next eviction
```

## Usage

```go
import "github.com/Harsh7115/go-lrucache"

cache := lrucache.New(128) // capacity 128

cache.Put("user:42", userData)

val, ok := cache.Get("user:42")
if ok {
    // cache hit
}

cache.Delete("user:42")
cache.Len()   // current size
cache.Clear() // evict all
```

## Install

```bash
go get github.com/Harsh7115/go-lrucache
```

## Complexity

| Operation | Time | Space |
|-----------|------|-------|
| Get | O(1) | O(1) |
| Put | O(1) | O(n) |
| Delete | O(1) | O(1) |
| Eviction | O(1) | O(1) |

## Design

- **Doubly linked list** for O(1) node moves and tail eviction
- **`sync.RWMutex`** — concurrent reads, exclusive writes
- Generic value type (`interface{}`) — store any type without reflection overhead
- No goroutines or background threads — deterministic, zero overhead when idle

## Tech Stack

Go · `container/list` · `sync`

---

Classic data structure interview problem — implemented properly for production use.
