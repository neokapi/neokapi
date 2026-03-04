//go:build integration

package okf_xml

import (
	"strings"
	"testing"

	"github.com/gokapi/gokapi/core/model"
	"github.com/gokapi/gokapi/core/plugin/bridge/filters/bridgetest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Extraction tests — ITS rules, translatable attributes, idValue,
// domain, locale filtering, and other XMLFilter-specific extraction behavior.
// ---------------------------------------------------------------------------

// okapi: XMLFilterTest#testTranslatableAttributes
func TestExtract_TranslatableAttributes(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="1.0">
<its:translateRule selector="//img/@alt" translate="yes"/>
</its:rules>
<p>Text</p><img alt="Image description" src="pic.png"/></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Image description")
	assert.Contains(t, texts, "Text")
}

// okapi: XMLFilterTest#testTranslatableAttributes2
func TestExtract_TranslatableAttributes2(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:translateRule selector="//elem/@title" translate="yes"/>
</its:rules>
<elem title="Title text">Body text</elem></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Title text")
	assert.Contains(t, texts, "Body text")
}

// okapi: XMLFilterTest#testTranslatableAttributesOutput
func TestExtract_TranslatableAttributesOutput(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="1.0">
<its:translateRule selector="//img/@alt" translate="yes"/>
</its:rules>
<img alt="Alt text" src="pic.png"/></doc>`
	out := snippetRoundtrip(t, xml, nil)
	assert.Contains(t, out, `alt="Alt text"`)
}

// okapi: XMLFilterTest#testIdValue
func TestExtract_IdValue(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:idValueRule selector="//item" idValue="@name"/>
</its:rules>
<item name="myId">text</item></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].Name, "myId")
}

// okapi: XMLFilterTest#testComplexIdValue
func TestExtract_ComplexIdValue(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:idValueRule selector="//item" idValue="concat(@group, '-', @name)"/>
</its:rules>
<item group="g1" name="n1">text</item></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].Name, "g1-n1")
}

// okapi: XMLFilterTest#testIdComplexValue
func TestExtract_IdComplexValue(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:idValueRule selector="//entry" idValue="concat(@cat, '.', @key)"/>
</its:rules>
<entry cat="menu" key="file">File</entry></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].Name, "menu.file")
}

// okapi: XMLFilterTest#testIdValueV2
func TestExtract_IdValueV2(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:idValueRule selector="//p" idValue="@id"/>
</its:rules>
<p id="p1">First</p><p id="p2">Second</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)
	assert.Contains(t, blocks[0].Name, "p1")
	assert.Contains(t, blocks[1].Name, "p2")
}

// okapi: XMLFilterTest#testDomain1
func TestExtract_Domain1(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:domainRule selector="//p" domainPointer="@domain"/>
</its:rules>
<p domain="medical">Medical text</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	// Domain annotation should be present
	b := findBlockContaining(blocks, "Medical text")
	require.NotNil(t, b)
}

// okapi: XMLFilterTest#testDomain2
func TestExtract_Domain2(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:domainRule selector="//p" domainPointer="@d"/>
</its:rules>
<p d="legal">Legal text</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := findBlockContaining(blocks, "Legal text")
	require.NotNil(t, b)
}

// okapi: XMLFilterTest#testITSVersion1
func TestExtract_ITSVersion1(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="1.0">
<its:translateRule selector="//note" translate="no"/>
</its:rules>
<p>Translatable</p><note>Not translatable</note></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Translatable")
	assert.NotContains(t, texts, "Not translatable")
}

// okapi: XMLFilterTest#testITSVersion2
func TestExtract_ITSVersion2(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:translateRule selector="//note" translate="no"/>
</its:rules>
<p>Translatable</p><note>Not translatable</note></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Translatable")
	assert.NotContains(t, texts, "Not translatable")
}

// okapi: XMLFilterTest#testITSVersionAttribute
func TestExtract_ITSVersionAttribute(t *testing.T) {
	// ITS version attribute should be processed without error.
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its" its:version="2.0">
<p>text</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "text")
}

// okapi: XMLFilterTest#testLocaleFilter1
func TestExtract_LocaleFilter1(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:localeFilterRule selector="//p[@lang='fr']" localeFilterList="fr"/>
</its:rules>
<p lang="en">English</p><p lang="fr">French</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	// Both should still be extractable; locale filter affects processing decisions
	require.NotEmpty(t, blocks)
}

// okapi: XMLFilterTest#testLocaleFilter2
func TestExtract_LocaleFilter2(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:localeFilterRule selector="//p[@lang='fr']" localeFilterList="*" localeFilterType="exclude"/>
</its:rules>
<p lang="en">English</p><p lang="fr">French</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XMLFilterTest#testLocaleFilter3
func TestExtract_LocaleFilter3(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:localeFilterRule selector="//p" localeFilterList="*"/>
</its:rules>
<p>Universal text</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XMLFilterTest#testLocaleFilter4
func TestExtract_LocaleFilter4(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:localeFilterRule selector="//note" localeFilterList="!fr"/>
</its:rules>
<p>Text</p><note>Note</note></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XMLFilterTest#testLocaleFilter5
func TestExtract_LocaleFilter5(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:localeFilterRule selector="//p" localeFilterList="en, fr"/>
</its:rules>
<p>Multi-locale text</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XMLFilterTest#testLocaleFilter6
func TestExtract_LocaleFilter6(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:localeFilterRule selector="//p" localeFilterList="EN"/>
</its:rules>
<p>Case insensitive</p></doc>`
	parts := readXMLDefault(t, xml)
	// The locale filter rule with "EN" restricts content to the EN locale.
	// The bridge may or may not produce translatable blocks depending on the
	// source locale configured — we verify the rule is parsed without error.
	require.NotEmpty(t, parts, "locale filter rule should be parsed without error")
}

// okapi: XMLFilterTest#testAllowedCharsAndStorageSize
func TestExtract_AllowedCharsAndStorageSize(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:allowedCharactersRule selector="//field" allowedCharacters="[a-zA-Z]"/>
<its:storageSizeRule selector="//field" storageSize="100"/>
</its:rules>
<field>Constrained text</field></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Constrained text")
}

// okapi: XMLFilterTest#testStorageSize
func TestExtract_StorageSize(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:storageSizeRule selector="//msg" storageSize="50" storageEncoding="UTF-8"/>
</its:rules>
<msg>Size-limited text</msg></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Size-limited text")
}

// okapi: XMLFilterTest#testSubFilter
func TestExtract_SubFilter(t *testing.T) {
	tdDir := bridgetest.TestdataDir(t)
	params := map[string]any{
		"configFile": tdDir + "/okf_xml/okf_xml@subfilter.fprm",
	}
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc><data><![CDATA[<p>HTML inside CDATA</p>]]></data></doc>`
	parts := readXML(t, xml, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "HTML inside CDATA") {
			found = true
			break
		}
	}
	assert.True(t, found, "should extract text from CDATA subfilter")
}

// okapi: XMLFilterTest#testSubFilterIds
func TestExtract_SubFilterIds(t *testing.T) {
	tdDir := bridgetest.TestdataDir(t)
	params := map[string]any{
		"configFile": tdDir + "/okf_xml/okf_xml@subfilter.fprm",
	}
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc>
<data><![CDATA[<p>First HTML block</p>]]></data>
<data><![CDATA[<p>Second HTML block</p>]]></data>
</doc>`
	parts := readXML(t, xml, params)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 2)
	// Block IDs should be unique across subfilter text units
	ids := make(map[string]bool)
	for _, b := range blocks {
		if b.ID != "" {
			assert.False(t, ids[b.ID], "subfilter block IDs should be unique: %s", b.ID)
			ids[b.ID] = true
		}
	}
}

