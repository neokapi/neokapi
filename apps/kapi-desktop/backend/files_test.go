package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// openTestProjectFile opens a .kapi project and registers cleanup to stop the file watcher.
func openTestProjectFile(t *testing.T, app *App, kapiPath string) *TabInfo {
	t.Helper()
	tab, err := app.OpenProject(kapiPath)
	require.NoError(t, err)
	t.Cleanup(func() { app.CloseProject(tab.ID) })
	return tab
}

func TestMatchContentBadTab(t *testing.T) {
	app := NewApp()
	matches, err := app.MatchContent("bad")
	require.NoError(t, err)
	assert.Nil(t, matches)
}

func TestMatchContentFindsFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "locales"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "locales", "en.json"), []byte(`{}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "locales", "fr.json"), []byte(`{}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.txt"), []byte("hi"), 0o644))

	kapiPath := filepath.Join(dir, "test.kapi")
	proj := &project.KapiProject{
		Version: "v1",
		Name:    "Test",
		Content: []project.ContentCollection{
			{Path: "locales/*.json", Format: &project.FormatSpec{Name: "json"}},
		},
	}
	require.NoError(t, project.Save(kapiPath, proj))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)

	matches, err := app.MatchContent(tab.ID)
	require.NoError(t, err)
	assert.Len(t, matches, 2)

	for _, m := range matches {
		assert.Equal(t, "json", m.Format)
		assert.NotEmpty(t, m.Relative)
		assert.Equal(t, "locales/*.json", m.Pattern)
	}
}

func TestMatchContentRejectsParentTraversal(t *testing.T) {
	dir := t.TempDir()
	kapiPath := filepath.Join(dir, "test.kapi")
	proj := &project.KapiProject{
		Version: "v1",
		Name:    "Test",
		Content: []project.ContentCollection{
			{Path: "../etc/passwd"},
			{Path: "foo/../../secret"},
		},
	}
	require.NoError(t, project.Save(kapiPath, proj))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)

	matches, err := app.MatchContent(tab.ID)
	require.NoError(t, err)
	assert.Empty(t, matches, "should reject patterns with ..")
}

func TestMatchContentRejectsAbsolutePaths(t *testing.T) {
	dir := t.TempDir()
	kapiPath := filepath.Join(dir, "test.kapi")
	proj := &project.KapiProject{
		Version: "v1",
		Name:    "Test",
		Content: []project.ContentCollection{
			{Path: "/etc/passwd"},
		},
	}
	require.NoError(t, project.Save(kapiPath, proj))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)

	matches, err := app.MatchContent(tab.ID)
	require.NoError(t, err)
	assert.Empty(t, matches, "should reject absolute paths")
}

func TestGetBasePathDefault(t *testing.T) {
	dir := t.TempDir()
	kapiPath := filepath.Join(dir, "test.kapi")
	require.NoError(t, project.Save(kapiPath, &project.KapiProject{Version: "v1", Name: "Test"}))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)
	assert.Equal(t, dir, app.GetBasePath(tab.ID))
}

func TestValidateContentPath(t *testing.T) {
	app := NewApp()
	assert.NoError(t, app.ValidateContentPath("locales/*.json"))
	assert.NoError(t, app.ValidateContentPath("src/i18n/en.json"))
	require.Error(t, app.ValidateContentPath("../secret"))

	// Absolute paths are rejected. Use /etc/passwd on Unix and C:\etc on Windows.
	if filepath.Separator == '/' {
		assert.Error(t, app.ValidateContentPath("/etc/passwd"))
	} else {
		assert.Error(t, app.ValidateContentPath(`C:\Windows\System32`))
	}
}

func TestIsEmptyProject(t *testing.T) {
	dir := t.TempDir()
	kapiPath := filepath.Join(dir, "project.kapi")
	require.NoError(t, project.Save(kapiPath, &project.KapiProject{Version: "v1"}))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)

	assert.True(t, app.IsEmptyProject(tab.ID), "project with only project.kapi should be empty")

	// Add a file — no longer empty.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "hello.json"), []byte(`{}`), 0o644))
	assert.False(t, app.IsEmptyProject(tab.ID))
}

func TestListProjectFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "input"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "input", "en.json"), []byte(`{}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0o644))

	kapiPath := filepath.Join(dir, "project.kapi")
	require.NoError(t, project.Save(kapiPath, &project.KapiProject{Version: "v1"}))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)

	files, err := app.ListProjectFiles(tab.ID)
	require.NoError(t, err)
	// Should find: input/ dir, input/en.json, readme.txt
	assert.Len(t, files, 3)

	// Should not include project.kapi or hidden files.
	for _, f := range files {
		assert.NotEqual(t, "project.kapi", f.Relative)
	}
}

func TestKapiIgnoreExcludesFiles(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "keep.json"), []byte(`{}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "scratch.tmp"), []byte("tmp"), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "build"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "build", "out.js"), []byte("x"), 0o644))

	// Write .kapiignore.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".kapiignore"), []byte("*.tmp\nbuild/\n"), 0o644))

	kapiPath := filepath.Join(dir, "project.kapi")
	require.NoError(t, project.Save(kapiPath, &project.KapiProject{Version: "v1"}))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)

	files, err := app.ListProjectFiles(tab.ID)
	require.NoError(t, err)

	// Only keep.json should remain (scratch.tmp and build/ excluded).
	assert.Len(t, files, 1)
	assert.Equal(t, "keep.json", files[0].Relative)
}

func TestMatchContentRespectsKapiIgnore(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "en.json"), []byte(`{}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "temp.json"), []byte(`{}`), 0o644))

	// Ignore temp.json.
	require.NoError(t, os.WriteFile(filepath.Join(dir, ".kapiignore"), []byte("temp.json\n"), 0o644))

	kapiPath := filepath.Join(dir, "project.kapi")
	proj := &project.KapiProject{
		Version: "v1",
		Content: []project.ContentCollection{{Path: "*.json"}},
	}
	require.NoError(t, project.Save(kapiPath, proj))

	app := NewApp()
	tab := openTestProjectFile(t, app, kapiPath)

	matches, err := app.MatchContent(tab.ID)
	require.NoError(t, err)

	// Only en.json should match (temp.json ignored).
	assert.Len(t, matches, 1)
	assert.Equal(t, "en.json", matches[0].Relative)
}

func TestApplyTemplateInputOutput(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	kapiPath := filepath.Join(dir, "project.kapi")
	require.NoError(t, project.Save(kapiPath, &project.KapiProject{Version: "v1"}))

	tab := openTestProjectFile(t, app, kapiPath)

	require.NoError(t, app.ApplyTemplate(tab.ID, "input-output"))

	// Directories should exist.
	_, err := os.Stat(filepath.Join(dir, "input"))
	require.NoError(t, err)
	_, err = os.Stat(filepath.Join(dir, "output"))
	require.NoError(t, err)

	// Content pattern should be set.
	proj := app.GetProject(tab.ID)
	require.Len(t, proj.Content, 1)
	assert.Equal(t, "input/*", proj.Content[0].Path)
	assert.Equal(t, "output/{lang}/*", proj.Content[0].Target)
}

func TestApplyTemplateEmpty(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	kapiPath := filepath.Join(dir, "project.kapi")
	require.NoError(t, project.Save(kapiPath, &project.KapiProject{Version: "v1"}))

	tab := openTestProjectFile(t, app, kapiPath)

	require.NoError(t, app.ApplyTemplate(tab.ID, "empty"))

	proj := app.GetProject(tab.ID)
	assert.Empty(t, proj.Content)
}

func TestCopyFileToProject(t *testing.T) {
	app := NewApp()
	dir := t.TempDir()
	kapiPath := filepath.Join(dir, "project.kapi")
	require.NoError(t, project.Save(kapiPath, &project.KapiProject{Version: "v1"}))

	tab := openTestProjectFile(t, app, kapiPath)

	// Create a source file outside the project.
	srcDir := t.TempDir()
	srcPath := filepath.Join(srcDir, "hello.json")
	require.NoError(t, os.WriteFile(srcPath, []byte(`{"greeting":"hello"}`), 0o644))

	rel, err := app.CopyFileToProject(tab.ID, srcPath, "input")
	require.NoError(t, err)
	assert.Equal(t, filepath.Join("input", "hello.json"), rel)

	// Verify file exists.
	data, err := os.ReadFile(filepath.Join(dir, "input", "hello.json"))
	require.NoError(t, err)
	assert.Contains(t, string(data), "greeting")
}

func TestDetectFormatByExtension(t *testing.T) {
	app := NewApp()
	tests := []struct {
		path   string
		expect string
	}{
		{"file.json", "json"},
		{"file.xliff", "xliff"},
		{"file.po", "po"},
		{"file.yaml", "yaml"},
		{"file.html", "html"},
		{"file.md", "markdown"},
		{"file.docx", "openxml"},
		{"file.xlsx", "openxml"},
		{"file.pptx", "openxml"},
		{"file.srt", "srt"},
		{"file.unknown", ""},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.expect, app.DetectFormat(tt.path), "for %s", tt.path)
	}
}
