# Architectural Fixes for xray-sub-rotation-service

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpawers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Fix critical data races, add observability, and improve security headers across the codebase.

**Architecture:** A single-threaded `sync.RWMutex` Store and an unseeded `rand.Intn` create race conditions under concurrent access. The handler performs unbounded `io.Copy` and fetches without context. The plan groups fixes into four phases: (1) correctness bugs that can cause data corruption or panics, (2) security and HTTP hygiene, (3) observability, and (4) cleanup. Each phase is self-contained and independently testable.

**Tech Stack:** Go 1.24+, `net/http` stdlib, `gopkg.in/yaml.v3`, `github.com/prometheus/client_golang/prometheus/promhttp` (new dependency for metrics).

---

## Phase 1: Correctness (P0 bugs — prevent data corruption and crashes)

### Task 1: Fix SIGHUP reload data race

**Files:**
- Modify: `cmd/xray-sub-rotation/main.go:139-156`
- Modify: `internal/store/store.go:109-121`
- Test: `internal/store/store_test.go`

**Problem:** `main.go:140-155` spawns a goroutine that calls `s.Reload(entries)` on SIGHUP. While `Reload()` holds `s.mu`, concurrent `GetBestEndpoint()` calls from the handler do NOT hold the lock at the point where they read `s.subIdToEndpoints` — they call `s.GetBestEndpoint(subId)` which locks, but the handler's `http.Get(best.URL)` (line 90 of handler.go) happens *after* releasing the lock. More critically, `Reload()` replaces `s.lastServed`, `s.health`, and `s.subIdToEndpoints` with fresh maps. If `GetBestEndpoint` is mid-flight when `Reload` runs, it races on reading these fields.

**Fix:** Use `sync.RWMutex.RLock` in `Reload()` for the duration of the map replacement, then swap a new `Store` instance atomically via `atomic.Pointer[Store]`, or better yet, add `Reload()` as a method that takes a lock and signals a `sync.Cond` or channel to block concurrent reads. The simplest correct approach: add a `Reload()` method that swaps the entire `Store` struct via an internal `atomic.Pointer` so `GetBestEndpoint` reads a consistent snapshot.

**Implementation approach:** Replace the mutex-wrapped maps with an `atomic.Pointer[storeState]`. `storeState` is a plain struct holding all maps. `GetBestEndpoint` loads the pointer once, works on the immutable snapshot, and releases. `Reload` builds a new `storeState` and calls `atomic.StorePointer`. This is a zero-allocation change and guarantees no concurrent access.

```go
// store.go imports — add to existing imports:
import (
    "sync/atomic"
    // ... existing imports
)

type storeState struct {
    endpoints        map[string]Endpoint
    subIdToEndpoints map[string][]string
    health           map[string]HealthInfo
    lastServed       map[string]string
    strategy         string
}

type Store struct {
    state atomic.Pointer[storeState]
}

func NewStore(strategy string) *Store {
    s := &Store{}
    s.state.Store(&storeState{
        endpoints:        make(map[string]Endpoint),
        subIdToEndpoints: make(map[string][]string),
        health:           make(map[string]HealthInfo),
        lastServed:       make(map[string]string),
        strategy:         strategy,
    })
    return s
}

// GetBestEndpoint loads a consistent snapshot and works on it.
func (s *Store) GetBestEndpoint(subId string) *Endpoint {
    st := s.state.Load()

    ids := st.subIdToEndpoints[subId]
    if len(ids) == 0 {
        return nil
    }

    healthy := make([]string, 0, len(ids))
    for _, id := range ids {
        info, ok := st.health[id]
        if ok && info.Healthy {
            healthy = append(healthy, id)
        }
    }

    var chosen *Endpoint
    if len(healthy) > 0 {
        switch st.strategy {
        case "random":
            chosen = s.pickRandom(st, healthy, subId)
        case "first":
            ep := st.endpoints[healthy[0]]
            chosen = &ep
        default:
            chosen = s.pickFastest(st, healthy)
        }
    }

    if chosen != nil {
        st.lastServed[subId] = chosen.ID
        return chosen
    }

    // All unhealthy — return the most recently checked
    var best *Endpoint
    for _, id := range ids {
        info, ok := st.health[id]
        if !ok {
            continue
        }
        ep := st.endpoints[id]
        if best == nil || info.LastChecked.After(st.health[best.ID].LastChecked) {
            best = &ep
        }
    }

    if best != nil {
        st.lastServed[subId] = best.ID
    }
    return best
}

// pickFastest and pickRandom are updated to work on *storeState snapshots.
func (s *Store) pickFastest(st *storeState, healthy []string) *Endpoint {
    var best *Endpoint
    for _, id := range healthy {
        ep := st.endpoints[id]
        if best == nil || st.health[id].LatencyMS < st.health[best.ID].LatencyMS {
            best = &ep
        }
    }
    return best
}

func (s *Store) pickRandom(st *storeState, healthy []string, subId string) *Endpoint {
    lastID := st.lastServed[subId]
    candidates := healthy
    if len(healthy) > 1 && lastID != "" {
        candidates = make([]string, 0, len(healthy)-1)
        for _, id := range healthy {
            if id != lastID {
                candidates = append(candidates, id)
            }
        }
        if len(candidates) == 0 {
            candidates = healthy
        }
    }
    id := s.rng.Intn(len(candidates))
    ep := st.endpoints[id]
    return &ep
}
```

