package store

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/makashov73/xray-sub-rotation-service/internal/sublist"
	"github.com/prometheus/client_golang/prometheus"
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

// RequestCounter returns the request counter for use in handlers.
func RequestCounter() *prometheus.CounterVec {
	return requestCounter
}

type storeState struct {
	mu sync.RWMutex

	endpoints        map[string]Endpoint
	subIdToEndpoints map[string][]string
	health           map[string]HealthInfo
	lastServed       map[string]string
	strategy         string
}

// Store holds the list of 3x-ui endpoints and their health status.
type Store struct {
	state atomic.Pointer[storeState]
	rng   *rand.Rand
}

func NewStore(strategy string) *Store {
	s := &Store{
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
	s.state.Store(&storeState{
		endpoints:        make(map[string]Endpoint),
		subIdToEndpoints: make(map[string][]string),
		health:           make(map[string]HealthInfo),
		lastServed:       make(map[string]string),
		strategy:         strategy,
	})
	return s
}

func (s *Store) AddEndpoint(url, subId, name string) {
	st := s.state.Load()

	st.mu.Lock()
	defer st.mu.Unlock()

	id := url
	e := Endpoint{
		ID:    id,
		URL:   url,
		SubId: subId,
		Name:  name,
	}
	st.endpoints[id] = e
	st.subIdToEndpoints[subId] = append(st.subIdToEndpoints[subId], id)
	st.health[id] = HealthInfo{
		Healthy:     true,
		LastChecked: time.Now(),
	}
}

func (s *Store) GetEndpoints() []Endpoint {
	st := s.state.Load()

	result := make([]Endpoint, 0, len(st.endpoints))
	for _, e := range st.endpoints {
		result = append(result, e)
	}
	return result
}

func (s *Store) GetUrlsForSubId(subId string) []string {
	st := s.state.Load()

	ids := st.subIdToEndpoints[subId]
	urls := make([]string, 0, len(ids))
	for _, id := range ids {
		urls = append(urls, st.endpoints[id].URL)
	}
	return urls
}

func (s *Store) RecordHealth(endpointID string, info HealthInfo) {
	st := s.state.Load()

	st.mu.Lock()
	defer st.mu.Unlock()
	st.health[endpointID] = info
}

func (s *Store) GetHealth(endpointID string) (HealthInfo, bool) {
	st := s.state.Load()

	info, ok := st.health[endpointID]
	return info, ok
}

// Reload clears current endpoints and adds new ones.
// This is used during SIGHUP config reload.
// It builds a new storeState and atomically swaps it, so concurrent reads
// always see a consistent snapshot.
func (s *Store) Reload(entries []sublist.Entry) {
	old := s.state.Load()

	old.mu.RLock()
	strategy := old.strategy
	old.mu.RUnlock()

	newState := &storeState{
		endpoints:        make(map[string]Endpoint),
		subIdToEndpoints: make(map[string][]string),
		health:           make(map[string]HealthInfo),
		lastServed:       make(map[string]string),
		strategy:         strategy,
	}
	for _, e := range entries {
		id := e.URL
		newState.endpoints[id] = Endpoint{
			ID:    id,
			URL:   e.URL,
			SubId: e.SubId,
			Name:  e.Name,
		}
		newState.subIdToEndpoints[e.SubId] = append(newState.subIdToEndpoints[e.SubId], id)
		newState.health[id] = HealthInfo{
			Healthy:     true,
			LastChecked: time.Now(),
		}
	}

	s.state.Store(newState)
}

// GetBestEndpoint returns the best endpoint for a subId based on the configured strategy.
// Strategies: "fastest" (lowest latency), "random" (random, avoids repeating last),
// "first" (first healthy). If all are down, returns the most recently checked.
func (s *Store) GetBestEndpoint(subId string) *Endpoint {
	st := s.state.Load()

	ids := st.subIdToEndpoints[subId]
	if len(ids) == 0 {
		return nil
	}

	// Collect healthy endpoints
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
		default: // "fastest"
			chosen = s.pickFastest(st, healthy)
		}
	}

	if chosen != nil {
		st.mu.Lock()
		st.lastServed[subId] = chosen.ID
		st.mu.Unlock()
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
		st.mu.Lock()
		st.lastServed[subId] = best.ID
		st.mu.Unlock()
	}
	return best
}

// pickFastest returns the healthy endpoint with the lowest latency.
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

// pickRandom returns a random healthy endpoint, avoiding the one last served
// for this subId when possible.
func (s *Store) pickRandom(st *storeState, healthy []string, subId string) *Endpoint {
	lastID := st.lastServed[subId]

	// If more than one option, exclude the last served
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

	id := candidates[s.rng.Intn(len(candidates))]
	ep := st.endpoints[id]
	return &ep
}

// Persist writes the health state to a JSON file.
// Creates parent directories if they don't exist.
func (s *Store) Persist(path string) error {
	st := s.state.Load()

	st.mu.Lock()
	defer st.mu.Unlock()

	data := make(map[string]HealthInfo, len(st.health))
	for id, info := range st.health {
		data[id] = info
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return err
	}

	b, err := json.Marshal(data)
	if err != nil {
		return err
	}
	return os.WriteFile(path, b, 0644)
}

// LoadFromDisk reads health state from a JSON file.
func (s *Store) LoadFromDisk(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}

	var healthMap map[string]HealthInfo
	if err := json.Unmarshal(data, &healthMap); err != nil {
		return err
	}

	st := s.state.Load()

	st.mu.Lock()
	defer st.mu.Unlock()
	for id, info := range healthMap {
		st.health[id] = info
	}
	return nil
}

// ServerHealth contains health status for a single server.
type ServerHealth struct {
	Healthy   bool    `json:"healthy"`
	LatencyMS float64 `json:"latency_ms"`
}

// HealthReport returns a map of subId -> serverName -> health info.
// This is used by the /health endpoint to report per-subscription, per-server status.
func (s *Store) HealthReport() map[string]map[string]ServerHealth {
	st := s.state.Load()

	st.mu.Lock()
	defer st.mu.Unlock()

	report := make(map[string]map[string]ServerHealth)

	for subId, ids := range st.subIdToEndpoints {
		subReport := make(map[string]ServerHealth, len(ids))
		for _, id := range ids {
			ep, ok := st.endpoints[id]
			if !ok {
				continue
			}
			info, ok := st.health[id]
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
