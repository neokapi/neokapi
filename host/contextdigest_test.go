package host

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/contextop"
	"github.com/neokapi/neokapi/core/profile"
	"github.com/neokapi/neokapi/core/workspace"
)

// TestContextDigest_SectionsFromTheLog drives the digest through the host API:
// an agent's suggestion lands under Suggested, a person's keep moves it to
// Established with how it got there, two rival suggestions land under Needs
// you, and the marker splits new from earlier without hiding anything.
func TestContextDigest_SectionsFromTheLog(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-digest")
	ctx := t.Context()

	kept := proposeUtilise(t, app, root, agentIn("s1"))
	_, err := app.KeepContextOperations(ctx, ContextKeepRequest{Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipeOf(root), IDs: []string{kept.ID}})
	require.NoError(t, err)

	spelling, err := app.RecordContextObservation(ctx, ContextObserveRequest{
		Actor: agentIn("s2"), Project: recipeOf(root),
		Term: "Quickcast", InsteadOf: []string{"Quick cast", "QuickCast"},
		Evidence: []contextop.Evidence{{Path: "config/app.yaml", Quote: "Try Quick cast today"}},
	})
	require.NoError(t, err)
	for _, use := range []string{"sign in", "log in"} {
		_, err = app.RecordContextObservation(ctx, ContextObserveRequest{
			Actor: agentIn("s-" + use), Project: recipeOf(root),
			Term: use, InsteadOf: []string{"login"},
		})
		require.NoError(t, err)
	}

	digest, err := app.ContextDigest(ctx, ContextDigestRequest{Project: recipeOf(root)})
	require.NoError(t, err)

	require.Len(t, digest.Established, 1)
	assert.Equal(t, "Write use, not utilise.", digest.Established[0].Sentence)
	assert.Equal(t, []string{"kept by you"}, digest.Established[0].How)
	assert.True(t, digest.Established[0].Revertible)
	assert.True(t, digest.Established[0].New, "nobody has looked, so everything is new")

	require.Len(t, digest.Conflicts, 1, "the two rules for login disagree")
	assert.Len(t, digest.Conflicts[0].Sides, 2)
	for _, side := range digest.Conflicts[0].Sides {
		assert.True(t, side.Keepable, "choosing a side keeps it")
	}

	require.Len(t, digest.Suggested, 1)
	assert.Equal(t, DigestThemeNames, digest.Suggested[0].Theme)
	item := digest.Suggested[0].Groups[0].Items[0]
	assert.Equal(t, spelling.ID, item.ID)
	assert.Contains(t, item.Sentence, "Write Quickcast, not Quick cast, QuickCast")
	require.NotNil(t, item.Quote)
	assert.Equal(t, "config/app.yaml", item.Quote.Path)
	assert.Equal(t, "config", item.Collection)
	assert.True(t, item.Keepable)

	assert.Equal(t, 1, digest.Numbers.Rules)
	assert.Equal(t, 1, digest.Numbers.NewThisWeek)
	assert.Equal(t, 1, digest.Numbers.Suggested)
	assert.Equal(t, 1, digest.Numbers.Conflicts)

	// A person at a command line reads it, which moves the marker. The next
	// digest still carries every item, flagged as seen.
	require.NoError(t, app.NoteContextDigestRead(digest))
	assert.False(t, ContextDigestMarker(workspace.ProjectKey(digest.Project)).IsZero())
	again, err := app.ContextDigest(ctx, ContextDigestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, again.Established, 1)
	assert.False(t, again.Established[0].New)
	require.Len(t, again.Suggested, 1)
	assert.False(t, again.Suggested[0].Groups[0].Items[0].New)
	assert.Equal(t, 0, again.Numbers.New)

	var text bytes.Buffer
	require.NoError(t, again.FormatText(&text))
	assert.Contains(t, text.String(), "Needs you")
	assert.Contains(t, text.String(), "Earlier")
	assert.Contains(t, text.String(), "kapi knows 1 rule for ctxops-digest; 1 is new this week.")

	// Choosing a side drops the rival and keeps the chosen rule.
	chosen := again.Conflicts[0].Sides[0]
	res, err := app.ChooseContextSide(ctx, ContextChooseRequest{Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipeOf(root), ID: chosen.ID})
	require.NoError(t, err)
	require.Len(t, res.SetAside, 1)
	assert.Equal(t, contextop.KindDrop, res.SetAside[0].Kind)
	after, err := app.ContextDigest(ctx, ContextDigestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Empty(t, after.Conflicts)
	assert.Equal(t, 2, after.Numbers.Rules)
	require.NotEmpty(t, after.Established)
	assert.Equal(t, chosen.ID, after.Established[0].ID, "the newest rule in force comes first")
	assert.True(t, after.Established[0].New)
}

// TestContextDigest_ContentMovingAwayFromARuleIsDrift: a check counts an
// established rule's forms, the content then moves to the rejected form, and the
// next check's count shows the rule under Drift. Uses the content already held
// when the rule was kept are the baseline, not drift.
func TestContextDigest_ContentMovingAwayFromARuleIsDrift(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-drift")
	ctx := t.Context()

	rule := proposeUtilise(t, app, root, agentIn("s1"))
	_, err := app.KeepContextOperations(ctx, ContextKeepRequest{Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipeOf(root), IDs: []string{rule.ID}})
	require.NoError(t, err)

	checkWith(t, app, root)
	digest, err := app.ContextDigest(ctx, ContextDigestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	assert.Empty(t, digest.Drift, "what the content held when the rule was kept is the baseline")
	require.Len(t, digest.Established, 1)
	require.NotNil(t, digest.Established[0].Usage, "a check counts an established rule's forms")
	assert.Equal(t, 1, digest.Established[0].Usage.RejectedCount)

	require.NoError(t, os.WriteFile(filepath.Join(root, "config", "app.yaml"),
		[]byte("greeting: We utilise the widget every day.\nfarewell: Utilise it, and utilise it again.\n"), 0o600))
	checkWith(t, app, root)

	digest, err = app.ContextDigest(ctx, ContextDigestRequest{Project: recipeOf(root)})
	require.NoError(t, err)
	require.Len(t, digest.Drift, 1)
	assert.Equal(t, rule.ID, digest.Drift[0].Rule.ID)
	assert.Equal(t, 1, digest.Drift[0].Before)
	assert.Equal(t, 3, digest.Drift[0].Rejected)
	assert.Contains(t, digest.Drift[0].Line, `"utilise" 3 times`)
	assert.True(t, digest.Drift[0].Rule.Revertible)
}

// TestUsage_UnchangedCountsRecordNothing: two checks over the same content
// record one usage signal per rule, and a changed count records a second, for
// a suggestion and for an established rule alike.
func TestUsage_UnchangedCountsRecordNothing(t *testing.T) {
	app, _ := contextOpsApp(t)
	root := contextOpsProject(t, "ctxops-usage-once")
	ctx := t.Context()

	suggestion := proposeUtilise(t, app, root, agentIn("s1"))
	established, err := app.RecordContextObservation(ctx, ContextObserveRequest{
		Actor: agentIn("s2"), Project: recipeOf(root),
		Term: "farewell", InsteadOf: []string{"Goodbye"},
		Evidence: []contextop.Evidence{{Path: "config/app.yaml", Unit: "farewell", Quote: "Goodbye"}},
	})
	require.NoError(t, err)
	_, err = app.KeepContextOperations(ctx, ContextKeepRequest{Actor: contextop.Actor{Kind: contextop.ActorPerson}, Project: recipeOf(root), IDs: []string{established.ID}})
	require.NoError(t, err)

	usageSignals := func(target string) int {
		t.Helper()
		log, lerr := app.ContextOperations(ctx, ContextLogRequest{Project: recipeOf(root)})
		require.NoError(t, lerr)
		n := 0
		for _, op := range log.Operations {
			if op.Kind == contextop.KindSignal && op.Target == target && op.Signal != nil && op.Signal.Source == contextop.SignalUsage {
				n++
			}
		}
		return n
	}

	checkWith(t, app, root)
	checkWith(t, app, root)
	assert.Equal(t, 1, usageSignals(suggestion.ID), "an unchanged count of a suggestion is recorded once")
	assert.Equal(t, 1, usageSignals(established.ID), "an unchanged count of an established rule is recorded once")

	require.NoError(t, os.WriteFile(filepath.Join(root, "config", "app.yaml"),
		[]byte("greeting: We utilise the widget every day, and utilise it again.\nfarewell: Goodbye\n"), 0o600))
	checkWith(t, app, root)
	assert.Equal(t, 2, usageSignals(suggestion.ID), "a changed count is recorded")
	assert.Equal(t, 1, usageSignals(established.ID), "a rule whose count held still records nothing")
}

// TestContextDigest_AgentReadLeavesTheMarker: an agent reading the digest does
// not move the person's marker.
func TestContextDigest_AgentReadLeavesTheMarker(t *testing.T) {
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv(EnvActor, "agent")
	app, _ := contextOpsApp(t)
	require.NoError(t, app.NoteContextDigestRead(ContextDigest{Project: "prj_x"}))
	assert.True(t, ContextDigestMarker("prj_x").IsZero())
}

// TestBuildDigest_EstablishedOnEvidenceAndDrift covers what the host API
// cannot produce without a merge: an establish operation naming a merge and a
// correction, and a later usage count moving to a rejected form.
func TestBuildDigest_EstablishedOnEvidenceAndDrift(t *testing.T) {
	now := time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC)
	at := func(d int) time.Time { return now.Add(time.Duration(d) * time.Hour) }
	rule := &profile.TermRule{Term: "business", Replacement: "studio"}
	key := workspace.ProjectKey("prj_fernwell")
	agent := contextop.Actor{Kind: contextop.ActorAgent, Name: "claude", Session: "s1"}

	// Newest first, as Ledger.Records returns them.
	records := []contextop.Record{
		{ID: "07", Project: key, Kind: contextop.KindSignal, Target: "01", At: at(-1), Status: contextop.StatusEstablished,
			Signal: &contextop.Signal{Source: contextop.SignalUsage, Preferred: 38, Rejected: 5, Within: "docs/"}},
		{ID: "05", Project: key, Kind: contextop.KindEstablish, Target: "01", At: at(-20), Because: []string{"03", "04"}, Status: contextop.StatusEstablished},
		{ID: "04", Project: key, Kind: contextop.KindCorrect, At: at(-22), Actor: contextop.Actor{Kind: contextop.ActorPerson},
			Correction: &contextop.Correction{From: "business", To: "studio"}, Evidence: []contextop.Evidence{{Path: "docs/billing.md"}},
			Status: contextop.StatusSuggested},
		{ID: "03", Project: key, Kind: contextop.KindSignal, Target: "01", At: at(-23), Status: contextop.StatusEstablished,
			Signal: &contextop.Signal{Source: contextop.SignalMerge, PR: 412}},
		{ID: "02", Project: key, Kind: contextop.KindSignal, Target: "01", At: at(-24), Status: contextop.StatusEstablished,
			Signal: &contextop.Signal{Source: contextop.SignalUsage, Preferred: 41, Rejected: 2, Within: "docs/"}},
		{ID: "01", Project: key, Kind: contextop.KindObserve, Actor: agent, At: at(-48), Status: contextop.StatusEstablished, Established: true,
			Subject: contextop.Subject{Kind: contextop.SubjectTerm, Term: rule}},
	}
	d := buildDigest(records, key, at(-10), now, nil)

	require.Len(t, d.Established, 1)
	assert.Equal(t, []string{"merged in #412", "your correction in docs/billing.md"}, d.Established[0].How)
	assert.False(t, d.Established[0].New, "it was established before the marker")

	require.Len(t, d.Drift, 1)
	assert.Equal(t, 5, d.Drift[0].Rejected)
	assert.Equal(t, 2, d.Drift[0].Before)
	assert.Equal(t, `docs/ says "studio" 38 times and "business" 5 times`, d.Drift[0].Line)
	assert.Equal(t, DigestThemeWords, d.Established[0].Theme)
}

func TestDigestUsageLine(t *testing.T) {
	u := digestUsage("studio", []string{"business"}, 41, 2, "docs/")
	assert.Equal(t, `docs/ says "studio" 41 times and "business" twice`, u.Line)
	assert.Equal(t, `The project says "Quickcast" once`, digestUsage("Quickcast", []string{"Quick cast"}, 1, 0, "").Line)
}
