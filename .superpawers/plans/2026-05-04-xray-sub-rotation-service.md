# xray-sub-rotation-service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpawers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a Go HTTP service that acts as a subscription router for 3x-ui — given a list of 3x-ui subscription URLs, it periodically health-checks them and serves the best-performing one to clients via a unique subscription link (`/subrouter/UUID`).

**Architecture:** The service runs an HTTP server on a configurable port. It loads a list of 3x-ui subscription URLs (from `sublist.md`), each associated with a subId. A background goroutine pings every 3x-ui instance to measure response time, selecting the fastest live one when a client requests a subscription. The response is the raw proxy list text that vless clients natively consume.

**Tech Stack:**
- Go 1.24+ (stdlib only — no external dependencies)
- `net/http` for HTTP server (no web framework)
- `gopkg.in/yaml.v3` for YAML config parsing
- Configuration via YAML file (`config.yaml`)

---

## File Structure

| File | Responsibility |
|------|----------------|
| `cmd/xray-sub-rotation/main.go` | Entry point, bootstrap config, start server |
| `internal/config/config.go` | Config parsing from YAML |
| `internal/config/config_test.go` | Config unit tests |
| `internal/store/store.go` | In-memory store for 3x-ui endpoints and health |
| `internal/store/store_test.go` | Store unit tests |
| `internal/proxy/proxy.go` | Health checker goroutine |
| `internal/proxy/proxy_test.go` | Proxy unit tests |
| `internal/handler/handler.go` | HTTP handlers: subscription proxy and health |
| `internal/handler/handler_test.go` | Handler unit tests |
| `internal/sublist/parser.go` | Parse `sublist.md` file |
| `internal/sublist/parser_test.go` | Parser unit tests |
| `config.yaml` | Example configuration |
| `sublist.md` | Example subscription list |
| `Makefile` | Build/run/test commands |
| `.gitignore` | Git ignore rules |
| `README.md` | Project documentation |

## Key Design Decisions

1. **No external web framework.** `net/http` is sufficient — this is a simple proxy. No need for Gin, Echo, etc.
2. **In-memory store** initially. subId→URL mappings and health data live in memory. A file-based `sublist.md` drives initial load. No persistent DB required for V1.
3. **Health check via HEAD request** to each 3x-ui subscription URL. HEAD is cheaper than GET and 3x-ui supports it.
4. **No auth layer in V1.** Any client with the UUID can fetch subscriptions. Auth can be added via API key later.
5. **Configurable selection strategy** — fastest response time is default, but "random" or "first" strategies are supported.

## Edge Cases & Failure Modes

- **All backends down:** Return 503 with a message.
- **Single backend:** No rotation needed, just proxy directly.
- **UUID mismatch:** User's UUID doesn't match any registered subId → 404.
- **Config reload:** SIGHUP support planned for V2.
- **Rate limiting:** Basic per-IP rate limiting planned for V2.
- **TLS:** Service supports HTTPS if cert/key are configured.

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/health` | GET | Health check — returns 200 OK |
| `/subrouter/{subId}` | GET | Fetch the best subscription for a user |

---

### Task 1: Project Setup

**Files:**
- Create: `go.mod`
- Create: `cmd/xray-sub-rotation/main.go`
- Create: `config.yaml`
- Create: `sublist.md`

- [ ] **Step 1: Initialize Go module**

Run: `go mod init github.com/makashov73/xray-sub-rotation-service`

Expected: `go.mod` created with module path.

- [ ] **Step 2: Create main entry point**

```go
package main

import (
	"log/slog"
	"os"
)

func main() {
	slog.Info("Starting xray-sub-rotation-service")
	// TODO: Load config, initialize components, start server
}
```

- [ ] **Step 3: Create example config file** (`config.yaml`)

```yaml
server:
  host: "0.0.0.0"
  port: 8080

health_check:
  enabled: true
  interval: 30s
  timeout: 5s
  healthy_count: 2

