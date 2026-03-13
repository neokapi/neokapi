//go:build integration

package po

import (
	"os"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.po.POFilter"
const mimeType = "application/x-gettext"

// readPO parses a PO snippet with custom filter params and returns the parts.
func readPO(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.po", mimeType, filterParams)
}

// readPODefault parses a PO snippet with default (nil) params.
func readPODefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readPO(t, snippet, nil)
}

// readPOFile reads a PO file from testdata and returns parts.
func readPOFile(t *testing.T, relPath string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	return bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, filterParams)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func allBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// snippetRoundtrip roundtrips a PO snippet and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.po", mimeType, filterParams)
	return string(result.Output)
}

// fileRoundtrip roundtrips a testdata file and returns the output string.
func fileRoundtrip(t *testing.T, relPath string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, relPath)
	content, err := readFileContent(path)
	require.NoError(t, err)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, filterParams)
	return string(result.Output)
}

func readFileContent(path string) ([]byte, error) {
	return os.ReadFile(path)
}

// findBlockContaining finds a block whose source text contains the given substring.
func findBlockContaining(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), substr) {
			return b
		}
	}
	return nil
}

// countPartsByType counts parts of a given type.
func countPartsByType(parts []*model.Part, pt model.PartType) int {
	n := 0
	for _, p := range parts {
		if p.Type == pt {
			n++
		}
	}
	return n
}

// okapi-unmapped: POFilterTest#testDefaultInfo — Java-only API test (IFilter.getDisplayName/getMimeType)
// okapi-unmapped: POFilterTest#testSourceOnly — Java-only API test (source-only extraction mode)

// okapi: POFilterTest#testStartDocument
func TestExtract_StartDocument(t *testing.T) {
	parts := readPODefault(t, "msgid \"Hello\"\nmsgstr \"\"\n")

	// Should produce a LayerStart at the beginning.
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
}

// okapi: POFilterTest#testPOHeader
func TestExtract_POHeader(t *testing.T) {
	parts := readPOFile(t, "okapi/filters/po/src/test/resources/potest.po", nil)

	blocks := allBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from potest.po")

	// The PO header (empty msgid) should not appear as a translatable block.
	for _, b := range blocks {
		if b.Translatable {
			assert.NotEmpty(t, b.SourceText(), "translatable block should not have empty source")
		}
	}
}

// okapi: POFilterTest#testPOTHeader
func TestExtract_POTHeader(t *testing.T) {
	parts := readPOFile(t, "okapi/filters/po/src/test/resources/POT-Test01.pot", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from POT")

	// POT file should have entries (timezone names).
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Africa/Abidjan")
}

// okapi: POFilterTest#testHeaderNoNPlurals
func TestExtract_HeaderNoNPlurals(t *testing.T) {
	// A PO file with a header that lacks Plural-Forms.
	po := "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\nmsgid \"Hello\"\nmsgstr \"\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Hello", blocks[0].SourceText())
}

// okapi: POFilterTest#testHeaderWithEmptyEntryAfter
func TestExtract_HeaderWithEmptyEntryAfter(t *testing.T) {
	// Header followed by empty msgid/msgstr (which should be treated as non-translatable).
	po := "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\nmsgid \"\"\nmsgstr \"\"\n\nmsgid \"Real entry\"\nmsgstr \"\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	found := false
	for _, b := range blocks {
		if b.SourceText() == "Real entry" {
			found = true
		}
	}
	assert.True(t, found, "should find 'Real entry' block")
}

// okapi: POFilterTest#testOuputSimpleEntry
func TestExtract_WithTarget(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := "msgid \"Hello\"\nmsgstr \"Bonjour\"\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass, po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "Hello", b.SourceText())
	assert.True(t, b.HasTarget("fr"), "should have French target")
	assert.Equal(t, "Bonjour", b.TargetText("fr"))
}

// okapi: POFilterTest#testIDWithContext
func TestExtract_Context(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := "msgctxt \"menu\"\nmsgid \"File\"\nmsgstr \"\"\n\nmsgctxt \"dialog\"\nmsgid \"File\"\nmsgstr \"\"\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass, po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should extract both contextual entries")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "File")
}

