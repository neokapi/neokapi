package openxml

import (
	"archive/zip"
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDocx(t *testing.T, path string) *Reader {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)

	reader := NewReader()
	doc := testutil.RawDocFromReader(f, path, model.LocaleEnglish)
	err = reader.Open(t.Context(), doc)
	require.NoError(t, err)
	return reader
}

func readDocx(t *testing.T, path string) []*model.Part {
	t.Helper()
	reader := openTestDocx(t, path)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(t.Context()))
}

func translatableBlocks(parts []*model.Part) []*model.Block {
	var blocks []*model.Block
	for _, p := range parts {
		if p.Type == model.PartBlock {
			if b, ok := p.Resource.(*model.Block); ok && b.Translatable {
				blocks = append(blocks, b)
			}
		}
	}
	return blocks
}

func blockTexts(blocks []*model.Block) []string {
	texts := make([]string, len(blocks))
	for i, b := range blocks {
		texts[i] = b.SourceText()
	}
	return texts
}

func hasInlineCodeRun(runs []model.Run) bool {
	for _, r := range runs {
		if r.Text == nil {
			return true
		}
	}
	return false
}

func runHasType(r model.Run, codeType string) bool {
	switch {
	case r.PcOpen != nil:
		return r.PcOpen.Type == codeType
	case r.PcClose != nil:
		return r.PcClose.Type == codeType
	case r.Ph != nil:
		return r.Ph.Type == codeType
	}
	return false
}

// okapi-filter: openxml

// --- Basic Reader Tests ---

func TestReadSimpleDocx(t *testing.T) {
	parts := readDocx(t, "testdata/simple.docx")

	// Should have layer start, blocks, layer end (nested)
	require.GreaterOrEqual(t, len(parts), 3, "expected at least 3 parts, got %d", len(parts))

	// First part should be root layer start
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	rootLayer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "openxml", rootLayer.Format)
	assert.Equal(t, "doc1", rootLayer.ID)

	// Last part should be root layer end
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	// Extract blocks
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

	assert.Contains(t, texts, "Hello, World!")
}

func TestReadSimpleDocxBlocks(t *testing.T) {
	parts := readDocx(t, "testdata/simple.docx")
	blocks := translatableBlocks(parts)

	require.Len(t, blocks, 3, "expected 3 paragraphs")

	texts := blockTexts(blocks)
	assert.Equal(t, "Hello, World!", texts[0])
	assert.Equal(t, "Bold text and normal text", texts[1])
	assert.Equal(t, "Italic then bold italic", texts[2])
}

func TestReadInlineFormatting(t *testing.T) {
	parts := readDocx(t, "testdata/simple.docx")
	blocks := translatableBlocks(parts)

	require.Len(t, blocks, 3)

	// First block: plain text, no inline-code runs
	runs := blocks[0].SourceRuns()
	assert.False(t, hasInlineCodeRun(runs), "simple text should have no inline-code runs")
	assert.Equal(t, "Hello, World!", model.RunsText(runs))

	// Second block: bold text + normal text → should have inline-code runs
	runs = blocks[1].SourceRuns()
	assert.True(t, hasInlineCodeRun(runs), "bold+normal should have inline-code runs")
	assert.Equal(t, "Bold text and normal text", model.RunsText(runs))

	// Verify bold inline-code runs exist
	hasBold := false
	for _, r := range runs {
		if runHasType(r, TypeBold) {
			hasBold = true
			break
		}
	}
	assert.True(t, hasBold, "should have bold inline-code run")

	// Third block: italic, normal, bold+italic
	runs = blocks[2].SourceRuns()
	assert.True(t, hasInlineCodeRun(runs))
	assert.Equal(t, "Italic then bold italic", model.RunsText(runs))

	hasItalic := false
	for _, r := range runs {
		if runHasType(r, TypeItalic) {
			hasItalic = true
			break
		}
	}
	assert.True(t, hasItalic, "should have italic inline-code run")
}

func TestReadFormattedDocx(t *testing.T) {
	parts := readDocx(t, "testdata/formatted.docx")
	blocks := translatableBlocks(parts)

	texts := blockTexts(blocks)
	t.Logf("Block texts: %v", texts)

	// Should contain these paragraphs (some from header)
	assert.Contains(t, texts, "Simple paragraph")
	assert.Contains(t, texts, "A Heading")
}