**Tests:** Add `TestReloadConcurrentGetBestEndpoint` in `store_test.go`:
```go
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
```
Run with `-race` flag to verify.

**Commit message:** `fix(store): eliminate data race in SIGHUP reload using atomic store swap`

---

### Task 2: Seed the random number generator

**Files:**
- Modify: `internal/store/store.go:1-11` (imports)
- Modify: `internal/store/store.go:39-47` (NewStore)

**Problem:** `rand.Intn(0)` (line 213 of store.go) panics when called with `rand.Intn(0)` which happens if `candidates` is empty. Additionally, `math/rand` defaults to a fixed seed, so `pickRandom` always returns the same endpoint in the same order.

**Fix:** Create a `*rand.Rand` with `rand.NewSource(time.Now().UnixNano())` in `NewStore`. Pass it to `pickRandom`.

```go
import (
    "math/rand"
    "time"
)

type Store struct {
    state atomic.Pointer[storeState]
    rng   *rand.Rand  // added here
}

func NewStore(strategy string) *Store {
    s := &Store{}
    s.state.Store(&storeState{
        endpoints:        make(map[string]Endpoint),
        subIdToEndpoints: make(map[string][]string),
        health:           make(map[string]HealthInfo),
        lastServed:       make(map[string]string),
        strategy:         strategy,
    })
    s.rng = rand.New(rand.NewSource(time.Now().UnixNano()))
    return s
}

// pickRandom is already defined in Task 1 above and uses s.rng — no further changes needed.
```

**Tests:** `TestRandomStrategyNonDeterministic` — verify two stores with same data return different endpoints.

**Commit message:** `fix(store): seed random generator for true randomness`

---

### Task 3: Add context to subscription fetch and limit io.Copy

**Files:**
- Modify: `internal/handler/handler.go:73-101`

**Problem:** `handler.go:90` uses `h.fetcher.Get(best.URL)` — no context means the fetch can hang forever if the upstream is unresponsive. `handler.go:100` uses `io.Copy(w, resp.Body)` with no size limit, allowing an attacker to send an unbounded response that consumes server memory.

**Fix:** Use `http.NewRequestWithContext(r.Context(), ...)` and wrap the body in `io.LimitReader` (like `proxy.go:82` already does).

```go
func (h *Handler) subscriptionHandler(w http.ResponseWriter, r *http.Request) {
    path := strings.TrimPrefix(r.URL.Path, "/subrouter/")
    subId := strings.TrimSuffix(path, "/")

    if subId == "" {
        http.Error(w, "subId required", http.StatusBadRequest)
        return
    }

    best := h.store.GetBestEndpoint(subId)
    if best == nil {
        http.Error(w, "no available endpoints for this subId", http.StatusNotFound)
        return
    }

    req, err := http.NewRequestWithContext(r.Context(), "GET", best.URL, nil)
    if err != nil {
        http.Error(w, "failed to create request", http.StatusBadGateway)
        return
    }

    resp, err := h.fetcher.Do(req)
    if err != nil {
        http.Error(w, "failed to fetch subscription", http.StatusBadGateway)
        return
    }
    defer resp.Body.Close()

    w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
    w.WriteHeader(resp.StatusCode)
    io.Copy(w, io.LimitReader(resp.Body, 10*1024*1024)) // 10MB limit
}
```

**Tests:** Add `TestSubscriptionHandlerContextCancellation` — cancel the request context and verify the handler returns early (not a leak). Add `TestSubscriptionHandlerBodyLimit` — verify body is truncated at 10MB.

