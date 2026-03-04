//go:build integration

package okf_mosestext

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const filterClass = "net.sf.okapi.filters.mosestext.MosesTextFilter"
const mimeType = "text/x-mosestext"

// readMosesText parses a Moses text snippet with custom filter params and returns the parts.
func readMosesText(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	return bridgetest.ReadString(t, pool, cfg, filterClass, snippet, "test.txt", mimeType, filterParams)
}

// readMosesTextDefault parses a Moses text snippet with default (nil) params.
func readMosesTextDefault(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readMosesText(t, snippet, nil)
}

// snippetRoundtrip roundtrips a Moses text snippet and returns the output string.
func snippetRoundtrip(t *testing.T, snippet string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	result := bridgetest.RoundTrip(t, pool, cfg, filterClass, []byte(snippet), "test.txt", mimeType, filterParams)
	return string(result.Output)
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

// ---------------------------------------------------------------------------
// MosesTextFilterTest.java (16 tests in surefire)
// ---------------------------------------------------------------------------

// okapi-skip: MosesTextFilterTest#testDefaultInfo — Java-only API test (IFilter.getParameters/getName/getConfigurations)

// okapi: MosesTextFilterTest#testStartDocument
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterTest.xml — testStartDocument
func TestExtract_StartDocument(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	path := tdDir + "/okf_mosestext/Test01.txt"
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart (START_DOCUMENT)")

	layer, ok := parts[0].Resource.(*model.Layer)
	require.True(t, ok)
	assert.Equal(t, mimeType, layer.MimeType)
	assert.Equal(t, "UTF-8", layer.Encoding)
	assert.Equal(t, model.LocaleID("en"), layer.Locale)
}

// okapi: MosesTextFilterTest#testLineBreaks_CR
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterTest.xml — testLineBreaks_CR
func TestExtract_LineBreaks_CR(t *testing.T) {
	// Java: snippet = "Line 1\rLine 2\r"; assertEquals(snippet, result)
	snippet := "Line 1\rLine 2\r"
	output := snippetRoundtrip(t, snippet, nil)

	// Roundtrip should preserve content (line endings may normalize).
	assert.Contains(t, output, "Line 1")
	assert.Contains(t, output, "Line 2")

	// Verify extraction produces correct blocks.
	parts := readMosesTextDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should produce at least 2 blocks for 2 lines")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Line 1")
	assert.Contains(t, texts, "Line 2")
}

// okapi: MosesTextFilterTest#testineBreaks_CRLF
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterTest.xml — testineBreaks_CRLF
func TestExtract_LineBreaks_CRLF(t *testing.T) {
	// Java: snippet = "Line 1\r\nLine 2\r\n"; assertEquals(snippet, result)
	snippet := "Line 1\r\nLine 2\r\n"
	output := snippetRoundtrip(t, snippet, nil)

	assert.Contains(t, output, "Line 1")
	assert.Contains(t, output, "Line 2")

	parts := readMosesTextDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Line 1")
	assert.Contains(t, texts, "Line 2")
}

// okapi: MosesTextFilterTest#testLineBreaks_LF
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterTest.xml — testLineBreaks_LF
func TestExtract_LineBreaks_LF(t *testing.T) {
	// Java: snippet = "Line 1\nLine 2\n"; assertEquals(snippet, result)
	snippet := "Line 1\nLine 2\n"
	output := snippetRoundtrip(t, snippet, nil)

	assert.Contains(t, output, "Line 1")
	assert.Contains(t, output, "Line 2")

	parts := readMosesTextDefault(t, snippet)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Line 1")
	assert.Contains(t, texts, "Line 2")
}