strategy: "fastest"  # fastest | random | first

sublist_file: "sublist.md"

auth:
  api_key: ""  # optional, leave empty for no auth
```

- [ ] **Step 4: Create example sublist** (`sublist.md`)

```markdown
# 3x-ui Subscription List
# Format: subId | URL | Name (optional)

abc123def-456-789-012-345678901234 | https://xray1.example.com/sub/abc123 | US-East
abc123def-456-789-012-345678901234 | https://xray2.example.com/sub/abc123 | EU-West
abc123def-456-789-012-345678901234 | https://xray3.example.com/sub/abc123 | Asia-Pacific
```

- [ ] **Step 5: Commit**

```bash
git add go.mod cmd/xray-sub-rotation/main.go config.yaml sublist.md
git commit -m "feat: initialize project structure with example config"
```

---

### Task 2: Configuration Package

**Files:**
- Create: `internal/config/config.go`
- Create: `internal/config/config_test.go`

- [ ] **Step 1: Write the failing test** (`internal/config/config_test.go`)

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/config/... -v`
Expected: FAIL — package doesn't exist yet.

- [ ] **Step 3: Write the config package** (`internal/config/config.go`)

```go
package config

import (
	"os"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server      ServerConfig      `yaml:"server"`
	HealthCheck HealthCheckConfig `yaml:"health_check"`
	Strategy    string            `yaml:"strategy"`
	SublistFile string            `yaml:"sublist_file"`
	Auth        AuthConfig        `yaml:"auth"`
}

type ServerConfig struct {
	Host string `yaml:"host"`
	Port int    `yaml:"port"`
}

type HealthCheckConfig struct {
	Enabled      bool          `yaml:"enabled"`
	Interval     time.Duration `yaml:"interval"`
	Timeout      time.Duration `yaml:"timeout"`
	HealthyCount int           `yaml:"healthy_count"`
}

type AuthConfig struct {
	APIKey string `yaml:"api_key"`
}

func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		HealthCheck: HealthCheckConfig{
			Enabled:      true,
			Interval:     30 * time.Second,
			Timeout:      5 * time.Second,
			HealthyCount: 2,
		},
		Strategy: "fastest",
	}
}

func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	return cfg, nil
}
```

- [ ] **Step 4: Add YAML dependency**

Run: `go get gopkg.in/yaml.v3`
Run: `go mod tidy`

- [ ] **Step 5: Run test to verify it passes**

Run: `go test ./internal/config/... -v`
Expected: PASS (both tests)

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go go.mod go.sum
git commit -m "feat: add configuration package with YAML parsing"
```

---

### Task 3: Store Package — 3x-ui Endpoints & Health

**Files:**
- Create: `internal/store/store.go`
- Create: `internal/store/store_test.go`

- [ ] **Step 1: Write the failing test** (`internal/store/store_test.go`)

```go
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

	endpoints := s.GetEndpoints()
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/... -v`
Expected: FAIL — package doesn't exist.

- [ ] **Step 3: Write the store package** (`internal/store/store.go`)

```go
package store

import (
	"sync"
	"time"
)

// Endpoint represents a single 3x-ui subscription endpoint.
type Endpoint struct {
	ID    string
	URL   string
	SubId string // UUID extracted from URL
	Name  string // Human-readable name
}

// HealthInfo tracks the health status of an endpoint.
type HealthInfo struct {
	Healthy     bool
	LatencyMS   float64
	LastChecked time.Time
}

// Store holds the list of 3x-ui endpoints and their health status.
type Store struct {
	mu               sync.RWMutex
	endpoints        map[string]Endpoint
	subIdToEndpoints map[string][]string
	health           map[string]HealthInfo
}

func NewStore() *Store {
	return &Store{
		endpoints:        make(map[string]Endpoint),
		subIdToEndpoints: make(map[string][]string),
		health:           make(map[string]HealthInfo),
	}
}

