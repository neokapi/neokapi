package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// writeReviewProject writes a project with a fully-translated nb target and a
// gate that needs 50% reviewed, plus a bound (initially absent) .klftm source.
func writeReviewProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	recipe := `version: v1
name: rev
defaults:
  source_language: en
  target_languages: [nb]
  tm_source: tm.klftm
content:
  - path: en.json
    target: "{lang}.json"
ship_gate: { translated: 100, reviewed: 50 }
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "proj.kapi"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))
	return root
}

// writeReviewedCorrection approves the unit whose source matches srcText (for nb)
// through the real state-store approval path — ApproveReviewUnit records the
// decision in the project state store, the authoritative carrier of review state.
// The target argument is ignored: approval blesses the translation already in the
// file. (Named for historical continuity with the prior .klftm-based helper.)
func writeReviewedCorrection(t *testing.T, root, srcText, _ string) {
	t.Helper()
	a := &App{}
	proj := filepath.Join(root, "proj.kapi")
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

func reviewQueue(t *testing.T) ReviewQueueOutput {
	t.Helper()
	a := &App{}
	cmd := a.NewStatusCmd()
	require.NoError(t, cmd.Flags().Set("review", "true"))
	require.NoError(t, cmd.Flags().Set("json", "true"))
	out, err := captureStdout(t, func() error { return a.runStatus(cmd, nil) })
	require.NoError(t, err)
	var q ReviewQueueOutput
	require.NoError(t, json.Unmarshal([]byte(out), &q), "review queue must emit valid JSON: %s", out)
	return q
}

func TestReview_ApprovalPromotesToReviewed(t *testing.T) {
	root := writeReviewProject(t)
	t.Chdir(root)

	// Before any approval: both units are translated (presence), none reviewed.
	before := runStatusJSON(t)
	nb, ok := locale(before, "nb")
	require.True(t, ok)
	assert.Equal(t, 100, nb.Pct["translated"])
	assert.Equal(t, 0, nb.Pct["reviewed"], "no approved corrections yet")
	assert.False(t, nb.Shippable, "reviewed:50 unmet at 0% reviewed")

	// Approve one of the two translations (Apple→Eple).
	writeReviewedCorrection(t, root, "Apple", "Eple")

	after := runStatusJSON(t)
	nb2, ok := locale(after, "nb")
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

// TestReview_ApplyTMCorrectionIsRecycleNotReview drives the real `kapi apply` verb
// and asserts the migrated boundary: a tm correction lands in the .klftm as
// RECYCLE leverage — it does NOT promote review coverage. Review state lives in
// the project state store now (set by ApproveReviewUnit), not the TM.
func TestReview_ApplyTMCorrectionIsRecycleNotReview(t *testing.T) {
	root := writeReviewProject(t)
	t.Chdir(root)

	a := &App{}
	a.InitRegistries()
	cmd := &cobra.Command{Use: "apply"}
	res := a.applyAssetEntry(context.Background(), cmd, changeEntry{
		Kind: kindTM, Op: "add", Source: "Apple", Target: "Eple",
		SourceLocale: "en", TargetLocale: "nb",
	})
	require.Equal(t, "applied", res.Status, "detail: %s", res.Detail)

	after := runStatusJSON(t)
	nb, ok := locale(after, "nb")
	require.True(t, ok)
	assert.Equal(t, 0, nb.Pct["reviewed"], "a tm correction is recycle leverage, not a review decision")
	assert.False(t, nb.Shippable, "reviewed:50 is not met by a tm correction alone")
}

func TestReview_EmptyQueueWhenNothingTranslated(t *testing.T) {
	t.Chdir(writeStatusProject(t)) // nb partially translated, ja absent; no tm_source
	q := reviewQueue(t)
	// Every present nb target awaits review; ja has no targets. Just assert it
	// renders and only lists translated units (no panics, no ja entries).
	for _, it := range q.Pending {
		assert.NotEqual(t, "ja", it.Locale, "absent targets are upstream of review")
	}
}

// TestReview_EditAfterApprovalInvalidatesReview proves the state model's upgrade
// over the old content-keyed .klftm: an approval is bound to the targetHash of the
// translation it blessed, so editing that translation drops the unit back below
// the reviewed rung — something the content-keyed TM index could not express.
func TestReview_EditAfterApprovalInvalidatesReview(t *testing.T) {
	root := writeReviewProject(t)
	t.Chdir(root)

	writeReviewedCorrection(t, root, "Apple", "Eple") // approve a→Eple
	assert.FileExists(t, filepath.Join(root, ".kapi-state.json"),
		"approval exports the committed state artifact")
	nb, ok := locale(runStatusJSON(t), "nb")
	require.True(t, ok)
	assert.Equal(t, 50, nb.Pct["reviewed"], "the approved unit counts as reviewed")

	// Edit the approved translation — the decision no longer blesses this text.
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple-EDITED","b":"Banan"}`), 0o644))
	nb2, ok := locale(runStatusJSON(t), "nb")
	require.True(t, ok)
	assert.Equal(t, 0, nb2.Pct["reviewed"],
		"editing the approved translation invalidates the review (targetHash link)")
}
