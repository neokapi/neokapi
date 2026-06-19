# DEEP-DIVE C — the current axes + how the new vision formats (mis)fit

Survey of `docs/internals/format-maturity.md` (§1 tiers, §2 the six axes),
`.claude/workflows/format-triage.js` (AXES table / dims / gates), and the
deterministic audit (`audit-format.py`) run over the four new
vision/structure formats (`image`, `pdf`, `docling`, `doclang`), grounded
against the new content-model `core/model/structure.go`. Read-only.

Headline: the support-gate/universe machinery **absorbed the new formats
cleanly and is green** — the only stale artifact is the *published dashboard
JSON*. But the six axes are **structurally blind to document
structure + geometry**: a reader whose entire purpose is OCR + table/heading
recovery + reading-order + bounding-boxes (`image`) scores the **same all-zero
maturity vector as a trivial stub**, because no axis — and no floor signal,
dimension, or gate term — measures structural/geometric fidelity. Vocabulary
is the conceptual nearest neighbor but misses it three independent ways.

---

## (1) The six axes — crisp table of exactly what each measures

Quoted "Measures" column verbatim from `docs/internals/format-maturity.md:99-106`
(§2 axis table), with the gate function each axis actually computes
(`format-triage.js`) and the floor signals that feed it.

| Axis | Ladder | "Measures" (rubric §2 verbatim) | What it concretely tests | Gate fn (format-triage.js) | Floor dims (AXIS_DIMS, fmt-triage.js:87-96) |
|---|---|---|---|---|---|
| **Engine** | L0–L4 | "Parse/round-trip/parity fidelity and robustness" | read→write byte/semantic equality, Okapi head-to-head parity, malformed-without-panic | `gateEngine` :422-440 | reader, writer, config, spec, parity, malformed, corpus, detection, docs |
| **Vocabulary** | V0–V3 | "How richly format semantics map into the canonical content-model vocabulary (and back)" | **inline** runs (`fmt:*`/`link:*`/`media:*`/`code:*`) + nominal block-kinds (heading/paragraph/list/cell/quote → `Block.Type`) | `gateVocab` :446-454 | vocabmap, vocabtypes, writecells, equivalence |
| **Editor** | E0–E4 | "How close kapi gets to the format's native editing surface" | PreviewBuilder/STRUCTURE_RULES → identity-anchor round-trip → embedded add-in → event feed | `gateEditor` :458-465 | preview, identity, embedded, events |
| **Knowledge** | K0–K3 | "The spec/learning assets that let a person or model work on the format" | dossier.yaml, spec.yaml refs, snapshot-resolved citations, context-pack | `gateKnowledge` :470-477 | dossier, sidecar, refs, citations, contextpack |
| **Corpus** | C0–C3 | "Reference files that validate support, with provenance" | testdata manifest (`corpus.yaml`), Tier-B fetch, externally-verified wild files, sweeps | `gateCorpus` :484-492 | corpusmanifest, corpus, fetchwiring, acceptance, sweep |
| **Security** | S0–S4 | "Resource-boundedness, fuzzing, and hostile-corpus hardening of the parser" (non-gating display) | `core/safeio` budgets, Go fuzz target + seed, clean ledger sweep | `gateSecurity` :499-506 | safeio, fuzz, sweepclean, sustained |

### Intuitive grouping (for the categorization redesign)

The six collapse into **three natural families**, which is the cleaner mental
model to design from:

1. **THE BYTES (engine correctness)** — *Engine* (does the serialization
   round-trip faithfully against the spec/Okapi?) + *Security* (does the parser
   survive hostile bytes?). Both live at the wire/byte boundary.
2. **THE MODEL (representation fidelity)** — *Vocabulary* (does the format's
   *meaning* land in the canonical content model and back?). Today this is the
   **only** axis about content-model fidelity, and it covers **only inline +
   block-kind** meaning.
3. **THE ECOSYSTEM (product/process around the format)** — *Editor* (native
   surface), *Knowledge* (specs/learning assets), *Corpus* (provenanced
   evidence). These measure how well a format is *supported as a product*, not
   what the engine extracts.

