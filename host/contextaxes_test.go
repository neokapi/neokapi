package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
)

// Two suggestions that say different things about one word disagree when they
// can both answer somewhere: one seen in the Nordic content and one seen in
// the Norwegian content both answer in Norway, so both are contested. One seen
// in the German content answers nowhere the Nordic one does, and stays a
// suggestion. Every reader of the log folds it the same way.
func TestContextAxes_ARegionAndACountryInsideItDisagree(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := axesProject(t, "ctxops-axes-contest")

	nordic := observeUtilise(t, app, root, "nordic/a.md", ContextObserveRequest{Actor: agentIn("s1")})
	observe := func(rel, term string) ContextOperation {
		t.Helper()
		op, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
			Actor: agentIn("s2"), Project: recipeOf(root), Term: term, InsteadOf: []string{"utilise"},
			Evidence: []contextop.Evidence{{Path: rel}},
		})
		require.NoError(t, err)
		return op
	}
	norway := observe("norway/a.md", "employ")
	germany := observe("germany/a.md", "apply")

	statusOf := func(id string) contextop.Status {
		t.Helper()
		log, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root), ID: id})
		require.NoError(t, err)
		for _, op := range log.Operations {
			if op.ID == id {
				return op.Status
			}
		}
		t.Fatalf("%s is not in the log", id)
		return ""
	}
	assert.Equal(t, contextop.StatusContested, statusOf(nordic.ID), "the Nordic rule meets the Norwegian one")
	assert.Equal(t, contextop.StatusContested, statusOf(norway.ID))
	assert.Equal(t, contextop.StatusSuggested, statusOf(germany.ID), "Germany is outside the Nordics")

	digest, err := app.ContextDigest(t.Context(), ContextDigestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Len(t, digest.Conflicts, 1, "the digest folds the log as the log read does")
}
