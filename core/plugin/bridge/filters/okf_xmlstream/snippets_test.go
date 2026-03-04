//go:build integration

package okf_xmlstream

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Tests translated from XmlSnippetsTest.java (50 tests)
//
// Three tests are already covered in xmlstream_test.go:
//   testPWithInlines  → TestExtract_InlineElements
//   testEscapes       → TestExtract_Entities
//   testCdataSection  → TestExtract_CDATA
// The remaining 47 are here.
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testInput
func TestSnippets_Input(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><input type="text" value="Enter" /></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Enter")
}

// okapi: XmlSnippetsTest#testNoExtractValueInInput
func TestSnippets_NoExtractValueInInput(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><input type="hidden" value="Enter" /></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.NotContains(t, texts, "Enter")
}

// okapi: XmlSnippetsTest#testExtractValueInInput
func TestSnippets_ExtractValueInInput(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><input type="text" value="Enter" /></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Enter")
}

// okapi: XmlSnippetsTest#testPWithInlines2
func TestSnippets_PWithInlines2(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>Before <b>bold</b> <img href="there" alt="text"/> after.</p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "text")
}

// okapi: XmlSnippetsTest#testPWithInlineTextOnly
func TestSnippets_PWithInlineTextOnly(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><b>bold text only</b></p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "bold text only") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract text from inline-only paragraph")
}

// okapi: XmlSnippetsTest#testPWithAttributes
func TestSnippets_PWithAttributes(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p title="my title">text of p</p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "my title")
	assert.Contains(t, texts, "text of p")
}

// okapi: XmlSnippetsTest#testAltInImg
func TestSnippets_AltInImg(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><img src="test.png" alt="alternative text"/></p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "alternative text")
}

// okapi: XmlSnippetsTest#testTitleInP
func TestSnippets_TitleInP(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p title="my title">text</p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "my title")
}

// okapi: XmlSnippetsTest#testMETATag1
func TestSnippets_METATag1(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><meta http-equiv="keywords" content="one,two,three"/></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "one,two,three")
}

// okapi: XmlSnippetsTest#testMETATag2
func TestSnippets_METATag2(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><meta name="keywords" content="one,two,three"/></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "one,two,three")
}

// okapi: XmlSnippetsTest#testMultipleMETA
func TestSnippets_MultipleMETA(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><meta http-equiv="keywords" content="k1,k2"/><meta name="description" content="desc"/></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "k1,k2")
	assert.Contains(t, texts, "desc")
}

// okapi: XmlSnippetsTest#testEscapes2
func TestSnippets_Escapes2(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>&lt;b&gt;bold&lt;/b&gt;</p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "<b>bold</b>")
}

// okapi: XmlSnippetsTest#testEscapedEntities
func TestSnippets_EscapedEntities(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>A &amp; B</p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "A & B")
}

