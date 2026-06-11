# Format-maturity multi-axis upgrade — engineering change surface

Scope: extend the single-ladder Engine L0–L4 maturity system with four new axes — **Vocabulary V0–V3, Editor E0–E3, Knowledge K0–K3, Corpus C0–C3** — across the triage workflow, the deterministic audit, the repro harness, the dashboard, and the Go guardrails. All paths relative to the worktree root `/Users/asgeirf/src/neokapi/neokapi/.claude/worktrees/format-process/`.

---

## 1. Current system — exact contracts

### 1.1 `.claude/workflows/format-triage.js` (434 lines) — the scorer pipeline

**Phases** (`meta.phases`, lines 5–11): Prep → Score → Triage → Remediate (opt) → Publish.

**Config** (lines 25–42): `TARGET` (default `'L4'`, a single L-string — line 26), `MODE`, `PUBLISH`, `LIMIT`, `SAMPLES` (ensemble size), `ANCHOR` (sticky, default true). `ALL_FORMATS` is a **hardcoded 49-id list** (lines 33–41) that must stay in sync with `audit-format.py all_formats()` (directory walk).

**Level/dimension machinery:**
- `ORDER`/`RANK`/`NEXT` (lines 110–112) — L-only lookup tables. `NEXT = { L0:'L1', …, L4:'—' }`.
- `CANON` (line 113) — the 9 dimension ids: `['reader','writer','config','spec','parity','malformed','corpus','detection','docs']`; `LABELS` (114–118) maps them to display strings.
- `normDim()` (121–133) — **substring** matcher mapping agent dimension names to canon ids (`n.includes('reader')` etc.). Unmatched → `null` → dropped.
- `enforceEvidence()` (137–149) — per-sample: any `complete|partial` cell with empty `evidence` is demoted to `'none'`; missing canon dims default to `'none'` (line 147 `for (const k of CANON) if (!(k in dims)) dims[k]='none'`).
- `modeDimensions()` (153–167) — per-dimension majority across N samples; ties break LOWER via `SVAL = {complete:3, partial:2, na:1, none:0}`.

**`dimsFromFloor(floor, type)`** (lines 174–189) — the deterministic floor → dimension grid:
```js
reader:   has.reader ? 'complete' : 'none',
writer:   has.writer ? 'complete' : (type === 'read-only' ? 'na' : 'none'),
config:   has.config ? 'complete' : 'none',
spec:     type === 'harvest' ? 'na' : (has.spec_yaml && k('spec') ? 'complete' : (has.spec_yaml ? 'partial' : 'none')),
parity:   type === 'harvest' ? 'na' : (has.parity_spec_test ? 'complete' : 'none'),
malformed: k('malformed') ? 'complete' : 'none',
corpus:   (k('corpus') || k('upstream')) ? 'complete' : (has.testdata ? 'partial' : 'none'),
detection: 'complete',   // ← HARDCODED, no floor signal
docs:      'complete',   // ← HARDCODED, no floor signal
```
Note: `floor.has.schema` and `floor.applymap_rejects_unknown` are **emitted by the audit but never consumed here** — they are free inputs for the Vocabulary axis.

**`reconcileDims()`** (195–208): floor wins everywhere except the `QUALITY = new Set(['writer','parity','corpus'])` dims, where the model may only **demote** (`DORDER` comparison line 205), and only when the floor value isn't `na`/`none`.

**`gateLevel(dims, type)`** (212–230) — the rubric as a pure function:
- L0 unless `reader+writer+config` all non-none (writer `partial` counts — "writable, just not byte-exact").
- L1 unless `malformed` full AND (`spec` full | harvest: `corpus` full).
- harvest: ceiling L3 via `has('corpus') && full('docs')`, else L2.
- parity: L2 unless `has(parity) && has(corpus) && full(docs)`; L4 iff `full(writer) && full(config) && full(parity) && full(corpus)`; else L3.

**`capByFloor(level, floor)`** (233–241): `has.reader===false → L0`; `has.writer===false && type!=='read-only' → min L1`; never above `floor.ceiling`.

**`applySticky(prior, derived, justification)`** (246–253): if `!ANCHOR || !prior` → publish derived (`derived_from:'dimensions'`, delta only when `derived !== prior && prior`). If prior ≠ derived, the move publishes only when the justification matches the citation regex (line 250):
```js
/[\w/.-]+\.(go|yaml|json)(:\d+)?|Test\w+|\bRED\b|added|removed|now (asserts|passes|fails)/
```
otherwise the prior stands with `derived_from:'sticky-prior'`, `delta.why:'SUPPRESSED: uncited move'`.

**`ftype()`** (255–261): floor.type wins; fallbacks `pdf→read-only`, `splicedlines→internal`, counterpart heuristic for harvest/parity.

