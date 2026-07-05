package model

import "encoding/json"

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
//
// JSON casing is camelCase, matching the canonical content-model JSON
// (protojson of neokapi.content.v1, AD-034) and the rest of core/model.
// Payloads serialized before the camelCase switch used snake_case keys
// (match_type, combined_score, …) — and the released okapi-bridge JARs still
// produce them — so UnmarshalJSON accepts both spellings.
type AltTranslation struct {
	Source        []Run     `json:"source,omitempty"`
	Target        []Run     `json:"target,omitempty"`
	Locale        LocaleID  `json:"locale,omitempty"`
	Origin        string    `json:"origin,omitempty"`        // Where this translation came from (TM, MT, etc.)
	Score         float64   `json:"score,omitempty"`         // Match quality (0.0 - 1.0)
	MatchType     MatchType `json:"matchType,omitempty"`     // MatchExact, MatchFuzzy, MatchMT, or MatchAI
	CombinedScore float64   `json:"combinedScore,omitempty"` // Combined match score (Okapi)
	FuzzyScore    float64   `json:"fuzzyScore,omitempty"`    // Fuzzy match score (Okapi)
	QualityScore  float64   `json:"qualityScore,omitempty"`  // Quality score (Okapi)
	Engine        string    `json:"engine,omitempty"`        // MT/AI engine name
	ToolID        string    `json:"toolId,omitempty"`        // Tool that produced this translation
	AltTransType  string    `json:"altTransType,omitempty"`  // Okapi alt-trans type (e.g., "proposal", "previous-version")
	FromOriginal  bool      `json:"fromOriginal,omitempty"`  // Whether this came from the original document
	SegmentIndex  int       `json:"segmentIndex,omitempty"`  // -1/0 = whole block; >0 = per-segment match at this segment index
}

// UnmarshalJSON decodes an AltTranslation, accepting the legacy snake_case
// key spellings alongside the canonical camelCase ones. Legacy producers are
// (a) payloads persisted before the camelCase switch (sync annotations_json,
// server annotation rows, blockstore caches, .klz parcels) and (b) released
// okapi-bridge JARs, whose AnnotationExtractor emits snake_case keys. When a
// document carries both spellings of a key, camelCase wins. The snake_case
// accepts are scheduled for removal one release after the switch — except the
// bridge keys, which stay until every registry-pinned bridge emits camelCase.
func (a *AltTranslation) UnmarshalJSON(data []byte) error {
	type plain AltTranslation // method-free alias: plain decode, no recursion
	var p plain
	if err := json.Unmarshal(data, &p); err != nil {
		return err
	}
	var legacy struct {
		MatchType     MatchType `json:"match_type"`
		CombinedScore float64   `json:"combined_score"`
		FuzzyScore    float64   `json:"fuzzy_score"`
		QualityScore  float64   `json:"quality_score"`
		ToolID        string    `json:"tool_id"`
		AltTransType  string    `json:"alt_trans_type"`
		FromOriginal  bool      `json:"from_original"`
		SegmentIndex  int       `json:"segment_index"`
	}
	// Best-effort: a legacy-key type mismatch must not fail a payload that
	// already decoded cleanly under the canonical keys.
	_ = json.Unmarshal(data, &legacy)
	if p.MatchType == "" {
		p.MatchType = legacy.MatchType
	}
	if p.CombinedScore == 0 {
		p.CombinedScore = legacy.CombinedScore
	}
	if p.FuzzyScore == 0 {
		p.FuzzyScore = legacy.FuzzyScore
	}
	if p.QualityScore == 0 {
		p.QualityScore = legacy.QualityScore
	}
	if p.ToolID == "" {
		p.ToolID = legacy.ToolID
	}
	if p.AltTransType == "" {
		p.AltTransType = legacy.AltTransType
	}
	if !p.FromOriginal {
		p.FromOriginal = legacy.FromOriginal
	}
	if p.SegmentIndex == 0 {
		p.SegmentIndex = legacy.SegmentIndex
	}
	*a = AltTranslation(p)
	return nil
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
