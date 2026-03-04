//go:build integration

package okf_xmlstream

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Tests translated from XmlStreamSubfilterTest.java (11 tests)
// ---------------------------------------------------------------------------

// okapi: XmlStreamSubfilterTest#testSimple
func TestSubfilter_Simple(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "subfilter-simple.yml")
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "subfilter-simple.xml")

	params := map[string]any{
		"configFile": configPath,
	}
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, xmlPath, mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	found := false
	for _, txt := range texts {
		if strings.Contains(txt, "Translate me") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract 'Translate me' via subfilter")
}

// okapi: XmlStreamSubfilterTest#testCdataSubfilter
func TestSubfilter_CdataSubfilter(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "simple_cdata.xml")

	params := map[string]any{
		"global_cdata_subfilter": "okf_html",
		"preserve_whitespace":   false,
		"elements": map[string]any{
			"entry": map[string]any{
				"ruleTypes":    []string{"TEXTUNIT"},
				"idAttributes": []string{"key"},
			},
		},
	}
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, xmlPath, mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)

	texts := bridgetest.BlockTexts(blocks)
	found := false
	for _, txt := range texts {
		if strings.Contains(txt, "About") || strings.Contains(txt, "Testing") || strings.Contains(txt, "Test") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract text from CDATA entries")
}

// okapi: XmlStreamSubfilterTest#testCdataSubfilterEmptyElement
func TestSubfilter_CdataSubfilterEmptyElement(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	params := map[string]any{
		"global_cdata_subfilter": "okf_html",
		"preserve_whitespace":   false,
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><doc><text/></doc>`,
		"test.xml", mimeType, params)

	require.NotEmpty(t, parts)
}

// okapi: XmlStreamSubfilterTest#testCdataMerging
func TestSubfilter_CdataMerging(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "multi_cdata.xml")

	params := map[string]any{
		"global_cdata_subfilter": "okf_html",
		"preserve_whitespace":   false,
	}
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, xmlPath, mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XmlStreamSubfilterTest#testNestedTextunits
func TestSubfilter_NestedTextunits(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	params := map[string]any{
		"global_pcdata_subfilter": "okf_html",
		"assumeWellformed":       true,
		"preserve_whitespace":    false,
		"elements": map[string]any{
			".*": map[string]any{
				"ruleTypes": []string{"TEXTUNIT"},
			},
		},
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><doc><text>&lt;p&gt;Hello&lt;/p&gt;</text></doc>`,
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
	assert.True(t, found, "should extract 'Hello' from HTML-encoded PCDATA")
}

// okapi: XmlStreamSubfilterTest#testSubfiltersProduceDistinctTextUnitIds
func TestSubfilter_ProduceDistinctTextUnitIds(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	params := map[string]any{
		"global_pcdata_subfilter": "okf_html",
		"assumeWellformed":       true,
		"preserve_whitespace":    false,
		"elements": map[string]any{
			".*": map[string]any{
				"ruleTypes": []string{"TEXTUNIT"},
			},
		},
	}
	parts := bridgetest.ReadString(t, pool, cfg, filterClass,
		`<?xml version="1.0" encoding="UTF-8"?><doc>`+
			`<text>&lt;p&gt;Text1&lt;/p&gt;</text>`+
			`<text>&lt;p&gt;Text2&lt;/p&gt;</text>`+
			`</doc>`,
		"test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)

	ids := make(map[string]bool)
	for _, b := range blocks {
		if b.ID != "" {
			assert.False(t, ids[b.ID], "block IDs should be unique, found duplicate: %s", b.ID)
			ids[b.ID] = true
		}
	}
}

// okapi: XmlStreamSubfilterTest#testJsonSubfilterEvents
func TestSubfilter_JsonSubfilterEvents(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "subfilter-json.yml")

	params := map[string]any{
		"configFile": configPath,
	}
	xml := `<?xml version="1.0" encoding="UTF-8"?><doc><json>{"key":"value"}</json></doc>`
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, xml, "test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "value") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract 'value' from JSON subfilter")
}

// okapi: XmlStreamSubfilterTest#testJsonSubfilterWithHtmlEvents
func TestSubfilter_JsonSubfilterWithHtmlEvents(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "subfilter-json-html.yml")

	params := map[string]any{
		"configFile": configPath,
	}
	xml := `<?xml version="1.0" encoding="UTF-8"?><doc><json>{"key":"&lt;p&gt;html value&lt;/p&gt;"}</json></doc>`
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, xml, "test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "html value") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract 'html value' from JSON+HTML subfilter")
}

// okapi: XmlStreamSubfilterTest#testTranslateAttributeSubfilter
func TestSubfilter_TranslateAttributeSubfilter(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "translate-attr-subfilter.yml")

	params := map[string]any{
		"configFile": configPath,
	}
	xml := `<?xml version="1.0" encoding="UTF-8"?><doc>` +
		`<foo translate="y">&lt;p&gt;translatable&lt;/p&gt;</foo>` +
		`<foo translate="n">&lt;p&gt;not translatable&lt;/p&gt;</foo>` +
		`</doc>`
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, xml, "test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	found := false
	for _, txt := range texts {
		if strings.Contains(txt, "translatable") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract from elements matching translate condition")
}

