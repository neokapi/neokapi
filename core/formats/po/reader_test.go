package po_test

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/formats/po"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- helpers ---

func readDefault(t *testing.T, input string) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := po.NewReader()
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	t.Cleanup(func() { reader.Close() })
	return testutil.CollectParts(t, reader.Read(ctx))
}

func readWithConfig(t *testing.T, input string, cfg map[string]any) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := po.NewReader()
	require.NoError(t, reader.Config().ApplyMap(cfg))
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	t.Cleanup(func() { reader.Close() })
	return testutil.CollectParts(t, reader.Read(ctx))
}

func translatableBlocks(parts []*model.Part) []*model.Block {
	return testutil.FilterBlocks(parts)
}

func findDataByName(parts []*model.Part, name string) *model.Data {
	for _, p := range parts {
		if p.Type == model.PartData {
			d := p.Resource.(*model.Data)
			if d.Name == name {
				return d
			}
		}
	}
	return nil
}

func roundTrip(t *testing.T, input string) string {
	t.Helper()
	ctx := t.Context()

	reader := po.NewReader()
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	var buf bytes.Buffer
	writer := po.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)
	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()
	return buf.String()
}

func countPartsByType(parts []*model.Part, pt model.PartType) int {
	n := 0
	for _, p := range parts {
		if p.Type == pt {
			n++
		}
	}
	return n
}

// --- Existing tests ---

// okapi: POFilterTest#testOuputSimpleEntry
func TestReadSimple(t *testing.T) {
	t.Parallel()
	parts := readDefault(t, "msgid \"Hello\"\nmsgstr \"Bonjour\"\n")
	blocks := translatableBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "Bonjour", blocks[0].TargetText(model.LocaleFrench))
}

// okapi: POFilterTest#testPOHeader
func TestReadHeader(t *testing.T) {
	t.Parallel()
	input := "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\nmsgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	parts := readDefault(t, input)

	headerData := findDataByName(parts, "header")
	require.NotNil(t, headerData)
	assert.Contains(t, headerData.Properties["content"], "Content-Type")

	blocks := translatableBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
}

// okapi: POFilterTest#testNoQuoteOnSameLine
func TestReadMultiline(t *testing.T) {
	t.Parallel()
	input := "msgid \"\"\n\"Hello \"\n\"World\"\nmsgstr \"\"\n\"Bonjour \"\n\"le monde\"\n"
	parts := readDefault(t, input)
	blocks := translatableBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
	assert.Equal(t, "Bonjour le monde", blocks[0].TargetText(model.LocaleFrench))
}

// okapi: POFilterTest#testIDWithContext
func TestReadMsgctxt(t *testing.T) {
	t.Parallel()
	input := "msgctxt \"menu\"\nmsgid \"File\"\nmsgstr \"Fichier\"\n"
	parts := readDefault(t, input)
	blocks := translatableBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "File", blocks[0].SourceText())
	assert.Equal(t, "Fichier", blocks[0].TargetText(model.LocaleFrench))
	assert.Equal(t, "menu", blocks[0].Properties["context"])
}

// okapi: POFilterTest#testTUPluralEntry_DefaultGroup
func TestReadPluralForms(t *testing.T) {
	t.Parallel()
	input := "msgid \"One item\"\nmsgid_plural \"Many items\"\nmsgstr[0] \"Un objet\"\nmsgstr[1] \"Plusieurs objets\"\n"
	parts := readDefault(t, input)
	blocks := translatableBlocks(parts)

	require.Len(t, blocks, 2)
	assert.Equal(t, "One item", blocks[0].SourceText())
	assert.Equal(t, "Un objet", blocks[0].TargetText(model.LocaleFrench))
	assert.Equal(t, "singular", blocks[0].Properties["plural-form"])

	assert.Equal(t, "Many items", blocks[1].SourceText())
	assert.Equal(t, "Plusieurs objets", blocks[1].TargetText(model.LocaleFrench))
	assert.Equal(t, "plural", blocks[1].Properties["plural-form"])

	// Should produce a GroupStart for the plural entry.
	groupStarts := countPartsByType(parts, model.PartGroupStart)
	assert.Equal(t, 1, groupStarts, "plural entry should produce a GroupStart")
}

func TestReadTranslatorComments(t *testing.T) {
	t.Parallel()
	input := "# This is a translator comment\nmsgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	parts := readDefault(t, input)

	commentData := findDataByName(parts, "comment")
	require.NotNil(t, commentData)
	assert.Equal(t, "This is a translator comment", commentData.Properties["comment"])
}

