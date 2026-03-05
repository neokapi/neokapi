//go:build integration

package ttx

import (
	"os"
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Output / roundtrip tests from TTXFilterTest.java
// ---------------------------------------------------------------------------

// okapi: TTXFilterTest#testOutputSimple
func TestOutput_Simple(t *testing.T) {
	snippet := startFile +
		"<Tu MatchPercent=\"0\">" +
		"<Tuv Lang=\"EN-US\">text en >=gt</Tuv>" +
		"<Tuv Lang=\"ES-EM\">text es >=gt</Tuv>" +
		"</Tu>" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "text en")
	assert.Contains(t, output, "text es")
	assert.Contains(t, output, ">=gt")
}

// okapi: TTXFilterTest#testOutputSimpleGTEscaped
func TestOutput_SimpleGTEscaped(t *testing.T) {
	snippet := startFile +
		"<Tu MatchPercent=\"0\">" +
		"<Tuv Lang=\"EN-US\">text en >=gt</Tuv>" +
		"<Tuv Lang=\"ES-EM\">text es >=gt</Tuv>" +
		"</Tu>" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, map[string]any{"escapeGT": true})
	assert.Contains(t, output, "text en")
	assert.Contains(t, output, "text es")
	// With escapeGT the > should be escaped as &gt; in the output.
	assert.Contains(t, output, "&gt;=gt")
}

// okapi: TTXFilterTest#testOutputTwoTU
func TestOutput_TwoTU(t *testing.T) {
	snippet := startFile +
		"<Tu MatchPercent=\"0\">" +
		"<Tuv Lang=\"EN-US\">text1 en</Tuv><Tuv Lang=\"ES-EM\">text1 es</Tuv></Tu>\n" +
		"  <ut Style=\"external\">some code</ut>  " +
		"<Tu MatchPercent=\"0\"><Tuv Lang=\"EN-US\">text2 en</Tuv><Tuv Lang=\"ES-EM\">text2 es</Tuv></Tu>\n" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "text1 en")
	assert.Contains(t, output, "text1 es")
	assert.Contains(t, output, "text2 en")
	assert.Contains(t, output, "text2 es")
	assert.Contains(t, output, "some code")
}

// okapi: TTXFilterTest#testOutputTUInfo
func TestOutput_TUInfo(t *testing.T) {
	snippet := startFileNoLB +
		"<Tu Origin=\"abc\" MatchPercent=\"50\"><Tuv Lang=\"EN-US\">en</Tuv><Tuv Lang=\"ES-EM\">es</Tuv></Tu>" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "Origin=\"abc\"")
	assert.Contains(t, output, "MatchPercent=\"50\"")
	assert.Contains(t, output, ">en</Tuv>")
	assert.Contains(t, output, ">es</Tuv>")
}

// okapi: TTXFilterTest#testOutputBasicTwoSegInOneTextUnit
func TestOutput_BasicTwoSegInOneTextUnit(t *testing.T) {
	snippet := startFileNoLB +
		"<Tu MatchPercent=\"0\"><Tuv Lang=\"EN-US\">text1 en</Tuv><Tuv Lang=\"ES-EM\">text1 es</Tuv></Tu>" +
		"  <Tu MatchPercent=\"0\"><Tuv Lang=\"EN-US\">text2 en</Tuv><Tuv Lang=\"ES-EM\">text2 es</Tuv></Tu>" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "text1 en")
	assert.Contains(t, output, "text1 es")
	assert.Contains(t, output, "text2 en")
	assert.Contains(t, output, "text2 es")
}

// okapi: TTXFilterTest#testOutputWithOriginalWithoutTraget
func TestOutput_WithOriginalWithoutTarget(t *testing.T) {
	snippet := startFile +
		"<Tu MatchPercent=\"0\">" +
		"<Tuv Lang=\"EN-US\">text1 en</Tuv></Tu>\n" +
		"  <ut Style=\"external\">some code</ut>  " +
		"<Tu MatchPercent=\"0\">" +
		"<Tuv Lang=\"EN-US\">text2 en</Tuv></Tu>\n" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	// The bridge should create target Tuvs with source copies when no target exists.
	assert.Contains(t, output, "text1 en")
	assert.Contains(t, output, "text2 en")
	assert.Contains(t, output, "ES-EM")
}

