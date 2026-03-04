//go:build integration

package okf_properties

import (
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// subfilterParams returns filter params that enable the HTML subfilter.
func subfilterParams() map[string]any {
	return map[string]any{
		"subfilter": "okf_html",
	}
}

// okapi-blocked: PropertiesFilterTest#testWithSubfilter — bridge does not set up FilterConfigurationMapper for subfilter resolution
// okapi: PropertiesFilterTest#testWithSubfilter
func TestSubfilter_BasicHTML(t *testing.T) {
	t.Skip("bridge limitation: Properties filter subfilter requires FilterConfigurationMapper (fcMapper is null)")
	snippet := "Key1=<b>Text with \\u00E3 more <br> test</b>"
	parts := readProps(t, snippet, subfilterParams())

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "Text with ã more")
	assert.Contains(t, text, "test")
}

// okapi-blocked: PropertiesFilterTest#testWithSubfilterTwoParas — bridge does not set up FilterConfigurationMapper for subfilter resolution
// okapi: PropertiesFilterTest#testWithSubfilterTwoParas
func TestSubfilter_TwoParas(t *testing.T) {
	t.Skip("bridge limitation: Properties filter subfilter requires FilterConfigurationMapper (fcMapper is null)")
	snippet := "Key1=<b>Text with \\u00E3 more</b> <p> test"
	parts := readProps(t, snippet, subfilterParams())

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)

	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "test")
}

// okapi-blocked: PropertiesFilterTest#testWithSubfilterWithEmbeddedMessagePH — bridge does not set up FilterConfigurationMapper for subfilter resolution
// okapi: PropertiesFilterTest#testWithSubfilterWithEmbeddedMessagePH
func TestSubfilter_EmbeddedMessagePlaceholders(t *testing.T) {
	t.Skip("bridge limitation: Properties filter subfilter requires FilterConfigurationMapper (fcMapper is null)")
	snippet := "Key1=<b>Text with {1} more {2} test</b>"
	parts := readProps(t, snippet, subfilterParams())

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.GreaterOrEqual(t, len(frag.Spans), 4)
}

// okapi-blocked: PropertiesFilterTest#testWithSubfilterWithHTMLEscapes — bridge does not set up FilterConfigurationMapper for subfilter resolution
// okapi: PropertiesFilterTest#testWithSubfilterWithHTMLEscapes
func TestSubfilter_HTMLEscapes(t *testing.T) {
	t.Skip("bridge limitation: Properties filter subfilter requires FilterConfigurationMapper (fcMapper is null)")
	snippet := "Key1=<b>Text with &amp;=amp test</b>"
	parts := readProps(t, snippet, subfilterParams())

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.GreaterOrEqual(t, len(frag.Spans), 2)
}

// okapi-blocked: PropertiesFilterTest#testWithSubfilterOutput — bridge does not set up FilterConfigurationMapper for subfilter resolution
// okapi: PropertiesFilterTest#testWithSubfilterOutput
func TestSubfilter_Output(t *testing.T) {
	t.Skip("bridge limitation: Properties filter subfilter requires FilterConfigurationMapper (fcMapper is null)")
	snippet := "Key1=<b>Text with &amp;=amp test</b>\n"
	result := snippetRoundtrip(t, snippet, subfilterParams())
	assert.Equal(t, snippet, result)
}

// okapi-blocked: PropertiesFilterTest#testWithSubfilterOutputEscapeExtended — bridge does not set up FilterConfigurationMapper for subfilter resolution
// okapi: PropertiesFilterTest#testWithSubfilterOutputEscapeExtended
func TestSubfilter_OutputEscapeExtended(t *testing.T) {
	t.Skip("bridge limitation: Properties filter subfilter requires FilterConfigurationMapper (fcMapper is null)")
	params := map[string]any{
		"subfilter": "okf_html",
	}

	inSnippet := "key=v\u00c3\u201el\u00c3\u00bc\u00c3\u00a9 w\u00c3\u00aeth <b>html</b>\n"
	outSnippet := "key=v\\u00c3\\u201el\\u00c3\\u00bc\\u00c3\\u00a9 w\\u00c3\\u00aeth <b>html</b>\n"

	result := snippetRoundtrip(t, inSnippet, params)
	assert.Equal(t, outSnippet, result)
}

// okapi-blocked: PropertiesFilterTest#testWithSubfilterOutputDoNotEscapeExtended — bridge does not set up FilterConfigurationMapper for subfilter resolution
// okapi: PropertiesFilterTest#testWithSubfilterOutputDoNotEscapeExtended
func TestSubfilter_OutputDoNotEscapeExtended(t *testing.T) {
	t.Skip("bridge limitation: Properties filter subfilter requires FilterConfigurationMapper (fcMapper is null)")
	params := map[string]any{
		"subfilter":           "okf_html",
		"escapeExtendedChars": false,
	}

	snippet := "key=v\u00c3\u201el\u00c3\u00bc\u00c3\u00a9 w\u00c3\u00aeth <b>html</b>\n"
	result := snippetRoundtrip(t, snippet, params)
	assert.Equal(t, snippet, result)
}

// okapi-blocked: PropertiesFilterTest#testWithSubfilterWithEmbeddedEscapedMessagePH — bridge does not set up FilterConfigurationMapper for subfilter resolution
// okapi: PropertiesFilterTest#testWithSubfilterWithEmbeddedEscapedMessagePH
func TestSubfilter_EmbeddedEscapedMessagePlaceholders(t *testing.T) {
	t.Skip("bridge limitation: Properties filter subfilter requires FilterConfigurationMapper (fcMapper is null)")
	snippet := "Key1=<b>Text with \\{1\\} more \\{2\\} test</b>"
	parts := readProps(t, snippet, subfilterParams())

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	frag := blocks[0].FirstFragment()
	require.NotNil(t, frag)
	assert.Equal(t, 2, len(frag.Spans))
}

// containsText checks if any of the texts equal the given string.
func containsText(texts []string, s string) bool {
	for _, text := range texts {
		if text == s {
			return true
		}
	}
	return false
}

// countSpansByType counts spans of the given type in a fragment.
func countSpansByType(frag *model.Fragment, spanType model.SpanType) int {
	count := 0
	for _, s := range frag.Spans {
		if s.SpanType == spanType {
			count++
		}
	}
	return count
}