**Commit message:** `fix(handler): add context to subscription fetch, limit response body to 10MB`

---

### Task 4: Fix Server.Shutdown timeout

**Files:**
- Modify: `cmd/xray-sub-rotation/main.go:158-167`

**Problem:** `server.Shutdown(context.Background())` will wait forever if connections can't be drained.

**Fix:** Use a 10-second timeout.

```go
ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
defer cancel()
server.Shutdown(ctx)
```

**Commit message:** `fix(main): add timeout to server shutdown`

---

## Phase 2: HTTP Security and Hygiene

### Task 5: Add X-Forwarded-For / X-Real-IP support to rate limiter

**Files:**
- Modify: `internal/ratelimit/ratelimit.go:34-41`

**Problem:** `ratelimit.go:36-40` uses `r.RemoteAddr` directly. Behind a reverse proxy (nginx, Cloudflare), this is the proxy's IP, not the client's.

**Fix:** Check `X-Forwarded-For` first, then `X-Real-IP`, then fall back to `RemoteAddr`.

```go
func extractClientIP(r *http.Request) string {
    if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
        // Use the first IP in the chain
        parts := strings.Split(xff, ",")
        ip := strings.TrimSpace(parts[0])
        if ip != "" {
            return ip
        }
    }
    if xri := r.Header.Get("X-Real-IP"); xri != "" {
        return xri
    }
    ip := r.RemoteAddr
    if host, _, err := net.SplitHostPort(ip); err == nil {
        ip = host
    }
    return ip
}
```

**Tests:** `TestRateLimitXForwardedFor`, `TestRateLimitXRealIP`, `TestRateLimitFallbackToRemoteAddr`.

**Commit message:** `fix(ratelimit): use X-Forwarded-For and X-Real-IP for client IP extraction`

---

### Task 6: Add Retry-After header on 429

**Files:**
- Modify: `internal/ratelimit/ratelimit.go:58-61`

**Problem:** `ratelimit.go:58-59` returns 429 without `Retry-After` header, making it hard for clients to back off gracefully.

**Fix:**
```go
if cs.count > rl.maxReqs {
    w.Header().Set("Retry-After", strconv.FormatInt(int64(rl.window.Seconds()), 10))
    http.Error(w, "rate limit exceeded", http.StatusTooManyRequests)
    return
}
```

Add `strconv` import.

**Tests:** Verify the header is present and has the correct value in `TestRateLimitExpiry` or a new test.

**Commit message:** `fix(ratelimit): add Retry-After header on 429 responses`

---

### Task 7: Add security headers middleware

**Files:**
- Create: `internal/handler/security.go`
- Modify: `internal/handler/handler.go:34-54` (imports and registration)

**Problem:** No security headers are set on any response.

**Fix:** Create a middleware that adds:
- `X-Content-Type-Options: nosniff`
- `X-Frame-Options: DENY`
- `X-XSS-Protection: 1; mode=block`
- `Strict-Transport-Security: max-age=31536000; includeSubDomains` (only for HTTPS, or always safe)
- `Cache-Control: no-store`

```go
// security.go
package handler

import "net/http"

// SecurityHeaders adds common security headers to responses.
func SecurityHeaders(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("X-Content-Type-Options", "nosniff")
        w.Header().Set("X-Frame-Options", "DENY")
        w.Header().Set("X-XSS-Protection", "1; mode=block")
        w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
        w.Header().Set("Cache-Control", "no-store")
        next.ServeHTTP(w, r)
    })
}
```

In `handler.go`, wrap the protected handler:
```go
protected = SecurityHeaders(protected)
```

**Tests:** `TestSecurityHeaders` — verify headers are present on a response.

**Commit message:** `fix(handler): add security headers middleware`

---

### Task 8: Remove or implement AuthConfig

**Files:**
- Modify: `internal/config/config.go:49-51` (AuthConfig)
- Modify: `cmd/xray-sub-rotation/main.go:16-20` (imports)

**Decision:** Since `AuthConfig` is parsed from config but never used, and there's no auth implementation, remove it entirely. This is cleaner than leaving dead code.

**Changes:**
- Remove `Auth AuthConfig` from `Config` struct
- Remove `AuthConfig` type
- Remove `Auth.APIKey` references from `main.go:73` (which logs it)
- Update `config_test.go:100-102` test that sets `auth.api_key`
- Update `config.yaml` to remove `auth:` section
- Update `reload_test.go:22-23` test config

