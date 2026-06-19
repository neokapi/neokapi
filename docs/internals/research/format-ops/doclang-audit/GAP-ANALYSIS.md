# GAP ANALYSIS — DocLang expressiveness vs neokapi (model + maturity axes + G ladder)

Inputs: `1-doclang-inventory.md` (DocLang v0.6 yardstick) and `2-framework-coverage.md`
(neokapi `core/model` + maturity axes + proposed Structure&Geometry **G0–G4** + the
`doclang` reader). Cross-checked against `core/model/structure.go`,
`core/formats/constructs.yaml`, and `docs/internals/research/format-ops/sharpen/SHARPEN-PROPOSAL.md`.

**Completeness question:** *what can DocLang express that neokapi does NOT measure or
represent?* Each gap is classified:

- **Model gap** — `core/model` cannot represent it (most serious).
- **Axis gap** — model represents it, but no maturity axis (incl. the new G) rewards it.
- **Floor-signal gap** — G (or another axis) conceptually covers it, but the deterministic
  grep floor signal wouldn't detect/distinguish it.
- **Vocabulary gap** — an inline/role construct missing from `constructs.yaml`.
- **Covered** — explicitly confirmed represented + measured.

---

## Headline verdict

The model is a **near-superset** of DocLang's *structural* expressiveness (it carries
role, plane, geometry-incl-glyphs, relations, level, reading-order, page — plus
overlay/metadata/visibility/variant planes DocLang lacks). So almost nothing is a *hard*
model gap. The real exposure is **measurement**: the G ladder, as drafted, has two
calibration defects that would make it mis-score the very formats it exists to reward, and
the model's two genuine representation holes (**table cell spans** and **per-axis geometry
resolution**) ride only the untyped `Block.Properties` escape hatch with no canonical field
and no signal. The largest DocLang surface neokapi ignores — the ~90 governance/PII/RAG/
training head elements — is correctly **out of scope** for a content/structure model.

---

## COVERED (big-ticket confirmations — the report is balanced)

