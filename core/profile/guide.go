package profile

import (
	"fmt"
	"sort"
	"strings"
)

// RenderVoiceGuide produces a markdown-formatted voice guide optimized for
// LLM consumption. It is the single source of truth for turning a VoiceProfile
// into prompt text — used by the AI translate prompt (so generation is on-brand),
// the voice check tool, the local `kapi voice guide` command, and the
// bowrain cloud MCP `get_voice_guide` tool.
//
// Output is deterministic: slices render in their declared order and map-derived
// sections (abbreviations) are sorted by key, so the same profile always yields
// byte-identical text.
func RenderVoiceGuide(p *VoiceProfile) string {
	var b strings.Builder
	if p == nil {
		return ""
	}

	fmt.Fprintf(&b, "# Voice Guide: %s\n\n", p.Name)
	if p.Description != "" {
		fmt.Fprintf(&b, "%s\n\n", p.Description)
	}

	// Tone
	b.WriteString("## Tone\n")
	if len(p.Tone.Personality) > 0 {
		fmt.Fprintf(&b, "- Personality: %s\n", strings.Join(p.Tone.Personality, ", "))
	}
	fmt.Fprintf(&b, "- Formality: %s\n", p.Tone.Formality)
	fmt.Fprintf(&b, "- Emotion: %s\n", p.Tone.Emotion)
	fmt.Fprintf(&b, "- Humor: %s\n", p.Tone.Humor)
	if p.Tone.Guidelines != "" {
		fmt.Fprintf(&b, "- Guidelines: %s\n", p.Tone.Guidelines)
	}
	b.WriteString("\n")

	// Style
	b.WriteString("## Style Rules\n")
	if p.Style.ActiveVoice {
		b.WriteString("- Use active voice\n")
	}
	fmt.Fprintf(&b, "- Sentence length: %s\n", p.Style.SentenceLength)
	fmt.Fprintf(&b, "- Point of view: %s\n", p.Style.PersonPOV)
	fmt.Fprintf(&b, "- Contractions: %s\n", p.Style.Contractions)
	// Both renderers name the words a pattern bans (#2240). The full guide is
	// what `kapi voice guide` prints and what an assistant is handed, so
	// fixing only the compact one would have left the surface most people see
	// still saying "avoid implementation vocabulary" and naming none of it.
	if len(p.Style.ProhibitedPatterns) > 0 {
		b.WriteString("- Prohibited patterns:\n")
		for _, pat := range p.Style.ProhibitedPatterns {
			fmt.Fprintf(&b, "  - %s (severity: %s)\n", patternHint(pat), pat.Severity)
		}
	}
	if len(p.Style.RequiredPatterns) > 0 {
		b.WriteString("- Required in the document:\n")
		for _, pat := range p.Style.RequiredPatterns {
			fmt.Fprintf(&b, "  - %s (severity: %s)\n", patternHint(pat), pat.Severity)
		}
	}
	b.WriteString("\n")

	// Vocabulary
	b.WriteString("## Vocabulary\n")
	if len(p.Vocabulary.PreferredTerms) > 0 {
		b.WriteString("### Preferred Terms\n")
		for _, t := range p.Vocabulary.PreferredTerms {
			b.WriteString(termLine(t) + "\n")
		}
		b.WriteString("\n")
	}
	if len(p.Vocabulary.ForbiddenTerms) > 0 {
		b.WriteString("### Forbidden Terms\n")
		for _, t := range p.Vocabulary.ForbiddenTerms {
			b.WriteString(termLine(t) + "\n")
		}
		b.WriteString("\n")
	}
	if len(p.Vocabulary.CompetitorTerms) > 0 {
		b.WriteString("### Competitor Terms\n")
		for _, t := range p.Vocabulary.CompetitorTerms {
			b.WriteString(termLine(t) + "\n")
		}
		b.WriteString("\n")
	}

	// Examples
	if len(p.Examples) > 0 {
		b.WriteString("## Examples\n")
		for i, ex := range p.Examples {
			fmt.Fprintf(&b, "### Example %d", i+1)
			if ex.Category != "" {
				fmt.Fprintf(&b, " (%s)", ex.Category)
			}
			b.WriteString("\n")
			fmt.Fprintf(&b, "- Before: %q\n", ex.Before)
			fmt.Fprintf(&b, "- After: %q\n", ex.After)
			if ex.Explanation != "" {
				fmt.Fprintf(&b, "- Why: %s\n", ex.Explanation)
			}
			b.WriteString("\n")
		}
	}

	return b.String()
}

