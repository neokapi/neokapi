package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	sqlmemory "github.com/neokapi/neokapi/bowrain/memory"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	fwmemory "github.com/neokapi/neokapi/memory"
	fwterms "github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerRecycle_TermRulesDecideWhatIsRecycled runs the convergence job over
// real PostgreSQL stores. The content memory answers every block, and the
// workspace terms mandate "tableau de bord" for "dashboard". A match that breaks
// the rule is left for the drafter, a match that keeps it is recycled, and a
// block whose source uses no ruled term is recycled. A project in a workspace
// with no terms recycles all three.
func TestWorkerRecycle_TermRulesDecideWhatIsRecycled(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()

	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	js, err := NewJobStore(db)
	require.NoError(t, err)

	pairs := map[string]string{
		"Open the dashboard":  "Ouvrir le panneau",
		"Close the dashboard": "Fermer le tableau de bord",
		"Hello":               "Bonjour",
	}

	run := func(t *testing.T, projectID, wsSlug string, governed bool) (map[string]string, *TranslationJob, EstimateLocaleWork) {
		t.Helper()
		// No translate_after hold, so the estimate prices every block and its split can be
		// compared with the run's.
		require.NoError(t, cs.CreateProject(ctx, &store.Project{
			ID:                    projectID,
			Name:                  projectID,
			DefaultSourceLanguage: "en",
			TargetLanguages:       []model.LocaleID{"fr"},
			Properties:            map[string]string{store.TranslateAfterProperty: string(model.TranslateAfterNone)},
		}))
		require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "en.json", []*model.Block{
			model.NewBlock("b1", "Open the dashboard"),
			model.NewBlock("b2", "Close the dashboard"),
			model.NewBlock("b3", "Hello"),
		}))

		tm, err := sqlmemory.NewPostgresStoreFromDB(db, wsSlug)
		require.NoError(t, err)
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

		deps := &WorkerDeps{
			JobStore:      js,
			ContentStore:  cs,
			Platform:      &PlatformProviderConfig{Provider: "demo"},
			ProviderStore: &fakeProviderResolver{cfg: bstore.ProviderConfig{Type: "demo"}},
			MemoryResolver: MemoryResolverFunc(func(slug string) (fwmemory.Store, error) {
				return sqlmemory.NewPostgresStoreFromDB(db, slug)
			}),
		}
		var tb fwterms.Terminology
		if governed {
			pgTerms, err := sqlterms.NewPostgresStoreFromDB(db, wsSlug)
			require.NoError(t, err)
			require.NoError(t, pgTerms.AddConcept(ctx, fwterms.Concept{
				ID: "c-dashboard",
				Terms: []fwterms.Term{
					{Text: "dashboard", Locale: "en", Status: model.TermPreferred},
					{Text: "tableau de bord", Locale: "fr", Status: model.TermPreferred},
				},
			}))
			tb = pgTerms
			deps.TermsResolver = TermsResolverFunc(func(slug string) (fwterms.Terminology, error) {
				return sqlterms.NewPostgresStoreFromDB(db, slug)
			})
		}

		// The estimate is read before the run changes anything.
		proj, err := cs.GetProject(ctx, projectID)
		require.NoError(t, err)
		est, err := EstimateConvergence(ctx, cs, tm, tb, proj)
		require.NoError(t, err)
		require.Equal(t, 3, est.Source.Ready, "every block is priced")
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
		reloaded, err := js.GetJob(ctx, job.ID)
		require.NoError(t, err)
		return targets, reloaded, est.Locales[0]
	}

	t.Run("governed", func(t *testing.T) {
		targets, job, estimate := run(t, "proj-terms-governed", "governed", true)
		assert.NotEmpty(t, targets["Open the dashboard"], "the unit that broke the rule is drafted")
		assert.NotEqual(t, "Ouvrir le panneau", targets["Open the dashboard"], "the breaking match is not recycled")
		assert.Equal(t, "Fermer le tableau de bord", targets["Close the dashboard"], "the match that keeps the rule is recycled")
		assert.Equal(t, "Bonjour", targets["Hello"], "a block with no ruled term is recycled")
		assert.Equal(t, 2, job.ViaMemory)
		assert.Equal(t, 1, job.ViaAI)
		assert.Equal(t, 2, estimate.ViaMemory, "the estimate prices the breaking match as AI work")
		assert.Equal(t, 1, estimate.ViaAI)
	})

	t.Run("no terms", func(t *testing.T) {
		targets, job, estimate := run(t, "proj-terms-bare", "bare", false)
		assert.Equal(t, "Ouvrir le panneau", targets["Open the dashboard"], "with no governing terms every match is recycled")
		assert.Equal(t, "Fermer le tableau de bord", targets["Close the dashboard"])
		assert.Equal(t, "Bonjour", targets["Hello"])
		assert.Equal(t, 3, job.ViaMemory)
		assert.Zero(t, job.ViaAI)
		assert.Equal(t, 3, estimate.ViaMemory)
		assert.Zero(t, estimate.ViaAI)
	})
}
