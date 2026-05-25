package brand

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

func TestSeverityWeight(t *testing.T) {
	tests := []struct {
		severity Severity
		expected int
	}{
		{SeverityNeutral, 0},
		{SeverityMinor, 1},
		{SeverityMajor, 5},
		{SeverityCritical, 25},
		{Severity("unknown"), 0},
	}
	for _, tt := range tests {
		t.Run(string(tt.severity), func(t *testing.T) {
			assert.Equal(t, tt.expected, SeverityWeight(tt.severity))
		})
	}
}

func TestCalculateScore_NoFindings(t *testing.T) {
	score := CalculateScore(nil)

	assert.Equal(t, 100, score.Overall)
	assert.Len(t, score.Dimensions, 5)
	for _, dim := range score.Dimensions {
		assert.Equal(t, 100, dim.Score)
		assert.Equal(t, 0, dim.Penalty)
		assert.Equal(t, 0, dim.Issues)
	}
}

func TestCalculateScore_MixedSeverities(t *testing.T) {
	findings := []BrandVoiceFinding{
		{Dimension: DimensionTone, Severity: SeverityMinor, Message: "too casual", Position: model.RunRange{EndOffset: 5}},
		{Dimension: DimensionTone, Severity: SeverityMajor, Message: "wrong emotion", Position: model.RunRange{StartOffset: 10, EndOffset: 20}},
		{Dimension: DimensionVocabulary, Severity: SeverityCritical, Message: "competitor term", Position: model.RunRange{StartOffset: 30, EndOffset: 40}},
	}

	score := CalculateScore(findings)

	// Total penalty: 1 + 5 + 25 = 31
	assert.Equal(t, 69, score.Overall)
	assert.Len(t, score.Findings, 3)

	// Check per-dimension breakdown
	for _, dim := range score.Dimensions {
		switch dim.Dimension {
		case DimensionTone:
			assert.Equal(t, 94, dim.Score) // 100 - 1 - 5
			assert.Equal(t, 6, dim.Penalty)
			assert.Equal(t, 2, dim.Issues)
		case DimensionVocabulary:
			assert.Equal(t, 75, dim.Score) // 100 - 25
			assert.Equal(t, 25, dim.Penalty)
			assert.Equal(t, 1, dim.Issues)
		default:
			assert.Equal(t, 100, dim.Score)
			assert.Equal(t, 0, dim.Penalty)
			assert.Equal(t, 0, dim.Issues)
		}
	}
}

func TestCalculateScore_ClampAtZero(t *testing.T) {
	// 5 critical findings = 125 penalty, should clamp to 0
	findings := make([]BrandVoiceFinding, 5)
	for i := range findings {
		findings[i] = BrandVoiceFinding{
			Dimension: DimensionBrand,
			Severity:  SeverityCritical,
			Message:   "critical issue",
			Position:  model.RunRange{StartOffset: i * 10, EndOffset: i*10 + 5},
		}
	}

	score := CalculateScore(findings)

	assert.Equal(t, 0, score.Overall)

	// Brand dimension should also clamp to 0
	for _, dim := range score.Dimensions {
		if dim.Dimension == DimensionBrand {
			assert.Equal(t, 0, dim.Score)
			assert.Equal(t, 125, dim.Penalty)
			assert.Equal(t, 5, dim.Issues)
		}
	}
}

func TestCalculateScore_AllDimensionsPresent(t *testing.T) {
	score := CalculateScore(nil)

	dims := make(map[Dimension]bool)
	for _, d := range score.Dimensions {
		dims[d.Dimension] = true
	}

	assert.True(t, dims[DimensionTone])
	assert.True(t, dims[DimensionStyle])
	assert.True(t, dims[DimensionVocabulary])
	assert.True(t, dims[DimensionClarity])
	assert.True(t, dims[DimensionBrand])
}