// RenderVoiceGuideCompact renders a condensed single-paragraph form of the
// profile's actionable rules, intended for inlining into a translation system
// prompt where the full guide would be too verbose. Every constraint the full
// guide renders is represented — tone (personality, formality, emotion, humor,
// guidelines), style (active voice, sentence length, point of view,
// contractions, prohibited and required patterns), and vocabulary bans (forbidden and
// competitor terms, with or without a replacement) — so no populated profile
// field is silently dead context at generation time. Only the illustrative
// material (preferred terms, examples) is left to the full guide; preferred
// term renderings travel as term rules instead. Output is deterministic.
// maxCompactExamples is how many before/after pairs the compact form carries.
// Three is what vendor guidance suggests is enough to steer, and this form
// exists to be short.
const maxCompactExamples = 3

func RenderVoiceGuideCompact(p *VoiceProfile) string {
	if p == nil {
		return ""
	}
	var parts []string
	if len(p.Tone.Personality) > 0 {
		parts = append(parts, "personality: "+strings.Join(p.Tone.Personality, ", "))
	}
	if p.Tone.Formality != "" {
		parts = append(parts, "formality: "+p.Tone.Formality)
	}
	if p.Tone.Emotion != "" {
		parts = append(parts, "emotion: "+p.Tone.Emotion)
	}
	if p.Tone.Humor != "" {
		parts = append(parts, "humor: "+p.Tone.Humor)
	}
	if p.Style.ActiveVoice {
		parts = append(parts, "use active voice")
	}
	if p.Style.SentenceLength != "" {
		parts = append(parts, "sentence length: "+p.Style.SentenceLength)
	}
	if p.Style.PersonPOV != "" {
		parts = append(parts, "point of view: "+p.Style.PersonPOV)
	}
	if p.Style.Contractions != "" {
		parts = append(parts, "contractions: "+p.Style.Contractions)
	}

	var b strings.Builder
	if len(parts) > 0 {
		fmt.Fprintf(&b, "Voice profile: %s.", strings.Join(parts, "; "))
	}

	if g := strings.TrimSpace(p.Tone.Guidelines); g != "" {
		fmt.Fprintf(&b, " Tone guidance: %s", g)
		if !strings.HasSuffix(g, ".") {
			b.WriteString(".")
		}
	}

	if hints := patternHints(p.Style.ProhibitedPatterns); len(hints) > 0 {
		b.WriteString(" Avoid these patterns: ")
		b.WriteString(strings.Join(hints, "; "))
		b.WriteString(".")
	}

	if hints := patternHints(p.Style.RequiredPatterns); len(hints) > 0 {
		b.WriteString(" The document must carry these: ")
		b.WriteString(strings.Join(hints, "; "))
		b.WriteString(".")
	}

	swaps := termSwaps(p)
	if len(swaps) > 0 {
		b.WriteString(" Never use these terms (use the replacement): ")
		b.WriteString(strings.Join(swaps, "; "))
		b.WriteString(".")
	}

	if bans := termBans(p); len(bans) > 0 {
		b.WriteString(" Never use these terms (rephrase to avoid them): ")
		b.WriteString(strings.Join(bans, "; "))
		b.WriteString(".")
	}

	// The examples, which this used to drop entirely.
	//
	// A before/after pair is the strongest steering a profile carries: describing
	// a register has been measured not to move a model's output, and showing one
	// has. Dropping them left the translation path — the one place kapi writes
	// prose on a user's behalf at scale — with only the description.
	//
	// Capped, because this form exists to be short. The full guide has them all.
	if len(p.Examples) > 0 {
		b.WriteString(" Rewrite in this direction: ")
		var pairs []string
		for i, ex := range p.Examples {
			if i >= maxCompactExamples {
				break
			}
			if ex.Before == "" || ex.After == "" {
				continue
			}
			pairs = append(pairs, fmt.Sprintf("%q becomes %q", ex.Before, ex.After))
		}
		b.WriteString(strings.Join(pairs, "; "))
		b.WriteString(".")
	}
	return strings.TrimSpace(b.String())
}

