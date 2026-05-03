package proxy

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/makashov73/xray-sub-rotation-service/internal/store"
)

func TestCheckEndpointSuccess(t *testing.T) {
	s := store.NewStore()
	s.AddEndpoint("http://test.example/sub/abc", "abc123", "test")

	p := New(s, "fastest", 2, 5*time.Second)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	// Update the endpoint URL to point to test server
	s.AddEndpoint(srv.URL+"/sub/abc", "abc123", "test")
	p.checkEndpoint(store.Endpoint{ID: srv.URL + "/sub/abc", URL: srv.URL + "/sub/abc"})

	health, ok := s.GetHealth(srv.URL + "/sub/abc")
	if !ok {
		t.Fatal("Health info not found")
	}
	if !health.Healthy {
		t.Error("Expected healthy endpoint")
	}
	if health.LatencyMS <= 0 {
		t.Errorf("Expected positive latency, got %f", health.LatencyMS)
	}
}

func TestCheckEndpointFailure(t *testing.T) {
	s := store.NewStore()
	s.AddEndpoint("http://nonexistent.invalid:99999/sub/abc", "abc123", "test")

	p := New(s, "fastest", 2, 5*time.Second)

	p.checkEndpoint(s.GetEndpoints()[0])

	health, _ := s.GetHealth(s.GetEndpoints()[0].ID)
	if health.Healthy {
		t.Error("Expected unhealthy endpoint")
	}
}
