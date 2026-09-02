package cli

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/host"
)

// Source drift under a translation NOBODY DECIDED.
//
// A decision carries the source it blessed, so #1966 could derive a rewrite
// under a decided translation and re-draft it. An undecided translation carried
// nothing. Where the loop healed the drift at all it healed it by the project
// block store remembering the previous read, which is derived and not in git —
// so on a fresh checkout the record absorber read the old translation as a pair
// with the new sentence, learned it, recycled it back, and reported the project
// caught up.
//
// The loop now records a basis for every unit it writes a target for, so the
// rewrite is derived from a committed record and healed on any machine.

// dropDerivedState removes everything under .kapi/ that is rebuilt rather than
// committed, which is what a CI checkout has.
func dropDerivedState(t *testing.T, root string) {
	t.Helper()
	require.NoError(t, os.RemoveAll(filepath.Join(root, ".kapi", "work")))
}

// committedBasis reads the committed state record for one unit and locale.
func committedBasis(t *testing.T, root, unit string) (state.UnitState, bool) {
	t.Helper()
	for _, u := range commitAndReadUnits(t, root) {
		if u.Unit == unit && string(u.Variant.Locale) == "nb" {
			return u, true
		}
	}
	return state.UnitState{}, false
}

// TestSourceDrift_UndecidedUnitIsRedrafted is the whole verb: converge, rewrite
// a source sentence nobody had decided, and the next run must price it, report
// it, and produce a translation of the wording the project has now.
func TestSourceDrift_UndecidedUnitIsRedrafted(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")

	first := runReviewUp(t, proj)
	assert.Zero(t, first.RedraftedUnits(), "nothing drifted yet")
	assert.Equal(t, map[string]string{"a": "Eple", "b": "Banan"}, nbTargets(t, root),
		"a converged pass reproduces the wording the record already holds")

	// The loop recorded what it translated, with no decision on it.
	basis, ok := committedBasis(t, root, "a")
	require.True(t, ok, "the run records the basis of every translation it writes")
	assert.Equal(t, state.SourceHash("Apple"), basis.ContentHash)
	assert.Equal(t, state.TargetHash("Eple"), basis.TargetHash)
	assert.Empty(t, basis.Decision.ReviewState, "producing is not deciding")

	rewriteSource(t, root, sourceEdited)

	a := &App{}
	defer a.Shutdown()
	priced, err := a.UpPlan(context.Background(), proj, "en")
	require.NoError(t, err)
	assert.Equal(t, 1, priced.Totals.Stale, "the rewrite is drift, and the plan owes the reader a number for it")
	assert.Equal(t, 1, priced.Totals.AIRemaining, "and a price, before the tokens burn")

	out := runReviewUp(t, proj)
	assert.Equal(t, 1, out.RedraftedUnits(), "the loop owes the rewritten source a translation")

	after := nbTargets(t, root)
	assert.NotEqual(t, "Eple", after["a"], "the drifted target is superseded by a draft of the new source")
	assert.NotEmpty(t, after["a"])
	assert.Equal(t, "Banan", after["b"], "the unit whose source did not move is left alone")

	// Nothing was decided, so nothing stays withheld: the re-draft settles it.
	redrafted, ok := committedBasis(t, root, "a")
	require.True(t, ok)
	assert.Equal(t, state.SourceHash("Apricot"), redrafted.ContentHash)
	assert.Equal(t, state.TargetHash(after["a"]), redrafted.TargetHash)
}

// TestSourceDrift_SurvivesAFreshCheckout is the same rewrite on a checkout with
// no derived state, which is where the loop runs unattended. It is the shape the
// defect was worst in: with nothing remembering what the committed translation
// translated, the absorber paired it with the sentence beside it and the loop
// recycled the drift back over itself, run after run.
func TestSourceDrift_SurvivesAFreshCheckout(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")

	runReviewUp(t, proj)
	require.Equal(t, "Eple", nbTargets(t, root)["a"])

	rewriteSource(t, root, sourceEdited)
	dropDerivedState(t, root)

	out := runReviewUp(t, proj)
	assert.Equal(t, 1, out.RedraftedUnits(),
		"the committed basis is what the loop reads on a machine that has run nothing")
	assert.NotEqual(t, "Eple", nbTargets(t, root)["a"])
}