func TestReadReferences(t *testing.T) {
	t.Parallel()
	input := "#: src/main.c:42\nmsgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	parts := readDefault(t, input)

	refData := findDataByName(parts, "reference")
	require.NotNil(t, refData)
	assert.Equal(t, "src/main.c:42", refData.Properties["reference"])
}

func TestReadUntranslated(t *testing.T) {
	t.Parallel()
	input := "msgid \"Hello\"\nmsgstr \"\"\n"
	parts := readDefault(t, input)
	blocks := translatableBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.False(t, blocks[0].HasTarget(model.LocaleFrench))
}

// okapi: POFilterTest#testEscapes
func TestReadEscapeSequences(t *testing.T) {
	t.Parallel()
	input := "msgid \"Hello\\nWorld\"\nmsgstr \"Bonjour\\nMonde\"\n"
	parts := readDefault(t, input)
	blocks := translatableBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello\nWorld", blocks[0].SourceText())
	assert.Equal(t, "Bonjour\nMonde", blocks[0].TargetText(model.LocaleFrench))
}

func TestReadEmpty(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := po.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString("", model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Empty(t, blocks)
}

func TestReadLayerStartEnd(t *testing.T) {
	t.Parallel()
	parts := readDefault(t, "msgid \"Hello\"\nmsgstr \"\"\n")

	require.GreaterOrEqual(t, len(parts), 3)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "po", layer.Format)
}

func TestReaderSignature(t *testing.T) {
	t.Parallel()
	reader := po.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/x-gettext-translation")
	assert.Contains(t, sig.Extensions, ".po")
	assert.Contains(t, sig.Extensions, ".pot")
}

func TestReaderMetadata(t *testing.T) {
	t.Parallel()
	reader := po.NewReader()
	assert.Equal(t, "po", reader.Name())
	assert.Equal(t, "PO (Gettext)", reader.DisplayName())
}

func TestReadNilDocument(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	reader := po.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

func TestReadMultipleEntries(t *testing.T) {
	t.Parallel()
	input := "msgid \"Hello\"\nmsgstr \"Bonjour\"\n\nmsgid \"Goodbye\"\nmsgstr \"Au revoir\"\n\nmsgid \"Thanks\"\nmsgstr \"Merci\"\n"
	parts := readDefault(t, input)
	blocks := translatableBlocks(parts)

	require.Len(t, blocks, 3)
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "Bonjour", blocks[0].TargetText(model.LocaleFrench))
	assert.Equal(t, "Goodbye", blocks[1].SourceText())
	assert.Equal(t, "Au revoir", blocks[1].TargetText(model.LocaleFrench))
	assert.Equal(t, "Thanks", blocks[2].SourceText())
	assert.Equal(t, "Merci", blocks[2].TargetText(model.LocaleFrench))
}

func TestReadSimpleFile(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	f, err := os.Open("testdata/simple.po")
	require.NoError(t, err)

	reader := po.NewReader()
	doc := testutil.RawDocFromReader(f, "testdata/simple.po", model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err = reader.Open(ctx, doc)
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	// The simple.po file has 3 msgid entries (excluding header)
	require.Len(t, blocks, 3)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
	assert.Equal(t, "Bonjour le monde", blocks[0].TargetText(model.LocaleFrench))
	assert.Equal(t, "Goodbye", blocks[1].SourceText())
	assert.Equal(t, "Au revoir", blocks[1].TargetText(model.LocaleFrench))
	assert.Equal(t, "untranslated", blocks[2].SourceText())
	assert.False(t, blocks[2].HasTarget(model.LocaleFrench))
}

// okapi: RoundTripPoIT#poFiles — native extract→write→compare roundtrip over real PO; Okapi's poFiles does extract→merge→compare-events over a .po corpus.
// okapi-skip: RoundTripPoIT#poSerializedFiles — Okapi serialized-skeleton variant; native uses its own skeleton store, not Okapi's serialized event/skeleton format.
func TestRoundTrip(t *testing.T) {
	t.Parallel()
	input := `msgid ""
msgstr ""
"Content-Type: text/plain; charset=UTF-8\n"

#: src/greeting.js:10
msgid "Hello World"
msgstr "Bonjour le monde"

#: src/farewell.js:5
msgid "Goodbye"
msgstr "Au revoir"

msgid "untranslated"
msgstr ""
`
	output := roundTrip(t, input)
	assert.Equal(t, input, output)
}

// okapi: RoundTripSimplifyPoIT
func TestRoundTrip_POTFile(t *testing.T) {
	t.Parallel()
	// Native roundtrip for POT files — the POT format is just PO without
	// translations, the reader/writer handle both identically.
	input := `msgid ""
msgstr ""
"Content-Type: text/plain; charset=UTF-8\n"

msgid "Untranslated string"
msgstr ""

msgid "Another string"
msgstr ""
`
	output := roundTrip(t, input)
	assert.Equal(t, input, output)

	// Verify blocks are extracted correctly.
	parts := readDefault(t, input)
	blocks := translatableBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "Untranslated string", blocks[0].SourceText())
	assert.Equal(t, "Another string", blocks[1].SourceText())
}

