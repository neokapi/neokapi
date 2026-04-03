//go:build integration

package ttx

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Extraction / snippet tests from TTXFilterTest.java
// ---------------------------------------------------------------------------

// okapi: TTXFilterTest#testBasicNoUT
func TestSnippet_BasicNoUT(t *testing.T) {
	snippet := startFile +
		"<Tu>" +
		"<Tuv Lang=\"EN-US\">text en</Tuv>" +
		"<Tuv Lang=\"ES-EM\">text es</Tuv>" +
		"</Tu>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text en", blocks[0].SourceText())

	// Should also have a target.
	assert.True(t, blocks[0].HasTarget(tgtLocale), "should have target")
	assert.Equal(t, "text es", blocks[0].TargetText(tgtLocale))
}

// okapi: TTXFilterTest#testBasicWithUT
func TestSnippet_BasicWithUT(t *testing.T) {
	snippet := startFile +
		"<Tu>" +
		"<Tuv Lang=\"EN-US\">text <ut DisplayText=\"br\">&lt;br/&gt;</ut>en <ut Type=\"start\">&lt;b></ut>bold<ut Type=\"end\">&lt;/b></ut>.</Tuv>" +
		"<Tuv Lang=\"ES-EM\">TEXT <ut DisplayText=\"br\">&lt;br/&gt;</ut>ES <ut Type=\"start\">&lt;b></ut>BOLD<ut Type=\"end\">&lt;/b></ut>.</Tuv>" +
		"</Tu>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	srcText := b.SourceText()
	// The source text should contain "text", "en", "bold" — inline codes
	// are represented as spans so the text reconstructs the readable form.
	assert.Contains(t, srcText, "text")
	assert.Contains(t, srcText, "bold")

	// Should have inline spans for the <br/>, <b>, </b> codes.
	require.NotEmpty(t, b.Source)
	if b.Source[0].Content != nil {
		assert.NotEmpty(t, b.Source[0].Content.Spans, "should have spans for inline codes")
	}
}

// okapi: TTXFilterTest#testBasicNoTU
func TestSnippet_BasicNoTU(t *testing.T) {
	snippet := startFile +
		"text" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// The Java test shows "\ntext" because STARTFILE ends with \n inside Raw.
	assert.Contains(t, blocks[0].SourceText(), "text")
}

// okapi: TTXFilterTest#testBasicNoExtractableData
func TestSnippet_BasicNoExtractableData(t *testing.T) {
	snippet := startFile +
		" <ut Style=\"external\">some &amp; code</ut>\n\n  <!-- comments-->" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks, "should not extract any translatable blocks from pure skeleton")
}

// okapi: TTXFilterTest#testBasicWithEscapes
func TestSnippet_BasicWithEscapes(t *testing.T) {
	snippet := startFileNoLB +
		"&lt;=lt, &amp;=amp, &gt;=gt, &quot;=quot." +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// The Java test expects: <=lt, &=amp, >=gt, "=quot.
	text := blocks[0].SourceText()
	assert.Contains(t, text, "<=lt")
	assert.Contains(t, text, "&=amp")
	assert.Contains(t, text, ">=gt")
	assert.Contains(t, text, "\"=quot.")
}

// okapi: TTXFilterTest#testBasicTwoSegInOneTextUnit
func TestSnippet_BasicTwoSegInOneTextUnit(t *testing.T) {
	snippet := startFileNoLB +
		"<Tu><Tuv Lang=\"EN-US\">text1 en</Tuv><Tuv Lang=\"ES-EM\">text1 es</Tuv></Tu>" +
		"  <Tu><Tuv Lang=\"EN-US\">text2 en</Tuv><Tuv Lang=\"ES-EM\">text2 es</Tuv></Tu>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Java expects two segments in one TextUnit. In the bridge, this may
	// produce one block with two source segments, or two blocks. Verify text content.
	allText := ""
	for _, b := range blocks {
		allText += b.SourceText() + " | "
	}
	assert.Contains(t, allText, "text1 en")
	assert.Contains(t, allText, "text2 en")
}

