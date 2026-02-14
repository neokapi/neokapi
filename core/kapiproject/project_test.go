package kapiproject

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gokapi/gokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindProject(t *testing.T) {
	t.Run("find in current directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		kapiDir := filepath.Join(tmpDir, ".kapi")
		require.NoError(t, os.MkdirAll(kapiDir, 0755))

		cfg := &Config{
			Project: ProjectMeta{
				Name:         "Test",
				SourceLocale: "en",
			},
		}
		require.NoError(t, SaveConfig(kapiDir, cfg))

		project, err := FindProject(tmpDir)
		require.NoError(t, err)
		require.NotNil(t, project)
		assert.Equal(t, tmpDir, project.Root)
		assert.Equal(t, kapiDir, project.KapiDir)
	})

	t.Run("find in parent directory", func(t *testing.T) {
		tmpDir := t.TempDir()
		kapiDir := filepath.Join(tmpDir, ".kapi")
		require.NoError(t, os.MkdirAll(kapiDir, 0755))

		cfg := &Config{
			Project: ProjectMeta{
				Name:         "Test",
				SourceLocale: "en",
			},
		}
		require.NoError(t, SaveConfig(kapiDir, cfg))

		// Search from subdirectory
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
		assert.Contains(t, err.Error(), ".kapi/")
	})

	t.Run("find from empty path uses current directory", func(t *testing.T) {
		// Save current directory
		origDir, err := os.Getwd()
		require.NoError(t, err)
		defer func() {
			_ = os.Chdir(origDir) // Best effort to restore
		}()

		tmpDir := t.TempDir()
		kapiDir := filepath.Join(tmpDir, ".kapi")
		require.NoError(t, os.MkdirAll(kapiDir, 0755))

		cfg := &Config{
			Project: ProjectMeta{
				Name:         "Test",
				SourceLocale: "en",
			},
		}
		require.NoError(t, SaveConfig(kapiDir, cfg))

		// Change to temp directory
		require.NoError(t, os.Chdir(tmpDir))

		project, err := FindProject("")
		require.NoError(t, err)
		require.NotNil(t, project)

		// Resolve symlinks for comparison (macOS has /var -> /private/var symlink)
		expectedRoot, _ := filepath.EvalSymlinks(tmpDir)
		actualRoot, _ := filepath.EvalSymlinks(project.Root)
		assert.Equal(t, expectedRoot, actualRoot)
	})

	t.Run("find and load project with server config", func(t *testing.T) {
		tmpDir := t.TempDir()
		kapiDir := filepath.Join(tmpDir, ".kapi")
		require.NoError(t, os.MkdirAll(kapiDir, 0755))

		cfg := &Config{
			Project: ProjectMeta{
				Name:         "Test Project",
				SourceLocale: "en-US",
			},
			Server: &ServerConfig{
				URL:       "https://test.example.com",
				ProjectID: "test123",
			},
		}
		require.NoError(t, SaveConfig(kapiDir, cfg))

		project, err := FindProject(tmpDir)
		require.NoError(t, err)
		require.NotNil(t, project)

		assert.Equal(t, tmpDir, project.Root)
		assert.Equal(t, kapiDir, project.KapiDir)
		assert.Equal(t, "Test Project", project.Config.Project.Name)
		assert.Equal(t, "https://test.example.com", project.Config.Server.URL)
	})

	t.Run("find project with config error", func(t *testing.T) {
		tmpDir := t.TempDir()
		kapiDir := filepath.Join(tmpDir, ".kapi")
		require.NoError(t, os.MkdirAll(kapiDir, 0755))

		// Write invalid config
		configPath := filepath.Join(kapiDir, "config.yaml")
		require.NoError(t, os.WriteFile(configPath, []byte("invalid: yaml: content:"), 0644))

		_, err := FindProject(tmpDir)
		require.Error(t, err)
	})
}