**Prep phase** (323–340): one agent returns verbatim `audit_json` (stdout of `audit-format.py --all --json`) + `prior_json` (contents of `web/static/data/format-maturity.json`). Parsed here: `floorByFmt[f.format] = f`; `priorByFmt[f.id] = f.level` (line 336 — **reads ONLY `formats[].id` and `formats[].level` from the prior dataset**; this is the entire backward-compat surface for the prior file).

**Score phase** (343–394): per format × SAMPLES agents return the `SCORE` schema (lines 66–93): `dimension_scores` is an array of exactly-9 cells with `dimension` constrained by enum `['Reader','Writer','Config','Spec','Parity','Malformed','Corpus','Detection','Docs']` (line 80), `score ∈ {complete,partial,none,na}`, `evidence` required for complete/partial; plus `delta_justification`, `blocking_gaps`, `top_risk`, `confidence`, `okapi_counterpart`, `is_real_format`. Pipeline per format (lines 359–392): `enforceEvidence → reconcileDims → per-sample gateLevel+capByFloor → modeDimensions → harvest/read-only na patch (369–370) → derived = capByFloor(gateLevel(mode)) → agreement = modal fraction → lead sample → applySticky → row`.

**Row shape** (lines 385–392) — what `buildDataset` receives:
```js
{ id, type, level, next_level, okapi_counterpart, dimensions, evidence,
  floor: floor.base, ceiling: floor.ceiling, derived_from, delta,
  agreement, samples, blocking_gaps (≤3), top_risk, confidence }
```

**Triage phase** (397–406): distribution over L0–L4; `plan` = rows with `ORDER[level] < ORDER[TARGET]`, sorted ascending by level.

**Remediate** (409–419): one agent per plan entry, `remediatePrompt` (281–296) — single verified improvement, format-dir-only edits, `go test -race -tags fts5` must pass; returns `REMEDIATE` schema (95–108).

**Publish** (421–429) + **`buildDataset`** (310–320). The dataset JSON contract emitted (scorer v2):
```json
{
  "generated_at": "__DATE__",            // literal token, substituted by the publish agent
  "target_level": "L4",
  "source": "format-triage workflow (deterministic floor + evidence-cited dimensions + sticky anchor)",
  "scorer_version": 2,
  "run_integrity": { "samples": N, "anchored": bool,
                     "moves": {"published": n, "suppressed": n},
                     "low_agreement": [ids], "golden_passed": bool },
  "summary": { "total": n, "by_level": {"L0":n,"L1":n,"L2":n,"L3":n,"L4":n} },
  "dimensions": ["reader",...9 ids],
  "dimension_labels": { id: label },
  "formats": [ <row shape above, sorted by id> ]
}
```
`publishPrompt` (298–308): the agent (1) runs `date -u +%Y-%m-%d` = TODAY; (2) replaces the literal `__DATE__` and overwrites `web/static/data/format-maturity.json`; (3) updates `format-maturity-history.json`: **remove any entry whose `date` == TODAY, then append** `{"date": TODAY, "total": summary.total, "by_level": summary.by_level, "golden_passed": run_integrity.golden_passed, "moves": run_integrity.moves}`, sorted by date ascending; (4) **"Both files MUST be 2-space indented (the repo formatter, `vp check`, requires it)"** — `JSON.stringify(dataset, null, 2)` at line 427 satisfies this for the dataset.

**Return value** (432–434): `{ distribution, target, levels: {id→level}, agreement: {id→frac}, moves, plan, remediated, dataset }` — consumed for reproducibility measurement.

**Committed-file drift (important):** the live `web/static/data/format-maturity.json` is still **scorer v1** — `source: "format-triage workflow (agent assessment)"`, **no `scorer_version`, no `run_integrity`**, rows have only `{id,type,level,next_level,okapi_counterpart,dimensions,blocking_gaps,top_risk,confidence}` (no evidence/floor/ceiling/derived_from/delta/agreement/samples). Treat `scorer_version` missing as v1. The history file has a single entry `{date:"2026-05-31", total:49, by_level:{...}}` — **without** `golden_passed`/`moves` (those only appear in snapshots appended by a v2 run).

### 1.2 `.skills/refresh-format-maturity/scripts/audit-format.py` (280 lines)