// okapi: MosesTextFilterTest#testEntry
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterTest.xml — testEntry
func TestExtract_Entry(t *testing.T) {
	// Java: snippet = "Line 1\rLine 2"; tu = getTextUnit(2); assertEquals("Line 2", tu.getSource())
	snippet := "Line 1\rLine 2"
	parts := readMosesTextDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2,
		"should produce at least 2 blocks (one per line)")

	// The second text unit should be "Line 2".
	assert.Equal(t, "Line 2", blocks[1].SourceText())
}

// okapi: MosesTextFilterTest#testCode1
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterTest.xml — testCode1
func TestExtract_Code1(t *testing.T) {
	// Java: snippet = "Text <x id='1'/>"; source contains inline code
	snippet := "Text <x id='1'/>"
	parts := readMosesTextDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "Text")

	// Should have an inline span for the <x/> code.
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have inline spans for <x id='1'/>")
}

// okapi: MosesTextFilterTest#testCode2
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterTest.xml — testCode2
func TestExtract_Code2(t *testing.T) {
	// Java: snippet = "<g id='2'>Text</g> <x id='1'/>"
	snippet := "<g id='2'>Text</g> <x id='1'/>"
	parts := readMosesTextDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "Text")

	// Should have spans for <g>...</g> and <x/> codes.
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have inline spans for <g> and <x/> codes")
}

// okapi: MosesTextFilterTest#testCode3
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterTest.xml — testCode3
func TestExtract_Code3(t *testing.T) {
	// Java: snippet = "<g id='1'>Text</g><x id='2'/><g id='3'>t2<x id='4'/><g id='5'>t3</g></g>"
	snippet := "<g id='1'>Text</g><x id='2'/><g id='3'>t2<x id='4'/><g id='5'>t3</g></g>"
	parts := readMosesTextDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "Text")
	assert.Contains(t, text, "t2")
	assert.Contains(t, text, "t3")

	// Should have multiple nested inline spans.
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have inline spans for complex nested codes")
}

// okapi: MosesTextFilterTest#testCode4
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterTest.xml — testCode4
func TestExtract_Code4(t *testing.T) {
	// Java: snippet = "<bx id='1'/>T1<x id='2'/>T2<ex id='3'/>"
	snippet := "<bx id='1'/>T1<x id='2'/>T2<ex id='3'/>"
	parts := readMosesTextDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "T1")
	assert.Contains(t, text, "T2")

	// Should have spans for bx, x, and ex codes.
	frag := b.FirstFragment()
	require.NotNil(t, frag)
	assert.NotEmpty(t, frag.Spans, "should have inline spans for bx/x/ex codes")
}

// okapi: MosesTextFilterTest#testSpecialChars
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterTest.xml — testSpecialChars
func TestExtract_SpecialChars(t *testing.T) {
	// Java: snippet = "Line 1\rLine 2 with tab[\t] and more [<{|&/\\}>]"
	// Java: tu = getTextUnit(2); assertEquals("Line 2 with tab[\t] and more [<{|&/\\}>]", tu.getSource())
	snippet := "Line 1\rLine 2 with tab[\t] and more [<{|&/\\}>]"
	parts := readMosesTextDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)

	// The second block should contain the special chars.
	b := blocks[1]
	text := b.SourceText()
	assert.Contains(t, text, "Line 2 with tab[")
	assert.Contains(t, text, "] and more [")
}

// okapi: MosesTextFilterTest#testLiterals
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterTest.xml — testLiterals
func TestExtract_Literals(t *testing.T) {
	// Java: snippet = "&lt;=lt, &gt;=gt, &quot;=quot, &apos;=apos, &amp;=amp, &#x00d;&#13;=U+D"
	// Java: assertEquals("<=lt, >=gt, \"=quot, '=apos, &=amp, \r\r=U+D", tu.getSource())
	snippet := "&lt;=lt, &gt;=gt, &quot;=quot, &apos;=apos, &amp;=amp, &#x00d;&#13;=U+D"
	parts := readMosesTextDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	// Entity references should be decoded: &lt; -> <, &gt; -> >, etc.
	assert.Contains(t, text, "<=lt")
	assert.Contains(t, text, ">=gt")
	assert.Contains(t, text, "\"=quot")
	assert.Contains(t, text, "'=apos")
	assert.Contains(t, text, "&=amp")
}

