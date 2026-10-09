// Package httpapi is the HTTP adapter for the API: server lifecycle, routing,
// middleware, handlers and error responses.
package httpapi

import (
	"encoding/json"
	"net/http"
)

func writeJSON(w http.ResponseWriter, status int, body any) {
	encode(w, status, "application/json", body)
}

// problem is an RFC 9457 Problem Details response body.
type problem struct {
	Type      string `json:"type"`
	Title     string `json:"title"`
	Status    int    `json:"status"`
	Detail    string `json:"detail,omitempty"`
	Instance  string `json:"instance,omitempty"`
	RequestID string `json:"requestId,omitempty"`
}

// writeProblem writes an error response. The type is "about:blank", which
// RFC 9457 defines as "no extra semantics beyond the HTTP status code"; the
// title is therefore the standard status text. detail is shown to clients and
// must never contain internal error messages.
func writeProblem(w http.ResponseWriter, r *http.Request, status int, detail string) {
	encode(w, status, "application/problem+json", problem{
		Type:      "about:blank",
		Title:     http.StatusText(status),
		Status:    status,
		Detail:    detail,
		Instance:  r.URL.Path,
		RequestID: requestIDFrom(r.Context()),
	})
}

func encode(w http.ResponseWriter, status int, contentType string, body any) {
	w.Header().Set("Content-Type", contentType)
	w.WriteHeader(status)
	// An encoding error here means the client went away; there is nobody left
	// to report it to.
	_ = json.NewEncoder(w).Encode(body)
}
