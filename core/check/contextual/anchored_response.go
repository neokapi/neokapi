package contextual

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/check"
)

// AnchoredEvidence retains selected original paragraphs beside the model's
// rationale. CandidateSpans are resolved locally, never supplied by the model.
type AnchoredEvidence struct {
	CandidateIDs   []string        `json:"candidate_ids"`
	CandidateSpans []CandidateSpan `json:"candidate_spans"`
	SourceIDs      []string        `json:"source_ids"`
	Rationale      string          `json:"rationale"`
}

// AnchoredConflict records two alleged incompatible claims. Parsing verifies
// references and field presence, not whether these interpretations are true.
type AnchoredConflict struct {
	CandidateClaim string `json:"candidate_claim"`
	SourceClaim    string `json:"source_claim"`
	AnchoredEvidence
}

// AnchoredRequirementAssessment records coverage independently of factual
// accuracy. Uncertain records abstention and never generates a shared finding.
type AnchoredRequirementAssessment struct {
	RequirementID string `json:"requirement_id"`
	Status        string `json:"status"`
	AnchoredEvidence
}

// AnchoredResult preserves the full versioned input snapshot and selected spans.
// It contains advisory hypotheses with no semantic acceptance or score flag.
type AnchoredResult struct {
	Schema             string                          `json:"schema"`
	AnalyzerContract   string                          `json:"analyzer_contract"`
	RequestID          string                          `json:"request_id"`
	RequestFingerprint string                          `json:"request_fingerprint"`
	Evidence           string                          `json:"evidence"`
	CandidateSpans     []CandidateSpan                 `json:"candidate_spans"`
	Conflicts          []AnchoredConflict              `json:"conflicts"`
	Requirements       []AnchoredRequirementAssessment `json:"requirements"`
	Suggestions        []AnchoredEvidence              `json:"suggestions"`
	Findings           []check.Finding                 `json:"findings"`
}

type anchoredResponse struct {
	Conflicts    []AnchoredConflict              `json:"conflicts"`
	Requirements []AnchoredRequirementAssessment `json:"requirements"`
	Suggestions  []AnchoredEvidence              `json:"suggestions"`
}

// ParseAnchoredResponse accepts only complete bare JSON with known paragraph
// and source IDs. It resolves original text locally and returns a zero result
// on any validation error. It performs no semantic filtering or entailment test.
func ParseAnchoredResponse(request Request, text string) (AnchoredResult, error) {
	input, err := prepareAnchored(request)
	if err != nil {
		return AnchoredResult{}, err
	}
	snapshot, err := encodeAnchoredSnapshot(input)
	if err != nil {
		return AnchoredResult{}, err
	}
	parsed, err := decodeAnchoredResponse(text)
	if err != nil {
		return AnchoredResult{}, err
	}
	if err := resolveAnchoredResponse(input, &parsed); err != nil {
		return AnchoredResult{}, err
	}
	result := AnchoredResult{
		Schema: AnchoredSchema, AnalyzerContract: AnchoredAnalyzerContract,
		RequestID: request.ID, RequestFingerprint: fingerprint(snapshot), Evidence: snapshot,
		CandidateSpans: input.CandidateSpans, Conflicts: parsed.Conflicts,
		Requirements: parsed.Requirements, Suggestions: parsed.Suggestions, Findings: []check.Finding{},
	}
	for _, conflict := range result.Conflicts {
		finding := anchoredFinding(result, conflict.AnchoredEvidence, "conflict")
		finding.Metadata["candidate_claim"] = conflict.CandidateClaim
		finding.Metadata["source_claim"] = conflict.SourceClaim
		result.Findings = append(result.Findings, finding)
	}
	for _, requirement := range result.Requirements {
		if requirement.Status != "missing" {
			continue
		}
		finding := anchoredFinding(result, requirement.AnchoredEvidence, "missing-requirement")
		finding.Metadata["requirement_id"] = requirement.RequirementID
		result.Findings = append(result.Findings, finding)
	}
	return result, nil
}

