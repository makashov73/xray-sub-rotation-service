# xray-sub-rotation-service (test)

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

## Install

One-liner — downloads the correct binary for your platform and installs it:

```bash
# Latest version
curl -fsSL https://raw.githubusercontent.com/makashov73/xray-sub-rotation-service/master/install.sh | bash

# Pin a version
curl -fsSL https://raw.githubusercontent.com/makashov73/xray-sub-rotation-service/master/install.sh | bash -s -- --version v1.0.0

# Install with config template
curl -fsSL https://raw.githubusercontent.com/makashov73/xray-sub-rotation-service/master/install.sh | bash -s -- --with-config

# Install to custom directory
curl -fsSL https://raw.githubusercontent.com/makashov73/xray-sub-rotation-service/master/install.sh | bash -s -- -d /usr/local/bin

# Install and run immediately
curl -fsSL https://raw.githubusercontent.com/makashov73/xray-sub-rotation-service/master/install.sh | bash -s -- --run
```

Or download the script first:

```bash
curl -LO https://raw.githubusercontent.com/makashov73/xray-sub-rotation-service/master/install.sh
chmod +x install.sh
./install.sh --help
./install.sh --dry-run           # preview actions
./install.sh --with-config --run # install, fetch config, and start
```

The script:
- Detects OS (linux/darwin) and architecture (amd64/arm64)
- Checks for required tools (curl/wget, tar, sha256sum/shasum)
- Verifies downloaded binary against SHA-256 checksums
- Installs to `~/.local/bin/` by default

## Uninstall

```bash
curl -fsSL https://raw.githubusercontent.com/makashov73/xray-sub-rotation-service/master/uninstall.sh | bash

# Remove binary + config
curl -fsSL https://raw.githubusercontent.com/makashov73/xray-sub-rotation-service/master/uninstall.sh | bash -s -- --with-config

# Custom install directory
curl -fsSL .../uninstall.sh | bash -s -- -d /usr/local/bin --with-config
```

### Building from source

```bash
make build    # compile binary
make run      # run the service
make test     # run tests
make lint     # run golangci-lint
```
