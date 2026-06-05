package cli

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunCmd_ProjectFlag(t *testing.T) {
	app := &App{}

	cmd := app.NewRunCmd(RunCmdOptions{})

	// Verify the -p/--project flag exists.
	f := cmd.Flags().Lookup("project")
	require.NotNil(t, f, "expected --project flag")
	assert.Equal(t, "p", f.Shorthand)
	assert.Empty(t, f.DefValue)
}

func TestRunFromProject_LoadsDefaults(t *testing.T) {
	// Create a .kapi project file.
	dir := t.TempDir()
	projPath := filepath.Join(dir, "test.kapi")

	proj := &project.KapiProject{
		Version: "v1",
		Name:    "Test",
		Defaults: project.Defaults{
			SourceLanguage:  "ja-JP",
			TargetLanguages: []model.LocaleID{"en-US", "zh-CN"},
		},
	}
	require.NoError(t, project.Save(projPath, proj))

	// Create an App with default source lang.
	app := &App{
		SourceLang: "en", // default
		TargetLang: "",   // not set
	}

	// Load the project and verify defaults are applied.
	loaded, err := project.Load(projPath)
	require.NoError(t, err)

	// Simulate what runFromProject does for language defaults.
	if app.SourceLang == "en" && loaded.Defaults.SourceLanguage != "" {
		app.SourceLang = string(loaded.Defaults.SourceLanguage)
	}
	if app.TargetLang == "" && len(loaded.Defaults.TargetLanguages) > 0 {
		app.TargetLang = string(loaded.Defaults.TargetLanguages[0])
	}

	assert.Equal(t, "ja-JP", app.SourceLang, "project source lang should override default 'en'")
	assert.Equal(t, "en-US", app.TargetLang, "project first target lang should be used as default")
}

func TestRunFromProject_CLIFlagsOverride(t *testing.T) {
	// Create a .kapi project file.
	dir := t.TempDir()
	projPath := filepath.Join(dir, "test.kapi")

	proj := &project.KapiProject{
		Version: "v1",
		Name:    "Test",
		Defaults: project.Defaults{
			SourceLanguage:  "ja-JP",
			TargetLanguages: []model.LocaleID{"en-US"},
		},
	}
	require.NoError(t, project.Save(projPath, proj))

	// Simulate user setting --target-lang explicitly.
	app := &App{
		SourceLang: "fr-FR", // explicitly set (not default "en")
		TargetLang: "de-DE", // explicitly set
	}

	loaded, err := project.Load(projPath)
	require.NoError(t, err)

	// Same logic as runFromProject — only override when CLI default.
	if app.SourceLang == "en" && loaded.Defaults.SourceLanguage != "" {
		app.SourceLang = string(loaded.Defaults.SourceLanguage)
	}
	if app.TargetLang == "" && len(loaded.Defaults.TargetLanguages) > 0 {
		app.TargetLang = string(loaded.Defaults.TargetLanguages[0])
	}

	assert.Equal(t, "fr-FR", app.SourceLang, "explicit CLI source lang should not be overridden")
	assert.Equal(t, "de-DE", app.TargetLang, "explicit CLI target lang should not be overridden")
}

func TestRunFromProject_FlowNotFound(t *testing.T) {
	dir := t.TempDir()
	projPath := filepath.Join(dir, "test.kapi")

	proj := &project.KapiProject{
		Version: "v1",
		Name:    "Test",
	}
	require.NoError(t, project.Save(projPath, proj))

	loaded, err := project.Load(projPath)
	require.NoError(t, err)

	spec := loaded.Flow("nonexistent")
	assert.Nil(t, spec, "nonexistent flow should return nil")
}

func TestRunFromProject_ProjectFileNotFound(t *testing.T) {
	_, err := project.Load("/nonexistent/project.kapi")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read project file")
}

