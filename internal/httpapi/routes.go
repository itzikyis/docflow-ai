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
