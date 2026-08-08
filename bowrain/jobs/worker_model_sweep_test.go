package jobs

import (
	"fmt"
	"testing"
	"time"

	brandpg "github.com/neokapi/neokapi/bowrain/brand"
	"github.com/neokapi/neokapi/bowrain/core/store"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	sqltb "github.com/neokapi/neokapi/bowrain/terms"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	brand "github.com/neokapi/neokapi/core/profile"
	fwterms "github.com/neokapi/neokapi/terms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestModelSweep_EndToEnd is the demo-provider proof of measured steerability
// over the real PostgreSQL stores: a seeded project (bound voice profile,
// workspace terms terminology, DNT terms, trap-bearing content) is swept
// through the durable worker path — both arms per candidate model — and the
// deterministic scorers produce a positive context lift (the context arm's
// enforced DNT masking preserves the product name the bare arm mangles),
// persisted rows per (project, locale, model, fixture_digest), and a
// deterministic recommendation.
func TestModelSweep_EndToEnd(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()

	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	js, err := NewJobStore(db)
	require.NoError(t, err)
	sweeps, err := NewModelSweepStore(db)
	require.NoError(t, err)
	quota, err := NewQuotaStore(db)
	require.NoError(t, err)

	const (
		projectID = "proj-sweep"
		wsSlug    = "acme-sweep"
		wsID      = "ws-sweep"
	)

	// Brand voice profile bound at the project rung: a competitor term makes
	// vocabulary measurable; the profile itself is part of the fixture digest.
	bs, err := brandpg.NewPostgresBrandStore(db)
	require.NoError(t, err)
	profile := &brand.VoiceProfile{
		Scope: wsID,
		Name:  "Acme Voice",
		Vocabulary: brand.VocabularyRules{
			CompetitorTerms: []brand.TermRule{{Term: "Localizely"}},
		},
	}
	require.NoError(t, bs.CreateProfile(ctx, profile))

	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID:                    projectID,
		Name:                  "Sweep Test",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
		WorkspaceID:           wsID,
		Properties: map[string]string{
			brand.PropertyProfileID: profile.ID,
			// The enforced do-not-translate product name — a real-word brand
			// name ("Send", like Slack or Sonos) is exactly the DNT case: any
			// translator, the demo stub's lexicon included, will happily render
			// it ("envoyer") unless the span is locked. The context arm's
			// mask/restore protects it; the bare arm demonstrably loses it.
			"dnt_terms": "Send",
		},
	}))

	// Workspace terms: "save" → "enregistrer" is deliberately a rendering
	// the demo lexicon also produces, so glossary fixtures measure the check
	// plumbing without depending on prompt adherence a stub cannot offer.
	tb, err := sqltb.NewPostgresStoreFromDB(db, wsSlug)
	require.NoError(t, err)
	require.NoError(t, tb.AddConcept(ctx, fwterms.Concept{
		ID: "c-save",
		Terms: []fwterms.Term{
			{Text: "save", Locale: "en", Status: model.TermPreferred},
			{Text: "enregistrer", Locale: "fr", Status: model.TermPreferred},
		},
	}))

	// Trap-bearing content: 6 glossary traps, 4 voice traps, 4 DNT traps.
	var blocks []*model.Block
	for i := range 6 {
		blocks = append(blocks, model.NewBlock(fmt.Sprintf("gloss-%d", i),
			fmt.Sprintf("Save the document %d", i)))
	}
	for i := range 4 {
		blocks = append(blocks, model.NewBlock(fmt.Sprintf("voice-%d", i),
			fmt.Sprintf("Faster than Localizely for job %d", i)))
	}
	for i := range 4 {
		blocks = append(blocks, model.NewBlock(fmt.Sprintf("zdnt-%d", i),
			fmt.Sprintf("Open Send panel %d", i)))
	}
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "en.json", blocks))

	settings := &fakeSweepSettings{enabled: true, models: []string{"demo-a", "demo-b"}}
	deps := &WorkerDeps{
		JobStore:      js,
		ContentStore:  cs,
		QuotaStore:    quota,
		SweepStore:    sweeps,
		SweepSettings: settings,
		BrandStore:    bs,
		Platform:      &PlatformProviderConfig{Provider: "demo"},
		TermsResolver: TermsResolverFunc(func(slug string) (fwterms.Terminology, error) {
			return sqltb.NewPostgresStoreFromDB(db, slug)
		}),
	}

	job := NewModelSweepJob(wsID, wsSlug, projectID, "fr")
	require.NoError(t, js.CreateJob(ctx, job))
	require.NoError(t, processJobWithDeps(ctx, deps, job.ID))

	got, err := js.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, got.Status)

	// One row per candidate model, all keyed to the same fixture digest.
	rows, err := sweeps.LatestModelSweepResults(ctx, projectID, "fr")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	assert.Equal(t, rows[0].FixtureDigest, rows[1].FixtureDigest)
	for _, r := range rows {
		assert.Equal(t, 14, r.FixtureCount, "6 glossary + 4 voice + 4 dnt traps")
		// The context arm's DNT mask/restore preserves "Send"; the bare arm
		// lets the stub translate it away — a genuine, deterministic context
		// lift measured by the real dnt-check.
		assert.Greater(t, r.Lift, 0.0, "context arm must beat bare on the DNT traps")
		assert.GreaterOrEqual(t, r.Adherence, RecommendAdherenceFloor)
		assert.Less(t, r.AdherenceBare, r.Adherence)
		assert.Positive(t, r.TokensUsed)
		assert.WithinDuration(t, time.Now().UTC(), r.MeasuredAt, time.Minute)
	}

	// The recommendation is deterministic: identical scores tie-break to the
	// lexicographically first candidate.
	best := RecommendModel(rows)
	require.NotNil(t, best)
	assert.Equal(t, "demo-a", best.Model)

	// Platform QC contract: usage was recorded under the distinct operation…
	usage, err := quota.GetUsageByModel(ctx, wsID, time.Now().Add(-time.Hour), time.Now().Add(time.Hour))
	require.NoError(t, err)
	ops := map[string]bool{}
	for _, u := range usage {
		ops[u.Operation] = true
	}
	assert.True(t, ops["model_sweep"], "sweep usage must land in ai_usage as operation=model_sweep")

	// …and the sweep never wrote a target into the project's content.
	stored, err := cs.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: "main"})
	require.NoError(t, err)
	require.NotEmpty(t, stored)
	for _, sb := range stored {
		assert.False(t, sb.Block.HasTarget("fr"),
			"sweep translations exist only to be measured, never persisted: %s", sb.Block.ID)
	}
}

