package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/makashov73/xray-sub-rotation-service/internal/config"
	"github.com/makashov73/xray-sub-rotation-service/internal/handler"
	"github.com/makashov73/xray-sub-rotation-service/internal/proxy"
	"github.com/makashov73/xray-sub-rotation-service/internal/ratelimit"
	"github.com/makashov73/xray-sub-rotation-service/internal/reload"
	"github.com/makashov73/xray-sub-rotation-service/internal/store"
	"github.com/makashov73/xray-sub-rotation-service/internal/sublist"
	"github.com/makashov73/xray-sub-rotation-service/internal/tls"
)

var version = "dev"

func main() {
	// --version flag
	if len(os.Args) > 1 && os.Args[1] == "--version" {
		fmt.Println("xray-sub-rotation", version)
		os.Exit(0)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// Load config
	cfgPath := "config.yaml"
	cfg, err := config.LoadConfig(cfgPath)
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	// Initialize store
	s := store.NewStore(cfg.Strategy)

	// Load subscription list
	entries, err := sublist.Parse(cfg.SublistFile)
	if err != nil {
		slog.Error("Failed to parse sublist", "error", err)
		os.Exit(1)
	}

	for _, e := range entries {
		s.AddEndpoint(e.URL, e.SubId, e.Name)
	}

	slog.Info("Loaded subscription list", "endpoints", len(entries))

	// Log unique subscription URLs
	scheme := "http"
	if cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		scheme = "https"
	}
	seen := make(map[string]bool)
	for _, e := range entries {
		if seen[e.SubId] {
			continue
		}
		seen[e.SubId] = true
		url := cfg.Server.BuildSubscriptionURL(cfg.Server.Domain, scheme, e.SubId)
		slog.Info("Subscription URL", "url", url)
	}

	// Load persisted health state
	if healthPath := cfg.HealthCheck.PersistPath; healthPath != "" {
		if err := s.LoadFromDisk(healthPath); err != nil {
			slog.Warn("Failed to load persisted health (will start fresh)", "error", err)
		} else {
			slog.Info("Loaded persisted health data")
		}
	}

	// Initialize proxy
	p := proxy.New(s, cfg.Strategy, cfg.HealthCheck.Timeout)

	// Start health check
	var stopChan chan struct{}
	if cfg.HealthCheck.Enabled {
		stopChan = make(chan struct{})
		p.StartHealthCheck(cfg.HealthCheck.Interval, stopChan)
		slog.Info("Health checker started", "interval", cfg.HealthCheck.Interval)
	}

	// Initialize rate limiter
	var rateLimiter *ratelimit.SlidingWindow
	if cfg.RateLimit.Enabled {
		rateLimiter = ratelimit.NewSlidingWindow(cfg.RateLimit.MaxReqs, cfg.RateLimit.Window)
		slog.Info("Rate limiting enabled", "max_reqs", cfg.RateLimit.MaxReqs, "window", cfg.RateLimit.Window)
	}

	// Initialize handler and register routes
	h := handler.New(s, p, rateLimiter)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Start server
	addr := cfg.Server.Host + ":" + strconv.Itoa(cfg.Server.Port)
	slog.Info("Starting server", "addr", addr)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	if cfg.TLS.CertFile != "" && cfg.TLS.KeyFile != "" {
		if err := tls.LoadAndVerify(cfg.TLS.CertFile, cfg.TLS.KeyFile); err != nil {
			slog.Error("Failed to verify TLS certificates", "error", err)
			os.Exit(1)
		}
		slog.Info("Starting HTTPS server", "addr", addr)
		go func() {
			if err := server.ListenAndServeTLS(cfg.TLS.CertFile, cfg.TLS.KeyFile); err != nil && err != http.ErrServerClosed {
				slog.Error("HTTPS server error", "error", err)
			}
		}()
	} else {
		slog.Info("Starting HTTP server", "addr", addr)
		go func() {
			if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("HTTP server error", "error", err)
			}
		}()
	}

	// SIGHUP for config reload
	slog.Info("Listening for SIGHUP for config reload")
	sighup := make(chan os.Signal, 1)
	signal.Notify(sighup, syscall.SIGHUP)
	go func() {
		for range sighup {
			slog.Info("Received SIGHUP, reloading config...")
			newCfg, err := reload.ReloadConfig(cfgPath)
			if err != nil {
				slog.Error("Failed to reload config", "error", err)
				continue
			}
			entries, err := reload.ReloadEndpoints(newCfg.SublistFile)
			if err != nil {
				slog.Error("Failed to reload sublist", "error", err)
				continue
			}
			s.Reload(entries)
			cfg = newCfg
			slog.Info("Config reloaded successfully")
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	cancel()

	if stopChan != nil {
		close(stopChan)
	}

	slog.Info("Shutting down server")
	server.Shutdown(context.Background())

	// Persist health state on shutdown
	if healthPath := cfg.HealthCheck.PersistPath; healthPath != "" {
		if err := s.Persist(healthPath); err != nil {
			slog.Error("Failed to persist health", "error", err)
		}
	}
}
