# Deep-dive: the executable spec engine (`spec.yaml`)

Worktree root: `/Users/asgeirf/src/neokapi/neokapi/.claude/worktrees/format-process`
All paths below are relative to that root unless absolute.

## 1. Where spec.yaml is parsed and executed

### Package map (4 code locations, 3 consumers)

| Component | Path | Role |
|---|---|---|
| Spec model + validation | `core/format/spec/spec.go`, `core/format/spec/load.go` | YAML schema (`Spec`, `Feature`, `Example`, `Assertions`) + `Load()` + `Validate()` |
| Shared helpers | `core/format/spec/helpers.go` | `ResolveInput`, `ResolveFilePath` (incl. `okapi:` scheme), `MergeConfig`, `ReadParts`, `BlockTexts`, `EvalAssertions` |
| Native runner | `core/format/spectest/runner.go` | `spectest.NativeRunner` — always-on, driven by each format's `spec_test.go` |
| Parity runner | `cli/parity/spec/runner.go` (`//go:build parity`) | `ParityRunner` — same spec through native reader AND okapi-bridge daemon |
| Drift detection | `scripts/contract-audit/main.go` | `okapi_refs` drift vs pinned surefire (`detectSpecRefDrift`, main.go:1705) + config-key drift vs bridge JSON Schema (`detectSpecConfigDrift`, main.go:1745) |

Spec files: **41** `core/formats/<id>/spec.yaml` (every real format except the
8 harvest formats `androidxml/applestrings/arb/designtokens/i18next/mdx/resx/xcstrings`
plus `mo` and the non-formats `exec/jsx/memorytest`). Parity wiring exists for
**38** of them (`cli/parity/formats/*_spec_test.go`).

Gating: `core/formats/maturity_test.go:71` `TestFormatSpecIsGated` hard-fails any
format that ships a `spec.yaml` without a `spec_test.go` driving `spec.NativeRunner`
("An ungated spec rots silently", maturity_test.go:69).

### spec.yaml schema as implemented (`core/format/spec/spec.go`)

Top level (`Spec`, spec.go:21–66):
- `format` (required) — okapi-bridge filter id, e.g. `okf_openxml` (joins with bridge manifest + parity report keys)
- `kind` — `top_level` (default) | `subfilter` (spec.go:236–250). Subfilters skip the parity bridge runner and the bridge-schema config-drift check; native runner still runs every example.
- `mime_type`, `description`
- `variants[]` — `{id, name, extension, mime_type, description}` (openxml→docx/xlsx/pptx etc.); features/config/examples reference them via `applies_to` / `variant`
- `config[]` — `ConfigKey{key, type, default, description, applies_to?, okapi_param?}` (spec.go:79–86). `key` maps **1:1** to a key recognised by the native `Config.ApplyMap`; `type` ∈ `"boolean" | "string" | "string_list" | "int" | "string_map"`; `okapi_param` is the upstream Java field used for drift checking.
- `features[]` (required, ≥1)

`Feature` (spec.go:89–121): `id`, `name`, `description`, `applies_to?`,
`config?` (applied to every example unless overridden), `examples[]` (required, ≥1),
and three reference-list fields:
- `okapi_refs[]` — `"ClassName#methodName"` upstream @Test refs; contract-audit asserts each still exists in the pinned Okapi surefire output
- `native_refs[]` — `"package.TestFuncName"` Go tests (docs linkage)
- `spec_refs[]` — free-text spec citations ("CommonMark §6.7", "YAML 1.2 §8.1.1.1") — the "your filter violates <cited spec>" lever for upstream conversations (spec.go:112–120)

