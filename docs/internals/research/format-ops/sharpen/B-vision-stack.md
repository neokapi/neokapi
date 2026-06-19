# DEEP-DIVE B — Vision/OCR/image stack and its implicit "comprehension depth" ladder

Worktree HEAD: `48fae7e8a fix(docs): dedupe vision models to default-locale output (#912)` — caught up to main. Read-only survey.

Bottom line up front: the codebase already carries **two distinct, orthogonal "how deeply do we understand this document" concepts**, both shipped and tested, but neither is named as a maturity axis:

1. **A richness ladder** = which content-model standoff payloads are present: `Media` → plain text → `GeometryAnnotation` → `StructureAnnotation.Role` → table Groups → reading order + `RelationAnnotation`. This is the ladder the user named ("metadata only / OCR text / OCR+structure+geometry"). It maps 1:1 onto the annotation carriers in `core/model/structure.go`.
2. **An authority tier** (Tier 1/2/3, from AD-028) = how that structure was *obtained* — authoritative tags vs geometric heuristic vs ML heuristic. This is provenance/confidence, NOT richness. A format can reach the top richness rung via Tier 1 (authoritative) or Tier 3 (ML guess) — same payloads, different trust.

A new Structure/Geometry axis should **measure the richness ladder** and **carry the authority tier as a confidence qualifier** — do not collapse them.

---

## 1. The content-model carriers (where "depth" is physically stored)

All structural depth rides three **block-scoped standoff annotations** registered on the existing payload registry — additive, no proto/KLF schema change (`core/model/structure.go:1-18`):

- `AnnoStructure = "structure"`, `AnnoGeometry = "geometry"`, `AnnoRelations = "relations"` (`structure.go:21-29`).
- `StructureAnnotation{Role, Layer, Visibility, Level, ReadingOrder}` (`structure.go:93-107`).
- `GeometryAnnotation{Page, BBox Rect, Resolution, Origin, Z, SourceRef, Glyphs []GlyphBox}` (`structure.go:127-149`); `Rect{X,Y,W,H}` (`structure.go:114-119`); `GlyphBox{Text, BBox}` (`structure.go:153-156`).
- `RelationAnnotation{Relations []Relation}`, `Relation{Type, Target}` (`structure.go:165-176`).

Normalized open vocabularies (the canonical sets, aligned to DocLang/DoclingDocument):
- **Roles** (`structure.go:34-52`): `paragraph title heading caption footnote list list-item table table-cell table-header code formula picture page-header page-footer form-field section`.
- **Layout layers / PLANE axis** (`structure.go:61-67`): `body furniture background overlay metadata`.
- **Visibility / presence-condition axis** (`structure.go:73-79`): `"" conditional hidden print-only screen-only`.
- **Relation types** (`structure.go:83-89`): `caption-of footnote-of label-for triggers references`.

Above blocks, structural containment is carried by **Groups** (`table` / `table-row` / `picture` / `list`) emitted as `PartGroupStart/End`. The whole-image asset itself is a `model.Media` part (`PartMedia`) — depth zero, the unit a flow replaces wholesale.

`GeometryAnnotation` is explicitly "read-only display/reconstruction metadata — native format writers ignore it; only the visual layout view and layout-target/DocLang serializers consume it" (`structure.go:121-126`). `Resolution` names a normalized grid (DocLang uses 512), `0` = absolute px/points (`structure.go:132-134`).

---

## 2. The TWO existing depth vocabularies

### 2a. Authority tier (AD-028, the named ladder)

`web/docs/contribute/architecture/028-pdf-reader-plugin.md:131-179`, "Structure tiers … in decreasing order of authority":

| Tier | Source | Where | Authority |
|---|---|---|---|
| **1 — Tagged struct tree** | PDF's own logical tree (Document › H1 › P › Table › TR › TH/TD) | Native plugin only | Authoritative (author's tags) |
| **2 — Geometric inference** | Row clustering, column alignment, line height | Native **and** browser | Heuristic |
| **3 — ML layout** | A vision model over the rendered page | Native plugin + host (kapi-vision) | Heuristic, highest recall |

Tier-2 is `core/structure.Analyze`/`ToParts` (format-agnostic, `analyze.go:1-12`). Tier-3 runs the kapi-vision layout model over a page raster and is "deliberately *not* part of the PDF format … applies to any format that can produce [raster+blocks]" (AD-028:163-179). Tiers degrade cleanly downward (3→2→1-absent). This is the ONLY place the word "tier" is a defined concept — and it is about *provenance/authority*, not feature richness.

