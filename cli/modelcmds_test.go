package cli

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
	assert.ErrorContains(t, err, "no model or plugin")

	// Explicit pair with a wrong model.
	_, _, err = app.resolveModelRef("llm/nope")
	assert.ErrorContains(t, err, "no model")
}

func TestResolveModelRefAmbiguous(t *testing.T) {
	app := appWith(
		mkPlugin("llm", mkModel("shared-id", true)),
		mkPlugin("other", mkModel("shared-id", true)),
	)
	_, _, err := app.resolveModelRef("shared-id")
	require.Error(t, err)
	assert.ErrorContains(t, err, "multiple plugins")
	assert.ErrorContains(t, err, "plugin/model")

	// ...but the ambiguity is resolvable by qualifying.
	p, a, err := app.resolveModelRef("other/shared-id")
	require.NoError(t, err)
	assert.Equal(t, "other", p)
	assert.Equal(t, "shared-id", a.ID)
}
