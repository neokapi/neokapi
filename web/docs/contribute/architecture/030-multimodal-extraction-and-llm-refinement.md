---
id: 030-multimodal-extraction-and-llm-refinement
sidebar_position: 30
title: "AD-030: Multimodal Extraction and LLM Refinement"
description: "Architecture decision: extracting translatable content from images, audio, and video is one pattern — a fast local extractor produces confidence-scored Blocks anchored to a slice of the source media, and a configurable multimodal LLM refines only the low-confidence units. The anchor facets and source provenance are the content model's (AD-002); the multimodal message is the provider's (AD-011); this AD adds the confidence-gated escalation pattern, the generic media-refine Transform, and the kapi-asr/video plugin symmetry."
keywords: [multimodal, OCR, ASR, speech recognition, video localization, audio localization, LLM refinement, confidence cascade, media-refine, MediaSlicer, vision LLM, Whisper, kapi-vision, kapi-asr, architecture decision, neokapi]
---

# AD-030: Multimodal Extraction and LLM Refinement

## Summary

Extracting translatable content from a **non-text medium** — text rendered into
an image, speech in an audio track, captions and on-screen text in video — is one
pattern, not three. In every case a **fast local extractor** turns raw media into
`model.Block`s, each anchored back to a *slice* of the source and carrying a
per-unit **confidence**. The hard units are then escalated, low-confidence-first,
to a **configurable multimodal LLM** that re-reads just that slice — a crop for an
image, a time span for audio, a frame or clip for video.

This rests on two foundations defined elsewhere — it adds no new content-model
primitive:

- The **content model** ([AD-002](002-content-model.md)) anchors a Block to its
  source by a per-medium facet (run range for text, `geometry` for rendered media,
  `timing` for timed media) and records how a *recognized* source was produced in
  its `Origin` (engine + confidence). Extractors populate these; the escalation
  reads them.
- The **provider interface** ([AD-011](011-ai-providers.md)) carries
  `image`/`audio`/`video` content parts and advertises each backend's input
  modalities.

On those, this AD adds three things:

- The **confidence-gated escalation pattern** — tier the readers, escalate only
  the units the local extractor is unsure of.
- One **generic `media-refine` tool** — a source-`Transform`
  ([AD-006](006-tool-system.md)) — that gates on the source `Origin` confidence,
  slices the source via a per-modality `MediaSlicer`, and rewrites the Block's
  source from the LLM response, behind faithfulness guards.
- **Plugin and pipeline symmetry** — `kapi-asr` and the video demux reader mirror
  `kapi-vision`, so audio and video reuse the same tier.

The escalation tier is identical across modalities; only the *anchor* facet, the
*slicer*, and the LLM *content part* differ. Heavy extractors stay out-of-core in
plugins, mirroring `kapi-vision` and the SaT segmenter
([AD-021](021-sat-segmenter-plugin.md)).

## Context

The vision work ([AD-029](029-vision-and-image-localization.md)) establishes an OCR
pipeline and an in-browser handwriting cascade — PP-OCRv5 reads every line fast,
and lines below a confidence threshold are re-read by a handwriting model (TrOCR).
That cascade is the seed of a general idea: a frontier multimodal LLM reads hard
handwriting, garbled scans, accented or noisy speech, and ambiguous on-screen text
far better than a small specialized model, because it brings a language prior and
world knowledge to the disambiguation. But it is slower, costs per call, returns
no calibrated confidence, and — the decisive risk for a faithfulness-first tool —
fails *dishonestly*: handed an illegible crop it confabulates a plausible-but-wrong
word rather than admitting defeat. So the LLM is never the primary reader; it is a
**narrow escalation** over only the units the fast local extractor is unsure of,
fed only the slice in question.

