package brand

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

// RenderVoiceGuideCompact renders a condensed single-paragraph form of the most
// actionable rules (personality, formality, forbidden/competitor term swaps).
// It is intended for inlining into a translation system prompt where the full
// guide would be too verbose. Output is deterministic.
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
	if p.Style.ActiveVoice {
		parts = append(parts, "use active voice")
	}
	if p.Style.Contractions != "" {
		parts = append(parts, "contractions: "+p.Style.Contractions)
	}

	var b strings.Builder
	if len(parts) > 0 {
		fmt.Fprintf(&b, "Brand voice — %s.", strings.Join(parts, "; "))
	}

	swaps := termSwaps(p)
	if len(swaps) > 0 {
		b.WriteString(" Never use these terms (use the replacement): ")
		b.WriteString(strings.Join(swaps, "; "))
		b.WriteString(".")
	}
	return strings.TrimSpace(b.String())
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
