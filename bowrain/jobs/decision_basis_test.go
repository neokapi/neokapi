package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	"github.com/neokapi/neokapi/bowrain/storage"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/state"
	"github.com/neokapi/neokapi/core/venue"
	fwmemory "github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// basisFixture is the three-way shape every test in this file needs, seeded in a
// real PostgreSQL store: one unit whose recorded basis names wording the source
// no longer carries (stale), one the ledger has no record of (unrecorded), and
// one whose basis is the source the project holds now (fresh). All three carry a
// French target.
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
		Properties:            map[string]string{"source_gate": "none"},
	}))

	blocks := []*model.Block{
		basisBlock("stale", basisStaleSource, "Sélecteur de couleur"),
		basisBlock("unrecorded", basisUnrecordedSource, "Enregistrer"),
		basisBlock("fresh", basisFreshSource, "Supprimer le compte"),
	}
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "ui.json", blocks))

	// The stale unit's record names a source wording the project has since
	// rewritten; the fresh unit's names the wording it still holds. The
	// unrecorded unit gets no record at all.
	_, err = cs.UpsertUnitDecisions(ctx, projectID, "main", []venue.UnitDecision{
		{
			ItemName:    "ui.json",
			Unit:        "stale",
			Variant:     "fr",
			TargetHash:  state.TargetHash("Sélecteur de couleur"),
			ContentHash: state.SourceHash("Colour picker (the wording before the fix)"),
			Updated:     "2026-01-01T00:00:00Z",
		},
		{
			ItemName:    "ui.json",
			Unit:        "fresh",
			Variant:     "fr",
			TargetHash:  state.TargetHash("Supprimer le compte"),
			ContentHash: state.SourceHash(basisFreshSource),
			Updated:     "2026-01-01T00:00:00Z",
		},
	})
	require.NoError(t, err)

	return basisFixture{db: db, cs: cs, projectID: projectID, item: "ui.json"}
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
	require.NotNil(t, ledger, "the fixture recorded two decisions")

	stored := f.storedFor(t)

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

	// With no ledger to read, every target reads as one the platform has no
	// record of, which is the honest answer and the one that changes nothing.
	assert.False(t, decisionLedger(nil).needsDraft(stored[basisStaleSource], "fr"))
	assert.True(t, decisionLedger(nil).needsDraft(stored[basisStaleSource], "de"))
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
	res, err := recycleBlocks(ctx, tm, rows, "en", "fr", 1.0, ledger)
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

	res, err := recycleBlocks(ctx, tm, []*venue.StoredBlock{stored[basisStaleSource]}, "en", "fr", 1.0, ledger)
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

	est, err := EstimateConvergence(ctx, f.cs, nil, proj)
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

// TestRecordProducedBasis pins what the convergence worker writes for the
// targets it produced: a basis for a unit it wrote, nothing over a decision, and
// nothing for a pairing already recorded.
func TestRecordProducedBasis(t *testing.T) {
	f := newBasisFixture(t)
	ctx := t.Context()
	stored := f.storedFor(t)

	// The unrecorded unit's translation is now the platform's own output, and the
	// fresh unit carries a decision.
	_, err := f.cs.UpsertUnitDecisions(ctx, f.projectID, "main", []venue.UnitDecision{{
		ItemName:    f.item,
		Unit:        "fresh",
		Variant:     "fr",
		Status:      string(model.TargetStatusReviewed),
		TargetHash:  state.TargetHash("Supprimer le compte"),
		ContentHash: state.SourceHash(basisFreshSource),
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
	recordProducedBasis(ctx, f.cs, f.projectID, "main", ledger, byBlockID, written, "fr")

	after, err := f.cs.ListUnitDecisions(ctx, f.projectID, "main")
	require.NoError(t, err)
	records := map[string]venue.UnitDecision{}
	for _, d := range after {
		records[d.Unit] = d
	}

	require.Contains(t, records, "unrecorded")
	assert.Equal(t, state.SourceHash(basisUnrecordedSource), records["unrecorded"].ContentHash,
		"the pass records the source it translated from")
	assert.Equal(t, state.TargetHash("Enregistrer"), records["unrecorded"].TargetHash)
	assert.Empty(t, records["unrecorded"].Status, "a basis claims no rung")
	assert.Empty(t, records["unrecorded"].ReviewState, "a basis is not a decision")

	assert.Equal(t, "approved", records["fresh"].ReviewState,
		"a decided unit keeps the reviewer's record")
	assert.Equal(t, "reviewer-1", records["fresh"].DecidedBy)

	// The target the pass wrote keeps the rung its producer put it on: a
	// status-less record projects nothing.
	sb, err := f.cs.GetBlock(ctx, f.projectID, "main", stored[basisUnrecordedSource].Block.ID)
	require.NoError(t, err)
	assert.Equal(t, "Enregistrer", sb.Block.TargetText("fr"))
}

// TestWorkerRecordsTheBasisOfWhatItDrafts drives the real translation worker over
// the fixture: the stale unit is re-drafted, the unrecorded and fresh ones are
// left alone, and the run records the source its draft was made from, so the
// next pass reads the unit as current rather than re-drafting it forever.
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
	records := map[string]venue.UnitDecision{}
	for _, d := range after {
		records[d.Unit] = d
	}
	require.Contains(t, records, "stale")
	assert.Equal(t, state.SourceHash(basisStaleSource), records["stale"].ContentHash,
		"the re-draft's basis is the source it was made from")
	assert.NotContains(t, records, "unrecorded",
		"the pass wrote nothing for that unit, so it claims nothing about it")

	// The second pass has nothing to do: the unit it re-drafted now reads current.
	ledger := loadDecisionLedger(ctx, f.cs, f.projectID, "main")
	for _, sb := range stored {
		assert.False(t, ledger.needsDraft(sb, "fr"),
			"every unit reads settled after the run: %s", sb.Block.SourceText())
	}
}
