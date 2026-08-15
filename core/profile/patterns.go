package profile

import (
	"fmt"
	"regexp"
	"strings"
	"sync"

	"github.com/neokapi/neokapi/core/model"
)

// PatternHit is one prohibited-pattern match in a piece of text: which rule
// matched, at what byte range, and at what severity. It is the pattern-side
// counterpart of VocabHit and carries the same byte offsets, so both feed the
// same run-anchored finding mapping.
type PatternHit struct {
	Category    Dimension
	Severity    Severity
	Regex       string // the rule's regex source, as authored
	Description string // the rule's human description; may be empty
	Start       int    // byte offset into the searched text (inclusive)
	End         int    // byte offset into the searched text (exclusive)
}

// patternCache holds the compiled form of every regex a profile has been matched
// with, keyed by the regex source. Profiles are matched once per block, so
// recompiling a pack's patterns for each block would dominate a corpus pass; the
// sources are configuration literals, so the cache is bounded by the packs and
// profiles a process loads. A source that does not compile caches a nil entry:
// ValidateProfile is where an author learns their regex is broken, and the
// matcher must not re-attempt (or panic on) it once per block.
var patternCache sync.Map // map[string]*regexp.Regexp

// compilePattern returns the compiled form of a regex source, or nil when the
// source does not compile.
func compilePattern(src string) *regexp.Regexp {
	if cached, ok := patternCache.Load(src); ok {
		re, _ := cached.(*regexp.Regexp)
		return re
	}
	re, err := regexp.Compile(src)
	if err != nil {
		re = nil
	}
	patternCache.Store(src, re)
	return re
}

// MatchPatterns returns every prohibited-pattern hit in text under the profile's
// style rules. Each rule's regex is matched as authored — no implicit case
// folding and no implicit word boundaries, because the rule IS the regex; an
// author who wants either writes `(?i)` or `\b`. Matches are leftmost and
// non-overlapping within a rule and independent across rules, so two rules that
// describe the same span each raise their own hit. Patterns default to major
// severity; a rule's own Severity, when set, overrides the default. A nil
// profile, an empty regex, a regex that does not compile, and a zero-width match
// all yield nothing.
//
// This is the single source of prohibited-pattern matching. Without it a rule
// declared in a profile existed only as prompt prose for the LLM (RenderVoiceGuide),
// so a pack could mark a rule critical and have it decide nothing while minor
// vocabulary rules decided the score.
func MatchPatterns(p *VoiceProfile, text string) []PatternHit {
	if p == nil || text == "" {
		return nil
	}
	var hits []PatternHit
	for _, pat := range p.Style.ProhibitedPatterns {
		src := strings.TrimSpace(pat.Regex)
		if src == "" {
			continue
		}
		re := compilePattern(src)
		if re == nil {
			continue
		}
		sev := severityForRule(pat.Severity, SeverityMajor)
		for _, m := range re.FindAllStringIndex(text, -1) {
			// A zero-width match marks no text, so it can neither be shown to a
			// reader nor anchored to a run.
			if m[0] == m[1] {
				continue
			}
			hits = append(hits, PatternHit{
				Category:    DimensionStyle,
				Severity:    sev,
				Regex:       pat.Regex,
				Description: pat.Description,
				Start:       m[0],
				End:         m[1],
			})
		}
	}
	return hits
}

// PatternHitsToFindings maps prohibited-pattern hits onto voice findings: the
// presentation message, the offending snippet, and the run-anchored position.
// text is the searched string the hits index into; runs are the source runs
// those offsets are anchored to — pass nil when matching against plain, run-less
// text (the position is then left zero).
func PatternHitsToFindings(hits []PatternHit, text string, runs []model.Run) []VoiceFinding {
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
			f.Position = model.RunRangeForBytes(runs, hit.Start, hit.End)
		}
		if desc := strings.TrimSpace(hit.Description); desc != "" {
			f.Message = "Prohibited pattern: " + desc
		} else {
			f.Message = fmt.Sprintf("Prohibited pattern %q matched", hit.Regex)
		}
		// The regex is the rule's identity — a host grouping findings by rule, or
		// an author tracing a finding back to the line in the profile that raised
		// it, has nothing else to key on.
		f.Metadata = map[string]string{"pattern": hit.Regex}
		findings = append(findings, f)
	}
	return findings
}

// Findings is the profile's whole deterministic gate over text: forbidden and
// competitor vocabulary plus prohibited style patterns, mapped onto voice
// findings in that order. runs anchors the findings' positions; pass nil for
// plain text.
//
// Every surface that reports what a profile says about a piece of text calls
// this rather than assembling the two matchers itself, so no surface can enforce
// one half of a profile and present it as the whole.
func Findings(p *VoiceProfile, text string, runs []model.Run) []VoiceFinding {
	findings := HitsToFindings(MatchVocabulary(p, text), text, runs)
	return append(findings, PatternHitsToFindings(MatchPatterns(p, text), text, runs)...)
}
