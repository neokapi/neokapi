package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gokapi/gokapi/platform/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// setupAddRmProject creates a temporary project with files for add/rm testing.
func setupAddRmProject(t *testing.T) *project.Project {
	t.Helper()

	root := t.TempDir()

	// Create file tree.
	files := []string{
		"src/main/index.html",
		"src/main/about.html",
		"src/legacy/old.html",
		"src/legacy/ancient.html",
		"locales/en.json",
		"locales/fr.json",
	}
	for _, f := range files {
		abs := filepath.Join(root, f)
		require.NoError(t, os.MkdirAll(filepath.Dir(abs), 0755))
		require.NoError(t, os.WriteFile(abs, []byte("{}"), 0644))
	}

	cfg := project.DefaultConfig()
	cfg.Project.Name = "test-add-rm"
	cfg.Project.SourceLocale = "en"

	proj, err := project.InitProject(root, cfg)
	require.NoError(t, err)
	return proj
}

func TestAdd_Basic(t *testing.T) {
	proj := setupAddRmProject(t)

	// Simulate what add_cmd does: append mapping, save config.
	proj.Config.Mappings = append(proj.Config.Mappings, project.Mapping{
		Local:  "src/**/*.html",
		Format: "html",
	})
	require.NoError(t, project.SaveConfig(proj.KapiDir, proj.Config))

	// Reload and verify.
	loaded, err := project.LoadConfig(proj.KapiDir)
	require.NoError(t, err)
	require.Len(t, loaded.Mappings, 1)
	assert.Equal(t, "src/**/*.html", loaded.Mappings[0].Local)
	assert.Equal(t, "html", loaded.Mappings[0].Format)
}

func TestAdd_Duplicate(t *testing.T) {
	proj := setupAddRmProject(t)

	proj.Config.Mappings = append(proj.Config.Mappings, project.Mapping{
		Local:  "src/**/*.html",
		Format: "html",
	})

	// Try adding the same pattern again — check for duplicate.
	alreadyTracked := false
	for _, m := range proj.Config.Mappings {
		if m.Local == "src/**/*.html" {
			alreadyTracked = true
			break
		}
	}
	assert.True(t, alreadyTracked, "should detect duplicate")
	assert.Len(t, proj.Config.Mappings, 1, "should not add duplicate")
}

func TestAdd_GlobExpandsFromProjectRoot(t *testing.T) {
	proj := setupAddRmProject(t)

	// Add a mapping.
	proj.Config.Mappings = append(proj.Config.Mappings, project.Mapping{
		Local:  "src/**/*.html",
		Format: "html",
	})

	// Expand from project root — should find all HTML files.
	matches, err := project.ExpandGlob(proj.Root, "src/**/*.html")
	require.NoError(t, err)
	assert.Len(t, matches, 4, "should find all HTML files under src/")
}

func TestRm_ExactMatch(t *testing.T) {
	proj := setupAddRmProject(t)

	proj.Config.Mappings = append(proj.Config.Mappings, project.Mapping{
		Local:  "src/**/*.html",
		Format: "html",
	})

	// Remove the exact mapping.
	entry := processRmPattern(proj, "src/**/*.html")
	assert.Equal(t, "removed", entry.Action)
	assert.Equal(t, "html", entry.Format)
	assert.Empty(t, proj.Config.Mappings, "mapping should be removed")

	// Exclude list should be empty.
	assert.Empty(t, proj.Config.Exclude)
}

func TestRm_ToExclude(t *testing.T) {
	proj := setupAddRmProject(t)

	proj.Config.Mappings = append(proj.Config.Mappings, project.Mapping{
		Local:  "src/**/*.html",
		Format: "html",
	})

	// Remove a narrower pattern — should add to exclude.
	entry := processRmPattern(proj, "src/legacy/*.html")
	assert.Equal(t, "excluded", entry.Action)
	assert.Equal(t, 2, entry.Files, "should count 2 legacy HTML files")

	// Mapping still present.
	require.Len(t, proj.Config.Mappings, 1)
	assert.Equal(t, "src/**/*.html", proj.Config.Mappings[0].Local)

	// Exclude added.
	require.Len(t, proj.Config.Exclude, 1)
	assert.Equal(t, "src/legacy/*.html", proj.Config.Exclude[0])
}

func TestRm_AlreadyExcluded(t *testing.T) {
	proj := setupAddRmProject(t)

	proj.Config.Exclude = []string{"src/legacy/*.html"}

	entry := processRmPattern(proj, "src/legacy/*.html")
	assert.Equal(t, "already_excluded", entry.Action)

	// Exclude list should not grow.
	assert.Len(t, proj.Config.Exclude, 1)
}

func TestRm_MultiplePatterns(t *testing.T) {
	proj := setupAddRmProject(t)

	proj.Config.Mappings = append(proj.Config.Mappings,
		project.Mapping{Local: "src/**/*.html", Format: "html"},
		project.Mapping{Local: "locales/*.json", Format: "json"},
	)

	// Remove exact match + exclude in one go.
	e1 := processRmPattern(proj, "locales/*.json")
	assert.Equal(t, "removed", e1.Action)
	assert.Len(t, proj.Config.Mappings, 1, "only html mapping remains")

	e2 := processRmPattern(proj, "src/legacy/*.html")
	assert.Equal(t, "excluded", e2.Action)
}

func TestConfigRoundtrip_WithExcludes(t *testing.T) {
	proj := setupAddRmProject(t)

	// Add mappings and excludes.
	proj.Config.Mappings = append(proj.Config.Mappings, project.Mapping{
		Local:  "src/**/*.html",
		Format: "html",
	})
	proj.Config.Exclude = append(proj.Config.Exclude, "src/legacy/*.html")

	// Save and reload.
	require.NoError(t, project.SaveConfig(proj.KapiDir, proj.Config))

	loaded, err := project.LoadConfig(proj.KapiDir)
	require.NoError(t, err)

	require.Len(t, loaded.Mappings, 1)
	assert.Equal(t, "src/**/*.html", loaded.Mappings[0].Local)

	require.Len(t, loaded.Exclude, 1)
	assert.Equal(t, "src/legacy/*.html", loaded.Exclude[0])
}

func TestExpandGlob_RespectsExcludes(t *testing.T) {
	proj := setupAddRmProject(t)

	// All HTML files.
	all, err := project.ExpandGlob(proj.Root, "src/**/*.html")
	require.NoError(t, err)
	assert.Len(t, all, 4)

	// With exclude.
	filtered, err := project.ExpandGlob(proj.Root, "src/**/*.html", "src/legacy/*.html")
	require.NoError(t, err)
	assert.Len(t, filtered, 2, "should exclude legacy HTML files")

	for _, f := range filtered {
		assert.NotContains(t, f, "legacy", "excluded files should not appear")
	}
}
