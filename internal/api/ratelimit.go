package api

import (
	"net"
	"net/http"
	"sync"
	"time"
)

type rateLimiter struct {
	mu        sync.Mutex
	entries   map[string]rateEntry
	limit     int
	window    time.Duration
	nextPrune time.Time
}

type rateEntry struct {
	count int
	reset time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{entries: make(map[string]rateEntry), limit: limit, window: window}
}

func (l *rateLimiter) allow(r *http.Request) bool {
	if l == nil {
		return true
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		host = r.RemoteAddr
	}
	now := time.Now()
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.nextPrune.IsZero() || now.After(l.nextPrune) {
		l.pruneLocked(now)
		l.nextPrune = now.Add(l.window)
	}
	entry := l.entries[host]
	if entry.reset.IsZero() || now.After(entry.reset) {
		entry = rateEntry{reset: now.Add(l.window)}
	}
	entry.count++
	l.entries[host] = entry
	return entry.count <= l.limit
}

func (l *rateLimiter) pruneLocked(now time.Time) {
	for host, entry := range l.entries {
		if now.After(entry.reset) {
			delete(l.entries, host)
		}
	}
}
