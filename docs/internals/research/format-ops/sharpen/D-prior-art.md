# Deep-dive D — Prior art + naming: an intuitive axis categorization and a Structure/Geometry ladder

Read-only survey. Repo: `/Users/asgeirf/src/neokapi/neokapi/.claude/worktrees/format-process` (HEAD caught up to main).
Anchors read in full: `core/model/structure.go`, `core/formats/docling/{schema.go,reader.go,spec.yaml}`, `core/formats/doclang/{reader.go,spec.yaml}`, `core/formats/image/reader.go`, `core/structure/analyze.go`, `web/docs/contribute/architecture/028-pdf-reader-plugin.md`, `web/docs/contribute/architecture/029-vision-and-image-localization.md`, `docs/internals/format-maturity.md`.

---

## 0. Headline finding (the gap this survey closes)

The repo **already contains two orthogonal "depth" ladders the maturity framework never captured**, and a content-model substrate purpose-built to carry them:

1. **AD-028 structure tiers (recovery *authority*)** — tier 1 tagged tree / tier 2 geometric / tier 3 ML. This is a *provenance/confidence* ladder, not a *representation* ladder.
2. **AD-029 image-enrichment ladder (representation *depth*)** — Media-only → alt-text/caption → metadata → OCR text → layout/structure+geometry. This is the exact ladder the user named ("metadata OR OCR-to-text OR OCR+structure+geometry").
3. **`core/model/structure.go`** — three stand-off block annotations (`StructureAnnotation`, `GeometryAnnotation`, `RelationAnnotation`) that can *represent* every rung of #2, deliberately "aligned with the DocLang / DoclingDocument taxonomy" (`structure.go:32-33`).

