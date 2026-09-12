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