// termLine renders one rule the way it is meant to be obeyed.
//
// Three shapes, and the middle one is why this exists. A rule whose replacement
// IS its term says "use this word, and here is how" — a convention, not a ban.
// Rendering it as a swap produced, from a profile kapi inferred itself:
//
//   - ~~grep~~ → use **grep**
//   - ~~PCRE2~~ → use **PCRE2**
//
// which instructs a model to avoid a word in favour of that word. The note said
// what the rule actually was ("named plainly and given its own entry; no
// put-downs") and was rendered for preferred terms only, so the one section
// where it carried the whole meaning was the one that dropped it.
//
// The comparison is exact rather than case-folded: `Ripgrep` → `ripgrep` is a
// real rule about capitalisation, and folding would silence it.
func termLine(t TermRule) string {
	var b strings.Builder
	switch {
	case t.Replacement != "" && t.Replacement != t.Term:
		fmt.Fprintf(&b, "- ~~%s~~ → use **%s**", t.Term, t.Replacement)
	default:
		// Either no replacement, or one identical to the term. Neither is a
		// swap, so neither is struck through.
		fmt.Fprintf(&b, "- **%s**", t.Term)
	}
	if t.Note != "" {
		fmt.Fprintf(&b, ": %s", t.Note)
	}
	return b.String()
}

// patternHints returns a pattern list's prompt-facing hints, in declared order.
//
// Both halves, and this is the fix for issue #2240. The description said WHY a
// pattern is banned and the regex says WHAT is banned, and the description won:
// a rule carrying
//
//	regex:       (?i)\b(?:endpoint|payload|webhook|HTTP POST|JSON|HMAC|API)\b
//	description: implementation vocabulary, which this reader does not have
//
// reached the model as "avoid these patterns: implementation vocabulary, which
// this reader does not have", naming none of the seven words. The document it
// then wrote used five of them, each a violation the check would flag. A user
// who wrote a careful description made the guide LESS actionable than one who
// wrote none.
//
// Duplicates are dropped: two patterns sharing a description rendered the same
// sentence twice and spent context on nothing.
func patternHints(pats []Pattern) []string {
	var hints []string
	seen := map[string]bool{}
	for _, pat := range pats {
		hint := patternHint(pat)
		if hint == "" || seen[hint] {
			continue
		}
		seen[hint] = true
		hints = append(hints, hint)
	}
	return hints
}

// patternHint renders one pattern as something a model can act on.
func patternHint(pat Pattern) string {
	desc := strings.TrimSpace(pat.Description)
	words := patternWords(pat.Regex)

	// A rate and a scope change what the rule asks for, so a guide that omitted
	// them would state a stricter or wider rule than the check enforces — the
	// same defect as a pattern arriving without its words (#2240), one field
	// along.
	var qualifiers []string
	if r := pat.Rate; r != nil && r.Max > 0 {
		per := r.Per
		if per <= 0 {
			per = DefaultRateWindow
		}
		qualifiers = append(qualifiers, fmt.Sprintf("at most %d per %d words", r.Max, per))
	}
	switch pat.Scope {
	case ScopeProse:
		qualifiers = append(qualifiers, "in prose, not in code")
	case ScopeCode:
		qualifiers = append(qualifiers, "in code samples only")
	case ScopeHeading:
		qualifiers = append(qualifiers, "in headings only")
	}
	if len(qualifiers) > 0 {
		suffix := ", " + strings.Join(qualifiers, ", ")
		if desc != "" && words != "" {
			return desc + " (" + words + ")" + suffix
		}
		if desc != "" {
			return desc + suffix
		}
		return strings.TrimSpace(pat.Regex) + suffix
	}

	switch {
	case desc != "" && words != "":
		return desc + " (" + words + ")"
	case desc != "":
		// Nothing extractable — an anchor, a character class, a backreference.
		// The description alone is what there is.
		return desc
	default:
		return strings.TrimSpace(pat.Regex)
	}
}

// patternWords pulls the literal words out of a regex, for the common shape a
// vocabulary rule takes: an alternation of plain words.
//
// A regex is poor prompt material and a word list is good prompt material, so
// this renders the list where the pattern is one and says nothing where it is
// not. Only literals survive: a piece carrying any regex syntax is dropped
// rather than shown to a model as though it were a word.
//
// It splits rather than matches. Matching each alternative with its delimiters
// cannot see consecutive ones — in `(a|b|c)` the match for `b` consumes both
// pipes, so `c` has no opening delimiter left and vanishes. That silently
// dropped every other word in a list.
func patternWords(regex string) string {
	if regex == "" {
		return ""
	}
	var words []string
	seen := map[string]bool{}
	for _, piece := range splitTopLevel(regex) {
		w := literalWord(piece)
		if w == "" || seen[strings.ToLower(w)] {
			continue
		}
		seen[strings.ToLower(w)] = true
		words = append(words, w)
	}
	if len(words) == 0 {
		return ""
	}
	if len(words) > maxPatternWords {
		words = append(words[:maxPatternWords:maxPatternWords], "…")
	}
	return "such as " + strings.Join(words, ", ")
}

