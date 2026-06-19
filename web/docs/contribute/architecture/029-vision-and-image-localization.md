---
id: 029-vision-and-image-localization
sidebar_position: 29
title: "AD-029: Vision and Image Localization"
description: "Architecture decision: images are first-class localizable assets, and the kapi-vision plugin adds optional document-vision enrichment — OCR (PP-OCRv5) and ML layout detection (PP-DocLayoutV3) — out-of-core, path-based, with localization modes from whole-image replacement to in-image text extraction."
keywords: [vision, OCR, layout detection, PP-OCRv5, PP-DocLayoutV3, image localization, kapi-vision, onnxruntime, plugin, reading order, architecture decision, neokapi]
---

# AD-029: Vision and Image Localization

## Summary

An **image is a localizable asset**, not merely a carrier of text. The `image`
format reads PNG/JPEG and always emits the picture as a `model.Media` part — the
unit a localization flow can **replace wholesale** with a per-locale variant.
On top of that base, the out-of-core **`kapi-vision`** plugin adds optional
*document-vision* enrichment:

- **OCR** — RapidOCR / PP-OCRv5 text detection + recognition (shipped v0.1.0).
- **Layout** — PP-DocLayoutV3 (RT-DETR) region detection + reading order, yielding
  tier-3 structure (shipped v0.2.0).

Vision mirrors the `kapi-sat` and `kapi-pdfium` plugins: a cgo `-tags onnx`
binary that loads onnxruntime at runtime, isolated from the portable `kapi`
binary, driven over a binary-framed stdin/stdout protocol. Like the PDF reader
([AD-028](028-pdf-reader-plugin.md)), it is **path-based** — the host passes a
file path, never image bytes, so the picture lives only in the plugin process.
OCR and layout are **opt-in capabilities** (`ocr`, `layout` config toggles);
with both off, an image is a Media asset only.

## Context

Localizing a document that contains images is not one problem but several, and
treating "image" as "OCR" conflates them. The distinct modes are:

| Mode | What it localizes | Mechanism |
|---|---|---|
| **Whole-image replacement** | the pixels | a localized image file per locale swaps the source (screenshots, graphics with baked-in text); pseudo-localization (a visible watermark variant) ships today |
| **Alt-text / caption** | accessible text, not pixels | the alt text is emitted as a translatable caption Block linked to the image (`RoleCaption` + `RelCaptionOf`) and localized through the normal block path |
| **Metadata** | embedded title/description/keywords | translatable metadata fields → metadata-plane Blocks; non-translatable fields → namespaced Layer properties (`core/docmeta`) |
| **In-image text (OCR)** | text rendered into the image | extract → translate → (optionally) re-render |
| **Layout / structure** | the document's regions | detect regions + reading order, with table regions reconstructed into row/column cell structure, for faithful reconstruction |

Whole-image replacement is the most common and the simplest to reason about: the
translator (or an automated pipeline) supplies a localized picture. The others
are enrichment. The content model already carries what these need —
`model.Media{Data│BlobKey│URI, AltText}` + `PartMedia`, and the structure/role
annotations ([AD-002](002-content-model.md)) — so the architecture's job is to
keep "image" generic and make vision an *optional* layer, not the identity of the
format.

The ML capabilities carry the same native-stack weight as the SaT segmenter
([AD-021](021-sat-segmenter-plugin.md)) — onnxruntime, large model assets — so
they live in a plugin, never in `kapi`.

## Decision

### The image format is a localizable asset

`core/formats/image` reads PNG/JPEG and **always** emits the image as a `Media`
part referenced by URI (never inline bytes — the binary never travels through the
kapi part stream). This alone supports whole-image localization: the Media is the
asset; a localized variant is a different file. A matching **image writer** emits
a `Media` part's bytes — the whole-image localization *sink* — so a transform
that produces a localized image variant can be written back out.

#### Alt-text / caption

An image's accessible text is localized as content, not as a Media field. When an
`<image>.alt.txt` sidecar sits beside the source, the reader attaches its text to
the `Media` (`AltText`, for display) **and** emits it as a translatable caption
`Block` linked to the image (`RoleCaption` + a `caption-of` relation to the Media
ID). That block flows through the ordinary block path — TM, AI translate, brand
voice, sessions, batching — with no special tool support, and gets per-locale
`Targets` like any other block. The image writer folds the localized target (or
the source text, as a round-trip fallback) back into a per-locale
`<output>.alt.txt` sidecar beside the written image. Modeling alt-text as a
linked block (rather than mutating the single `Media.AltText` field in place)
keeps it per-locale and reuses the whole translation stack; `Media.AltText`
remains the source value for display.

