# xray-sub-rotation-service

Go HTTP service routing 3x-ui subscription requests across multiple 3x-ui instances.

## Commands

```bash
make build     # go build -o xray-sub-rotation ./cmd/xray-sub-rotation/
make run       # go run ./cmd/xray-sub-rotation/
make test      # go test ./... -v
make lint      # golangci-lint run ./... (requires golangci-lint)
make clean     # rm -f xray-sub-rotation
```

Default verification: `go vet ./...` then `make test`, then `make build`.

## Architecture

| Package | Responsibility |
|---------|---------------|
| `cmd/xray-sub-rotation` | Entry point — wires config, store, proxy, handler; signal handling |
| `internal/config` | YAML config parsing (`config.yaml`) |
| `internal/store` | In-memory endpoint registry + health tracking |
| `internal/proxy` | Background HEAD-based health checker |
| `internal/handler` | HTTP routes (`/health`, `/subrouter/{subId}`) |
| `internal/sublist` | Parses `sublist.md` (pipe-delimited: `subId | URL | Name`) |

**Stack:** Go 1.24+, `net/http` stdlib only, `gopkg.in/yaml.v3` for config.

## Known gotchas

- The implementation plan (`.superpawers/plans/2026-05-04-xray-sub-rotation-service.md`) has a known bug: it references `h.proxy.GetBestEndpointForSubId(subId)` but the method lives on `h.store`, not `h.proxy`. Trust the code, not the plan's snippet.
- Handler uses `http.Client{Timeout: 30s}` for subscription fetches — never use the default unbounded `http.Get` on external URLs.
- Health check only counts 2xx as healthy (not 3xx).
- On `os.Exit(1)` for startup failures (config/load), no graceful shutdown.
- Auth is configured in `config.yaml` but not implemented yet (V2).
