package document

import "slices"

// Status is a document's position in the processing pipeline.
type Status string

// Document statuses.
const (
	StatusUploaded       Status = "UPLOADED"
	StatusQueued         Status = "QUEUED"
	StatusProcessing     Status = "PROCESSING"
	StatusCompleted      Status = "COMPLETED"
	StatusReviewRequired Status = "REVIEW_REQUIRED"
	StatusFailed         Status = "FAILED"
)

// transitions lists the statuses each status may move to. Statuses without an
// entry are terminal.
var transitions = map[Status][]Status{
	StatusUploaded:   {StatusQueued, StatusFailed},
	StatusQueued:     {StatusProcessing, StatusFailed},
	StatusProcessing: {StatusCompleted, StatusReviewRequired, StatusFailed},
}

// CanTransitionTo reports whether a document may move from s to next.
func (s Status) CanTransitionTo(next Status) bool {
	return slices.Contains(transitions[s], next)
}

// IsTerminal reports whether no further transitions are possible from s.
func (s Status) IsTerminal() bool {
	return len(transitions[s]) == 0
}