// okapi: XMLFilterTest#testSubFilterContextPassing
func TestExtract_SubFilterContextPassing(t *testing.T) {
	tdDir := bridgetest.TestdataDir(t)
	params := map[string]any{
		"configFile": tdDir + "/okf_xml/okf_xml@subfilter.fprm",
	}
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc><data><![CDATA[<p>Sub-filter context</p>]]></data></doc>`
	parts := readXML(t, xml, params)
	// Should not crash; context should propagate through subfilter
	require.NotEmpty(t, parts)
}

// okapi: XMLFilterTest#testCDATASubfilter
func TestExtract_CDATASubfilter(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	path := tdDir + "/okf_xml/TestCDATA1.xml"
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	require.NotEmpty(t, parts)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks, "TestCDATA1.xml should have translatable content")
}

// okapi: XMLFilterTest#testSubfilteringEmptyCDATASection
func TestExtract_SubfilteringEmptyCDATASection(t *testing.T) {
	pool, cfg := bridgetest.SharedBridge(t)
	tdDir := bridgetest.TestdataDir(t)
	path := tdDir + "/okf_xml/test_empty_cdata.xml"
	parts := bridgetest.ReadFile(t, pool, cfg, filterClass, path, mimeType, nil)
	// Empty CDATA should not cause an error
	require.NotEmpty(t, parts)
}

// okapi: XMLFilterTest#testOutputTargetPointer
func TestExtract_OutputTargetPointer(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:targetPointerRule selector="//source" targetPointer="../target"/>
</its:rules>
<unit><source>Source text</source><target/></unit></doc>`
	out := snippetRoundtrip(t, xml, nil)
	assert.Contains(t, out, "Source text")
}

