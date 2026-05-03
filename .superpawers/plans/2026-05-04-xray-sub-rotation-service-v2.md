# xray-sub-rotation-service V2 Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpawers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Extend the V1 service with production-ready features: TLS, SIGHUP config reload, rate limiting, and persistent health store.

**Design Principles:**
- Minimal external deps — stick to Go stdlib (crypto/tls, net/http, sync, os). For rate limiting, implement a sliding window counter from scratch rather than pulling in external libraries.
- Backward compatible — all features are opt-in via config. If `tls.cert_file` is empty, serve plain HTTP as before.
- Rate limiter middleware wraps existing handlers — the handler layer stays clean.

---

## File Structure Changes

| File | Change |
|------|--------|
| `internal/config/config.go` | Add `TLSConfig`, `RateLimitConfig` fields |
| `internal/ratelimit/` | New package — rate limiting middleware |
| `internal/store/store.go` | Add `Persist()` and `LoadFromDisk()` methods |
| `cmd/xray-sub-rotation/main.go` | Wire TLS, SIGHUP, middleware |
| `config.yaml` | Add TLS, rate limit, persistent store config sections |
| `Makefile` | Add `run-https` target |

---

### Task 1: TLS/HTTPS Support

**Files:**
- Create: `internal/tls/tls.go`
- Create: `internal/tls/tls_test.go`
- Modify: `internal/config/config.go` (add TLSConfig)
- Modify: `cmd/xray-sub-rotation/main.go` (use `ListenAndServeTLS`)
- Modify: `config.yaml` (add TLS config section)

**Complexity:** Medium

#### Step 1: Add TLS config to config.go

Add to `config.go` struct:

```go
type TLSConfig struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}
```

Add `TLS TLSConfig` to `Config`.

Update `DefaultConfig`:

```go
TLS: TLSConfig{},  // empty = no TLS (plain HTTP)
```

#### Step 2: Write TLS tests

```go
package tls

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadCertFiles(t *testing.T) {
	dir := t.TempDir()
	// Create a dummy cert/key (not valid, just for loading)
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	os.WriteFile(certPath, []byte("dummy-cert"), 0644)
	os.WriteFile(keyPath, []byte("dummy-key"), 0644)

	err := LoadAndVerify(certPath, keyPath)
	if err != nil {
		t.Fatalf("LoadAndVerify failed: %v", err)
	}
}

func TestLoadMissingCert(t *testing.T) {
	err := LoadAndVerify("/nonexistent/cert.pem", "/nonexistent/key.pem")
	if err == nil {
		t.Error("Expected error for missing cert")
	}
}
```

#### Step 3: Write TLS package

```go
package tls

import "crypto/tls"

// LoadAndVerify loads and verifies TLS cert/key files.
func LoadAndVerify(certFile, keyFile string) error {
	_, err := tls.LoadX509KeyPair(certFile, keyFile)
	return err
}
```

#### Step 4: Wire TLS into main.go

Replace the plain `ListenAndServe` call:

```go
if cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
	slog.Info("Starting HTTPS server", "cert", cfg.TLS.CertFile)
	go func() {
		if err := server.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTPS server error", "error", err)
		}
	}()
} else {
	slog.Info("Starting HTTP server", "addr", addr)
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("HTTP server error", "error", err)
		}
	}()
}
```

#### Step 5: Add Let's Encrypt / ACME support option

For production, use `acme.sh` or `certbot` externally (not a Go dependency). Document this in config.yaml:

```yaml
tls:
  cert_file: ""       # path to cert.pem (empty = HTTP only)
  key_file: ""        # path to key.pem
  # For Let's Encrypt, use certbot or acme.sh externally:
  # certbot certonly --webroot -w /path/to/webroot -d example.com
  # Then point cert_file to /etc/letsencrypt/live/example.com/fullchain.pem
  # and key_file to /etc/letsencrypt/live/example.com/privkey.pem
```

#### Step 6: Update config.yaml

```yaml
tls:
  cert_file: ""
  key_file: ""
```

#### Step 7: Run tests

Run: `go test ./internal/tls/... -v`
Expected: Tests pass.
Run: `go build ./cmd/xray-sub-rotation/`
Expected: Builds with TLS support.

