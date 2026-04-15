// dns_cache.go — Example: DNS resolver cache using go-lrucache
//
// Demonstrates using an LRU cache with TTL to build a minimal stub DNS
// resolver cache. Real DNS records have a TTL (time-to-live) field;
// the cache's per-entry TTL maps naturally to that concept.
//
// Run with:
//   go run ./examples/dns_cache.go

package main

import (
	"context"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	lrucache "github.com/Harsh7115/go-lrucache"
)

// ---------------------------------------------------------------------------
// DNSRecord holds the resolved addresses and the TTL from the upstream answer.
// ---------------------------------------------------------------------------

type DNSRecord struct {
	Addrs     []string
	ResolvedAt time.Time
	TTL       time.Duration
}

// ---------------------------------------------------------------------------
// DNSCache wraps an LRU cache and adds hit/miss counters for observability.
// ---------------------------------------------------------------------------

type DNSCache struct {
	cache  *lrucache.Cache[string, DNSRecord]
	hits   atomic.Int64
	misses atomic.Int64
}

// NewDNSCache creates a resolver cache that holds at most maxEntries hostnames.
// Each cached entry expires after defaultTTL even if accessed repeatedly.
func NewDNSCache(maxEntries int, defaultTTL time.Duration) *DNSCache {
	c := lrucache.New[string, DNSRecord](
		maxEntries,
		lrucache.WithTTL[string, DNSRecord](defaultTTL),
		lrucache.WithEvictCallback[string, DNSRecord](func(host string, rec DNSRecord) {
			fmt.Printf("[cache] evicted %s (resolved %s ago)\n",
				host, time.Since(rec.ResolvedAt).Round(time.Millisecond))
		}),
	)
	return &DNSCache{cache: c}
}

// Resolve returns the IP addresses for host, using the cache when possible.
// On a cache miss it performs a real DNS lookup via net.DefaultResolver.
func (d *DNSCache) Resolve(ctx context.Context, host string) ([]string, error) {
	// Normalise: strip trailing dot, lowercase
	host = strings.ToLower(strings.TrimSuffix(host, "."))

	if rec, ok := d.cache.Get(host); ok {
		d.hits.Add(1)
		return rec.Addrs, nil
	}

	d.misses.Add(1)

	// Cache miss — resolve upstream
	addrs, err := net.DefaultResolver.LookupHost(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("dns lookup %s: %w", host, err)
	}

	rec := DNSRecord{
		Addrs:      addrs,
		ResolvedAt: time.Now(),
		TTL:        60 * time.Second, // in real code: parse from DNS answer
	}
	d.cache.Put(host, rec)
	return addrs, nil
}

// Stats returns a formatted hit/miss summary.
func (d *DNSCache) Stats() string {
	h := d.hits.Load()
	m := d.misses.Load()
	total := h + m
	hitRate := 0.0
	if total > 0 {
		hitRate = float64(h) / float64(total) * 100
	}
	return fmt.Sprintf("lookups=%d hits=%d misses=%d hit_rate=%.1f%% cached=%d",
		total, h, m, hitRate, d.cache.Len())
}

// ---------------------------------------------------------------------------
// main: demo with a small set of well-known hostnames
// ---------------------------------------------------------------------------

func main() {
	// Tiny cache — capacity 4, entries expire after 5 seconds
	resolver := NewDNSCache(4, 5*time.Second)

	hosts := []string{
		"google.com",
		"cloudflare.com",
		"github.com",
		"golang.org",
	}

	ctx := context.Background()
	var wg sync.WaitGroup

	fmt.Println("=== Round 1: cold cache (all misses) ===")
	for _, h := range hosts {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			addrs, err := resolver.Resolve(ctx, host)
			if err != nil {
				fmt.Printf("  ERROR %s: %v\n", host, err)
				return
			}
			fmt.Printf("  %s -> %v\n", host, addrs[:min(2, len(addrs))])
		}(h)
	}
	wg.Wait()
	fmt.Println(" ", resolver.Stats())

	fmt.Println()
	fmt.Println("=== Round 2: warm cache (all hits) ===")
	for _, h := range hosts {
		wg.Add(1)
		go func(host string) {
			defer wg.Done()
			addrs, err := resolver.Resolve(ctx, host)
			if err != nil {
				fmt.Printf("  ERROR %s: %v\n", host, err)
				return
			}
			fmt.Printf("  %s -> %v (cached)\n", host, addrs[:min(2, len(addrs))])
		}(h)
	}
	wg.Wait()
	fmt.Println(" ", resolver.Stats())

	fmt.Println()
	fmt.Println("=== Round 3: overflow — add 5th host, LRU entry evicted ===")
	_, _ = resolver.Resolve(ctx, "pkg.go.dev")
	fmt.Println(" ", resolver.Stats())

	fmt.Println()
	fmt.Println("=== Round 4: wait for TTL expiry (6s) ===")
	fmt.Println("  sleeping 6s...")
	time.Sleep(6 * time.Second)
	for _, h := range hosts {
		_, _ = resolver.Resolve(ctx, h)
	}
	fmt.Println("  All entries expired and re-resolved.")
	fmt.Println(" ", resolver.Stats())
}

// min is a small helper for Go versions before 1.21's built-in min.
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