func TestReadBoldUnderline(t *testing.T) {
	parts := readDocx(t, "testdata/formatted.docx")
	blocks := translatableBlocks(parts)

	// Find the "Bold underlined" block
	var found *model.Block
	for _, b := range blocks {
		if b.SourceText() == "Bold underlined" {
			found = b
			break
		}
	}
	require.NotNil(t, found, "should find 'Bold underlined' block")

	runs := found.SourceRuns()

	hasBold := false
	hasUnderline := false
	for _, r := range runs {
		if runHasType(r, TypeBold) {
			hasBold = true
		}
		if runHasType(r, TypeUnderline) {
			hasUnderline = true
		}
	}
	assert.True(t, hasBold, "should have bold inline-code run")
	assert.True(t, hasUnderline, "should have underline inline-code run")
}

func TestReadHyperlink(t *testing.T) {
	parts := readDocx(t, "testdata/formatted.docx")
	blocks := translatableBlocks(parts)

	// Find the block with hyperlink
	var found *model.Block
	for _, b := range blocks {
		text := b.SourceText()
		if text == "Click here for more." {
			found = b
			break
		}
	}
	require.NotNil(t, found, "should find hyperlink block")

	runs := found.SourceRuns()
	assert.True(t, hasInlineCodeRun(runs))

	hasHyperlink := false
	for _, r := range runs {
		if runHasType(r, TypeHyperlink) {
			hasHyperlink = true
			break
		}
	}
	assert.True(t, hasHyperlink, "should have hyperlink inline-code run")
}

func TestReadLineBreak(t *testing.T) {
	parts := readDocx(t, "testdata/formatted.docx")
	blocks := translatableBlocks(parts)

	// Find the block with a line break
	var found *model.Block
	for _, b := range blocks {
		text := b.SourceText()
		if text == "Line oneLine two" {
			found = b
			break
		}
	}
	require.NotNil(t, found, "should find line break block")

	runs := found.SourceRuns()

	hasBreak := false
	for _, r := range runs {
		if runHasType(r, TypeBreak) {
			hasBreak = true
			break
		}
	}
	assert.True(t, hasBreak, "should have break inline-code run")
}

func TestReadStrikethroughAndSuperscript(t *testing.T) {
	parts := readDocx(t, "testdata/formatted.docx")
	blocks := translatableBlocks(parts)

	var found *model.Block
	for _, b := range blocks {
		if b.SourceText() == "Deleted text and super" {
			found = b
			break
		}
	}
	require.NotNil(t, found, "should find strikethrough block")

	runs := found.SourceRuns()

	hasStrike := false
	hasSuper := false
	for _, r := range runs {
		if runHasType(r, TypeStrikethrough) {
			hasStrike = true
		}
		if runHasType(r, TypeSuperscript) {
			hasSuper = true
		}
	}
	assert.True(t, hasStrike, "should have strikethrough inline-code run")
	assert.True(t, hasSuper, "should have superscript inline-code run")
}

func TestReadHiddenTextSkipped(t *testing.T) {
	parts := readDocx(t, "testdata/formatted.docx")
	blocks := translatableBlocks(parts)

	for _, b := range blocks {
		assert.NotEqual(t, "Hidden text", b.SourceText(), "hidden text should be skipped by default")
	}
}

func TestReadHeadersFooters(t *testing.T) {
	parts := readDocx(t, "testdata/formatted.docx")
	blocks := translatableBlocks(parts)

	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Header Text", "should extract header text")
}

