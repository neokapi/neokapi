package host

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/projector"
	"github.com/neokapi/neokapi/core/redaction"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
)

// stampedProject is a one-file project whose block store holds a French
// translation of each of its three blocks: one stamped with the block's
// source, one stamped with a source the block no longer holds, and one with
// no stamp.
func stampedProject(t *testing.T) (*App, string, string, blockstore.Store) {
	t.Helper()
	a, recipe, dir := newProcessOnlyProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"),
		[]byte(`{"current":"Hello world","moved":"Goodbye for now","unstamped":"Thank you"}`), 0o644))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(t.Context(), layout.Root)
	require.NoError(t, err)
	store := db.BlocksAutocommit()

	blocks := readStampedBlocks(t, a, dir)
	put := func(name, text, stamp string) {
		t.Helper()
		b := blocks[name]
		require.NotNil(t, b, "block %s", name)
		payload, err := json.Marshal(blockstore.TargetOverlay{Text: text, Status: string(model.TargetStatusDraft), Source: stamp})
		require.NoError(t, err)
		sess, err := store.Begin(t.Context())
		require.NoError(t, err)
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{
			Kind:      blockstore.TargetOverlayKind("fr"),
			BlockHash: blockstore.StoreKey(filepath.Join("src", "en.json"), b.ID, b.SourceText()),
			Payload:   payload,
		}))
		require.NoError(t, sess.Commit())
	}
	put("current", "Bonjour le monde", blockstore.SourceStamp("Hello world"))
	put("moved", "Au revoir", blockstore.SourceStamp("Goodbye"))
	put("unstamped", "Merci", "")
	return a, recipe, dir, store
}

// readStampedBlocks reads the project's source the way the merge does and
// returns its blocks by name.
func readStampedBlocks(t *testing.T, a *App, dir string) map[string]*model.Block {
	t.Helper()
	blocks, _, err := project.ReadSourceBlocks(t.Context(), a.FormatReg, "json", filepath.Join(dir, "src", "en.json"), "en", "fr", nil)
	require.NoError(t, err)
	byName := map[string]*model.Block{}
	for _, b := range blocks {
		byName[b.Name] = b
	}
	return byName
}

// A merge from the project store writes a stored translation whose stamp
// names the block's source and one with no stamp, and counts one whose stamp
// names another source as stale and leaves that block in the source language.
func TestMaterialize_AStaleSourceStampIsCountedAndNotWritten(t *testing.T) {
	a, recipe, dir, _ := stampedProject(t)
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	written, stale, err := a.materializeProject(context.Background(), io.Discard, proj, recipe, []model.LocaleID{"fr"}, true, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, written)
	assert.Equal(t, 1, stale)

	out, err := os.ReadFile(filepath.Join(dir, "src", "fr.json"))
	require.NoError(t, err)
	var got map[string]string
	require.NoError(t, json.Unmarshal(out, &got))
	assert.Equal(t, "Bonjour le monde", got["current"], "a current stamp applies")
	assert.Equal(t, "Merci", got["unstamped"], "a stampless overlay applies")
	assert.Equal(t, "Goodbye for now", got["moved"], "a stale overlay is not written; the block keeps its source")
	assert.NotContains(t, string(out), "Au revoir")
}

// The merge's write to the content memory leaves a stale translation out: the
// pair of the current source with the translation of an older one is not one
// the project made.
func TestAbsorbStoreTargets_LeavesAStaleSourceStampOut(t *testing.T) {
	a, recipe, dir, store := stampedProject(t)
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	w, err := a.Projector(t.Context(), layout.Root)
	require.NoError(t, err)
	absorber := a.newMemoryAbsorber(w.With(projector.Origin{By: "merge"}).Memory())
	require.NotNil(t, absorber)

	ctx := blockstore.WithSourceRel(context.Background(), filepath.Join("src", "en.json"))
	n, u, err := absorbStoreTargets(ctx, a.FormatReg, "json", filepath.Join(dir, "src", "en.json"), "en", "fr", store, nil, absorber, filepath.Join("src", "en.json"), nil)
	require.NoError(t, err)
	assert.Equal(t, 2, n+u, "the current and the stampless translation are absorbed")
	for _, e := range absorber.entries {
		assert.NotEqual(t, "Au revoir", e.VariantText("fr"), "the stale translation is not absorbed")
	}
}