### 2b. Localization-mode table (AD-029, a capability enumeration)

`web/docs/contribute/architecture/029-vision-and-image-localization.md:36-42` — "treating 'image' as 'OCR' conflates [several modes]":

| Mode | What it localizes | Mechanism |
|---|---|---|
| Whole-image replacement | the pixels | per-locale image file swaps the source |
| Alt-text / caption | accessible text | caption Block (`RoleCaption` + `RelCaptionOf`) |
| Metadata | embedded title/desc/keywords | `core/docmeta` → metadata-plane Blocks |
| In-image text (OCR) | text rendered into the image | extract → translate → re-render |
| Layout / structure | the document's regions | regions + reading order + table cells |

These are **capabilities/what-we-localize**, partially overlapping the richness ladder (OCR→Layout is the richness tail; whole-image/alt/metadata are separate localization targets, not richness rungs). Two default-on toggles gate enrichment (AD-029:119-122; `image/Config` at `core/formats/image/reader.go:46-55`): `ocr` (off → Media only), `layout` (off → tier-2).

### 2c. The IMPLICIT richness ladder (not named anywhere, but the real "comprehension depth")

Reconstructed from what each code path actually emits. Each rung adds one payload:

| Rung | Name | Payloads present | Emitted by (evidence) |
|---|---|---|---|
| **R0** | Opaque asset | `model.Media` only | image w/ `ocr=false` or no plugin (`image/reader.go:195-205, 226`) |
| **R0′** | Metadata | + metadata-plane Blocks (`LayerMetadata`) + namespaced `Layer.Properties` | `core/docmeta`; image (`reader.go:177`, `metadata.go`), PDF Info dict (AD-028:110-116) |
| **R1** | Plain text | text Blocks, no geometry | PDF fast path = "one plain-text Block per page" (AD-028:101-104) |
| **R2** | Positioned text | + `GeometryAnnotation{BBox[, Glyphs]}` | PDF geometry path (AD-028:105-108); `BlocksFromOCR` (`vision.go:166-183`); wasm rects |
| **R3** | Roles + plane | + `StructureAnnotation.Role/Level/Layer` | tier-2 prose (`analyze.go:336-341`); docling/doclang role maps |
| **R4** | Tables / containment | + `table`/`table-row` Groups + `table-cell`/`table-header` roles | `structure.Gridify`/`TableToParts` (`analyze.go:222-260, 375-398`); docling cell grid; doclang OTSL |
| **R5** | Reading order + relations | + reading-order sort + `RelationAnnotation` (caption-of), full plane/visibility | tier-3 `PartsFromLayout` (`layout.go:121-157`); docling `$ref` body tree; doclang doc order |

This R0–R5 ladder is the thing the user described and is what a Structure/Geometry maturity axis should formalize. It is currently expressed only as scattered code branches.

---

## 3. The vision stack (kapi-vision plugin + core/vision seam)

### Framework seam — `core/vision/`
- `Engine` interface = OCR only, path-based: `OCR(ctx, imagePath string, opts OCROptions) (*OCRResult, error)` + `Close()` (`vision.go:52-58`). Path-based "by design: the host (kapi) must never load a large image into memory" (`vision.go:48-51`).
- `OCRLine{Text, BBox model.Rect, Confidence}` (`vision.go:24-28`), `OCRResult{Lines, Width, Height}` (`vision.go:32-36`).
- `LayoutEngine` is an **optional, type-asserted** capability — `Layout(ctx, imagePath, opts) ([]Region, error)` (`layout.go:43-45`); "OCR-only backends (and the non-ONNX stub) need not implement it" (`layout.go:40-42`). `Region{Role, BBox, ReadingOrder, Confidence}` (`layout.go:23-28`).
- Registry mirrors `core/segment`: `RegisterEngine`/`Available`/`Open` (`vision.go:78-120`). Absent plugin ⇒ `ErrNoEngine` (`vision.go:66`) ⇒ image silently degrades to R0.

