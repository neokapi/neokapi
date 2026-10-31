package store

import (
	"context"
	"time"
)

// A ContextAssignment is one suggestion of a project's context ledger
// assigned to a member, so the member's queue lists it. It is team workflow
// and not part of the ledger: assigning records no context operation.
type ContextAssignment struct {
	ProjectID string
	// SuggestionID is the id of the suggestion's operation in the ledger, or
	// of one side of a disagreement.
	SuggestionID string
	// Assignee is the member the suggestion is assigned to, and AssignedBy
	// the member who assigned it.
	Assignee   string
	AssignedBy string
	AssignedAt time.Time
}

// ContextAssignmentStore holds the assignments of a project's suggestions.
type ContextAssignmentStore interface {
	// AssignContextSuggestion assigns a suggestion to a.Assignee, replacing
	// any assignment it holds. An empty Assignee removes the assignment.
	AssignContextSuggestion(ctx context.Context, a ContextAssignment) error
	// ListContextAssignments returns every assignment a project holds,
	// ordered by suggestion id.
	ListContextAssignments(ctx context.Context, projectID string) ([]ContextAssignment, error)
}

// ContextAssignments finds the assignment store behind a content store,
// looking through any decorator that names what it wraps, as ContextObjects
// does.
func ContextAssignments(cs ContentStore) (ContextAssignmentStore, bool) {
	for cs != nil {
		if s, ok := cs.(ContextAssignmentStore); ok {
			return s, true
		}
		inner, ok := cs.(interface{ Unwrap() ContentStore })
		if !ok {
			return nil, false
		}
		cs = inner.Unwrap()
	}
	return nil, false
}
