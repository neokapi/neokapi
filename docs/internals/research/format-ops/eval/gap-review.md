# Adversarial gap review — neokapi format-ops framework (worktree `format-process`)

Reviewed against the maintainer's 7 goals. All paths relative to
`/Users/asgeirf/src/neokapi/neokapi/.claude/worktrees/format-process/` unless absolute.

## 0. Inventory — what exists today

| Asset | Path | State |
|---|---|---|
| Maturity rubric L0–L4, 9 dimensions, audit procedure, scorer-v2 description, open questions | `docs/internals/format-maturity.md` | Snapshot block (`<!-- BEGIN: gap-analysis report (generated) -->`, line 139–256) dated **2026-05-30**, now stale |
| Engine knowledge base (architecture, spec.yaml grammar, parity harness, Okapi mapping, principles, wiring checklist) | `docs/internals/format-engineering.md` | Current, dense, good |
| Build-a-format skill | `.skills/implement-format/SKILL.md` | Good; mandates malformed_test for new formats |
| Per-format audit skill (+ Okapi GitLab sweep) | `.skills/refresh-format-maturity/SKILL.md` + `scripts/audit-format.py` (280 LOC, deterministic floor) + `scripts/repro-check.mjs` (variance bound) | Good |
| All-formats triage workflow (Prep→Score→Triage→Remediate→Publish) | `.claude/workflows/format-triage.js` (435 LOC) | Scorer v2: deterministic floor (`dimsFromFloor` :174), evidence-gating (`enforceEvidence` :137), demote-only quality dims (`reconcileDims` :197), pure-function level (`gateLevel` :211), sticky anchor (`applySticky` :247) |
| Structural guardrail tests | `core/formats/maturity_test.go` — `TestFormatSpecIsGated` :71, `TestRoundTripTestNamingConvention` :88 (+ `grandfatheredRoundtrip` ledger :30), `TestRobustnessCoverage` :116 (advisory only) | Live |
| Dashboards (pages) | `web/src/pages/{format-maturity,parity,contract-audit,test-comparison,formats,...}` ; format-maturity renders trend from history (`index.tsx:4` imports `format-maturity-history.json`) | Live |
| Dashboard data (committed caches) | `web/static/data/format-maturity.json` (generated_at **2026-05-31**), `format-maturity-history.json` (**1 entry**), `parity-report.json` (**2026-05-20T15:34:08Z**), `contract-audit.json` (**2026-05-21**, okapiTag v1.48.0, goCommitSHA a98fe30f) | All stale vs HEAD |
| CI | `.github/workflows/parity.yml` (push to main + PR + dispatch), `contract-audit.yml` (**cron weekly Mon 06:00 UTC**, comment: "catches new Okapi tests within ~7 days of an upstream tag"), `format-acceptance.yml`, `nightly.yml` (daily 03:00), `reference-data-drift.yml` | No workflow writes `web/static/data/*.json` back — dashboards are manual-publish caches |
| Fixture harvest | `scripts/okapi-test-scan/main.go`, `make regen-okapi-fixtures` (Makefile :561, manual), `spec.yaml input_file: okapi:` indirection (15 of 41 spec.yaml) | Okapi clone hard-pinned: `OKAPI_VERSION ?= 1.48.0` (Makefile :1004), absolute path `/Users/asgeirf/src/okapi/Okapi` baked into docs/skills/workflow (`format-triage.js:48`) |

Ground truth drifted under the docs since the snapshots: commit `b7201a9f5`
(**2026-06-04**, "test(formats): malformed-input robustness across 38 formats; fix 4
readers") means **42/49** formats now have `malformed_test.go` (missing only:
androidxml, applestrings, designtokens, html, i18next, mdx, mo). Yet:

