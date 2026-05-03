// Package main demonstrates using go-lrucache as a feature-flag evaluation
// cache. Feature-flag systems typically receive a (userID, flagKey) pair and
// return a boolean (or variant). Re-evaluating from a remote service every
// request is expensive; caching results with a short TTL gives correctness
// while keeping the hot path local.
//
// Run with:  go run examples/feature_flag_cache.go
package main

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	lru "github.com/Harsh7115/go-lrucache"
)

// FlagDecision is what the upstream evaluator returns.
type FlagDecision struct {
	Enabled bool
	Variant string
	Reason  string
}

// flagKey is the composite cache key.
type flagKey struct {
	UserID string
	Flag   string
}

// FlagEvaluator is the upstream service interface.
type FlagEvaluator interface {
	Evaluate(ctx context.Context, userID, flag string) (FlagDecision, error)
}

// fakeEvaluator simulates a slow remote flag service.
type fakeEvaluator struct {
	calls atomic.Int64
}

func (f *fakeEvaluator) Evaluate(_ context.Context, userID, flag string) (FlagDecision, error) {
	f.calls.Add(1)
	// Simulate network latency.
	time.Sleep(20 * time.Millisecond)
	return FlagDecision{
		Enabled: (len(userID)+len(flag))%2 == 0,
		Variant: "control",
		Reason:  "default-rule",
	}, nil
}

// CachedFlagEvaluator wraps a FlagEvaluator with an LRU+TTL cache and
// single-flight to collapse concurrent identical lookups.
type CachedFlagEvaluator struct {
	upstream FlagEvaluator
	cache    *lru.Cache[flagKey, FlagDecision]
	mu       sync.Mutex
	pending  map[flagKey]chan struct{}
}

// NewCachedFlagEvaluator returns a wrapper around the given upstream.
func NewCachedFlagEvaluator(upstream FlagEvaluator, capacity int, ttl time.Duration) *CachedFlagEvaluator {
	return &CachedFlagEvaluator{
		upstream: upstream,
		cache:    lru.NewWithTTL[flagKey, FlagDecision](capacity, ttl),
		pending:  make(map[flagKey]chan struct{}),
	}
}

// Evaluate returns the (possibly cached) decision for (userID, flag).
func (c *CachedFlagEvaluator) Evaluate(ctx context.Context, userID, flag string) (FlagDecision, error) {
	k := flagKey{UserID: userID, Flag: flag}

	if v, ok := c.cache.Get(k); ok {
		return v, nil
	}

	// Single-flight: only one goroutine populates the cache for this key.
	c.mu.Lock()
	if ch, exists := c.pending[k]; exists {
		c.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return FlagDecision{}, ctx.Err()
		}
		if v, ok := c.cache.Get(k); ok {
			return v, nil
		}
	} else {
		ch := make(chan struct{})
		c.pending[k] = ch
		c.mu.Unlock()
		defer func() {
			c.mu.Lock()
			delete(c.pending, k)
			c.mu.Unlock()
			close(ch)
		}()
	}

	v, err := c.upstream.Evaluate(ctx, userID, flag)
	if err != nil {
		return FlagDecision{}, err
	}
	c.cache.Put(k, v)
	return v, nil
}

func main() {
	upstream := &fakeEvaluator{}
	c := NewCachedFlagEvaluator(upstream, 1024, 5*time.Second)
	ctx := context.Background()

	users := []string{"alice", "bob", "carol", "dave"}
	flags := []string{"new-checkout", "dark-mode", "ai-recs"}

	start := time.Now()
	var wg sync.WaitGroup
	for i := 0; i < 1000; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			u := users[i%len(users)]
			f := flags[i%len(flags)]
			d, err := c.Evaluate(ctx, u, f)
			if err != nil {
				fmt.Println("err:", err)
			}
			_ = d
		}(i)
	}
	wg.Wait()

	fmt.Printf("1000 lookups in %s\n", time.Since(start))
	fmt.Printf("upstream calls: %d (expected ~%d)\n", upstream.calls.Load(), len(users)*len(flags))
}
