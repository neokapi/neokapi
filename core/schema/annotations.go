package schema

// LocaleCardinality declares how many locales a tool operates on per execution.
type LocaleCardinality string

const (
	// Monolingual — tool operates on a single locale.
	// Examples: word-count (source), encoding-detect (source),
	// target normalization (target).
	Monolingual LocaleCardinality = "monolingual"

	// Bilingual — tool operates on exactly two locales as a pair.
	// Examples: translate (source→target), qa (source vs target),
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

	// SideEffectRemoteSourceEgress marks a tool that sends source content to a
	// remote system (a cloud LLM or MT provider). It is deliberately distinct
	// from SideEffectAPICall: a local detector, termbase, or TM lookup calls no
	// remote sink and must not carry it, while every cloud-provider call must.
	// The flow placement pass (AD-006) keys its redaction-safety rule off this
	// effect — a recoverable transformer (redact) must run before any step that
	// egresses source remotely, except the step producing the inputs its
	// detection consumes (the AD-020 detection trade-off). Tools whose
	// remoteness depends on configuration (an AI tool pointed at a local Ollama
	// or the offline demo provider) refine it away via their contract resolver.
	SideEffectRemoteSourceEgress SideEffect = "remote-source-egress"
)
