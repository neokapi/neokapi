package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadConfig(t *testing.T) {
	t.Run("load valid config", func(t *testing.T) {
		tmpDir := t.TempDir()
		bowrainDir := filepath.Join(tmpDir, ".bowrain")
		require.NoError(t, os.MkdirAll(bowrainDir, 0755))

		configYAML := `version: v1

url: https://test.example.com/my-team/test123

defaults:
  source_language: en-US
  target_languages:
    - fr-FR
    - de-DE
  collection: ui/strings

content:
  - path: src/**/*.json
    format: json

hooks:
  pre-push:
    - qa-check
  post-pull:
    - segmentation
`
		configPath := filepath.Join(bowrainDir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte(configYAML), 0644))

		cfg, err := LoadConfig(bowrainDir)
		require.NoError(t, err)
		require.NotNil(t, cfg)

		assert.Equal(t, "v1", cfg.Version)
		assert.Equal(t, "en-US", string(cfg.Defaults.SourceLanguage))
		assert.Len(t, cfg.Defaults.TargetLanguages, 2)
		assert.Equal(t, "fr-FR", string(cfg.Defaults.TargetLanguages[0]))
		assert.Equal(t, "de-DE", string(cfg.Defaults.TargetLanguages[1]))
		assert.Equal(t, "ui/strings", cfg.Defaults.Collection)

		assert.Equal(t, "https://test.example.com", cfg.ServerURL())
		assert.Equal(t, "test123", cfg.ProjectID())
		assert.Equal(t, "my-team", cfg.Workspace())

		require.Len(t, cfg.Content, 1)
		assert.Equal(t, "src/**/*.json", cfg.Content[0].Path)
		assert.Equal(t, "json", cfg.Content[0].Format)

		require.NotNil(t, cfg.Hooks)
		assert.Len(t, cfg.Hooks["pre-push"], 1)
		assert.Equal(t, "qa-check", cfg.Hooks["pre-push"][0])
		assert.Len(t, cfg.Hooks["post-pull"], 1)
		assert.Equal(t, "segmentation", cfg.Hooks["post-pull"][0])
	})

	t.Run("config file not found", func(t *testing.T) {
		tmpDir := t.TempDir()
		bowrainDir := filepath.Join(tmpDir, ".bowrain")
		require.NoError(t, os.MkdirAll(bowrainDir, 0755))

		_, err := LoadConfig(bowrainDir)
		require.Error(t, err)
	})

	t.Run("invalid YAML", func(t *testing.T) {
		tmpDir := t.TempDir()
		bowrainDir := filepath.Join(tmpDir, ".bowrain")
		require.NoError(t, os.MkdirAll(bowrainDir, 0755))

		configPath := filepath.Join(bowrainDir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644))

		_, err := LoadConfig(bowrainDir)
		require.Error(t, err)
	})
}

func TestSaveConfig(t *testing.T) {
	t.Run("save and reload config", func(t *testing.T) {
		tmpDir := t.TempDir()
		bowrainDir := filepath.Join(tmpDir, ".bowrain")
		require.NoError(t, os.MkdirAll(bowrainDir, 0755))

		cfg := &Config{
			URL: FormatProjectURL("https://test.example.com", "my-team", "test123"),
			Defaults: Defaults{
				SourceLanguage:  "en-US",
				TargetLanguages: []model.LocaleID{"fr-FR", "de-DE"},
				Collection:      "ui/strings",
			},
			Content: []ContentEntry{
				{
					Path:   "src/**/*.json",
					Format: "json",
				},
			},
			Hooks: map[string][]string{
				"pre-push":  {"qa-check", "term-enforce"},
				"post-pull": {"segmentation"},
			},
		}

		err := SaveConfig(bowrainDir, cfg)
		require.NoError(t, err)

		// Verify file exists
		configPath := filepath.Join(bowrainDir, "config.yaml")
		_, err = os.Stat(configPath)
		require.NoError(t, err)

		// Reload and verify
		reloaded, err := LoadConfig(bowrainDir)
		require.NoError(t, err)

		assert.Equal(t, cfg.Defaults.SourceLanguage, reloaded.Defaults.SourceLanguage)
		assert.Equal(t, cfg.Defaults.TargetLanguages, reloaded.Defaults.TargetLanguages)
		assert.Equal(t, cfg.Defaults.Collection, reloaded.Defaults.Collection)
		assert.Equal(t, cfg.ServerURL(), reloaded.ServerURL())
		assert.Equal(t, cfg.ProjectID(), reloaded.ProjectID())
		assert.Equal(t, cfg.Content, reloaded.Content)
		assert.Equal(t, cfg.Hooks, reloaded.Hooks)
	})

	t.Run("save minimal config", func(t *testing.T) {
		tmpDir := t.TempDir()
		bowrainDir := filepath.Join(tmpDir, ".bowrain")
		require.NoError(t, os.MkdirAll(bowrainDir, 0755))

		cfg := &Config{
			Defaults: Defaults{
				SourceLanguage: "en",
			},
		}

		err := SaveConfig(bowrainDir, cfg)
		require.NoError(t, err)

		configPath := filepath.Join(bowrainDir, "config.yaml")
		_, err = os.Stat(configPath)
		require.NoError(t, err)
	})
}

