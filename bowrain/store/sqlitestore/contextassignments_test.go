package sqlitestore

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
)

// An assignment is one row per suggestion: assigning again replaces it,
// assigning to no one removes it, and another project holds its own.
func TestContextAssignments_OnePerSuggestion(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	other := createTestProject(t, s)
	at := time.Date(2026, 10, 30, 9, 0, 0, 0, time.UTC)

	none, err := s.ListContextAssignments(ctx, p.ID)
	require.NoError(t, err)
	assert.Empty(t, none)

	require.NoError(t, s.AssignContextSuggestion(ctx, platstore.ContextAssignment{
		ProjectID: p.ID, SuggestionID: "op-b", Assignee: "member-a", AssignedBy: "member-c", AssignedAt: at,
	}))
	require.NoError(t, s.AssignContextSuggestion(ctx, platstore.ContextAssignment{
		ProjectID: p.ID, SuggestionID: "op-a", Assignee: "member-a", AssignedBy: "member-c", AssignedAt: at,
	}))
	require.NoError(t, s.AssignContextSuggestion(ctx, platstore.ContextAssignment{
		ProjectID: p.ID, SuggestionID: "op-b", Assignee: "member-b", AssignedBy: "member-a", AssignedAt: at.Add(time.Hour),
	}))
	got, err := s.ListContextAssignments(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, []platstore.ContextAssignment{
		{ProjectID: p.ID, SuggestionID: "op-a", Assignee: "member-a", AssignedBy: "member-c", AssignedAt: at},
		{ProjectID: p.ID, SuggestionID: "op-b", Assignee: "member-b", AssignedBy: "member-a", AssignedAt: at.Add(time.Hour)},
	}, got, "assigning again replaces the assignment")

	elsewhere, err := s.ListContextAssignments(ctx, other.ID)
	require.NoError(t, err)
	assert.Empty(t, elsewhere, "another project holds its own assignments")

	require.NoError(t, s.AssignContextSuggestion(ctx, platstore.ContextAssignment{ProjectID: p.ID, SuggestionID: "op-a"}))
	got, err = s.ListContextAssignments(ctx, p.ID)
	require.NoError(t, err)
	require.Len(t, got, 1, "assigning to no one removes the assignment")
	assert.Equal(t, "op-b", got[0].SuggestionID)

	require.Error(t, s.AssignContextSuggestion(ctx, platstore.ContextAssignment{ProjectID: p.ID, Assignee: "member-a"}),
		"an assignment names its suggestion")
}