// The `.kpz` merge step applies a current and a stampless overlay and counts
// a stale one without applying it.
func TestHydrateTargets_AStaleSourceStampIsCountedAndNotApplied(t *testing.T) {
	ctx := context.Background()
	sess, err := blockstore.NewMemoryStore().Begin(ctx)
	require.NoError(t, err)
	put := func(id, text, stamp string) {
		payload, err := json.Marshal(blockstore.TargetOverlay{Text: text, Source: stamp})
		require.NoError(t, err)
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: blockstore.TargetOverlayKind("fr"), BlockHash: id, Payload: payload}))
	}
	put("current", "Bonjour", blockstore.SourceStamp("Hello"))
	put("moved", "Au revoir", blockstore.SourceStamp("Goodbye"))
	put("unstamped", "Merci", "")

	blocks := []*model.Block{
		model.NewBlock("current", "Hello"),
		model.NewBlock("moved", "Goodbye for now"),
		model.NewBlock("unstamped", "Thank you"),
	}
	tl := newHydrateTargetsTool("fr")
	in := make(chan *model.Part, len(blocks))
	out := make(chan *model.Part, len(blocks))
	for _, b := range blocks {
		b.Translatable = true
		in <- &model.Part{Type: model.PartBlock, Resource: b}
	}
	close(in)
	require.NoError(t, tl.SessionProcess(ctx, sess, in, out))

	assert.Equal(t, "Bonjour", blocks[0].TargetText("fr"))
	assert.False(t, blocks[1].HasTarget("fr"), "a stale overlay is not applied")
	assert.Equal(t, "Merci", blocks[2].TargetText("fr"))
	assert.Equal(t, 1, tl.Stale())

	// A skeleton-only workspace rebuilds each block from its id alone, so the
	// stamp has no source to be compared with and the overlay applies.
	skeleton := model.NewBlock("moved", "")
	skeleton.Translatable = true
	tl = newHydrateTargetsTool("fr")
	in = make(chan *model.Part, 1)
	out = make(chan *model.Part, 1)
	in <- &model.Part{Type: model.PartBlock, Resource: skeleton}
	close(in)
	require.NoError(t, tl.SessionProcess(ctx, sess, in, out))
	assert.Equal(t, "Au revoir", skeleton.TargetText("fr"))
	assert.Zero(t, tl.Stale())
}