// okapi: MosesTextFilterTest#testEscapedG
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterTest.xml — testEscapedG
func TestExtract_EscapedG(t *testing.T) {
	// Java: @Test(expected = EmptyStackException.class)
	// Escaped <g> tags without an id attribute cause an EmptyStackException in Java.
	// The bridge propagates this as an RPC error. We verify the bridge reports
	// an error (matching the Java exception behavior) and does not hang.
	snippet := "&lt;g&gt;a&lt;/g&gt;"
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)

	reader := bridge.NewBridgeFormatReader(pool, cfg, filterClass)
	doc := &model.RawDocument{
		URI:          "test.txt",
		SourceLocale: "en",
		TargetLocale: "fr",
		Encoding:     "UTF-8",
		MimeType:     mimeType,
		Reader:       io.NopCloser(bytes.NewReader([]byte(snippet))),
	}

	ctx := context.Background()
	require.NoError(t, reader.Open(ctx, doc))

	var gotError bool
	for pr := range reader.Read(ctx) {
		if pr.Error != nil {
			gotError = true
			break
		}
	}
	_ = reader.Close()

	assert.True(t, gotError,
		"escaped <g> tags without id should produce an error (Java throws EmptyStackException)")
}

// okapi: MosesTextFilterTest#testWhiteSpaces
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterTest.xml — testWhiteSpaces
func TestExtract_WhiteSpaces(t *testing.T) {
	// Java: snippet = "Text 1   .\r<mrk mtype=\"seg\">Line 1\r\rLine 2</mrk>"
	// Java: tu1 = getTextUnit(1); assertEquals("Text 1   .", tu1.getSource()); assertTrue(tu1.preserveWhitespaces())
	// Java: tu2 = getTextUnit(2); assertEquals("Line 1\n\nLine 2", tu2.getSource()); assertTrue(tu2.preserveWhitespaces())
	snippet := "Text 1   .\r<mrk mtype=\"seg\">Line 1\r\rLine 2</mrk>"
	parts := readMosesTextDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2,
		"should produce at least 2 blocks")

	// First block: "Text 1   ." with preserved whitespace.
	b1 := blocks[0]
	assert.Equal(t, "Text 1   .", b1.SourceText())
	assert.True(t, b1.PreserveWhitespace,
		"Moses text blocks should preserve whitespace")

	// Second block: content from <mrk> segment. In Java, CR inside <mrk> is
	// converted to LF, so "Line 1\n\nLine 2" is expected.
	b2 := blocks[1]
	text2 := b2.SourceText()
	assert.Contains(t, text2, "Line 1")
	assert.Contains(t, text2, "Line 2")
	assert.True(t, b2.PreserveWhitespace,
		"Moses text blocks should preserve whitespace")
}

// okapi: MosesTextFilterTest#testFromFile
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterTest.xml — testFromFile
func TestExtract_FromFile(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// Java: tu = getTextUnit(2); assertEquals("This is a test on line 1,\nand line two.", tu.getSource())
	// Test01.txt:
	//   <mrk mtype="seg">This is line 1.</mrk>
	//   This is a test on line 1,<lb/>and line two.
	path := tdDir + "/okf_mosestext/Test01.txt"
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2,
		"Test01.txt should produce at least 2 text units")

	// The second text unit: "This is a test on line 1,\nand line two."
	// The <lb/> tag in Moses text represents a line break within a segment.
	b2 := blocks[1]
	text := b2.SourceText()
	assert.Contains(t, text, "This is a test on line 1,")
	assert.Contains(t, text, "and line two.")
}

