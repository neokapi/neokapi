---
id: 030-multimodal-extraction-and-llm-refinement
sidebar_position: 30
title: "AD-030: Multimodal Extraction and LLM Refinement"
description: "Architecture decision: extracting translatable content from images, audio, and video is one pattern — a fast local extractor produces confidence-scored Blocks anchored to a slice of the source media, and a configurable multimodal LLM refines only the low-confidence units. A MediaAnchor + ExtractionProvenance overlay pair makes Blocks self-describing across space and time, a multimodal aiprovider carries image/audio/video content parts, and one generic media-refine Transform tool escalates per modality."
keywords: [multimodal, OCR, ASR, speech recognition, video localization, audio localization, LLM refinement, confidence cascade, MediaAnchor, ExtractionProvenance, multimodal aiprovider, vision LLM, Whisper, kapi-vision, kapi-asr, architecture decision, neokapi]
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

Four pieces make this generic across modalities:

- A **MediaAnchor** overlay generalizes the spatial `GeometryAnnotation`
  ([AD-029](029-vision-and-image-localization.md)) to locate a Block in source
  media across **space *and* time** (`page + bbox`, `[startMs, endMs]`, or both).
- An **ExtractionProvenance** overlay persists `{modality, engine, modelVersion,
  confidence, needsReview}` on every extracted Block — the gate the escalation
  reads, and the audit trail the editor shows.
- A **multimodal `aiprovider`** carries `image`/`audio`/`video` content parts and
  advertises which input modalities each backend accepts
  ([AD-011](011-ai-providers.md)).
- One **generic `media-refine` tool** — a source-`Transform`
  ([AD-006](006-tool-system.md)) — gates on confidence, slices the source via a
  per-modality `MediaSlicer`, and rewrites the Block's source from the LLM
  response, behind faithfulness guards.

The escalation tier is identical across modalities; only the *anchor*, the
*slicer*, and the LLM *content part* differ. Heavy extractors stay out-of-core in
plugins, mirroring `kapi-vision` and the SaT segmenter
([AD-021](021-sat-segmenter-plugin.md)).

## Context

The vision work ([AD-029](029-vision-and-image-localization.md)) shipped an OCR
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

Audio and video are the same problem with different coordinates. Today the content
model anchors content spatially only (`GeometryAnnotation`: page + pixel `Rect`),
and the OCR confidence that the cascade depends on is **discarded** by
`vision.BlocksFromOCR` — it never reaches a downstream tool. The `aiprovider`
`Message` is **text-only** across all five backends, so no tool can send an image,
let alone an audio clip. Building an OCR-shaped LLM refiner now would bake those
limits in. This AD instead defines the *generic* shape so audio — automatic
speech recognition (ASR), the audio counterpart of OCR: speech in, text out —
and video extraction plug into the same escalation tier without reinvention.

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

### Content model: MediaAnchor and ExtractionProvenance

Two stand-off overlays ([AD-002](002-content-model.md)) make an extracted Block
self-describing enough for one modality-generic tool, and close the two gaps named
above (dropped confidence, spatial-only anchoring).

**MediaAnchor** locates a Block within source media across space and time. It
subsumes `GeometryAnnotation` — an image Block carries only the spatial part, an
audio Block only the temporal part, an on-screen-text-in-video Block carries both:

```go
type MediaAnchor struct {
    Source   string              // ref to the source Media / child Layer
    Spatial  *GeometryAnnotation // page + bbox; nil for pure audio
    Temporal *TimeSpan           // {StartMS, EndMS}; nil for still images
}
```

**ExtractionProvenance** records how the Block's source text came to be — the gate
the escalation reads and the trail the editor surfaces:

```go
type ExtractionProvenance struct {
    Modality     Modality // closed set, named type (see below)
    Engine       EngineID // open ID: plugins register their own
    ModelVersion string   // free-form version string
    Confidence   float64  // [0,1]; persisted, never discarded
    NeedsReview  bool     // set on LLM divergence / refusal
}

// Modality is a closed enumeration, typed with constants like ProviderID /
// PartType / Capability — not a bare string.
type Modality string

const (
    ModalityImage Modality = "image"
    ModalityAudio Modality = "audio"
    ModalityVideo Modality = "video"
)

// EngineID is an open identifier — extractors register their own, as providers
// register ProviderIDs. Well-known values: "ppocr", "trocr", "whisper",
// "llm:<provider>".
type EngineID string
```

This follows neokapi's typing convention: **closed sets and registry IDs are
named types with constants** (`PartType`, `ProviderID`, `Capability`,
`LocaleID`), while **open or standardized free-form values stay `string`** with a
doc comment (`Message.Role`, `GeometryAnnotation.Origin`, IANA media types,
version strings). `MediaAnchor.Source` follows the existing `GeometryAnnotation.SourceRef`
precedent — an ID reference held as `string`.

`BlocksFromOCR` (and the future `BlocksFromASR`) attach both overlays. The browser
cascade's existing `OCRLine.engine` field is the same idea one layer up; this makes
it first-class and native.

### The keystone: a multimodal aiprovider

`aiprovider.Message.Content` becomes an ordered list of typed parts, and providers
advertise the input modalities they accept ([AD-011](011-ai-providers.md)):