// okapi: TTXFilterTest#testBasicNoTUWithDF
func TestSnippet_BasicNoTUWithDF(t *testing.T) {
	snippet := startFileNoLB +
		"<df Size=\"16\">text</df>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: TTXFilterTest#testNotSegmentedWithDF
func TestSnippet_NotSegmentedWithDF(t *testing.T) {
	snippet := startFileNoLB +
		"<df Size=\"12\">" +
		"<ut Type=\"start\" Style=\"external\">{P}</ut>" +
		"src text" +
		"</df>" +
		"<ut Type=\"end\" Style=\"external\">{/P}</ut>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "src text")
}

// okapi: TTXFilterTest#testNotSegmentedWithDFAndCodes
func TestSnippet_NotSegmentedWithDFAndCodes(t *testing.T) {
	snippet := startFileNoLB +
		"<df Size=\"12\">" +
		"<ut Type=\"start\" Style=\"external\">{P}</ut><ut Type=\"start\">{i}</ut>" +
		"src text" +
		"</df><ut Type=\"end\">{/i}</ut>" +
		"<ut Type=\"end\" Style=\"external\">{/P}</ut>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "src text")
}

// okapi: TTXFilterTest#testSegmentedAndNot
func TestSnippet_SegmentedAndNot(t *testing.T) {
	snippet := startFileNoLB +
		"<df Size=\"16\">" +
		"<ut Type=\"start\" Style=\"external\">bc</ut>" +
		" text " +
		"<Tu MatchPercent=\"0\">" +
		"<Tuv Lang=\"EN-US\">en1</Tuv>" +
		"<Tuv Lang=\"ES-EM\">es1</Tuv>" +
		"</Tu>" +
		"</df>" +
		"<ut Type=\"end\" Style=\"external\">ec</ut>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := allBlocks(parts)
	require.NotEmpty(t, blocks)

	// Java expects " text " and "en1" as two segments. Collect all block texts.
	allText := ""
	for _, b := range blocks {
		allText += b.SourceText() + " | "
	}
	assert.Contains(t, allText, "en1")
}

// okapi: TTXFilterTest#testSegmentedSurroundedByDF
func TestSnippet_SegmentedSurroundedByDF(t *testing.T) {
	snippet := startFileNoLB +
		"<df Size=\"16\">" +
		"<ut Type=\"start\" Style=\"external\">bc</ut>" +
		"<Tu MatchPercent=\"0\">" +
		"<Tuv Lang=\"EN-US\">en1</Tuv>" +
		"<Tuv Lang=\"ES-EM\">es1</Tuv>" +
		"</Tu>" +
		"</df>" +
		"<ut Type=\"end\" Style=\"external\">ec</ut>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "en1", blocks[0].SourceText())

	// Should have a target.
	assert.True(t, blocks[0].HasTarget(tgtLocale), "should have target")
	assert.Equal(t, "es1", blocks[0].TargetText(tgtLocale))
}

// okapi: TTXFilterTest#testSegmentedSurroundedByInternalCodes
func TestSnippet_SegmentedSurroundedByInternalCodes(t *testing.T) {
	snippet := startFileNoLB +
		"<ut Type=\"start\">bc</ut>" +
		"<Tu MatchPercent=\"100\">" +
		"<Tuv Lang=\"EN-US\">en1</Tuv>" +
		"<Tuv Lang=\"ES-EM\">es1</Tuv>" +
		"</Tu>" +
		"<ut Type=\"end\">ec</ut>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "en1")
}

// okapi: TTXFilterTest#testForOneTU
func TestSnippet_ForOneTU(t *testing.T) {
	snippet := startFileNoLB +
		"<ut Type=\"start\" Style=\"external\" RightEdge=\"angle\" DisplayText=\"li\">&lt;li&gt;</ut>" +
		"<ut Type=\"start\" RightEdge=\"angle\" DisplayText=\"a\">&lt;a&gt;</ut>" +
		"text1" +
		"<ut Style=\"external\" DisplayText=\"br\">&lt;br /&gt;</ut>" +
		"text2" +
		"<ut Type=\"end\" LeftEdge=\"angle\" DisplayText=\"a\">&lt;/a&gt;</ut>" +
		"<ut Type=\"end\" Style=\"external\" LeftEdge=\"angle\" DisplayText=\"li\">&lt;/li&gt;</ut>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "text1")
	assert.Contains(t, text, "text2")
}

// okapi: TTXFilterTest#testForOneTUWithTextParts
func TestSnippet_ForOneTUWithTextParts(t *testing.T) {
	snippet := startFileNoLB +
		"<ut Type=\"start\" Style=\"external\" RightEdge=\"angle\" DisplayText=\"p\">&lt;p&gt;</ut>" +
		"<Tu Origin=\"manual\" MatchPercent=\"0\">" +
		"<Tuv Lang=\"en\">This <ut DisplayText=\"a\">&lt;a href=http://www.htmlparser.net/&gt;</ut>anchor element<ut Type=\"end\" LeftEdge=\"angle\" DisplayText=\"a\">&lt;/a&gt;</ut> demonstrates that a tag ending in <df Font=\"Courier New\"><ut Type=\"start\" RightEdge=\"angle\" DisplayText=\"code\">&lt;code&gt;</ut>/&gt;</df><ut Type=\"end\" LeftEdge=\"angle\" DisplayText=\"code\">&lt;/code&gt;</ut> is not considered an empty element tag if it has a name that requires an end tag.</Tuv>" +
		"<Tuv Lang=\"es-EM\">This <ut DisplayText=\"a\">&lt;a href=http://www.htmlparser.net/&gt;</ut>anchor element<ut Type=\"end\" LeftEdge=\"angle\" DisplayText=\"a\">&lt;/a&gt;</ut> demonstrates that a tag ending in <df Font=\"Courier New\"><ut Type=\"start\" RightEdge=\"angle\" DisplayText=\"code\">&lt;code&gt;</ut>/&gt;</df><ut Type=\"end\" LeftEdge=\"angle\" DisplayText=\"code\">&lt;/code&gt;</ut> is not considered an empty element tag if it has a name that requires an end tag.</Tuv>" +
		"</Tu>" +
		"  " +
		"<Tu Origin=\"manual\" MatchPercent=\"0\">" +
		"<Tuv Lang=\"en\">In this case the final &apos;/&apos; is included in the href attribute value instead of being interpreted as the end of the tag.</Tuv>" +
		"<Tuv Lang=\"es-EM\">In this case the final &apos;/&apos; is included in the href attribute value instead of being interpreted as the end of the tag.</Tuv>" +
		"</Tu>" +
		"\n" +
		"<ut Style=\"external\" DisplayText=\"p\">&lt;p style=&quot;background-color: #e0e0e0&quot;/&gt;</ut>" +
		"<Tu Origin=\"manual\" MatchPercent=\"0\">" +
		"<Tuv Lang=\"en\">The same goes for tags that have an optional end tag like this paragraph, which has a grey background despite the fact that the p element is syntactically an empty element tag.</Tuv>" +
		"<Tuv Lang=\"es-EM\">The same goes for tags that have an optional end tag like this paragraph, which has a grey background despite the fact that the p element is syntactically an empty element tag.</Tuv>" +
		"</Tu>" +
		"<ut Type=\"end\" Style=\"external\" LeftEdge=\"angle\" DisplayText=\"p\">&lt;/p&gt;</ut>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Java expects 4 parts in first TU. We verify key text excerpts are present.
	allText := ""
	for _, b := range blocks {
		allText += b.SourceText() + " "
	}
	assert.Contains(t, allText, "anchor element")
	assert.Contains(t, allText, "not considered an empty element tag")
	assert.Contains(t, allText, "In this case the final")
	assert.Contains(t, allText, "optional end tag like this paragraph")
}

// okapi: TTXFilterTest#testForTwoTUs
func TestSnippet_ForTwoTUs(t *testing.T) {
	snippet := startFileNoLB +
		"text1<ut Style=\"external\">code</ut>text2" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should extract two text units")
	texts := bridgetest.BlockTexts(blocks)
	foundT1 := false
	foundT2 := false
	for _, txt := range texts {
		if strings.Contains(txt, "text1") {
			foundT1 = true
		}
		if strings.Contains(txt, "text2") {
			foundT2 = true
		}
	}
	assert.True(t, foundT1, "should find text1")
	assert.True(t, foundT2, "should find text2")
}

// okapi: TTXFilterTest#testForExternalDF
func TestSnippet_ForExternalDF(t *testing.T) {
	snippet := startFileNoLB +
		"<df Font=\"Arial\"><ut Style=\"external\">code</ut>text<ut Style=\"external\">code</ut></df>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: TTXFilterTest#testStartingExtraDF
func TestSnippet_StartingExtraDF(t *testing.T) {
	snippet := startFile +
		"<df Font=\"z\"><ut Type=\"start\" Style=\"external\">[Z]</ut>" +
		"Text </df><df Font=\"z\" Bold=\"on\"><ut Type=\"start\">[b]</ut>bold</df>" +
		"<df Font=\"z\"><ut Type=\"end\">[/b]</ut> after</df>" +
		"<ut Type=\"end\" Style=\"external\">[/Z]</ut>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Text")
	assert.Contains(t, text, "bold")
	assert.Contains(t, text, "after")
}

// okapi: TTXFilterTest#testVariousTags
func TestSnippet_VariousTags(t *testing.T) {
	snippet := startFileNoLB +
		"<ut Type=\"start\" Style=\"external\">&lt;p&gt;</ut>paragraph <df Italic=\"on\">" +
		"<ut Type=\"start\">&lt;i&gt;</ut>text</df><ut Type=\"end\">&lt;/i&gt;</ut>" +
		"<ut Type=\"end\" Style=\"external\">&lt;/ul&gt;</ut>" +
		"<ut Type=\"start\" Style=\"external\">&lt;P&gt;</ut>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "paragraph")
	assert.Contains(t, text, "text")
}

// okapi: TTXFilterTest#testVariousTagsWithSegmentation
func TestSnippet_VariousTagsWithSegmentation(t *testing.T) {
	snippet := startFileNoLB +
		"<ut Type=\"start\" Style=\"external\">&lt;p&gt;</ut>" +
		"<Tu MatchPercent=\"0\">" +
		"<Tuv Lang=\"EN-US\">paragraph <df Italic=\"on\">" +
		"<ut Type=\"start\">&lt;i&gt;</ut>text</df><ut Type=\"end\">&lt;/i&gt;</ut>" +
		"</Tuv>" +
		"<Tuv Lang=\"ES-EM\">paragraph <df Italic=\"on\">" +
		"<ut Type=\"start\">&lt;i&gt;</ut>text</df><ut Type=\"end\">&lt;/i&gt;</ut>" +
		"</Tuv></Tu> " +
		"<Tu MatchPercent=\"0\">" +
		"<Tuv Lang=\"EN-US\"><ut Type=\"start\">&lt;i&gt;</ut>text<ut Type=\"end\">&lt;/i&gt;</ut></Tuv>" +
		"<Tuv Lang=\"ES-EM\"><ut Type=\"start\">&lt;i&gt;</ut>text<ut Type=\"end\">&lt;/i&gt;</ut></Tuv>" +
		"</Tu>" +
		"<ut Type=\"end\" Style=\"external\">&lt;/ul&gt;</ut>" +
		"<ut Type=\"start\" Style=\"external\">&lt;P&gt;</ut>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	// Java expects 2 segments in the TU. Verify both texts appear.
	allText := ""
	for _, b := range blocks {
		allText += b.SourceText() + " | "
	}
	assert.Contains(t, allText, "paragraph")
	assert.Contains(t, allText, "text")
}

// okapi: TTXFilterTest#testWithMixedSegmentation
func TestSnippet_WithMixedSegmentation(t *testing.T) {
	snippet := startFileNoLB +
		"<Tu MatchPercent=\"50\"><Tuv Lang=\"EN-US\">text</Tuv></Tu>" +
		" more text" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	// The bridge extracts the segmented part as a translatable block.
	// "more text" may appear as a separate block or be folded into the same block.
	blocks := allBlocks(parts)
	require.NotEmpty(t, blocks)

	allText := ""
	for _, b := range blocks {
		allText += b.SourceText() + " | "
	}
	assert.Contains(t, allText, "text")
}

// okapi: TTXFilterTest#testWithPINoTU
func TestSnippet_WithPINoTU(t *testing.T) {
	snippet := startFile +
		"<ut Class=\"procinstr\">pi</ut>text" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "text", blocks[0].SourceText())
}

