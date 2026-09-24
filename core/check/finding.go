// Package check is the framework's content-verification core: deterministic and
// AI checks over content, acting as tests for AI output — deterministic and
// repeatable even when the generation that produced the content was not. A
// Checker inspects a block (read-only) and emits Findings; Findings carry a
// category, whether they fail, a human message, an optional suggested fix, and
// the run-range they apply to. Every checker — deterministic rule, small ML
// model, or LLM judge — emits the same Finding, so one scoring, annotation, and
// governance path serves terminology, do-not-translate, placeholder integrity,
// register, and voice profile alike.
package check

import "github.com/neokapi/neokapi/core/model"

// Penalty weights for the reported compliance score. A failing finding weighs
// enough that one of them keeps a block below the default compliance bar; a
// reported one costs a point.
const (
	FailingWeight   = 25
	ReportingWeight = 1
)

// Weight returns the score penalty a finding carries. A finding raised by a
// suggested rule weighs nothing: it is advice, and a rule nobody has settled
// must not move a score.
func Weight(f Finding) int {
	switch {
	case f.Suggested:
		return 0
	case f.Fails:
		return FailingWeight
	default:
		return ReportingWeight
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
	// Fails says whether the finding fails a check. The rule that raised it
	// decides: an established term or a voice pattern fails unless the rule is
	// marked advisory, and a style measure or a suggestion reports.
	Fails bool `json:"fails"`
	// Message is the human-readable explanation.
	Message string `json:"message"`
	// Suggestion is an optional remediation hint (e.g. the preferred term).
	Suggestion string `json:"suggestion,omitempty"`
	// Position is the run-range the finding applies to, anchored to source runs.
	Position model.Anchor `json:"position"`
	// OriginalText is the offending snippet, when available.
	OriginalText string `json:"original_text,omitempty"`
	// Check names the checker that produced this finding, stamped by Annotate.
	// Several checkers accumulate into one annotation and only the last one to
	// find something is named on it, so a reader that has nothing but the
	// annotation — a flow run, where the steps are whatever the recipe declared
	// — needs each finding to say who found it. A caller that already knows the
	// family it is mapping (kapi check groups findings into families of its own)
	// passes that family and this is not consulted.
	Check string `json:"check,omitempty"`
	// Metadata carries checker-specific detail (model name, confidence, the
	// matched rule id) without widening the struct per checker.
	Metadata map[string]string `json:"metadata,omitempty"`
	// Suggested marks a finding raised by a suggested rule: one a project has
	// accumulated and nobody has settled (core/contextop). Such a finding never
	// fails and weighs nothing in the score. A surface reads the flag to show
	// it as the suggestion it is rather than as a rule that was broken.
	Suggested bool `json:"suggested,omitempty"`
}
