package model

// TermStatus represents the lifecycle state of a term.
type TermStatus string

const (
	TermProposed   TermStatus = "proposed"
	TermApproved   TermStatus = "approved"
	TermPreferred  TermStatus = "preferred"  // use this term
	TermAdmitted   TermStatus = "admitted"   // acceptable alternative
	TermDeprecated TermStatus = "deprecated" // was valid, now outdated
	TermForbidden  TermStatus = "forbidden"  // never use this term
)

// MatchStrategy indicates how a term was matched during lookup.
type MatchStrategy string

const (
	MatchStrategyExact      MatchStrategy = "exact"      // exact string match
	MatchStrategyNormalized MatchStrategy = "normalized" // case/whitespace/diacritics normalized
	MatchStrategyFuzzy      MatchStrategy = "fuzzy"      // Levenshtein edit distance
	MatchStrategyStem       MatchStrategy = "stem"       // linguistic stemming
	MatchStrategyAI         MatchStrategy = "ai"         // LLM-assisted matching
)

// TermRef is a lightweight reference to a target term.
type TermRef struct {
	Text   string     // the target term text
	Locale LocaleID   // target locale
	Status TermStatus // lifecycle status
}

// TermAnnotation carries a matched term with its position in the source text.
// Implements the Annotation interface. Produced by the term-lookup pipeline tool.
type TermAnnotation struct {
	SourceTerm  string        // as found in source text
	ConceptID   string        // ID of the matched concept
	TargetTerms []TermRef     // preferred translations per locale
	Status      TermStatus    // lifecycle status of matched term
	Position    RunRange      // run-anchored span in the source runs
	Score       float64       // match confidence (1.0 for exact)
	MatchType   MatchStrategy // how it was matched
}

// AnnotationType returns the type identifier for term annotations.
func (ta *TermAnnotation) AnnotationType() string { return "term" }
