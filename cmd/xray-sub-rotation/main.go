package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/makashov73/xray-sub-rotation-service/internal/config"
	"github.com/makashov73/xray-sub-rotation-service/internal/handler"
	"github.com/makashov73/xray-sub-rotation-service/internal/proxy"
	"github.com/makashov73/xray-sub-rotation-service/internal/store"
	"github.com/makashov73/xray-sub-rotation-service/internal/sublist"
)

func main() {
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
	s := store.NewStore()

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

	// Initialize proxy
	p := proxy.New(s, cfg.Strategy, cfg.HealthCheck.HealthyCount, cfg.HealthCheck.Timeout)

	// Start health check
	var stopChan chan struct{}
	if cfg.HealthCheck.Enabled {
		stopChan = make(chan struct{})
		p.StartHealthCheck(cfg.HealthCheck.Interval, stopChan)
		slog.Info("Health checker started", "interval", cfg.HealthCheck.Interval)
	}

	// Initialize handler and register routes
	h := handler.New(s, p)
	mux := http.NewServeMux()
	h.RegisterRoutes(mux)

	// Start server
	addr := cfg.Server.Host + ":" + strconv.Itoa(cfg.Server.Port)
	slog.Info("Starting server", "addr", addr)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server error", "error", err)
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
}