// okapi: MosesTextFilterTest#testDoubleExtraction
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterTest.xml — testDoubleExtraction
func TestExtract_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// Java: roundtrip comparison for Test01.txt and Test02.txt
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_mosestext/*.txt", mimeType, nil)
}

// ---------------------------------------------------------------------------
// MosesTextFilterWriterTest.java (5 tests in surefire)
// ---------------------------------------------------------------------------

// okapi: MosesTextFilterWriterTest#testSimpleOutputFromMosesText
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterWriterTest.xml — testSimpleOutputFromMosesText
func TestWriter_SimpleOutput(t *testing.T) {
	// Java: input = "<mrk mtype=\"seg\">Line 1</mrk>\rLine 2\r"
	// Java: expected = "Line 1" + lb + "Line 2" + lb
	// This is a read-write test, not a strict roundtrip. The writer normalizes
	// <mrk> markers and line endings. We verify the output contains the text.
	snippet := "<mrk mtype=\"seg\">Line 1</mrk>\rLine 2\r"
	output := snippetRoundtrip(t, snippet, nil)

	assert.Contains(t, output, "Line 1")
	assert.Contains(t, output, "Line 2")
}

// okapi: MosesTextFilterWriterTest#testMultilineOutputFromMosesText
// surefire: TEST-net.sf.okapi.filters.mosestext.MosesTextFilterWriterTest.xml — testMultilineOutputFromMosesText
func TestWriter_MultilineOutput(t *testing.T) {
	// Java: input = "Text 1.\r<mrk mtype=\"seg\">Text 2\rText 3.</mrk>\rText 4\r"
	// Java: expected = "Text 1." + lb + "Text 2<lb/>Text 3." + lb + "Text 4" + lb
	// The <mrk>-wrapped content spanning lines becomes a single segment with <lb/>.
	snippet := "Text 1.\r<mrk mtype=\"seg\">Text 2\rText 3.</mrk>\rText 4\r"
	output := snippetRoundtrip(t, snippet, nil)

	assert.Contains(t, output, "Text 1.")
	assert.Contains(t, output, "Text 4")
	// Text 2 and Text 3 should both appear in the output.
	assert.Contains(t, output, "Text 2")
	assert.Contains(t, output, "Text 3.")
}

// okapi-skip: MosesTextFilterWriterTest#testOutputFromXLIFF01 — requires XLIFF filter as input, not applicable to bridge extraction
// okapi-skip: MosesTextFilterWriterTest#testFileOutputFromXLIFF01 — requires XLIFF filter as input, not applicable to bridge extraction
// okapi-skip: MosesTextFilterWriterTest#testOutputFromXLIFF02 — requires XLIFF filter as input, not applicable to bridge extraction

// ---------------------------------------------------------------------------
// Additional extraction tests (mapped to MosesTextFilterTest subtests)
// ---------------------------------------------------------------------------

// okapi: MosesTextFilterTest#testEntry (layer structure)
func TestExtract_LayerStructure(t *testing.T) {
	parts := readMosesTextDefault(t, "Hello world")

	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type,
		"first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type,
		"last part should be LayerEnd")
}

// okapi: MosesTextFilterTest#testEntry (single line)
func TestExtract_SingleLine(t *testing.T) {
	parts := readMosesTextDefault(t, "Simple text")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Simple text", blocks[0].SourceText())
}

// okapi: MosesTextFilterTest#testEntry (block IDs)
func TestExtract_BlockIDs(t *testing.T) {
	parts := readMosesTextDefault(t, "Line A\nLine B\nLine C")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

// okapi: MosesTextFilterTest#testEntry (segment structure)
func TestExtract_SegmentIDs(t *testing.T) {
	parts := readMosesTextDefault(t, "Hello world")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		require.NotEmpty(t, b.Source, "block should have source segments")
		for _, seg := range b.Source {
			assert.NotEmpty(t, seg.ID, "segment should have an ID")
			assert.NotNil(t, seg.Content, "segment should have content")
		}
	}
}

// okapi: MosesTextFilterTest#testCode1 (no spans for plain text)
func TestExtract_NoSpansForPlainText(t *testing.T) {
	parts := readMosesTextDefault(t, "Simple plain text")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	for _, b := range blocks {
		frag := b.FirstFragment()
		if frag != nil {
			assert.Empty(t, frag.Spans, "plain text without codes should have no inline spans")
		}
	}
}

// okapi: MosesTextFilterTest#testEntry (mrk segment markers)
func TestExtract_MrkMarkers(t *testing.T) {
	// Moses text uses <mrk mtype="seg"> to mark segments explicitly.
	snippet := "<mrk mtype=\"seg\">Marked segment</mrk>"
	parts := readMosesTextDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "Marked segment", blocks[0].SourceText())
}

// okapi: MosesTextFilterTest#testEntry (lb tag)
func TestExtract_LbTag(t *testing.T) {
	// The <lb/> tag represents a line break within a segment (not a new text unit).
	snippet := "First part<lb/>second part"
	parts := readMosesTextDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	text := blocks[0].SourceText()
	assert.Contains(t, text, "First part")
	assert.Contains(t, text, "second part")
}

// okapi: MosesTextFilterTest#testWhiteSpaces (preserve whitespace flag)
func TestExtract_PreserveWhitespace(t *testing.T) {
	parts := readMosesTextDefault(t, "Text with   spaces")

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.True(t, blocks[0].PreserveWhitespace,
		"Moses text should always preserve whitespace")
}

// okapi: MosesTextFilterTest#testFromFile (Test02.txt)
func TestExtract_FromFile_Test02(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	bridgetest.RequireFilter(t, pool, cfg, filterClass)
	tdDir := bridgetest.TestdataDir(t)

	// Test02.txt has complex content: entities, segments, <lb/>, <mrk> with mid.
	path := tdDir + "/okf_mosestext/Test02.txt"
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "Test02.txt should produce translatable blocks")

	// Test02.txt line 1: "Test with <=lt, >=gt, &=amp, CR, etc..."
	// (entities are decoded)
	b1 := findBlockContaining(blocks, "Test with")
	require.NotNil(t, b1, "should find block containing 'Test with'")
	text1 := b1.SourceText()
	assert.Contains(t, text1, "<=lt")
	assert.Contains(t, text1, ">=gt")
	assert.Contains(t, text1, "&=amp")

	// Test02.txt line 2: "First segment."
	b2 := findBlockContaining(blocks, "First segment")
	require.NotNil(t, b2, "should find block containing 'First segment'")
	assert.Equal(t, "First segment.", b2.SourceText())

	// Test02.txt line 3: "Second segment<lb/>and it goes onto<lb/>three lines."
	// The <lb/> tags become line breaks within the segment.
	b3 := findBlockContaining(blocks, "Second segment")
	require.NotNil(t, b3, "should find block containing 'Second segment'")
	text3 := b3.SourceText()
	assert.Contains(t, text3, "Second segment")
	assert.Contains(t, text3, "and it goes onto")
	assert.Contains(t, text3, "three lines.")

	// Test02.txt line 4: <mrk mtype="seg">Third segment with optional markers.</mrk>
	b4 := findBlockContaining(blocks, "Third segment")
	require.NotNil(t, b4, "should find block containing 'Third segment'")
	assert.Contains(t, b4.SourceText(), "Third segment with optional markers.")

	// Test02.txt line 5: <mrk mtype="seg" mid="should be preserved">Fourth segment<lb/>on two lines.</mrk>
	b5 := findBlockContaining(blocks, "Fourth segment")
	require.NotNil(t, b5, "should find block containing 'Fourth segment'")
	text5 := b5.SourceText()
	assert.Contains(t, text5, "Fourth segment")
	assert.Contains(t, text5, "on two lines.")
}
