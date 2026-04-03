package project

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindProject(t *testing.T) {
	t.Run("find in current directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		bowrainDir := filepath.Join(tmpDir, ".bowrain")
		require.NoError(t, os.MkdirAll(bowrainDir, 0755))

		cfg := &Config{
			Defaults: Defaults{
				SourceLanguage: "en",
			},
		}
		require.NoError(t, SaveConfig(bowrainDir, cfg))

		project, err := FindProject(tmpDir)
		require.NoError(t, err)
		require.NotNil(t, project)
		assert.Equal(t, tmpDir, project.Root)
		assert.Equal(t, bowrainDir, project.ConfigDir)
	})

	t.Run("find in parent directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		bowrainDir := filepath.Join(tmpDir, ".bowrain")
		require.NoError(t, os.MkdirAll(bowrainDir, 0755))

		cfg := &Config{
			Defaults: Defaults{
				SourceLanguage: "en",
			},
		}
		require.NoError(t, SaveConfig(bowrainDir, cfg))

		subDir := filepath.Join(tmpDir, "src", "locales")
		require.NoError(t, os.MkdirAll(subDir, 0755))

		project, err := FindProject(subDir)
		require.NoError(t, err)
		require.NotNil(t, project)
		assert.Equal(t, tmpDir, project.Root)
	})

	t.Run("project not found", func(t *testing.T) {
		tmpDir := t.TempDir()

		_, err := FindProject(tmpDir)
		require.Error(t, err)
		assert.Contains(t, err.Error(), ".bowrain/")
	})

	t.Run("find from empty path uses current directory", func(t *testing.T) {
		origDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() {
			_ = os.Chdir(origDir)
		}()

		tmpDir := t.TempDir()
		bowrainDir := filepath.Join(tmpDir, ".bowrain")
		require.NoError(t, os.MkdirAll(bowrainDir, 0755))

		cfg := &Config{
			Defaults: Defaults{
				SourceLanguage: "en",
			},
		}
		require.NoError(t, SaveConfig(bowrainDir, cfg))

		require.NoError(t, os.Chdir(tmpDir))

		project, err := FindProject("")
		require.NoError(t, err)
		require.NotNil(t, project)

		expectedRoot, _ := filepath.EvalSymlinks(tmpDir)
		actualRoot, _ := filepath.EvalSymlinks(project.Root)
		assert.Equal(t, expectedRoot, actualRoot)
	})

	t.Run("find and load project with server URL", func(t *testing.T) {
		tmpDir := t.TempDir()
		bowrainDir := filepath.Join(tmpDir, ".bowrain")
		require.NoError(t, os.MkdirAll(bowrainDir, 0755))

		cfg := &Config{
			URL: FormatProjectURL("https://test.example.com", "ws", "test123"),
			Defaults: Defaults{
				SourceLanguage: "en-US",
			},
		}
		require.NoError(t, SaveConfig(bowrainDir, cfg))

		project, err := FindProject(tmpDir)
		require.NoError(t, err)
		require.NotNil(t, project)

		assert.Equal(t, tmpDir, project.Root)
		assert.Equal(t, bowrainDir, project.ConfigDir)
		assert.Equal(t, "https://test.example.com", project.Config.ServerURL())
	})

	t.Run("find project with config error", func(t *testing.T) {
		tmpDir := t.TempDir()
		bowrainDir := filepath.Join(tmpDir, ".bowrain")
		require.NoError(t, os.MkdirAll(bowrainDir, 0755))

		configPath := filepath.Join(bowrainDir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644))

		_, err := FindProject(tmpDir)
		require.Error(t, err)
	})
}