Audio and video are the same problem in different coordinates — speech and
on-screen text are anchored in *time* (and, for video, time plus space) rather
than on a still page. Generalizing the spatial anchor to also cover time,
carrying the extractor's confidence as a first-class attribute the escalation
gate reads, and giving the provider interface image/audio/video content parts
are therefore one design, not an OCR feature: audio — automatic speech
recognition (ASR), the audio counterpart of OCR: speech in, text out — and video
extraction plug into the same escalation tier without reinvention.

## Decision

### The pattern: confidence-gated escalation over a media slice

Every modality runs the same three tiers, differing only by adapter:

1. **Tier 1 — fast local extractor.** OCR (image), ASR (audio), or demux→ASR+OCR
   (video) over the whole input, emitting confidence-scored Blocks.
2. **Tier 2 — specialized local model (optional).** A model tuned for the hard
   case: TrOCR for handwriting; a larger Whisper or domain ASR for difficult
   speech. Local, still credential-free.
3. **Tier 3 — configurable multimodal LLM.** The residual low-confidence units
   only, each re-read from its source slice, with the provider **explicitly
   selected** — never an implicit fallback.

What is shared across modalities is everything that governs correctness: the
confidence gate, a **context hint** (neighbouring extracted units passed as *text*
so the LLM gets the language prior without shipping the whole page or track), the
provenance tag, and the anti-confabulation guards below. What differs is captured
entirely in the per-modality adapters.

| | Image / OCR | Audio / ASR | Video |
|---|---|---|---|
| Tier-1 extractor | PP-OCRv5 (`kapi-vision`) | Whisper-family (`kapi-asr`) | demux → ASR on audio track + OCR on frames |
| Anchor | spatial: `page + Rect` | temporal: `[startMs, endMs]` | **both** — time span + optional frame bbox |
| Slice | crop pixels | cut time range | frame-extract (+crop) or short clip |
| Confidence | CTC mean logit | segment `avg_logprob` / `no_speech_prob` | per-track |
| LLM content part | `image` | `audio` | `image` (frame) or `video` (clip) |
| Refusal token | `[illegible]` | `[inaudible]` | per-track |

### What the extractor records

The escalation needs nothing the content model does not already define
([AD-002](002-content-model.md)). Each extractor's block builder
(`BlocksFromOCR`, `BlocksFromASR`) populates, on every Block it emits:

- the **anchor** facet for its medium — a `geometry` annotation (page + bounding
  box) for a rendered region, a `timing` annotation (time span) for an audio or
  video segment, both for on-screen text in video; and
- the source **`Origin`** — the extracting engine (`ocr`, `asr`) and a
  **confidence**, the same record a translation carries on its target side.

So a Block arrives at the refinement tier self-describing: confidence to gate on,
an anchor that says which slice of the source to re-read, and an engine for the
audit trail. The tier introduces no overlay of its own.

The provider side is equally already-defined: the multimodal `aiprovider`
([AD-011](011-ai-providers.md)) carries the slice as an `image`/`audio`/`video`
content part, and `media-refine` reads `InputModalities()` to pick a provider that
accepts the slice's modality — erroring clearly rather than silently degrading if
none is configured.

### The generic media-refine tool

One tool, dispatched by the anchor/modality, behind a `MediaSlicer` per modality:

```go
type MediaSlicer interface {
    // Returns the source slice as an LLM content part, reading the block's anchor
    // facet (AD-002): ImageCropper crops the geometry bbox; AudioCutter cuts the
    // timing span; VideoClipper extracts a frame (+crop) or a short clip.
    Slice(ctx context.Context, src MediaRef, b *model.Block) (aiprovider.ContentPart, error)
}
```

Control flow:

1. **Gate** — skip Blocks whose source `Origin` confidence is at or above the
   threshold.
2. **Slice** — resolve the modality's `MediaSlicer`; produce the content part.
3. **Prompt** — `[neighbouring extracted text as context] + [media part] +
   instruction to transcribe only the slice, returning the refusal token when
   unsure`.
