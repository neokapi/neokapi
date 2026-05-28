// Package check is the framework's content-verification core. A Checker
// inspects a block (read-only) and emits Findings; Findings carry a category, a
// severity, a human message, an optional suggested fix, and the run-range they
// apply to. Every checker — deterministic rule, small ML model, or LLM judge —
// emits the same Finding, so one scoring, annotation, and governance path serves
// terminology, do-not-translate, placeholder integrity, register, and brand
// voice alike. The checks act like tests for AI output: they are deterministic
// and repeatable even when the generation that produced the content was not.
package check

import "github.com/neokapi/neokapi/core/model"

// Severity is the impact level of a Finding. The four levels carry MQM-inspired
// penalty weights (see SeverityWeight) used by score aggregation.
type Severity string

const (
	// SeverityNeutral is informational; it carries no penalty.
	SeverityNeutral Severity = "neutral"
	// SeverityMinor is a low-impact issue (style nit, soft preference).
	SeverityMinor Severity = "minor"
	// SeverityMajor is a clear violation a reviewer would act on.
	SeverityMajor Severity = "major"
	// SeverityCritical is a release-blocking violation (e.g. a translated
	// do-not-translate term, a dropped placeholder).
	SeverityCritical Severity = "critical"
)

// SeverityWeight returns the MQM-inspired penalty weight for a severity:
// neutral=0, minor=1, major=5, critical=25.
func SeverityWeight(s Severity) int {
	switch s {
	case SeverityMinor:
		return 1
	case SeverityMajor:
		return 5
	case SeverityCritical:
		return 25
	case SeverityNeutral:
		return 0
	default:
		return 0
	}
}

// ParseSeverity normalizes a string to a Severity, defaulting to SeverityMinor
// for unrecognized input so an unknown level is never silently dropped to zero
// penalty.
func ParseSeverity(s string) Severity {
	switch Severity(s) {
	case SeverityNeutral:
		return SeverityNeutral
	case SeverityMinor:
		return SeverityMinor
	case SeverityMajor:
		return SeverityMajor
	case SeverityCritical:
		return SeverityCritical
	default:
		return SeverityMinor
	}
}

// Finding is a single content-verification result. It is producer-agnostic: a
// deterministic rule, a small ML model, and an LLM judge all emit this struct
// into the same scoring and annotation pipeline.
type Finding struct {
	// Category groups the finding (e.g. "terminology", "do-not-translate",
	// "placeholder", "register", or a brand dimension such as "tone"). Free-form
	// so new checkers add categories without touching the core.
	Category string `json:"category"`
	// Severity drives the penalty weight and the gate.
	Severity Severity `json:"severity"`
	// Message is the human-readable explanation.
	Message string `json:"message"`
	// Suggestion is an optional remediation hint (e.g. the preferred term).
	Suggestion string `json:"suggestion,omitempty"`
	// Position is the run-range the finding applies to, anchored to source runs.
	Position model.RunRange `json:"position"`
	// OriginalText is the offending snippet, when available.
	OriginalText string `json:"original_text,omitempty"`
	// Metadata carries checker-specific detail (model name, confidence, the
	// matched rule id) without widening the struct per checker.
	Metadata map[string]string `json:"metadata,omitempty"`
}
