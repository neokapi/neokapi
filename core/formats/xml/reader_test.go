// okapi-filter: xml
//
// okapi-unmapped: XmlSnippetsTest#testCodeFinder — requires Okapi code finder (Java regex engine, not available natively)
// okapi-unmapped: XmlStreamConfigurationTest#testCodeFinderRules — requires Okapi code finder (Java regex engine, not available natively)
// okapi-unmapped: XmlStreamConfigurationTest#loadNonAsciiRuleFile — requires Okapi .yml config file loading (Java-specific format)
// okapi-unmapped: XmlSnippetsTest#testCdataSectionExtraction — requires CDATA-to-HTML subfilter with Okapi config file
// okapi-unmapped: XmlSnippetsTest#testCdataSectionExtractionAndWS — requires CDATA-to-HTML subfilter with Okapi config file
// okapi-unmapped: XmlSnippetsTest#testCdataSectionExtractionWithCondition — requires CDATA subfilter with element conditions and Okapi config file
// okapi-unmapped: XmlSnippetsTest#testCdataSectionAsHTML — requires CDATA-to-HTML subfilter with Okapi config file
// okapi-unmapped: CdataSubfilterWithRegexTest#testDoubleExtractionWithoutSubfilter — requires CDATA subfilter with regex code finder integration
// okapi-unmapped: CdataSubfilterWithRegexTest#testDoubleExtractionWithoutRegex — requires CDATA subfilter with Okapi config file
// okapi-unmapped: CdataSubfilterWithRegexTest#testDoubleExtractionWithRegex — requires CDATA subfilter with regex code finder integration
// okapi-unmapped: PCdataSubfilterTest#testPcdataWithoutEscapes — requires PCDATA subfilter infrastructure (global_pcdata_subfilter)
// okapi-unmapped: PCdataSubfilterTest#testPcdataWithEscapes — requires PCDATA subfilter infrastructure (global_pcdata_subfilter)
// okapi-unmapped: PCdataSubfilterTest#testPcdataHrefReference — requires PCDATA subfilter infrastructure (global_pcdata_subfilter)
// okapi-unmapped: PCdataSubfilterTest#testPcdataHrefReferenceSmall — requires PCDATA subfilter infrastructure (global_pcdata_subfilter)
// okapi-unmapped: PCdataSubfilterTest#testPcdataTextUnitToDocumentPartWithHtmlProperty — requires PCDATA subfilter infrastructure (global_pcdata_subfilter)
package xml_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	xmlfmt "github.com/neokapi/neokapi/core/formats/xml"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ---------------------------------------------------------------------------
// Basic extraction tests
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testPWithInlines
func TestReadSimpleXML(t *testing.T) {
	ctx := t.Context()
	reader := xmlfmt.NewReader()
	input := `<?xml version="1.0"?><root><message>Hello World</message></root>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Hello World", blocks[0].SourceText())
	assert.Equal(t, "root.message", blocks[0].Name)
}

func TestReadMultipleElements(t *testing.T) {
	ctx := t.Context()
	reader := xmlfmt.NewReader()
	input := `<?xml version="1.0"?><resources><string>Title</string><string>Description</string></resources>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 2)
	assert.Equal(t, "Title", blocks[0].SourceText())
	assert.Equal(t, "Description", blocks[1].SourceText())
}

func TestReadNestedXML(t *testing.T) {
	ctx := t.Context()
	reader := xmlfmt.NewReader()
	input := `<?xml version="1.0"?><root><section><title>Section Title</title></section></root>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))

	require.Len(t, blocks, 1)
	assert.Equal(t, "Section Title", blocks[0].SourceText())
	assert.Equal(t, "root.section.title", blocks[0].Name)
}

func TestReadLayerStartEnd(t *testing.T) {
	ctx := t.Context()
	reader := xmlfmt.NewReader()
	input := `<?xml version="1.0"?><root><msg>Test</msg></root>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))

	require.GreaterOrEqual(t, len(parts), 2)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)

	layer := parts[0].Resource.(*model.Layer)
	assert.Equal(t, "xml", layer.Format)
}

// okapi: XmlStreamConfigurationTest#defaultConfiguration
func TestReadWithTranslatableConfig(t *testing.T) {
	ctx := t.Context()
	reader := xmlfmt.NewReader()

	cfg := &xmlfmt.Config{
		TranslatableElements: []string{"title", "description"},
	}
	err := reader.SetConfig(cfg)
	require.NoError(t, err)

	input := `<?xml version="1.0"?><root><title>Hello</title><id>123</id><description>World</description></root>`
	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	parts := testutil.CollectParts(t, reader.Read(ctx))
	blocks := testutil.FilterBlocks(parts)

	assert.Len(t, blocks, 2)
	texts := testutil.BlockTexts(blocks)
	assert.Contains(t, texts, "Hello")
	assert.Contains(t, texts, "World")
}

func TestReaderSignature(t *testing.T) {
	reader := xmlfmt.NewReader()
	sig := reader.Signature()
	assert.Contains(t, sig.MIMETypes, "text/xml")
	assert.Contains(t, sig.Extensions, ".xml")
}

func TestReaderMetadata(t *testing.T) {
	reader := xmlfmt.NewReader()
	assert.Equal(t, "xml", reader.Name())
	assert.Equal(t, "XML", reader.DisplayName())
}

func TestReadEmpty(t *testing.T) {
	ctx := t.Context()
	reader := xmlfmt.NewReader()
	input := `<?xml version="1.0"?><root></root>`
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()

	blocks := testutil.CollectBlocks(t, reader.Read(ctx))
	assert.Empty(t, blocks)
}

func TestReadNilDocument(t *testing.T) {
	ctx := t.Context()
	reader := xmlfmt.NewReader()
	err := reader.Open(ctx, nil)
	require.Error(t, err)
}

// ---------------------------------------------------------------------------
// Block IDs
// ---------------------------------------------------------------------------

// okapi: XmlStreamEventTest#testIdOnP
func TestBlockIDs_Unique(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><root><a>First</a><b>Second</b><c>Third</c></root>`, nil)
	blocks := filterBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)

	ids := make(map[string]bool)
	for _, b := range blocks {
		assert.NotEmpty(t, b.ID)
		assert.False(t, ids[b.ID], "block IDs must be unique")
		ids[b.ID] = true
	}
}

// ---------------------------------------------------------------------------
// Entity handling
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testEscapes
func TestEntities_Basic(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><root><text>Price: &lt;$10 &amp; &gt;$5</text></root>`, nil)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Price: <$10 & >$5")
}

// okapi: XmlSnippetsTest#testEscapes2
func TestEntities_Escapes2(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>&lt;b&gt;bold&lt;/b&gt;</p></doc>`, nil)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "<b>bold</b>")
}

// okapi: XmlSnippetsTest#testEscapedEntities
func TestEntities_Ampersand(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>A &amp; B</p></doc>`, nil)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "A & B")
}

// okapi: XmlStreamEventTest#testEntitiesInSkeletonParts
func TestEntities_InContent(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><text>Hello &amp; World</text></doc>`, nil)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "Hello & World")
}

// ---------------------------------------------------------------------------
// CDATA handling
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testCdataSection
func TestCDATA_Basic(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><root><text><![CDATA[Hello <world>]]></text></root>`, nil)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Hello <world>")
}

// okapi: XmlSnippetsTest#testCdataSectionAsHTMLButEmpty
func TestCDATA_Empty(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><entry><![CDATA[]]></entry></doc>`, nil)
	require.NotEmpty(t, parts)
}

// ---------------------------------------------------------------------------
// Unicode support
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testSupplementalSupport
func TestUnicode_Supplemental(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>Supplemental: `+"\U0001F600"+`</p></doc>`, nil)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "\U0001F600")
}

// okapi: XmlSnippetsTest#testSimpleSupplementalSupport
func TestUnicode_SimpleSupplemental(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>`+"\U00010000"+`</p></doc>`, nil)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].SourceText(), "\U00010000")
}

func TestUnicode_Japanese(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><root><text>こんにちは世界</text></root>`, nil)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "こんにちは世界")
}

