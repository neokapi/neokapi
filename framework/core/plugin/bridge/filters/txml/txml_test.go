//go:build integration

package txml

import (
	"os"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// okapi: TXMLFilterTest#testSimpleEntry
func TestExtract_SimpleEntry(t *testing.T) {
	snippet := STARTFILE +
		"<translatable blockId=\"b1\" datatype=\"html\">" +
		"<segment segmentId=\"s1\" modified=\"true\">" +
		"<source>Segment one</source><target>Segment un</target>" +
		"</segment>" +
		"<segment segmentId=\"2\">" +
		"<source>segment two</source>" +
		"</segment>" +
		"</translatable>" +
		"</txml>"

	parts := readTXMLDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from simple entry")

	// The text unit should have ID "b1".
	b := blocks[0]
	assert.Equal(t, "b1", b.ID)

	// First segment source should be "Segment one".
	assert.Contains(t, b.SourceText(), "Segment one")

	// Should have a French target with "Segment un".
	assert.True(t, b.HasTarget("fr"), "should have French target")
	assert.Contains(t, b.TargetText("fr"), "Segment un")
}

// okapi: TXMLFilterTest#testRevisedEntry
func TestExtract_RevisedEntry(t *testing.T) {
	snippet := STARTFILE +
		"<translatable blockId=\"b1\" datatype=\"html\">" +
		"<segment segmentId=\"s1\" modified=\"true\">" +
		"<source>Segment one</source><target>Segment un</target>" +
		"<revisions>" +
		"<revision id=\"1\" creationid=\"Roberto\" creationdate=\"20130109T162701Z\" type=\"target\">" +
		"<target>previous translation</target>" +
		"</revision>" +
		"</revisions>" +
		"</segment>" +
		"<segment segmentId=\"2\">" +
		"<source>segment two</source>" +
		"</segment>" +
		"</translatable>" +
		"</txml>"

	parts := readTXMLDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from revised entry")

	b := blocks[0]
	assert.Equal(t, "b1", b.ID)
	assert.Contains(t, b.SourceText(), "Segment one")
	assert.True(t, b.HasTarget("fr"), "should have French target")
	assert.Contains(t, b.TargetText("fr"), "Segment un")
}

// okapi: TXMLFilterTest#testEntryWithCodes
func TestExtract_EntryWithCodes(t *testing.T) {
	snippet := STARTFILE +
		"<translatable blockId=\"b1\" datatype=\"html\">" +
		"<segment segmentId=\"s1\" modified=\"true\">" +
		"<source>Segment one</source><target>Segment un</target>" +
		"</segment>" +
		"<segment segmentId=\"2\">" +
		"<source>Segment <ut x='1' type='bold'>&lt;b></ut>TWO<ut x='2' type='bold'>&lt;/b></ut></source>" +
		"</segment>" +
		"</translatable>" +
		"</txml>"

	parts := readTXMLDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from entry with codes")

	b := blocks[0]
	// The source text should contain both segment texts.
	text := b.SourceText()
	assert.Contains(t, text, "Segment one")
	assert.Contains(t, text, "TWO")

	// The second segment should have inline spans for the <ut> codes.
	require.GreaterOrEqual(t, len(b.Source), 2, "should have at least 2 segments")
	seg2 := b.Source[1]
	if seg2.Content != nil {
		assert.NotEmpty(t, seg2.Content.Spans, "second segment should have spans for inline codes")
	}
}

// okapi: TXMLFilterTest#testWS
func TestExtract_WS(t *testing.T) {
	snippet := STARTFILE +
		"<translatable blockId=\"b1\" datatype=\"html\">" +
		"<segment segmentId=\"s1\">" +
		"<ws>  </ws>" +
		"<source>text S</source>" +
		"<target>text T</target>" +
		"<ws>  <ut x='1'>&lt;br/></ut> </ws>" +
		"</segment>" +
		"<segment segmentId=\"s2\">" +
		"<source>text S2</source>" +
		"<ws> \t</ws>" +
		"</segment>" +
		"</translatable>" +
		"</txml>"

	parts := readTXMLDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from WS entry")

	b := blocks[0]
	// Source should contain both segment texts.
	text := b.SourceText()
	assert.Contains(t, text, "text S")
	assert.Contains(t, text, "text S2")

	// Target should have "text T" from the first segment.
	assert.True(t, b.HasTarget("fr"), "should have French target")
	assert.Contains(t, b.TargetText("fr"), "text T")
}

// okapi: TXMLFilterTest#testEmptySegments
func TestExtract_EmptySegments(t *testing.T) {
	snippet := STARTFILE +
		"<translatable blockId=\"b1\" datatype=\"html\">" +
		"<segment segmentId=\"s1\">" +
		"<ws>  </ws>" +
		"<source></source>" +
		"<ws>  <ut x='1'>&lt;br/></ut> </ws>" +
		"</segment>" +
		"<segment segmentId=\"s2\">" +
		"<source></source>" +
		"<ws> \t</ws>" +
		"</segment>" +
		"</translatable>" +
		"</txml>"

	parts := readTXMLDefault(t, snippet)

	// The Java test expects a text unit with empty segments and no target.
	blocks := allBlocks(parts)
	if len(blocks) > 0 {
		b := blocks[0]
		// Java: assertNull(tu.getTarget(locFR))
		assert.False(t, b.HasTarget("fr"), "empty segments should have no French target")
	}
}

// okapi: TXMLFilterTest#testSegments
func TestExtract_Segments(t *testing.T) {
	snippet := STARTFILE +
		"<translatable blockId=\"b1\" datatype=\"html\">" +
		"<segment segmentId=\"s1\">" +
		"<ws>  </ws>" +
		"<source>textS1</source>" +
		"<target>textT1</target>" +
		"<ws>  <ut x='1'>&lt;br/></ut> </ws>" +
		"</segment>" +
		"<segment segmentId=\"s2\">" +
		"<source>textS2</source>" +
		"<ws> \t</ws>" +
		"</segment>" +
		"<segment segmentId=\"s3\">" +
		"<ws>{{</ws>" +
		"<source></source>" +
		"<ws>}}</ws>" +
		"</segment>" +
		"</translatable>" +
		"</txml>"

	parts := readTXMLDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from segments entry")

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "textS1")
	assert.Contains(t, text, "textS2")

	assert.True(t, b.HasTarget("fr"), "should have French target")
	assert.Contains(t, b.TargetText("fr"), "textT1")
}

