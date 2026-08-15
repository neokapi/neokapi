package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/host"
)

// The basis: the source hash a decision blessed. Its absence is the defect these
// tests hold shut — a source edit left its translation approved, counted
// `translated`, and shippable, so the loop published a rendering of a sentence
// the project no longer had and reported nothing. Target-hash invalidation (an
// edit to the TRANSLATION dropping its approval) already worked; this is its
// missing half.

// rewriteSource replaces the whole source catalog.
func rewriteSource(t *testing.T, root, body string) {
	t.Helper()
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"), []byte(body), 0o644))
}

const (
	sourceOriginal = `{"a":"Apple","b":"Banana"}`
	sourceEdited   = `{"a":"Apricot","b":"Banana"}`
)

// TestReviewBasis_RecordedOnEveryDecision: every decision written locally binds
// the source it judged, not only the translation.
func TestReviewBasis_RecordedOnEveryDecision(t *testing.T) {
	root := writeReviewProject(t)
	writeReviewedCorrection(t, root, "Apple", "")

	units := commitAndReadUnits(t, root)
	require.Len(t, units, 1)
	assert.Equal(t, state.SourceHash("Apple"), units[0].ContentHash,
		"the committed shard carries the basis the approval blessed")
	assert.NotEmpty(t, units[0].TargetHash, "and the translation it blessed")
}

// TestReviewBasis_SourceEditWithdrawsTheUnit is the whole-verb regression for
// the reported defect, in both directions: approve, edit the source, and every
// surface that reports or gates must say the unit is stale; restore the source
// and it converges back on the decision already recorded — no second review.
func TestReviewBasis_SourceEditWithdrawsTheUnit(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	writeReviewedCorrection(t, root, "Apple", "")
	writeReviewedCorrection(t, root, "Banana", "")

	converged := func(t *testing.T) *host.ConvergenceReport {
		t.Helper()
		a := &App{}
		defer a.Shutdown()
		rep, err := a.ProjectConvergence(context.Background(), proj, "en")
		require.NoError(t, err)
		return rep
	}
	plan := func(t *testing.T) *host.UpPlanOutput {
		t.Helper()
		a := &App{}
		defer a.Shutdown()
		p, err := a.UpPlan(context.Background(), proj, "en")
		require.NoError(t, err)
		return p
	}

	// Converged: both units reviewed, nothing planned, the locale ships.
	before := converged(t)
	require.Len(t, before.Locales, 1)
	assert.Equal(t, 100, before.Locales[0].Pct["reviewed"])
	assert.Zero(t, before.Locales[0].Stale)
	assert.True(t, before.Locales[0].Shippable)
	assert.Empty(t, before.Review, "nothing is awaiting review")
	assert.Empty(t, plan(t).Scopes, "nothing to do")

	// The source sentence is rewritten. The translation in nb.json is untouched:
	// it still renders the old wording.
	rewriteSource(t, root, sourceEdited)

	after := converged(t)
	require.Len(t, after.Locales, 1)
	assert.Equal(t, 1, after.Locales[0].Stale, "the edited unit's decision blessed source that is gone")
	assert.False(t, after.Locales[0].Shippable, "stale content does not ship")
	assert.False(t, after.Locales[0].Verified)
	assert.Equal(t, 50, after.Locales[0].Pct["translated"],
		"the stale unit reads at draft, below translated — a target exists, but not of this source")
	assert.Equal(t, 50, after.Locales[0].Pct["reviewed"])
	require.Len(t, after.Review, 1, "the stale unit is back in the review queue")
	assert.Equal(t, "Apricot", after.Review[0].Source)

	// The plan no longer reports the project converged, and it prices the
	// re-draft the run will do rather than naming drift it quotes nothing for.
	stalePlan := plan(t)
	require.Len(t, stalePlan.Scopes, 1)
	assert.Equal(t, 1, stalePlan.Totals.Stale)
	assert.Zero(t, stalePlan.Totals.MissingTarget, "every unit still HAS a target — that was never the question")
	assert.Equal(t, 1, stalePlan.Totals.AIRemaining, "the rewritten source is work, and work is priced")
	assert.Positive(t, stalePlan.Totals.TokenEstimate)

	// The picker manifest withholds the locale.
	assert.False(t, host.BuildShipManifest(after.Locales)["nb"].Shippable)

	// The decision itself is untouched: what changed is whether it still
	// describes the project, not the record of who decided what.
	units := commitAndReadUnits(t, root)
	require.Len(t, units, 2)
	for _, u := range units {
		assert.Equal(t, "approved", u.Decision.ReviewState, "a decision is history; staleness is derived")
	}

	// Restore the source. The basis matches again and the unit converges back on
	// the approval already on record — nobody re-reviews anything.
	rewriteSource(t, root, sourceOriginal)

	restored := converged(t)
	assert.Zero(t, restored.Locales[0].Stale)
	assert.Equal(t, 100, restored.Locales[0].Pct["reviewed"])
	assert.True(t, restored.Locales[0].Shippable)
	assert.Empty(t, restored.Review)
	assert.Len(t, commitAndReadUnits(t, root), 2, "no new decision was needed")
}

