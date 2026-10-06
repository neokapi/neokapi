package sqlitestore

import (
	"path/filepath"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// nbTextRevision is the revision of a plain-text Norwegian translation: the
// revision a decision on it names.
func nbTextRevision(text string) string {
	return model.RunsRevision(model.Variant("nb"), []model.Run{model.TextR(text)})
}

// sourceRevision is the revision of a plain-text source as the store stamps it
// for a project written in English (createTestProject).
func sourceRevision(text string) string {
	return venue.SourceRevision(model.NewBlock("", text), model.LocaleEnglish)
}

// The SQLite ledger honors the same contract the Postgres store's
// decisions_test.go pins — one contract, two backends. Compact rather than
// exhaustive: the shared semantics (idempotency, last-writer-wins, freshness,
// the use-case-2 demotion) are asserted once here to catch backend drift.
func TestUnitDecisions_SQLiteContract(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	src := &model.Block{ID: "greeting", Translatable: true}
	src.SetSourceText("Hello")
	src.SetTargetText("nb", "Hei")
	src.SetEditionStatus(model.Variant("nb"), model.Status(model.TargetStatusTranslated))
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{src}))

	decision := venue.UnitDecision{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:    string(model.TargetStatusEstablished),
		Revision:  nbTextRevision("Hei"),
		Basis:     sourceRevision("Hello"),
		DecidedBy: "reviewer@example.com",
		Updated:   "2026-08-04T10:00:00Z",
	}
	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{decision})
	require.NoError(t, err)
	assert.Equal(t, 1, changed)

	status := func() model.TargetStatus {
		rows, err := s.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID: p.ID, Stream: "main", ItemName: "en.json", Limit: 10,
		})
		require.NoError(t, err)
		for _, sb := range rows {
			if sb.SourceID == "greeting" {
				if tgt, ok := sb.Block.Edition(model.Variant("nb")); ok {
					return model.TargetStatus(tgt.Status)
				}
			}
		}
		return ""
	}
	assert.Equal(t, model.TargetStatusEstablished, status(), "approval projects onto the stored target")

	// Idempotent replay.
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{decision})
	require.NoError(t, err)
	assert.Zero(t, changed)

	// Ledger round-trips.
	got, err := s.ListUnitDecisions(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, got, 1)
	assert.Equal(t, "reviewer@example.com", got[0].DecidedBy)

	// Use case 2: a source edit demotes the approval; the fact stays.
	edited := &model.Block{ID: "greeting", Translatable: true}
	edited.SetSourceText("Hello there")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{edited}))
	assert.Equal(t, model.TargetStatusTranslated, status(),
		"an approval must not survive an edit to the source it was made against")
	got, err = s.ListUnitDecisions(ctx, p.ID, "main")
	require.NoError(t, err)
	assert.Len(t, got, 1, "the decision stays in the ledger")
}

// TestUnitDecisions_RestoredSourceFindsItsApproval_SQLite: a source that comes
// back to the wording a decision blessed converges on that decision — the
// projection returns to the decided rung with no second review.
func TestUnitDecisions_RestoredSourceFindsItsApproval_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	src := &model.Block{ID: "greeting", Translatable: true}
	src.SetSourceText("Hello")
	src.SetTargetText("nb", "Hei")
	src.SetEditionStatus(model.Variant("nb"), model.Status(model.TargetStatusTranslated))
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{src}))

	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:   string(model.TargetStatusEstablished),
		Revision: nbTextRevision("Hei"),
		Basis:    sourceRevision("Hello"),
		Updated:  "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)

	status := func() model.TargetStatus {
		rows, err := s.GetBlocks(ctx, platstore.BlockQuery{
			ProjectID: p.ID, Stream: "main", ItemName: "en.json", Limit: 10,
		})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		nb, ok := rows[0].Block.Edition(model.Variant("nb"))
		require.True(t, ok)
		return model.TargetStatus(nb.Status)
	}
	require.Equal(t, model.TargetStatusEstablished, status())

	rewrite := func(text string) {
		edited := &model.Block{ID: "greeting", Translatable: true}
		edited.SetSourceText(text)
		require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{edited}))
	}
	rewrite("Hello there")
	assert.Equal(t, model.TargetStatusTranslated, status(), "the approval stops applying")

	rewrite("Hello")
	assert.Equal(t, model.TargetStatusEstablished, status(),
		"a restored source converges on the decision already recorded — no re-review")

	tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, tallies, 1)
	assert.Zero(t, tallies[0].Stale)
}

