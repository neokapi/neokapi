package contextual

import (
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/check"
)

// Evidence is a model's rationale anchored to supplied source IDs and an exact
// candidate substring. Reference validity does not prove semantic correctness.
type Evidence struct {
	CandidateQuote string   `json:"candidate_quote"`
	SourceIDs      []string `json:"source_ids"`
	Rationale      string   `json:"rationale"`
}

// RequirementAssessment records mandatory coverage, including explicit abstention.
// Covered means the required action or decision is addressed; factual accuracy
// is assessed independently through conflicts. Missing means an instruction is
// absent, and uncertain records unresolved coverage.
type RequirementAssessment struct {
	RequirementID string `json:"requirement_id"`
	Status        string `json:"status"`
	Evidence
}

// Result retains the model's assessments and advisory findings without a score
// or pass flag. Evidence is the immutable JSON string used for the fingerprint;
// it cannot alias mutable request maps or slices.
type Result struct {
	Schema             string                  `json:"schema"`
	AnalyzerContract   string                  `json:"analyzer_contract"`
	RequestID          string                  `json:"request_id"`
	RequestFingerprint string                  `json:"request_fingerprint"`
	Evidence           string                  `json:"evidence"`
	Conflicts          []Evidence              `json:"conflicts"`
	Requirements       []RequirementAssessment `json:"requirements"`
	Suggestions        []Evidence              `json:"suggestions"`
	Findings           []check.Finding         `json:"findings"`
}

type response struct {
	Conflicts    []Evidence              `json:"conflicts"`
	Requirements []RequirementAssessment `json:"requirements"`
	Suggestions  []Evidence              `json:"suggestions"`
}

// ParseResponse accepts a complete bare JSON response or returns a zero Result
// and an error. No partial finding can escape a failed protocol validation.
// It checks evidence references and quotes, not entailment or semantic accuracy.
func ParseResponse(request Request, text string) (Result, error) {
	snapshot, err := snapshotRequest(request)
	if err != nil {
		return Result{}, err
	}
	parsed, err := decodeResponse(text)
	if err != nil {
		return Result{}, err
	}
	if err := validateResponse(request, parsed); err != nil {
		return Result{}, err
	}
	result := Result{
		Schema: Schema, AnalyzerContract: AnalyzerContract,
		RequestID: request.ID, RequestFingerprint: fingerprint(snapshot), Evidence: snapshot,
		Conflicts: parsed.Conflicts, Requirements: parsed.Requirements, Suggestions: parsed.Suggestions,
		Findings: []check.Finding{},
	}
	for _, conflict := range result.Conflicts {
		result.Findings = append(result.Findings, advisoryFinding(result, conflict, "conflict", ""))
	}
	for _, requirement := range result.Requirements {
		if requirement.Status == "missing" {
			result.Findings = append(result.Findings,
				advisoryFinding(result, requirement.Evidence, "missing-requirement", requirement.RequirementID))
		}
	}
	return result, nil
}

func validateResponse(request Request, parsed response) error {
	sources := make(map[string]bool, len(request.Sources))
	for _, source := range request.Sources {
		sources[source.ID] = true
	}
	for _, conflict := range parsed.Conflicts {
		if err := validateEvidence(conflict, request.Candidate, sources, true); err != nil {
			return fmt.Errorf("conflict: %w", err)
		}
	}
	for _, suggestion := range parsed.Suggestions {
		if err := validateEvidence(suggestion, request.Candidate, sources, false); err != nil {
			return fmt.Errorf("suggestion: %w", err)
		}
	}
	requirements := make(map[string]Requirement, len(request.Requirements))
	for _, requirement := range request.Requirements {
		requirements[requirement.ID] = requirement
	}
	seen := make(map[string]bool, len(parsed.Requirements))
	for _, assessment := range parsed.Requirements {
		requirement, exists := requirements[assessment.RequirementID]
		if !exists {
			return fmt.Errorf("unknown requirement %q", assessment.RequirementID)
		}
		if seen[assessment.RequirementID] {
			return fmt.Errorf("duplicate assessment for requirement %q", assessment.RequirementID)
		}
		seen[assessment.RequirementID] = true
		switch assessment.Status {
		case "covered", "missing", "uncertain":
		default:
			return fmt.Errorf("invalid requirement status %q", assessment.Status)
		}
		if err := validateEvidence(assessment.Evidence, request.Candidate, sources, assessment.Status == "covered"); err != nil {
			return fmt.Errorf("requirement %q: %w", assessment.RequirementID, err)
		}
		if !sharesSource(assessment.SourceIDs, requirement.SourceIDs) {
			return fmt.Errorf("requirement %q cites none of its declared sources", assessment.RequirementID)
		}
	}
	if len(seen) != len(requirements) {
		return errors.New("response must assess every declared requirement exactly once")
	}
	return nil
}

func sharesSource(actual, declared []string) bool {
	for _, id := range actual {
		if slices.Contains(declared, id) {
			return true
		}
	}
	return false
}

func validateEvidence(evidence Evidence, candidate string, sources map[string]bool, needsQuote bool) error {
	if strings.TrimSpace(evidence.Rationale) == "" {
		return errors.New("rationale is required")
	}
	if needsQuote && strings.TrimSpace(evidence.CandidateQuote) == "" {
		return errors.New("nonempty candidate_quote is required")
	}
	if !strings.Contains(candidate, evidence.CandidateQuote) {
		return errors.New("candidate_quote is not an exact candidate substring")
	}
	return validateSourceIDs(evidence.SourceIDs, sources)
}

func advisoryFinding(result Result, evidence Evidence, category, requirementID string) check.Finding {
	// IDs have been validated as strings, so this encoding cannot fail.
	sourceIDs, _ := json.Marshal(evidence.SourceIDs)
	metadata := map[string]string{
		"schema": result.Schema, "analyzer_contract": result.AnalyzerContract,
		"request_id": result.RequestID, "request_fingerprint": result.RequestFingerprint,
		"source_ids": string(sourceIDs), "evidence": result.Evidence,
		"assessment": "advisory",
	}
	if requirementID != "" {
		metadata["requirement_id"] = requirementID
	}
	return check.Finding{
		Category: category, Check: "contextual", Severity: check.SeverityNeutral,
		Message: evidence.Rationale, OriginalText: evidence.CandidateQuote, Metadata: metadata,
	}
}
