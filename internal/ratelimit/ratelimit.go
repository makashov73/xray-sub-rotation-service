package ratelimit

import (
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"
)

// SlidingWindow provides per-IP rate limiting using a sliding window counter.
type SlidingWindow struct {
	mu      sync.Mutex
	clients map[string]*clientState
	maxReqs int
	window  time.Duration
}

type clientState struct {
	count  int
	window time.Time
}

// NewSlidingWindow creates a rate limiter allowing maxReqs per window duration.
func NewSlidingWindow(maxReqs int, window time.Duration) *SlidingWindow {
	return &SlidingWindow{
		clients: make(map[string]*clientState),
		maxReqs: maxReqs,
		window:  window,
	}
}

func extractClientIP(r *http.Request) string {
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		parts := strings.Split(xff, ",")
		ip := strings.TrimSpace(parts[0])
		if ip != "" {
			return ip
		}
	}
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	ip := r.RemoteAddr
	if host, _, err := net.SplitHostPort(ip); err == nil {
		ip = host
	}
	return ip
}

// Limit wraps an http.Handler with per-IP rate limiting.
// Returns 429 Too Many Requests when the limit is exceeded.
func (rl *SlidingWindow) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := extractClientIP(r)

		rl.mu.Lock()
		cs, ok := rl.clients[ip]
		if !ok {
			cs = &clientState{}
			rl.clients[ip] = cs
		}

		now := time.Now()
		if now.Sub(cs.window) > rl.window {
			cs.count = 0
			cs.window = now
		}

		cs.count++
		rl.mu.Unlock()

		if cs.count > rl.maxReqs {
			w.Header().Set("Retry-After", strconv.FormatInt(int64(rl.window.Seconds()), 10))
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
