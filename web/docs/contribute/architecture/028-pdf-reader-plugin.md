---
id: 028-pdf-reader-plugin
sidebar_position: 28
title: "AD-028: PDF Reader and Structure Tiers"
description: "Architecture decision: PDF is read out-of-core by PDFium — a native cgo plugin (kapi-pdfium) and a browser PDFium-WASM bridge — emitting positioned text with per-block and per-glyph geometry, and recovering document structure through a tiered model (tagged struct tree, geometric inference, and a future ML layout tier)."
keywords: [PDF, PDFium, kapi-pdfium, geometry, glyphs, tagged PDF, structure tree, table detection, WebAssembly, plugin, architecture decision, neokapi]
---

# AD-028: PDF Reader and Structure Tiers

## Summary

PDF is read **out-of-core** by [PDFium](https://pdfium.googlesource.com/pdfium/),
Google's PDF engine, through two backends that share their content-producing
logic:

- **Native** — `kapi-pdfium`, a first-party Mode-C plugin
  ([AD-007](007-plugin-system.md)) linking PDFium via cgo (go-pdfium) and running
  as an isolated daemon, so a malformed-PDF crash dies with the subprocess rather
  than `kapi`.
- **Browser** — a `WasmReader` (build-tagged `js`) that bridges Go-WASM to a
  PDFium **WebAssembly** module loaded by the web app, giving the in-browser Lab
  the same extraction without a server.

Both emit the same `Part` stream: positioned text Blocks carrying a
`GeometryAnnotation` (bounding box, optionally per-glyph boxes), grouped into
lines by a shared algorithm, with document structure (headings, paragraphs,
tables) recovered through a **tiered model** — an authoritative tagged-PDF
structure tree where available, geometric inference otherwise, and a future ML
layout tier. The hand-rolled pure-Go PDF reader that once lived in `core/formats/pdf`
has been retired; PDFium is the only PDF path on every platform.

## Context

A faithful PDF reader needs more than byte extraction. PDFs encode text with
font-program glyph indices (CID/Type0, common for CJK) that a naive scanner
garbles; correct extraction requires a real font/encoding engine. PDFium is that
engine, but it is a large C++ codebase with two consequences the framework must
contain:

- **Native weight.** Linking PDFium via cgo defeats pure-Go cross-compilation and
  inflates every `kapi` install for a capability many invocations never use — the
  same isolation rationale as the Okapi bridge and the SaT segmenter
  ([AD-021](021-sat-segmenter-plugin.md)). PDF therefore lives in a plugin, not in
  the framework.
- **Crash surface.** Malformed PDFs are a classic parser-crash vector. Running the
  reader in a subprocess daemon contains a crash to that process.

Beyond text, the visual editor ([AD-027](027-visual-editor-data-model.md)) and
the browser Lab need each text fragment's **position on the page**, and
downstream localization wants **document structure** — which lines are headings,
which blocks form a table — so that a translated document can be reflowed and so
that table cells are translated as cells. PDF carries none of this uniformly:
some PDFs are *tagged* with an explicit logical structure tree; most are not, and
structure must be inferred from geometry.

## Decision

### Two backends, one content contract

PDF reading is registered through a build-split so the right backend is wired per
target and the framework core stays free of PDFium:

- `core/formats/register_pdf_js.go` (`//go:build js`) registers the browser
  `WasmReader`.
- `core/formats/register_pdf_other.go` (`//go:build !js`) is a no-op: native
  builds have **no** in-core PDF reader, so the `pdf` format is supplied only by
  the installed `kapi-pdfium` plugin at runtime (or is absent with an actionable
  "install the plugin" error).

The native plugin is its own Go module, `github.com/neokapi/neokapi/plugins/pdfium`,
outside the workspace so its cgo/PDFium dependency never enters another module's
build graph:

```
plugins/pdfium/
├── go.mod                         module …/plugins/pdfium (replace … => ../..)
├── manifest.json                  Mode-C daemon manifest (declares the pdf format + schema)
├── formats/pdf/schema.json        config schema: geometry, glyphs
├── cmd/kapi-pdfium/main.go        daemon entry point (gRPC, Mode C)
└── internal/pdfreader/
    ├── reader.go                  ReadParts: fast path + geometry path
    └── structtree.go              tier-1 tagged struct tree (build-tag-free; runtime-gated)
```

Both backends produce an identical `Part` stream — a document `Layer`, per-page
`Layer`s, and Blocks — and share two pieces of framework logic so the native and
browser results match:

- **`core/formats/pdf.GroupRuns`** merges PDFium's per-rect fragments (it emits a
  fresh rect at every mid-line font/style change) into line-level runs, splitting
  only at large horizontal gaps (a column or cell boundary). Without it a styled
  or isolated glyph becomes a one-character Block.
- **`core/structure.Analyze` / `ToParts`** infer tier-2 structure (below).

### Two extraction modes

The reader has two granularities, selected by the `geometry` config flag
(`formats/pdf/schema.json`):

- **Fast path** (`geometry=false`, the default) — one plain-text Block per page.
  Fewest allocations, no positional work; the right choice for batch text
  operations (word counts, leverage, the `kgrep`/`kcat` toolbox utilities,
  [AD-023](023-toolbox-utilities.md)).
- **Geometry path** (`geometry=true`) — one Block per positioned text run, each
  carrying a `GeometryAnnotation`. With `glyphs=true` (implies geometry) each
  Block additionally carries per-character boxes for character-precise
  highlighting.

### Geometry model and coordinate flip

