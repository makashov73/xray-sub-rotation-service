package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server      ServerConfig      `yaml:"server"`
	HealthCheck HealthCheckConfig `yaml:"health_check"`
	Strategy    string            `yaml:"strategy"`
	SublistFile string            `yaml:"sublist_file"`
	TLS         TLSConfig         `yaml:"tls"`
	RateLimit   RateLimitConfig   `yaml:"rate_limit"`
}

type ServerConfig struct {
	Host   string `yaml:"host"`
	Port   int    `yaml:"port"`
	Domain string `yaml:"domain"`
}

func (s ServerConfig) BuildSubscriptionURL(domain, scheme, subID string) string {
	host := s.Host
	if domain != "" {
		host = domain
	}
	port := s.Port
	if domain != "" {
		if (scheme == "http" && port == 80) || (scheme == "https" && port == 443) {
			return scheme + "://" + host + "/subrouter/" + subID
		}
	}
	return scheme + "://" + host + ":" + strconv.Itoa(port) + "/subrouter/" + subID
}

type HealthCheckConfig struct {
	Enabled      bool          `yaml:"enabled"`
	Interval     time.Duration `yaml:"interval"`
	Timeout      time.Duration `yaml:"timeout"`
	PersistPath  string `yaml:"persist_path"`
}

type TLSConfig struct {
	CertFile string `yaml:"cert_file"`
	KeyFile  string `yaml:"key_file"`
}

type RateLimitConfig struct {
	Enabled  bool          `yaml:"enabled"`
	MaxReqs  int           `yaml:"max_reqs"`
	Window   time.Duration `yaml:"window"`
}

func DefaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Host: "0.0.0.0",
			Port: 8080,
		},
		HealthCheck: HealthCheckConfig{
			Enabled:  true,
			Interval: 30 * time.Second,
			Timeout:  5 * time.Second,
		},
		Strategy: "fastest",
		RateLimit: RateLimitConfig{
			Enabled: false,
			MaxReqs: 100,
			Window:  time.Minute,
		},
	}
}

func LoadConfig(path string) (Config, error) {
	cfg := DefaultConfig()

	if path == "" {
		return cfg, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return cfg, err
	}

	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}

	return cfg, nil
}

var validStrategies = map[string]bool{"fastest": true, "random": true, "first": true}

func (c *Config) Validate() error {
	if !validStrategies[c.Strategy] {
		return fmt.Errorf("invalid strategy %q: must be one of fastest, random, first", c.Strategy)
	}
	return nil
}
