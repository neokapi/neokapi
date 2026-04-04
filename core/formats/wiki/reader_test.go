package wiki_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"sync"
	"testing"

	"github.com/neokapi/neokapi/core/formats/wiki"
	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// helper: read a string with default (MediaWiki) config and return parts
func readString(t *testing.T, reader *wiki.Reader, content string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	err := reader.Open(ctx, testutil.RawDocFromString(content, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()
	return parts
}

// helper: read a string with default MediaWiki reader
func readDefault(t *testing.T, content string) []*model.Part {
	t.Helper()
	return readString(t, wiki.NewReader(), content)
}

// helper: create a DokuWiki reader
func newDokuWikiReader(t *testing.T) *wiki.Reader {
	t.Helper()
	reader := wiki.NewReader()
	cfg := reader.Config().(*wiki.Config)
	cfg.Variant = wiki.VariantDokuWiki
	return reader
}

// helper: roundtrip a snippet
func roundtrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := wiki.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := wiki.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)
	err = writer.Write(ctx, testutil.PartsToChannel(parts))
	require.NoError(t, err)
	writer.Close()

	return buf.String()
}

// ---- WikiFilterTest ----

// okapi: WikiFilterTest#testStartDocument
func TestExtract_StartDocument(t *testing.T) {
	parts := readDefault(t, "== Title ==\nSimple text.")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, "text/x-wiki", layer.MimeType)
	assert.Equal(t, "UTF-8", layer.Encoding)
	assert.Equal(t, model.LocaleEnglish, layer.Locale)
}

// okapi: WikiFilterTest#testSimpleLine
func TestExtract_SimpleLine(t *testing.T) {
	parts := readDefault(t, "The quick brown fox jumps over the lazy dog.")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "simple line should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "The quick brown fox jumps over the lazy dog.")
}

// okapi: WikiFilterTest#testMultipleLines
func TestExtract_MultipleLines(t *testing.T) {
	parts := readDefault(t, "Line one.\nLine two.\nLine three.")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "multiple lines should produce translatable blocks")

	// All lines should be part of one paragraph block
	texts := testutil.BlockTexts(blocks)
	found := false
	for _, txt := range texts {
		if strings.Contains(txt, "Line one") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find 'Line one' in extracted texts")
}

// okapi: WikiFilterTest#testHeader
func TestExtract_Header(t *testing.T) {
	parts := readDefault(t, "== Header Text ==\nBody text here.")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "header should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	// The header text should be extracted without == markers
	headerFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "Header Text") {
			headerFound = true
			break
		}
	}
	assert.True(t, headerFound, "should extract header text 'Header Text'")

	// Body text should also be extracted
	bodyFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "Body text here") {
			bodyFound = true
			break
		}
	}
	assert.True(t, bodyFound, "should extract body text 'Body text here'")
}

// okapi: WikiFilterTest#testTable
func TestExtract_Table(t *testing.T) {
	wikiText := "{|\n|-\n| Cell 1 || Cell 2\n|-\n| Cell 3 || Cell 4\n|}"
	parts := readDefault(t, wikiText)

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "table should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	cell1Found := false
	cell2Found := false
	for _, txt := range texts {
		if strings.Contains(txt, "Cell 1") {
			cell1Found = true
		}
		if strings.Contains(txt, "Cell 2") {
			cell2Found = true
		}
	}
	assert.True(t, cell1Found, "should extract table cell 'Cell 1'")
	assert.True(t, cell2Found, "should extract table cell 'Cell 2'")
}

// okapi: WikiFilterTest#testImageCaption
func TestExtract_ImageCaption(t *testing.T) {
	wikiText := "[[File:Example.jpg|thumb|A caption for the image]]"
	parts := readDefault(t, wikiText)

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "image caption should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	captionFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "caption") {
			captionFound = true
			break
		}
	}
	assert.True(t, captionFound, "should extract image caption text")
}

// okapi: WikiFilterTest#testSimilarHtmlTags
func TestExtract_SimilarHtmlTags(t *testing.T) {
	wikiText := "Text with <b>bold</b> and <br/> tags."
	parts := readDefault(t, wikiText)

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "wiki content with HTML tags should produce blocks")

	texts := testutil.BlockTexts(blocks)
	found := false
	for _, txt := range texts {
		if strings.Contains(txt, "bold") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract text containing 'bold'")
}

