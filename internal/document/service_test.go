package document

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
)

type recordingDispatcher struct {
	dispatched []Document
	err        error
}

func (d *recordingDispatcher) Dispatch(_ context.Context, doc Document) error {
	d.dispatched = append(d.dispatched, doc)
	return d.err
}

type failingRepository struct {
	*MemoryRepository
	err error
}

func (r failingRepository) Create(context.Context, Document) error { return r.err }

var testUpload = Upload{FileName: "invoice.pdf", ContentType: "application/pdf", SizeBytes: 1234, SHA256: "abc"}

func TestRegister(t *testing.T) {
	repo := NewMemoryRepository()
	dispatcher := &recordingDispatcher{}
	svc := NewService(repo, dispatcher)
	fixed := time.Date(2026, 10, 9, 12, 0, 0, 0, time.FixedZone("IDT", 3*60*60))
	svc.now = func() time.Time { return fixed }

	doc, err := svc.Register(t.Context(), testUpload)
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if doc.ID.Version() != 7 {
		t.Errorf("ID version = %d, want 7", doc.ID.Version())
	}
	if doc.Status != StatusUploaded {
		t.Errorf("Status = %s, want %s", doc.Status, StatusUploaded)
	}
	if doc.CreatedAt.Location() != time.UTC || !doc.CreatedAt.Equal(fixed) {
		t.Errorf("CreatedAt = %v, want %v in UTC", doc.CreatedAt, fixed)
	}
	if doc.FileName != testUpload.FileName || doc.SHA256 != testUpload.SHA256 {
		t.Errorf("document = %+v does not match upload %+v", doc, testUpload)
	}

	stored, err := svc.Get(t.Context(), doc.ID)
	if err != nil || stored != doc {
		t.Errorf("Get() = %+v, %v; want %+v", stored, err, doc)
	}
	if len(dispatcher.dispatched) != 1 || dispatcher.dispatched[0].ID != doc.ID {
		t.Errorf("dispatched = %+v, want exactly the registered document", dispatcher.dispatched)
	}
}

func TestRegisterFailures(t *testing.T) {
	errBoom := errors.New("boom")

	t.Run("repository failure is not dispatched", func(t *testing.T) {
		dispatcher := &recordingDispatcher{}
		svc := NewService(failingRepository{NewMemoryRepository(), errBoom}, dispatcher)

		if _, err := svc.Register(t.Context(), testUpload); !errors.Is(err, errBoom) {
			t.Errorf("Register() error = %v, want %v", err, errBoom)
		}
		if len(dispatcher.dispatched) != 0 {
			t.Error("document was dispatched although it was not stored")
		}
	})

	t.Run("dispatch failure is reported", func(t *testing.T) {
		svc := NewService(NewMemoryRepository(), &recordingDispatcher{err: errBoom})

		if _, err := svc.Register(t.Context(), testUpload); !errors.Is(err, errBoom) {
			t.Errorf("Register() error = %v, want %v", err, errBoom)
		}
	})
}

func TestGetUnknownDocument(t *testing.T) {
	svc := NewService(NewMemoryRepository(), &recordingDispatcher{})
	if _, err := svc.Get(t.Context(), uuid.New()); !errors.Is(err, ErrNotFound) {
		t.Errorf("Get() error = %v, want ErrNotFound", err)
	}
}