`Example` (spec.go:146–221):
- exactly one of `input_file` / `input_xml` / `input_bytes` (base64) — enforced by `Validate()` (load.go:110–122)
- `variant` (required for inline `input_xml` under a multi-variant spec, load.go:126–128)
- `bridge_only` — native runner skips, parity runner runs bridge-only
- `expected_fail: <reason>` — assertion failures log+record instead of failing; an unexpected PASS logs "remove the expected_fail tag" (spectest/runner.go:97–99)
- `divergence_kind` — explicit fault attribution, **8 recognised values** (spec.go:178–192): `native-bug`, `bridge-gap`, `okapi-bug`, `scope-diff`, `default-diff`, `missing-filter`, `fixture`, `contract`. When empty, contract-audit infers heuristically from the `expected_fail` text; the explicit value wins.
- `parity_strict` — promotes bridge↔native bytewise text mismatch to a hard failure (default: recorded as `parity_warn`)
- `config` — per-example overlay over feature config (`MergeConfig`, helpers.go:93)
- `origin` — provenance, three canonical grep-able forms (spec.go:204–216): `"authored: <reason>"`, `"okapi-fixture: <ref>"`, `"real-world: <source>"`. 575 `origin:` fields across the 41 specs (~all examples carry one).
- inline `Assertions`

### Assertion vocabulary — the complete set

`Assertions` struct (spec.go:225–233) — **7 fields, all reader-side, all over translatable-block source text only**:

```go
type Assertions struct {
	BlockCount       *int     `yaml:"block_count,omitempty"`
	BlockCountMin    *int     `yaml:"block_count_min,omitempty"`
	BlockCountMax    *int     `yaml:"block_count_max,omitempty"`
	FirstBlockText   *string  `yaml:"first_block_text,omitempty"`
	BlockTexts       []string `yaml:"block_texts,omitempty"`       // exact, ordered
	HasBlockWithText []string `yaml:"has_block_with_text,omitempty"` // substring
	NoBlockWithText  []string `yaml:"no_block_with_text,omitempty"`  // substring absent
}
```

Dispatch is `EvalAssertions` (helpers.go:153–198) — a flat field-by-field check, not a switch; the universe of observable state is `BlockTexts(parts)` (helpers.go:131–148), i.e. `model.RunsText(blk.Source)` for `Translatable` blocks with non-empty text. **Nothing about run structure, targets, overlays, layers, Data parts, notes, IDs, or writer output is assertable.**

### input_file resolution incl. `okapi:`

`ResolveFilePath` (helpers.go:42–54): `okapi:` prefix → `FindOkapiTestdataRoot()`
(helpers.go:60–90) walks up to `go.work`, then picks the **latest version dir**
under `<repo>/okapi-testdata/` (populated by `scripts/fetch-okapi-testdata.sh`);
the remainder of the path is joined under it
(e.g. `input_file: okapi:okapi/filters/idml/src/test/resources/foo.idml`).
Absolute paths pass through; everything else resolves against the spec's own dir
(`Spec.dir`, set by `Load`). When the corpus isn't fetched, both runners
**skip cleanly** for `okapi:` inputs (spectest/runner.go:65–69, parity/spec/runner.go:121–127) — a missing corpus never fails CI.

