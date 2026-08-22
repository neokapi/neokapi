package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	sqlterms "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	voicepg "github.com/neokapi/neokapi/bowrain/voice"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	fwterms "github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerVoiceContext_EndToEnd is the end-to-end proof of J-4 over the real
// PostgreSQL stores: a project whose Properties bind a voice profile
// (the project rung of the voicescope ladder) and whose workspace terms
// carries approved terminology produces a translate config that carries the
// rendered voice guide and the per-locale glossary — and the demo-provider
// translation job completes with them bound. A sibling project with neither
// runs bare through the very same worker path.
func TestWorkerVoiceContext_EndToEnd(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()

	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	js, err := NewJobStore(db)
	require.NoError(t, err)

	const (
		projectID = "proj-voicectx"
		wsSlug    = "acme"
	)

	// Real Postgres voice store with a profile the project binds explicitly.
	bs, err := voicepg.NewPostgresVoiceStore(db)
	require.NoError(t, err)
	profile := &coreprofile.VoiceProfile{
		Scope: "ws-1",
		Name:  "Acme Voice",
		Tone:  coreprofile.ToneProfile{Formality: "casual", Personality: []string{"friendly"}},
		Vocabulary: coreprofile.VocabularyRules{
			ForbiddenTerms: []coreprofile.TermRule{{Term: "utilize", Replacement: "use"}},
		},
	}
	require.NoError(t, bs.CreateProfile(ctx, profile))

	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID:                    projectID,
		Name:                  "Brand Context Test",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
		WorkspaceID:           "ws-1",
		Properties:            map[string]string{coreprofile.PropertyProfileID: profile.ID},
	}))
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "en.json", []*model.Block{
		model.NewBlock("b1", "Open the dashboard"),
	}))

	// Real Postgres terms (workspace-keyed, like the server's getTerms) with a
	// preferred fr rendering.
	tb, err := sqlterms.NewPostgresStoreFromDB(db, wsSlug)
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(ctx, fwterms.Concept{
		ID: "c-dashboard",
		Terms: []fwterms.Term{
			{Text: "dashboard", Locale: "en", Status: model.TermPreferred},
			{Text: "tableau de bord", Locale: "fr", Status: model.TermPreferred},
		},
	}))

	deps := &WorkerDeps{
		JobStore:      js,
		ContentStore:  cs,
		Platform:      &PlatformProviderConfig{Provider: "demo"},
		ProviderStore: &fakeProviderResolver{cfg: bstore.ProviderConfig{Type: "demo"}},
		VoiceStore:    bs,
		TermsResolver: TermsResolverFunc(func(slug string) (fwterms.Terminology, error) {
			return sqlterms.NewPostgresStoreFromDB(db, slug)
		}),
	}

	job := &TranslationJob{
		ID:               "job-voicectx",
		WorkspaceSlug:    wsSlug,
		WorkspaceID:      "ws-1",
		ProjectID:        projectID,
		ItemName:         "en.json",
		TargetLocale:     "fr",
		ProviderConfigID: "platform",
		Model:            "demo",
		Status:           StatusQueued,
	}

	// The constructed translate config carries the resolved brand context.
	proj, err := cs.GetProject(ctx, projectID)
	require.NoError(t, err)
	cfg := jobTranslateConfig(ctx, deps, job, proj)
	require.NotNil(t, cfg.Profile, "the project-bound profile must resolve")
	assert.Equal(t, "Acme Voice", cfg.Profile.Name)
	guide := coreprofile.RenderVoiceGuideCompact(cfg.Profile)
	assert.Contains(t, guide, "personality: friendly")
	assert.Contains(t, guide, "formality: casual")
	assert.Contains(t, guide, `"utilize" → "use"`)
	assert.Equal(t, []coreprofile.TermRule{{Term: "dashboard", Replacement: "tableau de bord"}}, cfg.TermRules)

	// And the full worker path completes a demo translation with them bound.
	require.NoError(t, js.CreateJob(ctx, job))
	claimed, epoch, err := js.ClaimJob(ctx, job.ID)
	require.NoError(t, err)
	require.True(t, claimed)
	require.NoError(t, executeTranslationWithDeps(ctx, deps, job, epoch))

	stored, err := cs.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: "main", ItemName: "en.json"})
	require.NoError(t, err)
	require.Len(t, stored, 1)
	assert.NotEmpty(t, stored[0].Block.TargetText("fr"), "the job must still produce a translation")

	// The drafted target leaves a deterministic voice score behind it (the
	// dashboard's compliance rate feeds on these): scored against the bound
	// profile at draft time, zero AI.
	scores, err := bs.GetScores(ctx, projectID, "fr")
	require.NoError(t, err)
	require.NotEmpty(t, scores, "a drafted block must persist a voice score")
	assert.Equal(t, stored[0].Block.ID, scores[0].BlockID,
		"the score references the STORED block id — the id the dashboard joins on")
	assert.Equal(t, profile.ID, scores[0].ProfileID)

	// Control: a project with no binding and no terminology runs bare through
	// the same config path.
	const bareID = "proj-barectx"
	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID:                    bareID,
		Name:                  "Bare Test",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
	}))
	bareProj, err := cs.GetProject(ctx, bareID)
	require.NoError(t, err)
	bareJob := &TranslationJob{ID: "job-bare", WorkspaceSlug: "no-terms", ProjectID: bareID, TargetLocale: "fr"}
	bareCfg := jobTranslateConfig(ctx, deps, bareJob, bareProj)
	assert.Nil(t, bareCfg.Profile, "no binding at any rung → no profile")
	assert.Nil(t, bareCfg.TermRules, "no terminology for the pair → no rules")
}
