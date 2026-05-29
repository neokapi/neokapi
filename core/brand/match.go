package brand

import (
	"strings"

	"github.com/neokapi/neokapi/core/check"
)

// VocabKind distinguishes the two kinds of vocabulary violation a profile can
// raise: a forbidden term (a word the brand avoids) and a competitor term (a
// rival's name that must not appear).
type VocabKind int

const (
	VocabForbidden VocabKind = iota
	VocabCompetitor
)

// VocabHit is one brand-vocabulary match in a piece of text: which rule matched,
// at what byte range, and at what severity. It is the shared output of the
// vocabulary matcher, consumed both by the brand-vocab check tool (which maps
// the byte range onto run-anchored positions for the streaming pipeline) and by
// the blast-radius evaluator (which only needs the counts and severities).
type VocabHit struct {
	Kind        VocabKind
	Category    Dimension
	Severity    Severity
	Term        string
	Replacement string
	Note        string
	Start       int // byte offset into the searched text (inclusive)
	End         int // byte offset into the searched text (exclusive)
}

// MatchVocabulary returns every forbidden- and competitor-term hit in text under
// the profile's vocabulary rules. Matching is whole-word and Unicode-aware
// (check.FindTerm), so "use" never matches inside "user". Forbidden terms default
// to major severity and competitor terms to critical; a rule's own Severity, when
// set, overrides the default. A nil profile yields no hits.
//
// This is the single source of brand-vocabulary matching: the check tool and the
// blast-radius evaluator both call it so they can never diverge.
func MatchVocabulary(p *VoiceProfile, text string) []VocabHit {
	if p == nil {
		return nil
	}
	var hits []VocabHit
	for _, rule := range p.Vocabulary.ForbiddenTerms {
		sev := severityForRule(rule.Severity, SeverityMajor)
		for _, h := range check.FindTerm(text, rule.Term) {
			hits = append(hits, VocabHit{
				Kind:        VocabForbidden,
				Category:    DimensionVocabulary,
				Severity:    sev,
				Term:        rule.Term,
				Replacement: rule.Replacement,
				Note:        rule.Note,
				Start:       h[0],
				End:         h[1],
			})
		}
	}
	for _, rule := range p.Vocabulary.CompetitorTerms {
		sev := severityForRule(rule.Severity, SeverityCritical)
		for _, h := range check.FindTerm(text, rule.Term) {
			hits = append(hits, VocabHit{
				Kind:        VocabCompetitor,
				Category:    DimensionVocabulary,
				Severity:    sev,
				Term:        rule.Term,
				Replacement: rule.Replacement,
				Note:        rule.Note,
				Start:       h[0],
				End:         h[1],
			})
		}
	}
	return hits
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
