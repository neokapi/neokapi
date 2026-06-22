package cli

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/neokapi/neokapi/cli/output"
	"github.com/neokapi/neokapi/cli/pluginhost"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	aiprovider "github.com/neokapi/neokapi/providers/ai"
)

func mkModel(id string, def bool) manifest.ModelAsset {
	return manifest.ModelAsset{
		ID: id, Version: "1", Default: def,
		Files: []manifest.ModelFile{{Path: "a.onnx", URL: "https://h/a", SHA256: strings.Repeat("a", 64)}},
	}
}

func mkPlugin(name string, models ...manifest.ModelAsset) *pluginhost.Plugin {
	return &pluginhost.Plugin{Manifest: &manifest.Manifest{
		ManifestVersion: "1", Plugin: name, Version: "1", Binary: "x", Models: models,
	}}
}

func appWith(plugins ...*pluginhost.Plugin) *App {
	return &App{PluginHost: pluginhost.NewHost(plugins, func(string) {})}
}

func TestResolveModelRef(t *testing.T) {
	app := appWith(mkPlugin("llm", mkModel("gemma-4-e2b", true), mkModel("gemma-tiny", false)))

	// Primary handle: a bare model id resolves to its plugin.
	p, a, err := app.resolveModelRef("gemma-4-e2b")
	require.NoError(t, err)
	assert.Equal(t, "llm", p)
	assert.Equal(t, "gemma-4-e2b", a.ID)

	// A non-default model is still addressable by id.
	_, a, err = app.resolveModelRef("gemma-tiny")
	require.NoError(t, err)
	assert.Equal(t, "gemma-tiny", a.ID)

	// A bare plugin name resolves to that plugin's default model.
	_, a, err = app.resolveModelRef("llm")
	require.NoError(t, err)
	assert.Equal(t, "gemma-4-e2b", a.ID)

	// Explicit plugin/model.
	_, a, err = app.resolveModelRef("llm/gemma-tiny")
	require.NoError(t, err)
	assert.Equal(t, "gemma-tiny", a.ID)

	// Unknown reference.
	_, _, err = app.resolveModelRef("nope")
	require.ErrorContains(t, err, "no model or plugin")

	// Explicit pair with a wrong model.
	_, _, err = app.resolveModelRef("llm/nope")
	require.ErrorContains(t, err, "no model")
}

// runModelsCmd executes `models <args...>` against app, capturing combined
// output. The models parent gets the persistent output flags the root command
// supplies in production, so subcommands honor --json.
func runModelsCmd(app *App, args ...string) (string, error) {
	root := app.NewModelsCmd()
	output.AddPersistentFlags(root)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// Filtering to a plugin name isolates the plugin section — and skips the Ollama
// network probe — so the command-level tests stay deterministic regardless of
// whether an Ollama runtime happens to be running on the test host.
func TestModelsListText(t *testing.T) {
	t.Setenv("KAPI_MODELS_CACHE", t.TempDir()) // isolate cache → "not cached"
	app := appWith(mkPlugin("llm", mkModel("gemma-4-e2b", true), mkModel("gemma-tiny", false)))

	out, err := runModelsCmd(app, "list", "--provider", "llm")
	require.NoError(t, err)
	assert.Contains(t, out, "Plugin models")
	assert.Contains(t, out, "gemma-4-e2b (default)")
	assert.Contains(t, out, "gemma-tiny")
	assert.Contains(t, out, "not cached")
}

func TestModelsListJSON(t *testing.T) {
	t.Setenv("KAPI_MODELS_CACHE", t.TempDir())
	app := appWith(mkPlugin("llm", mkModel("gemma-4-e2b", true)))

	out, err := runModelsCmd(app, "list", "--provider", "llm", "--json")
	require.NoError(t, err)
	var got struct {
		Models []struct {
			Source, Provider, Model, Status string
			Default                         bool
		} `json:"models"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &got), "output must be JSON: %s", out)
	require.Equal(t, 1, got.Total)
	assert.Equal(t, "plugin", got.Models[0].Source)
	assert.Equal(t, "llm", got.Models[0].Provider)
	assert.Equal(t, "gemma-4-e2b", got.Models[0].Model)
	assert.True(t, got.Models[0].Default)
}

func TestModelsBundledListedButNotPullable(t *testing.T) {
	t.Setenv("KAPI_MODELS_CACHE", t.TempDir())
	app := appWith(mkPlugin("vision", manifest.ModelAsset{
		ID: "ppocrv5", Version: "1", Default: true, Bundled: true,
		Files: []manifest.ModelFile{{Path: "det.onnx"}, {Path: "rec.onnx"}},
	}))

	// Listed, with a "bundled" status.
	out, err := runModelsCmd(app, "list", "--provider", "vision")
	require.NoError(t, err)
	assert.Contains(t, out, "ppocrv5")
	assert.Contains(t, out, "bundled")

	// But pull and prune refuse it.
	_, err = runModelsCmd(app, "pull", "ppocrv5")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bundled")
	_, err = runModelsCmd(app, "prune", "ppocrv5")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "bundled")
}

func TestBuildModelRows(t *testing.T) {
	t.Setenv("KAPI_MODELS_CACHE", t.TempDir())
	plugins := []pluginModel{
		{plugin: "sat", asset: mkModel("sat-3l-sm", true)},
	}
	installed := []aiprovider.OllamaModelInfo{
		{Name: "llama3.2:3b", Size: 2000000000}, // a recommended model, installed
		{Name: "mistral:7b", Size: 4000000000},  // an extra installed model
	}
	providers := aiprovider.Providers()

	rows := buildModelRows(plugins, installed, providers, "")

	// Ollama: recommended picks present; the installed one is marked installed,
	// the default is flagged, and the extra installed model is included.
	var llama, extra *output.ModelRow
	var sawCloud, sawPlugin bool
	for i := range rows {
		r := &rows[i]
		switch {
		case r.Source == output.ModelSourceOllama && r.Model == "llama3.2:3b":
			llama = r
		case r.Source == output.ModelSourceOllama && r.Model == "mistral:7b":
			extra = r
		case r.Source == output.ModelSourceCloud && r.Provider == "anthropic":
			sawCloud = true
		case r.Source == output.ModelSourcePlugin && r.Model == "sat-3l-sm":
			sawPlugin = true
		}
	}
	require.NotNil(t, llama, "default ollama model should be listed")
	assert.Equal(t, "installed", llama.Status)
	assert.True(t, llama.Default)
	require.NotNil(t, extra, "installed-but-not-recommended model should be listed")
	assert.Equal(t, "installed", extra.Status)
	assert.True(t, sawCloud, "cloud providers should appear")
	assert.True(t, sawPlugin, "plugin assets should appear")

	// Filter to one source.
	only := buildModelRows(plugins, installed, providers, output.ModelSourceCloud)
	require.NotEmpty(t, only)
	for _, r := range only {
		assert.Equal(t, output.ModelSourceCloud, r.Source)
	}
}

func TestResolveModelRefAmbiguous(t *testing.T) {
	app := appWith(
		mkPlugin("llm", mkModel("shared-id", true)),
		mkPlugin("other", mkModel("shared-id", true)),
	)
	_, _, err := app.resolveModelRef("shared-id")
	require.Error(t, err)
	require.ErrorContains(t, err, "multiple plugins")
	require.ErrorContains(t, err, "plugin/model")

	// ...but the ambiguity is resolvable by qualifying.
	p, a, err := app.resolveModelRef("other/shared-id")
	require.NoError(t, err)
	assert.Equal(t, "other", p)
	assert.Equal(t, "shared-id", a.ID)
}
