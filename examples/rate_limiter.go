// examples/rate_limiter.go
//
// Demonstrates using go-lrucache as the backing store for a fixed-window
// per-IP rate limiter served over HTTP.
//
// Each IP address gets a counter entry in the LRU cache. The entry TTL is
// set to the window duration, so the counter resets automatically — no
// background goroutine required.
//
// Run:
//   go run examples/rate_limiter.go
// Then in another terminal:
//   for i in $(seq 1 12); do curl -s http://localhost:8080/; echo; done

package main

import (
	"fmt"
	"log"
	"net"
	"net/http"
	"strconv"
	"sync"
	"time"

	lrucache "github.com/Harsh7115/go-lrucache"
)

// ---------------------------------------------------------------------------
// windowEntry tracks request count within the current fixed window.
// ---------------------------------------------------------------------------

type windowEntry struct {
	mu    sync.Mutex
	count int
}

// RateLimiter is a fixed-window, per-key rate limiter backed by an LRU cache.
// When an entry expires, the next request for that key starts a fresh window.
type RateLimiter struct {
	cache  *lrucache.Cache[string, *windowEntry]
	limit  int
	window time.Duration
}

// NewRateLimiter creates a limiter allowing at most limit requests per window
// for any single key, tracking up to maxKeys distinct keys.
func NewRateLimiter(maxKeys, limit int, window time.Duration) *RateLimiter {
	c := lrucache.New[string, *windowEntry](
		maxKeys,
		lrucache.WithTTL[string, *windowEntry](window),
	)
	return &RateLimiter{cache: c, limit: limit, window: window}
}

// Allow returns (allowed, remaining, resetAfter).
//   - allowed:    true if the request is within the rate limit.
//   - remaining:  requests left in the current window.
//   - resetAfter: approximate time until the window resets.
func (rl *RateLimiter) Allow(key string) (bool, int, time.Duration) {
	entry, ok := rl.cache.Get(key)
	if !ok {
		entry = &windowEntry{}
		rl.cache.Put(key, entry)
	}

	entry.mu.Lock()
	defer entry.mu.Unlock()

	entry.count++

	if entry.count > rl.limit {
		return false, 0, rl.window
	}
	return true, rl.limit - entry.count, rl.window
}

// ---------------------------------------------------------------------------
// HTTP handler
// ---------------------------------------------------------------------------

const (
	maxIPs     = 10000       // max distinct IPs tracked
	rateLimit  = 10          // requests per window
	rateWindow = time.Minute // window duration
)

func makeHandler(limiter *RateLimiter) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		// Extract the client IP (strip port).
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}

		allowed, remaining, reset := limiter.Allow(ip)

		w.Header().Set("X-RateLimit-Limit", strconv.Itoa(rateLimit))
		w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
		w.Header().Set("X-RateLimit-Reset",
			strconv.FormatInt(time.Now().Add(reset).Unix(), 10))

		if !allowed {
			w.Header().Set("Retry-After", strconv.Itoa(int(reset.Seconds())))
			http.Error(w, "429 Too Many Requests", http.StatusTooManyRequests)
			return
		}

		fmt.Fprintf(w, "Hello %s! Requests remaining this window: %d\n", ip, remaining)
	}
}

// ---------------------------------------------------------------------------
// main
// ---------------------------------------------------------------------------

func main() {
	limiter := NewRateLimiter(maxIPs, rateLimit, rateWindow)

	http.HandleFunc("/", makeHandler(limiter))

	addr := ":8080"
	log.Printf("rate-limiter demo: listening on %s", addr)
	log.Printf("policy: %d req / %s / IP", rateLimit, rateWindow)
	if err := http.ListenAndServe(addr, nil); err != nil {
		log.Fatal(err)
	}
}
