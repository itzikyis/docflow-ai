package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func serveProbe(t *testing.T, h *Health, path string) *httptest.ResponseRecorder {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, path, nil)
	NewHandler(Deps{Logger: discardLogger, Health: h}).ServeHTTP(rec, req)
	return rec
}

func TestLivenessIgnoresDependencies(t *testing.T) {
	h := NewHealth(discardLogger, map[string]ReadinessCheck{
		"db": func(context.Context) error { return errors.New("down") },
	})

	rec := serveProbe(t, h, "/health")
	if rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestReadiness(t *testing.T) {
	ok := func(context.Context) error { return nil }
	failing := func(context.Context) error { return errors.New("connection refused") }

	tests := []struct {
		name       string
		checks     map[string]ReadinessCheck
		draining   bool
		wantStatus int
		wantBody   string
	}{
		{"no checks", nil, false, http.StatusOK, `"status":"ok"`},
		{"all checks pass", map[string]ReadinessCheck{"db": ok, "queue": ok}, false, http.StatusOK, `"status":"ok"`},
		{"one check fails", map[string]ReadinessCheck{"db": failing, "queue": ok}, false, http.StatusServiceUnavailable, `"failedChecks":["db"]`},
		{"draining", map[string]ReadinessCheck{"db": ok}, true, http.StatusServiceUnavailable, `"status":"draining"`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			h := NewHealth(discardLogger, tt.checks)
			if tt.draining {
				h.StartDraining()
			}

			rec := serveProbe(t, h, "/ready")
			if rec.Code != tt.wantStatus {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantStatus)
			}
			if !strings.Contains(rec.Body.String(), tt.wantBody) {
				t.Errorf("body = %s, want it to contain %s", rec.Body, tt.wantBody)
			}
		})
	}
}

func TestReadinessDoesNotLeakErrorDetails(t *testing.T) {
	h := NewHealth(discardLogger, map[string]ReadinessCheck{
		"db": func(context.Context) error { return errors.New("dial tcp 10.0.0.5:5432: refused") },
	})

	rec := serveProbe(t, h, "/ready")
	if strings.Contains(rec.Body.String(), "10.0.0.5") {
		t.Errorf("readiness body exposes error details: %s", rec.Body)
	}
}

func TestReadinessCheckHasDeadline(t *testing.T) {
	h := NewHealth(discardLogger, map[string]ReadinessCheck{
		"db": func(ctx context.Context) error {
			if _, ok := ctx.Deadline(); !ok {
				return errors.New("no deadline")
			}
			return nil
		},
	})

	if rec := serveProbe(t, h, "/ready"); rec.Code != http.StatusOK {
		t.Errorf("status = %d, want %d: check context has no deadline", rec.Code, http.StatusOK)
	}
}