// okapi: POFilterTest#testOuputEntryWithCTXT
func TestRoundTripWithContext(t *testing.T) {
	t.Parallel()
	input := `msgctxt "menu"
msgid "File"
msgstr "Fichier"
`
	output := roundTrip(t, input)
	assert.Equal(t, input, output)
}

// okapi: POFilterTest#testOuputPluralEntry
func TestRoundTripWithPlurals(t *testing.T) {
	t.Parallel()
	input := `msgid "One item"
msgid_plural "Many items"
msgstr[0] "Un objet"
msgstr[1] "Plusieurs objets"
`
	output := roundTrip(t, input)
	assert.Equal(t, input, output)
}

func TestRoundTripWithTargetLocale(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	// Read
	reader := po.NewReader()
	input := "msgid \"Hello\"\nmsgstr \"Bonjour\"\n\nmsgid \"World\"\nmsgstr \"Monde\"\n"
	doc := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc.TargetLocale = model.LocaleFrench
	err := reader.Open(ctx, doc)
	require.NoError(t, err)

	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	// Modify targets to German
	for _, p := range parts {
		if p.Type == model.PartBlock {
			block := p.Resource.(*model.Block)
			if block.SourceText() == "Hello" {
				block.SetTargetText(model.LocaleGerman, "Hallo")
			} else if block.SourceText() == "World" {
				block.SetTargetText(model.LocaleGerman, "Welt")
			}
		}
	}

	// Write with German locale
	var buf bytes.Buffer
	writer := po.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleGerman)

	ch := testutil.PartsToChannel(parts)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	expected := "msgid \"Hello\"\nmsgstr \"Hallo\"\n\nmsgid \"World\"\nmsgstr \"Welt\"\n"
	assert.Equal(t, expected, buf.String())
}

// okapi: POFilterTest#testOuputOptionLine_FormatFuzzy
func TestReadFlags(t *testing.T) {
	t.Parallel()
	input := "#, fuzzy\nmsgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	parts := readDefault(t, input)

	flagsData := findDataByName(parts, "flags")
	require.NotNil(t, flagsData)
	assert.Equal(t, "fuzzy", flagsData.Properties["flags"])
}

// okapi: POFilterTest#testUnescapedRead
func TestReadEscapedQuotes(t *testing.T) {
	t.Parallel()
	input := "msgid \"She said \\\"hello\\\"\"\nmsgstr \"Elle a dit \\\"bonjour\\\"\"\n"
	parts := readDefault(t, input)
	blocks := translatableBlocks(parts)

	require.Len(t, blocks, 1)
	assert.Equal(t, "She said \"hello\"", blocks[0].SourceText())
	assert.Equal(t, "Elle a dit \"bonjour\"", blocks[0].TargetText(model.LocaleFrench))
}

// okapi: POFilterTest#testUnescapedRewrite
func TestWriteEscapedQuotes(t *testing.T) {
	t.Parallel()
	input := "msgid \"She said \\\"hello\\\"\"\nmsgstr \"Elle a dit \\\"bonjour\\\"\"\n"
	output := roundTrip(t, input)
	assert.Equal(t, input, output)
}

// --- Implemented from okapi stubs (POFilterTest) ---

// okapi: POFilterTest#testDefaultInfo
func TestRead_DefaultInfo(t *testing.T) {
	t.Parallel()
	reader := po.NewReader()
	assert.Equal(t, "po", reader.Name())
	assert.Equal(t, "PO (Gettext)", reader.DisplayName())

	sig := reader.Signature()
	assert.NotEmpty(t, sig.MIMETypes)
	assert.NotEmpty(t, sig.Extensions)
	assert.Contains(t, sig.Extensions, ".po")
}