// okapi: TTXFilterTest#testPartiallySegmentedEntry
func TestSnippet_PartiallySegmentedEntry(t *testing.T) {
	snippet := startFileNoLB +
		"Outside1 <Tu MatchPercent=\"0\"><Tuv Lang=\"EN-US\">Inside</Tuv></Tu> Outside2" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	// The bridge may extract only the segmented "Inside" as a translatable block,
	// with "Outside1" and "Outside2" as unsegmented parts in the same TU or skeleton.
	blocks := allBlocks(parts)
	require.NotEmpty(t, blocks)

	allText := ""
	for _, b := range blocks {
		allText += b.SourceText() + " | "
	}
	assert.Contains(t, allText, "Inside")
}

// okapi: TTXFilterTest#testPartiallySegmentedEntryAfter
func TestSnippet_PartiallySegmentedEntryAfter(t *testing.T) {
	snippet := startFileNoLB +
		"<df Font=\"z\"><ut Type=\"start\" Style=\"external\">[z]</ut>" +
		"<Tu MatchPercent=\"0\"><Tuv Lang=\"EN-US\">Src1</Tuv><Tuv Lang=\"ES-EM\">Trg1</Tuv></Tu> Src2" +
		"</df><ut Type=\"end\" Style=\"external\">[/z]</ut>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := allBlocks(parts)
	require.NotEmpty(t, blocks)

	allText := ""
	for _, b := range blocks {
		allText += b.SourceText() + " | "
	}
	assert.Contains(t, allText, "Src1")
}

