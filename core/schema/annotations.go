package schema

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
