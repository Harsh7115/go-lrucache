// db_query_cache.go demonstrates using go-lrucache as a read-through cache
// in front of a simulated SQL database.
//
// Pattern: "cache-aside" — the application checks the cache first; on a miss
// it queries the DB, stores the result, and returns it.  A short TTL keeps
// the cache from serving stale rows for too long without requiring explicit
// invalidation logic.
//
// Run:
//   go run db_query_cache.go

package main

import (
	"errors"
	"fmt"
	"log"
	"math/rand"
	"sync"
	"time"

	lrucache "github.com/Harsh7115/go-lrucache"
)

// ─── Domain model ─────────────────────────────────────────────────────────────

// User is a row returned by the users table.
type User struct {
	ID        int
	Username  string
	Email     string
	CreatedAt time.Time
}

func (u User) String() string {
	return fmt.Sprintf("User{id=%d name=%q email=%q}", u.ID, u.Username, u.Email)
}

// ─── Simulated database ───────────────────────────────────────────────────────

// FakeDB simulates a slow SQL database.
type FakeDB struct {
	mu      sync.Mutex
	queries int // total query count for stats
	rows    map[int]User
}

func newFakeDB() *FakeDB {
	db := &FakeDB{rows: make(map[int]User)}
	// Seed some rows.
	names := []string{"alice", "bob", "carol", "dave", "eve", "frank", "grace"}
	for i, name := range names {
		db.rows[i+1] = User{
			ID:        i + 1,
			Username:  name,
			Email:     name + "@example.com",
			CreatedAt: time.Now().Add(-time.Duration(rand.Intn(365)) * 24 * time.Hour),
		}
	}
	return db
}

// GetUserByID simulates a SELECT with ~20 ms network/disk latency.
func (db *FakeDB) GetUserByID(id int) (User, error) {
	time.Sleep(20 * time.Millisecond) // simulate latency
	db.mu.Lock()
	db.queries++
	db.mu.Unlock()

	user, ok := db.rows[id]
	if !ok {
		return User{}, fmt.Errorf("user %d not found", id)
	}
	return user, nil
}

func (db *FakeDB) TotalQueries() int {
	db.mu.Lock()
	defer db.mu.Unlock()
	return db.queries
}

// ─── Cached user repository ───────────────────────────────────────────────────

// UserRepo wraps FakeDB with an LRU read-through cache.
type UserRepo struct {
	db    *FakeDB
	cache *lrucache.Cache[int, User]

	hits   int
	misses int
	mu     sync.Mutex
}

func newUserRepo(db *FakeDB, capacity int, ttl time.Duration) *UserRepo {
	cache := lrucache.New[int, User](
		capacity,
		lrucache.WithTTL[int, User](ttl),
		lrucache.WithEvictCallback[int, User](func(id int, u User) {
			log.Printf("[cache] evicted user id=%d (%s)", id, u.Username)
		}),
	)
	return &UserRepo{db: db, cache: cache}
}

// GetByID fetches a user from the cache; falls through to the DB on a miss.
func (r *UserRepo) GetByID(id int) (User, error) {
	if user, ok := r.cache.Get(id); ok {
		r.mu.Lock()
		r.hits++
		r.mu.Unlock()
		return user, nil
	}

	// Cache miss — query the database.
	user, err := r.db.GetUserByID(id)
	if err != nil {
		if errors.Is(err, fmt.Errorf("user %d not found", id)) {
			// Don't cache negative results in this simple example.
		}
		return User{}, err
	}

	r.cache.Put(id, user)

	r.mu.Lock()
	r.misses++
	r.mu.Unlock()

	return user, nil
}

// Invalidate removes a user from the cache (call after an UPDATE/DELETE).
func (r *UserRepo) Invalidate(id int) {
	r.cache.Delete(id)
	log.Printf("[cache] invalidated user id=%d", id)
}

func (r *UserRepo) Stats() (hits, misses int, cacheSize int) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.hits, r.misses, r.cache.Len()
}

// ─── Demo ─────────────────────────────────────────────────────────────────────

func main() {
	const (
		cacheCapacity = 5   // small capacity to force LRU evictions
		cacheTTL      = 2 * time.Second
	)

	db := newFakeDB()
	repo := newUserRepo(db, cacheCapacity, cacheTTL)

	fmt.Println("=== Round 1: cold cache (all misses) ===")
	for id := 1; id <= 5; id++ {
		u, err := repo.GetByID(id)
		if err != nil {
			log.Printf("error: %v", err)
			continue
		}
		fmt.Printf("  fetched %s
", u)
	}

	fmt.Println()
	fmt.Println("=== Round 2: warm cache (all hits, no DB queries) ===")
	for id := 1; id <= 5; id++ {
		u, err := repo.GetByID(id)
		if err != nil {
			log.Printf("error: %v", err)
			continue
		}
		fmt.Printf("  fetched %s
", u)
	}

	fmt.Println()
	fmt.Println("=== Round 3: overflow — fetching id=6 evicts LRU entry ===")
	u, _ := repo.GetByID(6)
	fmt.Printf("  fetched %s
", u)
	// id=1 was LRU; next access will be a cache miss.
	u, _ = repo.GetByID(1)
	fmt.Printf("  re-fetched %s (was evicted, so DB hit)
", u)

	fmt.Println()
	fmt.Println("=== Round 4: explicit invalidation ===")
	repo.Invalidate(3)
	u, _ = repo.GetByID(3) // must go to DB
	fmt.Printf("  re-fetched %s after invalidation
", u)

	fmt.Println()
	fmt.Println("=== Round 5: TTL expiry ===")
	fmt.Printf("  sleeping %s to let TTL expire...
", cacheTTL)
	time.Sleep(cacheTTL + 50*time.Millisecond)
	u, _ = repo.GetByID(2) // cache expired, must re-query DB
	fmt.Printf("  re-fetched %s after TTL expiry
", u)

	// ─── Final stats ──────────────────────────────────────────────────────
	hits, misses, size := repo.Stats()
	total := hits + misses
	ratio := 0.0
	if total > 0 {
		ratio = float64(hits) / float64(total) * 100
	}

	fmt.Println()
	fmt.Println("=== Stats ===")
	fmt.Printf("  cache hits:     %d
", hits)
	fmt.Printf("  cache misses:   %d
", misses)
	fmt.Printf("  hit ratio:      %.1f%%
", ratio)
	fmt.Printf("  db queries:     %d
", db.TotalQueries())
	fmt.Printf("  cache size now: %d
", size)
}
