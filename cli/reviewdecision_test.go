package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/change"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/host"
)

// itemBySource finds the queued review item whose source preview matches.
func itemBySource(t *testing.T, items []ReviewQueueItem, source string) ReviewQueueItem {
	t.Helper()
	for _, it := range items {
		if it.Source == source {
			return it
		}
	}
	t.Fatalf("no review item with source %q in %v", source, items)
	return ReviewQueueItem{}
}

// A review decision is a decide operation through the change service. An
// establish records the reviewed state, and the same decision sent again on
// the same translation changes nothing.
func TestDecide_EstablishRecordsTheReviewAndARepeatChangesNothing(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	before, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	item := itemBySource(t, before.Review, "Apple")
	ref := ReviewUnitRef{File: item.File, Key: item.Key, Locale: item.Locale}

	changed, err := decideUnit(context.Background(), a, proj, ref, ReviewDecisionApproved, "")
	require.NoError(t, err)
	assert.True(t, changed)

	after, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	assert.Equal(t, 50, after.Locales[0].Pct["established"], "1 of 2 units reviewed")
	require.Len(t, after.Review, 1, "the approved unit left the queue")

	changed2, err := decideUnit(context.Background(), a, proj, ref, ReviewDecisionApproved, "")
	require.NoError(t, err)
	assert.False(t, changed2, "the same decision on the same translation is unchanged")
}

// A reject drops the unit out of the review queue, its coverage reads draft
// (below the translated presence baseline), and the change set's note is kept
// as the reviewer's reason in the committed state artifact.
func TestDecide_RejectKeepsTheNoteAndReturnsTheUnitToDraft(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	before, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	require.Len(t, before.Review, 2)
	assert.Equal(t, 100, before.Locales[0].Pct["translated"])
	item := itemBySource(t, before.Review, "Apple")
	ref := ReviewUnitRef{File: item.File, Key: item.Key, Locale: item.Locale}

	changed, err := decideUnit(context.Background(), a, proj, ref, ReviewDecisionRejected, "wrong register, too formal")
	require.NoError(t, err)
	assert.True(t, changed)

	after, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	require.Len(t, after.Review, 1, "the rejected unit left the review queue (back to the work queue)")
	assert.NotEqual(t, item.Key, after.Review[0].Key)
	assert.Equal(t, 50, after.Locales[0].Pct["translated"], "the rejected unit reads draft, below translated")
	assert.Equal(t, 100, after.Locales[0].Pct["draft"], "draft is the rejected unit's rung")
	assert.Equal(t, 0, after.Locales[0].Pct["established"])

	f := struct{ Units []state.UnitState }{Units: commitAndReadUnits(t, root)}
	require.Len(t, f.Units, 1)
	assert.Equal(t, "draft", string(f.Units[0].Status))
	assert.Equal(t, ReviewDecisionRejected, f.Units[0].Decision.ReviewState)
	assert.Equal(t, "wrong register, too formal", f.Units[0].Decision.Note)
	assert.NotEmpty(t, f.Units[0].TargetHash, "the rejection binds to the translation it judged")

	changed2, err := decideUnit(context.Background(), a, proj, ref, ReviewDecisionRejected, "wrong register, too formal")
	require.NoError(t, err)
	assert.False(t, changed2, "the same rejection with the same note changes nothing")
}

// Retranslating a rejected unit makes the rejection stale, so the unit
// re-enters the review queue as translated: the convergence loop closes.
func TestDecide_ARejectionGoesStaleWhenTheTranslationChanges(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	before, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	item := itemBySource(t, before.Review, "Apple")

	_, err = decideUnit(context.Background(), a, proj,
		ReviewUnitRef{File: item.File, Key: item.Key, Locale: item.Locale}, ReviewDecisionRejected, "typo")
	require.NoError(t, err)

	mid, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	require.Len(t, mid.Review, 1, "rejected unit is out of the queue")

	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Nytt eple","b":"Banan"}`), 0o644))

	after, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	require.Len(t, after.Review, 2, "the retranslated unit re-entered the review queue")
	assert.Equal(t, 100, after.Locales[0].Pct["translated"], "back at the translated baseline")
}

// An agent records a pre-review (advise), never a decision: the change
// service refuses its establish and its reject, and nothing it sent
// establishes a unit.
func TestDecide_AnAgentMayNotDecide(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	before, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	item := before.Review[0]
	ref := ReviewUnitRef{File: item.File, Key: item.Key, Locale: item.Locale}

	agent := change.Actor{Kind: change.ActorAgent, Name: "claude-code", Session: "s1"}
	for _, decision := range []string{ReviewDecisionApproved, ReviewDecisionRejected} {
		_, err := decideUnitAs(context.Background(), a, proj, ref, decision, "", agent)
		var ce *change.Error
		require.ErrorAs(t, err, &ce, "an agent must not %s a unit", decision)
		assert.Equal(t, change.CodeNotPermitted, ce.Code)
	}
	after, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	assert.Equal(t, 0, after.Locales[0].Pct["established"])
	require.Len(t, after.Review, 2, "both units still await a person")
}

// An outcome the contract does not name is refused as invalid, by the decoder
// a change set is read with and by the service an in-process sender reaches.
func TestDecide_AnUnknownOutcomeIsRefused(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	_, err := change.Decode(strings.NewReader(`{"ops":[{"op":"decide","at":{"doc":"nb.json","block":"a","edition":"nb"},"if_match":"*","outcome":"meh"}]}`))
	var ce *change.Error
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, change.CodeInvalid, ce.Code)
	assert.Equal(t, "/ops/0/outcome", ce.Pointer)

	_, err = decideUnit(context.Background(), a, proj, ReviewUnitRef{File: "nb.json", Key: "a", Locale: "nb"}, "meh", "")
	require.ErrorAs(t, err, &ce)
	assert.Equal(t, change.CodeInvalid, ce.Code)
}

// A decision on a unit the document does not hold is refused.
func TestDecide_AUnitTheDocumentDoesNotHold(t *testing.T) {
	root := writeReviewProject(t)
	_, err := decideUnit(context.Background(), &App{}, filepath.Join(root, "kapi.yaml"),
		ReviewUnitRef{File: "nb.json", Key: "missing", Locale: "nb"}, ReviewDecisionApproved, "")
	require.Error(t, err)
}

// TestReviewQueue_CarriesCollection: queue items name their parent collection so
// a surface can filter to (collection, locale).
func TestReviewQueue_CarriesCollection(t *testing.T) {
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	recipe := `version: v1
name: rev
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - name: app
    content:
      - path: en.json
        target: "{lang}.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"), []byte(`{"a":"Apple"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"), []byte(`{"a":"Eple"}`), 0o644))

	a := &App{}
	rep, err := a.ProjectConvergence(context.Background(), filepath.Join(root, "kapi.yaml"), "en")
	require.NoError(t, err)
	require.Len(t, rep.Review, 1)
	assert.Equal(t, "app", rep.Review[0].Collection)
}

// commitAndReadUnits commits the project's staged decisions and reads the
// resulting record.
//
// A decision is durable in the ledger from the moment it is made, so a test
// that asserts what the record holds reads the ledger.
func commitAndReadUnits(t *testing.T, root string) []state.UnitState {
	t.Helper()
	// A fresh App: the store is owned per App, and this one exists only to
	// read what the App under test recorded into the same file.
	reader := &host.App{}
	defer reader.Shutdown()
	st, err := reader.OpenProjectState(t.Context(), root)
	require.NoError(t, err)
	units, err := st.All(t.Context())
	require.NoError(t, err)
	return units
}