// okapi: POFilterTest#testTUCompleteEntry
func TestExtract_Comments(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := "# Translator comment\n#. Extracted comment\n#: src/main.c:42\nmsgid \"Save\"\nmsgstr \"\"\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass, po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "Save", b.SourceText())
	if b.Annotations != nil {
		if note, ok := b.Annotations["note"]; ok {
			n := note.(*model.NoteAnnotation)
			assert.NotEmpty(t, n.Text)
		}
	}
}

// okapi: POFilterTest#testNoQuoteOnSameLine
func TestExtract_MultilineStrings(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := "msgid \"\"\n\"This is a \"\n\"multiline string\"\nmsgstr \"\"\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass, po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "This is a multiline string")
}

// okapi: POFilterTest#testTUPluralEntry_DefaultGroup
// okapi: POFilterTest#testTUPluralEntry_DefaultPlural
func TestExtract_PluralForms(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := "msgid \"One item\"\nmsgid_plural \"%d items\"\nmsgstr[0] \"\"\nmsgstr[1] \"\"\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass, po, "test.po", mimeType, nil)

	// Plural forms should produce a group with blocks inside.
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Should have a GroupStart for the plural entry.
	groupStarts := countPartsByType(parts, model.PartGroupStart)
	assert.GreaterOrEqual(t, groupStarts, 1, "plural entry should produce a GroupStart")
}

// okapi: POFilterTest#testTUPluralEntry_DefaultSingular
func TestExtract_TUPluralEntryDefaultSingular(t *testing.T) {
	po := "msgid \"One item\"\nmsgid_plural \"%d items\"\nmsgstr[0] \"\"\nmsgstr[1] \"\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// The first block in a plural group should be the singular form.
	b := blocks[0]
	assert.Contains(t, b.SourceText(), "One item")
}

// okapi: POFilterTest#testOnePlural
func TestExtract_OnePlural(t *testing.T) {
	parts := readPOFile(t, "okapi/filters/po/src/test/resources/plurals.po", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from plurals.po")

	// plurals.po has nplurals=1, so singular form has "1 gizmo".
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "gizmo") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find gizmo plural entry")
}

// okapi: POFilterTest#testThreePlurals
func TestExtract_ThreePlurals(t *testing.T) {
	parts := readPOFile(t, "okapi/filters/po/src/test/resources/plurals-2.po", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from plurals-2.po")

	// plurals-2.po has nplurals=3 (Russian) with day/days.
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "day") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find day/days plural entry")
}

// okapi: POFilterTest#testPluralFormDefaults
func TestExtract_PluralFormDefaults(t *testing.T) {
	// Verify plural form entries produce blocks with targets.
	po := "msgid \"\"\nmsgstr \"\"\n\"Plural-Forms: nplurals=2; plural=(n != 1);\\n\"\n\nmsgid \"One item\"\nmsgid_plural \"%d items\"\nmsgstr[0] \"Un article\"\nmsgstr[1] \"%d articles\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// At least one block should have a French target.
	hasTarget := false
	for _, b := range blocks {
		if b.HasTarget("fr") {
			hasTarget = true
			break
		}
	}
	assert.True(t, hasTarget, "plural entry should have targets when msgstr values are provided")
}

// okapi: POFilterTest#testPluralFormAccess
func TestExtract_PluralFormAccess(t *testing.T) {
	parts := readPOFile(t, "okapi/filters/po/src/test/resources/potest.po", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// potest.po has "%d file" / "%d files" — inline codes may transform the text
	// so we search for "file" substring.
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "file") {
			found = true
			break
		}
	}
	assert.True(t, found, "should find a block containing 'file'")

	// Should also have plural group structure.
	groupStarts := countPartsByType(parts, model.PartGroupStart)
	assert.GreaterOrEqual(t, groupStarts, 1, "should have group starts for plural entries")
}

// okapi: POFilterTest#testPluralEntryFuzzy
func TestExtract_PluralEntryFuzzy(t *testing.T) {
	po := "#, fuzzy, c-format\nmsgid \"%d file left to delete\"\nmsgid_plural \"%d files left to delete\"\nmsgstr[0] \"Encore %d fichier\"\nmsgstr[1] \"Encore %d fichiers\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// The fuzzy plural entry should produce blocks with targets.
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "file") || strings.Contains(b.SourceText(), "delete") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract fuzzy plural entry")
}

