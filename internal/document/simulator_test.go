package document

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"github.com/google/uuid"
)

var discardLogger = slog.New(slog.DiscardHandler)

func registered(t *testing.T, repo *MemoryRepository) Document {
	t.Helper()
	svc := NewService(repo, &recordingDispatcher{})
	doc, err := svc.Register(t.Context(), testUpload)
	if err != nil {
		t.Fatal(err)
	}
	return doc
}

func waitForStatus(t *testing.T, repo *MemoryRepository, id uuid.UUID, want Status) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for {
		got, err := repo.Get(t.Context(), id)
		if err != nil {
			t.Fatal(err)
		}
		if got.Status == want {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("status = %s after 2s, want %s", got.Status, want)
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestSimulatorCompletesDocument(t *testing.T) {
	repo := NewMemoryRepository()
	sim := NewSimulator(repo, discardLogger, time.Millisecond)
	defer sim.Close()
	doc := registered(t, repo)

	if err := sim.Dispatch(t.Context(), doc); err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}
	waitForStatus(t, repo, doc.ID, StatusCompleted)
}

func TestSimulatorOutlivesRequestContext(t *testing.T) {
	repo := NewMemoryRepository()
	sim := NewSimulator(repo, discardLogger, time.Millisecond)
	defer sim.Close()
	doc := registered(t, repo)

	ctx, cancel := context.WithCancel(t.Context())
	if err := sim.Dispatch(ctx, doc); err != nil {
		t.Fatal(err)
	}
	cancel()

	waitForStatus(t, repo, doc.ID, StatusCompleted)
}

func TestSimulatorCloseStopsProcessing(t *testing.T) {
	repo := NewMemoryRepository()
	sim := NewSimulator(repo, discardLogger, time.Hour)
	doc := registered(t, repo)

	if err := sim.Dispatch(t.Context(), doc); err != nil {
		t.Fatalf("Dispatch() error = %v", err)
	}

	closed := make(chan struct{})
	go func() {
		sim.Close()
		close(closed)
	}()
	select {
	case <-closed:
	case <-time.After(2 * time.Second):
		t.Fatal("Close() did not return; background processing ignored shutdown")
	}

	if got, _ := repo.Get(t.Context(), doc.ID); got.Status != StatusUploaded {
		t.Errorf("status = %s after Close, want %s", got.Status, StatusUploaded)
	}
	if err := sim.Dispatch(t.Context(), doc); !errors.Is(err, ErrDispatcherClosed) {
		t.Errorf("Dispatch() after Close error = %v, want ErrDispatcherClosed", err)
	}
}
