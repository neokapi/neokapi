package openxml

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testdataDir returns the path to the okapi-testdata OpenXML directory.
// Returns "" if the testdata is not available (skips the test).
func testdataDir(t *testing.T) string {
	t.Helper()
	// Walk up from the test binary working directory to find the repo root.
	dir, err := os.Getwd()
	require.NoError(t, err)

	for {
		if _, err := os.Stat(filepath.Join(dir, "go.work")); err == nil {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Skip("could not find repo root (go.work)")
			return ""
		}
		dir = parent
	}

	baseDir := filepath.Join(dir, "okapi-testdata")
	if _, err := os.Stat(baseDir); os.IsNotExist(err) {
		t.Skip("okapi-testdata/ not found — run scripts/fetch-okapi-testdata.sh")
		return ""
	}

	entries, err := os.ReadDir(baseDir)
	require.NoError(t, err)

	var latest string
	for _, e := range entries {
		if e.IsDir() {
			if _, serr := os.Stat(filepath.Join(baseDir, e.Name(), "okf_openxml")); serr == nil {
				latest = e.Name()
			}
		}
	}
	if latest == "" {
		t.Skip("no okapi-testdata version found with okf_openxml/")
		return ""
	}

	return filepath.Join(baseDir, latest, "okf_openxml")
}

func readFile(t *testing.T, path string) []*model.Part {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)

	reader := NewReader()
	doc := testutil.RawDocFromReader(f, path, model.LocaleEnglish)
	err = reader.Open(context.Background(), doc)
	require.NoError(t, err)
	defer reader.Close()

	return testutil.CollectParts(t, reader.Read(context.Background()))
}

// --- DOCX tests mirroring bridge tests ---

func TestNative_SimpleDocx(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "948-1.docx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX should produce translatable blocks")

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Run1 Run3")
}

func TestNative_DocxLayerStructure(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "948-1.docx"))

	var starts, ends int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			starts++
		}
		if p.Type == model.PartLayerEnd {
			ends++
		}
	}
	assert.True(t, starts > 0, "should have LayerStart")
	assert.Equal(t, starts, ends, "layer starts and ends should be balanced")
}

func TestNative_DocxMultiLayer(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "948-1.docx"))

	var layerStartCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerStartCount++
		}
	}
	assert.Greater(t, layerStartCount, 1, "OpenXML should produce multiple layers")
}

func TestNative_DocxBlockIDs(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "948-1.docx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

func TestNative_DocxWithTabs(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "Document-with-tabs.docx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with tabs should produce translatable blocks")
}

func TestNative_DocxSoftLineBreaks(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "Document-with-soft-linebreaks.docx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with soft line breaks should produce blocks")
}

func TestNative_DocxTextBoxes(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "TextBoxes.docx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with text boxes should produce blocks")
}

func TestNative_DocxSmartArt(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "smart_art.docx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with SmartArt should produce blocks")
}

func TestNative_DocxWatermark(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "watermark.docx"))

	require.NotEmpty(t, parts, "DOCX with watermark should produce parts")

	var hasLayerStart bool
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			hasLayerStart = true
			break
		}
	}
	assert.True(t, hasLayerStart, "watermark DOCX should have layer structure")
}

func TestNative_DocxSpecialChars(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "special-chars-and-linebreaks.docx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with special chars should produce blocks")
}

func TestNative_DocxNotes(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "1413-notes.docx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with footnotes/endnotes should produce blocks")

	var layerCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerCount++
		}
	}
	assert.Greater(t, layerCount, 1, "footnotes/endnotes should create additional layers")
}

func TestNative_DocxExternalHyperlink(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "external_hyperlink.docx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with external hyperlinks should produce blocks")
}

func TestNative_DocxNestedTables(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "848-nested-tables.docx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "DOCX with nested tables should produce blocks")
}

func TestNative_DocxDocProperties(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "DocProperties.docx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "DocProperties.docx should produce translatable blocks")

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Ode to the IRS")
	assert.Contains(t, texts, "John Doe")
}

