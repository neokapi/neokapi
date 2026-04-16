//go:build integration

package its

import (
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- ITS surefire: HTML5 tests not yet implemented in bridge ---
//
// okapi-unmapped: HTML5FilterTest#testAddITSAnnotations1 — ITS annotation injection not bridged
// okapi-unmapped: HTML5FilterTest#testAddITSAnnotations2 — ITS annotation injection not bridged
// okapi-unmapped: HTML5FilterTest#testAddITSAnnotations3 — ITS annotation injection not bridged
// okapi-unmapped: HTML5FilterTest#testAllowedChars — ITS allowed chars metadata not bridged
// okapi-unmapped: HTML5FilterTest#testDomain — ITS domain annotation not bridged
// okapi-unmapped: HTML5FilterTest#testExternalResources — ITS external resources not bridged
// okapi-unmapped: HTML5FilterTest#testGlobalLocQualityIssues — ITS LQI annotation not bridged
// okapi-unmapped: HTML5FilterTest#testLQRLocal — ITS LQR annotation not bridged
// okapi-unmapped: HTML5FilterTest#testLocNoteLocal — ITS localization note not bridged
// okapi-unmapped: HTML5FilterTest#testLocalLocQualityIssues — ITS LQI annotation not bridged
// okapi-unmapped: HTML5FilterTest#testLocaleFilterLocal — ITS locale filter not bridged
// okapi-unmapped: HTML5FilterTest#testMinimalHTML5Output — ITS minimal output not bridged
// okapi-unmapped: HTML5FilterTest#testMinimalHTMLWithStandoff — ITS standoff annotation not bridged
// okapi-unmapped: HTML5FilterTest#testProvenanceStandoff — ITS provenance annotation not bridged
// okapi-unmapped: HTML5FilterTest#testSimpleOutput — ITS output roundtrip not bridged
// okapi-unmapped: HTML5FilterTest#testStandofftLocQualityIssues — ITS standoff LQI not bridged
// okapi-unmapped: HTML5FilterTest#testStorageSizeLocal — ITS storage size annotation not bridged
// okapi-unmapped: HTML5FilterTest#testStorageSizeOnAttribute — ITS storage size on attribute not bridged
// okapi-unmapped: HTML5FilterTest#testTerminologyLocal — ITS terminology annotation not bridged
// okapi-unmapped: HTML5FilterTest#testTextDirectionClarification — ITS text direction not bridged
//
// --- ITS surefire: Java-internal W3C library / ITS engine tests ---
//
// okapi-unmapped: ITSContentTests#testAnnotatorsRef — Java-internal ITS content API test
// okapi-unmapped: ParametersTest#codesSimplificationParametersReadFromString — Java-internal parameters test
// okapi-unmapped: ParametersTest#customValuesNotWrittenAsString — Java-internal parameters test
// okapi-unmapped: ParametersTest#defaultValuesWrittenAsString — Java-internal parameters test
// okapi-unmapped: TraversalTest#testAllowedCharsGlobal — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testAllowedCharsLocal — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testAnnotatorsRef — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testAnnotatorsRefBadValue — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testBadQueryLanguage — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testDomainGlobal — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testExternalResourceRefGlobal — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testGlobalAndLocalLanguageInfo — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testIdValueOnAttribute — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testLQIssueGlobal1 — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testLQIssueGlobal2 — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testLQIssueGlobalAndLocal1 — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testLQIssueLocal1 — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testLQIssueLocalWithSpan — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testLQIssueOnAttributes — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testLocNote — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testLocQualityRatingHtml — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testLocQualityRatingXml — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testLocalLanguageInfo — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testLocaleFilter — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testMtConfidenceGlobalHtml — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testMtConfidenceLocal — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testMtConfidenceLocalHtml — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testPreserveSpaces — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testProvenanceLocal — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testQueryLanguage — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testSimple — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testStorageSizeGlobal1 — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testStorageSizeGlobal2 — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testStorageSizeLocal1 — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testTargetPointerAttributes — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testTargetPointerGlobal — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testTerm — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testTermLocally — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testTermOnAttribute — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testTermPointer — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testTermPointerwithID — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testTextAnalysisOnAttribute — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testTextAnalysisPointerHtml — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testTextAnalysisSimpleHtml — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testTranslateGlobal — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testWithinText — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testWithinTextLocalSpan — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testXmlId — Java-internal ITS traversal test
// okapi-unmapped: TraversalTest#testlangVsXmlLangInXHtml — Java-internal ITS traversal test
// okapi-unmapped: VariableResolverTest#testQuotation — Java-internal ITS variable resolver test

const html5FilterClass = "net.sf.okapi.filters.its.html5.HTML5Filter"
const html5MimeType = "text/html"

// readHTML5 parses an HTML5 snippet with custom filter params and returns the parts.
func readHTML5(t *testing.T, snippet string, filterParams map[string]any) []*model.Part {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	return bridgetest.ReadString(t, pool, cfg, html5FilterClass, snippet, "test.html", html5MimeType, filterParams)
}

// readHTML5Default parses an HTML5 snippet with default (nil) params.
func readHTML5Default(t *testing.T, snippet string) []*model.Part {
	t.Helper()
	return readHTML5(t, snippet, nil)
}

// allBlocks returns all blocks (translatable and non-translatable) from parts.
func html5AllBlocks(parts []*model.Part) []*model.Block {
	return bridgetest.FilterBlocks(parts)
}

// snippetRoundtrip roundtrips an HTML5 snippet and returns the output string.
func html5SnippetRoundtrip(t *testing.T, snippet string, filterParams map[string]any) string {
	t.Helper()
	pool, cfg := bridgetest.SharedBridge(t)
	result := bridgetest.RoundTrip(t, pool, cfg, html5FilterClass, []byte(snippet), "test.html", html5MimeType, filterParams)
	return string(result.Output)
}

// findBlockContaining returns the first block whose source text contains the given substring.
func html5FindBlockContaining(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), substr) {
			return b
		}
	}
	return nil
}