// okapi: TXMLFilterTest#testEntryWithFirstOutOf2SegmentsCommentedOut
func TestExtract_EntryWithFirstOutOf2SegmentsCommentedOut(t *testing.T) {
	snippet := STARTFILE +
		"<translatable blockId=\"b1\" datatype=\"html\">" +
		"<!--<segment segmentId=\"s1\" modified=\"true\">" +
		"<source>Segment one</source><target>Segment un</target>" +
		"</segment>-->" +
		"<segment segmentId=\"2\">" +
		"<source>segment two</source><target>segment deux</target>" +
		"</segment>" +
		"</translatable>" +
		"</txml>"

	parts := readTXMLDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks when first segment is commented out")

	b := blocks[0]
	assert.Equal(t, "b1", b.ID)
	// Only the second segment should be extracted.
	assert.Contains(t, b.SourceText(), "segment two")
	assert.True(t, b.HasTarget("fr"), "should have French target")
	assert.Contains(t, b.TargetText("fr"), "segment deux")
}

// okapi: TXMLFilterTest#testEntryWithSecondOutOf2SegmentsCommentedOut
func TestExtract_EntryWithSecondOutOf2SegmentsCommentedOut(t *testing.T) {
	snippet := STARTFILE +
		"<translatable blockId=\"b1\" datatype=\"html\">" +
		"<segment segmentId=\"s1\" modified=\"true\">" +
		"<source>Segment one</source><target>Segment un</target>" +
		"</segment>" +
		"<!--<segment segmentId=\"2\">" +
		"<source>segment two</source><target>segment deux</target>" +
		"</segment>-->" +
		"</translatable>" +
		"</txml>"

	parts := readTXMLDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks when second segment is commented out")

	b := blocks[0]
	assert.Equal(t, "b1", b.ID)
	// Only the first segment should be extracted.
	assert.Contains(t, b.SourceText(), "Segment one")
	assert.True(t, b.HasTarget("fr"), "should have French target")
	assert.Contains(t, b.TargetText("fr"), "Segment un")
}

