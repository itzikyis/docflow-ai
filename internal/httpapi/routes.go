package httpapi

import (
	"log/slog"
	"net/http"
)

// Deps are the dependencies of the HTTP handlers.
type Deps struct {
	Logger *slog.Logger
	Health *Health
}

// NewHandler returns the API's root handler.
func NewHandler(d Deps) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", d.Health.live)
	mux.HandleFunc("GET /ready", d.Health.ready)
	return mux
}
