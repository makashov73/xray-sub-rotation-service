package store

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPersistAndLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "health.json")

	s := NewStore("fastest")
	s.AddEndpoint("https://server1.example.com/sub/abc", "abc123", "S1")
	s.AddEndpoint("https://server2.example.com/sub/abc", "abc123", "S2")

	s.RecordHealth("https://server1.example.com/sub/abc", HealthInfo{
		Healthy:     true,
		LatencyMS:   42,
		LastChecked: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	})
	s.RecordHealth("https://server2.example.com/sub/abc", HealthInfo{
		Healthy:     false,
		LatencyMS:   0,
		LastChecked: time.Date(2026, 1, 1, 11, 59, 0, 0, time.UTC),
	})

	// Persist
	if err := s.Persist(path); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	// Create new store and load
	s2 := NewStore("fastest")
	if err := s2.LoadFromDisk(path); err != nil {
		t.Fatalf("LoadFromDisk failed: %v", err)
	}

	// Verify health
	h1, ok := s2.GetHealth("https://server1.example.com/sub/abc")
	if !ok || !h1.Healthy || h1.LatencyMS != 42 {
		t.Error("Health data mismatch for server1")
	}
	h2, ok := s2.GetHealth("https://server2.example.com/sub/abc")
	if !ok || h2.Healthy {
		t.Error("Health data mismatch for server2")
	}
}

func TestPersistEmptyStore(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "health.json")

	s := NewStore("fastest")
	if err := s.Persist(path); err != nil {
		t.Fatalf("Persist failed: %v", err)
	}

	// File should exist but be empty object
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}
	if string(data) != "{}" {
		t.Errorf("Body = %q, want '{}'", string(data))
	}
}

func TestLoadFromDiskMissingFile(t *testing.T) {
	s := NewStore("fastest")
	err := s.LoadFromDisk("/nonexistent/health.json")
	if err == nil {
		t.Error("Expected error for missing file")
	}
}