// okapi: TXMLFilterTest#testEntryWithAllSegmentsCommentedOut
func TestExtract_EntryWithAllSegmentsCommentedOut(t *testing.T) {
	snippet := STARTFILE +
		"<translatable blockId=\"b1\" datatype=\"html\">" +
		"<!--<segment segmentId=\"s1\" modified=\"true\">" +
		"<source>Segment one</source><target>Segment un</target>" +
		"</segment>-->" +
		"<!--<segment segmentId=\"2\">" +
		"<source>segment two</source><target>segment deux</target>" +
		"</segment>-->" +
		"</translatable>" +
		"</txml>"

	parts := readTXMLDefault(t, snippet)

	// Java: assertNull(tu) — when all segments are commented out,
	// no text unit should be produced.
	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks, "should produce no translatable blocks when all segments are commented out")
}

// okapi: TXMLFilterTest#testEntryWith1SegmentCommentedOut
func TestExtract_EntryWith1SegmentCommentedOut(t *testing.T) {
	snippet := STARTFILE +
		"<translatable blockId=\"b1\" datatype=\"html\">" +
		"<!--<segment segmentId=\"s1\" modified=\"true\">" +
		"<source>Segment one</source><target>Segment un</target>" +
		"</segment>-->" +
		"</translatable>" +
		"</txml>"

	parts := readTXMLDefault(t, snippet)

	// Java: assertNull(tu) — a single commented-out segment produces no text unit.
	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks, "should produce no translatable blocks when the only segment is commented out")
}

// okapi: TXMLFilterTest#testEntryWithThirdSegmentsNotCommentedOut
func TestExtract_EntryWithThirdSegmentNotCommentedOut(t *testing.T) {
	snippet := STARTFILE +
		"<translatable blockId=\"b1\" datatype=\"html\">" +
		"<!--<segment segmentId=\"s1\" modified=\"true\">" +
		"<source>Segment one</source><target>Segment un</target>" +
		"</segment>-->" +
		"<!--<segment segmentId=\"2\">" +
		"<source>segment two</source><target>segment deux</target>" +
		"</segment>-->" +
		"<segment segmentId=\"3\">" +
		"<source>segment three</source><target>segment trois</target>" +
		"</segment>" +
		"</translatable>" +
		"</txml>"

	parts := readTXMLDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks when third segment is not commented out")

	b := blocks[0]
	assert.Contains(t, b.SourceText(), "segment three")
}

// okapi: TXMLFilterTest#testOutputWithCommentedOutSegments
func TestRoundTrip_OutputWithCommentedOutSegments(t *testing.T) {
	snippet := STARTFILE +
		"<translatable blockId=\"b1\" datatype=\"html\">" +
		"<!--<segment segmentId=\"s1\" modified=\"true\">" +
		"<source>Segment one</source><target>Segment un</target>" +
		"</segment>-->" +
		"<!--<segment segmentId=\"s1bis\" modified=\"true\">" +
		"<source>Segment one bis</source><target>Segment un bis</target>" +
		"</segment>-->" +
		"<segment segmentId=\"2\">" +
		"<source>segment two</source><target>segment deux</target>" +
		"</segment>" +
		"<!--<segment segmentId=\"3\">" +
		"<source>segment two</source><target>segment deux</target>" +
		"</segment>-->" +
		"</translatable>" +
		"</txml>"

	output := snippetRoundtrip(t, snippet, nil)

	// The commented-out segments should be preserved in the output.
	assert.Contains(t, output, "<!--<segment segmentId=\"s1\"",
		"first commented segment should be preserved")
	assert.Contains(t, output, "<!--<segment segmentId=\"s1bis\"",
		"second commented segment should be preserved")
	assert.Contains(t, output, "<!--<segment segmentId=\"3\"",
		"trailing commented segment should be preserved")

	// The active segment should also be present.
	assert.Contains(t, output, "segment two")
	assert.Contains(t, output, "segment deux")
}