**Tests:** Update existing `TestLoadConfig` and `TestReloadConfig` to no longer include auth config.

**Commit message:** `refactor(config): remove unused AuthConfig`

---

## Phase 3: Observability

### Task 9: Add `/metrics` endpoint with Prometheus

**Files:**
- Add dependency: `prometheus/client_golang/prometheus/promhttp`
- Modify: `cmd/xray-sub-rotation/main.go:101-104` (register metrics handler)
- Modify: `internal/store/store.go` (add metrics collection)

**Implementation:**
```go
// In store.go, add:
import "github.com/prometheus/client_golang/prometheus"

var (
    endpointHealthGauge = prometheus.NewGaugeVec(
        prometheus.GaugeOpts{
            Name: "xray_endpoint_health",
            Help: "Health status of endpoints (1=healthy, 0=unhealthy)",
        },
        []string{"subid", "name"},
    )
    requestCounter = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "xray_subscription_requests_total",
            Help: "Total subscription requests by status",
        },
        []string{"status", "subid"},
    )
)

func init() {
    prometheus.MustRegister(endpointHealthGauge, requestCounter)
}
```

In handler, increment counter on response:
```go
requestCounter.WithLabelValues(fmt.Sprintf("%d", resp.StatusCode), subId).Inc()
```

In `main.go`:
```go
mux.Handle("/metrics", promhttp.Handler())
```

**Tests:** Add `TestMetricsEndpoint` — verify the endpoint returns Prometheus format data.

**Commit message:** `feat(handler): add Prometheus metrics endpoint`

---

### Task 10: Add X-Request-Id middleware

**Files:**
- Create: `internal/handler/requestid.go`

```go
package handler

import (
    "crypto/rand"
    "encoding/hex"
    "net/http"
)

// RequestID generates a unique ID for each request.
func RequestID(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        id := make([]byte, 16)
        rand.Read(id)
        requestID := hex.EncodeToString(id)
        w.Header().Set("X-Request-Id", requestID)
        next.ServeHTTP(w, r)
    })
}
```

Add to the middleware chain:
```go
protected = RequestID(SecurityHeaders(protected))
```

**Tests:** `TestRequestID` — verify header is present and unique.

**Commit message:** `feat(handler): add X-Request-Id middleware`

---

### Task 11: Add liveness endpoint

**Files:**
- Modify: `internal/handler/handler.go` (add `liveHandler`)

```go
func (h *Handler) liveHandler(w http.ResponseWriter, r *http.Request) {
    w.WriteHeader(http.StatusOK)
    w.Write([]byte("ok"))
}
```

In `RegisterRoutes`, add:
```go
mux.HandleFunc("/livez", h.liveHandler)
```

**Commit message:** `feat(handler): add /livez liveness endpoint`

---

## Phase 4: Cleanup and Validation

### Task 12: Validate strategy in config

**Files:**
- Modify: `internal/config/config.go:85-102` (LoadConfig)

**Problem:** `config.go:76` accepts any string as strategy. Invalid values fall through to `default: "fastest"`.

**Fix:** Add `Validate()` method and call it from `LoadConfig`.

```go
var validStrategies = map[string]bool{"fastest": true, "random": true, "first": true}

func (c *Config) Validate() error {
    if !validStrategies[c.Strategy] {
        return fmt.Errorf("invalid strategy %q: must be one of fastest, random, first", c.Strategy)
    }
    return nil
}

func LoadConfig(path string) (Config, error) {
    cfg := DefaultConfig()
    data, err := os.ReadFile(path)
    if err != nil {
        return cfg, err
    }
    if err := yaml.Unmarshal(data, &cfg); err != nil {
        return cfg, err
    }
    if err := cfg.Validate(); err != nil {
        return cfg, err
    }
    return cfg, nil
}
```

**Tests:** `TestLoadConfigInvalidStrategy` — pass `"bogus"` and verify error.

**Commit message:** `fix(config): validate strategy field`

---

### Task 13: Validate sublist URLs

**Files:**
- Modify: `internal/sublist/parser.go:17-53`

**Problem:** `parser.go:38-39` accepts any string as a URL — malformed URLs can cause confusing errors downstream.

**Fix:** Validate URL format during parse.

