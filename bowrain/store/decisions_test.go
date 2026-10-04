package store

import (
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// blockWithTarget builds a translatable block carrying an nb target at the
// given rung, the shape a translated block holds after a convergence run.
func blockWithTarget(id, source, target string, status model.TargetStatus) *model.Block {
	b := blockWithText(id, source)
	b.SetTargetText("nb", target)
	b.SetEditionStatus(model.Variant("nb"), model.Status(status))
	return b
}

func listDecisions(t *testing.T, s *PostgresStore, projectID string) map[string]venue.UnitDecision {
	t.Helper()
	got, err := s.ListUnitDecisions(t.Context(), projectID, "main")
	require.NoError(t, err)
	byKey := map[string]venue.UnitDecision{}
	for _, d := range got {
		byKey[d.ItemName+"|"+d.Unit+"|"+d.Variant] = d
	}
	return byKey
}

func targetStatus(t *testing.T, s *PostgresStore, projectID, itemName, unit string) model.TargetStatus {
	t.Helper()
	rows, err := s.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: itemName, Limit: 10,
	})
	require.NoError(t, err)
	for _, sb := range rows {
		if sb.SourceID == unit {
			if tgt, ok := sb.Block.Edition(model.Variant("nb")); ok {
				return model.TargetStatus(tgt.Status)
			}
			return ""
		}
	}
	t.Fatalf("unit %s not found in %s", unit, itemName)
	return ""
}

// TestUnitDecisions_UpsertProjectsAndIsIdempotent covers the ledger's write
// contract: a fresh decision lands and projects its status onto the stored
// target; replaying the identical record changes nothing; a NEWER record
// replaces it and an OLDER replay never rolls it back.
func TestUnitDecisions_UpsertProjectsAndIsIdempotent(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated),
	}))

	decision := venue.UnitDecision{
		ItemName:    "en.json",
		Unit:        "greeting",
		Variant:     "nb",
		Status:      string(model.TargetStatusEstablished),
		Revision:    nbTextRevision("Hei"),
		Basis:       sourceRevision("Hello"),
		ReviewState: "approved",
		DecidedBy:   "reviewer@example.com",
		DecidedAt:   "2026-08-04T10:00:00Z",
		Updated:     "2026-08-04T10:00:00Z",
	}

	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{decision})
	require.NoError(t, err)
	assert.Equal(t, 1, changed)
	assert.Equal(t, model.TargetStatusEstablished, targetStatus(t, s, p.ID, "en.json", "greeting"),
		"a fresh approval must project onto the stored target")

	ledger := listDecisions(t, s, p.ID)
	require.Len(t, ledger, 1)
	got := ledger["en.json|greeting|nb"]
	assert.Equal(t, "reviewer@example.com", got.DecidedBy)
	assert.Equal(t, "approved", got.ReviewState)

	// Identical replay: the idempotency the full-set-every-push wire relies on.
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{decision})
	require.NoError(t, err)
	assert.Zero(t, changed, "an identical record is a no-op")

	// A newer decision (a later approval) replaces it.
	newer := decision
	newer.Status = string(model.TargetStatusEstablished)
	newer.ReviewState = "approved"
	newer.DecidedAt = "2026-08-04T11:00:00Z"
	newer.Updated = "2026-08-04T11:00:00Z"
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{newer})
	require.NoError(t, err)
	assert.Equal(t, 1, changed)
	assert.Equal(t, model.TargetStatusEstablished, targetStatus(t, s, p.ID, "en.json", "greeting"))

	// Replaying the OLD record must not roll the later approval back.
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{decision})
	require.NoError(t, err)
	assert.Zero(t, changed, "an older record never rolls a newer decision back")
	assert.Equal(t, model.TargetStatusEstablished, targetStatus(t, s, p.ID, "en.json", "greeting"))
}

