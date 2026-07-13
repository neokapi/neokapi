package prompt

import (
	"crypto/sha256"
	"encoding/hex"
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

// steering returns the project-owned sections — the ones that make output yours
// rather than merely correct. Every translate prompt carries them, so guidance
// applies identically whether a block goes through the single or batch path.
//
// Glossary terms are sorted, so the same inputs always render byte-identical
// prompt text. That determinism is load-bearing: the rendered prompt feeds the
// translate config fingerprint, and a prompt that reordered itself between runs
// would invalidate the cache on every run.
func (t Translate) steering() []Section {
	var out []Section

	if ins := strings.TrimSpace(t.Instruction); ins != "" {
		out = append(out, Section{
			Kind:    KindInstruction,
			Origin:  "--instruction / recipe",
			Heading: "Instruction (apply when translating):",
			Text:    ins,
		})
	}
	if g := strings.TrimSpace(t.VoiceGuide); g != "" {
		out = append(out, Section{
			Kind:    KindVoice,
			Origin:  "brand voice profile",
			Heading: "Brand voice (apply when translating):",
			Text:    g,
		})
	}
	if len(t.Glossary) > 0 {
		var b strings.Builder
		for _, k := range slices.Sorted(maps.Keys(t.Glossary)) {
			fmt.Fprintf(&b, "- %s → %s\n", k, t.Glossary[k])
		}
		out = append(out, Section{
			Kind:    KindGlossary,
			Origin:  fmt.Sprintf("termbase (%s)", plural(len(t.Glossary), "term")),
			Heading: "Glossary:",
			Text:    strings.TrimRight(b.String(), "\n"),
		})
	}
	return out
}

// Directives renders the steering sections as they appear in the prompt. Kept
// for callers that want the block on its own.
func (t Translate) Directives() string { return renderSections(t.steering()) }

// Fingerprint hashes every instruction this builder can send — the task, the
// constraints, and the steering — across all three prompt shapes. The translate
// tool folds it into its config fingerprint, so a *changed prompt* invalidates
// cached targets automatically.
//
// This is deliberately derived from the rendered text rather than from a
// hand-maintained version constant: a constant is a step someone forgets, and
// forgetting it means silently serving a translation produced by a prompt that
// no longer exists. Reword anything above and the hash moves on its own.
//
// The source text is excluded — it varies per block and is fingerprinted by the
// block's own content key.
func (t Translate) Fingerprint() string {
	var b strings.Builder
	for _, turns := range [][]Turn{
		t.Single("", false),
		t.Single("", true),
		t.Batch(nil),
	} {
		for _, turn := range turns {
			b.WriteString(turn.Role)
			b.WriteByte('\x00')
			b.WriteString(turn.Text)
			b.WriteByte('\x00')
		}
	}
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:8])
}

// constraints returns the framework-owned rules that make output usable at all:
// a translation that drops a placeholder or mangles an inline tag cannot be
// written back to the source format. They are not a configuration surface.
func (t Translate) constraints(preserveTags bool) []Section {
	out := []Section{{
		Kind:   KindConstraint,
		Origin: "framework",
		Text:   "Preserve placeholders such as {0}, %s and {{name}} exactly.",
	}}
	if preserveTags {
		out = append(out, Section{
			Kind:   KindConstraint,
			Origin: "framework (block has inline codes)",
			Text: "The text contains XML tags. Reproduce every tag exactly as it appears — " +
				"do not modify, reorder, add or remove any tag — and place each one where it " +
				"belongs in the target language.",
		})
	}
	return out
}

// task states the job for a single-block translation.
func (t Translate) task() Section {
	return Section{
		Kind:   KindTask,
		Origin: "framework",
		Text: fmt.Sprintf(
			"You are a software localization specialist. Translate the user's text from %s to %s. "+
				"Return ONLY the translation, with no explanation, preamble or quoting.",
			t.SourceLocale, t.TargetLocale,
		),
	}
}

// Single renders the prompt for one block. preserveTags marks a block whose
// source carries inline codes, rendered as placeholder-tagged text.
//
// The text to translate is the entire user turn: it is data, never instruction.
// (The inline-code path used to build a full instruction and pass it as the
// *source text* of another instruction, so the model was handed a prompt to
// translate rather than content.)
func (t Translate) Single(source string, preserveTags bool) []Turn {
	system := []Section{t.task()}
	system = append(system, t.constraints(preserveTags)...)
	system = append(system, t.steering()...)

	return []Turn{
		System(system...),
		User(Section{Kind: KindContent, Origin: "source block", Text: source}),
	}
}

// Batch renders the prompt for several blocks in one call. Segments are numbered
// so the structured response maps back by index rather than by parsing free text.
func (t Translate) Batch(texts []string) []Turn {
	system := []Section{{
		Kind:   KindTask,
		Origin: "framework",
		Text: fmt.Sprintf(
			"You are a software localization specialist. Translate each numbered segment from %s to %s. "+
				"Return a translation for every segment, keyed by its number.",
			t.SourceLocale, t.TargetLocale,
		),
	}}
	system = append(system, t.constraints(true)...)
	system = append(system, t.steering()...)

	var b strings.Builder
	for i, text := range texts {
		fmt.Fprintf(&b, "[%d] %s\n", i+1, text)
	}

	return []Turn{
		System(system...),
		User(Section{
			Kind:   KindContent,
			Origin: fmt.Sprintf("source blocks (%d)", len(texts)),
			Text:   strings.TrimRight(b.String(), "\n"),
		}),
	}
}