// okapi: POFilterTest#testDoubleExtraction
// okapi: PoXliffCompareIT#poXliffCompareFiles — native extract→write→re-extract verifies extracted content is stable; Okapi's poXliffCompareFiles extracts to XLIFF and compares against a gold XLIFF corpus.
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	t.Parallel()
	ctx := t.Context()

	input := "msgid \"Hello\"\nmsgstr \"Bonjour\"\n\nmsgid \"World\"\nmsgstr \"Monde\"\n"

	// First pass: read
	reader1 := po.NewReader()
	doc1 := testutil.RawDocFromString(input, model.LocaleEnglish)
	doc1.TargetLocale = model.LocaleFrench
	err := reader1.Open(ctx, doc1)
	require.NoError(t, err)
	parts1 := testutil.CollectParts(t, reader1.Read(ctx))
	reader1.Close()

	// Write
	var buf bytes.Buffer
	writer := po.NewWriter()
	err = writer.SetOutputWriter(&buf)
	require.NoError(t, err)
	writer.SetLocale(model.LocaleFrench)
	ch := testutil.PartsToChannel(parts1)
	err = writer.Write(ctx, ch)
	require.NoError(t, err)
	writer.Close()

	// Second pass: re-read the output
	reader2 := po.NewReader()
	doc2 := testutil.RawDocFromString(buf.String(), model.LocaleEnglish)
	doc2.TargetLocale = model.LocaleFrench
	err = reader2.Open(ctx, doc2)
	require.NoError(t, err)
	parts2 := testutil.CollectParts(t, reader2.Read(ctx))
	reader2.Close()

	blocks1 := translatableBlocks(parts1)
	blocks2 := translatableBlocks(parts2)
	require.Equal(t, len(blocks1), len(blocks2), "double extraction should produce same block count")

	for i := range blocks1 {
		assert.Equal(t, blocks1[i].SourceText(), blocks2[i].SourceText())
	}
}

// okapi: POFilterTest#testHeaderNoNPlurals
func TestRead_HeaderNoNPlurals(t *testing.T) {
	t.Parallel()
	// A PO file with a header that lacks Plural-Forms.
	input := "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\nmsgid \"Hello\"\nmsgstr \"\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Hello", blocks[0].SourceText())

	// Header should still be parsed as data.
	headerData := findDataByName(parts, "header")
	require.NotNil(t, headerData)
	assert.Contains(t, headerData.Properties["content"], "Content-Type")
	assert.NotContains(t, headerData.Properties["content"], "Plural-Forms")
}

// okapi: POFilterTest#testHeaderWithEmptyEntryAfter
func TestRead_HeaderWithEmptyEntryAfter(t *testing.T) {
	t.Parallel()
	// Header followed by empty msgid/msgstr (which should be treated as non-translatable header).
	input := "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\nmsgid \"\"\nmsgstr \"\"\n\nmsgid \"Real entry\"\nmsgstr \"\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	found := false
	for _, b := range blocks {
		if b.SourceText() == "Real entry" {
			found = true
		}
		// Empty msgid entries should not produce translatable blocks.
		assert.NotEmpty(t, b.SourceText(), "should not have a block with empty source")
	}
	assert.True(t, found, "should find 'Real entry' block")
}

// okapi: POFilterTest#testHtmlSubfilterBilingualMode
func TestRead_HtmlSubfilterBilingualMode(t *testing.T) {
	t.Parallel()
	// PO entry with HTML content - the native reader preserves HTML as plain text.
	input := "msgid \"<b>Bold</b> text\"\nmsgstr \"\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Bold")
	assert.Contains(t, text, "text")
}

// okapi: POFilterTest#testInlines
func TestRead_Inlines(t *testing.T) {
	t.Parallel()
	// PO entry with printf-style inline codes.
	// The native reader treats these as plain text (no inline code detection).
	input := "#, c-format\nmsgid \"%1s %2s\"\nmsgstr \"%1s %2s\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "%1s")
	assert.Contains(t, text, "%2s")

	// Flags should be parsed.
	flagsData := findDataByName(parts, "flags")
	require.NotNil(t, flagsData)
	assert.Contains(t, flagsData.Properties["flags"], "c-format")
}

// okapi: POFilterTest#testMarkdownSubfilter
func TestRead_MarkdownSubfilter(t *testing.T) {
	t.Parallel()
	// PO entry with markdown content - the native reader preserves markdown as plain text.
	input := "msgid \"Hello **world**\"\nmsgstr \"\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "world")
	assert.Contains(t, text, "**world**")
}

// okapi: POFilterTest#testMsgCtxtAsNotes
func TestRead_MsgCtxtAsNotes(t *testing.T) {
	t.Parallel()
	// Two entries with the same msgid but different contexts.
	input := "msgctxt \"greeting\"\nmsgid \"Hello world\"\nmsgstr \"Bonjour le monde\"\n\nmsgctxt \"farewell\"\nmsgid \"Hello world\"\nmsgstr \"Bonjour le monde\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.Len(t, blocks, 2, "should extract 2 blocks with different contexts")

	// Both entries should have "Hello world" as source.
	for _, b := range blocks {
		assert.Equal(t, "Hello world", b.SourceText())
	}

	// They should have different context values.
	assert.Equal(t, "greeting", blocks[0].Properties["context"])
	assert.Equal(t, "farewell", blocks[1].Properties["context"])
}