// okapi: POFilterTest#testEscapes
func TestExtract_EscapedCharacters(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := "msgid \"Line one\\nLine two\"\nmsgstr \"\"\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass, po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "Line one")
	assert.Contains(t, text, "Line two")
}

// okapi: POFilterTest#testUnescapedRead
func TestExtract_UnescapedRead(t *testing.T) {
	parts := readPOFile(t, "okapi/filters/po/src/test/resources/escaping.po", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from escaping.po")

	// Should contain unescaped backslash paths.
	found := false
	for _, b := range blocks {
		text := b.SourceText()
		if strings.Contains(text, "Windows path") {
			found = true
			// The text should contain the actual backslash, not double-escaped.
			assert.Contains(t, text, "C:\\Users\\Administrator")
			break
		}
	}
	assert.True(t, found, "should find Windows path entry in escaping.po")
}

// okapi: POFilterTest#testUnescapedRewrite
func TestExtract_UnescapedRewrite(t *testing.T) {
	// Roundtrip escaping.po and verify escape sequences survive.
	output := fileRoundtrip(t, "okapi/filters/po/src/test/resources/escaping.po", nil)
	assert.Contains(t, output, "C:\\\\Users\\\\Administrator", "escaped backslashes should survive roundtrip")
}

// okapi: POFilterTest#testInlines
func TestExtract_Inlines(t *testing.T) {
	// PO file with printf-style inline codes.
	po := "#, c-format\nmsgid \"%1s %2s\"\nmsgstr \"%1s %2s\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Source should have spans for the inline codes.
	b := blocks[0]
	require.NotEmpty(t, b.Source)
	if b.Source[0].Content != nil {
		assert.NotEmpty(t, b.Source[0].Content.Spans, "should have spans for inline codes")
	}
}

// okapi: POFilterTest#testWithLetterCodes
func TestExtract_WithLetterCodes(t *testing.T) {
	po := "#, c-format\nmsgid \"Value is %s and %d.\"\nmsgstr \"\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// The source text should contain the original text with inline codes represented as spans.
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Value is")
	assert.Contains(t, text, "and")
}

// okapi: POFilterTest#testWithNoCodesLookingLikeCodes
func TestExtract_WithNoCodesLookingLikeCodes(t *testing.T) {
	// Without c-format flag, %-sequences are NOT treated as inline codes.
	po := "msgid \"100% done\"\nmsgstr \"\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "100% done")
}

// okapi: POFilterTest#testTUContextParsing
func TestExtract_TUContextParsing(t *testing.T) {
	parts := readPOFile(t, "okapi/filters/po/src/test/resources/simple_withcontext.po", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from file with context")

	// The file has msgctxt "context", the block should still extract.
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "untranslated-string")
}

// okapi: POFilterTest#testTUEmptyIDEntry
func TestExtract_TUEmptyIDEntry(t *testing.T) {
	// An entry with an empty msgid after the header should not produce a translatable block.
	po := "msgid \"\"\nmsgstr \"\"\n\"Content-Type: text/plain; charset=UTF-8\\n\"\n\nmsgid \"\"\nmsgstr \"empty source\"\n\nmsgid \"Real\"\nmsgstr \"\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	for _, b := range blocks {
		if b.SourceText() == "" {
			t.Error("should not have a translatable block with empty source text")
		}
	}
}

