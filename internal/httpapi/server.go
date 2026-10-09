package httpapi

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"
)

// Server timeouts. Go's defaults are "no timeout", which lets slow or
// malicious clients hold connections open indefinitely. Read and write
// timeouts cover a whole request, including a document upload over a slow
// connection.
const (
	readHeaderTimeout = 5 * time.Second
	readTimeout       = 2 * time.Minute
	writeTimeout      = 2 * time.Minute
	idleTimeout       = 2 * time.Minute
)

// ShutdownConfig controls graceful shutdown.
type ShutdownConfig struct {
	// DrainDelay is how long to keep serving after readiness starts failing.
	DrainDelay time.Duration
	// Timeout bounds how long in-flight requests may take to finish.
	Timeout time.Duration
}

// Server is an HTTP server with graceful shutdown.
type Server struct {
	http     *http.Server
	health   *Health
	logger   *slog.Logger
	shutdown ShutdownConfig
}

// NewServer returns a server for handler. health is switched to draining when
// shutdown begins.
func NewServer(handler http.Handler, health *Health, logger *slog.Logger, shutdown ShutdownConfig) *Server {
	return &Server{
		http: &http.Server{
			Handler:           handler,
			ReadHeaderTimeout: readHeaderTimeout,
			ReadTimeout:       readTimeout,
			WriteTimeout:      writeTimeout,
			IdleTimeout:       idleTimeout,
			ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
		},
		health:   health,
		logger:   logger,
		shutdown: shutdown,
	}
}

// Serve accepts connections on ln until ctx is cancelled, then shuts down:
//
//  1. readiness starts failing;
//  2. the server keeps serving for DrainDelay, because Kubernetes removes the
//     pod from load-balancer endpoints asynchronously and traffic keeps
//     arriving for a few seconds after SIGTERM;
//  3. the listener closes and in-flight requests get up to Timeout to finish.
//
// Serve returns nil after a clean shutdown.
func (s *Server) Serve(ctx context.Context, ln net.Listener) error {
	serveErr := make(chan error, 1)
	go func() { serveErr <- s.http.Serve(ln) }()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	case <-ctx.Done():
	}

	s.logger.Info("shutdown started",
		"drain_delay", s.shutdown.DrainDelay.String(),
		"timeout", s.shutdown.Timeout.String())
	s.health.StartDraining()

	drain := time.NewTimer(s.shutdown.DrainDelay)
	defer drain.Stop()
	select {
	case <-drain.C:
	case err := <-serveErr:
		return fmt.Errorf("serve: %w", err)
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), s.shutdown.Timeout)
	defer cancel()
	if err := s.http.Shutdown(shutdownCtx); err != nil {
		_ = s.http.Close()
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	if err := <-serveErr; !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}

	s.logger.Info("shutdown complete")
	return nil
}