None of the six maturity axes (Engine L, Vocabulary V, Editor E, Knowledge K, Corpus C, Security S) scores any of this. The vision/OCR/structure stack (#900–#912) landed entirely *underneath* the rubric. The deliverable below proposes a seventh axis **G — Structure & Geometry (a.k.a. "Comprehension depth")** and an intuitive 3-family grouping of all seven.

---

## 1. Docling's layering model — the canonical reference

### 1.1 The DoclingDocument model as neokapi actually consumes it (`core/formats/docling/schema.go`)

The schema-comment states the shape precisely (`schema.go:3-8`):
> "Mirrors the docling-core pydantic model … : **a flat content store (texts/tables/pictures/groups) plus a tree of `$ref` pointers rooted at `body`, walked in reading order.** Unknown fields are ignored …"

Types (file:line):

- **`doclingDoc`** (`schema.go:95-105`) — top level: `SchemaName` ("DoclingDocument"), `Version`, `Name`, **two structural roots** `Body *docNode` + `Furniture *docNode`, and **four flat content stores** `Groups []docGroup`, `Texts []docText`, `Tables []docTable`, `Pictures []docPicture`.
- **`ref`** (`schema.go:11-13`) — a `"$ref"` pointer such as `"#/texts/0"`. The whole tree is `$ref` edges into the flat arrays.
- **`docNode`** (`schema.go:17-20`) — a structural root: `SelfRef` + ordered `Children []ref`. (body / furniture)
- **`docGroup`** (`schema.go:23-28`) — grouping node (list, ordered_list, inline, key-value region): `SelfRef`, `Children`, `Name`, `Label`.
- **`docText`** (`schema.go:33-41`) — text item: `SelfRef`, **`Label`** (the DocItemLabel), `Text`, `Orig`, **`Level`** (for section_header), **`Prov []prov`**, `Children`. Comment enumerates the DocItemLabel set: *"title, section_header, paragraph, list_item, caption, footnote, page_header, page_footer, code, formula"* (`schema.go:32-33`).
- **`docTable`** (`schema.go:44-50`) — `SelfRef`, `Label`, `Prov`, `Captions []ref`, `Data tableData`.
  - **`tableData`** (`schema.go:53-57`) — `NumRows`, `NumCols`, `Cells []tableCell`.
  - **`tableCell`** (`schema.go:61-73`) — `Text`, `RowSpan`, `ColSpan`, `StartRow/EndRow/StartCol/EndCol` offset idx, `ColumnHeader`, `ColHeaderLegacy`, `RowHeader`, `RowSection` (OTSL-derived spanning grid). `isHeader()` = ColumnHeader‖ColHeaderLegacy‖RowHeader (`schema.go:75-77`).
- **`docPicture`** (`schema.go:109-115`) — `SelfRef`, `Label`, `Prov`, `Captions`, `Children`. *Image binary is NOT ingested; only its captions are translatable.*
- **Provenance**: **`prov`** (`schema.go:80-83`) = `PageNo int` + `BBox *bbox`; **`bbox`** (`schema.go:86-92`) = `L,T,R,B float64` + **`CoordOrigin string`** ("TOPLEFT" or "BOTTOMLEFT").

**The five canonical Docling layers (as modeled here):**

| # | Layer | In `schema.go` |
|---|---|---|
| 1 | Document + structural roots (body, furniture) | `doclingDoc.Body/Furniture` (95-105), `docNode` (17-20) |
| 2 | Reading-order tree (`$ref` children walk) | `ref` (11-13), `docNode.Children` |
| 3 | Content items by kind (texts / tables / pictures / groups) | flat arrays (101-105) |
| 4 | Logical role per item (DocItemLabel) + level | `docText.Label/Level` (33-41) |
| 5 | Spatial provenance (page + bbox + origin) + OTSL spanning grid | `prov`/`bbox` (80-92), `tableCell` (61-73) |

### 1.2 How the reader projects those five layers onto neokapi (`core/formats/docling/reader.go`)

- **Reading-order walk**: `readContent` walks `doc.Body` then `doc.Furniture` children (`reader.go:158-168`); `walkRef` resolves a `$ref` to its item kind and dispatches (`reader.go:175-193`); a `visited` map de-dups shared refs.
- **DocItemLabel → SemanticRole** via the `labelRole` map (`reader.go:40-54`): `title→RoleTitle`, `section_header→RoleHeading`, `paragraph/text→RoleParagraph`, `list_item→RoleListItem`, `caption→RoleCaption`, `footnote→RoleFootnote`, `page_header→RolePageHeader`, `page_footer→RolePageFooter`, `code→RoleCode`, `formula→RoleFormula`.
- **Layout plane**: `furnitureLabel` (`reader.go:58-61`) maps page_header/page_footer → `LayerFurniture`.
- **Provenance → geometry**: `geometryFromProv` (`reader.go:402-424`) maps `bbox{L,T,R,B}` → `Rect{X,Y,W,H}`, records `Origin` top-left/bottom-left, and stashes `SourceRef = self_ref` for round-trip/debug.
- **Tables**: `emitTable` (`reader.go:268-311`) → `Group("table")` → `Group("table-row")` → cell Blocks; `isHeader` → `RoleTableHeader` else `RoleTableCell`; cells ordered by (row, col) via `cellsForRow` (`reader.go:387-396`).
- **Pictures**: `emitPicture` (`reader.go:315-325`) → `Group("picture")` carrying caption Blocks only.

The reader doc-comment is explicit that **inline formatting is out of the base model**: *"TextItem.text is plain, so each text item yields a single text run"* (`reader.go:18-20`). → Docling scores **high on structure, V0 on vocabulary** (key orthogonality argument, §4.4).

### 1.3 DocLang XML — the standardized serialization of the same model (`core/formats/doclang/reader.go`)

DocLang is "the standardized XML serialization of Docling's structured output: semantic role + layout layer + page geometry + reading structure" (`doclang/reader.go:6-9`), LF AI & Data standard, namespace `https://www.doclang.ai/ns/v0`, pinned v0.6.

- `blockRole` (`reader.go:39-47`): heading/text/footnote/page_header/page_footer/code/formula → SemanticRole.
- Element-head props `headElem` (`reader.go:56-59`): `label, thread, xref, href, layer, location, caption, custom`.
- **`<layer>` → LayoutLayer** (`parseBlock`, `reader.go:261-265`, default `LayerBody`).
- **`<location>` (4 values + resolution) → GeometryAnnotation** via `geometryFrom` (`reader.go:502-516`): DocLang uses a **normalized 512-edge grid** by default (`res = 512`), `Origin: "top-left"`.
- **OTSL tables** `otslCellTok` (`reader.go:72-79`): `fcel/ecel→table-cell`, `ched/rhed/srow/corn→table-header`; `<nl/>` terminates a row; span continuations `lcel/ucel/xcel` are **dropped** (`reader.go:18-19, 322-323`).
- **Inline formatting** `fmtTag` (`reader.go:62-69`): bold/italic/underline/strikethrough/superscript/subscript → `fmt:*` runs (DocLang carries inline vocabulary that base DoclingDocument JSON does not).

### 1.4 The neokapi substrate that holds all of it (`core/model/structure.go`) — the rung material for §4

Design note (`structure.go:3-18`): structure is two block-scoped **stand-off payloads** on the annotation registry, *strictly additive* (no protobuf/KLF schema change), serialized over every annotation-aware path. Vocabulary alignment is deliberate (`structure.go:30-33`): *"an open but normalized vocabulary … aligned with the DocLang taxonomy so a reader, the editor, an exporter, and a DocLang writer all speak the same role names."*

- Keys: `AnnoStructure="structure"`, `AnnoGeometry="geometry"`, `AnnoRelations="relations"` (`structure.go:21-29`).
- **`StructureAnnotation`** (`structure.go:93-107`): `Role`, `Layer`, `Visibility`, `Level int`, `ReadingOrder int`.
- **Role\*** (`structure.go:34-52`): paragraph, title, heading, caption, footnote, list, list-item, table, table-cell, table-header, code, formula, picture, page-header, page-footer, form-field, section.
- **Layer\* (the PLANE axis)** (`structure.go:61-67`): body, furniture, background, overlay, metadata.
- **Visibility\*** (`structure.go:73-79`): "" (visible) / conditional / hidden / print-only / screen-only.
- **`GeometryAnnotation`** (`structure.go:127-149`): `Page`, `BBox Rect`, `Resolution int` (DocLang 512; 0 = absolute px/pt), `Origin`, `Z int` (stacking), `SourceRef`, **`Glyphs []GlyphBox`** (per-character geometry).
- `Rect{X,Y,W,H}` (`structure.go:114-119`), `GlyphBox{Text, BBox}` (`structure.go:153-156`).
- **`RelationAnnotation`** (`structure.go:165-176`): typed cross-block edges; **Rel\*** (`structure.go:83-89`): `caption-of`, `footnote-of`, `label-for`, `triggers`, `references`.
- Field-like accessors: `SetSemanticRole(role, level)` (`structure.go:210-215`), `SetLayoutLayer` (`227-231`), `SetGeometry` (`255`), `AddRelation` (`264-276`) — these are the **grep floor-signals** for the proposed axis.

### 1.5 External confirmation (brief — the in-repo model is the anchor)

- **DoclingDocument canonical model** (docling-core DeepWiki; docling concepts page): the central Pydantic model = a *unified hierarchical structure* holding content + layout (**bounding boxes for all items**) + metadata; layout analysis produces **Cluster** objects with labels; document item types are text/tables/pictures/formulas; **reading order is kept so downstream tools don't have to guess a page's reading order**; the schema is **versioned with auto-upgrade** of older docs. This matches our 5-layer reading (pages/layout → clusters → items → reading order → provenance bboxes) exactly.
- **Document-AI pipeline tiering** (DeepLearning.AI "From OCR to Agentic Doc Extraction"; LlamaIndex layout-analysis glossary): canonical stages = image normalize/binarize → text-region detection → recognition (OCR) → **layout analysis (regions + region-type + reading order)** → logical structure → tables/captions. Rationale that *is* the axis's reason to exist: *"most text extraction can be destructive — when a document is flattened, structure is lost, columns and rows get mixed together, tables become meaningless floating blobs, captions detach from figures, and reading order becomes unpredictable."* The proposed G axis measures **how far up from the flattened blob a reader climbs.**

Sources:
- [DoclingDocument — docling-core (DeepWiki)](https://deepwiki.com/docling-project/docling-core/2.1-doclingdocument)
- [Docling document concepts](https://docling-project.github.io/docling/concepts/docling_document/)
- [Layout & Table Structure Models (DeepWiki)](https://deepwiki.com/docling-project/docling/4.2-layout-and-table-structure-models)
- [Document AI: From OCR to Agentic Doc Extraction — layout detection & reading order (DeepLearning.AI)](https://learn.deeplearning.ai/courses/document-ai-from-ocr-to-agentic-doc-extraction/lesson/60su3x/layout-detection-and-reading-order)
- [What is Document Layout Analysis? (LlamaIndex)](https://www.llamaindex.ai/glossary/document-layout-analysis)

---

## 2. The structure-tier prior art we already wrote (AD-028 / AD-029)

### 2.1 AD-028 — structure recovery *authority* (provenance ladder, NOT representation)

`028-pdf-reader-plugin.md` "Structure tiers" table (verbatim):

| Tier | Source | Where it runs | Authority |
|---|---|---|---|
| **1 — Tagged struct tree** | The PDF's own logical structure tree (Document › H1 › P › Table › TR › TH/TD …) | Native plugin only | Authoritative (the author's own tags) |
| **2 — Geometric inference** | Block positions: row clustering, column alignment, relative line height | Native **and** browser | Heuristic |
| **3 — ML layout** | A vision model over the rendered page | Native plugin + host (kapi-vision) | Heuristic, highest recall |

And two extraction **granularities** (`geometry` config flag, AD-028 "Two extraction modes"):
- **Fast path** (`geometry=false`, default) — one plain-text Block per page.
- **Geometry path** (`geometry=true`) — one Block per positioned run, each with `GeometryAnnotation`; `glyphs=true` adds per-character boxes.

Key insight for §4: **tier 1/2/3 is the *confidence/provenance* of where roles+geometry come from; the *fast/geometry* split is the *representation depth*.** These are different things and neither is scored today.

### 2.2 AD-029 — the representation-depth ladder (exactly the user's named ladder)

`029-vision-and-image-localization.md` Context table (verbatim modes):

| Mode | What it localizes |
|---|---|
| **Whole-image replacement** | the pixels |
| **Alt-text / caption** | accessible text, not pixels (`RoleCaption` + `RelCaptionOf`) |
| **Metadata** | embedded title/description/keywords (metadata-plane Blocks vs Layer props) |
| **In-image text (OCR)** | text rendered into the image |
| **Layout / structure** | the document's regions + reading order, tables reconstructed to row/col cells |

Two default-on toggles: `ocr` (Media-only when off) and `layout` (geometric tier-2 when off). AD-029 also confirms geometry is **format-conditional**: *"It exists only for formats with intrinsic spatial layout (PDF, PPTX slide coordinates, XLSX cell grid, rendered HTML) or for content ingested from a layout-aware source (Docling/DocLang)"* (`structure.go:121-126`).

### 2.3 The code that realizes both ladders

- **`core/formats/image/reader.go`** — `Config{OCR, Layout}` (`reader.go:46-57`, both default true). `Read` emits Media-always, then metadata blocks (`docmeta.Apply`, `reader.go:177`), then alt-text caption block (`reader.go:211-216`), then `ocrParts`. **`ocrParts`** (`reader.go:305-336`) is the tier ladder in code: tier-3 `LayoutEngine.Layout → SortReadingOrder → PartsFromLayout` (`reader.go:322-329`), falling back to tier-2 `structure.ToParts(structure.Analyze(blocks))` (`reader.go:331-335`).
- **`core/structure/analyze.go`** — the format-agnostic **tier-2** engine. `RegionKind{RegionBlock, RegionTable}` (`analyze.go:25-30`), `Region` (33-39), `Table`/`Cell` (42-51). `Analyze` clusters geometry into rows → tables (`analyze.go:61`), `Gridify` arranges known-table cells (`222`), `ToParts` (`351`) + `TableToParts` (`375`) emit the *same Part stream the docling reader produces* — so geometric tier-2 and ML tier-3 render tables identically (`analyze.go:373-374`).

---

## 3. The current maturity axes (what the grouping must organize) — `docs/internals/format-maturity.md`

Six axes (`format-maturity.md:99-106`); five gate the tier, Security is display-only:

| Axis | Ladder | Measures (verbatim) |
|---|---|---|
| **Engine** | L0–L4 | "Parse/round-trip/parity fidelity and robustness" |
| **Vocabulary** | V0–V3 | "How richly format semantics map into the canonical content-model vocabulary (and back)" |
| **Editor** | E0–E4 | "How close kapi gets to the format's native editing surface" |
| **Knowledge** | K0–K3 | "The spec/learning assets that let a person or model work on the format" |
| **Corpus** | C0–C3 | "Reference files that validate support, with provenance" |
| **Security** | S0–S4 | "Resource-boundedness, fuzzing, and hostile-corpus hardening" |

Design rules that the new axis + grouping must respect:
- **Promise ≠ score** (`format-maturity.md:16-30`): a 3-level *support tier* (the promise, human-gated) vs the *N-axis vector* (the score, recomputed each audit). Governance research codifies "*promises aggregate by minimum; averages are for dashboards only*" (`research/format-ops/followup-maturity-ladder-governance.md:73`).
- **Headline = min over gating axes, never weighted average** (`format-maturity.md:27`).
- **Each axis: own ladder + deterministic file floor + (some) quality dims** (`format-maturity.md:90-97`); model "cannot push any axis above what the files support" (`format-maturity.md:412-413`).
- **`na` is a countersigned state** for inapplicable cells (`format-maturity.md:112-117`) — the mechanism the new axis needs for catalog formats with no geometry.
- Vocabulary is the structural precedent: a per-format `vocabulary.yaml` matrix + `constructs.yaml` registry + grep floor (`format-maturity.md:160-217`). The new axis should mirror this exactly.

Current per-axis distribution (snapshot 2026-06-13, `format-maturity.md:581-586`): Vocabulary is V0:45/V1:4 and Editor E0:46 — i.e. nearly every format is *bottom-rung* on the "richness" axes. A Structure axis would similarly be G0 for ~40 catalog/line formats and light up only for docling/doclang/pdf/image/openxml/html — which is exactly why it deserves to be *its own* axis rather than buried in Engine L.

---

## 4. PROPOSED — a Structure & Geometry axis (G0–G4 / "Comprehension depth")

### 4.1 What it measures (and why it is NOT Engine or Vocabulary)

> **G = how much of the document's *logical and spatial structure* the reader recovers and the model represents** — block roles, reading order, table grids, cross-block relations, and page geometry.

- **vs Engine (L)** — L measures *byte/round-trip/parity fidelity of the serialization*. A reader can be L4 byte-faithful while emitting one flat `Block` per page (G0). PDF is `L2` today but its whole value proposition is the structure tiers — invisible to L.
- **vs Vocabulary (V)** — V measures **inline / run-level** meaning (`fmt:*`, `link:*`, `media:*`, `code:*` *within* a block, `format-maturity.md:160-166`). G measures **block-level + cross-block + spatial** structure. They are orthogonal: **docling is `V0` (plain text runs, `reader.go:18-20`) but represents headings/tables/geometry/reading-order = high G**; **html is `V1` (typed inline) but emits no geometry = low G**. Two genuinely independent axes.
- It subsumes both repo ladders: the AD-029 representation depth (the *what*) is the axis itself; the AD-028 tier 1/2/3 (the *how/authority*) is a **provenance qualifier** recorded per format, mirroring promise-vs-score (`GeometryAnnotation.SourceRef`, `structure.go:140-142`, already carries provenance).

### 4.2 The ladder — each rung defined by what `core/model/structure.go` + the Layer/Group tree can actually carry

| Level | Name | Entry criteria (grounded in deep-dive A's model) | Realized by |
|---|---|---|---|
| **G0** | Flat / opaque | `LayerStart → Block → LayerEnd`; no `StructureAnnotation.Role`, no Group nesting beyond the document layer, no geometry. The "destructive flatten." | AD-028 fast path (one Block/page); most current formats |
| **G1** | Linear + containers | Blocks in correct **reading order**, grouped by the `Layer`/`Group` tree (lists, sections), but roles are generic (`Block.Type` only) — no normalized `StructureAnnotation.Role`. | catalog/line formats with `PartGroupStart`; OCR text recovered |
| **G2** | Logical roles | Every block carries a normalized **`StructureAnnotation.Role`** (heading+`Level`, paragraph, caption, footnote, list-item, code, formula) and **`LayoutLayer`** where applicable (body/furniture/metadata). No geometry required. | DocLang/Docling text items; AD-028 tier-1 minus geometry; the level docling reaches for a geometry-less doc |
| **G3** | Tables + nesting + relations | G2 **plus** reconstructed table grids (`Group("table")→("table-row")→table-cell/table-header`), nested lists, and typed **`RelationAnnotation`** edges (`caption-of`, `footnote-of`). Reading order explicit. | docling/doclang readers + `structure.Gridify`/`TableToParts`; AD-028 tier-1 tables; vision tier-3 table reconstruction |
| **G4** | Full geometry / spatial fidelity | G3 **plus** a **`GeometryAnnotation`** on every block (page + bbox + origin + resolution), `Z` stacking for overlay planes, and — for the top sub-rung — per-glyph **`Glyphs`**. Enough to reconstruct the page and round-trip to DocLang `<location>`. | AD-028 geometry/glyphs path; DoclingDocument prov bboxes; visual editor + Vision Lab |

### 4.3 How the AD-029 enrichment ladder maps onto G (the unification)

| AD-029 mode | G rung |
|---|---|
| Whole-image Media only | G0 (no text comprehended) |
| + alt-text / caption / metadata blocks | G0–G1 (text present, no in-image structure) |
| + OCR in-image text | G1 (linear text recovered) |
| + geometric tier-2 (`structure.Analyze`) | G3 (tables) / G4 (bbox geometry) |
| + ML tier-3 layout (PP-DocLayoutV3) | G3 authoritative roles + reading order; G4 with region bboxes |

### 4.4 Applicability (the `na` rule, mirroring read-only-pdf writer cells)

- **Catalog/string formats** (json, properties, resx, yaml, po, the harvest set) have **no intrinsic geometry** → cap at G1/G2 (roles only where the format encodes a heading/list/section), and **geometry cells score `na`** (countersigned, `format-maturity.md:112-117`).
- **Only spatial/structured formats** can reach G3/G4: pdf, image, docling, doclang, openxml (pptx slide coords / xlsx grid), rendered html. This is verbatim the `GeometryAnnotation` applicability note (`structure.go:121-126`).

### 4.5 Deterministic floor signals (so G is reproducible like the other axes)

Mirror the Vocabulary `vocabtypes` grep (`format-maturity.md:204-211`), over non-test `.go` in `core/formats/<id>/`:

- **G1**: package emits `model.PartGroupStart` (Group nesting) and/or sets `StructureAnnotation.ReadingOrder`.
- **G2**: package calls `SetSemanticRole` / `SetStructure` (`structure.go:189,210`) **+** a roles test (e.g. `TestRead…RolesAndGeometry`).
- **G3**: package emits `Group{Type:"table"/"table-row"}` **+** `AddRelation` (`structure.go:264`) **+** a structure/table test.
- **G4**: package calls `SetGeometry` (`structure.go:255`) **+** a geometry test; G4-top adds `GeometryAnnotation.Glyphs`.

New per-format artifact (proposed): **`core/formats/<id>/structure.yaml`** — declares the G rung, the AD-028 provenance tier (authoritative / geometric / ML), and the `na` geometry cells with `reviewed_by`. Exactly the `vocabulary.yaml` + `constructs.yaml` shape.

---

## 5. Candidate names + intuitive groupings for the whole axis set

Seven axes once G lands: Engine (L), Vocabulary (V), **Structure/Geometry (G)**, Corpus (C), Security (S), Knowledge (K), Editor (E). Today they read as a flat list of cryptic letters. Group into **three families**, by the *question each answers*:

### Scheme A — **Comprehension / Assurance / Enablement** (recommended)

| Family | Axes | The question |
|---|---|---|
| **Comprehension** | Engine (L), Vocabulary (V), **Structure/Geometry (G)** | "How much of the document does the engine actually *understand* and reproduce?" — fidelity at three resolutions: bytes/round-trip (L), inline meaning (V), block+spatial structure (G). |
| **Assurance** | Corpus (C), Security (S) | "Can we *trust* it over real and hostile files?" |
| **Enablement** | Knowledge (K), Editor (E) | "Can a person / model / native editor actually *work* with it?" |

Why recommended: (a) gives the new G axis a natural home next to L and V as the third "fidelity-at-increasing-resolution" axis; (b) survives the framework's min-over-gating rule unchanged — the headline tier stays `min` over the designated gating subset (Engine ∧ Corpus ∧ Knowledge) even though those straddle families; (c) plain-language for the dashboard: *a reader must comprehend the document, we must be assured it works, and a person must be enabled to use it.*

### Scheme B — **Fidelity / Trust / Reach**

Fidelity = {Engine, Vocabulary, Structure/Geometry}; Trust = {Corpus, Security}; Reach = {Knowledge, Editor} (how far the format reaches into real people/tools/workflows). Crisper nouns; "Reach" is slightly less obvious than "Enablement."

### Scheme C — plain-language dashboard labels

**Understanding** {L, V, G} / **Robustness** {C, S} / **Workability** {K, E}. Best for end-user-facing copy; weakest as internal vocabulary.

### Naming the G axis itself

- **Structure & Geometry** — most literal; matches `core/model/structure.go` + `GeometryAnnotation`.
- **Comprehension depth** — best captures the user's named ladder ("metadata / OCR-to-text / OCR+structure+geometry" = increasing comprehension); doubles as the family name in Scheme A so the axis and its family share a word — a feature, not a clash.
- Letter: **G** (free; L/V/E/K/C/S taken). "D" (Depth) also free if "Comprehension depth" wins.

---

## 6. Pointers index (for the sharpening PR)

- Substrate: `core/model/structure.go` — `StructureAnnotation` (93-107), `GeometryAnnotation` (127-149), `RelationAnnotation` (165-176), Role\* (34-52), Layer\* (61-67), accessors `SetSemanticRole`/`SetGeometry`/`AddRelation` (210/255/264).
- Docling: `core/formats/docling/schema.go` (whole), `reader.go` `labelRole` (40-54), `geometryFromProv` (402-424), `emitTable` (268-311); `spec.yaml` features.
- DocLang: `core/formats/doclang/reader.go` `blockRole` (39-47), `<layer>`/`<location>` (261-269), `geometryFrom` 512-grid (502-516), OTSL `otslCellTok` (72-79).
- Image/vision ladder: `core/formats/image/reader.go` `Config{OCR,Layout}` (46-57), `ocrParts` tier-3→tier-2 (305-336).
- Tier-2 engine: `core/structure/analyze.go` `Analyze` (61), `Gridify` (222), `ToParts` (351), `TableToParts` (375).
- ADs: `web/docs/contribute/architecture/028-pdf-reader-plugin.md` (structure-tiers table, two extraction modes), `029-vision-and-image-localization.md` (Context modes table, ocr/layout toggles).
- Rubric to extend: `docs/internals/format-maturity.md` axes table (99-106), Vocabulary precedent (160-217), `na` rule (112-117), min-over-gating (27), promise-vs-score (16-30).
- Governance prior art (no axis-grouping written yet): `docs/internals/research/format-ops/followup-maturity-ladder-governance.md:73,96,112` (vector published, headline by min; promises aggregate by minimum).