### Plugin — `plugins/vision/` (own Go module, cgo `-tags onnx`)
- `manifest.json`: ops `["ping","info","ocr","layout"]`; engine `ppocrv5-mobile`; models = `layout` (`ppdoclayoutv3.onnx`, `on_demand:true`), `det` (`ppocrv5_det.onnx`), `rec` (`ppocrv5_rec.onnx`, default).
- Internals: `internal/ocr/{engine.go, engine_onnx.go, engine_stub.go, algo.go, layout_onnx.go, layoutmap.go}`, `internal/models/`.
- README/AD-029:141-162: **OCR** = PP-OCRv5 mobile DBNet detection + CRNN+CTC recognition (shipped v0.1.0); **Layout** = PP-DocLayoutV3 (RT-DETR, NMS-free), tier-3 (shipped v0.2.0).
- Layout class→role map (`internal/ocr/layoutmap.go`): 25 PP-DocLayoutV3 labels (`doc_title paragraph_title abstract content text table figure_title chart image header footer footnote display_formula inline_formula seal algorithm …`) → roles `title/heading/paragraph/table/caption/picture/code/formula/footnote/page-header/page-footer`; unmapped ⇒ paragraph (`layoutRole`).
- **No TrOCR / handwriting-recognition / OCR-cascade in this HEAD.** Grep across `core plugins cli web/docs` finds zero "trocr"/"handwriting OCR"/"cascade" for vision. ("handwriting" appears only as a DocLang *inline formatting category* in `core/formats/doclang/testdata/conformance/doclang.xsd:28,33,45`; "cascade" only in SRX/DTCG/openxml/wiki contexts.) The handwriting-cascade named in the survey brief is NOT present at `48fae7e8a` — treat it as a not-yet-landed PR.

### Where OCR depth is chosen at runtime — `image/reader.go:305-336` (`ocrParts`)
```
res, err := eng.OCR(...)                                  // R2: OCR lines + geometry
if le, ok := eng.(vision.LayoutEngine); ok && useLayout { // R5: tier-3 path
    regions := le.Layout(...); regions = vision.SortReadingOrder(regions)
    if parts := vision.PartsFromLayout(regions, res, ...); len(parts) > 0 { return parts }
}
blocks := vision.BlocksFromOCR(res, 1, &counter)          // R3/R4: tier-2 fallback
return structure.ToParts(structure.Analyze(blocks), ...)
```
So a single format (`image`) **spans R0→R5 at runtime**, gated on (a) plugin installed (`vision.Available("")`, `reader.go:226`), (b) `ocr` toggle, (c) `layout` toggle + engine implementing `LayoutEngine`. The depth of an image is therefore *not a static property of the format code* — it is environment-conditional. This is the single most important fact for a Structure/Geometry axis: for `image`/`pdf`, the achievable rung depends on installed plugins.

`PartsFromLayout` (`layout.go:121-157`): assigns OCR lines to regions by center-containment, emits regions in reading order, table regions reconstructed via `structure.Gridify` → `structure.TableToParts` (`layout.go:188-210`), unassigned lines appended as trailing paragraphs (nothing dropped). `StructureFromLayout` (`layout.go:172-183`) is the same path for any raster+blocks source (the PDF tier-3 reuse), backed by `OCRResultFromBlocks` (`vision.go:142-159`) so a vector PDF's own text fills ML-detected regions rather than re-OCRing.

---

## 4. Per-format placement on the R0–R5 ladder TODAY

### `image` (`core/formats/image/`) — spans R0–R5, plugin-conditional
- **Always (pure-Go, no plugin):** R0 Media (`reader.go:195-205`) + R0′ metadata (PNG tEXt/iTXt/zTXt + XMP DC fields, read without loading pixels — stops at IDAT/SOS; `metadata.go:25-78,140-192`) + alt-text caption Block `RoleCaption`+`RelCaptionOf` (`reader.go:211-216`). Writer folds localized alt-text back to a per-locale `.alt.txt` sidecar (AD-029:68-81).
- **With kapi-vision + `ocr=true`:** R2 (OCR lines+geometry) → R3/R4 via `structure.Analyze` (tier-2) → **R5 via PP-DocLayoutV3 tier-3** when `layout=true` (roles + reading order + table cells).
- Default config is `{OCR:true, Layout:true}` (`reader.go:57`) — i.e. aims for the top rung when the plugin is present, R0 when absent.
- It is the **only** `IsBinaryAssetFormat` (`core/project/asset.go:16-23`): an on-disk localized variant is authoritative; `kapi run`/`merge` won't clobber it. So R0 (asset replacement) is a first-class localization mode, not a degenerate failure.