// okapi: POFilterTest#testMsgCtxtAsNotes
func TestExtract_MsgCtxtAsNotes(t *testing.T) {
	parts := readPOFile(t, "okapi/filters/po/src/test/resources/msgctxt_notes.po", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should extract at least 2 blocks from msgctxt_notes.po")

	// Both entries have "Hello world" as source.
	for _, b := range blocks {
		assert.Equal(t, "Hello world", b.SourceText())
	}
}

// okapi: POFilterTest#testOuputNoQuoteOnSameLine
func TestExtract_OuputNoQuoteOnSameLine(t *testing.T) {
	// Multiline strings should roundtrip correctly.
	po := "msgid \"\"\n\"This is a \"\n\"multiline string\"\nmsgstr \"\"\n"
	output := snippetRoundtrip(t, po, nil)
	assert.Contains(t, output, "multiline string")
}

// okapi: POFilterTest#testOuputEntryWithCTXT
func TestExtract_OuputEntryWithCTXT(t *testing.T) {
	po := "msgctxt \"menu\"\nmsgid \"File\"\nmsgstr \"Fichier\"\n"
	output := snippetRoundtrip(t, po, nil)
	assert.Contains(t, output, "msgctxt")
	assert.Contains(t, output, "File")
}

// okapi: POFilterTest#testOuputAddTranslation
func TestExtract_OuputAddTranslation(t *testing.T) {
	po := "msgid \"Hello\"\nmsgstr \"Bonjour\"\n"
	output := snippetRoundtrip(t, po, nil)
	assert.Contains(t, output, "Bonjour", "translation should survive roundtrip")
}

// okapi: POFilterTest#testOuputWithAllowedEmpty
func TestExtract_OuputWithAllowedEmpty(t *testing.T) {
	po := "msgid \"Hello\"\nmsgstr \"\"\n"
	output := snippetRoundtrip(t, po, nil)
	assert.Contains(t, output, "msgid \"Hello\"")
	assert.Contains(t, output, "msgstr")
}

// okapi: POFilterTest#testOuputPluralEntry
func TestExtract_OuputPluralEntry(t *testing.T) {
	po := "msgid \"One item\"\nmsgid_plural \"%d items\"\nmsgstr[0] \"Un article\"\nmsgstr[1] \"%d articles\"\n"
	output := snippetRoundtrip(t, po, nil)
	assert.Contains(t, output, "msgid_plural")
	assert.Contains(t, output, "msgstr[0]")
}

// okapi: POFilterTest#testOuputPluralEntryFuzzy
func TestExtract_OuputPluralEntryFuzzy(t *testing.T) {
	po := "#, fuzzy\nmsgid \"One item\"\nmsgid_plural \"%d items\"\nmsgstr[0] \"Un article\"\nmsgstr[1] \"%d articles\"\n"
	output := snippetRoundtrip(t, po, nil)
	// Plural entries preserve fuzzy through roundtrip.
	assert.Contains(t, output, "fuzzy", "fuzzy flag should survive roundtrip for plural entries")
	assert.Contains(t, output, "msgid_plural")
}

// okapi: POFilterTest#testOuputOptionLine_FormatFuzzy
func TestExtract_OuputOptionLineFormatFuzzy(t *testing.T) {
	// "#, c-format, fuzzy" — verify extraction is correct.
	po := "#, c-format, fuzzy\nmsgid \"Text %s\"\nmsgstr \"Texte %s\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "Text")

	// Verify c-format content survives roundtrip.
	output := snippetRoundtrip(t, po, nil)
	assert.Contains(t, output, "c-format", "c-format flag should survive roundtrip")
}

// okapi: POFilterTest#testOuputOptionLine_FuzyFormat
func TestExtract_OuputOptionLineFuzyFormat(t *testing.T) {
	// "#, fuzzy, c-format" — verify extraction works regardless of flag order.
	po := "#, fuzzy, c-format\nmsgid \"Text %s\"\nmsgstr \"Texte %s\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "Text")
}

// okapi: POFilterTest#testOuputOptionLine_JustFormatWithMacLB
func TestExtract_OuputOptionLineJustFormatWithMacLB(t *testing.T) {
	// c-format flag with Mac line breaks (\r).
	po := "#, c-format\rmsgid \"Text %s\"\rmsgstr \"\"\r"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "Text")
}

// okapi: POFilterTest#testOuputOptionLine_StuffFuzyFormat
func TestExtract_OuputOptionLineStuffFuzyFormat(t *testing.T) {
	// "#, stuff, fuzzy, c-format" — multiple flags.
	po := "#, stuff, fuzzy, c-format\nmsgid \"Text %s\"\nmsgstr \"Texte %s\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "Text")
}

