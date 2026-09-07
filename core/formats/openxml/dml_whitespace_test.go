package openxml

// okapi-filter: openxml

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

// A deck saved with indented XML carries whitespace between a text body's
// children and between a paragraph's own. Nothing in DrawingML reads it, so a
// parser sees the same document either way, and a reviewer opening a deck
// nobody translated sees every slide rewritten.
//
// The text body replays it as character data. The paragraph replays the bytes
// on either side of its runs, which carries the <a:pPr>, the <a:endParaRPr> and
// the whitespace around all three. This is #2519 for DrawingML.

// dmlIndentedSlide is a shape a producer pretty-printed, indentation and CRLF
// line endings included.
const dmlIndentedSlide = `<?xml version="1.0" encoding="UTF-8" standalone="yes"?>` + "\r\n" +
	`<p:sld xmlns:a="http://schemas.openxmlformats.org/drawingml/2006/main" ` +
	`xmlns:p="http://schemas.openxmlformats.org/presentationml/2006/main">` + "\r\n" +
	"  <p:cSld>\r\n" +
	"    <p:spTree>\r\n" +
	"      <p:sp>\r\n" +
	`        <p:nvSpPr><p:cNvPr id="2" name="Title 1"/><p:cNvSpPr/><p:nvPr/></p:nvSpPr>` + "\r\n" +
	"        <p:spPr/>\r\n" +
	"        <p:txBody>\r\n" +
	"          <a:bodyPr/>\r\n" +
	"          <a:lstStyle/>\r\n" +
	"          <a:p>\r\n" +
	`            <a:pPr algn="just"/>` + "\r\n" +
	"            <a:r>\r\n" +
	`              <a:rPr lang="en-US" dirty="0"/>` + "\r\n" +
	"              <a:t>Test</a:t>\r\n" +
	"            </a:r>\r\n" +
	`            <a:endParaRPr lang="ru-RU" dirty="0"/>` + "\r\n" +
	"          </a:p>\r\n" +
	"        </p:txBody>\r\n" +
	"      </p:sp>\r\n" +
	"    </p:spTree>\r\n" +
	"  </p:cSld>\r\n" +
	"</p:sld>"

// TestDMLWhitespace_IndentedSlideIsByteIdentical is #2533 end to end.
func TestDMLWhitespace_IndentedSlideIsByteIdentical(t *testing.T) {
	out := skeletonRoundtripBytes(t, dmlDeck(t, dmlIndentedSlide), "deck.pptx")
	assert.Equal(t, dmlIndentedSlide, dmlSlideXML(t, out),
		"an untranslated slide keeps the whitespace a producer wrote")
}

// TestDMLWhitespace_TextBodyChildrenKeepTheirSeparators isolates the text-body
// half, which the paragraph's own replay does not reach.
func TestDMLWhitespace_TextBodyChildrenKeepTheirSeparators(t *testing.T) {
	out := skeletonRoundtripBytes(t, dmlDeck(t, dmlIndentedSlide), "deck.pptx")
	got := dmlSlideXML(t, out)
	assert.Contains(t, got, "<p:txBody>\r\n          <a:bodyPr/>\r\n          <a:lstStyle/>\r\n          <a:p>",
		"the whitespace between a text body's children comes back")
}

// TestDMLWhitespace_ParagraphKeepsWhatSurroundsItsRuns isolates the paragraph
// half: the properties on either side and the whitespace around them.
func TestDMLWhitespace_ParagraphKeepsWhatSurroundsItsRuns(t *testing.T) {
	out := skeletonRoundtripBytes(t, dmlDeck(t, dmlIndentedSlide), "deck.pptx")
	got := dmlSlideXML(t, out)
	assert.Contains(t, got, "<a:p>\r\n            <a:pPr algn=\"just\"/>\r\n            <a:r>",
		"the whitespace before the runs comes back")
	assert.Contains(t, got, "</a:r>\r\n            <a:endParaRPr lang=\"ru-RU\" dirty=\"0\"/>\r\n          </a:p>",
		"and the whitespace after them")
}

// TestDMLWhitespace_TranslatedParagraphKeepsItsSurroundings states what the
// skeleton carries rather than the block: a rebuilt paragraph still sits inside
// the layout the source wrote, even though its runs are the writer's.
func TestDMLWhitespace_TranslatedParagraphKeepsItsSurroundings(t *testing.T) {
	out := dmlWriteBack(t, dmlDeck(t, dmlIndentedSlide))
	got := dmlSlideXML(t, out)
	assert.Contains(t, got, "TEST", "the translation is written back")
	assert.Contains(t, got, "<a:p>\r\n            <a:pPr algn=\"just\"/>\r\n            <a:r>",
		"the paragraph properties and the whitespace before the runs stay")
	assert.Contains(t, got, "\r\n            <a:endParaRPr lang=\"ru-RU\" dirty=\"0\"/>\r\n          </a:p>",
		"and so does everything after them")
	assert.Equal(t, 1, strings.Count(got, "<a:endParaRPr"),
		"the paragraph mark is written once")
}
