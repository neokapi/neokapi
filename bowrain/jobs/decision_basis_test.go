package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/venue"
	fwmemory "github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// basisFixture is the three-way shape every test in this file needs, seeded in a
// real PostgreSQL store: one unit whose translation records a basis naming
// wording the source no longer carries (stale), one whose translation records
// none and that no decision names (unrecorded), and one whose translation was
// made from the source the project holds now (fresh). All three carry a
// French target. The basis lives on the edition (model.Edition.Derived), where
// the stream home records it when a change set writes the translation.
type basisFixture struct {
	db        *storage.PgDB
	cs        *bstore.PostgresStore
	projectID string
	item      string
}

const (
	basisStaleSource      = "Colour picker"
	basisUnrecordedSource = "Save changes"
	basisFreshSource      = "Delete account"
)

func newBasisFixture(t *testing.T) basisFixture {
	t.Helper()
	db := pgtest.NewTestDB(t)
	ctx := t.Context()

	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)

	const projectID = "proj-basis"
	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID:                    projectID,
		Name:                  "Basis",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
		Properties:            map[string]string{"translate_after": "none"},
	}))

	blocks := []*model.Block{
		derivedBlock("stale", basisStaleSource, "Sélecteur de couleur", srcRevision("Colour picker (the wording before the fix)")),
		basisBlock("unrecorded", basisUnrecordedSource, "Enregistrer"),
		derivedBlock("fresh", basisFreshSource, "Supprimer le compte", srcRevision(basisFreshSource)),
	}
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "ui.json", blocks))

	return basisFixture{db: db, cs: cs, projectID: projectID, item: "ui.json"}
}

// derivedBlock is basisBlock with the French translation recording the source
// revision it was made from.
func derivedBlock(id, source, target, basis string) *model.Block {
	b := basisBlock(id, source, target)
	b.SetDerivation(model.EditionKey{Locale: "fr"}, &model.Derivation{Rev: basis})
	return b
}

// srcRevision is the revision of a plain-text source as the store stamps it
// for the fixture's project, written in English.
func srcRevision(source string) string {
	return venue.SourceRevision(model.NewBlock("", source), "en")
}

// frRevision is the revision of a plain-text French translation.
func frRevision(target string) string {
	return model.RunsRevision(model.Variant("fr"), []model.Run{model.TextR(target)})
}

func basisBlock(id, source, target string) *model.Block {
	b := model.NewBlock(id, source)
	b.SourceLocale = "en"
	b.SetTargetText("fr", target)
	return b
}

// storedFor loads the fixture's blocks keyed by source text, so a test can name
// a unit by its wording rather than by the id the store assigned it.
func (f basisFixture) storedFor(t *testing.T) map[string]*venue.StoredBlock {
	t.Helper()
	rows, err := f.cs.GetBlocks(t.Context(), store.BlockQuery{
		ProjectID: f.projectID, Stream: "main", ItemName: f.item,
	})
	require.NoError(t, err)
	out := map[string]*venue.StoredBlock{}
	for _, sb := range rows {
		out[sb.Block.SourceText()] = sb
	}
	return out
}

