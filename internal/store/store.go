package store

import (
	"encoding/json"
	"math/rand"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/makashov73/xray-sub-rotation-service/internal/sublist"
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
	strategy         string
	lastServed       map[string]string // subId -> last served endpoint ID
}

func NewStore(strategy string) *Store {
	return &Store{
		endpoints:        make(map[string]Endpoint),
		subIdToEndpoints: make(map[string][]string),
		health:           make(map[string]HealthInfo),
		strategy:         strategy,
		lastServed:       make(map[string]string),
	}
}

func (s *Store) AddEndpoint(url, subId, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.addEndpointLocked(url, subId, name)
}

func (s *Store) addEndpointLocked(url, subId, name string) {
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

func (s *Store) GetHealth(endpointID string) (HealthInfo, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	info, ok := s.health[endpointID]
	return info, ok
}

// Reload clears current endpoints and adds new ones.
// This is used during SIGHUP config reload.
func (s *Store) Reload(entries []sublist.Entry) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.endpoints = make(map[string]Endpoint)
	s.subIdToEndpoints = make(map[string][]string)
	s.health = make(map[string]HealthInfo)
	s.lastServed = make(map[string]string)

	for _, e := range entries {
		s.addEndpointLocked(e.URL, e.SubId, e.Name)
	}
}

// GetBestEndpoint returns the best endpoint for a subId based on the configured strategy.
// Strategies: "fastest" (lowest latency), "random" (random, avoids repeating last),
// "first" (first healthy). If all are down, returns the most recently checked.
func (s *Store) GetBestEndpoint(subId string) *Endpoint {
	s.mu.Lock()
	defer s.mu.Unlock()

	ids := s.subIdToEndpoints[subId]
	if len(ids) == 0 {
		return nil
	}

	// Collect healthy endpoints
	healthy := make([]string, 0, len(ids))
	for _, id := range ids {
		info, ok := s.health[id]
		if ok && info.Healthy {
			healthy = append(healthy, id)
		}
	}

	var chosen *Endpoint

	if len(healthy) > 0 {
		switch s.strategy {
		case "random":
			chosen = s.pickRandom(healthy, subId)
		case "first":
			ep := s.endpoints[healthy[0]]
			chosen = &ep
		default: // "fastest"
			chosen = s.pickFastest(healthy)
		}
	}

	if chosen != nil {
		s.lastServed[subId] = chosen.ID
		return chosen
	}

	// All unhealthy — return the most recently checked
	var best *Endpoint
	for _, id := range ids {
		info, ok := s.health[id]
		if !ok {
			continue
		}
		ep := s.endpoints[id]
		if best == nil || info.LastChecked.After(s.health[best.ID].LastChecked) {
			best = &ep
		}
	}

	if best != nil {
		s.lastServed[subId] = best.ID
	}
	return best
}

// pickFastest returns the healthy endpoint with the lowest latency.
func (s *Store) pickFastest(healthy []string) *Endpoint {
	var best *Endpoint
	for _, id := range healthy {
		ep := s.endpoints[id]
		if best == nil || s.health[id].LatencyMS < s.health[best.ID].LatencyMS {
			best = &ep
		}
	}
	return best
}

// pickRandom returns a random healthy endpoint, avoiding the one last served
// for this subId when possible.
func (s *Store) pickRandom(healthy []string, subId string) *Endpoint {
	lastID := s.lastServed[subId]

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

	id := candidates[rand.Intn(len(candidates))]
	ep := s.endpoints[id]
	return &ep
}

// Persist writes the health state to a JSON file.
// Creates parent directories if they don't exist.
func (s *Store) Persist(path string) error {
	s.mu.RLock()
	defer s.mu.RUnlock()

	data := make(map[string]HealthInfo, len(s.health))
	for id, info := range s.health {
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

	s.mu.Lock()
	defer s.mu.Unlock()
	for id, info := range healthMap {
		s.health[id] = info
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
