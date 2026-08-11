package host

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/convergence"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/gate"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1846. A convergence pass runs in write-files mode so each pass's coverage
// sees real files, and its tools commit `targets/<locale>` overlays as they go.
// Those overlays are the only record merge/materialize has of the run's work,
// and materialize addresses them by the source-file-namespaced key
// (blockstore.StoreKey) that the store's own blocks carry. The write-files path
// left the session untagged, so the overlays landed under the bare, file-local
// block id ("tu1") instead — a namespace no reader ever asks for.
//
// The symptom is not an error. A missing overlay is blockstore.ErrNotFound, the
// legitimate "not translated yet" sentinel, so materialize wrote the SOURCE text
// over each already-translated target file and exited 0, and the memory absorber
// saw nothing to learn.
//
// These tests assert the deliverable: converge, then materialize, and the
// translated text must still be in the file and the pair must be in the memory.

// newLoopProject writes a project whose default flow translates with the
// deterministic demo provider (no network, no key, no spend) over the given
// source files, and returns the app, a flag-carrying command, the recipe path
// and the project root. Content is one collection over `src/*.json` so a
// one-file and a many-file project differ only in how many files exist.
func newLoopProject(t *testing.T, sources map[string]string) (*App, *EnvCommand, string, string) {
	t.Helper()
	dir := t.TempDir()
	// Dogfood isolation contract (CLAUDE.md): this test runs a real converge from
	// a cwd inside a temp dir, so pin every root it could otherwise inherit.
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	for name, body := range sources {
		require.NoError(t, os.WriteFile(filepath.Join(dir, "src", name), []byte(body), 0o644))
	}

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "ConvergeMaterializeTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb", "de"},
			Flow:            "translate",
			SourceGate:      string(model.SourceGateNone),
		},
		Collections: []project.Collection{
			{Name: "app", Path: "src/*.json", Target: "out/{lang}/{path}.json"},
		},
		Flows: map[string]*flow.StepsSpec{
			// The shape of the built-in convergence flow: content memory reuse
			// first, the model only for what it cannot cover.
			"translate": {Steps: []flow.FlowStep{
				{Tool: "recycle"},
				{Tool: "translate", Config: map[string]any{"provider": "demo"}},
			}},
		},
		ShipGate: gate.Gate{"translated": {Pct: 100}},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	require.NoError(t, project.EnsureLayout(layout))
	t.Chdir(dir)

	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)
	// The recipe is named explicitly, as `kapi up -p` does: KAPI_NO_PROJECT above
	// blocks discovery, and the tools that resolve project resources of their own
	// (recycle's content memory) go through the command rather than the recipe
	// argument.
	AddProjectFlag(cmd)
	require.NoError(t, cmd.Flags().Set("project", recipe))
	return a, cmd, recipe, dir
}

// converge drives one convergence run to its gate.
func converge(t *testing.T, a *App, cmd *EnvCommand, recipe string) ConvergeOutput {
	t.Helper()
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	var out ConvergeOutput
	require.NoError(t, a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{
		UntilGate: true,
		MaxPasses: 3,
		noChecks:  true,
		capture:   &out,
	}))
	return out
}

