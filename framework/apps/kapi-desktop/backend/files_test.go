package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

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

	// Save a .kapi file so the base path resolves to its directory.
	kapiPath := filepath.Join(dir, "test.kapi")
	proj := &project.KapiProject{
		Version: "v1",
		Name:    "Test",
		Content: []project.ContentEntry{
			{Path: "locales/*.json", Format: "json"},
		},
	}
	require.NoError(t, project.Save(kapiPath, proj))

	app := NewApp()
	tab, err := app.OpenProject(kapiPath)
	require.NoError(t, err)

	matches, err := app.MatchContent(tab.ID)
	require.NoError(t, err)
	assert.Len(t, matches, 2)

	for _, m := range matches {
		assert.Equal(t, "json", m.Format)
		assert.NotEmpty(t, m.Relative)
		assert.Equal(t, "locales/*.json", m.Pattern)
	}
}

func TestMatchContentWithBasePath(t *testing.T) {
	dir := t.TempDir()
	subdir := filepath.Join(dir, "src")
	require.NoError(t, os.MkdirAll(filepath.Join(subdir, "i18n"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(subdir, "i18n", "en.json"), []byte(`{}`), 0o644))

	kapiPath := filepath.Join(dir, "project.kapi")
	proj := &project.KapiProject{
		Version:  "v1",
		Name:     "Test",
		BasePath: "src",
		Content:  []project.ContentEntry{{Path: "i18n/*.json"}},
	}
	require.NoError(t, project.Save(kapiPath, proj))

	app := NewApp()
	tab, _ := app.OpenProject(kapiPath)

	base := app.GetBasePath(tab.ID)
	assert.Equal(t, subdir, base)

	matches, err := app.MatchContent(tab.ID)
	require.NoError(t, err)
	assert.Len(t, matches, 1)
}

func TestGetBasePathDefault(t *testing.T) {
	dir := t.TempDir()
	kapiPath := filepath.Join(dir, "test.kapi")
	require.NoError(t, project.Save(kapiPath, &project.KapiProject{Version: "v1", Name: "Test"}))

	app := NewApp()
	tab, _ := app.OpenProject(kapiPath)

	base := app.GetBasePath(tab.ID)
	assert.Equal(t, dir, base, "should default to .kapi file's directory")
}

func TestDetectFormatByExtension(t *testing.T) {
	tests := []struct {
		path   string
		expect string
	}{
		{"file.json", "json"},
		{"file.xliff", "xliff"},
		{"file.xlf", "xliff"},
		{"file.po", "po"},
		{"file.yaml", "yaml"},
		{"file.html", "html"},
		{"file.md", "markdown"},
		{"file.unknown", ""},
	}

	for _, tt := range tests {
		assert.Equal(t, tt.expect, detectFormatByExtension(tt.path), "for %s", tt.path)
	}
}
