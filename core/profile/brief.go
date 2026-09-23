package profile

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"
)

// RenderVoiceBrief renders a profile as the short brief a writer reads before
// changing a file: the name and description, the tone and style fields the
// profile sets as one sentence, then the patterns, comment limits, examples
// and shared constraints it carries. Unset fields render nothing.
//
// Vocabulary is left out. A caller that shows a brief shows the word rules
// beside it in one list, merged with the terms in force at the same point, so
// the same rule is never stated twice. RenderVoiceGuide is the full guide.
func RenderVoiceBrief(p *VoiceProfile) string {
	if p == nil {
		return ""
	}
	var lead []string
	lead = append(lead, "Voice: "+sentence(p.Name))
	if d := strings.TrimSpace(p.Description); d != "" {
		lead = append(lead, sentence(d))
	}
	if s := styleSentence(p); s != "" {
		lead = append(lead, s)
	}
	if g := strings.TrimSpace(p.Tone.Guidelines); g != "" {
		lead = append(lead, sentence(g))
	}

	var b strings.Builder
	b.WriteString(strings.Join(lead, " "))
	b.WriteString("\n")

	briefList(&b, "Avoid:", patternHints(p.Style.ProhibitedPatterns))
	briefList(&b, "Every document carries:", patternHints(p.Style.RequiredPatterns))
	if p.Style.Comments != nil {
		l := p.Style.Comments.Limits()
		fmt.Fprintf(&b, "\nCode comments: sentences up to %d words; a comment up to %d words, a doc comment up to %d, "+
			"a package doc comment up to %d; a change adding %d or more comment lines keeps to %s comment lines for each code line. "+
			"Code spans, references and links are not counted.\n",
			l.SentenceMinor, l.CommentWords, l.DocWords, l.PackageDocWords,
			l.DensityMinLines, strconv.FormatFloat(l.DensityRatio, 'f', -1, 64))
	}
	var pairs []string
	for _, ex := range p.Examples {
		if len(pairs) == maxCompactExamples {
			break
		}
		if ex.Before == "" || ex.After == "" {
			continue
		}
		pairs = append(pairs, fmt.Sprintf("%q becomes %q", ex.Before, ex.After))
	}
	briefList(&b, "Rewrite in this direction:", pairs)
	if guidance := constraintGuide(p); guidance != "" {
		fmt.Fprintf(&b, "\nShared constraints:\n%s\n", guidance)
	}
	return b.String()
}

// briefList writes a labelled list, or nothing when it is empty.
func briefList(b *strings.Builder, label string, items []string) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintf(b, "\n%s\n", label)
	for _, it := range items {
		fmt.Fprintf(b, "- %s\n", it)
	}
}

// styleSentence renders the tone and style fields a profile sets as one
// sentence, such as "Neutral register, short sentences, second person." It is
// empty when the profile sets none of them.
func styleSentence(p *VoiceProfile) string {
	var parts []string
	if len(p.Tone.Personality) > 0 {
		parts = append(parts, strings.Join(p.Tone.Personality, ", "))
	}
	if v := strings.TrimSpace(p.Tone.Formality); v != "" {
		parts = append(parts, v+" register")
	}
	if v := strings.TrimSpace(p.Tone.Emotion); v != "" {
		parts = append(parts, v+" tone")
	}
	switch v := strings.TrimSpace(p.Tone.Humor); v {
	case "":
	case "none":
		parts = append(parts, "no humor")
	default:
		parts = append(parts, v+" humor")
	}
	if p.Style.ActiveVoice {
		parts = append(parts, "active voice")
	}
	switch v := strings.TrimSpace(p.Style.SentenceLength); v {
	case "":
	case "varied":
		parts = append(parts, "varied sentence length")
	default:
		parts = append(parts, v+" sentences")
	}
	switch v := strings.TrimSpace(p.Style.PersonPOV); v {
	case "":
	case "first_plural":
		parts = append(parts, "first person plural (we)")
	case "second":
		parts = append(parts, "second person (you)")
	case "third":
		parts = append(parts, "third person")
	default:
		parts = append(parts, "point of view: "+v)
	}
	switch v := strings.TrimSpace(p.Style.Contractions); v {
	case "":
	case "always":
		parts = append(parts, "contractions")
	case "sometimes":
		parts = append(parts, "contractions where natural")
	case "never":
		parts = append(parts, "no contractions")
	default:
		parts = append(parts, "contractions: "+v)
	}
	if len(parts) == 0 {
		return ""
	}
	s := strings.Join(parts, ", ")
	r, size := utf8.DecodeRuneInString(s)
	return sentence(string(unicode.ToUpper(r)) + s[size:])
}

// sentence ends s with a full stop unless it already ends a sentence.
func sentence(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.HasSuffix(s, ".") || strings.HasSuffix(s, "!") || strings.HasSuffix(s, "?") {
		return s
	}
	return s + "."
}
