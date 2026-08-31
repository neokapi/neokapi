package host

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeReviewProject writes a project with a fully-translated nb target and a
// gate that needs 50% reviewed, plus a bound (initially absent) .memory.json source.
func writeReviewProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	recipe := `version: v1
name: rev
defaults:
  source_language: en
  target_languages: [nb]
  memory_source: memory.json
collections:
  - path: en.json
    target: "{lang}.json"
ship_gate: { translated: 100, reviewed: 50 }
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))
	return root
}

// writeMultiFileReviewProject writes a collection of two files whose block ids
// collide — the ordinary shape of a prose collection, where every page carries
// its own title and its own opening paragraph, so the ids are unique inside a
// file and repeat across the collection. Both files are fully translated into
// nb; nothing is reviewed yet.
func writeMultiFileReviewProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	recipe := `version: v1
name: rev-multi
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - name: docs
    content:
      - path: docs/*.json
        target: "i18n/{lang}/{path}.json"
ship_gate: { translated: 100, reviewed: 100 }
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	write := func(dir, name, body string) {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(root, dir, name), []byte(body), 0o644))
	}
	write("docs", "berths.json", `{"title":"Berths","intro":"Declare a berth."}`)
	write("docs", "alerts.json", `{"title":"Alerts","intro":"An alert opens."}`)
	write(filepath.Join("i18n", "nb"), "berths.json", `{"title":"Kaiplasser","intro":"Meld en kaiplass."}`)
	write(filepath.Join("i18n", "nb"), "alerts.json", `{"title":"Varsler","intro":"Et varsel åpnes."}`)
	return root
}

// TestReview_MultiFileCollectionCommitsEveryDecision drives the review round
// trip over a collection whose two files share their block ids.
//
// A decision belongs to a (document, unit, locale), and a unit id is unique only
// inside its document. Recorded against less, the second file's approval
// overwrites the first's: `apply` reports every decision applied, the record
// keeps one per id, and the file that lost its decision then reads as stale
// against a source nobody edited — a locale that goes backwards for having been
// reviewed, and cannot be recovered by reviewing it again.
func TestReview_MultiFileCollectionCommitsEveryDecision(t *testing.T) {
	root := writeMultiFileReviewProject(t)
	t.Chdir(root)

	a := &App{}
	recipe := filepath.Join(root, "kapi.yaml")
	rep, err := a.ProjectConvergence(t.Context(), recipe, "en")
	require.NoError(t, err)
	require.Len(t, rep.Review, 4, "two files × two units await review")

	files, ids := map[string]bool{}, map[string]bool{}
	for _, it := range rep.Review {
		files[it.File] = true
		ids[it.Key] = true
	}
	require.Len(t, files, 2, "the queue spans both files")
	require.Len(t, ids, 2, "and both files carry the same two ids — the shape that collides")

	for _, it := range rep.Review {
		changed, aerr := a.ApproveReviewUnit(t.Context(), recipe, "en", it.Locale, it.File, it.Key, "reviewed")
		require.NoError(t, aerr, "%s:%s", it.File, it.Key)
		assert.True(t, changed, "%s:%s reported no change", it.File, it.Key)
	}

	assertCommittedUnits(t, root, 4, "every approval reaches the committed record")

	nb, ok := localeCoverage(runStatusJSON(t), "nb")
	require.True(t, ok)
	assert.Equal(t, 100, nb.Pct["translated"], "reviewing does not un-translate anything")
	assert.Equal(t, 100, nb.Pct["reviewed"], "all four decisions count")
	assert.Zero(t, nb.Stale, "no source changed, so nothing is stale")
	assert.True(t, nb.Shippable, "reviewed:100 is met")

	assert.Empty(t, reviewQueue(t).Pending, "nothing is left awaiting review")
}

// writeReviewedCorrection approves the unit whose source matches srcText (for nb)
// through the real state-store approval path — ApproveReviewUnit records the
// decision in the project state store, the authoritative carrier of review state.
// The target argument is ignored: approval blesses the translation already in the
// file. (Named for historical continuity with the prior .memory.json-based helper.)
func writeReviewedCorrection(t *testing.T, root, srcText, _ string) {
	t.Helper()
	a := &App{}
	proj := filepath.Join(root, "kapi.yaml")
	rep, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	for _, it := range rep.Review {
		if it.Source == srcText {
			ok, err := a.ApproveReviewUnit(context.Background(), proj, "en", it.Locale, it.File, it.Key, "reviewed")
			require.NoError(t, err)
			require.True(t, ok)
			return
		}
	}
	t.Fatalf("no review unit with source %q in %v", srcText, rep.Review)
}

func reviewQueue(t *testing.T) reviewQueueOutput {
	t.Helper()
	a := &App{}
	cmd := NewEnvCommand(context.Background(), "status")
	AddProjectFlag(cmd)
	AddStatusFlags(cmd)
	require.NoError(t, cmd.Flags().Set("review", "true"))
	require.NoError(t, cmd.Flags().Set("json", "true"))
	out, err := captureStdout(t, func() error { return a.RunStatus(cmd, nil) })
	require.NoError(t, err)
	var q reviewQueueOutput
	require.NoError(t, json.Unmarshal([]byte(out), &q), "review queue must emit valid JSON: %s", out)
	return q
}

func TestReview_ApprovalPromotesToReviewed(t *testing.T) {
	root := writeReviewProject(t)
	t.Chdir(root)

	// Before any approval: both units are translated (presence), none reviewed.
	before := runStatusJSON(t)
	nb, ok := localeCoverage(before, "nb")
	require.True(t, ok)
	assert.Equal(t, 100, nb.Pct["translated"])
	assert.Equal(t, 0, nb.Pct["reviewed"], "no approved corrections yet")
	assert.False(t, nb.Shippable, "reviewed:50 unmet at 0% reviewed")

	// Approve one of the two translations (Apple→Eple).
	writeReviewedCorrection(t, root, "Apple", "Eple")

	after := runStatusJSON(t)
	nb2, ok := localeCoverage(after, "nb")
	require.True(t, ok)
	assert.Equal(t, 100, nb2.Pct["translated"], "still fully translated")
	assert.Equal(t, 50, nb2.Pct["reviewed"], "1 of 2 units now approved in the state store")
	assert.True(t, nb2.Shippable, "reviewed:50 is now met")
}

func TestReview_QueueListsUnreviewedUnits(t *testing.T) {
	root := writeReviewProject(t)
	t.Chdir(root)

	// Initially both translated units await review.
	q := reviewQueue(t)
	require.Len(t, q.Pending, 2)
	for _, it := range q.Pending {
		assert.Equal(t, "nb", it.Locale)
	}

	// Approve one; the queue shrinks to the other.
	writeReviewedCorrection(t, root, "Apple", "Eple")
	q2 := reviewQueue(t)
	require.Len(t, q2.Pending, 1)
	assert.Equal(t, "Banana", q2.Pending[0].Source, "only the unreviewed unit remains")
}

// TestReview_ApplyMemoryCorrectionIsRecycleNotReview drives the real `kapi apply` verb
// and asserts the migrated boundary: a tm correction lands in the .memory.json as
// RECYCLE leverage — it does NOT promote review coverage. Review state lives in
// the project state store now (set by ApproveReviewUnit), not the content memory.
func TestReview_ApplyMemoryCorrectionIsRecycleNotReview(t *testing.T) {
	root := writeReviewProject(t)
	t.Chdir(root)

	a := &App{}
	a.InitRegistries()
	cmd := NewEnvCommand(context.Background(), "apply")
	res := a.applyAssetEntry(context.Background(), cmd, changeEntry{
		Kind: kindMemory, Op: "add", Source: "Apple", Target: "Eple",
		SourceLocale: "en", TargetLocale: "nb",
	})
	require.Equal(t, "applied", res.Status, "detail: %s", res.Detail)

	after := runStatusJSON(t)
	nb, ok := localeCoverage(after, "nb")
	require.True(t, ok)
	assert.Equal(t, 0, nb.Pct["reviewed"], "a tm correction is recycle leverage, not a review decision")
	assert.False(t, nb.Shippable, "reviewed:50 is not met by a tm correction alone")
}

func TestReview_EmptyQueueWhenNothingTranslated(t *testing.T) {
	t.Chdir(writeStatusProject(t)) // nb partially translated, ja absent; no memory_source
	q := reviewQueue(t)
	// Every present nb target awaits review; ja has no targets. Just assert it
	// renders and only lists translated units (no panics, no ja entries).
	for _, it := range q.Pending {
		assert.NotEqual(t, "ja", it.Locale, "absent targets are upstream of review")
	}
}

// TestReview_EditAfterApprovalInvalidatesReview proves the state model's upgrade
// over the old content-keyed .memory.json: an approval is bound to the targetHash of the
// translation it blessed, so editing that translation drops the unit back below
// the reviewed rung — something the content-keyed content memory index could not express.
func TestReview_EditAfterApprovalInvalidatesReview(t *testing.T) {
	root := writeReviewProject(t)
	t.Chdir(root)

	writeReviewedCorrection(t, root, "Apple", "Eple") // approve a→Eple
	assertCommittedUnits(t, root, 1, "approval commits to the project's unit record")
	nb, ok := localeCoverage(runStatusJSON(t), "nb")
	require.True(t, ok)
	assert.Equal(t, 50, nb.Pct["reviewed"], "the approved unit counts as reviewed")

	// Edit the approved translation — the decision no longer blesses this text.
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple-EDITED","b":"Banan"}`), 0o644))
	nb2, ok := localeCoverage(runStatusJSON(t), "nb")
	require.True(t, ok)
	assert.Equal(t, 0, nb2.Pct["reviewed"],
		"editing the approved translation invalidates the review (targetHash link)")
}

// TestReview_ApplyReviewKindPromotesViaStateStore drives the CLI approval verb:
// `kapi apply` with a `kind:"review"` change-set records the decision in the
// project state store (the counterpart of the desktop approve), so the unit
// counts as reviewed — unlike a `kind:"memory"` entry, which is recycle-only.
func TestReview_ApplyReviewKindPromotesViaStateStore(t *testing.T) {
	root := writeReviewProject(t)
	t.Chdir(root)

	a := &App{}
	rep, err := a.ProjectConvergence(context.Background(), filepath.Join(root, "kapi.yaml"), "en")
	require.NoError(t, err)
	var item ReviewItem
	for _, it := range rep.Review {
		if it.Source == "Apple" {
			item = it
		}
	}
	require.NotEmpty(t, item.Key, "expected an 'Apple' review unit")

	a2 := &App{}
	a2.InitRegistries()
	res := a2.applyReviewEntry(context.Background(), NewEnvCommand(context.Background(), "apply"), changeEntry{
		Kind: kindReview, File: item.File, ID: item.Key, Locale: item.Locale, Status: "reviewed",
	})
	require.Equal(t, "applied", res.Status, "detail: %s", res.Detail)
	assertCommittedUnits(t, root, 1, "approval commits to the project's unit record")

	nb, ok := localeCoverage(runStatusJSON(t), "nb")
	require.True(t, ok)
	assert.Equal(t, 50, nb.Pct["reviewed"], "a kind:review apply promotes via the state store")

	// Idempotent: re-applying the same decision is a no-op.
	res2 := a2.applyReviewEntry(context.Background(), NewEnvCommand(context.Background(), "apply"), changeEntry{
		Kind: kindReview, File: item.File, ID: item.Key, Locale: item.Locale, Status: "reviewed",
	})
	assert.Equal(t, "skipped", res2.Status)
}

// assertCommittedUnits commits the project's staged decisions and asserts how
// many units the committed record then holds.
//
// It commits because recording no longer does: a decision is staged, and
// `kapi commit` publishes it.
func assertCommittedUnits(t *testing.T, root string, want int, msg string) {
	t.Helper()
	_, err := (&App{}).CommitProjectState(t.Context(), root)
	require.NoError(t, err)

	layout := project.Layout{StateDir: filepath.Join(root, project.StateDirName)}
	units, err := state.ReadCommitted(layout.UnitStateDir())
	require.NoError(t, err)
	assert.Len(t, units, want, msg)
}
