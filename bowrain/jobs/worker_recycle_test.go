package jobs

import (
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
	sqltm "github.com/neokapi/neokapi/bowrain/sievepen"
	bstore "github.com/neokapi/neokapi/bowrain/store"
	"github.com/neokapi/neokapi/bowrain/testutil/pgtest"
	"github.com/neokapi/neokapi/core/model"
	fwsievepen "github.com/neokapi/neokapi/sievepen"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestWorkerRecycle_FillsFromTMOnlyAITranslatesRemainder is the end-to-end proof
// of verification (a): a convergence translation job with TM entries fills the
// matching blocks from the project TM and sends only the remainder to the AI
// translator — and records a truthful ViaTM/ViaAI split on the job row (the
// input to the produce emitter's "TM N · AI M").
func TestWorkerRecycle_FillsFromTMOnlyAITranslatesRemainder(t *testing.T) {
	db := pgtest.NewTestDB(t)
	ctx := t.Context()

	cs, err := bstore.NewPostgresStoreFromDB(db)
	require.NoError(t, err)
	js, err := NewJobStore(db)
	require.NoError(t, err)

	const (
		projectID = "proj-recycle"
		wsSlug    = "acme"
	)
	require.NoError(t, cs.CreateProject(ctx, &store.Project{
		ID:                    projectID,
		Name:                  "Recycle Test",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
	}))
	// Two source blocks; only "Hello" has a TM match.
	require.NoError(t, cs.StoreBlocksForItem(ctx, projectID, "main", "en.json", []*model.Block{
		model.NewBlock("b1", "Hello"),
		model.NewBlock("b2", "Brand new string"),
	}))

	// Seed the project TM (workspace-scoped) with the "Hello" pair.
	tm, err := sqltm.NewPostgresTMFromDB(db, wsSlug)
	require.NoError(t, err)
	require.NoError(t, tm.Add(ctx, fwsievepen.TMEntry{
		ID: "e-hello",
		Variants: map[model.LocaleID][]model.Run{
			"en": {{Text: &model.TextRun{Text: "Hello"}}},
			"fr": {{Text: &model.TextRun{Text: "Bonjour"}}},
		},
		HintSrcLang: "en",
	}))

	deps := &WorkerDeps{
		JobStore:      js,
		ContentStore:  cs,
		Platform:      &PlatformProviderConfig{Provider: "demo"},
		ProviderStore: &fakeProviderResolver{cfg: bstore.ProviderConfig{Type: "demo"}},
		TMResolver: TMResolverFunc(func(slug string) (fwsievepen.TMStore, error) {
			return sqltm.NewPostgresTMFromDB(db, slug)
		}),
	}

	job := &TranslationJob{
		ID:               "job-recycle",
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

	// The TM-matched block carries the recycled target verbatim; the other block
	// carries an AI (demo) draft that is NOT the TM value. Read back by item
	// (StoreBlocksForItem remaps source IDs to internal IDs) and key on the
	// source text.
	stored, err := cs.GetBlocks(ctx, store.BlockQuery{ProjectID: projectID, Stream: "main", ItemName: "en.json"})
	require.NoError(t, err)
	targets := map[string]string{}
	for _, sb := range stored {
		targets[sb.Block.SourceText()] = sb.Block.TargetText("fr")
	}
	assert.Equal(t, "Bonjour", targets["Hello"], "the TM-matched block must be recycled from TM, not AI")
	assert.NotEmpty(t, targets["Brand new string"], "the unmatched block must be AI-translated")
	assert.NotEqual(t, "Bonjour", targets["Brand new string"])

	// The job records the truthful split: 1 via TM, 1 via AI (verification (c)).
	reloaded, err := js.GetJob(ctx, job.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, reloaded.ViaTM, "one block recycled from TM → ViaTM=1")
	assert.Equal(t, 1, reloaded.ViaAI, "one block sent to AI → ViaAI=1")
}