// TestUnitDecisions_StaleOnArrivalDoesNotProject pins the freshness rule: a
// decision blessing a translation the store does not currently hold, or naming
// no source at all, is recorded in the ledger but moves no status.
func TestUnitDecisions_StaleOnArrivalDoesNotProject(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithTarget("greeting", "Hello", "Hei der", model.TargetStatusTranslated),
	}))

	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:    string(model.TargetStatusEstablished),
		Revision:  nbTextRevision("Hei"), // blesses a DIFFERENT translation
		Basis:     sourceRevision("Hello"),
		DecidedBy: "reviewer@example.com",
		Updated:   "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "the fact is recorded")
	assert.Equal(t, model.TargetStatusTranslated, targetStatus(t, s, p.ID, "en.json", "greeting"),
		"a stale-on-arrival decision must not move the status")
	assert.Len(t, listDecisions(t, s, p.ID), 1)

	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:    string(model.TargetStatusEstablished),
		Revision:  nbTextRevision("Hei der"),
		DecidedBy: "reviewer@example.com",
		Updated:   "2026-08-04T11:00:00Z",
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "the fact is recorded")
	assert.Equal(t, model.TargetStatusTranslated, targetStatus(t, s, p.ID, "en.json", "greeting"),
		"a decision that names no source names none the store holds")
}

// TestUnitDecisions_SourceEditDemotesApproval is use case 2 at the store
// level: an approved unit whose SOURCE changes drops back to the presence
// baseline instead of shipping an approval nobody gave for the new text.
func TestUnitDecisions_SourceEditDemotesApproval(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated),
	}))
	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:    string(model.TargetStatusEstablished),
		Revision:  nbTextRevision("Hei"),
		Basis:     sourceRevision("Hello"),
		DecidedBy: "reviewer@example.com",
		Updated:   "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)
	require.Equal(t, model.TargetStatusEstablished, targetStatus(t, s, p.ID, "en.json", "greeting"))

	// The source edit arrives — the same shape a push produces: source only,
	// no targets riding along.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithText("greeting", "Hello there"),
	}))

	assert.Equal(t, model.TargetStatusTranslated, targetStatus(t, s, p.ID, "en.json", "greeting"),
		"an approval must not survive an edit to the source it was made against")

	ledger := listDecisions(t, s, p.ID)
	require.Len(t, ledger, 1)
	assert.Equal(t, "reviewer@example.com", ledger["en.json|greeting|nb"].DecidedBy,
		"the decision stays in the ledger — it is a fact about an older text")
}

// TestUnitDecisions_RestoredSourceFindsItsApproval: the decision is a fact, so
// a source that comes back to the wording it blessed converges on it — the
// projection returns to the decided rung with nobody reviewing anything twice,
// and both flips are in the history.
func TestUnitDecisions_RestoredSourceFindsItsApproval(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated),
	}))
	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:    string(model.TargetStatusEstablished),
		Revision:  nbTextRevision("Hei"),
		Basis:     sourceRevision("Hello"),
		DecidedBy: "reviewer@example.com",
		Updated:   "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)
	require.Equal(t, model.TargetStatusEstablished, targetStatus(t, s, p.ID, "en.json", "greeting"))

	// The source moves: the approval no longer describes the project.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithText("greeting", "Hello there"),
	}))
	require.Equal(t, model.TargetStatusTranslated, targetStatus(t, s, p.ID, "en.json", "greeting"))
	tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, tallies, 1)
	assert.Equal(t, 1, tallies[0].Stale)

	// The source comes back — the recorded decision applies again.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithText("greeting", "Hello"),
	}))
	assert.Equal(t, model.TargetStatusEstablished, targetStatus(t, s, p.ID, "en.json", "greeting"),
		"a restored source converges on the decision already recorded — no re-review")
	tallies, err = s.TallyDecisionBasis(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, tallies, 1)
	assert.Zero(t, tallies[0].Stale, "the basis matches the source again")

	blocks, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main", ItemName: "en.json"})
	require.NoError(t, err)
	require.Len(t, blocks, 1)
	history, err := s.GetBlockHistory(ctx, p.ID, "main", blocks[0].Block.ID, "nb", 50)
	require.NoError(t, err)
	var kinds []string
	for _, h := range history {
		kinds = append(kinds, h.ChangeType)
	}
	assert.Contains(t, kinds, DecisionStaleEvent, "the demotion is auditable")
	assert.Contains(t, kinds, DecisionRestoredEvent, "so is the recovery")
}

