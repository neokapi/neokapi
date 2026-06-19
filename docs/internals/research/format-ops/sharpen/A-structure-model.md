# Deep-Dive A — The Structural / Geometric Content Model

Survey of what structural + geometric information the neokapi content model can
now represent, at what granularity, and whether each thing is first-class,
overlay, or absent. This is the substrate a new "Structure/Geometry" maturity
axis would measure against. All paths relative to repo root
`/Users/asgeirf/src/neokapi/neokapi/.claude/worktrees/format-process`.

---

## 0. The headline: structure/geometry is a STAND-OFF BLOCK ANNOTATION, not a field, not an overlay

`core/model/structure.go:1-29` is explicit about the design choice:

> "A document's logical structure — what each block *is* (heading, caption,
> table cell, …), where it sits on a page (bounding box), and which layout layer
> it belongs to (body vs furniture) — is carried as **two block-scoped stand-off
> payloads, NOT as new top-level Block fields**. This keeps the addition strictly
> additive: both ride the existing payload registry (annotation_registry.go), so
> they serialize over every annotation-aware path (the gRPC bridge, the store)
> with no protobuf or KLF schema change."

There are **three** structural payload keys (`core/model/structure.go:21-29`):

```go
const (
	AnnoStructure = "structure"  // *StructureAnnotation (role, plane, visibility, level, readingOrder)
	AnnoGeometry  = "geometry"   // *GeometryAnnotation (page + bounding box + glyphs)
	AnnoRelations = "relations"  // *RelationAnnotation (cross-block edges)
)
```

These live in `Block.Annotations map[string]Payload` (`core/model/block.go:21`),
the **positionless** block-scoped metadata map — the counterpart to positional,
run-anchored `Block.Overlays` (`core/model/block.go:20`). Crucial distinction
(`core/model/overlay.go:16-17`, `:62`, `:77-79`):

- **Overlay** (`Block.Overlays []Overlay`) = typed, **run-anchored** (carries a
  `RunRange`), positional — segmentation, terms, entities, QA, alignment.
  Source rewrites invalidate it.
- **Annotation** (`Block.Annotations`) = typed, **positionless**, block-scoped.
  Structure/Geometry/Relations are here, so "a source rewrite never invalidates
  it" (`core/model/structure.go:92`, `:173`).

So the answer to "does Block carry a role/structure / geometry/bbox / region
ref?" is: **not as struct fields** — it carries them as keyed payloads in the
annotation map, surfaced through field-like accessor methods
(`core/model/structure.go:181-276`). `core/model/block.go:10-28` (the `Block`
struct) has **no** Role, Geometry, BBox, Page, or Region field.

---

## 1. `StructureAnnotation` — the logical role / plane / visibility record

`core/model/structure.go:93-107`:

```go
type StructureAnnotation struct {
	Role         string `json:"role,omitempty"`         // normalized semantic role; "" = unset
	Layer        string `json:"layer,omitempty"`        // layout plane; "" = body/unspecified
	Visibility   string `json:"visibility,omitempty"`   // presence condition; "" = visible
	Level        int    `json:"level,omitempty"`        // heading 1–6 / list nesting depth; 0 = n/a
	ReadingOrder int    `json:"readingOrder,omitempty"` // explicit reading-order index; 0 = unset
}
func (*StructureAnnotation) TypeName() string { return AnnoStructure } // :110
```

It carries **four orthogonal axes** of logical structure, all block-scoped:

### 1a. Role — the "what is this block" axis (open vocab, ~17 canonical values)
`core/model/structure.go:34-52`, aligned to the DocLang / DoclingDocument taxonomy:

```
RoleParagraph "paragraph"     RoleTitle "title"        RoleHeading "heading"
RoleCaption "caption"         RoleFootnote "footnote"  RoleList "list"
RoleListItem "list-item"      RoleTable "table"        RoleTableCell "table-cell"
RoleTableHeader "table-header" RoleCode "code"         RoleFormula "formula"
RolePicture "picture"         RolePageHeader "page-header"  RolePageFooter "page-footer"
RoleFormField "form-field"    RoleSection "section"
```
"The set is open; these are the canonical values. Unknown values are permitted
and degrade gracefully" (`:16-18`, `:33`).

### 1b. Layer — the PLANE axis (which visual stratum), `core/model/structure.go:54-67`
```
LayerBody "body"  LayerFurniture "furniture"  LayerBackground "background"
LayerOverlay "overlay"  LayerMetadata "metadata"
```
Doc: body = main content; furniture = running headers/footers/nav/page numbers;
background = watermarks; overlay = modal/dialog/popover/tooltip stacked above
body; metadata = outside the rendered flow (HTML `<title>`, meta description,
alt text). Open vocabulary.