// okapi: WikiFilterTest#testComplexSeparatingWhitespace
func TestExtract_ComplexSeparatingWhitespace(t *testing.T) {
	wikiText := "First paragraph.\n\n\nSecond paragraph."
	parts := readDefault(t, wikiText)

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "complex whitespace content should produce blocks")

	texts := testutil.BlockTexts(blocks)
	firstFound := false
	secondFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "First paragraph") {
			firstFound = true
		}
		if strings.Contains(txt, "Second paragraph") {
			secondFound = true
		}
	}
	assert.True(t, firstFound, "should extract 'First paragraph'")
	assert.True(t, secondFound, "should extract 'Second paragraph'")
}

// okapi: WikiFilterTest#testDoubleExtraction
func TestExtract_DoubleExtraction(t *testing.T) {
	input := "== Title ==\nSome text.\n\nAnother paragraph."
	ctx := t.Context()

	// First extraction
	reader1 := wiki.NewReader()
	err := reader1.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts1 := testutil.CollectParts(t, reader1.Read(ctx))
	reader1.Close()

	// Write output
	var buf bytes.Buffer
	writer := wiki.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)
	err = writer.Write(ctx, testutil.PartsToChannel(parts1))
	require.NoError(t, err)
	writer.Close()

	// Second extraction from output
	reader2 := wiki.NewReader()
	err = reader2.Open(ctx, testutil.RawDocFromString(buf.String(), model.LocaleEnglish))
	require.NoError(t, err)
	parts2 := testutil.CollectParts(t, reader2.Read(ctx))
	reader2.Close()

	blocks1 := testutil.FilterBlocks(parts1)
	blocks2 := testutil.FilterBlocks(parts2)

	require.NotEmpty(t, blocks1, "first extraction should produce blocks")
	require.NotEmpty(t, blocks2, "second extraction should produce blocks")
	assert.Equal(t, len(blocks1), len(blocks2), "double extraction should produce same block count")
}

// okapi: WikiFilterTest#testOpenTwiceWithString
func TestExtract_OpenTwiceWithString(t *testing.T) {
	wiki1 := "First document content."
	wiki2 := "Second document content."

	parts1 := readDefault(t, wiki1)
	parts2 := readDefault(t, wiki2)

	blocks1 := testutil.FilterBlocks(parts1)
	blocks2 := testutil.FilterBlocks(parts2)

	require.NotEmpty(t, blocks1, "first open should produce blocks")
	require.NotEmpty(t, blocks2, "second open should produce blocks")

	texts1 := testutil.BlockTexts(blocks1)
	texts2 := testutil.BlockTexts(blocks2)
	assert.Contains(t, texts1, "First document content.")
	assert.Contains(t, texts2, "Second document content.")
}

// ---- Additional extraction tests ----

// okapi: WikiFilterTest#testSimpleLine (layer structure variant)
func TestExtract_LayerStructure(t *testing.T) {
	parts := readDefault(t, "Hello wiki world")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

// okapi: WikiFilterTest#testSimpleLine (block IDs uniqueness)
func TestExtract_BlockIDs(t *testing.T) {
	parts := readDefault(t, "Line A.\n\nLine B.\n\nLine C.")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

// okapi: WikiFilterTest#testSimpleLine (segment structure)
func TestExtract_SegmentIDs(t *testing.T) {
	parts := readDefault(t, "Hello wiki world.")

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		require.NotEmpty(t, b.Source, "block should have source segments")
		for _, seg := range b.Source {
			assert.NotEmpty(t, seg.ID, "segment should have an ID")
			assert.NotNil(t, seg.Content, "segment should have content")
		}
	}
}

// okapi: WikiFilterTest#testHeader (multiple heading levels)
func TestExtract_MultipleHeadingLevels(t *testing.T) {
	wikiText := "== Level 2 ==\n=== Level 3 ===\n==== Level 4 ===="
	parts := readDefault(t, wikiText)

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "multiple heading levels should produce blocks")

	texts := testutil.BlockTexts(blocks)
	level2 := false
	level3 := false
	level4 := false
	for _, txt := range texts {
		if strings.Contains(txt, "Level 2") {
			level2 = true
		}
		if strings.Contains(txt, "Level 3") {
			level3 = true
		}
		if strings.Contains(txt, "Level 4") {
			level4 = true
		}
	}
	assert.True(t, level2, "should extract Level 2 heading")
	assert.True(t, level3, "should extract Level 3 heading")
	assert.True(t, level4, "should extract Level 4 heading")
}