// TestTallyDecisionBasis pins the Postgres grading of recorded decisions
// against the source the project holds now — the equality join between a
// decision's recorded basis and the block's source revision.
func TestTallyDecisionBasis(t *testing.T) {
	tests := []struct {
		name string
		// basis is the revision the decision records: "current" for the
		// revision of the stored source, "" for none.
		basis           string
		rewriteSourceTo string
		translatable    bool
		unit            string
		wantStale       int
		wantUnknown     int
		wantOwed        int
	}{
		{name: "basis matches the current source", basis: "current", translatable: true, unit: "greeting"},
		{
			name: "source rewritten under the decision", basis: "current",
			rewriteSourceTo: "Hello there", translatable: true, unit: "greeting", wantStale: 1, wantOwed: 1,
		},
		{name: "a record that names no source", basis: "", translatable: true, unit: "greeting", wantUnknown: 1},
		{
			name: "an unknown basis stays unknown when the source moves", basis: "",
			rewriteSourceTo: "Hello there", translatable: true, unit: "greeting", wantUnknown: 1,
		},
		{name: "decision for a unit this store holds no block for", basis: "current", translatable: true, unit: "absent"},
		{
			name: "a non-translatable block is outside every denominator", basis: "current",
			rewriteSourceTo: "Hello there", translatable: false, unit: "greeting",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := t.Context()
			p := createTestProject(t, s)

			src := blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated)
			src.Translatable = tt.translatable
			require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{src}))

			basis := tt.basis
			if basis == "current" {
				basis = sourceRevision("Hello")
			}
			_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
				ItemName: "en.json", Unit: tt.unit, Variant: "nb",
				Status:   string(model.TargetStatusEstablished),
				Revision: nbTextRevision("Hei"),
				Basis:    basis,
				Updated:  "2026-08-04T10:00:00Z",
			}})
			require.NoError(t, err)

			if tt.rewriteSourceTo != "" {
				edited := blockWithText("greeting", tt.rewriteSourceTo)
				edited.Translatable = tt.translatable
				require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{edited}))
			}

			tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
			require.NoError(t, err)
			if tt.wantStale == 0 && tt.wantUnknown == 0 {
				for _, got := range tallies {
					assert.Zero(t, got.Stale, "nothing is stale")
					assert.Zero(t, got.BasisUnknown, "no basis is unknown")
					assert.Zero(t, got.Owed, "nothing is owed")
				}
				return
			}
			require.Len(t, tallies, 1)
			assert.Equal(t, "en.json", tallies[0].ItemName)
			assert.Equal(t, "nb", tallies[0].Variant)
			assert.Equal(t, tt.wantStale, tallies[0].Stale)
			assert.Equal(t, tt.wantUnknown, tallies[0].BasisUnknown)
			assert.Equal(t, tt.wantOwed, tallies[0].Owed)
		})
	}
}

