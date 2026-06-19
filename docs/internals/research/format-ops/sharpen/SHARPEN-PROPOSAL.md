# Sharpening the Format-Maturity Framework — Structure/Geometry axis + intuitive axis families

Decision-ready proposal. Grounds: the four deep-dives in this dir (A structure
model, B vision stack, C axes-fit, D prior-art) plus first-hand reads of the
scorer (`.claude/workflows/format-triage.js`), the audit floor
(`.skills/refresh-format-maturity/scripts/audit-format.py`), the repro mirror
(`.skills/refresh-format-maturity/scripts/repro-check.mjs`), the rubric
(`docs/internals/format-maturity.md`), the tier file (`core/formats/support.yaml`),
and the dashboard (`web/src/pages/format-maturity/`). All paths repo-relative.

---

## 1. Intuitive categorization of the axes

Today the rubric lists six axes as a flat row of cryptic letters
(`format-maturity.md:99-106`): Engine L, Vocabulary V, Editor E, Knowledge K,
Corpus C, Security S. Once the new axis lands there are **seven**. They group
into **three families**, named by the *question each answers* — and the user's
own framing ("how deeply we read it / how we prove it / how we work with it")
maps onto this grouping one-to-one.

### Recommended grouping — Comprehension / Assurance / Enablement

| Family | Mental model (one line) | Axes |
|---|---|---|
| **Comprehension** | *How deeply we read it* — fidelity at three resolutions: bytes, inline, structure. | Engine (L), Vocabulary (V), **Structure/Geometry (G)** |
| **Assurance** | *How we prove it* — does it hold over real and hostile files? | Corpus (C), Security (S) |
| **Enablement** | *How we work with it* — can a person / model / native editor act on it? | Knowledge (K), Editor (E) |

The three "Comprehension" axes are the same fidelity question at increasing
resolution: **Engine** = byte/round-trip/parity fidelity of the serialization;
**Vocabulary** = inline/run-level meaning (`fmt:*`/`link:*`/`media:*`/`code:*`
*within* a block); **Structure/Geometry** = block-level + cross-block + spatial
structure. The new axis lands here as the natural third member — which is the
whole point of adding it (D §5 Scheme A; C §"Intuitive grouping").

### Mapping the gating rule onto families

The headline tier is `min` over the **gating axes** (Engine ∧ Corpus ∧
Knowledge — `format-maturity.md:26-27,40`). Note these three now straddle all
three families (Engine→Comprehension, Corpus→Assurance, Knowledge→Enablement).
That is fine and honest: the families are a *reading aid for the dashboard*, not
a gating unit. The `min`-over-a-named-set rule is unchanged — it operates on the
axis set, never on a family. Tradeoff to accept: no single family "gates the
tier," so the dashboard must keep marking the three gating axes individually.

### Tradeoffs vs the runner-up grouping

Report C's alternative — **Bytes** {Engine, Security} / **Model** {Vocabulary,
G} / **Ecosystem** {Editor, Knowledge, Corpus} — is crisper about *where in the
stack* each axis lives (wire vs content-model vs product). But it (a) does not
match the user's "read / prove / work" mental model, (b) splits Engine away from
the two content-model fidelity axes even though round-trip is also "how deeply we
read it," and (c) puts Security with Engine rather than with Corpus, where
"prove it over hostile + real files" reads more naturally. Recommend
**Comprehension / Assurance / Enablement**; keep Bytes/Model/Ecosystem only as an
optional secondary caption if the dashboard wants a stack-oriented view.

Naming variants for the families (all acceptable; first is recommended):
Comprehension/Assurance/Enablement › Fidelity/Trust/Reach › Understanding/Robustness/Workability.

---

## 2. The new axis — Structure & Geometry (G0–G4)

**Name:** Structure & Geometry. **Axis id:** `structure`. **Grade prefix:** `G`
(S is taken by Security; G signals the geometry top-rung). **Grades:** G0–G4.
**Subtitle for the dashboard:** "comprehension depth."