// TestSourceDrift_UnknownBasisIsNeverRedrafted is the boundary. A translation
// that was in the tree before kapi ever ran has no record of what it translates,
// and the loop must not manufacture one by reading it: nothing calls it stale
// and nothing re-drafts it, whatever happens to the source.
func TestSourceDrift_UnknownBasisIsNeverRedrafted(t *testing.T) {
	root := writeUngatedReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")

	// The source is rewritten before any run: nb.json holds a hand-written
	// translation of a sentence the project no longer has, and no record
	// anywhere says so.
	rewriteSource(t, root, sourceEdited)

	a := &App{}
	defer a.Shutdown()
	rep, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	require.Len(t, rep.Locales, 1)
	assert.Zero(t, rep.Locales[0].Stale,
		"nothing recorded what this translation translates, so nothing can call it stale")

	plan, perr := a.UpPlan(context.Background(), proj, "en")
	require.NoError(t, perr)
	assert.Zero(t, plan.Totals.Stale)

	_, ok := committedBasis(t, root, "a")
	assert.False(t, ok, "reading a translation is not a reason to claim authorship of it")
}

// TestSourceDrift_HandEditedTargetIsLeftAlone: the loop wrote the translation,
// then a person rewrote it. The record's target half no longer describes what is
// on disk, so the record stops speaking for the unit — a later source edit
// leaves the person's wording where they put it rather than drafting over it.
func TestSourceDrift_HandEditedTargetIsLeftAlone(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	runReviewUp(t, proj)

	basis, ok := committedBasis(t, root, "a")
	require.True(t, ok)
	require.Equal(t, state.TargetHash("Eple"), basis.TargetHash)

	// A person rewrites the translation in the committed catalog, then the
	// source moves under it.
	require.NoError(t, os.WriteFile(filepath.Join(root, "nb.json"),
		[]byte(`{"a":"Eplet mitt","b":"Banan"}`), 0o644))
	rewriteSource(t, root, sourceEdited)

	a := &App{}
	defer a.Shutdown()
	rep, err := a.ProjectConvergence(context.Background(), proj, "en")
	require.NoError(t, err)
	require.Len(t, rep.Locales, 1)
	assert.Zero(t, rep.Locales[0].Stale, "the record describes a translation nobody kept")

	out := runReviewUp(t, proj)
	assert.Zero(t, out.RedraftedUnits())
	assert.Equal(t, "Eplet mitt", nbTargets(t, root)["a"],
		"an edit to the translation is a basis change the loop respects")
}

// TestSourceDrift_DecidedUnitStillBehavesAsBefore is the #1966 regression guard.
// A decided unit's basis is the decision's: the rewrite re-drafts it, the
// approval is NOT restored, and the scope stays withheld until somebody reviews
// the new pairing.
func TestSourceDrift_DecidedUnitStillBehavesAsBefore(t *testing.T) {
	root := writeReviewProject(t)
	proj := filepath.Join(root, "kapi.yaml")
	writeReviewedCorrection(t, root, "Apple", "")
	runReviewUp(t, proj)

	decided, ok := committedBasis(t, root, "a")
	require.True(t, ok)
	require.Equal(t, "approved", decided.Decision.ReviewState)
	require.Equal(t, state.SourceHash("Apple"), decided.ContentHash)

	rewriteSource(t, root, sourceEdited)
	out := runReviewUp(t, proj)

	assert.Equal(t, 1, out.RedraftedUnits())
	assert.Equal(t, 1, out.StaleUnits(), "a re-draft is not a decision")
	assert.False(t, out.Converged)
	assert.NotEqual(t, "Eple", nbTargets(t, root)["a"])

	// The decision is history and the run never rewrote it: what changed is
	// whether it still describes the project.
	after, ok := committedBasis(t, root, "a")
	require.True(t, ok)
	assert.Equal(t, "approved", after.Decision.ReviewState)
	assert.Equal(t, state.SourceHash("Apple"), after.ContentHash,
		"the loop must never overwrite the basis a decision blessed")

	b := &App{}
	defer b.Shutdown()
	unit, uerr := b.ReviewUnit(context.Background(), proj, "en", host.ReviewUnitRef{
		File: "nb.json", Key: "a", Locale: "nb",
	})
	require.NoError(t, uerr)
	assert.Empty(t, unit.ReviewState, "a re-draft never inherits the approval the old pairing carried")
}
