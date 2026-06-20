---
title: Multimodal localization — implementation map
---

# Multimodal localization — implementation map

Tactical notes for the multimodal surface described in
[AD-030](../architecture/030-multimodal-extraction-and-llm-refinement.md)
(extraction + refinement) and
[AD-029](../architecture/029-vision-and-image-localization.md) (vision). The
content-model anchors are [AD-002](../architecture/002-content-model.md):
a block carries a temporal `TimingAnnotation`, a spatial `GeometryAnnotation`,
and a recognition `Origin` (OCR/ASR, with confidence).

## Two axes of localization

Every image / audio / video asset is localizable two ways, and both are
first-class:

- **Extracted content** — the text *inside* the asset (OCR text, speech,
  on-screen frame text, subtitle cues). Translate it, then round-trip into a
  text-carrying artifact (a subtitle file, an alt-text sidecar).
- **The whole asset** (`replace-asset`) — the file *is* the deliverable. The
  project pairs the source with a per-locale file the user/connector supplies,
  and treats it as authoritative. `core/project.ResolveAssetVariants` +
  `IsBinaryAssetFormat` now cover `image`, `audio`, and `video`; the audio/video
  writers are passthrough sinks that emit the (replacement) bytes. The engine
  does **not** synthesize localized media — no TTS / re-encode in core.

## Readers, writers, round-trip

| Format | Reader emits | Writer | Round-trip |
|---|---|---|---|
| image | OCR blocks (geometry + glyphs) + alt-text caption, or Media | image bytes + `.alt.txt` | alt-text sidecar; whole-image replace-asset |
| audio | ASR blocks (timing + Origin), or opaque Media | passthrough | subtitle file (via a flow); whole-audio replace-asset |
| video | audio child-layer (ASR) + frames child-layer (OCR geometry+timing), or opaque Media when no ffmpeg | passthrough | subtitle file (speech only); whole-video replace-asset |
| vtt / srt / ttml | subtitle cues with `TimingAnnotation` | yes | byte-exact subtitle round-trip |

The `srt` reader now populates the canonical `TimingAnnotation` (not just
`Properties["timecode"]`). The `video` reader degrades to opaque Media when no
ffmpeg/av engine is resolvable instead of erroring.

## Flows + the subtitle filter

Built-in flows (`core/flow.BuiltInFlows`) compose the existing tools; the
reader/writer are run-time bindings (AD-026), so a flow is just the tool chain:

- `audio-to-subtitles` — `translate` (the audio reader yields timed cues).
- `video-to-subtitles` — `subtitle-filter → translate`. The new
  `subtitle-filter` tool (`core/tools`) keeps only timing-anchored, non-geometry
  cues, dropping the frame-OCR (geometry-anchored) blocks so on-screen text never
  pollutes the subtitle track.
- `image-ocr-translate` — `translate` (round-trips translated alt-text).

```bash
kapi run video-to-subtitles -i talk.mp4 -o talk.fr.vtt --target-lang fr
```

## LLM refinement

`media-refine` (`core/ai/tools/media_refine.go`) is a confidence-gated
source-transform: low-confidence recognized units are re-read by a configurable
multimodal LLM over a bounded media slice (`ImageSlicer`, `AudioCutter`,
`VideoClipper`). A refined unit gets the canonical `OriginLLMRefined` kind
(`Engine` names the producing LLM, `Reference` keeps the prior recognizer) and a
`kapi-needs-review` flag when the LLM diverges from the original guess.

## Desktop viewing (kapi-desktop)

The `DocumentViewer` already renders **Structure** (role/reading-order tree) and
**Layout** (page-scaled bounding boxes) for geometry-bearing sources. The
multimodal additions:

- **Media tab** — auto-shown when the tree has a media node or timing. Image →
  `MediaCanvas` (raster + role-colored OCR overlay, bidirectional box↔block
  selection); audio → `AudioPlayer`; video → `VideoPlayer` (player + subtitle
  timeline synced to the timing cues). Components live in
  `@neokapi/ui-primitives/preview`; the shared coordinate/timecode math is in
  `geometry.ts` / `timeline.ts`.
- **Media serving** — the backend `MediaDataURL` reads a media node's file and
  returns a `data:` URL; `FilePreview` resolves each media node and passes
  `resolveMediaUrl` to the viewer.
- **On-demand engine install** — opening an image/audio/video installs the
  enriching engine plugin (`vision` / `asr` / `av`) when it isn't already
  available (`ensureMediaEngine`), the engine analogue of `ensureFormatPlugin`.

The `ContentTree` (`core/editor`) carries `TimingView` and `MediaView` on each
node so the frontend gets the timing/media descriptor alongside structure and
geometry.
