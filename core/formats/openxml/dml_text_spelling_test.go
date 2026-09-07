package openxml

// okapi-filter: openxml

import (
	"bytes"
	"encoding/xml"
	"strings"
	"testing"

	"github.com/neokapi/neokapi/core/format"
	"github.com/neokapi/neokapi/core/internal/testutil"
	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Two spellings a DrawingML shape carries that the writer has to answer for.
//
// An <a:t> may declare xml:space="preserve". PowerPoint writes it on four of
// the 548 runs in the upstream corpus whose text has whitespace at an edge, so
// the attribute is a producer's habit rather than a rule the format states:
// ECMA-376 Part 1 §21.1.2.3.11 types <a:t> as xsd:string, and nothing trims it.
// The writer therefore replays the source's spelling and synthesises no
// attribute. Adding one on every edge-whitespace run rewrites 544 runs the
// corpus authored without it.
//
// A character reference inside an attribute value is the opposite case: the
// bytes carry meaning the value alone does not. A parser rewrites a tab, a line
// feed and a carriage return in an attribute value as a space (XML 1.0 §3.3.3),
// so a shape's multi-line alt text written back with literal newlines is a
// single line on the next read.

// TestDMLTextSpelling_XMLSpaceIsReplayedNotSynthesised is #2534's first half.
func TestDMLTextSpelling_XMLSpaceIsReplayedNotSynthesised(t *testing.T) {
	para := `<a:p>` +
		`<a:r><a:rPr lang="en-US"/><a:t xml:space="preserve">Red </a:t></a:r>` +
		`<a:r><a:rPr lang="en-US" b="1"/><a:t>bold</a:t></a:r>` +
		`</a:p>`
	slide := dmlSlide(para)
	out := skeletonRoundtripBytes(t, dmlDeck(t, slide), "deck.pptx")
	assert.Equal(t, slide, dmlSlideXML(t, out),
		"the run that declared xml:space keeps the attribute")
}

// TestDMLTextSpelling_EdgeWhitespaceWithoutXMLSpaceStaysThatWay is the other
// side of the same rule, and the one the corpus is made of.
func TestDMLTextSpelling_EdgeWhitespaceWithoutXMLSpaceStaysThatWay(t *testing.T) {
	para := `<a:p>` +
		`<a:r><a:rPr lang="en-US"/><a:t>This </a:t></a:r>` +
		`<a:r><a:rPr lang="en-US" b="1"/><a:t>works</a:t></a:r>` +
		`</a:p>`
	slide := dmlSlide(para)
	out := skeletonRoundtripBytes(t, dmlDeck(t, slide), "deck.pptx")
	got := dmlSlideXML(t, out)
	assert.Equal(t, slide, got, "a run that declared no xml:space gets none back")
	assert.NotContains(t, got, `xml:space`,
		"the writer synthesises no attribute the source did not carry")
}

// TestDMLTextSpelling_TranslatedRunKeepsItsEdgeWhitespace states what makes the
// replay safe: the whitespace survives a rebuilt run too, because <a:t> content
// is significant without the attribute.
func TestDMLTextSpelling_TranslatedRunKeepsItsEdgeWhitespace(t *testing.T) {
	para := `<a:p>` +
		`<a:r><a:rPr lang="en-US"/><a:t>This </a:t></a:r>` +
		`<a:r><a:rPr lang="en-US" b="1"/><a:t>works</a:t></a:r>` +
		`</a:p>`
	out := dmlWriteBack(t, dmlDeck(t, dmlSlide(para)))
	got := dmlSlideXML(t, out)
	assert.Contains(t, got, `<a:t>THIS </a:t>`, "the translated run keeps the trailing space")

	// And a decoder reads it back as authored.
	var texts []string
	d := xml.NewDecoder(strings.NewReader(got))
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "t" {
			var body string
			require.NoError(t, d.DecodeElement(&body, &se))
			texts = append(texts, body)
		}
	}
	assert.Contains(t, texts, "THIS ", "and a reader sees the space")
}