### `pdf` (`core/formats/pdf/` + `plugins/pdfium/`) — spans R0′–R5, the ONLY format reaching Tier 1
- **Native:** in-core reader is a **no-op** (`register_pdf_other.go`); the format is supplied entirely by the `kapi-pdfium` plugin at runtime. Modes (AD-028:97-108): fast path = R1 (one text Block/page); geometry path = R2 (Block/run + `GeometryAnnotation`, +`Glyphs` with `glyphs=true`). Plus R0′ Info-dict metadata.
- **Structure:** Tier 1 = tagged struct tree (`plugins/pdfium/internal/pdfreader/structtree.go:17-30`, MCID→text/bbox walk, experimental PDFium APIs, runtime-gated; falls to tier-2 if absent) → Tier 2 `structure.Analyze` → Tier 3 (`tier3` option renders page→PNG@72dpi marked `vision.PageRasterProperty`; host decorator `cli/pluginhost/tier3_reader.go` + `format_factory.go:68-73` runs `vision.StructureFromLayout`, strips request if kapi-vision absent ⇒ clean fall to tier-2).
- **Browser** (`pdf/wasm_bridge.go`): extract → rects+geometry (R2) → **always tier-2** `structure.ToParts(structure.Analyze(blocks))` (`wasm_bridge.go:212-221`). No tier-1 (struct tree not exposed by the JS `extract()` contract), no tier-3 (no native vision) ⇒ documented native/browser asymmetry (AD-028:181-185).
- So PDF reaches every richness rung R3–R5 and is the **sole holder of Tier-1 authority** (author-fidelity structure). It reaches R5 by two different authority tiers (Tier-1 tagged tree OR Tier-3 ML), illustrating richness⊥authority.

### `docling` (`core/formats/docling/`) — top rung R5, structure-only (no pixels), always-on
- Reads **DoclingDocument JSON** = a fully pre-digested structure (the deepest *input*). Read-only, pure-Go, no plugin, no toggles (`config.go` empty). Sniff = `"schema_name"` + `DoclingDocument` (`reader.go:100-103`).
- Carries: **Roles** (`labelRole` map, `reader.go:40-54`: title/section_header/paragraph/list_item/caption/footnote/page_header/page_footer/code/formula), **heading level** (`emitText`:218-222), **layout layer** (page_header/footer → `LayerFurniture`, `reader.go:58-61,224-226`), **geometry** (`geometryFromProv` → `GeometryAnnotation{Page,BBox,Origin,SourceRef}`, honoring TOPLEFT/BOTTOMLEFT `coord_origin`, `reader.go:402-424`), **reading order** (walks `body.children` `$ref` tree, `reader.go:158-168`), **full tables** (cell grid with `start/end row/col offset`, `row_span`/`col_span`, header flags → table/table-row/cell groups, `schema.go:44-77`, `reader.go:268-311`), **pictures** as `picture` Groups w/ captions (binary NOT ingested, `reader.go:315-325`).
- Limitation pinning it just below "richest possible": `TextItem.text` is plain → "each text item yields a single text run" (no inline formatting; `reader.go:18-20`).

### `doclang` (`core/formats/doclang/`) — top rung R5 + inline formatting + roundtrip
- Reads/writes **DocLang XML v0.6** (LF AI & Data standard, the standardized serialization of Docling output; `reader.go:1-19`). Pure-Go, always-on, has a **writer** (faithful DocLang↔DocLang + native→DocLang projection) — unique among the four.
- Carries everything docling does, **plus inline formatting** as Pc runs (`fmtTag` map: bold/italic/underline/strikethrough/superscript/subscript → `fmt:*` PcOpen/PcClose, `reader.go:62-69,277-290`) — richer text model than docling. Roles `blockRole` (`reader.go:39-47`), layout layer (`<layer value=>`, `reader.go:261-265,310-312`), geometry (`<location>` 4-value block → `GeometryAnnotation`, **Resolution defaults to 512** = normalized grid, `geometryFrom` `reader.go:502-516`), tables via OTSL token state machine (`fcel/ched/rhed/ecel/srow/corn` → cell roles, `nl` row terminator; `otslCellTok` `reader.go:72-79`, `parseTable` `reader.go:324-411`).
- Declared not-yet-mapped read-through (still R5-class but lossy at edges): OTSL span continuations `lcel/ucel/xcel` (spanned cells dropped), picture/field/marker/checkbox, thread/xref/href continuity (`reader.go:14-19`).

### Summary placement table

| Format | Reaches | Asset pixels (R0) | Authority tier | Plugin-gated? | Writer? |
|---|---|---|---|---|---|
| `image` | R0–R5 | yes (the file IS the unit) | T2 (geom) or T3 (ML) | yes (kapi-vision) | yes (bytes + alt sidecar) |
| `pdf` (native) | R0′–R5 | no | **T1** tagged, else T2, else T3 | yes (kapi-pdfium [+kapi-vision]) | no (read-only) |
| `pdf` (browser) | R0′–R3/R4 | no | **T2 only** | wasm bridge | no |
| `docling` | R5 | no (pictures binary skipped) | n/a (given) | no | no (re-emit via doclang) |
| `doclang` | R5 (+inline fmt) | no (picture skipped) | n/a (given) | no | **yes** (faithful roundtrip) |

