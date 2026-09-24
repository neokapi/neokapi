package prompt

import (
	"fmt"
	"strings"
)

// Kind classifies prompt sections by purpose and source.
//
//   - The framework supplies Task and Constraint, including output structure
//     and placeholder preservation. These are fixed correctness requirements.
//   - The project supplies Instruction, Voice and Terminology through recipe
//     settings, voice profiles and terms.
//   - The document supplies Content, which is input data rather than instructions.
//
// Keeping sections distinct supports source attribution in --explain and the
// prompt reference documentation.
type Kind string

const (
	// KindTask states the job. Framework-owned.
	KindTask Kind = "task"
	// KindConstraint states the rules output must satisfy to be usable —
	// placeholder and inline-tag fidelity. Framework-owned.
	KindConstraint Kind = "constraint"
	// KindInstruction is the caller's steering (--instruction, recipe).
	KindInstruction Kind = "instruction"
	// KindVoice is voice profile guidance rendered from a VoiceProfile.
	KindVoice Kind = "voice"
	// KindPreferredTerms pins terminology from the terms store.
	KindPreferredTerms Kind = "preferred_terms"
	// KindDoNotTranslate names the terms that must survive verbatim: the
	// concepts a terms store marks do-not-translate, and the strings a recipe
	// names. Distinct from KindPreferredTerms, which pairs a term with the
	// wording to use instead.
	KindDoNotTranslate Kind = "do_not_translate"
	// KindContent is the text to act on. Data, never instruction.
	KindContent Kind = "content"
	// KindContext is reference material about the block — its key, its
	// neighbours. Sent so the model knows what the text *is*; never translated,
	// never echoed. Distinct from KindContent, which is the text to translate.
	KindContext Kind = "context"
)

// Section is one addressable piece of a prompt.
type Section struct {
	Kind Kind
	// Origin says where the section came from, in the user's terms — "framework",
	// "--instruction", "voice profile", "terms (12 terms)". It is shown
	// by --explain so a prompt can be traced back to the thing that produced it.
	Origin string
	// Heading, when set, is rendered above Text so the model sees a labelled
	// block rather than run-on prose.
	Heading string
	Text    string
}

// Render returns the section's text as it appears in the prompt.
func (s Section) Render() string {
	if s.Heading == "" {
		return s.Text
	}
	return s.Heading + "\n" + s.Text
}

// renderSections joins sections into one turn's text. Blank sections are
// dropped, so an unconfigured run ships no stray headings.
func renderSections(sections []Section) string {
	parts := make([]string, 0, len(sections))
	for _, s := range sections {
		if strings.TrimSpace(s.Text) == "" {
			continue
		}
		parts = append(parts, s.Render())
	}
	return strings.Join(parts, "\n\n")
}

// plural renders a count with its noun, pluralized. Section origins are shown to
// users by --explain, so "1 term" must not read "1 terms".
func plural(n int, noun string) string {
	if n == 1 {
		return "1 " + noun
	}
	return fmt.Sprintf("%d %ss", n, noun)
}