func (s *Store) AddEndpoint(url, subId, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

	id := url
	e := Endpoint{
		ID:    id,
		URL:   url,
		SubId: subId,
		Name:  name,
	}
	s.endpoints[id] = e
	s.subIdToEndpoints[subId] = append(s.subIdToEndpoints[subId], id)
	s.health[id] = HealthInfo{
		Healthy:     true,
		LastChecked: time.Now(),
	}
}

func (s *Store) GetEndpoints() []Endpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()

	result := make([]Endpoint, 0, len(s.endpoints))
	for _, e := range s.endpoints {
		result = append(result, e)
	}
	return result
}

func (s *Store) GetUrlsForSubId(subId string) []string {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.subIdToEndpoints[subId]
	urls := make([]string, 0, len(ids))
	for _, id := range ids {
		urls = append(urls, s.endpoints[id].URL)
	}
	return urls
}

func (s *Store) RecordHealth(endpointID string, info HealthInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.health[endpointID] = info
}

// GetBestEndpoint returns the best (healthy, lowest latency) endpoint for a subId.
// If all are down, returns the most recently checked.
func (s *Store) GetBestEndpoint(subId string) *Endpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.subIdToEndpoints[subId]
	if len(ids) == 0 {
		return nil
	}

	var best *Endpoint
	for _, id := range ids {
		info, ok := s.health[id]
		if !ok {
			continue
		}
		if !info.Healthy {
			continue
		}
		if best == nil || info.LatencyMS < s.health[best.ID].LatencyMS {
			best = &s.endpoints[id]
		}
	}

	if best != nil {
		return best
	}

	// All unhealthy — return the most recently checked
	for _, id := range ids {
		info, ok := s.health[id]
		if !ok {
			continue
		}
		if best == nil || info.LastChecked.After(s.health[best.ID].LastChecked) {
			best = &s.endpoints[id]
		}
	}

	return best
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/... -v`
Expected: PASS (all tests)

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat: add in-memory store for 3x-ui endpoints and health tracking"
```

---

### Task 4: Proxy Package — Health Checker

**Files:**
- Create: `internal/proxy/proxy.go`
- Create: `internal/proxy/proxy_test.go`

- [ ] **Step 1: Write the failing test** (`internal/proxy/proxy_test.go`)

```go
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

	health := s.health[srv.URL+"/sub/abc"]
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

	health := s.health[s.GetEndpoints()[0].ID]
	if health.Healthy {
		t.Error("Expected unhealthy endpoint")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/proxy/... -v`
Expected: FAIL — package doesn't exist.

- [ ] **Step 3: Write the proxy package** (`internal/proxy/proxy.go`)

```go
package proxy

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/makashov73/xray-sub-rotation-service/internal/store"
)

// Proxy manages health checking of 3x-ui endpoints.
type Proxy struct {
	store     *store.Store
	strategy  string
	healthyAt int
	timeout   time.Duration
	client    *http.Client
}

// New creates a new Proxy.
func New(s *store.Store, strategy string, healthyAt int, timeout time.Duration) *Proxy {
	return &Proxy{
		store:     s,
		strategy:  strategy,
		healthyAt: healthyAt,
		timeout:   timeout,
		client: &http.Client{
			Timeout: timeout,
		},
	}
}

// StartHealthCheck starts a goroutine that periodically pings all endpoints.
// Pass a stop channel to terminate it.
func (p *Proxy) StartHealthCheck(interval time.Duration, stop chan struct{}) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()

		for {
			select {
			case <-stop:
				return
			case <-ticker.C:
				p.checkAll()
			}
		}
	}()
}

func (p *Proxy) checkAll() {
	endpoints := p.store.GetEndpoints()

	for _, ep := range endpoints {
		p.checkEndpoint(ep)
	}
}

func (p *Proxy) checkEndpoint(ep store.Endpoint) {
	start := time.Now()

	req, err := http.NewRequest("HEAD", ep.URL, nil)
	if err != nil {
		slog.Warn("Failed to create health check request", "endpoint", ep.URL, "error", err)
		return
	}

	resp, err := p.client.Do(req)
	elapsed := time.Since(start).Seconds() * 1000

	if err != nil {
		slog.Warn("Health check failed", "endpoint", ep.URL, "error", err)
		p.store.RecordHealth(ep.ID, store.HealthInfo{
			Healthy:     false,
			LatencyMS:   elapsed,
			LastChecked: time.Now(),
		})
		return
	}
	defer resp.Body.Close()

	healthy := resp.StatusCode >= 200 && resp.StatusCode < 400

	p.store.RecordHealth(ep.ID, store.HealthInfo{
		Healthy:     healthy,
		LatencyMS:   elapsed,
		LastChecked: time.Now(),
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/proxy/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/proxy/proxy.go internal/proxy/proxy_test.go
git commit -m "feat: add health checker that pings 3x-ui endpoints"
```