// TestTallyDecisionBasis_SQLite pins the grading of recorded decisions against
// the source the project holds now: the basis a decision names and the block's
// source revision are taken the same way, so the verdict is an equality join.
func TestTallyDecisionBasis_SQLite(t *testing.T) {
	tests := []struct {
		name string
		// basis is the revision the decision records: "current" for the
		// revision of the stored source, "" for none.
		basis            string
		rewriteSourceTo  string
		translatable     bool
		unit             string
		wantStale        int
		wantUnknown      int
		wantOwed         int
		wantNoTallyRow   bool
		wantTallyForUnit string
	}{
		{
			name: "basis matches the current source", basis: "current",
			translatable: true, unit: "greeting", wantNoTallyRow: true,
		},
		{
			name: "source rewritten under the decision", basis: "current",
			rewriteSourceTo: "Hello there", translatable: true, unit: "greeting",
			wantStale: 1, wantOwed: 1,
		},
		{
			name: "a record that names no source", basis: "",
			translatable: true, unit: "greeting", wantUnknown: 1,
		},
		{
			name: "an unknown basis stays unknown when the source moves", basis: "",
			rewriteSourceTo: "Hello there", translatable: true, unit: "greeting",
			wantUnknown: 1,
		},
		{
			name: "decision for a unit this store holds no block for", basis: "current",
			translatable: true, unit: "absent", wantNoTallyRow: true,
		},
		{
			name: "a non-translatable block is outside every denominator", basis: "current",
			rewriteSourceTo: "Hello there", translatable: false, unit: "greeting",
			wantNoTallyRow: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newTestStore(t)
			ctx := t.Context()
			p := createTestProject(t, s)

			src := &model.Block{ID: "greeting", Translatable: tt.translatable}
			src.SetSourceText("Hello")
			src.SetTargetText("nb", "Hei")
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
				edited := &model.Block{ID: "greeting", Translatable: tt.translatable}
				edited.SetSourceText(tt.rewriteSourceTo)
				require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{edited}))
			}

			tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
			require.NoError(t, err)
			if tt.wantNoTallyRow || (tt.wantStale == 0 && tt.wantUnknown == 0) {
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

// TestRecordDraftBases_SQLite mirrors the Postgres store's TestRecordDraftBases:
// the draft mark beside the decision, never over it, on rows the ledger holds
// and on no others, and the owed count that follows the mark.
func TestRecordDraftBases_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	block := func(id, source, target string) *model.Block {
		b := &model.Block{ID: id, Translatable: true}
		b.SetSourceText(source)
		if target != "" {
			b.SetTargetText("nb", target)
		}
		return b
	}
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		block("greeting", "Hello", "Hei"),
		block("farewell", "Goodbye", "Ha det"),
		block("untranslated", "See you", ""),
	}))
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

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		block("greeting", "Hello there", ""),
		block("farewell", "Goodbye then", ""),
		block("untranslated", "See you soon", ""),
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

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		block("greeting", "Hello again", ""),
	}))
	got = tally()
	assert.Equal(t, 1, got.Owed, "a second rewrite is owed a second draft")
}

// TestUpsertUnitDecisions_StaleBasisDoesNotProject_SQLite: a decision arriving
// against source this store has since rewritten is recorded in the ledger and
// projects nothing — the approval was for wording the project no longer has.
func TestUpsertUnitDecisions_StaleBasisDoesNotProject_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	src := &model.Block{ID: "greeting", Translatable: true}
	src.SetSourceText("Hello there")
	src.SetTargetText("nb", "Hei")
	src.SetEditionStatus(model.Variant("nb"), model.Status(model.TargetStatusTranslated))
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{src}))

	changed, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:   string(model.TargetStatusEstablished),
		Revision: nbTextRevision("Hei"),
		Basis:    sourceRevision("Hello"), // the wording the reviewer saw
		Updated:  "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "the decision is a fact and is recorded")

	rows, err := s.GetBlocks(ctx, platstore.BlockQuery{
		ProjectID: p.ID, Stream: "main", ItemName: "en.json", Limit: 10,
	})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	nb, ok := rows[0].Block.Edition(model.Variant("nb"))
	require.True(t, ok)
	assert.Equal(t, model.Status(model.TargetStatusTranslated), nb.Status,
		"a decision blessing source the store has rewritten must not project an approval")
}