func TestReadHeadersFootersDisabled(t *testing.T) {
	f, err := os.Open("testdata/formatted.docx")
	require.NoError(t, err)

	reader := NewReader()
	reader.cfg.TranslateHeadersFooters = false
	doc := testutil.RawDocFromReader(f, "testdata/formatted.docx", model.LocaleEnglish)
	err = reader.Open(t.Context(), doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	blocks := translatableBlocks(parts)
	texts := blockTexts(blocks)

	for _, text := range texts {
		assert.NotEqual(t, "Header Text", text, "header text should not be extracted when disabled")
	}
}

func TestReaderSignature(t *testing.T) {
	reader := NewReader()
	sig := reader.Signature()

	assert.Contains(t, sig.Extensions, ".docx")
	assert.Contains(t, sig.Extensions, ".pptx")
	assert.Contains(t, sig.Extensions, ".xlsx")
	assert.Equal(t, [][]byte{{0x50, 0x4B, 0x03, 0x04}}, sig.MagicBytes)
}

func TestReaderLayerStructure(t *testing.T) {
	parts := readDocx(t, "testdata/simple.docx")

	// Count layer starts and ends
	var layerStarts, layerEnds int
	for _, p := range parts {
		switch p.Type {
		case model.PartLayerStart:
			layerStarts++
		case model.PartLayerEnd:
			layerEnds++
		}
	}

	assert.Equal(t, layerStarts, layerEnds, "layer starts should match layer ends")
	assert.GreaterOrEqual(t, layerStarts, 2, "should have at least root + one XML part layer")
}

func TestReaderNilDocument(t *testing.T) {
	reader := NewReader()
	err := reader.Open(t.Context(), nil)
	require.Error(t, err)
}

func TestReaderConfig(t *testing.T) {
	reader := NewReader()
	cfg := reader.Config()
	assert.Equal(t, "openxml", cfg.FormatName())

	err := cfg.ApplyMap(map[string]any{
		"translateDocProperties": false,
		"aggressiveCleanup":      false,
	})
	require.NoError(t, err)

	oxCfg := cfg.(*Config)
	assert.False(t, oxCfg.TranslateDocProperties)
	assert.False(t, oxCfg.AggressiveCleanup)
}

func TestReaderConfigListParams(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()

	err := cfg.ApplyMap(map[string]any{
		"excludeColors":            []any{"FF0000", "00FF00"},
		"excludeStyles":            []string{"CodeBlock", "Quote"},
		"excludedColumns":          []any{"A", "C"},
		"excludedSheets":           []any{"Sheet2"},
		"includedSlides":           []any{float64(1), float64(3)},
		"lineSeparatorReplacement": "\\n",
		"replaceLineSeparator":     true,
		"translateCharts":          true,
		"translateDiagrams":        true,
		"translateHiddenSlides":    true,
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"FF0000", "00FF00"}, cfg.ExcludeColors)
	assert.Equal(t, []string{"CodeBlock", "Quote"}, cfg.ExcludeStyles)
	assert.Equal(t, []string{"A", "C"}, cfg.ExcludedColumns)
	assert.Equal(t, []string{"Sheet2"}, cfg.ExcludedSheets)
	assert.Equal(t, []int{1, 3}, cfg.IncludedSlides)
	assert.Equal(t, "\\n", cfg.LineSeparatorReplacement)
	assert.True(t, cfg.ReplaceLineSeparator)
	assert.True(t, cfg.TranslateCharts)
	assert.True(t, cfg.TranslateDiagrams)
	assert.True(t, cfg.TranslateHiddenSlides)
}

func TestReaderConfigAllBoolKeys(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()

	// Set every boolean to the opposite of its default
	err := cfg.ApplyMap(map[string]any{
		"translateDocProperties":  false,
		"translateHiddenText":     true,
		"translateHeadersFooters": false,
		"translateFootnotes":      false,
		"translateComments":       true,
		"translateHyperlinks":     false,
		"aggressiveCleanup":       false,
		"tabAsCharacter":          true,
		"translateSlideNotes":     false,
		"translateSlideMasters":   true,
		"translateHiddenSlides":   true,
		"translateCharts":         true,
		"translateDiagrams":       true,
		"translateSheetNames":     true,
		"translateSharedStrings":  false,
		"replaceLineSeparator":    true,
	})
	require.NoError(t, err)

	assert.False(t, cfg.TranslateDocProperties)
	assert.True(t, cfg.TranslateHiddenText)
	assert.False(t, cfg.TranslateHeadersFooters)
	assert.False(t, cfg.TranslateFootnotes)
	assert.True(t, cfg.TranslateComments)
	assert.False(t, cfg.TranslateHyperlinks)
	assert.False(t, cfg.AggressiveCleanup)
	assert.True(t, cfg.TabAsCharacter)
	assert.False(t, cfg.TranslateSlideNotes)
	assert.True(t, cfg.TranslateSlideMasters)
	assert.True(t, cfg.TranslateHiddenSlides)
	assert.True(t, cfg.TranslateCharts)
	assert.True(t, cfg.TranslateDiagrams)
	assert.True(t, cfg.TranslateSheetNames)
	assert.False(t, cfg.TranslateSharedStrings)
	assert.True(t, cfg.ReplaceLineSeparator)
}

func TestReaderConfigAllListKeys(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()

	err := cfg.ApplyMap(map[string]any{
		"excludeColors":          []any{"FF0000"},
		"excludeHighlightColors": []any{"yellow", "red"},
		"includeHighlightColors": []any{"green"},
		"excludeStyles":          []any{"CodeBlock"},
		"includeStyles":          []any{"Normal", "Heading1"},
		"excludedSheets":         []any{"Sheet2", "Hidden"},
		"excludedColumns":        []any{"A", "C", "AA"},
		"includedSlides":         []any{float64(1), float64(5)},
	})
	require.NoError(t, err)

	assert.Equal(t, []string{"FF0000"}, cfg.ExcludeColors)
	assert.Equal(t, []string{"yellow", "red"}, cfg.ExcludeHighlightColors)
	assert.Equal(t, []string{"green"}, cfg.IncludeHighlightColors)
	assert.Equal(t, []string{"CodeBlock"}, cfg.ExcludeStyles)
	assert.Equal(t, []string{"Normal", "Heading1"}, cfg.IncludeStyles)
	assert.Equal(t, []string{"Sheet2", "Hidden"}, cfg.ExcludedSheets)
	assert.Equal(t, []string{"A", "C", "AA"}, cfg.ExcludedColumns)
	assert.Equal(t, []int{1, 5}, cfg.IncludedSlides)
}

func TestReaderConfigToBoolStringValues(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()

	err := cfg.ApplyMap(map[string]any{
		"translateDocProperties": "true",
		"translateHiddenText":    "yes",
		"translateComments":      "1",
		"aggressiveCleanup":      "false",
	})
	require.NoError(t, err)

	assert.True(t, cfg.TranslateDocProperties)
	assert.True(t, cfg.TranslateHiddenText)
	assert.True(t, cfg.TranslateComments)
	assert.False(t, cfg.AggressiveCleanup)
}

func TestReaderConfigNilListValues(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()

	err := cfg.ApplyMap(map[string]any{
		"excludeColors":  nil,
		"includedSlides": nil,
	})
	require.NoError(t, err)
	assert.Nil(t, cfg.ExcludeColors)
	assert.Nil(t, cfg.IncludedSlides)
}

func TestReaderConfigDirectSliceTypes(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()

	// Test direct []string and []int paths (not []any)
	err := cfg.ApplyMap(map[string]any{
		"excludeColors":  []string{"AABBCC"},
		"includedSlides": []int{2, 4, 6},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"AABBCC"}, cfg.ExcludeColors)
	assert.Equal(t, []int{2, 4, 6}, cfg.IncludedSlides)
}

func TestReaderConfigUnknownKey(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()
	err := cfg.ApplyMap(map[string]any{"unknownKey": true})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown config key")
}

func TestReaderConfigInvalidTypes(t *testing.T) {
	tests := []struct {
		name string
		key  string
		val  any
		msg  string
	}{
		{"string list gets int", "excludeColors", 42, "string list"},
		{"string list item not string", "excludeStyles", []any{42}, "string"},
		{"int list gets string", "includedSlides", "nope", "int list"},
		{"int list item not int", "includedSlides", []any{"bad"}, "int"},
		{"string key gets int", "lineSeparatorReplacement", 42, "string"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := &Config{}
			cfg.Reset()
			err := cfg.ApplyMap(map[string]any{tt.key: tt.val})
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.msg)
		})
	}
}

