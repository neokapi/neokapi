package host

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/blockstore"
	"github.com/neokapi/neokapi/core/blockstore/sqlitestore"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// #1474. `kapi run <flow> --process-only` keys the target overlays it commits by
// the source file's project-relative path (blockstore.StoreKey), and `kapi merge`
// looks them up by the same path taken from the recipe. The write side derived it
// with `filepath.Rel(projectRoot, inputPath)` and DISCARDED the error — and that
// error is the ordinary case, because inputPath arrives as the user typed it: a
// plain relative argument cannot be made relative to an absolute root.
//
// The result was not a degraded run but a wrong deliverable. The overlays landed
// under bare file-local ids ("tu1"), merge asked for the namespaced key, missed,
// and a miss is blockstore.ErrNotFound — the legitimate "not translated yet"
// sentinel. So merge wrote the SOURCE text into every target file and reported
// success: `Merged src/en.json → fr`, exit 0, 100% untranslated output.
//
// These tests assert the deliverable, not the error: the merged file must contain
// the translated text.

// newProcessOnlyProject writes a one-collection project and chdirs into it, so a
// relative input argument means what it means on a real command line.
func newProcessOnlyProject(t *testing.T) (*App, string, string) {
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
		[]byte(`{"greeting":"Hello world"}`), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "ProcessOnlyKeyTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			SourceGate:      string(model.SourceGateNone),
		},
		Content: []project.ContentCollection{
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

// runProcessOnly performs the write half exactly as host/flow.go does: a
// FileRunner carrying the project root, over the project's own block store.
func runProcessOnly(t *testing.T, a *App, recipe, dir, inputPath string) error {
	t.Helper()
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	store, err := sqlitestore.New(layout.BlockStorePath())
	require.NoError(t, err)
	defer store.Close()

	pseudo, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": "fr"}, "fr")
	require.NoError(t, err)

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    a.FormatReg,
		SourceLocale: "en",
		Store:        store,
		ProjectRoot:  dir,
	})
	return runner.RunFileProcessOnly(context.Background(), "pseudo-translate",
		[]tool.Tool{pseudo}, inputPath, "fr")
}

// TestProcessOnlyThenMerge_RelativeInputProducesTranslatedOutput is the core
// regression, asserted on the artefact: run with a relative path (what a user
// types), merge, and the target file must hold the translation.
func TestProcessOnlyThenMerge_RelativeInputProducesTranslatedOutput(t *testing.T) {
	a, recipe, dir := newProcessOnlyProject(t)

	// The reproducing input: relative, exactly as `-i src/en.json` arrives.
	require.NoError(t, runProcessOnly(t, a, recipe, dir, filepath.Join("src", "en.json")))

	proj, err := project.Load(recipe)
	require.NoError(t, err)
	written, err := a.materializeFromProjectStore(context.Background(), io.Discard, proj, recipe,
		[]model.LocaleID{"fr"}, true)
	require.NoError(t, err)
	assert.Equal(t, 1, written)

	out, err := os.ReadFile(filepath.Join(dir, "src", "fr.json"))
	require.NoError(t, err, "merge must have written the target file")

	// The expectation is derived from the same tool the run used, so the test
	// asserts "the translation the run produced reached the file" rather than a
	// hardcoded shape that would drift with the pseudo tool's config.
	assert.Contains(t, string(out), pseudoTranslated(t, "Hello world"),
		"the merged deliverable must contain the TRANSLATED text")
	assert.NotContains(t, string(out), `"Hello world"`,
		"merge wrote the untranslated SOURCE text and reported success — the run's "+
			"overlays were committed under a key merge cannot address")

	// The source is the input, not an output: it must be untouched.
	src, err := os.ReadFile(filepath.Join(dir, "src", "en.json"))
	require.NoError(t, err)
	assert.JSONEq(t, `{"greeting":"Hello world"}`, string(src))
}

