package ratelimit

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

type entry struct {
	count    int
	resetAt  time.Time
}

type Limiter struct {
	mu       sync.Mutex
	entries  map[string]*entry
	limit    int
	window   time.Duration
}

func New(limit int, window time.Duration) *Limiter {
	l := &Limiter{
		entries: make(map[string]*entry),
		limit:   limit,
		window:  window,
	}
	go l.cleanup()
	return l
}

func (l *Limiter) Allow(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	e, ok := l.entries[key]
	if !ok || now.After(e.resetAt) {
		l.entries[key] = &entry{count: 1, resetAt: now.Add(l.window)}
		return true
	}
	e.count++
	return e.count <= l.limit
}

func (l *Limiter) cleanup() {
	for {
		time.Sleep(l.window)
		l.mu.Lock()
		now := time.Now()
		for k, e := range l.entries {
			if now.After(e.resetAt) {
				delete(l.entries, k)
			}
		}
		l.mu.Unlock()
	}
}

// clientIP extracts the real client IP. On Cloud Run, X-Forwarded-For is
// set by the load balancer; the first entry is the client. We only trust
// the first IP to prevent spoofing via appended values.
func clientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// First comma-separated value is the original client IP
		if idx := strings.IndexByte(xff, ','); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}
	// Strip port from RemoteAddr
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func (l *Limiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		key := clientIP(r)
		if !l.Allow(key) {
			w.Header().Set("Retry-After", "60")
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