// TestModelSweep_DisabledFlagDropsJob proves the execution-side gate: a sweep
// job that reaches the worker with model_sweeps.enabled off completes as a
// no-op and records nothing.
func TestModelSweep_DisabledFlagDropsJob(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()

	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	js, err := NewJobStore(db)
	require.NoError(t, err)
	sweeps, err := NewModelSweepStore(db)
	require.NoError(t, err)

	deps := &WorkerDeps{
		JobStore:      js,
		ContentStore:  cs,
		SweepStore:    sweeps,
		SweepSettings: &fakeSweepSettings{enabled: false, models: []string{"demo-a"}},
		Platform:      &PlatformProviderConfig{Provider: "demo"},
	}

	job := NewModelSweepJob("ws1", "acme", "proj-x", "fr")
	require.NoError(t, js.CreateJob(ctx, job))
	require.NoError(t, processJobWithDeps(ctx, deps, job.ID))

	got, err := js.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, StatusCompleted, got.Status)

	rows, err := sweeps.LatestModelSweepResults(ctx, "proj-x", "fr")
	require.NoError(t, err)
	assert.Empty(t, rows)
}

// TestResolveProvider_MeasuredRecommendation proves the EV-4 consumption
// contract on the worker's platform model resolution: a fresh measured
// recommendation steers the platform model ONLY while the flag is on and the
// workspace has not pinned a model. The demo provider echoes its configured
// model, which makes the resolved choice observable.
func TestResolveProvider_MeasuredRecommendation(t *testing.T) {
	ctx := t.Context()
	freshRows := []*ModelSweepResult{{
		Model: "demo-rec", FixtureCount: 12, Adherence: 0.9, Lift: 0.4,
		MeasuredAt: time.Now().UTC(),
	}}
	job := &TranslationJob{
		ID: "job-1", ProjectID: "p1", TargetLocale: "fr",
		ProviderConfigID: "platform", Model: "demo-default",
	}

	resolvedModel := func(deps *WorkerDeps) string {
		t.Helper()
		resolved, err := resolveProvider(ctx, deps, job)
		require.NoError(t, err)
		resp, err := resolved.LLM.Chat(ctx, nil)
		require.NoError(t, err)
		return resp.Model
	}

	// Flag ON, model not pinned → the measured recommendation wins.
	deps := &WorkerDeps{
		Platform: &PlatformProviderConfig{Provider: "demo"},
		ModelRecommender: &SweepModelRecommender{
			Store:    &fakeSweepStore{rows: freshRows},
			Settings: &fakeSweepSettings{enabled: true, models: []string{"demo-rec"}},
		},
	}
	assert.Equal(t, "demo-rec", resolvedModel(deps))

	// Flag OFF → the recommendation is ignored; the job's model resolves as
	// before.
	deps.ModelRecommender = &SweepModelRecommender{
		Store:    &fakeSweepStore{rows: freshRows},
		Settings: &fakeSweepSettings{enabled: false, models: []string{"demo-rec"}},
	}
	assert.Equal(t, "demo-default", resolvedModel(deps))

	// Workspace pinned a model (customer model choice) → its pick always wins.
	deps.Platform = &PlatformProviderConfig{Provider: "demo", Model: "demo-pinned", ModelPinned: true}
	deps.ModelRecommender = &SweepModelRecommender{
		Store:    &fakeSweepStore{rows: freshRows},
		Settings: &fakeSweepSettings{enabled: true, models: []string{"demo-rec"}},
	}
	assert.Equal(t, "demo-pinned", resolvedModel(deps))
}