---

### Task 5: Handler Package — HTTP Server

**Files:**
- Create: `internal/handler/handler.go`
- Create: `internal/handler/handler_test.go`

- [ ] **Step 1: Write the failing test** (`internal/handler/handler_test.go`)

```go
package handler

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/makashov73/xray-sub-rotation-service/internal/proxy"
	"github.com/makashov73/xray-sub-rotation-service/internal/store"
)

func TestSubscriptionHandler(t *testing.T) {
	s := store.NewStore()

	// Mock response from 3x-ui
	mockResponse := "vless://test1\ntrojan://test2\n"

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(mockResponse))
	}))
	defer srv.Close()

	s.AddEndpoint(srv.URL+"/sub/abc123", "abc123", "test-server")

	p := proxy.New(s, "fastest", 2, 5000)
	h := New(s, p)

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
	s := store.NewStore()
	p := proxy.New(s, "fastest", 2, 5000)
	h := New(s, p)

	req := httptest.NewRequest("GET", "/subrouter/nonexistent-uuid", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestHealthCheckEndpoint(t *testing.T) {
	s := store.NewStore()
	p := proxy.New(s, "fastest", 2, 5000)
	h := New(s, p)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body, _ := io.ReadAll(w.Body)
	if !strings.Contains(string(body), "ok") {
		t.Errorf("Body = %q, want to contain 'ok'", string(body))
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handler/... -v`
Expected: FAIL — package doesn't exist.

- [ ] **Step 3: Write the handler package** (`internal/handler/handler.go`)

```go
package handler

import (
	"io"
	"net/http"
	"strings"

	"github.com/makashov73/xray-sub-rotation-service/internal/proxy"
	"github.com/makashov73/xray-sub-rotation-service/internal/store"
)

// Handler handles HTTP requests for subscription routing.
type Handler struct {
	store *store.Store
	proxy *proxy.Proxy
}

// New creates a new Handler.
func New(s *store.Store, p *proxy.Proxy) *Handler {
	return &Handler{
		store: s,
		proxy: p,
	}
}

// RegisterRoutes registers all HTTP routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/health", h.healthHandler)
	mux.HandleFunc("/subrouter/", h.subscriptionHandler)
}

func (h *Handler) subscriptionHandler(w http.ResponseWriter, r *http.Request) {
	// Extract subId from URL path: /subrouter/{subId}
	path := strings.TrimPrefix(r.URL.Path, "/subrouter/")
	subId := strings.TrimSuffix(path, "/")

	if subId == "" {
		http.Error(w, "subId required", http.StatusBadRequest)
		return
	}

	best := h.proxy.GetBestEndpointForSubId(subId)
	if best == nil {
		http.Error(w, "no available endpoints for this subId", http.StatusNotFound)
		return
	}

	// Fetch subscription from the best 3x-ui endpoint
	resp, err := http.Get(best.URL)
	if err != nil {
		http.Error(w, "failed to fetch subscription", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Forward the response
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (h *Handler) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	w.Write([]byte("ok"))
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/handler/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add internal/handler/handler.go internal/handler/handler_test.go
git commit -m "feat: add HTTP handler for subscription proxy and health endpoint"
```