#### Pseudo-localization

The first localized-image transform is **pseudo-localization** — the visual
analog of text pseudo-translation. The `pseudo-translate` tool, on encountering
an image `Media` part, replaces it with a clearly-visible watermarked variant (a
color wash + a solid border + a diagonal band; `core/imageops.PseudoLocalize`)
and pseudo-translates the alt-text. Read an image → pseudo-translate → write, and
the output is an unmistakably-marked image — proof, in a UI or build artifact,
that image localization actually swapped the asset. It is deterministic and
dependency-free (standard-library raster ops only).

#### Metadata

Embedded document metadata is localized the same way, via the shared
**`core/docmeta`** helper. Metadata is document-level — not anchored to any run —
so it lives on the **Layer**, never in a run-anchored overlay: translatable
fields (title, description, keywords) become Blocks on the metadata plane
(`StructureAnnotation.Layer == LayerMetadata`) that localize through the normal
block path, while non-translatable fields (author, copyright, software, dates)
are recorded as namespaced `Layer.Properties` (`png:author`, `xmp:dc:creator`,
…) — never translated, kept for inspection. This mirrors the OOXML reader's
treatment of `docProps/core.xml` (translatable Dublin-Core fields become blocks;
the rest stays skeleton), generalized to formats whose round-trip is a byte copy.
The image reader reads PNG text chunks (`tEXt`/`iTXt`/`zTXt`) and embedded XMP
(PNG and JPEG `dc:title`/`dc:description`/`dc:subject`/`dc:creator`) without
loading the pixel data — it stops scanning at the first image-data chunk. The
same `core/docmeta` path carries the PDF Info dictionary
([AD-028](028-pdf-reader-plugin.md)).

Scope: extraction surfaces metadata for translation, TM, and inspection. Whether
the *localized* metadata is re-embedded depends on the writer — a skeleton-based
format (OOXML) re-applies the translated field, and a cross-format conversion
(PDF → Markdown/HTML) carries the metadata blocks into the output document. The
byte-copy image writer preserves the source image's *original* embedded metadata
unchanged; re-encoding localized PNG text chunks / XMP back into the raster, like
binary EXIF/IPTC parsing, is a documented follow-up.

Two config toggles gate the enrichment, both default-on:

- `ocr` — run in-image text recognition (requires the plugin). Off → Media only.
- `layout` — run ML layout when OCR runs; off → geometric structure (tier 2).

### kapi-vision — out-of-core, path-based

The plugin is its own Go module (`plugins/vision`), isolated so its cgo +
onnxruntime stack never enters another build graph. Its engine has two builds,
like `kapi-sat`: a default pure-Go stub (so the module and the protocol/algorithm
tests build with no native dependency) and the real `-tags onnx` engine. The
host-side `vision` engine (`cli/vision_plugin.go`) discovers and spawns the
plugin and drives it over `visionproto` (a length-prefixed binary frame protocol,
not line-JSON — image references and structured results), mirroring the wire
structs rather than importing the plugin module.

`core/vision` is the framework seam: an `Engine` (OCR) interface + an optional
`LayoutEngine` interface (type-asserted, so OCR-only backends need not implement
it) + a name-keyed registry, exactly like `core/segment`. Both methods are
**path-based**.

### OCR — PP-OCRv5

The OCR engine runs the PP-OCRv5 mobile detection (DBNet) and recognition
(CRNN+CTC) models: it builds an MCID-free pipeline — binarize the detection
probability map, extract connected-component boxes, "unclip" them, recognize each
crop and CTC-decode against the PP-OCRv5 dictionary. Recognized lines carry
top-left pixel geometry; the image reader feeds them to the geometric tier-2
(`core/structure.Analyze`) when layout is unavailable.

### Layout — PP-DocLayoutV3 (tier 3)

The layout engine runs PP-DocLayoutV3, an RT-DETR detector. RT-DETR is NMS-free:
given PaddleDetection's `image` / `scale_factor` / `im_shape` inputs it returns
already-decoded detections in original pixel coordinates. Its 25 region classes
map to content roles (`doc_title`→title, `paragraph_title`→heading, `table`→table,
`figure`/`chart`/`image`→picture, formulas, footnotes, headers/footers, …). A
deterministic column-clustering heuristic assigns reading order. The image reader
then **assigns OCR lines to layout regions** by containment and emits role-tagged
blocks in reading order — tier-3 structure — with the geometric tier-2 as
fallback. A `table` region's lines are reconstructed into row/column **cell
structure** (`table` → `table-row` → `table-cell`/`table-header`) by reusing the
tier-2 grid clustering (`structure.Gridify`), so both tiers emit tables
identically (`structure.TableToParts`) and writers render a real table.

