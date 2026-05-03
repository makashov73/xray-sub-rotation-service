package store

import (
	"testing"
	"time"
)

func TestAddAndGetEndpoints(t *testing.T) {
	s := NewStore()
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "US-East")
	s.AddEndpoint("https://xray2.example.com/sub/abc", "abc123", "EU-West")

	endpoints := s.GetEndpoints()
	if len(endpoints) != 2 {
		t.Fatalf("Expected 2 endpoints, got %d", len(endpoints))
	}
}

func TestGetUrlsForSubId(t *testing.T) {
	s := NewStore()
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "US-East")
	s.AddEndpoint("https://xray2.example.com/sub/abc", "abc123", "EU-West")

	urls := s.GetUrlsForSubId("abc123")
	if len(urls) != 2 {
		t.Fatalf("Expected 2 URLs, got %d", len(urls))
	}
}

func TestGetUrlsForNonexistentSubId(t *testing.T) {
	s := NewStore()
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "US-East")

	urls := s.GetUrlsForSubId("nonexistent")
	if len(urls) != 0 {
		t.Errorf("Expected 0 URLs, got %d", len(urls))
	}
}

func TestRecordAndReadHealth(t *testing.T) {
	s := NewStore()
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "US-East")

	s.RecordHealth("https://xray1.example.com/sub/abc", HealthInfo{
		Healthy:     true,
		LatencyMS:   50,
		LastChecked: time.Now(),
	})

	health, ok := s.health["https://xray1.example.com/sub/abc"]
	if !ok {
		t.Fatal("Health info not found")
	}
	if !health.Healthy {
		t.Error("Expected healthy = true")
	}
	if health.LatencyMS != 50 {
		t.Errorf("Latency = %f, want 50", health.LatencyMS)
	}
}

func TestGetBestEndpoint(t *testing.T) {
	s := NewStore()
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "fast")
	s.AddEndpoint("https://xray2.example.com/sub/abc", "abc123", "slow")

	s.RecordHealth("https://xray1.example.com/sub/abc", HealthInfo{
		Healthy:     true,
		LatencyMS:   50,
		LastChecked: time.Now(),
	})
	s.RecordHealth("https://xray2.example.com/sub/abc", HealthInfo{
		Healthy:     true,
		LatencyMS:   200,
		LastChecked: time.Now(),
	})

	best := s.GetBestEndpoint("abc123")
	if best == nil {
		t.Fatal("Expected best endpoint, got nil")
	}
	if best.Name != "fast" {
		t.Errorf("Best endpoint name = %q, want %q", best.Name, "fast")
	}
}

func TestGetBestEndpointWhenDown(t *testing.T) {
	s := NewStore()
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "fast")
	s.AddEndpoint("https://xray2.example.com/sub/abc", "abc123", "slow")

	s.RecordHealth("https://xray1.example.com/sub/abc", HealthInfo{
		Healthy:     false,
		LatencyMS:   0,
		LastChecked: time.Now(),
	})
	s.RecordHealth("https://xray2.example.com/sub/abc", HealthInfo{
		Healthy:     true,
		LatencyMS:   100,
		LastChecked: time.Now(),
	})

	best := s.GetBestEndpoint("abc123")
	if best == nil {
		t.Fatal("Expected best endpoint, got nil")
	}
	if best.Name != "slow" {
		t.Errorf("Best endpoint name = %q, want %q", best.Name, "slow")
	}
}

func TestGetBestEndpointWhenAllDown(t *testing.T) {
	s := NewStore()
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "server1")
	s.AddEndpoint("https://xray2.example.com/sub/abc", "abc123", "server2")

	s.RecordHealth("https://xray1.example.com/sub/abc", HealthInfo{
		Healthy:     false,
		LatencyMS:   0,
		LastChecked: time.Now(),
	})
	s.RecordHealth("https://xray2.example.com/sub/abc", HealthInfo{
		Healthy:     false,
		LatencyMS:   0,
		LastChecked: time.Now(),
	})

	best := s.GetBestEndpoint("abc123")
	if best == nil {
		t.Error("Expected a best endpoint even when all are down")
	}
}
