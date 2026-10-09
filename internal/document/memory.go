package document

import (
	"context"
	"fmt"
	"sync"

	"github.com/google/uuid"
)

// MemoryRepository is an in-process Repository. Data is lost on restart and
// is not shared between replicas.
type MemoryRepository struct {
	mu   sync.RWMutex
	docs map[uuid.UUID]Document
}

// NewMemoryRepository returns an empty MemoryRepository.
func NewMemoryRepository() *MemoryRepository {
	return &MemoryRepository{docs: make(map[uuid.UUID]Document)}
}

// Create stores doc. It fails if a document with the same ID exists.
func (r *MemoryRepository) Create(_ context.Context, doc Document) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.docs[doc.ID]; ok {
		return fmt.Errorf("document %s already exists", doc.ID)
	}
	r.docs[doc.ID] = doc
	return nil
}

// Get returns a copy of the stored document.
func (r *MemoryRepository) Get(_ context.Context, id uuid.UUID) (Document, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	doc, ok := r.docs[id]
	if !ok {
		return Document{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	return doc, nil
}

// Update applies fn to a copy of the stored document under the write lock and
// saves the copy only if fn succeeds.
func (r *MemoryRepository) Update(_ context.Context, id uuid.UUID, fn func(*Document) error) (Document, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	doc, ok := r.docs[id]
	if !ok {
		return Document{}, fmt.Errorf("%w: %s", ErrNotFound, id)
	}
	if err := fn(&doc); err != nil {
		return Document{}, err
	}
	r.docs[id] = doc
	return doc, nil
}