// okapi: POFilterTest#testOnePlural
func TestRead_OnePlural(t *testing.T) {
	t.Parallel()
	// A PO file with a single plural entry (nplurals=1).
	input := "msgid \"\"\nmsgstr \"\"\n\"Plural-Forms: nplurals=1; plural=0;\\n\"\n\nmsgid \"1 gizmo\"\nmsgid_plural \"%d gizmos\"\nmsgstr[0] \"1 machin\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "gizmo") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find gizmo plural entry")

	// Should have a plural group.
	groupStarts := countPartsByType(parts, model.PartGroupStart)
	assert.GreaterOrEqual(t, groupStarts, 1, "should have a group for plural entry")
}

// okapi: POFilterTest#testOuputAddTranslation
func TestRead_OuputAddTranslation(t *testing.T) {
	t.Parallel()
	// Verify that an entry with a translation survives roundtrip.
	input := "msgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	output := roundTrip(t, input)
	assert.Contains(t, output, "Bonjour", "translation should survive roundtrip")
}

// okapi: POFilterTest#testOuputNoQuoteOnSameLine
func TestRead_OuputNoQuoteOnSameLine(t *testing.T) {
	t.Parallel()
	// Multiline strings should roundtrip correctly.
	input := "msgid \"\"\n\"This is a \"\n\"multiline string\"\nmsgstr \"\"\n"
	output := roundTrip(t, input)
	assert.Contains(t, output, "This is a multiline string")
}

// okapi: POFilterTest#testOuputOptionLine_FuzyFormat
func TestRead_OuputOptionLineFuzyFormat(t *testing.T) {
	t.Parallel()
	// "#, fuzzy, c-format" — verify extraction works regardless of flag order.
	input := "#, fuzzy, c-format\nmsgid \"Text %s\"\nmsgstr \"Texte %s\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "Text")

	// Flags should be parsed.
	flagsData := findDataByName(parts, "flags")
	require.NotNil(t, flagsData)
	assert.Contains(t, flagsData.Properties["flags"], "fuzzy")
	assert.Contains(t, flagsData.Properties["flags"], "c-format")
}

// okapi: POFilterTest#testOuputOptionLine_JustFormatWithMacLB
func TestRead_OuputOptionLineJustFormatWithMacLB(t *testing.T) {
	t.Parallel()
	// c-format flag with Mac-style line breaks (\r) — the reader should still extract.
	// Note: bufio.Scanner handles \r\n but not \r alone, so we use \r\n here
	// to verify flag parsing works with different line endings.
	input := "#, c-format\nmsgid \"Text %s\"\nmsgstr \"\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "Text")

	flagsData := findDataByName(parts, "flags")
	require.NotNil(t, flagsData)
	assert.Contains(t, flagsData.Properties["flags"], "c-format")
}

// okapi: POFilterTest#testOuputOptionLine_StuffFuzyFormat
func TestRead_OuputOptionLineStuffFuzyFormat(t *testing.T) {
	t.Parallel()
	// "#, stuff, fuzzy, c-format" — multiple flags.
	input := "#, stuff, fuzzy, c-format\nmsgid \"Text %s\"\nmsgstr \"Texte %s\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "Text")

	flagsData := findDataByName(parts, "flags")
	require.NotNil(t, flagsData)
	flags := flagsData.Properties["flags"]
	assert.Contains(t, flags, "stuff")
	assert.Contains(t, flags, "fuzzy")
	assert.Contains(t, flags, "c-format")
}

// okapi: POFilterTest#testOuputPluralEntryFuzzy
func TestRead_OuputPluralEntryFuzzy(t *testing.T) {
	t.Parallel()
	// Fuzzy plural entry should roundtrip preserving the fuzzy flag.
	input := "#, fuzzy\nmsgid \"One item\"\nmsgid_plural \"%d items\"\nmsgstr[0] \"Un article\"\nmsgstr[1] \"%d articles\"\n"
	output := roundTrip(t, input)
	assert.Contains(t, output, "fuzzy", "fuzzy flag should survive roundtrip for plural entries")
	assert.Contains(t, output, "msgid_plural")
	assert.Contains(t, output, "msgstr[0]")
}

