package profile

import (
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
)

// VocabKind distinguishes the kinds of word-rule violation: a forbidden term
// (a word the project avoids), a competitor term (a rival's name that must not
// appear) and a retired term (a word the project used until a decision
// replaced it).
type VocabKind int

const (
	VocabForbidden VocabKind = iota
	VocabCompetitor
	VocabRetired
)

// String names the kind as the change-set entry spells it: "forbidden",
// "competitor" or "retired".
func (k VocabKind) String() string {
	switch k {
	case VocabCompetitor:
		return "competitor"
	case VocabRetired:
		return "retired"
	}
	return "forbidden"
}

// VocabHit is one word-rule match in a piece of text: which rule matched, at
// what byte range, and whether it fails. It is the shared output of the
// word-rule matcher, consumed both by the voice-vocab check tool (which maps
// the byte range onto run-anchored positions for the streaming pipeline) and by
// the blast-radius evaluator (which only needs the counts).
type VocabHit struct {
	Kind        VocabKind
	Category    Dimension
	Fails       bool
	Term        string
	Replacement string
	Note        string
	ConceptID   string // knowledge-graph concept this rule denotes; empty for standalone profiles
	Scope       string // where the rule applies (TermRule.Scope); empty means everywhere
	// From names where the rule is held when that is not the terms store: a
	// starter pack ("pack technical-docs"), a voice file, the workspace.
	From  string
	Start int // byte offset into the searched text (inclusive)
	End   int // byte offset into the searched text (exclusive)
	// Suggested marks a hit against a suggested rule, one nobody has settled
	// yet. It never fails, so the hit is reported and settles nothing. A
	// surface shows it as the suggestion it is.
	Suggested bool
}

// A TermRuleSet is one source of word rules matched together: the rules, what
// kind of violation a hit against them is, where they are held, and whether
// they are settled. A rule marked Competitor is a competitor term whatever the
// set's kind.
//
// Grouping them this way is what lets one match run cover every source a
// caller holds (a tool's `term_rules:`, a starter pack's terms, the established
// and suggested rules a project holds) in a single pass over the text.
type TermRuleSet struct {
	Rules []TermRule
	Kind  VocabKind
	// From names where the rules are held, carried onto every hit.
	From string
	// Category the hits are raised under. Zero means DimensionVocabulary.
	Category Dimension
	// Suggested marks a set of suggested rules: ones a project has accumulated
	// and nobody has settled (core/contextop). Every hit against such a set
	// reports and never fails, whatever the rule says, so a suggestion is
	// reported everywhere a rule would be and can never fail a check. The
	// rule's own Advisory takes effect once it is established.
	Suggested bool
}