func TestReaderConfigValidate(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()
	require.NoError(t, cfg.Validate())
}

// okapi: OpenXMLConfigurationTest#defaultConfiguration
func TestReaderConfigDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()

	assert.True(t, cfg.TranslateDocProperties)
	assert.False(t, cfg.TranslateHiddenText)
	assert.True(t, cfg.TranslateHeadersFooters)
	assert.True(t, cfg.TranslateFootnotes)
	// Mirrors okapi ConditionalParameters.reset() line 781:
	//   setTranslateComments(true); // Word, Excel Comments
	assert.True(t, cfg.TranslateComments)
	assert.True(t, cfg.TranslateHyperlinks)
	assert.True(t, cfg.AggressiveCleanup)
	assert.False(t, cfg.TabAsCharacter)
	assert.True(t, cfg.TranslateSlideNotes)
	assert.False(t, cfg.TranslateSlideMasters)
	assert.False(t, cfg.TranslateHiddenSlides)
	assert.False(t, cfg.TranslateCharts)
	assert.False(t, cfg.TranslateDiagrams)
	assert.Nil(t, cfg.IncludedSlides)
	assert.False(t, cfg.TranslateSheetNames)
	assert.True(t, cfg.TranslateSharedStrings)
	assert.Nil(t, cfg.ExcludedSheets)
	assert.Nil(t, cfg.ExcludedColumns)
	assert.Nil(t, cfg.ExcludeColors)
	assert.Nil(t, cfg.ExcludeHighlightColors)
	assert.Nil(t, cfg.IncludeHighlightColors)
	assert.Nil(t, cfg.ExcludeStyles)
	assert.Nil(t, cfg.IncludeStyles)
	assert.False(t, cfg.ReplaceLineSeparator)
	assert.Equal(t, "\n", cfg.LineSeparatorReplacement)
}

