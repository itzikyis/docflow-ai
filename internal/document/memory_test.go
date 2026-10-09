package document

import (
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestMemoryRepositoryCreateRejectsDuplicates(t *testing.T) {
	repo := NewMemoryRepository()
	doc := Document{ID: uuid.New(), Status: StatusUploaded}

	if err := repo.Create(t.Context(), doc); err != nil {
		t.Fatalf("first Create() error = %v", err)
	}
	if err := repo.Create(t.Context(), doc); err == nil {
		t.Error("second Create() with the same ID succeeded")
	}
}

func TestMemoryRepositoryUpdate(t *testing.T) {
	repo := NewMemoryRepository()
	doc := Document{ID: uuid.New(), Status: StatusUploaded}
	if err := repo.Create(t.Context(), doc); err != nil {
		t.Fatal(err)
	}

	t.Run("failed update is not saved", func(t *testing.T) {
		errStop := errors.New("stop")
		_, err := repo.Update(t.Context(), doc.ID, func(d *Document) error {
			d.Status = StatusFailed
			return errStop
		})
		if !errors.Is(err, errStop) {
			t.Fatalf("Update() error = %v, want %v", err, errStop)
		}
		if got, _ := repo.Get(t.Context(), doc.ID); got.Status != StatusUploaded {
			t.Errorf("status = %s after failed update, want %s", got.Status, StatusUploaded)
		}
	})

	t.Run("unknown document", func(t *testing.T) {
		_, err := repo.Update(t.Context(), uuid.New(), func(*Document) error { return nil })
		if !errors.Is(err, ErrNotFound) {
			t.Errorf("Update() error = %v, want ErrNotFound", err)
		}
	})
}

// Concurrent transitions of the same document must be serialised: exactly one
// UPLOADED -> QUEUED transition can succeed.
func TestMemoryRepositoryConcurrentTransitions(t *testing.T) {
	repo := NewMemoryRepository()
	doc := Document{ID: uuid.New(), Status: StatusUploaded}
	if err := repo.Create(t.Context(), doc); err != nil {
		t.Fatal(err)
	}

	const workers = 20
	var (
		wg        sync.WaitGroup
		mu        sync.Mutex
		succeeded int
	)
	for range workers {
		wg.Go(func() {
			_, err := repo.Update(t.Context(), doc.ID, func(d *Document) error {
				return d.TransitionTo(StatusQueued, time.Now())
			})
			if err == nil {
				mu.Lock()
				succeeded++
				mu.Unlock()
			}
		})
	}
	wg.Wait()

	if succeeded != 1 {
		t.Errorf("%d concurrent transitions succeeded, want exactly 1", succeeded)
	}
}
