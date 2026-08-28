package profile

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/check"
	"github.com/neokapi/neokapi/core/model"
)

// VocabKind distinguishes the two kinds of vocabulary violation a profile can
// raise: a forbidden term (a word the brand avoids) and a competitor term (a
// rival's name that must not appear).
type VocabKind int

const (
	VocabForbidden VocabKind = iota
	VocabCompetitor
)

// VocabHit is one voice-vocabulary match in a piece of text: which rule matched,
// at what byte range, and at what severity. It is the shared output of the
// vocabulary matcher, consumed both by the voice-vocab check tool (which maps
// the byte range onto run-anchored positions for the streaming pipeline) and by
// the blast-radius evaluator (which only needs the counts and severities).
type VocabHit struct {
	Kind        VocabKind
	Category    Dimension
	Severity    Severity
	Term        string
	Replacement string
	Note        string
	ConceptID   string // knowledge-graph concept this rule denotes; empty for standalone profiles
	Start       int    // byte offset into the searched text (inclusive)
	End         int    // byte offset into the searched text (exclusive)
}

// A TermRuleSet is one source of term rules matched together: the rules, what
// kind of violation a hit against them is, and the severity a rule that names
// none of its own takes.
//
// The set is what carries kind and default severity, because a rule does not:
// the same TermRule is a forbidden term in one list and a competitor's name in
// another, and the two answer differently for how hard a hit bites. Grouping
// them this way is what lets one match run cover every source a caller holds —
// a voice profile's two lists, a tool's `term_rules:`, the concepts resolved
// from a terms store — in a single pass over the text.
type TermRuleSet struct {
	Rules []TermRule
	Kind  VocabKind
	// Category the hits are raised under. Zero means DimensionVocabulary.
	Category Dimension
	// Default severity for a rule that names none. Zero means SeverityMajor.
	Default Severity
}

// MatchTermRules returns every hit in text under the given rule sets. Matching
// is whole-word and Unicode-aware (check.FindTerm), so "use" never matches
// inside "user". A rule's own Severity, when set, overrides its set's default;
// a rule naming no term is skipped.
//
// This is the single definition of what it means for a text to use a declared
// term. Every caller reaches it: the voice-vocabulary gate through
// [MatchVocabulary], and the term-locating pass through this entry point
// directly, so a word is a hit for the whole gate or for none of it.
func MatchTermRules(sets []TermRuleSet, text string) []VocabHit {
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
		fallback := set.Default
		if fallback == "" {
			fallback = SeverityMajor
		}
		for _, rule := range set.Rules {
			if strings.TrimSpace(rule.Term) == "" {
				continue
			}
			sev := severityForRule(rule.Severity, fallback)
			// Every shape the rule declares, matched exactly. See
			// TermRule.Forms and core/check/forms.go.
			find := check.FindTermForms
			if rule.CaseSensitive {
				find = check.FindTermFormsCased
			}
			for _, h := range find(text, rule.AllForms()) {
				if !inScope(rule.Scope, text, codeAreas, h[0]) {
					continue
				}
				hits = append(hits, VocabHit{
					Kind:        set.Kind,
					Category:    category,
					Severity:    sev,
					Term:        rule.Term,
					Replacement: rule.Replacement,
					Note:        rule.Note,
					ConceptID:   rule.ConceptID,
					Start:       h[0],
					End:         h[1],
				})
			}
		}
	}
	return hits
}

// VocabularyRuleSets is a profile's vocabulary as rule sets: forbidden terms at
// major severity, a competitor's names at critical. A caller combining a
// profile's rules with rules from elsewhere passes these alongside its own, so
// one match run covers the lot. A nil profile declares no sets.
func VocabularyRuleSets(p *VoiceProfile) []TermRuleSet {
	if p == nil {
		return nil
	}
	return []TermRuleSet{
		{Rules: p.Vocabulary.ForbiddenTerms, Kind: VocabForbidden, Default: SeverityMajor},
		{Rules: p.Vocabulary.CompetitorTerms, Kind: VocabCompetitor, Default: SeverityCritical},
	}
}

// MatchVocabulary returns every forbidden- and competitor-term hit in text under
// the profile's vocabulary rules. A nil profile yields no hits.
//
// It is the profile-shaped reading of [MatchTermRules], for the callers that
// hold a whole profile and want its whole vocabulary: the check tool, the
// blast-radius evaluator, the scoring surfaces.
func MatchVocabulary(p *VoiceProfile, text string) []VocabHit {
	return MatchTermRules(VocabularyRuleSets(p), text)
}

// HitsToFindings maps vocabulary hits onto voice findings: the presentation
// message, the structured replacement and concept_id metadata, the offending
// snippet, and the run-anchored position. text is the searched string the hits
// index into (hit.Start/hit.End are byte offsets into it); runs are the source
// runs those offsets are anchored to, used to compute each finding's Anchor —
// pass nil when matching against plain, run-less text (the position is then left
// zero). It is the single hit→finding mapping shared by the streaming pipeline
// tool, the /check endpoint, and the check_vocabulary MCP tool, so none of them
// diverge on matching semantics, message wording, or concept propagation.
func HitsToFindings(hits []VocabHit, text string, runs []model.Run) []VoiceFinding {
	if len(hits) == 0 {
		return nil
	}
	findings := make([]VoiceFinding, 0, len(hits))
	for _, hit := range hits {
		f := VoiceFinding{
			Category:     string(hit.Category),
			Severity:     hit.Severity,
			OriginalText: text[hit.Start:hit.End],
		}
		if len(runs) > 0 {
			f.Position = model.RangeAnchorForBytes(runs, hit.Start, hit.End)
		}
		switch hit.Kind {
		case VocabCompetitor:
			f.Message = fmt.Sprintf("Competitor term %q found", hit.Term)
		default:
			f.Message = fmt.Sprintf("Forbidden term %q found", hit.Term)
			if hit.Note != "" {
				f.Message = fmt.Sprintf("Forbidden term %q found: %s", hit.Term, hit.Note)
			}
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
		findings = append(findings, f)
	}
	return findings
}

// severityForRule maps a TermRule's textual severity onto the framework scale,
// falling back to def when the rule does not set one.
func severityForRule(s string, def Severity) Severity {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "neutral":
		return SeverityNeutral
	case "minor":
		return SeverityMinor
	case "major":
		return SeverityMajor
	case "critical":
		return SeverityCritical
	default:
		return def
	}
}
