package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/core/tools"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// WriteBack is what a preview's File view shows: the file as the next merge
// will write it. These tests hold it to the artefact rather than to the call
// succeeding, because the failure that matters is the one merge already had —
// stored targets addressed under a key the read side cannot reach, so the
// source text is written back and the call reports success.

// newWriteBackProject writes a one-collection project holding a nested JSON
// catalog, and returns the app, the recipe path and the project dir.
func newWriteBackProject(t *testing.T) (*App, string, string) {
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
		[]byte(`{"title":"Kapimart","errors":{"network":{"timeout":"Request timed out"}}}`), 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "WriteBackTest",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
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

// commitTargets runs the pseudo-translate tool over the source and commits its
// overlays to the project block store, the way `kapi run --process-only` does.
func commitTargets(t *testing.T, a *App, recipe, dir string) {
	t.Helper()
	layout, err := project.LayoutFor(recipe)
	require.NoError(t, err)
	db, err := a.ProjectDB(t.Context(), layout.Root)
	require.NoError(t, err)

	pseudo, err := tools.NewPseudoTranslateFromConfig(map[string]any{"target_locale": "fr"}, "fr")
	require.NoError(t, err)

	runner := flow.NewFileRunner(flow.FileRunnerConfig{
		FormatReg:    a.FormatReg,
		SourceLocale: "en",
		Store:        db.BlocksAutocommit(),
		ProjectRoot:  dir,
	})
	require.NoError(t, runner.RunFileProcessOnly(context.Background(), "pseudo-translate",
		[]tool.Tool{pseudo}, filepath.Join("src", "en.json"), "fr"))
}

func TestWriteBackAppliesTheStoredTargets(t *testing.T) {
	a, recipe, dir := newWriteBackProject(t)
	commitTargets(t, a, recipe, dir)

	proj, err := project.Load(recipe)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, a.WriteBack(context.Background(), WriteBackOptions{
		Project:     proj,
		ProjectPath: recipe,
		SourcePath:  filepath.Join(dir, "src", "en.json"),
		Locale:      "fr",
	}, &buf))

	out := buf.String()
	assert.Contains(t, out, pseudoTranslated(t, "Kapimart"),
		"the write-back must carry the TRANSLATED text, not the source")
	assert.Contains(t, out, pseudoTranslated(t, "Request timed out"))
	assert.Contains(t, out, `"timeout"`, "the file keeps its own key structure")

	// Nothing is materialized: this is a reading, not a merge.
	_, statErr := os.Stat(filepath.Join(dir, "src", "fr.json"))
	assert.True(t, os.IsNotExist(statErr), "write-back must not put a file on disk")
}

func TestWriteBackWithNoLocaleReturnsTheSource(t *testing.T) {
	a, recipe, dir := newWriteBackProject(t)
	commitTargets(t, a, recipe, dir)

	proj, err := project.Load(recipe)
	require.NoError(t, err)

	var buf bytes.Buffer
	require.NoError(t, a.WriteBack(context.Background(), WriteBackOptions{
		Project:     proj,
		ProjectPath: recipe,
		SourcePath:  filepath.Join(dir, "src", "en.json"),
	}, &buf))

	assert.Contains(t, buf.String(), "Kapimart")
	assert.Contains(t, buf.String(), "Request timed out")
	assert.NotContains(t, buf.String(), pseudoTranslated(t, "Kapimart"))
}

func TestWriteBackRefusesAFileOutsideTheProject(t *testing.T) {
	a, recipe, dir := newWriteBackProject(t)

	proj, err := project.Load(recipe)
	require.NoError(t, err)

	stray := filepath.Join(dir, "notes.json")
	require.NoError(t, os.WriteFile(stray, []byte(`{"a":"b"}`), 0o644))

	var buf bytes.Buffer
	err = a.WriteBack(context.Background(), WriteBackOptions{
		Project:     proj,
		ProjectPath: recipe,
		SourcePath:  stray,
	}, &buf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not one of this project's content files")
}
