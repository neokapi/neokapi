package profile

import (
	"fmt"
	"sort"
	"strings"
)

// RenderVoiceGuide produces a markdown-formatted brand voice guide optimized for
// LLM consumption. It is the single source of truth for turning a VoiceProfile
// into prompt text — used by the AI translate prompt (so generation is on-brand),
// the brand-voice check tool, the local `kapi brand guide` command, and the
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

	fmt.Fprintf(&b, "# Brand Voice Guide: %s\n\n", p.Name)
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
	b.WriteString("\n")

	// Vocabulary
	b.WriteString("## Vocabulary\n")
	if len(p.Vocabulary.PreferredTerms) > 0 {
		b.WriteString("### Preferred Terms\n")
		for _, t := range p.Vocabulary.PreferredTerms {
			if t.Note != "" {
				fmt.Fprintf(&b, "- **%s**: %s\n", t.Term, t.Note)
			} else {
				fmt.Fprintf(&b, "- **%s**\n", t.Term)
			}
		}
		b.WriteString("\n")
	}
	if len(p.Vocabulary.ForbiddenTerms) > 0 {
		b.WriteString("### Forbidden Terms\n")
		for _, t := range p.Vocabulary.ForbiddenTerms {
			line := fmt.Sprintf("- ~~%s~~", t.Term)
			if t.Replacement != "" {
				line += fmt.Sprintf(" → use **%s**", t.Replacement)
			}
			b.WriteString(line + "\n")
		}
		b.WriteString("\n")
	}
	if len(p.Vocabulary.CompetitorTerms) > 0 {
		b.WriteString("### Competitor Terms (avoid)\n")
		for _, t := range p.Vocabulary.CompetitorTerms {
			line := fmt.Sprintf("- ~~%s~~", t.Term)
			if t.Replacement != "" {
				line += fmt.Sprintf(" → use **%s**", t.Replacement)
			}
			b.WriteString(line + "\n")
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
// contractions, prohibited patterns), and vocabulary bans (forbidden and
// competitor terms, with or without a replacement) — so no populated profile
// field is silently dead context at generation time. Only the illustrative
// material (preferred terms, examples) is left to the full guide; preferred
// term renderings travel via the glossary instead. Output is deterministic.
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
		fmt.Fprintf(&b, "Brand voice — %s.", strings.Join(parts, "; "))
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

// patternHints returns the prohibited patterns' prompt-facing hints in their
// declared order: the human description where present, else the raw regex — a
// pattern must not vanish from the prompt just because nobody described it.
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
			if t.Replacement != "" {
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