JSON output schema — `audit_one()` (lines 173–200), one object per format (array under `--all --json`, `json.dumps(..., indent=2)` lines 225/243):
```json
{ "format": id,
  "type": "parity|harvest|read-only|internal",        // _ftype(), lines 165–170 (pdf, splicedlines special-cased)
  "okapi_counterpart": "okf_x | none (harvest cohort) | none found … | maybe: …",
  "has": { "reader","writer","config","schema","spec_yaml","transform",
           "testdata","parity_spec_test","annotations" },               // lines 184–194
  "applymap_rejects_unknown": "yes (…)|UNCLEAR -- read ApplyMap|no config.go",  // lines 71–82
  "test_kinds": ["reader","roundtrip","spec","malformed","corpus","upstream","schema",…],  // probes lines 54–62
  "coarse_level": "L1 (to L2 add: …)" /* human string */,
  "base": "L0..L3", "ceiling": "L0..L4" }
```
- `test_kinds()` (53–68): substring probes over `*_test.go` filenames; probe list includes `schema_test`, `config_test`, `invariants_test`, `acceptance_test`, `okapi_skip_test`, `fuzz`, etc. — strings are stripped of `_test` in output.
- `okapi_counterpart()` (85–101): `KNOWN_HARVEST` set (42–45) → `"none (harvest cohort)"`; `OKAPI_ALIAS` map (31–40); directory match against `$OKAPI_SRC/okapi/filters`; loose substring fallback for ids ≥4 chars.
- `coarse_level()` (104–140): L1 floor = `writer.go + config.go + (roundtrip|skeleton|snippets)`; L2 = harvest ? `okapi_skip+invariants+corpus+malformed` : `spec.yaml+spec_test+malformed`; L3 = `cli/parity/formats/<id>_spec_test.go` exists + `corpus|upstream`; returns prose like `"L2 (to L3 add: …)"`.
- `floor_ceiling()` (143–162) maps the prose prefix → `(base, ceiling)`: `L2+→(L2,L3)` harvest ceiling, `L3→(L3,L4)` parity judgment band, else pinned.
- `all_formats()` (203–213): dir walk of `core/formats/`, requires `reader.go`, excludes `NOT_A_FORMAT = {exec,jsx,memorytest}` (line 46).
- Human (non-JSON) mode also prints `bridge_config` presence and the GitLab tracker query (lines 246–275); `parity_spec_test` checked at `cli/parity/formats/<id>_spec_test.go` (line 177).

### 1.3 `.skills/refresh-format-maturity/scripts/repro-check.mjs` (107 lines) — the spread model

Hand-synced **mirror** of the workflow's four functions (`dimsFromFloor` 25–40, `gateLevel` 42–56, `capByFloor` 58–65 — comment line 15–17: "kept in sync by hand; … the workflow is the source of truth"). `realistic(floorVal)` (68–72): `complete→[complete,partial]`, `partial→[partial,none]`, else fixed. `levelSpread()` (74–84) enumerates the 2×2×2 cube over `QUALITY = ['writer','parity','corpus']` and collects distinct `capByFloor(gateLevel(...))` levels. Output: per-format `floor / level-range / PINNED|1-step|N-STEP!` plus totals (lines 95–106). Input: audit JSON on stdin (`--all --json`); consumes only `format,type,has,test_kinds,base,ceiling` — that subset is its stdin contract.

### 1.4 Dashboard — `web/src/pages/format-maturity/`

`_types.ts` (45 lines): `Level = "L0".."L4"`, `DimScore = complete|partial|none|na`, `FormatType = parity|harvest|read-only|internal`. `FormatRow` (9–19) = `{id,type,level,next_level,okapi_counterpart,dimensions:Record<string,DimScore>,blocking_gaps,top_risk,confidence}` — **does not type** the v2 fields (evidence/floor/ceiling/derived_from/delta/agreement/samples). `MaturityData` (21–29) = `{generated_at,target_level,source,summary:{total,by_level},dimensions:string[],dimension_labels,formats}` — **does not type** `scorer_version`/`run_integrity`. `HistorySnapshot` (31–35) = `{date,total,by_level}` — **does not type** the `golden_passed`/`moves` fields a v2 publish appends (latent drift to fix in this change). `LEVELS` array + `LEVEL_NAME` map (37–45).

`index.tsx` (260 lines) — every consumed field:
- `data = maturity as unknown as MaturityData` (line 16) — **double cast; TS will not flag dataset/type drift at the import**.
- `lvlClass: Record<Level,string>` (19–25) → CSS `.lvlL0….lvlL4` (`index.module.css:63–89`); `dotClass`/`dotTitle` per `DimScore` (27–39, css 195–211).
- `DistBar` (41–72): `data.summary.{by_level,total}`, segment width = `n/total`, legend per `LEVELS`.
- `Trend` (74–105): iterates `hist[]`; bar column per snapshot keyed `h.date`; cell height = `(h.by_level[lv] ?? 0) / maxTotal` where `maxTotal = max(h.total)`; label `h.date.slice(5)`. **Consumes only `date,total,by_level`** — extra snapshot fields are ignored (additive-safe).
- Filters (108–122): `level` state typed `Level|null`, matched by `f.level === level`; `type` chips from `Set(formats.map(f=>f.type))`; search = `f.id.includes(q)` (id substring only). Sort: `a.level.localeCompare(b.level) || a.id.localeCompare(b.id)` (line 121) — lexicographic, valid only within one grade alphabet.
- Header (130–145): `data.target_level` (indexed into `lvlClass` — typed `Level`), `data.summary.total`, `data.generated_at`, `data.source`.
- Table head (180–189): columns = Format, Level, one per `data.dimensions` labeled via `data.dimension_labels[d] ?? d`, Next gap.
- `RowGroup` (209–260): `f.id`, `f.type`, `f.okapi_counterpart`, `f.level` badge, dot per `data.dimensions` from `f.dimensions[d] ?? "none"`, `f.blocking_gaps[0]` gap cell; expanded detail uses `f.next_level`, full `blocking_gaps`, `f.top_risk`. `colSpan = 3 + data.dimensions.length` (line 210).
- Nav links only: `web/sidebars.ts:157`, `web/docusaurus.config.ts:231`.