// dmlAltTextSlide puts a picture carrying multi-line alt text on a slide, the
// shape 1431-graphic-21.pptx carries on every layout.
func dmlAltTextSlide(descr string) string {
	return `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` +
		`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
		`xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` +
		`<p:cSld><p:spTree>` +
		`<p:pic><p:nvPicPr><p:cNvPr id="8" name="Picture 7" descr="` + descr + `"/>` +
		`<p:cNvPicPr/><p:nvPr/></p:nvPicPr><p:spPr/></p:pic>` +
		`<p:sp><p:nvSpPr><p:cNvPr id="2" name="Title 1"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` +
		`<p:spPr/><p:txBody><a:bodyPr/><a:lstStyle/>` +
		`<a:p><a:r><a:rPr lang="en-US"/><a:t>Title</a:t></a:r></a:p>` +
		`</p:txBody></p:sp></p:spTree></p:cSld></p:sld>`
}

// TestDMLTextSpelling_AltTextKeepsItsCharacterReferences is #2534's second
// half: the alt text goes back with the spelling that keeps its line breaks.
func TestDMLTextSpelling_AltTextKeepsItsCharacterReferences(t *testing.T) {
	slide := dmlAltTextSlide("Diagram&#xA;&#xA;Generated content")
	out := skeletonRoundtripBytes(t, dmlDeck(t, slide), "deck.pptx")
	assert.Equal(t, slide, dmlSlideXML(t, out),
		"the alt text goes back byte for byte")
}

// TestDMLTextSpelling_AltTextLineBreaksSurviveAReread is why the spelling
// matters: XML 1.0 §3.3.3 has a conforming parser turn a literal newline in an
// attribute value into a space, so the value has to leave as a character
// reference. The byte assertion is the one that discriminates, because
// encoding/xml's decoder skips that normalization.
func TestDMLTextSpelling_AltTextLineBreaksSurviveAReread(t *testing.T) {
	slide := dmlAltTextSlide("Diagram&#xA;&#xA;Generated content")
	out := skeletonRoundtripBytes(t, dmlDeck(t, slide), "deck.pptx")

	got := dmlSlideXML(t, out)
	_, after, found := strings.Cut(got, `<p:cNvPr id="8"`)
	require.True(t, found, "the picture's drawing properties are in the output")
	openTag, _, found := strings.Cut(after, ">")
	require.True(t, found, "and the tag is closed")
	assert.NotContains(t, openTag, "\n",
		"the alt text leaves as character references, not as literal newlines")

	d := xml.NewDecoder(strings.NewReader(got))
	var descr string
	for {
		tok, err := d.Token()
		if err != nil {
			break
		}
		if se, ok := tok.(xml.StartElement); ok && se.Name.Local == "cNvPr" {
			if v := attrVal(se, "descr"); v != "" {
				descr = v
			}
		}
	}
	assert.Equal(t, "Diagram\n\nGenerated content", descr,
		"a parser reads the alt text back with its line breaks")
}

// TestDMLTextSpelling_TranslatedAltTextIsEscapedForAnAttribute covers the
// translated path, where the writer has no source bytes to replay. Alt text is
// Translatable:false RoleCaption content, so the target is set on the block
// directly rather than through the ordinary translate helper.
func TestDMLTextSpelling_TranslatedAltTextIsEscapedForAnAttribute(t *testing.T) {
	slide := dmlAltTextSlide("Diagram&#xA;line two")
	locale := model.LocaleID("de")

	skelStore, err := format.NewSkeletonStore()
	require.NoError(t, err)
	defer skelStore.Close()

	pkg := dmlDeck(t, slide)
	reader := NewReader()
	reader.SetSkeletonStore(skelStore)
	require.NoError(t, reader.Open(t.Context(), &model.RawDocument{
		URI:          "deck.pptx",
		SourceLocale: model.LocaleEnglish,
		Encoding:     "UTF-8",
		Reader:       readCloserFromBytes(pkg),
	}))
	parts := testutil.CollectParts(t, reader.Read(t.Context()))
	require.NoError(t, reader.Close())

	found := false
	for _, b := range testutil.FilterBlocks(parts) {
		if b.Properties["element"] == propElementDrawingDescr {
			b.SetTargetRuns(locale, []model.Run{model.TextR("Diagramm\nZeile zwei")})
			found = true
		}
	}
	require.True(t, found, "the reader surfaced the alt text as its own block")

	var buf bytes.Buffer
	writer := NewWriter()
	writer.SetOriginalContent(pkg)
	writer.SetSkeletonStore(skelStore)
	writer.SetLocale(locale)
	require.NoError(t, writer.SetOutputWriter(&buf))
	require.NoError(t, writer.Write(t.Context(), testutil.PartsToChannel(parts)))
	require.NoError(t, writer.Close())

	assert.Contains(t, dmlSlideXML(t, buf.Bytes()), `descr="Diagramm&#xA;Zeile zwei"`,
		"a translated attribute value carries the line break as a character reference")
}

// TestPropertyGoesInAnAttribute names the two positions a property block's text
// lands in, so a core property keeps element-content escaping.
func TestPropertyGoesInAnAttribute(t *testing.T) {
	attribute := []*model.Block{
		{Type: "property", Properties: map[string]string{"element": propElementDrawingDescr}},
		{Type: "property", Properties: map[string]string{"element": propElementDrawingTitle}},
		{Type: "table-column", Properties: map[string]string{}},
	}
	for _, b := range attribute {
		assert.True(t, propertyGoesInAnAttribute(b), "%s/%s", b.Type, b.Properties["element"])
	}
	content := []*model.Block{
		{Type: "property", Properties: map[string]string{"element": "title"}},
		{Type: "property", Properties: map[string]string{"element": "keywords"}},
		{Type: "property", Properties: map[string]string{"element": "alt-content-text"}},
	}
	for _, b := range content {
		assert.False(t, propertyGoesInAnAttribute(b), "%s/%s", b.Type, b.Properties["element"])
	}
}