func TestInitProject(t *testing.T) {
	t.Run("initialize new project", func(t *testing.T) {
		tmpDir := t.TempDir()

		cfg := &Config{
			Project: ProjectMeta{
				Name:          "New Project",
				SourceLocale:  "en-US",
				TargetLocales: []model.LocaleID{"fr-FR", "de-DE"},
			},
		}

		project, err := InitProject(tmpDir, cfg)
		require.NoError(t, err)
		require.NotNil(t, project)

		// Verify directory structure
		assert.DirExists(t, filepath.Join(tmpDir, ".kapi"))
		assert.DirExists(t, filepath.Join(tmpDir, ".kapi", "flows"))
		assert.FileExists(t, filepath.Join(tmpDir, ".kapi", "config.yaml"))
		assert.FileExists(t, filepath.Join(tmpDir, ".kapi", ".gitignore"))

		// Verify config was saved correctly
		reloaded, err := LoadConfig(filepath.Join(tmpDir, ".kapi"))
		require.NoError(t, err)
		assert.Equal(t, "New Project", reloaded.Project.Name)
		assert.Equal(t, model.LocaleID("en-US"), reloaded.Project.SourceLocale)
		assert.Len(t, reloaded.Project.TargetLocales, 2)

		// Verify .gitignore content (inside .kapi directory)
		gitignoreContent, err := os.ReadFile(filepath.Join(tmpDir, ".kapi", ".gitignore"))
		require.NoError(t, err)
		assert.Contains(t, string(gitignoreContent), ".state.json")
	})

	t.Run("initialize fails if .kapi already exists", func(t *testing.T) {
		tmpDir := t.TempDir()
		kapiDir := filepath.Join(tmpDir, ".kapi")
		require.NoError(t, os.MkdirAll(kapiDir, 0755))

		cfg := &Config{
			Project: ProjectMeta{
				Name:         "Test",
				SourceLocale: "en",
			},
		}

		_, err := InitProject(tmpDir, cfg)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "already exists")
	})
}

func TestLoadState(t *testing.T) {
	tmpDir := t.TempDir()
	kapiDir := filepath.Join(tmpDir, ".kapi")
	require.NoError(t, os.MkdirAll(kapiDir, 0755))

	cfg := &Config{
		Project: ProjectMeta{
			Name:         "Test",
			SourceLocale: "en",
		},
	}
	require.NoError(t, SaveConfig(kapiDir, cfg))

	project, err := FindProject(tmpDir)
	require.NoError(t, err)

	ctx := context.Background()

	t.Run("load creates new state if not exists", func(t *testing.T) {
		state, err := project.LoadState(ctx)
		require.NoError(t, err)
		require.NotNil(t, state)
		assert.NotNil(t, state.Files)
		assert.NotNil(t, state.RemoteItems)
	})

	t.Run("load existing state", func(t *testing.T) {
		// Create state file
		modTime, _ := time.Parse(time.RFC3339, "2026-01-01T00:00:00Z")
		state := &State{
			Files: map[string]*FileState{
				"test.txt": {
					ContentHash: "abc123",
					Modified:    modTime,
				},
			},
		}
		require.NoError(t, project.SaveState(ctx, state))

		// Load it back
		loaded, err := project.LoadState(ctx)
		require.NoError(t, err)
		require.NotNil(t, loaded)
		assert.Len(t, loaded.Files, 1)
		assert.Equal(t, "abc123", loaded.Files["test.txt"].ContentHash)
	})
}

func TestResolvePath(t *testing.T) {
	tmpDir := t.TempDir()
	kapiDir := filepath.Join(tmpDir, ".kapi")
	require.NoError(t, os.MkdirAll(kapiDir, 0755))

	cfg := &Config{
		Project: ProjectMeta{
			Name:         "Test",
			SourceLocale: "en",
		},
	}
	require.NoError(t, SaveConfig(kapiDir, cfg))

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
	kapiDir := filepath.Join(tmpDir, ".kapi")
	require.NoError(t, os.MkdirAll(kapiDir, 0755))

	cfg := &Config{
		Project: ProjectMeta{
			Name:         "Test",
			SourceLocale: "en",
		},
	}
	require.NoError(t, SaveConfig(kapiDir, cfg))

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
		// Path outside project will have .. segments
		assert.Contains(t, relPath, "..")
	})
}

func TestFlowsDirPath(t *testing.T) {
	tmpDir := t.TempDir()
	kapiDir := filepath.Join(tmpDir, ".kapi")
	require.NoError(t, os.MkdirAll(kapiDir, 0755))

	cfg := &Config{
		Project: ProjectMeta{
			Name:         "Test",
			SourceLocale: "en",
		},
	}
	require.NoError(t, SaveConfig(kapiDir, cfg))

	project, err := FindProject(tmpDir)
	require.NoError(t, err)

	flowsDir := project.FlowsDirPath()
	expected := filepath.Join(kapiDir, "flows")
	assert.Equal(t, expected, flowsDir)
}