4. **Call** — the explicitly-configured provider (capability-checked).
5. **Rewrite** — emit an `EditPlan` rewriting the Block source; set its source
   `Origin` engine to `llm:<provider>`; add a `qa` finding
   ([AD-002](002-content-model.md)) marking the unit for review when the LLM output
   diverges sharply from the Tier-1/Tier-2 guess or returns the refusal token.

`media-refine` is a **source-`Transform`** ([AD-006](006-tool-system.md)): it
rewrites source, so it runs in a flow's leading **source-transform stage** — the
same slot redaction occupies ([AD-020](020-redaction.md)) — settling the source
before annotation and translation. It must access the source raster/track while it
still exists; the vision tier-3 reader consumes and deletes the page raster before
blocks reach tools, so `media-refine` runs *inside* the extraction boundary (the
slicer holds the source ref), not as an arbitrary downstream tool.

### Plugin and pipeline symmetry

- **`kapi-asr`** mirrors `kapi-vision` and the SaT plugin exactly: a cgo
  `-tags onnx` (whisper.cpp / ONNX) binary loading its native stack at runtime,
  isolated from the portable `kapi` binary, driven over a stdin/stdout protocol,
  and **path-based** — the host passes a media path, never bytes
  ([AD-021](021-sat-segmenter-plugin.md), [AD-029](029-vision-and-image-localization.md)).
- **Video** is a demux format reader that emits an **audio child Layer** (→ ASR)
  and a **visual child Layer** (→ frame OCR), reusing the Layer-nesting model that
  already handles embedded content (HTML-in-JSON → child Layer,
  [AD-002](002-content-model.md)). It writes no transcription code of its own — it
  composes `kapi-asr` + `kapi-vision`.
- **Labs** extend the same way: the in-browser Vision Lab is the image instance of
  the pattern; an Audio Lab is its direct analog (transformers.js runs a Whisper
  `automatic-speech-recognition` pipeline in-browser, as the Vision Lab runs OCR
  and TrOCR).

### Faithfulness and guards

The confabulation risk is identical across modalities, so the guards are too:

- **Refusal token.** The model is instructed to return `[illegible]` /
  `[inaudible]` rather than guess; the token marks the unit for review (a `qa`
  finding), not fabricated source.
- **Divergence check.** A Tier-3 result that disagrees sharply with the Tier-1/2
  guess (both low-confidence) is flagged for review rather than silently accepted.
- **Provenance is visible.** LLM-sourced source text is the least-verified tier;
  the editor renders the source `Origin` (engine + confidence) and any `qa` review
  finding ([AD-027](027-visual-editor-data-model.md)) so a reviewer sees exactly
  which units a model invented versus read.
- **Slice, never page.** Only the low-confidence slice plus a text context hint
  leaves the process — bounding both cost and data exposure.

### Provider credentials

The configurable multimodal LLM tier runs **server/CLI-side** and draws its
`provider` + `model` + key from the same credential path as the other AI tools —
the keychain and environment ([AD-011](011-ai-providers.md),
[AD-013](013-kapi-cli.md)). "Configurable like the other AI tools" is satisfied
there. The in-browser Labs demonstrate the local extraction tiers (OCR, ASR, the
handwriting cascade); credentialed cloud refinement is a CLI/desktop capability,
not a browser one.

## Related

- [AD-002 Content Model](002-content-model.md) — anchor facets (geometry/timing) and source `Origin` confidence this tier reads
- [AD-006 Tool System](006-tool-system.md) — capability views, source-transform stage
- [AD-011 AI Providers](011-ai-providers.md) — the multimodal `LLMProvider` this tier sends slices to
- [AD-020 Content Redaction](020-redaction.md) — the recoverable-Transform precedent
- [AD-021 SaT Segmenter Plugin](021-sat-segmenter-plugin.md) — native-stack plugin isolation template
- [AD-027 Visual Editor](027-visual-editor-data-model.md) — renders source provenance and qa review findings
- [AD-029 Vision and Image Localization](029-vision-and-image-localization.md) — the image instance of this pattern