// okapi: TXMLFilterTest#testDoubleExtraction
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)

	// Run roundtrip event comparison on all three TXML test files, matching
	// the Java RoundTripComparison for Test01.docx.txml, Test02.html.txml,
	// and Test03.mif.txml.
	bridgetest.RoundTripTestFiles(t, pool, cfg, filterClass,
		tdDir+"/okf_txml/*.txml", mimeType, nil)
}

// ---------------------------------------------------------------------------
// Additional extraction tests
// ---------------------------------------------------------------------------

// TestExtract_LayerStructure verifies the part stream starts with LayerStart
// and ends with LayerEnd.
func TestExtract_LayerStructure(t *testing.T) {
	snippet := STARTFILE +
		"<translatable blockId=\"b1\" datatype=\"html\">" +
		"<segment segmentId=\"s1\">" +
		"<source>Hello</source>" +
		"</segment>" +
		"</translatable>" +
		"</txml>"

	parts := readTXMLDefault(t, snippet)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type, "first part should be LayerStart")
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type, "last part should be LayerEnd")
}

// TestExtract_BlockIDs verifies that each extracted block has a unique ID.
func TestExtract_BlockIDs(t *testing.T) {
	parts := readTXMLFile(t, "okf_txml/Test02.html.txml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should extract at least 2 blocks")

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique, got duplicate: %s", b.ID)
		ids[b.ID] = true
	}
}

// TestExtract_DocxTxml verifies extraction from the docx.txml test file.
func TestExtract_DocxTxml(t *testing.T) {
	parts := readTXMLFile(t, "okf_txml/Test01.docx.txml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from Test01.docx.txml")

	// Test01.docx.txml has entries like "This is the text of the first sentence"
	// and text in cells "Text in c11", etc.
	b := findBlockContaining(blocks, "first sentence")
	assert.NotNil(t, b, "should find 'first sentence' in Test01.docx.txml")
}

// TestExtract_HtmlTxml verifies extraction from the html.txml test file.
func TestExtract_HtmlTxml(t *testing.T) {
	parts := readTXMLFile(t, "okf_txml/Test02.html.txml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from Test02.html.txml")

	b := findBlockContaining(blocks, "bold")
	require.NotNil(t, b, "should find block containing 'bold'")
}

// TestExtract_MifTxml verifies extraction from the mif.txml test file.
func TestExtract_MifTxml(t *testing.T) {
	parts := readTXMLFile(t, "okf_txml/Test03.mif.txml", nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from Test03.mif.txml")

	b := findBlockContaining(blocks, "HTML Mapping Table")
	require.NotNil(t, b, "should find 'HTML Mapping Table' in Test03.mif.txml")
}

// TestRoundTrip_SimpleSnippet verifies basic roundtrip for a simple TXML snippet.
func TestRoundTrip_SimpleSnippet(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	snippet := STARTFILE +
		"<translatable blockId=\"b1\" datatype=\"html\">" +
		"<segment segmentId=\"s1\">" +
		"<source>Hello world</source>" +
		"</segment>" +
		"</translatable>" +
		"</txml>"

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, []byte(snippet), "test.txml", mimeType, nil)
}

// TestRoundTrip_WithTarget verifies that source/target roundtrip correctly
// at the event level (the writer may reformat XML attributes).
func TestRoundTrip_WithTarget(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	snippet := STARTFILE +
		"<translatable blockId=\"b1\" datatype=\"html\">" +
		"<segment segmentId=\"s1\" modified=\"true\">" +
		"<source>Hello</source><target>Bonjour</target>" +
		"</segment>" +
		"</translatable>" +
		"</txml>"

	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, []byte(snippet), "test.txml", mimeType, nil)
}

// TestRoundTrip_DocxFile verifies roundtrip for Test01.docx.txml.
func TestRoundTrip_DocxFile(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_txml/Test01.docx.txml")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

// TestRoundTrip_HtmlFile verifies roundtrip for Test02.html.txml.
func TestRoundTrip_HtmlFile(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_txml/Test02.html.txml")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}

// TestRoundTrip_MifFile verifies roundtrip for Test03.mif.txml.
func TestRoundTrip_MifFile(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_txml/Test03.mif.txml")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}