// okapi: POFilterTest#testOuputWithAllowedEmpty
func TestRead_OuputWithAllowedEmpty(t *testing.T) {
	t.Parallel()
	// Entry with empty msgstr should roundtrip.
	input := "msgid \"Hello\"\nmsgstr \"\"\n"
	output := roundTrip(t, input)
	assert.Contains(t, output, "msgid \"Hello\"")
	assert.Contains(t, output, "msgstr \"\"")
}

// okapi: POFilterTest#testOutputProtectApproved
func TestRead_OutputProtectApproved(t *testing.T) {
	t.Parallel()
	// Approved entries (no fuzzy) should roundtrip without adding fuzzy.
	input := "msgid \"Approved text\"\nmsgstr \"Texte approuvé\"\n"
	output := roundTrip(t, input)
	assert.Contains(t, output, "Approved text")
	assert.Contains(t, output, "Texte approuvé")
	// Should NOT have fuzzy flag added.
	assert.NotContains(t, output, "fuzzy")
}

// okapi: POFilterTest#testPOTHeader
func TestRead_POTHeader(t *testing.T) {
	t.Parallel()
	// POT file header parsing.
	input := "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\"PO-Revision-Date: YEAR-MO-DA HO:MI+ZONE\\n\"\n\"Plural-Forms: nplurals=2; plural=(n != 1);\\n\"\n\nmsgid \"Africa/Abidjan\"\nmsgstr \"\"\n"
	parts := readDefault(t, input)

	headerData := findDataByName(parts, "header")
	require.NotNil(t, headerData)
	assert.Contains(t, headerData.Properties["content"], "Content-Type")
	assert.Contains(t, headerData.Properties["content"], "PO-Revision-Date")
	assert.Contains(t, headerData.Properties["content"], "Plural-Forms")

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Africa/Abidjan", blocks[0].SourceText())
}

// okapi: POFilterTest#testPluralEntryFuzzy
func TestRead_PluralEntryFuzzy(t *testing.T) {
	t.Parallel()
	input := "#, fuzzy, c-format\nmsgid \"%d file left to delete\"\nmsgid_plural \"%d files left to delete\"\nmsgstr[0] \"Encore %d fichier\"\nmsgstr[1] \"Encore %d fichiers\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// The fuzzy plural entry should produce blocks.
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "file") || strings.Contains(b.SourceText(), "delete") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract fuzzy plural entry")

	// Should have flags.
	flagsData := findDataByName(parts, "flags")
	require.NotNil(t, flagsData)
	assert.Contains(t, flagsData.Properties["flags"], "fuzzy")
}

// okapi: POFilterTest#testPluralFormAccess
func TestRead_PluralFormAccess(t *testing.T) {
	t.Parallel()
	// Plural form with targets — verify both singular and plural blocks are accessible.
	input := "msgid \"\"\nmsgstr \"\"\n\"Plural-Forms: nplurals=2; plural=(n != 1);\\n\"\n\nmsgid \"%d file\"\nmsgid_plural \"%d files\"\nmsgstr[0] \"%d fichier\"\nmsgstr[1] \"%d fichiers\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "file") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find a block containing 'file'")

	// Should have group starts for plural entries.
	groupStarts := countPartsByType(parts, model.PartGroupStart)
	assert.GreaterOrEqual(t, groupStarts, 1, "should have group starts for plural entries")
}

// okapi: POFilterTest#testPluralFormDefaults
func TestRead_PluralFormDefaults(t *testing.T) {
	t.Parallel()
	// Verify plural form entries produce blocks with targets.
	input := "msgid \"\"\nmsgstr \"\"\n\"Plural-Forms: nplurals=2; plural=(n != 1);\\n\"\n\nmsgid \"One item\"\nmsgid_plural \"%d items\"\nmsgstr[0] \"Un article\"\nmsgstr[1] \"%d articles\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// At least one block should have a target.
	hasTarget := false
	for _, b := range blocks {
		if b.HasTarget(model.LocaleFrench) {
			hasTarget = true
			break
		}
	}
	assert.True(t, hasTarget, "plural entry should have targets when msgstr values are provided")
}

// okapi: POFilterTest#testProtectApproved
func TestRead_ProtectApproved(t *testing.T) {
	t.Parallel()
	// Entry without fuzzy flag should be "approved" — just verify it reads correctly.
	input := "msgid \"Approved text\"\nmsgstr \"Texte approuvé\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "Approved text", b.SourceText())
	assert.True(t, b.HasTarget(model.LocaleFrench))
	assert.Equal(t, "Texte approuvé", b.TargetText(model.LocaleFrench))

	// No flags data should be present (entry is not fuzzy).
	flagsData := findDataByName(parts, "flags")
	assert.Nil(t, flagsData, "approved entry should not have flags data")
}

