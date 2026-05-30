package middleware

import (
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type RateLimiter struct {
	mu           sync.Mutex
	entries      map[string]rateEntry
	limit        int
	window       time.Duration
	now          func() time.Time
	trustedCIDRs []*net.IPNet
	stopCleanup  chan struct{}
}

type rateEntry struct {
	count     int
	windowEnd time.Time
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	rl := &RateLimiter{
		entries:     map[string]rateEntry{},
		limit:       limit,
		window:      window,
		now:         func() time.Time { return time.Now().UTC() },
		stopCleanup: make(chan struct{}),
	}
	go rl.cleanup()
	return rl
}

func (l *RateLimiter) SetTrustedProxies(cidrs string) {
	if cidrs == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	for _, c := range strings.Split(cidrs, ",") {
		c = strings.TrimSpace(c)
		if c == "" {
			continue
		}
		if !strings.Contains(c, "/") {
			c += "/32"
		}
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			continue
		}
		l.trustedCIDRs = append(l.trustedCIDRs, n)
	}
}

func (l *RateLimiter) Stop() {
	close(l.stopCleanup)
}

func (l *RateLimiter) cleanup() {
	ticker := time.NewTicker(10 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			now := l.now()
			for k, e := range l.entries {
				if now.After(e.windowEnd) {
					delete(l.entries, k)
				}
			}
			l.mu.Unlock()
		case <-l.stopCleanup:
			return
		}
	}
}

func (l *RateLimiter) Middleware(prefix string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if prefix == "" || strings.HasPrefix(r.URL.Path, prefix) {
				remaining, windowEnd := l.allowWithInfo(l.clientIP(r), r.URL.Path)
				if remaining < 0 {
					retryAfter := int(time.Until(windowEnd).Seconds()) + 1
					if retryAfter < 1 {
						retryAfter = 1
					}
					w.Header().Set("Retry-After", fmt.Sprintf("%d", retryAfter))
					http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}

func (l *RateLimiter) allow(ip, path string) bool {
	remaining, _ := l.allowWithInfo(ip, path)
	return remaining >= 0
}

// allowWithInfo returns remaining count (-1 if denied) and the current window end time.
func (l *RateLimiter) allowWithInfo(ip, path string) (int, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	key := ip + "|" + path
	now := l.now()
	entry, ok := l.entries[key]
	if !ok || now.After(entry.windowEnd) {
		l.entries[key] = rateEntry{count: 1, windowEnd: now.Add(l.window)}
		return l.limit - 1, now.Add(l.window)
	}
	if entry.count >= l.limit {
		return -1, entry.windowEnd
	}
	entry.count++
	l.entries[key] = entry
	return l.limit - entry.count, entry.windowEnd
}

func (l *RateLimiter) clientIP(r *http.Request) string {
	remoteIP := rawIP(r)
	if l.isTrustedProxy(remoteIP) {
		if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
			ips := strings.Split(xff, ",")
			return strings.TrimSpace(ips[0])
		}
	}
	return remoteIP
}

func rawIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (l *RateLimiter) isTrustedProxy(ip string) bool {
	parsed := net.ParseIP(ip)
	if parsed == nil {
		return false
	}
	for _, cidr := range l.trustedCIDRs {
		if cidr.Contains(parsed) {
			return true
		}
	}
	return false
}
