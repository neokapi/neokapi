package backend

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gokapi/gokapi/platform/store"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupProjectWithFile(t *testing.T) (*App, *ProjectInfo, string) {
	t.Helper()
	app := newTestApp(t)

	info, err := app.CreateProject("Editor Test", "en", []string{"fr", "de"})
	require.NoError(t, err)

	testFile := filepath.Join("testdata", "hello.txt")
	info, err = app.AddItems(info.ID, []string{testFile})
	require.NoError(t, err)
	require.Len(t, info.Items, 1)

	return app, info, "hello.txt"
}

func TestUpdateBlockTarget(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	blocks, err := app.GetItemBlocks(info.ID, itemName)
	require.NoError(t, err)
	require.Greater(t, len(blocks), 0)

	// Update the first block's target
	err = app.UpdateBlockTarget(UpdateBlockRequest{
		ProjectID:    info.ID,
		ItemName:     itemName,
		BlockID:      blocks[0].ID,
		TargetLocale: "fr",
		Text:         "Bonjour le monde",
	})
	require.NoError(t, err)

	// Verify the update
	updated, err := app.GetItemBlocks(info.ID, itemName)
	require.NoError(t, err)
	assert.Equal(t, "Bonjour le monde", updated[0].Targets["fr"])
}

func TestUpdateBlockTarget_NotFound(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	err := app.UpdateBlockTarget(UpdateBlockRequest{
		ProjectID:    info.ID,
		ItemName:     itemName,
		BlockID:      "nonexistent-block-id",
		TargetLocale: "fr",
		Text:         "test",
	})
	assert.Error(t, err)
}

func TestPseudoTranslateFile(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	stats, err := app.PseudoTranslateItem(info.ID, itemName, "fr")
	require.NoError(t, err)

	assert.Greater(t, stats.TotalBlocks, 0)
	assert.Equal(t, stats.TotalBlocks, stats.TranslatedBlocks)
	assert.Greater(t, stats.WordCount, 0)

	// Verify blocks have pseudo-translated targets
	blocks, err := app.GetItemBlocks(info.ID, itemName)
	require.NoError(t, err)

	for _, b := range blocks {
		if b.Translatable {
			assert.NotEmpty(t, b.Targets["fr"], "block %q should have fr target", b.ID)
			assert.Contains(t, b.Targets["fr"], "[", "pseudo target should have brackets")
			assert.Contains(t, b.Targets["fr"], "]", "pseudo target should have brackets")
		}
	}
}

func TestPseudoTranslateFile_PreservesSpans(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Inline Test", "en", []string{"fr"})
	require.NoError(t, err)

	htmlFile := filepath.Join("testdata", "inline.html")
	info, err = app.AddItems(info.ID, []string{htmlFile})
	require.NoError(t, err)

	// Verify we have blocks with spans before pseudo-translating
	blocksBefore, err := app.GetItemBlocks(info.ID, "inline.html")
	require.NoError(t, err)
	var spanBlock *BlockInfo
	for i := range blocksBefore {
		if blocksBefore[i].HasSpans {
			spanBlock = &blocksBefore[i]
			break
		}
	}
	require.NotNil(t, spanBlock, "expected at least one block with inline spans")
	require.NotEmpty(t, spanBlock.SourceSpans, "block should have source spans")

	// Pseudo-translate
	stats, err := app.PseudoTranslateItem(info.ID, "inline.html", "fr")
	require.NoError(t, err)
	assert.Greater(t, stats.TranslatedBlocks, 0)

	// Verify blocks after pseudo-translation
	blocksAfter, err := app.GetItemBlocks(info.ID, "inline.html")
	require.NoError(t, err)

	for _, b := range blocksAfter {
		if !b.HasSpans || !b.Translatable {
			continue
		}
		// Target coded text should be populated and contain markers
		targetCoded, ok := b.TargetsCoded["fr"]
		require.True(t, ok, "block %q should have coded target for fr", b.ID)
		assert.NotEmpty(t, targetCoded, "coded target should not be empty")

		// Should contain at least one marker character
		hasMarker := false
		for _, r := range targetCoded {
			if r >= '\uE001' && r <= '\uE003' {
				hasMarker = true
				break
			}
		}
		assert.True(t, hasMarker, "coded target should contain span markers")

		// Plain target should have brackets
		plainTarget := b.Targets["fr"]
		assert.Contains(t, plainTarget, "[")
		assert.Contains(t, plainTarget, "]")
	}
}