// MatchTermRules returns every hit in text under the given rule sets. Matching
// is whole-word and Unicode-aware (check.FindTerm), so "use" never matches
// inside "user". A hit fails unless its rule is advisory or its set is
// suggested; a rule naming no term is skipped. Case follows
// TermRule.MatchesCase.
//
// This is the single definition of what it means for a text to use a declared
// term. Every caller reaches it: the voice-vocabulary gate through
// [MatchVocabulary], and the term-locating pass through this entry point
// directly, so a word is a hit for the whole gate or for none of it.
func MatchTermRules(sets []TermRuleSet, text string) []VocabHit {
	// A placeholder's name is not a use of a term (check.PlaceholderText). Code
	// stays readable, so a rule that names no scope still finds a name written
	// wrongly in a code sample. The projection keeps every offset, so a hit
	// indexes the caller's text.
	text = check.PlaceholderText(text)

	// Where code sits, computed once and only when a rule asks: an unscoped
	// vocabulary — which is every profile written before scopes existed — pays
	// nothing for this.
	var codeAreas []span
	for _, set := range sets {
		for _, rule := range set.Rules {
			if rule.Scope != "" {
				codeAreas = codeSpans(text)
				break
			}
		}
		if codeAreas != nil {
			break
		}
	}

	var hits []VocabHit
	for _, set := range sets {
		category := set.Category
		if category == "" {
			category = DimensionVocabulary
		}
		for _, rule := range set.Rules {
			if strings.TrimSpace(rule.Term) == "" {
				continue
			}
			kind := set.Kind
			if rule.Competitor {
				kind = VocabCompetitor
			}
			// A suggested rule reports and never fails.
			fails := !rule.Advisory && !set.Suggested
			// Every shape the rule declares, matched exactly. See
			// TermRule.Forms and core/check/forms.go.
			find := check.FindTermForms
			if rule.MatchesCase() {
				find = check.FindTermFormsCased
			}
			matches := find(text, rule.AllForms())
			// A rule whose preferred replacement contains the term itself, such as
			// "cart" with the replacement "shopping cart", must not fire on the term
			// inside its own replacement phrase. Only such a rule pays for the
			// full-text scan: the containment test runs against the short
			// replacement, and the scan for replacement occurrences happens only
			// when it holds.
			var replacementSpans [][2]int
			if rule.Replacement != "" && len(find(rule.Replacement, rule.AllForms())) > 0 {
				replacementSpans = find(text, []string{rule.Replacement})
			}
			for _, h := range matches {
				if !inScope(rule.Scope, text, codeAreas, h[0]) {
					continue
				}
				if spanWithinAny(h, replacementSpans) {
					continue
				}
				hits = append(hits, VocabHit{
					Kind:        kind,
					Category:    category,
					Fails:       fails,
					Term:        rule.Term,
					Replacement: rule.Replacement,
					Note:        rule.Note,
					ConceptID:   rule.ConceptID,
					Scope:       rule.Scope,
					From:        set.From,
					Start:       h[0],
					End:         h[1],
					Suggested:   set.Suggested,
				})
			}
		}
	}
	return hits
}

// CarriedRuleSets is the word rules a profile's voice file carries (a starter
// pack's terms) as a rule set, named for where they come from. A caller
// combining them with rules from elsewhere passes this alongside its own, so
// one match run covers the lot. A profile carrying no rules declares no sets.
func CarriedRuleSets(p *VoiceProfile) []TermRuleSet {
	carried := p.CarriedTerms()
	if len(carried.Rules) == 0 {
		return nil
	}
	return []TermRuleSet{{Rules: carried.Rules, Kind: VocabForbidden, From: carried.From}}
}

// HasDeterministicRules reports whether p declares a rule the voice check
// applies to text: a word rule its file carries, a prohibited or required
// pattern, or a prohibited-pattern constraint in scope. Invalid constraints
// count too, because they report on every block. A profile holding only tone,
// guidance or comment limits declares none.
func HasDeterministicRules(p *VoiceProfile) bool {
	if p == nil {
		return false
	}
	if HasWordRules(p) {
		return true
	}
	patterns := slices.Concat(p.Style.ProhibitedPatterns, p.Style.RequiredPatterns)
	if slices.ContainsFunc(patterns, func(pat Pattern) bool { return strings.TrimSpace(pat.Regex) != "" }) {
		return true
	}
	return constraintError(p) != nil || constraintPatternCount(p) > 0
}

// HasWordRules reports whether the profile's voice file carries a word rule
// that rejects a term.
func HasWordRules(p *VoiceProfile) bool {
	return slices.ContainsFunc(p.CarriedTerms().Rules, func(rule TermRule) bool { return strings.TrimSpace(rule.Term) != "" })
}

// MatchCarriedTerms returns every hit in text under the word rules the
// profile's voice file carries. A nil profile yields no hits.
//
// It is the profile-shaped reading of [MatchTermRules], for the callers that
// hold a whole voice file and want all of its rules: the blast-radius
// evaluator and the scoring surfaces.
func MatchCarriedTerms(p *VoiceProfile, text string) []VocabHit {
	return MatchTermRules(CarriedRuleSets(p), text)
}

