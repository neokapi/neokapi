package host

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	"github.com/neokapi/neokapi/host/output"
	"github.com/neokapi/neokapi/memory"
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
// names the block's source. It counts one whose stamp names another source,
// and one with no stamp, which an earlier kapi wrote, as stale, names the file
// and locale of the one with no stamp, and leaves both blocks in the source
// language.
func TestMaterialize_AStaleOrMissingSourceStampIsCountedAndNotWritten(t *testing.T) {
	a, recipe, dir, _ := stampedProject(t)
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	written, stale, err := a.materializeProject(context.Background(), io.Discard, proj, recipe, []model.LocaleID{"fr"}, true, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, written)
	assert.Equal(t, 2, stale.Blocks)
	assert.Equal(t, []output.UnstampedDrafts{{File: "src/en.json", Locale: "fr", Blocks: 1}}, stale.Unstamped)

	out, err := os.ReadFile(filepath.Join(dir, "src", "fr.json"))
	require.NoError(t, err)
	var got map[string]string
	require.NoError(t, json.Unmarshal(out, &got))
	assert.Equal(t, "Bonjour le monde", got["current"], "a current stamp applies")
	assert.Equal(t, "Thank you", got["unstamped"], "an overlay an earlier kapi wrote is not written; the block keeps its source")
	assert.Equal(t, "Goodbye for now", got["moved"], "a stale overlay is not written; the block keeps its source")
	assert.NotContains(t, string(out), "Au revoir")
	assert.NotContains(t, string(out), "Merci")
}

// kapi merge with no -i prints the stale count and, under it, one line for
// each file and locale whose drafts an earlier kapi wrote.
func TestMergeFromProjectStore_ListsADraftAnEarlierKapiWrote(t *testing.T) {
	a, recipe, _, _ := stampedProject(t)
	cmd, stdout, stderr := mergeCommand(t, recipe)
	require.NoError(t, cmd.Flags().Set("no-memory-update", "true"))
	require.NoError(t, a.MergeFromProjectStore(cmd), stderr.String())

	var res output.MergeStoreOutput
	require.NoError(t, json.Unmarshal(stdout.Bytes(), &res), stdout.String())
	assert.Equal(t, 2, res.Stale)
	assert.Equal(t, []output.UnstampedDrafts{{File: "src/en.json", Locale: "fr", Blocks: 1}}, res.Unstamped)
	var text bytes.Buffer
	require.NoError(t, res.FormatText(&text))
	assert.Equal(t, "\nMerge complete. wrote=1 file(s) stale=2 from the project store.\n"+
		"  src/en.json (fr): 1 block(s) hold a draft an earlier kapi wrote; a translating pass drafts them again.\n", text.String())
}

// The merge's write to the content memory leaves a stale translation out: the
// pair of the current source with the translation of an older one is not one
// the project made, and a translation an earlier kapi wrote names no source.
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
	assert.Equal(t, 1, n+u, "the current translation alone is absorbed")
	for _, e := range absorber.entries {
		assert.NotEqual(t, "Au revoir", e.VariantText("fr"), "the stale translation is not absorbed")
		assert.NotEqual(t, "Merci", e.VariantText("fr"), "the translation an earlier kapi wrote is not absorbed")
	}
}

// The `.kpz` merge step applies a current overlay, and counts a stale one and
// one an earlier kapi wrote without applying either.
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
	assert.False(t, blocks[2].HasTarget("fr"), "an overlay an earlier kapi wrote is not applied")
	assert.Equal(t, 2, tl.Stale())
	assert.Equal(t, 1, tl.Unstamped())

	// A skeleton-only workspace rebuilds each block from its id alone, so the
	// stamp has no source to be compared with and a stamped overlay applies.
	// An overlay with no stamp needs no source to be told apart: it is counted
	// and left out.
	skeleton := model.NewBlock("moved", "")
	skeleton.Translatable = true
	unstampedSkeleton := model.NewBlock("unstamped", "")
	unstampedSkeleton.Translatable = true
	tl = newHydrateTargetsTool("fr")
	tl.skeleton = true
	in = make(chan *model.Part, 2)
	out = make(chan *model.Part, 2)
	in <- &model.Part{Type: model.PartBlock, Resource: skeleton}
	in <- &model.Part{Type: model.PartBlock, Resource: unstampedSkeleton}
	close(in)
	require.NoError(t, tl.SessionProcess(ctx, sess, in, out))
	assert.Equal(t, "Au revoir", skeleton.TargetText("fr"))
	assert.False(t, unstampedSkeleton.HasTarget("fr"), "an overlay an earlier kapi wrote is not applied")
	assert.Equal(t, 1, tl.Stale())
	assert.Equal(t, 1, tl.Unstamped())
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
		return got, stale.Blocks
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

