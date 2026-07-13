package prompt

import (
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// Translate renders the prompts for the translate tool. One builder serves
// every path — single block, block with inline codes, and batch — so they
// cannot drift apart.
type Translate struct {
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID

	// Instruction is a caller-supplied directive applied while translating —
	// a reviewer's "keep it informal", or the findings a fix pass must resolve.
	Instruction string
	// VoiceGuide is brand voice guidance rendered from a VoiceProfile, so output
	// is on-brand at generation time rather than only corrected afterwards.
	VoiceGuide string
	// Glossary pins the translation of specific terms.
	Glossary map[string]string
}

// Directives renders the instruction, brand-voice and glossary blocks shared by
// every translation prompt. Glossary terms are sorted, so the same inputs always
// render byte-identical prompt text. Returns "" when nothing is set.
func (t Translate) Directives() string {
	var b strings.Builder
	if ins := strings.TrimSpace(t.Instruction); ins != "" {
		b.WriteString("\n\nInstruction (apply when translating):\n")
		b.WriteString(ins)
		b.WriteString("\n")
	}
	if g := strings.TrimSpace(t.VoiceGuide); g != "" {
		b.WriteString("\n\nBrand voice (apply when translating):\n")
		b.WriteString(g)
		b.WriteString("\n")
	}
	if len(t.Glossary) > 0 {
		b.WriteString("\n\nGlossary:\n")
		for _, k := range slices.Sorted(maps.Keys(t.Glossary)) {
			fmt.Fprintf(&b, "- %s → %s\n", k, t.Glossary[k])
		}
	}
	return b.String()
}

// system renders the instruction turn shared by the single and inline-code
// paths. Task framing belongs in the system turn; only the content to translate
// belongs in the user turn.
func (t Translate) system(preserveTags bool) string {
	var b strings.Builder
	b.WriteString("You are a software localization specialist. ")
	fmt.Fprintf(&b, "Translate the user's text from %s to %s. ", t.SourceLocale, t.TargetLocale)
	b.WriteString("Return ONLY the translation, with no explanation, preamble or quoting. ")
	if preserveTags {
		b.WriteString("The text contains XML tags. Reproduce every tag exactly as it appears — do not modify, reorder, add or remove any tag — and place each one where it belongs in the target language. ")
	}
	b.WriteString("Preserve placeholders such as {0}, %s and {{name}} exactly.")
	b.WriteString(t.Directives())
	return b.String()
}

// Single renders the prompt for one block. preserveTags marks a block whose
// source carries inline codes, rendered as placeholder-tagged text.
//
// The text to translate is the entire user turn: it is data, never instruction.
// (The inline-code path used to build a full instruction and pass it as the
// *source text* of another instruction, so the model was handed a prompt to
// translate rather than content.)
func (t Translate) Single(source string, preserveTags bool) []Turn {
	return []Turn{
		System(t.system(preserveTags)),
		User(source),
	}
}

// Batch renders the prompt for several blocks in one call. Segments are numbered
// so the structured response maps back by index rather than by parsing free text.
func (t Translate) Batch(texts []string) []Turn {
	var sys strings.Builder
	sys.WriteString("You are a software localization specialist. ")
	fmt.Fprintf(&sys, "Translate each numbered segment from %s to %s. ", t.SourceLocale, t.TargetLocale)
	sys.WriteString("Reproduce any XML/HTML tags exactly as they appear. ")
	sys.WriteString("Preserve placeholders such as {0}, %s and {{name}} exactly. ")
	sys.WriteString("Return a translation for every segment, keyed by its number.")
	sys.WriteString(t.Directives())

	var user strings.Builder
	for i, text := range texts {
		fmt.Fprintf(&user, "[%d] %s\n", i+1, text)
	}

	return []Turn{
		System(sys.String()),
		User(strings.TrimSuffix(user.String(), "\n")),
	}
}