### 1.5 `core/formats/maturity_test.go` (127 lines) — guardrails

Package `formats_test`, cwd = `core/formats/` (reads `"."` — line 46). Exemptions: `nonFormats = {exec,jsx,memorytest}` (18–22); `grandfatheredRoundtrip` ledger (30–39, 8 formats; "NEW formats MUST NOT be added here").
1. `TestFormatSpecIsGated` (71–82) — **hard floor**: every `spec.yaml` must have a sibling `spec_test.go`.
2. `TestRoundTripTestNamingConvention` (88–110) — **hard floor with ledger**: every format with `writer.go` must have `roundtrip_test.go` or `skeleton_test.go` (or be grandfathered); also `t.Logf` nags when a grandfathered format gains a conventional test (97–99).
3. `TestRobustnessCoverage` (116–127) — **advisory** (`t.Logf` only): counts formats lacking `malformed_test.go`.

### 1.6 Rubric source of truth

`docs/internals/format-maturity.md` — level table (lines 72–78), 9-dimension rubric table (91–101), "scorer v2" reproducibility section (39–64, documents floor + 3 quality dims + sticky anchor + repro-check invocation), file-signal quick reference (132–137), generated gap-analysis block (`<!-- BEGIN/END: gap-analysis report -->`, 139–256), open questions (258–286). Companion: `.skills/refresh-format-maturity/SKILL.md` ("score the 9 rubric dimensions", line 48–49), `.skills/implement-format/SKILL.md` (lines 18, 147, 171), `web/docs/contribute/notes-internal/implementing-formats.md:19`.

---

## 2. Punch list — adding Vocabulary V0–V3, Editor E0–E3, Knowledge K0–K3, Corpus C0–C3 alongside Engine L0–L4

### 2.1 Axis model (shared vocabulary, define once)

Introduce an axis table used by workflow + repro-check + types:
```js
AXES = {
  engine:     { grades: ['L0','L1','L2','L3','L4'] },
  vocabulary: { grades: ['V0','V1','V2','V3'] },
  editor:     { grades: ['E0','E1','E2','E3'] },
  knowledge:  { grades: ['K0','K1','K2','K3'] },
  corpus:     { grades: ['C0','C1','C2','C3'] },
}
```
Generalize `RANK/ORDER/NEXT` (format-triage.js:110–112) to per-axis lookups (`rankOf(axis, grade)`, `nextOf(axis, grade)`). Partition `CANON`/`LABELS` (113–118) into per-axis dimension lists; existing 9 dims map: engine ← reader/writer/spec/parity/malformed/detection; vocabulary ← config (+ new `schema`, `speckeys` dims); knowledge ← docs (+ new `refs`, `sidecar`, `annotations` dims); corpus ← corpus (+ new `fixtures`, `upstream` dims); editor ← all-new (`preview`, `previewtest`).

### 2.2 `audit-format.py` — new deterministic floor signals (per axis, with source files)

Extend `audit_one()` (lines 173–200) additively — keep every existing key (`format/type/has/test_kinds/coarse_level/base/ceiling` are the repro-check stdin contract and the workflow floor contract):

- **Vocabulary V0–V3** (signals already half-collected):
  - `has.config` + `applymap_rejects_unknown` (lines 71–82, from `core/formats/<id>/config.go`) → V0/V1 floor.
  - `has.schema` (line 188, `core/formats/<id>/schema.go`) → V2 floor. **Currently emitted but unused by `dimsFromFloor` — this axis is where it finally gets consumed.**
  - `schema` in `test_kinds` (probe `schema_test`, line 61; e.g. `core/formats/json/schema_test.go`) → V3 floor.
  - NEW: parse `core/formats/<id>/spec.yaml` for `okapi_param:` presence and key list (regex; keys-1:1-with-ApplyMap is rubric dim 4) → V2/V3 quality signal.
