package host

import (
	"os"
	"os/exec"
	"path/filepath"
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

// TestSettle_ACheckCountsUses: a check of the whole project records how its
// content writes a suggestion's forms, the count shows as standing, and content
// moving to the avoided form counts against the suggestion, so a correction
// toward it no longer settles it.
func TestSettle_ACheckCountsUses(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-usage")
	proposed := proposeUtilise(t, app, root, agentIn("s1"))

	signals := func() []ContextOperation {
		t.Helper()
		log, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root)})
		require.NoError(t, err)
		var out []ContextOperation
		for _, op := range log.Operations {
			if op.Kind == contextop.KindSignal {
				out = append(out, op)
			}
		}
		return out
	}
	checkWith(t, app, root)
	checkWith(t, app, root)
	require.Len(t, signals(), 1, "the same counts are one signal")
	got, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root), Subjects: true})
	require.NoError(t, err)
	require.NotEmpty(t, got.Operations)
	assert.Contains(t, got.Operations[0].line(), "0 of 1 use")

	require.NoError(t, os.WriteFile(filepath.Join(root, "config", "app.yaml"),
		[]byte("greeting: We utilise the widget every day.\nfarewell: Utilise it again\n"), 0o600))
	checkWith(t, app, root)
	require.Len(t, signals(), 2)

	_, err = app.RecordContextCorrection(t.Context(), ContextCorrectRequest{
		Actor: agentIn("s2"), Project: recipeOf(root), From: "utilise", To: "use",
		Evidence: []contextop.Evidence{{Path: "config/app.yaml"}},
	})
	require.NoError(t, err)
	op, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root), Status: contextop.StatusContested})
	require.NoError(t, err)
	require.Len(t, op.Operations, 1)
	assert.Equal(t, proposed.ID, op.Operations[0].ID, "content moving to the avoided form contests it")
}

// TestSettle_AMergeIsEvidence:a change that reached the default branch
// writing the preferred form establishes the suggestion, naming the pull
// request, and settling the same range again records nothing new.
func TestSettle_AMergeIsEvidence(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-merge")
	gitIn := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", root, "-c", "user.name=Asgeir", "-c", "user.email=a@example.com"}, args...)...)
		out, err := cmd.CombinedOutput()
		require.NoError(t, err, string(out))
	}
	gitIn("init", "-q")
	gitIn("add", ".")
	gitIn("commit", "-qm", "Start")

	proposed := proposeUtilise(t, app, root, agentIn("s1"))
	require.NoError(t, os.WriteFile(filepath.Join(root, "config", "app.yaml"),
		[]byte("greeting: We use the widget every day.\nfarewell: Goodbye\n"), 0o600))
	gitIn("commit", "-qam", "Say use (#412)")

	settle := func() ContextSettleResult {
		t.Helper()
		res, err := app.SettleContext(t.Context(), ContextSettleRequest{Project: recipeOf(root), Merged: "HEAD~1..HEAD"})
		require.NoError(t, err)
		return res
	}
	first := settle()
	require.Len(t, first.Signals, 1)
	sig := first.Signals[0]
	assert.Equal(t, proposed.ID, sig.Target)
	require.NotNil(t, sig.Signal)
	assert.Equal(t, 412, sig.Signal.PR)
	assert.Equal(t, "Asgeir", sig.Signal.Merger)
	assert.Equal(t, 1, sig.Signal.Preferred)
	assert.Equal(t, 1, sig.Signal.Rejected)
	require.Len(t, first.Established, 1)

	again := settle()
	assert.Empty(t, again.Signals, "an established rule takes no more merge evidence")
	assert.Empty(t, again.Established)

	log, err := app.ContextOperations(t.Context(), ContextLogRequest{Project: recipeOf(root), Status: contextop.StatusEstablished, Subjects: true})
	require.NoError(t, err)
	require.Len(t, log.Operations, 1)
	assert.Equal(t, proposed.ID, log.Operations[0].ID)
	assert.Contains(t, log.Operations[0].line(), "merged in #412")
}
