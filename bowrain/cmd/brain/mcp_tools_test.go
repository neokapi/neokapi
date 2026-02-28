package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/gokapi/gokapi/core/formats"
	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/registry"
	"github.com/gokapi/gokapi/platform/cli"
	"github.com/gokapi/gokapi/platform/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func brainTestApp() *cli.App {
	a := &cli.App{}
	a.FormatReg = registry.NewFormatRegistry()
	formats.RegisterAll(a.FormatReg)
	return a
}

func TestHandleProjectConfig(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &project.Config{
		Project: project.ProjectMeta{
			Name:          "test-project",
			SourceLocale:  "en",
			TargetLocales: []model.LocaleID{"fr", "de"},
		},
		Mappings: []project.Mapping{
			{Local: "locales/*.json", Format: "json"},
		},
	}

	_, err := project.InitProject(tmpDir, cfg)
	require.NoError(t, err)

	origDir := chdir(t, tmpDir)
	defer chdir(t, origDir)

	_, out, err := handleProjectConfig()
	require.NoError(t, err)
	assert.Equal(t, "test-project", out.ProjectName)
	assert.Equal(t, "en", out.SourceLocale)
	assert.Equal(t, []string{"fr", "de"}, out.TargetLocales)
	assert.Equal(t, 1, out.MappingCount)
	assert.Empty(t, out.ServerURL)
}

func TestHandleProjectConfigWithServer(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &project.Config{
		Project: project.ProjectMeta{
			Name:         "server-project",
			SourceLocale: "en",
		},
		Server: &project.ServerConfig{
			URL:       "https://bowrain.example.com",
			ProjectID: "proj-123",
		},
		Mappings: []project.Mapping{},
	}

	_, err := project.InitProject(tmpDir, cfg)
	require.NoError(t, err)

	origDir := chdir(t, tmpDir)
	defer chdir(t, origDir)

	_, out, err := handleProjectConfig()
	require.NoError(t, err)
	assert.Equal(t, "https://bowrain.example.com", out.ServerURL)
	assert.Equal(t, "proj-123", out.ProjectID)
}

func TestHandleProjectLsFast(t *testing.T) {
	a := brainTestApp()
	tmpDir := t.TempDir()

	writeTestFile(t, tmpDir, "locales/en.json", `{"hello": "world"}`)

	cfg := &project.Config{
		Project: project.ProjectMeta{
			Name:         "ls-test",
			SourceLocale: "en",
		},
		Mappings: []project.Mapping{
			{Local: "locales/*.json", Format: "json"},
		},
	}

	proj, err := project.InitProject(tmpDir, cfg)
	require.NoError(t, err)

	_, out, err := handleProjectLsFast(a, proj, MCPLsInput{})
	require.NoError(t, err)
	assert.Equal(t, 1, out.Total)
	assert.Equal(t, "locales/en.json", out.Files[0].Path)
	assert.Equal(t, "json", out.Files[0].Format)
}

func TestHandleProjectLsPathFilter(t *testing.T) {
	a := brainTestApp()
	tmpDir := t.TempDir()

	writeTestFile(t, tmpDir, "locales/en.json", `{"hello": "world"}`)
	writeTestFile(t, tmpDir, "other/data.json", `{"key": "value"}`)

	cfg := &project.Config{
		Project: project.ProjectMeta{
			Name:         "filter-test",
			SourceLocale: "en",
		},
		Mappings: []project.Mapping{
			{Local: "locales/*.json", Format: "json"},
			{Local: "other/*.json", Format: "json"},
		},
	}

	proj, err := project.InitProject(tmpDir, cfg)
	require.NoError(t, err)

	_, out, err := handleProjectLsFast(a, proj, MCPLsInput{
		Paths: []string{"locales/"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, out.Total)
	assert.Equal(t, "locales/en.json", out.Files[0].Path)
}

func TestHandleBrainListFlows(t *testing.T) {
	a := brainTestApp()

	_, out, err := handleBrainListFlows(a)
	require.NoError(t, err)
	assert.NotEmpty(t, out.Flows)
	assert.Equal(t, len(out.Flows), out.Total)

	var names []string
	for _, f := range out.Flows {
		names = append(names, f.Name)
	}
	assert.Contains(t, names, "pseudo-translate")
	assert.Contains(t, names, "qa-check")
	assert.Contains(t, names, "ai-translate")
}

func TestMatchesMCPPathFilter(t *testing.T) {
	tests := []struct {
		name   string
		path   string
		filter []string
		want   bool
	}{
		{"no filter", "locales/en.json", nil, true},
		{"match prefix", "locales/en.json", []string{"locales/"}, true},
		{"no match", "other/data.json", []string{"locales/"}, false},
		{"match exact", "locales/en.json", []string{"locales/en.json"}, true},
		{"trailing slash stripped", "locales/en.json", []string{"locales/"}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, matchesMCPPathFilter(tt.path, tt.filter))
		})
	}
}

// --- Helpers ---

func chdir(t *testing.T, dir string) string {
	t.Helper()
	orig, err := os.Getwd()
	require.NoError(t, err)
	require.NoError(t, os.Chdir(dir))
	return orig
}

func writeTestFile(t *testing.T, baseDir, relPath, content string) {
	t.Helper()
	absPath := filepath.Join(baseDir, relPath)
	require.NoError(t, os.MkdirAll(filepath.Dir(absPath), 0755))
	require.NoError(t, os.WriteFile(absPath, []byte(content), 0644))
}
