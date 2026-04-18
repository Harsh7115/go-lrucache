# Design Notes

This document describes the internal design of `go-lrucache` — how the
cache actually works, why the data structures were chosen, and what the
trade-offs are. If you just want to *use* the cache, the README is a
better starting point; this file is for anyone modifying the code or
comparing `go-lrucache` to other implementations.

## Goals

- **O(1) Get / Put / Delete** for the common path. No linear scans, no
  amortised cost that occasionally spikes.
- **Thread-safe by default.** A correctly-used cache should never require
  the caller to hold an external mutex.
- **Generic keys and values** via Go 1.18+ type parameters, so the same
  cache works for `string -> []byte`, `int -> User`, etc., without
  `interface{}` boxing.
- **Optional TTL** that does not force a background goroutine on callers
  who do not need expiration.
- **Zero third-party dependencies** — this is a standard-library-only
  project.

## Data structures

Internally the cache combines two classic structures:

1. A doubly-linked list of entries, ordered from most-recently-used
   (front) to least-recently-used (back).
2. A map from key to `*list.Element`, giving O(1) lookup of the list
   node for a given key.

On `Get`, we take the lock, look up the map, and if present, move the
node to the front of the list. On `Put`, we either update and move the
existing node, or insert a new node at the front and evict the tail if
we are over capacity. All list surgery is local — there is no recursive
rebalancing, no allocation on the hot path (the `list.Element` is
allocated once per unique key and reused).

### Why `container/list` instead of a hand-rolled list?

Early versions of this library used a custom intrusive list for the
supposed performance win. Microbenchmarks showed the gain was below 3%
in realistic workloads, and the cost was a lot more code plus worse
escape-analysis behaviour. `container/list` is now used unapologetically.

## Locking strategy

A single `sync.Mutex` guards the map and the list. A `sync.RWMutex`
was tried and rejected: nearly every operation — including `Get` —
writes to the list (to bump recency), so read-mostly locks degenerate to
a write-lock pattern anyway. The extra bookkeeping of RWMutex made the
common case 10–15% slower in the `BenchmarkGet` suite.

For higher concurrency, callers can shard the cache (see
`examples/advanced_usage.go`): N independent caches keyed by
`hash(key) % N`. A built-in sharded wrapper is on the roadmap but not
yet exposed.

## TTL implementation

TTL is opt-in per `Put`. Entries carry a `deadline time.Time` (or
a zero value meaning "never"). We do two things to avoid a background
goroutine:

1. On `Get`, expired entries are lazily evicted and reported as a miss.
2. `Cache.Purge()` can be called by the user on any cadence they like
   to evict all expired entries in one pass.

If you want active expiration (a background goroutine that sweeps), you
can wrap the cache with a 10-line helper that calls `Purge` on a
`time.Ticker`. We deliberately don't do this inside the library: users
who don't need TTL at all should not pay for a goroutine.

## Why not `sync.Map`?

`sync.Map` is designed for read-heavy, append-only-ish workloads.
An LRU cache's most common operation (`Get` with recency bump) writes
to shared state, which defeats `sync.Map`'s optimistic path. In
benchmarks against a `sync.Map`-backed prototype, this library was 2–3×
faster on mixed Get/Put workloads.

## What we gave up

- **Strict LRU ordering under contention.** Under very high contention
  on a small number of hot keys, two Gets for the same key from two
  goroutines may interleave such that the final list order is not what
  either goroutine "saw". This does not corrupt the cache — it just
  means the eviction order is slightly fuzzy. In exchange we keep the
  hot path lock-short (one `MoveToFront` call).
- **Size-aware eviction.** The cache bounds by *count*, not bytes. An
  LFU- or size-aware variant is a different structure; a good addition
  but a separate package.

## Testing notes

- `lru_test.go` covers basic semantics: insertion, eviction order,
  TTL expiry, and concurrency.
- `lru_bench_test.go` covers performance: Get hit, Get miss, Put new,
  Put existing, and a 50/50 mix.
- The race detector is on by default in CI. Every merged change must
  pass `go test -race ./...`.

## Open design questions

- Should `Put` return the evicted value (if any)? Useful for "write
  through" caches that want to write evictions to a backing store.
  Currently it returns only a `bool` indicating eviction occurred.
- Is `OnEvict` callback worth adding? Low cost but widens the API.
  Tracking in issue #14.
