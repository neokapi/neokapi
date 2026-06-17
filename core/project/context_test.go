package project

import (
	"maps"
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- NewProjectContext ---

func TestNewProjectContext_Defaults(t *testing.T) {
	proj := &KapiProject{
		Version: CurrentVersion,
		Defaults: Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR", "de-DE"},
		},
	}
	ctx := NewProjectContext(proj, "/tmp/test/project.kapi")

	assert.Equal(t, "en-US", string(ctx.SourceLocale))
	assert.Len(t, ctx.TargetLocales, 2)
	assert.Equal(t, "fr-FR", string(ctx.TargetLocales[0]))
	assert.Equal(t, "UTF-8", ctx.Encoding)
	assert.Equal(t, "bcp-47", ctx.LocaleFormat)
	assert.Equal(t, 0, ctx.Concurrency)
	assert.Equal(t, 0, ctx.ParallelBlocks)
	assert.Equal(t, []string{registry.SourceBuiltIn}, ctx.AllowedSources)
}

func TestNewProjectContext_CustomDefaults(t *testing.T) {
	proj := &KapiProject{
		Version: CurrentVersion,
		Defaults: Defaults{
			SourceLanguage:  "ja-JP",
			TargetLanguages: []model.LocaleID{"en-US"},
			Encoding:        "Shift_JIS",
			Concurrency:     8,
			ParallelBlocks:  5,
			LocaleFormat:    "posix",
		},
	}
	ctx := NewProjectContext(proj, "/tmp/test/project.kapi")

	assert.Equal(t, "Shift_JIS", ctx.Encoding)
	assert.Equal(t, 8, ctx.Concurrency)
	assert.Equal(t, 5, ctx.ParallelBlocks)
	assert.Equal(t, "posix", ctx.LocaleFormat)
}

func TestNewProjectContext_WithPlugins(t *testing.T) {
	proj := &KapiProject{
		Version: CurrentVersion,
		Plugins: map[string]PluginSpec{
			"okapi-bridge": {},
			"my-plugin":    {},
		},
	}
	ctx := NewProjectContext(proj, "/tmp/test/project.kapi")

	assert.Contains(t, ctx.AllowedSources, registry.SourceBuiltIn)
	assert.Contains(t, ctx.AllowedSources, "okapi-bridge")
	assert.Contains(t, ctx.AllowedSources, "my-plugin")
	assert.Len(t, ctx.AllowedSources, 3)
}

func TestNewProjectContext_ProjectDir(t *testing.T) {
	dir := t.TempDir()
	kapiPath := filepath.Join(dir, "project.kapi")
	proj := &KapiProject{Version: CurrentVersion}
	ctx := NewProjectContext(proj, kapiPath)

	assert.Equal(t, dir, ctx.ProjectDir)
}

func TestNewProjectContext_FormatDefaults(t *testing.T) {
	proj := &KapiProject{
		Version: CurrentVersion,
		Defaults: Defaults{
			Formats: map[string]FormatDefaults{
				"html": {Preset: "strict", Config: map[string]any{"extractComments": true}},
			},
		},
	}
	ctx := NewProjectContext(proj, "/tmp/test/project.kapi")

	require.Contains(t, ctx.FormatDefaults, "html")
	assert.Equal(t, "strict", ctx.FormatDefaults["html"].Preset)
}

// --- DetectFormat ---

func TestDetectFormat_BuiltInOnly(t *testing.T) {
	reg := registry.NewFormatRegistry()
	registerBuiltIn(reg, "json", ".json")

	proj := &KapiProject{Version: CurrentVersion} // no plugins
	ctx := NewProjectContext(proj, "/tmp/test/project.kapi")

	assert.Equal(t, "json", ctx.DetectFormat(reg, "file.json"))
	assert.Empty(t, ctx.DetectFormat(reg, "file.unknown"))
}