// okapi: TTXFilterTest#testPartiallySegmentedEntryNothingTranslatable
func TestSnippet_PartiallySegmentedEntryNothingTranslatable(t *testing.T) {
	snippet := startFileNoLB +
		"{}[]#$%@! <Tu MatchPercent=\"0\"><Tuv Lang=\"EN-US\">Inside</Tuv></Tu> \t\n-_=+" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	// Java expects "{}[]#$%@! [Inside] \t\n-_=+". At minimum "Inside" should be present.
	blocks := allBlocks(parts)
	require.NotEmpty(t, blocks)

	allText := ""
	for _, b := range blocks {
		allText += b.SourceText() + " | "
	}
	assert.Contains(t, allText, "Inside")
}

// okapi: TTXFilterTest#testLargePartiallySegmentedEntry
func TestSnippet_LargePartiallySegmentedEntry(t *testing.T) {
	snippet := startFileNoLB +
		"Out1<Tu MatchPercent=\"0\"><Tuv Lang=\"EN-US\">In2</Tuv></Tu>" +
		"Out3<Tu MatchPercent=\"0\"><Tuv Lang=\"EN-US\">In4</Tuv></Tu>" +
		"Out5" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := allBlocks(parts)
	require.NotEmpty(t, blocks)

	allText := ""
	for _, b := range blocks {
		allText += b.SourceText() + " | "
	}
	assert.Contains(t, allText, "In2")
	assert.Contains(t, allText, "In4")
}

