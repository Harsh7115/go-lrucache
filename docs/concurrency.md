# Concurrency Model

This document describes how `go-lrucache` is safe to use from multiple
goroutines and where the cost of that safety lives.

## TL;DR

* One `sync.Mutex` per cache. No reader/writer split.
* Every public method takes the lock for the duration of the call.
* The lock is held across map lookups, list mutations, and the optional
  TTL deadline check, but never across user-supplied callbacks.
* No goroutine is spawned by the cache itself. TTL eviction is lazy
  (checked on Get) plus an opt-in `Janitor` you start yourself.

The cache is correct under any concurrent access pattern Go itself
permits, including `-race`. It is not lock-free; if you measure the
mutex as a bottleneck under your workload, see "Sharding" below.

## Why a single Mutex (not RWMutex)

Two reasons:

1. **A Get is a write.** It moves the accessed entry to the head of
   the recency list. That mutation makes `sync.RWMutex` actively
   harmful — `RLock` would not be safe, and a Get path that promotes
   itself to a write lock just adds overhead.
2. **The critical section is short.** A typical Get is a map lookup
   (~25 ns), a doubly-linked-list splice (~10 ns), and a deadline
   compare (~3 ns). That is well below the cost of contended RWMutex
   bookkeeping.

If your access pattern is read-heavy and you can tolerate stale recency
information (i.e. you're really after a TTL cache, not an LRU), build
on `sync.Map` directly — `go-lrucache` is not the right tool.

## Invariants protected by the lock

While the lock is held, the cache maintains:

| Invariant | Where checked |
|---|---|
| `len(c.m) == c.list.Len()` | every mutating method |
| Each `*entry` is in exactly one list, exactly once | `moveToFront`, `pushFront`, `remove` |
| The head of the list is the most recently touched entry | `Get`, `Put` |
| `size <= capacity` after every Put | `Put` (eviction loop) |
| Expired entries are reported as misses | `Get` (deadline check) |

Violating any of these would corrupt the cache silently, which is why
all of them are asserted by the test suite under `-race`.

## What the lock does *not* cover

User-supplied callbacks (`OnEvict`, value constructors used by
`GetOrCompute`) run *outside* the lock. The contract is:

* `OnEvict(key, value)` is called after the entry has been removed
  from the map and the list.
* The callback may call back into the same cache without deadlocking.
* The cache makes no ordering guarantee between concurrent
  `OnEvict` callbacks.

If your callback needs to see a coherent snapshot of the cache, take
your own lock around it — the cache will not.

## TTL eviction

`go-lrucache` does **not** start a background goroutine. There are two
paths by which entries leave the cache when they expire:

1. **Lazy.** `Get(key)` checks the deadline and treats an expired
   entry as a miss, removing it inline.
2. **Active (opt-in).** `Janitor(ctx, interval)` runs in the caller's
   goroutine and walks the list every `interval`, removing expired
   entries from the tail.

Lazy is enough for most workloads — entries that are never read again
will eventually be evicted by capacity pressure regardless of TTL.
Active sweeping matters only when (a) TTL is much shorter than the
working set churns, and (b) memory held by stale entries is itself a
problem.

## Sharding

For workloads where the single mutex is contended (typically: hot caches
hit by hundreds of goroutines per second), wrap the basic cache in a
shard set:

```go
type ShardedCache[K comparable, V any] struct {
    shards [N]*Cache[K, V]
    hash   func(K) uint64
}

func (s *ShardedCache[K, V]) Get(k K) (V, bool) {
    return s.shards[s.hash(k)%N].Get(k)
}
```

A power-of-two number of shards keyed off a fast 64-bit hash (xxhash,
fnv-1a) is enough. `N=16` is a good default; doubling beyond `N=64`
typically buys nothing. Sharding is intentionally outside the core API
because the right hash and shard count depend on the key type, and
hard-coding them would surprise people in micro-benchmarks.

## Read-modify-write helpers

`GetOrCompute` and `Update` are convenience wrappers that perform a
read and a write inside the same critical section. They exist because
the natural Go idiom

```go
v, ok := c.Get(k)
if !ok {
    v = expensive()
    c.Put(k, v)
}
```

races: two goroutines can both miss, both call `expensive()`, and one
of the results is silently dropped. `GetOrCompute` performs the miss,
the construction, and the insertion atomically with respect to the
cache (the constructor itself still runs outside the lock — see
"What the lock does not cover").