// TestConvergeThenMaterialize_KeepsTheTranslatedText is the core regression, on
// a single-file project: after a convergence run the target files hold
// translations, and materializing from the store must leave them holding
// translations rather than overwriting them with the source.
func TestConvergeThenMaterialize_KeepsTheTranslatedText(t *testing.T) {
	a, cmd, recipe, dir := newLoopProject(t, map[string]string{
		"en.json": `{"greeting":"Hello world","farewell":"Goodbye now"}`,
	})

	out := converge(t, a, cmd, recipe)
	require.True(t, out.Converged, "the project must converge before materialize means anything")

	afterRun := map[string]string{}
	for _, loc := range []string{"nb", "de"} {
		p := filepath.Join(dir, "out", loc, "en.json")
		b, rerr := os.ReadFile(p)
		require.NoError(t, rerr, "the convergence pass writes the target file")
		require.NotContains(t, string(b), "Hello world",
			"%s: the pass itself must have translated the file", loc)
		afterRun[loc] = string(b)
	}

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	for _, loc := range []model.LocaleID{"nb", "de"} {
		written, merr := a.materializeFromProjectStore(context.Background(), io.Discard,
			proj, recipe, []model.LocaleID{loc}, false)
		require.NoError(t, merr)
		assert.Equal(t, 1, written)
	}

	for _, loc := range []string{"nb", "de"} {
		b, rerr := os.ReadFile(filepath.Join(dir, "out", loc, "en.json"))
		require.NoError(t, rerr)
		assert.NotContains(t, string(b), "Hello world",
			"%s: materialize wrote the untranslated SOURCE over the run's own output", loc)
		assert.Equal(t, afterRun[loc], string(b),
			"%s: materializing the run's own overlays must reproduce the run's output", loc)
	}

	// The other half of the deliverable: the run taught the content memory, so
	// the next run has leverage to recycle.
	entries := memoryEntries(t, a, recipe)
	require.NotEmpty(t, entries, "the run's targets must reach the content memory")
	for _, e := range entries {
		assert.NotEmpty(t, e.VariantText("en"), "entry %s lost its source variant", e.ID)
		assert.NotEmpty(t, e.VariantText("nb"), "entry %s holds no nb variant", e.ID)
		assert.NotEmpty(t, e.VariantText("de"), "entry %s holds no de variant", e.ID)
	}
}

// TestConvergeThenMaterialize_MultiFileProject is the same deliverable over a
// project with several sources — the batch run path, which is where a
// per-(file, block) key namespace is the only thing keeping two files' `tu1`
// apart.
func TestConvergeThenMaterialize_MultiFileProject(t *testing.T) {
	a, cmd, recipe, dir := newLoopProject(t, map[string]string{
		"en.json":   `{"greeting":"Hello world"}`,
		"more.json": `{"greeting":"Another string entirely"}`,
	})

	out := converge(t, a, cmd, recipe)
	require.True(t, out.Converged)

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	written, merr := a.materializeFromProjectStore(context.Background(), io.Discard,
		proj, recipe, []model.LocaleID{"nb", "de"}, false)
	require.NoError(t, merr)
	assert.Equal(t, 4, written, "two sources × two locales")

	for _, loc := range []string{"nb", "de"} {
		one, rerr := os.ReadFile(filepath.Join(dir, "out", loc, "en.json"))
		require.NoError(t, rerr)
		two, rerr := os.ReadFile(filepath.Join(dir, "out", loc, "more.json"))
		require.NoError(t, rerr)
		assert.NotContains(t, string(one), "Hello world", "%s/en.json holds the source text", loc)
		assert.NotContains(t, string(two), "Another string entirely",
			"%s/more.json holds the source text", loc)
		assert.NotEqual(t, string(one), string(two),
			"%s: two sources sharing a block id must not share an overlay", loc)
	}
}

// TestUp_MaterializeOnConverge_ShipsTheTranslation is the whole verb: with
// `defaults.materialize: on-converge` the run materializes its own output the
// moment the loop finishes, so the corruption lands inside `kapi up` itself
// rather than waiting for a later `kapi merge`.
func TestUp_MaterializeOnConverge_ShipsTheTranslation(t *testing.T) {
	a, cmd, recipe, dir := newLoopProject(t, map[string]string{
		"en.json": `{"greeting":"Hello world","farewell":"Goodbye now"}`,
	})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Materialize = project.MaterializeOnConverge
	require.NoError(t, project.Save(recipe, proj))

	out := converge(t, a, cmd, recipe)
	require.True(t, out.Converged)
	assert.Equal(t, 2, out.MaterializedFiles, "one file per shippable locale")

	for _, loc := range []string{"nb", "de"} {
		b, rerr := os.ReadFile(filepath.Join(dir, "out", loc, "en.json"))
		require.NoError(t, rerr)
		assert.NotContains(t, string(b), "Hello world",
			"%s: `kapi up` materialized the source over its own translation", loc)
	}
}

