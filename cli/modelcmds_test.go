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

func TestModelsListText(t *testing.T) {
	t.Setenv("KAPI_MODELS_CACHE", t.TempDir()) // isolate cache → "not cached"
	app := appWith(mkPlugin("llm", mkModel("gemma-4-e2b", true), mkModel("gemma-tiny", false)))

	out, err := runModelsCmd(app, "list")
	require.NoError(t, err)
	assert.Contains(t, out, "PLUGIN")
	assert.Contains(t, out, "gemma-4-e2b (default)")
	assert.Contains(t, out, "gemma-tiny")
	assert.Contains(t, out, "not cached")
	// tabwriter keeps the header and rows column-aligned: the MODEL header and
	// each model name start at the same column.
	lines := strings.Split(strings.TrimSpace(out), "\n")
	require.GreaterOrEqual(t, len(lines), 3)
	col := strings.Index(lines[0], "MODEL")
	require.Positive(t, col)
	for _, ln := range lines[1:] {
		assert.Equal(t, "gemma", ln[col:col+5], "MODEL column misaligned: %q", ln)
	}
}

func TestModelsListJSON(t *testing.T) {
	t.Setenv("KAPI_MODELS_CACHE", t.TempDir())
	app := appWith(mkPlugin("llm", mkModel("gemma-4-e2b", true)))

	out, err := runModelsCmd(app, "list", "--json")
	require.NoError(t, err)
	var got struct {
		Models []struct {
			Plugin, Model, Status string
			Default               bool
		} `json:"models"`
		Total int `json:"total"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &got), "output must be JSON: %s", out)
	require.Equal(t, 1, got.Total)
	assert.Equal(t, "llm", got.Models[0].Plugin)
	assert.Equal(t, "gemma-4-e2b", got.Models[0].Model)
	assert.True(t, got.Models[0].Default)
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