func TestInitProject(t *testing.T) {
	t.Run("initialize new project", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &Config{
			Defaults: Defaults{
				SourceLanguage:  "en-US",
				TargetLanguages: []model.LocaleID{"fr-FR", "de-DE"},
			},
		}

		project, err := InitProject(tmpDir, cfg)
		require.NoError(t, err)
		require.NotNil(t, project)

		assert.DirExists(t, filepath.Join(tmpDir, ".bowrain"))
		assert.DirExists(t, filepath.Join(tmpDir, ".bowrain", "flows"))
		assert.FileExists(t, filepath.Join(tmpDir, ".bowrain", "config.yaml"))
		assert.FileExists(t, filepath.Join(tmpDir, ".bowrain", ".gitignore"))

		reloaded, err := LoadConfig(filepath.Join(tmpDir, ".bowrain"))
		require.NoError(t, err)
		assert.Equal(t, model.LocaleID("en-US"), reloaded.Defaults.SourceLanguage)
		assert.Len(t, reloaded.Defaults.TargetLanguages, 2)

		gitignoreContent, err := os.ReadFile(filepath.Join(tmpDir, ".bowrain", ".gitignore"))
		require.NoError(t, err)
		assert.Contains(t, string(gitignoreContent), ".sync-cache")
	})

	t.Run("initialize fails if .bowrain already exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		bowrainDir := filepath.Join(tmpDir, ".bowrain")
		require.NoError(t, os.MkdirAll(bowrainDir, 0755))

		cfg := &Config{
			Defaults: Defaults{
				SourceLanguage: "en",
			},
		}

		_, err := InitProject(tmpDir, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})
}

func TestResolvePath(t *testing.T) {
	tmpDir := t.TempDir()
	bowrainDir := filepath.Join(tmpDir, ".bowrain")
	require.NoError(t, os.MkdirAll(bowrainDir, 0755))

	cfg := &Config{
		Defaults: Defaults{SourceLanguage: "en"},
	}
	require.NoError(t, SaveConfig(bowrainDir, cfg))

	project, err := FindProject(tmpDir)
	require.NoError(t, err)

	t.Run("resolve relative path", func(t *testing.T) {
		resolved := project.ResolvePath("src/file.txt")
		expected := filepath.Join(tmpDir, "src/file.txt")
		assert.Equal(t, expected, resolved)
	})

	t.Run("resolve absolute path", func(t *testing.T) {
		absPath := filepath.Join(tmpDir, "file.txt")
		resolved := project.ResolvePath(absPath)
		assert.Equal(t, absPath, resolved)
	})
}

func TestRelativePath(t *testing.T) {
	tmpDir := t.TempDir()
	bowrainDir := filepath.Join(tmpDir, ".bowrain")
	require.NoError(t, os.MkdirAll(bowrainDir, 0755))

	cfg := &Config{
		Defaults: Defaults{SourceLanguage: "en"},
	}
	require.NoError(t, SaveConfig(bowrainDir, cfg))

	project, err := FindProject(tmpDir)
	require.NoError(t, err)

	t.Run("relativize absolute path", func(t *testing.T) {
		absPath := filepath.Join(tmpDir, "src", "file.txt")
		relPath, err := project.RelativePath(absPath)
		require.NoError(t, err)
		assert.Equal(t, filepath.Join("src", "file.txt"), relPath)
	})

	t.Run("relativize path outside project", func(t *testing.T) {
		outsidePath := filepath.Join(filepath.Dir(tmpDir), "outside.txt")
		relPath, err := project.RelativePath(outsidePath)
		require.NoError(t, err)
		assert.Contains(t, relPath, "..")
	})
}

func TestFlowsDirPath(t *testing.T) {
	tmpDir := t.TempDir()
	bowrainDir := filepath.Join(tmpDir, ".bowrain")
	require.NoError(t, os.MkdirAll(bowrainDir, 0755))

	cfg := &Config{
		Defaults: Defaults{SourceLanguage: "en"},
	}
	require.NoError(t, SaveConfig(bowrainDir, cfg))

	project, err := FindProject(tmpDir)
	require.NoError(t, err)

	flowsDir := project.FlowsDirPath()
	expected := filepath.Join(bowrainDir, "flows")
	assert.Equal(t, expected, flowsDir)
}
