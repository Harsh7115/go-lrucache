# Eviction Policy Deep Dive

This document explains the two eviction mechanisms in **go-lrucache** — LRU
order eviction and TTL-based expiry — and describes how they interact, their
performance characteristics, and guidance on tuning them for common workloads.

---

## 1. LRU Order Eviction

### How it works

The cache maintains a doubly-linked list of all entries ordered by recency of
access.  The most-recently used (MRU) entry sits at the head; the
least-recently used (LRU) entry sits at the tail.

On every `Get` hit the accessed node is unlinked from its current position and
spliced back at the head.  On every `Put`:

1. If the key already exists, update the value and move to head.
2. If the key is new and `len < capacity`, insert at head.
3. If the key is new and `len == capacity`, **evict the tail** first, then
   insert the new entry at head.

A companion `map[K]*node` provides O(1) key lookup so neither `Get` nor `Put`
needs to walk the list.

### Complexity

| Operation | Time  | Notes |
|-----------|-------|-------|
| `Get`     | O(1)  | map lookup + pointer splice |
| `Put`     | O(1)  | map insert/update + pointer splice + optional tail eviction |
| `Delete`  | O(1)  | map delete + pointer splice |
| `Peek`    | O(1)  | map lookup, **no** recency update |

Memory per entry is approximately `2 pointers (prev/next) + key + value`.
For small generic types this is typically 48–64 bytes per entry on amd64.

### When to use capacity-only eviction

- **Bounded working sets**: you know the hot set fits in N entries and items
  never go stale (e.g. compiled regex, parsed config).
- **High churn + skewed access**: LRU naturally keeps frequently accessed
  entries alive with zero extra bookkeeping.
- **Memory-sensitive environments**: a fixed capacity gives a hard upper bound
  on memory usage.

---

## 2. TTL-Based Expiry

### How it works

When the cache is created with a non-zero TTL, each entry stores a
`time.Time` deadline.  Entries are considered **expired** once
`time.Now().After(entry.expiresAt)`.

Expiry is **lazy**: the library does not run a background goroutine.
Instead, an expired entry is detected and discarded at the point of access:

```go
// simplified from lru.go
func (c *Cache[K, V]) Get(key K) (V, bool) {
    c.mu.Lock()
    defer c.mu.Unlock()

    node, ok := c.index[key]
    if !ok {
        return zero[V](), false
    }
    if c.ttl > 0 && time.Now().After(node.expiresAt) {
        c.removeNode(node)   // evict expired entry
        return zero[V](), false
    }
    c.moveToFront(node)
    return node.value, true
}
```

`Put` also resets the deadline on every write, so a re-inserted key gets a
fresh TTL window regardless of how stale the previous entry was.

### Passive vs. active expiry

| Strategy | Pro | Con |
|----------|-----|-----|
| **Lazy (this library)** | zero goroutines, zero background CPU | stale entries occupy memory until accessed |
| Active (ticker sweep) | memory freed promptly | background goroutine, periodic lock contention |

The lazy approach is the right default for most caches because:

- The working set is accessed regularly, so stale entries evict quickly.
- Avoiding a background goroutine simplifies the lifecycle (no `Stop()`
  needed, no goroutine leak on GC).

If you need prompt eviction (e.g. security tokens that must not be readable
after expiry even if never accessed again), run a sweep loop externally:

```go
go func() {
    ticker := time.NewTicker(ttl / 2)
    defer ticker.Stop()
    for range ticker.C {
        cache.Purge() // or iterate keys with Peek
    }
}()
```

---

## 3. Interaction Between LRU and TTL

When both capacity and TTL are set, the following ordering applies:

1. **TTL check comes first.** A `Get` on an expired key returns a miss and
   removes the entry — even if that key would have been at the MRU head.
2. **Capacity eviction uses the post-expiry list.** After an expired entry is
   lazily removed, the effective capacity increases by one, potentially
   deferring LRU eviction of a valid entry.

### Example timeline

```
capacity=3, TTL=5s

t=0  Put("a", 1)  Put("b", 2)  Put("c", 3)   list: [c, b, a]
t=3  Get("a")      -- hit, move to front       list: [a, c, b]
t=6  Get("b")      -- expired (>5s), evict b   list: [a, c]
t=6  Put("d", 4)   -- capacity=3, len=2, no LRU eviction needed
                                                list: [d, a, c]
t=6  Put("e", 5)   -- capacity=3, len=3, evict LRU tail (c)
                                                list: [e, d, a]
```

---

## 4. Choosing the Right Parameters

| Workload | Recommended settings |
|----------|----------------------|
| Session cache (auth tokens) | Small capacity + TTL matching session lifetime |
| DNS response cache | Moderate capacity + TTL matching record TTL |
| DB query result cache | Large capacity + short TTL (data freshness) |
| Compiled template cache | Large capacity + no TTL (templates never change) |
| Per-user rate-limit state | Capacity = expected concurrent users + TTL = window |

### Sizing capacity

A good starting point is to benchmark your hit rate with
`go test -bench=BenchmarkHitRate -benchmem` and plot hit rate vs. capacity.
The curve typically has a sharp knee; setting capacity at 110–120% of the
knee point balances memory usage against hit rate.

---

## 5. Thread Safety

All eviction logic runs under a single `sync.Mutex`.  For workloads with very
high concurrency (>10k goroutines contending on a single cache) consider
sharding: create N independent caches and route keys by `hash(key) % N`.
See `examples/advanced_usage.go` for a reference sharded implementation.

---

## See Also

- `docs/DESIGN.md` — overall architecture and data-structure choices
- `docs/concurrency.md` — lock granularity and shard patterns
- `docs/benchmarks.md` — benchmark results across Go versions and CPU counts
- `examples/db_query_cache.go` — real-world TTL cache usage
