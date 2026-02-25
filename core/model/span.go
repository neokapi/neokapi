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

// Span flag constants for metadata about inline codes.
const (
	SpanFlagHasRef        = 1 << iota // Code references another resource
	SpanFlagAdded                     // Code was added by a tool (not in original)
	SpanFlagMerged                    // Code was merged from multiple sources
	SpanFlagMarkerMasking             // Code masks a marker character
)

// Span represents an inline markup element within a Fragment.
type Span struct {
	SpanType    SpanType
	Type        string // Semantic type (e.g., "bold", "link", "image")
	ID          string
	Data        string // Original markup data (e.g., "<b>")
	OuterData   string
	Deletable   bool
	Cloneable   bool
	OriginalID  string              // Original ID before merging/splitting
	DisplayText string              // Human-readable display text for the code
	Flags       int                 // Bitfield of SpanFlag* constants
	EquivText   string              // Equivalent text representation
	CanReorder  bool                // Whether this code can be reordered in translation
	Annotations map[string]Annotation // Annotations attached to this span
}
