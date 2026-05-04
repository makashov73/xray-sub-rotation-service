package ratelimit

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimit(t *testing.T) {
	limiter := NewSlidingWindow(10, time.Second)

	// First 10 requests should succeed
	for i := 0; i < 10; i++ {
		req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
		// Simulate different IPs
		req.RemoteAddr = "1.2.3.4"
		w := httptest.NewRecorder()
		limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Request %d: status = %d, want %d", i+1, w.Code, http.StatusOK)
		}
	}

	// 11th request should be rate limited
	req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "1.2.3.4"
	w := httptest.NewRecorder()
	limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitPerIP(t *testing.T) {
	limiter := NewSlidingWindow(5, time.Second)

	// Client A: 5 requests — all pass
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
		req.RemoteAddr = "1.1.1.1"
		w := httptest.NewRecorder()
		limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Client A request %d: status = %d", i+1, w.Code)
		}
	}

	// Client B: 5 requests — all pass
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
		req.RemoteAddr = "2.2.2.2"
		w := httptest.NewRecorder()
		limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Client B request %d: status = %d", i+1, w.Code)
		}
	}

	// Client A again — rate limited
	req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "1.1.1.1"
	w := httptest.NewRecorder()
	limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Client A after limit: status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitExpiry(t *testing.T) {
	limiter := NewSlidingWindow(3, 500*time.Millisecond)

	// 3 requests
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
		req.RemoteAddr = "1.1.1.1"
		w := httptest.NewRecorder()
		limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
	}

	// Rate limited
	req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "1.1.1.1"
	w := httptest.NewRecorder()
	limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatal("Expected rate limited")
	}

	// Wait for expiry
	time.Sleep(600 * time.Millisecond)

	// Should pass again
	req = httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "1.1.1.1"
	w = httptest.NewRecorder()
	limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("After expiry: status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRateLimitXForwardedFor(t *testing.T) {
	limiter := NewSlidingWindow(2, time.Second)

	// First request with X-Forwarded-For — should pass
	req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "10.0.0.1"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	w := httptest.NewRecorder()
	limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Request 1: status = %d, want %d", w.Code, http.StatusOK)
	}

	// Second request — should pass
	req = httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "10.0.0.1"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	w = httptest.NewRecorder()
	limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Request 2: status = %d, want %d", w.Code, http.StatusOK)
	}

	// Third request — should be rate limited based on X-Forwarded-For IP
	req = httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "10.0.0.1"
	req.Header.Set("X-Forwarded-For", "203.0.113.50")
	w = httptest.NewRecorder()
	limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Request 3: status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimitXRealIP(t *testing.T) {
	limiter := NewSlidingWindow(2, time.Second)

	// Without X-Forwarded-For, falls back to X-Real-IP
	req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "10.0.0.1"
	req.Header.Set("X-Real-IP", "198.51.100.10")
	w := httptest.NewRecorder()
	limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Request 1: status = %d, want %d", w.Code, http.StatusOK)
	}

	// Second request with X-Real-IP — should pass
	req = httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "10.0.0.1"
	req.Header.Set("X-Real-IP", "198.51.100.10")
	w = httptest.NewRecorder()
	limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Request 2: status = %d, want %d", w.Code, http.StatusOK)
	}

	// Third request — rate limited by X-Real-IP
	req = httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "10.0.0.1"
	req.Header.Set("X-Real-IP", "198.51.100.10")
	w = httptest.NewRecorder()
	limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Errorf("Request 3: status = %d, want %d", w.Code, http.StatusTooManyRequests)
	}

	// Request without X-Real-IP uses RemoteAddr — should pass
	req = httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "10.0.0.1"
	w = httptest.NewRecorder()
	limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Request 4 (no X-Real-IP): status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestRateLimitRetryAfterHeader(t *testing.T) {
	limiter := NewSlidingWindow(1, 60*time.Second)

	// First request succeeds
	req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "1.2.3.4"
	w := httptest.NewRecorder()
	limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatal("Expected 200 on first request")
	}

	// Second request is rate limited
	req = httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "1.2.3.4"
	w = httptest.NewRecorder()
	limiter.Limit(http.HandlerFunc(nextHandler)).ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("Expected 429, got %d", w.Code)
	}
	retryAfter := w.Header().Get("Retry-After")
	if retryAfter == "" {
		t.Error("Expected Retry-After header")
	}
	if retryAfter != "60" {
		t.Errorf("Retry-After = %q, want %q", retryAfter, "60")
	}
}

func nextHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
