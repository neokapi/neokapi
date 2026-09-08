package server

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
)

// What the platform owes a unit a reviewer turned down.
//
// Rejecting a translation of the source the block still carries moves neither
// hash, so a reader that graded the basis alone reported the unit as settled
// while it sat at `draft` with a refusal on it, waiting for somebody to edit
// the source before the loop would look at it again (#2564). The verdict is
// read beside the basis, the count is kept apart from the stale one, and the
// draft mark buys exactly one draft per verdict.

// seedRejectedUnit stores two translated fr blocks under an item and records a
// reviewer's rejection on one of them, against the source the block still
// holds. Nothing is rewritten, which is the whole shape of the case.
func seedRejectedUnit(t *testing.T, cs *sqlitestore.SQLiteStore, projectID string) {
	t.Helper()
	ctx := t.Context()
	require.NoError(t, cs.StoreItem(ctx, projectID, "main", &platstore.Item{
		Name: "app.json", Format: "json", ItemType: "file",
	}))
	refused := model.NewBlock("greeting", "Hello")
	refused.SetTargetText(model.LocaleFrench, "Bonjour")
	kept := model.NewBlock("farewell", "Goodbye")
	kept.SetTargetText(model.LocaleFrench, "Au revoir")
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "app.json",
		[]*model.Block{refused, kept}))

	_, err := cs.UpsertUnitDecisions(ctx, projectID, "main", []venue.UnitDecision{{
		ItemName: "app.json", Unit: "greeting", Variant: string(model.LocaleFrench),
		Status: string(model.TargetStatusDraft), ReviewState: venue.ReviewStateRejected,
		DecidedBy:  "reviewer-1",
		TargetHash: state.TargetHash("Bonjour"), ContentHash: state.SourceHash("Hello"),
		Updated: "2026-09-01T00:00:00Z",
	}})
	require.NoError(t, err)
}

// TestDerive_RejectedTargetIsPending: a target a reviewer turned down is
// withheld from the produced count, so the locale is pending on production
// rather than reading fully covered over wording somebody refused. The derive,
// the ledger's grouped tally and the dashboard stats read the same number, and
// the rejected count sits beside the stale split rather than inside it.
func TestDerive_RejectedTargetIsPending(t *testing.T) {
	s, cs, _, p := newDeriveHarness(t, []model.LocaleID{model.LocaleFrench})
	ctx := t.Context()
	seedRejectedUnit(t, cs, p.ID)
	fr := string(model.LocaleFrench)

	st, err := s.convergence.deriveFunc(p.ID, "main", nil)(ctx)
	require.NoError(t, err)
	assert.Equal(t, []string{fr}, st.Pending, "a refused target is pending work, exactly as a missing one")
	assert.Equal(t, 2, st.UnitTotals[fr])
	assert.Equal(t, 1, st.Produced, "the refused unit is withheld from the produced count")

	basis, err := tallyDecisionBasis(ctx, cs, p.ID, "main", nil)
	require.NoError(t, err)
	counts := basis.forLocale(fr)
	assert.Zero(t, counts.Stale, "nothing rewrote the source, so nothing is stale")
	assert.Zero(t, counts.Owed)
	assert.Equal(t, 1, counts.RejectedOwed)
	assert.Equal(t, st.UnitTotals[fr]-st.Produced, counts.owed(),
		"the derive withholds exactly the units the ledger says are owed")

	stats, err := editorGetDashboardStats(ctx, cs, p, "main")
	require.NoError(t, err)
	require.NoError(t, applyShipStates(ctx, cs, nil, p.ID, "main", nil, stats))
	require.Len(t, stats.LocaleStats, 1)
	ls := stats.LocaleStats[0]
	assert.Equal(t, 1, ls.RejectedAwaitingDraftBlocks)
	assert.Zero(t, ls.StaleBlocks, "a rejection is not drift")
	assert.Equal(t, ls.StaleBlocks, ls.StaleAwaitingDraftBlocks+ls.StaleAwaitingReviewBlocks,
		"the stale split still sums to the stale count")
	assert.Equal(t, platstore.ShipStatePending, ls.ShipState,
		"a locale holding wording a person refused does not ship")
}

