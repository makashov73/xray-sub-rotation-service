package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadConfig(t *testing.T) {
	dir := t.TempDir()
	yaml := `
server:
  host: "127.0.0.1"
  port: 9090
health_check:
  enabled: false
  interval: 60s
  timeout: 3s
  healthy_count: 3
strategy: "random"
sublist_file: "/tmp/test-sublist.md"
auth:
  api_key: "test-key"
`
	path := filepath.Join(dir, "config.yaml")
	os.WriteFile(path, []byte(yaml), 0644)

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig failed: %v", err)
	}

	if cfg.Server.Host != "127.0.0.1" {
		t.Errorf("Host = %q, want %q", cfg.Server.Host, "127.0.0.1")
	}
	if cfg.Server.Port != 9090 {
		t.Errorf("Port = %d, want %d", cfg.Server.Port, 9090)
	}
	if cfg.HealthCheck.Enabled {
		t.Error("HealthCheck.Enabled = true, want false")
	}
	if cfg.Strategy != "random" {
		t.Errorf("Strategy = %q, want %q", cfg.Strategy, "random")
	}
	if cfg.Auth.APIKey != "test-key" {
		t.Errorf("APIKey = %q, want %q", cfg.Auth.APIKey, "test-key")
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()
	if cfg.Server.Host != "0.0.0.0" {
		t.Errorf("Default Host = %q, want %q", cfg.Server.Host, "0.0.0.0")
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Default Port = %d, want %d", cfg.Server.Port, 8080)
	}
	if !cfg.HealthCheck.Enabled {
		t.Error("Default HealthCheck.Enabled = false, want true")
	}
	if cfg.Strategy != "fastest" {
		t.Errorf("Default Strategy = %q, want %q", cfg.Strategy, "fastest")
	}
	if cfg.HealthCheck.Interval != 30*time.Second {
		t.Errorf("Default Interval = %v, want %v", cfg.HealthCheck.Interval, 30*time.Second)
	}
}