The structural/geometric dimension the user named is a **sibling of Vocabulary
inside family (2) THE MODEL**: Vocabulary today = "inline + block-kind meaning";
the missing axis = "document **structure + geometry** meaning" (roles, reading
order, layout plane, bounding boxes, cross-block relations). It is *not* an
Engine concern (round-trip) and *not* an Editor concern (rendering surface).

---

## (2) How the new vision formats score today — and where the axes fail

### Scores (deterministic floor, `audit-format.py --json`, run this survey)

| Format | type (ftype) | Engine | Vocab | Editor | Know | Corpus | Sec | reader/writer/config/spec/testdata |
|---|---|---|---|---|---|---|---|---|
| `image` | harvest* | **L0** | V0 | E0 | K0 | C0 | S0 | reader✓ writer✓ **config✗** spec✗ testdata✗ |
| `docling` | harvest* | **L0** | V0 | E0 | K0 | C0 | S0 | reader✓ writer✗(read-only) config✓ spec✓ testdata✓ |
| `doclang` | harvest* | **L1** | V0 | E0 | K0 | C0 | S0 | reader✓ writer✓ config✓ spec✓ testdata✓ |
| `pdf` | read-only | (L0; **excluded from `--all`**) | — | — | — | — | — | reader✗ writer✗ config✓ |

`*` All three are classed **type=harvest** purely because they have no Okapi
counterpart (`ftype` in format-triage.js:561-567 only models
read-only(=pdf-hardcoded)/internal(=splicedlines)/harvest/parity).

These vectors are an indictment: `image` is one of the most capable readers in
the tree (below) yet ties the floor on **every** axis.

### What the formats actually do (the value the vector hides)

**`core/model/structure.go`** (11 KB, landed with the vision stack) adds three
**block-scoped stand-off payloads** carrying exactly the structure/geometry the
axes don't measure:

- `StructureAnnotation` (struct ~L94-117): `Role string`, `Layer string`,
  `Visibility string`, `Level int`, `ReadingOrder int`. Key
  `AnnoStructure = "structure"`.
- `GeometryAnnotation` (~L127-159): `Page int`, `BBox Rect` (`Rect{X,Y,W,H}`),
  `Resolution int`, `Origin string`, `Z int`, `SourceRef string`,
  `Glyphs []GlyphBox` (per-character boxes). Key `AnnoGeometry = "geometry"`.
- `RelationAnnotation` (~L172-180): `Relations []Relation` where
  `Relation{Type, Target}`. Key `AnnoRelations = "relations"`.
- Canonical open vocabularies (consts): `Role*` (paragraph, title, heading,
  caption, footnote, list, list-item, **table, table-cell, table-header**,
  code, formula, picture, page-header, page-footer, form-field, section,
  L34-52); `Layer*` (body, furniture, background, overlay, metadata, L58-66);
  `Visibility*` (visible, conditional, hidden, print-only, screen-only, L72-78);
  `Rel*` (caption-of, footnote-of, label-for, triggers, references, L84-90).
- Block accessors: `SetSemanticRole`, `SetGeometry`, `AddRelation`,
  `SetLayoutLayer`, `SetVisibility`.

**`core/formats/image/reader.go`** (package doc L1-20): emits the image as a
localizable `Media` part; when the kapi-vision plugin is registered it runs
**OCR → positioned text Blocks with `GeometryAnnotation` bounding boxes**,
recovers **tier-2 structure (headings/paragraphs/tables) from OCR line
geometry**, runs **ML layout detection (regions + reading order)** when
`Config.Layout` is on (`Config{OCR, Layout}`, L46-58), folds an `<image>.alt.txt`
sidecar into a `RoleCaption` + `RelCaptionOf` block that round-trips back to a
per-locale sidecar, and maps PNG/XMP metadata onto metadata-plane Blocks.

