package store

import (
	"cmp"
	"encoding/json"
	"slices"
)

// A RuleTally counts the findings one rule raised. A rule is a check's rule
// id ("terms.vocabulary", "voice.style") and, for a rule held in the
// project's context, its subject: the term a word rule names, or the pattern
// a voice rule matches. Key joins the two, so one term rule broken in many
// blocks counts as one rule.
type RuleTally struct {
	Key  string `json:"key"`
	Rule string `json:"rule"`
	// Subject is the term or the pattern the rule names; empty for a
	// standard check, which has no subject beyond its rule id.
	Subject string `json:"subject,omitempty"`
	// Message is one finding's wording, which names the rule to a person.
	Message string `json:"message,omitempty"`
	// ConceptID is the terms-store concept a word rule comes from, when it
	// comes from one.
	ConceptID string `json:"concept_id,omitempty"`
	// Fails is set when any of the counted findings fails a check.
	Fails bool `json:"fails,omitempty"`
	Count int  `json:"count"`
}

// RuleTallyKey is the key of a rule and its subject.
func RuleTallyKey(rule, subject string) string {
	if subject == "" {
		return rule
	}
	return rule + ":" + subject
}

// RuleTallyLimit bounds the rules one item's tally or one translation's
// facts keep: the ones with the most findings.
const RuleTallyLimit = 50

// RuleTallies accumulates rule tallies by key, in the order each rule was
// first seen. The zero value is empty and ready to use.
type RuleTallies struct {
	order []string
	by    map[string]*RuleTally
}

// Add counts t into the tallies: a new rule is kept as it is, and a rule
// already held gains t's count and its failing mark.
func (r *RuleTallies) Add(t RuleTally) {
	if t.Count <= 0 || t.Rule == "" {
		return
	}
	if t.Key == "" {
		t.Key = RuleTallyKey(t.Rule, t.Subject)
	}
	if r.by == nil {
		r.by = map[string]*RuleTally{}
	}
	if have, ok := r.by[t.Key]; ok {
		have.Count += t.Count
		have.Fails = have.Fails || t.Fails
		if have.ConceptID == "" {
			have.ConceptID = t.ConceptID
		}
		if have.Message == "" {
			have.Message = t.Message
		}
		return
	}
	r.order = append(r.order, t.Key)
	r.by[t.Key] = &t
}

// Len is the number of rules held.
func (r *RuleTallies) Len() int { return len(r.order) }

// Top returns up to n rules, the most findings first and then by key. An n
// below 1 returns every rule.
func (r *RuleTallies) Top(n int) []RuleTally {
	out := make([]RuleTally, 0, len(r.order))
	for _, k := range r.order {
		out = append(out, *r.by[k])
	}
	slices.SortFunc(out, func(a, b RuleTally) int {
		if c := cmp.Compare(b.Count, a.Count); c != 0 {
			return c
		}
		return cmp.Compare(a.Key, b.Key)
	})
	if n > 0 && len(out) > n {
		out = out[:n]
	}
	return out
}

// EncodeRuleTallies is the JSON array a store keeps rule tallies as; an
// empty list encodes as "[]".
func EncodeRuleTallies(rules []RuleTally) string {
	if len(rules) == 0 {
		return "[]"
	}
	raw, err := json.Marshal(rules)
	if err != nil {
		return "[]"
	}
	return string(raw)
}

// DecodeRuleTallies reads what EncodeRuleTallies wrote. Anything else reads
// as no rules, and no rules read as nil.
func DecodeRuleTallies(raw []byte) []RuleTally {
	var out []RuleTally
	if len(raw) == 0 || json.Unmarshal(raw, &out) != nil || len(out) == 0 {
		return nil
	}
	return out
}