// A flow that redacts the source before it translates and restores it after
// commits translations the merge writes: the stamp names the source as read,
// so the redacted text the translation was made from does not read as a
// changed source. An edit to one source block afterwards makes that block's
// translation stale: the merge counts it and writes the others, and a target
// file written afresh holds that block in the source language.
func TestProcessOnlyRedactedFlowThenMerge_EveryTranslationLandsUntilTheSourceMoves(t *testing.T) {
	a, recipe, dir := newProcessOnlyProject(t)
	src := filepath.Join(dir, "src", "en.json")
	require.NoError(t, os.WriteFile(src,
		[]byte(`{"greeting":"Hello Mr Bean","farewell":"Goodbye Mr Bean"}`), 0o644))

	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(t.Context(), layout.Root)
	require.NoError(t, err)

	redact, err := tools.NewRedactTool(&tools.RedactConfig{
		Detectors: []string{tools.DetectRules},
		Rules:     []redaction.Rule{{Term: "Mr Bean", Category: "person"}},
	})
	require.NoError(t, err)
	pseudo, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": "fr"}, "fr")
	require.NoError(t, err)
	unredact, err := tools.NewUnredactTool(&tools.UnredactConfig{})
	require.NoError(t, err)
	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg: a.FormatReg, SourceLocale: "en", Store: db.BlocksAutocommit(), ProjectRoot: dir,
	})
	require.NoError(t, runner.RunFileProcessOnly(context.Background(), "redacted-translate",
		[]tool.Tool{redact, pseudo, unredact}, filepath.Join("src", "en.json"), "fr"))

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	merge := func() (map[string]string, int) {
		t.Helper()
		_, stale, err := a.materializeProject(context.Background(), io.Discard, proj, recipe, []model.LocaleID{"fr"}, true, nil)
		require.NoError(t, err)
		out, err := os.ReadFile(filepath.Join(dir, "src", "fr.json"))
		require.NoError(t, err)
		var got map[string]string
		require.NoError(t, json.Unmarshal(out, &got))
		return got, stale
	}

	got, stale := merge()
	assert.Zero(t, stale)
	for key, source := range map[string]string{"greeting": "Hello Mr Bean", "farewell": "Goodbye Mr Bean"} {
		assert.NotEqual(t, source, got[key], "%s: the translation lands", key)
		assert.Contains(t, got[key], "Mr Bean", "%s: the redacted value is restored", key)
	}
	farewell := got["farewell"]

	require.NoError(t, os.WriteFile(src,
		[]byte(`{"greeting":"Hello again Mr Bean","farewell":"Goodbye Mr Bean"}`), 0o644))
	got, stale = merge()
	assert.Equal(t, 1, stale)
	assert.Equal(t, farewell, got["farewell"])

	// The merge leaves what an existing target file holds for a block it
	// writes nothing for, so the stale translation is visible as not written
	// in a target file written afresh.
	require.NoError(t, os.Remove(filepath.Join(dir, "src", "fr.json")))
	got, stale = merge()
	assert.Equal(t, 1, stale)
	assert.Equal(t, "Hello again Mr Bean", got["greeting"], "the stale translation is not written")
	assert.Equal(t, farewell, got["farewell"])
}

// The `.kpz` round trip of a flow that redacts before it translates and
// restores after: extract, transform the workspace with the flow, merge, and
// every translation lands with the withheld value restored.
func TestKpzRedactedFlowThenMerge_EveryTranslationLands(t *testing.T) {
	t.Setenv("KAPI_KPZ_CACHE", t.TempDir())
	dir := t.TempDir()
	src := filepath.Join(dir, "messages.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello Mr Bean","farewell":"Goodbye Mr Bean"}`), 0o644))
	a := &App{SourceLang: "en", Quiet: true}
	a.InitRegistries()
	work := filepath.Join(dir, "work.kpz")
	require.NoError(t, a.ExtractToKpz(context.Background(), []string{src}, work, "fr", "", false))

	chain := func() ([]tool.Tool, func(), error) {
		redact, err := tools.NewRedactTool(&tools.RedactConfig{
			Detectors: []string{tools.DetectRules},
			Rules:     []redaction.Rule{{Term: "Mr Bean", Category: "person"}},
		})
		if err != nil {
			return nil, nil, err
		}
		pseudo, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": "fr"}, "fr")
		if err != nil {
			return nil, nil, err
		}
		unredact, err := tools.NewUnredactTool(&tools.UnredactConfig{})
		if err != nil {
			return nil, nil, err
		}
		return []tool.Tool{redact, pseudo, unredact}, func() {}, nil
	}
	require.NoError(t, a.transformKpzInPlace(context.Background(), work, "redacted-translate", chain, "fr", "", false, false))

	out := filepath.Join(dir, "out")
	require.NoError(t, a.MergeFromKpz(context.Background(), work, out))
	data, err := os.ReadFile(filepath.Join(out, "messages.json"))
	require.NoError(t, err)
	var got map[string]string
	require.NoError(t, json.Unmarshal(data, &got))
	for key, source := range map[string]string{"greeting": "Hello Mr Bean", "farewell": "Goodbye Mr Bean"} {
		assert.NotEqual(t, source, got[key], "%s: the translation lands", key)
		assert.Contains(t, got[key], "Mr Bean", "%s: the redacted value is restored", key)
	}
}
