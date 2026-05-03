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
