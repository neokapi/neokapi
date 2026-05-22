package schema

import (
	"fmt"
	"sync"
)

// sourceBuiltIn mirrors sourceBuiltIn to avoid an import cycle
// (schema cannot import registry because registry transitively depends on schema).
const sourceBuiltIn = "built-in"

// LocaleCardinality declares how many locales a tool operates on per execution.
type LocaleCardinality string

const (
	// Monolingual — tool operates on a single locale.
	// Examples: word-count (source), encoding-detect (source),
	// target normalization (target).
	Monolingual LocaleCardinality = "monolingual"

	// Bilingual — tool operates on exactly two locales as a pair.
	// Examples: ai-translate (source→target), qa-check (source vs target),
	// pivot comparison (de vs es).
	Bilingual LocaleCardinality = "bilingual"

	// Multilingual — tool operates on N locales simultaneously.
	// Examples: translation-comparison, cross-locale QA, consistency-check.
	Multilingual LocaleCardinality = "multilingual"
)

// AnnotationType identifies a kind of annotation that a tool produces on Blocks.
// Typed constants prevent typos and enable compile-time validation.
type AnnotationType string

const (
	AnnotationQAIssues       AnnotationType = "quality.qa-issues"
	AnnotationTMMatch        AnnotationType = "leverage.tm-match"
	AnnotationAltTranslation AnnotationType = "leverage.alt-translation"
	AnnotationTerms          AnnotationType = "terminology.annotations"
	AnnotationTermEnforce    AnnotationType = "terminology.enforcement"
	AnnotationWordCount      AnnotationType = "analysis.word-count"
	AnnotationCharCount      AnnotationType = "analysis.char-count"
	AnnotationSegCount       AnnotationType = "analysis.seg-count"
	AnnotationEntityMapping  AnnotationType = "entity.mapping"
	AnnotationComparison     AnnotationType = "analysis.comparison"
	AnnotationScopingReport  AnnotationType = "analysis.scoping-report"
	AnnotationRepetition     AnnotationType = "analysis.repetition"
	AnnotationTranslation    AnnotationType = "translation.output"
	AnnotationBrandVoice     AnnotationType = "quality.brand-voice"
)

// SideEffect identifies an external system interaction performed by a tool.
type SideEffect string

const (
	SideEffectTMRead        SideEffect = "tm-read"
	SideEffectTMWrite       SideEffect = "tm-write"
	SideEffectTermbaseRead  SideEffect = "termbase-read"
	SideEffectTermbaseWrite SideEffect = "termbase-write"
	SideEffectAPICall       SideEffect = "api-call"
	SideEffectAnalytics     SideEffect = "analytics"
)

// AnnotationTypeInfo describes a registered annotation type.
type AnnotationTypeInfo struct {
	Type        AnnotationType `json:"type"`
	DisplayName string         `json:"display_name"`
	Description string         `json:"description,omitempty"`
	Source      string         `json:"source"` // "built-in" or plugin name
}

// AnnotationRegistry manages known annotation types for validation
// and flow editor discoverability.
type AnnotationRegistry struct {
	mu    sync.RWMutex
	types map[AnnotationType]AnnotationTypeInfo
}

// NewAnnotationRegistry creates an empty registry.
func NewAnnotationRegistry() *AnnotationRegistry {
	return &AnnotationRegistry{
		types: make(map[AnnotationType]AnnotationTypeInfo),
	}
}

// Register adds an annotation type to the registry.
func (r *AnnotationRegistry) Register(info AnnotationTypeInfo) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.types[info.Type] = info
}

// Validate checks that an annotation type is registered. Returns an error
// if the type is unknown.
func (r *AnnotationRegistry) Validate(t AnnotationType) error {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if _, ok := r.types[t]; !ok {
		return fmt.Errorf("unknown annotation type: %s", t)
	}
	return nil
}

// List returns all registered annotation types.
func (r *AnnotationRegistry) List() []AnnotationTypeInfo {
	r.mu.RLock()
	defer r.mu.RUnlock()
	result := make([]AnnotationTypeInfo, 0, len(r.types))
	for _, info := range r.types {
		result = append(result, info)
	}
	return result
}

// Has reports whether an annotation type is registered.
func (r *AnnotationRegistry) Has(t AnnotationType) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.types[t]
	return ok
}

// RegisterBuiltIns registers all framework-defined annotation types.
func (r *AnnotationRegistry) RegisterBuiltIns() {
	builtins := []AnnotationTypeInfo{
		{AnnotationQAIssues, "QA Issues", "Quality check results", sourceBuiltIn},
		{AnnotationTMMatch, "TM Match", "Translation memory match score and type", sourceBuiltIn},
		{AnnotationAltTranslation, "Alt Translation", "Alternative translations from TM or AI", sourceBuiltIn},
		{AnnotationTerms, "Term Annotations", "Terminology matches found in source", sourceBuiltIn},
		{AnnotationTermEnforce, "Term Enforcement", "Terminology enforcement results", sourceBuiltIn},
		{AnnotationWordCount, "Word Count", "Word count per locale", sourceBuiltIn},
		{AnnotationCharCount, "Char Count", "Character count per locale", sourceBuiltIn},
		{AnnotationSegCount, "Segment Count", "Segment count", sourceBuiltIn},
		{AnnotationEntityMapping, "Entity Mapping", "Named entity annotations", sourceBuiltIn},
		{AnnotationComparison, "Comparison", "Cross-locale comparison results", sourceBuiltIn},
		{AnnotationScopingReport, "Scoping Report", "Detailed scoping analysis", sourceBuiltIn},
		{AnnotationRepetition, "Repetition", "Repeated segment analysis", sourceBuiltIn},
		{AnnotationTranslation, "Translation", "Translated target content", sourceBuiltIn},
		{AnnotationBrandVoice, "Brand Voice", "Brand voice compliance score and findings", sourceBuiltIn},
	}
	for _, info := range builtins {
		r.Register(info)
	}
}
