# Contributing

Pull requests are welcome.

## Adding Features

Common extensions worth adding:
- **Generic type parameter** — migrate from `interface{}` to `T any` (Go 1.18+)
- **Stats** — hit/miss counters, eviction count
- **Shard locking** — split the cache into N shards to reduce mutex contention under high concurrency

## Running Tests

```bash
go test ./...
go test -bench=. -benchmem ./...
go test -race ./...   # verify no data races
```

## Code Style

- `gofmt` before committing
- Keep concurrency properties documented on any exported method you add
- No external dependencies
