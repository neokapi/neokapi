package host

import (
	"context"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/manifest"
	"github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/core/tool"
	"github.com/neokapi/neokapi/host/config"
	"github.com/neokapi/neokapi/host/pluginhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// pluginJSON registers okf_json on the app's registry as a second engine for
// ".json": a plugin format (source okapi-bridge) whose reader and writer are
// the built-in JSON ones, counted on every construction. It stands in for an
// installed okapi-bridge in the one place that matters to detection, the
// registry; nothing is mocked in the plugin host.
type pluginJSON struct {
	readers int32
	writers int32
}

func registerPluginJSON(t *testing.T, reg *registry.FormatRegistry) *pluginJSON {
	t.Helper()
	p := &pluginJSON{}
	readJSON := reg.ReaderFactory("json")
	writeJSON := reg.WriterFactory("json")
	require.NotNil(t, readJSON)
	require.NotNil(t, writeJSON)
	sig := format.FormatSignature{Extensions: []string{".json"}}
	reg.RegisterFormatInfo("okf_json", registry.FormatInfo{
		DisplayName: "Okapi JSON", Extensions: sig.Extensions, Source: "okapi-bridge", HasReader: true, HasWriter: true,
	})
	reg.RegisterReader("okf_json", func() format.DataFormatReader {
		atomic.AddInt32(&p.readers, 1)
		return readJSON()
	}, sig, "Okapi JSON")
	reg.RegisterWriter("okf_json", func() format.DataFormatWriter {
		atomic.AddInt32(&p.writers, 1)
		return writeJSON()
	})
	reg.SetFormatSource("okf_json", "okapi-bridge")
	return p
}

func (p *pluginJSON) reads() int  { return int(atomic.SwapInt32(&p.readers, 0)) }
func (p *pluginJSON) writes() int { return int(atomic.SwapInt32(&p.writers, 0)) }

func engineTestApp(t *testing.T) (*App, *pluginJSON) {
	t.Helper()
	a := &App{SourceLang: "en-US", TargetLang: "qps", Quiet: true, Config: config.NewAppConfig()}
	a.InitRegistries()
	// The recipe's plugin check asks the plugin host what is installed; the
	// manifest alone answers it.
	a.PluginHost = pluginhost.NewHost([]*pluginhost.Plugin{{
		Manifest: &manifest.Manifest{Plugin: "okapi-bridge", Version: "1.0.0"},
	}}, nil)
	return a, registerPluginJSON(t, a.FormatReg)
}

// flowRunCmd builds a command carrying the flow-run flags. Registering a
// flag resets the field it binds, so the processing fields are restored
// after the command is built.
func flowRunCmd(a *App, name string) *EnvCommand {
	fmtFlag, engine, src, tgt := a.FormatFlag, a.EngineFlag, a.SourceLang, a.TargetLang
	cmd := NewEnvCommand(context.Background(), name)
	a.AddFlowRunFlags(cmd)
	a.FormatFlag, a.EngineFlag, a.SourceLang, a.TargetLang = fmtFlag, engine, src, tgt
	return cmd
}

func writeMessagesJSON(t *testing.T, dir string) string {
	t.Helper()
	src := filepath.Join(dir, "messages.json")
	require.NoError(t, os.WriteFile(src, []byte(`{"greeting":"Hello, world."}`), 0o644))
	return src
}

// runExec reads src the way `kapi exec <tool> src` does.
func runExec(t *testing.T, a *App, src string) {
	t.Helper()
	require.NoError(t, a.RunToolOnFiles(context.Background(), ToolRunConfig{
		ToolName: "pass-through", Files: []string{src}, FailOnUnknown: true,
		NewTool: func() (tool.Tool, error) {
			return &passThrough{BaseTool: &tool.BaseTool{ToolName: "pass-through"}}, nil
		},
	}))
}

// runSingleFile reads src the way `kapi pseudo-translate src` does, writing
// beside it.
func runSingleFile(t *testing.T, a *App, src string) {
	t.Helper()
	cmd := flowRunCmd(a, "pseudo-translate")
	require.NoError(t, cmd.Flags().Set("output", filepath.Join(filepath.Dir(src), "out", "{name}_{lang}.{ext}")))
	require.NoError(t, a.RunSingleFile(context.Background(), cmd, "pseudo-translate", src))
}

// A file resolves to the same engine from every entry point: native first
// by default, and the engine --engine names when it is given. An explicit
// --format wins over both.
func TestEngineSelection_SameEngineFromEveryEntryPoint(t *testing.T) {
	a, p := engineTestApp(t)
	src := writeMessagesJSON(t, t.TempDir())

	runExec(t, a, src)
	assert.Equal(t, 0, p.reads(), "exec reads with the built-in json")
	runSingleFile(t, a, src)
	assert.Equal(t, 0, p.reads(), "a single-file flow run reads with the built-in json")
	assert.Equal(t, 0, p.writes())

	a.EngineFlag = "okapi-bridge"
	require.NoError(t, a.ApplyEngineSelection())
	runExec(t, a, src)
	assert.Equal(t, 1, p.reads(), "exec reads with the plugin's json under --engine")
	runSingleFile(t, a, src)
	assert.Equal(t, 1, p.reads(), "a single-file flow run reads with the plugin's json under --engine")
	assert.Equal(t, 1, p.writes(), "and writes with it")

	a.FormatFlag = "json"
	runExec(t, a, src)
	runSingleFile(t, a, src)
	assert.Equal(t, 0, p.reads(), "--format names the format outright")
	assert.Equal(t, 0, p.writes(), "and pins the writer to the same engine (#431)")
}

// --engine must name an installed engine.
func TestEngineSelection_RejectsAnUnknownEngine(t *testing.T) {
	a, _ := engineTestApp(t)
	a.EngineFlag = "okapi"
	err := a.ApplyEngineSelection()
	require.ErrorIs(t, err, registry.ErrNoEngine)
	assert.Contains(t, err.Error(), "--engine")
	assert.Contains(t, err.Error(), "native, okapi-bridge")
}

// The config's formats.engine is the default the flag and the recipe rank
// above.
func TestEngineSelection_ConfigDefault(t *testing.T) {
	a, p := engineTestApp(t)
	src := writeMessagesJSON(t, t.TempDir())
	a.Config.Set(config.KeyFormatsEngine, "okapi-bridge")
	require.NoError(t, a.ApplyEngineSelection())

	runExec(t, a, src)
	assert.Equal(t, 1, p.reads())

	a.EngineFlag = "native"
	require.NoError(t, a.ApplyEngineSelection())
	runExec(t, a, src)
	assert.Equal(t, 0, p.reads())
}

// A project run reads with the recipe's engine: native unless
// defaults.engine names the plugin, and never a plugin the recipe does not
// declare, whatever the run prefers.
func TestEngineSelection_ProjectRun(t *testing.T) {
	run := func(t *testing.T, proj *project.KapiProject, engineFlag string) *pluginJSON {
		t.Helper()
		a, p := engineTestApp(t)
		dir, err := filepath.EvalSymlinks(t.TempDir())
		require.NoError(t, err)
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "src"), 0o755))
		writeMessagesJSON(t, filepath.Join(dir, "src"))
		recipe := filepath.Join(dir, project.RecipeFileName)
		proj.Version = project.CurrentVersion
		proj.Name = "engine"
		proj.Defaults.SourceLanguage = "en-US"
		proj.Defaults.TargetLanguages = []model.LocaleID{"qps"}
		proj.Collections = []project.Collection{{Path: "src/*.json", Target: "out/{lang}/*.json"}}
		proj.Flows = map[string]*flow.StepsSpec{"pseudo": {Steps: []flow.FlowStep{{Tool: "pseudo-translate"}}}}
		require.NoError(t, project.Save(recipe, proj))

		cmd := flowRunCmd(a, "run")
		AddProjectFlag(cmd)
		require.NoError(t, cmd.Flags().Set("project", recipe))
		require.NoError(t, cmd.Flags().Set("output", filepath.Join(dir, "out", "{lang}", "{name}.{ext}")))
		a.EngineFlag = engineFlag
		require.NoError(t, a.ApplyEngineSelection())
		require.NoError(t, a.RunFromProject(cmd, "pseudo", recipe, RunCmdOptions{}))
		return p
	}

	p := run(t, &project.KapiProject{Plugins: map[string]project.PluginSpec{"okapi-bridge": {}}}, "")
	assert.Equal(t, 0, p.reads(), "a declared plugin does not take an extension a built-in claims")

	p = run(t, &project.KapiProject{
		Plugins:  map[string]project.PluginSpec{"okapi-bridge": {}},
		Defaults: project.Defaults{Engine: "okapi-bridge"},
	}, "")
	assert.Positive(t, p.reads(), "defaults.engine prefers the plugin")
	assert.Positive(t, p.writes())

	p = run(t, &project.KapiProject{
		Plugins:  map[string]project.PluginSpec{"okapi-bridge": {}},
		Defaults: project.Defaults{Engine: "okapi-bridge"},
	}, "native")
	assert.Equal(t, 0, p.reads(), "--engine ranks above defaults.engine")

	p = run(t, &project.KapiProject{}, "okapi-bridge")
	assert.Equal(t, 0, p.reads(), "a plugin the recipe does not declare serves nothing in the project")
}

// The toolbox resolves a named file through the same resolver.
func TestEngineSelection_ToolboxResolvesThroughTheRegistry(t *testing.T) {
	a, _ := engineTestApp(t)
	src := writeMessagesJSON(t, t.TempDir())
	content, err := os.ReadFile(src)
	require.NoError(t, err)

	name, err := a.ResolveFormatName(src, content)
	require.NoError(t, err)
	assert.Equal(t, "json", name)

	a.EngineFlag = "okapi-bridge"
	require.NoError(t, a.ApplyEngineSelection())
	name, err = a.ResolveFormatName(src, content)
	require.NoError(t, err)
	assert.Equal(t, "okf_json", name)
}