func TestPseudoTranslateFile_FileNotFound(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	// PseudoTranslateItem on a nonexistent item returns no error but zero stats
	// because GetBlocks returns an empty slice for nonexistent items.
	stats, err := app.PseudoTranslateItem(info.ID, "nonexistent.txt", "fr")
	require.NoError(t, err)
	assert.Equal(t, 0, stats.TotalBlocks)
}

func TestGetWordCount(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	wc, err := app.GetWordCount(info.ID, itemName)
	require.NoError(t, err)

	assert.Greater(t, wc.SourceWords, 0)
	assert.Greater(t, wc.SourceChars, 0)

	// No translations yet, target counts should be zero
	assert.Equal(t, 0, wc.TargetWords["fr"])

	// Now pseudo-translate and check again
	_, err = app.PseudoTranslateItem(info.ID, itemName, "fr")
	require.NoError(t, err)

	wc, err = app.GetWordCount(info.ID, itemName)
	require.NoError(t, err)
	assert.Greater(t, wc.TargetWords["fr"], 0)
	assert.Greater(t, wc.TargetChars["fr"], 0)
}

func TestGetWordCount_FileNotFound(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	// GetWordCount on a nonexistent item returns zero counts (no error)
	// because GetBlocks returns an empty slice.
	wc, err := app.GetWordCount(info.ID, "nonexistent.txt")
	require.NoError(t, err)
	assert.Equal(t, 0, wc.SourceWords)
}

func TestExportTranslatedFile(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	// Pseudo-translate first
	_, err := app.PseudoTranslateItem(info.ID, itemName, "fr")
	require.NoError(t, err)

	outputPath, err := app.ExportTranslatedItem(info.ID, itemName, "fr")
	require.NoError(t, err)
	assert.Contains(t, outputPath, "_fr")
	assert.Contains(t, outputPath, ".txt")

	// Verify file was created
	_, err = os.Stat(outputPath)
	require.NoError(t, err)

	// Read and verify content
	content, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	assert.NotEmpty(t, content)
}

func TestExportTranslatedFile_FileNotFound(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("Test", "en", []string{"fr"})
	require.NoError(t, err)

	_, err = app.ExportTranslatedItem(info.ID, "nonexistent.txt", "fr")
	assert.Error(t, err)
}

func TestPseudoAccent(t *testing.T) {
	result := pseudoAccent("Hello World")
	assert.NotEqual(t, "Hello World", result)
	assert.Contains(t, result, "\u0124") // H -> Ĥ
}

func TestComputeStats(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	ctx := context.Background()
	storedBlocks, err := app.store.GetBlocks(ctx, store.BlockQuery{
		ProjectID: info.ID,
		ItemName:  itemName,
	})
	require.NoError(t, err)

	parts := storedBlocksToParts(storedBlocks)
	stats := computeStats(parts, "fr")

	assert.Greater(t, stats.TotalBlocks, 0)
	assert.Equal(t, 0, stats.TranslatedBlocks) // No translations yet
	assert.Greater(t, stats.WordCount, 0)
}

func TestHTMLFileBlocks(t *testing.T) {
	app := newTestApp(t)

	info, err := app.CreateProject("HTML Test", "en", []string{"fr"})
	require.NoError(t, err)

	htmlFile := filepath.Join("testdata", "page.html")
	info, err = app.AddItems(info.ID, []string{htmlFile})
	require.NoError(t, err)

	blocks, err := app.GetItemBlocks(info.ID, "page.html")
	require.NoError(t, err)
	assert.Greater(t, len(blocks), 0)

	// Check for expected content
	sources := make([]string, 0)
	for _, b := range blocks {
		sources = append(sources, b.Source)
	}
	assert.NotEmpty(t, sources)
}

func TestAITranslateFile_MockProvider(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	// Use mock provider (default when provider is empty/unknown)
	stats, err := app.AITranslateItem(AITranslateFileRequest{
		ProjectID:    info.ID,
		ItemName:     itemName,
		TargetLocale: "fr",
		Provider:     "mock",
	})
	require.NoError(t, err)
	assert.Greater(t, stats.TotalBlocks, 0)
}

func TestTMTranslateFile(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	// TM is empty so no matches expected, but should not error
	stats, err := app.TMTranslateItem(info.ID, itemName, "fr")
	require.NoError(t, err)
	assert.Greater(t, stats.TotalBlocks, 0)
	assert.Equal(t, 0, stats.TranslatedBlocks) // Empty TM = no matches
}