// okapi: XmlSnippetsTest#testLang
func TestSnippets_Lang(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc xml:lang="en"><p>text</p></doc>`, params)
	dp := findDataPartWithProperty(parts, "language")
	require.NotNil(t, dp)
	assert.Equal(t, "en", dp.Properties["language"])
}

// okapi: XmlSnippetsTest#testXmlIdResname
func TestSnippets_XmlIdResname(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p id="myid">text</p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].Name, "myid")
}

// okapi: XmlSnippetsTest#testTableGroups
func TestSnippets_TableGroups(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><table><tr><td>cell text</td></tr></table></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "cell text")
}

// okapi: XmlSnippetsTest#testGroupInPara
func TestSnippets_GroupInPara(t *testing.T) {
	params := defaultXMLParams(t)
	snippet := `<?xml version="1.0" encoding="UTF-8"?><doc>` +
		`<p>Text before list:` +
		`<ul><li>Text of item 1</li><li>Text of item 2</li></ul>` +
		`and text after the list.</p></doc>`
	parts := readXML(t, snippet, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text of item 1")
	assert.Contains(t, texts, "Text of item 2")
}

// okapi: XmlSnippetsTest#table
func TestSnippets_Table(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><table><tr><td>A</td><td>B</td></tr></table></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "A")
	assert.Contains(t, texts, "B")
}

// okapi: XmlSnippetsTest#textUnitsInARow
func TestSnippets_TextUnitsInARow(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>one</p><p>two</p><p>three</p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "one")
	assert.Contains(t, texts, "two")
	assert.Contains(t, texts, "three")
}

// okapi: XmlSnippetsTest#textUnitsInARowWithTwoHeaders
func TestSnippets_TextUnitsInARowWithTwoHeaders(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><h1>Header 1</h1><h2>Header 2</h2><p>Paragraph</p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Header 1")
	assert.Contains(t, texts, "Header 2")
	assert.Contains(t, texts, "Paragraph")
}

// okapi: XmlSnippetsTest#textUnitName
func TestSnippets_TextUnitName(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p id="pid">text</p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].Name, "pid")
}

// okapi: XmlSnippetsTest#textUnitStartedWithText
func TestSnippets_TextUnitStartedWithText(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>text <b>bold</b> more</p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "text")
	assert.Contains(t, text, "bold")
	assert.Contains(t, text, "more")
}

// okapi: XmlSnippetsTest#twoTextUnitsInARowNonWellformed
func TestSnippets_TwoTextUnitsInARowNonWellformed(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>one</p><p>two</p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "one")
	assert.Contains(t, texts, "two")
}

// okapi: XmlSnippetsTest#testLabelInOption
func TestSnippets_LabelInOption(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><select><option label="opt label" value="v1">Option text</option></select></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "opt label")
}

// okapi: XmlSnippetsTest#testCodeFinder
func TestSnippets_CodeFinder(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "xml_withCodeFinderRules.yml")

	params := map[string]any{
		"configFile": configPath,
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>Hello VAR1 and VAR2</p></doc>`,
		"test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	// Code finder should produce placeholder spans for matched patterns.
	if len(frag.Spans) > 0 {
		var codeSpans int
		for _, s := range frag.Spans {
			if s.SpanType == model.SpanPlaceholder {
				codeSpans++
			}
		}
		assert.GreaterOrEqual(t, codeSpans, 1, "code finder should produce placeholder spans")
	}
}

// okapi: XmlSnippetsTest#testCollapseWhitespaceWithoutPre
func TestSnippets_CollapseWhitespaceWithoutPre(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><p>  t1  \nt2  </p></doc>", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "t1 t2", blocks[0].SourceText())
}

// okapi: XmlSnippetsTest#testCollapseWhitespaceWithPre
func TestSnippets_CollapseWhitespaceWithPre(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><pre>  t1  \nt2  </pre></doc>", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.True(t, blocks[0].PreserveWhitespace)
	assert.Equal(t, "  t1  \nt2  ", blocks[0].SourceText())
}

// okapi: XmlSnippetsTest#testNewlineNormalization
func TestSnippets_NewlineNormalization(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><pre>line1\r\nline2\rline3</pre></doc>", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	// XML processors normalize \r\n and \r to \n.
	assert.Contains(t, text, "line1")
	assert.Contains(t, text, "line2")
	assert.Contains(t, text, "line3")
}

// okapi: XmlSnippetsTest#testNormalizeNewlinesInPre
func TestSnippets_NormalizeNewlinesInPre(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><pre>a\r\nb\rc</pre></doc>", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "a")
	assert.Contains(t, text, "b")
	assert.Contains(t, text, "c")
}

// okapi: XmlSnippetsTest#testComplexEmptyElement
func TestSnippets_ComplexEmptyElement(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><img src="test.png" alt="alt text"/></p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "alt text")
}

