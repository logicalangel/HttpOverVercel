package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/logicalangel/HttpOverVercel/internal/config"
	"github.com/logicalangel/HttpOverVercel/internal/mitm"
	"github.com/logicalangel/HttpOverVercel/internal/proxy"
	"github.com/logicalangel/HttpOverVercel/internal/relay"
)

const version = "1.0.0"

func main() {
	configPath := flag.String("c", os.Getenv("DFT_CONFIG"), "config file path")
	flag.StringVar(configPath, "config", os.Getenv("DFT_CONFIG"), "config file path (long form)")
	port := flag.Int("p", 0, "override listen port")
	host := flag.String("host", "", "override listen host")
	logLevel := flag.String("log-level", "", "log level: DEBUG|INFO|WARNING|ERROR")
	installCert := flag.Bool("install-cert", false, "print CA cert installation instructions and exit")
	showVersion := flag.Bool("version", false, "print version and exit")
	flag.Parse()

	if *showVersion {
		fmt.Printf("HttpOverVercel %s\n", version)
		os.Exit(0)
	}

	if *configPath == "" {
		*configPath = "config.json"
	}

	cfg, err := config.Load(*configPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error loading config: %v\n", err)
		os.Exit(1)
	}

	if *port != 0 {
		cfg.ListenPort = *port
	}
	if *host != "" {
		cfg.ListenHost = *host
	}
	if *logLevel != "" {
		cfg.LogLevel = *logLevel
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(os.Stderr, "Config error: %v\n", err)
		os.Exit(1)
	}

	var level slog.Level
	switch strings.ToUpper(cfg.LogLevel) {
	case "DEBUG":
		level = slog.LevelDebug
	case "WARNING":
		level = slog.LevelWarn
	case "ERROR":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
	slog.SetDefault(logger)

	ca, err := mitm.New()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error creating CA: %v\n", err)
		os.Exit(1)
	}

	if *installCert {
		fmt.Printf("Install the MITM CA certificate as a trusted root CA:\n\n")
		fmt.Printf("  Certificate file: %s\n\n", ca.CACertFile())
		fmt.Printf("macOS:\n")
		fmt.Printf("  sudo security add-trusted-cert -d -r trustRoot -k /Library/Keychains/System.keychain %s\n\n", ca.CACertFile())
		fmt.Printf("Linux (Debian/Ubuntu):\n")
		fmt.Printf("  sudo cp %s /usr/local/share/ca-certificates/HttpOverVercel.crt && sudo update-ca-certificates\n\n", ca.CACertFile())
		fmt.Printf("Windows (PowerShell as Administrator):\n")
		fmt.Printf("  Import-Certificate -FilePath \"%s\" -CertStoreLocation Cert:\\LocalMachine\\Root\n", ca.CACertFile())
		os.Exit(0)
	}

	rc := relay.NewClient(cfg.WorkerHost, cfg.AuthKey, cfg.AllRelayPaths(), cfg.VerifySSL)

	srv := proxy.New(cfg.ListenHost, cfg.ListenPort, ca, rc)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	logger.Info("Starting proxy", "addr", fmt.Sprintf("%s:%d", cfg.ListenHost, cfg.ListenPort))
	if err := srv.ListenAndServe(ctx); err != nil {
		logger.Error("Server error", "err", err)
		os.Exit(1)
	}
}
