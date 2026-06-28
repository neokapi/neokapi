package cli

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestProjectConvergence_Composes verifies the exported entry point bundles the
// same coverage, source-readiness, and review-queue derivations the CLI uses.
func TestProjectConvergence_Composes(t *testing.T) {
	root := writeReviewProject(t) // fully-translated nb, reviewed:50 gate, .klftm bound

	a := &App{}
	report, err := a.ProjectConvergence(context.Background(), filepath.Join(root, "proj.kapi"), "en")
	require.NoError(t, err)

	require.Len(t, report.Locales, 1)
	assert.Equal(t, "nb", report.Locales[0].Locale)
	assert.Equal(t, 100, report.Locales[0].Pct["translated"])
	assert.Equal(t, 0, report.Locales[0].Pct["reviewed"], "no approved corrections yet")
	assert.False(t, report.Locales[0].Shippable, "reviewed:50 unmet")

	// Both translated units await review.
	assert.Len(t, report.Review, 2)

	// Approving one lifts reviewed coverage and shrinks the queue on the next call.
	writeReviewedCorrection(t, root, "Apple", "Eple")
	report2, err := a.ProjectConvergence(context.Background(), filepath.Join(root, "proj.kapi"), "en")
	require.NoError(t, err)
	assert.Equal(t, 50, report2.Locales[0].Pct["reviewed"])
	assert.True(t, report2.Locales[0].Shippable)
	assert.Len(t, report2.Review, 1)
}

// TestApproveReviewUnit_PromotesAndLeavesQueue drives the approval path: a queue
// item, approved by (locale, file, key), records the correction and drops from
// the queue while reviewed coverage climbs.
func TestApproveReviewUnit_PromotesAndLeavesQueue(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "proj.kapi")
	a := &App{}

	before, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	require.Len(t, before.Review, 2, "both nb units await review")
	assert.Equal(t, 0, before.Locales[0].Pct["reviewed"])

	// Approve the first queued unit by its (locale, file, key).
	item := before.Review[0]
	ok, err := a.ApproveReviewUnit(context.Background(), proj, "en", item.Locale, item.File, item.Key, "reviewed")
	require.NoError(t, err)
	assert.True(t, ok, "a fresh approval records a correction")

	after, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	assert.Equal(t, 50, after.Locales[0].Pct["reviewed"], "1 of 2 units now reviewed")
	require.Len(t, after.Review, 1, "the approved unit left the queue")
	assert.NotEqual(t, item.Key, after.Review[0].Key, "the remaining item is the other unit")

	// Re-approving the same unit is a no-op (already an approved correction).
	ok2, err := a.ApproveReviewUnit(context.Background(), proj, "en", item.Locale, item.File, item.Key, "reviewed")
	require.NoError(t, err)
	assert.False(t, ok2, "re-approval is a no-op")
}

func TestApproveReviewUnit_NotFound(t *testing.T) {
	root := writeReviewProject(t)
	a := &App{}
	_, err := a.ApproveReviewUnit(context.Background(), filepath.Join(root, "proj.kapi"), "en", "nb", "nope.json", "missing", "reviewed")
	require.Error(t, err)
}

func TestApproveReviewUnit_SignOffReachesTopRung(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "proj.kapi")
	a := &App{}

	before, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	item := before.Review[0]

	ok, err := a.ApproveReviewUnit(context.Background(), proj, "en", item.Locale, item.File, item.Key, "signed-off")
	require.NoError(t, err)
	assert.True(t, ok)

	after, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	// signed-off implies reviewed (monotone ladder): 1 of 2 units at each.
	assert.Equal(t, 50, after.Locales[0].Pct["reviewed"], "signed-off implies reviewed")
	assert.Equal(t, 50, after.Locales[0].Pct["signed-off"], "1 of 2 units signed off")
}

func TestApproveReviewUnit_PromoteReviewedToSignedOff(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "proj.kapi")
	a := &App{}

	rep, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	item := rep.Review[0]

	// First approve (reviewed), then sign off the same unit.
	ok1, err := a.ApproveReviewUnit(context.Background(), proj, "en", item.Locale, item.File, item.Key, "reviewed")
	require.NoError(t, err)
	require.True(t, ok1)

	mid, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	assert.Equal(t, 0, mid.Locales[0].Pct["signed-off"], "reviewed but not yet signed off")

	ok2, err := a.ApproveReviewUnit(context.Background(), proj, "en", item.Locale, item.File, item.Key, "signed-off")
	require.NoError(t, err)
	assert.True(t, ok2, "promoting reviewed → signed-off is a change, not a no-op")

	after, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	assert.Equal(t, 50, after.Locales[0].Pct["signed-off"], "the unit is now signed off")
}