// okapi: TTXFilterTest#testOutputBasicWithEscapes
func TestOutput_BasicWithEscapes(t *testing.T) {
	snippet := startFileNoLB +
		"&lt;=lt, &amp;=amp, &gt;=gt, &quot;=quot." +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	// The Java expected output shows &lt;, &amp;, &quot; escaped.
	assert.Contains(t, output, "&lt;=lt")
	assert.Contains(t, output, "&amp;=amp")
	assert.Contains(t, output, "=quot.")
}

// okapi: TTXFilterTest#testOutputBasicNoTUWithSegmentation
func TestOutput_BasicNoTUWithSegmentation(t *testing.T) {
	snippet := startFileNoLB +
		"text" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	// The Java expected output wraps text in <Tu>/<Tuv>.
	assert.Contains(t, output, "text")
	assert.Contains(t, output, "<Tu")
	assert.Contains(t, output, "<Tuv")
}

// okapi: TTXFilterTest#testOutputBasicNoTUWithDFWithSegementation
func TestOutput_BasicNoTUWithDFWithSegmentation(t *testing.T) {
	snippet := startFileNoLB +
		"<df Size=\"16\">text</df>" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "text")
	assert.Contains(t, output, "<Tu")
}

// okapi: TTXFilterTest#testOutputNotSegmentedWithDF_ForcingOutSeg
func TestOutput_NotSegmentedWithDF_ForcingOutSeg(t *testing.T) {
	snippet := startFileNoLB +
		"<df Size=\"12\">" +
		"<ut Type=\"start\" Style=\"external\">{P}</ut>" +
		"src text" +
		"</df>" +
		"<ut Type=\"end\" Style=\"external\">{/P}</ut>" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "src text")
	assert.Contains(t, output, "<Tu")
	assert.Contains(t, output, "{P}")
	assert.Contains(t, output, "{/P}")
}

// okapi: TTXFilterTest#testOutputNotSegmentedWithLeadingWS
func TestOutput_NotSegmentedWithLeadingWS(t *testing.T) {
	snippet := startFileNoLB +
		"<ut Type=\"start\" Style=\"external\">bc</ut>" +
		"\n   text" +
		"<ut Type=\"end\" Style=\"external\">ec</ut>" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "text")
	assert.Contains(t, output, "bc")
	assert.Contains(t, output, "ec")
}

// okapi: TTXFilterTest#testOutputEscapesInSkeleton
func TestOutput_EscapesInSkeleton(t *testing.T) {
	snippet := startFileNoLB +
		"<ut Type=\"start\" RightEdge=\"angle\" DisplayText=\"![if !IE]&gt;&lt;p&gt;Text &lt;b&gt;&lt;u&gt;non-validating&lt;/u&gt; downlevel-revealed conditional comment&lt;/b&gt; etc &lt;/p&gt;&lt;![\">code</ut>" +
		"text" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "text")
	// Verify the escaped content in the DisplayText attribute survives.
	assert.Contains(t, output, "non-validating")
}

// okapi: TTXFilterTest#testOutputPartiallySegmentedEntryAfter
func TestOutput_PartiallySegmentedEntryAfter(t *testing.T) {
	snippet := startFileNoLB +
		"<df Font=\"z\"><ut Type=\"start\" Style=\"external\">[z]</ut>" +
		"<Tu MatchPercent=\"0\"><Tuv Lang=\"EN-US\">Src1</Tuv><Tuv Lang=\"ES-EM\">Trg1</Tuv></Tu> Src2" +
		"</df><ut Type=\"end\" Style=\"external\">[/z]</ut>" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "Src1")
	assert.Contains(t, output, "Trg1")
	assert.Contains(t, output, "Src2")
}

// okapi: TTXFilterTest#testOutputNoExtractableData
func TestOutput_NoExtractableData(t *testing.T) {
	snippet := startFile +
		" <ut Style=\"external\">some &amp; code</ut>\n\n  <!-- comments-->" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	// Should round-trip without modification since there's nothing translatable.
	assert.Contains(t, output, "some &amp; code")
	assert.Contains(t, output, "<!-- comments-->")
}

// okapi: TTXFilterTest#testOutputSegmentedSurroundedByDF
func TestOutput_SegmentedSurroundedByDF(t *testing.T) {
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
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "en1")
	assert.Contains(t, output, "es1")
	assert.Contains(t, output, "bc")
	assert.Contains(t, output, "ec")
}