> **G = how much of the document's logical and spatial structure the reader
> recovers and the model represents** — block roles, reading order, table grids,
> cross-block relations, and page geometry.

This is the ladder the user named ("metadata / OCR-to-text / OCR+structure +
geometry") and the only thing the vision/OCR/structure stack (#900–#912) does
that no current axis scores (C §2; D §0). It rides the standoff payloads in
`core/model/structure.go` — additive, no proto/KLF schema change
(`structure.go:1-18`).

### The ladder (cumulative; each rung = one richer standoff payload, in value order)

Grounded in what `core/model/structure.go` + the Layer/Group tree can carry,
Docling's five layers (D §1.1), and the AD-028/AD-029 tiers (B §2). Geometry is
the **top** rung — it is the hardest to recover faithfully and, for
localization, the least directly useful (it is declared read-only / writer-
ignored reconstruction metadata, `structure.go:121-126`); logical structure is
what a translation flow acts on. This ordering matches the user's example.

| Level | Name | What's recovered | Realized via (`structure.go` + readers) |
|---|---|---|---|
| **G0** | Opaque / flat | A `model.Media` part, or undifferentiated text blocks: no normalized `Role`, no Group nesting beyond the doc Layer, no geometry. The "destructive flatten." | image `ocr=false` (`image/reader.go:195-205`); PDF fast path (1 block/page); most key-value catalog formats |
| **G1** | Metadata | Text *about* the asset/doc classified onto the metadata plane (`SetLayoutLayer(LayerMetadata)`, `core/docmeta`) and/or alt-text/caption blocks (`RoleCaption` + `AddRelation(RelCaptionOf,…)`). No in-content body structure. | image metadata + alt-text (`image/reader.go:211-216`); PDF Info dict |
| **G2** | Linear body text | The body content recovered as text in correct reading order (stream order), optionally grouped by the `Layer`/`Group` tree — but roles still generic (`Block.Type` only), no normalized `StructureAnnotation.Role`. | OCR plain text (`vision.BlocksFromOCR`); catalog/line formats that emit `PartGroupStart` |
| **G3** | Logical structure | Every block carries a normalized **`StructureAnnotation.Role`** (heading+`Level`, paragraph, caption, footnote, list-item, code, formula) + **`LayoutLayer`**; reconstructed **table grids** (`Group{Type:"table"/"table-row"}` + `RoleTableCell`/`RoleTableHeader`); typed **`RelationAnnotation`** edges; explicit reading order. | docling/doclang readers; `structure.Gridify`/`TableToParts` (`analyze.go:222,375`); vision tier-3 |
| **G4** | Spatial geometry | G3 **plus** a **`GeometryAnnotation`** (page + bbox + origin + resolution) on blocks; `Z` for overlay planes; per-glyph **`Glyphs`** at the top sub-rung. Enough to reconstruct the page / round-trip to DocLang `<location>`. | AD-028 geometry/glyphs path; DoclingDocument prov bboxes; doclang 512-grid |

The user's concrete image example maps exactly: **metadata-only = G1, OCR-text =
G2, +headings/tables/reading-order = G3, +geometry/bboxes = G4** (G0 reserved for
the opaque Media-only case so the rung also covers catalog formats with no
document structure).

### Deterministic floor signals (greppable, mirroring Vocabulary's `vocabtypes`)

Computed by the audit over non-test `.go` in `core/formats/<id>/`, exactly as
`vocabtypes` is grepped (`audit-format.py:466-475`). Floor-only axis — no
model-judged quality dims (spread 0, like Editor and Security):

| Rung | Floor signal (dimension id) | Code evidence that proves it |
|---|---|---|
| **G1** | `metaplane` | grep `SetLayoutLayer(.*LayerMetadata`, an import of `core/docmeta`, or `AddRelation(RelCaptionOf` |
| **G2** | `readingorder` | the package emits `model.PartGroupStart` and/or sets `StructureAnnotation.ReadingOrder` (`SetStructure`, `structure.go:189`) |
| **G3** | `roles` | `SetSemanticRole`/`SetStructure` (`structure.go:210,189`) **and** emits table Groups (`Type:"table"`/`"table-row"`) and/or `AddRelation` (`structure.go:264`); **plus** a roles/structure test (e.g. `TestRead…RolesAndGeometry`) |
| **G4** | `geometry` | `SetGeometry` (`structure.go:255`) **plus** a geometry test; G4-top also emits `GeometryAnnotation.Glyphs` |

These are the same kind of deterministic file facts the audit already computes
for Security (`safeio` import / `Fuzz*` target — `audit-format.py:869-898`), so
the published level is fully pinned and reproducible.

### How G differs from Vocabulary and Engine (the orthogonality argument)

- **vs Engine (L) — round-trip.** L measures byte/round-trip/parity fidelity of
  the *serialization* (`gateEngine`, `format-triage.js:422-440`). A reader can be
  L4 byte-faithful while flattening to one block per page (G0). PDF is ~L2 but
  its entire value proposition is the structure tiers — invisible to L (D §4.1;
  B §4 PDF row). Orthogonal.
- **vs Vocabulary (V) — inline meaning.** V measures *inline / run-level* meaning
  (`fmt:*`/`link:*`/`media:*`/`code:*` within a block) plus block-kind via
  `constructs.yaml` → the **old** free-form `Block.Type` field. G measures
  *block-level + cross-block + spatial* structure via the **new**
  `StructureAnnotation.Role` standoff layer — a *different field* the new readers
  populate (`SetSemanticRole`) that `constructs.yaml` does not model at all (C
  finding #2). Proof of independence: **docling is V0** (plain runs,
  `docling/reader.go:18-20`) **but high-G** (roles/tables/geometry/order);
  **html is V1** (typed inline) **but low-G** (no geometry). Two genuinely
  independent axes — G is *not* an extension of V.

### Authority qualifier — orthogonal, do NOT collapse into the rung

The AD-028 tiers (1 tagged tree / 2 geometric inference / 3 ML layout) are
*provenance/confidence*, not richness: the same G5-class payloads can arrive via
authoritative tags (tier 1) or an ML guess (tier 3) — same depth, different trust
(B §2, §6; D §4.1). Carry the authority as a **per-format qualifier badge**, not
as a G rung. `GeometryAnnotation.SourceRef` (`structure.go:140-142`) already
records provenance; declare the tier in the new `structure.yaml` artifact (§4).

---

## 3. How the four new formats slot in (honest position on every axis)

Vectors below are the floor today (C §"Scores") plus the proposed G grade and the
authority tier. `image`/`pdf` are **capability-conditional** — score the ceiling
the format+plugin can reach and the floor it degrades to (B §6: depth is
environment-dependent on plugin install + `ocr`/`layout`/`tier3` toggles).

| Format | Engine | Vocab | Editor | Know | Corpus | Sec | **Structure/Geometry** | Authority |
|---|---|---|---|---|---|---|---|---|
| **image** | L0\* | V0 | E0 | K0 | C0 | S0 | **G1 floor → G4 ceiling** (Media+meta+alt at G1; OCR=G2; tier-2/3 structure=G3; bbox/glyph=G4) | geometric (T2) or ML (T3) |
| **pdf** | (L0; excluded†) | — | — | — | — | — | **G1 (Info) → G4** (the **only** Tier-1 holder; tagged tree=G3, geometry/glyphs=G4) | tagged (T1) › geometric › ML |
| **docling** | L0‡ | V0 | E0 | K0 | C0 | S0 | **G3–G4** (roles+tables+order always; geometry from prov bbox) | native (given) |
| **doclang** | L1 | V0 | E0 | K0 | C0 | S0 | **G3–G4** (as docling **+** inline fmt runs **+** faithful writer/round-trip) | native (given) |

\* `image` is L0 only because `Config` lives in `reader.go`, not `config.go`
(`gateEngine` L1 requires `has('config')`, `format-triage.js:427`) — a filename
artifact, not a real deficiency (C §"Secondary finding"). Fix in the same pass.
† `pdf` has no in-core `reader.go` so the dir-walk universe drops it
(`lib.mjs:94-101`) — it will vanish from the dashboard even though it is the G
axis's flagship Tier-1 format (open Q in §5).
‡ `docling` is L0 because read-only detection is hardcoded to `pdf`
(`ftype`, `format-triage.js:563`); a real read-only `na` writer cell never fires.

**What each needs to climb:**

- **image** → G2/G3/G4 are gated by the **kapi-vision plugin** at runtime, not by
  in-core code (`image/reader.go:305-336`). To *certify* the ceiling, the audit
  must read it from the plugin path / nightly `vision-onnx` job, not from a grep
  of the in-core package (which only proves G1). Engine: move `Config` to
  `config.go`; add `spec.yaml` + a malformed test.
- **pdf** → needs a dashboard seat (universe exception). G3/G4 are
  plugin-provided (`kapi-pdfium`) + tier-3 host decorator; the floor signal lives
  out-of-core, so its G grade must be declared in `structure.yaml`, not grepped.
- **docling** → fix the read-only classification so Engine isn't artificially L0;
  it is already a clean G3–G4. Inline runs (`reader.go:18-20`) keep it V0 — that's
  correct, not a gap.
- **doclang** → highest-ready: G3–G4 + a faithful writer. Add `spec.yaml` refs
  (K), a corpus manifest (C), and it is the natural **conformance oracle** for the
  whole role/layer/geometry vocabulary (every other format speaks the same role
  names by design, `structure.go:31-33`).

---

## 4. Implementation surface (mirror exactly how Security was added)

Security shipped as: scorer `AXES` entry + floor-only dim set + a cumulative gate
fn + an audit `_*_axis` floor + the repro mirror + rubric §2.6 + dashboard types
+ bootstrap re-export. Mirror that, minimally:

1. **Scorer `AXES`** — add `structure: ['G0','G1','G2','G3','G4']`
   (`format-triage.js:62-72`); add to `AXIS_LABELS` (`:74`). `RANK`/`NEXT`
   auto-derive (`:77-79`).
2. **`AXIS_DIMS`** — add `structure: ['metaplane','readingorder','roles','geometry']`
   (`:87-96`); `CANON`/`DIM_AXES` auto-build (`:97-104`); add to `LABELS`
   (`:106-115`); `QUALITY.structure = new Set()` — floor-only, no judged dims
   (`:119-126`).
3. **Gate fn `gateStructure(dims)`** — copy `gateSecurity` (`:499-506`) as a pure
   cumulative floor ladder G0→G4 (`full('metaplane')`→G1, `readingorder`→G2,
   `roles`→G3, `geometry`→G4); wire into `axisLevel` (`:533-542`).
4. **`SCORE` schema** — add `structure` to `delta_justification` (`:166-174`) and
   `blocking_gaps` (`:175-186`) property maps.
5. **Score-phase dims** — add `structureDims(floor)` echoing
   `floor.axes.structure.signals.cells` (copy `securityDims` `:381-390`); include
   in `floorDimsAll` (`:393-396`).
6. **Audit floor** (`audit-format.py`) — add `_structure_axis(d, fmt)` (copy
   `_security_axis` `:869-923`): the four greps above → `cells` + cumulative
   `base`; set a per-format **`ceiling`** for the `na` rule (see §4 applicability)
   exactly as Vocabulary/Security do (`:516`, `:909-913`). Add `structure` to the
   audit's `AXES` map (`:87`), to `_axes_block` (`:926-935`), and a hint block to
   `_axis_hints` (`:938+`).
7. **Repro mirror** (`repro-check.mjs`) — add `structure` to `AXES` (`:27-35`),
   `JUDGED.structure = []` (floor-only → spread 0, `:44-47`), a `structureDims`
   echo (`:161-173`), the gate, and the `axisLevel` wiring (`:175+`, `:273+`).
8. **Rubric §2.7** (`format-maturity.md`) — new **§2.7 Structure & Geometry
   (G0–G4)** in the §2.6-Security shape (ladder table + floor-signal paragraph +
   non-gating note); add the row to the §2 axis table (`:99-106`); update the
   "six axes" → "seven axes" prose at `:28`, `:90`, `:96-97`; add the open-Q to §6.
9. **Dashboard** (`web/src/pages/format-maturity/_types.ts`) — add `"structure"`
   to `AxisId` (`:20`), `AXIS_IDS` (`:127-134`), `AXIS_LABEL` (`:136-143`),
   `AXIS_GRADES` (`:145-152`), `AXIS_DIMS` (`:160-178`); add a `StructureGrade`
   union (`:17-18`); update the legend line in `index.tsx` (`:408-409`). The
   render path is data-driven (`axis_labels?.[a]`, `index.tsx:95`), so most of the
   UI follows automatically.
10. **`bootstrap-publish.mjs`** — re-exports the scorer prelude (`:66`), so
    `structure` flows through automatically; add one entry to the missing-artifact
    hint table (`:141-149`). `axes_published` slices `AXIS_IDS` (`:264`) so it
    self-includes.
11. **Bump `scorer_version` 3 → 4** — the published dataset shape gains an axis:
    `buildDataset` (`format-triage.js:696`), the publish-prompt watermark
    (`:672`), `bootstrap-publish.mjs:263`. Old datasets stay readable (the dataset
    reader is version-tolerant, `:719-728`).
12. **Go guardrail** (`core/formats/maturity_test.go`) — optional advisory
    `TestStructureCoverage` counting formats without `structure.yaml` (copy
    `TestVocabularyCoverage` `:398`); if `structure.yaml` carries `na`/authority
    declarations, validate its shape in `TestSupportYAML`-style.

**New per-format artifact `core/formats/<id>/structure.yaml`** (mirrors
`vocabulary.yaml`; only needed for formats claiming G3/G4 or declaring `na`):
declares the **authority tier** (`native|tagged|geometric|ml`) and the
countersigned **`na` geometry cell** for non-spatial formats
(`reviewed_by` + date — the `na` countersignature mechanism, `format-maturity.md:112-117`).

**`support.yaml` backfill** — none required for launch: G is non-gating (§5), and
the three in-core new formats already have entries (`doclang:53`, `docling:60`,
`image:67`). `last_certified` refreshes on the next `triage-score` publish; the
writer partition (tier/gates) is untouched (`support.yaml:3-6`).

### Applicability note for binary / OCR-only formats (the user's specific ask)

The G axis needs an explicit applicability paragraph because its top rung is
spatial:

- **Geometry rung is `na` for non-spatial formats.** Pure key-value catalogs
  (json, properties, resx, yaml, po, the harvest set) have no intrinsic geometry
  (`structure.go:121-126`). Their `geometry` floor cell is **`na` as a ceiling
  cap** — the audit sets `structure.ceiling = G3` (or G2), so they cannot reach
  G4. **Important nuance:** unlike Security/Engine, `na` on this *cumulative
  depth* ladder must mean "ceiling capped below this rung," **not** "gate passes
  this rung" — otherwise a roles-only catalog would falsely promote to G4. Mirror
  the audit's existing per-axis `ceiling` mechanism (`:516`, `:909-913`), not the
  gate's `full('na')==pass` shortcut.
- **Round-trip is N/A for the asset/OCR boundary, and Engine already handles
  it.** `image` is the only `IsBinaryAssetFormat` (`core/project/asset.go:16-23`):
  its writer emits bytes + an alt-text sidecar, and an on-disk localized variant
  is authoritative (`kapi run`/`merge` keep it). This is a *whole-asset
  replacement*, not a structural serialization round-trip — Engine's existing
  read-only `na` writer-cell path covers it; the G axis does not re-litigate
  round-trip. The fix needed is the `ftype` taxonomy: `read-only` is hardcoded to
  `pdf` (`format-triage.js:563`), so `docling` (and the OCR boundary) miss the
  `na` patch — generalize it to a `read-only` set.

---

## 5. Open questions / decisions for the maintainer

1. **One axis or two (logical structure vs physical geometry)?** —
   **Recommend ONE** cumulative axis (G0–G4) with geometry as the top rung,
   because (a) the user's example treats it as one ladder; (b) geometry without
   logical structure is rare and low-value (writer-ignored reconstruction
   metadata); (c) two axes double the dashboard width for a dimension only ~5
   formats reach. **But there is a real wrinkle the maintainer must rule on:**
   `odf` and `idml` emit `SetGeometry` but **no** `SetSemanticRole` (A §10 —
   `odf/geometry.go:103`, `idml/geometry.go:161`). Under a strict cumulative
   ladder they cap at **G2** despite carrying geometry, because they recover no
   roles. That is *defensible* ("positioned text we don't understand the structure
   of" ≠ comprehension) but counterintuitive. Three resolutions: (a) keep
   cumulative, odf/idml cap at G2 until they emit roles **[recommended]**; (b)
   make geometry an independent top-up (breaks the clean ladder); (c) split into
   two axes (Structure G + Geometry as its own). This is the single most important
   decision; it determines whether G is one ladder or two.

2. **Is G a gating axis for the support tier?** — **Recommend non-gating display
   axis at launch**, exactly as Security launched (`format-maturity.md:393-398`):
   it informs and ranks structure work without blocking releases. ~46 of 51
   formats are G0/G1, so gating it immediately would mass-demote the fleet.
   Promotion to gating (candidate rule: "Supported requires **G2** where structure
   is applicable, `na`-exempt for catalogs") is a later **tier-review** policy
   decision, recorded in §1 when taken — never an audit outcome.

3. **Naming.** — **Recommend** axis id `structure`, label **"Structure &
   Geometry"**, grades **G0–G4**, dashboard subtitle "comprehension depth."
   Alternatives: "Comprehension" (collides with the family name), "Layout,"
   "Depth (D-prefix)." G is the only free single letter (L/V/E/K/C/S taken) and
   reads as the geometry top-rung.

4. **Authority qualifier — ship now or later?** — Recommend declaring the AD-028
   tier (`native|tagged|geometric|ml`) in `structure.yaml` **now** (cheap;
   `SourceRef` provenance already exists) but rendering the badge **later**. Keep
   richness (the G rung) and authority (the tier) as separate fields — never one
   number (B §6).

5. **Does `pdf` (and other plugin-provided/out-of-core formats) get a dashboard
   seat?** — `pdf` has no in-core `reader.go`, so the dir-walk universe excludes
   it (`lib.mjs:94-101`) and it will *disappear* from the dashboard on the next
   publish — yet it is the G axis's flagship Tier-1 format. **Recommend** a small
   allowlist exception for plugin-provided formats whose G/authority is declared in
   `structure.yaml`, otherwise the axis's best example is invisible. (Tangential
   cleanup in the same pass: delint the hardcoded "49 real formats" counts —
   actual is 51 — at `lib.mjs:85`, rubric `:57,:667`; they violate the brand rule
   against code-controlled counts.)

6. **`na` semantics on a depth ladder.** — Confirm the implementation decision in
   §4: on this cumulative axis, `na` on the geometry rung is a **ceiling cap**, not
   a gate-pass. This differs from every other axis's `na` and must be coded in the
   audit's `ceiling`, not the gate's `full()`.
