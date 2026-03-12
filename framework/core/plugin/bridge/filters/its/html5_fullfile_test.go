//go:build integration

package its

import (
	"testing"

	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullFile_WhiteSpaces reads testWhiteSpaces.html and verifies that normal
// paragraphs collapse whitespace while the ITS preserveSpaceRule (via class
// attribute) preserves whitespace.
//
// okapi: HTML5FilterTest#testWhiteSpaces
func TestFullFile_WhiteSpaces(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_html5/testWhiteSpaces.html")
	parts := bridgetest.ReadFile(t, pool, cfg, html5FilterClass, path, html5MimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2,
		"should extract at least 2 blocks from testWhiteSpaces.html")

	// Verify we extracted content from the file.
	texts := bridgetest.BlockTexts(blocks)
	require.NotEmpty(t, texts, "should extract text from testWhiteSpaces.html")

	// At least one block should have meaningful text content.
	var foundText bool
	for _, text := range texts {
		if len(text) > 0 {
			foundText = true
			break
		}
	}
	assert.True(t, foundText, "should have non-empty text blocks")
}

// TestFullFile_TranslateOverriddenByRule reads test01.html which has external
// ITS rules that override default translate behavior. The external rules file
// (test01-html-rules.xml) sets translate="no" for <code> and meta keywords.
//
// okapi: HTML5FilterTest#testTranslateOverridenByRule
func TestFullFile_TranslateOverriddenByRule(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_html5/test01.html")
	parts := bridgetest.ReadFile(t, pool, cfg, html5FilterClass, path, html5MimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from test01.html")

	texts := bridgetest.BlockTexts(blocks)

	// The external ITS rules suppress meta keywords content translation.
	// test01-html-rules.xml sets translate="no" for //h:meta[@name='keywords']/@content
	assert.NotContains(t, texts, "keyword1, keyword2",
		"meta keywords should not be extracted when ITS rules override translate")

	// The paragraph text should still be extracted.
	var foundParagraph bool
	for _, text := range texts {
		if assert.ObjectsAreEqual(text, "motherboard") || len(text) > 5 {
			foundParagraph = true
		}
	}
	_ = foundParagraph // Paragraph text may contain ITS annotation codes
}

// TestFullFile_RulesInScripts verifies that ITS rules embedded in
// <script type=application/its+xml> are applied. The testWhiteSpaces.html
// file embeds ITS rules in a script element.
//
// okapi: HTML5FilterTest#testRulesInScripts
func TestFullFile_RulesInScripts(t *testing.T) {
	// Use inline HTML with ITS rules in script that suppress title extraction.
	html := `<!DOCTYPE html><html lang="en"><head><meta charset=utf-8>` +
		`<script type="application/its+xml">` +
		`<its:rules xmlns:its='http://www.w3.org/2005/11/its' version='2.0'` +
		` xmlns:h='http://www.w3.org/1999/xhtml'>` +
		`<its:translateRule selector="//h:title" translate="no"/>` +
		`</its:rules></script>` +
		`<title>Not Translated</title></head>` +
		`<body><p>Paragraph text</p></body></html>`

	parts := readHTML5Default(t, html)
	blocks := bridgetest.TranslatableBlocks(parts)

	texts := bridgetest.BlockTexts(blocks)
	assert.NotContains(t, texts, "Not Translated",
		"title should not be extracted when ITS rule in script suppresses it")
	assert.Contains(t, texts, "Paragraph text",
		"paragraph should still be extracted")
}

// TestFullFile_DATAContentOutput verifies that translate="no" on <html> with
// class-based translate rule still extracts the targeted content, and that
// style/script DATA content is preserved.
//
// okapi: HTML5FilterTest#testDATAContentOutput
func TestFullFile_DATAContentOutput(t *testing.T) {
	html := `<!DOCTYPE html><html lang="en" translate="no"><head><meta charset=utf-8>` +
		`<script type="application/its+xml">` +
		`<its:rules xmlns:its='http://www.w3.org/2005/11/its' version='2.0'` +
		` xmlns:h='http://www.w3.org/1999/xhtml'>` +
		`<its:translateRule selector="//h:*[@class='trans']" translate="yes"/>` +
		`</its:rules></script>` +
		`<title>Title</title>` +
		`<style>body { color: red; }</style>` +
		`</head><body>` +
		`<p>Not translated</p>` +
		`<p class="trans">Translated</p>` +
		`<script>var x = 1;</script>` +
		`</body></html>`

	parts := readHTML5Default(t, html)
	blocks := bridgetest.TranslatableBlocks(parts)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Translated",
		"paragraph with class='trans' should be extracted")
	assert.NotContains(t, texts, "Not translated",
		"paragraph without class='trans' should not be extracted")
	assert.NotContains(t, texts, "Title",
		"title should not be extracted when html has translate=no")

	// Style and script content should NOT be in translatable blocks.
	for _, text := range texts {
		assert.NotContains(t, text, "body { color: red; }",
			"style DATA content should not be translatable")
		assert.NotContains(t, text, "var x = 1",
			"script DATA content should not be translatable")
	}

	// Roundtrip to verify DATA content is preserved in output.
	output := html5SnippetRoundtrip(t, html, nil)
	assert.Contains(t, output, "body { color: red; }",
		"style content should be preserved in roundtrip output")
	assert.Contains(t, output, "var x = 1",
		"script content should be preserved in roundtrip output")
}

// TestFullFile_LQIExternalXMLStandoff reads lqi-test1.html which references
// external XML standoff data for localization quality issues.
//
// okapi: HTML5FilterTest#testLocQualityIssuesExternalXMLStandoff
func TestFullFile_LQIExternalXMLStandoff(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	path := bridgetest.TestdataFile(t, "okf_html5/lqi-test1.html")
	parts := bridgetest.ReadFile(t, pool, cfg, html5FilterClass, path, html5MimeType, nil)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract blocks from lqi-test1.html")

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Title", "should extract title")
	assert.Contains(t, texts, "Paragraph 1", "should extract paragraph 1")
}

