// Command api is the DocFlow AI HTTP API.
package main

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/itzikyis/docflow-ai/internal/config"
	"github.com/itzikyis/docflow-ai/internal/document"
	"github.com/itzikyis/docflow-ai/internal/httpapi"
	"github.com/itzikyis/docflow-ai/internal/logging"
)

// version is set at build time via -ldflags "-X main.version=...".
var version = "dev"

// simulatedStepDelay is how long the processing simulator spends in each
// status, slow enough to watch a document progress by polling its status.
const simulatedStepDelay = 3 * time.Second

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

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repo := document.NewMemoryRepository()
	simulator := document.NewSimulator(repo, logger, simulatedStepDelay)
	defer simulator.Close()

	health := httpapi.NewHealth(logger, nil)
	handler := httpapi.NewHandler(httpapi.Deps{
		Logger:         logger,
		Health:         health,
		Documents:      document.NewService(repo, simulator),
		MaxUploadBytes: cfg.MaxUploadBytes,
	})
	server := httpapi.NewServer(handler, health, logger, httpapi.ShutdownConfig{
		DrainDelay: cfg.ShutdownDrainDelay,
		Timeout:    cfg.ShutdownTimeout,
	})

	var lc net.ListenConfig
	ln, err := lc.Listen(ctx, "tcp", net.JoinHostPort("", strconv.Itoa(cfg.Port)))
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	logger.Info("api listening", "addr", ln.Addr().String())

	return server.Serve(ctx, ln)
}
