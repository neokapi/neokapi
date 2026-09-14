package check

// AnalyzerStatus describes execution, independently of finding severity.
type AnalyzerStatus string

const (
	AnalyzerPassed       AnalyzerStatus = "passed"
	AnalyzerFindings     AnalyzerStatus = "findings"
	AnalyzerNotRequested AnalyzerStatus = "not_requested"
	AnalyzerUnsupported  AnalyzerStatus = "unsupported"
	AnalyzerAbstained    AnalyzerStatus = "abstained"
	AnalyzerError        AnalyzerStatus = "error"
	AnalyzerStale        AnalyzerStatus = "stale"
	// AnalyzerInvalid means the analyzer missed its canary: whatever it reported
	// on the content cannot be trusted.
	AnalyzerInvalid AnalyzerStatus = "invalid"
	// AnalyzerDidNotRun means the analyzer was configured but had nothing it could
	// catch, so it checked nothing.
	AnalyzerDidNotRun AnalyzerStatus = "did_not_run"
)

// AnalyzerExecution records one analyzer's actual coverage of one input.
// Required means a requested analysis must complete; findings are still judged
// by the report's content gate. Missing coverage never means analysis passed.
type AnalyzerExecution struct {
	ID         string         `json:"id"`
	Status     AnalyzerStatus `json:"status"`
	File       string         `json:"file,omitempty"`
	Required   bool           `json:"required"`
	Findings   int            `json:"findings"`
	Reason     string         `json:"reason,omitempty"`
	DurationMS *float64       `json:"duration_ms,omitempty"`
	// Canary is what the analyzer made of the known-bad input it was given
	// beside the content. An analyzer that reports passed or findings without
	// one has not shown that it can fail.
	Canary *CanaryOutcome `json:"canary,omitempty"`
	// Point is the governance point of the blocks a governed analyzer ran
	// over, set when a file's comments sit at a point apart from its other
	// content and the analyzer ran once for each.
	Point *Point `json:"point,omitempty"`
}

// ExecutionTimings measures host work in milliseconds. Total excludes process
// startup and serialization. Phase sums can be less than total due to setup.
type ExecutionTimings struct {
	ContextMS    float64 `json:"context_ms"`
	ExtractionMS float64 `json:"extraction_ms"`
	AnalyzersMS  float64 `json:"analyzers_ms"`
	ReportMS     float64 `json:"report_ms"`
	TotalMS      float64 `json:"total_ms"`
}

// Execution is optional in v1: omitted means execution coverage is unreported.
// An invocation-aware producer supplies it; findings alone cannot reconstruct it.
type Execution struct {
	Analyzers []AnalyzerExecution `json:"analyzers"`
	Timings   ExecutionTimings    `json:"timings"`
	// Contexts records effective inputs to voice and terminology checks. Omitted
	// means context selection was not reported by this producer.
	Contexts []CheckContext `json:"contexts,omitempty"`
	// TermMatching records, per target language, how the terminology checks
	// matched terms. Omitted when no terminology check ran.
	TermMatching []TermMatching `json:"term_matching,omitempty"`
}

// How the terminology checks found a source term and recognised a rendering.
const (
	// TermSourceEnglishInflection finds an English source term with its
	// regular inflections, or with its declared forms when it has any.
	TermSourceEnglishInflection = "english-inflection"
	// TermSourceWholeWord finds a source term as a whole word, by its text and
	// declared forms only.
	TermSourceWholeWord = "whole-word"
	// TermTargetContainment accepts a target that contains a required wording.
	TermTargetContainment = "containment"
	// TermTargetContainmentForms also accepts a target that contains a declared
	// form of a required wording.
	TermTargetContainmentForms = "containment+forms"
)

// TermMatching is how the terminology checks matched terms for one target
// language.
type TermMatching struct {
	Locale string `json:"locale"`
	// Source is TermSourceEnglishInflection or TermSourceWholeWord.
	Source string `json:"source"`
	// Target is TermTargetContainment, or TermTargetContainmentForms when at
	// least one rule declares forms of a required wording.
	Target string `json:"target"`
	// Rules counts the rules applied, and RulesWithForms the rules that declare
	// forms of a required wording.
	Rules          int `json:"rules"`
	RulesWithForms int `json:"rules_with_forms"`
}

// MergeTermMatching adds m to list, keeping one entry per locale. A locale
// checked more than once, at points that bind different rules, reports the
// larger counts, and containment plus forms when any of its checks declared
// forms.
func MergeTermMatching(list []TermMatching, m TermMatching) []TermMatching {
	for i := range list {
		if list[i].Locale != m.Locale {
			continue
		}
		list[i].Rules = max(list[i].Rules, m.Rules)
		list[i].RulesWithForms = max(list[i].RulesWithForms, m.RulesWithForms)
		if m.Target == TermTargetContainmentForms {
			list[i].Target = TermTargetContainmentForms
		}
		return list
	}
	return append(list, m)
}

// CheckContext describes the guidance used for one checked input. ContextPath
// identifies an unwritten draft destination; File identifies extracted content.
type CheckContext struct {
	File        string       `json:"file,omitempty"`
	ContextPath string       `json:"context_path,omitempty"`
	Voice       VoiceContext `json:"voice"`
	// TermsApplied means a terminology store was supplied to the checks. It
	// does not assert that its terms matched this input or that findings exist.
	TermsApplied bool `json:"terms_applied"`
	// Point is the governance point this guidance was resolved at, set when a
	// project resolved it. A file whose comments sit apart has one entry for
	// each point.
	Point *Point `json:"point,omitempty"`
}

// VoiceContext records the selection that produced the actual checked profile.
// Selection is project, override or none; Applied distinguishes a resolved
// project location without a voice binding from a loaded profile.
type VoiceContext struct {
	Selection string `json:"selection"`
	Applied   bool   `json:"applied"`
	Name      string `json:"name,omitempty"`
	Source    string `json:"source,omitempty"`
	Profile   string `json:"profile,omitempty"`
	Channel   string `json:"channel,omitempty"`
}
