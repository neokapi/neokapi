package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/contextop"
)

// TestSettle_ACorrectionTowardASuggestionEstablishesIt: an agent suggests a
// rule, a later session records the person changing the text the rule's way,
// and the rule is established and lands where a keep would, so the check now
// fails on it.
func TestSettle_ACorrectionTowardASuggestionEstablishesIt(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-settle")

	proposed := proposeUtilise(t, app, root, agentIn("s1"))
	assert.Equal(t, contextop.StatusSuggested, proposed.Status)
	assert.NotEqual(t, check.VerdictFailed, checkWith(t, app, root).Verdict)

	corrected, err := app.RecordContextCorrection(t.Context(), ContextCorrectRequest{
		Actor:    agentIn("s2"),
		Project:  recipeOf(root),
		From:     "utilise",
		To:       "use",
		Evidence: []contextop.Evidence{{Path: "config/app.yaml", Unit: "farewell"}},
	})
	require.NoError(t, err)
	assert.Contains(t, corrected.Landed, "is established on its evidence")

	log, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	var establish, rule *ContextOperation
	for i, op := range log.Operations {
		switch {
		case op.Kind == contextop.KindEstablish:
			establish = &log.Operations[i]
		case op.ID == proposed.ID:
			rule = &log.Operations[i]
		}
	}
	require.NotNil(t, establish, "settling records what it established")
	assert.Equal(t, []string{corrected.ID}, establish.Because)
	require.NotNil(t, rule)
	assert.Equal(t, contextop.StatusEstablished, rule.Status)
	assert.Contains(t, rule.line(), "1 correction toward it")

	assert.Equal(t, check.VerdictFailed, checkWith(t, app, root).Verdict, "the established rule fails the check")
}
