package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// registerPluginJSON adds a second engine for ".json" to the app's registry:
// a plugin format whose reader is the built-in JSON one, the way the
// registry looks with okapi-bridge installed.
func registerPluginJSON(t *testing.T, app *App) {
	t.Helper()
	readJSON := app.formatReg.ReaderFactory("json")
	require.NotNil(t, readJSON)
	sig := format.FormatSignature{Extensions: []string{".json"}}
	app.formatReg.RegisterFormatInfo("okf_json", registry.FormatInfo{
		DisplayName: "Okapi JSON", Extensions: sig.Extensions, Source: "okapi-bridge", HasReader: true,
	})
	app.formatReg.RegisterReader("okf_json", readJSON, sig, "Okapi JSON")
	app.formatReg.SetFormatSource("okf_json", "okapi-bridge")
}

func formatsByName(app *App) map[string]FormatInfo {
	out := map[string]FormatInfo{}
	for _, fi := range app.ListFormats() {
		out[fi.Name] = fi
	}
	return out
}

// The listing names each format's engine and priority, and the extensions
// another engine also claims.
func TestListFormats_EngineAndSharedExtensions(t *testing.T) {
	app := NewApp()
	before := formatsByName(app)
	require.Contains(t, before, "json")
	assert.Equal(t, registry.EngineNative, before["json"].Engine)
	assert.Equal(t, "built-in", before["json"].Source)
	assert.Equal(t, format.DefaultBuiltInPriority, before["json"].Priority)
	assert.Empty(t, before["json"].SharedExtensions, "one engine reads .json")

	registerPluginJSON(t, app)
	after := formatsByName(app)
	require.Contains(t, after, "okf_json")
	assert.Equal(t, "okapi-bridge", after["okf_json"].Engine)
	assert.Equal(t, "okapi-bridge", after["okf_json"].Source)
	assert.Equal(t, []string{".json"}, after["okf_json"].SharedExtensions)
	assert.Equal(t, []string{".json"}, after["json"].SharedExtensions)
	assert.Empty(t, after["markdown"].SharedExtensions)
}

// Detection outside a project is native first, and prefers the engine the
// config's formats.engine names, as the CLI does.
func TestDetectFormat_ConfigEngineDefault(t *testing.T) {
	app := NewApp()
	registerPluginJSON(t, app)
	assert.Equal(t, "json", app.DetectFormat("file.json"))
	assert.Equal(t, "json", app.DetectProjectFormat("none", "file.json"))

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "kapi.yaml"), []byte("formats:\n  engine: okapi-bridge\n"), 0o644))
	t.Setenv("KAPI_CONFIG_DIR", dir)
	app = NewApp()
	registerPluginJSON(t, app)
	assert.Equal(t, "okf_json", app.DetectFormat("file.json"))
	assert.Equal(t, "markdown", app.DetectFormat("notes.md"), "an extension the plugin does not claim stays native")
}

// A project's detection follows its recipe: a declared plugin serves a file
// when defaults.engine prefers it or a per-format pin names it, and never
// when the recipe does not declare it.
func TestDetectProjectFormat_FollowsTheRecipe(t *testing.T) {
	app := NewApp()
	registerPluginJSON(t, app)
	tab := newTestProject(t, app, "Engines")
	proj := app.GetProject(tab.ID)
	require.NotNil(t, proj)

	assert.Equal(t, "json", app.DetectProjectFormat(tab.ID, "file.json"), "no plugin declared")

	proj.Plugins = map[string]project.PluginSpec{"okapi-bridge": {}}
	require.NoError(t, app.UpdateProject(tab.ID, proj))
	assert.Equal(t, "json", app.DetectProjectFormat(tab.ID, "file.json"), "declared, native still first")

	proj.Defaults.Engine = "okapi-bridge"
	require.NoError(t, app.UpdateProject(tab.ID, proj))
	assert.Equal(t, "okf_json", app.DetectProjectFormat(tab.ID, "file.json"), "defaults.engine prefers the plugin")

	proj.Defaults.Engine = ""
	proj.Defaults.Formats = map[string]project.FormatDefaults{".json": {Engine: "okapi-bridge"}}
	require.NoError(t, app.UpdateProject(tab.ID, proj))
	assert.Equal(t, "okf_json", app.DetectProjectFormat(tab.ID, "file.json"), "a per-format pin names the plugin")
}
