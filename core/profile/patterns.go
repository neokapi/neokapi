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
	// Where code sits, computed once: a scoped rule needs it and an unscoped
	// one does not pay for it.
	var spans []span
	scoped := false
	for _, pat := range p.Style.ProhibitedPatterns {
		if pat.Scope != "" {
			scoped = true
			break
		}
	}
	if scoped {
		spans = codeSpans(text)
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

		var found []PatternHit
		for _, m := range re.FindAllStringIndex(text, -1) {
			// A zero-width match marks no text, so it can neither be shown to a
			// reader nor anchored to a run.
			if m[0] == m[1] {
				continue
			}
			if !inScope(pat.Scope, text, spans, m[0]) {
				continue
			}
			found = append(found, PatternHit{
				Category:    DimensionStyle,
				Severity:    sev,
				Regex:       pat.Regex,
				Description: pat.Description,
				Start:       m[0],
				End:         m[1],
			})
		}

		// A rate permits some of what it matches. Under the ceiling nothing is
		// reported, because the rule says so; over it every match is, because
		// which occurrence is the excess is not a question the text answers.
		if pat.Rate != nil && len(found) <= pat.Rate.Allowance(countWords(text)) {
			continue
		}
		hits = append(hits, found...)
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
			f.Position = model.RangeAnchorForBytes(runs, hit.Start, hit.End)
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

// Findings is the profile's deterministic gate at BLOCK scope: forbidden and
// competitor vocabulary plus prohibited style patterns, mapped onto voice
// findings in that order. runs anchors the findings' positions; pass nil for
// plain text.
//
// Every surface that reports what a profile says about a piece of text calls
// this rather than assembling the two matchers itself, so no surface can enforce
// one half of a profile and present it as the whole.
//
// The required patterns are not here, and the omission is the rule's semantics
// rather than a gap: see [DocumentFindings].
func Findings(p *VoiceProfile, text string, runs []model.Run) []VoiceFinding {
	findings := HitsToFindings(MatchVocabulary(p, text), text, runs)
	return append(findings, PatternHitsToFindings(MatchPatterns(p, text), text, runs)...)
}

// DocumentFindings is the profile's deterministic gate at DOCUMENT scope: the
// required style patterns, over a whole document's text.
//
// "This text must contain X" is not a per-block assertion. Every paragraph of a
// page does not carry the call to action, the trademark line or the safety
// notice; the page does. Matched per block the way [Findings] matches the
// prohibited half, a required pattern would flag essentially every block of
// every document — so it is evaluated once, over the concatenation of a
// document's content, and the rules that found nothing are reported against the
// document rather than against any block in it. That is why a streaming tool,
// which sees one block at a time, cannot evaluate them and does not pretend to.
//
// Pass the whole document's text; an empty text still fails every required rule,
// because a document with no content carries nothing the rules ask for.
func DocumentFindings(p *VoiceProfile, text string) []VoiceFinding {
	return RequiredPatternFindings(UnmetRequiredPatterns(p, text))
}

// UnmetRequiredPatterns returns the profile's required patterns that text does
// not satisfy, in their declared order. Each rule's regex is matched as authored
// — no implicit case folding and no implicit word boundaries — exactly as
// [MatchPatterns] matches the prohibited half, so one profile's two pattern
// lists read the same way. An empty regex declares nothing and is skipped; a
// regex that does not compile is skipped for the same reason it is on the
// prohibited side (ValidateProfile is where an author learns it is broken, and
// a gate must not fail a document over a rule it cannot evaluate).
func UnmetRequiredPatterns(p *VoiceProfile, text string) []Pattern {
	if p == nil {
		return nil
	}
	var unmet []Pattern
	for _, pat := range p.Style.RequiredPatterns {
		src := strings.TrimSpace(pat.Regex)
		if src == "" {
			continue
		}
		re := compilePattern(src)
		if re == nil {
			continue
		}
		if !re.MatchString(text) {
			unmet = append(unmet, pat)
		}
	}
	return unmet
}

// RequiredPatternFindings maps unsatisfied required patterns onto voice
// findings. A required pattern's violation is an absence, so a finding carries
// no snippet and no position: there is no text to point at, which is the whole
// complaint. Patterns default to major severity; a rule's own Severity, when
// set, overrides the default.
func RequiredPatternFindings(unmet []Pattern) []VoiceFinding {
	if len(unmet) == 0 {
		return nil
	}
	findings := make([]VoiceFinding, 0, len(unmet))
	for _, pat := range unmet {
		f := VoiceFinding{
			Category: string(DimensionStyle),
			Severity: severityForRule(pat.Severity, SeverityMajor),
			// The regex is the rule's identity — the only thing an author has to
			// trace a finding back to the line in the profile that raised it.
			Metadata: map[string]string{"pattern": pat.Regex},
		}
		if desc := strings.TrimSpace(pat.Description); desc != "" {
			f.Message = "Required pattern absent: " + desc
		} else {
			f.Message = fmt.Sprintf("Required pattern %q is absent", pat.Regex)
		}
		findings = append(findings, f)
	}
	return findings
}

// PatternRuleCount is how many style pattern rules a profile applies — the
// number a surface reports as the profile's pattern-rule total. Both lists count
// because both are enforced: the prohibited patterns by [Findings] at block
// scope, the required ones by [DocumentFindings] at document scope. Reporting a
// count from anywhere else is how the two drift, and a rule counted but not
// applied is a profile claiming to govern something it does not.
func PatternRuleCount(p *VoiceProfile) int {
	if p == nil {
		return 0
	}
	return len(p.Style.ProhibitedPatterns) + len(p.Style.RequiredPatterns)
}

// span is a half-open byte range of the text.
type span struct{ start, end int }

// codeSpans finds fenced blocks, indented blocks and inline code spans.
//
// Byte ranges rather than a parse: this runs over whatever text a caller has,
// which may be a Markdown document, a paragraph out of one, or a block's runs
// joined together. A tolerant scan gets the common shapes right and never
// refuses to answer.
func codeSpans(text string) []span {
	var out []span

	// Fenced blocks, ``` or ~~~, to the matching fence or the end of the text.
	for _, m := range fencePattern.FindAllStringSubmatchIndex(text, -1) {
		out = append(out, span{m[0], m[1]})
	}
	// Inline spans, `like this`, outside any fence already found.
	for _, m := range inlineCodePattern.FindAllStringIndex(text, -1) {
		if !within(out, m[0]) {
			out = append(out, span{m[0], m[1]})
		}
	}
	return out
}

// fencePattern matches a fenced block including its fences. Non-greedy to the
// next fence, and tolerant of a block nobody closed.
var fencePattern = regexp.MustCompile("(?s)(?:^|\n)(?:```|~~~).*?(?:\n(?:```|~~~)|$)")

// inlineCodePattern matches a single-backtick span on one line.
var inlineCodePattern = regexp.MustCompile("`[^`\n]+`")

// headingLine reports whether the line containing pos is a Markdown heading.
func headingLine(text string, pos int) bool {
	start := strings.LastIndexByte(text[:pos], '\n') + 1
	line := text[start:]
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = line[:i]
	}
	return strings.HasPrefix(strings.TrimSpace(line), "#")
}

func within(spans []span, pos int) bool {
	for _, s := range spans {
		if pos >= s.start && pos < s.end {
			return true
		}
	}
	return false
}

// inScope reports whether a match at pos counts for a rule with this scope.
func inScope(scope, text string, spans []span, pos int) bool {
	switch scope {
	case "":
		return true
	case ScopeProse:
		return !within(spans, pos)
	case ScopeCode:
		return within(spans, pos)
	case ScopeHeading:
		return headingLine(text, pos)
	default:
		// An unrecognised scope is a rule nobody can read. Validation reports
		// it; matching treats it as unscoped rather than silently disabling it,
		// because a rule that quietly stops firing is the worse failure.
		return true
	}
}

// countWords counts whitespace-separated words, which is the unit a rate is
// stated in.
func countWords(text string) int { return len(strings.Fields(text)) }
