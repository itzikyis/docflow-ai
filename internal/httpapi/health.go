package httpapi

import (
	"context"
	"log/slog"
	"maps"
	"net/http"
	"slices"
	"sync/atomic"
	"time"
)

// ReadinessCheck reports whether a dependency the service needs to handle
// traffic is usable.
type ReadinessCheck func(ctx context.Context) error

const readinessCheckTimeout = 2 * time.Second

// Health serves the liveness and readiness probes.
//
// Liveness only says the process is serving. It deliberately ignores
// dependencies: if a database outage failed liveness, the orchestrator would
// restart every replica at once without fixing anything.
//
// Readiness says whether this replica should receive traffic. It fails while
// the server is draining for shutdown and when any registered check fails.
type Health struct {
	logger   *slog.Logger
	checks   map[string]ReadinessCheck
	names    []string
	draining atomic.Bool
}

// NewHealth returns probes that run checks, keyed by dependency name, on every
// readiness request.
func NewHealth(logger *slog.Logger, checks map[string]ReadinessCheck) *Health {
	return &Health{
		logger: logger,
		checks: checks,
		names:  slices.Sorted(maps.Keys(checks)),
	}
}

// StartDraining makes readiness fail from now on.
func (h *Health) StartDraining() {
	h.draining.Store(true)
}

type healthResponse struct {
	Status       string   `json:"status"`
	FailedChecks []string `json:"failedChecks,omitempty"`
}

func (h *Health) live(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}

func (h *Health) ready(w http.ResponseWriter, r *http.Request) {
	if h.draining.Load() {
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "draining"})
		return
	}

	var failed []string
	for _, name := range h.names {
		ctx, cancel := context.WithTimeout(r.Context(), readinessCheckTimeout)
		err := h.checks[name](ctx)
		cancel()
		if err != nil {
			// Details go to the log only: the probe endpoint is reachable from
			// outside and must not leak infrastructure errors.
			h.logger.WarnContext(r.Context(), "readiness check failed", "check", name, "error", err)
			failed = append(failed, name)
		}
	}

	if len(failed) > 0 {
		writeJSON(w, http.StatusServiceUnavailable, healthResponse{Status: "unavailable", FailedChecks: failed})
		return
	}
	writeJSON(w, http.StatusOK, healthResponse{Status: "ok"})
}