func TestNative_DocxReorderedZip(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "reordered-zip.docx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "reordered ZIP DOCX should produce blocks")
}

// --- XLSX tests ---

func TestNative_SimpleXlsx(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "pokemon.xlsx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "XLSX should produce translatable blocks")
}

func TestNative_XlsxBlockIDs(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "pokemon.xlsx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "XLSX block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

func TestNative_XlsxMultiLayer(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "pokemon.xlsx"))

	var layerStartCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerStartCount++
		}
	}
	assert.Greater(t, layerStartCount, 1, "XLSX should produce multiple layers")
}

func TestNative_XlsxInlineStrings(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "1199-inline-strings.xlsx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "XLSX with inline strings should produce blocks")
}

func TestNative_XlsxEmptyCells(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "894-empty-cells-and-rows.xlsx"))

	require.NotEmpty(t, parts, "XLSX with empty cells should produce parts")
}

func TestNative_XlsxSmartArt(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "smartart.xlsx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "XLSX with SmartArt should produce blocks")
}

func TestNative_XlsxSharedStrings(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "972-shared-strings-and-comments.xlsx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "XLSX with shared strings should produce blocks")
}

// --- PPTX tests ---

func TestNative_SimplePptx(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "794.pptx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX should produce translatable blocks")
}

func TestNative_PptxBlockIDs(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "794.pptx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "PPTX block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

func TestNative_PptxMultiLayer(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "794.pptx"))

	var layerStartCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerStartCount++
		}
	}
	assert.Greater(t, layerStartCount, 1, "PPTX should produce multiple layers")
}

func TestNative_PptxLineBreak(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "1421-line-break.pptx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with line breaks should produce blocks")
}

func TestNative_PptxSmartArt(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "SmartArt.pptx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with SmartArt should produce blocks")
}

func TestNative_PptxFormattings(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "1009-1.pptx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with formattings should produce blocks")
}

func TestNative_PptxSlideLayouts(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "slideLayouts.pptx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with slide layouts should produce blocks")

	var layerCount int
	for _, p := range parts {
		if p.Type == model.PartLayerStart {
			layerCount++
		}
	}
	assert.Greater(t, layerCount, 1, "PPTX with slide layouts should produce multiple layers")
}

func TestNative_PptxComments(t *testing.T) {
	dir := testdataDir(t)
	path := filepath.Join(dir, "Comments.pptx")
	f, err := os.Open(path)
	require.NoError(t, err)

	reader := NewReader()
	reader.cfg.TranslateComments = true
	doc := testutil.RawDocFromReader(f, path, model.LocaleEnglish)
	err = reader.Open(context.Background(), doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(context.Background()))
	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with comments should produce blocks")
}

func TestNative_PptxHiddenSlides(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "1010-slide1-hidden-slide2-hidden.pptx"))

	require.NotEmpty(t, parts, "PPTX with hidden slides should produce parts")
}

func TestNative_PptxCharts(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "1046.pptx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with charts should produce blocks")
}

func TestNative_PptxFormattedHyperlink(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "FormattedHyperlink.pptx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "PPTX with formatted hyperlinks should produce blocks")
}

// --- Cross-format tests ---

func TestNative_AllFormatsLayerBalance(t *testing.T) {
	dir := testdataDir(t)

	tests := []struct {
		name string
		file string
	}{
		{"docx", filepath.Join(dir, "948-1.docx")},
		{"xlsx", filepath.Join(dir, "pokemon.xlsx")},
		{"pptx", filepath.Join(dir, "794.pptx")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := readFile(t, tc.file)

			var starts, ends int
			for _, p := range parts {
				if p.Type == model.PartLayerStart {
					starts++
				}
				if p.Type == model.PartLayerEnd {
					ends++
				}
			}
			assert.Equal(t, starts, ends, "layer starts and ends should be balanced")
		})
	}
}

