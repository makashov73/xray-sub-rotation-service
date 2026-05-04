# Health Endpoint: Per-Subscription Health Report

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpawers:subagent-driven-development to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the current `/health` endpoint's plain-text `ok` response with a JSON body that reports per-subscription, per-server health and latency data.

**Architecture:** Add a single public method `HealthReport()` on `Store` that returns the health status grouped by subId and server name. Modify `healthHandler` in `handler.go` to marshal and return this report as JSON.

**Tech Stack:** Go 1.24+, stdlib only (`encoding/json`, `net/http`).

---

## Data Flow

Current state → desired state:

```
Store (in memory)
  ├── endpoints: map[string]Endpoint        {id → Endpoint{SubId, Name, URL}}
  ├── health:     map[string]HealthInfo     {id → {Healthy, LatencyMS}}
  └── subIdToEndpoints: map[string][]string {subId → [endpointIDs]}

GET /health
  └── JSON:
      {
        "abc123": {
          "US-East":    {Healthy: true,  LatencyMS: 42},
          "EU-West":    {Healthy: false, LatencyMS: 0},
          "Asia-Pacific": {Healthy: true, LatencyMS: 15}
        }
      }
```

## File Structure

| File | Change |
|------|--------|
| `internal/store/store.go` | Add `HealthReport()` method + `ServerHealth` response struct |
| `internal/store/store_test.go` | Tests for `HealthReport()` |
| `internal/handler/handler.go` | Change `healthHandler` to return JSON |
| `internal/handler/handler_test.go` | Update health test to expect JSON |

**No changes needed in:** proxy, sublist, config, cmd, or any other package. The `HealthInfo` struct already has `Healthy` and `LatencyMS` fields — we just need to expose them grouped by subId.

---

### Task 1: Add `HealthReport` Method to Store

**Files:**
- Modify: `internal/store/store.go`
- Create: `internal/store/store_test.go` (append new tests)

- [ ] **Step 1: Write the failing test**

Append to `internal/store/store_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/store/... -v -run TestHealthReport`
Expected: FAIL — `s.HealthReport` method does not exist.

- [ ] **Step 3: Add the `ServerHealth` struct and `HealthReport` method**

Append to `internal/store/store.go` (after the existing `LoadFromDisk` method):

```go
// ServerHealth contains health status for a single server.
type ServerHealth struct {
	Healthy   bool    `json:"healthy"`
	LatencyMS float64 `json:"latency_ms"`
}

// HealthReport returns a map of subId -> serverName -> health info.
// This is used by the /health endpoint to report per-subscription, per-server status.
func (s *Store) HealthReport() map[string]map[string]ServerHealth {
	s.mu.RLock()
	defer s.mu.RUnlock()

	report := make(map[string]map[string]ServerHealth)

	for subId, ids := range s.subIdToEndpoints {
		subReport := make(map[string]ServerHealth, len(ids))
		for _, id := range ids {
			ep, ok := s.endpoints[id]
			if !ok {
				continue
			}
			info, ok := s.health[id]
			if !ok {
				continue
			}
			subReport[ep.Name] = ServerHealth{
				Healthy:   info.Healthy,
				LatencyMS: info.LatencyMS,
			}
		}
		if len(subReport) > 0 {
			report[subId] = subReport
		}
	}

	return report
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/store/... -v -run TestHealthReport`
Expected: PASS (all three tests).

- [ ] **Step 5: Commit**

```bash
git add internal/store/store.go internal/store/store_test.go
git commit -m "feat: add HealthReport method to store for per-subscription health data"
```

---

### Task 2: Update `/health` Handler to Return JSON

**Files:**
- Modify: `internal/handler/handler.go`
- Modify: `internal/handler/handler_test.go`

- [ ] **Step 1: Update the failing test**

Replace the existing `TestHealthCheckEndpoint` function in `internal/handler/handler_test.go`:

