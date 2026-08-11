package host

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/neokapi/neokapi/memory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1839. Materializing writes the project content memory as it writes the
// files, and the entry it teaches is keyed by the SOURCE content hash — one
// entry per source string, whatever the target locale. `kapi up` materializes
// one locale at a time and `kapi merge` loops over every target language, so
// each locale wrote that same entry in turn. The memory ended a run holding
// only the locale written last: the deliverables were all on disk and the run
// reported success, while the leverage the next run was supposed to recycle had
// silently gone.

// newMultiLocaleProject writes a project with two target locales and returns
// the app, the recipe path and the project root.
func newMultiLocaleProject(t *testing.T) (*App, string, string) {
	t.Helper()
	dir := t.TempDir()
	// Dogfood isolation contract (CLAUDE.md): pin every root this could inherit.
	t.Setenv("KAPI_CONFIG_DIR", t.TempDir())
	t.Setenv("XDG_DATA_HOME", t.TempDir())
	t.Setenv("XDG_CACHE_HOME", t.TempDir())
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	t.Setenv("KAPI_NO_PROJECT", "1")

	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"),
		[]byte(`{"greeting":"Hello world","farewell":"Goodbye now"}`), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "MultiLocaleMaterializeTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"nb", "de"},
			SourceGate:      string(model.SourceGateNone),
		},
		Collections: []project.Collection{
			{Name: "app", Path: "src/en.json", Target: "src/{lang}.json"},
		},
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
	return a, recipe, dir
}

// translateIntoStore commits `targets/<locale>` overlays for one locale, the
// way a process-only run does (AD-026 §3) — the state materialize reads.
func translateIntoStore(t *testing.T, a *App, recipe, dir string, locale model.LocaleID) {
	t.Helper()
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(t.Context(), layout.Root)
	require.NoError(t, err)

	pseudo, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": string(locale)}, string(locale))
	require.NoError(t, err)

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    a.FormatReg,
		SourceLocale: "en",
		Store:        db.BlocksAutocommit(),
		ProjectRoot:  dir,
	})
	require.NoError(t, runner.RunFileProcessOnly(context.Background(), "pseudo-translate",
		[]tool.Tool{pseudo}, filepath.Join("src", "en.json"), string(locale)))
}

// memoryEntries reads back what the project content memory holds.
func memoryEntries(t *testing.T, a *App, recipe string) []memory.Entry {
	t.Helper()
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(context.Background(), layout.Root)
	require.NoError(t, err)
	tm := db.Memory()
	require.NotNil(t, tm, "the project store must expose its content memory")
	entries, err := tm.Entries(context.Background())
	require.NoError(t, err)
	return entries
}

// assertBothLocalesLearned states the deliverable: every entry the materialize
// pass taught carries the source and BOTH target locales.
func assertBothLocalesLearned(t *testing.T, entries []memory.Entry) {
	t.Helper()
	require.NotEmpty(t, entries, "materializing must teach the content memory something")
	for _, e := range entries {
		assert.NotEmpty(t, e.VariantText("en"), "entry %s lost its source variant", e.ID)
		assert.NotEmpty(t, e.VariantText("nb"), "entry %s holds no nb variant", e.ID)
		assert.NotEmpty(t, e.VariantText("de"), "entry %s holds no de variant", e.ID)
	}
}

// TestMaterialize_PerLocalePassesKeepEveryVariant is the core regression: the
// per-locale materialize the convergence loop performs — one pass per shippable
// locale, each with its own absorber — must leave the memory holding every
// locale, in either order.
func TestMaterialize_PerLocalePassesKeepEveryVariant(t *testing.T) {
	orders := [][]model.LocaleID{
		{"nb", "de"},
		{"de", "nb"},
	}
	for _, order := range orders {
		t.Run(string(order[0])+" then "+string(order[1]), func(t *testing.T) {
			a, recipe, dir := newMultiLocaleProject(t)
			for _, loc := range order {
				translateIntoStore(t, a, recipe, dir, loc)
			}

			proj, err := project.Load(recipe)
			require.NoError(t, err)
			for _, loc := range order {
				written, merr := a.materializeFromProjectStore(context.Background(), io.Discard,
					proj, recipe, []model.LocaleID{loc}, false)
				require.NoError(t, merr)
				assert.Equal(t, 1, written)
			}

			assertBothLocalesLearned(t, memoryEntries(t, a, recipe))
		})
	}
}

// TestMaterialize_OnePassOverEveryLocaleKeepsEveryVariant is the `kapi merge`
// shape: every target language handed to ONE pass, so both locales meet the
// same absorber. Deduplicating by entry id there dropped the second locale
// before it ever reached the store, and which locale survived depended on map
// iteration order.
func TestMaterialize_OnePassOverEveryLocaleKeepsEveryVariant(t *testing.T) {
	a, recipe, dir := newMultiLocaleProject(t)
	translateIntoStore(t, a, recipe, dir, "nb")
	translateIntoStore(t, a, recipe, dir, "de")

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	written, merr := a.materializeFromProjectStore(context.Background(), io.Discard,
		proj, recipe, []model.LocaleID{"nb", "de"}, false)
	require.NoError(t, merr)
	assert.Equal(t, 2, written)

	assertBothLocalesLearned(t, memoryEntries(t, a, recipe))

	// The files are the other half of the deliverable, and both carry a
	// translation rather than the source text.
	for _, loc := range []string{"nb", "de"} {
		got, rerr := os.ReadFile(filepath.Join(dir, "src", loc+".json"))
		require.NoError(t, rerr)
		assert.NotContains(t, string(got), "Hello world",
			"%s materialized the source text, not its stored target", loc)
	}
}