func TestDetectFormat_PluginFiltered(t *testing.T) {
	reg := registry.NewFormatRegistry()
	registerBuiltIn(reg, "json", ".json")
	reg.RegisterFormatInfo("okf_json", registry.FormatInfo{
		Extensions: []string{".json"},
		Source:     "okapi-bridge",
		HasReader:  true,
	})
	reg.SetFormatPriority("okf_json", format.DefaultPluginPriority)

	// Without plugin: built-in wins.
	ctx := NewProjectContext(&KapiProject{Version: CurrentVersion}, "/tmp/test/project.kapi")
	assert.Equal(t, "json", ctx.DetectFormat(reg, "file.json"))

	// With plugin: plugin wins (higher priority).
	ctx2 := NewProjectContext(&KapiProject{
		Version: CurrentVersion,
		Plugins: map[string]PluginSpec{"okapi-bridge": {}},
	}, "/tmp/test/project.kapi")
	assert.Equal(t, "okf_json", ctx2.DetectFormat(reg, "file.json"))
}

func TestDetectFormat_EmptyExtension(t *testing.T) {
	reg := registry.NewFormatRegistry()
	ctx := NewProjectContext(&KapiProject{Version: CurrentVersion}, "/tmp/test/project.kapi")
	assert.Empty(t, ctx.DetectFormat(reg, "noext"))
}

// A recipe's defaults.formats[name].priority steers detection when several
// formats claim the same extension at equal priority (the real .srt case where
// okf_vtt and okf_regex collide and the alphabetical tiebreak picks okf_regex).
func TestDetectFormat_PriorityOverride(t *testing.T) {
	reg := registry.NewFormatRegistry()
	for _, name := range []string{"okf_regex", "okf_vtt"} {
		reg.RegisterFormatInfo(registry.FormatID(name), registry.FormatInfo{
			Extensions: []string{".srt"},
			Source:     "okapi-bridge",
			HasReader:  true,
		})
		reg.SetFormatPriority(registry.FormatID(name), format.DefaultPluginPriority)
	}

	plugins := map[string]PluginSpec{"okapi-bridge": {}}

	// Without an override, the tiebreak picks okf_regex.
	ctx := NewProjectContext(&KapiProject{Version: CurrentVersion, Plugins: plugins}, "/tmp/test/project.kapi")
	assert.Equal(t, "okf_regex", ctx.DetectFormat(reg, "clip.srt"))

	// defaults.formats pins okf_vtt as the preferred engine for .srt.
	ctx2 := NewProjectContext(&KapiProject{
		Version: CurrentVersion,
		Plugins: plugins,
		Defaults: Defaults{
			Formats: map[string]FormatDefaults{"okf_vtt": {Priority: 110}},
		},
	}, "/tmp/test/project.kapi")
	assert.Equal(t, "okf_vtt", ctx2.DetectFormat(reg, "clip.srt"))
}

// --- ResolveContent ---

func TestResolveContent_BasicGlob(t *testing.T) {
	dir := t.TempDir()
	createFile(t, dir, "input/hello.json", `{"key": "value"}`)
	createFile(t, dir, "input/world.json", `{"key": "value2"}`)
	createFile(t, dir, "input/readme.md", "# Hello")

	reg := registry.NewFormatRegistry()
	registerBuiltIn(reg, "json", ".json")
	registerBuiltIn(reg, "markdown", ".md")

	proj := &KapiProject{
		Version: CurrentVersion,
		Content: []ContentCollection{
			{Path: "input/*.json"},
		},
	}
	ctx := NewProjectContext(proj, filepath.Join(dir, "project.kapi"))

	files, err := ctx.ResolveContent(reg)
	require.NoError(t, err)
	assert.Len(t, files, 2)

	for _, f := range files {
		assert.Equal(t, "json", f.Format)
		assert.True(t, filepath.IsAbs(f.Path))
		assert.NotEmpty(t, f.Relative)
	}
}