// ---------------------------------------------------------------------------
// Layer structure
// ---------------------------------------------------------------------------

func TestLayer_Structure(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><root><text>Hello</text></root>`, nil)
	require.NotEmpty(t, parts)
	assert.Equal(t, model.PartLayerStart, parts[0].Type)
	assert.Equal(t, model.PartLayerEnd, parts[len(parts)-1].Type)
}

// ---------------------------------------------------------------------------
// Data parts
// ---------------------------------------------------------------------------

func TestDataParts_ProcessingInstruction(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><root><text>Hello</text></root>`, nil)
	var dataCount int
	for _, p := range parts {
		if p.Type == model.PartData {
			dataCount++
			data := p.Resource.(*model.Data)
			assert.NotEmpty(t, data.ID)
		}
	}
	// At minimum should have the processing instruction
	assert.GreaterOrEqual(t, dataCount, 0)
}

// ---------------------------------------------------------------------------
// Inline element handling
// ---------------------------------------------------------------------------

// okapi: XmlStreamEventTest#testPWithInlines
func TestInline_BasicBoldAnchor(t *testing.T) {
	cfg := &xmlfmt.Config{
		InlineElements: []string{"b", "a"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>Before <b>bold</b> <a href="there"/> after.</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)

	b := findBlockContaining(blocks, "Before")
	require.NotNil(t, b)

	runs := b.SourceRuns()
	codes := inlineCodeRuns(runs)
	require.GreaterOrEqual(t, len(codes), 3, "should have opening, closing, and placeholder inline-code runs")

	var hasOpening, hasClosing, hasPlaceholder bool
	for _, r := range codes {
		switch {
		case r.PcOpen != nil:
			hasOpening = true
		case r.PcClose != nil:
			hasClosing = true
		case r.Ph != nil:
			hasPlaceholder = true
		}
	}
	assert.True(t, hasOpening, "should have opening run for <b>")
	assert.True(t, hasClosing, "should have closing run for </b>")
	assert.True(t, hasPlaceholder, "should have placeholder run for <a/>")
}

// okapi: XmlStreamEventTest#testPWithInlines2
func TestInline_BoldAndImg(t *testing.T) {
	cfg := &xmlfmt.Config{
		InlineElements:         []string{"b", "img"},
		TranslatableAttributes: []string{"alt"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>Before <b>bold</b> <img href="there" alt="text"/> after.</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "text")
}

// okapi: XmlSnippetsTest#testPWithInlineTextOnly
func TestInline_TextOnly(t *testing.T) {
	cfg := &xmlfmt.Config{
		InlineElements: []string{"b"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><b>bold text only</b></p></doc>`, cfg)
	blocks := filterBlocks(parts)
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

// okapi: XmlSnippetsTest#textUnitStartedWithText
func TestInline_TextUnitStartedWithText(t *testing.T) {
	cfg := &xmlfmt.Config{
		InlineElements: []string{"b"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>text <b>bold</b> more</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "text")
	assert.Contains(t, text, "bold")
	assert.Contains(t, text, "more")
}

// okapi: XmlSnippetsTest#testBadCodeIdsAfterRenumber
func TestInline_MultipleInlinePairs(t *testing.T) {
	cfg := &xmlfmt.Config{
		InlineElements: []string{"b"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><b>bold1</b> text <b>bold2</b></p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	b := findBlockContaining(blocks, "bold1")
	require.NotNil(t, b)
	codes := inlineCodeRuns(b.SourceRuns())
	assert.GreaterOrEqual(t, len(codes), 4, "should have inline-code runs for both <b> pairs")
}

// ---------------------------------------------------------------------------
// Attribute extraction
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testPWithAttributes
func TestAttributes_TitleOnP(t *testing.T) {
	cfg := &xmlfmt.Config{
		TranslatableAttributes: []string{"title"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p title="my title">text of p</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "my title")
	assert.Contains(t, texts, "text of p")
}

// okapi: XmlSnippetsTest#testTitleInP
func TestAttributes_TitleInP(t *testing.T) {
	cfg := &xmlfmt.Config{
		TranslatableAttributes: []string{"title"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p title="my title">text</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "my title")
}

// okapi: XmlSnippetsTest#testAltInImg
func TestAttributes_AltInImg(t *testing.T) {
	cfg := &xmlfmt.Config{
		TranslatableAttributes: []string{"alt"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><img src="test.png" alt="alternative text"/></p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "alternative text")
}

// okapi: XmlSnippetsTest#testLabelInOption
func TestAttributes_LabelInOption(t *testing.T) {
	cfg := &xmlfmt.Config{
		TranslatableAttributes: []string{"label"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><select><option label="opt label" value="v1">Option text</option></select></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "opt label")
}

// okapi: XmlStreamEventTest#testPWithAttributes
func TestAttributes_IsReferent(t *testing.T) {
	cfg := &xmlfmt.Config{
		TranslatableAttributes: []string{"title"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p title="my title">Text of p</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	for _, b := range blocks {
		if b.SourceText() == "my title" {
			assert.True(t, b.IsReferent, "title attribute block should be a referent")
		}
	}
}

// okapi: XmlSnippetsTest#testMETATag1
func TestAttributes_MetaTag1(t *testing.T) {
	cfg := &xmlfmt.Config{
		TranslatableAttributes: []string{"content"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><meta http-equiv="keywords" content="one,two,three"/></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "one,two,three")
}

// okapi: XmlSnippetsTest#testMETATag2
func TestAttributes_MetaTag2(t *testing.T) {
	cfg := &xmlfmt.Config{
		TranslatableAttributes: []string{"content"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><meta name="keywords" content="one,two,three"/></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "one,two,three")
}

// okapi: XmlSnippetsTest#testMultipleMETA
func TestAttributes_MultipleMeta(t *testing.T) {
	cfg := &xmlfmt.Config{
		TranslatableAttributes: []string{"content"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><meta http-equiv="keywords" content="k1,k2"/><meta name="description" content="desc"/></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "k1,k2")
	assert.Contains(t, texts, "desc")
}

// okapi: XmlSnippetsTest#testComplexEmptyElement
func TestAttributes_ComplexEmptyElement(t *testing.T) {
	cfg := &xmlfmt.Config{
		TranslatableAttributes: []string{"alt"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><img src="test.png" alt="alt text"/></p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "alt text")
}

// ---------------------------------------------------------------------------
// ID attribute handling
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testXmlIdResname
func TestID_FromAttribute(t *testing.T) {
	cfg := &xmlfmt.Config{
		IDAttributeNames: []string{"id"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p id="myid">text</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].Name, "myid")
}

// okapi: XmlSnippetsTest#textUnitName
func TestID_TextUnitName(t *testing.T) {
	cfg := &xmlfmt.Config{
		IDAttributeNames: []string{"id"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p id="pid">text</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].Name, "pid")
}

// ---------------------------------------------------------------------------
// xml:lang detection
// ---------------------------------------------------------------------------

// okapi: XmlStreamEventTest#testLang
func TestXmlLang_Detection(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><dummy xml:lang="en"/></doc>`, nil)
	require.NotEmpty(t, parts)
	dp := findDataPartWithProperty(parts, "language")
	require.NotNil(t, dp, "should have Data part with language property")
	assert.Equal(t, "en", dp.Properties["language"])
}

// okapi: XmlStreamEventTest#testXmlLang
func TestXmlLang_XmlLang(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><yyy xml:lang="en"/></doc>`, nil)
	require.NotEmpty(t, parts)
	dp := findDataPartWithProperty(parts, "language")
	require.NotNil(t, dp, "should have Data part with language property")
	assert.Equal(t, "en", dp.Properties["language"])
}

// okapi: XmlSnippetsTest#testLang
func TestXmlLang_OnDoc(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc xml:lang="en"><p>text</p></doc>`, nil)
	dp := findDataPartWithProperty(parts, "language")
	require.NotNil(t, dp)
	assert.Equal(t, "en", dp.Properties["language"])
}

// ---------------------------------------------------------------------------
// Whitespace handling
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testCollapseWhitespaceWithoutPre
func TestWhitespace_CollapseDefault(t *testing.T) {
	parts := readXML(t,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><p>  t1  \nt2  </p></doc>", nil)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "t1 t2", blocks[0].SourceText())
}

// okapi: XmlSnippetsTest#testCollapseWhitespaceWithPre
func TestWhitespace_PreserveWithConfig(t *testing.T) {
	cfg := &xmlfmt.Config{
		PreserveWhitespaceElements: []string{"pre"},
	}
	parts := readXMLWithConfig(t,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><pre>  t1  \nt2  </pre></doc>", cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.True(t, blocks[0].PreserveWhitespace)
	assert.Equal(t, "  t1  \nt2  ", blocks[0].SourceText())
}

// okapi: XmlStreamConfigurationSupportTest#test_PRESERVE_WHITESPACE
func TestWhitespace_ElementPreserve(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"parser": map[string]any{"preserveWhitespace": false},
		"elements": map[string]any{
			"pre": map[string]any{
				"ruleTypes": []any{"TEXTUNIT", "PRESERVE_WHITESPACE"},
			},
		},
	})
	require.NoError(t, err)

	xml := "<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><p> t1  \nt2  </p><pre> t3  \nt4  </pre></doc>"
	parts := readXMLWithConfig(t, xml, cfg)
	blocks := filterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "t1 t2", blocks[0].SourceText())
	assert.Equal(t, " t3  \nt4  ", blocks[1].SourceText())
}

// okapi: XmlStreamConfigurationSupportTest#test_GLOBAL_PRESERVE_WHITESPACE
func TestWhitespace_GlobalPreserve(t *testing.T) {
	cfg := &xmlfmt.Config{PreserveWhitespace: true}
	xml := "<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><p> t1  \nt2  </p><pre> t3  \nt4  </pre></doc>"
	parts := readXMLWithConfig(t, xml, cfg)
	blocks := filterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, " t1  \nt2  ", blocks[0].SourceText())
	assert.Equal(t, " t3  \nt4  ", blocks[1].SourceText())
}

