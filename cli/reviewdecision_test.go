package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/project"
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

// TestApplyReviewDecision_ApprovedMatchesApprove verifies the generalized entry
// point records the same reviewed state the ApproveReviewUnit veneer does.
func TestApplyReviewDecision_ApprovedMatchesApprove(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	before, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	item := itemBySource(t, before.Review, "Apple")

	changed, err := a.ApplyReviewDecision(context.Background(), proj, "en",
		ReviewUnitRef{File: item.File, Key: item.Key, Locale: item.Locale}, ReviewDecisionApproved, "")
	require.NoError(t, err)
	assert.True(t, changed)

	after, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	assert.Equal(t, 50, after.Locales[0].Pct["reviewed"], "1 of 2 units reviewed")
	require.Len(t, after.Review, 1, "the approved unit left the queue")

	// A redundant approval is a no-op.
	changed2, err := a.ApplyReviewDecision(context.Background(), proj, "en",
		ReviewUnitRef{File: item.File, Key: item.Key, Locale: item.Locale}, ReviewDecisionApproved, "")
	require.NoError(t, err)
	assert.False(t, changed2)
}

// TestApplyReviewDecision_RejectedReturnsToDraft drives the rejection path: the
// unit drops out of the review queue, its coverage reads draft (below the
// translated presence baseline), and the reviewer's note is kept in the
// committed state artifact.
func TestApplyReviewDecision_RejectedReturnsToDraft(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	before, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	require.Len(t, before.Review, 2)
	assert.Equal(t, 100, before.Locales[0].Pct["translated"])
	item := itemBySource(t, before.Review, "Apple")

	changed, err := a.ApplyReviewDecision(context.Background(), proj, "en",
		ReviewUnitRef{File: item.File, Key: item.Key, Locale: item.Locale},
		ReviewDecisionRejected, "wrong register — too formal")
	require.NoError(t, err)
	assert.True(t, changed)

	after, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	require.Len(t, after.Review, 1, "the rejected unit left the review queue (back to the work queue)")
	assert.NotEqual(t, item.Key, after.Review[0].Key)
	assert.Equal(t, 50, after.Locales[0].Pct["translated"], "the rejected unit reads draft, below translated")
	assert.Equal(t, 100, after.Locales[0].Pct["draft"], "draft is the rejected unit's rung")
	assert.Equal(t, 0, after.Locales[0].Pct["reviewed"])

	// The note survives in the committed state artifact.
	f := struct{ Units []state.UnitState }{Units: commitAndReadUnits(t, root)}
	require.Len(t, f.Units, 1)
	assert.Equal(t, "draft", string(f.Units[0].Status))
	assert.Equal(t, ReviewDecisionRejected, f.Units[0].Decision.ReviewState)
	assert.Equal(t, "wrong register — too formal", f.Units[0].Decision.Note)
	assert.NotEmpty(t, f.Units[0].TargetHash, "the rejection binds to the translation it judged")

	// A redundant rejection (same translation, same note) is a no-op.
	changed2, err := a.ApplyReviewDecision(context.Background(), proj, "en",
		ReviewUnitRef{File: item.File, Key: item.Key, Locale: item.Locale},
		ReviewDecisionRejected, "wrong register — too formal")
	require.NoError(t, err)
	assert.False(t, changed2)
}

// TestApplyReviewDecision_RejectionStaleAfterEdit: retranslating a rejected unit
// makes the rejection stale, so the unit re-enters the review queue as
// translated — the convergence loop closes.
func TestApplyReviewDecision_RejectionStaleAfterEdit(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	before, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	item := itemBySource(t, before.Review, "Apple")

	_, err = a.ApplyReviewDecision(context.Background(), proj, "en",
		ReviewUnitRef{File: item.File, Key: item.Key, Locale: item.Locale}, ReviewDecisionRejected, "typo")
	require.NoError(t, err)

	mid, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	require.Len(t, mid.Review, 1, "rejected unit is out of the queue")

	// Retranslate the rejected unit — the recorded rejection no longer judges
	// the current translation.
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Nytt eple","b":"Banan"}`), 0o644))

	after, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	require.Len(t, after.Review, 2, "the retranslated unit re-entered the review queue")
	assert.Equal(t, 100, after.Locales[0].Pct["translated"], "back at the translated baseline")
}

// TestApplyReviewDecision_SignedOffTopRung mirrors the sign-off path through the
// generalized entry point.
func TestApplyReviewDecision_SignedOffTopRung(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	before, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	item := before.Review[0]

	changed, err := a.ApplyReviewDecision(context.Background(), proj, "en",
		ReviewUnitRef{File: item.File, Key: item.Key, Locale: item.Locale}, ReviewDecisionSignedOff, "")
	require.NoError(t, err)
	assert.True(t, changed)

	after, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	assert.Equal(t, 50, after.Locales[0].Pct["signed-off"])
	assert.Equal(t, 50, after.Locales[0].Pct["reviewed"], "signed-off implies reviewed")
}

func TestApplyReviewDecision_UnknownDecision(t *testing.T) {
	root := writeReviewProject(t)
	a := &App{}
	_, err := a.ApplyReviewDecision(context.Background(), filepath.Join(root, "kapi.yaml"), "en",
		ReviewUnitRef{File: "nb.json", Key: "a", Locale: "nb"}, "meh", "")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown review outcome")
}

func TestApplyReviewDecision_UnitNotFound(t *testing.T) {
	root := writeReviewProject(t)
	a := &App{}
	_, err := a.ApplyReviewDecision(context.Background(), filepath.Join(root, "kapi.yaml"), "en",
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
// Recording no longer writes the committed record: a decision is staged, and
// `kapi commit` publishes it. So a test that asserts what the record holds has
// to commit first — which is also what pins that the decision made it into the
// working store at all.
func commitAndReadUnits(t *testing.T, root string) []state.UnitState {
	t.Helper()
	// A fresh App: the store is owned per App, and this one exists only to
	// publish what the App under test staged into the same file.
	committer := &host.App{}
	defer committer.Shutdown()
	_, err := committer.CommitProjectState(t.Context(), root)
	require.NoError(t, err)

	layout := project.Layout{StateDir: filepath.Join(root, project.StateDirName)}
	units, err := state.ReadCommitted(layout.UnitStateDir())
	require.NoError(t, err)
	return units
}