// A reader's block for an empty value carries the empty source, and a stored
// translation of "Hello world" is stale for it: the merge counts it and writes
// the empty source, and writes the other block's current translation.
func TestMaterialize_AnEmptiedSourceMakesItsTranslationStale(t *testing.T) {
	a, recipe, dir := newProcessOnlyProject(t)
	src := filepath.Join(dir, "src", "en.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello world","farewell":"Goodbye"}`), 0o644))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(t.Context(), layout.Root)
	require.NoError(t, err)
	blocks := readStampedBlocks(t, a, dir)
	sess, err := db.BlocksAutocommit().Begin(t.Context())
	require.NoError(t, err)
	for name, tr := range map[string]string{"greeting": "Bonjour le monde", "farewell": "Au revoir"} {
		b := blocks[name]
		require.NotNil(t, b, name)
		payload, err := json.Marshal(blockstore.TargetOverlay{Text: tr, Source: blockstore.SourceStamp(b.SourceText())})
		require.NoError(t, err)
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{
			Kind:      blockstore.TargetOverlayKind("fr"),
			BlockHash: blockstore.StoreKey(filepath.Join("src", "en.json"), b.ID, b.SourceText()),
			Payload:   payload,
		}))
	}
	require.NoError(t, sess.Commit())

	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"","farewell":"Goodbye"}`), 0o644))
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	written, stale, err := a.materializeProject(context.Background(), io.Discard, proj, recipe, []model.LocaleID{"fr"}, true, nil)
	require.NoError(t, err)
	assert.Equal(t, 1, written)
	assert.Equal(t, 1, stale.Blocks)
	assert.Empty(t, stale.Unstamped)
	out, err := os.ReadFile(filepath.Join(dir, "src", "fr.json"))
	require.NoError(t, err)
	var got map[string]string
	require.NoError(t, json.Unmarshal(out, &got))
	assert.Empty(t, got["greeting"], "the stale translation is not written; the empty source is")
	assert.Equal(t, "Au revoir", got["farewell"])
}

