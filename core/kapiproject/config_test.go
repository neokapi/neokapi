package kapiproject

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
		kapiDir := filepath.Join(tmpDir, ".kapi")
		require.NoError(t, os.MkdirAll(kapiDir, 0755))

		configYAML := `project:
  name: Test Project
  source_locale: en-US
  target_locales:
    - fr-FR
    - de-DE

server:
  url: https://test.example.com
  project_id: test123

mappings:
  - local: src/**/*.json
    remote: app/{path}
    format: json

hooks:
  pre-push:
    - qa-check
  post-pull:
    - segmentation
`
		configPath := filepath.Join(kapiDir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte(configYAML), 0644))

		cfg, err := LoadConfig(kapiDir)
		require.NoError(t, err)
		require.NotNil(t, cfg)

		assert.Equal(t, "Test Project", cfg.Project.Name)
		assert.Equal(t, "en-US", string(cfg.Project.SourceLocale))
		assert.Len(t, cfg.Project.TargetLocales, 2)
		assert.Equal(t, "fr-FR", string(cfg.Project.TargetLocales[0]))
		assert.Equal(t, "de-DE", string(cfg.Project.TargetLocales[1]))

		require.NotNil(t, cfg.Server)
		assert.Equal(t, "https://test.example.com", cfg.Server.URL)
		assert.Equal(t, "test123", cfg.Server.ProjectID)

		require.Len(t, cfg.Mappings, 1)
		assert.Equal(t, "src/**/*.json", cfg.Mappings[0].Local)
		assert.Equal(t, "app/{path}", cfg.Mappings[0].Remote)
		assert.Equal(t, "json", cfg.Mappings[0].Format)

		require.NotNil(t, cfg.Hooks)
		assert.Len(t, cfg.Hooks["pre-push"], 1)
		assert.Equal(t, "qa-check", cfg.Hooks["pre-push"][0])
		assert.Len(t, cfg.Hooks["post-pull"], 1)
		assert.Equal(t, "segmentation", cfg.Hooks["post-pull"][0])
	})

	t.Run("config file not found", func(t *testing.T) {
		tmpDir := t.TempDir()
		kapiDir := filepath.Join(tmpDir, ".kapi")
		require.NoError(t, os.MkdirAll(kapiDir, 0755))

		_, err := LoadConfig(kapiDir)
		require.Error(t, err)
	})

	t.Run("invalid YAML", func(t *testing.T) {
		tmpDir := t.TempDir()
		kapiDir := filepath.Join(tmpDir, ".kapi")
		require.NoError(t, os.MkdirAll(kapiDir, 0755))

		configPath := filepath.Join(kapiDir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644))

		_, err := LoadConfig(kapiDir)
		require.Error(t, err)
	})
}

func TestSaveConfig(t *testing.T) {
	t.Run("save and reload config", func(t *testing.T) {
		tmpDir := t.TempDir()
		kapiDir := filepath.Join(tmpDir, ".kapi")
		require.NoError(t, os.MkdirAll(kapiDir, 0755))

		cfg := &Config{
			Project: ProjectMeta{
				Name:          "Test Project",
				SourceLocale:  "en-US",
				TargetLocales: []model.LocaleID{"fr-FR", "de-DE"},
			},
			Server: &ServerConfig{
				URL:       "https://test.example.com",
				ProjectID: "test123",
			},
			Mappings: []Mapping{
				{
					Local:  "src/**/*.json",
					Remote: "app/{path}",
					Format: "json",
				},
			},
			Hooks: map[string][]string{
				"pre-push":  {"qa-check", "term-enforce"},
				"post-pull": {"segmentation"},
			},
		}

		err := SaveConfig(kapiDir, cfg)
		require.NoError(t, err)

		// Verify file exists
		configPath := filepath.Join(kapiDir, "config.yaml")
		_, err = os.Stat(configPath)
		require.NoError(t, err)

		// Reload and verify
		reloaded, err := LoadConfig(kapiDir)
		require.NoError(t, err)

		assert.Equal(t, cfg.Project.Name, reloaded.Project.Name)
		assert.Equal(t, cfg.Project.SourceLocale, reloaded.Project.SourceLocale)
		assert.Equal(t, cfg.Project.TargetLocales, reloaded.Project.TargetLocales)
		assert.Equal(t, cfg.Server.URL, reloaded.Server.URL)
		assert.Equal(t, cfg.Server.ProjectID, reloaded.Server.ProjectID)
		assert.Equal(t, cfg.Mappings, reloaded.Mappings)
		assert.Equal(t, cfg.Hooks, reloaded.Hooks)
	})

	t.Run("save to existing directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		kapiDir := filepath.Join(tmpDir, ".kapi")
		require.NoError(t, os.MkdirAll(kapiDir, 0755))

		cfg := &Config{
			Project: ProjectMeta{
				Name:         "Test",
				SourceLocale: "en",
			},
		}

		err := SaveConfig(kapiDir, cfg)
		require.NoError(t, err)

		// Verify file exists
		configPath := filepath.Join(kapiDir, "config.yaml")
		_, err = os.Stat(configPath)
		require.NoError(t, err)
	})
}