// pseudoTranslated returns what the run's pseudo-translate tool makes of text.
func pseudoTranslated(t *testing.T, text string) string {
	t.Helper()
	pseudo, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": "fr"}, "fr")
	require.NoError(t, err)
	applier, ok := pseudo.(interface {
		Apply(*model.Part) (*model.Part, error)
	})
	require.True(t, ok, "the pseudo tool must be drivable on a single part")
	block := model.NewBlock("probe", text)
	block.Translatable = true
	block.SourceLocale = "en"
	part, err := applier.Apply(&model.Part{Type: model.PartBlock, Resource: block})
	require.NoError(t, err)
	got, ok := part.Resource.(*model.Block)
	require.True(t, ok)
	out := got.TargetText("fr")
	require.NotEmpty(t, out, "the probe must produce a target, or it asserts nothing")
	require.NotEqual(t, text, out, "the pseudo translation must differ from the source")
	return out
}

// TestProcessOnly_AbsoluteAndRelativeInputAgreeOnTheKey is the discriminating
// control for the fix: the point is not "tag with something" but "tag with the
// SAME thing merge will ask for". An absolute and a relative spelling of one file
// must produce one key, and that key must equal what the recipe resolver
// produces — the string merge actually uses.
func TestProcessOnly_AbsoluteAndRelativeInputAgreeOnTheKey(t *testing.T) {
	_, recipe, dir := newProcessOnlyProject(t)

	relForm, err := blockstore.ProjectRel(dir, filepath.Join("src", "en.json"))
	require.NoError(t, err)
	absForm, err := blockstore.ProjectRel(dir, filepath.Join(dir, "src", "en.json"))
	require.NoError(t, err)
	assert.Equal(t, absForm, relForm, "two spellings of one file must give one namespace")

	// And that namespace must be the resolver's `Relative`, which is what
	// host/merge.go passes to blockstore.WithSourceRel.
	proj, err := project.Load(recipe)
	require.NoError(t, err)
	a := &App{}
	a.InitRegistries()
	files, err := project.NewProjectContext(proj, recipe).ResolveContent(a.FormatReg)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, files[0].Relative, relForm,
		"the run's key namespace must be spelled exactly as merge's is, or the "+
			"overlays are committed where merge will never look")
}

// TestProcessOnly_FileOutsideTheProjectIsRefused: the failure that remains a
// failure. Work stored under `../elsewhere/app.json` can never be collected —
// merge only materializes files the recipe resolves — so the run must say so
// rather than commit overlays nobody will read.
func TestProcessOnly_FileOutsideTheProjectIsRefused(t *testing.T) {
	a, recipe, dir := newProcessOnlyProject(t)

	outside := filepath.Join(t.TempDir(), "elsewhere.json")
	require.NoError(t, os.WriteFile(outside, []byte(`{"greeting":"Hello world"}`), 0o644))

	err := runProcessOnly(t, a, recipe, dir, outside)
	require.Error(t, err, "a file outside the project has no addressable key namespace")
	assert.Contains(t, err.Error(), outside, "the message must name the file")
	assert.Contains(t, err.Error(), dir, "and the project root it is outside of")
	assert.Contains(t, err.Error(), "could never be collected")
	assert.Contains(t, err.Error(), "-o", "and the remedy")
}

// TestProcessOnly_NoProjectRootStillRunsOnRawIDs guards what the fix must NOT
// break: an ad-hoc run (no project) keys overlays by the raw block id on purpose
// — a single document cannot collide with itself — and must not start failing.
func TestProcessOnly_NoProjectRootStillRunsOnRawIDs(t *testing.T) {
	a, recipe, dir := newProcessOnlyProject(t)
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	store, err := sqlitestore.New(layout.BlockStorePath())
	require.NoError(t, err)
	defer store.Close()

	pseudo, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": "fr"}, "fr")
	require.NoError(t, err)

	// ProjectRoot empty: the documented ad-hoc case.
	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    a.FormatReg,
		SourceLocale: "en",
		Store:        store,
	})
	require.NoError(t, runner.RunFileProcessOnly(context.Background(), "pseudo-translate",
		[]tool.Tool{pseudo}, filepath.Join(dir, "src", "en.json"), "fr"))

	sess, err := store.Begin(context.Background())
	require.NoError(t, err)
	defer sess.Close()
	n := 0
	for ov, oerr := range sess.ListOverlays("targets/fr") {
		require.NoError(t, oerr)
		assert.NotEmpty(t, ov.Payload)
		n++
	}
	assert.Positive(t, n, "an ad-hoc process-only run still commits its overlays")
}