### 1c. Visibility — the PRESENCE-CONDITION axis, `core/model/structure.go:69-79`
```
VisibilityVisible ""  VisibilityConditional "conditional"  VisibilityHidden "hidden"
VisibilityPrintOnly "print-only"  VisibilityScreenOnly "screen-only"
```
"Orthogonal to the plane (a modal is plane=overlay, visibility=conditional)."

### 1d. Level + ReadingOrder — heading/nesting depth and an explicit order hint
`Level` = heading 1–6 or list nesting depth. `ReadingOrder` = explicit index
"when the source provides one. 0 = unset; **fall back to Part-stream order**"
(`:104-106`). See §6 — reading order is mostly carried by stream order, not this
field.

Accessors (`core/model/structure.go:199-247`): `SemanticRole()/SetSemanticRole(role, level)`,
`LayoutLayer()/SetLayoutLayer(layer)`, `Visibility()/SetVisibility(v)`. All
**upsert** (via `structureOrNew()` `:192-197`) so setting one axis never clobbers
the others — proven by `structure_test.go:28-35` (SetLayoutLayer keeps Role) and
`:59-67` (SetVisibility keeps Layer).

---

## 2. `GeometryAnnotation` — the spatial / page-coordinate record

`core/model/structure.go:121-149`:

```go
type GeometryAnnotation struct {
	Page       int        `json:"page,omitempty"`       // 1-based page; 0 = unpaginated/unknown
	BBox       Rect       `json:"bbox"`                 // x_min, y_min, width, height, top-left
	Resolution int        `json:"resolution,omitempty"` // normalized grid edge (DocLang=512); 0 = absolute px/pt
	Origin     string     `json:"origin,omitempty"`     // "" defaults to "top-left"
	Z          int        `json:"z,omitempty"`          // stacking order within plane; 0 = base
	SourceRef  string     `json:"sourceRef,omitempty"`  // provenance pointer (e.g. DoclingDocument JSON pointer)
	Glyphs     []GlyphBox `json:"glyphs,omitempty"`     // optional per-character geometry
}
func (*GeometryAnnotation) TypeName() string { return AnnoGeometry } // :159
```

The bounding box itself, `core/model/structure.go:114-119`:
```go
type Rect struct { X, Y, W, H float64 }  // axis-aligned, top-left origin
```

Per-glyph geometry, `core/model/structure.go:151-156`:
```go
type GlyphBox struct {
	Text string `json:"text,omitempty"`
	BBox Rect   `json:"bbox"`
}
```
"populated only when a reader is asked for glyph-level geometry (e.g. the PDFium
reader's 'glyphs' option)" — lets a consumer render character-precise highlights
while blocks stay meaningful text units (`:143-148`).

**What geometry can express, at block granularity:** page number, an
axis-aligned bbox in either absolute units or a normalized grid (Resolution),
named coordinate origin, a Z stacking index for overlapping strata, a provenance
back-pointer, and optional per-glyph boxes. Doc (`:121-126`): "It exists only for
formats with intrinsic spatial layout (PDF, PPTX slide coordinates, XLSX cell
grid, rendered HTML) or content ingested from a layout-aware source
(Docling/DocLang). It is **read-only** display/reconstruction metadata — native
format writers ignore it; only the visual layout view and
layout-target/DocLang serializers consume it." Round-trip proven by
`structure_test.go:38-52`, `:119-131`.

---

## 3. `RelationAnnotation` — typed cross-block edges (the non-containment graph)

`core/model/structure.go:161-179`:

```go
type Relation struct {
	Type   string `json:"type"`   // see Rel* constants
	Target string `json:"target"` // ID of the related Block/Group/Layer
}
type RelationAnnotation struct {
	Relations []Relation `json:"relations"`
}
func (*RelationAnnotation) TypeName() string { return AnnoRelations } // :179
```

Relation types (`core/model/structure.go:83-89`):
```
RelCaptionOf "caption-of"   RelFootnoteOf "footnote-of"   RelLabelFor "label-for"
RelTriggers "triggers"      RelReferences "references"
```
Purpose (`:81-89`, `:161-170`): "Edges capture non-containment relationships that
the Layer/Group tree cannot — a caption's figure, a footnote's marker, a label's
field, a trigger's modal." Target is a **string ID** referencing a Block/Group/
Layer — there is no typed/strongly-resolved reference, just an ID.
`AddRelation(relType, target)` (`:264-276`) dedupes (type,target) pairs;
`structure_test.go:70-88` proves dedup.