func TestNative_PartSequenceIntegrity(t *testing.T) {
	dir := testdataDir(t)

	tests := []struct {
		name string
		file string
	}{
		{"docx", filepath.Join(dir, "948-1.docx")},
		{"xlsx", filepath.Join(dir, "pokemon.xlsx")},
		{"pptx", filepath.Join(dir, "794.pptx")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := readFile(t, tc.file)
			require.NotEmpty(t, parts)

			assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")

			for i, p := range parts {
				assert.NotNil(t, p.Resource, "part[%d] resource should not be nil", i)
			}
		})
	}
}

// --- Bulk DOCX extraction test ---

func TestNative_BulkDocxExtraction(t *testing.T) {
	dir := testdataDir(t)

	files, err := filepath.Glob(filepath.Join(dir, "*.docx"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "should find DOCX test files")

	for _, file := range files {
		name := filepath.Base(file)
		t.Run(name, func(t *testing.T) {
			parts := readFile(t, file)
			require.NotEmpty(t, parts, "%s should produce parts", name)

			// Check layer balance
			var starts, ends int
			for _, p := range parts {
				if p.Type == model.PartLayerStart {
					starts++
				}
				if p.Type == model.PartLayerEnd {
					ends++
				}
			}
			assert.Equal(t, starts, ends, "%s: layers unbalanced", name)

			// First and last should be layers
			assert.Equal(t, model.PartLayerStart, parts[0].Type, "%s: first part should be LayerStart", name)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "%s: last part should be LayerEnd", name)
		})
	}
}

// --- Bulk XLSX extraction test ---

func TestNative_BulkXlsxExtraction(t *testing.T) {
	dir := testdataDir(t)

	files, err := filepath.Glob(filepath.Join(dir, "*.xlsx"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "should find XLSX test files")

	for _, file := range files {
		name := filepath.Base(file)
		t.Run(name, func(t *testing.T) {
			parts := readFile(t, file)
			require.NotEmpty(t, parts, "%s should produce parts", name)

			var starts, ends int
			for _, p := range parts {
				if p.Type == model.PartLayerStart {
					starts++
				}
				if p.Type == model.PartLayerEnd {
					ends++
				}
			}
			assert.Equal(t, starts, ends, "%s: layers unbalanced", name)
			assert.Equal(t, model.PartLayerStart, parts[0].Type, "%s: first part should be LayerStart", name)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "%s: last part should be LayerEnd", name)
		})
	}
}

// --- Bulk PPTX extraction test ---

func TestNative_BulkPptxExtraction(t *testing.T) {
	dir := testdataDir(t)

	files, err := filepath.Glob(filepath.Join(dir, "*.pptx"))
	require.NoError(t, err)
	require.NotEmpty(t, files, "should find PPTX test files")

	for _, file := range files {
		name := filepath.Base(file)
		t.Run(name, func(t *testing.T) {
			parts := readFile(t, file)
			require.NotEmpty(t, parts, "%s should produce parts", name)

			var starts, ends int
			for _, p := range parts {
				if p.Type == model.PartLayerStart {
					starts++
				}
				if p.Type == model.PartLayerEnd {
					ends++
				}
			}
			assert.Equal(t, starts, ends, "%s: layers unbalanced", name)
			assert.Equal(t, model.PartLayerStart, parts[0].Type, "%s: first part should be LayerStart", name)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "%s: last part should be LayerEnd", name)
		})
	}
}

// --- Formatting preservation tests ---

func TestNative_FormattingPreservation(t *testing.T) {
	dir := testdataDir(t)

	tests := []struct {
		name string
		file string
	}{
		{"docx-formatting", filepath.Join(dir, "948-1.docx")},
		{"pptx-formatting", filepath.Join(dir, "1009-1.pptx")},
		{"xlsx-formatting", filepath.Join(dir, "pokemon.xlsx")},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			parts := readFile(t, tc.file)
			blocks := translatableBlocks(parts)
			require.NotEmpty(t, blocks)
		})
	}
}

// --- DOCX InlineCodes test ---

func TestNative_DocxInlineCodes(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "948-1.docx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	var withSpans int
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil && len(frag.Spans) > 0 {
			withSpans++
		}
	}
	if withSpans > 0 {
		t.Logf("found %d blocks with inline codes", withSpans)
	}
}