// okapi: WikiFilterTest#testMultipleLines (full-file extraction: simple.wiki)
func TestExtract_SimpleWikiFile(t *testing.T) {
	ctx := t.Context()
	f, err := os.Open("testdata/simple.wiki")
	require.NoError(t, err)

	reader := wiki.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/simple.wiki", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "simple.wiki should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	titleFound := false
	foxFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "Title") {
			titleFound = true
		}
		if strings.Contains(txt, "quick brown fox") {
			foxFound = true
		}
	}
	assert.True(t, titleFound, "should extract 'Title' from simple.wiki")
	assert.True(t, foxFound, "should extract 'quick brown fox' from simple.wiki")
}

// okapi: WikiFilterTest#testMultipleLines (full-file extraction: mediawiki.wiki)
func TestExtract_MediawikiFile(t *testing.T) {
	ctx := t.Context()
	f, err := os.Open("testdata/mediawiki.wiki")
	require.NoError(t, err)

	reader := wiki.NewReader()
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/mediawiki.wiki", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "mediawiki.wiki should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	landgroveFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "Landgrove") {
			landgroveFound = true
			break
		}
	}
	assert.True(t, landgroveFound, "should extract 'Landgrove' from mediawiki.wiki")
}

// okapi: WikiFilterTest#testDoubleExtraction (full-file: dokuwiki.txt)
func TestExtract_DokuWikiFile(t *testing.T) {
	ctx := t.Context()
	f, err := os.Open("testdata/dokuwiki.txt")
	require.NoError(t, err)

	reader := newDokuWikiReader(t)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/dokuwiki.txt", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "dokuwiki.txt should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	formattingFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "Formatting Syntax") {
			formattingFound = true
			break
		}
	}
	assert.True(t, formattingFound, "should extract 'Formatting Syntax' from dokuwiki.txt")
}

// ---- WikiWriterTest ----

// okapi: WikiWriterTest#testOutput
func TestWrite_Output(t *testing.T) {
	wikiText := "== Title ==\nSimple text."
	output := roundtrip(t, wikiText)
	assert.Contains(t, output, "Title", "title should survive roundtrip")
	assert.Contains(t, output, "Simple text", "body text should survive roundtrip")
}

// okapi: WikiWriterTest#testOutputTable
func TestWrite_OutputTable(t *testing.T) {
	wikiText := "{|\n|-\n| Cell 1 || Cell 2\n|-\n| Cell 3 || Cell 4\n|}"
	output := roundtrip(t, wikiText)
	assert.Contains(t, output, "Cell 1", "table cell 1 should survive roundtrip")
	assert.Contains(t, output, "Cell 2", "table cell 2 should survive roundtrip")
}

// okapi: WikiWriterTest#testWhitespaces
func TestWrite_Whitespaces(t *testing.T) {
	wikiText := "First paragraph.\n\nSecond paragraph."
	output := roundtrip(t, wikiText)
	assert.Contains(t, output, "First paragraph", "first paragraph should survive roundtrip")
	assert.Contains(t, output, "Second paragraph", "second paragraph should survive roundtrip")
}

// ---- RoundTrip tests ----

// okapi: RoundTripWikiIT (simple inline snippet)
func TestRoundTrip_Simple(t *testing.T) {
	output := roundtrip(t, "== Title ==\nSimple text here.\n")
	assert.Contains(t, output, "Title")
	assert.Contains(t, output, "Simple text here")
}

// okapi: RoundTripWikiIT (multi-line snippet)
func TestRoundTrip_MultiLine(t *testing.T) {
	output := roundtrip(t, "== Title ==\nFirst line.\nSecond line.\n")
	assert.Contains(t, output, "Title")
	assert.Contains(t, output, "First line")
	assert.Contains(t, output, "Second line")
}

// okapi: RoundTripWikiIT (table snippet)
func TestRoundTrip_Table(t *testing.T) {
	output := roundtrip(t, "{|\n|-\n| Cell 1 || Cell 2\n|-\n| Cell 3 || Cell 4\n|}")
	require.NotEmpty(t, output, "roundtrip should produce output")
	assert.Contains(t, output, "Cell 1")
}

