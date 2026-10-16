package host

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host/output"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A format reference names its format outright; `@engine` selects that
// engine's format for the same extensions, and a recipe's per-format pin
// does the same for a bare name.
func TestFormatRefName(t *testing.T) {
	a, _ := engineTestApp(t)

	for ref, want := range map[string]string{
		"json":                  "json",
		"okf_json":              "okf_json",
		"json@okapi-bridge":     "okf_json",
		"okf_json@native":       "json",
		"okf_json@built-in":     "json",
		"json@native":           "json",
		"json@okapi-bridge@1.0": "okf_json@1.0",
		"json:strict":           "json",
	} {
		got, err := a.FormatRefName(ref)
		require.NoError(t, err, ref)
		assert.Equal(t, want, got, ref)
	}

	_, err := a.FormatRefName("json@okapi")
	require.ErrorIs(t, err, registry.ErrNoEngine, "an engine nothing installed provides")
	_, err = a.FormatRefName("markdown@okapi-bridge")
	require.ErrorIs(t, err, registry.ErrNoEngineFormat, "an engine with no format for the extensions")

	// The recipe pins JSON to the plugin: `--format json` reads with it.
	a.ProjectContext = project.NewProjectContext(&project.KapiProject{
		Plugins:  map[string]project.PluginSpec{"okapi-bridge": {}},
		Defaults: project.Defaults{Formats: map[string]project.FormatDefaults{"json": {Engine: "okapi-bridge"}}},
	}, filepath.Join(t.TempDir(), project.RecipeFileName))
	got, err := a.FormatRefName("json")
	require.NoError(t, err)
	assert.Equal(t, "okf_json", got)
	got, err = a.FormatRefName("json@native")
	require.NoError(t, err)
	assert.Equal(t, "json", got, "an engine in the reference ranks above the pin")
}

// resolveFormatRef hands the toolbox and the runners the engine's format,
// so `-f json@okapi-bridge` reads with the plugin.
func TestResolveFormatRef_Engine(t *testing.T) {
	a, p := engineTestApp(t)
	src := writeMessagesJSON(t, t.TempDir())
	a.FormatFlag = "json@okapi-bridge"
	runExec(t, a, src)
	assert.Equal(t, 1, p.reads())
	content, err := os.ReadFile(src)
	require.NoError(t, err)
	name, err := a.ResolveFormatName(src, content)
	require.NoError(t, err)
	assert.Equal(t, "okf_json", name)
}

func explainCmd(t *testing.T, recipe string) Command {
	t.Helper()
	cmd := NewEnvCommand(context.Background(), "explain")
	AddProjectFlag(cmd)
	if recipe != "" {
		require.NoError(t, cmd.Flags().Set("project", recipe))
	}
	return cmd
}

func TestExplainFormats_NoProject(t *testing.T) {
	t.Setenv(project.NoProjectEnvVar, "1")
	a, _ := engineTestApp(t)
	dir := t.TempDir()
	src := writeMessagesJSON(t, dir)
	odd := filepath.Join(dir, "notes.nothing")

	out, err := a.ExplainFormats(explainCmd(t, ""), []string{src, odd})
	require.NoError(t, err)
	require.Len(t, out.Files, 2)

	got := out.Files[0]
	assert.Equal(t, src, got.Path)
	assert.Equal(t, "json", got.Format)
	assert.Equal(t, registry.EngineNative, got.Engine)
	assert.Equal(t, registry.RuleNativeFirst, got.Rule)
	assert.Equal(t, []string{registry.EngineNative, "okapi-bridge"}, got.Order)
	assert.Contains(t, got.Candidates, output.FormatCandidate{Format: "json", Engine: "native"})
	assert.Contains(t, got.Candidates, output.FormatCandidate{Format: "okf_json", Engine: "okapi-bridge"})
	assert.Empty(t, got.Error)

	assert.Equal(t, odd, out.Files[1].Path)
	assert.Empty(t, out.Files[1].Format)
	assert.Contains(t, out.Files[1].Error, ".nothing")

	// --engine ranks the plugin first and is reported as the override.
	a.EngineFlag = "okapi-bridge"
	require.NoError(t, a.ApplyEngineSelection())
	out, err = a.ExplainFormats(explainCmd(t, ""), []string{src})
	require.NoError(t, err)
	assert.Equal(t, "okf_json", out.Files[0].Format)
	assert.Equal(t, "okapi-bridge", out.Files[0].Engine)
	assert.Equal(t, registry.RuleEngineOverride, out.Files[0].Rule)
	assert.Equal(t, []string{"okapi-bridge", registry.EngineNative}, out.Files[0].Order)
}

func TestExplainFormats_Project(t *testing.T) {
	a, _ := engineTestApp(t)
	dir, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
	src := writeMessagesJSON(t, filepath.Join(dir, "src"))
	pinned := filepath.Join(dir, "src", "pinned.json")
	require.NoError(t, os.WriteFile(pinned, []byte(`{"a":"b"}`), 0o644))
	recipe := filepath.Join(dir, project.RecipeFileName)
	proj := &project.KapiProject{
		Version: project.CurrentVersion, Name: "explain",
		Plugins: map[string]project.PluginSpec{"okapi-bridge": {}},
		Defaults: project.Defaults{
			SourceLanguage: "en-US", TargetLanguages: []model.LocaleID{"qps"},
			Engine: "okapi-bridge",
		},
		Collections: []project.Collection{
			{Path: "src/pinned.json", Target: "out/{lang}/pinned.json", Format: &project.FormatSpec{Name: "json"}},
			{Path: "src/*.json", Target: "out/{lang}/*.json"},
		},
	}
	require.NoError(t, project.Save(recipe, proj))

	out, err := a.ExplainFormats(explainCmd(t, recipe), []string{src, pinned})
	require.NoError(t, err)
	require.Len(t, out.Files, 2)

	assert.Equal(t, "okf_json", out.Files[0].Format, "defaults.engine prefers the plugin")
	assert.Equal(t, registry.RuleEnginePreference, out.Files[0].Rule)

	assert.Equal(t, "json", out.Files[1].Format, "the content item names the format")
	assert.Equal(t, registry.EngineNative, out.Files[1].Engine)
	assert.Equal(t, RuleContentItemFormat, out.Files[1].Rule)
	assert.Empty(t, out.Files[1].Candidates)

	// A plugin the recipe does not declare serves none of its files. The
	// recipe is written afresh: Save edits the file in place and keeps a key
	// the struct no longer sets.
	proj.Plugins = nil
	proj.Defaults.Engine = ""
	require.NoError(t, os.Remove(recipe))
	require.NoError(t, project.Save(recipe, proj))
	a.EngineFlag = "okapi-bridge"
	require.NoError(t, a.ApplyEngineSelection())
	out, err = a.ExplainFormats(explainCmd(t, recipe), []string{src})
	require.NoError(t, err)
	assert.Equal(t, "json", out.Files[0].Format)
	assert.Equal(t, registry.RuleNativeFirst, out.Files[0].Rule, "the override was consulted and had nothing the recipe allows")
	for _, c := range out.Files[0].Candidates {
		assert.Equal(t, registry.EngineNative, c.Engine, c.Format)
	}
}
