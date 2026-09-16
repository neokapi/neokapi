package jobs

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/bowrain/core/store"
	sqlmemory "github.com/neokapi/neokapi/bowrain/memory"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	fwmemory "github.com/neokapi/neokapi/memory"
	fwterms "github.com/neokapi/neokapi/terms"
)

// TestWorkerRecycle_DoNotTranslateDecidesWhatIsRecycled runs the convergence job
// over real PostgreSQL stores with a workspace concept marked do-not-translate.
// The content memory answers every block, and one of its answers translates the
// product name. That match breaks the concept, so it is left for the drafter
// rather than filled; a match that keeps the term verbatim is recycled, and so
// is a block whose source never uses it.
//
// The estimate is read before the run and must split the work the same way, or
// a project is quoted for work the run does not do.
func TestWorkerRecycle_DoNotTranslateDecidesWhatIsRecycled(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()

	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	js, err := NewJobStore(db)
	require.NoError(t, err)

	const projectID, wsSlug = "proj-dnt-recycle", "dnt-recycle"

	// No source gate, so the estimate prices every block and its split can be
	// compared with the run's.
	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID:                    projectID,
		Name:                  projectID,
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
		Properties:            map[string]string{store.SourceGateProperty: string(model.SourceGateNone)},
	}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "en.json", []*model.Block{
		model.NewBlock("b1", "Open kapi to begin"),
		model.NewBlock("b2", "Save in kapi"),
		model.NewBlock("b3", "Hello"),
	}))

	tm, err := sqlmemory.NewPostgresStoreFromDB(db, wsSlug)
	require.NoError(t, err)
	pairs := map[string]string{
		// Translates the product name: this is the match the concept forbids.
		"Open kapi to begin": "Ouvrir capi pour commencer",
		"Save in kapi":       "Enregistrer dans kapi",
		"Hello":              "Bonjour",
	}
	for source, target := range pairs {
		require.NoError(t, tm.Add(ctx, fwmemory.Entry{
			ID: "e-" + source,
			Variants: map[model.LocaleID][]model.Run{
				"en": {{Text: &model.TextRun{Text: source}}},
				"fr": {{Text: &model.TextRun{Text: target}}},
			},
			HintSrcLang: "en",
		}))
	}

	pgTerms, err := sqlterms.NewPostgresStoreFromDB(db, wsSlug)
	require.NoError(t, err)
	require.NoError(t, pgTerms.AddConcept(ctx, fwterms.Concept{
		ID:             "c-kapi",
		DoNotTranslate: true,
		Terms:          []fwterms.Term{{Text: "kapi", Locale: "en", Status: model.TermPreferred}},
	}))

	deps := &WorkerDeps{
		JobStore:      js,
		ContentStore:  cs,
		Platform:      &PlatformProviderConfig{Provider: "demo"},
		ProviderStore: &fakeProviderResolver{cfg: bstore.ProviderConfig{Type: "demo"}},
		MemoryResolver: MemoryResolverFunc(func(slug string) (fwmemory.Store, error) {
			return sqlmemory.NewPostgresStoreFromDB(db, slug)
		}),
		TermsResolver: TermsResolverFunc(func(slug string) (fwterms.Terminology, error) {
			return sqlterms.NewPostgresStoreFromDB(db, slug)
		}),
	}

	proj, err := cs.GetProject(ctx, projectID)
	require.NoError(t, err)
	est, err := EstimateConvergence(ctx, cs, tm, pgTerms, proj)
	require.NoError(t, err)
	require.Len(t, est.Locales, 1)

	job := &TranslationJob{
		ID:               "job-" + projectID,
		WorkspaceSlug:    wsSlug,
		ProjectID:        projectID,
		ItemName:         "en.json",
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

	stored, err := cs.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: "main", ItemName: "en.json"})
	require.NoError(t, err)
	targets := map[string]string{}
	for _, sb := range stored {
		targets[sb.Block.SourceText()] = sb.Block.TargetText("fr")
	}

	assert.NotEmpty(t, targets["Open kapi to begin"], "the unit that breaks the concept is drafted")
	assert.NotEqual(t, "Ouvrir capi pour commencer", targets["Open kapi to begin"],
		"a match that translates a do-not-translate term is not recycled")
	assert.Equal(t, "Enregistrer dans kapi", targets["Save in kapi"],
		"a match that keeps the term verbatim is recycled")
	assert.Equal(t, "Bonjour", targets["Hello"],
		"a block whose source never uses the term is recycled")

	reloaded, err := js.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, 2, reloaded.ViaMemory)
	assert.Equal(t, 1, reloaded.ViaAI)

	assert.Equal(t, 2, est.Locales[0].ViaMemory, "the estimate prices the breaking match as AI work")
	assert.Equal(t, 1, est.Locales[0].ViaAI)
}
