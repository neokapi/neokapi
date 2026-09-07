package server

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
)

// The basis a platform verdict re-stamps.
//
// A ledger row's basis is the source it vouches for and the context it vouches
// for it under, and the grading reads it to answer whether the unit still
// translates the wording the project holds. An approval made on the source in
// front of the reviewer re-stamps it; a rejection carries the row's basis
// forward and clears the platform's draft mark, so the unit stays stale and is
// owed a draft again rather than waiting on a review that is already in.

// restampFixture is the derive harness with one decided unit whose source has
// since been rewritten and whose re-draft the worker has already marked.
type restampFixture struct {
	srv    *Server
	cs     *sqlitestore.SQLiteStore
	proj   *platstore.Project
	ledger *reviewLedger
	block  *venue.StoredBlock
	// variant is the ledger's spelling of the locale under test.
	variant string
	// rewritten is the source wording the block holds now.
	rewritten string
}

func newRestampFixture(t *testing.T) *restampFixture {
	t.Helper()
	s, cs, _, p := newDeriveHarness(t, []model.LocaleID{model.LocaleFrench})
	rewritten := seedStaleUnit(t, cs, p.ID, true)
	variant := string(model.LocaleFrench)

	// The worker re-drafts a stale decided unit once per source change and
	// marks what it drafted against, which is what leaves the unit waiting on
	// a reviewer rather than on another pass.
	require.NoError(t, cs.RecordDraftBases(t.Context(), p.ID, "main", []platstore.DraftBasis{{
		ItemName: "app.json", Unit: "greeting", Variant: variant,
		SourceHash: state.SourceHash(rewritten),
	}}))

	blocks, err := cs.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: p.ID, Stream: "main", ItemName: "app.json",
	})
	require.NoError(t, err)
	var sb *venue.StoredBlock
	for _, b := range blocks {
		if b.SourceID == "greeting" {
			sb = b
		}
	}
	require.NotNil(t, sb, "the stale unit must be readable as a stored block")
	require.Equal(t, rewritten, sb.Block.SourceText())

	return &restampFixture{
		srv: s, cs: cs, proj: p, block: sb, variant: variant, rewritten: rewritten,
		ledger: &reviewLedger{
			srv: s, ds: cs, projectID: p.ID, stream: "main",
			decider: "reviewer@example.com", governing: map[string]string{},
		},
	}
}

// counts grades the ledger the way the dashboard and the derive do.
func (f *restampFixture) counts(t *testing.T) basisCounts {
	t.Helper()
	basis, err := tallyDecisionBasis(t.Context(), f.cs, f.proj.ID, "main", nil)
	require.NoError(t, err)
	return basis.forLocale(string(model.LocaleFrench))
}

// row reads the unit's ledger row back.
func (f *restampFixture) row(t *testing.T) *venue.UnitDecision {
	t.Helper()
	d := f.srv.unitDecisionFor(t.Context(), f.proj.ID, "main", f.block, f.variant)
	require.NotNil(t, d, "the unit must hold a ledger row")
	return d
}

// TestPlatformRejectionKeepsTheBasisAndOwesADraft: the verdict lands, the basis
// does not move, and the unit is owed a draft again.
func TestPlatformRejectionKeepsTheBasisAndOwesADraft(t *testing.T) {
	f := newRestampFixture(t)
	before := f.row(t)
	require.Equal(t, "approved", before.ReviewState)

	c := f.counts(t)
	require.Equal(t, 1, c.Stale, "the source moved out from under the approval")
	require.Zero(t, c.Owed, "the worker has already drafted against the source it holds now")

	f.ledger.write(t.Context(), []venue.UnitDecision{unitDecisionFor(
		f.block, f.variant, model.TargetStatusDraft, false, f.ledger.decider, "fp-now", before)})
	f.ledger.clearDraftBasis(t.Context(), f.block, f.variant)

	after := f.row(t)
	assert.Equal(t, "rejected", after.ReviewState)
	assert.Equal(t, before.ContentHash, after.ContentHash,
		"a rejection endorses nothing, so the basis stays where the approval left it")
	assert.Equal(t, before.GoverningFingerprint, after.GoverningFingerprint)

	c = f.counts(t)
	assert.Equal(t, 1, c.Stale, "the rejection leaves the unit stale")
	assert.Equal(t, 1, c.Owed, "and owed a draft, because the one it turned down will not do")
}