// nbStatus is the rung the stored Norwegian translation of unit projects.
func nbStatus(t *testing.T, s *SQLiteStore, projectID, unit string) model.TargetStatus {
	t.Helper()
	rows, err := s.GetBlocks(t.Context(), platstore.BlockQuery{
		ProjectID: projectID, Stream: "main", ItemName: "en.json", Limit: 10,
	})
	require.NoError(t, err)
	for _, sb := range rows {
		if sb.SourceID == unit {
			nb, ok := sb.Block.Edition(model.Variant("nb"))
			require.True(t, ok)
			return model.TargetStatus(nb.Status)
		}
	}
	t.Fatalf("unit %s not found", unit)
	return ""
}

// translatedBlock is a block with source and an nb translation at the
// translated rung.
func translatedBlock(id, source, translation string) *model.Block {
	b := &model.Block{ID: id, Translatable: true}
	b.SetSourceText(source)
	b.SetTargetText("nb", translation)
	b.SetEditionStatus(model.Variant("nb"), model.Status(model.TargetStatusTranslated))
	return b
}

// TestUnitDecisions_ALoweringVerdictNeedsNoBasis_SQLite: the Postgres store's
// TestUnitDecisions_ALoweringVerdictNeedsNoBasis, on the working copy.
func TestUnitDecisions_ALoweringVerdictNeedsNoBasis_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		translatedBlock("greeting", "Hello", "Hei"),
		translatedBlock("farewell", "Goodbye", "Ha det"),
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
	assert.Equal(t, model.TargetStatusDraft, nbStatus(t, s, p.ID, "greeting"),
		"a rejection that names no source lowers the translation it names")
	assert.Equal(t, model.TargetStatusTranslated, nbStatus(t, s, p.ID, "farewell"),
		"a rejection of a translation of another source lowers nothing")

	_, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
		Revision: nbTextRevision("Hei"), DecidedAt: "2026-08-04T12:00:00Z", Updated: "2026-08-04T12:00:00Z",
	}})
	require.NoError(t, err)
	assert.Equal(t, model.TargetStatusDraft, nbStatus(t, s, p.ID, "greeting"),
		"an approval that names no source raises nothing")
}

// TestStoreBlocks_ASameLanguageTranslationLeavesTheSourceRevision_SQLite: the
// Postgres store's test of the same name, on the working copy.
func TestStoreBlocks_ASameLanguageTranslationLeavesTheSourceRevision_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	bilingual := translatedBlock("greeting", "Hello", "Hei")
	bilingual.SetTargetText(model.LocaleEnglish, "Hello there")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{bilingual}))
	stamped := func() string {
		rows, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main", ItemName: "en.json"})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		assert.Equal(t, venue.SourceRevision(rows[0].Block, model.LocaleEnglish), rows[0].SourceRevision)
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
	require.Equal(t, model.TargetStatusEstablished, nbStatus(t, s, p.ID, "greeting"))

	again := translatedBlock("greeting", "Hello", "Hei")
	again.SetEditionStatus(model.Variant("nb"), model.Status(model.TargetStatusEstablished))
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{again}))
	assert.Equal(t, first, stamped(), "the source did not move")
	assert.Equal(t, model.TargetStatusEstablished, nbStatus(t, s, p.ID, "greeting"))
	assert.Zero(t, countRows(t, s, "change_log", `project_id=? AND change_type='source_modified'`, p.ID))
}

// TestUpdateProject_SourceLanguageIsFixedOnceItHoldsContent_SQLite: the
// Postgres store's test of the same name, on the working copy.
func TestUpdateProject_SourceLanguageIsFixedOnceItHoldsContent_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	p.DefaultSourceLanguage = "en-GB"
	require.NoError(t, s.UpdateProject(ctx, p), "a project with no content may change its language")
	p.DefaultSourceLanguage = model.LocaleEnglish
	require.NoError(t, s.UpdateProject(ctx, p))

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		translatedBlock("greeting", "Hello", "Hei"),
	}))
	p.Name = "Renamed"
	require.NoError(t, s.UpdateProject(ctx, p), "settings other than the language change")

	p.DefaultSourceLanguage = "en-GB"
	require.ErrorIs(t, s.UpdateProject(ctx, p), platstore.ErrSourceLanguageFixed)
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