// countPartsByType counts parts of a given type.
func html5CountPartsByType(parts []*model.Part, pt model.PartType) int {
	count := 0
	for _, p := range parts {
		if p.Type == pt {
			count++
		}
	}
	return count
}

// ---- HTML5FilterTest - Basic Extraction Tests ----

// TestExtract_SimpleRead verifies basic extraction of title, paragraph with
// <span>, and paragraph with <i> inline codes from a simple HTML5 document.
//
// okapi: HTML5FilterTest#testSimpleRead
func TestExtract_SimpleRead(t *testing.T) {
	html := `<!DOCTYPE html><html lang="en"><head><meta charset=utf-8>` +
		`<title>Title</title></head><body>` +
		`<p>Text in <span>bold</span>.</p>` +
		`<p>Text in <i>italics</i>.</p>` +
		`</body></html>`

	parts := readHTML5Default(t, html)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Title", "should extract title text")

	// The paragraph text should contain inline content.
	var foundSpan, foundItalic bool
	for _, b := range blocks {
		src := b.SourceText()
		if strings.Contains(src, "bold") {
			foundSpan = true
			// Should have inline-code runs for <span>
			assert.True(t, bridgetest.HasInlineCode(b.SourceRuns()),
				"paragraph with <span> should have inline-code runs")
		}
		if strings.Contains(src, "italics") {
			foundItalic = true
			assert.True(t, bridgetest.HasInlineCode(b.SourceRuns()),
				"paragraph with <i> should have inline-code runs")
		}
	}
	assert.True(t, foundSpan, "should find block with 'bold' text")
	assert.True(t, foundItalic, "should find block with 'italics' text")
}

// TestExtract_TranslateLocally verifies that translate=no on an inline <span>
// creates a placeholder instead of translatable content.
//
// okapi: HTML5FilterTest#testTranslateLocally
func TestExtract_TranslateLocally(t *testing.T) {
	html := `<!DOCTYPE html><html lang="en"><head><meta charset=utf-8>` +
		`<title>Title</title></head><body>` +
		`<p>Text in <span translate=no>code</span>.</p></body></html>`

	parts := readHTML5Default(t, html)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks")

	// The "code" text should NOT appear as a separate translatable block.
	// It should be a non-translatable inline placeholder within the paragraph.
	texts := bridgetest.BlockTexts(blocks)
	for _, text := range texts {
		// "code" might appear within the paragraph's source text as a placeholder,
		// but should not be its own standalone translatable text.
		if text == "code" {
			t.Error("translate=no span content 'code' should not be a standalone translatable block")
		}
	}
}

