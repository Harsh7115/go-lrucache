# Benchmarks

This document records the methodology and results for the benchmarks shipped under
`bench/` in this repository. The numbers are intended to be repeatable rather than
flattering — they exist so regressions get caught early, not so the cache "looks fast".

## Methodology

All benchmarks use Go's built-in `testing.B` framework. To reproduce on your own
hardware:

```bash
go test -run=^$ -bench=. -benchmem -benchtime=3s ./bench/...
```

Each benchmark is run with `-benchtime=3s` so the variance settles. We report the
median of five runs. The capacity of the cache under test is fixed at 100,000
entries unless noted, and the working set size is varied to control hit rate.

The numbers below were captured on:

- CPU: Apple M2 Pro, 8 performance cores, 4 efficiency cores
- RAM: 16 GiB LPDDR5
- Go: 1.22.3, GOMAXPROCS=8
- OS: macOS 14.4

## Results

| Benchmark            | Working set | Hit rate | ns/op | allocs/op | B/op |
|----------------------|-------------|----------|-------|-----------|------|
| GetHot               | 1,000       |   100%   |    52 |         0 |    0 |
| GetWarm              | 50,000      |    78%   |    71 |         0 |    0 |
| GetCold              | 1,000,000   |    11%   |   148 |         1 |   24 |
| Put_NoEviction       | 100,000     |     n/a  |    96 |         1 |   48 |
| Put_HeavyEviction    | 1,000,000   |     n/a  |   161 |         1 |   48 |
| Mixed_8020Read       | 100,000     |    92%   |    88 |         0 |    8 |
| Parallel_Read_8core  | 1,000       |   100%   |    14 |         0 |    0 |
| Parallel_Mixed_8core | 100,000     |    91%   |    52 |         1 |   16 |

A few notes on the numbers:

1. `GetHot` is the steady-state lookup of items already in cache. The cost is one
   map lookup, one mutex acquire, and a constant-time list splice; everything else
   is amortized.
2. `GetCold` allocates because the inner `compute` callback is forced to run for
   most lookups. The 24 B/op figure is the heap escape of the returned value, not
   work done by the cache itself.
3. The parallel benchmarks scale roughly linearly to 4 cores and then begin to
   plateau as the single shard mutex becomes a contention point. Sharding is on
   the roadmap (see `docs/roadmap.md`).

## Comparison to other libraries

The intent of this library is to be small and dependency-free, not to win
microbenchmarks. That said, on the `Mixed_8020Read` workload above the steady-state
cost is within ~10% of `hashicorp/golang-lru/v2` at the same capacity. Other LRU
implementations were not benchmarked because their feature sets and concurrency
models differ enough that the comparison is misleading.

## Profiling

To capture a CPU profile while running the benchmarks:

```bash
go test -run=^$ -bench=Mixed -benchtime=10s -cpuprofile=cpu.out ./bench/...
go tool pprof -http=:8080 cpu.out
```

The hottest path tends to be `runtime.mapaccess2` followed by the linked-list
splice in `(*Cache).touch`. Optimizations that reduce either of those should show
up immediately in the `GetHot` numbers.
