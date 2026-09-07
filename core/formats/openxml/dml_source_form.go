// Source-form retention for DrawingML paragraphs: the bytes a shape's runs were
// written with, kept on the block when the writer would spell them differently.

package openxml

import "github.com/neokapi/neokapi/core/model"

// Property a DrawingML paragraph block carries when the writer would spell its
// run sequence differently from the source: the paragraph's run-bearing
// children as the source wrote them, from the first <a:r>, <a:br> or opaque
// child to the last.
//
// Three forms live in those bytes and in nothing else. Two adjacent runs whose
// <a:rPr> is identical are one run by the time the model sees them, because
// that is the rule upstream Okapi's RunMerger states and #2512 kept. A run
// holding an empty <a:t/> carries no text, so nothing opens a run for it and
// its envelope goes with it. The whitespace a producer indented an <a:r>, an
// <a:rPr> and an <a:t> with belongs to none of them.
//
// The reader stores this only where it differs from what the writer would
// produce, so a deck authored the way the writer writes carries nothing extra,
// and the writer replays it only for a paragraph whose content nothing has
// changed. See dmlSourceContent.
const dmlSourceProp = "openxml:dml-source"

// dmlSourceForm returns the paragraph content to keep on a block, or "" when
// the writer would rebuild it byte for byte from the runs alone.
func dmlSourceForm(content string, runs []model.Run) string {
	if content == renderDMLRuns(runs) {
		return ""
	}
	return content
}

// spanGrow widens a span to cover one more element. start is the span's own
// start, or -1 when nothing has been covered yet.
func spanGrow(start, elemStart, elemEnd int64) (int64, int64) {
	if start < 0 {
		start = elemStart
	}
	return start, elemEnd
}

// The element a property block's text came from, when the text goes back into
// an attribute rather than into element content: a shape's alt text and its
// object title, on <p:cNvPr> in a deck and <wp:docPr> in a document.
const (
	propElementDrawingDescr = "drawing-descr"
	propElementDrawingTitle = "drawing-title"
)

// propertyGoesInAnAttribute reports whether a property block's text is written
// back into an attribute value.
//
// The two positions escape differently. An XML parser rewrites a tab, a line
// feed and a carriage return in an attribute value as a space (XML 1.0 §3.3.3),
// so a shape's multi-line alt text has to travel as character references or it
// comes back a single line. In element content, where a core property sits,
// those three characters carry themselves and the references would be a
// spelling the source did not use.
func propertyGoesInAnAttribute(block *model.Block) bool {
	if block == nil {
		return false
	}
	if block.Type == "table-column" {
		return true
	}
	switch block.Properties["element"] {
	case propElementDrawingDescr, propElementDrawingTitle:
		return true
	}
	return false
}