---

## 5. How tiers are tested (nightly, #905/#906)

`.github/workflows/nightly.yml`:
- **`vision-onnx` job** (L69-130): real PP-OCRv5 + PP-DocLayoutV3 models + real onnxruntime 1.25.0 (cached by version/tag). Gated behind `-tags onnx` + `KAPI_VISION_ORT_LIB` + `KAPI_VISION_MODELS_DIR`; downloads `ppocrv5_det.onnx ppocrv5_rec.onnx ppocrv5_dict.txt ppdoclayoutv3.onnx` (SHA-256 pinned). Runs `cd plugins/vision && CGO_ENABLED=1 go test -tags onnx -v ./internal/ocr/...` (`GOWORK=off`). Validates **OCR + layout model inference directly** (R2→R5 numeric path the per-PR fake-engine suite can't reach).
- **`vision-pdf-e2e` job** (L132-195+): builds real `kapi` + `kapi-pdfium` + `kapi-vision`, installs them, reads a real PDF with `tier3` on so pdfium renders pages and vision runs layout over the raster — the **cross-process Tier-3 wiring** the smoke lane can't cover. Reuses the onnxruntime/models caches; caches `libpdfium` (`chromium/7891`).
- Per-PR suite uses **fakes** (`vision.ResetForTest`, `vision.go:124-129`); the OCR/structure algorithms are pure-Go and tested without native deps (`core/structure/analyze_test.go`, `core/vision/*_test.go`, `cli/pluginhost/tier3_reader_test.go` incl. `TestEnrichTier3_FallbackTier2`).
- Release lanes: `.github/workflows/release-vision.yml`, `release-sat.yml`, `release-check.yml` (the three sibling onnxruntime plugins).
- Note (AD-028:163 / nightly L75-77): a full render→layout PDF e2e is "a planned extension" — the e2e job is the first cut.

---

## 6. Recommendation for the framework sharpening (the deliverable question)

A new **Structure/Geometry axis should DEFINE the R0–R5 richness ladder** (it does not exist as a named concept) and **REUSE the existing Tier-1/2/3 vocabulary as an orthogonal authority/confidence qualifier** (it already exists, AD-028). Concretely:

- The axis levels should be the payload-presence ladder: **G0 opaque-asset/Media → G0′ metadata → G1 plain text → G2 positioned text (`GeometryAnnotation`) → G3 roles+plane (`StructureAnnotation`) → G4 tables/containment (Groups) → G5 reading-order+relations (`RelationAnnotation`, reading order)**. These are directly measurable from the emitted Part stream (which annotations/groups appear) — no new model needed.
- Each level above G2 also carries an **authority tag** (Tier 1 authoritative / Tier 2 geometric-heuristic / Tier 3 ML-heuristic / "native" when the source format declares it, e.g. docling/doclang). Same richness, different trust — do not merge into one number.
- The axis must be **capability-conditional, not static**, for `image`/`pdf`: score the *ceiling* the format+plugin can reach and the *floor* it degrades to (R0/R1), because the rung is environment-dependent (plugin install + `ocr`/`layout`/`tier3` toggles).
- `docling`/`doclang` are the **reference top-rung** (G5, native authority) and the natural conformance oracles for the role/layer/geometry vocabularies — every other format's structural output is expressed in the *same* `model.Role*`/`Layer*` vocabulary (`structure.go:34-67`) precisely so a reader, the editor, an exporter, and a DocLang writer "all speak the same role names" (`structure.go:31-33`). The axis should reuse that vocabulary as its rubric, not invent a parallel one.
- Note the AD-029 mode table (whole-image / alt / metadata / OCR / layout) is a **localization-capability** enumeration that partly overlaps but is not identical to the richness ladder; keep it as a separate "what can we localize" capability list (it includes R0 asset-replacement and alt-text/metadata which are localization *targets*, not comprehension depth).

Key files: `core/model/structure.go`, `core/structure/analyze.go`, `core/vision/{vision.go,layout.go}`, `core/formats/{image,pdf,docling,doclang}/`, `plugins/vision/` (manifest + `internal/ocr/layoutmap.go`), `plugins/pdfium/internal/pdfreader/structtree.go`, `cli/pluginhost/tier3_reader.go`, AD-028 `web/docs/contribute/architecture/028-pdf-reader-plugin.md`, AD-029 `029-vision-and-image-localization.md`, `.github/workflows/nightly.yml`.