// TestPlatformApprovalRestampsTheBasis: the re-approval is the decision the
// basis records, and the unit stops reading stale.
func TestPlatformApprovalRestampsTheBasis(t *testing.T) {
	f := newRestampFixture(t)
	require.Equal(t, 1, f.counts(t).Stale)

	f.ledger.write(t.Context(), []venue.UnitDecision{unitDecisionFor(
		f.block, f.variant, model.TargetStatusReviewed, true, f.ledger.decider, "fp-now", nil)})

	after := f.row(t)
	assert.Equal(t, "approved", after.ReviewState)
	assert.Equal(t, state.SourceHash(f.rewritten), after.ContentHash,
		"the approval binds the source the reviewer read")
	assert.Equal(t, "fp-now", after.GoverningFingerprint,
		"and the context they decided under")

	c := f.counts(t)
	assert.Zero(t, c.Stale, "the re-approval clears the stale grading")
	assert.Zero(t, c.Owed)
}

// TestPlatformUnReviewKeepsTheBasis: withdrawing an approval is not an
// endorsement either, so the row's basis survives it and the unit reads exactly
// as it did before the click.
func TestPlatformUnReviewKeepsTheBasis(t *testing.T) {
	f := newRestampFixture(t)
	before := f.row(t)

	f.ledger.write(t.Context(), []venue.UnitDecision{unitDecisionFor(
		f.block, f.variant, model.TargetStatusTranslated, false, f.ledger.decider, "fp-now", before)})

	after := f.row(t)
	assert.Empty(t, after.ReviewState, "an un-review is no verdict")
	assert.Equal(t, before.ContentHash, after.ContentHash)
	assert.Equal(t, before.GoverningFingerprint, after.GoverningFingerprint)
	assert.Equal(t, 1, f.counts(t).Stale)
}

// TestPlatformVerdictOnAnUnrecordedUnitCarriesNoBasis: a rejection of a unit
// the ledger has never held claims no basis rather than inventing one from the
// source in front of it.
func TestPlatformVerdictOnAnUnrecordedUnitCarriesNoBasis(t *testing.T) {
	f := newRestampFixture(t)
	d := unitDecisionFor(f.block, f.variant, model.TargetStatusDraft, false, f.ledger.decider, "fp-now", nil)
	assert.Empty(t, d.ContentHash)
	assert.Empty(t, d.GoverningFingerprint)
	assert.Equal(t, "rejected", d.ReviewState)
	assert.Equal(t, state.TargetHash("Bonjour"), d.TargetHash,
		"the translation it turned down is the rejection's own")
}

// TestDashboardStaleSplitSumsToTheStaleCount: the dashboard stats carry what a
// stale pair is waiting on, so a reader is sent to the loop or to a reviewer
// rather than to whichever the total happens to suggest.
func TestDashboardStaleSplitSumsToTheStaleCount(t *testing.T) {
	_, cs, _, p := newDeriveHarness(t, []model.LocaleID{model.LocaleFrench})
	ctx := t.Context()
	rewritten := seedStaleUnit(t, cs, p.ID, true)

	stats := func() platstore.LocaleTranslationStats {
		t.Helper()
		st, err := editorGetDashboardStats(ctx, cs, p, "main")
		require.NoError(t, err)
		require.NoError(t, applyShipStates(ctx, cs, nil, p.ID, "main", nil, st))
		require.Len(t, st.LocaleStats, 1)
		return st.LocaleStats[0]
	}

	owed := stats()
	require.Equal(t, 1, owed.StaleBlocks)
	assert.Equal(t, 1, owed.StaleAwaitingDraftBlocks, "nothing has translated the wording the block holds now")
	assert.Zero(t, owed.StaleAwaitingReviewBlocks)
	assert.Equal(t, owed.StaleBlocks, owed.StaleAwaitingDraftBlocks+owed.StaleAwaitingReviewBlocks)

	// The worker drafts the unit against the source the block holds now and
	// marks what it drafted against. The decision is not its to replace, so the
	// pair stays stale and starts waiting on a person.
	require.NoError(t, cs.RecordDraftBases(ctx, p.ID, "main", []platstore.DraftBasis{{
		ItemName: "app.json", Unit: "greeting", Variant: string(model.LocaleFrench),
		SourceHash: state.SourceHash(rewritten),
	}}))

	drafted := stats()
	require.Equal(t, 1, drafted.StaleBlocks)
	assert.Zero(t, drafted.StaleAwaitingDraftBlocks)
	assert.Equal(t, 1, drafted.StaleAwaitingReviewBlocks, "the draft is in and the reviewer is who it waits on")
	assert.Equal(t, drafted.StaleBlocks, drafted.StaleAwaitingDraftBlocks+drafted.StaleAwaitingReviewBlocks)
}
