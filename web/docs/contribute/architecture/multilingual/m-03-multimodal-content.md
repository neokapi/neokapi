---
id: m-03-multimodal-content
sidebar_position: 3
title: "M-03: Multimodal content"
description: "An image, an audio track and a video are adaptable assets whose text is recovered by a fast local extractor that records an anchor and a confidence, with a configurable multimodal model re-reading only the slices the extractor was unsure of."
keywords: [neokapi, architecture decision, multimodal, OCR, ASR, vision, video, subtitles, confidence, media-refine, image adaptation]
---

import { PhaseFlow, StreamDiagram } from "@neokapi/docs-shared";

# M-03: Multimodal content

## Summary

Recovering translatable content from a **non-text medium** (text rendered into
an image, speech in an audio track, captions and on-screen text in video) is
one pattern for all three. A **fast local extractor** turns raw media into
blocks, each anchored back to a *slice* of the source and carrying a per-unit
**confidence**. The units the extractor was unsure of are then escalated to a
**configurable multimodal model** that re-reads just that slice: a crop for an
image, a time span for audio, a frame or clip for video.

Underneath that, an image, an audio file and a video are each also a whole
**adaptable asset**, a thing a per-locale variant can replace outright. The two
modes are independent and compose on one file.

Nothing here is a new content-model primitive. The anchors are the content
model's existing per-medium facets, the confidence is a field on the source
provenance record, and the multimodal message is the provider interface's. What
this decision adds is the escalation tier, one generic transform that implements
it, and the plugin symmetry that makes audio and video reuse the image path.

## Every medium is an adaptable asset first

Adapting a document that contains an image is not one problem, and treating
"image" as "OCR" conflates them. The distinct modes are:

| Mode | What it adapts | Mechanism |
| --- | --- | --- |
| Whole-asset replacement | the bytes | a per-locale file replaces the source |
| Alt text / caption | accessible text, not pixels | a translatable caption block linked to the asset |
| Metadata | embedded title, description, keywords | translatable fields become metadata-plane blocks |
| In-asset text | text rendered into pixels, speech in a waveform | extract, translate, deliver as a companion |
| Layout / structure | the document's regions | detect regions and reading order |

Whole-asset replacement is the most common and the simplest to reason about: a
translator or an automated pipeline supplies a per-locale picture. The rest is
enrichment. The content model already carries what these need (a media part
with its bytes, blob key or URI plus alt text, and the standoff structure and
role annotations ([F-02](../foundations/f-02-content-model.md))), so the
architectural job is to keep the format generic and make vision an *optional*
layer rather than the identity of the format.

The image format therefore reads PNG and JPEG and **always** emits the picture
as a media part referenced by URI, never as inline bytes: the binary never
travels through the part stream. A matching writer emits a media part's bytes,
which is the whole-image sink. Two boolean toggles, both default-on, gate the
enrichment: `ocr` runs in-image text recognition, and `layout` runs ML layout
detection when OCR runs. With `ocr` off, or with the plugin absent, an image is
a media asset and nothing else.

### Alt text

An image's accessible text is translated as content, not as a media field. When
an `<image>.alt.txt` sidecar sits beside the source, the reader attaches its
text to the media for display **and** emits it as a translatable caption block
linked to the image by a `caption-of` relation. That block flows through the
ordinary path (content memory, translation, voice profile, checks, batching)
with no special tool support, and gets per-locale targets like any other block.
The writer folds the translated target (or the source, as a round-trip
fallback) back into a per-locale sidecar beside the written image.

Modelling alt text as a linked block rather than mutating the single alt-text
field in place is what makes it per-locale and what lets it reuse the whole
translation stack.

### Metadata

Embedded document metadata is document-level, not anchored to any run, so it
lives on the layer rather than in a run-anchored overlay. A shared helper splits
it: translatable fields (title, description, keywords) become blocks on the
metadata plane and travel the normal block path, while non-translatable fields
(author, copyright, software, dates) are recorded as namespaced layer properties,
never translated but kept for inspection. The image reader reads PNG text chunks
and embedded XMP without decoding the pixels: it stops scanning at the first
image-data chunk.

Extraction surfaces metadata for translation, memory and inspection. Whether the
*translated* metadata is re-embedded is the writer's decision: a skeleton-based
format re-applies the field, and a cross-format conversion carries the metadata
blocks into the output document. The byte-copy image writer preserves the source
image's original embedded metadata unchanged.

### The pseudo variant

