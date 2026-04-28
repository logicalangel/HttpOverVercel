package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Mode        string            `json:"mode"`
	FrontDomain string            `json:"front_domain"`
	FrontIP     string            `json:"front_ip"`
	WorkerHost  string            `json:"worker_host"`
	RelayPath   string            `json:"relay_path"`
	RelayPaths  []string          `json:"relay_paths"`
	AuthKey     string            `json:"auth_key"`
	EnableBatch bool              `json:"enable_batch"`
	EnableH2    bool              `json:"enable_h2"`
	ListenHost  string            `json:"listen_host"`
	ListenPort  int               `json:"listen_port"`
	LogLevel    string            `json:"log_level"`
	VerifySSL   bool              `json:"verify_ssl"`
	Hosts       map[string]string `json:"hosts"`
}

func applyDefaults(c *Config) {
	if c.Mode == "" {
		c.Mode = "vercel_edge"
	}
	if c.ListenHost == "" {
		c.ListenHost = "127.0.0.1"
	}
	if c.ListenPort == 0 {
		c.ListenPort = 8085
	}
	if c.RelayPath == "" {
		c.RelayPath = "/api/api"
	}
	if c.LogLevel == "" {
		c.LogLevel = "INFO"
	}
	if c.Hosts == nil {
		c.Hosts = map[string]string{}
	}
}

func normalizeModes(c *Config) {
	switch c.Mode {
	case "google_fronting":
		c.Mode = "domain_fronting"
	case "apps_script":
		c.Mode = "vercel_edge"
	}
}

func applyEnvOverrides(c *Config) {
	if v := os.Getenv("DFT_AUTH_KEY"); v != "" {
		c.AuthKey = v
	}
	if v := os.Getenv("DFT_RELAY_PATH"); v != "" {
		c.RelayPath = v
	}
	if v := os.Getenv("DFT_HOST"); v != "" {
		c.ListenHost = v
	}
	if v := os.Getenv("DFT_PORT"); v != "" {
		if p, err := strconv.Atoi(v); err == nil {
			c.ListenPort = p
		}
	}
	if v := os.Getenv("DFT_LOG_LEVEL"); v != "" {
		c.LogLevel = v
	}
}

// Load reads a JSON config file, applies defaults, env overrides, and normalizes legacy modes.
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config %q: %w", path, err)
	}

	var cfg Config
	// Set VerifySSL default to true before unmarshalling
	cfg.VerifySSL = true
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config %q: %w", path, err)
	}

	applyDefaults(&cfg)
	applyEnvOverrides(&cfg)
	normalizeModes(&cfg)

	return &cfg, nil
}

// Validate checks required fields.
func (c *Config) Validate() error {
	if strings.TrimSpace(c.AuthKey) == "" {
		return errors.New("auth_key is required")
	}
	if c.Mode == "vercel_edge" {
		if strings.TrimSpace(c.WorkerHost) == "" {
			return errors.New("worker_host is required for vercel_edge mode")
		}
		if len(c.AllRelayPaths()) == 0 {
			return errors.New("relay_path or relay_paths required for vercel_edge mode")
		}
	}
	return nil
}

// AllRelayPaths returns relay_paths if set, else [relay_path], else ["/api/api"].
func (c *Config) AllRelayPaths() []string {
	if len(c.RelayPaths) > 0 {
		return c.RelayPaths
	}
	if c.RelayPath != "" {
		return []string{c.RelayPath}
	}
	return []string{"/api/api"}
}

// ConnectHost returns front_ip if set, else front_domain, else worker_host.
func (c *Config) ConnectHost() string {
	if c.FrontIP != "" {
		return c.FrontIP
	}
	if c.FrontDomain != "" {
		return c.FrontDomain
	}
	return c.WorkerHost
}
