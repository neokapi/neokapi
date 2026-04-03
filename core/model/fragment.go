package model

import "strings"

// Marker characters in the Unicode private use area (U+E000-U+E0FF).
// These mark positions of inline spans in the coded text.
const (
	// MarkerOpening marks an opening span position.
	MarkerOpening rune = '\uE001'
	// MarkerClosing marks a closing span position.
	MarkerClosing rune = '\uE002'
	// MarkerPlaceholder marks a placeholder span position.
	MarkerPlaceholder rune = '\uE003'
)

// isMarker returns true if the rune is a span marker character.
func isMarker(r rune) bool {
	return r >= '\uE001' && r <= '\uE003'
}

// Fragment holds text content with inline Spans.
// Spans are represented as markers in the CodedText with metadata in the Spans slice.
type Fragment struct {
	CodedText string  // Text with span markers (special Unicode chars)
	Spans     []*Span // Inline markup elements
}

// NewFragment creates a Fragment from plain text (no spans).
func NewFragment(text string) *Fragment {
	return &Fragment{CodedText: text}
}

// Text returns the plain text with all span markers stripped.
func (f *Fragment) Text() string {
	var buf strings.Builder
	for _, r := range f.CodedText {
		if !isMarker(r) {
			buf.WriteRune(r)
		}
	}
	return buf.String()
}

// HasSpans returns true if this Fragment contains any inline spans.
func (f *Fragment) HasSpans() bool {
	return len(f.Spans) > 0
}

// AppendText appends plain text to the Fragment.
func (f *Fragment) AppendText(text string) {
	f.CodedText += text
}

// AppendSpan appends an inline span and its marker to the Fragment.
func (f *Fragment) AppendSpan(span *Span) {
	var marker rune
	switch span.SpanType {
	case SpanOpening:
		marker = MarkerOpening
	case SpanClosing:
		marker = MarkerClosing
	case SpanPlaceholder:
		marker = MarkerPlaceholder
	}
	f.CodedText += string(marker)
	f.Spans = append(f.Spans, span)
}

// IsEmpty returns true if the Fragment has no content.
func (f *Fragment) IsEmpty() bool {
	return len(f.CodedText) == 0
}

// Length returns the length of the coded text in runes (including markers).
func (f *Fragment) Length() int {
	return len([]rune(f.CodedText))
}

// Clone creates a deep copy of the Fragment.
func (f *Fragment) Clone() *Fragment {
	clone := &Fragment{
		CodedText: f.CodedText,
	}
	if f.Spans != nil {
		clone.Spans = make([]*Span, len(f.Spans))
		for i, s := range f.Spans {
			sc := *s
			clone.Spans[i] = &sc
		}
	}
	return clone
}