- **Editor E0–E3**:
  - E0 baseline is universal: the generic fallback `core/editor/preview_generic.go` (`buildGenericPreview`, line 12) covers every format via `editor.BuildPreview` (`core/editor/parse.go:67`, `PreviewHTML` field line 26/74).
  - NEW `has.preview`: `preview.go` implementing `format.PreviewBuilder` in the format package — today only `core/formats/html/preview.go:10`, `markdown/preview.go:10`, `mdx/preview.go:10` (`var _ format.PreviewBuilder = (*Reader)(nil)`), plus `core/formats/jsx/jsx.go:527–537`. Signal: `has_file(d,'preview.go')` or grep for `format.PreviewBuilder`.
  - NEW test-kind probe `preview_test` (add to the probes list, audit-format.py:54–62) → E2; anatomy/block-index coverage (`core/editor/anatomy.go`, `anatomy_test.go`) per format → E3.
- **Knowledge K0–K3**:
  - NEW `has.nativedocs`: sidecar `scripts/gen-refs/nativedocs/formats/<id>.yaml` exists and differs from `_TEMPLATE.yaml` (dir verified: androidxml.yaml…; rubric dim 9 "sidecar filename == id exactly").
  - NEW `refs` census: count `spec_refs:`/`okapi_refs:`/`native_refs:` occurrences in `core/formats/<id>/spec.yaml` (e.g. `json/spec.yaml:86,94,96`) → K1/K2.
  - `has.annotations` (line 193, `parity-annotations.yaml`) + NEW attributed-reason check (every `expected_fail` has `severity`/`skip.reason` — fields named in scorePrompt line 275) → K2/K3.
  - Reference-data wiring (`make generate-reference-docs` output; format present in `@neokapi/reference-data`) → K3.
- **Corpus C0–C3** (promotes the existing 1 dim to a ladder):
  - `has.testdata` (line 191) → C1.
  - NEW `testdata_census`: file count + extension census via `os.walk` (distinguishes empty/synthetic-ish) → C1/C2.
  - `corpus`/`upstream` test kinds (probes line 60) → C2.
  - NEW `has.okapi_fixture_input`: regex `input_file:\s*okapi:` in `spec.yaml` (rubric dim 7 wording) → C3 floor signal.
  - NEW `has.parity_fixtures`: `cli/parity/formats/fixtures_<id>_generated.go` exists (verified: dtd/html/json/markdown/po/properties/regex/tmx/ts/wiki/xliff/yaml) → C2/C3.
  - Real-vs-synthetic stays the one **model-judged** corpus quality dim (existing `QUALITY` member).
- Replace single `base`/`ceiling` with an additive `axes: { engine:{base,ceiling}, vocabulary:{base,ceiling}, … }` block; **keep** the top-level `base`/`ceiling`/`coarse_level` (engine values) for the old consumers. Per-axis ceilings differ for harvest: engine ceiling stays L3 (`floor_ceiling` line 157), but a harvest format can reach V3/E3/K3/C3.
- Extend the human-mode print block (253–267) with the new signals. Keep `json.dumps(..., indent=2)`.

### 2.3 `format-triage.js` — function-by-function

