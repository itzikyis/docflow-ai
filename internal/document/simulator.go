package document

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/itzikyis/docflow-ai/internal/logging"
)

// ErrDispatcherClosed is returned by Dispatch after Close.
var ErrDispatcherClosed = errors.New("dispatcher is closed")

// simulatedPipeline is the sequence of statuses a simulated document passes
// through after upload.
var simulatedPipeline = []Status{StatusQueued, StatusProcessing, StatusCompleted}

// Simulator is a Dispatcher that advances documents through the pipeline
// in-process, one status every step. It stands in for real asynchronous
// processing so the API's behaviour can be exercised end to end.
type Simulator struct {
	repo   Repository
	logger *slog.Logger
	step   time.Duration

	mu     sync.Mutex
	closed bool
	done   chan struct{}
	wg     sync.WaitGroup
}

// NewSimulator returns a Simulator that updates documents in repo.
func NewSimulator(repo Repository, logger *slog.Logger, step time.Duration) *Simulator {
	return &Simulator{repo: repo, logger: logger, step: step, done: make(chan struct{})}
}

// Dispatch starts simulated processing of doc in the background. The
// processing outlives ctx's cancellation (the HTTP request ends long before
// processing does) but keeps its values, such as the request ID for logging.
func (s *Simulator) Dispatch(ctx context.Context, doc Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return ErrDispatcherClosed
	}
	ctx = logging.WithAttrs(context.WithoutCancel(ctx), slog.String("document_id", doc.ID.String()))
	s.wg.Go(func() { s.process(ctx, doc.ID) })
	return nil
}

// Close stops all simulated processing and waits for it to exit. Documents
// being processed keep whatever status they had reached.
func (s *Simulator) Close() {
	s.mu.Lock()
	if !s.closed {
		s.closed = true
		close(s.done)
	}
	s.mu.Unlock()
	s.wg.Wait()
}

func (s *Simulator) process(ctx context.Context, id uuid.UUID) {
	for _, next := range simulatedPipeline {
		select {
		case <-s.done:
			s.logger.InfoContext(ctx, "simulated processing stopped by shutdown")
			return
		case <-time.After(s.step):
		}

		_, err := s.repo.Update(ctx, id, func(d *Document) error {
			return d.TransitionTo(next, time.Now().UTC())
		})
		if err != nil {
			s.logger.ErrorContext(ctx, "simulated processing failed", "error", err)
			return
		}
		s.logger.InfoContext(ctx, "document status changed", "status", next)
	}
}
