package prompt

import (
	"fmt"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// The prompts below were previously written inline at each tool's call site.
// They live here for the same reason the translate prompt does: a prompt is an
// input to a build, and the reference documentation is generated from these
// builders, so the docs cannot describe a prompt kapi does not send.
//
// Each renders provider-neutral Turns from typed sections, so --explain-prompts
// attributes every block to what produced it.

// BrandCheck scores text against a brand voice profile.
type BrandCheck struct {
	// VoiceGuide is the rendered voice profile (brand.RenderVoiceGuide).
	VoiceGuide string
}

func (p BrandCheck) Turns(text string) []Turn {
	system := []Section{{
		Kind:   KindTask,
		Origin: "framework",
		Text: "You are a brand voice compliance checker. Analyze the user's text against the brand voice " +
			"guidelines and report any issues with tone, style, clarity, or brand compliance. " +
			"Return an empty findings array if the text fully complies.",
	}}
	if g := strings.TrimSpace(p.VoiceGuide); g != "" {
		system = append(system, Section{
			Kind:    KindVoice,
			Origin:  "brand voice profile",
			Heading: "Brand voice guidelines:",
			Text:    g,
		})
	}
	return []Turn{
		System(system...),
		User(Section{Kind: KindContent, Origin: "text to check", Text: text}),
	}
}

// BrandInfer drafts a brand voice profile from a corpus of existing content.
type BrandInfer struct {
	// Domain is an optional subject-domain hint ("medical", "developer tools").
	Domain string
	// MaxExamples caps the before/after pairs requested.
	MaxExamples int
}

func (p BrandInfer) Turns(corpus string) []Turn {
	var b strings.Builder
	b.WriteString("You are a brand voice analyst. Study the corpus below and infer a draft brand voice profile. ")
	b.WriteString("Ground every rule in evidence from the text; do not invent rules the corpus does not support.\n\n")
	b.WriteString("Report:\n")
	b.WriteString("- tone: personality traits, formality (casual|neutral|formal|technical), emotion, humor (none|light|frequent), and short guidelines\n")
	b.WriteString("- style: active_voice, sentence_length (short|medium|varied), person_pov (first_plural|second|third), contractions (always|sometimes|never), and any patterns the corpus consistently avoids (as prohibited patterns)\n")
	b.WriteString("- vocabulary: preferred, forbidden, and competitor terms (term, replacement, note); leave lists empty when the corpus shows no evidence\n")
	fmt.Fprintf(&b, "- examples: up to %d before/after pairs (before = off-voice, after = on-voice, with an explanation)\n", p.MaxExamples)
	b.WriteString("- evidence: for each of tone, style, vocabulary, and examples, a confidence between 0 and 1 and a short source note citing the corpus evidence")
	if d := strings.TrimSpace(p.Domain); d != "" {
		fmt.Fprintf(&b, "\n\nThe content is in the %s domain.", d)
	}

	system := []Section{{Kind: KindTask, Origin: "framework", Text: b.String()}}

	// The corpus follows CorpusDelimiter, which offline providers rely on to
	// locate it — a declared part of the contract, not incidental wording.
	return []Turn{
		System(system...),
		User(Section{
			Kind:   KindContent,
			Origin: "content corpus",
			Text:   strings.TrimPrefix(CorpusDelimiter, "\n") + corpus,
		}),
	}
}

// QualityCheck asks the model to find issues in a finished translation.
type QualityCheck struct {
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
	// Checks names the issue classes to look for (terminology, fluency, …).
	Checks []string
}

func (p QualityCheck) Turns(source, target string) []Turn {
	return []Turn{
		System(Section{
			Kind:   KindTask,
			Origin: "framework",
			Text: fmt.Sprintf(
				"You are a translation quality reviewer. Analyze the user's translation for quality issues. "+
					"Check for: %s. Return all issues found, or an empty array if none.",
				strings.Join(p.Checks, ", "),
			),
		}),
		User(Section{
			Kind:   KindContent,
			Origin: "source and translation",
			Text: fmt.Sprintf("Source (%s): %s\nTranslation (%s): %s",
				p.SourceLocale, source, p.TargetLocale, target),
		}),
	}
}

// Review is the LLM judge: it scores a translation and reports findings.
type Review struct {
	SourceLocale model.LocaleID
	TargetLocale model.LocaleID
}

func (p Review) Turns(source, target string) []Turn {
	return []Turn{
		System(Section{
			Kind:   KindTask,
			Origin: "framework",
			Text: "You are a translation reviewer. Review the user's translation for accuracy and fluency.\n\n" +
				"Respond with ONLY a JSON object in this exact shape, no other text:\n" +
				`{"score": <overall quality 0-100>, "findings": [{"severity": "critical|major|minor|info", "message": "<issue>", "suggestion": "<improved translation or fix, optional>"}]}` +
				"\nReturn an empty findings array when the translation has no issues.",
		}),
		User(Section{
			Kind:   KindContent,
			Origin: "source and translation",
			Text: fmt.Sprintf("Source (%s): %s\nTranslation (%s): %s",
				p.SourceLocale, source, p.TargetLocale, target),
		}),
	}
}

// TermExtract proposes terminology candidates from source content.
type TermExtract struct {
	Locale model.LocaleID
	// Domain is an optional subject-domain hint.
	Domain string
}

func (p TermExtract) Turns(text string) []Turn {
	var domainHint string
	if d := strings.TrimSpace(p.Domain); d != "" {
		domainHint = fmt.Sprintf(" in the %s domain", d)
	}
	return []Turn{
		System(Section{
			Kind:   KindTask,
			Origin: "framework",
			Text: fmt.Sprintf(
				"You are a terminologist. Extract key terminology%s from the user's %s text. "+
					"Return notable terms, or an empty array if none found.",
				domainHint, p.Locale,
			),
		}),
		User(Section{Kind: KindContent, Origin: "source text", Text: text}),
	}
}

// CatalogEntry describes one prompt kapi ships. The prompt reference is
// generated from Catalog(), so a prompt appears in the docs by existing.
type CatalogEntry struct {
	ID string `json:"id"`
	// Tool is the tool or command that sends this prompt.
	Tool string `json:"tool"`
	// Summary says what the prompt asks the model to do.
	Summary string `json:"summary"`
	// Structured is false for a prompt still built inline at its call site,
	// whose text this catalog therefore cannot render. Reported honestly rather
	// than described from memory.
	Structured bool `json:"structured"`
	// Turns is the prompt rendered with representative inputs. Empty when
	// Structured is false.
	Turns []Turn `json:"turns,omitempty"`
}

// sampleSource is the placeholder content used when rendering a prompt for the
// reference: the docs show the shape of the prompt, not a real document.
const sampleSource = "<your content>"

// Catalog returns every prompt kapi ships, rendered with representative inputs,
// in a stable order. It is the source of the generated prompt reference — so
// rewording a prompt updates the documentation, and the drift gate fails if the
// committed reference is stale.
func Catalog() []CatalogEntry {
	tr := Translate{SourceLocale: "en", TargetLocale: "fr"}

	return []CatalogEntry{
		{
			ID:         IDTranslateSingle,
			Tool:       "translate",
			Summary:    "Translate one block. Carries the placeholder rule, plus the inline-tag rule when the block has markup.",
			Structured: true,
			Turns:      tr.Single(sampleSource, false),
		},
		{
			ID:         IDTranslateBatch,
			Tool:       "translate",
			Summary:    "Translate several blocks in one call, numbered so the structured reply maps back by index.",
			Structured: true,
			Turns:      tr.Batch([]string{"<block 1>", "<block 2>"}),
		},
		{
			ID:         IDBrandCheck,
			Tool:       "brand-voice-check",
			Summary:    "Score text against a brand voice profile and report tone, style and compliance issues.",
			Structured: true,
			Turns:      BrandCheck{VoiceGuide: "<your brand voice profile>"}.Turns(sampleSource),
		},
		{
			ID:         IDBrandInfer,
			Tool:       "brand-voice-infer",
			Summary:    "Draft a brand voice profile from a corpus of your existing content.",
			Structured: true,
			Turns:      BrandInfer{MaxExamples: 3}.Turns("<your content corpus>"),
		},
		{
			ID:         IDQualityCheck,
			Tool:       "qa (AI mode)",
			Summary:    "Find quality issues in a finished translation.",
			Structured: true,
			Turns:      QualityCheck{SourceLocale: "en", TargetLocale: "fr", Checks: []string{"terminology", "fluency", "accuracy"}}.Turns(sampleSource, "<the translation>"),
		},
		{
			ID:         IDReview,
			Tool:       "review",
			Summary:    "Score a translation 0-100 and report findings by severity.",
			Structured: true,
			Turns:      Review{SourceLocale: "en", TargetLocale: "fr"}.Turns(sampleSource, "<the translation>"),
		},
		{
			ID:         IDTermExtract,
			Tool:       "term-extract",
			Summary:    "Propose terminology candidates from source content.",
			Structured: true,
			Turns:      TermExtract{Locale: "en"}.Turns(sampleSource),
		},
		{
			ID:         IDEntityExtract,
			Tool:       "entity-extract",
			Summary:    "Identify entities and do-not-translate spans, and classify terminology candidates.",
			Structured: false,
		},
		{
			ID:         IDMediaRefine,
			Tool:       "media-refine",
			Summary:    "Re-read a cropped image line, speech clip or video frame that OCR or ASR got wrong.",
			Structured: false,
		},
		{
			ID:         IDSegment,
			Tool:       "segment (llm engine)",
			Summary:    "Split text into segments, reproducing the source verbatim.",
			Structured: false,
		},
	}
}