---

## 4. Serialization: structure is generic over the wire/store (no schema change)

`core/model/annotation_registry.go:24-39` registers all three in the global
payload registry at `init()`:
```go
RegisterPayload(AnnoStructure, func() Payload { return &StructureAnnotation{} })
RegisterPayload(AnnoGeometry,  func() Payload { return &GeometryAnnotation{} })
RegisterPayload(AnnoRelations, func() Payload { return &RelationAnnotation{} })
```
The `Payload` interface (`annotation_registry.go:10-13`) requires only
`TypeName() string`; `NewPayload(typeName)` (`:51-61`) rehydrates the concrete
type from its name. `structure_test.go:90-103` asserts all three rehydrate.
This is *why* structure/geometry/relations ride every annotation-aware path
(gRPC bridge, store, KLF) with zero protobuf/KLF schema work.

---

## 5. Table cells — LOWERED to Group + role-tagged Block, NOT a first-class type

There is **no** `Table`/`Row`/`Cell` type in `core/model` (verified: grep for
`type (Page|Region|Column|Table|Cell|Row)` in `core/model/*.go` → none). Tables
are represented in the Part stream as:

- `GroupStart{Name:"table", Type:"table"}` → per-row `GroupStart{Name:"table-row",
  Type:"table-row"}` → cell `Block`s with `Block.Type="table-cell"` and
  `SemanticRole` = `RoleTableCell` / `RoleTableHeader`
  (`core/structure/analyze.go:375-398`, `TableToParts`). The Group type is
  `model.GroupStart`/`GroupEnd` (`core/model/group.go:4-20`) — a flat
  start/end marker pair with ID/Name/Type/Properties, no geometry of its own.

