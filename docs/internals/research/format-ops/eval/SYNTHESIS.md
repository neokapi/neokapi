# SYNTHESIS — multi-axis format framework upgrade (decision-ready)

Inputs: `spec-engine.md`, `triage-contract.md`, `vocabulary-reality.md`, `editor-integrations.md`,
`corpus-and-assets.md`, `gap-review.md` (all in `/tmp/neokapi-format-ops/eval/`). Repo root:
`/Users/asgeirf/src/neokapi/neokapi/.claude/worktrees/format-process/`. Format universe: 49 scored ids
(`format-triage.js:33-41`), 52 dirs under `core/formats/` minus `{exec,jsx,memorytest}` non-formats
(`audit-format.py:46`).

---

## 1. Current-state scorecard per future axis

### Engine (L0–L4) — mature ladder, stale outputs
**Measurable today:**
- Deterministic floor: `audit-format.py` (280 LOC) emits `{format,type,has{9 booleans},applymap_rejects_unknown,test_kinds,coarse_level,base,ceiling}` per format (`audit_one()`, lines 173–200).
- Scorer v2 pipeline: `.claude/workflows/format-triage.js` (434 LOC) — floor (`dimsFromFloor` :174), evidence-or-demote (`enforceEvidence` :137), demote-only quality dims `{writer,parity,corpus}` (`reconcileDims` :195), pure-function rubric (`gateLevel` :211), floor caps (`capByFloor` :233), sticky anchor (`applySticky` :246), ensemble mode (`modeDimensions` :153). Variance bound: `repro-check.mjs` (hand-synced mirror, lines 15–17).
- Executable specs: 41 `core/formats/<id>/spec.yaml` (17,053 lines; 575 `origin:` fields), 3 consumers — native runner (`core/format/spectest/runner.go`), parity runner (`cli/parity/spec/runner.go`), drift audit (`scripts/contract-audit/main.go:1705,1745`). 38 parity wirings (`cli/parity/formats/*_spec_test.go`).
- Guardrails: `core/formats/maturity_test.go` — `TestFormatSpecIsGated` :71 (hard), `TestRoundTripTestNamingConvention` :88 + `grandfatheredRoundtrip` ledger :30 (hard+ledger), `TestRobustnessCoverage` :116 (advisory).
- Dashboard: `web/src/pages/format-maturity/` reading `web/static/data/format-maturity.json` + `format-maturity-history.json`.

**But:** committed dashboard is **scorer v1** (`source:"…(agent assessment)"`, no `scorer_version`/`run_integrity` — triage-contract.md §1.1 "Committed-file drift"); history has **1 entry** (2026-05-31); ground truth moved under it — commit `b7201a9f5` (2026-06-04) → **42/49 malformed tests** while the dashboard still ranks "add malformed_test.go" as top gap and `format-maturity.md:179` says "only arb/resx/xcstrings" (gap-review.md §0). Assertion vocabulary = 7 reader-side fields over block source text only (`core/format/spec/spec.go:225-233`); **no round-trip/writer assertion exists** despite being promised in `html/spec.yaml:659-661`, `csv/spec.yaml:671-675`, `format-engineering.md:133` (spec-engine.md §2).

### Vocabulary (V0–V3) — registry exists, adoption 5/49, drift live
**Measurable today:**
- Canonical registry `core/model/vocabulary.go:69-127` + 4 embedded packs (`core/model/vocabularies/`: common-formatting, rich-html, rich-jsx, code-tokens); semantic-HTML projection `core/model/run_semantic_html.go:70` (consumed by `core/mt/tools/translate.go:101,108`); constraint enforcement `core/model/text_edit.go:36-38,214-222`.
- Emitters of canonical `fmt:*`/`link:*`/`media:*` types: **html, markdown, openxml, xliff(1.2), jsx = 5/49** (html mapping `core/formats/html/reader.go:92-105`; openxml constants file `core/formats/openxml/vocabulary.go`; xliff ctype table `core/formats/xliff/reader.go:1713-1732`). Only **openxml's writer is Type-aware** (`writer.go:4203-4273,2643,2678`).
- Floor signals already emitted but unconsumed: `has.schema`, `applymap_rejects_unknown` (`audit-format.py:71-82,188`; noted free at `format-triage.js` dimsFromFloor — triage-contract.md §1.1).