// TestOrchestrator_RejectionRedraftsOnce drives a full run over a project with
// one refused unit, with the REAL derive and a Produce that writes what the
// translation worker writes: the draft and the mark of the source it drafted
// against. The run produces once and converges; the rejection stays on the row;
// a second run finds nothing to produce; and a second rejection owes one more
// draft and no more than one.
func TestOrchestrator_RejectionRedraftsOnce(t *testing.T) {
	s, cs, runStore, p := newDeriveHarness(t, []model.LocaleID{model.LocaleFrench})
	seedRejectedUnit(t, cs, p.ID)
	ctx := context.Background()
	fr := string(model.LocaleFrench)

	produceCalls := 0
	var drafted []string
	produce := func(ctx context.Context, locale string, pass int, emit *convergence.Emitter) (convergence.PassProduction, error) {
		produceCalls++
		// The worker's own partition, read from the store's marks: a unit is
		// owed a draft when it has no target, or when the platform has not
		// drafted it against the current source since the row's basis went
		// stale or a reviewer turned it down (jobs.decisionLedger.needsDraft).
		type unitKey struct{ item, unit, variant string }
		records := map[unitKey]venue.UnitDecision{}
		list, err := cs.ListUnitDecisions(ctx, p.ID, "main")
		if err != nil {
			return convergence.PassProduction{}, err
		}
		for _, d := range list {
			records[unitKey{d.ItemName, d.Unit, d.Variant}] = d
		}
		marks := map[unitKey]string{}
		bases, err := cs.ListDraftBases(ctx, p.ID, "main")
		if err != nil {
			return convergence.PassProduction{}, err
		}
		for _, d := range bases {
			marks[unitKey{d.ItemName, d.Unit, d.Variant}] = d.SourceHash
		}
		blocks, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main"})
		if err != nil {
			return convergence.PassProduction{}, err
		}
		var toStore []*model.Block
		var stamps []platstore.DraftBasis
		for _, sb := range blocks {
			if sb.Block == nil || !sb.Block.Translatable {
				continue
			}
			key := unitKey{sb.ItemName, sb.SourceID, locale}
			rec, recorded := records[key]
			owed := !sb.Block.HasTarget(model.LocaleID(locale))
			if !owed && recorded && marks[key] != sb.ContentHash {
				owed = rec.ReviewState == venue.ReviewStateRejected ||
					(rec.ContentHash != "" && rec.ContentHash != sb.ContentHash)
			}
			if !owed {
				continue
			}
			sb.Block.SetTargetText(model.LocaleID(locale), "Bonjour à vous")
			toStore = append(toStore, sb.Block)
			drafted = append(drafted, sb.Block.SourceText())
			stamps = append(stamps, platstore.DraftBasis{
				ItemName: sb.ItemName, Unit: sb.SourceID, Variant: locale, SourceHash: sb.ContentHash,
			})
		}
		if len(toStore) == 0 {
			return convergence.PassProduction{}, nil
		}
		if err := cs.StoreBlocks(ctx, p.ID, "main", toStore); err != nil {
			return convergence.PassProduction{}, err
		}
		if err := cs.RecordDraftBases(ctx, p.ID, "main", stamps); err != nil {
			return convergence.PassProduction{}, err
		}
		return convergence.PassProduction{Done: len(toStore), ViaAI: len(toStore)}, nil
	}
	drive := func() *bstore.ConvergenceRun {
		t.Helper()
		run := &bstore.ConvergenceRun{ProjectID: p.ID, Trigger: "review", State: bstore.ConvergenceRunRunning}
		require.NoError(t, runStore.CreateRun(ctx, run))
		s.convergence.driveWith(ctx, run, convergence.LoopFuncs{
			Derive:  s.convergence.deriveFunc(p.ID, "main", nil),
			Produce: produce,
		})
		got, err := runStore.GetRun(ctx, run.ID)
		require.NoError(t, err)
		return got
	}

	first := drive()
	assert.Equal(t, bstore.ConvergenceRunConverged, first.State)
	assert.Equal(t, 1, first.Passes, "the refused unit is pending work, so the run produces")
	assert.Equal(t, []string{"Hello"}, drafted, "only the refused unit is drafted")

	records, err := cs.ListUnitDecisions(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, records, 1)
	assert.Equal(t, venue.ReviewStateRejected, records[0].ReviewState, "the verdict is never written over")
	assert.Equal(t, state.SourceHash("Hello"), records[0].ContentHash)

	basis, err := tallyDecisionBasis(ctx, cs, p.ID, "main", nil)
	require.NoError(t, err)
	assert.Zero(t, basis.forLocale(fr).owed(), "drafted since the rejection")

	second := drive()
	assert.Equal(t, bstore.ConvergenceRunConverged, second.State)
	assert.Zero(t, second.Passes, "the unit waits on the next verdict, not on another pass")
	assert.Equal(t, 1, produceCalls)

	// A second rejection, which clears the mark, and one more draft for it.
	blocks, err := cs.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main"})
	require.NoError(t, err)
	var sb *venue.StoredBlock
	for _, b := range blocks {
		if b.SourceID == "greeting" {
			sb = b
		}
	}
	require.NotNil(t, sb)
	ledger := &reviewLedger{srv: s, ds: cs, projectID: p.ID, stream: "main",
		decider: "reviewer@example.com", governing: map[string]string{}}
	prev := s.unitDecisionFor(ctx, p.ID, "main", sb, fr)
	ledger.write(ctx, []venue.UnitDecision{unitDecisionFor(
		sb, fr, model.TargetStatusDraft, false, ledger.decider, "fp-now", prev)})
	ledger.clearDraftBasis(ctx, sb, fr)

	basis, err = tallyDecisionBasis(ctx, cs, p.ID, "main", nil)
	require.NoError(t, err)
	assert.Equal(t, 1, basis.forLocale(fr).RejectedOwed, "the second rejection owes a second draft")

	third := drive()
	assert.Equal(t, 1, third.Passes)
	assert.Equal(t, 2, produceCalls)
	fourth := drive()
	assert.Zero(t, fourth.Passes, "and one draft only")
	assert.Equal(t, 2, produceCalls)
}
