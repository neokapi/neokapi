package model

// SpanType classifies inline markup elements.
type SpanType int

const (
	SpanOpening     SpanType = iota // Opening tag (e.g., <b>)
	SpanClosing                     // Closing tag (e.g., </b>)
	SpanPlaceholder                 // Self-closing/standalone (e.g., <br/>)
)

// String returns the name of the SpanType.
func (st SpanType) String() string {
	switch st {
	case SpanOpening:
		return "Opening"
	case SpanClosing:
		return "Closing"
	case SpanPlaceholder:
		return "Placeholder"
	default:
		return "Unknown"
	}
}

// Span represents an inline markup element within a Fragment.
type Span struct {
	SpanType  SpanType
	Type      string // Semantic type (e.g., "bold", "link", "image")
	ID        string
	Data      string // Original markup data (e.g., "<b>")
	OuterData string
	Deletable bool
	Cloneable bool
}
