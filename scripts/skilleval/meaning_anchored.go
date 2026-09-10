package main

import "github.com/neokapi/neokapi/core/check/contextual"

func validateMeaningAnchored(text string, input meaningInput) meaningIntegrity {
	integrity := meaningIntegrity{
		Errors: []string{},
		Scope:  "JSON shape, candidate/source references and requirement coverage only; claim grounding and semantics unmeasured",
	}
	result, err := contextual.ParseAnchoredResponse(meaningContextualRequest(input), text)
	if err != nil {
		integrity.Errors = append(integrity.Errors, err.Error())
		return integrity
	}
	integrity.Anchored = &result
	integrity.Review = meaningAnchoredReview(result)
	integrity.Valid = true
	return integrity
}

func meaningAnchoredReview(result contextual.AnchoredResult) *meaningReview {
	review := &meaningReview{Findings: []meaningFinding{}, Abstentions: []meaningAbstention{}}
	for _, conflict := range result.Conflicts {
		review.Findings = append(review.Findings, meaningFinding{
			CandidateQuote: meaningSpanQuote(conflict.CandidateSpans),
			SourceIDs:      conflict.SourceIDs, Kind: "conflict", Rationale: conflict.Rationale,
		})
	}
	for _, requirement := range result.Requirements {
		switch requirement.Status {
		case "missing":
			review.Findings = append(review.Findings, meaningFinding{
				CandidateQuote: meaningSpanQuote(requirement.CandidateSpans),
				SourceIDs:      requirement.SourceIDs, Kind: "omission", Rationale: requirement.Rationale,
			})
		case "uncertain":
			review.Abstentions = append(review.Abstentions, meaningAbstention{
				CandidateQuote: meaningSpanQuote(requirement.CandidateSpans), SourceIDs: requirement.SourceIDs,
				MissingEvidence: requirement.Rationale,
			})
		}
	}
	return review
}

// Multiple selected paragraphs remain separate evidence spans. Joining them
// would manufacture a quotation that does not occur in the original candidate.
func meaningSpanQuote(spans []contextual.CandidateSpan) string {
	if len(spans) == 1 {
		return spans[0].Text
	}
	return ""
}