| Location | Change |
|---|---|
| `meta` 1–12 | description/whenToUse mention axes; `target` arg becomes per-axis object or keeps `target` for engine + `targets:{vocabulary:'V2',…}`. |
| `TARGET` 26, `ORDER/RANK/NEXT` 110–112 | per-axis tables (2.1). |
| `CANON/LABELS` 113–118 | per-axis dim lists + labels for new dims (`schema`, `speckeys`, `preview`, `previewtest`, `refs`, `sidecar`, `fixtures`, `upstream`). |
| `normDim` 121–133 | **switch to exact-match on canonical ids** — the substring matching is a collision trap for new names (e.g. any dim containing "spec"/"corpus"/"doc" silently aliases; "preview"/"reader" are safe but "schema"-vs-"spec" via `includes('spec')` is not — note `'spec' in 'speckeys'`). |
| `enforceEvidence` 137–149 | iterate the union of all axis dims; missing→`none` per axis (same rule). |
| `modeDimensions` 153–167 | iterate per-axis canon; unchanged logic. |
| `dimsFromFloor` 174–189 | add the new floor rows per 2.2. **Replace the hardcoded `detection:'complete'`/`docs:'complete'` (186–187)** for any dim moving onto a judged axis — Knowledge is vacuous if `docs` stays a constant. |
| `QUALITY` 195 | becomes per-axis sets, e.g. engine:{writer,parity}, corpus:{corpus}, vocabulary:{speckeys}, knowledge:{refs/annotations hygiene}, editor:{} (fully file-pinned). Update `reconcileDims` to consult the right set. |
| `gateLevel` 212–230 | split into five pure gates: `gateEngine` (existing body), `gateVocab`, `gateEditor`, `gateKnowledge`, `gateCorpus` — each a function of its own dims, returning its own grade alphabet. |
| `capByFloor` 233–241 | per-axis: clamp to `floor.axes[axis].ceiling`; engine keeps the reader/writer hard caps. |
| `applySticky` 246–253 | called once per axis with `priorByFmt[f][axis]`. The `!prior` early-return (247) already publishes derived without a delta when prior is undefined — that IS the new-axis migration path; do not count first-time axis grades as moves (the `derived !== prior && prior ? … : null` guard already ensures `delta:null`). |
| `ftype` 255–261 | unchanged; but the harvest/read-only `na` patches (369–370) become per-axis. |
| `SCORE` schema 66–93 | extend the `dimension` enum (80) and the "EXACTLY these 9 dimensions" text (75); `delta_justification` becomes per-axis (e.g. object keyed by axis, or array of `{axis, justification}`) so one cited engine move can't smuggle four axis moves past the regex gate (250). |
| `scorePrompt` 263–278 | list per-axis dims; show ALL prior axis grades (`PRIOR PUBLISHED LEVEL` line 269 → per-axis block; absent axes print "(none — first scoring)"); judgment instructions for the new quality dims with the same cite-or-drop rule. |
| Prep parser 336 | `priorByFmt[f.id] = f.levels ?? { engine: f.level }` — **migration shim: the committed v1/v2 dataset has only `level`, which seeds the engine anchor; the four new axes start unanchored.** |
| Score loop 359–392 | compute `perSampleLevel`/`agreement`/`derived`/`sticky` per axis; row gains `levels:{engine,vocabulary,editor,knowledge,corpus}`, `next:{axis→grade}`, `derived_from`/`delta`/`agreement` per axis — **keep `level` + `next_level` mirroring the engine axis** so the prior-parser of the next run and the un-migrated page both still work. |
| Triage 397–406 | per-axis distributions; plan entries gain the axis whose gap is ranked; below-target test uses per-axis targets. |
| `buildDataset` 310–320 | bump `scorer_version: 2 → 3`; add `axes` (id→grades list), `axis_labels`, `summary.by_axis: {axis → {grade→count}}` (zero-init every grade for stable JSON diffs, like `by_level` at 312); add `dimension_axes: {dim→axis}`; keep `summary.by_level` = engine distribution, `dimensions`/`dimension_labels` as the full ordered union. |
| `publishPrompt` 298–308 | step 3 history append gains `"by_axis": <summary.by_axis>` **added to the new snapshot only** — explicitly forbid touching prior entries (the remove-TODAY-then-append+sort logic at 306 already preserves them); keep step 4's 2-space-indent + valid-JSON checks verbatim. |
| run_integrity 422–423 | `moves` and `low_agreement` per axis (e.g. `moves:{engine:{published,suppressed},…}`); `golden_passed` = all axes all formats. |
| return 432–434 | `levels` → per-axis maps (or `levels:{id→{axis→grade}}`); keep flat engine `levels` for any caller of the workflow result. |

### 2.4 `repro-check.mjs`

Mirror every workflow change by hand (its stated contract, lines 15–17): per-axis `dimsFromFloor`/`gate*`/`capByFloor`/`QUALITY`; `levelSpread` enumerates the quality cube **per axis** and reports a spread per axis (engine spread target stays 0/1; editor should be spread-0 by construction since it has no quality dims). Table output gains axis columns; totals per axis. Update the header comment naming the three quality dims (lines 4–12).

### 2.5 Dashboard

`web/src/pages/format-maturity/_types.ts`:
- Add `AxisId`, per-axis grade unions (`VGrade = "V0"|…|"V3"` etc.), `AXIS_GRADES: Record<AxisId, string[]>`, `GRADE_NAME` per axis (mirror of `LEVELS`/`LEVEL_NAME` 37–45).
- `FormatRow`: add **optional** `levels?: Partial<Record<AxisId,string>>`, plus the already-emitted-but-untyped v2 fields (`evidence?`, `floor?`, `ceiling?`, `derived_from?`, `delta?`, `agreement?`, `samples?`). Keep `level`/`next_level` required.
- `MaturityData`: add optional `scorer_version?`, `run_integrity?`, `axes?`, `axis_labels?`, `dimension_axes?`, `summary.by_axis?`.
- `HistorySnapshot`: add optional `by_axis?`, and fix the existing drift — `golden_passed?`, `moves?` (publishPrompt already appends them; the type never knew).