// TestDecisionLedger_NeedsDraft pins the predicate the recycle pass partitions on
// and the estimate prices from: a target whose recorded basis names source
// wording the block no longer holds is work; one recorded against the current
// source is done; one the ledger has never heard of is left alone whatever the
// source says.
func TestDecisionLedger_NeedsDraft(t *testing.T) {
	f := newBasisFixture(t)
	ctx := t.Context()
	ledger := loadDecisionLedger(ctx, f.cs, f.projectID, "main")
	assert.Nil(t, ledger, "the fixture records no decision")

	stored := f.storedFor(t)
	require.NotNil(t, stored[basisStaleSource].Block.Editions, "the stored block keeps its editions")
	fr, _ := stored[basisStaleSource].Block.Edition(model.EditionKey{Locale: "fr"})
	require.NotNil(t, fr.Derived, "the store keeps the derivation the translation records")

	assert.True(t, ledger.needsDraft(stored[basisStaleSource], "fr"),
		"the recorded basis names wording the source no longer carries")
	assert.False(t, ledger.needsDraft(stored[basisUnrecordedSource], "fr"),
		"a target the platform has no record of writing is left where it is")
	assert.False(t, ledger.needsDraft(stored[basisFreshSource], "fr"),
		"the recorded basis is the source the project holds now")

	// A locale with no target at all is pending whatever the ledger says, and a
	// locale nobody has a record for is not.
	assert.True(t, ledger.needsDraft(stored[basisStaleSource], "de"),
		"no target for the locale is work")

	// A decision names the basis it blessed, whatever the translation records.
	_, err := f.cs.UpsertUnitDecisions(ctx, f.projectID, "main", []venue.UnitDecision{{
		ItemName: f.item, Unit: "stale", Variant: "fr", Status: string(model.TargetStatusEstablished),
		Revision: frRevision("Sélecteur de couleur"), Basis: srcRevision(basisStaleSource),
		ReviewState: "approved", DecidedBy: "reviewer-1", Updated: "2026-02-01T00:00:00Z",
	}})
	require.NoError(t, err)
	ledger = loadDecisionLedger(ctx, f.cs, f.projectID, "main")
	assert.False(t, ledger.needsDraft(stored[basisStaleSource], "fr"),
		"an approval of the current source settles the unit")
}

// TestRecycleBlocks_PartitionsOnTheRecordedBasis proves the recycle pass acts on
// the same answer: only the stale unit is a candidate, and a content-memory hit
// for the CURRENT source replaces the translation of the wording that is gone.
func TestRecycleBlocks_PartitionsOnTheRecordedBasis(t *testing.T) {
	f := newBasisFixture(t)
	ctx := t.Context()
	ledger := loadDecisionLedger(ctx, f.cs, f.projectID, "main")
	stored := f.storedFor(t)

	tm := fwmemory.NewInMemoryStore()
	seedMemoryEntry(t, tm, basisStaleSource, "Sélecteur de couleurs")

	rows := []*venue.StoredBlock{
		stored[basisStaleSource], stored[basisUnrecordedSource], stored[basisFreshSource],
	}
	res, err := recycleBlocks(ctx, tm, rows, "en", "fr", 1.0, ledger, nil)
	require.NoError(t, err)

	require.Len(t, res.filled, 1, "only the stale unit is a candidate, and the corpus answers it")
	assert.Equal(t, basisStaleSource, res.filled[0].SourceText())
	assert.Equal(t, "Sélecteur de couleurs", res.filled[0].TargetText("fr"),
		"the recycled wording replaces the translation of the source that is gone")
	assert.Empty(t, res.remainder, "the other two units are done")
	assert.Equal(t, 1, res.memoryCount)
}

// TestRecycleBlocks_StaleWithNoCorpusAnswerGoesToAI is the other half of the
// partition: a stale unit the content memory cannot answer arrives at the AI
// remainder carrying its previous translation, rather than being counted as
// recycled because a target happens to be there.
func TestRecycleBlocks_StaleWithNoCorpusAnswerGoesToAI(t *testing.T) {
	f := newBasisFixture(t)
	ctx := t.Context()
	ledger := loadDecisionLedger(ctx, f.cs, f.projectID, "main")
	stored := f.storedFor(t)

	tm := fwmemory.NewInMemoryStore()

	res, err := recycleBlocks(ctx, tm, []*venue.StoredBlock{stored[basisStaleSource]}, "en", "fr", 1.0, ledger, nil)
	require.NoError(t, err)

	assert.Zero(t, res.memoryCount)
	assert.Empty(t, res.filled)
	require.Len(t, res.remainder, 1, "the stale unit is paid AI work, not free content-memory leverage")
	assert.Equal(t, "Sélecteur de couleur", res.remainder[0].TargetText("fr"),
		"the previous translation travels with it, for the reviewer to compare against")
}

