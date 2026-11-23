package host

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
)

// An image read without the vision plugin gets no target: its text was never
// read, so a copy of the source would claim a translation that is not there.
// The run names it, reads as not converged, and still translates the rest.
func TestConverge_WritesNoTargetForMediaReadWithoutItsEngine(t *testing.T) {
	t.Setenv("KAPI_ASR_PLUGIN", "")
	t.Setenv("KAPI_PLUGINS_DIR_ONLY", "1")
	t.Setenv("KAPI_PLUGINS_DIR", t.TempDir())
	RefreshMediaEngines()
	t.Cleanup(RefreshMediaEngines)

	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "src", "en.json"), []byte(`{"greeting":"Hello world"}`), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "media", "en"), 0o755))
	png, err := os.ReadFile(filepath.Join("..", "core", "formats", "image", "testdata", "swatch.png"))
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "media", "en", "swatch.png"), png, 0o644))

	proj := &project.KapiProject{
		Version: project.CurrentVersion,
		Name:    "UnreadMedia",
		Defaults: project.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr"},
			Flow:            "translate",
		},
		Collections: []project.Collection{
			{Name: "docs", Path: "src/en.json", Target: "src/{lang}.json"},
			{Name: "images", Path: "media/en/*.png", Target: "media/{lang}/{name}.png", Format: &project.FormatSpec{Name: "image"}},
		},
		Flows: map[string]*flow.StepsSpec{
			"translate": {Steps: []flow.FlowStep{{Tool: "translate", Config: map[string]any{"provider": "demo"}}}},
		},
	}
	recipe := filepath.Join(dir, project.RecipeFileName)
	require.NoError(t, project.Save(recipe, proj))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, project.StateDirName), 0o755))
	t.Chdir(dir)

	a := &App{}
	a.InitRegistries()
	a.SourceLang = "en"
	cmd := NewEnvCommand(context.Background(), "up")
	a.AddFlowRunFlags(cmd)

	var out ConvergeOutput
	require.NoError(t, a.RunDefaultFlowConverge(cmd, proj, recipe, ConvergeOptions{
		UntilGate: true, MaxPasses: 3, noChecks: true, capture: &out,
	}))

	assert.FileExists(t, filepath.Join(dir, "src", "fr.json"), "the text collection is translated")
	assert.NoFileExists(t, filepath.Join(dir, "media", "fr", "swatch.png"), "no target for an image read without OCR")
	require.Len(t, out.UnreadMedia, 1)
	assert.Equal(t, "media/en/swatch.png", out.UnreadMedia[0].File)
	assert.Equal(t, []string{"vision"}, out.UnreadMedia[0].Engines)
	assert.Equal(t, "images", out.UnreadMedia[0].Collection)
	assert.False(t, out.Converged)

	// `kapi status` names the file and the engine it waits for.
	scmd := NewEnvCommand(t.Context(), "status")
	AddProjectFlag(scmd)
	AddStatusFlags(scmd)
	require.NoError(t, scmd.Flags().Set("project", recipe))
	var serr bytes.Buffer
	scmd.SetErr(&serr)
	text, err := captureStdout(t, func() error { return (&App{}).RunStatus(scmd, nil) })
	require.NoError(t, err, serr.String())
	assert.Contains(t, text, "media/en/swatch.png")
	assert.Contains(t, text, "No target is written for them")
	assert.Regexp(t, `fr/images\s.*waits for vision.*not read`, text, "the collection has a row in the grid")
}