func TestSaveConfig_WritesV1YAML(t *testing.T) {
	tmpDir := t.TempDir()
	bowrainDir := filepath.Join(tmpDir, ".bowrain")
	require.NoError(t, os.MkdirAll(bowrainDir, 0755))

	cfg := &Config{
		Defaults: Defaults{
			SourceLanguage:  "en-US",
			TargetLanguages: []model.LocaleID{"fr-FR"},
		},
	}

	err := SaveConfig(bowrainDir, cfg)
	require.NoError(t, err)

	configPath := filepath.Join(bowrainDir, "config.yaml")
	data, err := os.ReadFile(configPath)
	require.NoError(t, err)

	content := string(data)
	assert.Contains(t, content, "version: v1")
	assert.Contains(t, content, "defaults:")
	assert.Contains(t, content, "source_language:")

	reloaded, err := LoadConfig(bowrainDir)
	require.NoError(t, err)
	assert.Equal(t, "v1", reloaded.Version)
	assert.Equal(t, model.LocaleID("en-US"), reloaded.Defaults.SourceLanguage)
	assert.Len(t, reloaded.Defaults.TargetLanguages, 1)
}

func TestSaveConfig_RoundTrip(t *testing.T) {
	tmpDir := t.TempDir()
	bowrainDir := filepath.Join(tmpDir, ".bowrain")
	require.NoError(t, os.MkdirAll(bowrainDir, 0755))

	cfg := &Config{
		URL: FormatProjectURL("https://test.example.com", "ws", "proj-42"),
		Defaults: Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr", "de", "ja"},
		},
		Content: []ContentEntry{
			{Path: "src/**/*.json", Format: "json"},
			{Path: "docs/**/*.md", Format: "markdown"},
		},
		Hooks: map[string][]string{
			"pre-push": {"qa-check"},
		},
	}

	require.NoError(t, SaveConfig(bowrainDir, cfg))

	reloaded, err := LoadConfig(bowrainDir)
	require.NoError(t, err)

	assert.Equal(t, "v1", reloaded.Version)
	assert.Equal(t, cfg.Defaults.SourceLanguage, reloaded.Defaults.SourceLanguage)
	assert.Equal(t, cfg.Defaults.TargetLanguages, reloaded.Defaults.TargetLanguages)
	assert.Equal(t, cfg.ServerURL(), reloaded.ServerURL())
	assert.Equal(t, cfg.ProjectID(), reloaded.ProjectID())
	assert.Equal(t, cfg.Content, reloaded.Content)
	assert.Equal(t, cfg.Hooks, reloaded.Hooks)
}

func TestGetSetConfigValue(t *testing.T) {
	dir := t.TempDir()
	bowrainDir := filepath.Join(dir, BowrainDir)
	require.NoError(t, os.MkdirAll(bowrainDir, 0755))

	cfg := DefaultConfig()
	require.NoError(t, SaveConfig(bowrainDir, cfg))

	// Read existing value.
	assert.Equal(t, "en", GetConfigValue(bowrainDir, "defaults.source_language"))

	// Set a new value.
	require.NoError(t, SetConfigValue(bowrainDir, "defaults.source_language", "fr"))
	assert.Equal(t, "fr", GetConfigValue(bowrainDir, "defaults.source_language"))

	// Set a URL.
	require.NoError(t, SetConfigValue(bowrainDir, "url", "https://example.com/ws/proj"))
	assert.Equal(t, "https://example.com/ws/proj", GetConfigValue(bowrainDir, "url"))

	// Unset key returns empty.
	assert.Equal(t, "", GetConfigValue(bowrainDir, "nonexistent.key"))
}
