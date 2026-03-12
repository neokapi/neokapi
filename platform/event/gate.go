package event

// GateType determines whether a quality gate is blocking or advisory.
type GateType string

const (
	GateBlocking GateType = "blocking"
	GateAdvisory GateType = "advisory"
)

// QualityGate defines a quality check that runs during a push or version operation.
type QualityGate struct {
	Name      string
	Type      GateType
	Threshold float64 // 0.0-1.0 pass threshold
	Evaluator GateEvaluator
}

// GateResult contains the outcome of a quality gate evaluation.
type GateResult struct {
	GateName string
	Type     GateType
	Score    float64
	Passed   bool
	Details  string
}

// GateEvaluator is a function that evaluates a quality gate.
// It receives the project ID and returns a score from 0.0 to 1.0.
type GateEvaluator func(projectID string) (float64, string, error)

// Evaluate runs the quality gate and returns the result.
func (g *QualityGate) Evaluate(projectID string) (*GateResult, error) {
	score, details, err := g.Evaluator(projectID)
	if err != nil {
		return nil, err
	}
	return &GateResult{
		GateName: g.Name,
		Type:     g.Type,
		Score:    score,
		Passed:   score >= g.Threshold,
		Details:  details,
	}, nil
}

// EvaluateGates runs all gates and returns results.
// Returns an error only if a blocking gate fails.
func EvaluateGates(gates []QualityGate, projectID string) ([]GateResult, error) {
	var results []GateResult
	for _, gate := range gates {
		result, err := gate.Evaluate(projectID)
		if err != nil {
			return nil, err
		}
		results = append(results, *result)
		if !result.Passed && gate.Type == GateBlocking {
			return results, &GateError{Result: *result}
		}
	}
	return results, nil
}

// GateError is returned when a blocking quality gate fails.
type GateError struct {
	Result GateResult
}

func (e *GateError) Error() string {
	return "quality gate failed: " + e.Result.GateName + " (" + e.Result.Details + ")"
}
