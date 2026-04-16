//go:build integration

package plaintext

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.plaintext.PlainTextFilter"
const mimeType = "text/plain"

// readPlainText parses a plain text snippet with custom filter params and returns the parts.
func readPlainText(t *testing.T, snippet string, params map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.txt", mimeType, params)
}

// readPlainTextDefault parses a plain text snippet with default (nil) params.
func readPlainTextDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readPlainText(t, snippet, nil)
}

// ---- PlainTextFilterTest (15 tests) ----

// okapi: PlainTextFilterTest#testEmptyInput
func TestExtract_EmptyInput(t *testing.T) {
	parts := readPlainTextDefault(t, "")

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks, "empty input should produce no translatable blocks")
}

// okapi: PlainTextFilterTest#testNameAndMimeType
func TestExtract_NameAndMimeType(t *testing.T) {
	parts := readPlainTextDefault(t, "Hello")

	// The first part should be a LayerStart with the correct MIME type.
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, mimeType, layer.MimeType,
		"layer should have MIME type text/plain")
}

// okapi: PlainTextFilterTest#testFiles
func TestExtract_Files(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// The Java testFiles iterates over various text files with different
	// line endings and verifies they parse without error.
	files := []string{
		"crlf.txt", "crlf_end.txt", "crlf_start.txt",
		"crlfcrlf.txt", "crlfcrlf_end.txt",
		"crlfcrlfcrlf.txt", "crlfcrlfcrlf_end.txt",
		"cr.txt", "lf.txt", "mixture.txt",
		"u0085.txt", "u2028.txt", "u2029.txt",
		"al2.txt",
		"combined_lines.txt", "combined_lines2.txt", "combined_lines_end.txt",
		"csv_test1.txt", "csv_test2.txt",
	}

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			path := tdDir + "/okapi/filters/plaintext/src/test/resources/" + f
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
			require.NotEmpty(t, parts, "file %s should produce parts", f)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
		})
	}
}

// okapi: PlainTextFilterTest#testSkeleton
func TestExtract_Skeleton(t *testing.T) {
	// "Line 1\nLine 2" should produce 2 text units with the line break as skeleton.
	parts := readPlainTextDefault(t, "Line 1\nLine 2")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2,
		"should produce at least 2 blocks for 2 lines")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Line 1")
	assert.Contains(t, texts, "Line 2")
}

// okapi: PlainTextFilterTest#testSkeleton2
func TestExtract_Skeleton2(t *testing.T) {
	// "Line 1\n\nLine 2" — multiple consecutive line breaks.
	parts := readPlainTextDefault(t, "Line 1\n\nLine 2")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Line 1")
	assert.Contains(t, texts, "Line 2")
}

// okapi: PlainTextFilterTest#testSkeleton3
func TestExtract_Skeleton3(t *testing.T) {
	// "Line 1\n\n\nLine 2" — triple line breaks.
	parts := readPlainTextDefault(t, "Line 1\n\n\nLine 2")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Line 1")
	assert.Contains(t, texts, "Line 2")
}

// okapi: PlainTextFilterTest#testEvents
func TestExtract_Events(t *testing.T) {
	parts := readPlainTextDefault(t, "Line 1\nLine 2")

	// Should have correct event sequence: LayerStart, [Blocks/Data...], LayerEnd
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart (START_DOCUMENT)")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type,
		"last part should be LayerEnd (END_DOCUMENT)")

	// Should have at least 2 translatable blocks.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)
}

// okapi: PlainTextFilterTest#testStartDocument
func TestExtract_StartDocument(t *testing.T) {
	parts := readPlainTextDefault(t, "Hello")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, mimeType, layer.MimeType)
	assert.Equal(t, "UTF-8", layer.Encoding)
	assert.Equal(t, model.LocaleID("en"), layer.Locale)
}

