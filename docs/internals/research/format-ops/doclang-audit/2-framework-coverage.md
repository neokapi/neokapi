# TASK 2 — neokapi framework coverage surface (vs DocLang v0.6)

Read-only inventory of what the neokapi **content model + maturity axes + the
`doclang` format** can MEASURE/REPRESENT today, so a later gap analysis can diff
against the DocLang spec (`/Users/asgeirf/src/doclang-project/doclang/spec.md`,
v0.6, 3734 lines). All neokapi paths are repo-relative to
`/Users/asgeirf/src/neokapi/neokapi/.claude/worktrees/format-process`.

Headline: the model already carries the full role+plane+geometry+relations
standoff layer; the **G0–G4 axis is design-only** (proposal, not wired); the
`doclang` reader implements roughly the G3 floor (roles/tables/order) + part of
G4 (bbox geometry) but **drops** DocLang's relations/threading, fields, picture
payloads, markers, checkboxes, and span continuations.

---

## 1. Content model — what it can REPRESENT

### 1.1 `core/model/structure.go` (the WS1 structural layer) — quoted

Three block-scoped **stand-off** payloads (positionless; ride
`Block.Annotations` via the payload registry, no proto/KLF change — `structure.go:3-18`).
Annotation keys (`structure.go:21-29`): `AnnoStructure="structure"`,
`AnnoGeometry="geometry"`, `AnnoRelations="relations"`.

**Roles** — `Role*` constants, "canonical but open" set (`structure.go:34-52`):
`paragraph, title, heading, caption, footnote, list, list-item, table, table-cell,
table-header, code, formula, picture, page-header, page-footer, form-field, section`.
Comment: "Aligned with the DocLang / DoclingDocument taxonomy" (`structure.go:31-33`).

**Layout layers / PLANE axis** — `Layer*` (`structure.go:54-67`):
`body, furniture, background, overlay, metadata`. Comment: "Aligned with DocLang's
`<layer>` values and extended for the strata reflowable documents add"
(`structure.go:54-60`). DocLang `<layer>` allows only `{body,background,furniture}`
(`spec.md:2587`); model adds `overlay`+`metadata` (superset).

**Visibility / presence-condition** (`structure.go:69-79`):
`"" (visible), conditional, hidden, print-only, screen-only`. No DocLang counterpart
(model superset).

**Relation types** — `Rel*` (`structure.go:81-89`):
`caption-of, footnote-of, label-for, triggers, references`.

`StructureAnnotation` (`structure.go:91-107`): `Role, Layer, Visibility string;
Level int (heading 1–6 / list depth); ReadingOrder int ("explicit reading-order
index when the source provides one; 0 = unset; fall back to Part-stream order")`.

`Rect` (`structure.go:114-119`): `X,Y,W,H float64`.
`GeometryAnnotation` (`structure.go:121-149`, declared **read-only**
reconstruction metadata, "native format writers ignore it"): `Page int (1-based);
BBox Rect; Resolution int ("DocLang uses 512"; 0=absolute); Origin string
("top-left"); Z int (stacking within plane, overlays); SourceRef string (provenance
JSON pointer); Glyphs []GlyphBox`.
`GlyphBox` (`structure.go:153-156`): `Text string; BBox Rect` — per-character geometry (G4-top).

`Relation` (`structure.go:165-170`): `Type, Target string` (Target = ID of
Block/Group/Layer). `RelationAnnotation` (`structure.go:174-176`): `Relations []Relation`.

Accessors (`structure.go:181-276`): `Block.Structure()/SetStructure`,
`SemanticRole()/SetSemanticRole(role,level)`, `LayoutLayer()/SetLayoutLayer`,
`Visibility()/SetVisibility`, `Geometry()/SetGeometry`, `Relations()/AddRelation(type,target)`.

### 1.2 Block / Run / Layer / Target / Overlay / annotation_registry

