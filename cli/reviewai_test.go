package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/state"
)

// them stale (the queue drops the score).
func TestRecordAIReviews_AnnotatesQueue(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	before, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	apple := itemBySource(t, before.Review, "Apple")

	n, err := a.RecordAIReviews(context.Background(), proj, "en", apple.Locale, apple.File,
		map[string]state.AIReview{
			apple.Key: {Score: 92, Model: "claude-x", Findings: []state.AIReviewFinding{
				{Severity: "minor", Message: "slightly literal"},
			}},
		})
	require.NoError(t, err)
	assert.Equal(t, 1, n)

	mid, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	require.Len(t, mid.Review, 2, "annotations never move a unit — both still pending")
	got := itemBySource(t, mid.Review, "Apple")
	require.NotNil(t, got.AIScore, "the queue surfaces the fresh annotation")
	assert.Equal(t, 92, *got.AIScore)
	assert.Equal(t, "claude-x", got.AIModel)
	other := itemBySource(t, mid.Review, "Banana")
	assert.Nil(t, other.AIScore, "unannotated units carry no score")

	// Editing the translation invalidates the annotation (hash-bound).
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Nytt eple","b":"Banan"}`), 0o644))
	after, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	stale := itemBySource(t, after.Review, "Apple")
	assert.Nil(t, stale.AIScore, "the edited unit's annotation is stale")

	// Unknown keys are skipped, not errors.
	n2, err := a.RecordAIReviews(context.Background(), proj, "en", apple.Locale, apple.File,
		map[string]state.AIReview{"missing": {Score: 10}})
	require.NoError(t, err)
	assert.Equal(t, 0, n2)
}

// TestRecordAIReviews_SurvivesDecision: an approval recorded after an
// annotation keeps the annotation on the unit's state record.
func TestRecordAIReviews_SurvivesDecision(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	rep, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	apple := itemBySource(t, rep.Review, "Apple")

	_, err = a.RecordAIReviews(context.Background(), proj, "en", apple.Locale, apple.File,
		map[string]state.AIReview{apple.Key: {Score: 97, Model: "claude-x"}})
	require.NoError(t, err)
	_, err = a.ApplyReviewDecision(context.Background(), proj, "en",
		ReviewUnitRef{File: apple.File, Key: apple.Key, Locale: apple.Locale},
		ReviewDecisionApproved, "")
	require.NoError(t, err)

	f := struct{ Units []state.UnitState }{Units: commitAndReadUnits(t, root)}
	require.Len(t, f.Units, 1)
	require.NotNil(t, f.Units[0].AIReview, "the annotation rides along with the decision")
	assert.Equal(t, 97, f.Units[0].AIReview.Score)
	assert.Empty(t, f.Units[0].Decision.By, "the person at the keyboard decided")
}

// TestReviewUnit_FullPicture: the CLI-layer unit read returns full text, the
// recorded decision with identity, and the fresh AI annotation.
func TestReviewUnit_FullPicture(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	rep, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	apple := itemBySource(t, rep.Review, "Apple")
	ref := ReviewUnitRef{File: apple.File, Key: apple.Key, Locale: apple.Locale}

	info, err := a.ReviewUnit(context.Background(), proj, "en", ref)
	require.NoError(t, err)
	assert.Equal(t, "Apple", info.Source)
	assert.Equal(t, "Eple", info.Target)
	assert.Equal(t, "translated", info.Status)
	assert.Empty(t, info.ReviewState)
	assert.Nil(t, info.AIScore)

	_, err = a.RecordAIReviews(context.Background(), proj, "en", apple.Locale, apple.File,
		map[string]state.AIReview{apple.Key: {Score: 88, Model: "claude-x",
			Findings: []state.AIReviewFinding{{Severity: "info", Message: "fine"}}}})
	require.NoError(t, err)
	_, err = a.ApplyReviewDecisionAs(context.Background(), proj, "en", ref,
		ReviewDecisionApproved, "", "ada")
	require.NoError(t, err)

	info, err = a.ReviewUnit(context.Background(), proj, "en", ref)
	require.NoError(t, err)
	assert.Equal(t, "established", info.Status)
	assert.Equal(t, ReviewDecisionApproved, info.ReviewState)
	assert.Equal(t, "ada", info.By)
	require.NotNil(t, info.AIScore)
	assert.Equal(t, 88, *info.AIScore)
	assert.Equal(t, "claude-x", info.AIModel)
	require.Len(t, info.AIFindings, 1)

	_, err = a.ReviewUnit(context.Background(), proj, "en",
		ReviewUnitRef{File: apple.File, Key: "missing", Locale: apple.Locale})
	require.Error(t, err)
}