// The re-draft. Reporting a stale unit and withholding it is only half a loop:
// the other half is producing a translation of the source the project has NOW.
// A catalog entry keeps its key across a source rewrite, so its old translation
// stays sitting beside the new sentence — and the loop read that adjacency back
// out of the committed record as an exact content-memory answer for the new
// wording, recycled it over itself, and reported the drift it had just
// confirmed. These tests hold both directions shut: a rewritten source is
// re-drafted, a restored one is not.

// runReviewUp drives the whole verb — `kapi up` through the exported embedded
// entry point — over the fixture, on the offline demo provider so the AI step
// runs hermetically.
func runReviewUp(t *testing.T, proj string) *host.ConvergeOutput {
	t.Helper()
	a := demoProviderApp(t)
	defer a.Shutdown()
	out, err := a.RunUp(context.Background(), proj, "en", UpOptions{UntilGate: true, MaxPasses: 3})
	require.NoError(t, err, "target-language drift is never a run failure")
	require.NotNil(t, out)
	return out
}

// nbTargets reads the materialized nb catalog.
func nbTargets(t *testing.T, root string) map[string]string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "nb.json"))
	require.NoError(t, err)
	var out map[string]string
	require.NoError(t, json.Unmarshal(data, &out))
	return out
}

// TestReviewBasis_StaleUnitIsRedrafted is the whole-verb regression for the
// defect: approve, rewrite the source, and `kapi up` must produce a translation
// of the new wording — priced, reported, and NOT carrying the old approval —
// with the unit shippable again only once a person reviews the new draft.
func TestReviewBasis_StaleUnitIsRedrafted(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	writeReviewedCorrection(t, root, "Apple", "")
	writeReviewedCorrection(t, root, "Banana", "")

	// A converged run first: it absorbs the committed catalog, so the memory
	// holds the reviewed wording and a second run recycles rather than redrafts.
	// Nothing is stale, so nothing is re-drafted and both approvals survive.
	first := runReviewUp(t, proj)
	assert.Zero(t, first.RedraftedUnits())
	assert.Equal(t, map[string]string{"a": "Eple", "b": "Banan"}, nbTargets(t, root),
		"a converged pass must not overwrite the wording it recycled from the record")

	// The source sentence is rewritten. Its key survives, so the old translation
	// is still sitting beside it in the catalog.
	rewriteSource(t, root, sourceEdited)

	a := &App{}
	defer a.Shutdown()
	priced, err := a.UpPlan(context.Background(), proj, "en")
	require.NoError(t, err)
	assert.Equal(t, 1, priced.Totals.Stale)
	assert.Equal(t, 1, priced.Totals.AIRemaining, "the re-draft is quoted before the tokens burn")

	out := runReviewUp(t, proj)
	assert.Equal(t, 1, out.RedraftedUnits(), "the loop owes the rewritten source a translation")

	after := nbTargets(t, root)
	assert.NotEqual(t, "Eple", after["a"], "the stale target is superseded by a draft of the new source")
	assert.NotEmpty(t, after["a"])
	assert.Equal(t, "Banan", after["b"],
		"the unit whose source did not move keeps the wording its reviewer approved")

	// Producing is not deciding. The unit is back at the presence baseline,
	// un-approved, held out of shipping, and in the queue with the draft a
	// reviewer has to judge.
	require.Len(t, out.Locales, 1)
	assert.Equal(t, 1, out.Locales[0].Stale)
	assert.False(t, out.Locales[0].Shippable)
	assert.False(t, out.Converged)

	b := &App{}
	defer b.Shutdown()
	rep, rerr := b.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, rerr)
	require.Len(t, rep.Review, 1)
	assert.Equal(t, "Apricot", rep.Review[0].Source)
	assert.Equal(t, after["a"], rep.Review[0].Target, "the queue shows the draft that is up for review")

	unit, uerr := b.ReviewUnit(context.Background(), proj, "en", host.ReviewUnitRef{
		File: rep.Review[0].File, Key: rep.Review[0].Key, Locale: "nb",
	})
	require.NoError(t, uerr)
	assert.Empty(t, unit.ReviewState, "a re-draft never inherits the approval the old pairing carried")

	// Review the new draft: the decision records the source it blessed this
	// time, and the locale ships.
	c := &App{}
	defer c.Shutdown()
	changed, aerr := c.ApproveReviewUnit(context.Background(), proj, "en", "nb",
		rep.Review[0].File, rep.Review[0].Key, "reviewed")
	require.NoError(t, aerr)
	require.True(t, changed)

	settled := runReviewUp(t, proj)
	assert.Zero(t, settled.StaleUnits(), "the decision now blesses the source the project has")
	assert.Zero(t, settled.RedraftedUnits(), "and there is nothing left to re-draft")
	assert.True(t, settled.Converged)
	assert.Equal(t, after["a"], nbTargets(t, root)["a"], "the approved draft is what ships")
}