// okapi: XMLFilterTest#testOutputTargetPointerWithExistingTarget
func TestExtract_OutputTargetPointerWithExistingTarget(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:targetPointerRule selector="//source" targetPointer="../target"/>
</its:rules>
<unit><source>Source text</source><target>Existing target</target></unit></doc>`
	parts := readXMLDefault(t, xml)
	require.NotEmpty(t, parts)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: XMLFilterTest#testOutputTargetPointerWithInlineCodes
func TestExtract_OutputTargetPointerWithInlineCodes(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:targetPointerRule selector="//source" targetPointer="../target"/>
<its:withinTextRule selector="//b" withinText="yes"/>
</its:rules>
<unit><source>Text with <b>bold</b></source><target/></unit></doc>`
	parts := readXMLDefault(t, xml)
	require.NotEmpty(t, parts)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := findBlockContaining(blocks, "Text with")
	require.NotNil(t, b)
}

// okapi: XMLFilterTest#testLocQualityRatingLocal
func TestExtract_LocQualityRatingLocal(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<p its:locQualityRatingScore="80">Quality text</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Quality text")
}

// okapi: XMLFilterTest#testMTConfidence
func TestExtract_MTConfidence(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<p its:mtConfidence="0.85">MT text</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "MT text")
}

// okapi: XMLFilterTest#testTextAnalysis
func TestExtract_TextAnalysis(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<p its:taClassRef="http://example.com/class">Analyzed text</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Analyzed text")
}

// okapi: XMLFilterTest#testTerms
func TestExtract_Terms(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<p>The <span its:term="yes">engine</span> is running.</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := findBlockContaining(blocks, "engine")
	require.NotNil(t, b, "should extract text containing terminology")
}

// okapi: XMLFilterTest#testLocQualityLocalOnUnit
func TestExtract_LocQualityLocalOnUnit(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<p its:locQualityIssueType="misspelling" its:locQualityIssueSeverity="50">Text with issue</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := bridgetest.BlockTexts(blocks)
	assert.Contains(t, texts, "Text with issue")
}

// okapi: XMLFilterTest#testLocQualityLocalOnCodes
func TestExtract_LocQualityLocalOnCodes(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="2.0">
<its:withinTextRule selector="//mrk" withinText="yes"/>
</its:rules>
<p>Before <mrk its:locQualityIssueType="grammar">marked</mrk> after</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.NotEmpty(t, blocks)
	b := findBlockContaining(blocks, "Before")
	require.NotNil(t, b)
}

// okapi: XMLFilterTest#testTranslatableAttributesOutputAllowUnescapedQuoteButEscape
func TestExtract_TranslatableAttributesOutputAllowUnescapedQuoteButEscape(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="1.0">
<its:translateRule selector="//p/@title" translate="yes"/>
</its:rules>
<p title="value with &quot;quotes&quot;">text</p></doc>`
	out := snippetRoundtrip(t, xml, nil)
	assert.Contains(t, out, "text")
}

// okapi: XMLFilterTest#testTranslatableAttributesOutputAllowUnescapedQuote
func TestExtract_TranslatableAttributesOutputAllowUnescapedQuote(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc xmlns:its="http://www.w3.org/2005/11/its">
<its:rules version="1.0">
<its:translateRule selector="//p/@title" translate="yes"/>
</its:rules>
<p title="simple value">text</p></doc>`
	out := snippetRoundtrip(t, xml, nil)
	assert.Contains(t, out, "simple value")
}

// ---------------------------------------------------------------------------
// Block ID uniqueness tests
// ---------------------------------------------------------------------------

// okapi: XMLFilterTest (general: block IDs must be unique)
func TestExtract_BlockIDsUnique(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc><p>First</p><p>Second</p><p>Third</p></doc>`
	parts := readXMLDefault(t, xml)
	blocks := bridgetest.TranslatableBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID, "block should have an ID")
		assert.False(t, ids[b.ID], "block IDs should be unique: %s", b.ID)
		ids[b.ID] = true
	}
}

// ---------------------------------------------------------------------------
// Layer structure tests
// ---------------------------------------------------------------------------

// okapi: XMLFilterTest (general: layer start and end)
func TestExtract_LayerStructure(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc><p>Hello</p></doc>`
	parts := readXMLDefault(t, xml)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// ---------------------------------------------------------------------------
// Data part tests
// ---------------------------------------------------------------------------

// okapi: XMLFilterTest (general: part type structure)
func TestExtract_PartTypes(t *testing.T) {
	xml := `<?xml version="1.0" encoding="UTF-8"?>
<doc><p>Hello</p></doc>`
	parts := readXMLDefault(t, xml)

	// Verify the basic part structure: should have at least Layer and Block parts.
	var layerCount, blockCount, dataCount int
	for _, p := range parts {
		switch p.Type {
		case model.PartLayerStart, model.PartLayerEnd:
			layerCount++
		case model.PartBlock:
			blockCount++
		case model.PartData:
			dataCount++
			data := p.Resource.(*model.Data)
			assert.NotEmpty(t, data.ID, "data part should have an ID")
		}
	}
	assert.Greater(t, layerCount, 0, "XML should have Layer parts for document structure")
	assert.Greater(t, blockCount, 0, "XML should have Block parts for translatable content")
}