The first image transform is the **pseudo variant**, the visual analogue of
pseudo-translated text. On encountering an image media part, the
`pseudo-translate` tool replaces it with a clearly-visible watermarked variant (a
colour wash, a solid border, a diagonal band) and pseudo-translates the alt text.
Read an image, pseudo-translate, write, and the output is unmistakably marked:
proof, in a build artefact or a UI, that the asset was actually swapped rather
than passed through. It is deterministic and dependency-free, standard-library
raster operations only.

## The extraction plugins

The ML capabilities carry native-stack weight, so they live in plugins and never
in `kapi`, on the same reasoning as the ML segmenter
([M-02](m-02-segmentation.md), [E-05](../engine/e-05-plugin-system.md)). Three
plugins cover the media formats, and the framework holds a seam for each:

- **`kapi-vision`** runs the document-vision models: PP-OCRv5 mobile detection
  and recognition, and PP-DocLayoutV3 region detection with reading order. It is
  driven over a **length-prefixed binary frame** protocol rather than the
  line-delimited JSON the segmenter uses, because vision requests carry raw
  image bytes: megabyte-scale payloads that would blow a line buffer and waste
  a third of their size on base64.
- **`kapi-asr`** runs a Whisper-family model behind a line-delimited JSON
  protocol.
- **`kapi-av`** bundles the per-platform demux binaries. Unlike the other two it
  runs no subprocess of its own: the demux engine is in-process and only needs to
  find the bundled binaries, so the plugin's job is to be discoverable. Its
  release archives carry the LGPL-2.1 licence text beside the ffmpeg binaries
  they ship.

Every engine is **path-based**. The host passes a filesystem path, never bytes,
so a large image or audio track lives only in the engine's process. The
framework seams (an OCR engine interface with an optional layout interface
type-asserted on top, and a transcription engine interface) are name-keyed
registries shaped exactly like the segment registry, so a capability is absent
when its plugin is not installed and a browser build can register a
WebAssembly-backed engine under the same name.

### What the extractors record

Each extractor's block builder populates two things on every block it emits, and
they are all the escalation tier needs:

- the **anchor** facet for its medium: a `geometry` annotation (page and
  bounding box) for a rendered region, a `timing` annotation (a millisecond
  span) for an audio or video segment, both for on-screen text in video; and
- the source **origin**: the extracting engine (`ocr`, `asr`) and a
  **confidence** in `[0,1]`, the same provenance record a translation carries on
  its target side.

A block therefore arrives at the refinement tier self-describing: a confidence
to gate on, an anchor that says which slice of the source to re-read, and an
engine for the audit trail.

### Layout gives structure a third tier

OCR alone produces lines with pixel geometry, which a geometric analyser turns
into inferred structure. With layout enabled, the detector returns already-decoded
regions in original pixel coordinates, its region classes map onto content roles
(title, heading, table, picture, formula, footnote, header and footer), and a
deterministic column-clustering heuristic assigns reading order. The reader then
assigns OCR lines to regions by containment and emits role-tagged blocks in
reading order: authoritative structure rather than inferred.

A table region's lines are reconstructed into row and column **cell structure**
by reusing the same grid clustering the geometric tier uses, so both tiers emit
tables through one path and a writer renders a real table. The full tier model
lives in [E-08](../engine/e-08-document-structure-tiers.md); vision is its
top-tier engine, and because it is format-agnostic over rasters it serves any
format that can produce one.

## Confidence-gated escalation

A frontier multimodal model reads hard handwriting, garbled scans, accented or
noisy speech and ambiguous on-screen text far better than a small specialised
model, because it brings a language prior and world knowledge to the
disambiguation. It is also slower, costs per call, returns no calibrated
confidence, and, the decisive risk for a faithfulness-first tool, fails
*dishonestly*: handed an illegible crop it confabulates a plausible wrong word
rather than admitting defeat.

So it is never the primary reader. It is a narrow escalation over only the units
the fast local extractor was unsure of, fed only the slice in question.

<PhaseFlow
  nodes={[
    { label: "Tier 1: local extractor", sub: "OCR · ASR · demux → both", role: "annotate" },
    { label: "Tier 2: specialised local model", sub: "optional; credential-free", role: "annotate", edge: "below the confidence gate" },
    { label: "Tier 3: multimodal model", sub: "explicitly configured provider", role: "translate", edge: "still below the gate" },
    { label: "rewritten source + provenance", sub: "origin kind: llm-refined", role: "io", edge: "behind the guards" },
  ]}
  caption="The tier structure is identical across modalities; only the anchor, the slicer and the content part differ."