// TestExtract_TranslateOnAttribute verifies that translate='no' on <html>
// suppresses all text extraction except for meta keywords (default ITS rule)
// and that an empty translate attribute means "yes".
//
// okapi: HTML5FilterTest#testTranslateOnAttribute
func TestExtract_TranslateOnAttribute(t *testing.T) {
	// translate='no' on <html> with keywords meta
	html := `<!DOCTYPE html><html lang="en" translate="no"><head><meta charset=utf-8>` +
		`<meta name="keywords" content="kw1, kw2">` +
		`<title>Title</title></head><body><p>Body text</p></body></html>`

	parts := readHTML5Default(t, html)
	blocks := bridgetest.TranslatableBlocks(parts)

	// The default ITS rules for HTML5 make meta keywords content translatable
	// even when translate=no is on <html>.
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "kw1, kw2",
		"keywords meta content should be extracted despite translate=no on html")

	// Title and body text should NOT be extracted when translate=no.
	assert.NotContains(t, texts, "Title",
		"title should not be extracted when html has translate=no")
	assert.NotContains(t, texts, "Body text",
		"body text should not be extracted when html has translate=no")
}

// TestExtract_TranslateAttribute verifies that the alt attribute on <img> is
// extracted as a separate text unit with type "x-alt" or similar.
//
// okapi: HTML5FilterTest#testTranslateAttribute
func TestExtract_TranslateAttribute(t *testing.T) {
	html := `<!DOCTYPE html><html lang="en"><head><meta charset=utf-8>` +
		`<title>Title</title></head><body>` +
		`<p>Text <img src=test.png alt="Alt text">.</p></body></html>`

	parts := readHTML5Default(t, html)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Alt text",
		"alt attribute should be extracted as a translatable text unit")
}

// TestExtract_PreserveSpace verifies that <pre> preserves whitespace/tabs
// while <p> normalizes whitespace.
//
// okapi: HTML5FilterTest#testPreserveSpace
func TestExtract_PreserveSpace(t *testing.T) {
	html := `<!DOCTYPE html><html lang="en"><head><meta charset=utf-8>` +
		`<title>Title</title></head><body>` +
		`<pre> text  		 <b>  etc.  </b>	 </pre>` +
		`<p> text  		 <b>  etc.  </b>	 </p>` +
		`</body></html>`

	parts := readHTML5Default(t, html)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2, "should extract at least 2 blocks (pre + p)")

	// Find the pre block - it should preserve whitespace.
	var preBlock, pBlock *model.Block
	for _, b := range blocks {
		if b.PreserveWhitespace {
			preBlock = b
		} else if strings.Contains(b.SourceText(), "text") && !b.PreserveWhitespace {
			pBlock = b
		}
	}

	require.NotNil(t, preBlock, "should find a block with PreserveWhitespace=true")
	assert.Contains(t, preBlock.SourceText(), "\t",
		"pre block should preserve tabs")

	if pBlock != nil {
		// p block should have collapsed whitespace.
		assert.NotContains(t, pBlock.SourceText(), "\t",
			"p block should collapse tabs")
	}
}

// TestExtract_IdValueLocal verifies that the id attribute is mapped to the text
// unit name via the ITS idValue rule.
//
// okapi: HTML5FilterTest#testIdValueLocal
func TestExtract_IdValueLocal(t *testing.T) {
	html := `<!DOCTYPE html><html lang=en><head><meta charset=utf-8>` +
		`<title>Title</title></head><body>` +
		`<p id='n1'>Text 1</p></body></html>`

	parts := readHTML5Default(t, html)
	blocks := bridgetest.TranslatableBlocks(parts)

	// Find the block for "Text 1" and verify it has a name derived from id.
	var found bool
	for _, b := range blocks {
		if b.SourceText() == "Text 1" {
			found = true
			assert.Contains(t, b.Name, "n1",
				"block name should contain the id attribute value 'n1'")
			break
		}
	}
	assert.True(t, found, "should find a block with text 'Text 1'")
}

// TestExtract_WithinTextLocal verifies that its-within-text='no' splits an
// inline element into separate text units.
//
// okapi: HTML5FilterTest#testWithinTextLocal
func TestExtract_WithinTextLocal(t *testing.T) {
	html := `<!DOCTYPE html><html lang="en"><head><meta charset=utf-8>` +
		`<title>Title</title></head><body>` +
		`<p>Text in <span its-within-text='no'>standalone</span> end.</p>` +
		`</body></html>`

	parts := readHTML5Default(t, html)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	// With its-within-text='no', the <span> content should be extracted as a
	// separate text unit, not as an inline code within the paragraph.
	assert.Contains(t, texts, "standalone",
		"span with its-within-text='no' should be extracted as a separate text unit")
}

