package reload

import (
	"log/slog"

	"github.com/makashov73/xray-sub-rotation-service/internal/config"
	"github.com/makashov73/xray-sub-rotation-service/internal/sublist"
)

// ReloadConfig re-reads config.yaml and returns the new config.
func ReloadConfig(configPath string) (config.Config, error) {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return config.DefaultConfig(), err
	}
	return cfg, nil
}

// ReloadEndpoints re-reads sublist.md and returns updated entries.
func ReloadEndpoints(sublistPath string) ([]sublist.Entry, error) {
	entries, err := sublist.Parse(sublistPath)
	if err != nil {
		return nil, err
	}
	return entries, nil
}

// ReloadConfigAndLog is like ReloadConfig but logs a message on success.
func ReloadConfigAndLog(path string) (config.Config, error) {
	cfg, err := ReloadConfig(path)
	if err != nil {
		return cfg, err
	}
	slog.Info("Config reloaded successfully")
	return cfg, nil
}

// ReloadEndpointsAndLog is like ReloadEndpoints but logs a message on success.
func ReloadEndpointsAndLog(path string) ([]sublist.Entry, error) {
	entries, err := ReloadEndpoints(path)
	if err != nil {
		return nil, err
	}
	slog.Info("Sublist reloaded", "endpoints", len(entries))
	return entries, nil
}