// TestEstimateConvergence_PricesTheRunsOwnPredicate is the quote/run agreement:
// the estimate's Pending is exactly the set the recycle pass would take.
func TestEstimateConvergence_PricesTheRunsOwnPredicate(t *testing.T) {
	f := newBasisFixture(t)
	ctx := t.Context()

	proj, err := f.cs.GetProject(ctx, f.projectID)
	require.NoError(t, err)

	est, err := EstimateConvergence(ctx, f.cs, nil, nil, proj)
	require.NoError(t, err)

	require.Len(t, est.Locales, 1)
	assert.Equal(t, "fr", est.Locales[0].Locale)
	assert.Equal(t, 1, est.Locales[0].Pending, "only the stale unit is owed a draft")
	assert.Equal(t, 1, est.Locales[0].ViaAI, "with no content memory, the pending unit is AI work")
	assert.Equal(t, 1, est.Totals.Pending)

	// The run's own partition over the same corpus agrees with the quote.
	ledger := loadDecisionLedger(ctx, f.cs, f.projectID, "main")
	stored := f.storedFor(t)
	rows := []*venue.StoredBlock{
		stored[basisStaleSource], stored[basisUnrecordedSource], stored[basisFreshSource],
	}
	pending := 0
	for _, sb := range rows {
		if ledger.needsDraft(sb, "fr") {
			pending++
		}
	}
	assert.Equal(t, est.Locales[0].Pending, pending)
}

// TestRecordDraftMarks pins what the convergence worker writes into the ledger
// for the targets it produced: a draft mark on the row of a decided unit, and
// nothing for a unit no decision names. The basis of the draft itself is on the
// edition the stream home wrote.
func TestRecordDraftMarks(t *testing.T) {
	f := newBasisFixture(t)
	ctx := t.Context()
	stored := f.storedFor(t)

	_, err := f.cs.UpsertUnitDecisions(ctx, f.projectID, "main", []venue.UnitDecision{{
		ItemName:    f.item,
		Unit:        "fresh",
		Variant:     "fr",
		Status:      string(model.TargetStatusEstablished),
		Revision:    frRevision("Supprimer le compte"),
		Basis:       srcRevision(basisFreshSource),
		ReviewState: "approved",
		DecidedBy:   "reviewer-1",
		Updated:     "2026-02-01T00:00:00Z",
	}})
	require.NoError(t, err)

	ledger := loadDecisionLedger(ctx, f.cs, f.projectID, "main")
	byBlockID := map[string]*venue.StoredBlock{}
	var written []*model.Block
	for _, sb := range stored {
		byBlockID[sb.Block.ID] = sb
		written = append(written, sb.Block)
	}
	recordDraftMarks(ctx, f.cs, f.projectID, "main", ledger, byBlockID, written, "fr")

	after, err := f.cs.ListUnitDecisions(ctx, f.projectID, "main")
	require.NoError(t, err)
	require.Len(t, after, 1, "the pass writes no record for a unit no decision names")
	assert.Equal(t, "fresh", after[0].Unit)
	assert.Equal(t, "approved", after[0].ReviewState, "a decided unit keeps the reviewer's record")

	drafts, err := f.cs.ListDraftBases(ctx, f.projectID, "main")
	require.NoError(t, err)
	require.Len(t, drafts, 1)
	assert.Equal(t, srcRevision(basisFreshSource), drafts[0].Basis, "the decided unit's row is marked with the source it was drafted against")
}