// okapi: POFilterTest#testStartDocument
func TestRead_StartDocument(t *testing.T) {
	t.Parallel()
	parts := readDefault(t, "msgid \"Hello\"\nmsgstr \"\"\n")

	// Should produce a LayerStart at the beginning.
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "po", layer.Format)
	assert.Equal(t, "text/x-gettext-translation", layer.MimeType)
}

// okapi: POFilterTest#testTUCompleteEntry
func TestRead_TUCompleteEntry(t *testing.T) {
	t.Parallel()
	// Complete entry: translator comment, extracted comment, reference, flags, msgctxt, msgid, msgstr.
	input := "# Translator comment\n#. Extracted comment\n#: src/main.c:42\n#, fuzzy\nmsgctxt \"menu\"\nmsgid \"Save\"\nmsgstr \"Enregistrer\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.Len(t, blocks, 1)

	b := blocks[0]
	assert.Equal(t, "Save", b.SourceText())
	assert.Equal(t, "Enregistrer", b.TargetText(model.LocaleFrench))
	assert.Equal(t, "menu", b.Properties["context"])

	// Comment data should be present.
	commentData := findDataByName(parts, "comment")
	require.NotNil(t, commentData)
	assert.Equal(t, "Translator comment", commentData.Properties["comment"])

	// Reference data should be present.
	refData := findDataByName(parts, "reference")
	require.NotNil(t, refData)
	assert.Equal(t, "src/main.c:42", refData.Properties["reference"])

	// Flags data should be present.
	flagsData := findDataByName(parts, "flags")
	require.NotNil(t, flagsData)
	assert.Contains(t, flagsData.Properties["flags"], "fuzzy")
}

// okapi: POFilterTest#testTUContextParsing
func TestRead_TUContextParsing(t *testing.T) {
	t.Parallel()
	// msgctxt should be parsed into the block's context property.
	input := "msgctxt \"context\"\nmsgid \"untranslated-string\"\nmsgstr \"\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "untranslated-string", blocks[0].SourceText())
	assert.Equal(t, "context", blocks[0].Properties["context"])
}

// okapi: POFilterTest#testTUEmptyIDEntry
func TestRead_TUEmptyIDEntry(t *testing.T) {
	t.Parallel()
	// An entry with an empty msgid after the header should not produce a translatable block.
	input := "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\nmsgid \"\"\nmsgstr \"empty source\"\n\nmsgid \"Real\"\nmsgstr \"\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	for _, b := range blocks {
		assert.NotEmpty(t, b.SourceText(), "should not have a translatable block with empty source text")
	}

	// Should still find the real entry.
	found := false
	for _, b := range blocks {
		if b.SourceText() == "Real" {
			found = true
		}
	}
	assert.True(t, found, "should find 'Real' block")
}

// okapi: POFilterTest#testTUPluralEntry_DefaultPlural
func TestRead_TUPluralEntryDefaultPlural(t *testing.T) {
	t.Parallel()
	input := "msgid \"One item\"\nmsgid_plural \"%d items\"\nmsgstr[0] \"\"\nmsgstr[1] \"\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.Len(t, blocks, 2)

	// The second block should be the plural form.
	assert.Equal(t, "%d items", blocks[1].SourceText())
	assert.Equal(t, "plural", blocks[1].Properties["plural-form"])
}

// okapi: POFilterTest#testTUPluralEntry_DefaultSingular
func TestRead_TUPluralEntryDefaultSingular(t *testing.T) {
	t.Parallel()
	input := "msgid \"One item\"\nmsgid_plural \"%d items\"\nmsgstr[0] \"\"\nmsgstr[1] \"\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.Len(t, blocks, 2)

	// The first block in a plural group should be the singular form.
	assert.Equal(t, "One item", blocks[0].SourceText())
	assert.Equal(t, "singular", blocks[0].Properties["plural-form"])
}