| Capability | Model | Axis | Status |
|---|---|---|---|
| **Logical roles** (20 DocLang semantic elements) | `Role*` ×17, *open* vocab, "aligned with DocLang taxonomy" (`structure.go:31-52`) | G3 `roles` floor signal (`SetSemanticRole`) | **Covered.** Open vocab represents any role string; canonical set names the big ones. |
| **Tables / grids** (OTSL) | `Group{Type:"table"/"table-row"}` + `RoleTableCell`/`RoleTableHeader` | G3 (table-Group signal) | **Covered for topology** — *spans excepted* (Gap #2). |
| **Reading order** | implicit Part-stream order + `StructureAnnotation.ReadingOrder` | G2 `readingorder` | **Covered** (signal is weak — Gap #9). |
| **Geometry / bbox / layer / z** | `GeometryAnnotation` (page+bbox+resolution+origin+**Z**+**Glyphs**); `LayoutLayer` 5-value superset of DocLang's 3 | G4 `geometry`/glyphs | **Covered, model EXCEEDS** — DocLang has no glyphs/Z/overlay/metadata plane (*per-axis resolution excepted* — Gap #6). |
| **Provenance / authority** | `GeometryAnnotation.SourceRef`; `Target.Origin{Kind,Engine,…}` | SHARPEN authority qualifier `native\|tagged\|geometric\|ml` in `structure.yaml` | **Covered.** DocLang's own provenance is thin (classifier `score` on `<language>`/`<topic>` head only); model meets/exceeds. |
| **Captions** | `RoleCaption` + `RelCaptionOf` | G1/G3 | **Covered** (reader-impl drops it; not a framework gap). |
| **Overlays / segmentation / change-tracking / L10n targets+variants** | `Overlay`+`RunRange`+`Span`; `Targets map[VariantKey]`; `meta.change-tracking` | Vocabulary | **Covered — model EXCEEDS DocLang** (DocLang is monolingual, no standoff/segmentation). |
| **Visibility / presence-condition** | `Visibility*` ×5 | — | **Model superset** (no DocLang counterpart). |

---

## Ranked gaps (most severe first)

### #1 — G1 `metaplane` signal is too narrow; strict-cumulative gating caps the conformance oracle at G0
**Class: Floor-signal gap (calibration bug). Severity: HIGH — leaves format value
grossly mis-measured on day one.**

The proposed G1 floor signal trips on exactly three things (`SHARPEN:110`):
`SetLayoutLayer(…LayerMetadata)`, an import of `core/docmeta`, or `AddRelation(RelCaptionOf`.
But DocLang's `<layer>` enum is only `{body, background, furniture}` (`spec.md:2587`) — it
has **no metadata value** — and the `doclang` reader emits no relations and drops captions on
read (`2-framework-coverage.md §4`). So the doclang reader can **never** call any of the
three. Under a strict cumulative ladder (copied from `gateSecurity`, where G3 requires
G1∧G2), **doclang — the intended role/layer/geometry conformance oracle — is capped at G0**,
as is any format that recovers furniture/background/overlay planes but not the *metadata*
plane (openxml, html, odf). This is the dual of the odf/idml "geometry but no roles → cap at
G2" wrinkle already flagged in `SHARPEN:283-297`, but worse and unflagged.

**Recommendation (BEFORE commit):** broaden the G1 `metaplane` signal to *any non-body*
`SetLayoutLayer(...)` (plane recovery), OR demote plane recovery out of the gating chain so a
roles-rich format isn't capped by a missing metadata plane. Re-derive doclang's G grade after
the fix (it should land G3–G4, per `SHARPEN:160`).

---

### #2 — Table cell spans (colspan/rowspan) have no canonical model field and no signal
**Class: Model gap (soft) + Floor-signal gap. Severity: HIGH — blocks faithful
representation of any merged-cell table (ubiquitous).**

DocLang carries `<lcel/>` (colspan continuation), `<ucel/>` (rowspan continuation), `<xcel/>`
(cross span) (`spec.md:2937-2969`). The model represents a table only as nested
`Group{table}→Group{table-row}→cell Block`; there is **no `ColSpan`/`RowSpan`** on
`StructureAnnotation` (or anywhere typed). Span counts can only ride the untyped
`Block.Properties` with no reader/writer convention. The `doclang` reader **drops
lcel/ucel/xcel** (`2-framework-coverage.md §4`), so spanned grids reconstruct misaligned —
yet it would still score **G3** because the G3 signal only checks for table *Groups*, not span
fidelity. The axis would therefore *certify* a table reader that silently mangles spans.

**Recommendation:** add typed `ColSpan int`/`RowSpan int` to `StructureAnnotation` (additive,
rides the same standoff payload). **BEFORE commit**, either add a G3 span sub-signal/corpus
check or explicitly document in the rubric that G3 does **not** certify span fidelity (so the
grade isn't read as a faithfulness claim).

---

### #3 — Cross-block continuation/threading: no canonical relation type, and relations are OR-optional at G3
**Class: Axis gap + Vocabulary gap (relation vocab). Severity: MODERATE-HIGH — blocks
faithful representation of multi-column and cross-page split content.**

DocLang's `<thread thread_id>` joins fragments of one logical component split across bounding
boxes / columns / pages (`spec.md:2478-2494`); `<h_thread>` does the horizontal/cross-page
case (FUTURE). The model's relation type set (`RelCaptionOf, RelFootnoteOf, RelLabelFor,
RelTriggers, RelReferences`, `structure.go:83-89`) has **no continuation/same-flow relation**.
The set is *open* so a reader could `AddRelation("continues", …)`, but there is no canonical
constant and the doclang reader emits **zero** relations. Worse, the G3 signal is
"`SetSemanticRole` **AND** (table Groups **OR** AddRelation)" — relations are *optional*, so a
roles+tables reader passes G3 with no relation graph at all. Multi-column reading order is
linearizable via `ReadingOrder`, but the *join* semantics (re-assemble a paragraph split
across a column break) require the continuation edge.

**Recommendation:** add a canonical `RelContinues` (thread) constant (follow-up).
**BEFORE commit**, decide and *document* whether relations are a distinct G3 sub-signal or
deliberately OR-lenient — otherwise the grade overstates relation recovery.

---

### #4 — Forms / fields model is thin and entirely unmeasured
**Class: Axis gap (+ soft model). Severity: MODERATE — blocks faithful representation of
forms / AcroForm-style content.**

DocLang has a rich form cluster: `field_region, field_heading, field_item, key,
value@class{read_only|fillable}, hint, checkbox@class{selected|unselected}`
(`spec.md:2148-2434,2635`). The model has only generic `RoleFormField` + `RelLabelFor`; no
canonical roles for key/value/hint/field-item, **no fillable-vs-read-only state, no
checkbox-checked state** (these can only ride free-form `Block.Properties`). No G signal
distinguishes form recovery — a form reader scores generic G3. The doclang reader drops the
whole cluster.

**Recommendation (follow-up):** add canonical form roles + a `checked`/`fillable`
`Block.Properties` convention (or `StructureAnnotation` fields); optionally a forms sub-signal.
Not a G-axis blocker — note as a known under-measurement.

---

### #5 — Inline directionality (RTL) is not an inline construct
**Class: Vocabulary gap (+ minor model nit). Severity: MODERATE — localization-relevant
(bidi correctness).**

DocLang `<rtl>` is an **inline**, nestable direction marker in raw text (`spec.md:2817`).
`constructs.yaml` has only block-level `meta.directionality` (`model_mapping: block:property`,
line 877) — there is no inline directionality run construct, and the Run union has no
direction-isolate member. Inline RTL spans inside an LTR paragraph cannot be represented as a
run or scored.

**Recommendation (follow-up, but localization-relevant):** add an inline directionality
construct (`run:pc`, e.g. a `fmt:bidi` / Unicode bidi-isolate pack type) to `constructs.yaml`.

---

### #6 — Per-axis (x vs y) geometry resolution is flattened to one `Resolution int`
**Class: Model gap (minor). Severity: LOW-MODERATE — geometry is read-only reconstruction
metadata; only bites non-square normalized grids.**

DocLang `location@resolution` is a per-axis exclusive bound defaulting to
`default_resolution@width|height` (`spec.md:2568, 3025`) — width and height resolution can
differ. `GeometryAnnotation.Resolution` is a single `int` (`structure.go:134`), so an
asymmetric grid (e.g. 1000×512) cannot round-trip faithfully.

**Recommendation (follow-up):** add `ResolutionY int` (default = `Resolution`), or document as
an accepted reconstruction-only loss. Not a G-axis blocker.

---

### #7 — `handwriting` inline construct missing
**Class: Vocabulary gap. Severity: LOW.**

DocLang `<handwriting>` (inline, `spec.md:2797`) is absent from `constructs.yaml`'s
inline-format set (bold…subscript, code, highlight, ruby, ui-widget). Representable as a
`PcOpen/PcClose` run only with a new type.

**Recommendation (follow-up):** add `inline.handwriting` + a `fmt:handwriting` pack type (or
treat as a provenance property — but DocLang models it as formatting).

---

### #8 — Fine subclass / subtype dropped (code language, picture chart-type, free `<label>`)
**Class: Vocabulary/model nit. Severity: LOW — but code-language is a DNT signal.**

DocLang carries fine subtype via `<label value>` (free) and `class` enums: code language
(`label@value` = Linguist key), picture chart subclass (bar/pie/…), etc. (`spec.md:2460,
3050-3075`). The model's `Role` is the coarse type; the subclass has no home but
`Block.Properties`, and the reader drops it.

**Recommendation (follow-up):** define a `Block.Properties` convention (`code.language`,
`picture.subclass`). Code language matters for L10n (do-not-translate / syntax).

---

### #9 — G2 `readingorder` signal is weak (PartGroupStart ≠ correct order)
**Class: Floor-signal gap. Severity: LOW-MODERATE — G2 false-positives.**

The G2 signal accepts emitting `model.PartGroupStart` *or* setting `ReadingOrder`
(`SHARPEN:111`). Almost every grouped/catalog format emits `PartGroupStart` trivially, so G2
("body text in *correct reading order*") is passable without computing any reading order.

**Recommendation (BEFORE commit, doc-only):** note the leniency in the rubric; consider a
corpus-level reading-order check rather than tightening the grep.

---

### #10 — Document head / governance / PII / RAG / training metadata (~90 elements) not modeled
**Class: Out-of-scope (Model can carry opaque). Severity: LOW for structure; HIGH only if
byte round-trip of a governance-annotated DocLang doc is a goal.**

DocLang's biggest differentiator surface — `<head>` core metadata + the FUTURE
governance/PII/extraction/RAG/training blocks (`spec.md:3211-3712`) — has no neokapi
representation and no axis. It is orthogonal to a localization content/structure model.

**Recommendation:** **out of scope** for the model and the G axis. If round-trip fidelity is
ever required, carry it opaque on `Layer.Properties`/`Annotations`. Do **not** add G rungs for
it.

---

### #11 — Pagination as a structural event; Page not separately ensured by G4
**Class: Floor-signal gap (minor) + reader-impl gap. Severity: LOW.**

`<page_break/>` and each block's page assignment are representable via
`GeometryAnnotation.Page` (`structure.go:129`), but: there is no Part-stream page-boundary
marker, and the G4 signal greps `SetGeometry` without checking `Page` — a reader can pass G4
setting only BBox. The doclang reader skips `<page_break>` and never sets `Page`.

**Recommendation (follow-up):** treat as reader-impl (doclang should set `Page`); optionally
add a Page sub-check to G4. Page-boundary-as-Part is not needed (Page field suffices).

---

### #12 — Canonical role-set completeness nits (index/TOC, marker, header sub-kinds)
**Class: Vocabulary nit (open vocab already represents them). Severity: LOW.**

No canonical `RoleIndex`/`RoleMarker`; the four OTSL header kinds (`ched/rhed/corn/srow`)
collapse to one `RoleTableHeader`. All representable via the *open* role/Properties vocab; only
the canonical naming is incomplete.

**Recommendation (follow-up):** add `RoleIndex`/`RoleMarker` and a header-subkind Property if
faithful header typing is wanted; otherwise accept open-vocab degradation.

---

## Prioritization: fold into the G axis BEFORE commit, vs track as follow-up

**BEFORE commit (calibration/correctness — the axis mis-scores without these):**
1. **Fix the G1 `metaplane` signal + cumulative gating** so plane-rich/role-rich formats
   (doclang, openxml, html, odf) aren't capped at G0/G2 (#1). *This is the single most
   important pre-commit fix — it determines whether the headline grades are meaningful.*
2. **Resolve the table-span measurement** (#2): add a G3 span sub-signal **or** document that
   G3 does not certify span fidelity; land `ColSpan`/`RowSpan` model fields alongside.
3. **Decide + document relation semantics at G3** (#3): distinct sub-signal vs OR-lenient.
4. **Rule on the odf/idml "geometry without roles → G2 cap"** open question
   (`SHARPEN:283-297`) — it and #1 are the two cumulative-ladder edge cases that must be
   settled before the ladder is frozen.
5. **Document the G2 reading-order leniency** (#9, doc-only).

**Track as follow-up (model/vocab enrichment — not axis blockers):**
- `ColSpan`/`RowSpan` typed fields (if not folded into #2) and `RelContinues` constant (#2,#3).
- Form roles + checkbox/fillable state convention + optional forms sub-signal (#4).
- Inline directionality construct/run type (#5).
- Per-axis `ResolutionY` (#6).
- `inline.handwriting` construct + pack type (#7).
- `code.language`/subclass `Properties` convention (#8).
- doclang reader: set `Page`; optional G4 Page sub-check (#11).
- Canonical `RoleIndex`/`RoleMarker` + header sub-kinds (#12).

**Explicitly out of scope:** governance/PII/RAG/training head metadata (#10) — opaque
passthrough only, never a G rung.
