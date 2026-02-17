package model

// Annotation is an extensible metadata attachment on Blocks.
type Annotation interface {
	AnnotationType() string
}

// AltTranslation holds an alternative translation with metadata.
type AltTranslation struct {
	Source    *Fragment
	Target    *Fragment
	Locale    LocaleID
	Origin    string  // Where this translation came from (TM, MT, etc.)
	Score     float64 // Match quality (0.0 - 1.0)
	MatchType string  // "exact", "fuzzy", "mt", "ai"
}

// AnnotationType returns the type identifier for this annotation.
func (at *AltTranslation) AnnotationType() string { return "alt-translation" }
