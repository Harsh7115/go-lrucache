// examples/http_cache.go demonstrates using go-lrucache as an HTTP response
// cache with TTL-based expiration. Repeated requests to the same URL are served
// from the in-process LRU cache, cutting latency from ~200 ms to under 1 ms.
//
// Usage:
//
//	go run ./examples/http_cache.go
//	curl 'http://localhost:8080/weather?city=Phoenix'   # first call: MISS (~200ms)
//	curl 'http://localhost:8080/weather?city=Phoenix'   # second call: HIT (<1ms)
//	curl 'http://localhost:8080/cache/stats'
package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	lrucache "github.com/Harsh7115/go-lrucache"
)

// cachedResponse holds a serialised HTTP response body and its content-type.
type cachedResponse struct {
	Body        []byte
	ContentType string
	CachedAt    time.Time
}

// responseCache wraps an LRU cache keyed by request URL string.
type responseCache struct {
	cache *lrucache.Cache[string, cachedResponse]
	ttl   time.Duration
}

// newResponseCache creates a cache with the given entry capacity and TTL.
func newResponseCache(capacity int, ttl time.Duration) *responseCache {
	c := lrucache.New[string, cachedResponse](
		capacity,
		lrucache.WithTTL[string, cachedResponse](ttl),
		lrucache.WithEvictCallback[string, cachedResponse](func(key string, val cachedResponse) {
			age := time.Since(val.CachedAt).Round(time.Millisecond)
			log.Printf("[cache] evict key=%q  age=%s", key, age)
		}),
	)
	return &responseCache{cache: c, ttl: ttl}
}

// Middleware returns an http.Handler that caches 200-OK responses by URL.
func (rc *responseCache) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.String()

		if entry, ok := rc.cache.Get(key); ok {
			age := time.Since(entry.CachedAt).Round(time.Millisecond)
			w.Header().Set("Content-Type", entry.ContentType)
			w.Header().Set("X-Cache", "HIT")
			w.Header().Set("Age", fmt.Sprintf("%.0f", age.Seconds()))
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(entry.Body)
			log.Printf("[cache] HIT  key=%q  age=%s  bytes=%d", key, age, len(entry.Body))
			return
		}

		// Capture the upstream response so we can store it.
		rec := newRecorder(w)
		next.ServeHTTP(rec, r)

		if rec.status == http.StatusOK && len(rec.body) > 0 {
			rc.cache.Put(key, cachedResponse{
				Body:        append([]byte(nil), rec.body...),
				ContentType: rec.Header().Get("Content-Type"),
				CachedAt:    time.Now(),
			})
			w.Header().Set("X-Cache", "MISS")
			log.Printf("[cache] MISS key=%q  bytes=%d  ttl=%s", key, len(rec.body), rc.ttl)
		}
	})
}

// recorder buffers the response body and status code for inspection.
type recorder struct {
	http.ResponseWriter
	status int
	body   []byte
}

func newRecorder(w http.ResponseWriter) *recorder {
	return &recorder{ResponseWriter: w, status: http.StatusOK}
}

func (r *recorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *recorder) Write(b []byte) (int, error) {
	r.body = append(r.body, b...)
	return r.ResponseWriter.Write(b)
}

// ---------------------------------------------------------------------------
// Demo handlers
// ---------------------------------------------------------------------------

type weatherResponse struct {
	City        string  `json:"city"`
	TempCelsius float64 `json:"temp_celsius"`
	Condition   string  `json:"condition"`
	FetchedAt   string  `json:"fetched_at"`
}

// weatherHandler simulates a slow upstream API call (200 ms latency).
func weatherHandler(w http.ResponseWriter, r *http.Request) {
	city := r.URL.Query().Get("city")
	if city == "" {
		city = "Unknown"
	}
	time.Sleep(200 * time.Millisecond) // simulate network latency

	resp := weatherResponse{
		City:        city,
		TempCelsius: 22.5,
		Condition:   "Partly cloudy",
		FetchedAt:   time.Now().Format(time.RFC3339),
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// statsHandler returns live cache metrics (bypasses the caching middleware).
func statsHandler(rc *responseCache) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"live_entries": rc.cache.Len(),
			"keys_mru":     rc.cache.Keys(),
			"ttl":          rc.ttl.String(),
		})
	}
}

func main() {
	const (
		capacity   = 128
		ttl        = 10 * time.Second
		listenAddr = ":8080"
	)

	cache := newResponseCache(capacity, ttl)

	mux := http.NewServeMux()
	mux.HandleFunc("/weather", weatherHandler)
	mux.HandleFunc("/cache/stats", statsHandler(cache)) // not cached

	// Route /cache/stats directly; everything else goes through the cache middleware.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cache/stats" {
			mux.ServeHTTP(w, r)
			return
		}
		cache.Middleware(mux).ServeHTTP(w, r)
	})

	log.Printf("Listening on %s  capacity=%d  ttl=%s", listenAddr, capacity, ttl)
	log.Println("Try:")
	log.Println("  curl 'http://localhost:8080/weather?city=Phoenix'  # MISS ~200ms")
	log.Println("  curl 'http://localhost:8080/weather?city=Phoenix'  # HIT  <1ms")
	log.Println("  curl 'http://localhost:8080/cache/stats'")

	if err := http.ListenAndServe(listenAddr, handler); err != nil {
		log.Fatal(err)
	}
}
