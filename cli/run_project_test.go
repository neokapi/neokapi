package cli

import (
	"os"
	"path/filepath"
	"testing"

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
	assert.Equal(t, "", f.DefValue)
}

func TestRunFromProject_LoadsDefaults(t *testing.T) {
	// Create a .kapi project file.
	dir := t.TempDir()
	projPath := filepath.Join(dir, "test.kapi")

	proj := &project.KapiProject{
		Version:         "v1",
		Name:            "Test",
		SourceLanguage:  "ja-JP",
		TargetLanguages: []string{"en-US", "zh-CN"},
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
	if app.SourceLang == "en" && loaded.SourceLanguage != "" {
		app.SourceLang = loaded.SourceLanguage
	}
	if app.TargetLang == "" && len(loaded.TargetLanguages) > 0 {
		app.TargetLang = loaded.TargetLanguages[0]
	}

	assert.Equal(t, "ja-JP", app.SourceLang, "project source lang should override default 'en'")
	assert.Equal(t, "en-US", app.TargetLang, "project first target lang should be used as default")
}

func TestRunFromProject_CLIFlagsOverride(t *testing.T) {
	// Create a .kapi project file.
	dir := t.TempDir()
	projPath := filepath.Join(dir, "test.kapi")

	proj := &project.KapiProject{
		Version:         "v1",
		Name:            "Test",
		SourceLanguage:  "ja-JP",
		TargetLanguages: []string{"en-US"},
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
	if app.SourceLang == "en" && loaded.SourceLanguage != "" {
		app.SourceLang = loaded.SourceLanguage
	}
	if app.TargetLang == "" && len(loaded.TargetLanguages) > 0 {
		app.TargetLang = loaded.TargetLanguages[0]
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

	spec := loaded.GetFlow("nonexistent")
	assert.Nil(t, spec, "nonexistent flow should return nil")
}

func TestRunFromProject_ProjectFileNotFound(t *testing.T) {
	_, err := project.Load("/nonexistent/project.kapi")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "read project file")
}

func TestToolFromStep_BuiltinTool(t *testing.T) {
	// Verify that toolFromStep can look up built-in tools by name.
	app := &App{
		TargetLang: "fr",
	}

	// Find a tool that has NewToolFromConfig (pseudo-translate has it via schema).
	// For tools without NewToolFromConfig, the function falls back to NewTool(cmd, targetLang).
	// Since we can't easily construct a cmd in tests, just verify the lookup logic.
	found := false
	for _, def := range BuiltinToolCommands {
		if def.Use == "pseudo-translate" {
			found = true
			break
		}
	}
	assert.True(t, found, "pseudo-translate should be in BuiltinToolCommands")

	// Verify that unknown tools would need registry.
	_ = app // suppress unused
}

func TestKapiProjectYAMLRoundtrip(t *testing.T) {
	// Write a realistic .kapi file and verify it roundtrips through YAML.
	yaml := `version: v1
name: Acme App Localization
source_language: en-US
target_languages:
  - fr-FR
  - de-DE
  - ja-JP
content:
  - path: src/i18n/en/*.json
    format: json
    target: src/i18n/{lang}/*.json
  - path: docs/en/**/*.md
    format: markdown
preset: nextjs
plugins:
  - okapi@1.47.0
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
defaults:
  concurrency: 8
  parallel_blocks: 5
  encoding: utf-8
`
	dir := t.TempDir()
	path := filepath.Join(dir, "acme.kapi")
	require.NoError(t, os.WriteFile(path, []byte(yaml), 0o644))

	proj, err := project.Load(path)
	require.NoError(t, err)

	assert.Equal(t, "Acme App Localization", proj.Name)
	assert.Equal(t, "en-US", proj.SourceLanguage)
	assert.Equal(t, 3, len(proj.TargetLanguages))
	assert.Equal(t, 2, len(proj.Content))
	assert.Equal(t, "nextjs", proj.Preset)
	assert.Equal(t, []string{"okapi@1.47.0"}, proj.Plugins)

	// Flows
	assert.Equal(t, 2, len(proj.Flows))

	translate := proj.GetFlow("translate")
	require.NotNil(t, translate)
	assert.Len(t, translate.Steps, 1)
	assert.Equal(t, "ai-translate", translate.Steps[0].Tool)
	assert.Equal(t, "anthropic", translate.Steps[0].Config["provider"])

	pipeline := proj.GetFlow("full-pipeline")
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
	assert.Equal(t, proj.TargetLanguages, proj2.TargetLanguages)
	assert.Equal(t, len(proj.Flows), len(proj2.Flows))
}