type anchoredResolver struct {
	spans   map[string]CandidateSpan
	sources map[string]bool
}

func resolveAnchoredResponse(input anchoredInput, parsed *anchoredResponse) error {
	resolver := anchoredResolver{
		spans:   make(map[string]CandidateSpan, len(input.CandidateSpans)),
		sources: make(map[string]bool, len(input.Sources)),
	}
	for _, span := range input.CandidateSpans {
		resolver.spans[span.ID] = span
	}
	for _, source := range input.Sources {
		resolver.sources[source.ID] = true
	}
	for i := range parsed.Conflicts {
		conflict := &parsed.Conflicts[i]
		if strings.TrimSpace(conflict.CandidateClaim) == "" || strings.TrimSpace(conflict.SourceClaim) == "" {
			return errors.New("conflict requires explicit candidate_claim and source_claim")
		}
		if err := resolver.resolve(&conflict.AnchoredEvidence, true); err != nil {
			return fmt.Errorf("conflict: %w", err)
		}
	}
	for i := range parsed.Suggestions {
		if err := resolver.resolve(&parsed.Suggestions[i], false); err != nil {
			return fmt.Errorf("suggestion: %w", err)
		}
	}
	requirements := make(map[string]Requirement, len(input.Requirements))
	for _, requirement := range input.Requirements {
		requirements[requirement.ID] = requirement
	}
	seen := make(map[string]bool, len(parsed.Requirements))
	for i := range parsed.Requirements {
		assessment := &parsed.Requirements[i]
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
		if err := resolver.resolve(&assessment.AnchoredEvidence, assessment.Status == "covered"); err != nil {
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

func (resolver anchoredResolver) resolve(evidence *AnchoredEvidence, needsSpan bool) error {
	if strings.TrimSpace(evidence.Rationale) == "" {
		return errors.New("rationale is required")
	}
	if evidence.CandidateIDs == nil {
		return errors.New("candidate_ids must be an array")
	}
	if needsSpan && len(evidence.CandidateIDs) == 0 {
		return errors.New("nonempty candidate_ids are required")
	}
	if err := validateSourceIDs(evidence.SourceIDs, resolver.sources); err != nil {
		return err
	}
	seen := make(map[string]bool, len(evidence.CandidateIDs))
	evidence.CandidateSpans = make([]CandidateSpan, 0, len(evidence.CandidateIDs))
	for _, id := range evidence.CandidateIDs {
		span, exists := resolver.spans[id]
		if !exists {
			return fmt.Errorf("unknown candidate ID %q", id)
		}
		if seen[id] {
			return fmt.Errorf("duplicate candidate reference %q", id)
		}
		seen[id] = true
		evidence.CandidateSpans = append(evidence.CandidateSpans, span)
	}
	return nil
}

func anchoredFinding(result AnchoredResult, evidence AnchoredEvidence, category string) check.Finding {
	// These slices contain only strings and integer offsets, so encoding cannot fail.
	sourceIDs, _ := json.Marshal(evidence.SourceIDs)
	candidateIDs, _ := json.Marshal(evidence.CandidateIDs)
	spans, _ := json.Marshal(evidence.CandidateSpans)
	metadata := map[string]string{
		"schema": result.Schema, "analyzer_contract": result.AnalyzerContract,
		"request_id": result.RequestID, "request_fingerprint": result.RequestFingerprint,
		"source_ids": string(sourceIDs), "evidence": result.Evidence, "assessment": "advisory",
		"candidate_ids": string(candidateIDs), "candidate_spans": string(spans),
	}
	var originalText string
	if len(evidence.CandidateSpans) == 1 {
		originalText = evidence.CandidateSpans[0].Text
	}
	return check.Finding{
		Category: category, Check: "contextual", Severity: check.SeverityNeutral,
		Message: evidence.Rationale, OriginalText: originalText, Metadata: metadata,
	}
}