// TestUnitDecisions_GoverningFingerprintRoundTrips_SQLite: the fingerprint of
// the context a decision was made under is stored beside it and read back, a
// record that gains one is a change the store writes, and one made under a
// moved context replaces the one before it.
func TestUnitDecisions_GoverningFingerprintRoundTrips_SQLite(t *testing.T) {
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
	list, err := s.ListUnitDecisions(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "fp-governing", list[0].GoverningFingerprint)

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

// TestTallyDecisionBasis_RejectionOwesADraft_SQLite mirrors the Postgres
// store's TestTallyDecisionBasis_RejectionOwesADraft: the rejected count reads
// the verdict beside the basis, stays disjoint from the stale count, needs a
// target, and follows the draft mark.
func TestTallyDecisionBasis_RejectionOwesADraft_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	block := func(id, source, target string) *model.Block {
		b := &model.Block{ID: id, Translatable: true}
		b.SetSourceText(source)
		if target != "" {
			b.SetTargetText("nb", target)
		}
		return b
	}
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		block("refused", "Hello", "Hei"),
		block("drifted", "See you", "Vi ses"),
		block("bare", "Sign out", ""),
	}))
	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{
		{
			ItemName: "en.json", Unit: "refused", Variant: "nb",
			Status: string(model.TargetStatusDraft), ReviewState: venue.ReviewStateRejected,
			DecidedBy: "reviewer-1", Revision: nbTextRevision("Hei"),
			Basis: sourceRevision("Hello"), Updated: "2026-09-01T10:00:00Z",
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
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{
		block("drifted", "See you soon", ""),
	}))

	tally := func() platstore.DecisionBasisTally {
		t.Helper()
		tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
		require.NoError(t, err)
		require.Len(t, tallies, 1, "one (item, variant) scope")
		return tallies[0]
	}
	got := tally()
	assert.Equal(t, 1, got.Stale)
	assert.Equal(t, 1, got.Owed)
	assert.Equal(t, 1, got.RejectedOwed, "the rejection on an unmoved source is owed a draft")

	require.NoError(t, s.RecordDraftBases(ctx, p.ID, "main", []platstore.DraftBasis{
		{ItemName: "en.json", Unit: "refused", Variant: "nb", Basis: sourceRevision("Hello")},
	}))
	assert.Zero(t, tally().RejectedOwed, "drafted since the rejection")

	require.NoError(t, s.RecordDraftBases(ctx, p.ID, "main", []platstore.DraftBasis{
		{ItemName: "en.json", Unit: "refused", Variant: "nb"},
	}))
	assert.Equal(t, 1, tally().RejectedOwed, "a second rejection owes a second draft")
}

// TestUnitDecisions_RevisionPairingRoundTrips_SQLite: the revision pairing a
// decision carries is stored and read back, an identical record is a no-op,
// and a verdict on other content is a change.
func TestUnitDecisions_RevisionPairingRoundTrips_SQLite(t *testing.T) {
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
	list, err := s.ListUnitDecisions(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, "r:1111111111111111", list[0].Revision)
	assert.Equal(t, "r:aaaaaaaaaaaaaaaa", list[0].Basis)

	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{paired})
	require.NoError(t, err)
	assert.Zero(t, changed, "an identical record is a no-op")

	recoded := paired
	recoded.Revision = "r:2222222222222222"
	recoded.Updated = "2026-08-04T11:00:00Z"
	changed, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{recoded})
	require.NoError(t, err)
	assert.Equal(t, 1, changed, "a verdict on other content is a change")
}

