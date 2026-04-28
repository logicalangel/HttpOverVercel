package config

import (
	"os"
	"path/filepath"
	"testing"
)

func writeConfigFile(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatalf("writeConfigFile: %v", err)
	}
	return path
}

func TestLoadDefaults(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, dir, `{"auth_key":"secret","worker_host":"example.vercel.app"}`)

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Mode != "vercel_edge" {
		t.Errorf("Mode: got %q want %q", cfg.Mode, "vercel_edge")
	}
	if cfg.ListenHost != "127.0.0.1" {
		t.Errorf("ListenHost: got %q want %q", cfg.ListenHost, "127.0.0.1")
	}
	if cfg.ListenPort != 8085 {
		t.Errorf("ListenPort: got %d want %d", cfg.ListenPort, 8085)
	}
	if cfg.RelayPath != "/api/api" {
		t.Errorf("RelayPath: got %q want %q", cfg.RelayPath, "/api/api")
	}
	if cfg.LogLevel != "INFO" {
		t.Errorf("LogLevel: got %q want %q", cfg.LogLevel, "INFO")
	}
}

func TestLoadEnvOverride(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, dir, `{"auth_key":"original","worker_host":"example.vercel.app"}`)

	t.Setenv("DFT_AUTH_KEY", "overridden")
	defer os.Unsetenv("DFT_AUTH_KEY")

	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.AuthKey != "overridden" {
		t.Errorf("AuthKey: got %q want %q", cfg.AuthKey, "overridden")
	}
}

func TestValidateMissingAuthKey(t *testing.T) {
	cfg := &Config{
		Mode:       "vercel_edge",
		WorkerHost: "example.vercel.app",
		RelayPath:  "/api/api",
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing auth_key")
	}
}

func TestValidateMissingWorkerHost(t *testing.T) {
	cfg := &Config{
		Mode:      "vercel_edge",
		AuthKey:   "secret",
		RelayPath: "/api/api",
	}
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for missing worker_host in vercel_edge mode")
	}
}

func TestValidateOK(t *testing.T) {
	cfg := &Config{
		Mode:       "vercel_edge",
		AuthKey:    "secret",
		WorkerHost: "example.vercel.app",
		RelayPath:  "/api/api",
	}
	if err := cfg.Validate(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAllRelayPaths_single(t *testing.T) {
	cfg := &Config{RelayPath: "/api/api"}
	paths := cfg.AllRelayPaths()
	if len(paths) != 1 || paths[0] != "/api/api" {
		t.Errorf("AllRelayPaths: got %v", paths)
	}
}

func TestAllRelayPaths_multi(t *testing.T) {
	cfg := &Config{RelayPaths: []string{"/api/api", "/api/api2"}}
	paths := cfg.AllRelayPaths()
	if len(paths) != 2 {
		t.Errorf("AllRelayPaths: got %v", paths)
	}
}

func TestAllRelayPaths_default(t *testing.T) {
	cfg := &Config{}
	paths := cfg.AllRelayPaths()
	if len(paths) != 1 || paths[0] != "/api/api" {
		t.Errorf("AllRelayPaths default: got %v", paths)
	}
}

func TestLoadInvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := writeConfigFile(t, dir, `{not valid json}`)
	if _, err := Load(path); err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadNotFound(t *testing.T) {
	if _, err := Load("/nonexistent/path/config.json"); err == nil {
		t.Error("expected error for nonexistent file")
	}
}
