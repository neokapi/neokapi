package host

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/contextop"
)

// TestAPersonsCorrectionNeverFailsTheirBuild establishes a rule, has a person
// write against it, and checks that the rule then reports instead of failing,
// until the person sets their correction aside.
func TestAPersonsCorrectionNeverFailsTheirBuild(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-contest")

	suggested := proposeUtilise(t, app, root, agentIn("s1"))
	_, err := app.KeepContextOperation(t.Context(), ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: suggested.ID})
	require.NoError(t, err)
	require.Equal(t, check.VerdictFailed, checkWith(t, app, root).Verdict, "an established rule fails the gate")

	correction, err := app.RecordContextCorrection(t.Context(), ContextCorrectRequest{
		Actor: person, Project: recipeOf(root), From: "use", To: "utilise",
		Evidence: []contextop.Evidence{{Path: "config/app.yaml"}},
	})
	require.NoError(t, err)
	assert.Contains(t, correction.Landed, "is contested", "the correction says what it took out of force")

	rule, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root), Subjects: true, Status: contextop.StatusContested})
	require.NoError(t, err)
	require.Len(t, rule.Operations, 1)
	assert.Equal(t, suggested.ID, rule.Operations[0].ID)
	assert.Equal(t, []string{correction.ID}, rule.Operations[0].ContestedBy)

	contested := checkWith(t, app, root)
	assert.NotEqual(t, check.VerdictFailed, contested.Verdict, "a person's own edit never fails their build")
	assert.NotEmpty(t, vocabularyFindings(contested), "the contested rule still reports")

	_, err = app.DropContextOperation(t.Context(), ContextDropRequest{Actor: person, Project: recipeOf(root), ID: correction.ID})
	require.NoError(t, err)
	assert.Equal(t, check.VerdictFailed, checkWith(t, app, root).Verdict, "setting the correction aside puts the rule back")
}

// TestKeepSessionLeavesContestedSuggestions keeps a session and checks that the
// suggestion another session disagrees with waits for a person to choose.
func TestKeepSessionLeavesContestedSuggestions(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-keep-session")

	observe := func(session, term string, insteadOf ...string) ContextOperation {
		op, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
			Actor: agentIn(session), Project: recipeOf(root), Term: term, InsteadOf: insteadOf,
			Evidence: []contextop.Evidence{{Path: "config/app.yaml"}},
		})
		require.NoError(t, err)
		return op
	}
	quick := observe("s1", "Quickcast", "Quick cast")
	disputed := observe("s1", "use", "utilise")
	other := observe("s2", "employ", "utilise")
	assert.Equal(t, contextop.StatusContested, other.Status)
	assert.Equal(t, []string{disputed.ID}, other.ContestedBy)

	rule, ok := quick.Rule()
	require.True(t, ok)
	assert.Equal(t, "Quick cast", rule.Term)
	assert.Equal(t, []string{"Quick-cast", "QuickCast", "quickcast"}, rule.Forms)

	res, err := app.KeepContextOperations(t.Context(), ContextKeepRequest{Actor: person, Project: recipeOf(root), Session: "s1"})
	require.NoError(t, err)
	require.Len(t, res.Kept, 1)
	assert.Equal(t, quick.ID, res.Kept[0].Target)
	require.Len(t, res.Skipped, 1)
	assert.Equal(t, disputed.ID, res.Skipped[0].ID)
	assert.Contains(t, res.Skipped[0].Reason, "#"+other.Short)

	_, err = app.KeepContextOperation(t.Context(), ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: disputed.ID})
	require.Error(t, err, "naming a contested suggestion is refused until a person chooses")
	assert.Contains(t, err.Error(), "kapi context drop")

	_, err = app.DropContextOperation(t.Context(), ContextDropRequest{Actor: person, Project: recipeOf(root), ID: other.ID})
	require.NoError(t, err)
	_, err = app.KeepContextOperation(t.Context(), ContextKeepRequest{Actor: person, Project: recipeOf(root), ID: disputed.ID})
	require.NoError(t, err, "once the other side is dropped the choice is made")
}

// TestContextSearchFindsWhatWasObserved asks about a word an agent has just
// observed and expects the suggestion back rather than "nothing recorded".
func TestContextSearchFindsWhatWasObserved(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-search")
	op, err := app.RecordContextObservation(t.Context(), ContextObserveRequest{
		Actor: agentIn("s1"), Project: recipeOf(root), Text: "the forecast feature is one word",
		Term: "Quickcast", InsteadOf: []string{"Quick cast"},
		Evidence: []contextop.Evidence{{Path: "config/app.yaml"}},
	})
	require.NoError(t, err)

	cmd := NewEnvCommand(t.Context(), "context-search")
	cmd.Flags().String(projectFlagName, recipeOf(root), "")
	src, cleanup := app.ContextSearchSourcesFor(cmd, "", "")
	defer cleanup()
	res, err := SearchContext(t.Context(), src, ContextSearchRequest{Query: "quickcast"})
	require.NoError(t, err)
	require.Len(t, res.Suggestions, 1)
	assert.Equal(t, op.ID, res.Suggestions[0].Operation)
	assert.Equal(t, "Quickcast", res.Suggestions[0].Replacement)
	assert.NotEqual(t, CoverageEmpty, res.Coverage)
}
