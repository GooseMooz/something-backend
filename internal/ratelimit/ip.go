package ratelimit

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

type visitor struct {
	count       int
	windowStart time.Time
}

type IPRateLimiter struct {
	mu      sync.Mutex
	limit   int
	window  time.Duration
	clients map[string]visitor
}

func NewIPRateLimiter(limit int, window time.Duration) func(http.Handler) http.Handler {
	rl := &IPRateLimiter{
		limit:   limit,
		window:  window,
		clients: make(map[string]visitor),
	}
	return rl.Middleware
}

func (rl *IPRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !rl.allow(clientIP(r), time.Now()) {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Retry-After", formatRetryAfter(rl.window))
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (rl *IPRateLimiter) allow(key string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	current := rl.clients[key]
	if current.windowStart.IsZero() || now.Sub(current.windowStart) >= rl.window {
		rl.clients[key] = visitor{count: 1, windowStart: now}
		return true
	}
	if current.count >= rl.limit {
		return false
	}
	current.count++
	rl.clients[key] = current
	return true
}

func clientIP(r *http.Request) string {
	host := strings.TrimSpace(r.RemoteAddr)
	if host == "" {
		return "unknown"
	}
	if parsed, _, err := net.SplitHostPort(host); err == nil {
		return parsed
	}
	return host
}

func formatRetryAfter(window time.Duration) string {
	seconds := int(window.Seconds())
	if seconds < 1 {
		return "1"
	}
	return strconv.Itoa(seconds)
}
