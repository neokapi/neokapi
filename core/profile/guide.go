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
	if len(p.Style.ProhibitedPatterns) > 0 {
		b.WriteString("- Prohibited patterns:\n")
		for _, pat := range p.Style.ProhibitedPatterns {
			fmt.Fprintf(&b, "  - %s (severity: %s)\n", pat.Description, pat.Severity)
		}
	}
	if len(p.Style.RequiredPatterns) > 0 {
		b.WriteString("- Required in the document:\n")
		for _, pat := range p.Style.RequiredPatterns {
			fmt.Fprintf(&b, "  - %s (severity: %s)\n", pat.Description, pat.Severity)
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
		fmt.Fprintf(&b, "Voice profile — %s.", strings.Join(parts, "; "))
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

// patternHints returns a pattern list's prompt-facing hints in its declared
// order: the human description where present, else the raw regex — a pattern
// must not vanish from the prompt just because nobody described it.
func patternHints(pats []Pattern) []string {
	var hints []string
	for _, pat := range pats {
		hint := strings.TrimSpace(pat.Description)
		if hint == "" {
			hint = pat.Regex
		}
		if hint != "" {
			hints = append(hints, hint)
		}
	}
	return hints
}

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