// `kapi up` over a flow with no producing step for a block cannot draft it
// again, so a stored draft of an earlier source stays: the materialize step
// leaves it out, and the run counts it per locale and says so.
func TestUp_AProducerlessFlowNotesAStaleDraft(t *testing.T) {
	a, cmd, recipe, dir := newLoopProject(t, map[string]string{
		"en.json": `{"greeting":"Hello world","farewell":"Goodbye now"}`,
	})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Materialize = project.MaterializeOnConverge
	proj.Flows["translate"] = &flow.StepsSpec{Steps: []flow.FlowStep{{Tool: "recycle"}}}
	require.NoError(t, project.Save(recipe, proj))

	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(context.Background(), layout.Root)
	require.NoError(t, err)
	require.NoError(t, db.Memory().Add(context.Background(), memory.Entry{
		ID: "Hello world", HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {model.TextR("Hello world")}, "nb": {model.TextR("Hallo verden")}, "de": {model.TextR("Hallo Welt")},
		},
	}))
	// A draft of the farewell made when its source read "Goodbye".
	farewell := readStampedBlocks(t, a, dir)["farewell"]
	require.NotNil(t, farewell)
	sess, err := db.BlocksAutocommit().Begin(context.Background())
	require.NoError(t, err)
	for loc, text := range map[string]string{"nb": "Ha det", "de": "Tschüss"} {
		payload, err := json.Marshal(blockstore.TargetOverlay{Text: text, Status: string(model.TargetStatusDraft), Source: blockstore.SourceStamp("Goodbye")})
		require.NoError(t, err)
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{
			Kind:      blockstore.TargetOverlayKind(model.LocaleID(loc)),
			BlockHash: blockstore.StoreKey(filepath.Join("src", "en.json"), farewell.ID, farewell.SourceText()),
			Payload:   payload,
		}))
	}
	require.NoError(t, sess.Commit())

	out := converge(t, a, cmd, recipe)
	require.Equal(t, 2, out.MaterializedFiles, "one file per locale")
	assert.Equal(t, 2, out.StaleDraftUnits())
	for _, lc := range out.Locales {
		assert.Equal(t, 1, lc.StaleDrafts, lc.Locale)
		data, rerr := os.ReadFile(filepath.Join(dir, "out", lc.Locale, "en.json"))
		require.NoError(t, rerr)
		assert.NotContains(t, string(data), "Ha det", "%s: the stale draft is not written", lc.Locale)
		assert.NotContains(t, string(data), "Tschüss", "%s: the stale draft is not written", lc.Locale)
	}
	var text bytes.Buffer
	require.NoError(t, out.FormatText(&text))
	assert.Contains(t, text.String(), "Note: 2 block(s) hold a stale draft (an earlier source, or an earlier kapi) and were not written.")
}

// putFarewellDrafts stores an nb and a de draft of the loop project's
// farewell block, stamped with stamp ("" for a draft an earlier kapi wrote).
func putFarewellDrafts(t *testing.T, a *App, recipe, dir, stamp string) {
	t.Helper()
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(context.Background(), layout.Root)
	require.NoError(t, err)
	farewell := readStampedBlocks(t, a, dir)["farewell"]
	require.NotNil(t, farewell)
	sess, err := db.BlocksAutocommit().Begin(context.Background())
	require.NoError(t, err)
	for loc, text := range map[string]string{"nb": "Ha det", "de": "Tschüss"} {
		payload, err := json.Marshal(blockstore.TargetOverlay{Text: text, Status: string(model.TargetStatusDraft), Source: stamp})
		require.NoError(t, err)
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{
			Kind:      blockstore.TargetOverlayKind(model.LocaleID(loc)),
			BlockHash: blockstore.StoreKey(filepath.Join("src", "en.json"), farewell.ID, farewell.SourceText()),
			Payload:   payload,
		}))
	}
	require.NoError(t, sess.Commit())
}

// `kapi up` over a flow with no producing step for a block leaves a stored
// draft an earlier kapi wrote as it is: the materialize step leaves it out,
// counts it, and lists its file and locale under the count.
func TestUp_ListsADraftAnEarlierKapiWrote(t *testing.T) {
	a, cmd, recipe, dir := newLoopProject(t, map[string]string{
		"en.json": `{"greeting":"Hello world","farewell":"Goodbye now"}`,
	})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Materialize = project.MaterializeOnConverge
	proj.Flows["translate"] = &flow.StepsSpec{Steps: []flow.FlowStep{{Tool: "recycle"}}}
	require.NoError(t, project.Save(recipe, proj))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(context.Background(), layout.Root)
	require.NoError(t, err)
	require.NoError(t, db.Memory().Add(context.Background(), memory.Entry{
		ID: "Hello world", HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{
			"en": {model.TextR("Hello world")}, "nb": {model.TextR("Hallo verden")}, "de": {model.TextR("Hallo Welt")},
		},
	}))
	putFarewellDrafts(t, a, recipe, dir, "")

	out := converge(t, a, cmd, recipe)
	require.Equal(t, 2, out.MaterializedFiles, "one file per locale")
	assert.Equal(t, 2, out.StaleDraftUnits())
	for _, lc := range out.Locales {
		assert.Equal(t, 1, lc.StaleDrafts, lc.Locale)
		assert.Equal(t, []output.UnstampedDrafts{{File: "src/en.json", Locale: lc.Locale, Blocks: 1}}, lc.UnstampedDrafts, lc.Locale)
		data, rerr := os.ReadFile(filepath.Join(dir, "out", lc.Locale, "en.json"))
		require.NoError(t, rerr)
		assert.NotContains(t, string(data), "Ha det", "%s: the draft is not written", lc.Locale)
		assert.NotContains(t, string(data), "Tschüss", "%s: the draft is not written", lc.Locale)
	}
	var text bytes.Buffer
	require.NoError(t, out.FormatText(&text))
	var want strings.Builder
	want.WriteString("Note: 2 block(s) hold a stale draft (an earlier source, or an earlier kapi) and were not written.\n")
	for _, lc := range out.Locales {
		want.WriteString("  src/en.json (" + lc.Locale + "): 1 block(s) hold a draft an earlier kapi wrote; a translating pass drafts them again.\n")
	}
	assert.Contains(t, text.String(), want.String())
}

