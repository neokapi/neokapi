package brand

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBrandVoiceEvaluation_JSONRoundTrip(t *testing.T) {
	eval := BrandVoiceEvaluation{
		Stream:          "experiment",
		BaselineStream:  "main",
		StreamProfile:   "profile-new",
		BaselineProfile: "profile-old",
		BlocksEvaluated: 150,
		StreamScore: AggregateScore{
			Overall: 82.5,
			Min:     45,
			Max:     100,
			Median:  85,
			Distribution: map[string]int{
				"0-25":   2,
				"26-50":  8,
				"51-75":  30,
				"76-100": 110,
			},
		},
		BaselineScore: AggregateScore{
			Overall:      78.0,
			Min:          30,
			Max:          100,
			Median:       80,
			Distribution: map[string]int{"0-25": 5, "26-50": 15, "51-75": 35, "76-100": 95},
		},
		ScoreDelta: 4,
		BlastRadius: BlastRadius{
			TotalBlocks:        150,
			AffectedBlocks:     42,
			ImprovedBlocks:     35,
			DegradedBlocks:     7,
			NewViolations:      3,
			ResolvedViolations: 12,
			CriticalCount:      1,
			Collections: []CollectionBlastRadius{
				{
					CollectionID:   "col-1",
					CollectionName: "Homepage",
					AffectedBlocks: 10,
					AvgScoreDelta:  3.5,
				},
				{
					CollectionID:   "col-2",
					CollectionName: "FAQ",
					AffectedBlocks: 32,
					AvgScoreDelta:  -1.2,
				},
			},
		},
		DimensionComparison: []DimensionComparison{
			{Dimension: "tone", StreamAvg: 85.0, BaselineAvg: 80.0, Delta: 5.0},
			{Dimension: "style", StreamAvg: 78.0, BaselineAvg: 82.0, Delta: -4.0},
		},
		TopFindings: []EvaluationFinding{
			{
				BlockID:      "block-42",
				ItemName:     "welcome_msg",
				CollectionID: "col-1",
				SourceText:   "Welcome to our platform",
				TargetText:   "Bienvenue sur notre plateforme",
				Finding: BrandVoiceFinding{
					Category:   string(DimensionTone),
					Severity:   SeverityMajor,
					Message:    "Too informal for formal brand voice",
					Suggestion: "Use a more professional greeting",
				},
				IsNew: true,
			},
		},
	}

	data, err := json.Marshal(eval)
	require.NoError(t, err)

	var decoded BrandVoiceEvaluation
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, eval.Stream, decoded.Stream)
	assert.Equal(t, eval.BaselineStream, decoded.BaselineStream)
	assert.Equal(t, eval.BlocksEvaluated, decoded.BlocksEvaluated)
	assert.Equal(t, eval.ScoreDelta, decoded.ScoreDelta)
	assert.InDelta(t, eval.StreamScore.Overall, decoded.StreamScore.Overall, 0.001)
	assert.Equal(t, eval.StreamScore.Distribution, decoded.StreamScore.Distribution)
	assert.Equal(t, eval.BlastRadius.AffectedBlocks, decoded.BlastRadius.AffectedBlocks)
	assert.Equal(t, eval.BlastRadius.ImprovedBlocks, decoded.BlastRadius.ImprovedBlocks)
	assert.Equal(t, eval.BlastRadius.DegradedBlocks, decoded.BlastRadius.DegradedBlocks)
	assert.Len(t, decoded.BlastRadius.Collections, 2)
	assert.Equal(t, "Homepage", decoded.BlastRadius.Collections[0].CollectionName)
	assert.Len(t, decoded.DimensionComparison, 2)
	assert.InDelta(t, 5.0, decoded.DimensionComparison[0].Delta, 0.001)
	require.Len(t, decoded.TopFindings, 1)
	assert.True(t, decoded.TopFindings[0].IsNew)
	assert.Equal(t, string(DimensionTone), decoded.TopFindings[0].Finding.Category)
}

func TestEvaluateRequest_JSONRoundTrip(t *testing.T) {
	req := EvaluateRequest{
		Stream:             "experiment",
		BaselineStream:     "main",
		ProfileTag:         "v2-candidate",
		BaselineProfileTag: "v1-release",
		Locale:             "fr-FR",
		Collection:         "col-homepage",
		SampleSize:         50,
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var decoded EvaluateRequest
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, req, decoded)
}

func TestAggregateScore_EmptyDistribution(t *testing.T) {
	score := AggregateScore{
		Overall: 0,
		Min:     0,
		Max:     0,
		Median:  0,
	}

	data, err := json.Marshal(score)
	require.NoError(t, err)

	var decoded AggregateScore
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Nil(t, decoded.Distribution)
	assert.Equal(t, 0, decoded.Min)
}

func TestBlastRadius_ZeroValues(t *testing.T) {
	radius := BlastRadius{
		TotalBlocks: 100,
	}

	data, err := json.Marshal(radius)
	require.NoError(t, err)

	var decoded BlastRadius
	err = json.Unmarshal(data, &decoded)
	require.NoError(t, err)

	assert.Equal(t, 100, decoded.TotalBlocks)
	assert.Equal(t, 0, decoded.AffectedBlocks)
	assert.Equal(t, 0, decoded.CriticalCount)
	assert.Nil(t, decoded.Collections)
}