**`core/formats/docling/reader.go`**: walks DoclingDocument reading order
(`body.children`), maps `DocItemLabel → SemanticRole` (`labelRole` map L40-53),
provenance bbox → `GeometryAnnotation` (`geometryFromProv` L398+), tables →
Group-of-rows-of-cells with `RoleTableCell`/`RoleTableHeader` (L293-301),
captions → `RoleCaption` (L346). Genuinely **read-only** ("Docling owns the
JSON", package doc L18).

**`core/formats/doclang/`**: DocLang v0.6 XML reader **and** writer with a
faithful structure round-trip (roundtrip_test, conformance_test,
writer_schema_test).

### Which axis reflects "this reader understands tables/headings/reading-order/geometry"? — **NONE.** (hypothesis confirmed, and stronger)

- **Engine** measures *only* serialization round-trip/parity. It is **orthogonal
  to structure**: a reader that recovers perfect structure+geometry but doesn't
  byte-round-trip scores low; a reader that byte-round-trips and extracts zero
  structure scores high. (`gateEngine` :422-440 never touches role/geometry.)
- **Vocabulary** is the **nearest neighbor** but misses it **three independent
  ways**:
  1. *Not gated.* `gateVocab` (:446-454) keys only on `vocabmap`/`vocabtypes`/
     `writecells`/`equivalence`. Block-kinds are **V2 prose** ("block-level
     kinds populated where the format expresses them", rubric L196) with **no
     floor signal, no dimension, no gate term**. Geometry / reading-order /
     plane / relations aren't even prose anywhere in the rubric.
  2. *Wrong field.* The block-kind rows in `core/formats/constructs.yaml`
     (`block.heading` L612, `block.paragraph` L633, …) map to `block:type`
     (the free-form `Block.Type`, "grounded in core/formats/html/reader.go
     blockTypeMap and the markdown reader", L80-82, L608-611) — the **old**
     structure representation. The vision/docling readers populate the **new**
     `StructureAnnotation.Role` stand-off layer via `SetSemanticRole`, which
     `constructs.yaml` does **not** model at all. Two parallel structure
     representations now exist; the axis tracks the one the new formats don't use.
  3. *Inline-only oracle.* The equivalence test
     (`core/formats/vocab_equivalence_test.go`) asserts a **"bold/italic/link/
     image" sentence** yields the same canonical `Type` sequence (rubric L196) —
     purely inline. It never asserts a role, bbox, reading-order, or relation.
- **Editor E1** ("structure-true preview", rubric L229) is the closest *in
  spirit* but measures a *rendering surface* (PreviewBuilder/STRUCTURE_RULES),
  not whether the **reader recovered** structure. None of these formats has one.
- **Knowledge / Corpus / Security**: unrelated to structure extraction.

**Net:** the maturity vector is blind to the single most valuable thing the
vision stack does. The user's named depth ladder — image extracts *(a)* just
metadata, *(b)* OCR plain text, *(c)* OCR + structure (tables/headings/reading
order) + geometry (bounding boxes/layout) — has **no axis to live on**. This is
the gap the sharpening should fill: a structural/geometric-fidelity axis (call
it a sibling of Vocabulary in family "THE MODEL") whose rungs ladder exactly
that: G0 none → G1 plain text only → G2 semantic roles (`SemanticRole`) → G3
roles + reading-order + layer/plane → G4 + geometry (`GeometryAnnotation`
bbox/page) + cross-block relations (`RelationAnnotation`), each rung with a
deterministic floor signal (the package emits `SetSemanticRole`/`SetGeometry`/
`AddRelation`; a structure-equivalence test exists) the same way Vocabulary's
`vocabtypes` grep works.

### Secondary finding: measurement artifacts inflate the badness

Even the **Engine** scores are artifactually depressed:

- **`image` is L0 only because `Config` lives in `reader.go`, not `config.go`.**
  The floor's `config` signal is the presence of a `config.go` *file*
  (audit: `config.go : False`), but `image`'s `Config`+`ApplyMap` are defined in
  `reader.go` (L44-83). `gateEngine` L1 requires `has('config')` (:427), so a
  working reader+writer+`roundtrip_test` is capped at **L0** by a filename
  convention, not a real deficiency.
- **`docling` is L0 because read-only detection is hardcoded to `pdf`.**
  `ftype` (format-triage.js:565) only returns `read-only` for `pdf`; `docling`
  (a genuine read-only "consume Docling" boundary) has no Okapi counterpart so
  it falls to `harvest`, the read-only `na`-patch for `writer` never fires
  (Score phase :788 patches `writer`/`writecells` to `na` only for
  `type==='read-only'`), and `gateEngine` L1's `has('writer')` (:427) caps it at
  **L0**.
- **All three default to `type=harvest`** (no Okapi counterpart) and get pushed
  onto the okapi_skip+invariants+corpus ladder, but they are really a **new
  class** (structure-extraction / read-only ingestion) the `ftype` taxonomy
  doesn't model.

---

## (3) Did adding the new formats break support-gates / the universe? — **No. Green. Only the published dashboard is stale.**

The universe is the **dir-walk over `core/formats/` keeping dirs that ship a
`reader.go`, minus `exec`/`jsx`/`memorytest`** — defined once and mirrored
everywhere:

- `scripts/format-ops/lib.mjs:94-101` `realFormatDirs()` (filters `reader.go`;
  comment L85-93 explicitly notes "pdf … carries only config.go + wasm_bridge.go
  and is correctly excluded here").
- `core/formats/maturity_test.go:49-60` `realFormatDirs` (Go mirror; L60
  `fileExists(.../reader.go)`).
- `audit-format.py --all` applies the same `reader.go` gate.

**Filesystem reality:** 52 real format dirs exist (incl. `image`/`docling`/
`doclang`/`pdf`). `pdf` has **no `reader.go`** (dir =
`config.go`/`grouping.go`/`grouping_test.go`/`wasm_bridge.go`; native PDF is
read out-of-core by the kapi-pdfium plugin, browser by PDFium-wasm — see
`core/formats/register_pdf_js.go` / `register_pdf_other.go`). So the
reader.go-gated universe = **51** (52 − pdf).

**Everything agrees at 51 and is green:**

- `audit-format.py --all --json` → **51** formats; `image`/`docling`/`doclang`
  present, `pdf` absent.
- `core/formats/support.yaml` → **51** entries; `doclang` (L53), `docling`
  (L60), `image` (L67) all added (tier `available`, `grandfathered: true`, no
  `last_certified` yet — awaiting first cert); `pdf` correctly **absent**.
- `node scripts/format-ops/check-support-gates.mjs` → `OK — 51 formats
  (available: 16, maintained: 35)`, **EXIT 0**.
- `go test ./core/formats/ -run 'TestSupportYAML|TestRealFormatDirs|TestMaturity'`
  → **`ok` (0.476s)**.
- All three registered + auto-discovered: `register.go` (`image` L99/L111,
  `doclang` L141/L148, `docling` L159) and `register_test.go` (reader list
  L33-37 incl. `image`; writer list incl. `image` L68; pdf explicitly noted
  absent on native L38-40).

So the dir-walk auto-discovery did its job; the three were also hand-added to
`support.yaml`, and `pdf` losing its `reader.go` cleanly dropped it from the
universe.

**The one stale artifact** is the *published* dashboard dataset and its
committed mirror — regenerated only by a `format-triage` **Publish** run
(`bootstrap-publish.mjs` / the workflow's Publish phase), never auto:

- `web/static/data/format-maturity.json` → **49** formats, still includes
  **`pdf`** (L2 read-only), **missing** `image`/`docling`/`doclang`.
- `docs/internals/format-maturity.md` §5 snapshot table (L591-640) → same **49
  rows**, incl. `pdf`, no new three.

No test or CI gates the dashboard JSON against the dir-walk, so this is **stale,
not red**. The next triage-score run will **add** `image`/`docling`/`doclang`
and **drop** `pdf`. (Note: `pdf` will vanish from the dashboard even though it
remains a usable format — a UX wrinkle worth a sharpening decision: an
out-of-core/plugin-provided format like `pdf` arguably deserves a place in the
vector even without an in-core `reader.go`.)

**Stale hardcoded counts** (harmless prose, but ironically violate the brand
rule against hardcoding code-controlled counts): `lib.mjs:85` "The 49 real
format dirs"; rubric L57 "universe = exactly the 49 real formats", L667 open-Q
"49 real formats"; `support.yaml` header. Actual is now **51**. These should be
delinted to "the real format dirs" rather than a literal.