func TestExtractMediaConfig(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()
	assert.False(t, cfg.ExtractMedia)

	err := cfg.ApplyMap(map[string]any{"extractMedia": true})
	require.NoError(t, err)
	assert.True(t, cfg.ExtractMedia)
}

func TestExtractMediaFromDocx(t *testing.T) {
	// Build a minimal DOCX ZIP with an embedded PNG in word/media/.
	docx := buildDocxWithMedia(t)

	reader := NewReader()
	require.NoError(t, reader.Config().ApplyMap(map[string]any{"extractMedia": true}))

	doc := testutil.RawDocFromReader(docx, "test.docx", model.LocaleEnglish)
	require.NoError(t, reader.Open(t.Context(), doc))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(t.Context()))

	// Find PartMedia parts.
	var mediaParts []*model.Media
	for _, p := range parts {
		if p.Type == model.PartMedia {
			if m, ok := p.Resource.(*model.Media); ok {
				mediaParts = append(mediaParts, m)
			}
		}
	}

	require.Len(t, mediaParts, 1, "should extract one embedded image")
	m := mediaParts[0]
	assert.Equal(t, "image/png", m.MimeType)
	assert.Equal(t, "test.png", m.Filename)
	assert.NotEmpty(t, m.BlobKey, "should compute SHA-256 blob key")
	assert.Equal(t, int64(len(testPNG)), m.Size)
	assert.Equal(t, "word/media/test.png", m.Properties["zipPath"])
}

func TestExtractMediaDisabled(t *testing.T) {
	docx := buildDocxWithMedia(t)

	reader := NewReader()
	// ExtractMedia defaults to false.
	doc := testutil.RawDocFromReader(docx, "test.docx", model.LocaleEnglish)
	require.NoError(t, reader.Open(t.Context(), doc))
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(t.Context()))

	for _, p := range parts {
		assert.NotEqual(t, model.PartMedia, p.Type, "should not emit PartMedia when ExtractMedia is false")
	}
}

// testPNG is a minimal 1x1 red PNG (67 bytes).
var testPNG = []byte{
	0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A, // PNG signature
	0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
	0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01, // 1x1
	0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53, // 8-bit RGB
	0xDE, 0x00, 0x00, 0x00, 0x0C, 0x49, 0x44, 0x41, // IDAT chunk
	0x54, 0x08, 0xD7, 0x63, 0xF8, 0xCF, 0xC0, 0x00,
	0x00, 0x00, 0x02, 0x00, 0x01, 0xE2, 0x21, 0xBC,
	0x33, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, // IEND chunk
	0x44, 0xAE, 0x42, 0x60, 0x82,
}

// buildDocxWithMedia constructs a minimal DOCX ZIP with a test PNG in word/media/.
func buildDocxWithMedia(t *testing.T) *os.File {
	t.Helper()

	tmp, err := os.CreateTemp(t.TempDir(), "test-*.docx")
	require.NoError(t, err)

	zw := zip.NewWriter(tmp)

	// [Content_Types].xml
	writeZipEntry(t, zw, "[Content_Types].xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">
  <Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>
  <Default Extension="xml" ContentType="application/xml"/>
  <Default Extension="png" ContentType="image/png"/>
  <Override PartName="/word/document.xml" ContentType="application/vnd.openxmlformats-officedocument.wordprocessingml.document.main+xml"/>
</Types>`)

	// _rels/.rels
	writeZipEntry(t, zw, "_rels/.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="word/document.xml"/>
</Relationships>`)

	// word/_rels/document.xml.rels
	writeZipEntry(t, zw, "word/_rels/document.xml.rels", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">
  <Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/image" Target="media/test.png"/>
</Relationships>`)

	// word/document.xml — simple paragraph with text
	writeZipEntry(t, zw, "word/document.xml", `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<w:document xmlns:w="http://schemas.openxmlformats.org/wordprocessingml/2006/main">
  <w:body>
    <w:p><w:r><w:t>Hello World</w:t></w:r></w:p>
  </w:body>
</w:document>`)

	// word/media/test.png — embedded image
	w, err := zw.Create("word/media/test.png")
	require.NoError(t, err)
	_, err = w.Write(testPNG)
	require.NoError(t, err)

	require.NoError(t, zw.Close())

	// Reopen for reading.
	name := tmp.Name()
	tmp.Close()
	f, err := os.Open(name)
	require.NoError(t, err)
	t.Cleanup(func() { f.Close() })
	return f
}

func writeZipEntry(t *testing.T, zw *zip.Writer, name, content string) {
	t.Helper()
	w, err := zw.Create(name)
	require.NoError(t, err)
	_, err = w.Write([]byte(content))
	require.NoError(t, err)
}
