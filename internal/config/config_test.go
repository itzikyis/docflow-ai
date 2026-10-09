package config

import (
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/itzikyis/docflow-ai/internal/logging"
)

func env(vars map[string]string) LookupFunc {
	return func(key string) (string, bool) {
		v, ok := vars[key]
		return v, ok
	}
}

func TestLoadDefaults(t *testing.T) {
	cfg, err := Load(env(nil))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := Config{
		Port:               8080,
		LogLevel:           slog.LevelInfo,
		LogFormat:          logging.FormatJSON,
		MaxUploadBytes:     20 << 20,
		ShutdownDrainDelay: 5 * time.Second,
		ShutdownTimeout:    20 * time.Second,
	}
	if cfg != want {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoadOverrides(t *testing.T) {
	cfg, err := Load(env(map[string]string{
		"PORT":                 "9090",
		"LOG_LEVEL":            "debug",
		"LOG_FORMAT":           "text",
		"MAX_UPLOAD_BYTES":     "1024",
		"SHUTDOWN_DRAIN_DELAY": "0s",
		"SHUTDOWN_TIMEOUT":     "1m",
	}))
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	want := Config{
		Port:               9090,
		LogLevel:           slog.LevelDebug,
		LogFormat:          logging.FormatText,
		MaxUploadBytes:     1024,
		ShutdownDrainDelay: 0,
		ShutdownTimeout:    time.Minute,
	}
	if cfg != want {
		t.Errorf("Load() = %+v, want %+v", cfg, want)
	}
}

func TestLoadRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		vars    map[string]string
		wantErr string
	}{
		{"non-numeric port", map[string]string{"PORT": "http"}, "PORT: invalid integer"},
		{"port out of range", map[string]string{"PORT": "70000"}, "PORT: must be between"},
		{"unknown log level", map[string]string{"LOG_LEVEL": "verbose"}, "LOG_LEVEL: invalid level"},
		{"unknown log format", map[string]string{"LOG_FORMAT": "xml"}, "LOG_FORMAT: must be"},
		{"zero upload limit", map[string]string{"MAX_UPLOAD_BYTES": "0"}, "MAX_UPLOAD_BYTES: must be positive"},
		{"duration without unit", map[string]string{"SHUTDOWN_TIMEOUT": "20"}, "SHUTDOWN_TIMEOUT: invalid duration"},
		{"negative drain delay", map[string]string{"SHUTDOWN_DRAIN_DELAY": "-1s"}, "SHUTDOWN_DRAIN_DELAY: must not be negative"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := Load(env(tt.vars))
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("Load() error = %v, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestLoadReportsAllErrors(t *testing.T) {
	_, err := Load(env(map[string]string{"PORT": "x", "LOG_FORMAT": "xml"}))
	if err == nil {
		t.Fatal("Load() error = nil, want errors")
	}
	for _, key := range []string{"PORT", "LOG_FORMAT"} {
		if !strings.Contains(err.Error(), key) {
			t.Errorf("error %q does not mention %s", err, key)
		}
	}
}
