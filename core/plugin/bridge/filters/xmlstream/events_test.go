//go:build integration

package xmlstream

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/neokapi/neokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Tests translated from XmlStreamEventTest.java (14 tests)
// ---------------------------------------------------------------------------

// okapi: XmlStreamEventTest#testMetaTagContent
func TestEvents_MetaTagContent(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><meta http-equiv="keywords" content="one,two,three"/></doc>`, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "one,two,three")

	// Meta content block should be a referent.
	for _, b := range blocks {
		if b.SourceText() == "one,two,three" {
			assert.True(t, b.IsReferent, "meta content block should be a referent")
			break
		}
	}
}

// okapi: XmlStreamEventTest#testPWithInlines
func TestEvents_PWithInlines(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>Before <b>bold</b> <a href="there"/> after.</p></doc>`, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	paraBlock := findBlockContaining(blocks, "Before")
	require.NotNil(t, paraBlock)
	assert.Equal(t, "paragraph", paraBlock.Type)

	runs := paraBlock.SourceRuns()
	require.NotEmpty(t, runs)
	var inlineCount int
	for _, r := range runs {
		if r.Text == nil {
			inlineCount++
		}
	}
	require.GreaterOrEqual(t, inlineCount, 3)

	var hasOpening, hasClosing, hasPlaceholder bool
	for _, r := range runs {
		switch {
		case r.PcOpen != nil:
			hasOpening = true
		case r.PcClose != nil:
			hasClosing = true
		case r.Ph != nil:
			hasPlaceholder = true
		}
	}
	assert.True(t, hasOpening, "should have PcOpen run for <b>")
	assert.True(t, hasClosing, "should have PcClose run for </b>")
	assert.True(t, hasPlaceholder, "should have Ph run for <a/>")
}

// okapi: XmlStreamEventTest#testPWithInlines2
func TestEvents_PWithInlines2(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>Before <b>bold</b> <img href="there" alt="text"/> after.</p></doc>`, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "text")

	paraBlock := findBlockContaining(blocks, "Before")
	require.NotNil(t, paraBlock)

	runs := paraBlock.SourceRuns()
	require.NotEmpty(t, runs)
	var inlineCount int
	for _, r := range runs {
		if r.Text == nil {
			inlineCount++
		}
	}
	require.GreaterOrEqual(t, inlineCount, 3)
}

// okapi: XmlStreamEventTest#testPWithAttributes
func TestEvents_PWithAttributes(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p title="my title" dir="rtl">Text of p</p></doc>`, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "my title")
	assert.Contains(t, texts, "Text of p")

	for _, b := range blocks {
		if b.SourceText() == "my title" {
			assert.True(t, b.IsReferent, "title attribute block should be a referent")
		}
		if b.SourceText() == "Text of p" {
			assert.Equal(t, "paragraph", b.Type)
		}
	}
}

// okapi: XmlStreamEventTest#testIdOnP
func TestEvents_IdOnP(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p id="foo">text</p></doc>`, params)

	blocks := allBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.Equal(t, "paragraph", b.Type)
	assert.Contains(t, b.Name, "foo")
}

// okapi: XmlStreamEventTest#testLang
func TestEvents_Lang(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><dummy xml:lang="en"/></doc>`, params)

	require.NotEmpty(t, parts)
	dp := findDataPartWithProperty(parts, "language")
	require.NotNil(t, dp, "should have Data part with language property")
	assert.Equal(t, "en", dp.Properties["language"])
}

// okapi: XmlStreamEventTest#testXmlLang
func TestEvents_XmlLang(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><yyy xml:lang="en"/></doc>`, params)

	require.NotEmpty(t, parts)
	dp := findDataPartWithProperty(parts, "language")
	require.NotNil(t, dp, "should have Data part with language property")
	assert.Equal(t, "en", dp.Properties["language"])
}

// okapi: XmlStreamEventTest#testTableGroups
func TestEvents_TableGroups(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><table id="100"><tr><td>text</td></tr></table></doc>`, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "text")

	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// okapi: XmlStreamEventTest#testGroupInPara
func TestEvents_GroupInPara(t *testing.T) {
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

// okapi: XmlStreamEventTest#testPreserveWhitespace
func TestEvents_PreserveWhitespace(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><pre>\twhitespace is preserved</pre></doc>", params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	assert.True(t, b.PreserveWhitespace, "block should have PreserveWhitespace=true")
	text := b.SourceText()
	assert.Contains(t, text, "\t")
	assert.Contains(t, text, "whitespace is preserved")
}

// okapi: XmlStreamEventTest#testExcludeByDefault
func TestEvents_ExcludeByDefault(t *testing.T) {
	params := map[string]any{
		"exclude_by_default": true,
		"parser":             map[string]any{"preserveWhitespace": false},
		"elements": map[string]any{
			".*": map[string]any{
				"ruleTypes":  []string{"INCLUDE"},
				"conditions": []string{"translate", "EQUALS", "y"},
			},
		},
	}
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc>`+
			`<item translate="y">Included</item>`+
			`<item translate="n">Excluded</item>`+
			`<item>Also excluded</item>`+
			`</doc>`, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Included")
	assert.NotContains(t, texts, "Excluded")
	assert.NotContains(t, texts, "Also excluded")
}

// okapi: XmlStreamEventTest#testPWithComment
func TestEvents_PWithComment(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>Before <!--comment--> after.</p></doc>`, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "Before")
	assert.Contains(t, text, "after.")

	runs := b.SourceRuns()
	require.NotEmpty(t, runs)

	var hasPlaceholder bool
	for _, r := range runs {
		if r.Ph != nil {
			hasPlaceholder = true
			break
		}
	}
	assert.True(t, hasPlaceholder, "XML comment should produce a Ph run")
}

// okapi: XmlStreamEventTest#testPWithProcessingInstruction
func TestEvents_PWithProcessingInstruction(t *testing.T) {
	params := defaultXMLParams(t)
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>Before <?PI?> after.</p></doc>`, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "Before")
	assert.Contains(t, text, "after.")

	runs := b.SourceRuns()
	require.NotEmpty(t, runs)

	var hasPlaceholder bool
	for _, r := range runs {
		if r.Ph != nil {
			hasPlaceholder = true
			break
		}
	}
	assert.True(t, hasPlaceholder, "processing instruction should produce a Ph run")
}

// okapi: XmlStreamEventTest#testEntitiesInSkeletonParts
func TestEvents_EntitiesInSkeletonParts(t *testing.T) {
	parts := readXMLDefault(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><text>Hello &amp; World</text></doc>`)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Hello & World")

	// Roundtrip should preserve entities.
	output := snippetRoundtrip(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><text>Hello &amp; World</text></doc>`, nil)
	assert.Contains(t, output, "&amp;")
}