**But:** ~23 formats emit generic/format-local types (json/po/properties literal `"code"` — `json/reader.go:544-548`; odf `x-*` :465; xliff2 deliberately non-canonical `xliff2/reader.go:488-548`); block-level semantics free-form (`core/model/block.go:10-28`; `DisplayHint` set by no reader — only `bowrain/connector/figma.go:164`); **real Go↔TS pack drift** (TS copies missing strikethrough/sub/sup; `rich-jsx.json` unshipped — vocabulary-reality.md §2); no cross-format equivalence test, no per-format mapping artifact, no rubric dimension (the word "vocabulary" appears once in `format-maturity.md`).

### Editor (E0–E3) — three preview tiers, no registry, deepest layer unmerged
**Measurable today:**
- E0 universal: generic preview `core/editor/preview_generic.go` via `editor.BuildPreview` (`core/editor/preview.go:11-16`).
- E1 probe-able: `format.PreviewBuilder` (`core/format/preview.go:12`) — **3/49 implement** (`core/formats/{html,markdown,mdx}/preview.go`) + `jsx/jsx.go:527-537`. TS-side shape table `STRUCTURE_RULES` (`packages/ui/src/components/preview/renderDoc.ts:375,436-477`) covers pptx/xlsx/docx — invisible to Go tooling (editor-integrations.md §6).
- E2 evidence: roundtrip byte-equality tests per format (maturity framework) + skeleton splice; but the `originalContentSetter` interface is **locally re-declared** in `bowrain/connector/microsoft365.go:27-32` — no exported probe in `core/format` (editor-integrations.md "E2").
- E3 inventory: Office task pane + Google add-on + google-workspace/microsoft365 connectors are **all on unmerged PR #776** (`origin/worktree-google-ms-addon`, head `3967be87b`); on HEAD only wordpress/figma/hubspot (shallow: free-string `Format:"html"` labels, figma `Publish` errors at runtime — `bowrain/connector/figma.go:105-109`) and the platform's own editor (`bowrain/server/editor.go:615-905`, format-aware preview only for html/markdown/mdx).

**Missing for determinism:** no integration registry/manifest; `ContentItem.Format` free string (`bowrain/core/connector/connector.go`); no connector capability declaration; preview knowledge split Go/TS (editor-integrations.md "Missing for determinism" 1–5).

### Knowledge (K0–K3) — strong spec engine, scattered + manually-synced prose
**Measurable today:** spec.yaml `okapi_refs`/`native_refs`/`spec_refs` three-way grounding (`spec.go:89-121`); `origin:` provenance near-universal (575×); contract-audit weekly cron (`.github/workflows/contract-audit.yml`); `nativedocs` sidecars + `reference-data-drift.yml` gate; `parity-annotations.yaml` sidecars ×10 with severity/issue/spec_ref (`cli/parity/roundtrip/annotations.go:69-72`); per-format GitLab sweep recipe (`.skills/refresh-format-maturity/SKILL.md` Step 4).
**But:** explicit `divergence_kind` on only **3 of 140+** `expected_fail`s (spec-engine.md §4); two disjoint divergence vocabularies (spec.yaml `divergence_kind` vs annotations `severity`); spec_refs unversioned free strings; docs snapshot hand-pasted and stale within 5 days (`format-maturity.md:139-256`, names a workflow that no longer exists — gap-review.md G6.1); **two rubric truths** (prose table :72-78 vs `gateLevel()`, already subtly diverged — G6.2); no per-format dossier; sweep results unstored (G5.1); no Okapi version-bump runbook (pin `OKAPI_VERSION ?= 1.48.0`, Makefile:1004, spread across ≥6 locations — G5.2).

### Corpus (C0–C3) — three storage idioms proven, no manifest, single-laptop riser
**Measurable today:** 207 files / 3.0 MB across `core/formats/*/testdata` (corpus-and-assets.md §1); **10 real-corpus formats** with provenance-pinned `testdata/corpus/SOURCES.md` (commit-pinned re-fetch cmds, SPDX licenses — arb/idml exemplars); `corpus_test.go` ×10, `upstream_test.go` ×4; fetched external corpus `okapi-testdata` (versioned tag `okapi-testdata-1.48.0` on okapi-bridge, `scripts/fetch-okapi-testdata.sh`, resolver `core/format/spec/helpers.go:60-90`, `okapi:` scheme :42-49, skip-not-fail); 8 acceptance suites against real validators (`make format-acceptance`, Makefile:141; `.github/workflows/format-acceptance.yml`); merge-never-drop publish pattern (`scripts/publish-corpus.sh`). **Rollup: 10 real-corpus / 17 some-real / 21 synthetic-only / 4 none** (corpus-and-assets.md §6).
**But:** no sha256 anywhere ("byte-identical by assertion only"); no machine-readable manifest; Okapi clone hard-pinned to `/Users/asgeirf/src/okapi/Okapi` (load-bearing for 15 specs + parity + fixtures regen — gap-review.md G4.1); no fuzz targets (`grep 'func Fuzz' core/formats` → empty), no malicious-input corpus (G4.4); no generation leg for formats lacking real corpora (G4.3).