// TestWorkerRecordsTheBasisOfWhatItDrafts drives the real translation worker over
// the fixture: the stale unit is re-drafted, the unrecorded and fresh ones are
// left alone, and the draft's edition records the source it was made from, so
// the next pass reads the unit as current rather than re-drafting it forever.
func TestWorkerRecordsTheBasisOfWhatItDrafts(t *testing.T) {
	f := newBasisFixture(t)
	ctx := t.Context()

	db := f.db
	js, err := NewJobStore(db)
	require.NoError(t, err)

	deps := &WorkerDeps{
		JobStore:      js,
		ContentStore:  f.cs,
		Platform:      &PlatformProviderConfig{Provider: "demo"},
		ProviderStore: &fakeProviderResolver{cfg: bstore.ProviderConfig{Type: "demo"}},
	}
	job := &TranslationJob{
		ID:               "job-basis",
		WorkspaceSlug:    "acme",
		ProjectID:        f.projectID,
		ItemName:         f.item,
		TargetLocale:     "fr",
		ProviderConfigID: "platform",
		Model:            "demo",
		Status:           StatusQueued,
	}
	require.NoError(t, js.CreateJob(ctx, job))
	claimed, epoch, err := js.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, executeTranslationWithDeps(ctx, deps, job, epoch))

	stored := f.storedFor(t)
	assert.NotEqual(t, "Sélecteur de couleur", stored[basisStaleSource].Block.TargetText("fr"),
		"the stale unit is re-drafted from the source the project holds now")
	assert.Equal(t, "Enregistrer", stored[basisUnrecordedSource].Block.TargetText("fr"),
		"a target the platform has no record of writing is left alone")
	assert.Equal(t, "Supprimer le compte", stored[basisFreshSource].Block.TargetText("fr"),
		"a target recorded against the current source is done")

	after, err := f.cs.ListUnitDecisions(ctx, f.projectID, "main")
	require.NoError(t, err)
	assert.Empty(t, after, "the worker writes no basis record into the decision ledger")
	fr, ok := stored[basisStaleSource].Block.Edition(model.EditionKey{Locale: "fr"})
	require.True(t, ok)
	require.NotNil(t, fr.Derived, "the re-draft records what it was made from on the edition")
	assert.Equal(t, srcRevision(basisStaleSource), fr.Derived.Rev,
		"the re-draft's basis is the source it was made from")

	// The second pass has nothing to do: the unit it re-drafted now reads current.
	ledger := loadDecisionLedger(ctx, f.cs, f.projectID, "main")
	for _, sb := range stored {
		assert.False(t, ledger.needsDraft(sb, "fr"),
			"every unit reads settled after the run: %s", sb.Block.SourceText())
	}
}

// decideStaleUnit puts a reviewer's approval on the fixture's stale unit,
// recorded against the wording the source no longer carries. The approval is
// what a source edit withdraws and what the worker must never write over.
func (f basisFixture) decideStaleUnit(t *testing.T) {
	t.Helper()
	_, err := f.cs.UpsertUnitDecisions(t.Context(), f.projectID, "main", []venue.UnitDecision{{
		ItemName:    f.item,
		Unit:        "stale",
		Variant:     "fr",
		Status:      string(model.TargetStatusEstablished),
		Revision:    frRevision("Sélecteur de couleur"),
		Basis:       srcRevision("Colour picker (the wording before the fix)"),
		ReviewState: "approved",
		DecidedBy:   "reviewer-1",
		Updated:     "2026-02-01T00:00:00Z",
	}})
	require.NoError(t, err)
}

// TestDecisionLedger_NeedsDraft_DraftedStaleUnitWaitsOnReview pins the guard on
// a decided unit whose source moved: owed a draft until the platform records
// the source it drafted against, then waiting on a reviewer with the decision
// still on the row, then owed again when the source moves once more. The
// estimate prices the same predicate at each step.
func TestDecisionLedger_NeedsDraft_DraftedStaleUnitWaitsOnReview(t *testing.T) {
	f := newBasisFixture(t)
	ctx := t.Context()
	f.decideStaleUnit(t)

	ledger := loadDecisionLedger(ctx, f.cs, f.projectID, "main")
	stored := f.storedFor(t)
	assert.True(t, ledger.needsDraft(stored[basisStaleSource], "fr"),
		"the approval blessed wording the source no longer carries")

	proj, err := f.cs.GetProject(ctx, f.projectID)
	require.NoError(t, err)
	est, err := EstimateConvergence(ctx, f.cs, nil, nil, proj)
	require.NoError(t, err)
	assert.Equal(t, 1, est.Totals.Pending, "the quote prices the re-draft")

	require.NoError(t, f.cs.RecordDraftBases(ctx, f.projectID, "main", []store.DraftBasis{{
		ItemName: f.item, Unit: "stale", Variant: "fr", Basis: srcRevision(basisStaleSource),
	}}))
	ledger = loadDecisionLedger(ctx, f.cs, f.projectID, "main")
	assert.False(t, ledger.needsDraft(stored[basisStaleSource], "fr"),
		"drafted against the source the project holds now: the unit waits on a reviewer")
	rec, ok := ledger[decisionUnitKey{item: f.item, unit: "stale", variant: "fr"}]
	require.True(t, ok)
	assert.Equal(t, "approved", rec.ReviewState, "the decision is still the reviewer's")
	assert.Equal(t, srcRevision("Colour picker (the wording before the fix)"), rec.Basis)
	assert.Equal(t, srcRevision(basisStaleSource), rec.draftBasis)

	est, err = EstimateConvergence(ctx, f.cs, nil, nil, proj)
	require.NoError(t, err)
	assert.Zero(t, est.Totals.Pending, "the quote owes nothing for a unit awaiting review")

	// The source moves again, away from the mark.
	require.NoError(t, f.cs.StoreBlocksForItem(ctx, f.projectID, "main", f.item, []*model.Block{
		basisBlock("stale", "Colour picker, revised", "Sélecteur de couleur"),
	}))
	ledger = loadDecisionLedger(ctx, f.cs, f.projectID, "main")
	stored = f.storedFor(t)
	assert.True(t, ledger.needsDraft(stored["Colour picker, revised"], "fr"),
		"a second rewrite is owed a second draft")
}