`web/src/pages/format-maturity/index.tsx`:
- `lvlClass` (19–25) is `Record<Level,string>` — extend to a grade→class map covering V/E/K/C grades; **new CSS classes** in `index.module.css` next to `.lvlL0–.lvlL4` (lines 63–89). Typecheck will force this once `target_level`/badges accept new grades — but remember the `as unknown as MaturityData` cast (line 16) means the JSON itself is never checked; only usage sites are.
- `DistBar` (41–72): keep the engine bar; add per-axis mini-bars driven by `data.summary.by_axis` (guard `?? {}` for old datasets).
- `Trend` (74–105): must keep rendering the existing snapshot (`{date,total,by_level}` only). Add axis trends **conditionally** (`h.by_axis &&`); never assume the field. Old entries simply contribute nothing to axis lanes.
- Filters (108–122): the level-chip state is engine-only today (`f.level === level`); add an axis selector or per-axis chips matching `f.levels?.[axis] ?? (axis==='engine' ? f.level : undefined)`. Search (`f.id.includes(q)`) and type chips unchanged.
- Sort (121): `localeCompare` is only meaningful within one alphabet — sort by the **selected axis's** grade, falling back to engine.
- Table: either one badge column per axis (update `colSpan = 3 + data.dimensions.length` at line 210 → `3 + axesShown + dims`) or a compact multi-badge Level cell; dimension dot columns can be grouped by `dimension_axes`.

### 2.6 `core/formats/maturity_test.go` — new guardrails

Follow the existing pattern: hard floors only where there are zero violators; otherwise advisory `t.Logf` burndowns (like `TestRobustnessCoverage` 116–127) or an explicit grandfather ledger (like `grandfatheredRoundtrip` 30–39):
- **Vocabulary**: `TestSchemaIsGated` — every `schema.go` should have `schema_test.go` (advisory; the rubric says only html/json have it → ledger or Logf), and a hard floor that `config.go` exists wherever `schema.go` does.
- **Editor**: advisory `TestPreviewCoverage` — list formats lacking `preview.go` (E-axis burndown; only html/markdown/mdx today).
- **Knowledge**: advisory — formats with `spec.yaml` lacking `spec_refs`, and formats missing a `scripts/gen-refs/nativedocs/formats/<id>.yaml` sidecar (note: this test runs with cwd `core/formats/`, so the sidecar path is `../../scripts/gen-refs/nativedocs/formats/` — keep it `filepath.Join("..","..",…)` and skip if the dir is absent to stay hermetic).
- **Corpus**: advisory — formats whose `testdata/` is missing or empty.
These give each axis the same "mechanically checkable floor" property the engine ladder has (file header comment, lines 3–8).

### 2.7 Docs / skills

- `docs/internals/format-maturity.md`: add four ladder tables (V0–V3/E0–E3/K0–K3/C0–C3 entry criteria); update the level table intro (66–70: "a format sits at exactly one level" → one grade **per axis**); rewrite "scorer v2" §(39–64) as v3 incl. per-axis quality dims + repro invocation; tag each rubric row (91–101) with its axis; regenerate the gap-analysis block between the BEGIN/END markers (139–256).
- `.skills/refresh-format-maturity/SKILL.md`: step 2 "score the 9 rubric dimensions" (48) and step 6 report (111–116) gain axes; step 1 mentions the new audit fields.
- `.skills/implement-format/SKILL.md` (18, 147, 171) + `web/docs/contribute/notes-internal/implementing-formats.md:19`: rewording only.
- Nav (`web/sidebars.ts:157`, `web/docusaurus.config.ts:231`): no change.

### 2.8 History & trend extension — without losing existing history

1. **Never rewrite old entries.** `format-maturity-history.json` currently holds one snapshot `{date:"2026-05-31",total:49,by_level:{…}}`. The publish step's only mutation of existing data is remove-same-date-then-append (format-triage.js:306) — keep exactly that.
2. New snapshots are a **superset**: `{date,total,by_level,(golden_passed),(moves),by_axis:{vocabulary:{V0..V3},editor:{…},knowledge:{…},corpus:{…}}}`. `by_level` stays the engine distribution so `Trend`'s `h.by_level[lv]` keeps working over the whole array.
3. The page guards every new field (`h.by_axis?.[axis]?.[grade] ?? 0`); axis trend lanes simply start at the first multi-axis snapshot. No backfill — the data doesn't exist.
4. `HistorySnapshot` type gains only optional fields (2.5).

### 2.9 What stays backward-compatible (explicit list)

- Dataset top-level keys `generated_at,target_level,source,summary.{total,by_level},dimensions,dimension_labels,formats` and row keys `id,type,level,next_level,okapi_counterpart,dimensions,blocking_gaps,top_risk,confidence` — everything the page consumes today — unchanged in shape; all multi-axis data is additive.
- The Prep prior-parser contract (`formats[].id` + `formats[].level`, line 336) — preserved because rows keep mirroring engine into `level`; old datasets (v1, no `levels`) seed only the engine anchor.
- `repro-check.mjs` stdin contract: audit JSON keeps `format,type,has,test_kinds,base,ceiling` (engine) top-level; `axes{}` is additive.
- History array element shape for existing entries; Trend renders pre-axis snapshots forever.
- `audit-format.py` single-format human output and `--all` (non-json) one-line-per-format output keep their first columns.
- `maturity_test.go` existing three tests and both ledgers untouched.