// HitsToFindings maps word-rule hits onto findings: the presentation
// message, the structured replacement and concept_id metadata, the offending
// snippet, and the run-anchored position. text is the searched string the hits
// index into (hit.Start/hit.End are byte offsets into it); runs are the source
// runs those offsets are anchored to, used to compute each finding's Anchor —
// pass nil when matching against plain, run-less text (the position is then left
// zero). It is the single hit→finding mapping shared by the streaming pipeline
// tool, the /check endpoint, and the check_vocabulary MCP tool, so none of them
// diverge on matching semantics, message wording, or concept propagation. A
// word rule reads the same wherever it is held; the "from" metadata names a
// source other than the terms store.
func HitsToFindings(hits []VocabHit, text string, runs []model.Run) []VoiceFinding {
	if len(hits) == 0 {
		return nil
	}
	findings := make([]VoiceFinding, 0, len(hits))
	for _, hit := range hits {
		f := VoiceFinding{
			Category:     string(hit.Category),
			Fails:        hit.Fails,
			OriginalText: text[hit.Start:hit.End],
			Suggested:    hit.Suggested,
		}
		if len(runs) > 0 {
			f.Position = model.RangeAnchorForBytes(runs, hit.Start, hit.End)
		}
		switch {
		case hit.Suggested:
			// A suggestion reads as one. "Forbidden" would say a decision has
			// been made, and nobody has made it yet.
			f.Message = fmt.Sprintf("Suggested rule about %q, not yet established", hit.Term)
			if hit.Note != "" {
				f.Message = fmt.Sprintf("Suggested rule about %q, not yet established: %s", hit.Term, hit.Note)
			}
		case hit.Kind == VocabCompetitor:
			f.Message = fmt.Sprintf("Competitor term %q found", hit.Term)
		case hit.Kind == VocabRetired:
			f.Message = fmt.Sprintf("Retired term %q found", hit.Term)
		default:
			f.Message = fmt.Sprintf("Forbidden term %q found", hit.Term)
			if hit.Note != "" {
				f.Message = fmt.Sprintf("Forbidden term %q found: %s", hit.Term, hit.Note)
			}
		}
		// Name the rule that fired. A host reporting "which rule was this" has
		// only OriginalText otherwise, which is the matched spelling rather than
		// the rule: a hit on "Utilise" or on a declared form like "utilising"
		// both come from the rule for "utilise", and only the rule is a thing a
		// reader can go and change.
		if hit.Term != "" {
			if f.Metadata == nil {
				f.Metadata = make(map[string]string)
			}
			f.Metadata["term"] = hit.Term
		}
		if hit.Replacement != "" {
			f.Suggestion = fmt.Sprintf("Use %q instead", hit.Replacement)
			// Carry the preferred term as a structured replacement so a host (the
			// desktop Checks panel) can offer a one-click fix alongside the message.
			if f.Metadata == nil {
				f.Metadata = make(map[string]string)
			}
			f.Metadata["replacement"] = hit.Replacement
		}
		// Link the finding to the knowledge-graph concept this rule denotes, so a
		// host can pivot from the violation to the concept story. Empty for
		// standalone profiles, so the key is simply absent there.
		if hit.ConceptID != "" {
			if f.Metadata == nil {
				f.Metadata = make(map[string]string)
			}
			f.Metadata["concept_id"] = hit.ConceptID
		}
		if hit.From != "" {
			if f.Metadata == nil {
				f.Metadata = make(map[string]string)
			}
			f.Metadata["from"] = hit.From
		}
		findings = append(findings, f)
	}
	return findings
}

// spanWithinAny reports whether the hit range [h[0],h[1]) lies wholly inside one
// of the given spans. It backs containment suppression: a term hit that falls
// inside an occurrence of its rule's own replacement phrase is not a violation.
func spanWithinAny(h [2]int, spans [][2]int) bool {
	for _, s := range spans {
		if s[0] <= h[0] && h[1] <= s[1] {
			return true
		}
	}
	return false
}
