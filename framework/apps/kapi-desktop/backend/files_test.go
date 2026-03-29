package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/neokapi/neokapi/core/project"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMatchContentNoProject(t *testing.T) {
	app := NewApp()
	matches, err := app.MatchContent("/tmp")
	require.NoError(t, err)
	assert.Nil(t, matches)
}

func TestMatchContentFindsFiles(t *testing.T) {
	dir := t.TempDir()

	// Create test files.
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "locales"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "locales", "en.json"), []byte(`{}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "locales", "fr.json"), []byte(`{}`), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "other.txt"), []byte("hi"), 0o644))

	app := NewApp()
	_, _ = app.NewProject("Test", "en", nil)
	app.project.Content = []project.ContentEntry{
		{Path: "locales/*.json", Format: "json"},
	}

	matches, err := app.MatchContent(dir)
	require.NoError(t, err)
	assert.Len(t, matches, 2)

	for _, m := range matches {
		assert.Equal(t, "json", m.Format)
		assert.NotEmpty(t, m.Relative)
	}
}

func TestMatchContentAutoDetectFormat(t *testing.T) {
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "test.xliff"), []byte(`<?xml?>`), 0o644))

	app := NewApp()
	_, _ = app.NewProject("Test", "en", nil)
	app.project.Content = []project.ContentEntry{
		{Path: "*.xliff"}, // no format specified
	}

	matches, err := app.MatchContent(dir)
	require.NoError(t, err)
	require.Len(t, matches, 1)
	assert.Equal(t, "xliff", matches[0].Format)
}

func TestDetectFormat(t *testing.T) {
	app := NewApp()

	tests := []struct {
		path   string
		expect string
	}{
		{"file.json", "json"},
		{"file.xliff", ""},  // may or may not be registered
		{"file.unknown", ""}, // unknown extension
	}

	for _, tt := range tests {
		result := app.DetectFormat(tt.path)
		if tt.expect != "" {
			assert.Equal(t, tt.expect, result, "for %s", tt.path)
		}
	}
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
		{"file.properties", "java-properties"},
		{"file.yaml", "yaml"},
		{"file.yml", "yaml"},
		{"file.xml", "xml"},
		{"file.html", "html"},
		{"file.md", "markdown"},
		{"file.csv", "csv"},
		{"file.txt", "plaintext"},
		{"file.unknown", ""},
		{"noextension", ""},
	}

	for _, tt := range tests {
		result := detectFormatByExtension(tt.path)
		assert.Equal(t, tt.expect, result, "for %s", tt.path)
	}
}