// `kapi up` over a flow that translates drafts a block whose stored draft an
// earlier kapi wrote again: the producer takes that draft for no answer, the
// target file gets the new translation, and the run lists nothing.
func TestUp_DraftsAgainADraftAnEarlierKapiWrote(t *testing.T) {
	a, cmd, recipe, dir := newLoopProject(t, map[string]string{
		"en.json": `{"greeting":"Hello world","farewell":"Goodbye now"}`,
	})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Materialize = project.MaterializeOnConverge
	require.NoError(t, project.Save(recipe, proj))
	putFarewellDrafts(t, a, recipe, dir, "")

	out := converge(t, a, cmd, recipe)
	assert.Zero(t, out.StaleDraftUnits())
	for _, lc := range out.Locales {
		assert.Empty(t, lc.UnstampedDrafts, lc.Locale)
		data, rerr := os.ReadFile(filepath.Join(dir, "out", lc.Locale, "en.json"))
		require.NoError(t, rerr)
		var got map[string]string
		require.NoError(t, json.Unmarshal(data, &got))
		assert.NotEmpty(t, got["farewell"], lc.Locale)
		assert.NotEqual(t, "Goodbye now", got["farewell"], "%s: the block is translated", lc.Locale)
		assert.NotContains(t, []string{"Ha det", "Tschüss"}, got["farewell"], "%s: the draft an earlier kapi wrote is drafted again", lc.Locale)
	}
	var text bytes.Buffer
	require.NoError(t, out.FormatText(&text))
	assert.NotContains(t, text.String(), "an earlier kapi wrote")
}

// unstampFarewell removes the source stamp from the nb and de drafts of the
// loop project's farewell block, as an earlier kapi wrote them, and returns
// the stamp each held.
func unstampFarewell(t *testing.T, a *App, recipe, dir string) {
	t.Helper()
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(context.Background(), layout.Root)
	require.NoError(t, err)
	farewell := readStampedBlocks(t, a, dir)["farewell"]
	require.NotNil(t, farewell)
	key := blockstore.StoreKey(filepath.Join("src", "en.json"), farewell.ID, farewell.SourceText())
	sess, err := db.BlocksAutocommit().Begin(context.Background())
	require.NoError(t, err)
	for _, loc := range []model.LocaleID{"nb", "de"} {
		o, err := sess.GetOverlay(blockstore.TargetOverlayKind(loc), key)
		require.NoError(t, err, loc)
		var p blockstore.TargetOverlay
		require.NoError(t, json.Unmarshal(o.Payload, &p))
		require.NotEmpty(t, p.Source, "%s: the run stamped its draft", loc)
		p.Source = ""
		payload, err := json.Marshal(p)
		require.NoError(t, err)
		require.NoError(t, sess.PutOverlay(blockstore.Overlay{Kind: o.Kind, BlockHash: key, Payload: payload}))
	}
	require.NoError(t, sess.Commit())
}