**Block** (`block.go:10-28`): `ID, Name, Type (free-form format tag), MimeType,
Translatable, SourceLocale, Skeleton, Source []Run, Targets map[VariantKey]*Target,
Overlays []Overlay, Annotations map[string]Payload, Properties map[string]string,
Identity *BlockIdentity, ContentRef *ContentRef, DisplayHint, PreserveWhitespace,
IsReferent`. Note: structural role/geometry/relations live in `Annotations`, **not**
as Block fields. `Block.Type` is the *old* free-form kind (csv/html/markdown set it);
`StructureAnnotation.Role` is the *new* normalized field.

**Run** (`run.go:123-148`) — discriminated union, exactly one of:
`Text *TextRun; Ph *PlaceholderRun; PcOpen *PcOpenRun; PcClose *PcCloseRun; Sub
*SubRun; Plural *PluralRun; Select *SelectRun`. Inline-code runs carry
`ID,Type,SubType,Data,Equiv,Disp,Constraints` (`run.go:62-97`). `PluralRun`
keyed by ICU `PluralForm` (`run.go:15-25,111-114`); `SelectRun` by string case.
`PlaceholderKind` = variable|element|node|icu-pivot (`run.go:46-51`). This is the
inline vocabulary surface DocLang's formatting + payload elements map to.

**Layer** (`layer.go:6-20`): `ID,Name,Format,Locale,Encoding,MimeType,LineBreak,
IsMultilingual,ParentID (nesting tree),Properties,Overlays,Annotations,HasBOM`.
Layers nest via `ParentID` (`layer.go:26-29`) — the document/section/embedded tree.

**Group** (`group.go:3-20`): `GroupStart{ID,Name,Type,Properties}` / `GroupEnd{ID}`
— structural grouping bracketed in the Part stream. This is how lists/tables/rows
are represented (no first-class Table type).

**Part types** (`part.go:13-26`): `LayerStart/End, GroupStart/End, Block, Data,
Media, RawDocument, Custom`. Reading order = **Part-stream order** (implicit);
`Media` carries binary; `Data` carries non-translatable structure.

**Target / variant** (`target.go`): `VariantKey{Locale,Tone,Channel}`
(`target.go:16-20`); `Target{Runs,Status,Origin,Score}` (`target.go:83-88`);
`TargetStatus` new|draft|translated|reviewed|signed-off (`target.go:64-70`);
`Origin{Kind(human|tm|mt|ai),Engine,Tool,Reference,Timestamp}` provenance
(`target.go:73-79`).

**Overlay** (run-anchored stand-off, `overlay.go:74-92`): `Type, Variant *VariantKey
(nil=source), Layer string, Spans []Span`. `OverlayType` =
segmentation|term|entity|qa|alignment|term-candidate (`overlay.go:20-33`).
`RunRange{StartRun,StartOffset,EndRun,EndOffset}` half-open (`overlay.go:40-45`);
`Span{ID,Range,Props,Value Payload}` (`overlay.go:63-68`). Segmentation can be
multi-layer (`overlay.go:87-99`). These are *interpretations of* content, not
document structure.

**annotation_registry** (`annotation_registry.go`): `Payload` interface =
`TypeName() string` (`:10-13`); registered factories (`:24-39`): `alt-translation,
note, generic, entity, term, term-candidate, editor-anchor`, **and the structural
trio `structure, geometry, relations` (`:36-38`)**. `AltTranslations/Notes/
GenericAnnotation` defined in `annotation.go`. `Skeleton` (`skeleton.go`) preserves
non-translatable structure for reconstruction.

---

## 2. Vocabulary registry — `core/formats/constructs.yaml` (V0–V3 row space)

`version: 1` (`:106`). 8 format **families** (`:117-162`): rich-markup, office-doc,
bilingual-interchange, catalog-keyvalue, subtitle-timedtext, plain-text,
data-config, binary-readonly. ~31 constructs (`:164-981`), by category:

- **inline-format** (`run:pc`): `inline.bold, inline.italic, inline.underline,
  inline.strikethrough, inline.superscript, inline.subscript, inline.code,
  inline.highlight, inline.ruby, inline.ui-widget`.
