package reload

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReloadConfig(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "config.yaml")
	sublistPath := filepath.Join(dir, "sublist.md")

	// Initial config
	os.WriteFile(configPath, []byte(`server:
  host: "0.0.0.0"
  port: 8080
health_check:
  enabled: false
strategy: "fastest"
sublist_file: "sublist.md"
auth:
  api_key: ""
`), 0644)

	// Initial sublist
	os.WriteFile(sublistPath, []byte(`abc | https://server1.example.com/sub/abc | S1
`), 0644)

	cfg, err := ReloadConfig(configPath)
	if err != nil {
		t.Fatalf("ReloadConfig failed: %v", err)
	}
	if cfg.Server.Port != 8080 {
		t.Errorf("Port = %d, want 8080", cfg.Server.Port)
	}

	// Update config
	os.WriteFile(configPath, []byte(`server:
  host: "0.0.0.0"
  port: 9090
health_check:
  enabled: true
  interval: 10s
  timeout: 3s
  healthy_count: 1
strategy: "random"
sublist_file: "sublist.md"
auth:
  api_key: "new-key"
`), 0644)

	// Update sublist
	os.WriteFile(sublistPath, []byte(`abc | https://server1.example.com/sub/abc | S1
def | https://server2.example.com/sub/def | S2
`), 0644)

	newCfg, err := ReloadConfig(configPath)
	if err != nil {
		t.Fatalf("ReloadConfig failed: %v", err)
	}
	if newCfg.Server.Port != 9090 {
		t.Errorf("Port = %d, want 9090", newCfg.Server.Port)
	}
	if newCfg.Strategy != "random" {
		t.Errorf("Strategy = %q, want %q", newCfg.Strategy, "random")
	}
}
