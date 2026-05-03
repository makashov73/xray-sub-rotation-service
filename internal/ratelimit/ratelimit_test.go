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

func nextHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
