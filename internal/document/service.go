package document

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Repository stores documents.
type Repository interface {
	Create(ctx context.Context, doc Document) error
	// Get returns ErrNotFound if no document has the given ID.
	Get(ctx context.Context, id uuid.UUID) (Document, error)
	// Update applies fn to the stored document atomically with respect to
	// other updates of the same document, and saves the result. If fn returns
	// an error, nothing is saved and that error is returned.
	Update(ctx context.Context, id uuid.UUID, fn func(*Document) error) (Document, error)
}

// Dispatcher hands a registered document over for asynchronous processing.
type Dispatcher interface {
	Dispatch(ctx context.Context, doc Document) error
}

// Upload describes a validated file to register.
type Upload struct {
	FileName    string
	ContentType string
	SizeBytes   int64
	SHA256      string
}

// Service registers documents and reports their state.
type Service struct {
	repo       Repository
	dispatcher Dispatcher
	now        func() time.Time
}

// NewService returns a Service storing documents in repo and handing them to
// dispatcher for processing.
func NewService(repo Repository, dispatcher Dispatcher) *Service {
	return &Service{repo: repo, dispatcher: dispatcher, now: time.Now}
}

// Register records an uploaded document with status UPLOADED and dispatches
// it for processing.
//
// The write and the dispatch are two separate operations: if dispatch fails,
// the document stays stored but will never be processed. Closing that gap
// requires making them atomic (a transactional outbox).
func (s *Service) Register(ctx context.Context, u Upload) (Document, error) {
	// UUIDv7 is time-ordered, so new rows append to the end of a B-tree
	// primary-key index instead of landing at random positions.
	id, err := uuid.NewV7()
	if err != nil {
		return Document{}, fmt.Errorf("generate document ID: %w", err)
	}

	now := s.now().UTC()
	doc := Document{
		ID:          id,
		FileName:    u.FileName,
		ContentType: u.ContentType,
		SizeBytes:   u.SizeBytes,
		SHA256:      u.SHA256,
		Status:      StatusUploaded,
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	if err := s.repo.Create(ctx, doc); err != nil {
		return Document{}, fmt.Errorf("create document: %w", err)
	}
	if err := s.dispatcher.Dispatch(ctx, doc); err != nil {
		return Document{}, fmt.Errorf("dispatch document %s: %w", doc.ID, err)
	}
	return doc, nil
}

// Get returns the document with the given ID, or ErrNotFound.
func (s *Service) Get(ctx context.Context, id uuid.UUID) (Document, error) {
	return s.repo.Get(ctx, id)
}
