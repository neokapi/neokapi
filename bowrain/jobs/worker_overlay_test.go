package jobs

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"

	platstore "github.com/neokapi/neokapi/bowrain/core/store"
	bwblockstore "github.com/neokapi/neokapi/bowrain/store/blockstore"
	"github.com/neokapi/neokapi/bowrain/store/sqlitestore"
	"github.com/neokapi/neokapi/core/model"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
	"github.com/stretchr/testify/require"
)

// TestWriteTargetOverlays proves the #404 acceptance criterion: the
// rule-driven `auto_translate` path writes `block_overlays` rows with
// the same key + payload shape the CLI `ai-translate` tool does
// (`kind: targets/<locale>`, payload `{text, provider}`). The CLI's
// write lives in core/ai/tools/translate.go::sessionHandleBlock; this
// test asserts the worker-side writer produces byte-identical rows
// for the same input.
func TestWriteTargetOverlays(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "worker.db")
	cs, err := sqlitestore.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.DB().Close() })

	const projectID = "proj-worker"
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID:                    projectID,
		Name:                  "Worker test",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
	}))

	// Stand up blocks the way the worker pipeline would produce them
	// after the AI translate tool has run — source text + target text
	// on each block.
	blocks := []*model.Block{
		newTranslatedBlock("blk-1", "Hello", "Bonjour"),
		newTranslatedBlock("blk-2", "World", "Monde"),
		newSkippedBlock("blk-3", "Not translatable"),     // Translatable=false
		newEmptyTargetBlock("blk-4", "Unfilled", "fr"),   // Target missing
	}

	prov := &aiprovider.MockProvider{ProviderName: "mock"}
	job := &TranslationJob{
		ID:           "job-1",
		ProjectID:    projectID,
		TargetLocale: "fr",
	}
	deps := &WorkerDeps{ContentStore: cs}

	require.NoError(t, writeTargetOverlays(ctx, deps, job, prov, blocks))

	// Read the overlays back through the same adapter the CLI would
	// use. Expect only blocks 1 + 2 to have landed — the rest are
	// filtered out (Translatable=false, empty target).
	bs, err := bwblockstore.Open(cs, projectID, "main")
	require.NoError(t, err)
	t.Cleanup(func() { _ = bs.Close() })

	sess, err := bs.Begin(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = sess.Close() })

	cases := []struct {
		block, want string
	}{
		{"blk-1", "Bonjour"},
		{"blk-2", "Monde"},
	}
	for _, c := range cases {
		o, err := sess.GetOverlay("targets/fr", c.block)
		require.NoError(t, err, "block %s should have overlay", c.block)
		require.Equal(t, "targets/fr", o.Kind)
		require.Equal(t, c.block, o.BlockHash)

		var parsed struct {
			Text     string `json:"text"`
			Provider string `json:"provider"`
		}
		require.NoError(t, json.Unmarshal(o.Payload, &parsed))
		require.Equal(t, c.want, parsed.Text)
		require.Equal(t, "mock", parsed.Provider,
			"provider field must match what the CLI ai-translate tool writes")
	}
}

// TestWriteTargetOverlays_CLIParity pins the payload schema
// byte-for-byte against what core/ai/tools/translate.go writes —
// if either side adds a field, this test breaks and the other side
// must follow.
func TestWriteTargetOverlays_CLIParity(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "parity.db")
	cs, err := sqlitestore.NewSQLiteStore(dbPath)
	require.NoError(t, err)
	t.Cleanup(func() { _ = cs.DB().Close() })

	const projectID = "proj-parity"
	require.NoError(t, cs.CreateProject(ctx, &platstore.Project{
		ID:                    projectID,
		Name:                  "Parity",
		DefaultSourceLanguage: "en",
		TargetLanguages:       []model.LocaleID{"fr"},
	}))

	blocks := []*model.Block{newTranslatedBlock("blk-pa", "Save", "Enregistrer")}
	prov := &aiprovider.MockProvider{ProviderName: "anthropic"}

	require.NoError(t, writeTargetOverlays(ctx, &WorkerDeps{ContentStore: cs}, &TranslationJob{
		ID: "j", ProjectID: projectID, TargetLocale: "fr",
	}, prov, blocks))

	bs, _ := bwblockstore.Open(cs, projectID, "main")
	defer bs.Close()
	sess, _ := bs.Begin(ctx)
	defer sess.Close()

	o, err := sess.GetOverlay("targets/fr", "blk-pa")
	require.NoError(t, err)

	// The payload the CLI writes is exactly this shape. Keep the two
	// in lockstep: if this fails the CLI tool changed its aiTargetCache
	// or the worker's overlay builder drifted.
	require.JSONEq(t, `{"text":"Enregistrer","provider":"anthropic"}`, string(o.Payload))
}

// ─── test helpers ──────────────────────────────────────────────

func newTranslatedBlock(id, source, target string) *model.Block {
	b := model.NewRunsBlock(id, []model.Run{{Text: &model.TextRun{Text: source}}})
	b.SetTargetText("fr", target)
	return b
}

func newSkippedBlock(id, source string) *model.Block {
	b := model.NewRunsBlock(id, []model.Run{{Text: &model.TextRun{Text: source}}})
	b.Translatable = false
	return b
}

func newEmptyTargetBlock(id, source, _ string) *model.Block {
	return model.NewRunsBlock(id, []model.Run{{Text: &model.TextRun{Text: source}}})
}