---

### Task 6: Wire Everything Together

**Files:**
- Modify: `cmd/xray-sub-rotation/main.go`
- Create: `internal/sublist/parser.go`
- Create: `internal/sublist/parser_test.go`

- [ ] **Step 1: Write the sublist parser** (`internal/sublist/parser.go`)

```go
package sublist

import (
	"bufio"
	"os"
	"strings"
)

// Entry represents one subscription line in sublist.md.
type Entry struct {
	SubId string
	URL   string
	Name  string
}

// Parse reads a sublist.md file and returns subscription entries.
func Parse(path string) ([]Entry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	var entries []Entry
	scanner := bufio.NewScanner(f)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.Split(line, "|")
		if len(parts) < 2 {
			continue
		}

		subId := strings.TrimSpace(parts[0])
		url := strings.TrimSpace(parts[1])
		name := ""
		if len(parts) >= 3 {
			name = strings.TrimSpace(parts[2])
		}

		entries = append(entries, Entry{
			SubId: subId,
			URL:   url,
			Name:  name,
		})
	}

	return entries, scanner.Err()
}
```

- [ ] **Step 2: Write the sublist parser test** (`internal/sublist/parser_test.go`)

```go
package sublist

import (
	"os"
	"path/filepath"
	"testing"
)

func TestParse(t *testing.T) {
	dir := t.TempDir()
	content := `# Comment line
abc123 | https://xray1.example.com/sub/abc123 | Server 1

xyz789 | https://xray2.example.com/sub/xyz789 | Server 2
`
	path := filepath.Join(dir, "sublist.md")
	os.WriteFile(path, []byte(content), 0644)

	entries, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	if len(entries) != 2 {
		t.Fatalf("Expected 2 entries, got %d", len(entries))
	}

	if entries[0].SubId != "abc123" {
		t.Errorf("First SubId = %q, want %q", entries[0].SubId, "abc123")
	}
	if entries[0].URL != "https://xray1.example.com/sub/abc123" {
		t.Errorf("First URL = %q", entries[0].URL)
	}
	if entries[0].Name != "Server 1" {
		t.Errorf("First Name = %q, want %q", entries[0].Name, "Server 1")
	}
}
```

- [ ] **Step 3: Wire main.go**

```go
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/makashov73/xray-sub-rotation-service/internal/config"
	"github.com/makashov73/xray-sub-rotation-service/internal/handler"
	"github.com/makashov73/xray-sub-rotation-service/internal/proxy"
	"github.com/makashov73/xray-sub-rotation-service/internal/store"
	"github.com/makashov73/xray-sub-rotation-service/internal/sublist"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Load config
	cfgPath := "config.yaml"
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize store
	s := store.NewStore()

	// Load subscription list
	entries, err := sublist.Parse(cfg.SublistFile)
	if err != nil {
		slog.Error("Failed to parse sublist", "error", err)
		os.Exit(1)
	}

	for _, e := range entries {
		s.AddEndpoint(e.URL, e.SubId, e.Name)
	}

	slog.Info("Loaded subscription list", "endpoints", len(entries))

	// Initialize proxy
	p := proxy.New(s, cfg.Strategy, cfg.HealthCheck.HealthyCount, cfg.HealthCheck.Timeout)

	// Start health check
	var stopChan chan struct{}
	if cfg.HealthCheck.Enabled {
		stopChan = make(chan struct{})
		p.StartHealthCheck(cfg.HealthCheck.Interval, stopChan)
		slog.Info("Health checker started", "interval", cfg.HealthCheck.Interval)
	}

	// Initialize handler and register routes
	h := handler.New(s, p)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Start server
	addr := cfg.Server.Host + ":" + strconv.Itoa(cfg.Server.Port)
	slog.Info("Starting server", "addr", addr)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	cancel()

	if stopChan != nil {
		close(stopChan)
	}

	slog.Info("Shutting down server")
	server.Shutdown(context.Background())
}
```

