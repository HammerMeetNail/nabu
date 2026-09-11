package middleware

import (
	"container/heap"
	"fmt"
	"net/http"
	"net/netip"
	"strings"
	"sync"
	"time"

	"github.com/HammerMeetNail/nabu/internal/clientip"
)

const defaultMaxRateClients = 4096

type RateLimiter struct {
	mu           sync.Mutex
	entries      map[string]rateEntry
	expiry       rateExpiries
	limit        int
	maxEntries   int
	window       time.Duration
	now          func() time.Time
	trustedCIDRs []netip.Prefix
	stopCleanup  chan struct{}
	cleanupDone  chan struct{}
	stopOnce     sync.Once
}

type rateEntry struct {
	count     int
	windowEnd time.Time
}
type rateExpiry struct {
	ip  string
	end time.Time
}
type rateExpiries []rateExpiry

func (q rateExpiries) Len() int           { return len(q) }
func (q rateExpiries) Less(i, j int) bool { return q[i].end.Before(q[j].end) }
func (q rateExpiries) Swap(i, j int)      { q[i], q[j] = q[j], q[i] }
func (q *rateExpiries) Push(value any)    { *q = append(*q, value.(rateExpiry)) }
func (q *rateExpiries) Pop() any {
	old := *q
	value := old[len(old)-1]
	old[len(old)-1] = rateExpiry{}
	*q = old[:len(old)-1]
	return value
}

func NewRateLimiter(limit int, window time.Duration) *RateLimiter {
	l := &RateLimiter{entries: map[string]rateEntry{}, limit: limit, maxEntries: defaultMaxRateClients,
		window: window, now: time.Now, stopCleanup: make(chan struct{}), cleanupDone: make(chan struct{})}
	go l.cleanup()
	return l
}

// Max clients bounds both the map and expiration heap. Saturation rejects new
// sources until a window expires; evicting live entries would reset allowances.
func (l *RateLimiter) SetMaxClients(max int) {
	if max <= 0 {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.maxEntries = max
}

func (l *RateLimiter) SetTrustedProxies(cidrs string) error {
	prefixes, err := clientip.ParseTrustedProxies(cidrs)
	if err != nil {
		return err
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	l.trustedCIDRs = prefixes
	return nil
}

func (l *RateLimiter) Stop() {
	l.stopOnce.Do(func() { close(l.stopCleanup) })
	<-l.cleanupDone
}

func (l *RateLimiter) expire(now time.Time) {
	for len(l.expiry) > 0 && !now.Before(l.expiry[0].end) {
		item := heap.Pop(&l.expiry).(rateExpiry)
		if entry, ok := l.entries[item.ip]; ok && entry.windowEnd.Equal(item.end) {
			delete(l.entries, item.ip)
		}
	}
}

func (l *RateLimiter) cleanup() {
	defer close(l.cleanupDone)
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			l.mu.Lock()
			l.expire(l.now())
			l.mu.Unlock()
		case <-l.stopCleanup:
			return
		}
	}
}

// Each limiter is one normalized scope: the aggregate API budget, the stricter
// auth prefix, or the fixed join endpoint. Resource IDs never affect its key.
func (l *RateLimiter) Middleware(prefix string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			matches := prefix == "" || r.URL.Path == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(r.URL.Path, strings.TrimSuffix(prefix, "/")+"/")
			if matches {
				remaining, end := l.allowWithInfo(l.clientIP(r), r.URL.Path)
				if remaining < 0 {
					l.mu.Lock()
					now := l.now()
					l.mu.Unlock()
					retryAfter := int((end.Sub(now) + time.Second - 1) / time.Second)
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

func (l *RateLimiter) allowWithInfo(ip, _ string) (int, time.Time) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := l.now()
	l.expire(now)
	entry, ok := l.entries[ip]
	if !ok {
		if len(l.entries) >= l.maxEntries {
			return -1, l.expiry[0].end
		}
		end := now.Add(l.window)
		l.entries[ip] = rateEntry{count: 1, windowEnd: end}
		heap.Push(&l.expiry, rateExpiry{ip: ip, end: end})
		return l.limit - 1, end
	}
	if entry.count >= l.limit {
		return -1, entry.windowEnd
	}
	entry.count++
	l.entries[ip] = entry
	return l.limit - entry.count, entry.windowEnd
}

func (l *RateLimiter) clientIP(r *http.Request) string {
	l.mu.Lock()
	prefixes := l.trustedCIDRs
	l.mu.Unlock()
	return clientip.Address(r, prefixes)
}