#### Step 8: Commit

```bash
git add internal/tls/ internal/config/config.go config.yaml cmd/xray-sub-rotation/main.go
git commit -m "feat: add TLS/HTTPS support with external cert files"
```

---

### Task 2: SIGHUP Config Reload

**Files:**
- Modify: `cmd/xray-sub-rotation/main.go`
- Create: `internal/reload/reload.go`
- Create: `internal/reload/reload_test.go`
- Modify: `internal/store/store.go` (add `ReloadEndpoints`)
- Modify: `internal/config/config.go` (add `Reload`)

**Complexity:** Medium

#### Step 1: Write SIGHUP reload test

```go
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
```

#### Step 2: Write reload package

```go
package reload

import (
	"log/slog"

	"github.com/makashov73/xray-sub-rotation-service/internal/config"
	"github.com/makashov73/xray-sub-rotation-service/internal/sublist"
)

// ReloadConfig re-reads config.yaml and returns the new config.
func ReloadConfig(configPath string) (config.Config, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return config.DefaultConfig(), err
	}
	return cfg, nil
}

// ReloadEndpoints re-reads sublist.md and returns updated entries.
func ReloadEndpoints(sublistPath string) ([]sublist.Entry, error) {
	entries, err := sublist.Parse(sublistPath)
	if err != nil {
		return nil, err
	}
	return entries, nil
}
```

#### Step 3: Add store reload method

In `store.go`, add:

```go
// Reload clears current endpoints and adds new ones.
// This is used during SIGHUP config reload.
func (s *Store) Reload(entries []Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.endpoints = make(map[string]Endpoint)
	s.subIdToEndpoints = make(map[string][]string)
	s.health = make(map[string]HealthInfo)

	for _, e := range entries {
		s.AddEndpoint(e.URL, e.SubId, e.Name)
	}
}
```

#### Step 4: Wire SIGHUP into main.go

```go
// After initial setup, set up SIGHUP:
slog.Info("Listening for SIGHUP for config reload")
sighup := make(chan os.Signal, 1)
signal.Notify(sighup, syscall.SIGHUP)
go func() {
	for range sighup {
		slog.Info("Received SIGHUP, reloading config...")
		newCfg, err := reload.ReloadConfig(cfgPath)
		if err != nil {
			slog.Error("Failed to reload config", "error", err)
			continue
		}
		entries, err := reload.ReloadEndpoints(newCfg.SublistFile)
		if err != nil {
			slog.Error("Failed to reload sublist", "error", err)
			continue
		}
		s.Reload(entries)
		cfg = newCfg
		slog.Info("Config reloaded successfully")
	}
}()
```

#### Step 5: Run tests

Run: `go test ./internal/reload/... -v`
Run: `go test ./... -v`
Expected: All pass.

#### Step 6: Commit

```bash
git add internal/reload/ internal/store/store.go cmd/xray-sub-rotation/main.go
git commit -m "feat: add SIGHUP config and sublist reload without restart"
```

---

### Task 3: Rate Limiting

**Files:**
- Create: `internal/ratelimit/ratelimit.go`
- Create: `internal/ratelimit/ratelimit_test.go`
- Modify: `internal/handler/handler.go` (wrap with rate limiter)
- Modify: `config.yaml` (add rate limit section)

**Complexity:** Medium

#### Step 1: Write rate limiting test

```go
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
		limiter.Limit(nextHandler).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Request %d: status = %d, want %d", i+1, w.Code, http.StatusOK)
		}
	}

	// 11th request should be rate limited
	req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "1.2.3.4"
	w := httptest.NewRecorder()
	limiter.Limit(nextHandler).ServeHTTP(w, req)
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
		limiter.Limit(nextHandler).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Client A request %d: status = %d", i+1, w.Code)
		}
	}

	// Client B: 5 requests — all pass
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
		req.RemoteAddr = "2.2.2.2"
		w := httptest.NewRecorder()
		limiter.Limit(nextHandler).ServeHTTP(w, req)
		if w.Code != http.StatusOK {
			t.Errorf("Client B request %d: status = %d", i+1, w.Code)
		}
	}

	// Client A again — rate limited
	req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "1.1.1.1"
	w := httptest.NewRecorder()
	limiter.Limit(nextHandler).ServeHTTP(w, req)
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
		limiter.Limit(nextHandler).ServeHTTP(w, req)
	}

	// Rate limited
	req := httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "1.1.1.1"
	w := httptest.NewRecorder()
	limiter.Limit(nextHandler).ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatal("Expected rate limited")
	}

	// Wait for expiry
	time.Sleep(600 * time.Millisecond)

	// Should pass again
	req = httptest.NewRequest("GET", "/subrouter/abc123", nil)
	req.RemoteAddr = "1.1.1.1"
	w = httptest.NewRecorder()
	limiter.Limit(nextHandler).ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("After expiry: status = %d, want %d", w.Code, http.StatusOK)
	}
}

func nextHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
}
```

