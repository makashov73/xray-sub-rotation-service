package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/makashov73/xray-sub-rotation-service/internal/proxy"
	"github.com/makashov73/xray-sub-rotation-service/internal/store"
)

func TestSubscriptionHandler(t *testing.T) {
	s := store.NewStore("fastest")

	// Mock response from 3x-ui
	mockResponse := "vless://test1\ntrojan://test2\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer srv.Close()

	s.AddEndpoint(srv.URL+"/sub/abc123", "abc123", "test-server")

	p := proxy.New(s, "fastest", 5*time.Second)
	h := New(s, p, nil)

	// Request subscription for abc123
	req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := strings.TrimSpace(w.Body.String())
	if body != strings.TrimSpace(mockResponse) {
		t.Errorf("Body = %q, want %q", body, mockResponse)
	}
}

func TestSubscriptionNotFound(t *testing.T) {
	s := store.NewStore("fastest")
	p := proxy.New(s, "fastest", 5*time.Second)
	h := New(s, p, nil)

	req := httptest.NewRequest("GET", "/subrouter/nonexistent-uuid", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHealthCheckEndpoint(t *testing.T) {
	s := store.NewStore("fastest")
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "US-East")
	s.AddEndpoint("https://xray2.example.com/sub/abc", "abc123", "EU-West")

	// Record some health data
	s.RecordHealth("https://xray1.example.com/sub/abc", store.HealthInfo{
		Healthy:     true,
		LatencyMS:   42,
		LastChecked: time.Now(),
	})
	s.RecordHealth("https://xray2.example.com/sub/abc", store.HealthInfo{
		Healthy:     false,
		LatencyMS:   0,
		LastChecked: time.Now(),
	})

	p := proxy.New(s, "fastest", 5*time.Second)
	h := New(s, p, nil)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"abc123"`) {
		t.Errorf("Body missing subId: %s", body)
	}
	if !strings.Contains(body, `"US-East"`) {
		t.Errorf("Body missing server name: %s", body)
	}
}

func TestHealthCheckEndpointEmpty(t *testing.T) {
	s := store.NewStore("fastest")
	p := proxy.New(s, "fastest", 5*time.Second)
	h := New(s, p, nil)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	// json.Encoder adds a trailing newline, so we compare the trimmed body
	body := strings.TrimSpace(w.Body.String())
	if body != "{}" {
		t.Errorf("Body = %q, want '{}'", body)
	}
}
