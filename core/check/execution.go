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
}