### Model distribution

OCR's models are small (~21 MB) and **bundled** in the release tarball beside the
binary (resolved with no configuration), with onnxruntime 1.25.0 (matching
`yalue/onnxruntime_go`'s C API). The layout model is large (~132 MB) and
**download-on-demand** to the XDG cache on first use. Model resolution searches an
override dir, the bundled dir beside the binary, then the cache; downloads of
on-demand models go to the writable cache. All models are Apache-2.0, mirrored on
a neokapi release asset with pinned hashes. The plugin is **not** a `kapi-cli`
dependency — vision is opt-in (`kapi plugins install vision`).

### Browser Vision Lab

The docs **Vision Lab** (`/lab/vision`) runs the *same* PP-OCRv5 and
PP-DocLayoutV3 ONNX models in the browser via **onnxruntime-web** — the ML is the
real model, not a mock; only the runtime differs (the native plugin's cgo
onnxruntime can't compile to wasm). The deterministic pre/post-processing
(`packages/kapi-playground/src/visionBridge.ts`) is a faithful TS port of the Go
pipeline in `plugins/vision/internal/ocr`, kept in lockstep with it. Loading is
tiered: OCR (~21 MB) on first use, layout (~132 MB) only on opt-in.

GitHub release download URLs are **CORS-blocked** for browser `fetch()`, so the
models are served **same-origin**: `make fetch-vision-models` stages them into
`web/static/models/vision` at docs build. The docs deploy git-pushes the built
site, so files must stay under Git's 100 MB limit — the OCR models ship whole,
and the ~132 MB layout model is **split into sub-100 MB parts plus a
`<name>.json` manifest**, which the browser fetches and concatenates before
inference (`visionBridge.fetchModel`) — identical bytes to the whole file, just
Pages-safe. So the full lab (OCR + opt-in layout) works same-origin on GitHub
Pages with no external host. `VisionExplorer`'s `modelBase` keeps the source
configurable if a CDN is ever preferred.

## Consequences

- "Image" stays a generic, localizable format; OCR and layout are optional layers
  that degrade gracefully (absent plugin, or toggled off) to whole-image Media.
- The portable `kapi` binary stays pure-Go and small; the onnxruntime stack is
  confined to the plugin, and image bytes never enter the host.
- Tier-3 structure (authoritative roles + reading order) is available for images,
  and — once a page rasterizer is wired — for the PDF tier-3 slot in
  [AD-028](028-pdf-reader-plugin.md), since the vision engine is format-agnostic
  over rasters.
- **Whole-image replacement** is supported end-to-end: the image is emitted as a
  localizable Media, a writer emits localized bytes, pseudo-localization produces
  a visible variant, and the **target-asset** model pairs a source image with its
  per-locale files. `project.ResolveAssetVariants` resolves each locale's target
  path (via the recipe's `target:` template) and reports which variants exist —
  the local counterpart of Bowrain's server-side asset-variant model (AD-007).
  Because kapi cannot regenerate a *real* image localization, a localized variant
  already on disk is **authoritative**: `kapi run`/`kapi merge` keep it rather
  than clobber it by reprocessing the source (`project.IsBinaryAssetFormat`
  gates this for binary-asset formats), while a missing variant falls through to
  the flow to produce a pseudo/copy fallback.

## Related

- [AD-002: Content Model](002-content-model.md) — Media, standoff structure/role
  annotations, the structure stream vision produces
- [AD-021: SaT Segmenter Plugin](021-sat-segmenter-plugin.md) — the precedent for
  an isolated onnxruntime plugin
- [AD-028: PDF Reader and Structure Tiers](028-pdf-reader-plugin.md) — the
  tier-1/2/3 structure model; vision is its tier-3 engine
- [AD-030: Multimodal Extraction and LLM Refinement](030-multimodal-extraction-and-llm-refinement.md)
  — image OCR as one instance of the confidence-gated escalation pattern; the
  handwriting cascade generalized to a configurable multimodal LLM tier across
  audio and video
- [`plugins/vision/`](https://github.com/neokapi/neokapi/tree/main/plugins/vision) —
  plugin module, engine, protocol, and README
