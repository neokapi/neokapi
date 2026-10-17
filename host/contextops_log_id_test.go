package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
)

// A log read for one id is that rule's history: the rule and every decision
// about it, and nothing else the project recorded.
func TestContextLogForOneID(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-log-id")

	proposed := proposeUtilise(t, app, root, agentIn("s1"))
	other, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: agentIn("s1"), Project: recipeOf(root),
		Text: "The greeting is informal.",
	})
	require.NoError(t, err)
	kept, err := app.KeepContextOperation(t.Context(), ContextKeepRequest{
		Actor: person, Project: recipeOf(root), ID: proposed.ID,
	})
	require.NoError(t, err)

	for _, id := range []string{proposed.Short, kept.ID} {
		log, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root), ID: id})
		require.NoError(t, err, id)
		var ids []string
		for _, op := range log.Operations {
			ids = append(ids, op.ID)
		}
		assert.ElementsMatch(t, []string{proposed.ID, kept.ID}, ids, "named by %s", id)
		assert.NotContains(t, ids, other.ID)
	}

	// The limit counts the rule's history, not the newest operations of the
	// whole log, which here would hold only the unrelated note.
	limited, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root), ID: proposed.ID, Limit: 1})
	require.NoError(t, err)
	require.Len(t, limited.Operations, 1)
	assert.Equal(t, kept.ID, limited.Operations[0].ID, "newest first")

	_, err = app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root), ID: "zzzzzzzz"})
	require.ErrorIs(t, err, contextop.ErrNotFound)
}
