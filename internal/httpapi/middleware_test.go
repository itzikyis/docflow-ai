package httpapi

import (
	"bufio"
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/itzikyis/docflow-ai/internal/logging"
)

// logLines parses JSON log output into one map per line.
func logLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	sc := bufio.NewScanner(buf)
	sc.Buffer(nil, 1<<20)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("log line is not JSON: %v\n%s", err, sc.Text())
		}
		lines = append(lines, m)
	}
	return lines
}

func findLog(lines []map[string]any, message string) map[string]any {
	for _, l := range lines {
		if l["message"] == message {
			return l
		}
	}
	return nil
}

// newTestHandler returns the full middleware chain around extra routes, and a
// buffer receiving debug-level JSON logs.
func newTestHandler(t *testing.T, routes map[string]http.HandlerFunc) (http.Handler, *bytes.Buffer) {
	t.Helper()
	var buf bytes.Buffer
	logger := logging.New(&buf, slog.LevelDebug, logging.FormatJSON)

	mux := http.NewServeMux()
	for pattern, h := range routes {
		mux.HandleFunc(pattern, h)
	}
	return chain(mux, withRequestID, accessLog(logger), recoverPanics(logger)), &buf
}

func TestRequestID(t *testing.T) {
	tests := []struct {
		name     string
		incoming string
		wantSame bool
	}{
		{"generated when absent", "", false},
		{"reused when valid", "abc-123_X.y", true},
		{"replaced when too long", strings.Repeat("a", maxRequestIDLength+1), false},
		{"replaced when it could inject log lines", "abc\nlevel=ERROR", false},
		{"replaced when it contains spaces", "abc def", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var seen string
			h, logs := newTestHandler(t, map[string]http.HandlerFunc{
				"GET /x": func(_ http.ResponseWriter, r *http.Request) { seen = requestIDFrom(r.Context()) },
			})

			req := httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/x", nil)
			if tt.incoming != "" {
				req.Header.Set(requestIDHeader, tt.incoming)
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)

			got := rec.Header().Get(requestIDHeader)
			if got == "" || got != seen {
				t.Fatalf("response header %q, handler saw %q; want equal and non-empty", got, seen)
			}
			if (got == tt.incoming) != tt.wantSame {
				t.Errorf("request ID = %q, incoming %q, wantSame %v", got, tt.incoming, tt.wantSame)
			}
			if !validRequestID(got) {
				t.Errorf("request ID %q is not itself valid", got)
			}

			access := findLog(logLines(t, logs), "http request")
			if access == nil || access["request_id"] != got {
				t.Errorf("access log request_id = %v, want %q", access["request_id"], got)
			}
		})
	}
}

func TestAccessLog(t *testing.T) {
	h, logs := newTestHandler(t, map[string]http.HandlerFunc{
		"GET /items/{id}": func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusTeapot)
			_, _ = w.Write([]byte("hello"))
		},
	})

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/items/42", nil))

	l := findLog(logLines(t, logs), "http request")
	if l == nil {
		t.Fatal("no access log line")
	}
	want := map[string]any{
		"severity": "INFO",
		"method":   "GET",
		"path":     "/items/42",
		"route":    "GET /items/{id}",
		"status":   float64(http.StatusTeapot),
		"bytes":    float64(5),
	}
	for k, v := range want {
		if l[k] != v {
			t.Errorf("%s = %v, want %v", k, l[k], v)
		}
	}
	if _, ok := l["duration_ms"]; !ok {
		t.Error("duration_ms missing")
	}
}

func TestAccessLogProbesAtDebug(t *testing.T) {
	h, logs := newTestHandler(t, map[string]http.HandlerFunc{
		"GET /health": func(http.ResponseWriter, *http.Request) {},
	})

	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/health", nil))

	if l := findLog(logLines(t, logs), "http request"); l == nil || l["severity"] != "DEBUG" {
		t.Errorf("probe access log = %v, want severity DEBUG", l)
	}
}

func TestRecoverPanics(t *testing.T) {
	h, logs := newTestHandler(t, map[string]http.HandlerFunc{
		"GET /boom": func(http.ResponseWriter, *http.Request) { panic("nil map write") },
	})

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("Content-Type = %q, want application/problem+json", ct)
	}
	var p problem
	if err := json.Unmarshal(rec.Body.Bytes(), &p); err != nil {
		t.Fatalf("body is not a problem document: %v", err)
	}
	if p.Status != http.StatusInternalServerError || p.RequestID != rec.Header().Get(requestIDHeader) {
		t.Errorf("problem = %+v", p)
	}
	if strings.Contains(rec.Body.String(), "nil map") {
		t.Error("response leaks the panic value")
	}

	lines := logLines(t, logs)
	if l := findLog(lines, "panic serving request"); l == nil || l["severity"] != "ERROR" || l["stack"] == "" {
		t.Errorf("panic log = %v, want an ERROR line with a stack", l)
	}
	if l := findLog(lines, "http request"); l == nil || l["status"] != float64(http.StatusInternalServerError) {
		t.Errorf("access log = %v, want status 500", l)
	}
}

func TestRecoverPanicsRethrowsAbortHandler(t *testing.T) {
	h, _ := newTestHandler(t, map[string]http.HandlerFunc{
		"GET /abort": func(http.ResponseWriter, *http.Request) { panic(http.ErrAbortHandler) },
	})

	defer func() {
		if v := recover(); v != http.ErrAbortHandler { //nolint:errorlint // identity check on the panic value
			t.Errorf("recovered %v, want http.ErrAbortHandler", v)
		}
	}()
	h.ServeHTTP(httptest.NewRecorder(), httptest.NewRequestWithContext(t.Context(), http.MethodGet, "/abort", nil))
}