// TestUnitDecisions_AnInlineCodeRetiresAnApproval: a source whose wording
// stays and whose link moves is another source. The approval made for the old
// link drops to the presence baseline, the tally reads it stale, and the link
// moving back restores it, as a change to the wording does.
func TestUnitDecisions_AnInlineCodeRetiresAnApproval(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	b := linkedBlock("guide", "the guide", "/v1/guide")
	b.SetTargetText("nb", "Les veiledningen")
	b.SetEditionStatus(model.Variant("nb"), model.Status(model.TargetStatusTranslated))
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{b}))
	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "guide", Variant: "nb",
		Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
		Revision: nbTextRevision("Les veiledningen"), Basis: linkedSourceRevision("the guide", "/v1/guide"),
		DecidedBy: "reviewer@example.com", Updated: "2026-10-04T10:00:00Z",
	}})
	require.NoError(t, err)
	require.Equal(t, model.TargetStatusEstablished, targetStatus(t, s, p.ID, "en.json", "guide"))

	moved := linkedBlock("guide", "the guide", "/v2/guide")
	require.Equal(t, b.SourceText(), moved.SourceText(), "the wording is the same")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{moved}))
	assert.Equal(t, model.TargetStatusTranslated, targetStatus(t, s, p.ID, "en.json", "guide"),
		"an approval does not survive a change to a link in the source it was made for")
	tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, tallies, 1)
	assert.Equal(t, 1, tallies[0].Stale)
	assert.Equal(t, 1, tallies[0].Owed, "and the loop owes the unit a draft carrying the new link")

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{linkedBlock("guide", "the guide", "/v1/guide")}))
	assert.Equal(t, model.TargetStatusEstablished, targetStatus(t, s, p.ID, "en.json", "guide"),
		"the link moving back restores the approval")
	tallies, err = s.TallyDecisionBasis(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, tallies, 1)
	assert.Zero(t, tallies[0].Stale)
}

// TestRecordDraftBases pins the draft mark beside the decision: a stale unit is
// owed a draft until the platform stamps the source it drafted against, the
// stamp never touches the decision, a unit the ledger does not hold gets no
// row, a stale decision on a unit with no target is not owed (it is pending as
// untranslated already), and a second source rewrite makes the unit owed again.
func TestRecordDraftBases(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	decided := blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusEstablished)
	undecided := blockWithTarget("farewell", "Goodbye", "Ha det", model.TargetStatusTranslated)
	bare := blockWithText("untranslated", "See you")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{decided, undecided, bare}))

	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{
		{
			ItemName: "en.json", Unit: "greeting", Variant: "nb",
			Status: string(model.TargetStatusEstablished), ReviewState: "approved", DecidedBy: "reviewer-1",
			Revision: nbTextRevision("Hei"), Basis: sourceRevision("Hello"),
			Updated: "2026-08-04T10:00:00Z",
		},
		{
			ItemName: "en.json", Unit: "farewell", Variant: "nb",
			Revision: nbTextRevision("Ha det"), Basis: sourceRevision("Goodbye"),
			Updated: "2026-08-04T10:00:00Z",
		},
		{
			ItemName: "en.json", Unit: "untranslated", Variant: "nb",
			Status: string(model.TargetStatusEstablished), ReviewState: "approved", DecidedBy: "reviewer-1",
			Basis:   sourceRevision("See you"),
			Updated: "2026-08-04T10:00:00Z",
		},
	})
	require.NoError(t, err)

	// Every source moves. All three records read stale; the two units that
	// carry a target are owed a draft.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithText("greeting", "Hello there"),
		blockWithText("farewell", "Goodbye then"),
		blockWithText("untranslated", "See you soon"),
	}))
	tally := func() platstore.DecisionBasisTally {
		t.Helper()
		tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
		require.NoError(t, err)
		require.Len(t, tallies, 1, "one (item, variant) scope")
		return tallies[0]
	}
	got := tally()
	assert.Equal(t, 3, got.Stale)
	assert.Equal(t, 2, got.Owed, "a stale decision on a unit with no target is not owed")

	require.NoError(t, s.RecordDraftBases(ctx, p.ID, "main", []platstore.DraftBasis{
		{ItemName: "en.json", Unit: "greeting", Variant: "nb", Basis: sourceRevision("Hello there")},
		{ItemName: "en.json", Unit: "farewell", Variant: "nb", Basis: sourceRevision("Goodbye then")},
		{ItemName: "en.json", Unit: "absent", Variant: "nb", Basis: sourceRevision("nothing")},
	}))
	got = tally()
	assert.Equal(t, 3, got.Stale, "the decisions stay stale until a person replaces them")
	assert.Zero(t, got.Owed, "drafted against the current source: the loop owes nothing")

	drafts, err := s.ListDraftBases(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, drafts, 2, "a unit the ledger does not hold gets no row")
	assert.Equal(t, platstore.DraftBasis{ItemName: "en.json", Unit: "farewell", Variant: "nb", Basis: sourceRevision("Goodbye then")}, drafts[0])
	assert.Equal(t, platstore.DraftBasis{ItemName: "en.json", Unit: "greeting", Variant: "nb", Basis: sourceRevision("Hello there")}, drafts[1])

	records, err := s.ListUnitDecisions(ctx, p.ID, "main")
	require.NoError(t, err)
	byUnit := map[string]venue.UnitDecision{}
	for _, d := range records {
		byUnit[d.Unit] = d
	}
	require.Len(t, byUnit, 3)
	assert.Equal(t, "approved", byUnit["greeting"].ReviewState, "the stamp never touches the decision")
	assert.Equal(t, "reviewer-1", byUnit["greeting"].DecidedBy)
	assert.Equal(t, sourceRevision("Hello"), byUnit["greeting"].Basis)
	assert.Equal(t, nbTextRevision("Hei"), byUnit["greeting"].Revision)

	// The source moves again, away from the mark: owed once more.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithText("greeting", "Hello again"),
	}))
	got = tally()
	assert.Equal(t, 3, got.Stale)
	assert.Equal(t, 1, got.Owed, "a second rewrite is owed a second draft")
}

