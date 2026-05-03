package store

import (
	"sync"
	"time"
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
}

func NewStore() *Store {
	return &Store{
		endpoints:        make(map[string]Endpoint),
		subIdToEndpoints: make(map[string][]string),
		health:           make(map[string]HealthInfo),
	}
}

func (s *Store) AddEndpoint(url, subId, name string) {
	s.mu.Lock()
	defer s.mu.Unlock()

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

// GetBestEndpoint returns the best (healthy, lowest latency) endpoint for a subId.
// If all are down, returns the most recently checked.
func (s *Store) GetBestEndpoint(subId string) *Endpoint {
	s.mu.RLock()
	defer s.mu.RUnlock()

	ids := s.subIdToEndpoints[subId]
	if len(ids) == 0 {
		return nil
	}

	var best *Endpoint
	for _, id := range ids {
		info, ok := s.health[id]
		if !ok {
			continue
		}
		if !info.Healthy {
			continue
		}
		ep := s.endpoints[id]
		if best == nil || info.LatencyMS < s.health[best.ID].LatencyMS {
			best = &ep
		}
	}

	if best != nil {
		return best
	}

	// All unhealthy — return the most recently checked
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

	return best
}
