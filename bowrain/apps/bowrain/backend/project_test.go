package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateProject(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Test Project", "en", []string{"fr", "de"})
	require.NoError(t, err)
	assert.NotEmpty(t, info.ID)
	assert.Equal(t, "Test Project", info.Name)
	assert.Equal(t, "en", info.DefaultSourceLanguage)
	assert.Equal(t, []string{"fr", "de"}, info.TargetLanguages)
	assert.NotEmpty(t, info.CreatedAt)
	assert.Empty(t, info.Items)
}

func TestCreateProject_Validation(t *testing.T) {
	app := newTestApp(t)

	tests := []struct {
		name     string
		projName string
		srcLang  string
		tgtLangs []string
		wantErr  string
	}{
		{"empty name", "", "en", []string{"fr"}, "project name is required"},
		{"empty source", "Test", "", []string{"fr"}, "source language is required"},
		{"no targets", "Test", "en", nil, "at least one target language"},
		{"empty targets", "Test", "en", []string{}, "at least one target language"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := app.CreateProject(tt.projName, tt.srcLang, tt.tgtLangs)
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

func TestListProjects(t *testing.T) {
	app := newTestApp(t)

	// Initially empty
	projects := app.ListProjects()
	assert.Empty(t, projects)

	// Create two projects
	_, err := app.CreateProject("Project A", "en", []string{"fr"})
	require.NoError(t, err)
	_, err = app.CreateProject("Project B", "en", []string{"de"})
	require.NoError(t, err)

	projects = app.ListProjects()
	assert.Len(t, projects, 2)
}

func TestCloseProject(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	err = app.CloseProject(info.ID)
	require.NoError(t, err)

	// Should not be findable
	_, err = app.GetProject(info.ID)
	assert.Error(t, err)
}

func TestCloseProject_NotFound(t *testing.T) {
	app := newTestApp(t)

	err := app.CloseProject("nonexistent")
	assert.Error(t, err)
}

func TestAddFiles(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	testFile := filepath.Join("testdata", "hello.txt")
	info, err = app.AddItems(info.ID, []string{testFile})
	require.NoError(t, err)

	assert.Len(t, info.Items, 1)
	assert.Equal(t, "hello.txt", info.Items[0].Name)
	assert.Equal(t, "plaintext", info.Items[0].Format)
	assert.Greater(t, info.Items[0].BlockCount, 0)
	assert.Greater(t, info.Items[0].WordCount, 0)
}

func TestAddFiles_Multiple(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	txtFile := filepath.Join("testdata", "hello.txt")
	htmlFile := filepath.Join("testdata", "page.html")

	info, err = app.AddItems(info.ID, []string{txtFile, htmlFile})
	require.NoError(t, err)

	assert.Len(t, info.Items, 2)

	// Check that both formats were detected.
	formats := make(map[string]bool)
	for _, item := range info.Items {
		formats[item.Format] = true
	}
	assert.True(t, formats["plaintext"], "expected plaintext format")
	assert.True(t, formats["html"], "expected html format")
}

func TestAddFiles_UnsupportedFormat(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	// Create a temp file with unsupported extension
	tmpDir := t.TempDir()
	tmpFile := filepath.Join(tmpDir, "test.xyz")
	err = os.WriteFile(tmpFile, []byte("test content"), 0644)
	require.NoError(t, err)

	// Should not error, just skip unsupported file
	info, err = app.AddItems(info.ID, []string{tmpFile})
	require.NoError(t, err)
	assert.Empty(t, info.Items)
}

func TestAddFiles_ProjectNotFound(t *testing.T) {
	app := newTestApp(t)

	_, err := app.AddItems("nonexistent", []string{"test.txt"})
	assert.Error(t, err)
}

func TestRemoveFile(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	testFile := filepath.Join("testdata", "hello.txt")
	info, err = app.AddItems(info.ID, []string{testFile})
	require.NoError(t, err)
	assert.Len(t, info.Items, 1)

	info, err = app.RemoveItem(info.ID, "hello.txt")
	require.NoError(t, err)
	assert.Empty(t, info.Items)
}

func TestRemoveFile_NotFound(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	_, err = app.RemoveItem(info.ID, "nonexistent.txt")
	assert.Error(t, err)
}

func TestGetFileBlocks(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	testFile := filepath.Join("testdata", "hello.txt")
	info, err = app.AddItems(info.ID, []string{testFile})
	require.NoError(t, err)

	blocks, err := app.GetItemBlocks(info.ID, "hello.txt")
	require.NoError(t, err)
	assert.NotEmpty(t, blocks)

	// Check that blocks have source text
	for _, b := range blocks {
		assert.NotEmpty(t, b.FlattenSource())
		assert.NotEmpty(t, b.ID)
	}
}

func TestGetFileBlocks_FileNotFound(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	// GetBlocks for a nonexistent item returns an empty slice (no error).
	blocks, err := app.GetItemBlocks(info.ID, "nonexistent.txt")
	require.NoError(t, err)
	assert.Empty(t, blocks)
}

func TestCountWords(t *testing.T) {
	tests := []struct {
		text     string
		expected int
	}{
		{"Hello world", 2},
		{"  leading spaces  ", 2},
		{"", 0},
		{"one", 1},
		{"multiple   spaces   between", 3},
	}

	for _, tt := range tests {
		t.Run(tt.text, func(t *testing.T) {
			assert.Equal(t, tt.expected, countWords(tt.text))
		})
	}
}

func TestCountChars(t *testing.T) {
	assert.Equal(t, 5, countChars("Hello"))
	assert.Equal(t, 0, countChars(""))
	assert.Equal(t, 3, countChars("日本語"))
}
