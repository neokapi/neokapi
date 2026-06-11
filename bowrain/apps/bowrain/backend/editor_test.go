package backend

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/bowrain/core/store"
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
	require.NotEmpty(t, blocks)

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
	frRuns := updated[0].TargetRuns["fr"]
	require.Len(t, frRuns, 1)
	require.NotNil(t, frRuns[0].Text)
	assert.Equal(t, "Bonjour le monde", frRuns[0].Text.Text)
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
			assert.NotEmpty(t, flattenTargetRuns(b, "fr"), "block %q should have fr target", b.ID)
			assert.Contains(t, flattenTargetRuns(b, "fr"), "[", "pseudo target should have brackets")
			assert.Contains(t, flattenTargetRuns(b, "fr"), "]", "pseudo target should have brackets")
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

	// Verify we have blocks with inline-code runs before pseudo-translating.
	blocksBefore, err := app.GetItemBlocks(info.ID, "inline.html")
	require.NoError(t, err)
	var spanBlock *BlockInfo
	for i := range blocksBefore {
		if runInfosHaveInline(blocksBefore[i].SourceRuns) {
			spanBlock = &blocksBefore[i]
			break
		}
	}
	require.NotNil(t, spanBlock, "expected at least one block with inline codes")
	require.NotEmpty(t, spanBlock.SourceRuns, "block should have source runs")

	// Pseudo-translate
	stats, err := app.PseudoTranslateItem(info.ID, "inline.html", "fr")
	require.NoError(t, err)
	assert.Greater(t, stats.TranslatedBlocks, 0)

	// Verify blocks after pseudo-translation.
	blocksAfter, err := app.GetItemBlocks(info.ID, "inline.html")
	require.NoError(t, err)

	for _, b := range blocksAfter {
		if !b.Translatable || !runInfosHaveInline(b.SourceRuns) {
			continue
		}
		targetRuns, ok := b.TargetRuns["fr"]
		require.True(t, ok, "block %q should have target runs for fr", b.ID)
		require.NotEmpty(t, targetRuns, "target runs should not be empty")

		// The translation must preserve the inline-code runs
		// (non-deletable Ph/PcOpen/PcClose). Count matches source.
		srcInline := countInlineCodes(b.SourceRuns)
		tgtInline := countInlineCodes(targetRuns)
		assert.Equal(t, srcInline, tgtInline, "inline-code count should match source")

		// Pseudo-translated text should carry the [...] wrapping
		// emitted by the pseudo-translate tool.
		hasBracket := false
		for _, r := range targetRuns {
			if r.Text != nil && (strings.Contains(r.Text.Text, "[") || strings.Contains(r.Text.Text, "]")) {
				hasBracket = true
				break
			}
		}
		assert.True(t, hasBracket, "pseudo-translated target should contain bracket wrap")
	}
}

// runInfosHaveInline reports whether a RunInfo slice contains any
// non-text run (inline code).
func runInfosHaveInline(runs []RunInfo) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}

// countInlineCodes counts non-text runs in a RunInfo slice.
func countInlineCodes(runs []RunInfo) int {
	n := 0
	for _, r := range runs {
		if r.Text == nil {
			n++
		}
	}
	return n
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

	// Server-side export is no longer supported (source bytes removed).
	_, err := app.ExportTranslatedItem(info.ID, itemName, "fr")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "server-side export not available")
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

	ctx := t.Context()
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
	assert.NotEmpty(t, blocks)

	// Check for expected content
	sources := make([]string, 0)
	for _, b := range blocks {
		sources = append(sources, b.FlattenSource())
	}
	assert.NotEmpty(t, sources)
}

func TestTMTranslateFile(t *testing.T) {
	app, info, itemName := setupProjectWithFile(t)

	// TM is empty so no matches expected, but should not error
	stats, err := app.TMTranslateItem(info.ID, itemName, "fr")
	require.NoError(t, err)
	assert.Greater(t, stats.TotalBlocks, 0)
	assert.Equal(t, 0, stats.TranslatedBlocks) // Empty TM = no matches
}
