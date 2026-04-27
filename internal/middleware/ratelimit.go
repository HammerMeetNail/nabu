package middleware

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type RateLimiter struct {
	mu      sync.Mutex
	entries map[string]rateEntry
	limit   int
	window  time.Duration
	now     func() time.Time
}

type rateEntry struct {
	count     int
	windowEnd time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	return &RateLimiter{
		entries: map[string]rateEntry{},
		limit:   limit,
		window:  window,
		now:     func() time.Time { return time.Now().UTC() },
	}
}

func (l *RateLimiter) Middleware(prefix string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if prefix == "" || strings.HasPrefix(r.URL.Path, prefix) {
				if !l.allow(clientIP(r), r.URL.Path) {
					http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (l *RateLimiter) allow(ip, path string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := ip + "|" + path
	now := l.now()
	entry, ok := l.entries[key]
	if !ok || now.After(entry.windowEnd) {
		l.entries[key] = rateEntry{count: 1, windowEnd: now.Add(l.window)}
		return true
	}
	if entry.count >= l.limit {
		return false
	}
	entry.count++
	l.entries[key] = entry
	return true
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