#### Step 2: Write rate limiter package

```go
package ratelimit

import (
	"net/http"
	"sync"
	"time"
)

// SlidingWindow provides per-IP rate limiting using a sliding window counter.
type SlidingWindow struct {
	mu       sync.Mutex
	clients  map[string]*clientState
	maxReqs  int
	window   time.Duration
}

type clientState struct {
	count  int
	window time.Time
}

// NewSlidingWindow creates a rate limiter allowing maxReqs per window duration.
func NewSlidingWindow(maxReqs int, window time.Duration) *SlidingWindow {
	return &SlidingWindow{
		clients: make(map[string]*clientState),
		maxReqs: maxReqs,
		window:  window,
	}
}

// Limit wraps an http.Handler with per-IP rate limiting.
// Returns 429 Too Many Requests when the limit is exceeded.
func (rl *SlidingWindow) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := r.RemoteAddr
		// Strip port for IPv6 addresses like [::1]:54321
		if host, _, err := net.SplitHostPort(ip); err == nil {
			ip = host
		}

		rl.mu.Lock()
		cs, ok := rl.clients[ip]
		if !ok {
			cs = &clientState{}
			rl.clients[ip] = cs
		}

		now := time.Now()
		if now.Sub(cs.window) > rl.window {
			cs.count = 0
			cs.window = now
		}

		cs.count++
		rl.mu.Unlock()

		if cs.count > rl.maxReqs {
			http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}
```

#### Step 3: Wire rate limiter into handler

```go
// In handler.go — add rate limiter field
type Handler struct {
	store       *store.Store
	proxy       *proxy.Proxy
	rateLimiter *ratelimit.SlidingWindow
}

// In RegisterRoutes:
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	protected := http.NewServeMux()
	protected.HandleFunc("/health", h.healthHandler)
	protected.HandleFunc("/subrouter/", h.subscriptionHandler)

	chain := h.rateLimiter.Limit(protected)
	mux.Handle("/health", chain)
	mux.Handle("/subrouter/", chain)
}
```

#### Step 4: Add rate limit config

Add to `config.go`:

```go
type RateLimitConfig struct {
	Enabled  bool          `yaml:"enabled"`
	MaxReqs  int           `yaml:"max_reqs"`
	Window   time.Duration `yaml:"window"`
}
```

Update `DefaultConfig`:

```go
RateLimit: RateLimitConfig{
	Enabled: false,
	MaxReqs: 100,
	Window:  time.Minute,
},
```

Update `config.yaml`:

```yaml
rate_limit:
  enabled: false  # set to true to enable
  max_reqs: 100   # requests per window
  window: 1m      # window duration
```

#### Step 5: Wire into main.go

```go
var rateLimiter *ratelimit.SlidingWindow
if cfg.RateLimit.Enabled {
	rateLimiter = ratelimit.NewSlidingWindow(cfg.RateLimit.MaxReqs, cfg.RateLimit.Window)
	slog.Info("Rate limiting enabled", "max_reqs", cfg.RateLimit.MaxReqs, "window", cfg.RateLimit.Window)
} else {
	rateLimiter = nil
}

h := handler.New(s, p, rateLimiter)
```

#### Step 6: Run tests

Run: `go test ./internal/ratelimit/... -v`
Expected: All pass.
Run: `go test ./... -v`

#### Step 7: Commit

```bash
git add internal/ratelimit/ internal/handler/handler.go internal/config/config.go config.yaml cmd/xray-sub-rotation/main.go
git commit -m "feat: add per-IP sliding window rate limiting"
```