---

## 3. Pitfalls (named in the task + found while reading)

1. **2-space JSON indent** — publishPrompt step 4 (format-triage.js:307): *"Both files MUST be 2-space indented (the repo formatter, `vp check`, requires it)."* `JSON.stringify(dataset, null, 2)` (line 427) and `json.dumps(..., indent=2)` (audit-format.py:225/243) already comply; any new emitter (per-axis files, scripts) must too, or `vp check` fails on the next frontend gate.
2. **`__DATE__` mechanism** — `buildDataset` hardcodes `generated_at:'__DATE__'` (line 315); the **publish agent** substitutes the output of `date -u +%Y-%m-%d` (publishPrompt steps 1–2, lines 301–302) and the history dedupe keys on that same TODAY (line 306). If axes add any timestamps, route them through the single `__DATE__` token — never let an agent invent a date, and never use local time (the `-u` matters for the dedupe key).
3. **`scorer_version` bump** — `buildDataset` line 317 says `2`; bump to `3` for multi-axis. Note the **committed** dataset has *no* `scorer_version` at all (it predates v2: `source:"format-triage workflow (agent assessment)"`, no `run_integrity`) — treat missing as v1 in any reader, and don't gate the Prep parser on version (it must read v1/v2/v3 priors alike).
4. **Sticky-anchor migration for new axes** — `applySticky`'s `!prior` branch (line 247) publishes the derived grade with `delta:null` when no prior exists; that is the correct first-run behavior for V/E/K/C. Two traps: (a) don't synthesize a prior for new axes from the engine level — they'd all be "moves" and get SUPPRESSED by the citation regex (line 250), freezing every axis at a bogus grade; (b) make `delta_justification` per-axis, otherwise one format's single justification string gates five independent moves through one regex.
5. **`normDim` substring matching** (121–133) — `includes('spec')`/`includes('corpus')`/`includes('docs')` will mis-bucket new dimension names (e.g. `speckeys`→spec, a per-axis `corpus_fixtures`→corpus). Move to exact ids, and update the `SCORE` enum (line 80) in lockstep — `enforceEvidence` silently defaults any unmatched/missing dim to `'none'` (line 147), which would zero a whole axis without an error.
6. **Hardcoded `detection:'complete'` / `docs:'complete'`** (dimsFromFloor 186–187, mirrored in repro-check 37–38) — if `docs` seeds the Knowledge axis it needs a real signal (nativedocs sidecar, refs census) or K is vacuously maxed.
7. **`repro-check.mjs` is a hand-synced mirror** (its lines 14–17) — every gate/floor change must land in both files in the same commit, or the published "variance bound" lies. Consider generating both from one module later; for now the punch list pairs each workflow edit with a repro-check edit.
8. **`ALL_FORMATS` (workflow 33–41) vs `all_formats()` (audit 203–213)** — two sources of the format universe; a format added to `core/formats/` appears in the audit floor but won't be scored until the hardcoded list is updated.
9. **Page double-cast** — `maturity as unknown as MaturityData` (index.tsx:16) means JSON/type drift is invisible to `vp check`; only usage sites typecheck. The `lvlClass`/`target_level` `Record<Level,…>` indexings are the few places that will actually error when grades widen — use them as the forcing function, and add the new CSS classes (`index.module.css:63–89` block) in the same change.
10. **Sort/filter assumptions** — `a.level.localeCompare(b.level)` (index.tsx:121) and the `level` chip equality are engine-only; cross-axis comparisons of grade strings are meaningless (V2 vs L2). Keep filters axis-scoped.
11. **Harvest ceilings diverge per axis** — engine harvest ceiling is L3 (`floor_ceiling` audit:157; `gateLevel` workflow:221–224), but harvest formats can top out V3/E3/K3/C3; per-axis `ceiling` in the audit's new `axes{}` block must not inherit the engine band.
12. **History snapshot fields already drifted once** — publishPrompt appends `golden_passed`/`moves` that `HistorySnapshot` never typed; fix while adding `by_axis` so the type matches what's actually on disk going forward.
13. **`maturity_test.go` runs with cwd `core/formats/`** (reads `"."`, line 46) — any new guardrail reaching outside (nativedocs sidecars, `cli/parity/formats/`) needs `../../`-relative paths and a skip-if-absent guard to stay green in partial checkouts.
14. **Evidence-gate strings** — the citation regex (line 250) accepts `.go|.yaml|.json` paths, `Test\w+`, `RED`, `added|removed|now (asserts|passes|fails)`; axis prompts must instruct citations in exactly these shapes or every legitimate axis move gets suppressed.
