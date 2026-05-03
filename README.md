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