---

### Task 4: Persistent Store

**Files:**
- Modify: `internal/store/store.go` (add persistence)
- Create: `internal/store/persist_test.go`
- Modify: `config.yaml` (add persist config)

**Complexity:** Medium-High

#### Step 1: Write persistence tests

```go
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

	s := NewStore()
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
	s2 := NewStore()
	if err := s2.LoadFromDisk(path); err != nil {
		t.Fatalf("LoadFromDisk failed: %v", err)
	}

	// Verify endpoints
	endpoints := s2.GetEndpoints()
	if len(endpoints) != 2 {
		t.Fatalf("Expected 2 endpoints, got %d", len(endpoints))
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

	s := NewStore()
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
	s := NewStore()
	err := s.LoadFromDisk("/nonexistent/health.json")
	if err == nil {
		t.Error("Expected error for missing file")
	}
}
```

#### Step 2: Add persistence to store.go

Add to `store.go` struct:

```go
import (
	"encoding/json"
	"os"
)

func (s *Store) Persist(path string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data := make(map[string]HealthInfo, len(s.health))
	for id, info := range s.health {
		data[id] = info
	}
	return os.WriteFile(path, func() []byte {
		b, _ := json.Marshal(data)
		return b
	}(), 0644)
}

func (s *Store) LoadFromDisk(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var healthMap map[string]HealthInfo
	if err := json.Unmarshal(data, &healthMap); err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	for id, info := range healthMap {
		s.health[id] = info
	}
	return nil
}
```

#### Step 3: Wire persistence into main.go

```go
// After loading sublist, try to load persisted health:
if healthPath := cfg.HealthCheck.PersistPath; healthPath != "" {
	if err := s.LoadFromDisk(healthPath); err != nil {
		slog.Warn("Failed to load persisted health (will start fresh)", "error", err)
	} else {
		slog.Info("Loaded persisted health data")
	}
}

// On shutdown, persist health state
defer func() {
	if healthPath := cfg.HealthCheck.PersistPath; healthPath != "" {
		if err := s.Persist(healthPath); err != nil {
			slog.Error("Failed to persist health", "error", err)
		}
	}
}()
```

#### Step 4: Add persist path to config.yaml

```yaml
health_check:
  enabled: true
  interval: 30s
  timeout: 5s
  healthy_count: 2
  persist_path: "/var/lib/xray-sub-rotation/health.json"  # optional
```

#### Step 5: Run tests

Run: `go test ./internal/store/... -v`
Expected: All store tests pass (including new persistence tests).

#### Step 6: Commit

```bash
git add internal/store/store.go internal/store/persist_test.go config.yaml cmd/xray-sub-rotation/main.go
git commit -m "feat: add persistent health store (JSON file)"
```

---

## Summary

| Task | Feature | Complexity | Files Changed |
|------|---------|------------|---------------|
| 1 | TLS/HTTPS | Medium | `tls/tls.go`, `config.go`, `main.go`, `config.yaml` |
| 2 | SIGHUP reload | Medium | `reload/reload.go`, `store.go`, `main.go` |
| 3 | Rate limiting | Medium | `ratelimit/ratelimit.go`, `handler.go`, `config.go`, `config.yaml` |
| 4 | Persistent store | Medium-High | `store/store.go` |

**Total estimated files:** 4 new, ~5 modified

**Order rationale:**
1. **TLS first** — essential for production, pairs with all other features.
2. **SIGHUP second** — infrastructure feature, useful for TLS cert rotation and all subsequent features.
3. **Rate limiting third** — sits cleanly as middleware on top of handlers.
4. **Persistence last** — touches the store (core data structure), best done after the API surface is stable.

**Edge cases to watch:**
- Let's Encrypt cert renewal: certs change on disk; the service reads them at startup only. For zero-downtime cert rotation, consider `acme.sh --onchange` hook to send SIGHUP.
- Rate limiting with SIGHUP: the in-memory rate limiter state is reset on config reload. This is acceptable — clients will briefly see 429s during reload, which is a known limitation.
- Persistent store race condition: `Persist` is called in `defer` at shutdown. If the process is killed with SIGKILL, health data is lost. Acceptable trade-off.
