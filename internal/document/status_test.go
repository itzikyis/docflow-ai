package document

import (
	"errors"
	"testing"
	"time"
)

var allStatuses = []Status{
	StatusUploaded, StatusQueued, StatusProcessing,
	StatusCompleted, StatusReviewRequired, StatusFailed,
}

func TestStatusTransitions(t *testing.T) {
	allowed := map[[2]Status]bool{
		{StatusUploaded, StatusQueued}:           true,
		{StatusUploaded, StatusFailed}:           true,
		{StatusQueued, StatusProcessing}:         true,
		{StatusQueued, StatusFailed}:             true,
		{StatusProcessing, StatusCompleted}:      true,
		{StatusProcessing, StatusReviewRequired}: true,
		{StatusProcessing, StatusFailed}:         true,
	}

	for _, from := range allStatuses {
		for _, to := range allStatuses {
			want := allowed[[2]Status{from, to}]
			if got := from.CanTransitionTo(to); got != want {
				t.Errorf("%s -> %s: CanTransitionTo = %v, want %v", from, to, got, want)
			}
		}
	}
}

func TestTerminalStatuses(t *testing.T) {
	terminal := map[Status]bool{StatusCompleted: true, StatusReviewRequired: true, StatusFailed: true}
	for _, s := range allStatuses {
		if got := s.IsTerminal(); got != terminal[s] {
			t.Errorf("%s.IsTerminal() = %v, want %v", s, got, terminal[s])
		}
	}
}

func TestTransitionTo(t *testing.T) {
	created := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	later := created.Add(time.Minute)

	t.Run("valid transition updates status and time", func(t *testing.T) {
		d := Document{Status: StatusUploaded, UpdatedAt: created}
		if err := d.TransitionTo(StatusQueued, later); err != nil {
			t.Fatalf("TransitionTo() error = %v", err)
		}
		if d.Status != StatusQueued || !d.UpdatedAt.Equal(later) {
			t.Errorf("document = %+v, want QUEUED at %v", d, later)
		}
	})

	t.Run("late message cannot reopen a finished document", func(t *testing.T) {
		d := Document{Status: StatusCompleted, UpdatedAt: created}
		err := d.TransitionTo(StatusProcessing, later)
		if !errors.Is(err, ErrInvalidTransition) {
			t.Fatalf("TransitionTo() error = %v, want ErrInvalidTransition", err)
		}
		if d.Status != StatusCompleted || !d.UpdatedAt.Equal(created) {
			t.Errorf("document changed after a rejected transition: %+v", d)
		}
	})
}
