package review

import "github.com/neokapi/neokapi/core/profile"

// TermRuleLimit caps the term rules a point renders. The rules bearing on this
// unit's own wording lead the list, so a capped list still holds every rule
// the prompt would have scoped to the text; the point reports the total either
// way.
const TermRuleLimit = 25

// LeadTermRules puts the rules bearing on this unit's own wording at the front
// and caps the list, so a truncated list still holds every rule the prompt
// would have scoped to the text. A list within the cap is returned as it is.
func LeadTermRules(rules []profile.TermRule, sourceText string) []profile.TermRule {
	if len(rules) <= TermRuleLimit {
		return rules
	}
	scoped := profile.ScopeTermRules(rules, sourceText)
	lead := make(map[string]bool, len(scoped))
	for _, r := range scoped {
		lead[r.Term] = true
	}
	out := make([]profile.TermRule, 0, TermRuleLimit)
	out = append(out, scoped...)
	for _, r := range rules {
		if len(out) >= TermRuleLimit {
			break
		}
		if !lead[r.Term] {
			out = append(out, r)
		}
	}
	if len(out) > TermRuleLimit {
		out = out[:TermRuleLimit]
	}
	return out
}
