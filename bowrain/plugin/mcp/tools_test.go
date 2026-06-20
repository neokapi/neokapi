package bowrainmcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/project"
	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func bowrainTestApp() *cli.App {
	a := &cli.App{}
	a.FormatReg = registry.NewFormatRegistry()
	formats.RegisterAll(a.FormatReg)
	return a
}

// jsonFormat is a small helper that builds a FormatSpec for the JSON format.
func jsonFormat() *coreproj.FormatSpec {
	return &coreproj.FormatSpec{Name: "json"}
}

func TestHandleProjectConfig(t *testing.T) {
	tmpDir := t.TempDir()

	recipe := &project.Recipe{
		KapiProject: coreproj.KapiProject{
			Defaults: coreproj.Defaults{
				SourceLanguage:  "en",
				TargetLanguages: []model.LocaleID{"fr", "de"},
			},
			Content: []coreproj.ContentCollection{
				{Path: "locales/*.json", Format: jsonFormat()},
			},
		},
	}

	_, err := project.InitProject(tmpDir, recipe)
	require.NoError(t, err)

	origDir := chdir(t, tmpDir)
	defer chdir(t, origDir)

	_, out, err := handleProjectConfig()
	require.NoError(t, err)
	assert.Equal(t, MCPLocaleInfo{Code: "en", DisplayName: "English"}, out.SourceLanguage)
	assert.Equal(t, []MCPLocaleInfo{
		{Code: "fr", DisplayName: "French"},
		{Code: "de", DisplayName: "German"},
	}, out.TargetLanguages)
	assert.Equal(t, 1, out.ContentCount)
	assert.Empty(t, out.ServerURL)
}

func TestHandleProjectConfigWithServer(t *testing.T) {
	tmpDir := t.TempDir()

	recipe := &project.Recipe{
		KapiProject: coreproj.KapiProject{
			Defaults: coreproj.Defaults{
				SourceLanguage: "en",
			},
		},
		Server: &project.ServerSpec{
			URL: project.FormatProjectURL("https://bowrain.example.com", "", "proj-123"),
		},
	}

	_, err := project.InitProject(tmpDir, recipe)
	require.NoError(t, err)

	origDir := chdir(t, tmpDir)
	defer chdir(t, origDir)

	_, out, err := handleProjectConfig()
	require.NoError(t, err)
	assert.Equal(t, "https://bowrain.example.com", out.ServerURL)
	assert.Equal(t, "proj-123", out.ProjectID)
}

func TestHandleProjectLsFast(t *testing.T) {
	a := bowrainTestApp()
	tmpDir := t.TempDir()

	writeTestFile(t, tmpDir, "locales/en.json", `{"hello": "world"}`)

	recipe := &project.Recipe{
		KapiProject: coreproj.KapiProject{
			Defaults: coreproj.Defaults{
				SourceLanguage: "en",
			},
			Content: []coreproj.ContentCollection{
				{Path: "locales/*.json", Format: jsonFormat()},
			},
		},
	}

	proj, err := project.InitProject(tmpDir, recipe)
	require.NoError(t, err)

	_, out, err := handleProjectLsFast(a, proj, MCPLsInput{})
	require.NoError(t, err)
	assert.Equal(t, 1, out.Total)
	assert.Equal(t, "locales/en.json", out.Files[0].Path)
	assert.Equal(t, "json", out.Files[0].Format)
}

func TestHandleProjectLsPathFilter(t *testing.T) {
	a := bowrainTestApp()
	tmpDir := t.TempDir()

	writeTestFile(t, tmpDir, "locales/en.json", `{"hello": "world"}`)
	writeTestFile(t, tmpDir, "other/data.json", `{"key": "value"}`)

	recipe := &project.Recipe{
		KapiProject: coreproj.KapiProject{
			Defaults: coreproj.Defaults{
				SourceLanguage: "en",
			},
			Content: []coreproj.ContentCollection{
				{Path: "locales/*.json", Format: jsonFormat()},
				{Path: "other/*.json", Format: jsonFormat()},
			},
		},
	}

	proj, err := project.InitProject(tmpDir, recipe)
	require.NoError(t, err)

	_, out, err := handleProjectLsFast(a, proj, MCPLsInput{
		Paths: []string{"locales/"},
	})
	require.NoError(t, err)
	assert.Equal(t, 1, out.Total)
	assert.Equal(t, "locales/en.json", out.Files[0].Path)
}

func TestHandleBowrainListFlows(t *testing.T) {
	a := bowrainTestApp()

	_, out, err := handleBowrainListFlows(a)
	require.NoError(t, err)
	assert.NotEmpty(t, out.Flows)
	assert.Equal(t, len(out.Flows), out.Total)

	var names []string
	for _, f := range out.Flows {
		names = append(names, f.Name)
	}
	assert.Contains(t, names, "pseudo-translate")
	assert.Contains(t, names, "qa")
	assert.Contains(t, names, "translate")
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