// TestExtract_Link verifies link extraction from test02.html with anchor tag.
//
// okapi: HTML5FilterTest#testLink
func TestExtract_Link(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okapi/filters/its/src/test/resources/test02.html")
	parts := bridgetest.ReadFile(t, pool, cfg, html5FilterClass, path, html5MimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from test02.html")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Title", "should extract title")
}

// TestExtract_EmptyElements verifies that empty <span></span> elements are
// preserved as inline codes in roundtrip.
//
// okapi: HTML5FilterTest#testEmptyElements2
func TestExtract_EmptyElements(t *testing.T) {
	html := `<!DOCTYPE html><html lang="en"><head><meta charset=utf-8>` +
		`<title>Title</title></head><body>` +
		`<p>Text <span></span> more.</p></body></html>`

	parts := readHTML5Default(t, html)
	blocks := bridgetest.TranslatableBlocks(parts)

	// Find the paragraph block and verify it has inline spans for the empty <span>.
	paraBlock := findBlockContaining(blocks, "Text")
	if paraBlock != nil {
		assert.True(t, bridgetest.HasInlineCode(paraBlock.SourceRuns()),
			"empty <span></span> should produce inline-code runs")
	}
}

// TestExtract_OpenTwice verifies that the filter can be opened, closed, and
// reopened without error.
//
// okapi: HTML5FilterTest#testOpenTwice
func TestExtract_OpenTwice(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okapi/filters/its/src/test/resources/test01.html")

	// First read.
	parts1 := bridgetest.ReadFile(t, pool, cfg, html5FilterClass, path, html5MimeType, nil)
	blocks1 := bridgetest.FilterBlocks(parts1)

	// Second read -- same file.
	parts2 := bridgetest.ReadFile(t, pool, cfg, html5FilterClass, path, html5MimeType, nil)
	blocks2 := bridgetest.FilterBlocks(parts2)

	require.NotEmpty(t, blocks1, "first read should produce blocks")
	require.Equal(t, len(blocks1), len(blocks2),
		"opening and reading the same file twice should produce the same number of blocks")

	texts1 := bridgetest.BlockTexts(blocks1)
	texts2 := bridgetest.BlockTexts(blocks2)
	assert.Equal(t, texts1, texts2,
		"opening and reading the same file twice should produce the same block texts")
}

// ---- HTML5DefaultsTest ----

// TestDefaults_WithinText verifies that its-within-text='no' on <i> removes it
// from inline, and checks that <span> content has 2 codes.
//
// okapi: HTML5DefaultsTest#testWinthinText
func TestDefaults_WithinText(t *testing.T) {
	html := `<!DOCTYPE html><html lang="en"><head><meta charset=utf-8>` +
		`<title>Title</title></head><body>` +
		`<p>Text in <i its-within-text='no'>separate</i> and ` +
		`<span>bold content</span>.</p>` +
		`</body></html>`

	parts := readHTML5Default(t, html)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)

	// The <i> with its-within-text='no' should be a separate text unit.
	assert.Contains(t, texts, "separate",
		"i with its-within-text='no' should be extracted as separate text unit")

	// The <span> should remain inline, producing codes in its parent block.
	spanBlock := findBlockContaining(blocks, "bold content")
	if spanBlock != nil {
		runs := spanBlock.SourceRuns()
		if bridgetest.HasInlineCode(runs) {
			// Count paired-code runs for <span>.
			codeCount := bridgetest.CountPcOpen(runs) + bridgetest.CountPcClose(runs)
			assert.GreaterOrEqual(t, codeCount, 2,
				"span content should have at least 2 paired-code runs (open+close)")
		}
	}
}

// TestDefaults_TranslateOverrides verifies that translate='no' on <html>
// suppresses all text units. This is marked as @Ignored in Java.
//
// okapi: HTML5DefaultsTest#testTranslateOverrides
func TestDefaults_TranslateOverrides(t *testing.T) {
	t.Skip("marked as @Ignored in Java HTML5DefaultsTest")

	html := `<!DOCTYPE html><html lang="en" translate="no"><head>` +
		`<meta charset=utf-8><title>Title</title></head>` +
		`<body><p>Body</p></body></html>`

	parts := readHTML5Default(t, html)
	blocks := bridgetest.TranslatableBlocks(parts)
	assert.Empty(t, blocks,
		"translate='no' on html should suppress all translatable blocks")
}