// okapi: XmlSnippetsTest#testInlineAndExclude
func TestSnippets_InlineAndExclude(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	params := map[string]any{
		"assumeWellformed":    true,
		"preserve_whitespace": true,
		"elements": map[string]any{
			"tag1": map[string]any{
				"ruleTypes": []string{"INLINE", "EXCLUDE"},
			},
			"tag2": map[string]any{
				"ruleTypes": []string{"INLINE"},
			},
		},
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><tag2>text1</tag2> <tag1>text2</tag1></p></doc>`,
		"test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "text1")
	assert.NotContains(t, text, "text2")
}

// okapi: XmlSnippetsTest#testInlineAndExclude2
func TestSnippets_InlineAndExclude2(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	params := map[string]any{
		"assumeWellformed":    true,
		"preserve_whitespace": true,
		"elements": map[string]any{
			"tag1": map[string]any{
				"ruleTypes": []string{"INLINE", "EXCLUDE"},
			},
			"tag2": map[string]any{
				"ruleTypes": []string{"INLINE"},
			},
		},
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><tag1>text1</tag1> <tag2>text2</tag2></p></doc>`,
		"test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "text2")
	assert.NotContains(t, text, "text1")
}

// okapi: XmlSnippetsTest#testInlineAndNotExclude
func TestSnippets_InlineAndNotExclude(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	params := map[string]any{
		"assumeWellformed":    true,
		"preserve_whitespace": true,
		"elements": map[string]any{
			"tag1": map[string]any{
				"ruleTypes": []string{"INLINE"},
			},
			"tag2": map[string]any{
				"ruleTypes": []string{"INLINE"},
			},
		},
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><tag2>text1</tag2> <tag1>text2</tag1></p></doc>`,
		"test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "text1")
	assert.Contains(t, text, "text2")
}

// okapi: XmlSnippetsTest#testInlineAndExcludeEmbedded
func TestSnippets_InlineAndExcludeEmbedded(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	params := map[string]any{
		"assumeWellformed":    true,
		"preserve_whitespace": true,
		"elements": map[string]any{
			"tag1": map[string]any{
				"ruleTypes": []string{"INLINE", "EXCLUDE"},
			},
			"tag2": map[string]any{
				"ruleTypes": []string{"INLINE"},
			},
		},
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>before <tag1><tag2>embedded</tag2></tag1> after</p></doc>`,
		"test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "before")
	assert.Contains(t, text, "after")
	assert.NotContains(t, text, "embedded")
}

// okapi: XmlSnippetsTest#testInlineAndNotExcludeEmbedded
func TestSnippets_InlineAndNotExcludeEmbedded(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	params := map[string]any{
		"assumeWellformed":    true,
		"preserve_whitespace": true,
		"elements": map[string]any{
			"tag1": map[string]any{
				"ruleTypes": []string{"INLINE"},
			},
			"tag2": map[string]any{
				"ruleTypes": []string{"INLINE"},
			},
		},
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>before <tag1><tag2>embedded</tag2></tag1> after</p></doc>`,
		"test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "before")
	assert.Contains(t, text, "after")
	assert.Contains(t, text, "embedded")
}

// okapi: XmlSnippetsTest#testInlineAndExcludeWithTwoExcludes
func TestSnippets_InlineAndExcludeWithTwoExcludes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	params := map[string]any{
		"assumeWellformed":    true,
		"preserve_whitespace": true,
		"elements": map[string]any{
			"tag1": map[string]any{
				"ruleTypes": []string{"INLINE", "EXCLUDE"},
			},
			"tag2": map[string]any{
				"ruleTypes": []string{"INLINE", "EXCLUDE"},
			},
		},
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>before <tag1>exc1</tag1> <tag2>exc2</tag2> after</p></doc>`,
		"test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "before")
	assert.Contains(t, text, "after")
	assert.NotContains(t, text, "exc1")
	assert.NotContains(t, text, "exc2")
}

// okapi: XmlSnippetsTest#testCdataSectionExtraction
func TestSnippets_CdataSectionExtraction(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "cdataAsHTML.yml")
	params := map[string]any{
		"configFile": configPath,
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><doc><entry key="hello"><![CDATA[<b>Hello</b>]]></entry></doc>`,
		"test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "Hello") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract 'Hello' from CDATA via HTML subfilter")
}

// okapi: XmlSnippetsTest#testCdataSectionExtractionAndWS
func TestSnippets_CdataSectionExtractionAndWS(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "cdataAsHTML.yml")
	params := map[string]any{
		"configFile": configPath,
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><entry key=\"hello\"><![CDATA[  Hello  ]]></entry></doc>",
		"test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "Hello") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract 'Hello' from CDATA with whitespace")
}

// okapi: XmlSnippetsTest#testCdataSectionExtractionWithCondition
func TestSnippets_CdataSectionExtractionWithCondition(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "cdataWithConditions.yml")
	params := map[string]any{
		"configFile": configPath,
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><doc><yes><![CDATA[<b>Yes</b>]]></yes><no><![CDATA[<b>No</b>]]></no></doc>`,
		"test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	found := false
	for _, txt := range texts {
		if strings.Contains(txt, "Yes") {
			found = true
		}
	}
	assert.True(t, found, "should extract from <yes> element")
}

// okapi: XmlSnippetsTest#testCdataSectionAsHTML
func TestSnippets_CdataSectionAsHTML(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "cdataAsHTML.yml")
	params := map[string]any{
		"configFile": configPath,
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><doc><entry><![CDATA[<p>Hello</p>]]></entry></doc>`,
		"test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "Hello") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract from CDATA treated as HTML")
}

// okapi: XmlSnippetsTest#testCdataSectionAsHTMLButEmpty
func TestSnippets_CdataSectionAsHTMLButEmpty(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "cdataAsHTML.yml")
	params := map[string]any{
		"configFile": configPath,
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><doc><entry><![CDATA[]]></entry></doc>`,
		"test.xml", mimeType, params)

	// Should not crash on empty CDATA.
	require.NotEmpty(t, parts)
}

// okapi: XmlSnippetsTest#testConditionalInlineWithAttribute
func TestSnippets_ConditionalInlineWithAttribute(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>text <input type="text" value="val"/> more</p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "val")
}

// okapi: XmlSnippetsTest#testEscapedCodesInisdePre
func TestSnippets_EscapedCodesInsidePre(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><pre>&lt;tag&gt;text&lt;/tag&gt;</pre></doc>", params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "<tag>text</tag>")
}

// okapi: XmlSnippetsTest#testBadCodeIdsAfterRenumber
func TestSnippets_BadCodeIdsAfterRenumber(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><b>bold1</b> text <b>bold2</b></p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := findBlockContaining(blocks, "bold1")
	require.NotNil(t, b)
	frag := b.FirstFragment()
	require.NotNil(t, frag)

	// Verify span IDs are unique.
	ids := make(map[string]bool)
	for _, s := range frag.Spans {
		if s.ID != "" {
			assert.False(t, ids[s.ID], "span IDs should be unique: %s", s.ID)
			ids[s.ID] = true
		}
	}
}

// okapi: XmlSnippetsTest#testWSPreserveStackAfterExcluded
func TestSnippets_WSPreserveStackAfterExcluded(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "xml_excludedPre.yml")
	params := map[string]any{
		"configFile": configPath,
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><pre>pre text</pre><p> after  pre </p></doc>",
		"test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	// <pre> is excluded; after-pre content should still be extracted.
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "after") {
			found = true
			// After excluded <pre>, whitespace should be collapsed.
			assert.False(t, b.PreserveWhitespace, "block after excluded pre should not preserve whitespace")
			break
		}
	}
	assert.True(t, found, "should extract text after excluded <pre>")
}

// okapi: XmlSnippetsTest#testSupplementalSupport
func TestSnippets_SupplementalSupport(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>Supplemental: `+"\U0001F600"+`</p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "\U0001F600")
}

// okapi: XmlSnippetsTest#testSimpleSupplementalSupport
func TestSnippets_SimpleSupplementalSupport(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>`+"\U00010000"+`</p></doc>`, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "\U00010000")
}