// TestReviewBasis_RestoredSourceManufacturesNoWork is the other direction, and
// the control that stops the fix above from becoming "re-translate on any
// doubt": a source restored to the wording its decision blessed converges on the
// decision already on record. No pass, no provider call, no re-draft.
func TestReviewBasis_RestoredSourceManufacturesNoWork(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	writeReviewedCorrection(t, root, "Apple", "")
	writeReviewedCorrection(t, root, "Banana", "")
	runReviewUp(t, proj)

	rewriteSource(t, root, sourceEdited)
	rewriteSource(t, root, sourceOriginal)

	out := runReviewUp(t, proj)
	assert.Zero(t, out.StaleUnits(), "the basis matches again")
	assert.Zero(t, out.RedraftedUnits(), "so no work was manufactured")
	assert.True(t, out.Converged)
	assert.Equal(t, map[string]string{"a": "Eple", "b": "Banan"}, nbTargets(t, root),
		"the approved wording is untouched")

	units := commitAndReadUnits(t, root)
	require.Len(t, units, 2)
	for _, u := range units {
		assert.Equal(t, "approved", u.Decision.ReviewState, "nobody re-reviewed anything")
	}
}

// TestReviewBasis_UngatedStaleScopeStillRunsAPass: a stale unit tallies at
// `draft`, which is the top of the ungated rung test — so the scope read as
// complete and the loop skipped it entirely. An ungated project is exactly the
// one whose only account of the drift is the loop's own, and it must still work
// on it.
func TestReviewBasis_UngatedStaleScopeStillRunsAPass(t *testing.T) {
	root := writeUngatedReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	writeReviewedCorrection(t, root, "Apple", "")
	runReviewUp(t, proj)

	rewriteSource(t, root, sourceEdited)

	out := runReviewUp(t, proj)
	assert.Equal(t, 1, out.RedraftedUnits(), "no declared bar is not a reason to leave the drift alone")
	assert.NotEqual(t, "Eple", nbTargets(t, root)["a"])
}

// TestReviewBasis_RedraftIsStableAcrossRuns: a stale unit stays stale until a
// person reviews the draft, so every `kapi up` in between fans out over it
// again. Those runs must reproduce the draft, not churn it — the committed
// catalog is what a reviewer reads in a diff, and a unit that rewrote itself on
// every run would make that diff unreadable and the review impossible to finish.
func TestReviewBasis_RedraftIsStableAcrossRuns(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	writeReviewedCorrection(t, root, "Apple", "")
	writeReviewedCorrection(t, root, "Banana", "")
	runReviewUp(t, proj)
	rewriteSource(t, root, sourceEdited)
	runReviewUp(t, proj)
	drafted := nbTargets(t, root)

	for range 2 {
		out := runReviewUp(t, proj)
		assert.Equal(t, 1, out.StaleUnits(), "still stale: nobody has reviewed the draft")
		assert.Equal(t, drafted, nbTargets(t, root), "and the catalog does not move under them")
	}
}

// TestReviewBasis_ShipGateWithholdsStale: `kapi check --ship` fails on a stale
// pairing, and it fails for the reason it is — not as a coverage shortfall.
func TestReviewBasis_ShipGateWithholdsStale(t *testing.T) {
	root := writeReviewProject(t)
	t.Chdir(root)
	writeReviewedCorrection(t, root, "Apple", "")
	writeReviewedCorrection(t, root, "Banana", "")
	rewriteSource(t, root, sourceEdited)

	a := &App{}
	defer a.Shutdown()
	cmd := NewCheckCmd(a)
	require.NoError(t, cmd.Flags().Set("ship", "true"))

	out, runErr := captureStdout(t, func() error { return a.RunCheck(cmd, nil) })
	require.ErrorIs(t, runErr, ErrQualityGate, "a stale pairing exits non-zero")
	assert.Contains(t, out, "stale",
		"the ship gate names staleness, not a coverage shortfall")
	assert.Contains(t, out, "the source changed since the translation was decided")
}

