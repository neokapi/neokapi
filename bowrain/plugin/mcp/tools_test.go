package bowrainmcp

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/cli"
	"github.com/neokapi/neokapi/core/flow"
	"github.com/neokapi/neokapi/core/formats"
	"github.com/neokapi/neokapi/core/model"
	coreproj "github.com/neokapi/neokapi/core/project"
	"github.com/neokapi/neokapi/core/registry"
	"github.com/neokapi/neokapi/host/venue/project"
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
		Defaults: coreproj.Defaults{
			SourceLanguage:  "en",
			TargetLanguages: []model.LocaleID{"fr", "de"},
		},
		Collections: []coreproj.Collection{
			{Path: "locales/*.json", Format: jsonFormat()},
		},
	}

	_, err := project.InitProject(tmpDir, recipe)
	require.NoError(t, err)

	origDir := chdir(t, tmpDir)
	defer chdir(t, origDir)

	_, out, err := handleProjectConfig(bowrainTestApp(), MCPProjectInput{})
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
		Defaults: coreproj.Defaults{
			SourceLanguage: "en",
		},
		Server: &project.ServerSpec{
			URL: project.FormatProjectURL("https://bowrain.example.com", "", "proj-123"),
		},
	}

	_, err := project.InitProject(tmpDir, recipe)
	require.NoError(t, err)

	origDir := chdir(t, tmpDir)
	defer chdir(t, origDir)

	_, out, err := handleProjectConfig(bowrainTestApp(), MCPProjectInput{})
	require.NoError(t, err)
	assert.Equal(t, "https://bowrain.example.com", out.ServerURL)
	assert.Equal(t, "proj-123", out.ProjectID)
}

