package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	voicepg "github.com/neokapi/neokapi/bowrain/voice"
	"github.com/neokapi/neokapi/core/model"
	coreprofile "github.com/neokapi/neokapi/core/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestPersistDraftVoiceScores covers the worker's zero-AI draft scoring: only
// translatable blocks with a target are scored, a forbidden-vocabulary hit
// drops the score below the default on-brand bar, and nil profile / nil brand
// store are safe no-ops.
func TestPersistDraftVoiceScores(t *testing.T) {
	db := pgtest.NewTestDB(t)
	bs, err := voicepg.NewPostgresVoiceStore(db)
	require.NoError(t, err)
	ctx := t.Context()

	profile := &coreprofile.VoiceProfile{
		Scope: "ws-1",
		Name:  "Voice",
		Vocabulary: coreprofile.VocabularyRules{
			ForbiddenTerms: []coreprofile.TermRule{{Term: "synergy", Replacement: "teamwork", Severity: "critical"}},
		},
	}
	require.NoError(t, bs.CreateProfile(ctx, profile))

	deps := &WorkerDeps{VoiceStore: bs}
	job := &TranslationJob{ID: "job-vs", ProjectID: "proj-vs", TargetLocale: "fr"}

	clean := model.NewBlock("b-clean", "Hello")
	clean.SetTargetText("fr", "Bonjour")
	offBrand := model.NewBlock("b-off", "Teamwork wins")
	offBrand.SetTargetText("fr", "La synergy des équipes gagne")
	untranslated := model.NewBlock("b-untranslated", "No target yet")
	nonTranslatable := model.NewBlock("b-data", "raw")
	nonTranslatable.Translatable = false
	nonTranslatable.SetTargetText("fr", "raw")

	persistDraftVoiceScores(ctx, deps, job, profile,
		[]*model.Block{clean, offBrand, untranslated, nonTranslatable, nil}, "fr")

	scores, err := bs.GetScores(ctx, "proj-vs", "fr")
	require.NoError(t, err)
	byBlock := map[string]*coreprofile.StoredScore{}
	for _, sc := range scores {
		byBlock[sc.BlockID] = sc
	}
	require.Len(t, byBlock, 2, "only translatable blocks with a target are scored")

	require.NotNil(t, byBlock["b-clean"])
	assert.Equal(t, 100, byBlock["b-clean"].Score, "no vocabulary hits → full score")
	assert.Equal(t, profile.ID, byBlock["b-clean"].ProfileID)

	off := byBlock["b-off"]
	require.NotNil(t, off)
	assert.Less(t, off.Score, coreprofile.DefaultMinScore,
		"a critical vocabulary hit drops the draft below the default on-brand bar")
	assert.NotEmpty(t, off.Findings, "the persisted score carries the findings behind it")

	// Empty locale reads project-wide (all locales) — the shape the score
	// endpoints and brand rollup consume.
	all, err := bs.GetScores(ctx, "proj-vs", "")
	require.NoError(t, err)
	assert.Len(t, all, 2)

	// Safe no-ops: nil profile, brand-store-less deps, nil deps.
	persistDraftVoiceScores(ctx, deps, job, nil, []*model.Block{clean}, "fr")
	persistDraftVoiceScores(ctx, &WorkerDeps{}, job, profile, []*model.Block{clean}, "fr")
	persistDraftVoiceScores(ctx, nil, job, profile, []*model.Block{clean}, "fr")
	scores, err = bs.GetScores(ctx, "proj-vs", "fr")
	require.NoError(t, err)
	assert.Len(t, scores, 2, "no-op paths persist nothing")
}