// okapi: XmlStreamSubfilterTest#testApplySubfilterOnAttribute
func TestSubfilter_ApplySubfilterOnAttribute(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "subfilter-attributes.yml")
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "subfilter-attributes.xml")

	params := map[string]any{
		"configFile": configPath,
	}
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, xmlPath, mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "should extract translatable blocks from attribute subfilter")
}

// okapi: XmlStreamSubfilterTest#issue375
func TestSubfilter_Issue375(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "Issue375.yml")

	params := map[string]any{
		"configFile": configPath,
	}
	xml := `<?xml version="1.0" encoding="UTF-8"?><doc>` +
		`<solutions><![CDATA[<p>Solution text</p>]]></solutions>` +
		`<other><![CDATA[<p>Other text</p>]]></other>` +
		`</doc>`
	parts := bridgetest.ReadString(t, pool, cfg, filterClass, xml, "test.xml", mimeType, params)

	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	found := false
	for _, txt := range texts {
		if strings.Contains(txt, "Solution") {
			found = true
		}
	}
	assert.True(t, found, "should extract from 'solutions' INCLUDE element")
}

// ---------------------------------------------------------------------------
// Tests from CdataSubfilterWithRegexTest.java (3 tests)
// ---------------------------------------------------------------------------

// okapi: CdataSubfilterWithRegexTest#testDoubleExtractionWithoutSubfilter
func TestCdataRegex_DoubleExtractionWithoutSubfilter(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "cdata_with_group", "xml-freemarker.xml")

	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, xmlPath, mimeType, nil)
}

// okapi: CdataSubfilterWithRegexTest#testDoubleExtractionWithoutRegex
func TestCdataRegex_DoubleExtractionWithoutRegex(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "cdata_with_group", "xml-freemarker.xml")
	configPath := filepath.Join(tdDir, "okf_xmlstream", "cdata_with_group", "okf_xmlstream@cdata.fprm")

	params := map[string]any{
		"configFile": configPath,
	}
	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, xmlPath, mimeType, params)
}

// okapi: CdataSubfilterWithRegexTest#testDoubleExtractionWithRegex
func TestCdataRegex_DoubleExtractionWithRegex(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "cdata_with_group", "xml-freemarker.xml")

	params := map[string]any{
		"global_cdata_subfilter": "okf_html",
		"preserve_whitespace":   true,
		"useCodeFinder":         true,
		"codeFinderRules":       "#v1\ncount.i=1\nrule0=\\$\\{[^}]+\\}",
	}
	content, err := os.ReadFile(xmlPath)
	require.NoError(t, err)
	bridgetest.AssertRoundTripEvents(t, pool, cfg, filterClass, content, xmlPath, mimeType, params)
}

// ---------------------------------------------------------------------------
// Tests from PCdataSubfilterTest.java (5 tests)
// ---------------------------------------------------------------------------

// okapi: PCdataSubfilterTest#testPcdataWithoutEscapes
func TestPcdata_WithoutEscapes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "pcdata_subfilter", "okf_xmlstream@pcdata.fprm")
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "pcdata_subfilter", "success.xml")

	params := map[string]any{
		"configFile": configPath,
	}
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, xmlPath, mimeType, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: PCdataSubfilterTest#testPcdataWithEscapes
func TestPcdata_WithEscapes(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "pcdata_subfilter", "okf_xmlstream@pcdata.fprm")
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "pcdata_subfilter", "failure.xml")

	params := map[string]any{
		"configFile": configPath,
	}
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, xmlPath, mimeType, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: PCdataSubfilterTest#testPcdataHrefReference
func TestPcdata_HrefReference(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "pcdata_subfilter", "okf_xmlstream@pcdata.fprm")
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "pcdata_subfilter", "test_href_reference.xml")

	params := map[string]any{
		"configFile": configPath,
	}
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, xmlPath, mimeType, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: PCdataSubfilterTest#testPcdataHrefReferenceSmall
func TestPcdata_HrefReferenceSmall(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	configPath := filepath.Join(tdDir, "okf_xmlstream", "pcdata_subfilter", "okf_xmlstream@pcdata.fprm")
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "test_href_reference_small.xml")

	params := map[string]any{
		"configFile": configPath,
	}
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, xmlPath, mimeType, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: PCdataSubfilterTest#testPcdataTextUnitToDocumentPartWithHtmlProperty
func TestPcdata_TextUnitToDocumentPartWithHtmlProperty(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	xmlPath := filepath.Join(tdDir, "okf_xmlstream", "test_property_empty_TU.xml")

	params := map[string]any{
		"global_pcdata_subfilter": "okf_html",
		"assumeWellformed":       true,
		"preserve_whitespace":    true,
		"elements": map[string]any{
			"value": map[string]any{
				"ruleTypes": []string{"TEXTUNIT"},
			},
		},
	}
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, xmlPath, mimeType, params)
	require.NotEmpty(t, parts)
}