// TestUnitDecisions_StaleBasisDoesNotProject: a decision arriving against
// source this store has since rewritten is recorded and projects nothing — the
// approval was for wording the project no longer has.
func TestUnitDecisions_StaleBasisDoesNotProject(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithTarget("greeting", "Hello there", "Hei", model.TargetStatusTranslated),
	}))

	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:    string(model.TargetStatusEstablished),
		Revision:  nbTextRevision("Hei"),
		Basis:     sourceRevision("Hello"), // the wording the reviewer saw
		DecidedBy: "reviewer@example.com",
		Updated:   "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "the fact is recorded")
	assert.Equal(t, model.TargetStatusTranslated, targetStatus(t, s, p.ID, "en.json", "greeting"),
		"a decision blessing source the store has rewritten must not project an approval")
	assert.Len(t, listDecisions(t, s, p.ID), 1)
}

// TestUnitDecisions_ALoweringVerdictNeedsNoBasis: a checkout rejects a
// translation it holds no record of (one written by hand, outside kapi), so the
// rejection names the translation and no source. The rejection lowers the
// translation on the platform as it does on the checkout. A rejection made
// against another source than the block holds lowers nothing, and an approval
// that names no source raises nothing.
func TestUnitDecisions_ALoweringVerdictNeedsNoBasis(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated),
		blockWithTarget("farewell", "Goodbye", "Ha det", model.TargetStatusTranslated),
	}))
	rejection := func(unit, translation, basis, at string) venue.UnitDecision {
		return venue.UnitDecision{
			ItemName: "en.json", Unit: unit, Variant: "nb",
			Status: string(model.TargetStatusDraft), ReviewState: venue.ReviewStateRejected,
			Revision: nbTextRevision(translation), Basis: basis,
			DecidedAt: at, Updated: at,
		}
	}

	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{
		rejection("greeting", "Hei", "", "2026-08-04T10:00:00Z"),
		rejection("farewell", "Ha det", sourceRevision("Goodbye then"), "2026-08-04T10:00:00Z"),
	})
	require.NoError(t, err)
	assert.Equal(t, model.TargetStatusDraft, targetStatus(t, s, p.ID, "en.json", "greeting"),
		"a rejection that names no source lowers the translation it names")
	assert.Equal(t, model.TargetStatusTranslated, targetStatus(t, s, p.ID, "en.json", "farewell"),
		"a rejection of a translation of another source lowers nothing")

	_, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{
		rejection("farewell", "Ha det", sourceRevision("Goodbye"), "2026-08-04T11:00:00Z"),
	})
	require.NoError(t, err)
	assert.Equal(t, model.TargetStatusDraft, targetStatus(t, s, p.ID, "en.json", "farewell"),
		"a rejection of a translation of the source the block holds lowers it")

	_, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
		Revision: nbTextRevision("Hei"), DecidedAt: "2026-08-04T12:00:00Z", Updated: "2026-08-04T12:00:00Z",
	}})
	require.NoError(t, err)
	assert.Equal(t, model.TargetStatusDraft, targetStatus(t, s, p.ID, "en.json", "greeting"),
		"an approval that names no source raises nothing")
}

