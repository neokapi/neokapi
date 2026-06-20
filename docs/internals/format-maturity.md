# Format Maturity — Tiers, Axes & Audit

> New here? Start with the one-page [format-ops-overview.md](./format-ops-overview.md).

This is the bar a neokapi format must clear, and how to measure where a format
sits today. It is the guardrail companion to
[format-engineering.md](./format-engineering.md) (how the engine works) and the
contract that the `format-ops` runbook skill, the `implement-format` and
`refresh-format-maturity` skills (`.skills/`), and the `format-triage` workflow
(`.claude/workflows/format-triage.js`) operate against. The operating process —
rituals, cadences, ledger, prompts — lives in [format-ops.md](./format-ops.md);
the executable spec-case design in
[format-spec-cases.md](./format-spec-cases.md); the research base behind both
in `docs/internals/research/format-ops/`.

Two artifacts are published per format, and they are deliberately different
things:

1. **The promise** — a three-level **support tier** (§1). Tiers are what users
   may rely on. They change only by explicit, human-approved promotion or
   demotion events, and a tier claim must be backed by a CI gate — a tier not
   enforced by CI is marketing. (Rust's target-tier policy is the model.)
2. **The score** — a seven-**axis** maturity vector (§2), recomputed by every
   audit from deterministic file floors plus evidence-cited quality judgments.
   The vector is diagnostic: it ranks work and explains the tier; it does not
   itself promise anything.

The headline tier is derived as the **minimum over the gating axes** — never a
weighted average (a scalar aggregate is where measurement validity collapses).
Two of the seven axes — **Security** (§2.6) and **Structure & Geometry**
(§2.7) — are non-gating display axes: they score and rank work (parser
hardening; structure-recovery depth) but do not enter the tier minimum (for
now).

## 1. Support tiers (the promise)

Declared per format in `core/formats/support.yaml` (committed,
machine-validated). Each entry: `tier`, `tier_since`, `last_certified`,
`gates` (the CI workflows that enforce it), optional `grandfathered: true`
(bootstrap only, see [format-ops.md §9](./format-ops.md)), optional `notes`.

| Tier | Meaning | Entry requirement (gating axes) | CI enforcement |
|---|---|---|---|
| **Supported** | Release-gating. A regression in this format blocks any release. | Engine ≥ L3 **and** Corpus ≥ C2 **and** Knowledge ≥ K2 | Parity suite (or, for harvest formats, invariants + corpus + acceptance) wired into `parity.yml` / `format-acceptance.yml`; corpus manifests verified. |
| **Maintained** | Tested and kept green; regressions fixed on cadence, do not necessarily block release. | Engine ≥ L2 | Package tests in `make test`; malformed suite present. |
| **Available** | Registered and usable; explicitly experimental, no fidelity promise. | Engine ≥ L0 | Registration tests only. |

**Enforcement direction.** CI may enforce *more* than the tier promises, never
less. Today `parity.yml` blocks on every parity-wired format regardless of
tier — that is over-delivery and acceptable; the asymmetric failure (a
Supported format whose named gate does not actually run) is the one the
validators exist to prevent.

**Validation (named mechanisms, not process prose):**

- `scripts/format-ops/check-support-gates.mjs` — schema-validates
  `support.yaml` (universe = exactly the real format dirs, valid tier enums and
  dates), asserts every Supported format's `gates` name workflows that exist
  and exercise that format (parity wiring or acceptance/invariants presence),
  and maps the latest parity/acceptance results to tiers so a Supported-format
  failure is distinguishable from a Maintained one. Wired into
  `reference-data-drift.yml` (whose path filters include `support.yaml` and
  `scripts/format-ops/**`).
- `TestSupportYAML` in `core/formats/maturity_test.go` — one entry per real
  format dir and no extras; tier enums/dates parse; each `gates:` entry
  matches a file under `../../.github/workflows/` (skip-if-absent for partial
  checkouts).

Rules:

- **Promotion/demotion are explicit events**, proposed into the ops ledger's
  `pending[]` queue by the `tier-review` ritual and approved by the
  maintainer. The audit vector *suggests* tier changes; it never applies them.
- **Writers are partitioned.** `tier-review` (human-approved) writes
  `tier`/`tier_since`/`gates`/`notes`; `triage-score` may mechanically refresh
  **only** `last_certified` on a passing run. No other writer exists.
- **Certification decays.** When `last_certified` is older than 45 days the
  dashboard flags the tier *stale* (a long absence produces stale badges, not
  breakage — one triage-score run clears them); older than 120 days, the
  dashboard displays the decayed tier (one level down) alongside the declared
  one and `tier-review` becomes due. A dead process cannot preserve a stale
  promise. The dataset carries per-row
  `tier: {declared, since, last_certified, gates[]}` (additive fields read
  from `support.yaml` at publish time); the page computes staleness
  client-side from `generated_at − last_certified` using these thresholds.
- **Demotion is normal and is never deletion.** Announce before the release,
  record the reason in `docs/internals/format-demotions.md`, drop at most one
  tier at a time (Supported → Maintained → Available → plugin/retired).
  Retirement (out of the default binary, into a plugin) is a product decision,
  not an audit outcome.
- The vector may exceed the tier; it must never durably *under*-run it
  (suspended per-format while `grandfathered: true` during bootstrap).

## 2. The seven axes (the score)

Each axis has its own ladder, its own deterministic file floor, and (where
defined) quality dimensions. A format sits at exactly one level per axis: the
highest level whose criteria are fully met (a missing lower-tier requirement
caps the level). Axes evolve independently — a format can be E3 (embedded in
its native editor) at V1, or L3 at K1. Five axes (Engine, Vocabulary, Editor,
Knowledge, Corpus) feed the tier; two — Security (§2.6) and Structure &
Geometry (§2.7) — are display-only.

| Axis | Ladder | Measures | Primary artifacts |
|---|---|---|---|
| **Engine** | L0–L4 | Parse/round-trip/parity fidelity and robustness | `reader.go`, `writer.go`, `config.go`, `spec.yaml`, parity wiring, `malformed_test.go` |
| **Vocabulary** | V0–V3 | How richly format semantics map into the canonical content-model vocabulary (and back) | `vocabulary.yaml`, canonical run types (`core/model/vocabulary.go`), equivalence tests |
| **Editor** | E0–E4 | How close kapi gets to the format's native editing surface | `integrations.yaml`, `format.PreviewBuilder`, connectors/add-ins |
| **Knowledge** | K0–K3 | The spec/learning assets that let a person or model work on the format | `dossier.yaml`, the spec knowledge base (`specs/`), `spec.yaml` refs, divergence attribution |
| **Corpus** | C0–C3 | Reference files that validate support, with provenance | `corpus.yaml` manifests, `testdata/`, fetched corpus tiers, acceptance validators |
| **Security** | S0–S4 | Resource-boundedness, fuzzing, and hostile-corpus hardening of the parser (non-gating display axis) | `core/safeio` imports, `Fuzz*` targets + `testdata/fuzz/` seeds, corpus-sweep ledger records |
| **Structure & Geometry** | G0–G4 | How much of the document's logical and spatial structure the reader recovers — roles, reading order, tables, relations, geometry (non-gating display axis) | `core/model/structure.go` standoff payloads (`SetSemanticRole`/`SetStructure`/`SetGeometry`/`SetLayoutLayer`/`AddRelation`), optional `structure.yaml` |

### Axis families (a reading aid)

The seven axes group into three families, named by the question each answers —
"how deeply we read it / how we prove it / how we work with it":

| Family | Mental model (one line) | Axes |
|---|---|---|
| **Comprehension** | *How deeply we read it* — fidelity at three resolutions: bytes, inline, structure. | Engine, Vocabulary, Structure & Geometry |
| **Assurance** | *How we prove it* — does support hold over real and hostile files? | Corpus, Security |
| **Enablement** | *How we work with it* — can a person, model, or native editor act on it? | Knowledge, Editor |

The three **Comprehension** axes are the same fidelity question at increasing
resolution: Engine = byte/round-trip/parity fidelity of the serialization;
Vocabulary = inline / run-level meaning (`fmt:*`/`link:*`/`media:*`/`code:*`
*within* a block); Structure & Geometry = block-level, cross-block, and spatial
structure.

Families are a **reading aid** for the dashboard and this rubric, not a gating
unit. The headline tier is still the `min` over the gating axes (Engine ∧
Corpus ∧ Knowledge), which now straddle all three families
(Engine→Comprehension, Corpus→Assurance, Knowledge→Enablement). The
`min`-over-a-named-set rule operates on the axis set, never on a family, so the
dashboard keeps marking the three gating axes individually.

Shared signals are deliberate: the `corpus` quality dimension feeds both the
Engine gate (as in scorer v2) and the Corpus gate, so one cited judgment moves
both axes coherently rather than publishing an inconsistent state.

**Applicability.** Dimensions that cannot apply score `na` and are excluded
from the gate (writer dimensions for the read-only `pdf`; parity for harvest
formats; vocabulary write cells for read-only formats). `na` on any
**tier-gating** criterion is a *countersigned* state: the claim lives in the
relevant artifact with `reviewed_by` + date, applied through the `tier-review`
ritual — never a bare self-declaration.

### 2.1 Engine (L0–L4) — gates unchanged from scorer v2

The Engine ladder is the original maturity ladder. Its gate function and all
nine dimensions (reader, writer, config, spec, parity, malformed, corpus,
detection, docs) are intentionally **identical to scorer v2** so published
history and sticky anchors stay numerically comparable. Consequences, stated
explicitly:

- The floor's `docs` and `detection` cells remain constants
  (`'complete'`), as in v2. The *real* documentation signals (nativedocs
  sidecar, refs census) are measured on the **Knowledge axis**; the Engine L3
  prose criterion "reference docs wired" is therefore *measured* at K1–K2 and
  *enforced* for users via the Supported tier (Engine ≥ L3 ∧ Knowledge ≥ K2).
  `detection` keeps its constant pending a real floor signal (presence in
  `register_test.go`'s detection lists) — tracked as a scorer issue, not
  silently changed.
- The `config` dimension stays on Engine (the L0/L1 gates depend on it); the
  Vocabulary axis introduces only new dimensions and borrows nothing from
  Engine's nine.

| Level | Name | Entry criteria |
|---|---|---|
| **L0** | Experimental | Reader compiles and emits `LayerStart → Block → LayerEnd` for the happy path; registered in `register.go` + `ids.go` + the `register_test` lists so `make test` passes. May lack a writer or config validation. No round-trip guarantee, no `spec.yaml`, no parity. Use only behind an explicit experimental label. |
| **L1** | Readable + writable | L0 **plus**: writer applies target-else-source via `RenderRunsWithData`; one declared round-trip strategy; a `reader_test` **and** a `roundtrip_test` (or `skeleton_test`) prove read→write fidelity for core cases; `Config` rejects unknown keys; inline codes preserved as runs. |
| **L2** | Specified | L1 **plus**: `spec.yaml` + `spec_test.go` driving `spec.NativeRunner` green (keys 1:1 with `ApplyMap`, `okapi_param` recorded, `spec_refs`) **OR**, for a harvest format with no Okapi counterpart, `okapi_skip_test.go` + `invariants_test` + `corpus_test` in its place; `schema.go` present; a `malformed_test` asserts clean errors without panic. |
| **L3** | Parity-verified | L2 **plus** (Okapi counterpart): `cli/parity/formats/<id>_spec_test.go` passes head-to-head (`bridge_config` where param shapes differ); every divergence is an `expected_fail` with an explicit non-`native-bug` `divergence_kind` grounded in spec + Okapi citations (zero pure `default-diff` xfails); a corpus/upstream test exercises real files; reference docs + `nativedocs` sidecar + `metadata.json` wired with no drift (measured on the Knowledge axis — see above). |
| **L4** | Rock-solid | L3 **plus**: byte-faithful round-trip proven across an edge-case matrix (encodings, line endings, unicode, malformed-but-recoverable) **and** over a real-world corpus; `schema_test` asserts schema == config struct; `transform_test` where a transform exists; bench/perf for perf-sensitive formats; zero `native-bug` xfails; any remaining divergence is a tracked, attributed, spec-justified faithful-class item; parity / contract-audit dashboards green and freshly regenerated. |

> **Harvest formats** (no Okapi counterpart: androidxml, applestrings, arb,
> designtokens, i18next, mdx, resx, xcstrings) cannot use the parity bridge;
> their L2/L3 path is the self-contained ladder (okapi_skip + invariants +
> corpus + acceptance) and the parity dimension is `na`. Their Engine ceiling
> is not artificially capped — L4's edge-case and corpus bars apply unchanged.

**Robustness beyond malformed.** Go fuzz targets (`func Fuzz*` with seeds in
`testdata/fuzz/`), `core/safeio` budgets, and hostile-corpus sweeps are now
scored on their own **Security axis** (S0–S4: bounded → fuzzed →
hostile-hardened → continuously assured — §2.6), not on Engine. The Engine
ladder keeps its `malformed_test` robustness floor; deeper hardening reads on
Security, which is a non-gating display axis.

### 2.2 Vocabulary (V0–V3) — representation fidelity

Measures whether the format's *meaning* (inline styling, links, media,
placeholders, block roles) survives into the canonical vocabulary
(`core/model/vocabulary.go` types: `fmt:*`, `link:*`, `media:*`, `code:*`) and
back out — versus surviving only as opaque bytes in `Run.Data`.

Two artifacts define the axis:

- **`core/formats/constructs.yaml`** (repo-level registry, versioned, stable
  IDs) — the construct row space, seeded from ITS 2.0 data categories, the
  XLIFF 2.x module set, the canonical run-type packs, and block-level kinds
  (heading/paragraph/list/cell/quote). Each construct carries a **default
  expressibility class** per format family, so a format's `expressible: false`
  is a reviewed diff against a baseline, not a free self-declaration.
- **`core/formats/<id>/vocabulary.yaml`** — the format's matrix. Per
  construct: `expressible: true|false` (overrides of the baseline need
  `reviewed_by` + date when they affect a gate); separate **`read`** and
  **`write`** cells (they rot independently), each
  `lossless|lossy|dropped|rejected|unknown`; `policy` for any non-lossless
  cell (`preserve-as-skeleton|preserve-as-overlay|drop|error`); mandatory
  `notes` whenever not lossless; `evidence:` binding every non-`unknown`
  claim to a test the audit can resolve.

**Evidence resolves or it does not count.** The deterministic floor resolves
every `evidence` entry: `pkg.TestFunc` refs must grep to a `func TestFunc` in
that package; spec-case IDs must exist in the format's `spec.yaml`. An
unresolvable citation makes the cell count as `unknown` in the census — an
agent cannot promote the floor by stamping evidence strings. An
`expressible: false` override is rejected by the validator when the package
demonstrably emits the corresponding canonical type (the grep below).

| Level | Name | Entry criteria |
|---|---|---|
| **V0** | Opaque | Inline content survives round-trip as raw runs, but run types are absent, format-local (`x-*`), or generic (`"code"`). No `vocabulary.yaml`, or one that is all-`unknown`. |
| **V1** | Typed reading | The reader emits canonical types for the constructs the format can express (placeholders typed `code:*`; styling `fmt:*`; links/media `link:*`/`media:*`); `vocabulary.yaml` exists with `read` cells claimed and **resolved-evidence-bound** for every expressible construct. |
| **V2** | Bidirectional | V1 **plus**: the writer consumes canonical types — a target authored in canonical vocabulary serializes to correct native markup (`write` cells claimed and resolved); the cross-format equivalence test passes (`core/formats/vocab_equivalence_test.go`: a shared fixture sentence — bold/italic/link/image — yields the same canonical `Type` sequence as the reference formats); block-level kinds populated where the format expresses them. |
| **V3** | Fidelity-proven | V2 **plus**: zero `unknown` cells for expressible constructs; every non-lossless cell carries `policy` + `notes` + evidence; the directional loss table is published in the reference docs; preview/editor surfaces render this format's canonical vocabulary (it *looks* bold, not like a chip). |

Floor signals (dimension ids in parentheses):

- (`vocabmap`) `vocabulary.yaml` presence + resolved-cell census
  ({expressible, read/write claimed, unknown, evidence-resolved} counts).
- (`vocabtypes`) a **package-wide** grep over non-test `.go` files in
  `core/formats/<id>/` for canonical-type string literals
  (`"(fmt|link|media|code):`) or imports of the `core/model` vocabulary
  constants. (Both real emitter styles exist: openxml's constants live in
  `vocabulary.go`, not `reader.go`; html emits raw literals.) This grep is a
  *necessary* condition only — the binding signal is the resolved cell census.
- (`writecells`) the write-cell census — the V2/V3 seed, and the axis's one
  **quality dimension** (demote-only, citation required): do the cited tests
  actually author-from-canonical, or merely echo what was read?
- (`equivalence`) the per-format case in `vocab_equivalence_test.go`.

Formats with no expressible inline or block constructs beyond plain text cap
at V1 (typed placeholders); their `expressible: false` rows are baseline
classes in `constructs.yaml`, not per-format claims.

### 2.3 Editor (E0–E4) — native-surface embedding

Measures how close kapi gets to the editor that owns the format. Grounded in
what each ecosystem's API actually permits — record the gate evidence, do not
promise past it (Word permits full-file extraction + tracked-changes
write-back; Google's modern add-on track and Canva structurally cap below E3;
PowerPoint's text APIs fragment across license channels).

| Level | Name | Entry criteria |
|---|---|---|
| **E0** | None | Files in, files out. |
| **E1** | Faithful preview | A styled, structure-true preview rendered from the canonical model: `format.PreviewBuilder` implemented (or the format id present in the exported `STRUCTURE_RULES` index), with a preview test or story proving structure. |
| **E2** | Round-trip workflow | Push/pull into the native tool with **stable identity binding**: the canonical `editor-anchor` overlay (one overlay kind, per-ecosystem payload — Word content-control tag / Figma node-id+range / Docs named-range / CMS entry+field path) survives an edit cycle in the native editor, proven by the integration's round-trip test (an `in-out` anchor-survivability case per [format-spec-cases.md](./format-spec-cases.md)) — not a demo. |
| **E3** | Embedded | A live add-in/plugin panel inside the native editor: reads the open document, writes targets in the editor's native idiom (e.g. tracked changes in Word), distributed through that ecosystem's channel, present on HEAD. |
| **E4** | Continuous | Editor events drive the pipeline bidirectionally (webhooks/app events); edits flow without manual export/import. Structurally available only where the ecosystem has a change feed (CMS app frameworks, WordPress hooks). |

**Floor = min(declared, probed).** Depth is *declared* in the committed
integrations index `core/formats/integrations.yaml` (per format: surface →
depth → gate evidence → key files), and every claim is *corroborated* by an
independent deterministic probe in the audit; the floor caps at the highest
probed level regardless of declaration:

- E1 probe: a `format.PreviewBuilder` implementation in the package, or the
  format id in the exported STRUCTURE_RULES index.
- E2 probe: the entry's `evidence` is a `path/file_test.go:TestName` that
  resolves on HEAD (file exists, `func TestName` greps).
- E3 probe: a committed add-in/connector manifest path exists on HEAD
  (integrations on unmerged branches do not count).
- E4 probe: a registered webhook/event-handler symbol exists on HEAD.

The Editor axis has **no quality dimensions** — but its determinism comes from
the probes, not from trusting the YAML: `integrations.yaml` is on the
change-control list (§3), so a score-improving change may not edit it.

### 2.4 Knowledge (K0–K3) — spec & learning assets

Measures whether a person — or a model — can pick this format up and work on
it correctly from in-repo assets alone.

Three artifacts define the axis:

- **`core/formats/<id>/dossier.yaml`** — the per-format backbone:
  authoritative spec sources (`{id, version, url, watch}`), other
  implementations (Okapi filter id + the obvious others —
  Pandoc/Tika/LibreOffice/translate-toolkit — each with repo pointer, optional
  `watch` feed, and a license note: GPL sources are *read-about*, never
  harvested), learning-material pointers, and divergence-ledger pointers.
- **The spec knowledge base** (`specs/` at the repo root): `catalog.yaml`
  ({id, version, url, rights class}); `snapshots/<spec>/<version>/`
  (vendor-class committed; cache-class gitignored; **ISO text never** — use
  the free Ecma-376/OASIS twins; Apple link-only);
  `sections/<spec>/<version>/<anchor>.md` per-clause retrieval units. Pin or
  don't cite. The section files are also the retrieval substrate for
  AI test generation ([format-spec-cases.md](./format-spec-cases.md)) and the
  context pack.
- **Citations** (in `spec.yaml`/`dossier.yaml`):
  `{spec, version, url#fragment, clause, heading, quote ≤1 sentence,
  quote_sha256}` — resolved against the pinned snapshot by
  `scripts/format-ops/check-citations.mjs` (HTTP/anchor checks only where the
  target is anchor-addressable; otherwise the recorded resolution mode is
  `pinned-version-only`). PDF-only specs resolve by quote-hash against the
  snapshot, never the live network.

| Level | Name | Entry criteria |
|---|---|---|
| **K0** | Undocumented | Nothing beyond the code. |
| **K1** | Grounded | `dossier.yaml` present with ≥1 versioned spec source (catalog-registered) and the implementations table; reference docs + `nativedocs` sidecar wired and current. |
| **K2** | Executable | K1 **plus**: `spec.yaml` (or the harvest ladder) green with `spec_refs`/`okapi_refs`/`native_refs` populated; **every** `expected_fail` carries an explicit `divergence_kind`; `schema.go` present and reference docs regenerate without drift. |
| **K3** | Living | K2 **plus**: `check-citations.mjs` output green for this format within the upstream-watch cadence; zero stale xfails per the last hygiene run's recorded output; a context pack generates cleanly (`scripts/format-ops/context-pack.mjs <id>` joins dossier + spec.yaml + vocabulary.yaml + corpus.yaml + relevant section files into one schema-checked artifact — the standard input to `implement-format` and `case-gen`). |

K3's checks gate on **recorded script output** (exit status + output hash in
the ledger's `runs[].evidence`), not on bare watermarks — a watermark updated
without its check is detectable, and the calibration phase of `process-health`
spot-replays one recorded check per cycle.

Floor signals (dimension ids): (`dossier`) presence + field census;
(`sidecar`) nativedocs sidecar present and named exactly the id; (`refs`)
refs census over `spec.yaml` + `divergence_kind` coverage ratio — the axis's
**quality dimension** (demote-only, citation required): do the refs actually
point at the clause that justifies the behavior?; (`citations`) latest
check-citations result for this format; (`contextpack`) context-pack
generation result.

### 2.5 Corpus (C0–C3) — reference files with provenance

Three corpus tiers, composed: **Tier A exemplars** (committed `testdata/`,
small, license-clean — CC0/own-created/US-gov only), **Tier B harvested**
(fetch-on-demand, sha256-cached, never vendored when the license is unclear),
**Tier C synthetic** (deterministic seeded generators + fuzz seeds — the
generator is committed and reviewed, not the files).

**`core/formats/<id>/corpus.yaml` is the canonical manifest for all tiers.**
Per entry: `path` (root-relative: committed `core/formats/<id>/testdata/…` or
fetched `corpus/<version>/<id>/…`), `tier: A|B|C`, `sha256`, `size`,
`origin: vendored|url|archive-member|bug|generated`, `source_url`, `license`
(SPDX; `LicenseRef-Unverified` only for Tier B), `redistributable`,
`creator_tool`, `harvest_date`, `notes`. The legacy
`testdata/corpus/SOURCES.md` files migrate into `corpus.yaml`
(`origin: vendored` + their pinned commits/licenses) and are thereafter
**generated from it** as the human-readable view; the corpus-census ritual
fails when a `SOURCES.md` exists without a covering manifest. Every bug fix
lands a minimized failing file with `origin: bug` plus a case named after the
issue — the bug→corpus flywheel is mandatory, not aspirational.

**Tier B mechanics** (so C2 is implementable, not aspirational):

- Input scheme: `corpus:<relpath>` in `spec.yaml`/tests, resolved by
  `FindCorpusRoot()` in `core/format/spec/helpers.go` mirroring
  `FindOkapiTestdataRoot` (walk up to `go.work`, pick the lexically-latest
  version dir under `corpus/`); absent corpus ⇒ **skip with the fetch
  command in the skip message**, never fail.
- Storage: a single release tag `format-corpus-vN` on neokapi/neokapi with
  **per-format assets** `corpus-<id>.tar.gz` (a one-format respin does not
  re-ship every binary), published merge-never-drop by
  `scripts/publish-corpus.sh` (download → overlay → repack → `--clobber`, with a
  first-publish auto-create guard); fetched by `scripts/fetch-corpus.sh`
  (`make fetch-corpus [FORMAT=<id>]`) into `corpus/<version>/<id>/`
  (gitignored, next to the `okapi-testdata/` entry).

**Provenance is verified, not trusted.** sha256 proves integrity, not origin:
for `origin: url|archive-member` entries the corpus-census ritual re-fetches
`source_url` and verifies `sha256(fetched) == manifest.sha256`, recording the
per-file result in the ledger. Only **externally-verified** entries count
toward the C3 wild-files floor; `origin: vendored|generated` never do. The
acceptance-validator signal is the latest `format-acceptance.yml` CI
conclusion for the format (recorded as a ledger watermark) — test *presence*
is not "green" (acceptance suites skip when the validator binary is absent).

| Level | Name | Entry criteria |
|---|---|---|
| **C0** | Unprovenanced | No testdata, or files with no manifest entry. |
| **C1** | Exemplars | Committed Tier A files covered 100% by `corpus.yaml`; fidelity tests consume them (`corpus_test`/`upstream_test`/round-trip over testdata). |
| **C2** | Manifested + fetched | C1 **plus**: Tier B wired (scheme + fetch + skip-not-fail) **or** a countersigned `na` (no harvestable wild corpus exists — `reviewed_by` via tier-review); manifests sha256-verified by the latest census. |
| **C3** | Broad | C2 **plus**: externally-verified wild files beyond Okapi's fixtures; the edge-case matrix (encodings, BOM, CRLF, all-unicode, malformed-but-recoverable) present as fixtures; a generator or fuzz seed set where applicable; latest acceptance CI conclusion green; **a green corpus-sweep record over the wild set within the sweep cadence** (read→write→read, one file one subprocess, `OK/OK_ROUNDTRIP/EXPECTED_REJECT/CRASH/HANG/OOM/ROUNDTRIP_DRIFT` taxonomy — wild files that are never executed validate nothing); flywheel evidence (≥1 issue-named fixture or case). |

Floor signals (dimension ids): (`corpusmanifest`) `corpus.yaml` presence +
per-tier census + last census verification result; (`corpus`) the
real-vs-synthetic census — **the shared quality dimension**: seeded from the
manifest origin census, demote-only with citation, and consumed by *both* the
Engine gate (its v2 corpus slot, unchanged) and the Corpus gate;
(`fetchwiring`) scheme + fetch-script probes; (`acceptance`) latest
acceptance CI conclusion; (`sweep`) latest corpus-sweep record.

### 2.6 Security (S0–S4) — resource-boundedness & hostile-input hardening

Measures how well a format's parser resists malicious or pathological input —
the structural advantage of a memory-safe Go engine made measurable. The
residual risk classes for the pure-Go readers are resource DoS
(CPU/memory/stack, decompression bombs), path traversal on extraction, and
round-trip semantic corruption; cgo/bridge formats re-introduce memory-unsafety
and need subprocess isolation. (Research base:
[`research/format-ops/followup-format-parser-security-ops.md`](./research/format-ops/followup-format-parser-security-ops.md)
and SYNTHESIS D7.)

This is a **pure floor ladder with no quality dimensions** — every rung is a
deterministic file or ledger signal, so the published level is fully pinned
(`repro-check.mjs` asserts spread 0, exactly as for Editor).

| Level | Name | Entry criteria |
|---|---|---|
| **S0** | Unbounded | No `core/safeio` budgets wired and no fuzz target — the default for an un-hardened reader. |
| **S1** | Bounded | The format's package imports `core/safeio` (byte budget, depth guard, zip ratio/entry caps, `SafeJoin`) — boundedness is structurally present and applied identically across CLI/server/WASM. |
| **S2** | Fuzzed | S1 **plus** a Go native fuzz target (`FuzzRead*`/`FuzzRoundTrip*`) with ≥1 committed `testdata/fuzz/` seed for the format (OpenSSF Scorecard `fuzzedWithGoNative` would pass). |
| **S3** | Hostile-hardened | S2 **plus** a clean corpus-sweep record in the ops ledger — 0 `CRASH`/`HANG`/`OOM` **and** 0 `ROUNDTRIP_DRIFT` over the wild/hostile set. (`govulncheck`-clean is module-wide — a noted co-signal, not a per-format gate.) Absent a ledger sweep record, the ceiling stays S2. |
| **S4** | Continuously-assured | S3 **plus** a sustained ledger signal (batch fuzzing / repeated green sweeps over the cadence). A ledger-only ceiling rung, reachable later. |

Floor signals (dimension ids): (`safeio`) a package-wide grep of non-test `.go`
files for a `core/safeio` import; (`fuzz`) a `Fuzz*` target plus a
`testdata/fuzz/` seed; (`sweepclean`) the latest corpus-sweep record for the
format (clean = 0 CRASH/HANG/OOM/ROUNDTRIP_DRIFT); (`sustained`) a
ledger-recorded sustained-green signal. `base` is the structural file floor
(S0–S2); the ledger rungs raise only the `ceiling` (S3/S4), exactly like
Knowledge and Corpus. The format-triage gate then computes the published level
from the cells and caps it at that ceiling.

**Non-gating, for now.** Security is a **display axis**: it does *not* enter the
headline-tier minimum (§1) and does not cap a format's support tier. It informs
and ranks hardening work without blocking releases. Promoting it to a gating
axis (e.g. "Supported requires S2") is a future **tier-policy** decision made
through the `tier-review` ritual, recorded here when taken — not an audit
outcome.

### 2.7 Structure & Geometry (G0–G4) — logical & spatial structure recovery

Measures how much of the document's logical and spatial structure the reader
recovers and the model represents — block roles, reading order, table grids,
cross-block relations, and page geometry. It is the comprehension-depth ladder
the vision / OCR / structure stack populates, riding the standoff payloads in
`core/model/structure.go` (additive — no proto/KLF schema change). It is the
third **Comprehension** axis, at coarser resolution than Engine (serialization
round-trip) and Vocabulary (inline run-level meaning): block-level, cross-block,
and spatial structure.

This is a **cumulative floor ladder with no quality dimensions** — every rung is
a deterministic file grep, so the published level is fully pinned
(`repro-check.mjs` asserts spread 0, exactly as for Editor and Security). Each
rung adds one richer standoff payload, in value order; **geometry is the top
rung** — the hardest to recover faithfully and, for localization, the least
directly useful (it is read-only reconstruction metadata that native writers
ignore, `structure.go:124-126`), while logical structure is what a translation
flow acts on.

| Level | Name | Entry criteria |
|---|---|---|
| **G0** | Opaque / flat | A `model.Media` part, or undifferentiated text blocks: no normalized `Role`, no Group nesting beyond the document Layer, no geometry — the destructive flatten. The default for key-value catalog formats and the OCR-off / fast-path binary case. |
| **G1** | Metadata | Text *about* the asset or document classified onto the metadata plane (`SetLayoutLayer(LayerMetadata)`, `core/docmeta`) and/or alt-text / caption blocks (`RoleCaption` + `AddRelation(RelCaptionOf, …)`). No in-content body structure yet. |
| **G2** | Linear body text | G1 **plus**: the body content recovered as text in correct reading order (stream order), optionally grouped by the `Layer`/`Group` tree — but roles still generic (`Block.Type` only), no normalized `StructureAnnotation.Role`. |
| **G3** | Logical structure | G2 **plus**: every block carries a normalized `StructureAnnotation.Role` (heading + `Level`, paragraph, caption, footnote, list-item, code, formula) and `LayoutLayer`; reconstructed table grids (`Group{Type:"table"/"table-row"}` + `RoleTableCell`/`RoleTableHeader`); typed `RelationAnnotation` edges; explicit reading order. |
| **G4** | Spatial geometry | G3 **plus**: a `GeometryAnnotation` (page + bbox + origin + resolution) on blocks, `Z` for overlay planes, and per-glyph `Glyphs` at the top sub-rung — enough to reconstruct the page or round-trip to DocLang `<location>`. |

Floor signals (dimension ids in parentheses), computed by the audit over
non-test `.go` in `core/formats/<id>/` exactly as Vocabulary's `vocabtypes` is
grepped, then **down-filled** so a deeper payload implies the shallower rungs
(`roles` ⇒ metaplane + readingorder; `geometry` ⇒ metaplane + readingorder but
**not** roles):

- (`metaplane`) a `SetLayoutLayer(…LayerMetadata)` call, an import of
  `core/docmeta`, or `AddRelation(RelCaptionOf, …)`.
- (`readingorder`) the package emits `model.PartGroupStart` and/or sets
  `StructureAnnotation.ReadingOrder` (`SetStructure`).
- (`roles`) `SetSemanticRole`/`SetStructure` **and** table Groups
  (`Type:"table"/"table-row"`) and/or `AddRelation`, **plus** a roles/structure
  test.
- (`geometry`) `SetGeometry` **plus** a geometry test (the top sub-rung also
  emits `GeometryAnnotation.Glyphs`).

These are the same kind of deterministic file facts the audit already computes
for Security (`core/safeio` import / `Fuzz*` target), so the published level is
fully pinned and reproducible.

**Orthogonal to Vocabulary and Engine.** Engine (L) measures byte/round-trip/
parity fidelity of the *serialization* — a reader can be L4 byte-faithful while
flattening to one block per page (G0). Vocabulary (V) measures *inline /
run-level* meaning (`fmt:*`/`link:*`/`media:*`/`code:*` within a block) plus
block-kind via `constructs.yaml` → the free-form `Block.Type` field; G measures
*block-level + cross-block + spatial* structure via the **new**
`StructureAnnotation.Role` standoff layer — a different field the new readers
populate (`SetSemanticRole`) that `constructs.yaml` does not model. Proof of
independence: `docling` is V0 (plain runs) but high-G (roles / tables / geometry
/ order); `html` is V1 (typed inline) but low-G (no geometry). G is not an
extension of V.

**`na` is a ceiling cap, not a gate-pass.** Pure key-value catalogs (json,
properties, resx, yaml, po, the harvest set) have no intrinsic geometry
(`structure.go:124-126`); their `geometry` cell is `na` as a **ceiling cap** —
the audit sets `structure.ceiling = G3` (or G2), so they cannot reach G4. Unlike
every other axis, `na` on this *cumulative depth* ladder must mean "ceiling
capped below this rung," **not** "this rung passes" — otherwise a roles-only
catalog would falsely promote to G4. It is recorded in the audit's per-format
`ceiling`, never as the gate's `full('na') == pass` shortcut.

**Geometry without roles caps at G2.** Because the ladder is cumulative (G4
requires G3), formats that emit `SetGeometry` but no `SetSemanticRole` — `odf`
and `idml` — cap at **G2** despite carrying geometry: positioned text whose
structure we do not understand is not comprehension. They climb when they emit
roles.

**What a grade does and does not certify** (calibrated against DocLang, the
richest structural format — see `docs/internals/research/format-ops/sharpen/`):

- **G3 certifies table *topology*, not span fidelity.** The `roles` signal
  proves a reader recovers table grids (rows/cells/headers); it does **not**
  prove merged cells (`colspan`/`rowspan`) survive. Span extents have a typed
  home — `StructureAnnotation.ColSpan`/`RowSpan` (DocLang `lcel`/`ucel`/`xcel`) —
  and a reader that populates them + proves it with a span test reads as the
  display-only **`span_fidelity`** sub-signal (a per-reader quality detail
  surfaced beside the rung, still not part of the G3 gate).
- **G3 relation recovery is OR-lenient.** The signal accepts table Groups *or*
  `AddRelation`, so a roles+tables reader reaches G3 with no cross-block relation
  graph. Cross-block continuation/threading (DocLang `<thread>`, the
  `RelContinues` relation) is therefore a noted sub-capability, not a G3
  guarantee. Recovery of the canonical forms cluster (key/value/hint/checkbox
  roles + fillable/checked state) likewise reads as the display-only **`forms`**
  sub-signal, not a rung.
- **G2 reading-order is grep-lenient.** The `readingorder` signal accepts
  emitting `PartGroupStart`, which most grouped formats do trivially — it proves
  the reader *can* express order, not that the order is *correct*. True
  reading-order *correctness* is a corpus-level oracle: a fixture whose logical
  order is known by construction, asserting the recovered block sequence matches
  exactly (`doclang.TestCorpus_ReadingOrder`), independent of the G2 grep.

**Authority is an orthogonal qualifier, not a rung.** The AD-028 provenance
tiers (1 tagged tree / 2 geometric inference / 3 ML layout) are
confidence/provenance, not richness: the same G3/G4 payloads can arrive via
authoritative tags or an ML guess — same depth, different trust
(`GeometryAnnotation.SourceRef` already records provenance). Carry the authority
as a per-format qualifier in the new `core/formats/<id>/structure.yaml` artifact
(which also countersigns the `na` geometry cell) and render its badge later;
never collapse richness (the rung) and authority (the tier) into one number.

**Plugin-provided G is certified out-of-core, like Corpus acceptance.** The two
spatial flagships whose real depth lives in a plugin — `pdf` (kapi-pdfium) and
`image` (kapi-vision) — have a thin in-core grep floor (pdf G0, image G1) but a
high declared ceiling (G4) recorded in their `structure.yaml` `plugin:` block.
The published level stays at the conservative in-core floor **until a nightly
plugin job certifies the out-of-core path**: a green `<fmt>`-`structure`
evidence entry in the ops ledger (the kapi-pdfium / vision-onnx nightly), checked
exactly like the Corpus `acceptance` signal. Without the signal the declared
ceiling is only the promotable ceiling the dashboard shows (floor → ceiling);
with it, the audit fills the published cells up to the declared plugin ceiling so
the gate publishes it. This certifies the plugin's tagged-tree G3 / geometry G4
from the plugin path + CI, never by the in-core grep (which cannot see the
plugin). The mechanism is deterministic (the ledger is a committed file), so the
floor-ladder's spread-0 reproducibility holds.

**Non-gating, for now.** Like Security, Structure & Geometry is a **display
axis**: it does *not* enter the headline-tier minimum (§1) and does not cap a
format's support tier. Most formats are G0/G1, so gating it immediately would
mass-demote the fleet. Promoting it to a gating axis (candidate rule: "Supported
requires G2 where structure is applicable, `na`-exempt for catalogs") is a
future **tier-policy** decision made through the `tier-review` ritual, recorded
here when taken — not an audit outcome.

## 3. How scores are computed (scorer v4 — reproducible by design)

The triage workflow does **not** let the model pick levels; each axis level is
*computed* from two inputs so re-runs are reproducible:

1. **A deterministic file floor per axis** — `audit-format.py --json` emits an
   additive `axes:{engine|vocabulary|editor|knowledge|corpus|security|structure:
   {base, ceiling, signals}}` block alongside the legacy top-level fields (the
   stdin contract of `repro-check.mjs` is preserved). The audit **parses the
   axis artifacts**
   (`vocabulary.yaml` cell census with evidence resolution, `dossier.yaml`
   field census, `corpus.yaml` tier census, `integrations.yaml` + probes) —
   floors are computed from the artifacts the rubric names, not proxies. The
   model cannot push any axis above what the files support, nor re-decide a
   file fact.
2. **Per-axis quality dimensions** — exact dimension ids, each with a named
   floor seed, demote-only, each demotion requiring a `file:line`/`TestName`
   citation or it is dropped:

   | Axis | Quality dims | Floor seed |
   |---|---|---|
   | Engine | `writer`, `parity`, `corpus` | unchanged from v2 |
   | Vocabulary | `writecells` | write-cell census |
   | Knowledge | `refs` | refs + divergence_kind census |
   | Corpus | `corpus` | manifest origin census (shared with Engine) |
   | Editor | — | floor-only (probes) |
   | Security | — | floor-only (safeio import / fuzz target+seed / ledger sweep; spread 0) |
   | Structure & Geometry | — | floor-only (metaplane / readingorder / roles / geometry greps, down-filled; spread 0) |

   `normDim` matches dimension ids **exactly** (no substring matching); the
   SCORE schema enum, the per-axis QUALITY sets, and `repro-check.mjs`'s
   spread cube update against these exact ids in the same commit as any gate
   change.

Published levels are **sticky-anchored per axis** with one asymmetry:

- A *move* on an axis publishes only with a cited `delta_justification` for
  that axis; otherwise the prior stands.
- **Sticky may never preserve a prior above the floor ceiling.** If the prior
  outranks the axis's floor ceiling (an artifact was deleted, a test removed),
  the derived level publishes regardless of citation, recorded as
  `delta.why: "FLOOR-FORCED demotion"`. Floor-forced demotions are
  citation-free; only promotions and within-band moves need justification.
- On the first publish of a new axis there is no prior — the computed level
  publishes directly (priors are never synthesized from another axis).

`scorer_version: 4` (the v3→v4 bump adds the `structure` axis — additive, the
six prior axes' gates and floors unchanged); datasets without a `scorer_version`
are v1 (priors seed engine-only). A v3 dataset (six axes) still renders —
`structure` is simply absent until the next publish derives it.

**Remediation-introduced tests count as `partial` until mutation-checked.**
The floor promotes robustness dimensions on file presence — so for test files
whose introducing commit is a remediation-run commit (matched via the ledger's
`runs[].commit`), the floor scores `partial` until a mutation-check entry
(the command that broke the reader + the red test output) exists in
`runs[].evidence`. `audit-format.py --ledger <path>` consumes this;
`validate-ledger.mjs` enforces the evidence shape; calibration spot-replays
one recorded mutation check per cycle.

Verify the variance bound deterministically (no LLM cost):

```
python3 .skills/refresh-format-maturity/scripts/audit-format.py --all --json \
  | node .skills/refresh-format-maturity/scripts/repro-check.mjs
```

It reports, per format **and per axis**, the level range over every
combination of that axis's quality dimensions — `PINNED` (spread 0) or a
single-step boundary. Any `>=2-STEP` spread is a scoring leak to fix.

**Change control.** The scorer (`format-triage.js`), the audit script, the
mirror (`repro-check.mjs`), this rubric, `constructs.yaml`, and
`integrations.yaml` form one change-controlled surface: a change that improves
any score may not touch them or relax a test assertion in the same change;
gate/floor changes land across all of them in one commit and trigger the
calibration phase of `process-health` (see
[format-ops.md](./format-ops.md)).

**Anti-gaming rules** (mechanisms, with their checks):

- Scorer/worker separation — enforced by the change-control list above
  (checkable as a CI path rule on remediation PRs).
- Evidence must resolve — unresolvable citations degrade to `unknown`/dropped
  (audit-time, deterministic).
- Mutation checks gate robustness credit (ledger-wired, above).
- Ladder tops anchor on oracles outside the scoring agent's control: Okapi
  parity, external validators (CI conclusions, not test presence),
  *externally-verified* corpora (re-fetched hashes, not self-computed ones),
  snapshot-resolved citations.
- `golden_passed` is measured against the **adjudicated** golden set in the
  ops ledger (human-graded axis levels, versioned by `rubric_sha`), not
  against self-agreement.

**Dataset & history contract (v4)** — backward compatibility, verbatim rules
(the v3 rules carry forward unchanged; the v4 axis-add is additive, so a v3
dataset renders with `structure` absent):

- Rows keep `level`/`next_level` mirroring the **engine** axis (the prior
  parser reads only `formats[].id` + `.level`; the un-migrated page indexes
  `lvlClass[f.level]`). `levels:{axis→grade}`, per-axis dimension grids, and
  `tier:{…}` are **additive** fields.
- `summary.by_level` remains the engine distribution; `summary.by_axis` is
  additive.
- History mutation is remove-today's-entry-then-append only; old entries are
  never rewritten; `by_axis` appears on new snapshots only and the page guards
  (`h.by_axis?.… ?? 0`).
- `__DATE__` is substituted solely by the publish step from
  `date -u +%Y-%m-%d`; history dedupe keys on it.
- Both JSON files are 2-space indented (`vp check` gate).
- Missing `scorer_version` ⇒ v1 priors (engine-only seed); the prior parser
  never gates on version.

## 4. How to score a format (audit procedure)

The `refresh-format-maturity` skill automates this per format; the
`format-triage` workflow does it fleet-wide. By hand:

1. **Identify** `core/formats/<id>/` and whether it has an Okapi counterpart.
   Exclude `exec`/`jsx`/`memorytest` (the reporting denominator is the real
   format dirs).
2. **Run the floor**: `python3 .skills/refresh-format-maturity/scripts/audit-format.py <id>` —
   note each axis's `base..ceiling` band and missing-artifact list.
3. **Score Engine** as before: open the tests and judge whether assertions
   prove fidelity (byte/semantic equality, not "no error"); run
   `go test ./core/formats/<id>/...` and, if parity applies,
   `make parity-sandbox` + the tagged parity test.
4. **Score Vocabulary**: open `vocabulary.yaml`; confirm the audit resolved
   the evidence; spot-check the reader emits the claimed canonical types and
   (for V2) the writer serializes canonical-authored targets.
5. **Score Editor**: check `integrations.yaml` entries and that the probes
   corroborate each declared depth on HEAD.
6. **Score Knowledge**: open `dossier.yaml`; run
   `scripts/format-ops/check-citations.mjs <id>`; check `divergence_kind`
   coverage over `expected_fail`s; confirm reference docs regenerate clean;
   try `scripts/format-ops/context-pack.mjs <id>`.
7. **Score Corpus**: validate `corpus.yaml` (spot re-fetch one
   `origin: url` entry); confirm fetch wiring skips-not-fails; check the
   latest acceptance CI conclusion and corpus-sweep record.
8. **Assign one level per axis** from the strictest unmet criterion; list the
   missing items blocking each axis's next level, ranked by tier impact
   (gating axes first).
9. **Audit divergences** (unchanged): every `expected_fail` has a
   non-`native-bug`, non-pure-`default-diff` `divergence_kind` citing spec +
   Okapi; flag xfails whose runner now logs "assertions pass."
10. **Check upstream + drift** (unchanged): Okapi tracker sweep recipe in
    [format-engineering.md §6](./format-engineering.md#6-okapi-mapping);
    `make contract-audit`; regenerate dashboards — a regression can hide
    behind a stale cache.

## 5. Quick-reference file floors

```
Engine   L1 : reader.go + writer.go + config.go(rejects unknown) + (roundtrip|skeleton)_test
         L2 : + (spec.yaml+spec_test | okapi_skip+invariants+corpus) + schema.go + malformed_test
         L3 : + parity spec_test (or harvest ladder) + corpus|upstream_test + reference wiring
         L4 : + edge-case matrix + schema_test + zero native-bug xfails + fresh dashboards
Vocab    V1 : vocabulary.yaml (read cells RESOLVED-evidenced) + canonical-type literals in package
         V2 : + write cells resolved + vocab_equivalence_test case + block kinds
         V3 : + zero unknown expressible cells + loss table published + vocab-aware preview
Editor   E1 : PreviewBuilder (or exported STRUCTURE_RULES entry) + preview test      [probed]
         E2 : integrations.yaml entry + resolving round-trip/anchor test on HEAD     [probed]
         E3 : + committed add-in/connector manifest on HEAD                          [probed]
         E4 : + registered webhook/event-handler symbol on HEAD                      [probed]
Knowledge K1: dossier.yaml (catalog-registered spec source + implementations) + nativedocs sidecar
         K2 : + spec.yaml refs populated + divergence_kind on every xfail + schema.go
         K3 : + green check-citations output + zero stale xfails + context pack generates
Corpus   C1 : corpus.yaml covering all testdata (tier/origin/license/sha256) + tests consume it
         C2 : + Tier B wiring (corpus: scheme, fetch, skip-not-fail) or countersigned na + verified manifests
         C3 : + externally-verified wild files + edge matrix + generator/fuzz seeds
              + acceptance CI green + green corpus-sweep record + flywheel evidence
Security S1 : reader package imports core/safeio (bounded)                          [floor-only]
         S2 : + a Fuzz* target + testdata/fuzz seed                                 [floor-only]
         S3 : + clean corpus-sweep record in the ledger (0 crash/hang/oom/drift)    [ledger ceiling]
         S4 : + sustained green-sweep / batch-fuzz signal                           [ledger ceiling]
```

<!-- BEGIN: gap-analysis report (generated) -->
## Maturity report (snapshot: 2026-06-19)

Generated by the `bootstrap-publish` (deterministic floor; no quality
demotions) over 55 real formats. Regenerated by every ritual that
republishes the dashboard — do not edit by hand. The dashboard
(`/format-maturity`) carries the live, filterable view.

### Per-axis distribution

- **Engine** — L0:5 · L1:12 · L2:30 · L3:8 · L4:0
- **Vocabulary** — V0:50 · V1:5 · V2:0 · V3:0
- **Editor** — E0:52 · E1:3 · E2:0 · E3:0 · E4:0
- **Knowledge** — K0:6 · K1:36 · K2:13 · K3:0
- **Corpus** — C0:26 · C1:29 · C2:0 · C3:0
- **Security** — S0:39 · S1:14 · S2:2 · S3:0 · S4:0
- **Structure & Geometry** — G0:41 · G1:1 · G2:7 · G3:4 · G4:2

### Per-format vector

| Format | Tier | Engine | Vocabulary | Editor | Knowledge | Corpus | Security | Structure & Geometry | Top engine gap |
|---|---|---|---|---|---|---|---|---|---|
| `androidxml` | available | L1 | V0 | E0 | K2 | C0 | S0 | G0 | add malformed_test.go |
| `applestrings` | available | L1 | V0 | E0 | K2 | C0 | S0 | G0 | add malformed_test.go |
| `arb` | maintained | L3 | V0 | E0 | K2 | C0 | S0 | G0 | — |
| `asciidoc` | available | L3 | V1 | E0 | K2 | C1 | S1 | G3 | — |
| `audio` | available | L0 | V0 | E0 | K0 | C0 | S0 | G0 | add malformed_test.go |
| `csv` | maintained | L3 | V0 | E0 | K1 | C1 | S1 | G3 | — |
| `designtokens` | available | L1 | V0 | E0 | K2 | C0 | S0 | G0 | add malformed_test.go |
| `doclang` | available | L1 | V0 | E0 | K0 | C0 | S0 | G4 | add malformed_test.go |
| `docling` | available | L1 | V0 | E0 | K0 | C0 | S0 | G4 | add cli/parity spec_test |
| `doxygen` | maintained | L2 | V0 | E0 | K1 | C1 | S0 | G0 | add a corpus/upstream test |
| `dtd` | maintained | L2 | V0 | E0 | K1 | C1 | S0 | G0 | add a corpus/upstream test |
| `epub` | available | L0 | V0 | E0 | K1 | C1 | S1 | G0 | add a corpus/upstream test |
| `fixedwidth` | maintained | L2 | V0 | E0 | K1 | C1 | S0 | G0 | add a corpus/upstream test |
| `html` | available | L1 | V1 | E1 | K1 | C0 | S2 | G3 | add malformed_test.go |
| `i18next` | available | L1 | V0 | E0 | K2 | C0 | S0 | G0 | add malformed_test.go |
| `icml` | maintained | L3 | V0 | E0 | K1 | C1 | S0 | G0 | — |
| `idml` | available | L3 | V0 | E0 | K1 | C0 | S1 | G2 | — |
| `image` | available | L1 | V0 | E0 | K0 | C0 | S0 | G1 | add malformed_test.go |
| `json` | maintained | L2 | V0 | E0 | K2 | C0 | S2 | G0 | add a corpus/upstream test |
| `markdown` | available | L2 | V1 | E1 | K1 | C1 | S1 | G3 | add a corpus/upstream test |
| `mdx` | available | L1 | V0 | E1 | K2 | C0 | S1 | G0 | add malformed_test.go |
| `messageformat` | maintained | L2 | V0 | E0 | K1 | C1 | S0 | G0 | add a corpus/upstream test |
| `mif` | maintained | L3 | V0 | E0 | K1 | C0 | S0 | G0 | — |
| `mo` | available | L0 | V0 | E0 | K2 | C0 | S0 | G0 | add malformed_test.go |
| `mosestext` | maintained | L2 | V0 | E0 | K1 | C1 | S0 | G0 | add a corpus/upstream test |
| `odf` | available | L1 | V0 | E0 | K1 | C0 | S1 | G2 | — |
| `openxml` | maintained | L2 | V1 | E0 | K1 | C1 | S1 | G2 | add a corpus/upstream test |
| `paraplaintext` | maintained | L2 | V0 | E0 | K1 | C1 | S1 | G0 | add a corpus/upstream test |
| `pdf` | available | L0 | V0 | E0 | K0 | C0 | S0 | G0 | add malformed_test.go |
| `phpcontent` | maintained | L2 | V0 | E0 | K1 | C1 | S0 | G0 | add cli/parity spec_test |
| `plaintext` | maintained | L2 | V0 | E0 | K1 | C1 | S1 | G0 | add a corpus/upstream test |
| `po` | maintained | L2 | V0 | E0 | K1 | C1 | S1 | G2 | add a corpus/upstream test |
| `properties` | maintained | L2 | V0 | E0 | K1 | C1 | S1 | G0 | add a corpus/upstream test |
| `regex` | maintained | L2 | V0 | E0 | K1 | C1 | S0 | G0 | add a corpus/upstream test |
| `resx` | maintained | L3 | V0 | E0 | K2 | C0 | S0 | G0 | — |
| `rtf` | maintained | L2 | V0 | E0 | K1 | C1 | S0 | G0 | add a corpus/upstream test |
| `splicedlines` | available | L1 | V0 | E0 | K1 | C1 | S0 | G0 | add a corpus/upstream test |
| `srt` | maintained | L2 | V0 | E0 | K1 | C1 | S0 | G0 | add cli/parity spec_test |
| `tex` | maintained | L2 | V0 | E0 | K1 | C1 | S0 | G0 | add a corpus/upstream test |
| `tmx` | maintained | L2 | V0 | E0 | K1 | C1 | S0 | G0 | add a corpus/upstream test |
| `transtable` | maintained | L2 | V0 | E0 | K1 | C0 | S0 | G0 | add a corpus/upstream test |
| `ts` | maintained | L2 | V0 | E0 | K1 | C1 | S0 | G2 | add a corpus/upstream test |
| `ttml` | maintained | L2 | V0 | E0 | K1 | C1 | S0 | G0 | add a corpus/upstream test |
| `ttx` | maintained | L2 | V0 | E0 | K1 | C0 | S0 | G0 | add a corpus/upstream test |
| `txml` | maintained | L2 | V0 | E0 | K1 | C0 | S0 | G0 | add a corpus/upstream test |
| `versifiedtext` | available | L1 | V0 | E0 | K1 | C1 | S0 | G0 | add a corpus/upstream test |
| `video` | available | L0 | V0 | E0 | K0 | C0 | S0 | G0 | add malformed_test.go |
| `vignette` | maintained | L2 | V0 | E0 | K1 | C0 | S0 | G0 | add a corpus/upstream test |
| `vtt` | maintained | L2 | V0 | E0 | K1 | C1 | S0 | G0 | add a corpus/upstream test |
| `wiki` | maintained | L2 | V0 | E0 | K2 | C1 | S0 | G0 | add a corpus/upstream test |
| `xcstrings` | maintained | L3 | V0 | E0 | K2 | C0 | S0 | G0 | — |
| `xliff` | maintained | L2 | V1 | E0 | K1 | C0 | S0 | G2 | add a corpus/upstream test |
| `xliff2` | maintained | L2 | V0 | E0 | K1 | C0 | S0 | G2 | add a corpus/upstream test |
| `xml` | maintained | L2 | V0 | E0 | K2 | C1 | S1 | G0 | add cli/parity spec_test |
| `yaml` | maintained | L2 | V0 | E0 | K1 | C1 | S1 | G0 | add a corpus/upstream test |
<!-- END: gap-analysis report -->

## 6. Open questions

Genuine design decisions recorded rather than silently resolved. Items
resolved by the multi-axis redesign are kept with their resolution for one
cycle, then pruned (an editorial decision made in the `process-health`
ritual, not by the mechanical snapshot regeneration).

1. **Harvest formats and `spec.yaml`** — *direction decided*: harvest formats
   move onto native-only `spec.yaml` (no parity examples) so the Knowledge
   axis and contract-audit coverage become uniform; until migrated, the
   okapi_skip/invariants/corpus ladder remains the accepted L2/K2 substitute.
   Implementation tracked as a GitHub issue.
2. **Byte-exactness at L4** — unchanged stance: faithful-class divergence
   (xliff2's normalizing DOM writer, #560) is L4-compatible when tracked,
   attributed, and spec-justified.
3. **Legacy `formatSpecs` parity table** — *direction decided*: retire into
   `spec.yaml` (migrate `NewWriter`/Tikal/Skip knowledge into spec fields;
   fold generated fixtures in as `origin:`-tagged examples). Tracked as a
   GitHub issue.
4. **Multi-view spec cases** — *designed*: see
   [format-spec-cases.md](./format-spec-cases.md) (case grammar, neutral
   block-event oracle, accept-mode guard rails, AI generation loop).
   Implementation tracked as a GitHub issue; the `case-gen` ritual is blocked
   on it.
5. **Reporting denominator** — resolved: the real format dirs
   (`exec`/`jsx`/`memorytest` excluded everywhere, including the dashboard); the
   count is code-controlled — the dashboard and the generated snapshot derive it
   from the universe, never a hardcoded number.
6. **Step parity** — still open: steps are tools, not formats; out of scope
   for the format axes. Revisit if step regressions recur.
7. **Security axis** — *resolved: scored as of 2026-06-13.* The S0–S4 axis
   (§2.6) is wired across the scorer (`format-triage.js`), the audit
   (`audit-format.py` `axes.security`), the mirror (`repro-check.mjs`), and the
   `/format-maturity` dashboard now that `core/safeio` budgets and the
   corpus-sweep harness exist. It is a **non-gating display axis** for now;
   promoting it into the headline-tier minimum is a future `tier-review`
   decision.
8. **`detection` floor signal** — currently a constant (v2 compatibility);
   candidate real signal: presence in `register_test.go`'s detection lists.
   Change requires a gate change ritual (calibration + same-commit mirror
   updates).
9. **Structure & Geometry axis** — *resolved: scored as of 2026-06-19.* The
   G0–G4 axis (§2.7) is wired across the scorer (`format-triage.js`), the audit
   (`audit-format.py` `axes.structure`), the mirror (`repro-check.mjs`), and the
   `/format-maturity` dashboard, riding the `core/model/structure.go` standoff
   payloads. Resolved sub-decisions: it is **one cumulative ladder** with
   **geometry as the top rung** (not two axes, not an independent geometry
   top-up); under that cumulative rule `odf`/`idml` emit geometry but no roles
   and therefore **cap at G2**; `na` geometry is a **ceiling cap, not a
   gate-pass**; the AD-028 authority tier is declared in `structure.yaml` now
   with the badge rendered later. It is a **non-gating display axis** for now;
   promoting it into the headline-tier minimum is a future `tier-review`
   decision. The seven axes now group into three reading-aid families
   (Comprehension / Assurance / Enablement, §2) without changing the gating rule
   (`min` over Engine ∧ Corpus ∧ Knowledge).
