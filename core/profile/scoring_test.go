package profile

import (
	"testing"

	"github.com/neokapi/neokapi/core/model"
	"github.com/stretchr/testify/assert"
)

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
	findings := []VoiceFinding{
		{Category: string(DimensionTone), Message: "too casual", Position: model.SpanAnchor(model.RunPos{Run: 0}, model.RunPos{Run: 0, Offset: 5})},
		{Category: string(DimensionTone), Fails: true, Message: "wrong emotion", Position: model.SpanAnchor(model.RunPos{Run: 0, Offset: 10}, model.RunPos{Run: 0, Offset: 20})},
		{Category: string(DimensionVocabulary), Fails: true, Message: "competitor term", Position: model.SpanAnchor(model.RunPos{Run: 0, Offset: 30}, model.RunPos{Run: 0, Offset: 40})},
	}

	score := CalculateScore(findings)

	// Total penalty: 1 + 25 + 25 = 51
	assert.Equal(t, 49, score.Overall)
	assert.Len(t, score.Findings, 3)

	// Check per-dimension breakdown
	for _, dim := range score.Dimensions {
		switch dim.Dimension {
		case DimensionTone:
			assert.Equal(t, 74, dim.Score) // 100 - 1 - 25
			assert.Equal(t, 26, dim.Penalty)
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
	findings := make([]VoiceFinding, 5)
	for i := range findings {
		findings[i] = VoiceFinding{
			Category: string(DimensionCompliance),
			Fails:    true,
			Message:  "critical issue",
			Position: model.SpanAnchor(model.RunPos{Run: 0, Offset: i * 10}, model.RunPos{Run: 0, Offset: i*10 + 5}),
		}
	}

	score := CalculateScore(findings)

	assert.Equal(t, 0, score.Overall)

	// Brand dimension should also clamp to 0
	for _, dim := range score.Dimensions {
		if dim.Dimension == DimensionCompliance {
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
	assert.True(t, dims[DimensionCompliance])
}
