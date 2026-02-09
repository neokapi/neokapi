package event

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestQualityGatePass(t *testing.T) {
	gate := QualityGate{
		Name:      "completeness",
		Type:      GateBlocking,
		Threshold: 0.8,
		Evaluator: func(projectID string) (float64, string, error) {
			return 0.95, "95% translated", nil
		},
	}

	result, err := gate.Evaluate("proj-1")
	require.NoError(t, err)
	assert.True(t, result.Passed)
	assert.Equal(t, 0.95, result.Score)
}

func TestQualityGateFail(t *testing.T) {
	gate := QualityGate{
		Name:      "completeness",
		Type:      GateBlocking,
		Threshold: 0.8,
		Evaluator: func(projectID string) (float64, string, error) {
			return 0.5, "50% translated", nil
		},
	}

	result, err := gate.Evaluate("proj-1")
	require.NoError(t, err)
	assert.False(t, result.Passed)
}

func TestEvaluateGatesBlocking(t *testing.T) {
	gates := []QualityGate{
		{
			Name: "check1", Type: GateBlocking, Threshold: 0.9,
			Evaluator: func(projectID string) (float64, string, error) {
				return 0.5, "failing", nil
			},
		},
		{
			Name: "check2", Type: GateAdvisory, Threshold: 0.5,
			Evaluator: func(projectID string) (float64, string, error) {
				return 0.8, "ok", nil
			},
		},
	}

	results, err := EvaluateGates(gates, "proj-1")
	assert.Error(t, err) // Blocking gate should cause error
	assert.IsType(t, &GateError{}, err)
	assert.Len(t, results, 1) // Stopped at first blocking failure
}

func TestEvaluateGatesAdvisory(t *testing.T) {
	gates := []QualityGate{
		{
			Name: "advisory1", Type: GateAdvisory, Threshold: 0.9,
			Evaluator: func(projectID string) (float64, string, error) {
				return 0.5, "low score", nil
			},
		},
		{
			Name: "check2", Type: GateBlocking, Threshold: 0.5,
			Evaluator: func(projectID string) (float64, string, error) {
				return 0.8, "ok", nil
			},
		},
	}

	results, err := EvaluateGates(gates, "proj-1")
	require.NoError(t, err) // Advisory failures don't cause errors
	assert.Len(t, results, 2)
	assert.False(t, results[0].Passed) // Advisory failed but not blocking
	assert.True(t, results[1].Passed)
}
