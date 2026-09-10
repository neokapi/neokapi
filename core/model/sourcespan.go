package model

import "strconv"

// SourceSpan locates a block's original source bytes, including its enclosing
// syntax where a reader can provide it. Part is a recognized package part
// (OpenXML's partPath), or empty for a flat document. [Start, End) uses bytes,
// not run positions or character offsets. The enclosing document snapshot
// scopes the span; these locators never contribute to content identity.
type SourceSpan struct {
	Part  string `json:"part,omitempty"`
	Start int    `json:"start"`
	End   int    `json:"end"`
}

const (
	sourceSpanPart  = AdvisoryPropertyPrefix + "source-part"
	sourceSpanStart = AdvisoryPropertyPrefix + "source-start"
	sourceSpanEnd   = AdvisoryPropertyPrefix + "source-end"
)

// SetSourceSpan records reader-derived source location as advisory properties.
func (b *Block) SetSourceSpan(span SourceSpan) {
	b.SetProperty(sourceSpanPart, span.Part)
	b.SetProperty(sourceSpanStart, strconv.Itoa(span.Start))
	b.SetProperty(sourceSpanEnd, strconv.Itoa(span.End))
}

// SourceSpan returns a reader-provided source location, if present and valid.
func (b *Block) SourceSpan() (SourceSpan, bool) {
	start, startErr := strconv.Atoi(b.Properties[sourceSpanStart])
	end, endErr := strconv.Atoi(b.Properties[sourceSpanEnd])
	if startErr != nil || endErr != nil || start < 0 || end < start {
		return SourceSpan{}, false
	}
	return SourceSpan{Part: b.Properties[sourceSpanPart], Start: start, End: end}, true
}
