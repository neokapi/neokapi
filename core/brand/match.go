package brand

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
	ConceptID   string // knowledge-graph concept this rule denotes; empty for standalone profiles
	Start       int    // byte offset into the searched text (inclusive)
	End         int    // byte offset into the searched text (exclusive)
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
				ConceptID:   rule.ConceptID,
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
				ConceptID:   rule.ConceptID,
				Start:       h[0],
				End:         h[1],
			})
		}
	}
	return hits
}

// HitsToFindings maps vocabulary hits onto brand findings: the presentation
// message, the structured replacement and concept_id metadata, the offending
// snippet, and the run-anchored position. text is the searched string the hits
// index into (hit.Start/hit.End are byte offsets into it); runs are the source
// runs those offsets are anchored to, used to compute each finding's RunRange —
// pass nil when matching against plain, run-less text (the position is then left
// zero). It is the single hit→finding mapping shared by the streaming pipeline
// tool, the /check endpoint, and the check_vocabulary MCP tool, so none of them
// diverge on matching semantics, message wording, or concept propagation.
func HitsToFindings(hits []VocabHit, text string, runs []model.Run) []BrandVoiceFinding {
	if len(hits) == 0 {
		return nil
	}
	findings := make([]BrandVoiceFinding, 0, len(hits))
	for _, hit := range hits {
		f := BrandVoiceFinding{
			Category:     string(hit.Category),
			Severity:     hit.Severity,
			OriginalText: text[hit.Start:hit.End],
		}
		if len(runs) > 0 {
			f.Position = model.RunRangeForBytes(runs, hit.Start, hit.End)
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
