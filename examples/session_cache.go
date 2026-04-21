// Package main demonstrates using go-lrucache as an in-memory session
// store for a simple HTTP server. Sessions have a sliding TTL: whenever a
// request hits an active session, the cache is re-Put with a fresh TTL so
// that a user who is actively browsing is never logged out unexpectedly,
// while idle sessions are naturally garbage-collected by the LRU bound.
//
// Run:
//
//	go run ./examples/session_cache.go
//
// Then in a separate shell:
//
//	curl -c cookies.txt 'http://localhost:8080/login?user=alice'
//	curl -b cookies.txt 'http://localhost:8080/whoami'
//	curl -b cookies.txt 'http://localhost:8080/logout'
package main

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Harsh7115/go-lrucache/lru"
)

type Session struct {
	UserID    string
	CreatedAt time.Time
	LastSeen  time.Time
}

// Our session cache holds at most 10_000 live sessions; anything older than
// 30 minutes without activity is evicted automatically.
const (
	maxSessions = 10_000
	sessionTTL  = 30 * time.Minute
)

var sessions = lru.NewWithTTL[string, *Session](maxSessions, sessionTTL)

// newSessionID returns a cryptographically random opaque identifier.
func newSessionID() string {
	var b [16]byte
	if _, err := rand.Read(b[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b[:])
}

// touch extends the sliding expiration of a live session.
func touch(id string, s *Session) {
	s.LastSeen = time.Now()
	sessions.Put(id, s) // Put resets the TTL.
}

func loginHandler(w http.ResponseWriter, r *http.Request) {
	user := r.URL.Query().Get("user")
	if user == "" {
		http.Error(w, "missing ?user=", http.StatusBadRequest)
		return
	}
	id := newSessionID()
	now := time.Now()
	sessions.Put(id, &Session{UserID: user, CreatedAt: now, LastSeen: now})

	http.SetCookie(w, &http.Cookie{
		Name:     "sid",
		Value:    id,
		Path:     "/",
		HttpOnly: true,
		MaxAge:   int(sessionTTL.Seconds()),
	})
	fmt.Fprintf(w, "hello %s, session %s\n", user, id[:8])
}

func whoamiHandler(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie("sid")
	if err != nil {
		http.Error(w, "not logged in", http.StatusUnauthorized)
		return
	}
	s, ok := sessions.Get(c.Value)
	if !ok {
		http.Error(w, "session expired", http.StatusUnauthorized)
		return
	}
	touch(c.Value, s)
	fmt.Fprintf(w, "user=%s since=%s\n", s.UserID, s.CreatedAt.Format(time.RFC3339))
}

func logoutHandler(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie("sid"); err == nil {
		sessions.Delete(c.Value)
	}
	http.SetCookie(w, &http.Cookie{Name: "sid", Value: "", Path: "/", MaxAge: -1})
	fmt.Fprintln(w, "bye")
}

func main() {
	http.HandleFunc("/login", loginHandler)
	http.HandleFunc("/whoami", whoamiHandler)
	http.HandleFunc("/logout", logoutHandler)

	addr := ":8080"
	log.Printf("session-cache demo listening on %s (cap=%d, ttl=%s)",
		addr, maxSessions, sessionTTL)
	log.Fatal(http.ListenAndServe(addr, nil))
}