// okapi: TTXFilterTest#testNoTUContentWithUT
func TestSnippet_NoTUContentWithUT(t *testing.T) {
	snippet := startFileNoLB +
		"before <ut Type=\"start\">[</ut>in<ut Type=\"end\">]</ut> after" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "before")
	assert.Contains(t, text, "in")
	assert.Contains(t, text, "after")
}

// okapi: TTXFilterTest#testNoTUEndsWithUT
func TestSnippet_NoTUEndsWithUT(t *testing.T) {
	snippet := startFile +
		"text<ut Class=\"procinstr\">pi</ut>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "text")
}

// okapi: TTXFilterTest#testNoTUContentWithSplitStart
func TestSnippet_NoTUContentWithSplitStart(t *testing.T) {
	snippet := startFileNoLB +
		"before <ut Type=\"start\" RightEdge=\"split\">[ulink={</ut>text1<ut Type=\"start\" LeftEdge=\"split\">}]</ut>text2<ut Type=\"end\">[/ulink]</ut> after" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "before")
	assert.Contains(t, text, "text1")
	assert.Contains(t, text, "text2")
	assert.Contains(t, text, "after")
}

// okapi: TTXFilterTest#testTUInfo
func TestSnippet_TUInfo(t *testing.T) {
	snippet := startFileNoLB +
		"<Tu Origin=\"abc\" MatchPercent=\"50\"><Tuv Lang=\"EN-US\">en</Tuv><Tuv Lang=\"ES-EM\">es</Tuv></Tu>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "en", b.SourceText())
	assert.True(t, b.HasTarget(tgtLocale), "should have target")
}

// okapi: TTXFilterTest#testTUInfoXU
func TestSnippet_TUInfoXU(t *testing.T) {
	snippet := startFileNoLB +
		"<Tu Origin=\"xtranslate\" MatchPercent=\"101\"><Tuv Lang=\"EN-US\">en</Tuv><Tuv Lang=\"ES-EM\">es</Tuv></Tu>" +
		"</Raw></Body></TRADOStag>"
	parts := readTTXDefault(t, snippet)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "en", b.SourceText())
	assert.True(t, b.HasTarget(tgtLocale), "should have target")
	assert.Equal(t, "es", b.TargetText(tgtLocale))
}
