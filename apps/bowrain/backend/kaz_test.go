package backend

import (
	"archive/zip"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

func TestSaveAndOpenKaz(t *testing.T) {
	app := NewApp()

	// Create project and add a file
	info, err := app.CreateProject("Round Trip", "en", []string{"fr"})
	require.NoError(t, err)

	testFile := filepath.Join("testdata", "hello.txt")
	info, err = app.AddFiles(info.ID, []string{testFile})
	require.NoError(t, err)
	require.Len(t, info.Files, 1)

	// Save as .kaz
	tmpDir := t.TempDir()
	kazPath := filepath.Join(tmpDir, "test.kaz")
	err = app.SaveProjectAs(info.ID, kazPath)
	require.NoError(t, err)

	// Verify .kaz file exists
	_, err = os.Stat(kazPath)
	require.NoError(t, err)

	// Open the .kaz in a fresh app
	app2 := NewApp()
	info2, err := app2.OpenProject(kazPath)
	require.NoError(t, err)

	assert.Equal(t, "Round Trip", info2.Name)
	assert.Equal(t, "en", info2.SourceLocale)
	assert.Equal(t, []string{"fr"}, info2.TargetLocales)
	assert.Equal(t, kazPath, info2.Path)
	require.Len(t, info2.Files, 1)
	assert.Equal(t, "hello.txt", info2.Files[0].Name)
	assert.Equal(t, "plaintext", info2.Files[0].Format)
	assert.Greater(t, info2.Files[0].BlockCount, 0)
}

func TestKazManifest(t *testing.T) {
	app := NewApp()

	info, err := app.CreateProject("Manifest Test", "en", []string{"fr", "de"})
	require.NoError(t, err)

	testFile := filepath.Join("testdata", "hello.txt")
	_, err = app.AddFiles(info.ID, []string{testFile})
	require.NoError(t, err)

	tmpDir := t.TempDir()
	kazPath := filepath.Join(tmpDir, "manifest.kaz")
	err = app.SaveProjectAs(info.ID, kazPath)
	require.NoError(t, err)

	// Read the .kaz and check manifest
	r, err := zip.OpenReader(kazPath)
	require.NoError(t, err)
	defer r.Close()

	var manifest kazManifest
	for _, f := range r.File {
		if f.Name == "manifest.yaml" {
			rc, err := f.Open()
			require.NoError(t, err)
			defer rc.Close()

			dec := yaml.NewDecoder(rc)
			err = dec.Decode(&manifest)
			require.NoError(t, err)
			break
		}
	}

	assert.Equal(t, "Manifest Test", manifest.Name)
	assert.Equal(t, "1.0", manifest.Version)
	assert.Equal(t, "0.1.0", manifest.GokapiVersion)
	assert.Equal(t, "en", manifest.SourceLocale)
	assert.Equal(t, []string{"fr", "de"}, manifest.TargetLocales)
	assert.NotEmpty(t, manifest.CreatedAt)
	assert.NotEmpty(t, manifest.ModifiedAt)
	require.Len(t, manifest.Files, 1)
	assert.Equal(t, "hello.txt", manifest.Files[0].Path)
	assert.Equal(t, "plaintext", manifest.Files[0].Format)
}

func TestKazWithMultipleFiles(t *testing.T) {
	app := NewApp()

	info, err := app.CreateProject("Multi File", "en", []string{"fr"})
	require.NoError(t, err)

	txtFile := filepath.Join("testdata", "hello.txt")
	htmlFile := filepath.Join("testdata", "page.html")
	_, err = app.AddFiles(info.ID, []string{txtFile, htmlFile})
	require.NoError(t, err)

	tmpDir := t.TempDir()
	kazPath := filepath.Join(tmpDir, "multi.kaz")
	err = app.SaveProjectAs(info.ID, kazPath)
	require.NoError(t, err)

	// Open in fresh app
	app2 := NewApp()
	info2, err := app2.OpenProject(kazPath)
	require.NoError(t, err)

	assert.Len(t, info2.Files, 2)

	names := make(map[string]bool)
	for _, f := range info2.Files {
		names[f.Name] = true
	}
	assert.True(t, names["hello.txt"])
	assert.True(t, names["page.html"])
}

func TestKazWithTranslations(t *testing.T) {
	app := NewApp()

	info, err := app.CreateProject("Translated", "en", []string{"fr"})
	require.NoError(t, err)

	testFile := filepath.Join("testdata", "hello.txt")
	info, err = app.AddFiles(info.ID, []string{testFile})
	require.NoError(t, err)

	// Pseudo-translate
	_, err = app.PseudoTranslateFile(info.ID, "hello.txt", "fr")
	require.NoError(t, err)

	// Verify blocks have translations
	blocks, err := app.GetFileBlocks(info.ID, "hello.txt")
	require.NoError(t, err)

	hasTranslated := false
	for _, b := range blocks {
		if b.Targets["fr"] != "" {
			hasTranslated = true
			break
		}
	}
	assert.True(t, hasTranslated, "expected at least one translated block")

	// Save and reopen
	tmpDir := t.TempDir()
	kazPath := filepath.Join(tmpDir, "translated.kaz")
	err = app.SaveProjectAs(info.ID, kazPath)
	require.NoError(t, err)

	// Note: translations are preserved in the XLIFF within the .kaz
	// Reopening will re-parse source files (translations persist via XLIFF)
	app2 := NewApp()
	info2, err := app2.OpenProject(kazPath)
	require.NoError(t, err)
	assert.Len(t, info2.Files, 1)
}

func TestOpenInvalidKaz(t *testing.T) {
	app := NewApp()

	// Try to open a non-existent file
	_, err := app.OpenProject("/nonexistent/path.kaz")
	assert.Error(t, err)

	// Create a file that's not a valid ZIP
	tmpDir := t.TempDir()
	badFile := filepath.Join(tmpDir, "bad.kaz")
	err = os.WriteFile(badFile, []byte("not a zip"), 0644)
	require.NoError(t, err)

	_, err = app.OpenProject(badFile)
	assert.Error(t, err)

	// Create a ZIP without manifest
	noManifest := filepath.Join(tmpDir, "nomanifest.kaz")
	f, err := os.Create(noManifest)
	require.NoError(t, err)
	w := zip.NewWriter(f)
	fw, _ := w.Create("dummy.txt")
	fw.Write([]byte("test"))
	w.Close()
	f.Close()

	_, err = app.OpenProject(noManifest)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "manifest.yaml not found")
}

func TestSaveProject_NoPath(t *testing.T) {
	app := NewApp()

	info, err := app.CreateProject("No Path", "en", []string{"fr"})
	require.NoError(t, err)

	err = app.SaveProject(info.ID)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no save path")
}

func TestKazExtensionAdded(t *testing.T) {
	app := NewApp()

	info, err := app.CreateProject("Ext Test", "en", []string{"fr"})
	require.NoError(t, err)

	tmpDir := t.TempDir()
	kazPath := filepath.Join(tmpDir, "test")
	err = app.SaveProjectAs(info.ID, kazPath)
	require.NoError(t, err)

	// Should have added .kaz extension
	info, err = app.GetProject(info.ID)
	require.NoError(t, err)
	assert.True(t, filepath.Ext(info.Path) == ".kaz")
}