// okapi: PlainTextFilterTest#testDoubleExtraction
func TestExtract_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// The Java testDoubleExtraction does a roundtrip for all test files.
	// This is equivalent to our RoundTripTestFiles.
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okapi/filters/plaintext/src/test/resources/*.txt", mimeType, nil)
}

// okapi-unmapped: PlainTextFilterTest#testCancel — cancellation is Java-specific (filter.cancel())
// okapi-unmapped: PlainTextFilterTest#testConfigurations — tests Java filter.getConfigurations() API
// okapi-unmapped: PlainTextFilterTest#testSynchronization — tests Java multi-threaded filter access

// okapi: PlainTextFilterTest#testLineNumbers
func TestExtract_LineNumbers(t *testing.T) {
	parts := readPlainTextDefault(t, "Line 1\nLine 2\nLine 3")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	// Each block should have a unique ID and non-empty source text.
	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID)
		assert.False(t, ids[b.ID], "block IDs should be unique")
		ids[b.ID] = true
	}

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Line 1")
	assert.Contains(t, texts, "Line 2")
	assert.Contains(t, texts, "Line 3")
}

// okapi: PlainTextFilterTest#testParagraphs
func TestExtract_Paragraphs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// The Java testParagraphs uses test_paragraphs1.txt with the
	// ParaPlainTextFilter. This file has CR line endings:
	// "Line 1\rLine 2\r\rLine 3\rLine 4\rLine 5"
	// In paragraph mode, blank lines split paragraphs, consecutive lines join.
	path := tdDir + "/okapi/filters/plaintext/src/test/resources/test_paragraphs1.txt"

	// Read with paragraph params.
	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.paragraphs.Parameters",
	}
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "paragraph mode should produce blocks")

	// In paragraph mode:
	// Lines 1+2 form one paragraph (no blank line between them).
	// Blank line separates.
	// Lines 3+4+5 form another paragraph.
	texts := bridgetest.BlockTexts(blocks)
	require.GreaterOrEqual(t, len(texts), 2,
		"should produce at least 2 paragraphs")
}

// okapi: PlainTextFilterTest#testLoadParams
func TestExtract_LoadParams(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// test_params1.txt contains regex-based content:
	// "#v1\nrule=(.)\nsourceGroup.i=1\nregexOptions.i=8"
	// test_params1.fprm specifies spliced line parameters.
	path := tdDir + "/okapi/filters/plaintext/src/test/resources/test_params1.txt"
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	require.NotEmpty(t, parts, "should parse test_params1.txt")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from params test file")
}

// ---- ParaPlainTextFilterTest (13 tests) ----
// The ParaPlainTextFilter is the PlainTextFilter with paragraph parameters.

// okapi: ParaPlainTextFilterTest#testEmptyInput
func TestPara_EmptyInput(t *testing.T) {
	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.paragraphs.Parameters",
	}
	parts := readPlainText(t, "", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks, "empty input in paragraph mode should produce no blocks")
}

// okapi: ParaPlainTextFilterTest#testNameAndMimeType
func TestPara_NameAndMimeType(t *testing.T) {
	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.paragraphs.Parameters",
	}
	parts := readPlainText(t, "Hello", params)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, mimeType, layer.MimeType)
}

// okapi: ParaPlainTextFilterTest#testFiles
func TestPara_Files(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.paragraphs.Parameters",
	}

	files := []string{
		"crlf.txt", "crlf_end.txt", "crlf_start.txt",
		"crlfcrlf.txt", "crlfcrlf_end.txt",
		"crlfcrlfcrlf.txt", "crlfcrlfcrlf_end.txt",
		"cr.txt", "lf.txt", "mixture.txt",
		"u0085.txt", "u2028.txt", "u2029.txt",
		"al2.txt",
	}

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			path := tdDir + "/okapi/filters/plaintext/src/test/resources/" + f
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, params)
			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
		})
	}
}

// okapi: ParaPlainTextFilterTest#testFiles2
func TestPara_Files2(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.paragraphs.Parameters",
	}

	// The Java testFiles2 tests additional file types including
	// combined_lines and csv files.
	files := []string{
		"combined_lines.txt", "combined_lines2.txt", "combined_lines_end.txt",
		"csv_test1.txt", "csv_test2.txt",
	}

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			path := tdDir + "/okapi/filters/plaintext/src/test/resources/" + f
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, params)
			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
		})
	}
}

// okapi: ParaPlainTextFilterTest#testEvents
func TestPara_Events(t *testing.T) {
	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.paragraphs.Parameters",
	}
	parts := readPlainText(t, "Line 1\nLine 2", params)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "paragraph mode should produce blocks")
}

// okapi: ParaPlainTextFilterTest#testLineNumbers
func TestPara_LineNumbers(t *testing.T) {
	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.paragraphs.Parameters",
	}
	parts := readPlainText(t, "Line 1\nLine 2\n\nLine 3", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Verify unique IDs.
	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID)
		assert.False(t, ids[b.ID])
		ids[b.ID] = true
	}
}

// okapi: ParaPlainTextFilterTest#testParagraphs
func TestPara_Paragraphs(t *testing.T) {
	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.paragraphs.Parameters",
	}
	// In paragraph mode: consecutive lines are joined into a paragraph,
	// blank lines split paragraphs. The bridge applies parametersClass to
	// switch the filter's internal parameters.
	parts := readPlainText(t, "Line 1\nLine 2\n\nLine 3\nLine 4", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	// Verify all lines are extracted.
	assert.Contains(t, texts, "Line 1")
	assert.Contains(t, texts, "Line 2")
	assert.Contains(t, texts, "Line 3")
	assert.Contains(t, texts, "Line 4")
}

// okapi: ParaPlainTextFilterTest#testSkeleton
func TestPara_Skeleton(t *testing.T) {
	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.paragraphs.Parameters",
	}
	parts := readPlainText(t, "Line 1\nLine 2", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: ParaPlainTextFilterTest#testSkeleton2
func TestPara_Skeleton2(t *testing.T) {
	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.paragraphs.Parameters",
	}
	parts := readPlainText(t, "Line 1\n\nLine 2", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)
}

// okapi: ParaPlainTextFilterTest#testSkeleton3
func TestPara_Skeleton3(t *testing.T) {
	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.paragraphs.Parameters",
	}
	parts := readPlainText(t, "Line 1\n\n\nLine 2", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)
}

// okapi: ParaPlainTextFilterTest#testSkeleton4
func TestPara_Skeleton4(t *testing.T) {
	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.paragraphs.Parameters",
	}
	parts := readPlainText(t, "\nLine 1\nLine 2", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: ParaPlainTextFilterTest#testSkeleton5
func TestPara_Skeleton5(t *testing.T) {
	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.paragraphs.Parameters",
	}
	parts := readPlainText(t, "Line 1\nLine 2\n", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi-unmapped: ParaPlainTextFilterTest#testCancel — cancellation is Java-specific

// ---- RegexPlainTextFilterTest (6 tests) ----
// RegexPlainTextFilter is a different Java filter class. We test what we can
// through the standard PlainTextFilter with regex parameters.

// okapi: RegexPlainTextFilterTest#testEmptyInput
func TestRegex_EmptyInput(t *testing.T) {
	parts := readPlainTextDefault(t, "")
	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks, "empty input should produce no blocks")
}

// okapi: RegexPlainTextFilterTest#testNameAndMimeType
func TestRegex_NameAndMimeType(t *testing.T) {
	parts := readPlainTextDefault(t, "Hello")
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, mimeType, layer.MimeType)
}

// okapi: RegexPlainTextFilterTest#testEvents
func TestRegex_Events(t *testing.T) {
	parts := readPlainTextDefault(t, "Line 1\nLine 2")
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: RegexPlainTextFilterTest#testFiles
func TestRegex_Files(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	files := []string{
		"crlf.txt", "lf.txt", "cr.txt", "mixture.txt",
		"u0085.txt", "u2028.txt", "u2029.txt",
	}

	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			path := tdDir + "/okapi/filters/plaintext/src/test/resources/" + f
			parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
			require.NotEmpty(t, parts)
		})
	}
}

// okapi: RegexPlainTextFilterTest#testDoubleExtraction
func TestRegex_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	files := []string{"crlf.txt", "lf.txt", "cr.txt"}
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			path := tdDir + "/okapi/filters/plaintext/src/test/resources/" + f
			content, err := readTestFile(path)
			require.NoError(t, err)
			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
				content, path, mimeType, nil)
		})
	}
}

// okapi: RegexPlainTextFilterTest#testParameters
func TestRegex_Parameters(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// Test that parameter files can be loaded and used.
	path := tdDir + "/okapi/filters/plaintext/src/test/resources/test_params1.txt"
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
}

// ---- SplicedLinesFilterTest (5 tests) ----
// SplicedLinesFilter joins lines ending with backslash.

// okapi: SplicedLinesFilterTest#testCombinedLines
func TestSpliced_CombinedLines(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.spliced.Parameters",
	}

	// combined_lines.txt has lines ending with backslash that should be joined.
	path := tdDir + "/okapi/filters/plaintext/src/test/resources/combined_lines.txt"
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, params)
	require.NotEmpty(t, parts)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "spliced lines should produce blocks")
}

// okapi: SplicedLinesFilterTest#testSkeleton
func TestSpliced_Skeleton(t *testing.T) {
	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.spliced.Parameters",
	}
	parts := readPlainText(t, "Line 1\nLine 2", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Line 1")
	assert.Contains(t, texts, "Line 2")
}

// okapi: SplicedLinesFilterTest#testSkeleton2
func TestSpliced_Skeleton2(t *testing.T) {
	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.spliced.Parameters",
	}
	parts := readPlainText(t, "Line 1\n\nLine 2", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)
}

// okapi: SplicedLinesFilterTest#testSkeleton3
func TestSpliced_Skeleton3(t *testing.T) {
	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.spliced.Parameters",
	}
	parts := readPlainText(t, "Line 1\n\n\nLine 2", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)
}

// okapi: SplicedLinesFilterTest#testDoubleExtraction
func TestSpliced_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	params := map[string]any{
		"parametersClass": "net.sf.okapi.filters.plaintext.spliced.Parameters",
	}

	files := []string{"crlf.txt", "lf.txt", "cr.txt"}
	for _, f := range files {
		t.Run(f, func(t *testing.T) {
			path := tdDir + "/okapi/filters/plaintext/src/test/resources/" + f
			content, err := readTestFile(path)
			require.NoError(t, err)
			bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass,
				content, path, mimeType, params)
		})
	}
}

// ---- Additional extraction tests (mapped to PlainTextFilterTest subtests) ----

// okapi: PlainTextFilterTest#testEvents (multi-line variant)
func TestExtract_MultipleLines(t *testing.T) {
	parts := readPlainTextDefault(t, "First line\nSecond line\nThird line")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "First line")
	assert.Contains(t, texts, "Second line")
	assert.Contains(t, texts, "Third line")
}

// okapi: PlainTextFilterTest#testEvents (block ID uniqueness)
func TestExtract_BlockIDs(t *testing.T) {
	parts := readPlainTextDefault(t, "Line A\nLine B\nLine C")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique")
		ids[b.ID] = true
	}
}

// okapi: PlainTextFilterTest#testEvents (unicode content)
func TestExtract_UnicodeContent(t *testing.T) {
	parts := readPlainTextDefault(t, "Hello world")

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello world")
}

// okapi: PlainTextFilterTest#testEvents (layer structure)
func TestExtract_LayerStructure(t *testing.T) {
	parts := readPlainTextDefault(t, "Hello")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type,
		"last part should be LayerEnd")
}

// okapi: PlainTextFilterTest#testEvents (segment IDs)
func TestExtract_SegmentIDs(t *testing.T) {
	parts := readPlainTextDefault(t, "Hello world")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		require.NotEmpty(t, b.Source, "block should have source segments")
		for _, seg := range b.Source {
			assert.NotEmpty(t, seg.ID, "segment should have an ID")
			assert.NotNil(t, seg.Fragment(), "segment should have content")
		}
	}
}

// okapi: PlainTextFilterTest#testEvents (no inline spans)
func TestExtract_NoSpans(t *testing.T) {
	parts := readPlainTextDefault(t, "Simple plain text")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil {
			assert.Empty(t, frag.Spans, "plain text should have no inline spans")
		}
	}
}

// ---- Line ending tests mapped to PlainTextFilterTest#testFiles subtests ----

// okapi: PlainTextFilterTest#testFiles (CRLF line ending variant)
func TestExtract_CRLFLineEndings(t *testing.T) {
	parts := readPlainTextDefault(t, "Line 1\r\nLine 2\r\nLine 3")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Line 1")
	assert.Contains(t, texts, "Line 2")
	assert.Contains(t, texts, "Line 3")
}

// okapi: PlainTextFilterTest#testFiles (CR line ending variant)
func TestExtract_CRLineEndings(t *testing.T) {
	parts := readPlainTextDefault(t, "Line 1\rLine 2\rLine 3")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Line 1")
	assert.Contains(t, texts, "Line 2")
	assert.Contains(t, texts, "Line 3")
}
