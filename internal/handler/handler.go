package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/makashov73/xray-sub-rotation-service/internal/proxy"
	"github.com/makashov73/xray-sub-rotation-service/internal/ratelimit"
	"github.com/makashov73/xray-sub-rotation-service/internal/store"
)

// Handler handles HTTP requests for subscription routing.
type Handler struct {
	store       *store.Store
	proxy       *proxy.Proxy
	fetcher     *http.Client
	rateLimiter *ratelimit.SlidingWindow
}

// New creates a new Handler.
func New(s *store.Store, p *proxy.Proxy, rl *ratelimit.SlidingWindow) *Handler {
	return &Handler{
		store:       s,
		proxy:       p,
		fetcher:     &http.Client{Timeout: 30 * time.Second},
		rateLimiter: rl,
	}
}

// RegisterRoutes registers all HTTP routes.
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	// Protect routes with rate limiter if configured
	var protected http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/health" || r.URL.Path == "/health/" {
			h.healthHandler(w, r)
			return
		}
		if strings.HasPrefix(r.URL.Path, "/subrouter/") {
			h.subscriptionHandler(w, r)
			return
		}
		http.NotFound(w, r)
	})

	if h.rateLimiter != nil {
		protected = h.rateLimiter.Limit(protected)
	}

	mux.Handle("/health", protected)
	mux.Handle("/subrouter/", protected)
}

// ServeHTTP implements http.Handler for the subscription handler.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	if path == "/health" || path == "/health/" {
		h.healthHandler(w, r)
		return
	}

	if strings.HasPrefix(path, "/subrouter/") {
		h.subscriptionHandler(w, r)
		return
	}

	http.NotFound(w, r)
}

func (h *Handler) subscriptionHandler(w http.ResponseWriter, r *http.Request) {
	// Extract subId from URL path: /subrouter/{subId}
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

	// Fetch subscription from the best 3x-ui endpoint
	resp, err := h.fetcher.Get(best.URL)
	if err != nil {
		http.Error(w, "failed to fetch subscription", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	// Forward the response
	w.Header().Set("Content-Type", resp.Header.Get("Content-Type"))
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
}

func (h *Handler) healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(h.store.HealthReport())
}
