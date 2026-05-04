package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBuildSubscriptionURL(t *testing.T) {
	tests := []struct {
		name    string
		domain  string
		host    string
		port    int
		scheme  string
		subID   string
		wantURL string
	}{
		{
			name:    "no domain - http",
			domain:  "",
			host:    "0.0.0.0",
			port:    8080,
			scheme:  "http",
			subID:   "abc",
			wantURL: "http://0.0.0.0:8080/subrouter/abc",
		},
		{
			name:    "no domain - https",
			domain:  "",
			host:    "0.0.0.0",
			port:    8443,
			scheme:  "https",
			subID:   "abc",
			wantURL: "https://0.0.0.0:8443/subrouter/abc",
		},
		{
			name:    "domain with default http port",
			domain:  "example.com",
			host:    "0.0.0.0",
			port:    80,
			scheme:  "http",
			subID:   "abc",
			wantURL: "http://example.com/subrouter/abc",
		},
		{
			name:    "domain with default https port",
			domain:  "example.com",
			host:    "0.0.0.0",
			port:    443,
			scheme:  "https",
			subID:   "abc",
			wantURL: "https://example.com/subrouter/abc",
		},
		{
			name:    "domain with non-default port",
			domain:  "example.com",
			host:    "0.0.0.0",
			port:    8443,
			scheme:  "https",
			subID:   "abc",
			wantURL: "https://example.com:8443/subrouter/abc",
		},
		{
			name:    "domain with non-default http port",
			domain:  "example.com",
			host:    "0.0.0.0",
			port:    8080,
			scheme:  "http",
			subID:   "abc",
			wantURL: "http://example.com:8080/subrouter/abc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := ServerConfig{Host: tt.host, Port: tt.port}
			got := s.BuildSubscriptionURL(tt.domain, tt.scheme, tt.subID)
			if got != tt.wantURL {
				t.Errorf("BuildSubscriptionURL() = %q, want %q", got, tt.wantURL)
			}
		})
	}
}

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