```go
type ContentPart struct {
    Kind      ContentKind // closed set, named type
    Text      string      // Kind == ContentText
    Data      []byte      // Kind == ContentImage|Audio|Video (base64-encoded on the wire)
    MediaType string      // IANA media type: "image/png", "audio/wav", "video/mp4"
}

// ContentKind discriminates a part (named type with constants, like PartType).
// Distinct from Modality: it includes text, which every provider accepts.
type ContentKind string

const (
    ContentText  ContentKind = "text"
    ContentImage ContentKind = "image"
    ContentAudio ContentKind = "audio"
    ContentVideo ContentKind = "video"
)

type Message struct {
    Role  string        // unchanged from the existing type: "system"|"user"|"assistant"
    Parts []ContentPart
}

// Capability descriptor — media-refine selects a provider that accepts the
// modality it needs, rather than discovering the limit at call time. Returns
// the non-text modalities a backend accepts (text is always supported).
func (p Provider) InputModalities() []Modality
```

A bare string folds to a single `text` part, so existing AI tools
([AD-022](022-brand-voice.md), translate/QA/terminology) are unaffected. This one
change unlocks *all three* modalities, not just OCR. Backends differ in reach —
roughly:

| Provider | Accepts |
|---|---|
| Gemini | text, image, audio, video |
| OpenAI / Azure OpenAI | text, image (audio on audio-capable models) |
| Anthropic | text, image |
| Ollama (vision models) | text, image |

`media-refine` reads `InputModalities()` and errors clearly if the chosen provider
cannot accept the slice's modality — never silently degrades.

### The generic media-refine tool

One tool, dispatched by the anchor/modality, behind a `MediaSlicer` per modality:

```go
type MediaSlicer interface {
    // Returns the source slice as an LLM content part: ImageCropper crops the
    // raster bbox; AudioCutter cuts [startMs,endMs]; VideoClipper extracts a
    // frame (+crop) or a short clip.
    Slice(ctx context.Context, src MediaRef, a MediaAnchor) (aiprovider.ContentPart, error)
}
```

Control flow:

1. **Gate** — skip Blocks whose `ExtractionProvenance.Confidence ≥ threshold` and
   not `NeedsReview`.
2. **Slice** — resolve the modality's `MediaSlicer`; produce the content part.
3. **Prompt** — `[neighbouring extracted text as context] + [media part] +
   instruction to transcribe only the slice, returning the refusal token when
   unsure`.
4. **Call** — the explicitly-configured provider (capability-checked).
5. **Rewrite** — emit an `EditPlan` rewriting the Block source; tag
   `ExtractionProvenance.Engine = "llm:<provider>"`; set `NeedsReview` when the LLM
   output diverges sharply from the Tier-1/Tier-2 guess or returns the refusal
   token.

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
  `[inaudible]` rather than guess; the token maps to `NeedsReview`, not to
  fabricated source.
- **Divergence check.** A Tier-3 result that disagrees sharply with the Tier-1/2
  guess (both low-confidence) is flagged `NeedsReview` rather than silently
  accepted.
- **Provenance is visible.** LLM-sourced source text is the least-verified tier;
  the editor renders the `Engine`/`NeedsReview` provenance ([AD-027](027-visual-editor-data-model.md))
  so a reviewer sees exactly which units a model invented versus read.
- **Slice, never page.** Only the low-confidence slice plus a text context hint
  leaves the process — bounding both cost and data exposure.

### Credentials and the browser boundary

A configurable cloud LLM tier belongs **server/CLI-side**, where credentials
already have a home: the keychain and env path the AI tools use
([AD-011](011-ai-providers.md), [AD-013](013-kapi-cli.md)). "Configurable like the
other AI tools" — `provider` + `model` + key from the credential store — is
satisfied there.

The static-site **Labs have no backend and no credential store**, and a fully
OAuth-automated, no-key-pasting path to a frontier cloud LLM from a browser does
**not** exist across providers: OpenAI and Anthropic authenticate the API with keys
only (Anthropic's subscription OAuth token is contractually prohibited in
third-party tools as of 2026), and Google's in-browser token flow does not bill a
user's own Gemini account without a site-registered OAuth client and a billing-
enabled project. Pasting raw API keys into a docs page is rejected on security
grounds. The Labs therefore use **keyless local inference only** — Ollama on
`localhost` (a vision/ASR model the user runs) or an in-browser model via
transformers.js (TrOCR today; a small VLM where an ONNX export exists). The Lab is
a *local-inference demonstrator* of the pattern; cloud-provider refinement is a
CLI/desktop capability. This keeps the browser surface credential-free and matches
neokapi's local-first posture and the kapi/bowrain product boundary.

## Status

This AD defines the target architecture. Implemented today
([AD-029](029-vision-and-image-localization.md)): the image OCR + layout pipeline,
and the in-browser Vision Lab handwriting cascade (PP-OCR → TrOCR, with
`OCRLine.confidence` + `engine`). Not yet built: native `ExtractionProvenance` /
`MediaAnchor` persistence (OCR confidence is currently dropped by
`BlocksFromOCR`), the multimodal `aiprovider` (`Message` is text-only), the generic
`media-refine` tool and `MediaSlicer`s, the Vision Lab Tier-3 (keyless-local) LLM
option, and the `kapi-asr` plugin + video demux reader. The phased build order is
tracked in the implementation issue, not here.

## Related

- [AD-002 Content Model](002-content-model.md) — overlays, Layers, semantic vocabulary
- [AD-006 Tool System](006-tool-system.md) — capability views, source-transform stage
- [AD-011 AI Providers](011-ai-providers.md) — `LLMProvider`, the multimodal extension
- [AD-020 Content Redaction](020-redaction.md) — the recoverable-Transform precedent
- [AD-021 SaT Segmenter Plugin](021-sat-segmenter-plugin.md) — native-stack plugin isolation template
- [AD-029 Vision and Image Localization](029-vision-and-image-localization.md) — the image instance of this pattern