// --- DOCX SegmentIDs test ---

func TestNative_DocxSegmentIDs(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "948-1.docx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		require.NotEmpty(t, b.Source, "block should have source segments")
		for _, seg := range b.Source {
			assert.NotEmpty(t, seg.ID, "segment should have an ID")
			assert.NotNil(t, seg.Content, "segment should have content")
		}
	}
}

// --- PPTX line breaks as tags ---

func TestNative_PptxLineBreaksAsTags(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "1421-line-break.pptx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	hasSpans := false
	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil && len(frag.Spans) > 0 {
			hasSpans = true
			break
		}
	}
	assert.True(t, hasSpans, "line breaks should be represented as inline codes")
}

// --- XLSX cross-sheet references ---

func TestNative_XlsxCrossSheetReferences(t *testing.T) {
	dir := testdataDir(t)

	tests := []struct {
		name string
		file string
	}{
		{"cross-sheets", "1051-cross-sheets-references.xlsx"},
		{"table-refs", "1051-cross-sheets-table-references.xlsx"},
		{"table-refs-2", "1051-cross-sheets-table-references-2.xlsx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.file)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Skipf("test file not found: %s", tc.file)
			}
			parts := readFile(t, path)
			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
		})
	}
}

// --- Known limitation DOCX extraction ---

func TestNative_KnownLimitationDocx(t *testing.T) {
	dir := testdataDir(t)

	files := []struct {
		name       string
		limitation string
	}{
		{"1102.docx", "structural Data dropped in complex revision markup"},
		{"830-3.docx", "structural Data added between consecutive blocks"},
		{"847-2.docx", "tracked changes cause Data part drop"},
		{"847-3.docx", "tracked changes cause Data part drop"},
		{"956.docx", "complex structure causes Data part drop"},
	}

	for _, tt := range files {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Skipf("test file not found: %s", tt.name)
			}
			parts := readFile(t, path)
			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

			blocks := translatableBlocks(parts)
			assert.NotEmpty(t, blocks,
				"%s should extract translatable blocks (limitation: %s)", tt.name, tt.limitation)
		})
	}
}

// --- Known limitation PPTX extraction ---

func TestNative_KnownLimitationPptx(t *testing.T) {
	dir := testdataDir(t)

	files := []struct {
		name       string
		limitation string
	}{
		{"1329-styles-clarification.pptx", "PPTX theme-based style inheritance collapse"},
		{"1435-text-for-masking.pptx", "font stack truncation during roundtrip"},
	}

	for _, tt := range files {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(dir, tt.name)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Skipf("test file not found: %s", tt.name)
			}
			parts := readFile(t, path)
			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

			blocks := translatableBlocks(parts)
			assert.NotEmpty(t, blocks,
				"%s should extract translatable blocks (limitation: %s)", tt.name, tt.limitation)
		})
	}
}

// --- PPTX visible/hidden slides ---

func TestNative_PptxVisibleHiddenSlides(t *testing.T) {
	dir := testdataDir(t)

	tests := []struct {
		name string
		file string
	}{
		{"visible-hidden", "1010-slide1-visible-slide2-hidden.pptx"},
		{"both-hidden", "1010-slide1-hidden-slide2-hidden.pptx"},
		{"visible-hidden-2", "1011-slide1-visible-slide2-hidden.pptx"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(dir, tc.file)
			if _, err := os.Stat(path); os.IsNotExist(err) {
				t.Skipf("test file not found: %s", tc.file)
			}
			parts := readFile(t, path)
			require.NotEmpty(t, parts)
			assert.Equal(t, model.PartLayerStart, parts[0].Type)
			assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
		})
	}
}

// --- Tabs ---

func TestNative_TabAsCharVariants(t *testing.T) {
	dir := testdataDir(t)
	parts := readFile(t, filepath.Join(dir, "Document-with-tabs.docx"))

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks, "tab-as-char document should produce blocks")
}