func TestHandleProjectLsFast(t *testing.T) {
	a := bowrainTestApp()
	tmpDir := t.TempDir()

	writeTestFile(t, tmpDir, "locales/en.json", `{"hello": "world"}`)

	recipe := &project.Recipe{
		Defaults: coreproj.Defaults{
			SourceLanguage: "en",
		},
		Collections: []coreproj.Collection{
			{Path: "locales/*.json", Format: jsonFormat()},
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

// TestHandleProjectLsFast_CompoundExtension pins the fix for the MCP ls path:
// a format-less .kbf.json item resolves to the KBF reader through the shared
// content-aware ProjectContext detector, not the generic JSON reader its
// .json tail would otherwise select.
func TestHandleProjectLsFast_CompoundExtension(t *testing.T) {
	a := bowrainTestApp()
	tmpDir := t.TempDir()

	writeTestFile(t, tmpDir, "i18n/en.kbf.json", `{"kind":"kapi-block-format","blocks":[]}`)

	recipe := &project.Recipe{
		Defaults: coreproj.Defaults{SourceLanguage: "en"},
		// No format: — detection must resolve it.
		Collections: []coreproj.Collection{{Path: "i18n/**/*.kbf.json"}},
	}

	proj, err := project.InitProject(tmpDir, recipe)
	require.NoError(t, err)

	_, out, err := handleProjectLsFast(a, proj, MCPLsInput{})
	require.NoError(t, err)
	require.Equal(t, 1, out.Total)
	assert.Equal(t, "kbf", out.Files[0].Format,
		"a compound suffix must out-rank its tail: .kbf.json is the KBF reader")
}

func TestHandleProjectLsPathFilter(t *testing.T) {
	a := bowrainTestApp()
	tmpDir := t.TempDir()

	writeTestFile(t, tmpDir, "locales/en.json", `{"hello": "world"}`)
	writeTestFile(t, tmpDir, "other/data.json", `{"key": "value"}`)

	recipe := &project.Recipe{
		Defaults: coreproj.Defaults{
			SourceLanguage: "en",
		},
		Collections: []coreproj.Collection{
			{Path: "locales/*.json", Format: jsonFormat()},
			{Path: "other/*.json", Format: jsonFormat()},
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

// Outside a project, list_flows lists the composed built-in flows `kapi flows`
// lists.
func TestHandleBowrainListFlows(t *testing.T) {
	t.Chdir(t.TempDir())

	_, out, err := handleBowrainListFlows(bowrainTestApp(), MCPProjectInput{})
	require.NoError(t, err)
	assert.NotEmpty(t, out.Flows)
	assert.Equal(t, len(out.Flows), out.Total)
	assert.Empty(t, out.Warning)

	sources := map[string]string{}
	for _, f := range out.Flows {
		sources[f.Name] = f.Source
	}
	assert.Equal(t, "builtin", sources["translate"])
	assert.Equal(t, "builtin", sources["translate-qa"])
}

// In a project, list_flows lists what `kapi flows` lists: the recipe's inline
// flows and its flows_dir files, each name once, for the flow `kapi run`
// resolves it to. A file that will not run is listed with its problem.
func TestHandleBowrainListFlows_ListsInlineAndFileFlows(t *testing.T) {
	root := t.TempDir()
	recipe := &project.Recipe{
		Version:  coreproj.CurrentVersion,
		Name:     "FlowsTest",
		FlowsDir: "flows",
		Defaults: coreproj.Defaults{SourceLanguage: "en"},
		Flows: map[string]*flow.StepsSpec{
			"inline-check": {Steps: []flow.FlowStep{{Tool: "qa"}}},
			"guard":        {Steps: []flow.FlowStep{{Tool: "qa"}, {Tool: "qa"}}},
			// A built-in's name: `kapi run translate` runs the recipe's flow,
			// so the listing shows it in place of the built-in.
			"translate": {Steps: []flow.FlowStep{{Tool: "qa"}}},
		},
	}
	proj, err := project.InitProject(root, recipe)
	require.NoError(t, err)
	flowsDir := proj.FlowsDirPath()
	require.NoError(t, os.MkdirAll(flowsDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(flowsDir, "file-check.yaml"),
		[]byte("description: Check from a file\nsteps:\n  - tool: qa\n"), 0o644))
	// The inline guard wins over a file of its name.
	require.NoError(t, os.WriteFile(filepath.Join(flowsDir, "guard.yaml"),
		[]byte("steps:\n  - tool: qa\n"), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(flowsDir, "broken.yaml"),
		[]byte("steps: []\n"), 0o644))
	t.Chdir(root)

	_, out, err := handleBowrainListFlows(bowrainTestApp(), MCPProjectInput{})
	require.NoError(t, err)
	assert.Empty(t, out.Warning)

	byName := map[string][]MCPFlowEntry{}
	for _, f := range out.Flows {
		byName[f.Name] = append(byName[f.Name], f)
	}
	for name, entries := range byName {
		assert.Len(t, entries, 1, "%s is listed once", name)
	}
	require.Contains(t, byName, "inline-check")
	assert.Equal(t, "project", byName["inline-check"][0].Source)
	require.Contains(t, byName, "file-check")
	assert.Equal(t, "project", byName["file-check"][0].Source)
	assert.Equal(t, "Check from a file", byName["file-check"][0].Description)
	require.Contains(t, byName, "guard")
	assert.Equal(t, 2, byName["guard"][0].Steps, "the inline guard, not the file")
	require.Contains(t, byName, "broken")
	assert.Contains(t, byName["broken"][0].Description, "declares no steps")
	require.Contains(t, byName, "translate")
	assert.Equal(t, "project", byName["translate"][0].Source)
	require.Contains(t, byName, "translate-qa")
	assert.Equal(t, "builtin", byName["translate-qa"][0].Source)
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

// A file two items match is listed once, with the format of the first item
// that matches it (#2288).
func TestHandleProjectLsFast_FirstMatchingItemClaims(t *testing.T) {
	a := bowrainTestApp()
	tmpDir := t.TempDir()

	writeTestFile(t, tmpDir, "locales/en.json", `{"hello": "world"}`)
	writeTestFile(t, tmpDir, "locales/menu.json", `{"file": "File"}`)

	recipe := &project.Recipe{
		Defaults: coreproj.Defaults{SourceLanguage: "en"},
		Collections: []coreproj.Collection{
			{Path: "locales/en.json", Format: &coreproj.FormatSpec{Name: "kbf"}},
			{Path: "locales/*.json", Format: jsonFormat()},
		},
	}

	proj, err := project.InitProject(tmpDir, recipe)
	require.NoError(t, err)

	_, out, err := handleProjectLsFast(a, proj, MCPLsInput{})
	require.NoError(t, err)
	require.Equal(t, 2, out.Total)
	byPath := map[string]string{}
	for _, f := range out.Files {
		byPath[f.Path] = f.Format
	}
	assert.Equal(t, map[string]string{
		"locales/en.json":   "kbf",
		"locales/menu.json": "json",
	}, byPath)
}
