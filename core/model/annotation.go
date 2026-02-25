package model

// Annotation is an extensible metadata attachment on Blocks and Spans.
type Annotation interface {
	AnnotationType() string
}

// AltTranslation holds an alternative translation with metadata.
type AltTranslation struct {
	Source       *Fragment
	Target       *Fragment
	Locale       LocaleID
	Origin       string  // Where this translation came from (TM, MT, etc.)
	Score        float64 // Match quality (0.0 - 1.0)
	MatchType    string  // "exact", "fuzzy", "mt", "ai"
	CombinedScore float64 // Combined match score (Okapi)
	FuzzyScore   float64 // Fuzzy match score (Okapi)
	QualityScore float64 // Quality score (Okapi)
	Engine       string  // MT/AI engine name
	ToolID       string  // Tool that produced this translation
	AltTransType string  // Okapi alt-trans type (e.g., "proposal", "previous-version")
	FromOriginal bool    // Whether this came from the original document
}

// AnnotationType returns the type identifier for this annotation.
func (at *AltTranslation) AnnotationType() string { return "alt-translation" }

// NoteAnnotation holds a note/comment attached to a block or span.
type NoteAnnotation struct {
	Text      string // Note text content
	From      string // Who wrote the note (e.g., "developer", "translator")
	Priority  int    // Priority level (1=highest)
	Annotates string // What this note annotates ("source", "target", "general")
}

// AnnotationType returns the type identifier for this annotation.
func (n *NoteAnnotation) AnnotationType() string { return "note" }

// GenericAnnotation holds arbitrary metadata as key-value pairs.
// Used for ITS metadata, custom annotations, and any annotation type
// that doesn't have a dedicated struct.
type GenericAnnotation struct {
	Type_  string         // The annotation type name
	Fields map[string]any // Arbitrary key-value payload
}

// AnnotationType returns the type identifier for this annotation.
func (g *GenericAnnotation) AnnotationType() string { return g.Type_ }