Fixture-sourcing rule (doc comment, spec.go:130–145): binary formats must PREFER
`okapi:` upstream fixtures; synthetic `testdata/*` fixtures routinely omit
attributes real authoring tools emit and produce false "bridge bug" divergences (#482).

### How runs are executed

`NativeRunner.Run` (spectest/runner.go:37–54): `t.Run(feature.ID)/t.Run(example.Name)`
subtests; per example: skip `bridge_only` → `ResolveInput` → `NewReader(variant)` →
`Config().ApplyMap(MergeConfig(feat.Config, ex.Config))` → `ReadParts`
(Open/Read/Close with 30s timeout, `src=en trg=fr`, helpers.go:106–127) →
`EvalAssertions` → `expected_fail` downgrade logic.

`ParityRunner.runExample` (cli/parity/spec/runner.go:104–261): same resolution,
then **three independent checks**: bridge-satisfies-spec, native-satisfies-spec,
and bridge==native bytewise on `BlockTexts`. Outcome rows are emitted via
`parity.Report` under `Kind="format-spec-feature"` with ID
`<format>::<featureID>::<exampleName>` and status ∈
`pass | fail | skip | expected_fail | parity_warn` (runner.go:107–118, 227–260);
`parity_warn` = both sides pass the spec but emit different text and
`parity_strict` is false. Report rows flush to `$KAPI_PARITY_REPORT`
(default `$KAPI_PARITY_SANDBOX/test-comparison.json`, cli/parity/report.go:73–86)
and feed the contract-audit dashboard (`specSummary`/`specExample` in
scripts/contract-audit/main.go:190–222, incl. the `Divergence` colouring).

### bridge_config

`ParityRunner.BridgeConfig func(cfg map[string]any) (map[string]any, error)`
(cli/parity/spec/runner.go:54): translates the **neokapi-keyed** merged config into
the bridge daemon's parameter shape; native always receives the original map —
"spec.yaml stays monolingual in neokapi terms" (runner.go:52–54). Five translators
exist: `cli/parity/formats/{csv,fixedwidth,regex,ttml,xmlstream}_bridge_config.go`.
The csv one (csv_bridge_config.go:11–40) is the canonical example: it renders the
entire config as a single Okapi `#v1` ParametersString blob under the reserved
`fprmContent` key because the bridge's per-key `setString` path never re-runs
`Parameters.load()` (#530).

### parity-annotations.yaml (separate sidecar, different engine)

Consumed by `cli/parity/roundtrip/annotations.go` (NOT by the spec runners).
On-disk shape (`annotationFile`, annotations.go:69–72):

```yaml
format: csv                # must match dir name (annotations.go:170–174)
fixtures:
  <fixture-filename>:
    severity: ...          # bug | cosmetic | native-more-correct | fixture-bug | unknown (annotations.go:31)
    issue: 123             # optional GH issue number
    summary: "..."         # WHY it diverges + what would clear it
    spec_ref: "ECMA-376-1 §17.3.2.35"   # optional spec citation
    notes_anchor: "..."    # anchor in the format's PARITY_NOTES.md
    skip:                  # optional per-engine skip directive
      engines: [okapi]     # "okapi" = whole fixture; other names = that engine only
      reason: "..."
```

10 formats ship one (csv, icml, yaml, openxml, vignette, ts, txml, tmx, odf,
transtable). It annotates **round-trip corpus fixtures** (the pseudo-translate
3-engine harness over okapi-testdata), not spec.yaml examples.
`cli/parity/roundtrip/failnew_test.go:17` gates: every new round-trip failure must
have a matching annotation entry. Note: there is **no `expected_fail` key in
parity-annotations.yaml** — `expected_fail`/`divergence_kind` live on spec.yaml
examples; the annotation file's analogue is `severity` + `skip`.

## 2. Promised but unimplemented

The round-trip / writer-output assertion type is promised in spec headers and
docs, and explicitly does not exist:

- `core/formats/html/spec.yaml:659–661`: "These are writer-side behaviors that the reader-only assertion vocabulary in this spec cannot express. Native roundtrip tests (html.TestRoundtrip_*) cover the reader → writer path; **a future spec iteration can add a roundtrip-output assertion type.**"
- `core/formats/csv/spec.yaml:671–675`: "…are not expressible through this spec's BlockTexts assertion vocabulary — **there is no roundtrip / writer-output assertion type. Captured here as a known coverage gap.**"
- `docs/internals/format-engineering.md:133`: "**Assertions see only translatable block source text — there is no round-trip / writer-output assertion type.**"
- `docs/internals/format-maturity.md` Open question 4: "Worth adding a round-trip/writer-output assertion type to the spec `Assertions` vocabulary (**promised in spec headers but unimplemented**), or is delegating round-trip to native `TestRoundTrip_*` the intended permanent split?"
- `cli/parity/roundtrip/doc.go:11–14` states the gap as design rationale: "The spec.yaml runners verify reader contracts (Block count, source text, IDs) but never invoke a writer. They prove 'we can read' — not 'we can read, modify, and write back coherently'."

Other under-specified-by-design notes embedded in spec tails (grep `Intentionally under-specified`):
doxygen has a `roundtrip_preserves_text` feature (doxygen/spec.yaml:761) that fakes
round-trip coverage with a read-only assertion ("Bytewise roundtrip is exercised by
the dedicated native writer [tests]"). epub/spec.yaml:247 same pattern. No TODO/FIXME
markers exist in `core/format/spec/`, `core/format/spectest/`, or `cli/parity/spec/`
— the gap is documented in prose, not code comments.

## 3. cli/parity consumption + fixtures_*_generated.go

Two **coexisting** parity systems (format-maturity.md Open question 3 asks whether
to retire the legacy one):

1. **Spec-driven** — `cli/parity/formats/<id>_spec_test.go` (38 files) each load
   the same `core/formats/<id>/spec.yaml` via `parityspec.LoadSpec` (an alias of
   `formatspec.Load`, cli/parity/spec/runner.go:25–27) and run `ParityRunner`
   (e.g. `TestParityHtmlSpec`, cli/parity/formats/html_spec_test.go:20–31).
   One source of truth: the identical file drives the always-on native test.

2. **Legacy table-driven** — `cli/parity/formats/spec.go` defines
   `var formatSpecs = []FormatSpec{...}` (spec.go:204) — one row per okf_* filter
   in the bridge 1.48.0 manifest, with `FormatInput` fixtures (inline strings),
   `NewReader`/`NewWriter` (NewWriter triggers an extra round-trip pass reported
   under `Kind="format-roundtrip"`, spec.go:117–123), `Skip*` reasons
   (SkipBinary/SkipDivergence453/SkipBridgeBug452/SkipBridgeConfig, spec.go:73–92),
   `BridgeFilterClass`+`ConfigID` for config-preset formats (okf_dita/docbook/resx),
   and Tikal third-corner wiring (`TikalExt`/`TikalConfig`).

`fixtures_*_generated.go` (11 files: dtd, html, json, markdown, po, properties,
regex, tmx, ts, wiki, xliff, yaml) belong to the **legacy** system. They are
emitted by `scripts/okapi-test-scan/main.go` — a deliberately lossy regex scanner
over Okapi Java test sources that recovers `String snippet = "..."` literals from
`@Test` methods (doc comment, main.go:1–35) and emits
`Generated<Class>Inputs []FormatInput` slices with
`OkapiTest: "Class#method"` (3-way dashboard join) and `Informational: true`
(failures logged + reported, never fail CI — spec.go:103–107). They are merged
into curated fixtures via `mergeInputs` (spec.go:186–193; e.g. okf_html row,
spec.go:210–222). They do **not** feed the spec.yaml runners — but spec.yaml's
`okapi_refs` cite the same Java tests, joined by contract-audit.

The third system, `cli/parity/roundtrip/` (engine.go, pseudo.go, coverage_test.go),
runs deterministic pseudo-translation extract→translate→merge over the full
okapi-testdata corpus through native / bridge / upstream-Okapi engines and is where
parity-annotations.yaml applies.

## 4. Spec file census: two mature + one thin

41 specs, 17,053 total lines. Largest: doxygen (789), csv (696), html (685),
messageformat (664), xml (630). Smallest: srt (88), versifiedtext (117),
splicedlines (135).

### html (`core/formats/html/spec.yaml`, 685 lines) — mature
- **14 features, 45 examples** (~3.2 examples/feature), 3 `string_map` config keys (`parser`, `elements`, `attributes`).
- 30-line authoring header: maps ~177 upstream @Test methods into 14 user-facing features; documents what's deliberately excluded.
- Dense cross-referencing: every feature carries `spec_refs` (HTML Living Standard §§), `okapi_refs`, `native_refs`; every example has `origin: "authored: …"`.
- 2 `expected_fail`s; 65-line "intentionally under-specified" tail (lines ~620–685) enumerating writer-side behaviors, rule-engine subsets, and quote-mode gaps — including the explicit promise of a future roundtrip-output assertion type (line 661).

### properties (`core/formats/properties/spec.yaml`, ~530 lines) — mature
- **12 features, 35 examples**, ~11 config keys all named with Okapi parameter names (useKeyCondition, extractOnlyMatchingKey, keyCondition, extraComments, escapeExtendedChars, convertLFandTab, idLikeResname, …) so spec↔bridge-schema drift is testable 1:1.
- Header documents why inline-codes (`useCodeFinder`) is a feature but NOT a config key (nested `$defs` leaf names too ambiguous for the flat surface).
- 3 `expected_fail`s; mixes inline `input_xml` snippets with `okapi:` corpus refs.

### srt (`core/formats/srt/spec.yaml`, 88 lines) — thin
- **3 features, 3 examples** (1 example/feature), zero config keys, zero `okapi_refs` (native-only: the bridge has no okf_subrip — upstream handles SubRip via okf_regex, so no parity test exists; header explains the absence of `srt_spec_test.go` in cli/parity).
- All `spec_refs` point at the community Wikipedia spec; every example asserts `block_count` + `block_texts`. Structurally complete but shallow: no malformed-input, no config surface, no negative assertions.

Density signal: examples per feature ranges from 1 (srt) to ~3.2 (html); `origin:`
provenance is near-universal (575 occurrences); `expected_fail` appears 140+ times
across 30 specs but explicit `divergence_kind` only 3 times (all `bridge-gap`) —
attribution is mostly left to contract-audit's text heuristic.

## 5. Concrete extension points

### (a) Round-trip assertions
- **Schema**: add to `Assertions` (core/format/spec/spec.go:225) e.g.
  `RoundTrip *RoundTripAssertion` with `{mode: byte_exact|idempotent|pseudo, output_file?: string, output_contains?: []string}`. Keep it nil-means-skip like every other field.
- **Runner hook**: `NativeRunner` (spectest/runner.go:24) needs a `NewWriter func(variant string) format.DataFormatWriter` field — the exact pattern already proven in the legacy table (`FormatSpec.NewWriter`, cli/parity/formats/spec.go:139, which reports `Kind="format-roundtrip"`). Drive reader→(optional pseudo transform)→writer and compare against input (byte_exact) or against a committed expectation.
- **Eval**: `EvalAssertions(parts, a)` (helpers.go:153) only sees parts; add a parallel `EvalOutputAssertions(output []byte, a)` so the parts-only callers stay untouched.
- **Reuse**: the pseudo-translate transform and 3-engine comparison logic already exist in `cli/parity/roundtrip/{pseudo.go,engine.go}`; the spec engine only needs the native single-engine flavor, with the parity flavor delegating bridge round-trip to the existing `Kind="format-roundtrip"` rows.
- **Gate alignment**: `TestRoundTripTestNamingConvention` (maturity_test.go:88) currently enforces a *file-naming* convention; once specs can assert round-trip, the grandfathered ledger (maturity_test.go:30) becomes burn-down fuel, and format-maturity.md Open question 4 gets resolved.

### (b) Vocabulary / semantic assertions (e.g. `<b>` → typed bold code)
- The model already carries everything needed: `Run` is a discriminated union (core/model/run.go:127–136) with `Kind()` ∈ text/ph/pcOpen/pcClose/sub/plural/select (run.go:141–147), and paired codes carry semantic `Type` strings (`PcOpenRun.Type`, run.go:80–82) like `"fmt:bold"`, governed by the vocabulary registry in `core/model/vocabulary.go` (`reg.Lookup("fmt:bold")`, `reg.HTMLOpen/HTMLClose`, vocabulary_test.go:17,118–119). The html spec *describes* this ("paired pcOpen/pcClose runs … carrying a semantic type (fmt:bold, fmt:italic, …)", html/spec.yaml inline_formatting feature) but cannot assert it.
- **Proposed assertion fields**: `block_runs` — a compact per-block run signature list, e.g. `["text", "pcOpen:fmt:bold", "text", "pcClose:fmt:bold", "text"]`; plus `has_run_with_type: [fmt:bold]` / `placeholder_kinds: [element]` for non-ordered checks. Implementation = a `RunSignature(run model.Run) string` helper next to `BlockTexts` in helpers.go (switch on `run.Kind()`, append `Type`/`SubType`).
- **Parity caveat**: bridge output arrives through `parity.TryRunBridge` as parts too (`EvalAssertions(bridge, …)`, cli/parity/spec/runner.go:165), so run-shape assertions work head-to-head — but bridge inline-code typing differs from native (`parity_warn` exists precisely because "divergent representations (e.g. inline-formatting markers)", spec.go:196–197). Run-shape assertions should default to native-only with an opt-in `parity` flag, or accept a per-side expectation.
- This is the highest-leverage semantic upgrade: it turns spec.yaml from "what text comes out" into "what *structure* comes out", which is the actual format-engine contract.

### (c) Ingesting AI-generated examples safely
- **Provenance**: `Origin` (spec.go:216) already defines three canonical grep-able forms; add a fourth: `"generated: <model>/<date> verified-by: <human|test-run>"`. Optionally promote to structured fields (`origin_kind`, `origin_ref`) — but the existing free-text-after-tag convention ("the form is for grep not for parsing") suggests keeping it one string.
- **Validation already in place**: `Load()` → `Validate()` (load.go:29) rejects duplicate feature/example names, unknown config keys, unknown variants, and input-shape violations — so AI output that hallucinates a config key fails at load time, before any reader runs. `TestFormatSpecIsGated` ensures generated examples are immediately executed.
- **Dedup**: no current mechanism beyond per-feature unique example names (load.go:106–109). Add to `Validate()` (or a separate lint pass in contract-audit) a content-hash check over `(normalized input, config)` across the whole spec, so a generated example identical to an authored one is rejected; the legacy scanner solved the same problem socially via the `gen-` name prefix + `Informational: true` (fixtures_*_generated.go).
- **Safety tier**: mirror the legacy `FormatInput.Informational` flag (cli/parity/formats/spec.go:103–112) as an `informational: true` example field — generated examples report to the dashboard but can't fail CI until a human flips them to strict. Combined with the existing `expected_fail` warn-on-pass behavior (spectest/runner.go:98), this gives a clean promotion ladder: generated/informational → verified/strict.
- **Assertion grounding rule**: spec headers already encode the norm ("All numbers were verified against the live reader before being written", html/spec.yaml:18–20). For AI ingestion, make it mechanical: a `kapi`-side `spec verify --fill` mode that runs the reader and *writes back* observed assertion values for human review, instead of trusting model-predicted block counts.

## Punch list: evolving spec.yaml into the Knowledge asset

1. **Implement the promised round-trip assertion type** (`roundtrip:` on `Assertions`, `NewWriter` hook on both runners, `EvalOutputAssertions`) — resolves format-maturity.md Open question 4 and the html/csv spec-header IOUs; report under the existing `Kind="format-roundtrip"`.
2. **Add run-shape semantic assertions** (`block_runs` signatures + `has_run_with_type`) keyed to the `core/model/vocabulary.go` registry, native-strict / parity-opt-in — makes typed-code behavior (fmt:bold, placeholders, plurals) part of the executable contract.
3. **Close the spec coverage gap**: put the 8 harvest formats + `mo` on native-only spec.yaml (Open question 1); their invariants/corpus tests stay, but the knowledge surface becomes uniform across all 49 formats.
4. **Retire the legacy `formatSpecs` table** (Open question 3): migrate the per-row knowledge that has no spec.yaml home — `NewWriter` round-trip, Tikal wiring, `BridgeFilterClass`/`ConfigID` presets, Skip ledgers — into spec.yaml fields, then fold `fixtures_*_generated.go` inputs into specs as `origin: "okapi-fixture: …"` + `informational: true` examples.
5. **Make `divergence_kind` mandatory for every `expected_fail`** (currently 3 explicit vs 140+ xfails relying on the dashboard's text heuristic) — enforce in `Validate()` or contract-audit; the fault-attribution taxonomy is the most valuable judgment data in the system and it's mostly implicit today.
6. **Structured provenance + AI-ingestion ladder**: extend `origin` with a `generated:` form, add `informational:` to `Example`, content-hash dedup in `Validate()`, and a `spec verify --fill` reader-grounding mode so generated assertions are observed, not predicted.
7. **Unify the annotation planes**: parity-annotations.yaml (per-fixture severity/skip for the round-trip corpus) and spec.yaml `expected_fail`/`divergence_kind` (per-example) use overlapping but disjoint vocabularies (`severity: native-more-correct` ≈ `divergence_kind: okapi-bug`). Define one divergence taxonomy and have both files reference it.
8. **Surface assertions about non-Block parts**: Data/Layer/notes/ids are streamed (`ReadParts` returns all parts) but invisible to `EvalAssertions`; add `data_count`, `note_texts`, `layer_depth`-style fields so skeleton-preservation knowledge stops living only in prose tails.
9. **Promote the "intentionally under-specified" prose tails to structured `gaps:` entries** (id + reason + okapi_refs) so the dashboard can count known-unspecified surface instead of it living in comments only grep can find.