func TestResolveContent_NamedCollection(t *testing.T) {
	dir := t.TempDir()
	createFile(t, dir, "docs/guide.md", "# Guide")
	createFile(t, dir, "store/ui.json", "{}")

	reg := registry.NewFormatRegistry()
	registerBuiltIn(reg, "json", ".json")
	registerBuiltIn(reg, "markdown", ".md")

	proj := &KapiProject{
		Version: CurrentVersion,
		Content: []ContentCollection{
			{
				Name: "Docs",
				Items: []ContentItem{
					{Path: "docs/*.md"},
				},
			},
			{
				Name: "Store",
				Items: []ContentItem{
					{Path: "store/*.json"},
				},
			},
		},
	}
	ctx := NewProjectContext(proj, filepath.Join(dir, "project.kapi"))

	files, err := ctx.ResolveContent(reg)
	require.NoError(t, err)
	assert.Len(t, files, 2)
	assert.Equal(t, "Docs", files[0].Collection)
	assert.Equal(t, "Store", files[1].Collection)
}

func TestResolveContent_ExplicitFormat(t *testing.T) {
	dir := t.TempDir()
	createFile(t, dir, "input/data.json", "{}")

	reg := registry.NewFormatRegistry()
	registerBuiltIn(reg, "json", ".json")
	reg.RegisterFormatInfo("okf_json", registry.FormatInfo{
		Extensions: []string{".json"},
		Source:     "okapi-bridge",
		HasReader:  true,
	})

	proj := &KapiProject{
		Version: CurrentVersion,
		Content: []ContentCollection{
			{
				Path:   "input/*.json",
				Format: &FormatSpec{Name: "okf_json"},
			},
		},
	}
	// No plugins declared — but explicit format should still be honored.
	ctx := NewProjectContext(proj, filepath.Join(dir, "project.kapi"))

	files, err := ctx.ResolveContent(reg)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "okf_json", files[0].Format, "explicit format overrides detection")
}

