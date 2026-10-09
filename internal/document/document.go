// Package document is the document domain: the Document entity, its status
// state machine, and the service that registers and tracks documents.
package document

import (
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Domain errors.
var (
	ErrNotFound          = errors.New("document not found")
	ErrInvalidTransition = errors.New("invalid status transition")
)

// Document is an uploaded file and its processing state.
type Document struct {
	ID          uuid.UUID
	FileName    string
	ContentType string
	SizeBytes   int64
	SHA256      string // hex-encoded checksum of the content
	StoragePath string
	Status      Status
	CreatedAt   time.Time
	UpdatedAt   time.Time
}

// TransitionTo moves the document to next, or returns ErrInvalidTransition and
// leaves it unchanged. Enforcing transitions here means a late or duplicate
// message can never move a finished document backwards.
func (d *Document) TransitionTo(next Status, now time.Time) error {
	if !d.Status.CanTransitionTo(next) {
		return fmt.Errorf("%w: %s to %s", ErrInvalidTransition, d.Status, next)
	}
	d.Status = next
	d.UpdatedAt = now
	return nil
}