// TestUp_RecycledWorkSurvivesMaterialize isolates the producer whose work the
// store never saw. `recycle` is the first step of the built-in convergence flow;
// it fills the target from the content memory through a plain Produce handler and
// implements no SessionTool, so its output reached the written file and nothing
// else. Materializing then found no overlay for those blocks — which reads as
// "not translated yet" — and wrote the source back over them. The flow here has
// no other producer, so the assertion is about recycled work alone.
func TestUp_RecycledWorkSurvivesMaterialize(t *testing.T) {
	a, cmd, recipe, dir := newLoopProject(t, map[string]string{
		"en.json": `{"greeting":"Hello world","farewell":"Goodbye now"}`,
	})
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	proj.Defaults.Materialize = project.MaterializeOnConverge
	proj.Flows["translate"] = &flow.StepsSpec{Steps: []flow.FlowStep{{Tool: "recycle"}}}
	require.NoError(t, project.Save(recipe, proj))

	// Seed the leverage the run will recycle, covering every block in both locales
	// so the loop can converge on memory alone.
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(context.Background(), layout.Root)
	require.NoError(t, err)
	tm := db.Memory()
	require.NotNil(t, tm)
	seeded := map[string]map[string]string{
		"Hello world": {"nb": "Hallo verden", "de": "Hallo Welt"},
		"Goodbye now": {"nb": "Ha det bra", "de": "Auf Wiedersehen"},
	}
	for source, targets := range seeded {
		variants := map[model.LocaleID][]model.Run{"en": {model.TextR(source)}}
		for loc, text := range targets {
			variants[model.LocaleID(loc)] = []model.Run{model.TextR(text)}
		}
		require.NoError(t, tm.Add(context.Background(), memory.Entry{
			ID: source, Variants: variants, HintSrcLang: "en",
		}))
	}

	out := converge(t, a, cmd, recipe)
	require.True(t, out.Converged, "memory covering every block must converge the loop")
	assert.Equal(t, 2, out.MaterializedFiles)

	for loc := range map[string]bool{"nb": true, "de": true} {
		b, rerr := os.ReadFile(filepath.Join(dir, "out", loc, "en.json"))
		require.NoError(t, rerr)
		for source, targets := range seeded {
			assert.Contains(t, string(b), targets[loc],
				"%s: the recycled target never reached the store, so materialize wrote the source", loc)
			assert.NotContains(t, string(b), source, "%s holds the source text", loc)
		}
	}
}

// TestRunFlowAllLocales_RecordsItsWork covers the other file-writing project
// path — the multi-locale orchestrator the desktop's run button and `kapi run`
// share. Its files are half the deliverable; without the overlays the store reads
// "never translated" and the next materialize writes the source back.
func TestRunFlowAllLocales_RecordsItsWork(t *testing.T) {
	a, _, recipe, dir := newLoopProject(t, map[string]string{
		"en.json": `{"greeting":"Hello world","farewell":"Goodbye now"}`,
	})
	proj, err := project.Load(recipe)
	require.NoError(t, err)

	_, err = a.RunFlowAllLocales(context.Background(), FlowRunOptions{
		FlowName:    "translate",
		Project:     proj,
		ProjectPath: recipe,
		InputPaths:  []string{filepath.Join(dir, "src", "en.json")},
	}, nil)
	require.NoError(t, err)

	afterRun := map[string]string{}
	for _, loc := range []string{"nb", "de"} {
		b, rerr := os.ReadFile(filepath.Join(dir, "out", loc, "en.json"))
		require.NoError(t, rerr)
		require.NotContains(t, string(b), "Hello world", "%s: the run must have translated the file", loc)
		afterRun[loc] = string(b)
	}

	written, merr := a.materializeFromProjectStore(context.Background(), io.Discard,
		proj, recipe, []model.LocaleID{"nb", "de"}, true)
	require.NoError(t, merr)
	assert.Equal(t, 2, written)
	for _, loc := range []string{"nb", "de"} {
		b, rerr := os.ReadFile(filepath.Join(dir, "out", loc, "en.json"))
		require.NoError(t, rerr)
		assert.Equal(t, afterRun[loc], string(b),
			"%s: materializing the run's own overlays must reproduce the run's output", loc)
	}
}

