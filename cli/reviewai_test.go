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

// writeReviewProjectWithGate writes the two-unit nb review fixture with an
// explicit ship gate spelling.
func writeReviewProjectWithGate(t *testing.T, gateYAML string) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	recipe := `version: v1
name: rev
defaults:
  source_language: en
  target_languages: [nb]
content:
  - path: en.json
    target: "{lang}.json"
ship_gate: ` + gateYAML + `
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"),
		[]byte(`{"a":"Apple","b":"Banana"}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))
	return root
}

// TestApplyReviewDecisionAs_RecordsIdentity: the decision's By lands in the
// committed state artifact — "ai/<model>" for autonomous approvals,
// "agent/<client>" for MCP agents, empty for plain human decisions.
func TestApplyReviewDecisionAs_RecordsIdentity(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	before, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	apple := itemBySource(t, before.Review, "Apple")
	banana := itemBySource(t, before.Review, "Banana")

	_, err = a.ApplyReviewDecisionAs(context.Background(), proj, "en",
		ReviewUnitRef{File: apple.File, Key: apple.Key, Locale: apple.Locale},
		ReviewDecisionApproved, "", "ai/claude-x")
	require.NoError(t, err)
	_, err = a.ApplyReviewDecisionAs(context.Background(), proj, "en",
		ReviewUnitRef{File: banana.File, Key: banana.Key, Locale: banana.Locale},
		ReviewDecisionApproved, "", "agent/claude-code")
	require.NoError(t, err)

	f := struct{ Units []state.UnitState }{Units: readCommittedUnits(t, root)}
	byUnit := map[string]state.UnitState{}
	for _, u := range f.Units {
		byUnit[u.Unit] = u
	}
	assert.Equal(t, "ai/claude-x", byUnit[apple.Key].Decision.By)
	assert.Equal(t, "agent/claude-code", byUnit[banana.Key].Decision.By)
	assert.True(t, state.IsAIDecision(byUnit[apple.Key].Decision.By))
	assert.False(t, state.IsAIDecision(byUnit[banana.Key].Decision.By),
		"agent decisions act for a person — not AI class")
}

// TestShipGate_ShortFormRequiresHumanReview: an AI approval promotes the unit
// to reviewed in the display percentages (with the AIReviewed qualifier), but
// the legacy short-form `reviewed: 100` gate treats it as NOT reviewed — the
// honest default.
func TestShipGate_ShortFormRequiresHumanReview(t *testing.T) {
	root := writeReviewProjectWithGate(t, `{ translated: 100, reviewed: 100 }`)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	rep, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	for _, it := range rep.Review {
		by := ""
		if it.Source == "Apple" {
			by = "ai/claude-x" // one AI approval, one human approval
		}
		_, err = a.ApplyReviewDecisionAs(context.Background(), proj, "en",
			ReviewUnitRef{File: it.File, Key: it.Key, Locale: it.Locale},
			ReviewDecisionApproved, "", by)
		require.NoError(t, err)
	}

	after, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	lc := after.Locales[0]
	assert.Equal(t, 100, lc.Pct["reviewed"], "display: both units read reviewed")
	assert.Equal(t, 1, lc.AIReviewed, "one of them via an ai/ decision")
	assert.False(t, lc.Shippable, "short-form reviewed: 100 requires human review")
	require.Len(t, lc.Pending, 1)
	assert.Equal(t, "reviewed", lc.Pending[0].State)
	assert.InDelta(t, 50.0, lc.Pending[0].Actual, 1e-9, "only the human approval counts")
	assert.Empty(t, after.Review, "both decided units left the queue")
}

// TestShipGate_ByAnyAdmitsAIApprovals: the extended form `by: any` lets AI
// approvals satisfy the reviewed threshold.
func TestShipGate_ByAnyAdmitsAIApprovals(t *testing.T) {
	root := writeReviewProjectWithGate(t, `{ translated: 100, reviewed: { pct: 100, by: any } }`)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	rep, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	for _, it := range rep.Review {
		_, err = a.ApplyReviewDecisionAs(context.Background(), proj, "en",
			ReviewUnitRef{File: it.File, Key: it.Key, Locale: it.Locale},
			ReviewDecisionApproved, "", "ai/claude-x")
		require.NoError(t, err)
	}

	after, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	lc := after.Locales[0]
	assert.True(t, lc.Shippable, "by: any admits ai/ approvals")
	assert.Equal(t, 2, lc.AIReviewed)
}

// TestShipGate_ExplicitHumanExcludesAIApprovals: the extended form `by: human`
// behaves exactly like the short form for ai/ decisions.
func TestShipGate_ExplicitHumanExcludesAIApprovals(t *testing.T) {
	root := writeReviewProjectWithGate(t, `{ reviewed: { pct: 50, by: human } }`)
	proj := filepath.Join(root, "kapi.yaml")
	a := &App{}

	rep, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	apple := itemBySource(t, rep.Review, "Apple")
	_, err = a.ApplyReviewDecisionAs(context.Background(), proj, "en",
		ReviewUnitRef{File: apple.File, Key: apple.Key, Locale: apple.Locale},
		ReviewDecisionApproved, "", "ai/claude-x")
	require.NoError(t, err)

	after, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	assert.False(t, after.Locales[0].Shippable)

	// A human approval of the second unit clears the 50% human bar.
	banana := itemBySource(t, rep.Review, "Banana")
	_, err = a.ApplyReviewDecision(context.Background(), proj, "en",
		ReviewUnitRef{File: banana.File, Key: banana.Key, Locale: banana.Locale},
		ReviewDecisionApproved, "")
	require.NoError(t, err)
	final, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	assert.True(t, final.Locales[0].Shippable)
}

// TestRecordAIReviews_AnnotatesQueue: annotations store score + findings in the
// state store, the queue surfaces them, and an edit to the translation makes
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
	_, err = a.ApplyReviewDecisionAs(context.Background(), proj, "en",
		ReviewUnitRef{File: apple.File, Key: apple.Key, Locale: apple.Locale},
		ReviewDecisionApproved, "", "ai/claude-x")
	require.NoError(t, err)

	f := struct{ Units []state.UnitState }{Units: readCommittedUnits(t, root)}
	require.Len(t, f.Units, 1)
	require.NotNil(t, f.Units[0].AIReview, "the annotation rides along with the decision")
	assert.Equal(t, 97, f.Units[0].AIReview.Score)
	assert.Equal(t, "ai/claude-x", f.Units[0].Decision.By)
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
		ReviewDecisionApproved, "", "agent/claude-code")
	require.NoError(t, err)

	info, err = a.ReviewUnit(context.Background(), proj, "en", ref)
	require.NoError(t, err)
	assert.Equal(t, "reviewed", info.Status)
	assert.Equal(t, ReviewDecisionApproved, info.ReviewState)
	assert.Equal(t, "agent/claude-code", info.By)
	require.NotNil(t, info.AIScore)
	assert.Equal(t, 88, *info.AIScore)
	assert.Equal(t, "claude-x", info.AIModel)
	require.Len(t, info.AIFindings, 1)

	_, err = a.ReviewUnit(context.Background(), proj, "en",
		ReviewUnitRef{File: apple.File, Key: "missing", Locale: apple.Locale})
	require.Error(t, err)
}