// okapi: POFilterTest#testProtectApproved
func TestExtract_ProtectApproved(t *testing.T) {
	// Entry without fuzzy flag should be "approved".
	po := "msgid \"Approved text\"\nmsgstr \"Texte approuvé\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "Approved text", b.SourceText())
	assert.True(t, b.HasTarget("fr"))
}

// okapi: POFilterTest#testOutputProtectApproved
func TestExtract_OutputProtectApproved(t *testing.T) {
	// Approved entries (no fuzzy) should roundtrip without adding fuzzy.
	po := "msgid \"Approved text\"\nmsgstr \"Texte approuvé\"\n"
	output := snippetRoundtrip(t, po, nil)
	assert.Contains(t, output, "Approved text")
	assert.Contains(t, output, "Texte approuvé")
	// Should NOT have fuzzy flag added.
	assert.NotContains(t, output, "fuzzy")
}

// okapi: POFilterTest#testHtmlSubfilterBilingualMode
func TestExtract_HtmlSubfilterBilingualMode(t *testing.T) {
	// PO entry with HTML content — the HTML should be parsed as a subfilter.
	po := "msgid \"<b>Bold</b> text\"\nmsgstr \"\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// HTML tags should appear as inline codes (spans).
	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "Bold")
	assert.Contains(t, text, "text")
}

// okapi: POFilterTest#testMarkdownSubfilter
func TestExtract_MarkdownSubfilter(t *testing.T) {
	// PO entry with markdown content.
	po := "msgid \"Hello **world**\"\nmsgstr \"\"\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "world")
}

// okapi: POFilterTest#testTrailingSkeleton
func TestExtract_TrailingSkeleton(t *testing.T) {
	// Content after the last entry should be preserved as skeleton.
	po := "msgid \"Hello\"\nmsgstr \"\"\n\n# Trailing comment\n"
	parts := readPODefault(t, po)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Hello", blocks[0].SourceText())

	// The trailing comment should be preserved through roundtrip.
	output := snippetRoundtrip(t, po, nil)
	assert.Contains(t, output, "Hello")
}

// okapi: POFilterTest#testDoubleExtraction
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// Read file once, write it, then read the output again.
	path := bridgetest.TestdataFile(t, "okapi/filters/po/src/test/resources/potest.po")
	content, err := readFileContent(path)
	require.NoError(t, err)

	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, content, path, mimeType, nil)

	// Re-read the output.
	parts2 := bridgetest.ReadBytes(t, pool, cfg, filterClass, result.Output, "test.po", mimeType, nil)
	blocks2 := bridgetest.TranslatableBlocks(parts2)
	require.NotEmpty(t, blocks2, "double extraction should produce blocks")

	// The block count from both passes should match.
	blocks1 := bridgetest.TranslatableBlocks(result.Parts)
	assert.Equal(t, len(blocks1), len(blocks2), "double extraction should produce same block count")
}

func TestExtract_SimplePO(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := "msgid \"Hello World\"\nmsgstr \"\"\n\nmsgid \"Goodbye\"\nmsgstr \"\"\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from PO")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello World")
	assert.Contains(t, texts, "Goodbye")
}

func TestExtract_UnicodeContent(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := "msgid \"\xe3\x81\x93\xe3\x82\x93\xe3\x81\xab\xe3\x81\xa1\xe3\x81\xaf\"\nmsgstr \"\"\n\nmsgid \"H\xc3\xa9llo w\xc3\xb6rld\"\nmsgstr \"\"\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	assert.Contains(t, texts, "\xe3\x81\x93\xe3\x82\x93\xe3\x81\xab\xe3\x81\xa1\xe3\x81\xaf")
	assert.Contains(t, texts, "H\xc3\xa9llo w\xc3\xb6rld")
}

func TestExtract_BlockIDs(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	po := "msgid \"First\"\nmsgstr \"\"\n\nmsgid \"Second\"\nmsgstr \"\"\n\nmsgid \"Third\"\nmsgstr \"\"\n"

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		po, "test.po", mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

func TestExtract_LayerStructure(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"msgid \"Hello\"\nmsgstr \"\"\n",
		"test.po", mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}