// TestWorker_DraftsAStaleDecidedUnitOncePerSourceChange drives the real worker
// over a decided unit whose source moved: the first job re-drafts it and
// records the source it drafted against, leaving the reviewer's approval on the
// row; the second job finds nothing owed; and a further source rewrite is
// drafted once more.
func TestWorker_DraftsAStaleDecidedUnitOncePerSourceChange(t *testing.T) {
	f := newBasisFixture(t)
	ctx := t.Context()
	f.decideStaleUnit(t)

	js, err := NewJobStore(f.db)
	require.NoError(t, err)
	deps := &WorkerDeps{
		JobStore:      js,
		ContentStore:  f.cs,
		Platform:      &PlatformProviderConfig{Provider: "demo"},
		ProviderStore: &fakeProviderResolver{cfg: bstore.ProviderConfig{Type: "demo"}},
	}
	runJob := func(id string) *TranslationJob {
		t.Helper()
		job := &TranslationJob{
			ID:               id,
			WorkspaceSlug:    "acme",
			ProjectID:        f.projectID,
			ItemName:         f.item,
			TargetLocale:     "fr",
			ProviderConfigID: "platform",
			Model:            "demo",
			Status:           StatusQueued,
		}
		require.NoError(t, js.CreateJob(ctx, job))
		claimed, epoch, err := js.ClaimJob(ctx, job.ID)
		require.NoError(t, err)
		require.True(t, claimed)
		require.NoError(t, executeTranslationWithDeps(ctx, deps, job, epoch))
		done, err := js.GetJob(ctx, job.ID)
		require.NoError(t, err)
		return done
	}
	ledgerRow := func() (venue.UnitDecision, string) {
		t.Helper()
		records, err := f.cs.ListUnitDecisions(ctx, f.projectID, "main")
		require.NoError(t, err)
		var rec venue.UnitDecision
		for _, d := range records {
			if d.Unit == "stale" {
				rec = d
			}
		}
		drafts, err := f.cs.ListDraftBases(ctx, f.projectID, "main")
		require.NoError(t, err)
		mark := ""
		for _, d := range drafts {
			if d.Unit == "stale" {
				mark = d.Basis
			}
		}
		return rec, mark
	}

	first := runJob("job-redraft-1")
	assert.Equal(t, 1, first.TotalBlocks, "only the stale unit is owed")
	stored := f.storedFor(t)
	redrafted := stored[basisStaleSource].Block.TargetText("fr")
	assert.NotEqual(t, "Sélecteur de couleur", redrafted, "the stale unit is re-drafted")
	rec, mark := ledgerRow()
	assert.Equal(t, "approved", rec.ReviewState, "the reviewer's decision stays on the row")
	assert.Equal(t, "reviewer-1", rec.DecidedBy)
	assert.Equal(t, srcRevision("Colour picker (the wording before the fix)"), rec.Basis,
		"the decision's basis is never written over")
	assert.Equal(t, srcRevision(basisStaleSource), mark, "the draft is marked with the source it was made from")

	second := runJob("job-redraft-2")
	assert.Zero(t, second.TotalBlocks, "nothing is owed: the unit waits on a reviewer")
	assert.Zero(t, second.DoneBlocks)
	stored = f.storedFor(t)
	assert.Equal(t, redrafted, stored[basisStaleSource].Block.TargetText("fr"))

	// The source moves again: one more draft, and one only.
	require.NoError(t, f.cs.StoreBlocksForItem(ctx, f.projectID, "main", f.item, []*model.Block{
		basisBlock("stale", "Colour picker, revised", redrafted),
	}))
	third := runJob("job-redraft-3")
	assert.Equal(t, 1, third.TotalBlocks, "a second rewrite is owed a second draft")
	_, mark = ledgerRow()
	assert.Equal(t, srcRevision("Colour picker, revised"), mark)
	fourth := runJob("job-redraft-4")
	assert.Zero(t, fourth.TotalBlocks)
}

