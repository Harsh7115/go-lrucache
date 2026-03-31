# Changelog

All notable changes to **go-lrucache** are documented here.
The format follows [Keep a Changelog](https://keepachangelog.com/en/1.0.0/)
and the project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

---

## [Unreleased]

### Planned
- Background TTL reaper goroutine (opt-in via `WithReaper` option)
- Prometheus metrics integration (`WithMetrics` option)
- Weighted capacity support (evict by cost, not just count)
- `GetOrSet` atomic read-modify-write helper

---

## [1.1.0] – 2026-03-27

### Added
- `Resize(newCap int) int` — dynamically change the cache capacity at runtime.
  Returns the number of entries evicted to satisfy the new limit.
- `Peek(key K) (V, bool)` — fetch a value without updating LRU order or
  resetting a TTL clock. Useful for observability / logging sidecars.
- `Contains(key K) bool` — existence check that does **not** promote the entry
  to MRU. Cheaper than `Get` when the caller only needs presence information.
- `Keys() []K` — snapshot of all live (non-expired) keys in MRU → LRU order.
- Evict callback support via `WithEvictCallback[K, V](fn)`. The callback fires
  for both capacity-driven evictions and TTL expiry, letting callers hook into
  lifecycle events (e.g. write-through cache invalidation).

### Changed
- `New[K, V]` now accepts a variadic `...Option[K, V]` parameter. Existing
  call-sites that pass only a capacity are fully backward-compatible.

### Fixed
- Race condition where a concurrent `Put` and `Delete` on the same key could
  temporarily leave a stale list node in the eviction queue. All exported
  methods are now fully serialised via a single `sync.Mutex`.

---

## [1.0.1] – 2026-03-15

### Fixed
- `Len()` incorrectly counted expired entries that had not yet been lazily
  evicted. It now walks live entries only.
- `Purge()` did not invoke the evict callback. Callers relying on the callback
  for write-through invalidation would silently miss a full-cache flush.

### Performance
- Replaced `time.Now()` calls inside the hot `Get` path with a single call
  cached at the top of the function, reducing syscall overhead on TTL-heavy
  workloads by ~8 % on `BenchmarkGetHit`.

---

## [1.0.0] – 2026-03-01

### Added
- Initial release of `go-lrucache`.
- Generic `Cache[K comparable, V any]` backed by a doubly-linked list + hash
  map, giving O(1) `Get`, `Put`, and `Delete`.
- Optional per-entry TTL via `WithTTL[K, V](d time.Duration)`. Expiry is
  enforced lazily on access — no background goroutine required.
- Thread-safe by design: a single `sync.RWMutex` guards all state.
- `Purge()` to atomically clear the entire cache.
- Full test suite runnable with `go test -race ./...`.
- MIT licence.

---

## Versioning policy

| Change type | Version bump |
|---|---|
| New public API, backward-compatible | Minor (1.**x**.0) |
| Bug fix, no API change | Patch (1.0.**x**) |
| Breaking API change | Major (**x**.0.0) |

Versions are tagged as `vX.Y.Z` in Git and published to
[pkg.go.dev](https://pkg.go.dev/github.com/Harsh7115/go-lrucache).