// TestStoreBlocks_ASameLanguageTranslationLeavesTheSourceRevision: a write
// that carries a block's source with a translation filed under the source
// language, and a later write of the same source without it, stamp one source
// revision. The source did not move, so an approval made against it stays.
func TestStoreBlocks_ASameLanguageTranslationLeavesTheSourceRevision(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	bilingual := blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated)
	bilingual.SetTargetText(model.LocaleEnglish, "Hello there")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{bilingual}))
	stamped := func() string {
		rows, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main", ItemName: "en.json"})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, venue.SourceRevision(rows[0].Block, model.LocaleEnglish), rows[0].SourceRevision,
			"the stamp is the revision of the source the row holds")
		return rows[0].SourceRevision
	}
	first := stamped()
	assert.Equal(t, sourceRevision("Hello"), first)

	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
		Revision: nbTextRevision("Hei"), Basis: first,
		DecidedAt: "2026-08-04T10:00:00Z", Updated: "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)
	require.Equal(t, model.TargetStatusEstablished, targetStatus(t, s, p.ID, "en.json", "greeting"))

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusEstablished),
	}))
	assert.Equal(t, first, stamped(), "the source did not move")
	assert.Equal(t, model.TargetStatusEstablished, targetStatus(t, s, p.ID, "en.json", "greeting"))
	assert.Zero(t, countRows(t, s, "change_log", `project_id=$1 AND change_type='source_modified'`, p.ID),
		"no source change is logged")
}

// TestUpdateProject_SourceLanguageIsFixedOnceItHoldsContent: the store takes
// every source revision under the project's source language, so a project that
// holds a block keeps its language. Every other setting still changes, and a
// project with no content may change its language.
func TestUpdateProject_SourceLanguageIsFixedOnceItHoldsContent(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	p.DefaultSourceLanguage = "en-GB"
	require.NoError(t, s.UpdateProject(ctx, p), "a project with no content may change its language")
	p.DefaultSourceLanguage = model.LocaleEnglish
	require.NoError(t, s.UpdateProject(ctx, p))

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithTarget("greeting", "Hello", "Hei", model.TargetStatusTranslated),
	}))
	p.Name = "Renamed"
	require.NoError(t, s.UpdateProject(ctx, p), "settings other than the language change")

	p.DefaultSourceLanguage = "en-GB"
	err := s.UpdateProject(ctx, p)
	require.ErrorIs(t, err, platstore.ErrSourceLanguageFixed)
	got, err := s.GetProject(ctx, p.ID)
	require.NoError(t, err)
	assert.Equal(t, model.LocaleEnglish, got.DefaultSourceLanguage)
	assert.Equal(t, "Renamed", got.Name)

	missing := *p
	missing.ID = "no-such-project"
	err = s.UpdateProject(ctx, &missing)
	require.Error(t, err)
	assert.NotErrorIs(t, err, platstore.ErrSourceLanguageFixed)
}