// reject records a reviewer's rejection on one of the fixture's units and
// clears the platform's draft mark, which is the pair of writes the server
// makes when somebody turns a translation down
// (server.unitDecisionFor + reviewLedger.clearDraftBasis).
func (f basisFixture) reject(t *testing.T, unit, source, target string) {
	t.Helper()
	ctx := t.Context()
	_, err := f.cs.UpsertUnitDecisions(ctx, f.projectID, "main", []venue.UnitDecision{{
		ItemName:    f.item,
		Unit:        unit,
		Variant:     "fr",
		Status:      string(model.TargetStatusDraft),
		Revision:    frRevision(target),
		Basis:       srcRevision(source),
		ReviewState: venue.ReviewStateRejected,
		DecidedBy:   "reviewer-1",
		Updated:     "2026-02-01T00:00:00Z",
	}})
	require.NoError(t, err)
	require.NoError(t, f.cs.RecordDraftBases(ctx, f.projectID, "main", []store.DraftBasis{{
		ItemName: f.item, Unit: unit, Variant: "fr",
	}}))
}

// TestDecisionLedger_NeedsDraft_RejectedUnitOwesADraft pins the predicate on a
// unit nothing has rewritten: the reviewer turned the translation down, so both
// revisions still name what the row recorded and the basis alone reads as
// settled.
// The unit is owed a draft until the platform has made one since the rejection,
// then it waits on the next verdict, and a second rejection owes one more.
func TestDecisionLedger_NeedsDraft_RejectedUnitOwesADraft(t *testing.T) {
	f := newBasisFixture(t)
	ctx := t.Context()
	f.reject(t, "fresh", basisFreshSource, "Supprimer le compte")

	stored := f.storedFor(t)
	ledger := loadDecisionLedger(ctx, f.cs, f.projectID, "main")
	rec, ok := ledger[decisionUnitKey{item: f.item, unit: "fresh", variant: "fr"}]
	require.True(t, ok)
	require.Equal(t, srcRevision(basisFreshSource), rec.Basis,
		"the rejection moved neither revision: the row names the source the block holds")
	assert.True(t, ledger.needsDraft(stored[basisFreshSource], "fr"),
		"a reviewer said the wording will not do, so the unit is work")

	proj, err := f.cs.GetProject(ctx, f.projectID)
	require.NoError(t, err)
	est, err := EstimateConvergence(ctx, f.cs, nil, nil, proj)
	require.NoError(t, err)
	assert.Equal(t, 2, est.Totals.Pending, "the quote prices the stale unit and the refused one")

	require.NoError(t, f.cs.RecordDraftBases(ctx, f.projectID, "main", []store.DraftBasis{{
		ItemName: f.item, Unit: "fresh", Variant: "fr", Basis: srcRevision(basisFreshSource),
	}}))
	ledger = loadDecisionLedger(ctx, f.cs, f.projectID, "main")
	assert.False(t, ledger.needsDraft(stored[basisFreshSource], "fr"),
		"drafted since the rejection: the unit waits on the next verdict")
	rec = ledger[decisionUnitKey{item: f.item, unit: "fresh", variant: "fr"}]
	assert.Equal(t, venue.ReviewStateRejected, rec.ReviewState, "the verdict is still the reviewer's")

	est, err = EstimateConvergence(ctx, f.cs, nil, nil, proj)
	require.NoError(t, err)
	assert.Equal(t, 1, est.Totals.Pending, "the quote owes nothing more for the refused unit")

	f.reject(t, "fresh", basisFreshSource, "Supprimer le compte")
	ledger = loadDecisionLedger(ctx, f.cs, f.projectID, "main")
	assert.True(t, ledger.needsDraft(stored[basisFreshSource], "fr"),
		"a second rejection of the same source owes a second draft")
}