/>

What is shared across modalities is everything that governs correctness: the
confidence gate, a **context hint** (neighbouring extracted units passed as
*text*, so the model gets a language prior without shipping the whole page or
track), the provenance stamp, and the guards below. What differs is captured
entirely in per-modality adapters:

| | Image | Audio | Video |
| --- | --- | --- | --- |
| Tier-1 extractor | OCR (`kapi-vision`) | Whisper-family (`kapi-asr`) | demux → ASR on the audio track, OCR on frames |
| Anchor | spatial: page and bounding box | temporal: a millisecond span | both: a time span plus an optional frame box |
| Slice | crop the pixels | cut the time range | extract a frame (and crop it) |
| Content part | image | audio | image |

The tier is never an implicit fallback: the provider is explicitly selected, and
its declared input modalities are checked against the slice's modality, so an
unconfigured or incapable provider is a clear error rather than a silent
degradation.

## The media-refine transform

One tool implements the tier, dispatched by modality behind a slicer interface:

```go
type MediaSlicer interface {
    Slice(ctx context.Context, src MediaRef, b *model.Block) (aiprovider.ContentPart, error)
    Modality() aiprovider.Modality
}
```

`ImageSlicer` crops the block's geometry box out of the source raster;
`AudioCutter` cuts the block's timing span out of the track; `VideoClipper`
extracts the frame at the block's timing and crops its on-frame geometry. The
source stays a reference end to end (a path, a blob key or a URI), so only the
bounded slice is ever materialized: a whole raster or track never enters the part
stream or a provider call.

The control flow is:

1. **Gate**: skip any block whose source confidence is at or above the
   threshold.
2. **Slice**: resolve the modality's slicer and produce the content part.
3. **Prompt**: the neighbouring extracted text as context, the media part, and
   an instruction to transcribe only the slice and return the refusal token when
   unsure. Each modality has its own prompt id, so the prompt reference
   enumerates all three and `--explain-prompts` names the one a call used
   ([M-05](m-05-prompts-and-batching.md)).
4. **Call**: the explicitly configured, capability-checked provider.
5. **Rewrite**: emit an edit plan rewriting the block's source, stamp the source
   origin with the **`llm-refined`** kind (engine `llm:<provider>`, with the
   prior recognizer's engine kept in the reference so a refined unit is queryable
   without losing the original provenance), and mark the unit for review when the
   result diverges sharply from the tier-1 guess or comes back as a refusal.

`media-refine` is a **source transform** ([E-03](../engine/e-03-tool-system.md)):
it rewrites source, so it runs in a flow's leading source-transform stage, the
same slot redaction occupies ([C-10](../context/c-10-redaction.md)), settling
the source before annotation and translation. It must reach the source raster or
track while it still exists, so it runs inside the extraction boundary with the
slicer holding the source reference, not as an arbitrary downstream tool.

### Guards

The confabulation risk is identical across modalities, so the guards are too:

- **A refusal token.** The model is instructed to return it rather than guess.
  The token marks the unit for review; it never becomes fabricated source.
- **A divergence check.** A tier-3 result that disagrees sharply with the
  tier-1 guess is flagged for review rather than silently accepted.
- **Visible provenance.** Model-sourced source text is the least-verified tier,
  so the editor renders the source origin (engine and confidence) and any
  review finding ([S-06](../surfaces/s-06-visual-editor.md)), and a reviewer can
  see exactly which units a model invented versus read.
- **Slice, never page.** Only the low-confidence slice plus a text context hint
  leaves the process, which bounds both cost and data exposure.

The credentialed tier runs where the other model-backed tools run and draws its
provider, model and key from the same credential path
([E-07](../engine/e-07-model-providers.md),
[S-01](../surfaces/s-01-kapi-cli.md)).

## Two output modes

Extraction feeds two independent adaptation modes, and one asset can use both.

**Round-trip the text.** The anchored blocks are translated and return to where
they came from. The path depends on how the text lives in the asset:

- **A text track or sidecar** round-trips fully today. WebVTT, SubRip and TTML
  are first-class formats with both a reader and a writer, so cues extract,
  translate and merge back into a per-locale track the source platform ingests.
  For raw audio or video, transcription produces the cues and the **timing
  anchor** is the hand-off into the timed-text writer. Built-in flows compose
  this end to end: `audio-to-subtitles`, `video-to-subtitles` and
  `image-ocr-translate`.
