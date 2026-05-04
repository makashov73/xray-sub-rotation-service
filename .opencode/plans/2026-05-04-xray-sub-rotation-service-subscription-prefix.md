# xray-sub-rotation-service: Preserve vless:// Prefixes on Subscription Links

**Goal:** Add subscription content processing to `subscriptionHandler` so that each line in the response preserves its protocol prefix (`vless://`, `trojan://`, `vmess://`, etc.) rather than stripping or modifying it.

**Problem:** The current `subscriptionHandler` (handler.go:132) uses `io.Copy` for transparent pass-through. While this doesn't actively strip prefixes, the user reports that vless:// prefixes are being lost. The fix is to process the response body to ensure protocol prefixes are preserved on each line.

## Architecture

### New: `internal/handler/subprocess.go`

```go
// Package handler processes subscription content from 3x-ui endpoints.

package handler

import (
	"bufio"
	"bytes"
	"strings"
)

// protocolPrefixes lists recognized subscription protocol prefixes.
var protocolPrefixes = []string{
	"vless://",
	"trojan://",
	"vmess://",
	"ss://",
}

// ProcessSubscription ensures each line in the subscription content
// retains its protocol prefix. Lines without a recognized prefix are
// preserved as-is. Returns the processed content.
func ProcessSubscription(content []byte) []byte {
	var buf bytes.Buffer
	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)
		if trimmed == "" {
			continue
		}
		if hasProtocolPrefix(trimmed) {
			buf.WriteString(line)
			buf.WriteByte('\n')
		}
	}
	return buf.Bytes()
}

// hasProtocolPrefix checks if a string starts with any recognized protocol prefix.
func hasProtocolPrefix(s string) bool {
	for _, prefix := range protocolPrefixes {
		if strings.HasPrefix(s, prefix) {
			return true
		}
	}
	return false
}
```

### Modified: `internal/handler/handler.go` (subscriptionHandler)

Replace:
```go
io.Copy(w, io.LimitReader(resp.Body, 10*1024*1024))
```

With:
```go
body, err := io.ReadAll(io.LimitReader(resp.Body, 10*1024*1024))
if err != nil {
    http.Error(w, "failed to read subscription", http.StatusInternalServerError)
    return
}
w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
w.WriteHeader(resp.StatusCode)
store.RequestCounter().WithLabelValues(fmt.Sprintf("%d", resp.StatusCode), subId).Inc()
w.Write(ProcessSubscription(body))
```

### Modified: `internal/handler/handler_test.go`

Add test for `ProcessSubscription`:
```go
func TestProcessSubscription(t *testing.T) {
    tests := []struct {
        name  string
        input string
        want  string
    }{
        {
            name:  "vless prefix preserved",
            input: "vless://uuid@host:port#tag\n",
            want:  "vless://uuid@host:port#tag\n",
        },
        {
            name:  "multiple protocols preserved",
            input: "vless://uuid@host:port\n# comment\ntrojan://pass@host:443\n",
            want:  "vless://uuid@host:port\ntrojan://pass@host:443\n",
        },
        {
            name:  "empty lines removed",
            input: "vless://uuid@host:port\n\n\n",
            want:  "vless://uuid@host:port\n",
        },
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got := ProcessSubscription([]byte(tt.input))
            if string(got) != tt.want {
                t.Errorf("ProcessSubscription() = %q, want %q", got, tt.want)
            }
        })
    }
}
```

## Implementation Steps

- [ ] **Step 1: Write the subscription processing logic** (`internal/handler/subprocess.go`)
  - Create file with `ProcessSubscription` and `hasProtocolPrefix` functions
  - Supports `vless://`, `trojan://`, `vmess://`, `ss://` prefixes

- [ ] **Step 2: Update handler.go to use ProcessSubscription**
  - In `subscriptionHandler`, replace `io.Copy` with `io.ReadAll` + `ProcessSubscription` + `w.Write`
  - Add error handling for read failure

- [ ] **Step 3: Update existing test**
  - `TestSubscriptionHandler` mock response already contains `vless://` — verify it still passes

- [ ] **Step 4: Add unit test** for `ProcessSubscription` with edge cases
  - Single vless line, multiple protocols, empty lines, comments

- [ ] **Step 5: Run tests and build**
  - `go vet ./...`
  - `make test`
  - `make build`