func TestResolveContent_RejectsParentTraversal(t *testing.T) {
	dir := t.TempDir()
	proj := &KapiProject{
		Version: CurrentVersion,
		Content: []ContentCollection{
			{Path: "../escape/*.json"},
		},
	}
	ctx := NewProjectContext(proj, filepath.Join(dir, "project.kapi"))

	files, err := ctx.ResolveContent(registry.NewFormatRegistry())
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestResolveContent_RejectsAbsolutePaths(t *testing.T) {
	dir := t.TempDir()
	proj := &KapiProject{
		Version: CurrentVersion,
		Content: []ContentCollection{
			{Path: "/etc/passwd"},
		},
	}
	ctx := NewProjectContext(proj, filepath.Join(dir, "project.kapi"))

	files, err := ctx.ResolveContent(registry.NewFormatRegistry())
	require.NoError(t, err)
	assert.Empty(t, files)
}

func TestResolveContent_Empty(t *testing.T) {
	proj := &KapiProject{Version: CurrentVersion}
	ctx := NewProjectContext(proj, "/tmp/test/project.kapi")

	files, err := ctx.ResolveContent(registry.NewFormatRegistry())
	require.NoError(t, err)
	assert.Nil(t, files)
}

func TestResolveContent_PluginScopedDetection(t *testing.T) {
	dir := t.TempDir()
	createFile(t, dir, "input/page.html", "<html></html>")

	reg := registry.NewFormatRegistry()
	registerBuiltIn(reg, "html", ".html")
	reg.RegisterFormatInfo("okf_html", registry.FormatInfo{
		Extensions: []string{".html"},
		Source:     "okapi-bridge",
		HasReader:  true,
	})
	reg.SetFormatPriority("okf_html", format.DefaultPluginPriority)

	// Project without okapi-bridge plugin.
	proj := &KapiProject{
		Version: CurrentVersion,
		Content: []ContentCollection{
			{Path: "input/*.html"},
		},
	}
	ctx := NewProjectContext(proj, filepath.Join(dir, "project.kapi"))

	files, err := ctx.ResolveContent(reg)
	require.NoError(t, err)
	require.Len(t, files, 1)
	assert.Equal(t, "html", files[0].Format, "should use built-in format when plugin not declared")
}

// --- ConfigureReader ---

func TestConfigureReader_AppliesConfig(t *testing.T) {
	proj := &KapiProject{
		Version: CurrentVersion,
		Defaults: Defaults{
			Formats: map[string]FormatDefaults{
				"json": {Config: map[string]any{"indent": true}},
			},
		},
	}
	ctx := NewProjectContext(proj, "/tmp/test/project.kapi")

	reader := &stubConfigurable{applied: make(map[string]any)}
	err := ctx.ConfigureReader(reader, "json")
	require.NoError(t, err)
	assert.Equal(t, true, reader.applied["indent"])
}

func TestConfigureReader_NoDefaults(t *testing.T) {
	ctx := NewProjectContext(&KapiProject{Version: CurrentVersion}, "/tmp/test/project.kapi")

	reader := &stubConfigurable{applied: make(map[string]any)}
	err := ctx.ConfigureReader(reader, "json")
	require.NoError(t, err)
	assert.Empty(t, reader.applied, "should not apply anything when no format defaults")
}

func TestConfigureReader_NoConfig(t *testing.T) {
	ctx := NewProjectContext(&KapiProject{
		Version: CurrentVersion,
		Defaults: Defaults{
			Formats: map[string]FormatDefaults{
				"json": {Config: map[string]any{"x": 1}},
			},
		},
	}, "/tmp/test/project.kapi")

	reader := &stubNoConfig{}
	err := ctx.ConfigureReader(reader, "json")
	require.NoError(t, err) // should be a no-op, not an error
}

// --- AllowedTools ---

func TestAllowedTools_BuiltInOnly(t *testing.T) {
	ctx := NewProjectContext(&KapiProject{Version: CurrentVersion}, "/tmp/test/project.kapi")

	allTools := []registry.ToolInfo{
		{Name: "pseudo-translate", Source: registry.SourceBuiltIn},
		{Name: "qa-check", Source: ""}, // empty = built-in
		{Name: "okf-step", Source: "okapi-bridge"},
	}

	allowed := ctx.AllowedTools(allTools)
	assert.Len(t, allowed, 2)
	names := toolNames(allowed)
	assert.Contains(t, names, "pseudo-translate")
	assert.Contains(t, names, "qa-check")
	assert.NotContains(t, names, "okf-step")
}

func TestAllowedTools_WithPlugin(t *testing.T) {
	ctx := NewProjectContext(&KapiProject{
		Version: CurrentVersion,
		Plugins: map[string]PluginSpec{"okapi-bridge": {}},
	}, "/tmp/test/project.kapi")

	allTools := []registry.ToolInfo{
		{Name: "pseudo-translate", Source: registry.SourceBuiltIn},
		{Name: "okf-step", Source: "okapi-bridge"},
		{Name: "other-step", Source: "other-plugin"},
	}

	allowed := ctx.AllowedTools(allTools)
	assert.Len(t, allowed, 2)
	names := toolNames(allowed)
	assert.Contains(t, names, "pseudo-translate")
	assert.Contains(t, names, "okf-step")
	assert.NotContains(t, names, "other-step")
}

// --- ValidateFlows ---

func TestValidateFlows_AllValid(t *testing.T) {
	ctx := NewProjectContext(&KapiProject{
		Version: CurrentVersion,
		Flows: map[string]*flow.StepsSpec{
			"translate": {Steps: []flow.FlowStep{
				{Tool: "ai-translate"},
				{Tool: "qa-check"},
			}},
		},
	}, "/tmp/test/project.kapi")

	allTools := []registry.ToolInfo{
		{Name: "ai-translate", Source: registry.SourceBuiltIn},
		{Name: "qa-check", Source: registry.SourceBuiltIn},
	}

	issues := ctx.ValidateFlows(allTools)
	assert.Nil(t, issues)
}

func TestValidateFlows_UndeclaredPlugin(t *testing.T) {
	ctx := NewProjectContext(&KapiProject{
		Version: CurrentVersion,
		// No plugins declared
		Flows: map[string]*flow.StepsSpec{
			"bridge-flow": {Steps: []flow.FlowStep{
				{Tool: "okf-segmentation"},
			}},
		},
	}, "/tmp/test/project.kapi")

	allTools := []registry.ToolInfo{
		{Name: "okf-segmentation", Source: "okapi-bridge"},
	}

	issues := ctx.ValidateFlows(allTools)
	require.Len(t, issues, 1)
	assert.Equal(t, "bridge-flow", issues[0].FlowName)
	assert.Equal(t, "okf-segmentation", issues[0].StepTool)
	assert.Equal(t, "undeclared_plugin", issues[0].Type)
	assert.Equal(t, "okapi-bridge", issues[0].Source)
	assert.Contains(t, issues[0].Message, "okapi-bridge")
}

func TestValidateFlows_ParallelSteps(t *testing.T) {
	ctx := NewProjectContext(&KapiProject{
		Version: CurrentVersion,
		Flows: map[string]*flow.StepsSpec{
			"parallel": {Steps: []flow.FlowStep{
				{Parallel: []flow.FlowStep{
					{Tool: "ai-translate"},
					{Tool: "okf-step"},
				}},
			}},
		},
	}, "/tmp/test/project.kapi")

	allTools := []registry.ToolInfo{
		{Name: "ai-translate", Source: registry.SourceBuiltIn},
		{Name: "okf-step", Source: "okapi-bridge"},
	}

	issues := ctx.ValidateFlows(allTools)
	require.Len(t, issues, 1)
	assert.Equal(t, "okf-step", issues[0].StepTool)
}

func TestValidateFlows_NoFlows(t *testing.T) {
	ctx := NewProjectContext(&KapiProject{Version: CurrentVersion}, "/tmp/test/project.kapi")
	assert.Nil(t, ctx.ValidateFlows(nil))
}

func TestValidateFlows_UnknownToolFlagged(t *testing.T) {
	ctx := NewProjectContext(&KapiProject{
		Version: CurrentVersion,
		Flows: map[string]*flow.StepsSpec{
			"custom": {Steps: []flow.FlowStep{
				{Tool: "unknown-tool"},
			}},
		},
	}, "/tmp/test/project.kapi")

	issues := ctx.ValidateFlows([]registry.ToolInfo{})
	require.Len(t, issues, 1)
	assert.Equal(t, "unknown", issues[0].Type)
	assert.Equal(t, "unknown-tool", issues[0].StepTool)
	assert.Equal(t, "custom", issues[0].FlowName)
	assert.Contains(t, issues[0].Message, "not installed")
}

// --- Helpers ---

func createFile(t *testing.T, base, rel, content string) {
	t.Helper()
	path := filepath.Join(base, rel)
	require.NoError(t, os.MkdirAll(filepath.Dir(path), 0o755))
	require.NoError(t, os.WriteFile(path, []byte(content), 0o644))
}

func toolNames(tools []registry.ToolInfo) []string {
	names := make([]string, len(tools))
	for i, t := range tools {
		names[i] = string(t.Name)
	}
	return names
}

// registerBuiltIn registers a format with "built-in" source and default priority.
func registerBuiltIn(reg *registry.FormatRegistry, name, ext string) {
	reg.RegisterFormatInfo(registry.FormatID(name), registry.FormatInfo{
		Extensions: []string{ext},
		Source:     registry.SourceBuiltIn,
		HasReader:  true,
		Priority:   format.DefaultBuiltInPriority,
	})
}

// stubConfigurable implements Configurable with a config that tracks ApplyMap calls.
type stubConfigurable struct {
	applied map[string]any
}

func (r *stubConfigurable) Config() format.DataFormatConfig {
	return &stubConfig{applied: r.applied}
}

type stubConfig struct {
	applied map[string]any
}

func (c *stubConfig) FormatName() string { return "stub" }
func (c *stubConfig) Reset()             {}
func (c *stubConfig) Validate() error    { return nil }
func (c *stubConfig) ApplyMap(values map[string]any) error {
	maps.Copy(c.applied, values)
	return nil
}

// stubNoConfig implements Configurable but returns nil config.
type stubNoConfig struct{}

func (r *stubNoConfig) Config() format.DataFormatConfig { return nil }