// TestUnitDecisions_GoverningFingerprintRoundTrips: the fingerprint of the
// context a decision was made under is stored beside it and read back, a record
// that gains one is a change the store writes, and one made under a moved
// context replaces the one before it. The SQLite store pins the same contract.
func TestUnitDecisions_GoverningFingerprintRoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	legacy := venue.UnitDecision{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status: string(model.TargetStatusEstablished), Revision: nbTextRevision("Hei"),
		ReviewState: "approved", DecidedBy: "reviewer@example.com", Updated: "2026-08-04T10:00:00Z",
	}
	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{legacy})
	require.NoError(t, err)
	require.Equal(t, 1, changed)

	stamped := legacy
	stamped.GoverningFingerprint = "fp-governing"
	stamped.Updated = "2026-08-04T10:30:00Z"
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{stamped})
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "a record that gains a fingerprint is a change")

	got, err := s.GetUnitDecision(ctx, p.ID, "main", "en.json", "greeting", "nb")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "fp-governing", got.GoverningFingerprint)
	assert.Equal(t, "fp-governing", listDecisions(t, s, p.ID)["en.json|greeting|nb"].GoverningFingerprint)

	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{stamped})
	require.NoError(t, err)
	assert.Zero(t, changed, "an identical record is a no-op")

	moved := stamped
	moved.GoverningFingerprint = "fp-moved"
	moved.Updated = "2026-08-04T11:00:00Z"
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{moved})
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "the same verdict under a moved context is a new decision")
	got, err = s.GetUnitDecision(ctx, p.ID, "main", "en.json", "greeting", "nb")
	require.NoError(t, err)
	assert.Equal(t, "fp-moved", got.GoverningFingerprint)
}

// TestTallyDecisionBasis_RejectionOwesADraft pins the count the basis grading
// cannot produce: a reviewer turning down a translation of the source the block
// still carries moves neither hash, so the row reads settled while the unit sits
// at `draft` with a refusal on it (#2564). The rejected count is kept apart from
// the stale one, a rejection on a unit with no target is not owed, the draft
// mark settles it, and clearing the mark (what a second rejection does) owes
// exactly one more draft.
func TestTallyDecisionBasis_RejectionOwesADraft(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithTarget("refused", "Hello", "Hei", model.TargetStatusDraft),
		blockWithTarget("blessed", "Goodbye", "Ha det", model.TargetStatusEstablished),
		blockWithTarget("drifted", "See you", "Vi ses", model.TargetStatusEstablished),
		blockWithText("bare", "Sign out"),
	}))
	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{
		{
			ItemName: "en.json", Unit: "refused", Variant: "nb",
			Status: string(model.TargetStatusDraft), ReviewState: venue.ReviewStateRejected,
			DecidedBy: "reviewer-1", Revision: nbTextRevision("Hei"),
			Basis: sourceRevision("Hello"), Updated: "2026-09-01T10:00:00Z",
		},
		{
			ItemName: "en.json", Unit: "blessed", Variant: "nb",
			Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
			DecidedBy: "reviewer-1", Revision: nbTextRevision("Ha det"),
			Basis: sourceRevision("Goodbye"), Updated: "2026-09-01T10:00:00Z",
		},
		{
			ItemName: "en.json", Unit: "drifted", Variant: "nb",
			Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
			DecidedBy: "reviewer-1", Revision: nbTextRevision("Vi ses"),
			Basis: sourceRevision("See you"), Updated: "2026-09-01T10:00:00Z",
		},
		{
			ItemName: "en.json", Unit: "bare", Variant: "nb",
			Status: string(model.TargetStatusDraft), ReviewState: venue.ReviewStateRejected,
			DecidedBy: "reviewer-1", Basis: sourceRevision("Sign out"),
			Updated: "2026-09-01T10:00:00Z",
		},
	})
	require.NoError(t, err)

	// The drifted unit's source is rewritten and the reviewer turns the old
	// translation down, which carries the approval's basis forward: a unit that
	// is stale AND rejected, and must be counted once.
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		blockWithText("drifted", "See you soon"),
	}))
	_, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "drifted", Variant: "nb",
		Status: string(model.TargetStatusDraft), ReviewState: venue.ReviewStateRejected,
		DecidedBy: "reviewer-1", Revision: nbTextRevision("Vi ses"),
		Basis: sourceRevision("See you"), Updated: "2026-09-01T11:00:00Z",
	}})
	require.NoError(t, err)

	tally := func() platstore.DecisionBasisTally {
		t.Helper()
		tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
		require.NoError(t, err)
		require.Len(t, tallies, 1, "one (item, variant) scope")
		return tallies[0]
	}
	got := tally()
	assert.Equal(t, 1, got.Stale, "only the rewritten source is stale")
	assert.Equal(t, 1, got.Owed, "and it is owed a draft")
	assert.Equal(t, 1, got.RejectedOwed,
		"the rejection on an unmoved source is owed a draft no grading of the basis can see")
	assert.LessOrEqual(t, got.Owed, got.Stale, "Owed stays a subset of Stale")

	require.NoError(t, s.RecordDraftBases(ctx, p.ID, "main", []platstore.DraftBasis{
		{ItemName: "en.json", Unit: "refused", Variant: "nb", Basis: sourceRevision("Hello")},
	}))
	got = tally()
	assert.Zero(t, got.RejectedOwed, "drafted since the rejection: the unit waits on the next verdict")
	assert.Equal(t, 1, got.Owed, "the stale unit is untouched by another unit's mark")

	require.NoError(t, s.RecordDraftBases(ctx, p.ID, "main", []platstore.DraftBasis{
		{ItemName: "en.json", Unit: "drifted", Variant: "nb", Basis: sourceRevision("See you soon")},
	}))
	got = tally()
	assert.Equal(t, 1, got.Stale, "the withdrawn approval is still a person's to replace")
	assert.Zero(t, got.Owed)
	assert.Zero(t, got.RejectedOwed, "a unit counted as stale is never counted as rejected too")

	// A second rejection clears the mark again, which is the whole of the
	// re-draft guard: one more draft, and one only.
	require.NoError(t, s.RecordDraftBases(ctx, p.ID, "main", []platstore.DraftBasis{
		{ItemName: "en.json", Unit: "refused", Variant: "nb"},
	}))
	assert.Equal(t, 1, tally().RejectedOwed, "a second rejection owes a second draft")
}

