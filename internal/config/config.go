// Package config loads service configuration from environment variables.
package config

import (
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/itzikyis/docflow-ai/internal/logging"
)

// Config is the API service configuration. Every field has a production-safe
// default, so an empty environment yields a valid configuration.
type Config struct {
	Port      int
	LogLevel  slog.Level
	LogFormat string

	// MaxUploadBytes is the largest document accepted by POST /documents.
	MaxUploadBytes int64

	// ShutdownDrainDelay is how long the server keeps serving after it starts
	// failing readiness, giving load balancers time to stop routing to it.
	ShutdownDrainDelay time.Duration

	// ShutdownTimeout bounds how long in-flight requests may take to finish.
	// DrainDelay + ShutdownTimeout must stay below the orchestrator's grace
	// period (Kubernetes terminationGracePeriodSeconds, 30s by default).
	ShutdownTimeout time.Duration
}

// LookupFunc has the signature of os.LookupEnv.
type LookupFunc func(key string) (string, bool)

// Load reads the configuration using lookup and validates it. All problems are
// reported together so a misconfigured deployment can be fixed in one pass.
func Load(lookup LookupFunc) (Config, error) {
	l := loader{lookup: lookup}

	cfg := Config{
		Port:               l.int("PORT", 8080),
		LogLevel:           l.logLevel("LOG_LEVEL", slog.LevelInfo),
		LogFormat:          l.string("LOG_FORMAT", logging.FormatJSON),
		MaxUploadBytes:     l.int64("MAX_UPLOAD_BYTES", 20<<20),
		ShutdownDrainDelay: l.duration("SHUTDOWN_DRAIN_DELAY", 5*time.Second),
		ShutdownTimeout:    l.duration("SHUTDOWN_TIMEOUT", 20*time.Second),
	}

	if cfg.Port < 1 || cfg.Port > 65535 {
		l.fail("PORT", "must be between 1 and 65535")
	}
	if cfg.LogFormat != logging.FormatJSON && cfg.LogFormat != logging.FormatText {
		l.fail("LOG_FORMAT", fmt.Sprintf("must be %q or %q", logging.FormatJSON, logging.FormatText))
	}
	if cfg.MaxUploadBytes <= 0 {
		l.fail("MAX_UPLOAD_BYTES", "must be positive")
	}
	if cfg.ShutdownDrainDelay < 0 {
		l.fail("SHUTDOWN_DRAIN_DELAY", "must not be negative")
	}
	if cfg.ShutdownTimeout <= 0 {
		l.fail("SHUTDOWN_TIMEOUT", "must be positive")
	}

	if err := errors.Join(l.errs...); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

type loader struct {
	lookup LookupFunc
	errs   []error
}

func (l *loader) fail(key, reason string) {
	l.errs = append(l.errs, fmt.Errorf("%s: %s", key, reason))
}

func (l *loader) string(key, def string) string {
	if v, ok := l.lookup(key); ok && v != "" {
		return v
	}
	return def
}

func (l *loader) int(key string, def int) int {
	v, ok := l.lookup(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		l.fail(key, fmt.Sprintf("invalid integer %q", v))
		return def
	}
	return n
}

func (l *loader) int64(key string, def int64) int64 {
	v, ok := l.lookup(key)
	if !ok || v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		l.fail(key, fmt.Sprintf("invalid integer %q", v))
		return def
	}
	return n
}

func (l *loader) duration(key string, def time.Duration) time.Duration {
	v, ok := l.lookup(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		l.fail(key, fmt.Sprintf("invalid duration %q (use Go syntax such as 5s or 1m30s)", v))
		return def
	}
	return d
}

func (l *loader) logLevel(key string, def slog.Level) slog.Level {
	v, ok := l.lookup(key)
	if !ok || v == "" {
		return def
	}
	var level slog.Level
	if err := level.UnmarshalText([]byte(v)); err != nil {
		l.fail(key, fmt.Sprintf("invalid level %q (use debug, info, warn or error)", v))
		return def
	}
	return level
}
