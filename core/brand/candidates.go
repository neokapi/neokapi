package brand

import "strings"

// AutonomyConfig controls progressive autonomy — how far a workspace lets the
// loop promote correction-derived rules without human review. It starts fully
// manual (zero value) and a team dials it up as it learns to trust the loop.
type AutonomyConfig struct {
	// AutoPromoteAtCount auto-promotes a candidate rule once the number of
	// corrections behind it reaches this threshold, with no human review. 0
	// (the default) keeps every promotion manual.
	AutoPromoteAtCount int `json:"auto_promote_at_count,omitempty" yaml:"auto_promote_at_count,omitempty"`
}

// ShouldAutoPromote reports whether a candidate rule has enough corrections
// behind it to be promoted automatically under the profile's autonomy settings.
func (p *VoiceProfile) ShouldAutoPromote(r SuggestedRule) bool {
	if p == nil || p.Autonomy.AutoPromoteAtCount <= 0 {
		return false
	}
	return r.CorrectionCount >= p.Autonomy.AutoPromoteAtCount
}

// MergeCandidates joins correction-derived suggestions with the recorded
// decisions for a profile, producing the candidate list the review UI and MCP
// tools consume. A suggestion with no decision is RuleDecisionPending. When
// includeResolved is false, candidates already rejected or promoted are dropped
// (the default review view — only what still needs a human); pass true to show
// the full history. Decisions are matched to suggestions case-insensitively by
// term, mirroring how the vocabulary matcher and ApplySuggestedRule compare terms.
func MergeCandidates(suggestions []*SuggestedRule, decisions []*RuleDecision, includeResolved bool) []CandidateRule {
	byTerm := make(map[string]*RuleDecision, len(decisions))
	for _, d := range decisions {
		byTerm[strings.ToLower(strings.TrimSpace(d.Term))] = d
	}
	out := make([]CandidateRule, 0, len(suggestions))
	for _, s := range suggestions {
		if s == nil {
			continue
		}
		c := CandidateRule{SuggestedRule: *s, Status: RuleDecisionPending}
		if d := byTerm[strings.ToLower(strings.TrimSpace(s.Term))]; d != nil {
			c.Status = d.Status
			c.PromotedVersion = d.PromotedVersion
			c.Auto = d.Auto
			c.DecidedBy = d.DecidedBy
			at := d.DecidedAt
			c.DecidedAt = &at
		}
		if !includeResolved && (c.Status == RuleDecisionRejected || c.Status == RuleDecisionPromoted) {
			continue
		}
		out = append(out, c)
	}
	return out
}