// TestReviewBasis_UngatedScopeIsWithheldToo: a project that declared no coverage
// bar did not thereby agree to ship a translation of a sentence it rewrote —
// and it is exactly the project with nothing else to catch it.
func TestReviewBasis_UngatedScopeIsWithheldToo(t *testing.T) {
	root := writeUngatedReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	writeReviewedCorrection(t, root, "Apple", "")

	a := &App{}
	defer a.Shutdown()
	before, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	require.False(t, before.Locales[0].Gated)
	assert.True(t, before.Locales[0].Shippable)

	rewriteSource(t, root, sourceEdited)

	b := &App{}
	defer b.Shutdown()
	after, err := b.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	assert.Equal(t, 1, after.Locales[0].Stale)
	assert.False(t, after.Locales[0].Shippable, "ungated is not the same as unconditional")
}

// TestReviewBasis_MissingBasisIsUnknownNotStale is the backfill posture. A
// decision recorded before the basis was tracked says nothing about the source
// it blessed. Reading that silence as drift would un-ship every locale of every
// project that already holds decisions, so such a unit keeps its rung and is
// counted instead — visible, and self-clearing on the next decision.
func TestReviewBasis_MissingBasisIsUnknownNotStale(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	seedBasislessApproval(t, root, "a", "Eple")

	a := &App{}
	defer a.Shutdown()
	rep, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	require.Len(t, rep.Locales, 1)
	assert.Zero(t, rep.Locales[0].Stale, "no basis is not drift")
	assert.Equal(t, 1, rep.Locales[0].BasisUnknown, "but the assumption is counted, not silent")
	assert.Equal(t, 50, rep.Locales[0].Pct["reviewed"], "the decision keeps its rung")

	// And it clears itself: deciding the unit again records a basis.
	writeReviewedCorrection(t, root, "Banana", "")
	units := commitAndReadUnits(t, root)
	for _, u := range units {
		if u.Unit == "a" {
			assert.Empty(t, u.ContentHash, "the old record is not rewritten")
		} else {
			assert.NotEmpty(t, u.ContentHash, "the new one carries its basis")
		}
	}
}

// TestReviewBasis_UntranslatedUnitIsNotPromotedByAStaleDecision: staleness
// tallies at `draft`, which is a rung — so it must never be reached by a unit
// that has no target at all. A decision left behind by a deleted translation
// would otherwise promote an untranslated unit into "produced" and count it
// toward the loop's progress metric.
func TestReviewBasis_UntranslatedUnitIsNotPromotedByAStaleDecision(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	writeReviewedCorrection(t, root, "Apple", "")

	// The translation is gone and the source has moved on: a decision remains,
	// with nothing left of the pairing it blessed.
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"), []byte(`{"b":"Banan"}`), 0o644))
	rewriteSource(t, root, sourceEdited)

	a := &App{}
	defer a.Shutdown()
	rep, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	require.Len(t, rep.Locales, 1)
	assert.Zero(t, rep.Locales[0].Stale, "there is no pairing left to be stale about")
	assert.Equal(t, 50, rep.Locales[0].Pct["draft"], "the untranslated unit stays below every rung")
}

// writeUngatedReviewProject is writeReviewProject with no ship_gate at all.
func writeUngatedReviewProject(t *testing.T) string {
	t.Helper()
	t.Setenv("KAPI_NO_PROJECT", "")
	root := t.TempDir()
	recipe := `version: v1
name: rev
defaults:
  source_language: en
  target_languages: [nb]
collections:
  - path: en.json
    target: "{lang}.json"
`
	require.NoError(t, os.WriteFile(filepath.Join(root, "kapi.yaml"), []byte(recipe), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "en.json"), []byte(sourceOriginal), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eple","b":"Banan"}`), 0o644))
	return root
}

// seedBasislessApproval writes a committed approval with no basis — the shape
// every project's record already holds.
func seedBasislessApproval(t *testing.T, root, unit, target string) {
	t.Helper()
	layout := project.Layout{StateDir: filepath.Join(root, project.StateDirName)}
	require.NoError(t, state.WriteCommitted(layout.UnitStateDir(), []state.UnitState{{
		Unit:       unit,
		Variant:    model.Variant("nb"),
		Status:     model.TargetStatusReviewed,
		TargetHash: state.TargetHash(target),
		Decision:   state.Decision{ReviewState: "approved", At: "2026-01-01T00:00:00Z"},
		Updated:    "2026-01-01T00:00:00Z",
		Scope:      "en.json",
	}}))
}
