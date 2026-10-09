package httpapi

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"time"

	"github.com/itzikyis/docflow-ai/internal/logging"
)

const (
	requestIDHeader    = "X-Request-ID"
	maxRequestIDLength = 64
)

type requestIDKey struct{}

func requestIDFrom(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// withRequestID gives every request an ID, returned in the X-Request-ID
// response header and attached to every log line written with the request
// context. A caller-supplied ID is reused so a request can be followed across
// systems, but only if it is short and uses safe characters: the value is
// written into logs, and accepting arbitrary input would allow log injection.
func withRequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(requestIDHeader)
		if !validRequestID(id) {
			id = rand.Text()
		}
		w.Header().Set(requestIDHeader, id)

		ctx := context.WithValue(r.Context(), requestIDKey{}, id)
		ctx = logging.WithAttrs(ctx, slog.String("request_id", id))
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func validRequestID(id string) bool {
	if id == "" || len(id) > maxRequestIDLength {
		return false
	}
	for _, c := range []byte(id) {
		switch {
		case 'a' <= c && c <= 'z', 'A' <= c && c <= 'Z', '0' <= c && c <= '9':
		case c == '-' || c == '_' || c == '.':
		default:
			return false
		}
	}
	return true
}

// statusRecorder captures the status code and body size for the access log.
type statusRecorder struct {
	http.ResponseWriter
	status int
	bytes  int64
}

func (s *statusRecorder) WriteHeader(code int) {
	if s.status == 0 {
		s.status = code
	}
	s.ResponseWriter.WriteHeader(code)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	if s.status == 0 {
		s.status = http.StatusOK
	}
	n, err := s.ResponseWriter.Write(b)
	s.bytes += int64(n)
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (s *statusRecorder) Unwrap() http.ResponseWriter {
	return s.ResponseWriter
}

// probePaths are logged at debug level: orchestrators call them every few
// seconds per replica, which would otherwise drown out real traffic.
var probePaths = map[string]bool{"/health": true, "/ready": true}

// accessLog logs one line per request. route is the matched pattern (such as
// "GET /documents/{id}"), which unlike the path has low cardinality and can be
// used to group requests.
func accessLog(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := &statusRecorder{ResponseWriter: w}
			next.ServeHTTP(rec, r)

			status := rec.status
			if status == 0 {
				status = http.StatusOK
			}
			level := slog.LevelInfo
			if probePaths[r.URL.Path] {
				level = slog.LevelDebug
			}
			logger.LogAttrs(r.Context(), level, "http request",
				slog.String("method", r.Method),
				slog.String("path", r.URL.Path),
				slog.String("route", r.Pattern),
				slog.Int("status", status),
				slog.Int64("bytes", rec.bytes),
				slog.Float64("duration_ms", float64(time.Since(start).Microseconds())/1000),
			)
		})
	}
}

// recoverPanics turns a panic in a handler into a logged error and a 500
// response, so one bad request cannot crash the process. http.ErrAbortHandler
// is re-panicked: it is net/http's sanctioned way to abort a response.
func recoverPanics(logger *slog.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				v := recover()
				if v == nil {
					return
				}
				if err, ok := v.(error); ok && errors.Is(err, http.ErrAbortHandler) {
					panic(v)
				}
				logger.ErrorContext(r.Context(), "panic serving request",
					"panic", fmt.Sprint(v),
					"stack", string(debug.Stack()))
				writeProblem(w, r, http.StatusInternalServerError, "")
			}()
			next.ServeHTTP(w, r)
		})
	}
}
