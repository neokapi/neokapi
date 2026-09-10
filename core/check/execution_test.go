package check

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExecutionIsOptionalAndPreservesReport(t *testing.T) {
	report := BuildReport(Target{Kind: "text", Blocks: 1}, nil, DefaultGate())
	legacy, err := json.Marshal(report)
	require.NoError(t, err)
	assert.NotContains(t, string(legacy), "execution")
	report.Execution = &Execution{Analyzers: []AnalyzerExecution{
		{ID: "hygiene", Status: AnalyzerPassed, Required: true},
		{ID: "voice.llm", Status: AnalyzerNotRequested, Reason: "Not requested."},
	}}
	data, err := json.Marshal(report)
	require.NoError(t, err)
	var oldClient struct {
		Schema  string  `json:"schema"`
		Pass    bool    `json:"pass"`
		Summary Summary `json:"summary"`
	}
	require.NoError(t, json.Unmarshal(data, &oldClient))
	assert.Equal(t, ReportSchema, oldClient.Schema)
	assert.True(t, oldClient.Pass)
	assert.Equal(t, 100, oldClient.Summary.Score)
	var decoded Report
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.NotNil(t, decoded.Execution)
	assert.Equal(t, AnalyzerNotRequested, decoded.Execution.Analyzers[1].Status)
}

// BenchmarkReportJSONMarshal isolates Go JSON encoding, excluding report
// construction, process startup, checker work and transport.
func BenchmarkReportJSONMarshal(b *testing.B) {
	for _, count := range []int{1, 300} {
		b.Run(fmt.Sprintf("findings-%d", count), func(b *testing.B) {
			diags := make([]Diagnostic, count)
			for i := range diags {
				diags[i] = Diagnostic{
					Rule: "hygiene.doubled-word", Check: "hygiene", Severity: SeverityMinor,
					Message: "Repeated word.", Location: Location{File: "sample.json", Block: fmt.Sprintf("block-%d", i)},
				}
			}
			report := BuildReport(Target{Kind: "file", File: "sample.json", Blocks: count}, diags, DefaultGate())
			report.Execution = &Execution{Analyzers: []AnalyzerExecution{
				{ID: "hygiene", Status: AnalyzerFindings, Required: true, Findings: count},
			}}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				if _, err := json.Marshal(report); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

func TestExecutionPreservesFindingDetailsOnWire(t *testing.T) {
	finding := Finding{
		Category: "preferred-term", Severity: SeverityMajor, Message: "Use the approved term.",
		Suggestion: "service", OriginalText: "product",
		Position: model.SpanAnchor(model.RunPos{Run: 1}, model.RunPos{Run: 2}),
		Metadata: map[string]string{"constraint_id": "service-description", "version": "1"},
	}
	diagnostic := DiagnosticFrom(finding, "voice", Location{File: "app.json", Block: "title"})
	report := BuildReport(Target{Kind: "file", File: "app.json", Blocks: 1}, []Diagnostic{diagnostic}, DefaultGate())
	report.Execution = &Execution{Analyzers: []AnalyzerExecution{{ID: "voice.rules", Status: AnalyzerFindings, Required: true, Findings: 1}}}
	data, err := json.Marshal(report)
	require.NoError(t, err)
	var decoded Report
	require.NoError(t, json.Unmarshal(data, &decoded))
	require.Len(t, decoded.Findings, 1)
	assert.Equal(t, diagnostic, decoded.Findings[0])
}
