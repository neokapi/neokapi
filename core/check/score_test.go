package check

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalculateScoreRaw(t *testing.T) {
	assert.Equal(t, 100, CalculateScore(nil).Overall, "no findings → perfect")

	s := CalculateScore([]Finding{
		{Category: "terminology", Severity: SeverityMajor},         // 5
		{Category: "terminology", Severity: SeverityMinor},         // 1
		{Category: "do-not-translate", Severity: SeverityCritical}, // 25
	})
	assert.Equal(t, 100-31, s.Overall)

	// Per-category breakdown, sorted by category.
	assert.Len(t, s.Categories, 2)
	assert.Equal(t, "do-not-translate", s.Categories[0].Category)
	assert.Equal(t, 100-25, s.Categories[0].Score)
	assert.Equal(t, 1, s.Categories[0].Issues)
	assert.Equal(t, "terminology", s.Categories[1].Category)
	assert.Equal(t, 100-6, s.Categories[1].Score)
	assert.Equal(t, 2, s.Categories[1].Issues)
}

func TestCalculateScoreClampsAtZero(t *testing.T) {
	var findings []Finding
	for range 5 {
		findings = append(findings, Finding{Category: "x", Severity: SeverityCritical})
	}
	assert.Equal(t, 0, CalculateScore(findings).Overall, "125 penalty clamps to 0")
}

func TestCalculateScoreLengthNormalized(t *testing.T) {
	// A single minor nit (penalty 1) bites less in a long paragraph than in a
	// short string — this is the WordCount fix.
	one := []Finding{{Category: "style", Severity: SeverityMinor}}

	short := CalculateScore(one, WithWordCount(5))  // 1*100/5 = 20 → 80
	long := CalculateScore(one, WithWordCount(200)) // 1*100/200 = 0.5 → 1 → 99
	assert.Less(t, short.Overall, long.Overall, "short=%d long=%d", short.Overall, long.Overall)
	assert.Equal(t, 80, short.Overall)
	assert.Equal(t, 99, long.Overall)
	assert.True(t, short.Normalized)
	assert.Equal(t, 5, short.WordCount)

	// Without word count it is the raw roll-up.
	raw := CalculateScore(one)
	assert.Equal(t, 99, raw.Overall)
	assert.False(t, raw.Normalized)
}