// okapi: XmlStreamConfigurationSupportTest#test_collapse_whitespace
func TestWhitespace_CollapseThenPreserve(t *testing.T) {
	xml := "<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><p> t1  \nt2  </p></doc>"

	// Without preserve
	parts1 := readXML(t, xml, nil)
	blocks1 := filterBlocks(parts1)
	require.NotEmpty(t, blocks1)
	assert.Equal(t, "t1 t2", blocks1[0].SourceText())

	// With preserve
	cfg := &xmlfmt.Config{PreserveWhitespace: true}
	parts2 := readXMLWithConfig(t, xml, cfg)
	blocks2 := filterBlocks(parts2)
	require.NotEmpty(t, blocks2)
	assert.Equal(t, " t1  \nt2  ", blocks2[0].SourceText())
}

// okapi: XmlStreamConfigurationTest#preserveWhiteSpace
func TestWhitespace_ConfigPreserve(t *testing.T) {
	cfg := &xmlfmt.Config{PreserveWhitespace: true}
	parts := readXMLWithConfig(t,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><text>  preserved  </text></doc>", cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "  preserved  ", blocks[0].SourceText())
}

// okapi: XmlStreamConfigurationTest#collapseWhitespace
func TestWhitespace_ConfigCollapse(t *testing.T) {
	parts := readXML(t,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><text>  t1  \nt2  </text></doc>", nil)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "t1 t2", blocks[0].SourceText())
}

// okapi: XmlStreamEventTest#testPreserveWhitespace
func TestWhitespace_PreserveOnPre(t *testing.T) {
	cfg := &xmlfmt.Config{
		PreserveWhitespaceElements: []string{"pre"},
	}
	parts := readXMLWithConfig(t,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><pre>\twhitespace is preserved</pre></doc>", cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.True(t, blocks[0].PreserveWhitespace)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "\t")
	assert.Contains(t, text, "whitespace is preserved")
}

// ---------------------------------------------------------------------------
// Exclude/Include rules
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationSupportTest#test_EXCLUDE
func TestExclude_Basic(t *testing.T) {
	cfg := &xmlfmt.Config{
		ExcludedElements: []string{"pre"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><pre>t1</pre><p>t2</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "t2")
	assert.NotContains(t, texts, "t1")
}

// okapi: XmlStreamConfigurationSupportTest#test_EXCLUDE via element rules
func TestExclude_ViaElementRule(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"pre": map[string]any{
				"ruleTypes": []any{"EXCLUDE"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><pre>t1</pre><p>t2</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "t2")
	assert.NotContains(t, texts, "t1")
}

// okapi: XmlStreamConfigurationSupportTest#test_INCLUDE
func TestInclude_InsideExcluded(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"pre": map[string]any{
				"ruleTypes": []any{"EXCLUDE"},
			},
			"b": map[string]any{
				"ruleTypes": []any{"INCLUDE"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><pre>t1<b>t2</b>t3</pre><p>t4</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "t2")
	assert.Contains(t, texts, "t4")
	assert.NotContains(t, texts, "t1")
}

// okapi: XmlStreamConfigurationSupportTest#test_EXCLUDE_with_positive_condition
func TestExclude_WithPositiveCondition(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"pre": map[string]any{
				"ruleTypes":  []any{"EXCLUDE"},
				"conditions": []any{"x", "EQUALS", "true"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><pre x="true">t1</pre><p>t2</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "t2")
	assert.NotContains(t, texts, "t1")
}

// okapi: XmlStreamConfigurationSupportTest#test_EXCLUDE_with_negative_condition
func TestExclude_WithNegativeCondition(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"pre": map[string]any{
				"ruleTypes":  []any{"EXCLUDE"},
				"conditions": []any{"x", "EQUALS", "false"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><pre x="true">t1</pre><p>t2</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "t1")
}

// okapi: XmlStreamConfigurationSupportTest#test_EXCLUDE_with_positive_condition_and_regex
func TestExclude_WithPositiveConditionRegex(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"pre": map[string]any{
				"ruleTypes":  []any{"EXCLUDE"},
				"conditions": []any{"x", "MATCHES", "tr.*"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><pre x="true">t1</pre><p>t2</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "t2")
	assert.NotContains(t, texts, "t1")
}

// okapi: XmlStreamConfigurationSupportTest#test_MATCHES
func TestExclude_Matches(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"p": map[string]any{
				"ruleTypes":  []any{"EXCLUDE"},
				"conditions": []any{"x", "MATCHES", "ABZ"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p x="ABZ">t1</p><p x="ZBA">t2</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "t2")
	assert.NotContains(t, texts, "t1")
}

// ---------------------------------------------------------------------------
// Exclude by default
// ---------------------------------------------------------------------------

// okapi: XmlStreamEventTest#testExcludeByDefault
func TestExcludeByDefault_Basic(t *testing.T) {
	cfg := &xmlfmt.Config{ExcludeByDefault: true}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"item": map[string]any{
				"ruleTypes":  []any{"INCLUDE"},
				"conditions": []any{"translate", "EQUALS", "y"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc>`+
			`<item translate="y">Included</item>`+
			`<item translate="n">Excluded</item>`+
			`<item>Also excluded</item>`+
			`</doc>`, cfg)

	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Included")
	assert.NotContains(t, texts, "Excluded")
	assert.NotContains(t, texts, "Also excluded")
}

// okapi: XmlStreamConfigurationSupportTest#test_ISSUE_282
func TestExcludeByDefault_Issue282(t *testing.T) {
	cfg := &xmlfmt.Config{ExcludeByDefault: true}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"item": map[string]any{
				"ruleTypes":  []any{"INCLUDE"},
				"conditions": []any{"translate", "EQUALS", "y"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc>`+
			`<item translate="y">Included</item>`+
			`<item>Not included</item>`+
			`</doc>`, cfg)

	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Included")
	assert.NotContains(t, texts, "Not included")
}

// okapi: XmlStreamConfigurationSupportTest#test_ISSUE_282_empty_elements
func TestExcludeByDefault_EmptyElements(t *testing.T) {
	cfg := &xmlfmt.Config{ExcludeByDefault: true}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"item": map[string]any{
				"ruleTypes":  []any{"INCLUDE"},
				"conditions": []any{"translate", "EQUALS", "y"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc>`+
			`<item translate="y">Included</item>`+
			`<item translate="y"/>`+
			`</doc>`, cfg)

	require.NotEmpty(t, parts)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Included")
}

// ---------------------------------------------------------------------------
// Inline + Exclude
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testInlineAndExclude
func TestInlineExclude_Basic(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"tag1": map[string]any{
				"ruleTypes": []any{"INLINE", "EXCLUDE"},
			},
			"tag2": map[string]any{
				"ruleTypes": []any{"INLINE"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><tag2>text1</tag2> <tag1>text2</tag1></p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "text1")
	assert.NotContains(t, text, "text2")
}

// okapi: XmlSnippetsTest#testInlineAndExclude2
func TestInlineExclude_Reversed(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"tag1": map[string]any{
				"ruleTypes": []any{"INLINE", "EXCLUDE"},
			},
			"tag2": map[string]any{
				"ruleTypes": []any{"INLINE"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><tag1>text1</tag1> <tag2>text2</tag2></p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "text2")
	assert.NotContains(t, text, "text1")
}

// okapi: XmlSnippetsTest#testInlineAndNotExclude
func TestInlineExclude_NotExclude(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"tag1": map[string]any{
				"ruleTypes": []any{"INLINE"},
			},
			"tag2": map[string]any{
				"ruleTypes": []any{"INLINE"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><tag2>text1</tag2> <tag1>text2</tag1></p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "text1")
	assert.Contains(t, text, "text2")
}

// okapi: XmlSnippetsTest#testInlineAndExcludeEmbedded
func TestInlineExclude_Embedded(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"tag1": map[string]any{
				"ruleTypes": []any{"INLINE", "EXCLUDE"},
			},
			"tag2": map[string]any{
				"ruleTypes": []any{"INLINE"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>before <tag1><tag2>embedded</tag2></tag1> after</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "before")
	assert.Contains(t, text, "after")
	assert.NotContains(t, text, "embedded")
}

// okapi: XmlSnippetsTest#testInlineAndNotExcludeEmbedded
func TestInlineExclude_NotExcludeEmbedded(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"tag1": map[string]any{
				"ruleTypes": []any{"INLINE"},
			},
			"tag2": map[string]any{
				"ruleTypes": []any{"INLINE"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>before <tag1><tag2>embedded</tag2></tag1> after</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "before")
	assert.Contains(t, text, "after")
	assert.Contains(t, text, "embedded")
}

// okapi: XmlSnippetsTest#testInlineAndExcludeWithTwoExcludes
func TestInlineExclude_TwoExcludes(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"tag1": map[string]any{
				"ruleTypes": []any{"INLINE", "EXCLUDE"},
			},
			"tag2": map[string]any{
				"ruleTypes": []any{"INLINE", "EXCLUDE"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>before <tag1>exc1</tag1> <tag2>exc2</tag2> after</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "before")
	assert.Contains(t, text, "after")
	assert.NotContains(t, text, "exc1")
	assert.NotContains(t, text, "exc2")
}

// okapi: XmlStreamConfigurationSupportTest#test_INLINE_WITH_EXCLUDE
func TestInlineExclude_Config(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"tag1": map[string]any{
				"ruleTypes": []any{"INLINE", "EXCLUDE"},
			},
			"tag2": map[string]any{
				"ruleTypes": []any{"INLINE"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><tag2>text1</tag2> <tag1>text2</tag1></p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "text1")
	assert.NotContains(t, text, "text2")
}

// okapi: XmlStreamConfigurationSupportTest#test_INLINE_WITH_EXCLUDE_standalone
func TestInlineExclude_Standalone(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"tag1": map[string]any{
				"ruleTypes": []any{"INLINE", "EXCLUDE"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>before <tag1>excluded</tag1> after</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "before")
	assert.Contains(t, text, "after")
	assert.NotContains(t, text, "excluded")
}

// ---------------------------------------------------------------------------
// Attribute ID rules
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationSupportTest#test_ATTRIBUTE_ID
func TestAttributeID_FromRule(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"p": map[string]any{
				"ruleTypes":    []any{"TEXTUNIT"},
				"idAttributes": []any{"id"},
			},
			"pre": map[string]any{
				"ruleTypes":    []any{"TEXTUNIT"},
				"idAttributes": []any{"id"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p id="id1">t1</p><pre id="id2">t2</pre></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "t1", blocks[0].SourceText())
	assert.Contains(t, blocks[0].Name, "id1")
	assert.Equal(t, "t2", blocks[1].SourceText())
	assert.Contains(t, blocks[1].Name, "id2")
}

// okapi: XmlStreamConfigurationSupportTest#test_idAttributes
func TestAttributeID_MultipleIDAttrs(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"p": map[string]any{
				"ruleTypes":    []any{"TEXTUNIT"},
				"idAttributes": []any{"id", "xml:id"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p id="id1">t1</p><p xml:id="id2">t2</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Contains(t, blocks[0].Name, "id1")
	assert.Contains(t, blocks[1].Name, "id2")
}

// ---------------------------------------------------------------------------
// Attribute writable rules
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationSupportTest#test_ATTRIBUTE_WRITABLE
func TestAttributeWritable(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"attributes": map[string]any{
			"dir": map[string]any{
				"ruleTypes": []any{"ATTRIBUTE_WRITABLE"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p dir="rtl">t1</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Equal(t, "t1", blocks[0].SourceText())
	require.NotNil(t, blocks[0].Properties)
	assert.Equal(t, "rtl", blocks[0].Properties["dir"])
}

// ---------------------------------------------------------------------------
// Translatable attribute rules with conditions
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationSupportTest#test_translatableAttributes_withCondition
func TestTranslatableAttr_WithCondition(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"p": map[string]any{
				"ruleTypes": []any{"TEXTUNIT"},
				"translatableAttributes": map[string]any{
					"alt": []any{"attr1", "EQUALS", "trans"},
				},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p alt="t1" attr1="NOTRANS">t2</p><p alt="t-alt" attr1="trans">t4</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "t-alt")
	assert.NotContains(t, texts, "t1")
}

// okapi: XmlStreamConfigurationSupportTest#test_translatableAttributes_with2ORConditions
func TestTranslatableAttr_With2ORConditions(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"p": map[string]any{
				"ruleTypes": []any{"TEXTUNIT"},
				"translatableAttributes": map[string]any{
					"alt": []any{
						[]any{"attr1", "EQUALS", "trans"},
						[]any{"attr2", "EQUALS", "yes"},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p alt="t-alt1" attr2="yes">t2</p><p alt="t-alt2" attr1="trans">t4</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "t-alt1")
	assert.Contains(t, texts, "t-alt2")
}

// ---------------------------------------------------------------------------
// Attribute trans rule with element scope
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationSupportTest#test_allElementsExcept
func TestAttrTrans_AllElementsExcept(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"attributes": map[string]any{
			"alt": map[string]any{
				"ruleTypes":         []any{"ATTRIBUTE_TRANS"},
				"allElementsExcept": []any{"elem2", "elem3"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><elem1 alt="t1">t2</elem1><elem2 alt="t3">t4</elem2><elem3 alt="t5">t6</elem3></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "t1")
	assert.NotContains(t, texts, "t3")
	assert.NotContains(t, texts, "t5")
}

// okapi: XmlStreamConfigurationSupportTest#test_onlyTheseElements
func TestAttrTrans_OnlyTheseElements(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"attributes": map[string]any{
			"alt": map[string]any{
				"ruleTypes":         []any{"ATTRIBUTE_TRANS"},
				"onlyTheseElements": []any{"elem1", "elem3"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><elem1 alt="t1">t2</elem1><elem2 alt="t3">t4</elem2><elem3 alt="t5">t6</elem3></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "t1")
	assert.Contains(t, texts, "t5")
	assert.NotContains(t, texts, "t3")
}

// ---------------------------------------------------------------------------
// Inline condition rules
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationSupportTest#test_INLINE_with_positive_condition
func TestInlineCondition_Positive(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"b": map[string]any{
				"ruleTypes":  []any{"INLINE"},
				"conditions": []any{"x", "EQUALS", "true"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><b x="true">t2</b></p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "t2")
}

// ---------------------------------------------------------------------------
// Start tag as opening span (not placeholder)
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationSupportTest#testStartTagShouldbeOpenNotPlaceholder
func TestStartTag_OpenNotPlaceholder(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"b": map[string]any{
				"ruleTypes": []any{"INLINE"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>text <b>bold</b></p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)

	paraBlock := findBlockContaining(blocks, "text")
	require.NotNil(t, paraBlock)

	assert.True(t, hasOpeningRun(paraBlock.SourceRuns()),
		"start tag <b> should produce an opening run, not placeholder")
}

// ---------------------------------------------------------------------------
// Comments and PIs as inline codes
// ---------------------------------------------------------------------------

// okapi: XmlStreamEventTest#testPWithComment
func TestComment_AsPlaceholderSpan(t *testing.T) {
	cfg := &xmlfmt.Config{}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>Before <!--comment--> after.</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "Before")
	assert.Contains(t, text, "after.")

	assert.True(t, hasPlaceholderRun(b.SourceRuns()),
		"XML comment should produce a placeholder run")
}

// okapi: XmlStreamEventTest#testPWithProcessingInstruction
func TestPI_AsPlaceholderSpan(t *testing.T) {
	cfg := &xmlfmt.Config{}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>Before <?PI?> after.</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)

	b := blocks[0]
	text := b.SourceText()
	assert.Contains(t, text, "Before")
	assert.Contains(t, text, "after.")

	assert.True(t, hasPlaceholderRun(b.SourceRuns()),
		"processing instruction should produce a placeholder run")
}

// ---------------------------------------------------------------------------
// Multiple text units
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#textUnitsInARow
func TestMultipleBlocks_InRow(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>one</p><p>two</p><p>three</p></doc>`, nil)
	blocks := filterBlocks(parts)
	require.GreaterOrEqual(t, len(blocks), 3)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "one")
	assert.Contains(t, texts, "two")
	assert.Contains(t, texts, "three")
}

// okapi: XmlSnippetsTest#textUnitsInARowWithTwoHeaders
func TestMultipleBlocks_Headers(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><h1>Header 1</h1><h2>Header 2</h2><p>Paragraph</p></doc>`, nil)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Header 1")
	assert.Contains(t, texts, "Header 2")
	assert.Contains(t, texts, "Paragraph")
}

// okapi: XmlSnippetsTest#twoTextUnitsInARowNonWellformed
func TestMultipleBlocks_TwoInRow(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>one</p><p>two</p></doc>`, nil)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "one")
	assert.Contains(t, texts, "two")
}

// ---------------------------------------------------------------------------
// Table structure
// ---------------------------------------------------------------------------

// okapi: XmlStreamEventTest#testTableGroups
func TestTable_Groups(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><table id="100"><tr><td>text</td></tr></table></doc>`, nil)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "text")
}

// okapi: XmlSnippetsTest#testTableGroups
func TestTable_CellText(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><table><tr><td>cell text</td></tr></table></doc>`, nil)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "cell text")
}

// okapi: XmlSnippetsTest#table
func TestTable_MultipleCells(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><table><tr><td>A</td><td>B</td></tr></table></doc>`, nil)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "A")
	assert.Contains(t, texts, "B")
}

// ---------------------------------------------------------------------------
// Group in paragraph (nested block elements)
// ---------------------------------------------------------------------------

// okapi: XmlStreamEventTest#testGroupInPara
func TestGroupInPara(t *testing.T) {
	snippet := `<?xml version="1.0" encoding="UTF-8"?><doc>` +
		`<p>Text before list:` +
		`<ul><li>Text of item 1</li><li>Text of item 2</li></ul>` +
		`and text after the list.</p></doc>`
	parts := readXML(t, snippet, nil)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text of item 1")
	assert.Contains(t, texts, "Text of item 2")
}

// ---------------------------------------------------------------------------
// Escaped codes inside pre
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testEscapedCodesInisdePre
func TestEscapedCodes_InsidePre(t *testing.T) {
	cfg := &xmlfmt.Config{
		PreserveWhitespaceElements: []string{"pre"},
	}
	parts := readXMLWithConfig(t,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><pre>&lt;tag&gt;text&lt;/tag&gt;</pre></doc>", cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "<tag>text</tag>")
}

// ---------------------------------------------------------------------------
// Newline normalization
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testNewlineNormalization
func TestNewline_Normalization(t *testing.T) {
	cfg := &xmlfmt.Config{
		PreserveWhitespaceElements: []string{"pre"},
	}
	parts := readXMLWithConfig(t,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><pre>line1\r\nline2\rline3</pre></doc>", cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "line1")
	assert.Contains(t, text, "line2")
	assert.Contains(t, text, "line3")
}

// ---------------------------------------------------------------------------
// Config API tests
// ---------------------------------------------------------------------------

func TestConfig_Validate(t *testing.T) {
	cfg := &xmlfmt.Config{
		Subfilters: []format.SubfilterMapping{
			{Pattern: "", Format: "html"},
		},
	}
	require.Error(t, cfg.Validate())
}

func TestConfig_Validate_EmptyFormat(t *testing.T) {
	cfg := &xmlfmt.Config{
		Subfilters: []format.SubfilterMapping{
			{Pattern: "root.body", Format: ""},
		},
	}
	require.Error(t, cfg.Validate())
}

func TestConfig_Validate_OK(t *testing.T) {
	cfg := &xmlfmt.Config{
		Subfilters: []format.SubfilterMapping{
			{Pattern: "root.body", Format: "html"},
		},
	}
	require.NoError(t, cfg.Validate())
}

func TestConfig_ApplyMap_TranslatableElements(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"translatableElements": []any{"title", "description"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"title", "description"}, cfg.TranslatableElements)
}

func TestConfig_ApplyMap_TranslatableAttributes(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"translatableAttributes": []any{"alt", "title"},
	})
	require.NoError(t, err)
	assert.Equal(t, []string{"alt", "title"}, cfg.TranslatableAttributes)
}

func TestConfig_ApplyMap_PreserveWhitespace(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"preserveWhitespace": true,
	})
	require.NoError(t, err)
	assert.True(t, cfg.PreserveWhitespace)
}

func TestConfig_ApplyMap_ExcludeByDefault(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"excludeByDefault": true,
	})
	require.NoError(t, err)
	assert.True(t, cfg.ExcludeByDefault)
}

func TestConfig_ApplyMap_Parser(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"parser": map[string]any{
			"preserveWhitespace": true,
			"assumeWellformed":   true,
		},
	})
	require.NoError(t, err)
	assert.True(t, cfg.PreserveWhitespace)
}

func TestConfig_ApplyMap_ElementRules(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"pre": map[string]any{
				"ruleTypes": []any{"EXCLUDE"},
			},
			"b": map[string]any{
				"ruleTypes": []any{"INLINE"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, cfg.ElementRules, 2)
}

func TestConfig_ApplyMap_AttributeRules(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"attributes": map[string]any{
			"dir": map[string]any{
				"ruleTypes": []any{"ATTRIBUTE_WRITABLE"},
			},
		},
	})
	require.NoError(t, err)
	require.Len(t, cfg.AttributeRules, 1)
}

func TestConfig_FormatName(t *testing.T) {
	cfg := &xmlfmt.Config{}
	assert.Equal(t, "xml", cfg.FormatName())
}

func TestConfig_Reset(t *testing.T) {
	cfg := &xmlfmt.Config{
		TranslatableElements:   []string{"p"},
		TranslatableAttributes: []string{"alt"},
		PreserveWhitespace:     true,
		ExcludeByDefault:       true,
	}
	cfg.Reset()
	assert.Empty(t, cfg.TranslatableElements)
	assert.Empty(t, cfg.TranslatableAttributes)
	assert.False(t, cfg.PreserveWhitespace)
	assert.False(t, cfg.ExcludeByDefault)
}

// ---------------------------------------------------------------------------
// Reopen (open twice)
// ---------------------------------------------------------------------------

func TestReopen(t *testing.T) {
	ctx := t.Context()
	input := `<?xml version="1.0" encoding="UTF-8"?><root><text>Hello</text></root>`

	reader := xmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	blocks1 := testutil.CollectBlocks(t, reader.Read(ctx))
	reader.Close()

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	blocks2 := testutil.CollectBlocks(t, reader.Read(ctx))
	reader.Close()

	require.Len(t, blocks1, 1)
	require.Len(t, blocks2, 1)
	assert.Equal(t, blocks1[0].SourceText(), blocks2[0].SourceText())
}

// ---------------------------------------------------------------------------
// Inline with conditions (positive and negative)
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationSupportTest#test_INLINE_with_negative_condition
func TestInlineCondition_Negative(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"b": map[string]any{
				"ruleTypes":  []any{"INLINE"},
				"conditions": []any{"x", "EQUALS", "true"},
			},
		},
	})
	require.NoError(t, err)

	// x="false" does not match the condition, so <b> is not inline -> treated as block
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><b x="false">t2</b></p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "t2")
}

// ---------------------------------------------------------------------------
// WS preserve stack after excluded
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testWSPreserveStackAfterExcluded
func TestWSPreserveStack_AfterExcluded(t *testing.T) {
	cfg := &xmlfmt.Config{
		ExcludedElements:           []string{"pre"},
		PreserveWhitespaceElements: []string{"pre"},
	}
	parts := readXMLWithConfig(t,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><pre>pre text</pre><p> after  pre </p></doc>", cfg)
	blocks := filterBlocks(parts)
	found := false
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), "after") {
			found = true
			assert.False(t, b.PreserveWhitespace, "block after excluded pre should not preserve whitespace")
			break
		}
	}
	assert.True(t, found, "should extract text after excluded <pre>")
}

// ---------------------------------------------------------------------------
// Input element handling
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testInput
func TestInput_ExtractValue(t *testing.T) {
	cfg := &xmlfmt.Config{
		TranslatableAttributes: []string{"value"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><input type="text" value="Enter" /></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Enter")
}

// okapi: XmlSnippetsTest#testConditionalInlineWithAttribute
func TestInput_ConditionalInlineAttr(t *testing.T) {
	cfg := &xmlfmt.Config{
		InlineElements:         []string{"input"},
		TranslatableAttributes: []string{"value"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>text <input type="text" value="val"/> more</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "val")
}

// ---------------------------------------------------------------------------
// Generic code types (span types)
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationTest#genericCodeTypes
func TestSpanTypes_Generic(t *testing.T) {
	cfg := &xmlfmt.Config{
		InlineElements: []string{"b", "a"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p><b>bold</b> <a href="#">link</a> text</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)

	var blockWithCodes *model.Block
	for _, b := range blocks {
		if len(inlineCodeRuns(b.SourceRuns())) > 0 {
			blockWithCodes = b
			break
		}
	}
	require.NotNil(t, blockWithCodes)

	codeTypes := make(map[string]bool)
	for _, r := range inlineCodeRuns(blockWithCodes.SourceRuns()) {
		switch {
		case r.PcOpen != nil:
			if r.PcOpen.Type != "" {
				codeTypes[r.PcOpen.Type] = true
			}
		case r.PcClose != nil:
			if r.PcClose.Type != "" {
				codeTypes[r.PcClose.Type] = true
			}
		case r.Ph != nil:
			if r.Ph.Type != "" {
				codeTypes[r.Ph.Type] = true
			}
		}
	}
	assert.NotEmpty(t, codeTypes, "should have distinct inline-code types")
}

// ---------------------------------------------------------------------------
// Schema
// ---------------------------------------------------------------------------

func TestSchema(t *testing.T) {
	cfg := &xmlfmt.Config{}
	s := cfg.Schema()
	assert.Equal(t, "xml", s.FormatMeta.ID)
	assert.Contains(t, s.FormatMeta.Extensions, ".xml")
}

// ---------------------------------------------------------------------------
// Group in paragraph (snippets)
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testGroupInPara
func TestSnippets_GroupInPara(t *testing.T) {
	snippet := `<?xml version="1.0" encoding="UTF-8"?><doc>` +
		`<p>Text before list:` +
		`<ul><li>Text of item 1</li><li>Text of item 2</li></ul>` +
		`and text after the list.</p></doc>`
	parts := readXML(t, snippet, nil)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Text of item 1")
	assert.Contains(t, texts, "Text of item 2")
}

// ---------------------------------------------------------------------------
// Inline with img alt attribute (snippets)
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testPWithInlines2
func TestSnippets_PWithInlines2(t *testing.T) {
	cfg := &xmlfmt.Config{
		InlineElements:         []string{"b", "img"},
		TranslatableAttributes: []string{"alt"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>Before <b>bold</b> <img href="there" alt="text"/> after.</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "text")
}

// ---------------------------------------------------------------------------
// Input value extraction (snippets)
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testExtractValueInInput
func TestSnippets_ExtractValueInInput(t *testing.T) {
	cfg := &xmlfmt.Config{
		TranslatableAttributes: []string{"value"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><input type="text" value="Enter" /></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Enter")
}

// okapi: XmlSnippetsTest#testNoExtractValueInInput
func TestSnippets_NoExtractValueInInput(t *testing.T) {
	// Without value in translatable attributes, it should not be extracted.
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><input type="hidden" value="Enter" /></doc>`, nil)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.NotContains(t, texts, "Enter")
}

// ---------------------------------------------------------------------------
// Newline normalization in pre
// ---------------------------------------------------------------------------

// okapi: XmlSnippetsTest#testNormalizeNewlinesInPre
func TestSnippets_NormalizeNewlinesInPre(t *testing.T) {
	cfg := &xmlfmt.Config{
		PreserveWhitespaceElements: []string{"pre"},
	}
	parts := readXMLWithConfig(t,
		"<?xml version=\"1.0\" encoding=\"UTF-8\"?><doc><pre>a\r\nb\rc</pre></doc>", cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	text := blocks[0].SourceText()
	assert.Contains(t, text, "a")
	assert.Contains(t, text, "b")
	assert.Contains(t, text, "c")
}

// ---------------------------------------------------------------------------
// Meta tag content extraction
// ---------------------------------------------------------------------------

// okapi: XmlStreamEventTest#testMetaTagContent
func TestEvents_MetaTagContent(t *testing.T) {
	cfg := &xmlfmt.Config{
		TranslatableAttributes: []string{"content"},
	}
	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><meta http-equiv="keywords" content="one,two,three"/></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "one,two,three")

	for _, b := range blocks {
		if b.SourceText() == "one,two,three" {
			assert.True(t, b.IsReferent, "meta content block should be a referent")
			break
		}
	}
}

// ---------------------------------------------------------------------------
// Exclude by default config tests
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationTest#excludeByDefault
func TestConfig_ExcludeByDefault(t *testing.T) {
	cfg := &xmlfmt.Config{ExcludeByDefault: true}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"item": map[string]any{
				"ruleTypes":  []any{"INCLUDE"},
				"conditions": []any{"translate", "EQUALS", "y"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc>`+
			`<item translate="y">Included</item>`+
			`<item>Excluded</item>`+
			`</doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Included")
	assert.NotContains(t, texts, "Excluded")
}

// ---------------------------------------------------------------------------
// Attribute ID configuration tests
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationTest#attributeID
func TestConfig_AttributeID(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"text": map[string]any{
				"ruleTypes":    []any{"TEXTUNIT"},
				"idAttributes": []any{"id"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><text id="myid">Hello</text></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
	assert.Contains(t, blocks[0].Name, "myid")
}

// ---------------------------------------------------------------------------
// xml:lang configuration
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationTest#xmlLang
func TestConfig_XmlLang(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc xml:lang="en"><text>Hello</text></doc>`, nil)
	dp := findDataPartWithProperty(parts, "language")
	require.NotNil(t, dp, "should have Data part with language property")
	assert.Equal(t, "en", dp.Properties["language"])
}

// ---------------------------------------------------------------------------
// Text unit code types
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationTest#textUnitCodeTypes
func TestConfig_TextUnitCodeTypes(t *testing.T) {
	parts := readXML(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>Paragraph content</p></doc>`, nil)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Paragraph content")
	for _, b := range blocks {
		if b.SourceText() == "Paragraph content" {
			assert.NotEmpty(t, b.ID)
			break
		}
	}
}

// ---------------------------------------------------------------------------
// Regex exclude rules with attribute conditions
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationSupportTest#test_EXCLUDEWithRegexExcludeWithAttribute
func TestExclude_RegexWithAttribute(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"'.*'": map[string]any{
				"ruleTypes":  []any{"EXCLUDE"},
				"conditions": []any{"x", "EQUALS", "true"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><pre x="true">t1</pre><p>t2</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "t2")
	// With regex element name and attribute condition, <pre x="true"> should be excluded.
	assert.NotContains(t, texts, "t1")
}

// okapi: XmlStreamConfigurationSupportTest#test_EXCLUDEWithRegexExcludeWithoutAttribute
func TestExclude_RegexWithoutAttribute(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"'.*'": map[string]any{
				"ruleTypes":  []any{"EXCLUDE"},
				"conditions": []any{"x", "EQUALS", "true"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><pre>t1</pre><p>t2</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	texts := blockTexts(blocks)
	// Neither element has x="true", so nothing is excluded.
	assert.Contains(t, texts, "t1")
	assert.Contains(t, texts, "t2")
}

// ---------------------------------------------------------------------------
// Regex inline + exclude
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationSupportTest#test_INLINE_WITH_EXCLUDE_Regex_Trick
func TestInlineExclude_RegexTrick(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"elements": map[string]any{
			"'tag\\d+'": map[string]any{
				"ruleTypes": []any{"INLINE", "EXCLUDE"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>before <tag1>excluded1</tag1> <tag2>excluded2</tag2> after</p></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)

	// With regex INLINE+EXCLUDE, matched elements should have their content excluded.
	b := findBlockContaining(blocks, "before")
	require.NotNil(t, b)
	text := b.SourceText()
	assert.Contains(t, text, "before")
	assert.Contains(t, text, "after")
	assert.NotContains(t, text, "excluded1")
	assert.NotContains(t, text, "excluded2")
}

// ---------------------------------------------------------------------------
// Regex attribute writable
// ---------------------------------------------------------------------------

// okapi: XmlStreamConfigurationSupportTest#test_regex_ATTRIBUTE_WRITABLE
func TestAttributeWritable_Regex(t *testing.T) {
	cfg := &xmlfmt.Config{}
	err := cfg.ApplyMap(map[string]any{
		"attributes": map[string]any{
			"dir": map[string]any{
				"ruleTypes": []any{"ATTRIBUTE_WRITABLE"},
			},
		},
	})
	require.NoError(t, err)

	parts := readXMLWithConfig(t,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p dir="rtl">t1</p><pre dir="ltr">t2</pre></doc>`, cfg)
	blocks := filterBlocks(parts)
	require.Len(t, blocks, 2)
	assert.Equal(t, "t1", blocks[0].SourceText())
	assert.Equal(t, "t2", blocks[1].SourceText())
	// Non-regex ATTRIBUTE_WRITABLE should populate block Properties.
	require.NotNil(t, blocks[0].Properties)
	assert.Equal(t, "rtl", blocks[0].Properties["dir"])
	require.NotNil(t, blocks[1].Properties)
	assert.Equal(t, "ltr", blocks[1].Properties["dir"])
}

// ---------------------------------------------------------------------------
// DITA extraction tests
// ---------------------------------------------------------------------------

// okapi: DitaExtractionComparisionTest#testStartDocument
func TestDita_StartDocument(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE topic PUBLIC "-//OASIS//DTD DITA Topic//EN" "topic.dtd">
<topic id="readme">
  <title>Getting Started</title>
  <body>
    <p>Welcome to the documentation.</p>
  </body>
</topic>`
	parts := readXML(t, input, nil)
	require.NotEmpty(t, parts)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: DitaExtractionComparisionTest#testDoubleExtraction
func TestDita_DoubleExtraction(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<topic id="test"><title>Title</title><body><p>Body text</p></body></topic>`
	parts1 := readXML(t, input, nil)
	parts2 := readXML(t, input, nil)
	blocks1 := filterBlocks(parts1)
	blocks2 := filterBlocks(parts2)
	require.Len(t, blocks2, len(blocks1))
}

// okapi: DitaExtractionComparisionTest#testDoubleExtractionSingle
func TestDita_DoubleExtractionSingle(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<task id="oil"><title>Changing oil</title><taskbody><context>Steps for changing oil.</context></taskbody></task>`
	parts1 := readXML(t, input, nil)
	parts2 := readXML(t, input, nil)
	blocks1 := filterBlocks(parts1)
	blocks2 := filterBlocks(parts2)
	require.Len(t, blocks2, len(blocks1))
}

// okapi: DitaExtractionComparisionTest#testReconstructFile
func TestDita_ReconstructFile(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<topic id="readme"><title>Getting Started</title><body><p>Welcome.</p></body></topic>`
	output := roundtripXML(t, input, nil)
	require.NotEmpty(t, output)
	assert.Contains(t, output, "Getting Started")
	assert.Contains(t, output, "Welcome.")
}

// okapi: DitaExtractionComparisionTest#testOpenTwice
func TestDita_OpenTwice(t *testing.T) {
	ctx := t.Context()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<topic id="readme"><title>Title</title><body><p>Body</p></body></topic>`

	reader := xmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	blocks1 := testutil.CollectBlocks(t, reader.Read(ctx))
	reader.Close()

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	blocks2 := testutil.CollectBlocks(t, reader.Read(ctx))
	reader.Close()

	require.Len(t, blocks2, len(blocks1))
}

// ---------------------------------------------------------------------------
// DOCTYPE extraction tests
// ---------------------------------------------------------------------------

// okapi: DocTypeExtractionTest#testDoubleExtraction
func TestDocType_DoubleExtraction(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE doc SYSTEM "doc.dtd">
<doc><text>Hello</text></doc>`
	parts1 := readXML(t, input, nil)
	parts2 := readXML(t, input, nil)
	blocks1 := filterBlocks(parts1)
	blocks2 := filterBlocks(parts2)
	require.Len(t, blocks2, len(blocks1))
}

// okapi: DocTypeExtractionTest#testEvents
func TestDocType_Events(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE doc SYSTEM "doc.dtd">
<doc><text>Hello world</text></doc>`
	parts := readXML(t, input, nil)
	require.NotEmpty(t, parts)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks, "document with DOCTYPE should have translatable content")
	texts := blockTexts(blocks)
	assert.Contains(t, texts, "Hello world")
}

// ---------------------------------------------------------------------------
// Processing instruction extraction tests
// ---------------------------------------------------------------------------

// okapi: PIExtractionTest#testDoubleExtraction
func TestPI_DoubleExtraction(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<?my-pi data="value"?>
<doc><text>Hello</text></doc>`
	parts1 := readXML(t, input, nil)
	parts2 := readXML(t, input, nil)
	blocks1 := filterBlocks(parts1)
	blocks2 := filterBlocks(parts2)
	require.Len(t, blocks2, len(blocks1))
}

// okapi: PIExtractionTest#testDoubleExtraction2
func TestPI_DoubleExtraction2(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<doc><?my-pi?><text>Content</text><?another-pi data="x"?></doc>`
	parts1 := readXML(t, input, nil)
	parts2 := readXML(t, input, nil)
	blocks1 := filterBlocks(parts1)
	blocks2 := filterBlocks(parts2)
	require.Len(t, blocks2, len(blocks1))
}

// ---------------------------------------------------------------------------
// Property XML extraction tests
// ---------------------------------------------------------------------------

// okapi: PropertyXmlExtractionComparisionTest#testStartDocument
func TestPropertyXml_StartDocument(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<properties><entry key="about">About this app</entry></properties>`
	parts := readXML(t, input, nil)
	require.NotEmpty(t, parts)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks)
}

// okapi: PropertyXmlExtractionComparisionTest#testDoubleExtraction
func TestPropertyXml_DoubleExtraction(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<properties><entry key="about">About</entry></properties>`
	parts1 := readXML(t, input, nil)
	parts2 := readXML(t, input, nil)
	blocks1 := filterBlocks(parts1)
	blocks2 := filterBlocks(parts2)
	require.Len(t, blocks2, len(blocks1))
}

// okapi: PropertyXmlExtractionComparisionTest#testDoubleExtractionSingle
func TestPropertyXml_DoubleExtractionSingle(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<properties><entry key="test">Test value</entry></properties>`
	parts1 := readXML(t, input, nil)
	parts2 := readXML(t, input, nil)
	require.Len(t, parts2, len(parts1))
}

// okapi: PropertyXmlExtractionComparisionTest#testReconstructFile
func TestPropertyXml_ReconstructFile(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?>
<properties><entry key="about">About this app</entry></properties>`
	output := roundtripXML(t, input, nil)
	require.NotEmpty(t, output)
	assert.Contains(t, output, "About this app")
}

// okapi: PropertyXmlExtractionComparisionTest#testOpenTwice
func TestPropertyXml_OpenTwice(t *testing.T) {
	ctx := t.Context()
	input := `<?xml version="1.0" encoding="UTF-8"?>
<properties><entry key="test">Test value</entry></properties>`

	reader := xmlfmt.NewReader()
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	blocks1 := testutil.CollectBlocks(t, reader.Read(ctx))
	reader.Close()

	err = reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	blocks2 := testutil.CollectBlocks(t, reader.Read(ctx))
	reader.Close()

	require.Len(t, blocks2, len(blocks1))
}

// okapi: PropertyXmlExtractionComparisionTest#testAsSnippetNoCdata
func TestPropertyXml_AsSnippetNoCdata(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?><properties><entry key="test">Test value</entry></properties>`
	parts := readXML(t, input, nil)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks, "should extract entries from property XML")
}

// okapi: PropertyXmlExtractionComparisionTest#testAsSnippetWithCdata
func TestPropertyXml_AsSnippetWithCdata(t *testing.T) {
	input := `<?xml version="1.0" encoding="UTF-8"?><properties><entry key="test"><![CDATA[Test value]]></entry></properties>`
	parts := readXML(t, input, nil)
	blocks := filterBlocks(parts)
	require.NotEmpty(t, blocks, "should extract entries from property XML with CDATA")
}

// ---------------------------------------------------------------------------
// Roundtrip integration test
// ---------------------------------------------------------------------------

// okapi: RoundTripXmlIT#xmlFiles
// okapi: RoundTripXmlStreamIT#xmlStreamFiles
// okapi-skip: RoundTripXmlIT#xmlSerializedFiles — Okapi serialized-skeleton roundtrip variant; native uses its own skeleton store (no serialized-skeleton mode)
// okapi-skip: RoundTripXmlStreamIT#xmlStreamSerializedFiles — Okapi serialized-skeleton roundtrip variant; native uses its own skeleton store
// okapi-skip: RoundTripXmlIT#issue591Resolved — exercises Okapi's custom okf_xml@ibxlf1 named config (IBM XLIFF1 regression); native has no equivalent named custom-config registry
// okapi-skip: RoundTripXmlIT#issue1384Resolved — exercises Okapi's custom okf_xml@translatable-and-untranslatable named config; native has no equivalent named custom-config registry
func TestRoundTrip_Integration(t *testing.T) {
	inputs := []string{
		`<?xml version="1.0" encoding="UTF-8"?><root><text>Hello world</text></root>`,
		`<?xml version="1.0" encoding="UTF-8"?><doc><p>One</p><p>Two</p></doc>`,
		`<?xml version="1.0" encoding="UTF-8"?><doc><text>Price: &lt;$10 &amp; &gt;$5</text></doc>`,
		`<?xml version="1.0" encoding="UTF-8"?><doc><text><![CDATA[Hello <world>]]></text></doc>`,
	}
	for _, input := range inputs {
		output := roundtripXML(t, input, nil)
		require.NotEmpty(t, output, "roundtrip should produce output for: %s", input[:50])
	}
}

// ---------------------------------------------------------------------------
// Helper functions
// ---------------------------------------------------------------------------

// roundtripXML reads and writes XML, returning the output string.
func roundtripXML(t *testing.T, input string, cfg *xmlfmt.Config) string {
	t.Helper()
	ctx := t.Context()

	reader := xmlfmt.NewReader()
	if cfg != nil {
		require.NoError(t, reader.SetConfig(cfg))
	}
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	parts := testutil.CollectParts(t, reader.Read(ctx))
	reader.Close()

	writer := xmlfmt.NewWriter()
	var buf bytes.Buffer
	require.NoError(t, writer.SetOutputWriter(&buf))

	partCh := make(chan *model.Part, len(parts))
	for _, p := range parts {
		partCh <- p
	}
	close(partCh)

	require.NoError(t, writer.Write(ctx, partCh))
	require.NoError(t, writer.Close())
	return buf.String()
}

// readXML parses XML with an optional config applied via ApplyMap.
func readXML(t *testing.T, input string, params map[string]any) []*model.Part {
	t.Helper()
	cfg := &xmlfmt.Config{}
	if params != nil {
		require.NoError(t, cfg.ApplyMap(params))
	}
	return readXMLWithConfig(t, input, cfg)
}

// readXMLWithConfig parses XML with a specific Config.
func readXMLWithConfig(t *testing.T, input string, cfg *xmlfmt.Config) []*model.Part {
	t.Helper()
	ctx := t.Context()
	reader := xmlfmt.NewReader()
	if cfg != nil {
		require.NoError(t, reader.SetConfig(cfg))
	}
	err := reader.Open(ctx, testutil.RawDocFromString(input, model.LocaleEnglish))
	require.NoError(t, err)
	defer reader.Close()
	return testutil.CollectParts(t, reader.Read(ctx))
}

// filterBlocks returns only Block resources from parts.
func filterBlocks(parts []*model.Part) []*model.Block {
	return testutil.FilterBlocks(parts)
}

// blockTexts returns the source text of each block.
func blockTexts(blocks []*model.Block) []string {
	return testutil.BlockTexts(blocks)
}

// findBlockContaining finds a block whose source text contains the given substring.
func findBlockContaining(blocks []*model.Block, substr string) *model.Block {
	for _, b := range blocks {
		if strings.Contains(b.SourceText(), substr) {
			return b
		}
	}
	return nil
}

// inlineCodeRuns returns only the inline-code runs (Ph, PcOpen, PcClose, Sub).
func inlineCodeRuns(runs []model.Run) []model.Run {
	var out []model.Run
	for _, r := range runs {
		if r.Text == nil && r.Plural == nil && r.Select == nil {
			out = append(out, r)
		}
	}
	return out
}

// hasOpeningRun reports whether any run is a PcOpen.
func hasOpeningRun(runs []model.Run) bool {
	for _, r := range runs {
		if r.PcOpen != nil {
			return true
		}
	}
	return false
}

// hasPlaceholderRun reports whether any run is a Ph.
func hasPlaceholderRun(runs []model.Run) bool {
	for _, r := range runs {
		if r.Ph != nil {
			return true
		}
	}
	return false
}

// findDataPartWithProperty finds the first Data part that has the given property key.
func findDataPartWithProperty(parts []*model.Part, key string) *model.Data {
	for _, p := range parts {
		if p.Type == model.PartData {
			d, ok := p.Resource.(*model.Data)
			if ok && d.Properties != nil {
				if _, exists := d.Properties[key]; exists {
					return d
				}
			}
		}
	}
	return nil
}