```go
func TestHealthCheckEndpoint(t *testing.T) {
	s := store.NewStore("fastest")
	s.AddEndpoint("https://xray1.example.com/sub/abc", "abc123", "US-East")
	s.AddEndpoint("https://xray2.example.com/sub/abc", "abc123", "EU-West")

	// Record some health data
	s.RecordHealth("https://xray1.example.com/sub/abc", store.HealthInfo{
		Healthy:     true,
		LatencyMS:   42,
		LastChecked: time.Now(),
	})
	s.RecordHealth("https://xray2.example.com/sub/abc", store.HealthInfo{
		Healthy:     false,
		LatencyMS:   0,
		LastChecked: time.Now(),
	})

	p := proxy.New(s, "fastest", 2, 5*time.Second)
	h := New(s, p, nil)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !strings.Contains(body, `"abc123"`) {
		t.Errorf("Body missing subId: %s", body)
	}
	if !strings.Contains(body, `"US-East"`) {
		t.Errorf("Body missing server name: %s", body)
	}
}

func TestHealthCheckEndpointEmpty(t *testing.T) {
	s := store.NewStore("fastest")
	p := proxy.New(s, "fastest", 2, 5*time.Second)
	h := New(s, p, nil)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	h.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if body != "{}" {
		t.Errorf("Body = %q, want '{}'", body)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/handler/... -v -run TestHealthCheckEndpoint`
Expected: FAIL — body contains `"ok"`, not JSON.

- [ ] **Step 3: Update the health handler**

Replace the `healthHandler` function in `internal/handler/handler.go` (after the existing `healthHandler` at line 102):

```go
func (h *Handler) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(h.store.HealthReport())
}
```

And add `encoding/json` to the imports at the top of `handler.go`:

Change the import block from:
```go
import (
	"io"
	"net/http"
	"strings"
	"time"
	...
)
```

To:
```go
import (
	"coding/json"
	"io"
	"net/http"
	"strings"
	"time"
	...
)
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/handler/... -v -run TestHealthCheckEndpoint`
Expected: PASS.

Run: `go test ./internal/handler/... -v -run TestHealthCheckEndpointEmpty`
Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add internal/handler/handler.go internal/handler/handler_test.go
git commit -m "feat: return JSON health report from /health endpoint"
```

---

### Task 3: Run Full Test Suite & Build

**Files:** None (verification only)

- [ ] **Step 1: Run all tests**

Run: `go test ./... -v`
Expected: All tests pass.

- [ ] **Step 2: Verify build**

Run: `go build -o /tmp/xray-sub-rotation ./cmd/xray-sub-rotation/`
Expected: Binary builds successfully.

- [ ] **Step 3: Commit**

```bash
git commit -m "test: run full test suite and verify build"
```

---

## Summary

This plan has 3 tasks across 3 files:

1. **Store method** — `HealthReport()` returns `map[string]map[string]ServerHealth` (subId → serverName → health). No data model changes needed; `HealthInfo` already has `Healthy` and `LatencyMS`.
2. **Handler update** — `healthHandler` marshals `HealthReport()` to JSON with `application/json` content type.
3. **Verification** — full test suite + build.

No changes needed in proxy, sublist, config, or cmd — all data is already tracked in `Store` and the handler already has a `*store.Store` reference.

## Concerns

1. **JSON `null` vs `{}` for empty store:** `json.Encode(nil)` produces `null`. Since `HealthReport()` returns a non-nil `map[string]map[string]ServerHealth{}`, empty store produces `"{}"` — this is correct and tested in `TestHealthCheckEndpointEmpty`.
2. **Server name uniqueness:** If two endpoints under the same subId share the same `Name` in `sublist.md`, the second will overwrite the first in the report. This is unlikely in practice (names are meant to be unique per subId), and if needed, the endpoint `ID` (URL) could be used as a fallback key. Not addressed in V1.
3. **Backward compatibility:** Clients currently expecting `"ok"` text will get JSON. This is an intentional breaking change — the spec explicitly requests JSON output.