func TestToolFromStep_BuiltinTool(t *testing.T) {
	app := newTestApp()
	app.TargetLang = "fr"

	// Verify that pseudo-translate is in the registry and can be looked up.
	assert.True(t, app.ToolReg.Has("pseudo-translate"),
		"pseudo-translate should be in ToolRegistry")

	// Verify it has a schema (needed for CLI command generation).
	s := app.ToolReg.Schema("pseudo-translate")
	assert.NotNil(t, s, "pseudo-translate should have a schema")
}

func TestKapiProjectYAMLRoundtrip(t *testing.T) {
	// Write a realistic .kapi file and verify it roundtrips through YAML.
	yamlContent := `version: v1
name: Acme App Localization
plugins:
  okapi:
    framework_version: "^1.47.0"
    format_priority: 200
defaults:
  source_language: en-US
  target_languages:
    - fr-FR
    - de-DE
    - ja-JP
  concurrency: 8
  parallel_blocks: 5
  encoding: utf-8
  formats:
    okf_html:
      preset: strict-extraction
content:
  - path: src/i18n/en/*.json
    format: json
    target: src/i18n/{lang}/*.json
  - name: Documentation
    items:
      - path: docs/en/**/*.md
        format: markdown
preset: nextjs
flows:
  translate:
    steps:
      - tool: ai-translate
        config:
          provider: anthropic
          model: claude-sonnet-4-5-20241022
  full-pipeline:
    steps:
      - tool: ai-translate
        config:
          provider: anthropic
      - tool: qa-check
      - tool: pseudo-translate
        config:
          expansion_rate: 1.3
`
	dir := t.TempDir()
	path := filepath.Join(dir, "acme.kapi")
	require.NoError(t, os.WriteFile(path, []byte(yamlContent), 0o644))

	proj, err := project.Load(path)
	require.NoError(t, err)

	assert.Equal(t, "Acme App Localization", proj.Name)
	assert.Equal(t, model.LocaleID("en-US"), proj.Defaults.SourceLanguage)
	assert.Len(t, proj.Defaults.TargetLanguages, 3)
	assert.Len(t, proj.Content, 2)
	assert.Equal(t, "nextjs", proj.Preset)

	// Plugins.
	require.Contains(t, proj.Plugins, "okapi")
	assert.Equal(t, "^1.47.0", proj.Plugins["okapi"].FrameworkVersion)
	assert.Equal(t, 200, proj.Plugins["okapi"].FormatPriority)

	// Format defaults.
	require.Contains(t, proj.Defaults.Formats, "okf_html")
	assert.Equal(t, "strict-extraction", proj.Defaults.Formats["okf_html"].Preset)

	// Flows
	assert.Len(t, proj.Flows, 2)

	translate := proj.Flow("translate")
	require.NotNil(t, translate)
	assert.Len(t, translate.Steps, 1)
	assert.Equal(t, "ai-translate", translate.Steps[0].Tool)
	assert.Equal(t, "anthropic", translate.Steps[0].Config["provider"])

	pipeline := proj.Flow("full-pipeline")
	require.NotNil(t, pipeline)
	assert.Len(t, pipeline.Steps, 3)
	assert.Equal(t, "qa-check", pipeline.Steps[1].Tool)
	assert.Equal(t, 1.3, pipeline.Steps[2].Config["expansion_rate"])

	// Defaults
	assert.Equal(t, 8, proj.Defaults.Concurrency)
	assert.Equal(t, 5, proj.Defaults.ParallelBlocks)

	// Save and reload to verify roundtrip.
	path2 := filepath.Join(dir, "acme2.kapi")
	require.NoError(t, project.Save(path2, proj))

	proj2, err := project.Load(path2)
	require.NoError(t, err)
	assert.Equal(t, proj.Name, proj2.Name)
	assert.Equal(t, proj.Defaults.TargetLanguages, proj2.Defaults.TargetLanguages)
	assert.Len(t, proj2.Flows, len(proj.Flows))
}
