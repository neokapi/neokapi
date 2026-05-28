package check

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSeverityWeight(t *testing.T) {
	tests := []struct {
		sev  Severity
		want int
	}{
		{SeverityNeutral, 0},
		{SeverityMinor, 1},
		{SeverityMajor, 5},
		{SeverityCritical, 25},
		{Severity("bogus"), 0},
	}
	for _, tt := range tests {
		assert.Equal(t, tt.want, SeverityWeight(tt.sev), "weight for %q", tt.sev)
	}
}

func TestParseSeverity(t *testing.T) {
	assert.Equal(t, SeverityNeutral, ParseSeverity("neutral"))
	assert.Equal(t, SeverityCritical, ParseSeverity("critical"))
	// Unknown defaults to minor so it is never silently zero-weighted.
	assert.Equal(t, SeverityMinor, ParseSeverity("nonsense"))
	assert.Equal(t, SeverityMinor, ParseSeverity(""))
}

func TestFindingJSONUsesCategory(t *testing.T) {
	f := Finding{
		Category:     "terminology",
		Severity:     SeverityMajor,
		Message:      "forbidden term",
		Suggestion:   "use this instead",
		OriginalText: "leverage",
		Metadata:     map[string]string{"rule": "vocab-1"},
	}
	b, err := json.Marshal(f)
	require.NoError(t, err)
	assert.Contains(t, string(b), `"category":"terminology"`)
	assert.Contains(t, string(b), `"severity":"major"`)

	var back Finding
	require.NoError(t, json.Unmarshal(b, &back))
	assert.Equal(t, f.Category, back.Category)
	assert.Equal(t, f.Severity, back.Severity)
	assert.Equal(t, "vocab-1", back.Metadata["rule"])
}