// okapi: RoundTripWikiIT (header roundtrip)
func TestRoundTrip_Header(t *testing.T) {
	output := roundtrip(t, "== Header ==\nBody text.\n")
	require.NotEmpty(t, output, "roundtrip should produce output")
	assert.Contains(t, output, "Header")
	assert.Contains(t, output, "Body text")
}

// okapi: RoundTripWikiIT (image caption roundtrip)
func TestRoundTrip_ImageCaption(t *testing.T) {
	output := roundtrip(t, "[[File:Example.jpg|thumb|A caption for the image]]")
	require.NotEmpty(t, output, "roundtrip should produce output")
}

// okapi: RoundTripWikiIT (wiki markup formatting roundtrip)
func TestRoundTrip_WikiFormatting(t *testing.T) {
	tests := []struct {
		name  string
		input string
	}{
		{"bold", "'''Bold text''' here."},
		{"italic", "''Italic text'' here."},
		{"link", "A [[link|link text]] in text."},
		{"header_l2", "== Header 2 =="},
		{"header_l3", "=== Header 3 ==="},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output := roundtrip(t, tt.input)
			require.NotEmpty(t, output, "roundtrip should produce output for %s", tt.name)
		})
	}
}

// okapi: RoundTripWikiIT#testWikiFiles (*.wiki files)
func TestRoundTrip_WikiFiles(t *testing.T) {
	tests := []struct {
		name string
		file string
	}{
		{"simple", "testdata/simple.wiki"},
		{"mediawiki", "testdata/mediawiki.wiki"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := t.Context()
			f, err := os.Open(tt.file)
			require.NoError(t, err)

			reader := wiki.NewReader()
			err = reader.Open(ctx, testutil.RawDocFromReader(f, tt.file, model.LocaleEnglish))
			require.NoError(t, err)
			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()

			var buf bytes.Buffer
			writer := wiki.NewWriter()
			err = writer.SetOutputWriter(&buf)
			require.NoError(t, err)
			writer.SetLocale(model.LocaleEnglish)
			err = writer.Write(ctx, testutil.PartsToChannel(parts))
			require.NoError(t, err)
			writer.Close()

			require.NotEmpty(t, buf.String(), "roundtrip should produce output for %s", tt.file)
		})
	}
}

