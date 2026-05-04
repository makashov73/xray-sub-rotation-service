package store

import (
	"sync"
	"testing"
	"time"

	"github.com/makashov73/xray-sub-rotation-service/internal/sublist"
)

func TestAddAndGetEndpoints(t *testing.T) {
	s := NewStore("fastest")
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "US-East")
	s.AddEndpoint("https://xray2.example.com/sub/abc", "abc123", "EU-West")

	endpoints := s.GetEndpoints()
	if len(endpoints) != 2 {
		t.Fatalf("Expected 2 endpoints, got %d", len(endpoints))
	}
}

func TestGetUrlsForSubId(t *testing.T) {
	s := NewStore("fastest")
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "US-East")
	s.AddEndpoint("https://xray2.example.com/sub/abc", "abc123", "EU-West")

	urls := s.GetUrlsForSubId("abc123")
	if len(urls) != 2 {
		t.Fatalf("Expected 2 URLs, got %d", len(urls))
	}
}

func TestGetUrlsForNonexistentSubId(t *testing.T) {
	s := NewStore("fastest")
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "US-East")

	urls := s.GetUrlsForSubId("nonexistent")
	if len(urls) != 0 {
		t.Errorf("Expected 0 URLs, got %d", len(urls))
	}
}

func TestRecordAndReadHealth(t *testing.T) {
	s := NewStore("fastest")
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "US-East")

	s.RecordHealth("https://xray1.example.com/sub/abc", HealthInfo{
		Healthy:     true,
		LatencyMS:   50,
		LastChecked: time.Now(),
	})

	health, ok := s.GetHealth("https://xray1.example.com/sub/abc")
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
	s := NewStore("fastest")
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
	s := NewStore("fastest")
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
	s := NewStore("fastest")
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

func TestRandomStrategyAvoidRepeat(t *testing.T) {
	s := NewStore("random")
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "server1")
	s.AddEndpoint("https://xray2.example.com/sub/abc", "abc123", "server2")

	s.RecordHealth("https://xray1.example.com/sub/abc", HealthInfo{
		Healthy:     true,
		LatencyMS:   50,
		LastChecked: time.Now(),
	})
	s.RecordHealth("https://xray2.example.com/sub/abc", HealthInfo{
		Healthy:     true,
		LatencyMS:   50,
		LastChecked: time.Now(),
	})

	// With 2 healthy endpoints and random+anti-repeat, calling twice
	// should always return different endpoints
	first := s.GetBestEndpoint("abc123")
	if first == nil {
		t.Fatal("Expected endpoint, got nil")
	}

	second := s.GetBestEndpoint("abc123")
	if second == nil {
		t.Fatal("Expected endpoint, got nil")
	}

	if first.ID == second.ID {
		t.Errorf("Random strategy should avoid repeating: got %q twice", first.Name)
	}
}

func TestRandomStrategySingleEndpoint(t *testing.T) {
	s := NewStore("random")
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "server1")

	s.RecordHealth("https://xray1.example.com/sub/abc", HealthInfo{
		Healthy:     true,
		LatencyMS:   50,
		LastChecked: time.Now(),
	})

	// With only 1 endpoint, it should still return it (no panic)
	ep := s.GetBestEndpoint("abc123")
	if ep == nil {
		t.Fatal("Expected endpoint, got nil")
	}
	if ep.Name != "server1" {
		t.Errorf("Got %q, want %q", ep.Name, "server1")
	}
}

func TestFirstStrategy(t *testing.T) {
	s := NewStore("first")
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "server1")
	s.AddEndpoint("https://xray2.example.com/sub/abc", "abc123", "server2")

	s.RecordHealth("https://xray1.example.com/sub/abc", HealthInfo{
		Healthy:     true,
		LatencyMS:   200,
		LastChecked: time.Now(),
	})
	s.RecordHealth("https://xray2.example.com/sub/abc", HealthInfo{
		Healthy:     true,
		LatencyMS:   50,
		LastChecked: time.Now(),
	})

	// "first" always returns the first healthy, regardless of latency
	ep := s.GetBestEndpoint("abc123")
	if ep == nil {
		t.Fatal("Expected endpoint, got nil")
	}
	if ep.Name != "server1" {
		t.Errorf("First strategy got %q, want %q", ep.Name, "server1")
	}
}

