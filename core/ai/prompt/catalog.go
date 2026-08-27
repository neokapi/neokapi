package prompt

import (
	"fmt"
	"slices"
	"strings"

	"github.com/neokapi/neokapi/core/model"
)

// The prompts below live here, rather than inline at each tool's call site, for
// the same reason the translate prompt does: a prompt is an input to a build,
// and the reference documentation is generated from these builders, so the docs
// cannot describe a prompt kapi does not send.
//
// Each renders provider-neutral Turns from typed sections, so --explain-prompts
// attributes every block to what produced it.

// VoiceCheck scores text against a voice profile.
type VoiceCheck struct {
	// VoiceGuide is the rendered voice profile (profile.RenderVoiceGuide).
	VoiceGuide string
}

func (p VoiceCheck) Turns(text string) []Turn {
	system := []Section{{
		Kind:   KindTask,
		Origin: "framework",
		Text: "You are a voice profile compliance checker. Analyze the user's text against the voice profile " +
			"guidelines and report any issues with tone, style, clarity, or voice compliance. " +
			"Return an empty findings array if the text fully complies.",
	}}
	if g := strings.TrimSpace(p.VoiceGuide); g != "" {
		system = append(system, Section{
			Kind:    KindVoice,
			Origin:  "voice profile",
			Heading: "Voice profile guidelines:",
			Text:    g,
		})
	}
	return []Turn{
		System(system...),
		User(Section{Kind: KindContent, Origin: "text to check", Text: text}),
	}
}

// TermForms asks for the surface shapes a term takes in one language.
//
// The alternative was generating them: English suffix rules over the term
// string. That worked for English, produced non-words for Norwegian
// ("løsninges" for "løsning"), reached none of the forms Norwegian actually
// uses, and needed a minimum term length to stop `Go` matching inside "going".
// Morphology is per-language knowledge, and the tools that do it properly carry
// a linguistic pack per language.
//
// So the knowledge is the model's and the timing is authoring, not checking.
// This runs once per rule, its answer is written into the profile where a
// person reviews it in a diff, and the check that consumes it stays exact, free
// and language-neutral. See issue #2226.
type TermForms struct {
	// Language is what the forms are wanted in, as a BCP-47 tag or a name.
	// Without it the model answers in whichever language the term looks like,
	// which for a borrowed word is a guess.
	Language string
	// MaxPerTerm caps the list. A long tail of rare forms costs precision at
	// check time for a violation nobody writes.
	MaxPerTerm int
}

func (p TermForms) Turns(terms string) []Turn {
	var b strings.Builder
	b.WriteString("You are a morphology assistant. For each term below, list the other surface forms it takes ")
	fmt.Fprintf(&b, "in %s: inflections, declensions, conjugations, the shapes the same word appears as in running text.\n\n", p.Language)
	b.WriteString("Rules:\n")
	b.WriteString("- Only forms of the SAME word. Not synonyms, not derivations with a different meaning, not compounds that merely contain it.\n")
	b.WriteString("- Only forms that occur in ordinary writing. Skip archaic and dialectal ones.\n")
	b.WriteString("- Do not repeat the term itself.\n")
	b.WriteString("- A term with no other forms gets an empty list. That is a normal answer, not a failure.\n")
	fmt.Fprintf(&b, "- At most %d forms per term.\n", p.MaxPerTerm)
	b.WriteString("\nThese forms are matched literally and whole-word against content, so a wrong one becomes a false ")
	b.WriteString("accusation against text that broke no rule. Prefer omitting a doubtful form to including it.")

	return []Turn{
		System(Section{Kind: KindTask, Origin: "framework", Text: b.String()}),
		User(Section{Kind: KindContent, Origin: "terms", Text: terms}),
	}
}

// VoiceInfer drafts a voice profile from a corpus of existing content.
type VoiceInfer struct {
	// Domain is an optional subject-domain hint ("medical", "developer tools").
	Domain string
	// MaxExamples caps the before/after pairs requested.
	MaxExamples int
}