// TestUnitDecisions_RevisionPairingRoundTrips: a decision carries the revision
// of the translation it blesses and of the source it blessed it for. The
// ledger stores both and reads them back, an identical record is a no-op, and
// a verdict on a translation that differs in an inline code alone replaces it.
// The SQLite store pins the same contract.
func TestUnitDecisions_RevisionPairingRoundTrips(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	paired := venue.UnitDecision{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status: string(model.TargetStatusEstablished), Revision: "r:1111111111111111", Basis: "r:aaaaaaaaaaaaaaaa",
		ReviewState: "approved", DecidedBy: "reviewer@example.com", Updated: "2026-08-04T10:00:00Z",
	}
	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{paired})
	require.NoError(t, err)
	require.Equal(t, 1, changed)
	got, err := s.GetUnitDecision(ctx, p.ID, "main", "en.json", "greeting", "nb")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "r:1111111111111111", got.Revision)
	assert.Equal(t, "r:aaaaaaaaaaaaaaaa", got.Basis)
	listed := listDecisions(t, s, p.ID)["en.json|greeting|nb"]
	assert.Equal(t, "r:1111111111111111", listed.Revision)
	assert.Equal(t, "r:aaaaaaaaaaaaaaaa", listed.Basis)

	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{paired})
	require.NoError(t, err)
	assert.Zero(t, changed, "an identical record is a no-op")

	recoded := paired
	recoded.Revision = "r:2222222222222222"
	recoded.Updated = "2026-08-04T11:00:00Z"
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{recoded})
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "a verdict on other content is a change")
	got, err = s.GetUnitDecision(ctx, p.ID, "main", "en.json", "greeting", "nb")
	require.NoError(t, err)
	assert.Equal(t, "r:2222222222222222", got.Revision)

	rebased := recoded
	rebased.Basis = "r:bbbbbbbbbbbbbbbb"
	rebased.Updated = "2026-08-04T12:00:00Z"
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{rebased})
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "a verdict for another source is a change")
}