PDFium reports boxes in PDF user space, where the origin is **bottom-left** and Y
increases upward. neokapi's content and editor model use a **top-left** origin (Y
increases downward), matching screen and most document coordinate systems. Every
box is flipped once, at extraction, when the page height is known:
`Y_top-left = pageHeight − Y_upper`. The flip is implemented identically at all
three sites — the native fast/geometry reader (`geometryFromRect`), the native
tagged-tree reader (`treeWalker.flip`), and the browser bridge (`flipBox`) — and
each stamps `Origin: "top-left"` (or falls back to `"bottom-left"` when the page
height is unavailable). `GeometryAnnotation` carries the page number, the union
`BBox`, the origin, and the optional `[]GlyphBox` (text + box per character).

### Structure tiers

Document structure is recovered through three tiers, in decreasing order of
authority:

| Tier | Source | Where it runs | Authority |
|---|---|---|---|
| **1 — Tagged struct tree** | The PDF's own logical structure tree (Document › H1 › P › Table › TR › TH/TD …) | Native plugin only | Authoritative (the author's own tags) |
| **2 — Geometric inference** | Block positions: row clustering, column alignment, relative line height | Native **and** browser | Heuristic |
| **3 — ML layout** | A vision model over the rendered page | Future ("kapi vision" plugin) | Heuristic, highest recall |

**Tier 1** (`internal/pdfreader/structtree.go`) reads a tagged PDF's structure
tree directly: it builds a marked-content-ID → text/bounds map from the page's
text objects, walks the struct tree, and maps elements onto the content model —
`Table` → a table group of rows and cells (TH → header, TD → cell), `H1`–`H6` →
headings, `P` and friends → paragraphs. The marked-content accessors are PDFium
*experimental* APIs that go-pdfium wires only under the `pdfium_experimental`
build tag (the bundled libpdfium exports the symbols), so the shipped plugin is
built with that tag. The code itself carries **no** build tag: it compiles either
way and, when the experimental API is unavailable, returns "no tagged structure"
so the reader falls through to tier 2. Untagged PDFs fall through the same way.

**Tier 2** (`core/structure.Analyze`) is format-agnostic: it consumes Blocks
carrying geometry and clusters them into rows, detects tables where cells align
into stable columns across consecutive rows, and tags remaining prose as heading
or paragraph by relative line height. `ToParts` emits the result as the same
table/heading/paragraph `Part` stream the
[docling reader](002-content-model.md) produces, so the existing Markdown and
HTML writers render real tables with no PDF-specific code. Both backends run
tier 2 as their fallback.

**Tier 3** is the planned third tier: a reusable, cross-format **vision** plugin
running a layout model over the rendered page for the cases geometry cannot
reach — borderless tables, multi-column reading order, scanned pages, figure and
caption association. It is deliberately *not* part of the PDF format: it operates
on a page raster plus blocks and so applies to any format that can produce them.
It is not yet built.

A consequence of this design is a **native/browser asymmetry**: the browser
`extract()` contract exposes only rects, not the struct tree, so the in-browser
Lab always uses tier 2 even for a tagged PDF, while a native `kapi` run uses
tier 1. The structure of a tagged PDF can therefore differ between the Lab and the
CLI; this is documented at the call site.

### Distribution

`kapi-pdfium` bundles the PDFium **shared** library at `lib/<name>` beside the
binary in the release tarball, found via an rpath baked into the binary
(`@loader_path/lib` on macOS, `$ORIGIN/lib` on Linux, same-dir on Windows) — the
same bundling shape as the SaT plugin's ONNX runtime
([AD-021](021-sat-segmenter-plugin.md)), not a static link. Tarballs are built
per platform on native runners (cgo, `-tags pdfium_experimental`,
PDFium pinned to a bblanchon release), cosign-signed, and indexed in the registry,
so `kapi plugins install pdfium` verifies the download like any other plugin. The
plugin is bundled with the kapi CLI (Homebrew `kapi-cli` depends on `kapi-pdfium`)
and the desktop app installs it on demand the first time a PDF is opened; the
browser self-hosts PDFium's WASM next to the kapi WASM.

## Consequences

- The portable `kapi` binary stays pure-Go, small, and cross-compilable; the
  PDFium native stack is confined to a separately built, separately installed
  plugin, and a malformed PDF can crash only the daemon.
- Native and browser produce the same positioned-text `Part` stream because they
  share `GroupRuns` and `structure.Analyze`/`ToParts`; the only intended
  divergence is tier-1 structure, which the browser cannot see.
- Geometry is available for the visual editor and Lab without changing the
  content model — it rides on the standoff `GeometryAnnotation`, off by default so
  batch text work pays nothing for it.
- Tagged PDFs get author-fidelity structure for free; untagged ones get a
  best-effort geometric reconstruction; the door is open to an ML tier without
  reworking either backend.

## Related

- [AD-002: Content Model](002-content-model.md) — Blocks, standoff annotations,
  table groups, and the docling-shaped structure stream `ToParts` mirrors
- [AD-007: Plugin System and Okapi Bridge](007-plugin-system.md) — Mode-C daemon
  discovery, native-stack isolation, signed registry distribution
- [AD-021: SaT Segmenter Plugin](021-sat-segmenter-plugin.md) — the precedent for
  isolating a native stack in a bundled-shared-library plugin
- [AD-027: Visual Editor Data Model](027-visual-editor-data-model.md) — how the
  editor consumes geometry and structure
- [`plugins/pdfium/`](https://github.com/neokapi/neokapi/tree/main/plugins/pdfium) —
  plugin module, readers, and README
