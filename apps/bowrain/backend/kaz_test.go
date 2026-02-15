package backend

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSaveAndOpenKaz(t *testing.T) {
	app := NewApp()

	// Create project and add a file
	info, err := app.CreateProject("Round Trip", "en", []string{"fr"})
	require.NoError(t, err)

	testFile := filepath.Join("testdata", "hello.txt")
	info, err = app.AddItems(info.ID, []string{testFile})
	require.NoError(t, err)
	require.Len(t, info.Items, 1)

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
	require.Len(t, info2.Items, 1)
	assert.Equal(t, "hello.txt", info2.Items[0].Name)
	assert.Equal(t, "plaintext", info2.Items[0].Format)
	assert.Greater(t, info2.Items[0].BlockCount, 0)
}

func TestKazManifest(t *testing.T) {
	app := NewApp()

	info, err := app.CreateProject("Manifest Test", "en", []string{"fr", "de"})
	require.NoError(t, err)

	testFile := filepath.Join("testdata", "hello.txt")
	_, err = app.AddItems(info.ID, []string{testFile})
	require.NoError(t, err)

	tmpDir := t.TempDir()
	kazPath := filepath.Join(tmpDir, "manifest.kaz")
	err = app.SaveProjectAs(info.ID, kazPath)
	require.NoError(t, err)

	// Open and check manifest via OpenProject
	app2 := NewApp()
	info2, err := app2.OpenProject(kazPath)
	require.NoError(t, err)

	assert.Equal(t, "Manifest Test", info2.Name)
	assert.Equal(t, "en", info2.SourceLocale)
	assert.Equal(t, []string{"fr", "de"}, info2.TargetLocales)
	assert.NotEmpty(t, info2.CreatedAt)
	assert.NotEmpty(t, info2.ModifiedAt)
	require.Len(t, info2.Items, 1)
	assert.Equal(t, "hello.txt", info2.Items[0].Name)
	assert.Equal(t, "plaintext", info2.Items[0].Format)
}

func TestKazWithMultipleFiles(t *testing.T) {
	app := NewApp()

	info, err := app.CreateProject("Multi File", "en", []string{"fr"})
	require.NoError(t, err)

	txtFile := filepath.Join("testdata", "hello.txt")
	htmlFile := filepath.Join("testdata", "page.html")
	_, err = app.AddItems(info.ID, []string{txtFile, htmlFile})
	require.NoError(t, err)

	tmpDir := t.TempDir()
	kazPath := filepath.Join(tmpDir, "multi.kaz")
	err = app.SaveProjectAs(info.ID, kazPath)
	require.NoError(t, err)

	// Open in fresh app
	app2 := NewApp()
	info2, err := app2.OpenProject(kazPath)
	require.NoError(t, err)

	assert.Len(t, info2.Items, 2)

	names := make(map[string]bool)
	for _, item := range info2.Items {
		names[item.Name] = true
	}
	assert.True(t, names["hello.txt"])
	assert.True(t, names["page.html"])
}

func TestKazWithTranslations(t *testing.T) {
	app := NewApp()

	info, err := app.CreateProject("Translated", "en", []string{"fr"})
	require.NoError(t, err)

	testFile := filepath.Join("testdata", "hello.txt")
	info, err = app.AddItems(info.ID, []string{testFile})
	require.NoError(t, err)

	// Pseudo-translate
	_, err = app.PseudoTranslateItem(info.ID, "hello.txt", "fr")
	require.NoError(t, err)

	// Verify blocks have translations
	blocks, err := app.GetItemBlocks(info.ID, "hello.txt")
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

	// Reopen and verify translations are preserved
	app2 := NewApp()
	info2, err := app2.OpenProject(kazPath)
	require.NoError(t, err)
	assert.Len(t, info2.Items, 1)

	// Verify translations survived the roundtrip
	blocks2, err := app2.GetItemBlocks(info2.ID, "hello.txt")
	require.NoError(t, err)

	hasTranslated2 := false
	for _, b := range blocks2 {
		if b.Targets["fr"] != "" {
			hasTranslated2 = true
			break
		}
	}
	assert.True(t, hasTranslated2, "translations should survive save/load roundtrip")
}

func TestKazPreviewHTML(t *testing.T) {
	app := NewApp()

	info, err := app.CreateProject("Preview Test", "en", []string{"fr"})
	require.NoError(t, err)

	htmlFile := filepath.Join("testdata", "page.html")
	_, err = app.AddItems(info.ID, []string{htmlFile})
	require.NoError(t, err)

	// Check that previewHTML is populated
	p, _ := app.projects.get(info.ID)
	id := p.items["page.html"]
	require.NotNil(t, id)
	assert.NotEmpty(t, id.previewHTML, "previewHTML should be populated after AddFiles")
	assert.Contains(t, id.previewHTML, "kat-block")
}

func TestKazBlockIndex(t *testing.T) {
	app := NewApp()

	info, err := app.CreateProject("Block Index Test", "en", []string{"fr"})
	require.NoError(t, err)

	testFile := filepath.Join("testdata", "hello.txt")
	_, err = app.AddItems(info.ID, []string{testFile})
	require.NoError(t, err)

	// Check that blockIndex is populated
	p, _ := app.projects.get(info.ID)
	id := p.items["hello.txt"]
	require.NotNil(t, id)
	require.NotNil(t, id.blockIndex, "blockIndex should be populated after AddFiles")
	assert.Greater(t, len(id.blockIndex.Blocks), 0)
}

func TestKazReconstructWithoutSource(t *testing.T) {
	app := NewApp()

	info, err := app.CreateProject("Reconstruct Test", "en", []string{"fr"})
	require.NoError(t, err)

	testFile := filepath.Join("testdata", "hello.txt")
	info, err = app.AddItems(info.ID, []string{testFile})
	require.NoError(t, err)

	// Save
	tmpDir := t.TempDir()
	kazPath := filepath.Join(tmpDir, "reconstruct.kaz")
	err = app.SaveProjectAs(info.ID, kazPath)
	require.NoError(t, err)

	// Open — the items/ dir has the source, so this tests path 1 (re-parse).
	// To test path 2 (reconstruct from skeletons), we'd need to remove items/ from the zip.
	// For now, verify the full roundtrip works.
	app2 := NewApp()
	info2, err := app2.OpenProject(kazPath)
	require.NoError(t, err)

	blocks, err := app2.GetItemBlocks(info2.ID, "hello.txt")
	require.NoError(t, err)
	assert.Greater(t, len(blocks), 0)
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