// TestUnitDecisions_AnInlineCodeRetiresAnApproval_SQLite mirrors the Postgres
// store's test: a change to a link alone in the source moves its revision, so
// the approval made for the old link stops applying and returns with it.
func TestUnitDecisions_AnInlineCodeRetiresAnApproval_SQLite(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	linked := func(href string) *model.Block {
		b := &model.Block{ID: "guide", Translatable: true}
		b.SetSourceRuns([]model.Run{
			model.TextR("Read "),
			model.PcOpenR(model.PcOpenRun{ID: "1", Type: "link", Data: `<a href="` + href + `">`}),
			model.TextR("the guide"),
			model.PcCloseR(model.PcCloseRun{ID: "1", Type: "link", Data: "</a>"}),
		})
		return b
	}
	b := linked("/v1/guide")
	b.SetTargetText("nb", "Les veiledningen")
	b.SetEditionStatus(model.Variant("nb"), model.Status(model.TargetStatusTranslated))
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{b}))
	_, err := s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "guide", Variant: "nb",
		Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
		Revision: nbTextRevision("Les veiledningen"), Basis: venue.SourceRevision(linked("/v1/guide"), model.LocaleEnglish),
		DecidedBy: "reviewer@example.com", Updated: "2026-10-04T10:00:00Z",
	}})
	require.NoError(t, err)
	status := func() model.TargetStatus {
		rows, err := s.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main", ItemName: "en.json", Limit: 10})
		require.NoError(t, err)
		require.Len(t, rows, 1)
		nb, ok := rows[0].Block.Edition(model.Variant("nb"))
		require.True(t, ok)
		return model.TargetStatus(nb.Status)
	}
	require.Equal(t, model.TargetStatusEstablished, status())

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{linked("/v2/guide")}))
	assert.Equal(t, model.TargetStatusTranslated, status(), "the link moved under the approval")
	tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, tallies, 1)
	assert.Equal(t, 1, tallies[0].Stale)

	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{linked("/v1/guide")}))
	assert.Equal(t, model.TargetStatusEstablished, status(), "the link moving back restores it")
}

// TestMigrations_AWorkingCopyGradedByHashLosesThePairing: a working copy
// written before version 36 holds its decisions' text hashes, draft marks and
// settlement stamps that name a text hash, and blocks with no source revision.
// Version 36 drops the hashes, clears the marks and the stamps, and gives every
// block an empty source revision until its source is written again. The
// verdicts themselves read back.
func TestMigrations_AWorkingCopyGradedByHashLosesThePairing(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.db")
	s, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	ctx := t.Context()
	p := createTestProject(t, s)
	src := &model.Block{ID: "greeting", Translatable: true}
	src.SetSourceText("Hello")
	src.SetTargetText("nb", "Hei")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{src}))
	_, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status: string(model.TargetStatusEstablished), ReviewState: "approved",
		DecidedBy: "reviewer@example.com", Updated: "2026-08-04T10:00:00Z",
	}})
	require.NoError(t, err)

	// The schema a working copy at version 35 carries.
	for _, stmt := range []string{
		`ALTER TABLE unit_decisions ADD COLUMN target_hash TEXT NOT NULL DEFAULT ''`,
		`ALTER TABLE unit_decisions ADD COLUMN content_hash TEXT NOT NULL DEFAULT ''`,
		`UPDATE unit_decisions SET target_hash='t-hash', content_hash='s-hash', draft_basis='s-hash'`,
		`UPDATE blocks SET properties = json_set(properties, '$.__source_settled_hash', 's-hash')`,
		`ALTER TABLE blocks DROP COLUMN source_revision`,
		`DELETE FROM schema_migrations WHERE version >= 36`,
	} {
		_, err = s.db.ExecContext(ctx, stmt)
		require.NoError(t, err, stmt)
	}
	require.NoError(t, s.Close())

	reopened, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = reopened.Close() })
	columns := func(table string) []string {
		rows, err := reopened.db.QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, table)
		require.NoError(t, err)
		defer rows.Close()
		var out []string
		for rows.Next() {
			var name string
			require.NoError(t, rows.Scan(&name))
			out = append(out, name)
		}
		return out
	}
	assert.NotContains(t, columns("unit_decisions"), "target_hash")
	assert.NotContains(t, columns("unit_decisions"), "content_hash")
	assert.Contains(t, columns("blocks"), "source_revision")

	got, err := reopened.GetUnitDecision(ctx, p.ID, "main", "en.json", "greeting", "nb")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "approved", got.ReviewState, "the verdict reads back")
	assert.Empty(t, got.Basis, "it names no source")
	drafts, err := reopened.ListDraftBases(ctx, p.ID, "main")
	require.NoError(t, err)
	assert.Empty(t, drafts, "a mark that named a text hash is cleared")
	rows, err := reopened.GetBlocks(ctx, platstore.BlockQuery{ProjectID: p.ID, Stream: "main", ItemName: "en.json"})
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.Empty(t, rows[0].SourceRevision, "a block holds no source revision until its source is written again")
	assert.NotContains(t, rows[0].Block.Properties, "__source_settled_hash", "a settlement stamp that named a text hash is cleared")
}