// okapi: RoundTripWikiIT#testWikiFiles (*.txt DokuWiki files)
func TestRoundTrip_DokuWikiTxtFiles(t *testing.T) {
	ctx := t.Context()
	f, err := os.Open("testdata/dokuwiki.txt")
	require.NoError(t, err)

	reader := newDokuWikiReader(t)
	err = reader.Open(ctx, testutil.RawDocFromReader(f, "testdata/dokuwiki.txt", model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := wiki.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleEnglish)
	err = writer.Write(ctx, testutil.PartsToChannel(parts))
	require.NoError(t, err)
	writer.Close()

	require.NotEmpty(t, buf.String(), "roundtrip should produce output for dokuwiki.txt")
}

// ---- Additional tests ----

func TestRead_NameAndMimeType(t *testing.T) {
	reader := wiki.NewReader()
	assert.Equal(t, "wiki", reader.Name())
	assert.Equal(t, "Wiki", reader.DisplayName())

	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/x-wiki")
	assert.Contains(t, sig.Extensions, ".wiki")
}

func TestRead_Configurations(t *testing.T) {
	reader := wiki.NewReader()
	cfg := reader.Config()
	assert.Equal(t, "wiki", cfg.FormatName())

	wikiCfg, ok := cfg.(*wiki.Config)
	require.True(t, ok)
	assert.Equal(t, wiki.VariantMediaWiki, wikiCfg.Variant)

	// Reset restores defaults
	wikiCfg.Variant = wiki.VariantDokuWiki
	wikiCfg.Reset()
	assert.Equal(t, wiki.VariantMediaWiki, wikiCfg.Variant)
}

func TestRead_LoadParams(t *testing.T) {
	cfg := &wiki.Config{}
	err := cfg.ApplyMap(map[string]any{"variant": "dokuwiki"})
	require.NoError(t, err)
	assert.Equal(t, wiki.VariantDokuWiki, cfg.Variant)

	// Unknown parameter should error
	err = cfg.ApplyMap(map[string]any{"unknownParam": "value"})
	require.Error(t, err)

	// Wrong type should error
	err = cfg.ApplyMap(map[string]any{"variant": 123})
	require.Error(t, err)
}

func TestRead_EmptyInput(t *testing.T) {
	parts := readDefault(t, "")
	blocks := testutil.FilterBlocks(parts)
	assert.Empty(t, blocks)
}

func TestRead_NilDocument(t *testing.T) {
	ctx := t.Context()
	reader := wiki.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestRead_Cancel(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	reader := wiki.NewReader()
	// Create a large input
	lines := make([]string, 10000)
	for i := range lines {
		lines[i] = "This is a test line for cancellation"
	}
	input := strings.Join(lines, "\n\n")

	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	ch := reader.Read(ctx)

	count := 0
	for range ch {
		count++
		if count >= 5 {
			cancel()
			break
		}
	}
	// Drain remaining
	for range ch {
	}

	assert.GreaterOrEqual(t, count, 5, "should have read at least 5 parts before cancel")
}

func TestRead_Synchronization(t *testing.T) {
	var wg sync.WaitGroup
	for range 10 {
		wg.Go(func() {
			ctx := t.Context()
			reader := wiki.NewReader()
			err := reader.Open(ctx, testutil.RawDocFromString("== Title ==\nHello", model.LocaleEnglish))
			if err != nil {
				t.Errorf("Open failed: %v", err)
				return
			}
			parts := testutil.CollectParts(t, reader.Read(ctx))
			reader.Close()
			blocks := testutil.FilterBlocks(parts)
			if len(blocks) < 1 {
				t.Errorf("expected at least 1 block, got %d", len(blocks))
			}
		})
	}
	wg.Wait()
}

func TestRoundTrip_WithTargetLocale(t *testing.T) {
	ctx := t.Context()

	reader := wiki.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("== Title ==\nHello world.", model.LocaleEnglish))
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Set French targets
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Title" {
				block.SetTargetText(model.LocaleFrench, "Titre")
			} else if block.SourceText() == "Hello world." {
				block.SetTargetText(model.LocaleFrench, "Bonjour le monde.")
			}
		}
	}

	// Write with French locale
	var buf bytes.Buffer
	writer := wiki.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)

	err = writer.Write(ctx, testutil.PartsToChannel(parts))
	require.NoError(t, err)
	writer.Close()

	assert.Contains(t, buf.String(), "Titre")
	assert.Contains(t, buf.String(), "Bonjour le monde.")
}

func TestRead_Schema(t *testing.T) {
	cfg := &wiki.Config{}
	schema := cfg.Schema()
	assert.Equal(t, "Wiki Filter", schema.Title)
	assert.Equal(t, "wiki", schema.FormatMeta.ID)
	assert.Contains(t, schema.FormatMeta.MimeTypes, "text/x-wiki")
	assert.Contains(t, schema.Properties, "variant")
}

// okapi: WikiFilterTest#testTable (DokuWiki variant)
func TestExtract_DokuWikiTable(t *testing.T) {
	wikiText := "^ Header 1 ^ Header 2 ^\n| Cell 1 | Cell 2 |"
	reader := newDokuWikiReader(t)
	parts := readString(t, reader, wikiText)

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks, "DokuWiki table should produce translatable blocks")

	texts := testutil.BlockTexts(blocks)
	header1Found := false
	cell1Found := false
	for _, txt := range texts {
		if strings.Contains(txt, "Header 1") {
			header1Found = true
		}
		if strings.Contains(txt, "Cell 1") {
			cell1Found = true
		}
	}
	assert.True(t, header1Found, "should extract DokuWiki table header 'Header 1'")
	assert.True(t, cell1Found, "should extract DokuWiki table cell 'Cell 1'")
}

// okapi: WikiFilterTest#testHeader (DokuWiki variant)
func TestExtract_DokuWikiHeader(t *testing.T) {
	wikiText := "====== Title ======\nBody text."
	reader := newDokuWikiReader(t)
	parts := readString(t, reader, wikiText)

	blocks := testutil.FilterBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := testutil.BlockTexts(blocks)
	titleFound := false
	for _, txt := range texts {
		if strings.Contains(txt, "Title") {
			titleFound = true
			break
		}
	}
	assert.True(t, titleFound, "should extract DokuWiki title")
}