// TestWorker_DraftsARejectedUnitOncePerVerdict drives the real worker over a
// unit a reviewer turned down on a source nothing has rewritten: the first job
// drafts it and marks what it drafted against, leaving the rejection on the
// row; the second finds nothing owed; and a second rejection buys exactly one
// more draft. Red before the fix: the first job found nothing owed either, and
// the unit sat at `draft` until somebody edited the source.
func TestWorker_DraftsARejectedUnitOncePerVerdict(t *testing.T) {
	f := newBasisFixture(t)
	ctx := t.Context()
	f.reject(t, "fresh", basisFreshSource, "Supprimer le compte")

	js, err := NewJobStore(f.db)
	require.NoError(t, err)
	deps := &WorkerDeps{
		JobStore:      js,
		ContentStore:  f.cs,
		Platform:      &PlatformProviderConfig{Provider: "demo"},
		ProviderStore: &fakeProviderResolver{cfg: bstore.ProviderConfig{Type: "demo"}},
	}
	runJob := func(id string) *TranslationJob {
		t.Helper()
		job := &TranslationJob{
			ID:               id,
			WorkspaceSlug:    "acme",
			ProjectID:        f.projectID,
			ItemName:         f.item,
			TargetLocale:     "fr",
			ProviderConfigID: "platform",
			Model:            "demo",
			Status:           StatusQueued,
		}
		require.NoError(t, js.CreateJob(ctx, job))
		claimed, epoch, err := js.ClaimJob(ctx, job.ID)
		require.NoError(t, err)
		require.True(t, claimed)
		require.NoError(t, executeTranslationWithDeps(ctx, deps, job, epoch))
		done, err := js.GetJob(ctx, job.ID)
		require.NoError(t, err)
		return done
	}
	freshRow := func() (venue.UnitDecision, string) {
		t.Helper()
		records, err := f.cs.ListUnitDecisions(ctx, f.projectID, "main")
		require.NoError(t, err)
		var rec venue.UnitDecision
		for _, d := range records {
			if d.Unit == "fresh" {
				rec = d
			}
		}
		drafts, err := f.cs.ListDraftBases(ctx, f.projectID, "main")
		require.NoError(t, err)
		mark := ""
		for _, d := range drafts {
			if d.Unit == "fresh" {
				mark = d.Basis
			}
		}
		return rec, mark
	}

	first := runJob("job-rejected-1")
	assert.Equal(t, 2, first.TotalBlocks, "the stale unit and the refused one are both owed")
	stored := f.storedFor(t)
	redrafted := stored[basisFreshSource].Block.TargetText("fr")
	assert.NotEqual(t, "Supprimer le compte", redrafted, "the wording the reviewer turned down is replaced")
	assert.Equal(t, "Enregistrer", stored[basisUnrecordedSource].Block.TargetText("fr"),
		"a target the platform has no record of writing is still left alone")

	rec, mark := freshRow()
	assert.Equal(t, venue.ReviewStateRejected, rec.ReviewState, "the verdict stays on the row")
	assert.Equal(t, "reviewer-1", rec.DecidedBy)
	assert.Equal(t, srcRevision(basisFreshSource), rec.Basis, "and its basis is never written over")
	assert.Equal(t, srcRevision(basisFreshSource), mark, "the draft is marked with the source it was made from")

	second := runJob("job-rejected-2")
	assert.Zero(t, second.TotalBlocks, "nothing is owed: the unit waits on the next verdict")
	assert.Equal(t, redrafted, f.storedFor(t)[basisFreshSource].Block.TargetText("fr"))

	f.reject(t, "fresh", basisFreshSource, redrafted)
	third := runJob("job-rejected-3")
	assert.Equal(t, 1, third.TotalBlocks, "a second rejection owes a second draft")
	fourth := runJob("job-rejected-4")
	assert.Zero(t, fourth.TotalBlocks, "and one only")
}