- `docs/internals/format-maturity.md:179` still says "only arb/resx/xcstrings have one today";
- the committed dashboard (2026-05-31) still ranks "add malformed_test.go" as the top gap for ~24 formats and shows L1:36;
- the committed dashboard is **pre-scorer-v2**: `format-maturity.json:4` reads `"source": "format-triage workflow (agent assessment)"` with **no** `scorer_version`/`run_integrity` fields, while `format-triage.js:317` now emits `scorer_version: 2` and `publishPrompt` (:306) appends history entries with `golden_passed`/`moves` keys the committed history lacks. **Scorer v2 shipped (f042a3a2f) but has never published a run.**

This live three-way drift (code ↔ dashboard ↔ docs snapshot) is itself the
strongest evidence for the runbook: every artifact is a cache with no freshness
contract.

---

## Goal-by-goal review

### G1 — Self-healing / self-improving systems (processes that write prompts and adapt)

**Exists.** `format-triage.js` is genuinely state-of-the-art for one loop:
deterministic floor the model can't override, evidence-or-it-didn't-happen
demotion, ensemble + conservative tie-break (`modeDimensions` :153), sticky
anchor so re-runs reproduce, `repro-check.mjs` as a no-LLM variance bound, and a
remediate mode that does exactly one verified improvement per format
(`remediatePrompt` :281 — "It MUST compile and pass… report with
test_passed=false rather than papering over it").

**Structurally missing.**
1. **No loop closes back onto the prompts/rubric.** `scorePrompt`/`remediatePrompt`/`publishPrompt` are hardcoded strings inside the JS; nothing evaluates prompt performance across runs, no prompt changelog, no A/B against a golden set. "Processes that write prompts" does not exist anywhere.
2. **Run outcomes are not persisted.** The workflow's return value (plan, remediated[], agreement, suppressed moves) evaporates; `test_passed=false` remediation reports and `low_agreement` formats are logged, not stored. A future run cannot ask "what did the last remediation fail on?"
3. **Docs snapshot is hand-pasted.** The `BEGIN/END gap-analysis` block in format-maturity.md has generated markers but no generator wired to the dashboard — so it rots independently (it already has).
4. **The sticky anchor masks scorer drift by design.** `applySticky` (:247) suppresses uncited moves — excellent for stability, but a *systematically* different new model generation will have all its level moves suppressed and you'll never see the drift. There is no periodic **anchor-free calibration run** (`anchor:false`) against a frozen golden set of formats with adjudicated known levels; `golden_passed` (:422) currently measures only self-agreement, not correctness.
5. **Self-gaming channel in remediation.** The floor scores `malformed: k('malformed') ? 'complete' : 'none'` (:185) — *file presence*. The model's quality judgment is restricted to writer/parity/corpus (`QUALITY` set :195); **Malformed is never quality-checked**. So the remediate agent adds a file → the floor auto-promotes → the dashboard improves, with no independent check that the new test would actually fail on a broken reader. An AI improving the metric it is scored by, with the quality gate removed for exactly the artifact it most often adds.

**Blind spots implied but unlisted:** mutation-style spot checks of AI-added tests (break the reader, assert the new test goes red); recording **model id + scorer_version + prompt hash** per history snapshot; regression protection when remediation touches shared behavior — remediate agents run only `go test ./core/formats/<fmt>/` (:293), never `make test`, and "review the diff before committing" (format-maturity.md:36) is the only human gate.

### G2 — Workflows/maturity/dashboard evaluation; vocabulary maturity; native-editor embedding

**Exists.** The 9-dimension grid + trend chart + per-dimension evidence is a solid skeleton; `run_integrity` adds audit metadata.

**Structurally missing.**
1. **Visibility over time is nominal.** `format-maturity-history.json` has exactly one datapoint (2026-05-31). `parity-report.json` and `contract-audit.json` carry `generated_at` but no history array at all. No CI job re-publishes any of them (grep over `.github/workflows/*.yml` finds no writer of these files), and the parity native round-trip tier "never fails CI unless a per-format MinTier is set" (format-engineering.md:213) — so the only longitudinal signal is whatever a human remembers to run.
2. **Vocabulary maturity is not a dimension.** The "internal common representation of style/formatting" exists only informally: `core/model/run.go:70` — `Type string` (free-form). The convention `"fmt:bold"` appears **only in tests/bench** (`core/model/run_semantic_html_test.go:23,84`, `core/model/bench_test.go:54`); `core/model/run_semantic_html.go` does HTML semantics only. There is no canonical cross-format vocabulary (bold/italic/link/break/note/context as enumerated Run.Type/SubType values), no doc defining it, and **no rubric dimension scoring whether a format maps `<b>` / `<w:b>` / `**…**` to the same canonical type**. Without it, cross-format tooling (TM leverage across formats, brand checks, editors) sees opaque per-format type strings. This needs a 10th dimension ("Vocabulary: canonical Run.Type/SubType coverage + a conformance test") plus a vocabulary spec doc the way Okapi has Code.type conventions.
3. **Native-editor embedding is absent from the model.** Surfaces exist in the org (kapi-desktop; `kapi mcp`; kgrep/ksed toolbox; WASM lab/playground; Office/Google add-ins on the unmerged PR #776 branch — `bowrain/addin` is **not** in this tree), but the maturity model has no "editor story" axis: per format, *where does the author live* (Word→openxml, Xcode→xcstrings, Figma→connector, VS Code→json/yaml/po) and what embed surface reaches them. Nothing links format ↔ authoring tool ↔ embed channel; L4 can be reached with zero in-editor presence.

**Blind spots:** the dashboard measures artifact presence, not user-observable quality (no published corpus round-trip success-rate, no perf trend — bench exists only for html/json/openxml/wiki); WASM/browser viability of each format (cgo-less: no ICU/native SQLite) is invisible — a format can be L3 native and broken in the lab; no SLO gate ("an L3+ format turning red blocks merge").

### G3 — Spec harvesting, learning, cross-framework research, example-based specs → generated tests

**Exists.** `spec.yaml` is a real executable spec (3 consumers: native runner, parity runner, contract-audit — format-engineering.md §3); `okapi_refs`/`native_refs`/`spec_refs` give three-way grounding; `scripts/okapi-test-scan` harvests Okapi inline fixtures; the maturity_test gate `TestFormatSpecIsGated` stops spec rot.

**Structurally missing.**
1. **"Other frameworks" = exactly one (Okapi).** No model, workflow, or even doc section for translate-toolkit, ICU/MessageFormat 2, Fluent, Pandoc/LibreOffice import filters, Crowdin/Lokalise format matrices, gettext upstream. The okapi-expert skill and GitLab sweep are Okapi-only.
2. **`spec_refs` are unversioned free strings.** No spec edition/date pinned, no link-rot check, no "which clause" anchor — so "the spec wins ties" (format-engineering.md:309) is unenforceable mechanically.
3. **No spec-document → examples generation loop.** Examples are hand-authored; the goal's "example-based specs that AI systems can generate tests from" currently runs spec.yaml→assertions only. There is no workflow that reads an RFC/W3C section (or an Okapi test class) and proposes new spec.yaml examples with citations, nor coverage accounting ("which features have <2 examples / no real-file example").
4. **Assertion vocabulary capped at block text** (format-engineering.md:133 — "no round-trip / writer-output assertion type"; open question #4 in format-maturity.md). Specs cannot express the thing the engine most prizes (byte fidelity).
5. **No per-format learning dossier.** Knowledge is scattered across spec.yaml descriptions, parity-annotations, test comments; nothing aggregates "what we know about format X" into a regenerable artifact a new model can be pointed at.

**Blind spots:** licensing of harvested learning material — Okapi is Apache-2.0 (compatible), but translate-toolkit is **GPL-2.0**; harvesting its fixtures/tests into this Apache-2.0 framework module is a license violation. No provenance policy exists: `find core/formats -name 'LICENSE*' -o -name 'NOTICE*'` → empty; spec.yaml has an `origin` field but `testdata/` files carry nothing.

### G4 — Harvesting / storage / generation of reference files

**Exists.** Per-format `testdata/`; `okapi:` fixture indirection (15 specs) keeping big files out of git; corpus/upstream test kinds; the synthetic-fixture warning (#482, implement-format SKILL "Footguns"); `make regen-okapi-fixtures` (manual by documented design — format-engineering.md:192 "no //go:generate, no Makefile target" is now stale, the target exists at Makefile:561).

**Structurally missing.**
1. **Single-machine dependency.** The Okapi corpus lives at a hardcoded absolute path on one laptop (docs, both skills, and `format-triage.js:48`). No mirroring (e.g. a versioned GitHub-release mirror of the harvested corpus, like `format-corpus`), so 15 specs and the whole parity harness die with that directory.
2. **No corpus manifest.** Nothing records per file: source URL, license, sha256, harvest date, which spec feature it exercises. Corpus erosion or silent mutation is invisible; "synthetic-only" vs "real" is re-judged by an LLM each triage run instead of being a recorded fact.
3. **The "at worst generation" leg is unbuilt.** No pipeline generates reference files for formats lacking real corpora (LibreOffice/Word headless automation for openxml/odf; Xcode/`xcstrings-tool` for xcstrings; pandoc for markdown variants; fontforge-style binary emitters). The rubric demands "real-world corpus" for L4 with no story for where it comes from.
4. **No malicious-input corpus.** malformed_test covers truncation/garbage; nothing covers zip bombs, XML entity expansion, deeply-nested JSON, RTL-override spoofing — robustness rubric line 98 stops at "clean Error + NotPanics". No fuzz targets exist (`grep -rln 'func Fuzz' core/formats` → empty) despite audit-format.py probing for a "fuzz" test kind.

**Blind spots:** large-binary policy (git vs LFS vs release-hosted — IDML/openxml corpora will grow); corpus freshness cadence (xcstrings/arb gain keys every platform release; no re-harvest ritual); confidentiality screening of harvested "real-world" files.

### G5 — Format-spec changes and tracking other implementations

**Exists.** The strongest part: refresh-format-maturity Step 4 (per-format GitLab issue sweep, REST recipe, "fixed upstream means relative to 1.48.0"); `contract-audit.yml` weekly cron catching new Okapi tests; issue-number cross-referencing into fixtures.

**Structurally missing.**
1. **Sweep results are not stored.** A sweep reports to chat; the next run cannot diff "issues seen since last sweep" (no last-seen issue iid / updated_at watermark anywhere).
2. **No Okapi version-bump runbook.** The 1.48.0 pin appears in the Makefile (:1004), contract-audit config, parity manifest (`cli/parity/formats/spec.go`), the docs, both skills, and okapi-bridge's 11-version Maven matrix — no single ritual covers "Okapi 1.49 released: re-pin, regen fixtures (`make regen-okapi-fixtures`), re-run contract-audit-all, sweep new tests, re-publish".
3. **Format *specs* themselves are untracked.** Nothing watches W3C (TTML/WebVTT), OASIS (XLIFF 2.2), Unicode (MF2/CLDR), Apple/Google platform format changes. `spec_refs` point at specs but no ritual re-checks them.
4. **Other OSS implementations untracked** (same as G3.1) — no comparison ledger of what translate-toolkit/Fluent/ICU4X fixed or diverged on.

**Blind spots:** format **retirement/deprecation lifecycle** — the rubric only goes up; `mo` is acknowledged as "genuinely thin… or formally retire it as a stub" (format-maturity.md:239) but no retired/deprecated level or exit criteria exist; legacy Okapi-era formats (vignette, transtable, ttx) have no sunset review. Also: policy for when neokapi *intentionally surpasses* a stagnant Okapi (when does an `okapi-bug` divergence become the permanent canonical behavior?). okapi-bridge release coordination is never mentioned in the format-ops docs.

### G6 — Maintaining and generating learning materials, specs, configuration

**Exists.** `make generate-reference-docs` + `nativedocs` sidecars + the `reference-data-drift.yml` CI gate; `make kapi-i18n-generate` git-diff gate; the published tutorial vs internal-notes split is explicitly documented (format-engineering.md:27–30).

**Structurally missing.**
1. **Generated-looking prose with no generator.** The maturity report block in format-maturity.md says "(generated)" but regeneration is manual narrative ("Regenerate it with… the format-maturity-gap-analysis workflow" — a workflow name that no longer exists; the file is `format-triage.js`). Stale within 5 days of writing.
2. **Two sources of rubric truth.** The L0–L4 prose table (format-maturity.md:72–78) and `gateLevel()` (`format-triage.js:211`) encode the rubric independently; no test asserts they agree, and they already subtly differ (gateLevel's L3 requires corpus presence; the prose's L3 requires "corpus/upstream test exercises real files").
3. **Known-stale docs are annotated, not fixed**: `implementing-formats.md` "won't compile" note, "CLAUDE.md is stale on this" (format-engineering.md:197–198) — the framework documents its own rot instead of having a sweep ritual that fixes it.
4. **No prompt/rubric versioning** (overlaps G1): scorer_version exists, prompt text and rubric edition do not; history snapshots can't be compared across rubric changes.

**Blind spots:** the score/remediate prompts duplicate rubric knowledge inline (e.g. `scorePrompt` instructs "Use the real fields (severity / skip.reason), not 'divergence_kind'" — :275 — while the docs teach `divergence_kind` for spec.yaml; both are right for *different files*, which is exactly the kind of nuance that will desync); per-format reference pages don't surface maturity level; no "format dossier" generator unifying spec.yaml + maturity row + parity row + corpus manifest per format.

### G7 — Identifying key new formats from l10n / content-world trends

**Exists.** Effectively nothing committed. The 8-format harvest wave was a one-off strategic exercise (its thesis lives in agent memory, not in `docs/internals/` — grep for it finds nothing). The `prefer-configured-readers` principle is the only standing decision rule, and `implement-format` SKILL's "When to use" gate.

**Structurally missing.**
1. **No format radar artifact**: candidate list with scoring criteria (TMS support matrices, GitHub file-extension frequency, l10n-industry surveys, CMS ecosystems), no review cadence, no decision log for rejected candidates ("considered SDLXLIFF / DITA / TOML / TBX; rejected because…" — silent rejections will be re-litigated every run).
2. **No demand telemetry**: the strongest possible signal — unrecognized-extension attempts in kapi CLI/desktop (privacy-aware, opt-in) — is not collected.
3. **No plugin-first staging policy**: the registry/plugin system could host radar candidates before core adoption; no path is defined from radar → plugin → core.

**Blind spots (the maintainer's own "not only textual" hint, made concrete):** no SVG (text elements are translatable — notable absence given 49 formats); no audio/transcript formats beyond subtitles (srt/vtt/ttml exist; no .srt-adjacent audio sidecars, no podcast chapter files); no image-with-text or Figma-file story (Figma is a *connector* in bowrain, not a format); no game-l10n formats (Unreal/Unity catalogs); no DITA, no TOML, no JSON5, no .ipynb (notebooks carry translatable markdown), no AI-era artifacts (prompt files, agents.md/skills — ironic given the repo's own dogfooding); video formats only as subtitle text, no embedded-caption containers.

---

## Cross-cutting blind spots (not on the maintainer's list)

1. **Remediation regression surface** — package-local tests only; no `make test`/parity re-run in the loop; human diff review optional (G1).
2. **Scorer drift across model generations actively hidden** by the sticky anchor; needs scheduled anchor-free calibration vs a frozen adjudicated golden set, with model-id recorded per snapshot (G1/G6).
3. **Metric self-gaming** — file-presence floor + AI-authored artifacts, with the Malformed dimension exempt from quality judgment (G1).
4. **Licensing/provenance of everything harvested** — fixtures, quoted spec text, other frameworks' tests; no NOTICE/manifest anywhere under core/formats (G3/G4).
5. **Format retirement lifecycle** — maturity is monotonic by construction; sticky anchoring even resists downgrades (G5).
6. **Single-laptop corpus** — `/Users/asgeirf/src/okapi/Okapi` is load-bearing for 15 specs, parity, contract-audit, fixtures regen (G4).
7. **Vocabulary dimension missing** and `Run.Type` free-form (G2) — blocks the editor-embedding and cross-format-tooling goals downstream.
8. **WASM/browser maturity invisible** — formats have a second runtime with different capabilities; not scored (G2).
9. **Issue linkage is prose, not data** — #560/#617/#504/openxml-RunFonts live in markdown sentences; no run can mechanically notice one closed (feeds the ledger below).
10. **Ops surface fragmentation** — knowledge is split across 2 docs, 2 skills, 1 workflow, 1 guardrail test, 5 CI workflows with no single entry point stating cadence — precisely the runbook's job.

---

## Runbook ledger — design input

### Durable signals already in the repo (a future run can compute "what happened since last run" from these alone)

| # | Signal | How to read it | What it dates/versions |
|---|---|---|---|
| S1 | `git log --format='%H %ad' --date=short -1 -- core/formats` (and per-format `core/formats/<id>/`) | last format-code change; diff against a stored sha gives the changed-format set | triage due-ness |
| S2 | `web/static/data/format-maturity.json` → `generated_at` (now 2026-05-31), `source`, (v2:) `scorer_version`, `run_integrity` | last triage publish + which scorer produced it | triage/publish |
| S3 | `web/static/data/format-maturity-history.json` → `[].date` array (1 entry) | full publish timeline; trend chart input (`web/src/pages/format-maturity/index.tsx:4`) | visibility-over-time |
| S4 | `web/static/data/parity-report.json` → `generated_at` (2026-05-20T15:34:08Z) + totals | last `make parity-publish` | parity ritual |
| S5 | `web/static/data/contract-audit.json` → `generatedAt` (2026-05-21), `okapiTag` (v1.48.0), `goCommitSHA` (a98fe30f) | last contract audit + the exact commit it audited | drift ritual |
| S6 | `docs/internals/format-maturity.md:140` — `## Maturity report (snapshot: 2026-05-30)` inside `BEGIN/END` markers | docs-snapshot age vs S2 | docs-sync ritual |
| S7 | `core/formats/maturity_test.go` ledgers: `grandfatheredRoundtrip` map size (:30) + `go test ./core/formats/ -run TestRobustnessCoverage -v` advisory count | live debt counters, recomputable | burndown delta |
| S8 | `audit-format.py --all --json` output (cheap, deterministic) vs the `floor`/`ceiling` fields stored per row in S2 | file-floor drift since last publish, per format | re-score targeting |
| S9 | `gh run list --workflow={parity,contract-audit,format-acceptance,nightly,reference-data-drift}.yml -L1` | last green CI run per gate; contract-audit cron = weekly Mon 06:00 UTC, nightly daily 03:00 | CI-health check |
| S10 | `gh issue view 560 617 504 448 --json state,updatedAt` (+ any issue ids found in `expected_fail` reasons / `parity-annotations.yaml`) | tracked-divergence issues closing | xfail-hygiene ritual |
| S11 | GitLab: `curl https://gitlab.com/api/v4/projects/62298414/issues?order_by=updated_at&per_page=1` → newest `updated_at`/`iid`; `…/repository/tags` → newest tag vs `OKAPI_VERSION ?= 1.48.0` (Makefile:1004) | upstream Okapi movement | okapi-sweep + version-bump rituals |
| S12 | `git log -1 --format='%H %ad' -- docs/internals/format-{maturity,engineering}.md .skills .claude/workflows/format-triage.js` | process-definition edits (rubric/prompt changes) | calibration trigger |
| S13 | `git log --diff-filter=A -- 'core/formats/*/spec.yaml' 'core/formats/*/malformed_test.go' …` | artifact-class waves (e.g. b7201a9f5 added 38 malformed tests on 2026-06-04) | what-was-built-since |
| S14 | `ls core/formats/*/parity-annotations.yaml | wc -l` (15), `ls core/formats/*/spec.yaml | wc -l` (41) + git mtimes | annotation/spec footprint growth | hygiene sweep scope |

Missing-but-cheap signals worth adding when building the runbook: a corpus
manifest (per-file sha/license/source — G4.2), last-seen Okapi issue watermark
(S11 has no stored counterpart), and model-id/prompt-hash in each S3 history
entry.

### Proposed minimal ledger schema

One committed file, e.g. `docs/internals/format-ops-ledger.json` (committed so
`git log` on it is itself a signal; small enough to hand-audit). Principle: the
**signals stay authoritative** — the ledger stores only *watermarks* (what each
ritual last consumed) plus datestamps, so "what's due" is a pure diff:
`due(ritual) = (today − last_run > cadence_days) OR any(current(signal) ≠ watermark)`.

```json
{
  "ledger_version": 1,
  "rituals": {
    "triage-score":     { "cadence_days": 14, "last_run": "2026-05-31",
                          "watermarks": { "core_formats_sha": "<S1>", "scorer_version": 2,
                                          "model_id": "", "prompt_sha": "<sha of format-triage.js>" } },
    "remediate":        { "cadence_days": 30, "last_run": null,
                          "watermarks": { "dashboard_generated_at": "<S2>" },
                          "carryover": [ { "format": "", "gap": "", "last_outcome": "" } ] },
    "docs-snapshot-sync": { "cadence_days": 0, "last_run": "2026-05-30",
                          "watermarks": { "dashboard_generated_at": "<S2 — due whenever ≠ S6>" } },
    "parity-publish":   { "cadence_days": 30, "last_run": "2026-05-20",
                          "watermarks": { "report_generated_at": "<S4>", "main_sha": "" } },
    "contract-audit":   { "cadence_days": 7,  "last_run": "2026-05-21", "ci_owned": true,
                          "watermarks": { "generatedAt": "<S5>", "okapiTag": "v1.48.0" } },
    "okapi-sweep":      { "cadence_days": 30, "last_run": null,
                          "watermarks": { "last_issue_iid": 0, "last_issue_updated_at": "",
                                          "latest_upstream_tag": "1.48.0", "pinned": "1.48.0" },
                          "per_format_last_swept": { "html": null } },
    "xfail-hygiene":    { "cadence_days": 60, "last_run": null,
                          "watermarks": { "tracked_issues": { "560": "open", "617": "open", "504": "open" } } },
    "calibration":      { "cadence_days": 90, "last_run": null,
                          "watermarks": { "model_id": "", "prompt_sha": "" },
                          "golden_set": ["html", "json", "mo", "xcstrings", "xliff2", "properties"] },
    "corpus-census":    { "cadence_days": 90, "last_run": null,
                          "watermarks": { "manifest_sha": null } },
    "format-radar":     { "cadence_days": 90, "last_run": null,
                          "decided": { "accepted": [], "rejected": { "<id>": "<one-line reason>" } } }
  },
  "runs": [
    { "date": "2026-05-31", "ritual": "triage-score", "commit": "<sha>",
      "outcome": "L1:36 L2:3 L3:10; published dashboard", "followups": [] }
  ]
}
```

Notes for the runbook author:
- `calibration` is the anchor-free golden-set run (G1.4) — trigger also on any S12 change (rubric/prompt edit) or model-generation change, not just cadence.
- `docs-snapshot-sync` has cadence 0: it is purely watermark-driven (due whenever S2 ≠ S6).
- `remediate.carryover` is where `test_passed=false` reports stop evaporating (G1.2).
- The `runs[]` append-only log doubles as the "visibility over time" record for rituals the dashboards don't cover, and each entry's `commit` lets a future Claude `git diff <commit>..HEAD -- core/formats` to enumerate exactly what was built since.
- Today's bootstrap values are all recoverable from S1–S14 (e.g. `last_run` for triage-score = S2's 2026-05-31), so the ledger can be seeded mechanically on the runbook's first run.