func (p VoiceInfer) Turns(corpus string) []Turn {
	var b strings.Builder
	b.WriteString("You are a voice profile analyst. Study the corpus below and infer a draft voice profile. ")
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

// AxisDiscover reads a corpus for the dimensions its content varies along.
//
// The axis set is OPEN. A project's context space is whatever that project
// actually distinguishes — one has product lines, another has regional markets,
// another has audiences — so the prompt teaches by example rather than offering
// a list to choose from. Enumerating axes would cap discovery at what we
// happened to think of, and the whole point of an open map on the wire is that
// it does not need to be.
type AxisDiscover struct {
	// Known are axes the project already declares, so the model proposes VALUES
	// on them rather than proposing the axis again under a synonym.
	Known []string
	// Domain is an optional subject-domain hint ("medical", "developer tools").
	Domain string
	// MaxAxes caps how many axes to report, so a corpus with weak structure
	// yields the few strongest rather than a long tail of noise.
	MaxAxes int
}

func (p AxisDiscover) Turns(corpus string) []Turn {
	var b strings.Builder
	b.WriteString("You are a content strategist. Read the corpus below and report the dimensions along which its content genuinely varies — the coordinates a piece of this content sits at.\n\n")
	b.WriteString("An axis is a dimension; its values are the positions on it. For example, a corpus might vary by:\n")
	b.WriteString("- brand — the brand it speaks as (acme, northwind)\n")
	b.WriteString("- product_line — a family of products (cloud, desktop)\n")
	b.WriteString("- product — one product within a line (analytics, editor)\n")
	b.WriteString("- channel — the surface it ships on (docs, app, marketing)\n")
	b.WriteString("- market — where it is read (emea, japan)\n")
	b.WriteString("- audience — who it addresses (developer, buyer, operator)\n\n")
	b.WriteString("Those are EXAMPLES, not a list to choose from. Name the axes this corpus actually distinguishes, in its own vocabulary; if it varies along something none of the examples covers, report that instead.\n\n")
	b.WriteString("Rules:\n")
	b.WriteString("- Report an axis only when the corpus shows content differing along it. Two documents about the same product are not a product axis.\n")
	b.WriteString("- An axis needs at least two distinct values in the corpus. One value is a fact about the project, not a dimension it varies along.\n")
	b.WriteString("- Use lower_snake_case for axis names and values, and the corpus' own terms for values.\n")
	fmt.Fprintf(&b, "- Report at most %d axes, strongest evidence first.\n", p.MaxAxes)
	b.WriteString("- For each axis give a confidence between 0 and 1 and short quotes or references from the corpus as evidence.\n")
	b.WriteString("- Report nothing rather than guessing. A corpus with no internal variation has no axes, and an empty list is the correct answer.")
	if len(p.Known) > 0 {
		fmt.Fprintf(&b, "\n\nThis project already declares these axes: %s. Propose additional VALUES on them where the corpus shows them, and do not re-propose the same dimension under a different name.", strings.Join(p.Known, ", "))
	}
	if d := strings.TrimSpace(p.Domain); d != "" {
		fmt.Fprintf(&b, "\n\nThe content is in the %s domain.", d)
	}

	system := []Section{{Kind: KindTask, Origin: "framework", Text: b.String()}}

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

// EntityExtract identifies entities and do-not-translate spans across a batch of
// blocks, and proposes terminology candidates.
type EntityExtract struct {
	Locale model.LocaleID
	// KnownTerms are terms already in the terms store; the model is told not to
	// re-propose them.
	KnownTerms []string
}

// EntityBlock is one block submitted for extraction. The ID travels with the
// text so the model can report offsets against the block it came from.
type EntityBlock struct {
	ID   string
	Text string
}

func (p EntityExtract) Turns(blocks []EntityBlock) []Turn {
	system := []Section{{
		Kind:   KindTask,
		Origin: "framework",
		Text: `You are a linguist and terminologist reading source content before it is translated.

Given text blocks, identify:

1. Named entities: people, organizations, products, locations, dates, times, currencies, measurements. For each, indicate whether it should be marked do-not-translate (DNT).
   - Person names: usually DNT unless the project adapts names per language
   - Brand/product names: usually DNT
   - Dates/times/currencies/measurements: usually NOT DNT (they need locale-specific formatting)
   - Locations: context-dependent

2. Terminology candidates: domain-specific terms that should be translated consistently across the project. These are words/phrases that carry specific meaning in this context and would benefit from a terms entry. Exclude common words.
   - "dnt" = never translate (brand names, acronyms that stay in source language)
   - "consistent" = translate, but the same way everywhere
   - "free" = translate naturally, no consistency requirement

Report character offsets relative to each block's text. Only report genuinely useful entities and terms — quality over quantity.`,
	}}

	// Sorted, so the same terms renders the same prompt every run. This used
	// to range over a map, which reordered the prompt on every invocation.
	if len(p.KnownTerms) > 0 {
		terms := slices.Sorted(slices.Values(p.KnownTerms))
		system = append(system, Section{
			Kind:    KindPreferredTerms,
			Origin:  fmt.Sprintf("terms (%s)", plural(len(terms), "known term")),
			Heading: "Existing terms (do not re-propose):",
			Text:    strings.Join(terms, ", "),
		})
	}

	var b strings.Builder
	fmt.Fprintf(&b, "Analyze the following %s of %s source content:\n", plural(len(blocks), "text block"), p.Locale)
	for _, blk := range blocks {
		fmt.Fprintf(&b, "\nBlock (id: %s):\n%q\n", blk.ID, blk.Text)
	}

	return []Turn{
		System(system...),
		User(Section{
			Kind:   KindContent,
			Origin: fmt.Sprintf("source blocks (%d)", len(blocks)),
			Text:   strings.TrimSpace(b.String()),
		}),
	}
}

// Modality is the kind of media a refine prompt re-reads.
type Modality string

const (
	ModalityImage Modality = "image"
	ModalityAudio Modality = "audio"
	ModalityVideo Modality = "video"
)

// MediaRefine asks a multimodal model to re-read a slice of media that OCR or
// ASR transcribed with low confidence. The media itself rides along as a binary
// part on the user turn — see Attachment.
type MediaRefine struct {
	Modality Modality
	// RefuseToken is what the model must return when the slice is illegible, so
	// "I can't read this" is a value rather than a hallucinated guess.
	RefuseToken string
}

// ID returns the prompt ID for this modality.
func (p MediaRefine) ID() string {
	switch p.Modality {
	case ModalityAudio:
		return IDMediaRefineAudio
	case ModalityVideo:
		return IDMediaRefineVideo
	default:
		return IDMediaRefineImage
	}
}

// Attachment describes the binary part the call site appends to the user turn.
// It is not prompt text and is never rendered as such — naming it keeps the
// reference honest about a call that ships more than the words shown.
func (p MediaRefine) Attachment() string {
	switch p.Modality {
	case ModalityAudio:
		return "the speech clip cut from the block's timing span"
	case ModalityVideo:
		return "the video frame at the block's timestamp"
	default:
		return "the image region cropped to the block's bounding box"
	}
}

func (p MediaRefine) task() string {
	switch p.Modality {
	case ModalityAudio:
		return "You transcribe a single short speech clip. " +
			"Return only the exact words spoken, with no commentary. " +
			"If the clip is unintelligible, return " + p.RefuseToken + "."
	case ModalityVideo:
		return "You transcribe the on-screen text in a region of a single video frame. " +
			"Return only the exact text you read, with no commentary. " +
			"If it is unreadable, return " + p.RefuseToken + "."
	default:
		return "You transcribe a single line cropped from a document image. " +
			"Return only the exact text you read, with no commentary. " +
			"If the crop is unreadable, return " + p.RefuseToken + "."
	}
}

// Turns renders the text of the refine prompt. neighbors are the surrounding
// block texts, which give the model the language prior without shipping the
// whole page. The caller appends the media part to the user turn.
func (p MediaRefine) Turns(neighbors []string) []Turn {
	var b strings.Builder
	b.WriteString("Surrounding lines for context:\n")
	for _, n := range neighbors {
		b.WriteString("- ")
		b.WriteString(n)
		b.WriteByte('\n')
	}
	b.WriteString("\nRe-read the highlighted unit and return only its exact text:")

	return []Turn{
		System(Section{Kind: KindTask, Origin: "framework", Text: p.task()}),
		User(Section{
			Kind:   KindContent,
			Origin: "neighbouring blocks",
			Text:   b.String(),
		}),
	}
}

// Segment splits a block into translation-sized chunks with an LLM, for the
// `llm` segmentation engine.
type Segment struct {
	// Language is the source language, when known.
	Language string
	// MaxChunkRunes is a soft size hint; 0 means no hint.
	MaxChunkRunes int
	// Instruction is caller-supplied steering.
	Instruction string
}

func (p Segment) Turns(text string) []Turn {
	var b strings.Builder
	b.WriteString("Split the user's text into coherent, contiguous chunks suitable for translation. ")
	b.WriteString("Prefer sentence or clause boundaries so each chunk stands on its own.")
	if p.Language != "" {
		fmt.Fprintf(&b, " The source language is %s.", p.Language)
	}

	system := []Section{
		{Kind: KindTask, Origin: "framework", Text: b.String()},
		// The verbatim rule is what makes the result usable: segmentation that
		// rewrites the text cannot be mapped back onto the source block.
		{
			Kind:   KindConstraint,
			Origin: "framework",
			Text: "Every chunk must be a verbatim, contiguous slice of the input text; " +
				"concatenating the chunks in order (ignoring leading/trailing whitespace) must reconstruct the input exactly. " +
				"Do not translate, paraphrase, reorder, add, or drop any content.",
		},
		{
			Kind:   KindConstraint,
			Origin: "framework",
			Text:   `Return a JSON object {"chunks": ["...", ...]}.`,
		},
	}
	if p.MaxChunkRunes > 0 {
		system = append(system, Section{
			Kind:   KindConstraint,
			Origin: "segmenter config (max_chunk_runes)",
			Text: fmt.Sprintf(
				"Aim for chunks no longer than about %d characters, but never split mid-word.",
				p.MaxChunkRunes,
			),
		})
	}
	if instr := strings.TrimSpace(p.Instruction); instr != "" {
		system = append(system, Section{
			Kind:    KindInstruction,
			Origin:  "--instruction / recipe",
			Heading: "Additional instruction:",
			Text:    instr,
		})
	}

	return []Turn{
		System(system...),
		User(Section{Kind: KindContent, Origin: "source block", Text: text}),
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
	// Turns is the prompt rendered with representative inputs.
	Turns []Turn `json:"turns,omitempty"`
	// Attachment names a non-text part sent alongside the turns (a cropped
	// image, a speech clip). Empty for a text-only prompt. Listed separately
	// because it is not prompt text and must not be rendered as if it were.
	Attachment string `json:"attachment,omitempty"`
}

// sampleSource is the placeholder content used when rendering a prompt for the
// reference: the docs show the shape of the prompt, not a real document.
const sampleSource = "<your content>"

// RefuseToken is what a media-refine prompt tells the model to return when the
// slice is illegible, so "I cannot read this" is a value the tool can act on
// rather than a hallucinated guess.
//
// It lives here, with the prompt that demands it, because it is half of a
// contract: the tool that compares against it and the prompt that asks for it
// must agree, and a second copy is a second thing to get wrong.
const RefuseToken = "[illegible]"

// Catalog returns every prompt kapi ships, rendered with representative inputs,
// in a stable order. It is the source of the generated prompt reference — so
// rewording a prompt updates the documentation, and the drift gate fails if the
// committed reference is stale.
func Catalog() []CatalogEntry {
	tr := Translate{SourceLocale: "en", TargetLocale: "fr"}
	media := func(m Modality, summary string) CatalogEntry {
		p := MediaRefine{Modality: m, RefuseToken: RefuseToken}
		return CatalogEntry{
			ID:         p.ID(),
			Tool:       "media-refine",
			Summary:    summary,
			Turns:      p.Turns([]string{"<the line above>", "<the line below>"}),
			Attachment: p.Attachment(),
		}
	}

	return []CatalogEntry{
		{
			ID:      IDTranslateSingle,
			Tool:    "translate",
			Summary: "Translate one block. Carries the placeholder rule, plus the inline-tag rule when the block has markup.",
			Turns:   tr.Single(sampleSource, false),
		},
		{
			ID:      IDTranslateBatch,
			Tool:    "translate",
			Summary: "Translate several blocks in one call, numbered so the structured reply maps back by index.",
			Turns:   tr.Batch(BatchSegments([]string{"<block 1>", "<block 2>"})),
		},
		{
			ID:      IDVoiceCheck,
			Tool:    "voice-check",
			Summary: "Score text against a voice profile and report tone, style and compliance issues.",
			Turns:   VoiceCheck{VoiceGuide: "<your voice profile>"}.Turns(sampleSource),
		},
		{
			ID:      IDVoiceInfer,
			Tool:    "voice-infer",
			Summary: "Draft a voice profile from a corpus of your existing content.",
			Turns:   VoiceInfer{MaxExamples: 3}.Turns("<your content corpus>"),
		},
		{
			ID:      IDTermForms,
			Tool:    "voice-expand",
			Summary: "List the other surface forms a term takes in one language, so the check can match them without guessing at morphology.",
			Turns:   TermForms{Language: "nb", MaxPerTerm: 8}.Turns("løsning\nutnytte"),
		},
		{
			ID:      IDAxisDiscover,
			Tool:    "context-scan",
			Summary: "Discover the dimensions a corpus of content varies along.",
			Turns:   AxisDiscover{MaxAxes: 4}.Turns("<your content corpus>"),
		},
		{
			ID:      IDQualityCheck,
			Tool:    "qa (AI mode)",
			Summary: "Find quality issues in a finished translation.",
			Turns:   QualityCheck{SourceLocale: "en", TargetLocale: "fr", Checks: []string{"terminology", "fluency", "accuracy"}}.Turns(sampleSource, "<the translation>"),
		},
		{
			ID:      IDReview,
			Tool:    "review",
			Summary: "Score a translation 0-100 and report findings by severity.",
			Turns:   Review{SourceLocale: "en", TargetLocale: "fr"}.Turns(sampleSource, "<the translation>"),
		},
		{
			ID:      IDTermExtract,
			Tool:    "term-extract",
			Summary: "Propose terminology candidates from source content.",
			Turns:   TermExtract{Locale: "en"}.Turns(sampleSource),
		},
		{
			ID:      IDEntityExtract,
			Tool:    "entity-extract",
			Summary: "Identify entities and do-not-translate spans, and classify terminology candidates.",
			Turns: EntityExtract{Locale: "en", KnownTerms: []string{"<a term you already have>"}}.
				Turns([]EntityBlock{{ID: "<block id>", Text: sampleSource}}),
		},
		media(ModalityImage, "Re-read a cropped image line that OCR read with low confidence."),
		media(ModalityAudio, "Re-listen to a speech clip that ASR transcribed with low confidence."),
		media(ModalityVideo, "Re-read on-screen text in a video frame that OCR read with low confidence."),
		{
			ID:      IDSegment,
			Tool:    "segment (llm engine)",
			Summary: "Split text into segments, reproducing the source verbatim.",
			Turns:   Segment{Language: "en"}.Turns(sampleSource),
		},
	}
}