---

## 2. Master gap list vs the 7 goals (deduped, with evidence)

**G1 — Self-healing/self-improving loops**
1. No loop closes onto prompts/rubric; prompts are hardcoded strings, no changelog/A-B (gap-review G1.1; `format-triage.js:263-308`).
2. Run outcomes evaporate — `test_passed=false`, low_agreement, suppressed moves unstored (G1.2; workflow return :432-434).
3. Sticky anchor masks systematic scorer drift; `golden_passed` measures self-agreement not correctness; no anchor-free calibration ritual (G1.4; `applySticky` :246-253).
4. **Malformed self-gaming channel**: floor promotes on file presence (`dimsFromFloor` :185), `QUALITY` set excludes malformed → AI adds file, metric improves, no mutation check (G1.5).
5. Remediation regression surface: package-local tests only (`remediatePrompt` :293 — `go test ./core/formats/<fmt>/`), never `make test`/parity (gap-review cross-cutting 1).

**G2 — Maturity visibility / vocabulary / editor axes**
6. Longitudinal visibility nominal: 1 history entry; no CI writes `web/static/data/*.json` (G2.1; grep over workflows = no writer).
7. No vocabulary dimension; 5/49 canonical emitters; Go↔TS drift live, no gate (vocabulary-reality §2, §7; G2.2).
8. No editor axis; PreviewBuilder 3/49; E2 interface unexported (microsoft365.go:27-32); E3 unmerged (#776) and unindexed (editor-integrations "Missing" 1–5; G2.3).
9. WASM/browser viability per format unscored (cgo-less: no ICU/native SQLite) (G2 blind spots).
10. Scorer v2 never published; committed dataset/dashboard/docs three-way drift (gap-review §0; triage-contract §1.1/§1.4 — page types miss v2 fields, `HistorySnapshot` misses `golden_passed`/`moves` already appended by publishPrompt :306).

**G3 — Spec harvesting / cross-framework / example-based specs**
11. "Other frameworks" = exactly one (Okapi); no translate-toolkit/Fluent/MF2/Pandoc model (G3.1; note GPL-2.0 licensing wall for translate-toolkit — G3 blind spot).
12. No round-trip/writer assertion type (spec-engine §2 — five documented IOUs); no run-shape/semantic assertions (spec-engine §5b); no non-Block-part assertions (§ punch 8).
13. 8 harvest formats + `mo` have no spec.yaml → knowledge surface non-uniform (spec-engine §1; Open question 1).
14. `spec_refs` unversioned/unanchored; "the spec wins ties" unenforceable (G3.2).
15. No spec-document→examples generation loop, no coverage accounting, no AI-ingestion ladder (G3.3; spec-engine §5c).
16. Legacy `formatSpecs` table coexists with spec runners; per-row knowledge (NewWriter, Tikal, Skip ledgers) has no spec.yaml home (spec-engine §3; Open question 3).

**G4 — Reference-file harvesting/storage/generation**
17. Single-machine corpus dependency (`/Users/asgeirf/src/okapi/Okapi` in Makefile :598, docs, both skills, `format-triage.js:48`) (G4.1).
18. No corpus manifest (source/license/sha256/feature-coverage); real-vs-synthetic re-judged by LLM each run (G4.2; corpus-and-assets §5.4).
19. No generation pipeline for missing corpora (headless Word/LibreOffice, xcstrings-tool, pandoc) (G4.3).
20. No malicious-input corpus, no fuzz targets (G4.4).
21. 21 synthetic-only + 4 none formats blocked from L3/L4 by corpus rubric (corpus-and-assets §6 rollup; `format-maturity.md:76-78,99`).

**G5 — Spec-change & other-implementation tracking**
22. GitLab sweep results unstored — no watermark (G5.1).
23. No Okapi version-bump runbook across ≥6 pin locations (G5.2).
24. W3C/OASIS/Unicode/platform spec changes unwatched (G5.3).
25. No format retirement lifecycle; maturity monotonic; sticky anchor resists downgrades (G5 blind spot; `mo` case `format-maturity.md:239`).
26. Issue linkage is prose not data (#560/#617/#504) — no run notices a closure (cross-cutting 9).

**G6 — Learning-material generation/maintenance**
27. "(generated)" docs block with no generator; rubric duplicated prose-vs-code with no agreement test (G6.1–2).
28. No prompt/rubric versioning in history; prompts duplicate rubric knowledge inline (G6.4 + blind spot — `scorePrompt` :275 vs docs `divergence_kind` teaching).
29. No per-format dossier generator (spec.yaml + maturity row + parity row + corpus manifest + notes) (G6 blind spot; G3.5).

**G7 — New-format radar**
30. Nothing committed: no radar artifact, no rejection log, no demand telemetry, no plugin-first staging path; concrete absences (SVG, DITA, TOML, ipynb, game-l10n, AI-era artifacts) (gap-review G7.1-3 + blind spots).

---

## 3. Consolidated engineering punch list

### 3.1 Multi-axis scorer + dashboard (triage-contract §2 is the canonical file-by-file table)
- **`.claude/workflows/format-triage.js`**: add `AXES` table (engine L, vocabulary V0–V3, editor E0–E3, knowledge K0–K3, corpus C0–C3); generalize `ORDER/RANK/NEXT` (:110-112) per-axis; partition `CANON/LABELS` (:113-118) per axis (engine←reader/writer/spec/parity/malformed/detection; vocabulary←config+new `schema`,`speckeys`; knowledge←docs+new `refs`,`sidecar`,`annotations`; corpus←corpus+new `fixtures`,`upstream`; editor←new `preview`,`previewtest`); **`normDim` :121 → exact-match** (substring collisions: `speckeys`⊃`spec`); replace hardcoded `detection:'complete'/docs:'complete'` (:186-187) with real signals; split `gateLevel` :212 into 5 pure gates; per-axis `QUALITY` sets, `capByFloor`, `applySticky` (per-axis `delta_justification` so one citation can't gate five moves); SCORE schema enum :80 extended; Prep parser shim `priorByFmt[f.id] = f.levels ?? {engine: f.level}` (:336); rows keep `level`/`next_level` mirroring engine; `buildDataset` :310 → `scorer_version: 3`, `axes`, `axis_labels`, `dimension_axes`, `summary.by_axis` (zero-init grades); `publishPrompt` :298 history append gains `by_axis` on new snapshots only; `run_integrity.moves`/`low_agreement` per axis.
- **`audit-format.py`**: additive `axes:{<axis>:{base,ceiling}}` block (keep top-level `base/ceiling/coarse_level` — repro stdin contract); new signals: V — consume `has.schema`/`applymap_rejects_unknown`, `schema_test` probe, `okapi_param:` census from spec.yaml; E — `has.preview` (preview.go / `format.PreviewBuilder` grep), `preview_test` probe; K — `has.nativedocs` (`scripts/gen-refs/nativedocs/formats/<id>.yaml` ≠ template), refs census (`spec_refs/okapi_refs/native_refs` counts), annotations-attribution check; C — `testdata_census` (count+extensions), `has.okapi_fixture_input` (`input_file:\s*okapi:`), `has.parity_fixtures` (`fixtures_<id>_generated.go`). Per-axis harvest ceilings ≠ engine L3 band.
- **`repro-check.mjs`**: mirror every gate/floor change same-commit; per-axis spread (editor spread-0 by construction — no quality dims).
- **Dashboard** `web/src/pages/format-maturity/`: `_types.ts` — `AxisId`, per-axis grade unions, optional `FormatRow.levels?`, type the already-shipped v2 fields + `HistorySnapshot.{golden_passed?,moves?,by_axis?}`; `index.tsx` — grade→class map for V/E/K/C (+ CSS next to `.lvlL0-.lvlL4`, `index.module.css:63-89`), axis mini-bars from `summary.by_axis ?? {}`, conditional axis trend lanes (`h.by_axis &&`), axis-scoped filters/sort (never cross-alphabet `localeCompare`), `colSpan` update (:210).
- **`core/formats/maturity_test.go`** new advisory guardrails (hard only at zero violators): `TestSchemaIsGated` (V), `TestPreviewCoverage` (E — html/markdown/mdx burndown), nativedocs/spec_refs presence (K — `../../`-relative + skip-if-absent; cwd is `core/formats/`), empty-testdata census (C). Add a vocab guardrail (cross-format equivalence — §3.4).
- **Docs**: `docs/internals/format-maturity.md` four ladder tables, scorer-v3 §, per-axis rubric tags, regenerate gap block; `.skills/refresh-format-maturity/SKILL.md` + `.skills/implement-format/SKILL.md` rewording.

### 3.2 Dossier / knowledge files (spec-engine punch list 1–9)
- **Round-trip assertion type**: `Assertions.RoundTrip *RoundTripAssertion{mode: byte_exact|idempotent|pseudo, output_file?, output_contains?}` (`core/format/spec/spec.go:225`); `NewWriter func(variant) format.DataFormatWriter` hook on `NativeRunner` (spectest/runner.go:24) and ParityRunner (pattern proven at `cli/parity/formats/spec.go:139`, reports `Kind="format-roundtrip"`); `EvalOutputAssertions(output []byte, a)` beside `EvalAssertions` (helpers.go:153). Resolves Open question 4 + the html/csv IOUs; turns `grandfatheredRoundtrip` ledger into burndown fuel.
- **Run-shape semantic assertions**: `block_runs` signatures (`["text","pcOpen:fmt:bold",…]`) + `has_run_with_type` keyed to `core/model/vocabulary.go`; `RunSignature(run)` helper next to `BlockTexts`; native-strict, parity opt-in (bridge typing differs — `parity_warn` rationale, spec.go:196-197).
- **Uniform coverage**: native-only spec.yaml for the 8 harvest formats + `mo` (Open question 1).
- **Retire legacy `formatSpecs`** (cli/parity/formats/spec.go:204): migrate NewWriter/Tikal/BridgeFilterClass/Skip ledgers into spec.yaml fields; fold `fixtures_*_generated.go` inputs in as `origin:"okapi-fixture:…"` + `informational: true` examples.
- **Mandatory `divergence_kind`** for every `expected_fail` (3 explicit vs 140+) — enforce in `Validate()` (load.go) or contract-audit; **unify** the spec/annotation divergence taxonomies (severity≈divergence_kind).
- **AI-ingestion ladder**: `origin: "generated: <model>/<date> verified-by: …"` fourth form; `informational:` example field; content-hash dedup in `Validate()`; `kapi spec verify --fill` (run reader, write back observed assertion values).
- **Non-Block + gap structure**: `data_count`/`note_texts`/`layer_depth` assertion fields; promote "intentionally under-specified" prose tails to structured `gaps:` entries (id+reason+okapi_refs) so the dashboard can count them.
- **Per-format dossier generator**: one regenerable artifact per format joining spec.yaml + maturity row + parity rows + corpus manifest + parity-annotations + PARITY_NOTES anchor (gap G29); also fixes the "(generated)" docs block by giving it a real generator.

### 3.3 Corpus store (corpus-and-assets §5 — compose the three proven idioms)
- **Release**: versioned tag `format-corpus-vN` on neokapi/neokapi (okapi-testdata model, NOT a mutable singleton bucket); per-format assets in one release; first-publish auto-create with `--latest=false` (the form now implemented in `scripts/publish-corpus.sh`).
- **`scripts/fetch-corpus.sh`**: clone of `fetch-okapi-testdata.sh` mechanics — versioned gitignored `corpus/<version>/<format>/`, idempotent skip, `FORCE_FETCH=1`, `-vN` respin suffix, API asset-URL + token-via-header-file; resolver mirrors `FindOkapiTestdataRoot` (helpers.go:60-90); add **`corpus:` input_file scheme** beside `okapi:` in `ResolveFilePath` (helpers.go:42-49); tests skip-with-fetch-command (wiki/openxml pattern).
- **`scripts/publish-corpus.sh`**: merge-never-drop (download→mktemp→`rsync -a` overlay→repack→`--clobber`) — the canonical live implementation of this idiom (the retired `docs-assets` flow was its precedent).
- **Manifest**: promote SOURCES.md to machine-readable `corpus/<format>/manifest.yaml` — `source_repo, source_path, commit, license (SPDX), fetch_cmd, roundtrip_contract (byte-exact|semantic), sha256` (the one missing field). Publish verifies sha256 pre-pack; fetch/corpus_test verify post-extract. Generate human SOURCES.md from it. This also makes "real-vs-synthetic" a recorded fact, ending per-run LLM re-judgment (gap 18) and feeding the C-axis floor.
- **CI staging**: best-effort `gh release download` step à la `docs-kapi.yml:96-110` (`::warning` degradation) + make-prerequisite wiring like `parity-sandbox.sh:88-105`.
- **Scope**: 3.0 MB vendored testdata stays in git (offline `make test`); release is for growth (idml/mif/pdf/openxml binaries, license-cleared catalogs) targeting the 21 synthetic + 4 none formats' L3→L4 burndown.
- **Later legs**: generation pipeline (G4.3) and malicious/fuzz corpus (G4.4) ride the same store.

### 3.4 Vocabulary registry (vocabulary-reality §7 ranked list)
- **Per-format `vocab-map` data artifact** (native element ⇄ canonical type ⇄ subtype), replacing 5 Go map literals (`html/reader.go:92-105`, markdown switch, `openxml/vocabulary.go`, `xliff/reader.go:1713`, `tools/span_classify.go:24-50`); consumed by readers/writers/bridge + a completeness report → V-axis floor signal.
- **Cross-format parity test**: shared fixture sentence (bold/italic/link/image) asserted to yield identical `Type` sequences across html/markdown/openxml/xliff; per-format completeness vs vocab-map; guardrail in `maturity_test.go`.
- **Go↔TS single-sourcing + drift gate** (mirror `make check-contract-types`): fixes the live drift (Go `common-formatting.json` ⊃ TS copies; `rich-jsx.json` unshipped to `packages/ui`/`bowrain/packages/ui` vocabularies).
- **Block-level vocabulary**: canonical block-kind registry (paragraph/heading/listitem/cell/title/quote + level), readers adopt `DisplayHint.ContentType` (`core/model/displayhint.go`), openxml pStyle→block-kind mapping (`core/formats/openxml/styles.go` currently drops it).
- **XLIFF 2 canonicalization** in `inlinesToRuns` (`xliff2/reader.go:488-548`) and write-back; **long-tail cleanup** (json/po/properties `"code"`→`code:placeholder`, odf `x-*`, bridge `okapi:*` instead of post-hoc span_classify laundering).
- **Editor parity**: make `RunSequence`/BlockInspector (kapi-desktop/kapi-lab, `packages/ui/src/components/preview/RunSequence.tsx`) vocabulary-aware via the already-exported `tagSemantics.ts`.
- Optional, zero-model-change: stand-off `style` overlay via the open `OverlayType` + payload registry (`core/model/overlay.go:18-33`, `annotation_registry.go:27-40`) for non-paired-codeable styling (ODF spans).

### 3.5 Editor-integration registry (editor-integrations "Missing for determinism")
- **Export the E2 interface** from `core/format` (today locally re-declared `originalContentSetter`, `bowrain/connector/microsoft365.go:27-32`) so writers' skeleton-splice capability is probe-able alongside `format.PreviewBuilder` (E1 probe, `core/format/preview.go:12`).
- **Connector capability declarations**: `CanPublish()`-style capabilities replacing runtime `errors.New` (figma.go:105-109); `ContentItem.Format` → `registry.FormatID` (or validated against it) in `bowrain/core/connector/connector.go`.
- **Committed integrations index JSON** generated from registries (`bowrain/connector/register.go` RegisterAll/RegisterRemote + add-in manifests `bowrain/apps/office-addin/manifest.{xml,json}`, `google-workspace-addon/{appsscript.json,deployment.json}`): connector/add-in → editors → formats → capabilities → depth. E3 becomes scoreable.
- **Export `STRUCTURE_RULES` as JSON** (repo's `//go:generate`-committed-artifact convention) so Go scoring sees TS preview shapes (`renderDoc.ts:364-477`).
- Sequencing note: most E3 substance is on **PR #776** — merge (or explicitly score HEAD-only) before the first E-axis publish, else every format scores E≤2.

### 3.6 Runbook + ledger
- **`docs/internals/format-ops-runbook.md`**: single entry point naming every ritual + cadence (fixes ops fragmentation, cross-cutting 10): triage-score, remediate, docs-snapshot-sync, parity-publish, contract-audit (CI-owned), okapi-sweep, **okapi version-bump** (all ≥6 pin locations enumerated), xfail-hygiene (gh issue states), **calibration** (anchor-free golden-set, triggered by cadence OR rubric/prompt/model change — closes G1.3/4), corpus-census, format-radar (+ rejection log — G7), vocab-drift check.
- **`docs/internals/format-ops-ledger.json`**: §5 below. Seeded mechanically from signals S1–S14 (gap-review "Durable signals" table).
- **Self-gaming fixes** alongside: mutation spot-check for AI-added malformed tests (break reader → assert new test reds); remediation must run `make test` or at minimum parity for shared-surface diffs; record `model_id`+`prompt_sha`+`scorer_version` per history snapshot.

---

## 4. Risks / pitfalls

**Backward compatibility**
1. Dataset/row keys the page consumes (`generated_at,target_level,source,summary,dimensions,dimension_labels,formats[]{id,type,level,next_level,…}`) must stay shape-stable; all axis data additive (triage-contract §2.9).
2. Prep prior-parser reads only `formats[].id` + `.level` (format-triage.js:336) — rows must keep mirroring engine into `level` or the next run loses its anchor.
3. History: never rewrite old entries; remove-TODAY-then-append only (:306); `Trend` consumes `{date,total,by_level}` forever; new fields optional + guarded (`h.by_axis?.…?? 0`).
4. `repro-check.mjs` stdin contract = `format,type,has,test_kinds,base,ceiling`; `axes{}` strictly additive in audit JSON.
5. Treat missing `scorer_version` as v1 — committed dataset predates v2; never gate the Prep parser on version (triage-contract pitfall 3).

**Determinism / scoring integrity**
6. Sticky-anchor migration traps: do NOT synthesize priors for new axes from engine level (all moves suppressed by the citation regex :250, freezing bogus grades); per-axis `delta_justification`; the `!prior` branch (:247) is the correct first-run path (pitfall 4).
7. `normDim` substring matching mis-buckets new dims (`speckeys`→spec); unmatched dims silently default to `'none'` (:147) and zero a whole axis — switch to exact ids + update the SCORE enum in lockstep (pitfall 5).
8. Hardcoded `detection/docs:'complete'` makes the K-axis vacuous unless replaced with real signals (pitfall 6).
9. Hand-synced mirror: every gate/floor change lands in `format-triage.js` AND `repro-check.mjs` in one commit, or the published variance bound lies (pitfall 7).
10. Two format-universe sources: `ALL_FORMATS` hardcoded list (:33-41) vs `all_formats()` dir-walk — new format scores only after the list edit (pitfall 8).
11. Malformed self-gaming stays open unless the mutation check ships (gap-review G1.5) — adding axes multiplies file-presence-promotable surface (preview.go, schema_test, manifests).
12. Per-axis harvest ceilings must NOT inherit the engine L3 band (pitfall 11).

**JSON contracts**
13. 2-space indent everywhere (`vp check` gate; publishPrompt step 4; `JSON.stringify(...,2)` / `json.dumps(indent=2)`) — any new emitter (vocab-map, integrations index, manifest tooling) must comply (pitfall 1).
14. `__DATE__` token: only the publish agent substitutes `date -u +%Y-%m-%d`; history dedupe keys on it; never agent-invented or local-time dates (pitfall 2).
15. Page double-cast `maturity as unknown as MaturityData` (index.tsx:16) hides JSON drift from `vp check`; the `Record<Level,…>` indexings are the forcing function — add CSS classes same-change (pitfall 9). Fix the already-drifted `HistorySnapshot` type while at it (pitfall 12).
16. Evidence-citation regex shapes (:250 — `.go|.yaml|.json` paths, `Test\w+`, `RED`, `added|removed|now …`) must be taught verbatim in every new axis prompt or legitimate moves get suppressed (pitfall 14).

**Test guardrails**
17. `maturity_test.go` cwd is `core/formats/` — out-of-dir probes (nativedocs, cli/parity) need `../../` paths + skip-if-absent for partial checkouts (pitfall 13).
18. Hard floors only where violators = 0; otherwise advisory `t.Logf` or explicit ledger (the file's own pattern, lines 3-8) — V/E/K/C all start with violators.
19. Corpus tests must keep skip-not-fail on missing fetched corpora (spectest/runner.go:65-69 precedent) — a release outage must never red CI.
20. Roundtrip assertion rollout vs the 21 pre-existing xliff2 byte-equal failures (#560): land as `expected_fail`/informational first.

**Provenance / legal / infra**
21. translate-toolkit is GPL-2.0 — harvesting its fixtures/tests into the Apache-2.0 framework is a license violation; manifest `license` field + a provenance policy gate any cross-framework leg (gap-review G3 blind spot).
22. Single-laptop Okapi clone is load-bearing until the corpus release mirrors it (G4.1) — sequence fetch-corpus before deepening dependence.
23. Confidentiality screening for harvested "real-world" files; sha256 in the manifest is also the tamper/erosion detector (G4 blind spots).

---

## 5. Ledger schema — final recommendation

Adopt gap-review's proposal (`gap-review.md` "Proposed minimal ledger schema") with these refinements. One committed file `docs/internals/format-ops-ledger.json` (committed so `git log` on it is itself a signal); **signals stay authoritative, ledger stores watermarks only**; due-ness is the pure function `due(ritual) = (today − last_run > cadence_days) OR any(current(signal) ≠ watermark)`.

Refinements over the draft:
1. `ledger_version: 1` kept; add top-level `"signals_doc": "gap-review.md S1–S14"` provenance pointer.
2. **`triage-score.watermarks` gains `audit_sha` (sha256 of `audit-format.py --all --json` output) and `axes_published: ["engine","vocabulary",…]`** — so the first multi-axis publish is detectable, and floor drift (S8) is a cheap hash compare instead of a JSON diff.
3. **`remediate.carryover` entries get a structured shape**: `{format, axis, gap, attempt_date, outcome: "test_passed=false|landed|skipped", evidence}` — this is where G1.2's evaporating reports stop evaporating, and it feeds the next run's plan ordering.
4. **`calibration.watermarks` carries `model_id`, `prompt_sha` (sha of format-triage.js), `rubric_sha` (sha of format-maturity.md ladder sections)** and triggers on any S12 change, not just cadence; `golden_set` stays the frozen adjudicated 6 (`html,json,mo,xcstrings,xliff2,properties`) and gains stored `adjudicated: {id→{axis→grade}}` so `golden_passed` measures correctness, not self-agreement (fixes G1.4).
5. **New ritual `vocab-drift`** `{cadence_days: 30, watermarks: {go_packs_sha, ts_packs_sha}}` — until the single-sourcing gate ships, the ledger is the drift alarm (vocabulary-reality §2 drift is live today).
6. **`corpus-census.watermarks.manifest_sha`** becomes per-format: `{<format>: <sha256 of manifest.yaml>}` plus `release_tag: "format-corpus-vN"` — a respun release or mutated manifest flags the ritual due.
7. **`okapi-sweep.watermarks`** keeps `last_issue_iid`/`last_issue_updated_at`/`latest_upstream_tag`/`pinned` and the `per_format_last_swept` map (S11 finally gets a stored counterpart); **`okapi-version-bump` split out as its own ritual** (cadence ∞, purely watermark-driven: due when `latest_upstream_tag ≠ pinned`) since its runbook spans 6+ files.
8. **`format-radar.decided.rejected`** entries are `{id: {reason, date, revisit_after?}}` — silent rejections stop being re-litigated (G7.1).
9. **`runs[]` append-only log**: keep `{date, ritual, commit, outcome, followups[]}`; add `model_id` and `duration_min?`. Each entry's `commit` lets a future run `git diff <commit>..HEAD -- core/formats` to enumerate exactly what was built since (the draft's strongest property — keep it).
10. **Bootstrap mechanically** from S1–S14 on the runbook's first run (triage-score.last_run = S2's `generated_at`; docs-snapshot-sync due immediately since S2 ≠ S6 already; parity-publish from S4; contract-audit from S5). `docs-snapshot-sync` keeps `cadence_days: 0` (purely watermark-driven).
11. Validation: a tiny schema check (2-space indent, known ritual ids, ISO dates) wired into an existing drift workflow (`reference-data-drift.yml` is the natural host) so the ledger itself can't rot unnoticed.

The three-way drift documented in gap-review §0 (code shipped scorer v2 → dashboard still v1 → docs snapshot names a dead workflow, all within 5 days) is the existence proof: every artifact in this system is a cache, and the ledger is the missing freshness contract.