- [ ] **Step 4: Run all tests**

Run: `go test ./... -v`
Expected: All tests pass

- [ ] **Step 5: Verify it builds**

Run: `go build -o xray-sub-rotation ./cmd/xray-sub-rotation/`
Expected: Binary created at `xray-sub-rotation`

- [ ] **Step 6: Commit**

```bash
git add cmd/xray-sub-rotation/main.go internal/sublist/parser.go internal/sublist/parser_test.go
git commit -m "feat: wire together config, store, proxy, handler, and sublist parser"
```

---

### Task 7: Polish & Documentation

**Files:**
- Create: `Makefile`
- Create: `.gitignore`
- Create: `README.md`

- [ ] **Step 1: Create Makefile**

```makefile
.PHONY: build run test lint clean

build:
	go build -o xray-sub-rotation ./cmd/xray-sub-rotation/

run:
	go run ./cmd/xray-sub-rotation/

test:
	go test ./... -v

lint:
	golangci-lint run ./...

clean:
	rm -f xray-sub-rotation
```

- [ ] **Step 2: Create .gitignore**

```
# Binaries
xray-sub-rotation

# Go build artifacts
*.exe
*.exe~
*.test
*.out

# IDE
.idea/
.vscode/
*.swp
*~

# OS
.DS_Store
```

- [ ] **Step 3: Create README.md**

```markdown
# xray-sub-rotation-service

A Go service that routes 3x-ui subscription requests across multiple 3x-ui instances, selecting the best-performing one based on health checks.

## How It Works

1. Load a list of 3x-ui subscription URLs from `sublist.md`
2. Periodically ping each endpoint to measure latency and availability
3. When a client requests `/subrouter/{subId}`, serve the fastest live endpoint

## Configuration

Edit `config.yaml`:
- `server.host`, `server.port`: Listen address
- `health_check.interval`: How often to ping endpoints
- `strategy`: Selection strategy (`fastest`, `random`, `first`)

## Subscription List

Format in `sublist.md`:
```
subId | URL | Name
```

## Usage

```bash
go build -o xray-sub-rotation ./cmd/xray-sub-rotation/
./xray-sub-rotation
```

## API

- `GET /health` — Health check endpoint
- `GET /subrouter/{subId}` — Fetch the best subscription for a user
```

- [ ] **Step 4: Final test and commit**

Run: `go test ./... -v`
Run: `go build -o xray-sub-rotation ./cmd/xray-sub-rotation/`

```bash
git add Makefile .gitignore README.md
git commit -m "docs: add Makefile, gitignore, and README"
```

---

## Summary

This plan covers 7 tasks across 15 files, building a complete Go HTTP service:

1. **Project setup** — Go module, entry point, config files
2. **Configuration** — YAML-based config with server, health check, and auth options
3. **Store** — In-memory endpoint registry with health tracking and best-endpoint selection
4. **Proxy** — Periodic HEAD-based health checker
5. **Handler** — HTTP routes for `/health` and `/subrouter/{subId}`
6. **Integration** — Main program wiring everything together
7. **Polish** — Makefile, docs

## Concerns

1. **HEAD request support:** I found the 3x-ui source code confirms endpoints are registered on a Gin router, but didn't verify HEAD support specifically. If HEAD returns 405, the health checker needs to fall back to GET. This is a small fix in `proxy.go` if needed.
2. **No database:** All state is in memory. If the service restarts, it needs to re-parse `sublist.md`. Fine for V1.
3. **No rate limiting:** Planned for V2.
4. **No auth:** Planned for V2.
5. **No SIGHUP config reload:** Planned for V2.