- **An embedded text layer with a skeleton** re-applies targets through the
  format's own writer ([E-02](../engine/e-02-format-system.md)).
- **Text baked into pixels or a waveform** cannot return to the same rendered
  asset without re-rendering or speech synthesis, neither of which the engine
  does. The result is delivered as a **companion** instead: a generated
  per-locale subtitle track, which is itself an ordinary write.

**Replace the asset.** Independently, the whole file is an adaptable media
asset. The target-asset variant model pairs a source file with its per-locale
files: a resolver walks each locale's target path from the recipe's template and
reports which variants exist. This is medium-agnostic: the binary-asset
predicate covers image, audio and video, each with a passthrough writer that
emits the supplied per-locale bytes.

Because kapi cannot regenerate a *genuinely* adapted asset, a per-locale variant
already on disk is **authoritative**: a run keeps it rather than clobbering it by
reprocessing the source, while a missing variant falls through to the flow to
produce a pseudo or copy fallback. The engine never synthesizes per-locale media;
a replacement is a file a person or a connector provides.

## Video is composition

<StreamDiagram
  title="video reader"
  items={[
    { kind: "PartLayer", detail: "the video document", role: "layer" },
    { kind: "PartLayer", detail: "audio track", depth: 1, role: "layer", note: "→ transcription → timing-anchored blocks" },
    { kind: "PartBlock", detail: "speech cue", depth: 2, role: "block" },
    { kind: "PartLayer", detail: "sampled frames", depth: 1, role: "layer", note: "→ OCR → geometry + timing anchored blocks" },
    { kind: "PartBlock", detail: "on-screen text", depth: 2, role: "block" },
  ]}
  ariaLabel="The video reader emitting an audio child layer and a visual child layer"
  caption="The reader demuxes and composes; it writes no recognition code of its own."
/>

The video format reader demuxes a file into the two streams a flow extracts
from and emits each under its own **child layer**, reusing the layer-nesting the
content model already uses for embedded content
([F-02](../foundations/f-02-content-model.md)). Frames are sampled and
deduplicated before OCR, so a static shot is read once. An engine that is not
installed is skipped, so the reader degrades rather than failing.

The browser labs extend the same way: the same models, only the runtime differs
(WebAssembly instead of native). The Vision Lab runs the detection, recognition
and layout models in-page; the Audio and Video lab transcribes a clip and, for
video, demuxes it in the browser into an audio track and sampled frames before
running the same two engines. Nothing is mocked: a lab that showed a stub would
be demonstrating the plumbing rather than the capability.

## Consequences

- "Image", "audio" and "video" stay generic, adaptable formats. Recognition and
  layout are optional layers that degrade gracefully (absent plugin, or toggled
  off) to a whole-asset media part.
- The portable `kapi` binary stays pure-Go and small, the native ML stacks are
  confined to their plugins, and media bytes never enter the host process.
- One escalation tier serves every modality. Adding a medium means adding an
  extractor, an anchor facet it already has, and a slicer, not a second
  refinement design.
- Whole-asset replacement is supported end to end for every binary asset format:
  emitted as media, written per locale, pseudo-variant for proof, and an existing
  variant treated as authoritative.

## Related

- [F-02: The content model](../foundations/f-02-content-model.md): media parts, anchor facets, child layers, source provenance
- [E-02: The format system](../engine/e-02-format-system.md): the image, audio, video and timed-text formats and their writers
- [E-03: The tool system](../engine/e-03-tool-system.md): the source-transform stage `media-refine` runs in
- [E-05: The plugin system](../engine/e-05-plugin-system.md): native-stack isolation, discovery, signed distribution
- [E-07: Model and translation providers](../engine/e-07-model-providers.md): the multimodal content parts and declared input modalities
- [E-08: Document structure tiers](../engine/e-08-document-structure-tiers.md): the tier model layout detection tops out
- [M-02: Segmentation](m-02-segmentation.md): the plugin-isolation precedent
- [M-05: Prompts and batching](m-05-prompts-and-batching.md): the per-modality refinement prompts
- [C-10: Redaction](../context/c-10-redaction.md): the other source transform in the same stage
- [S-06: The visual editor](../surfaces/s-06-visual-editor.md): where source provenance and review findings surface
- [Multimodal content](/contribute/implementation/multilingual/multimodal-content): the tactical note
