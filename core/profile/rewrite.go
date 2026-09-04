package profile

import (
	"slices"
	"sort"
	"strings"
)

// RewriteReason says why a vocabulary rule that matched left its text in place.
type RewriteReason string

const (
	// RewriteSkipNoReplacement is a rule that names a term and nothing to say
	// instead, so there is nothing to substitute.
	RewriteSkipNoReplacement RewriteReason = "no_replacement"
	// RewriteSkipInflectedForm is a match on one of the rule's declared forms.
	// The replacement is worded for the bare term, and substituting it for an
	// inflection leaves the wrong shape in the sentence ("we are use it").
	RewriteSkipInflectedForm RewriteReason = "inflected_form"
)

// RewriteChange is one substitution the rewrite made: every occurrence of the
// rule's term, as the profile declares it, replaced by the rule's replacement.
type RewriteChange struct {
	Term        string `json:"from"`
	Replacement string `json:"to"`
	List        string `json:"list"`
	Count       int    `json:"count"`
}

// RewriteSkip is a vocabulary rule that matched the text and was left in
// place, with the reason. It carries what a caller needs to finish the edit by
// hand: the rule's term, the list it sits in and the severity it bites at,
// where it applies, the spellings that matched, and the note or replacement
// the rule offers.
type RewriteSkip struct {
	Term        string        `json:"term"`
	List        string        `json:"list"`
	Severity    Severity      `json:"severity"`
	Scope       string        `json:"scope,omitempty"`
	Replacement string        `json:"replacement,omitempty"`
	Note        string        `json:"note,omitempty"`
	ConceptID   string        `json:"concept_id,omitempty"`
	Matched     []string      `json:"matched"`
	Count       int           `json:"count"`
	Reason      RewriteReason `json:"reason"`
}

// RewriteResult is what [RewriteVocabulary] returns: the text with the
// substitutions applied, the substitutions, and the rules that matched and
// were left in place.
type RewriteResult struct {
	Text    string
	Changes []RewriteChange
	Skipped []RewriteSkip
}

// RewriteVocabulary substitutes the profile's forbidden and competitor terms
// with the replacement each rule names and reports every rule that matched
// and could not be substituted.
//
// Matching is [MatchVocabulary], so the rewrite sees exactly the hits the
// vocabulary check reports: whole words, declared forms, case sensitivity,
// scope and containment suppression all apply. A hit is substituted when its
// rule names a replacement and the matched text is the term itself, in any
// casing for a case-insensitive rule. Every other hit stays in the text and is
// reported under Skipped with its reason, so a caller can tell "nothing to
// fix" from "violations found and not fixed" and finish the edit by hand.
//
// Where two rules claim overlapping text, the hit that starts first wins, and
// the longer one on a tie. A hit inside replaced text is neither substituted
// nor reported, because the text it matched is gone.
func RewriteVocabulary(p *VoiceProfile, text string) RewriteResult {
	res := RewriteResult{Text: text}
	hits := MatchVocabulary(p, text)
	if len(hits) == 0 {
		return res
	}

	// The spans to replace, chosen left to right among the hits whose rule can
	// substitute them.
	var candidates []int
	for i, h := range hits {
		if h.Replacement != "" && strings.EqualFold(text[h.Start:h.End], h.Term) {
			candidates = append(candidates, i)
		}
	}
	sort.SliceStable(candidates, func(a, b int) bool {
		ha, hb := hits[candidates[a]], hits[candidates[b]]
		if ha.Start != hb.Start {
			return ha.Start < hb.Start
		}
		return ha.End > hb.End
	})
	var chosenIdx []int
	var spans [][2]int
	end := 0
	for _, i := range candidates {
		h := hits[i]
		if h.Start < end {
			continue
		}
		chosenIdx = append(chosenIdx, i)
		spans = append(spans, [2]int{h.Start, h.End})
		end = h.End
	}

	chosen := make(map[int]bool, len(chosenIdx))
	var b strings.Builder
	pos := 0
	for _, i := range chosenIdx {
		h := hits[i]
		chosen[i] = true
		b.WriteString(text[pos:h.Start])
		b.WriteString(h.Replacement)
		pos = h.End
	}
	b.WriteString(text[pos:])
	res.Text = b.String()

	type key struct {
		kind   VocabKind
		term   string
		reason RewriteReason
	}
	changeAt := map[key]int{}
	skipAt := map[key]int{}
	for i, h := range hits {
		if chosen[i] {
			k := key{h.Kind, h.Term, ""}
			if j, ok := changeAt[k]; ok {
				res.Changes[j].Count++
				continue
			}
			changeAt[k] = len(res.Changes)
			res.Changes = append(res.Changes, RewriteChange{
				Term:        h.Term,
				Replacement: h.Replacement,
				List:        h.Kind.String(),
				Count:       1,
			})
			continue
		}
		if overlapsAny(h, spans) {
			continue
		}
		reason := RewriteSkipNoReplacement
		if h.Replacement != "" {
			reason = RewriteSkipInflectedForm
		}
		matched := text[h.Start:h.End]
		k := key{h.Kind, h.Term, reason}
		if j, ok := skipAt[k]; ok {
			s := &res.Skipped[j]
			s.Count++
			if !slices.Contains(s.Matched, matched) {
				s.Matched = append(s.Matched, matched)
			}
			continue
		}
		skipAt[k] = len(res.Skipped)
		res.Skipped = append(res.Skipped, RewriteSkip{
			Term:        h.Term,
			List:        h.Kind.String(),
			Severity:    h.Severity,
			Scope:       h.Scope,
			Replacement: h.Replacement,
			Note:        h.Note,
			ConceptID:   h.ConceptID,
			Matched:     []string{matched},
			Count:       1,
			Reason:      reason,
		})
	}
	return res
}

// overlapsAny reports whether the hit shares any byte with one of the spans.
func overlapsAny(h VocabHit, spans [][2]int) bool {
	for _, sp := range spans {
		if h.Start < sp[1] && sp[0] < h.End {
			return true
		}
	}
	return false
}
