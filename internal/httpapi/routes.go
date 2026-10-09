package httpapi

import (
	"log/slog"
	"net/http"

	"github.com/itzikyis/docflow-ai/internal/document"
)

// Deps are the dependencies of the HTTP handlers.
type Deps struct {
	Logger         *slog.Logger
	Health         *Health
	Documents      *document.Service
	MaxUploadBytes int64
}

// NewHandler returns the API's root handler.
func NewHandler(d Deps) http.Handler {
	docs := &documentHandlers{svc: d.Documents, logger: d.Logger, maxUploadBytes: d.MaxUploadBytes}

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", d.Health.live)
	mux.HandleFunc("GET /ready", d.Health.ready)
	mux.HandleFunc("POST /documents", docs.upload)
	mux.HandleFunc("GET /documents/{id}", docs.get)
	mux.HandleFunc("GET /documents/{id}/status", docs.status)

	return chain(mux,
		withRequestID,
		accessLog(d.Logger),
		recoverPanics(d.Logger),
	)
}

// chain wraps h so that the first middleware is the outermost: a request
// passes through them in the order given.
func chain(h http.Handler, middleware ...func(http.Handler) http.Handler) http.Handler {
	for i := len(middleware) - 1; i >= 0; i-- {
		h = middleware[i](h)
	}
	return h
}