// TestConverge_BlockStoreCoverageSeesTheRunsWork is the third reader of the same
// overlays: the block-store coverage tally behind the desktop status panel. It
// must address them by the same key everything else does, or a fully translated
// project reports nothing translated.
func TestConverge_BlockStoreCoverageSeesTheRunsWork(t *testing.T) {
	a, cmd, recipe, _ := newLoopProject(t, map[string]string{
		"en.json": `{"greeting":"Hello world","farewell":"Goodbye now"}`,
	})
	require.True(t, converge(t, a, cmd, recipe).Converged)

	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(context.Background(), layout.Root)
	require.NoError(t, err)
	sess, err := db.Blocks().Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	tally, totals, err := convergence.TallyBlockStore(sess, []convergence.BlockStoreScope{{
		Collection: "app",
		Label:      project.CollectionLabel("app"),
		Locales:    []string{"nb", "de"},
	}})
	require.NoError(t, err)
	assert.Equal(t, map[string]int{"app": 2}, totals)
	for _, loc := range []string{"nb", "de"} {
		cov, ok := tally.Coverage(convergence.Scope{Collection: "app", Locale: loc})
		require.True(t, ok, "%s must be tallied", loc)
		assert.Equal(t, 2, cov.AtLeastCount(gate.TargetLadder(), string(model.TargetStatusTranslated)),
			"%s: the run translated every block, so the store must say so", loc)
	}
}

// TestConvergeOverlayKey_IsTheStoreBlockHash pins the canonical key. Every
// overlay a run commits in project scope is addressed by
// blockstore.StoreKey(sourceRel, blockID, sourceText) — which is exactly the
// Hash the store's own block carries — whichever mode wrote it. A run that keys
// its overlays any other way is invisible to merge, and invisibility reads as
// "not translated yet".
func TestConvergeOverlayKey_IsTheStoreBlockHash(t *testing.T) {
	a, cmd, recipe, _ := newLoopProject(t, map[string]string{
		"en.json": `{"greeting":"Hello world","farewell":"Goodbye now"}`,
	})
	require.True(t, converge(t, a, cmd, recipe).Converged)

	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(context.Background(), layout.Root)
	require.NoError(t, err)
	sess, err := db.BlocksAutocommit().Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	files, err := project.NewProjectContext(proj, recipe).ResolveContent(a.FormatReg)
	require.NoError(t, err)
	require.Len(t, files, 1)
	blocks, _, err := project.ReadSourceBlocks(context.Background(), a.FormatReg,
		files[0].Format, files[0].Path, "en", "", nil)
	require.NoError(t, err)

	seen := 0
	for _, b := range blocks {
		if !b.Translatable || b.ID == "" {
			continue
		}
		key := blockstore.StoreKey(files[0].Relative, b.ID, b.SourceText())
		assert.Equal(t, key, project.BlockStoreHash(files[0].Relative, b.ID, b.SourceText()))
		for _, loc := range []string{"nb", "de"} {
			ov, oerr := sess.GetOverlay("targets/"+loc, key)
			require.NoError(t, oerr,
				"block %s has no targets/%s overlay under the store's own block key", b.ID, loc)
			assert.NotEmpty(t, ov.Payload)
			seen++
		}
		// The bare, file-local id is not a key in project scope.
		_, oerr := sess.GetOverlay("targets/nb", b.ID)
		require.ErrorIs(t, oerr, blockstore.ErrNotFound,
			"an overlay under the collision-prone file-local id is one no reader addresses")
	}
	assert.Positive(t, seen, "the run must have committed overlays at all")
}