// splitTopLevel splits a regex on the alternation pipes at its shallowest
// depth, leaving nested ones alone.
//
// Splitting on every pipe reaches inside nested groups: `chang(er|ing)` yielded
// the piece `ing)`, which trimmed to `ing` and was published to a model as a
// word to avoid. A fragment of a word is worse than no word, because a reader
// of the guide cannot tell it is one.
func splitTopLevel(regex string) []string {
	depth, min := 0, -1
	for _, r := range regex {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case '|':
			if min < 0 || depth < min {
				min = depth
			}
		}
	}
	if min < 0 {
		return []string{regex}
	}
	var out []string
	depth, start := 0, 0
	for i, r := range regex {
		switch r {
		case '(':
			depth++
		case ')':
			if depth > 0 {
				depth--
			}
		case '|':
			if depth == min {
				out = append(out, regex[start:i])
				start = i + 1
			}
		}
	}
	return append(out, regex[start:])
}

// literalWord strips the syntax around one alternative and returns what is left
// when that is a plain word or phrase, else "".
func literalWord(piece string) string {
	// Until nothing more comes off: the syntax nests, so `(?i)\b(?:endpoint`
	// needs three passes and one pass each dropped the first and last word of
	// every list.
	for changed := true; changed; {
		changed = false
		for _, prefix := range []string{"(?i)", "(?s)", "(?m)", "(?:", "(", `\b`, "^"} {
			if t := strings.TrimPrefix(strings.TrimSpace(piece), prefix); t != piece {
				piece, changed = t, true
			}
		}
		for _, suffix := range []string{")", `\b`, "$"} {
			if t := strings.TrimSuffix(strings.TrimSpace(piece), suffix); t != piece {
				piece, changed = t, true
			}
		}
	}
	piece = strings.TrimSpace(piece)
	if len(piece) < 3 {
		return ""
	}
	// Anything still carrying syntax is not a word. Checked after trimming, so
	// a genuine word that merely sat inside a group is kept.
	if strings.ContainsAny(piece, `\[](){}*+?.^$|`) {
		return ""
	}
	for i, r := range piece {
		ok := (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') ||
			(r >= '0' && r <= '9') || r == ' ' || r == '-'
		if !ok || (i == 0 && !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z'))) {
			return ""
		}
	}
	return piece
}

// maxPatternWords caps the list. A rule banning fifty words does not need all
// fifty in the prompt to be followed, and the check still holds every one.
const maxPatternWords = 12

// termSwaps returns deterministic "term → replacement" hints derived from
// forbidden and competitor terms that declare a replacement.
func termSwaps(p *VoiceProfile) []string {
	if p == nil {
		return nil
	}
	var swaps []string
	add := func(rules []TermRule) {
		for _, t := range rules {
			// A replacement identical to its term is a convention, not a swap;
			// rendering it as one tells a model to avoid a word in favour of
			// itself. See termLine.
			if t.Replacement != "" && t.Replacement != t.Term {
				swaps = append(swaps, fmt.Sprintf("%q → %q", t.Term, t.Replacement))
			}
		}
	}
	add(p.Vocabulary.ForbiddenTerms)
	add(p.Vocabulary.CompetitorTerms)
	sort.Strings(swaps)
	return swaps
}

// termBans returns deterministic quoted bans derived from forbidden and
// competitor terms that declare NO replacement — the counterpart of termSwaps,
// so a bare ban still reaches the model instead of being dead context.
func termBans(p *VoiceProfile) []string {
	if p == nil {
		return nil
	}
	var bans []string
	add := func(rules []TermRule) {
		for _, t := range rules {
			if t.Term != "" && t.Replacement == "" {
				bans = append(bans, fmt.Sprintf("%q", t.Term))
			}
		}
	}
	add(p.Vocabulary.ForbiddenTerms)
	add(p.Vocabulary.CompetitorTerms)
	sort.Strings(bans)
	return bans
}