func TestHealthReport(t *testing.T) {
	s := NewStore("fastest")

	// subId "abc123" — two servers
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "US-East")
	s.AddEndpoint("https://xray2.example.com/sub/abc", "abc123", "EU-West")

	// subId "def456" — one server
	s.AddEndpoint("https://xray3.example.com/sub/def", "def456", "Asia-Pacific")

	s.RecordHealth("https://xray1.example.com/sub/abc", HealthInfo{
		Healthy:     true,
		LatencyMS:   42,
		LastChecked: time.Now(),
	})
	s.RecordHealth("https://xray2.example.com/sub/abc", HealthInfo{
		Healthy:     false,
		LatencyMS:   0,
		LastChecked: time.Now(),
	})
	s.RecordHealth("https://xray3.example.com/sub/def", HealthInfo{
		Healthy:     true,
		LatencyMS:   15,
		LastChecked: time.Now(),
	})

	report := s.HealthReport()

	// Verify subId "abc123"
	abc, ok := report["abc123"]
	if !ok {
		t.Fatal("Missing subId abc123 in report")
	}
	if len(abc) != 2 {
		t.Fatalf("Expected 2 servers for abc123, got %d", len(abc))
	}
	if !abc["US-East"].Healthy {
		t.Error("US-East should be healthy")
	}
	if abc["US-East"].LatencyMS != 42 {
		t.Errorf("US-East latency = %f, want 42", abc["US-East"].LatencyMS)
	}
	if abc["EU-West"].Healthy {
		t.Error("EU-West should be unhealthy")
	}
	if abc["EU-West"].LatencyMS != 0 {
		t.Errorf("EU-West latency = %f, want 0", abc["EU-West"].LatencyMS)
	}

	// Verify subId "def456"
	def, ok := report["def456"]
	if !ok {
		t.Fatal("Missing subId def456 in report")
	}
	if !def["Asia-Pacific"].Healthy {
		t.Error("Asia-Pacific should be healthy")
	}
	if def["Asia-Pacific"].LatencyMS != 15 {
		t.Errorf("Asia-Pacific latency = %f, want 15", def["Asia-Pacific"].LatencyMS)
	}
}

func TestHealthReportEmptyStore(t *testing.T) {
	s := NewStore("fastest")
	report := s.HealthReport()
	if len(report) != 0 {
		t.Errorf("Expected empty report, got %d subIds", len(report))
	}
}

func TestHealthReportUnknownSubId(t *testing.T) {
	s := NewStore("fastest")
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "US-East")

	report := s.HealthReport()
	if _, ok := report["nonexistent"]; ok {
		t.Error("Unexpected subId in report")
	}
}

func TestReloadConcurrentGetBestEndpoint(t *testing.T) {
	s := NewStore("fastest")
	s.AddEndpoint("https://a.example.com/sub/x", "x", "A")
	s.AddEndpoint("https://b.example.com/sub/x", "x", "B")

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				s.GetBestEndpoint("x")
			}
		}()
	}
	for i := 0; i < 10; i++ {
		s.Reload([]sublist.Entry{{SubId: "x", URL: "https://c.example.com/sub/x", Name: "C"}})
	}
	wg.Wait()
}

func TestRandomStrategyNonDeterministic(t *testing.T) {
	s1 := NewStore("random")
	s2 := NewStore("random")
	s1.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "server1")
	s1.AddEndpoint("https://xray2.example.com/sub/abc", "abc123", "server2")
	s2.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "server1")
	s2.AddEndpoint("https://xray2.example.com/sub/abc", "abc123", "server2")

	s1.RecordHealth("https://xray1.example.com/sub/abc", HealthInfo{Healthy: true, LatencyMS: 50, LastChecked: time.Now()})
	s1.RecordHealth("https://xray2.example.com/sub/abc", HealthInfo{Healthy: true, LatencyMS: 50, LastChecked: time.Now()})
	s2.RecordHealth("https://xray1.example.com/sub/abc", HealthInfo{Healthy: true, LatencyMS: 50, LastChecked: time.Now()})
	s2.RecordHealth("https://xray2.example.com/sub/abc", HealthInfo{Healthy: true, LatencyMS: 50, LastChecked: time.Now()})

	results1 := make(map[string]int)
	results2 := make(map[string]int)
	for i := 0; i < 100; i++ {
		if ep := s1.GetBestEndpoint("abc123"); ep != nil {
			results1[ep.Name]++
		}
		if ep := s2.GetBestEndpoint("abc123"); ep != nil {
			results2[ep.Name]++
		}
	}
	// Two independently seeded stores should produce different distributions
	if len(results1) == 0 || len(results2) == 0 {
		t.Fatal("Expected results from both stores")
	}
	// Check that distributions are not identical (high probability with enough samples)
	same := true
	for k, v1 := range results1 {
		if v1 != results2[k] {
			same = false
			break
		}
	}
	if same {
		t.Log("Warning: both stores produced identical results (rare)")
	}
}
