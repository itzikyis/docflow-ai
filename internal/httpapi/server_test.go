package httpapi

import (
	"context"
	"errors"
	"net"
	"net/http"
	"testing"
	"time"
)

type testServer struct {
	base   string
	cancel context.CancelFunc
	done   chan error
}

// startServer serves mux on a random local port with the given shutdown
// settings. Cancelling the returned server's context starts shutdown.
func startServer(t *testing.T, mux *http.ServeMux, health *Health, cfg ShutdownConfig) *testServer {
	t.Helper()
	var lc net.ListenConfig
	ln, err := lc.Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	ts := &testServer{base: "http://" + ln.Addr().String(), cancel: cancel, done: make(chan error, 1)}
	srv := NewServer(mux, health, discardLogger, cfg)
	go func() { ts.done <- srv.Serve(ctx, ln) }()
	return ts
}

func (ts *testServer) wait(t *testing.T) error {
	t.Helper()
	select {
	case err := <-ts.done:
		return err
	case <-time.After(5 * time.Second):
		t.Fatal("server did not stop within 5s")
		return nil
	}
}

func TestServeDrainsThenFinishesInFlightRequests(t *testing.T) {
	health := NewHealth(discardLogger, nil)
	started, release := make(chan struct{}), make(chan struct{})

	mux := http.NewServeMux()
	mux.HandleFunc("GET /ready", health.ready)
	mux.HandleFunc("GET /slow", func(w http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
		w.WriteHeader(http.StatusOK)
	})

	ts := startServer(t, mux, health, ShutdownConfig{DrainDelay: 500 * time.Millisecond, Timeout: 5 * time.Second})

	slowStatus := make(chan int, 1)
	go func() {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.base+"/slow", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			slowStatus <- 0
			return
		}
		_ = resp.Body.Close()
		slowStatus <- resp.StatusCode
	}()
	<-started
	ts.cancel()

	// During the drain delay the server still answers, but reports not ready.
	deadline := time.Now().Add(400 * time.Millisecond)
	for {
		code, _ := get(t, ts.base+"/ready")
		if code == http.StatusServiceUnavailable {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("/ready = %d during drain, want %d", code, http.StatusServiceUnavailable)
		}
		time.Sleep(10 * time.Millisecond)
	}

	close(release)
	if code := <-slowStatus; code != http.StatusOK {
		t.Errorf("in-flight request status = %d, want %d", code, http.StatusOK)
	}
	if err := ts.wait(t); err != nil {
		t.Errorf("Serve() = %v, want nil after clean shutdown", err)
	}
}

func TestServeShutdownTimesOut(t *testing.T) {
	health := NewHealth(discardLogger, nil)
	started, release := make(chan struct{}), make(chan struct{})
	t.Cleanup(func() { close(release) })

	mux := http.NewServeMux()
	mux.HandleFunc("GET /stuck", func(_ http.ResponseWriter, _ *http.Request) {
		close(started)
		<-release
	})

	ts := startServer(t, mux, health, ShutdownConfig{DrainDelay: 0, Timeout: 100 * time.Millisecond})

	go func() {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodGet, ts.base+"/stuck", nil)
		if resp, err := http.DefaultClient.Do(req); err == nil {
			_ = resp.Body.Close()
		}
	}()
	<-started
	ts.cancel()

	if err := ts.wait(t); !errors.Is(err, context.DeadlineExceeded) {
		t.Errorf("Serve() = %v, want context.DeadlineExceeded", err)
	}
}