```go
import "net/url"

func Parse(path string) ([]Entry, error) {
    f, err := os.Open(path)
    if err != nil {
        return nil, err
    }
    defer f.Close()

    var entries []Entry
    scanner := bufio.NewScanner(f)
    lineNum := 0

    for scanner.Scan() {
        lineNum++
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
        if _, err := url.Parse(url); err != nil {
            return entries, fmt.Errorf("line %d: invalid URL %q: %w", lineNum, url, err)
        }
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

Add line number tracking.

**Tests:** `TestParseInvalidURL` — verify error on bad URL.

**Commit message:** `fix(sublist): validate URLs during parsing`

---

### Task 14: Fix health check to use HEAD

**Files:**
- Modify: `internal/proxy/proxy.go:61`

**Problem:** `proxy.go:61` uses `GET` for health checks. HEAD is more efficient — the server doesn't need to send the body.

**Fix:**
```go
req, err := http.NewRequestWithContext(r.Context(), "HEAD", ep.URL, nil)
```

**Tests:** `TestCheckEndpointUsesHEAD` — verify method is HEAD.

**Commit message:** `fix(proxy): use HEAD for health checks`

---

### Task 15: Clean up deprecated `healthy_count` config field

**Files:**
- Modify: `internal/config/config.go:45` (remove `HealthyCount`)
- Modify: `config.yaml:9` (remove comment)
- Modify: `internal/config/config_test.go:96-97` (remove healthy_count from test)
- Modify: `internal/reload/reload_test.go:46` (remove healthy_count)

**Commit message:** `refactor(config): remove deprecated healthy_count field`

---

## Testing Strategy

### A. Enable race detection

**Files:**
- Modify: `Makefile:9-10`

```makefile
test-race:
    go test -race ./... -v
```

### B. Integration tests

**Files:**
- Create: `internal/handler/integration_test.go`

```go
package handler_test

import (
    "net/http"
    "net/http/httptest"
    "testing"
    "time"

    "github.com/makashov73/xray-sub-rotation-service/internal/handler"
    "github.com/makashov73/xray-sub-rotation-service/internal/proxy"
    "github.com/makashov73/xray-sub-rotation-service/internal/ratelimit"
    "github.com/makashov73/xray-sub-rotation-service/internal/store"
)

func TestFullServer(t *testing.T) {
    s := store.NewStore("random")
    p := proxy.New(s, "random", 5*time.Second)
    rl := ratelimit.NewSlidingWindow(10, time.Second)
    h := handler.New(s, p, rl)
    mux := http.NewServeMux()
    h.RegisterRoutes(mux)

    srv := httptest.NewServer(mux)
    defer srv.Close()

    resp, err := http.Get(srv.URL + "/subrouter/abc123")
    if err != nil {
        t.Fatal(err)
    }
    if resp.StatusCode != http.StatusNotFound {
        t.Errorf("Status = %d, want %d", resp.StatusCode, http.StatusNotFound)
    }
}
```

### C. Concurrency tests

**Files:**
- Add to `internal/store/store_test.go`:
  - `TestReloadConcurrentGetBestEndpoint` (see Task 1)
  - `TestConcurrentHealthRecord`
  - `TestConcurrentPersistLoad`

### D. TLS failure tests

**Files:**
- Add to `internal/tls/tls_test.go`:
  - `TestLoadCorruptCert`
  - `TestLoadMissingKey`

---

## Migration Notes

1. **Strategy validation is a breaking change:** Config files with invalid strategy values will now fail to load. Update any non-standard strategy values in `config.yaml`.

2. **`healthy_count` removal:** The deprecated `healthy_count` field is removed. Remove it from your `config.yaml`.

3. **AuthConfig removal:** If `auth.api_key` was set in your config, it will be silently ignored. This field was never used, so this is safe.

4. **Prometheus dependency:** Add `github.com/prometheus/client_golang/prometheus/promhttp` to `go.mod`.

---

## Verification Checklist

- [ ] `go vet ./...` passes
- [ ] `make test` passes
- [ ] `make test-race` passes (race detection enabled)
- [ ] `make build` succeeds
- [ ] SIGHUP reload with concurrent requests doesn't panic (manual test or race test)
- [ ] `/health` returns valid JSON
- [ ] `/metrics` returns Prometheus-format data
- [ ] `/livez` returns 200 OK
- [ ] 429 responses include `Retry-After` header
- [ ] All responses include `X-Content-Type-Options: nosniff`
- [ ] All responses include `X-Request-Id`
- [ ] Strategy validation rejects invalid values
- [ ] Sublist parsing rejects malformed URLs
- [ ] Health checks use HEAD method
- [ ] Subscription fetches respect request context (cancel test)
- [ ] Subscription response body is limited to 10MB
- [ ] Server shuts down within 10s timeout
