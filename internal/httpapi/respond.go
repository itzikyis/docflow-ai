// Package httpapi is the HTTP adapter for the API: server lifecycle, routing,
// middleware, handlers and error responses.
package httpapi

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	// An encoding error here means the client went away; there is nobody left
	// to report it to.
	_ = json.NewEncoder(w).Encode(body)
}