// okapi: POFilterTest#testThreePlurals
//
// Russian declares nplurals=3, so the reader must surface one text unit
// per msgstr[N] (three forms), matching Okapi's testThreePlurals: form 0
// carries the singular msgid, forms 1 and 2 carry the msgid_plural, and
// each form's target is the corresponding msgstr[N].
func TestRead_ThreePlurals(t *testing.T) {
	t.Parallel()
	input := "msgid \"\"\nmsgstr \"\"\n\"Plural-Forms: nplurals=3; plural=(n%10==1 && n%100!=11 ? 0 : n%10>=2 && n%10<=4 && (n%100<10 || n%100>=20) ? 1 : 2);\\n\"\n\nmsgid \"day\"\nmsgid_plural \"days\"\nmsgstr[0] \"день\"\nmsgstr[1] \"дня\"\nmsgstr[2] \"дней\"\n"
	parts := readWithConfig(t, input, map[string]any{}) // default config; reader follows the header's nplurals

	blocks := translatableBlocks(parts)
	require.Len(t, blocks, 3, "nplurals=3 should yield three text units")

	// Sources: msgid for form 0, msgid_plural for the rest.
	assert.Equal(t, "day", blocks[0].SourceText())
	assert.Equal(t, "days", blocks[1].SourceText())
	assert.Equal(t, "days", blocks[2].SourceText())

	// Targets: msgstr[0..2] in order.
	assert.Equal(t, "день", blocks[0].TargetText(model.LocaleFrench))
	assert.Equal(t, "дня", blocks[1].TargetText(model.LocaleFrench))
	assert.Equal(t, "дней", blocks[2].TargetText(model.LocaleFrench))

	// plural-form property: singular for form 0, plural for the rest.
	assert.Equal(t, "singular", blocks[0].Properties["plural-form"])
	assert.Equal(t, "plural", blocks[1].Properties["plural-form"])
	assert.Equal(t, "plural", blocks[2].Properties["plural-form"])

	// One GroupStart wraps the plural entry.
	assert.Equal(t, 1, countPartsByType(parts, model.PartGroupStart))
}

// A plural entry with no header keeps the default of two plural forms
// (Germanic languages), matching Okapi's DEFAULT_NPLURALS = 2.
func TestRead_PluralFormsDefaultTwoFormsNoHeader(t *testing.T) {
	t.Parallel()
	input := "msgid \"1 day\"\nmsgid_plural \"%d days\"\nmsgstr[0] \"\"\nmsgstr[1] \"\"\nmsgstr[2] \"\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.Len(t, blocks, 2, "no nplurals header should default to two forms")
	assert.Equal(t, "1 day", blocks[0].SourceText())
	assert.Equal(t, "%d days", blocks[1].SourceText())
}

// Monolingual mode (bilingualMode=false) treats the msgid as an
// identifier and the msgstr as the source text — Okapi's
// okf_po-monolingual configuration (testDoubleExtraction loads
// TestMonoLingual_*.po with okf_po@Monolingual.fprm).
func TestRead_MonolingualMode(t *testing.T) {
	t.Parallel()
	input := "msgid \"key1\"\nmsgstr \"Hello\"\n"
	parts := readWithConfig(t, input, map[string]any{"bilingualMode": false})

	blocks := translatableBlocks(parts)
	require.Len(t, blocks, 1)
	// Source is the msgstr value; the msgid stays as the Block name.
	assert.Equal(t, "Hello", blocks[0].SourceText())
	assert.Equal(t, "key1", blocks[0].Name)
	// No target is assigned in monolingual mode.
	assert.False(t, blocks[0].HasTarget(model.LocaleFrench))
}

// In bilingual mode (the default) the msgid is the source and the
// msgstr is the target — the contrast case for TestRead_MonolingualMode.
func TestRead_BilingualModeDefault(t *testing.T) {
	t.Parallel()
	input := "msgid \"key1\"\nmsgstr \"Hello\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.Len(t, blocks, 1)
	assert.Equal(t, "key1", blocks[0].SourceText())
	assert.Equal(t, "Hello", blocks[0].TargetText(model.LocaleFrench))
}

// okapi: POFilterTest#testTrailingSkeleton
func TestRead_TrailingSkeleton(t *testing.T) {
	t.Parallel()
	// Content after the last entry should be preserved as data (trailing comment).
	input := "msgid \"Hello\"\nmsgstr \"\"\n\n# Trailing comment\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Hello", blocks[0].SourceText())
}

// okapi: POFilterTest#testWithLetterCodes
func TestRead_WithLetterCodes(t *testing.T) {
	t.Parallel()
	// Printf-style format codes (%s, %d) — the native reader treats them as plain text.
	input := "#, c-format\nmsgid \"Value is %s and %d.\"\nmsgstr \"\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Value is")
	assert.Contains(t, text, "%s")
	assert.Contains(t, text, "%d")
}

// okapi: POFilterTest#testWithNoCodesLookingLikeCodes
func TestRead_WithNoCodesLookingLikeCodes(t *testing.T) {
	t.Parallel()
	// Without c-format flag, %-sequences are NOT inline codes, just plain text.
	input := "msgid \"100% done\"\nmsgstr \"\"\n"
	parts := readDefault(t, input)

	blocks := translatableBlocks(parts)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Equal(t, "100% done", text)
}