// TestMigrations_AWorkingCopyFromBeforeTheRebuildStillOpens: a working copy
// made before the store was rebuilt from its baseline records versions up to
// 34 and holds no ledger, so it skips every version below 35. Version 35 gives
// it the ledger before adding the revision columns, and the store opens and
// records decisions as any other does.
func TestMigrations_AWorkingCopyFromBeforeTheRebuildStillOpens(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "store.db")
	s, err := NewSQLiteStore(dbPath)
	require.NoError(t, err)
	ctx := t.Context()
	_, err = s.db.ExecContext(ctx, `DROP TABLE unit_decisions`)
	require.NoError(t, err)
	_, err = s.db.ExecContext(ctx, `ALTER TABLE blocks DROP COLUMN source_revision`)
	require.NoError(t, err)
	_, err = s.db.ExecContext(ctx, `DELETE FROM schema_migrations WHERE version >= 28`)
	require.NoError(t, err)
	for v := 28; v <= 34; v++ {
		_, err = s.db.ExecContext(ctx, `INSERT INTO schema_migrations (version, description) VALUES (?, 'issued before the rebuild')`, v)
		require.NoError(t, err)
	}
	require.NoError(t, s.Close())

	reopened, err := NewSQLiteStore(dbPath)
	require.NoError(t, err, "the working copy opens")
	t.Cleanup(func() { _ = reopened.Close() })
	p := createTestProject(t, reopened)
	d := venue.UnitDecision{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status:   string(model.TargetStatusEstablished),
		Revision: "r:1111111111111111", Basis: "r:aaaaaaaaaaaaaaaa",
		ReviewState: "approved", Updated: "2026-08-04T10:00:00Z",
	}
	_, err = reopened.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{d})
	require.NoError(t, err)
	got, err := reopened.GetUnitDecision(ctx, p.ID, "main", "en.json", "greeting", "nb")
	require.NoError(t, err)
	require.NotNil(t, got)
	assert.Equal(t, "r:1111111111111111", got.Revision)
}

// TestTallyDecisionBasis_SQLiteGradesARecordedDerivation pins the grading of a translation no decision records: the
// derivation its edition records (model.Edition.Derived) is its basis, so a
// source rewritten under it is stale and owed a draft, and a decision on the
// unit takes over from it.
func TestTallyDecisionBasis_SQLiteGradesARecordedDerivation(t *testing.T) {
	s := newTestStore(t)
	ctx := t.Context()
	p := createTestProject(t, s)

	src := &model.Block{ID: "greeting", Translatable: true}
	src.SetSourceText("Hello")
	src.SetTargetText("nb", "Hei")
	src.SetDerivation(model.EditionKey{Locale: "nb"}, &model.Derivation{Rev: sourceRevision("Hello")})
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{src}))

	tallies, err := s.TallyDecisionBasis(ctx, p.ID, "main")
	require.NoError(t, err)
	assert.Empty(t, tallies, "a translation made from the current source is not stale")

	edited := blockWithText("greeting", "Hello there")
	require.NoError(t, s.StoreBlocksForItem(ctx, p.ID, "main", "en.json", []*model.Block{edited}))
	tallies, err = s.TallyDecisionBasis(ctx, p.ID, "main")
	require.NoError(t, err)
	require.Len(t, tallies, 1)
	assert.Equal(t, "en.json", tallies[0].ItemName)
	assert.Equal(t, "nb", tallies[0].Variant)
	assert.Equal(t, 1, tallies[0].Stale, "the source moved under the translation's recorded basis")
	assert.Equal(t, 1, tallies[0].Owed, "and the loop owes it a draft")

	_, err = s.UpsertUnitDecisions(ctx, p.ID, "main", []venue.UnitDecision{{
		ItemName: "en.json", Unit: "greeting", Variant: "nb",
		Status: string(model.TargetStatusEstablished), ReviewState: venue.ReviewStateApproved,
		Revision: nbTextRevision("Hei"), Basis: sourceRevision("Hello there"),
		DecidedBy: "reviewer@example.com", Updated: "2026-10-06T10:00:00Z",
	}})
	require.NoError(t, err)
	tallies, err = s.TallyDecisionBasis(ctx, p.ID, "main")
	require.NoError(t, err)
	for _, got := range tallies {
		assert.Zero(t, got.Stale, "a decision's basis takes over from the derivation")
		assert.Zero(t, got.Owed)
	}
}