The *analysis-time* table types are **transient**, in `core/structure` (a
geometric inference package, "tier 2"), not in the content model
(`core/structure/analyze.go:42-51`):
```go
type Table struct { Rows [][]Cell }          // :42-44
type Cell  struct { Blocks []*model.Block; Header bool }  // :48-51
```
These are consumed by `TableToParts`/`Gridify` and never persisted — they exist
only to *produce* the Group+Block lowering. Granularity consequence: a localized
table round-trips as cells (each cell is a translatable Block in a row group),
but **merged cells, colspan/rowspan, and nested tables are out of scope** of the
model — `analyze.go:11` explicitly: "Merged cells, nested tables, and
borderless edge cases are out of scope here (that's the ML tier)." Even the ML
tier (`core/vision`) reconstructs tables through the *same* `Gridify` →
`TableToParts` lowering (`core/vision/layout.go:185-200`), so there is no
colspan/rowspan representation anywhere.

---

## 6. Reading order — carried by Part-STREAM ORDER, with an optional index hint

Two mechanisms, neither a first-class ordered container:

1. **Stream order is authoritative.** `StructureAnnotation.ReadingOrder`
   (`structure.go:104-106`) is an *optional hint*; default behaviour is "fall
   back to Part-stream order." Blocks are emitted in reading order and that
   ordering *is* the reading order.
2. **Analysis-time `ReadingOrder int`** on the transient `vision.Region`
   (`core/vision/layout.go:23-28`) and a deterministic
   `SortReadingOrder(regions)` heuristic (`layout.go:47-102`: column-cluster by
   horizontal-center proximity, left-to-right columns, top-to-bottom within).
   This decides the *order regions are emitted* — once lowered to Parts it
   becomes stream order. `PartsFromLayout` (`layout.go:111-157`) emits regions
   sorted by `ReadingOrder`.

So reading order is representable at block granularity, but as ordering of the
stream (+ an int hint on the block), not as an explicit ordered region list in
the model.

---

## 7. Regions / columns — ABSENT from the model (transient analysis types only)

There is **no** Region or Column type in `core/model`. Regions exist only as
transient analysis structs that get lowered into the Part stream:

- `core/structure.Region` (`analyze.go:32-39`): geometric tier-2 region
  (`Kind RegionKind` ∈ {`RegionBlock`, `RegionTable`} `:22-30`, plus `Block`,
  `Role`, `Level`, `Table`). Consumed by `ToParts` (`:351-365`).
- `core/vision.Region` (`layout.go:22-28`): ML tier-3 region (`Role`, `BBox`,
  `ReadingOrder`, `Confidence`). Consumed by `PartsFromLayout`.

Columns are **never represented** — they exist only inside clustering heuristics
(`structure/analyze.go:265-301` `columnCenters`; `vision/layout.go:69-92` column
buckets) and are discarded after row/reading-order assignment. A multi-column
layout collapses into reading-order stream order; the column structure itself is
not recoverable from the model.

---

## 8. Pages — represented ONLY as `GeometryAnnotation.Page int`

No `PartPageStart`/`PartPageEnd` (`core/model/part.go:6-26` PartType enum: only
LayerStart/End, GroupStart/End, Block, Data, Media, RawDocument, Custom — values
8–11 are reserved holes from retired Batch parts). No first-class Page type.
A page is just the `Page` int on each block's `GeometryAnnotation` (`:128-129`).
Page boundaries are inferred by grouping blocks with the same `Page` value; there
is no page container, page size, or page-level layer.

---

## 9. The "target-asset" model (#904 / AD-029) — image localization is a Media swap + path-pairing, NOT a model type

`core/model/target.go` (the variant-keyed `Target`, `:83-88`) has **no** asset
field. There is no `TargetAsset` type in `core/model`. The image-as-localizable-
asset model is split between `model.Media` and `core/project`:

- **The asset itself** = `model.Media` (`core/model/media.go:7-17`): `Data` |
  `BlobKey` | `URI` (priority order), `MimeType`, `Filename`, `AltText`, `Size`,
  `Properties`. Carried as `PartMedia` (`part.go:19`). AD-029 (`:44-50`): the
  image reader "always emits the picture as a `model.Media` part — the unit a
  localization flow can replace wholesale." The image never enters the part
  stream as bytes — emitted **by URI** (AD-029 `:60-65`).
- **Per-locale variant pairing** = `project.AssetVariant` (`core/project/asset.go:30-34`):
  ```go
  type AssetVariant struct {
  	Locale model.LocaleID
  	Path   string // resolved per-locale output path
  	Exists bool
  }
  ```
  `ResolveAssetVariants(root, item, source, locales)` (`asset.go:42-59`) resolves
  each locale's target path via the recipe `target:` template and reports which
  files exist — "the coverage view a whole-image-replacement workflow needs."
  `IsBinaryAssetFormat(name)` (`asset.go:16-23`) currently returns true **only**
  for `"image"`; for these, an existing localized variant on disk is
  authoritative (`kapi run`/`merge` keep it rather than reprocess the source).
- **Alt-text / caption** = a linked translatable Block, not a Media field
  (AD-029 `:67-80`): `image/reader.go:213-214` emits a caption block with
  `SetSemanticRole(RoleCaption,0)` + `AddRelation(RelCaptionOf, "img1")`.
  `Media.AltText` stays the source value for display; per-locale lives in the
  caption block's `Targets`.
- **Metadata** = `core/docmeta` (`docmeta.go:65-67`): translatable embedded
  fields become Blocks with `SetSemanticRole(role,0)` +
  `SetLayoutLayer(LayerMetadata)`; non-translatable → namespaced Layer.Properties.

So "how is an image/asset target represented?" — as a `model.Media` (the asset)
+ a file-path pairing (`project.AssetVariant`) + linked caption/metadata Blocks.
The localized variant is a *different file on disk*, resolved by template, not a
field on `Target`.

---

## 10. Which readers populate what (the substrate's real coverage today)

From grep of `SetGeometry|SetSemanticRole|SetLayoutLayer|AddRelation|SetVisibility`
across `core/`/`providers/` (non-test):

| Format / pkg | Role | Layer | Visibility | Geometry | Relations | file:line |
|---|---|---|---|---|---|---|
| **doclang** | ✓ | ✓ | — | ✓ (Resolution=512 grid) | — | `formats/doclang/reader.go:308,311,314,345` |
| **docling** | ✓ | ✓ furniture | — | ✓ (SourceRef=SelfRef) | caption(✓) | `formats/docling/reader.go:222,225,228,301,346,348` |
| **html** | ✓ | ✓ plane | ✓ | — | — | `formats/html/reader.go:557,566,569` |
| **openxml** (wml/sml/dml/roles) | ✓ | ✓ | ✓ hidden | ✓ (XLSX cell grid `sml.go:295`, DML `dml.go:176`) | — | `formats/openxml/roles.go:227-237`, `dml.go:68-72,176`, `sml.go:295` |
| **markdown** | ✓ (heading/list-item) | — | — | — | — | `formats/markdown/reader.go:850,1478,1544` |
| **csv** | ✓ (table-header/cell) | — | — | — | — | `formats/csv/reader.go:234,328` |
| **odf** | — | — | — | ✓ | — | `formats/odf/geometry.go:103` |
| **idml** | — | — | — | ✓ | — | `formats/idml/geometry.go:161` |
| **pdf** (wasm bridge / kapi-pdfium plugin) | (tier tree) | — | — | ✓ (+glyphs) | — | `formats/pdf/wasm_bridge.go:198` |
| **image** (+ kapi-vision tier-3) | ✓ caption / via vision | metadata | — | ✓ via vision | ✓ caption-of | `formats/image/reader.go:213-214` |
| **docmeta** (shared) | ✓ | ✓ metadata | — | — | — | `core/docmeta/docmeta.go:65,67` |

The depth ladder the user named maps onto AD-028's tier model
(`web/docs/contribute/architecture/028-pdf-reader-plugin.md`) and AD-029's modes:

- **Tier 1** = authoritative tagged structure tree (tagged PDF, OOXML styles,
  DocLang/Docling native roles).
- **Tier 2** = geometric inference: `core/structure.Analyze` (`analyze.go:61-110`)
  clusters positioned blocks (those carrying `GeometryAnnotation`) into rows →
  detects tables by column alignment → tags prose heading/paragraph by relative
  height. Deterministic, format-agnostic, consumes geometry only.
- **Tier 3** = ML layout: `core/vision` (`vision.go`, `layout.go`) — OCR lines +
  PP-DocLayoutV3 region roles + reading order, lowered via `PartsFromLayout` /
  `StructureFromLayout` (`layout.go:111-183`). Format-agnostic over any raster.

For an **image** specifically (AD-029 `:36-42`), the depth modes are exactly the
user's ladder: Media-only (metadata) → alt-text/caption → in-image OCR text →
OCR+layout structure (regions+reading order, tables to cells)+geometry.

---

## 11. First-class vs overlay vs absent — the summary table for a Structure/Geometry axis

| Structural concept | Status in model | Where |
|---|---|---|
| Block semantic role (heading/caption/table-cell/…) | **Block annotation** (stand-off, positionless) | `StructureAnnotation.Role` |
| Heading / nesting level | **Block annotation** | `StructureAnnotation.Level` |
| Layout plane (body/furniture/overlay/metadata/background) | **Block annotation** | `StructureAnnotation.Layer` |
| Visibility / presence condition | **Block annotation** | `StructureAnnotation.Visibility` |
| Bounding box (x/y/w/h) | **Block annotation** | `GeometryAnnotation.BBox` |
| Page number | **Block annotation** (int only; no Page container) | `GeometryAnnotation.Page` |
| Coordinate grid / origin / Z-stack | **Block annotation** | `GeometryAnnotation.Resolution/Origin/Z` |
| Per-glyph geometry | **Block annotation (opt-in)** | `GeometryAnnotation.Glyphs []GlyphBox` |
| Geometry provenance pointer | **Block annotation** | `GeometryAnnotation.SourceRef` |
| Cross-block edges (caption-of, footnote-of, label-for, triggers, references) | **Block annotation** (edge = string ID) | `RelationAnnotation` |
| Reading order | **Part-stream order** (+ optional int hint) | stream + `StructureAnnotation.ReadingOrder` |
| Table / row / cell | **LOWERED to Group + role-tagged Block** (no model type) | `GroupStart` + `RoleTableCell/Header` |
| Containment / sectioning tree | **first-class** structural tree | `Layer` (`layer.go`) + `Group` (`group.go`) parts |
| Region / column | **ABSENT** (transient analysis structs only) | `structure.Region`, `vision.Region` — discarded after lowering |
| Page container / page size | **ABSENT** (only `Page int` on geometry) | — |
| Merged cells / colspan / rowspan / nested tables | **ABSENT** | out of scope (`analyze.go:11`) |
| Image / binary asset | `model.Media` part + `project.AssetVariant` path-pairing | `media.go`, `core/project/asset.go` |
| OCR / layout confidence | **ABSENT from model** (lives on transient `OCRLine.Confidence` / `vision.Region.Confidence`, dropped on lowering) | `vision.go:24-28`, `layout.go:27` |

**Key gaps a sharpened axis should note:** (1) regions/columns and pages have no
first-class representation — multi-column/page geometry collapses to stream
order; (2) tables have no merged-cell/span representation; (3) model confidence
(OCR/layout) is computed but never persisted onto blocks; (4) geometry is
declared read-only / writer-ignored (`structure.go:124-126`) — only DocLang/
layout-target serializers consume it, so geometric *round-trip fidelity* is
narrow; (5) the only `IsBinaryAssetFormat` today is `"image"` (`asset.go:18`).