// okapi: TTXFilterTest#testOutputForExternalDFwithSegmentation
func TestOutput_ForExternalDFWithSegmentation(t *testing.T) {
	snippet := startFileNoLB +
		"<df Font=\"Arial\"><ut Style=\"external\">code</ut>text<ut Style=\"external\">code</ut></df>" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "text")
	assert.Contains(t, output, "<Tu")
}

// okapi: TTXFilterTest#testOutputForTwoTUsWithSegmentation
func TestOutput_ForTwoTUsWithSegmentation(t *testing.T) {
	snippet := startFileNoLB +
		"text1<ut Style=\"external\">code</ut>text2" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "text1")
	assert.Contains(t, output, "text2")
	assert.Contains(t, output, "code")
}

// okapi: TTXFilterTest#testOutputStartingExtraDFWithSegmentation
func TestOutput_StartingExtraDFWithSegmentation(t *testing.T) {
	snippet := startFileNoLB +
		"<df Font=\"z\"><ut Type=\"start\" Style=\"external\">[Z]</ut>" +
		"Text </df><df Font=\"z\" Bold=\"on\"><ut Type=\"start\">[b]</ut>bold</df>" +
		"<df Font=\"z\"><ut Type=\"end\">[/b]</ut> after</df>" +
		"<ut Type=\"end\" Style=\"external\">[/Z]</ut>" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "Text")
	assert.Contains(t, output, "bold")
	assert.Contains(t, output, "after")
	assert.Contains(t, output, "[Z]")
	assert.Contains(t, output, "[/Z]")
}

// okapi: TTXFilterTest#testOutputVariousTagsWithSegmentation
func TestOutput_VariousTagsWithSegmentation(t *testing.T) {
	snippet := startFileNoLB +
		"<ut Type=\"start\" Style=\"external\">&lt;p&gt;</ut>paragraph <df Italic=\"on\">" +
		"<ut Type=\"start\">&lt;i&gt;</ut>text</df><ut Type=\"end\">&lt;/i&gt;</ut>" +
		"<ut Type=\"end\" Style=\"external\">&lt;/ul&gt;</ut>" +
		"<ut Type=\"start\" Style=\"external\">&lt;P&gt;</ut>" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "paragraph")
	assert.Contains(t, output, "text")
}

// okapi: TTXFilterTest#testOutputWithMixedSegmentation
func TestOutput_WithMixedSegmentation(t *testing.T) {
	snippet := startFileNoLB +
		"<Tu MatchPercent=\"50\"><Tuv Lang=\"EN-US\">text</Tuv></Tu>" +
		" more text" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "text")
	assert.Contains(t, output, "more text")
}

// okapi: TTXFilterTest#testOutputNoTUContentWithUTWithSegmentation
func TestOutput_NoTUContentWithUTWithSegmentation(t *testing.T) {
	snippet := startFileNoLB +
		"before <ut Type=\"start\">[</ut>in<ut Type=\"end\">]</ut> after" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "before")
	assert.Contains(t, output, "in")
	assert.Contains(t, output, "after")
}

// okapi: TTXFilterTest#testOutputWithPINoTUWithSegmentation
func TestOutput_WithPINoTUWithSegmentation(t *testing.T) {
	snippet := startFileNoLB +
		"<ut Class=\"procinstr\">pi</ut>text" +
		"</Raw></Body></TRADOStag>"
	output := snippetRoundtrip(t, snippet, nil)
	assert.Contains(t, output, "pi")
	assert.Contains(t, output, "text")
}

// okapi: TTXFilterTest#testDoubleExtraction
func TestRoundTrip_DoubleExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	// Test01.html.ttx — uses en / fr locales per its UserSettings.
	path1 := bridgetest.TestdataFile(t, "okf_ttx/Test01.html.ttx")
	content1, err := os.ReadFile(path1)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content1, path1, mimeType, nil)

	// Test02_allseg.html.ttx
	path2 := bridgetest.TestdataFile(t, "okf_ttx/Test02_allseg.html.ttx")
	content2, err := os.ReadFile(path2)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content2, path2, mimeType, nil)
}

// okapi: TTXFilterTest#textDoubleExtractionOriginalAllSegmented
func TestRoundTrip_DoubleExtractionOriginalAllSegmented(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)

	path := bridgetest.TestdataFile(t, "okf_ttx/Test02_allseg.html.ttx")
	content, err := os.ReadFile(path)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, path, mimeType, nil)
}