- **link**: `link.hyperlink`. **media** (`run:ph`): `media.image`.
- **placeholder** (`run:ph`/`run:pc`): `placeholder.line-break, .tab, .footnote-ref,
  .named, .positional, .function-call, .markup-opaque, .component-element`.
- **sub** (`run:sub`): `sub.flow`. **plural-select** (`run:plural`/`run:select`):
  `i18n.plural, i18n.select`.
- **block-kind** (`block:type`): `block.heading, .paragraph, .list-item,
  .table-cell, .quote, .title`.
- **i18n-meta** (ITS-2.0 seeded): `meta.translate-flag, .localization-note,
  .terminology(overlay:term), .text-analysis(overlay:entity), .id-value(block:id),
  .preserve-space, .language(layer:property), .directionality, .locale-filter,
  .size-restriction, .target-pointer(block:target), .change-tracking`.

**Critical for the gap analysis (constructs.yaml comment + SHARPEN finding #2):**
the block-kind rows pin the *old free-form `Block.Type`* values
(`constructs.yaml:607-611`: "Block.Type is a free-form string today … Level/depth …
rides Block.Properties"). They do **not** model `StructureAnnotation.Role`, layout
layer, geometry, reading order, or relations at all — so the entire DocLang
role/plane/geometry surface is **outside** the Vocabulary axis. That is the
orthogonality the G axis is meant to fill.

---

## 3. The maturity axes — what each MEASURES

Source: `docs/internals/format-maturity.md`. The rubric still says **"six axes"**
(`:90,:97`); the seventh (G) is **not yet in the rubric, scorer, or audit** — it
lives only in the SHARPEN proposal (§4 below).

| Axis | Ladder | Measures (rung summary) | file:line |
|---|---|---|---|
| **Engine** | L0–L4 | Parse/round-trip/parity fidelity + robustness. L0 reader emits LayerStart→Block→LayerEnd; L1 writer+roundtrip_test+Config-rejects-unknown; L2 spec.yaml+malformed_test (or harvest ladder); L3 parity head-to-head, every divergence an attributed `expected_fail`; L4 byte-faithful over edge-case matrix+real corpus, schema==config, zero native-bug xfails. | `:119-158` |
| **Vocabulary** | V0–V3 | How richly format semantics map into canonical run/block vocab and back. V0 opaque/raw runs; V1 typed reading (`fmt:*`/`link:*`/`media:*`/`code:*` emitted, evidence-bound read cells); V2 bidirectional (writer consumes canonical types, `vocab_equivalence_test` passes, block-kinds populated); V3 fidelity-proven (zero unknown cells, loss table published, preview renders the vocab). | `:160-216` |
| **Editor** | E0–E4 | How close kapi gets to the native editing surface. E0 files-in/out; E1 faithful preview (`PreviewBuilder`/STRUCTURE_RULES); E2 round-trip with `editor-anchor` overlay surviving a native edit; E3 embedded add-in on HEAD; E4 continuous (webhook/event feed). Floor=min(declared `integrations.yaml`, probed). | `:218-250` |
| **Knowledge** | K0–K3 | Whether a person/model can work on it from in-repo assets. K0 nothing; K1 `dossier.yaml`+≥1 versioned spec+impl table+nativedocs sidecar; K2 `spec.yaml` green w/ refs + every `expected_fail` has `divergence_kind` + schema.go + docs regen no drift; K3 living (check-citations green, zero stale xfails, context-pack generates). | `:252-299` |
| **Corpus** | C0–C3 | Reference files with provenance. C0 unprovenanced; C1 Tier-A exemplars 100% in `corpus.yaml`+fidelity tests; C2 Tier-B fetch wired (or countersigned `na`)+sha256-verified; C3 externally-verified wild files + edge-case matrix + green corpus-sweep + flywheel fixture. | `:301-359` |
| **Security** | S0–S4 | Parser resource-boundedness + hostile-input hardening (**display-only, non-gating**). S0 unbounded; S1 imports `core/safeio`; S2 +Go `Fuzz*` target+seed; S3 +clean corpus-sweep ledger record; S4 sustained green. Pure floor ladder, spread 0. | `:361-398` |

Tier = `min` over gating axes Engine ∧ Corpus ∧ Knowledge; Vocabulary/Editor feed
tier too; Security is display-only (`:97`).

### 3.1 NEW Structure & Geometry G0–G4 — **proposal only, not implemented**

Confirmed mid-flight / unwired:
- `format-triage.js` has **no `gateStructure`** and no `structure` axis (grep: 0 hits).
- `audit-format.py` has **no `_structure_axis`**; the only `structure` symbol is
  `_structure_rules_json_text()` (`:277-285`), which feeds the **Editor** E1 probe
  (`:566-567`), not a G axis. No `structure.yaml` artifacts exist
  (`find core/formats -name structure.yaml` → none).
- `format-maturity.md` still reads "six axes."

The ladder is defined in
`docs/internals/research/format-ops/sharpen/SHARPEN-PROPOSAL.md` §2 (`:64-145`).
Axis id `structure`, prefix `G`, "comprehension depth," cumulative:

| Rung | Recovers | Floor signal (proposed greps over non-test `.go` in `core/formats/<id>/`) |
|---|---|---|
| **G0** opaque/flat | `Media` part or undifferentiated text blocks; no `Role`, no Group nesting, no geometry | — (default) |
| **G1** metadata | text *about* the doc on the metadata plane / alt-text+caption | `metaplane`: `SetLayoutLayer(.*LayerMetadata`, import of `core/docmeta`, or `AddRelation(RelCaptionOf` (`SHARPEN:110`) |
| **G2** linear body | body text in correct reading order, optionally Group-nested; roles still generic | `readingorder`: emits `model.PartGroupStart` and/or sets `StructureAnnotation.ReadingOrder` (`SHARPEN:111`) |
| **G3** logical structure | normalized `Role`+`Level`+`LayoutLayer`; reconstructed table grids (`Group{Type:"table"/"table-row"}`+cell roles); typed `RelationAnnotation`; explicit reading order | `roles`: `SetSemanticRole`/`SetStructure` **and** table Groups and/or `AddRelation`, **plus** a roles/structure test (`SHARPEN:112`) |
| **G4** spatial geometry | G3 + `GeometryAnnotation` (page+bbox+origin+resolution); `Z`; per-glyph `Glyphs` at top sub-rung | `geometry`: `SetGeometry` **plus** a geometry test; G4-top emits `GeometryAnnotation.Glyphs` (`SHARPEN:113`) |

Orthogonality (`SHARPEN:119-135`): G is independent of L (a reader can be L4
byte-faithful yet G0 one-block-per-page) and of V (docling is V0 plain-runs but
high-G; html is V1 typed-inline but low-G). Authority tier
(`native|tagged|geometric|ml`) is a **separate qualifier**, not a G rung
(`SHARPEN:137-144`); declared in the proposed `structure.yaml`. `na` on the
geometry rung for non-spatial catalogs must be a **ceiling cap**, not a gate-pass
(`SHARPEN:255-277,329-332`).

---

## 4. The `doclang` format — what it actually reads/writes today

`core/formats/doclang/` pinned to **spec v0.6**, ns `https://www.doclang.ai/ns/v0`
(`reader.go:35-36`). Detection: `<doclang` sniff, ext `.dclg.xml`
(`reader.go:107-113`). Self-declared coverage in `reader.go:16-19` and `spec.yaml:11-21`.

**Reader (`reader.go`):**
- Root/head: `<head>`, `<page_break>`, and **all element-head property elements**
  (`label,thread,xref,href,layer,location,caption,custom`) are listed but `head`
  + most are **skipped** (`reader.go:55-59,178-180,273-276`). Only `<layer>` and
  `<location>` are consumed (`reader.go:261-272`).
- Block-level semantic elements → `SetSemanticRole` via `blockRole`
  (`reader.go:39-47`): `heading→heading(+level), text→paragraph, footnote, page_header,
  page_footer, code, formula`. `<text>` inside `<list>` → `list-item`
  (`reader.go:198-200`). `parseBlock` (`reader.go:234-318`).
- Containers → `Group` (`containerElem`, `reader.go:51-53`): `list, group,
  field_region, picture`. `<ldiv>` list-item delimiter (+`<marker>`) is **dropped**
  (`reader.go:182-187`).
- OTSL tables (`parseTable`, `reader.go:324-411`): `table/index` → Group of row
  Groups of cell Blocks; cell tokens `fcel,ecel→table-cell; ched,rhed,srow,corn→
  table-header` (`otslCellTok`, `reader.go:72-79`); `<nl>` ends a row. Span
  continuations `lcel/ucel/xcel` are **dropped** (`reader.go:323,392`).
- Inline formatting → `PcOpen/PcClose` runs typed `fmt:*` (`fmtTag`,
  `reader.go:62-69,277-290`): bold/italic/underline/strikethrough/superscript/subscript.
- Geometry: 4-value `<location>` block → `GeometryAnnotation` via `geometryFrom`
  (`reader.go:266-269,500-516`): `BBox{x0,y0,x1-x0,y1-y0}`, Resolution (default 512),
  Origin "top-left". Layer `<layer value>` → `SetLayoutLayer` unless `body`
  (`reader.go:261-262,310-312`).

**Writer (`writer.go`):** re-emits role→element via `roleElem`
(`writer.go:50-70`; `title`→level-1 `heading`, no `<title>` body elem),
`<layer>` + 4-value `<location>` (gated by `cfg.EmitGeometry`, `writer.go:242-264`),
inline fmt **from each run's vocabulary `Type`** not its `Data`
(`typeToDocTag`, `writer.go:73-80,281-314`) — so output is source-format-independent
(faithful round-trip + native→DocLang projection). Tables: cell→`fcel`/`ched`, row→
`<nl>`, captions buffered into one `<caption>` (`writer.go:184-216`). Lists: every
child preceded by `<ldiv/>` (`writer.go:139-141,218-224`). Multiple nested source
layers flattened to a single `<doclang>` root (`writer.go:99-111`).

**Tests:** `conformance_test.go` validates writer output against the **official
vendored XSD** with xmllint (`:18-31,71-85`) — round-trip of corpus fixtures
(`:89-105`) + native (DoclingDocument JSON) projection (`:110-132`); surfaced 3 real
writer bugs (`:22-25`). `roundtrip_test.go` asserts block text+role+geometry survive
DocLang→model→DocLang→model (`:18-83`). `spec.yaml` features: block_elements, lists,
tables_otsl; `parity.skip` (no Okapi bridge, `spec.yaml:96-97`).

**Reader does NOT map (read-through skips, no content loss but no structure):**
OTSL span continuations `lcel/ucel/xcel` (spanned cells dropped); `picture` `<src>`
(`spec.md:2597`) + `<tabular>` chart data (`spec.md:2615`); `marker` glyph
(`spec.md:2292`); the entire **field** model `field_region/field_heading/field_item/
key/value/hint` (`spec.md:2148,2332-2434`) + `checkbox` (`spec.md:2635`); `thread/
xref/href` continuity & cross-references (`spec.md:2478-2526`) → **no
`RelationAnnotation` ever emitted**; `caption` in element head → dropped on read
(only buffered on write); `custom` metadata (`spec.md:2536`); `content`
whitespace-preserve (`spec.md:2653`) → `Block.PreserveWhitespace` not set;
`default_resolution` head element (`spec.md:3025`); subclass `<label>`/code-language
(`spec.md:2460,2252`).

---

## 5. Capability-area map — MODEL · AXIS · doclang reader

DocLang spec yardstick anchors in parentheses.

| Area | MODEL represents? | AXIS measures? (rung) | doclang reader implements? |
|---|---|---|---|
| **Block roles** (semantic elements, `spec.md:2042-2454`) | YES — 17 `Role*` (`structure.go:34-52`) + `Block.Type` legacy | **G3** `roles` (proposal only); Vocabulary block-kind rows cover only 6 old `Block.Type` values, NOT `Role` (`constructs.yaml:607-723`) | Partial — 7 roles + table-cell/header + list-item (`reader.go:39-47,72-79,198-200`); `picture/field/marker` roles unmapped |
| **Layout layer / plane** (`<layer>` body/background/furniture, `spec.md:2575-2587`) | YES — 5 `Layer*` incl. overlay/metadata superset (`structure.go:54-67`) | **G1/G3** `metaplane` (proposal); no current axis | YES — `<layer>` → `SetLayoutLayer` (`reader.go:261-262,310-312`) |
| **Visibility** | YES — 5 `Visibility*` (`structure.go:69-79`) | none (model superset, no DocLang construct) | NO (DocLang has no visibility element) |
| **Inline formatting** (`<bold>`…`<subscript>`, `spec.md:2673+`) | YES — `PcOpen/PcClose` + `fmt:*` (`run.go:79-97`; `constructs.yaml:166-269`) | **Vocabulary V1/V2** (`format-maturity.md:160-216`) | YES — 6 fmt tags ↔ `fmt:*` both directions (`reader.go:62-69`, `writer.go:73-80`) |
| **Inline other** (`<href>` inline link, `<checkbox>`, sub-flows) | YES — `link:*`/`media:*`/`Sub`/`code:*` runs (`run.go`, `constructs.yaml`) | **Vocabulary** | NO — href/checkbox/marker dropped; doclang emits no link/media runs |
| **Geometry / bbox** (`<location>` 4-val, `spec.md:2556-2569`; `<default_resolution>` `:3025`) | YES — `GeometryAnnotation` (bbox+page+resolution+origin+Z+sourceRef) + `Glyphs` (`structure.go:121-156`) | **G4** `geometry`/glyphs (proposal only) | Partial — 4-value location → BBox+Resolution+Origin (`reader.go:500-516`); no Page (page_break unmapped), no per-axis x/y resolution, no glyphs |
| **Reading order** (stream/thread order, `spec.md:142-168`) | YES — implicit Part-stream order + explicit `StructureAnnotation.ReadingOrder` (`structure.go:104-106`) | **G2** `readingorder` (proposal) | Implicit only — stream order; `ReadingOrder` field never set; `<thread>` fragment-reassembly (`spec.md:2478-2490`) unmapped |
| **Tables / grids** (OTSL, `spec.md:2190-2228,2837-2999`) | YES — `Group{Type:"table"/"table-row"}` + cell-roles (`group.go`; `structure.go:42-44`) | **G3** `roles` (table-Group signal) | Partial — fcel/ched/rhed/ecel/srow/corn+nl mapped; **span continuations lcel/ucel/xcel dropped** (no rowspan/colspan) (`reader.go:323,392`) |
| **Lists** (`<list>`/`<ldiv>`/`<marker>`, `spec.md:2168-2188,3001-3019`) | YES — Group + `list-item` role (`structure.go:40-41`) | **G3** | Partial — list→Group, item→list-item; ldiv+marker dropped (`reader.go:182-187`) |
| **Fields / forms** (`field_region…value/hint/checkbox`, `spec.md:2148,2332-2434,2635`) | Partial — `RoleFormField` + `RelLabelFor` exist (`structure.go:50,86`) | **G3** (if emitted) | NO — field_region→generic Group; key/value/hint/checkbox dropped |
| **Pictures / media** (`<picture>`+`<src>`+`<tabular>`, `spec.md:2270-2290,2597-2633`) | YES — `RolePicture`, `media:image` (`run:ph`), Media part (`constructs.yaml:367`; `part.go:19`) | Vocabulary (media) + **G** | NO — picture→generic Group; src URI + tabular chart data dropped |
| **Cross-block relations** (`<thread>/<xref>/<href>/<caption>`, `spec.md:2478-2534,2436`) | YES — `RelationAnnotation`, 5 `Rel*` (`structure.go:81-89,161-176`) | **G3** `roles` (AddRelation signal, proposal) | NO — head elements skipped; **no relations emitted**; caption only buffered on write |
| **Provenance / authority** (Docling confidence, tier) | Partial — `GeometryAnnotation.SourceRef` (`structure.go:140-142`); `Target.Origin` for translations (`target.go:73-79`) | proposed **authority qualifier badge** (`structure.yaml`, not a G rung — `SHARPEN:137-144`) | NO — SourceRef not populated by reader |
| **Document metadata / head** (`<head>`, `<default_resolution>`, `spec.md:1996,3025`) | Partial — `Layer.Properties/Annotations`; `core/docmeta` puts metadata blocks on metadata plane (`docmeta.go:67`) | **G1** `metaplane`; Vocabulary `meta.*` (`constructs.yaml:725-981`) | NO — `<head>` fully skipped (`reader.go:178-180`) |
| **Overlays / standoff** (segmentation/term/entity/QA/alignment) | YES — `Overlay`+`RunRange`+`Span` (`overlay.go`) | not a G concern (interpretations, not structure); meta.terminology/text-analysis on Vocabulary | N/A — doclang carries none |
| **L10n / targets / variants** | YES — `Targets map[VariantKey]*Target`, Tone/Channel, Status, Origin (`target.go`) | Vocabulary `meta.target-pointer` (`constructs.yaml:938-958`) | Writer emits target-else-source per locale (`writer.go:266-275`); DocLang is monolingual |
| **Whitespace / preserve** (`<content>` xml:space, `spec.md:2653`) | YES — `Block.PreserveWhitespace` (`block.go:26`) | Vocabulary `meta.preserve-space` | NO — `<content>` not specially handled; reader trims whitespace (`reader.go:518-534`) |
| **Pagination** (`<page_break>`, `spec.md:2016`) | Partial — `GeometryAnnotation.Page` (`structure.go:129`) | **G4** | NO — `<page_break>` skipped (`reader.go:178`); Page never set |

### Cross-format reach (who already emits the G signals — for calibration)
`SetSemanticRole`: markdown, docling, html, image, csv, openxml(roles/wml), doclang.
`SetGeometry`: odf, pdf(wasm), docling, openxml(sml/dml), doclang, idml, core/vision.
`SetLayoutLayer`: docmeta, docling, html, openxml(roles/dml), doclang.
`AddRelation`: image only (+model). `Glyphs`: pdf(wasm), editor/anatomy.
`ReadingOrder` set: image, core/vision/layout. (All via grep over non-test `.go`.)
Note (`SHARPEN:283-297` open Q): `odf`/`idml` emit geometry but **no** roles → cap
at G2 under a strict cumulative ladder.

---

## 6. Bottom line for the gap analysis
- **MODEL is a near-superset of DocLang's structural expressiveness**: it carries
  every DocLang axis (role, plane, geometry incl. per-glyph, relations, level,
  reading-order, page) plus visibility/overlay/metadata planes DocLang lacks. The
  only DocLang nuance the model flattens: per-axis (x vs y) `<location>` resolution
  (model has one `Resolution`), checkbox/marker glyph state, code-language label.
- **No shipped axis measures any of it.** Vocabulary scores only inline runs + the
  legacy `Block.Type` (6 block-kinds), explicitly not `StructureAnnotation.Role`.
  The G0–G4 axis that would measure roles/order/grids/geometry **exists only as a
  proposal** (`SHARPEN-PROPOSAL.md §2`) — no `gateStructure`, no `_structure_axis`,
  no `structure.yaml`, rubric still "six axes."
- **The `doclang` reader implements ~G3 + partial G4** but is the weakest link
  vs its own spec: it drops relations/threading (no `RelationAnnotation`), the
  entire field/form model, picture payloads, markers, checkboxes, span
  continuations, head metadata, and page breaks. The writer is faithful for the
  subset the reader recovers (XSD-validated). doclang is positioned as the
  conformance oracle for the role/layer/geometry vocab (`SHARPEN:184-187`).
