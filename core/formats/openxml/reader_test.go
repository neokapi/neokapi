package openxml

import (
	"context"
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func openTestDocx(t *testing.T, path string) *Reader {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)

	reader := NewReader()
	doc := testutil.RawDocFromReader(f, path, model.LocaleEnglish)
	err = reader.Open(context.Background(), doc)
	require.NoError(t, err)
	return reader
}

func readDocx(t *testing.T, path string) []*model.Part {
	t.Helper()
	reader := openTestDocx(t, path)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(context.Background()))
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

// --- Basic Reader Tests ---

func TestReadSimpleDocx(t *testing.T) {
	parts := readDocx(t, "testdata/simple.docx")

	// Should have layer start, blocks, layer end (nested)
	require.True(t, len(parts) >= 3, "expected at least 3 parts, got %d", len(parts))

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

	// First block: plain text, no spans
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.False(t, frag.HasSpans(), "simple text should have no spans")
	assert.Equal(t, "Hello, World!", frag.CodedText)

	// Second block: bold text + normal text → should have spans
	frag = blocks[1].FirstFragment()
	require.NotNil(t, frag)
	assert.True(t, frag.HasSpans(), "bold+normal should have spans")
	assert.Equal(t, "Bold text and normal text", frag.Text())

	// Verify bold spans exist
	hasBold := false
	for _, s := range frag.Spans {
		if s.Type == TypeBold {
			hasBold = true
			break
		}
	}
	assert.True(t, hasBold, "should have bold span")

	// Third block: italic, normal, bold+italic
	frag = blocks[2].FirstFragment()
	require.NotNil(t, frag)
	assert.True(t, frag.HasSpans())
	assert.Equal(t, "Italic then bold italic", frag.Text())

	hasItalic := false
	for _, s := range frag.Spans {
		if s.Type == TypeItalic {
			hasItalic = true
			break
		}
	}
	assert.True(t, hasItalic, "should have italic span")
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

	frag := found.FirstFragment()
	require.NotNil(t, frag)

	hasBold := false
	hasUnderline := false
	for _, s := range frag.Spans {
		if s.Type == TypeBold {
			hasBold = true
		}
		if s.Type == TypeUnderline {
			hasUnderline = true
		}
	}
	assert.True(t, hasBold, "should have bold span")
	assert.True(t, hasUnderline, "should have underline span")
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

	frag := found.FirstFragment()
	require.NotNil(t, frag)
	assert.True(t, frag.HasSpans())

	hasHyperlink := false
	for _, s := range frag.Spans {
		if s.Type == TypeHyperlink {
			hasHyperlink = true
			break
		}
	}
	assert.True(t, hasHyperlink, "should have hyperlink span")
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

	frag := found.FirstFragment()
	require.NotNil(t, frag)

	hasBreak := false
	for _, s := range frag.Spans {
		if s.Type == TypeBreak {
			hasBreak = true
			break
		}
	}
	assert.True(t, hasBreak, "should have break span")
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

	frag := found.FirstFragment()
	require.NotNil(t, frag)

	hasStrike := false
	hasSuper := false
	for _, s := range frag.Spans {
		if s.Type == TypeStrikethrough {
			hasStrike = true
		}
		if s.Type == TypeSuperscript {
			hasSuper = true
		}
	}
	assert.True(t, hasStrike, "should have strikethrough span")
	assert.True(t, hasSuper, "should have superscript span")
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
	err = reader.Open(context.Background(), doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(context.Background()))
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
	assert.True(t, layerStarts >= 2, "should have at least root + one XML part layer")
}

func TestReaderNilDocument(t *testing.T) {
	reader := NewReader()
	err := reader.Open(context.Background(), nil)
	assert.Error(t, err)
}

func TestReaderConfig(t *testing.T) {
	reader := NewReader()
	cfg := reader.Config()
	assert.Equal(t, "openxml", cfg.FormatName())

	err := cfg.ApplyMap(map[string]any{
		"translateDocProperties": false,
		"aggressiveCleanup":     false,
	})
	assert.NoError(t, err)

	oxCfg := cfg.(*Config)
	assert.False(t, oxCfg.TranslateDocProperties)
	assert.False(t, oxCfg.AggressiveCleanup)
}

func TestReaderConfigListParams(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()

	err := cfg.ApplyMap(map[string]any{
		"excludeColors":          []any{"FF0000", "00FF00"},
		"excludeStyles":          []string{"CodeBlock", "Quote"},
		"excludedColumns":        []any{"A", "C"},
		"excludedSheets":         []any{"Sheet2"},
		"includedSlides":         []any{float64(1), float64(3)},
		"lineSeparatorReplacement": "\\n",
		"replaceLineSeparator":   true,
		"translateCharts":        true,
		"translateDiagrams":      true,
		"translateHiddenSlides":  true,
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
		"aggressiveCleanup":      false,
		"tabAsCharacter":         true,
		"translateSlideNotes":    false,
		"translateSlideMasters":  true,
		"translateHiddenSlides":  true,
		"translateCharts":        true,
		"translateDiagrams":      true,
		"translateSheetNames":    true,
		"translateSharedStrings": false,
		"replaceLineSeparator":   true,
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
		"aggressiveCleanup":     "false",
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
	assert.Error(t, err)
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
	assert.NoError(t, cfg.Validate())
}

func TestReaderConfigDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.Reset()

	assert.True(t, cfg.TranslateDocProperties)
	assert.False(t, cfg.TranslateHiddenText)
	assert.True(t, cfg.TranslateHeadersFooters)
	assert.True(t, cfg.TranslateFootnotes)
	assert.False(t, cfg.TranslateComments)
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
