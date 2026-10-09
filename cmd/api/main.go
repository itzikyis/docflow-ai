// Command api is the DocFlow AI HTTP API.
package main

import (
	"fmt"
	"os"

	"github.com/itzikyis/docflow-ai/internal/config"
	"github.com/itzikyis/docflow-ai/internal/logging"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "api: %v\n", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load(os.LookupEnv)
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}

	logger := logging.New(os.Stdout, cfg.LogLevel, cfg.LogFormat).
		With("service", "api", "version", version)

	logger.Info("api starting", "port", cfg.Port, "max_upload_bytes", cfg.MaxUploadBytes)
	return nil
}