// farewellStamps returns the source stamp the nb and de drafts of the
// farewell block carry.
func farewellStamps(t *testing.T, a *App, recipe, dir string) map[model.LocaleID]string {
	t.Helper()
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(context.Background(), layout.Root)
	require.NoError(t, err)
	farewell := readStampedBlocks(t, a, dir)["farewell"]
	key := blockstore.StoreKey(filepath.Join("src", "en.json"), farewell.ID, farewell.SourceText())
	sess, err := db.BlocksAutocommit().Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()
	out := map[model.LocaleID]string{}
	for _, loc := range []model.LocaleID{"nb", "de"} {
		o, err := sess.GetOverlay(blockstore.TargetOverlayKind(loc), key)
		require.NoError(t, err, loc)
		var p blockstore.TargetOverlay
		require.NoError(t, json.Unmarshal(o.Payload, &p))
		out[loc] = p.Source
	}
	return out
}

// A draft an earlier kapi wrote for a block whose target file already holds a
// translation is work for `kapi up`: the next run with a translating flow
// drafts the block again and stamps the draft, and the run after it has no
// stale draft to note.
func TestUp_DraftsAgainAnUnstampedDraftWhoseFileIsTranslated(t *testing.T) {
	a, cmd, recipe, dir := newLoopProject(t, map[string]string{
		"en.json": `{"greeting":"Hello world","farewell":"Goodbye now"}`,
	})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Materialize = project.MaterializeOnConverge
	require.NoError(t, project.Save(recipe, proj))

	first := converge(t, a, cmd, recipe)
	require.True(t, first.Converged)
	for _, loc := range []string{"nb", "de"} {
		require.FileExists(t, filepath.Join(dir, "out", loc, "en.json"))
	}
	unstampFarewell(t, a, recipe, dir)

	second := converge(t, a, cmd, recipe)
	assert.Zero(t, second.StaleDraftUnits(), "the run drafts the block again rather than leaving the draft stale")
	want := blockstore.SourceStamp("Goodbye now")
	for loc, stamp := range farewellStamps(t, a, recipe, dir) {
		assert.Equal(t, want, stamp, "%s: the new draft carries the source stamp", loc)
	}

	third := converge(t, a, cmd, recipe)
	assert.Zero(t, third.StaleDraftUnits())
	var text bytes.Buffer
	require.NoError(t, third.FormatText(&text))
	assert.NotContains(t, text.String(), "Note:")
}

// The content-memory prefill a .kpz extract writes is stamped with the source
// it answers, so a merge compares it with the source it reads like any other
// stored translation.
func TestExtractKpz_StampsTheContentMemoryPrefill(t *testing.T) {
	a, recipe, _ := newProcessOnlyProject(t)
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(context.Background(), layout.Root)
	require.NoError(t, err)
	require.NoError(t, db.Memory().Add(context.Background(), memory.Entry{
		ID: "Hello world", HintSrcLang: "en",
		Variants: map[model.LocaleID][]model.Run{"en": {model.TextR("Hello world")}, "fr": {model.TextR("Bonjour le monde")}},
	}))

	outDir := t.TempDir()
	cmd := NewEnvCommand(t.Context(), "extract")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	addKpzExtractFlags(cmd, recipe)
	require.NoError(t, cmd.Flags().Set("out-dir", outDir))
	require.NoError(t, a.RunExtractKpz(cmd))

	packages, err := filepath.Glob(filepath.Join(outDir, "*.kpz"))
	require.NoError(t, err)
	require.Len(t, packages, 1)
	pkg, err := LoadWorkspace(packages[0])
	require.NoError(t, err)
	var prefills []blockstore.TargetOverlay
	for _, ov := range pkg.Overlays {
		if blockstore.CanonicalOverlayKind(ov.Kind) != blockstore.TargetOverlayKind("fr") {
			continue
		}
		var p blockstore.TargetOverlay
		require.NoError(t, json.Unmarshal(ov.Payload, &p))
		prefills = append(prefills, p)
	}
	require.Len(t, prefills, 1)
	assert.Equal(t, "Bonjour le monde", prefills[0].TargetText())
	assert.Equal(t, blockstore.SourceStamp("Hello world"), prefills[0].Source)
}
