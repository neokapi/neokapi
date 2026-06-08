package model

// MatchType classifies how an alternative translation was produced.
type MatchType string

const (
	MatchExact MatchType = "exact"
	MatchFuzzy MatchType = "fuzzy"
	MatchMT    MatchType = "mt"
	MatchAI    MatchType = "ai"
)

// AltTranslation holds an alternative translation with metadata.
// Source and Target are Run sequences — use FlattenRuns to materialise a
// string representation when needed.
type AltTranslation struct {
	Source        []Run     `json:"source,omitempty"`
	Target        []Run     `json:"target,omitempty"`
	Locale        LocaleID  `json:"locale,omitempty"`
	Origin        string    `json:"origin,omitempty"`         // Where this translation came from (TM, MT, etc.)
	Score         float64   `json:"score,omitempty"`          // Match quality (0.0 - 1.0)
	MatchType     MatchType `json:"match_type,omitempty"`     // MatchExact, MatchFuzzy, MatchMT, or MatchAI
	CombinedScore float64   `json:"combined_score,omitempty"` // Combined match score (Okapi)
	FuzzyScore    float64   `json:"fuzzy_score,omitempty"`    // Fuzzy match score (Okapi)
	QualityScore  float64   `json:"quality_score,omitempty"`  // Quality score (Okapi)
	Engine        string    `json:"engine,omitempty"`         // MT/AI engine name
	ToolID        string    `json:"tool_id,omitempty"`        // Tool that produced this translation
	AltTransType  string    `json:"alt_trans_type,omitempty"` // Okapi alt-trans type (e.g., "proposal", "previous-version")
	FromOriginal  bool      `json:"from_original,omitempty"`  // Whether this came from the original document
	SegmentIndex  int       `json:"segment_index,omitempty"`  // -1/0 = whole block; >0 = per-segment match at this segment index
}

// AltTranslations is the block annotation holding all alternative-translation
// candidates for a block — TM/MT/AI proposals and xliff <alt-trans> entries.
// It is one typed payload under the AnnoAltTranslation key: multiplicity lives
// in the slice, not in numbered keys ("alt-translation-1", …). It implements
// AnnotationType so the payload registry rehydrates it like any other.
type AltTranslations struct {
	Items []*AltTranslation `json:"items,omitempty"`
}

// AnnotationType returns the type identifier for a single alt-translation.
func (*AltTranslation) TypeName() string { return "alt-translation" }

// AnnotationType returns the type identifier for the alt-translation collection.
func (*AltTranslations) TypeName() string { return "alt-translation" }

// NoteAnnotation holds a note/comment attached to a block or span.
type NoteAnnotation struct {
	Text      string `json:"text"`                // Note text content
	From      string `json:"from,omitempty"`      // Who wrote the note (e.g., "developer", "translator")
	Priority  int    `json:"priority,omitempty"`  // Priority level (1=highest)
	Annotates string `json:"annotates,omitempty"` // What this note annotates ("source", "target", "general")
}

// AnnotationType returns the type identifier for a single note.
func (n *NoteAnnotation) TypeName() string { return "note" }

// Notes is the block annotation holding all notes/comments on a block. It is
// one typed payload under the AnnoNote key: multiplicity lives in the slice,
// not in numbered keys ("note-1", …).
type Notes struct {
	Items []*NoteAnnotation `json:"items,omitempty"`
}

// AnnotationType returns the type identifier for the note collection.
func (*Notes) TypeName() string { return "note" }

// GenericAnnotation holds arbitrary metadata as key-value pairs.
// Used for ITS metadata, custom annotations, and any annotation type
// that doesn't have a dedicated struct.
type GenericAnnotation struct {
	Kind   string         `json:"type,omitempty"`   // The annotation type name
	Fields map[string]any `json:"fields,omitempty"` // Arbitrary key-value payload
}

// AnnotationType returns the type identifier for this annotation.
func (g *GenericAnnotation) TypeName() string { return g.Kind }
