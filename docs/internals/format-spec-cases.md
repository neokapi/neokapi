# Format Spec Cases — The Executable Spec-Case Model

This document defines neokapi's spec-case model: the grammar of an executable
format-specification case, the neutral oracle encoding that any implementation
can be tested against, and the generation loop that grows the corpus. It is
the design that [format-maturity.md](./format-maturity.md) references for the
multi-view expected model (Engine L1/L3/L4 view mapping, §2.1 there), the E2
anchor-survivability case (§2.3 there), and the citation contract (§2.4
there); and that [format-ops.md §3](./format-ops.md) ritual 9 (`case-gen`)
executes against. **Implementation status (#847): the multi-view runner has
landed — the block-event dump (`core/format/spec/blockevents.go`,
`DumpBlockEvents`), the additive case grammar + meta-schema gate
(`spec.go`, `validate.go`), the multi-view + accept-mode runner
(`core/format/spectest/`), and the case-gen differential-oracle hook
(`RunNativeCase`, `core/format/spec/oracle.go`). See §10 for the entrypoints.**
The
research base is `docs/internals/research/format-ops/` (SYNTHESIS §3,
`executable-specs.md`, `eval/spec-engine.md`); this document is its
decision-ready distillation, written as end state.

The grammar below is not invented: it is the convergent case grammar across
CommonMark, the JSON Schema Test Suite, toml-test, yaml-test-suite, WPT,
test262, Unicode Data-Driven Testing, and the XLIFF test-suite
(https://github.com/commonmark/commonmark-spec,
https://github.com/json-schema-org/JSON-Schema-Test-Suite,
https://github.com/toml-lang/toml-test,
https://github.com/yaml/yaml-test-suite,
https://web-platform-tests.org/test-suite-design.html,
https://github.com/tc39/test262/blob/main/CONTRIBUTING.md,
https://github.com/unicode-org/conformance,
https://github.com/oasis-tcs/xliff-xliff-22). Every borrowed mechanism keeps
its citation.

## 1. Why not the status quo

The current engine (`core/format/spec/spec.go`, `core/format/spectest/`,
`cli/parity/spec/`) is genuinely good at one thing — reader-contract
verification across 41 formats with parity wiring for 38 — and structurally
incapable of three things:

1. **The round-trip IOU.** The writer-output assertion type is promised in
   prose in five places and exists in none:
   `core/formats/html/spec.yaml:659–661` ("a future spec iteration can add a
   roundtrip-output assertion type"), `core/formats/csv/spec.yaml:671–675`
   ("there is no roundtrip / writer-output assertion type. Captured here as a
   known coverage gap"), `docs/internals/format-engineering.md:133`,
   format-maturity.md's former Open question 4 (resolved by this document),
   and `cli/parity/roundtrip/doc.go:11–14` ("They prove 'we can read' — not
   'we can read, modify, and write back coherently'"). Two specs (doxygen,
   epub) fake round-trip coverage with read-only assertions.
2. **Assertions see only translatable block source text.** The complete
   assertion vocabulary is seven fields (`block_count`, `block_count_min`,
   `block_count_max`, `first_block_text`, `block_texts`,
   `has_block_with_text`, `no_block_with_text` — `Assertions`, spec.go:225);
   the entire observable universe is `BlockTexts(parts)`, i.e.
   `model.RunsText(blk.Source)` over translatable blocks. Run structure,
   typed inline codes, targets, overlays, layers, Data parts, and writer
   output are all unassertable — the actual format-engine contract (what
   *structure* comes out, and whether it goes back in) lives only in Go test
   code.
3. **The Go engine is the blessed oracle.** Expected behavior is encoded as
   in-process Go assertions; okapi-bridge results arrive only through the
   build-tagged parity runner, and a WASM build or a future port cannot run
   the corpus at all. toml-test names the failure mode exactly: "a data
   exchange format is needed… TOML cannot be used, as it would imply that a
   particular parser has a blessing of correctness"
   (https://github.com/toml-lang/toml-test). A spec corpus must outlive
   implementations.

## 2. The case

A case is the atomic unit: one input, one validity class, one or more
expected views, one citation. Cases live grouped under the format's existing
`spec.yaml` sections (the `features` grouping survives as the section layer;
see §11).

```yaml
- id: QF7K            # stable, 4–6 chars, never reused (yaml-test-suite IDs)
  name: entity-in-attribute
  class: valid        # valid | invalid | operation
  input:
    inline: '<p title="a &amp; b">x</p>'
  config: { attributes: { title: extract } }
  expected:
    blocks: cases/QF7K.events.jsonl     # or inline JSONL string
    extracted:                          # today's Assertions vocabulary, verbatim
      block_texts: ["a & b", "x"]
    roundtrip: { mode: byte_exact }
  cite: { spec: whatwg-html, version: "2026-05", url: "https://html.spec.whatwg.org/multipage/syntax.html#character-references", clause: "13.1.4", heading: "Character references", quote: "Character references must start with a U+0026 AMPERSAND character (&).", quote_sha256: "…" }
  tags: [entities, attributes]
  since: "5.0"        # format-version bounds; omit when version-independent
  origin: { kind: authored, note: "attribute entity decoding per spec" }
```

| Field | Contract |
|---|---|
| `id` | 4–6 char alphanumeric, unique per format, **never reused** after deletion (yaml-test-suite, https://github.com/yaml/yaml-test-suite). Results stay comparable across corpus revisions. |
| `name` | Human-readable. For `class: invalid`, the name **is the fault**: one fault per case, named after it (toml-test "Invalid tests… should try to test one thing and one thing only"; XLIFF `bad_DataIdNotUnique.xlf`, https://github.com/oasis-tcs/xliff-xliff-22). |
| `input` | Exactly one of `inline:` (text), `bytes:` (base64), `file:` (spec-relative path, or the `corpus:<relpath>` scheme resolved by `FindCorpusRoot()` per [format-maturity.md §2.5](./format-maturity.md), or the existing `okapi:<path>` scheme resolved by `FindOkapiTestdataRoot()`, helpers.go:60). Missing fetched corpora skip with the fetch command in the message, never fail. |
| `class` | `valid` \| `invalid` \| `operation` (§3). |
| `expected` | One or more views (§5). `class: invalid` uses exactly `expected.error: {category, message_contains?}`. |
| `cite` | Machine-checkable citation (§6). Required for `valid`/`invalid` cases of spec-backed formats; behavioral conventions without formal spec authority may omit it with a `tags: [convention]` marker. |
| `tags` | Free-form selectors; conventions follow yaml-test-suite's tag-usage discipline (https://github.com/yaml/yaml-test-suite/blob/main/doc/tag-usage.md). |
| `since` / `until` | Format-version bounds; per-version manifests aggregate them (§9). |
| `origin` | Structured provenance (§9, §10): `kind: authored \| okapi-fixture \| real-world \| suite \| bug \| generated` plus kind-specific fields. |
| `variant`, `config` | Unchanged from the current model (`Example.Variant`, `Example.Config` overlaying the section's config via `MergeConfig`, helpers.go:93). |

Notably absent from the case: `expected_fail`, `divergence_kind`,
`bridge_only`, `parity_strict`. Implementation-specific status never lives in
the corpus (§7).

## 3. Three validity classes

The XLIFF 2.1 test suite's triad (`valid/`, `invalid/`, `core/in-out/`
transformation pairs) is the closest existing analog to neokapi's
read→transform→write unit and is adopted whole
(https://github.com/oasis-tcs/xliff-xliff-22).

| Class | Semantics | Expected |
|---|---|---|
| `valid` | The input parses; extraction produces the asserted model. | Any of the §5 views. |
| `invalid` | The input **must be rejected**, cleanly (no panic, bounded resources). One fault per case, fault-named. | `expected.error: {category}` — the error-category assertion. Never auto-updated (§8). |
| `operation` | An in-out pair: input + named operation → expected output. | Operation-specific (below). |

Operations are the executable form of the engine's actual value proposition:

| `operation:` | Input → output | Expected view |
|---|---|---|
| `extract` | native → block-event dump | `expected.blocks` (the default for `valid`; listed for symmetry) |
| `merge` | block-event dump (typically the extract dump with targets edited) → native | `expected.roundtrip` against committed native output |
| `segment` | native (+ segmenter config) → block-event dump with segmentation overlays | `expected.blocks` including `overlays` |
| `redact` | native → block-event dump with placeholder substitutions, then restore → native | `expected.blocks` + `expected.roundtrip` |
| `anchor` | extract → edit-in-native-editor fixture → re-extract | the **E2 anchor-survivability case**: the `editor-anchor` overlay survives the edit cycle, per [format-maturity.md §2.3](./format-maturity.md) — E2 evidence is "an `in-out` anchor-survivability case", not a demo |

## 4. The neutral expected encoding: the block-event dump

The canonical oracle is a deterministic JSON event dump of the part stream —
the analog of yaml-test-suite's `test.event` DSL plus toml-test's tagged-value
JSON (https://github.com/yaml/yaml-test-suite,
https://github.com/toml-lang/toml-test). It (a) prevents the Go engine from
being the blessed oracle, (b) lets okapi-bridge, the WASM build, and future
ports run the corpus via a thin stdin/stdout shim (the Unicode DDT `#TEST`
executor protocol, https://github.com/unicode-org/conformance), and (c) gives
AI generation a schema-checkable target.

### 4.1 Shape

JSON Lines: one event object per `model.Part`, with exactly one top-level key
naming the part type — mirroring the Run discriminated-union convention. The
part types are the model's own (`core/model/part.go`: LayerStart, LayerEnd,
GroupStart, GroupEnd, Block, Data, Media).

```jsonl
{"layer_start":{"id":"doc","format":"html","locale":"en","mime_type":"text/html"}}
{"data":{"id":"d1"}}
{"block":{"id":"b1","translatable":true,"source":[{"type":"text","text":"Press "},{"type":"pcOpen","id":"1","semantic":"fmt:bold","data":"<b>"},{"type":"text","text":"Start"},{"type":"pcClose","id":"1","semantic":"fmt:bold","data":"</b>"}],"properties":{"resname":"intro"}}}
{"block":{"id":"b2","translatable":true,"source":[{"type":"text","text":"Hello"}],"targets":{"fr-FR":[{"type":"text","text":"Bonjour"}]},"overlays":[{"type":"segmentation","spans":[{"id":"s1","range":[0,0,0,5]}]}]}}
{"layer_end":{"id":"doc"}}
```

Field contracts, grounded in `core/model`:

- **`layer_start` / `layer_end`** — from `model.Layer`: `id`, optional
  `name`, `format`, `locale`, `mime_type`, `encoding`, `multilingual`,
  `properties`. `layer_end` carries `id` only.
- **`group_start` / `group_end`** — `id`, optional `name`, `properties`.
- **`block`** — from `model.Block`: `id`, optional `name`, `type`,
  `translatable`, `source` (run list), optional `targets` (map keyed by the
  `VariantKey` text form — bare locale, `;tone=`/`;channel=` suffixes, per
  `VariantKey.MarshalText` — to a run list), optional `properties` (sorted
  keys), optional `overlays`, optional `preserve_whitespace`.
- **runs** — each run dumps as `{type, …}` where `type` is the Run
  discriminator (`text|ph|pcOpen|pcClose|sub|plural|select`,
  `model.RunKind`): `text` → `{type,text}`; `ph`/`pcOpen`/`pcClose` →
  `{type, id, semantic?, subtype?, data?, equiv?}` where `semantic` is the
  run's vocabulary `Type` (e.g. `fmt:bold`, governed by
  `core/model/vocabulary.go`) and `data` is the captured native markup;
  `sub` → `{type, id, ref}`; `plural`/`select` → `{type, pivot, forms|cases}`
  with branches recursing (sorted branch keys). This is what finally makes
  typed-code behavior (`<b>` → `pcOpen` with `semantic: fmt:bold`) part of
  the executable contract rather than prose in the html spec.
- **overlays** — `{type, layer?, variant?, spans: [{id?, range:
  [startRun, startOffset, endRun, endOffset], props?}]}` per `model.Overlay`
  / `model.Span` / `model.RunRange` (half-open, rune offsets). Present when
  the case's operation produces them (`segment`, `anchor`); absent
  otherwise.
- **`data` / `media`** — `id`, optional `name`, `properties`.

### 4.2 Determinism rules

The dump is canonical or it is useless as an oracle: object keys in fixed
schema order; map-valued fields (`properties`, `targets`, plural `forms`,
select `cases`) sorted by key; empty/zero fields omitted; HTML escaping
disabled (matching `Run.MarshalJSON` and the KLF wire form, so `<b>` stays
literal and content hashes are implementation-independent); UTF-8, LF
separators, no trailing whitespace. Excluded by design: `Skeleton`,
`Identity`, `ContentRef`, `DisplayHint`, `Annotations`, `IsReferent` — engine
internals and tool products, not the reader contract. `kapi spec-dump` emits
it; the same encoder backs accept-mode (§8).

### 4.3 The shim contract

Any implementation runs the corpus by providing one executable (the ~30-line
shim; toml-test decoder/encoder protocol + DDT `#TEST`):

```
stdin   line 1: JSON header {"format","variant"?,"config"?,"src","trg","operation"}
        rest:   the payload, verbatim to EOF
stdout  the result payload
exit    0 = accepted; 1 = rejected (stderr line 1 = error category)
```

Payload direction follows the operation: `extract`-direction shims read
native bytes and write the event dump; `merge`-direction shims read the event
dump and write native bytes. The harness (`kapi spec-test`) orchestrates,
diffs dumps structurally, and never needs the implementation's language. The
okapi-bridge shim wraps the existing gRPC daemon; the WASM shim is the same
binary compiled cgo-less; a hypothetical Rust port needs only the 30 lines.

## 5. Expected views and the Engine-level mapping

One input, multiple assertion planes, adopted incrementally — yaml-test-suite
(`test.event` / `in.json` / `out.yaml`) and Unicode DDT test/verify pairs
(https://github.com/yaml/yaml-test-suite,
https://github.com/unicode-org/conformance).

| View | Asserts | Form |
|---|---|---|
| `expected.blocks` | The full structural contract: the §4 event dump, compared structurally. | Inline JSONL or sibling file `cases/<id>.events.jsonl`. |
| `expected.extracted` | Text-only extraction. **The assertion set is today's `Assertions` vocabulary, unchanged** (the seven fields of spec.go:225). | Inline assertion fields. |
| `expected.roundtrip` | Writer output. `mode: byte_exact` (output == input), `idempotent` (read→write→read→write fixpoint), or `normalized` (output == committed normalized fixture — the xliff2 normalizing-DOM-writer class, #560). | Mode + optional `output_file`. |
| `expected.valid_by` | Writer output passes an external validator: ODF Validator, Open XML SDK / openxml-audit, Okapi Lynx/Schematron for XLIFF, `msgfmt -c`, CommonMark `spec_tests.py` (https://odftoolkit.org/conformance/ODFValidator.html, https://github.com/dotnet/Open-XML-SDK). | Validator id from the acceptance harness. |
| `expected.error` | (`class: invalid` only) clean rejection with category. | `{category, message_contains?}`. |

Views map onto the Engine ladder
([format-maturity.md §2.1](./format-maturity.md)): **L1** is the blocks-view
contract made executable (read fidelity as structure, not prose); **L3**
requires the roundtrip view green across the format's cases; **L4** requires
all applicable views, an invalid corpus, and `valid_by` wherever a validator
exists. `expected.extracted` is the floor view every migrated case already
has.

## 6. Machine-checkable citations

Free-text `spec_refs` ("CommonMark §6.7") become a resolvable structure —
test262's `esid`, WPT's `<link rel=help>` (lint-verified anchors), and BCD's
"Each URL must contain a fragment identifier" rule
(https://github.com/tc39/test262/blob/main/CONTRIBUTING.md,
https://web-platform-tests.org/writing-tests/lint-tool.html,
https://github.com/mdn/browser-compat-data/blob/main/schemas/compat-data-schema.md):

```yaml
cite: { spec: <catalog-id>, version: "…", url: "…#fragment",
        clause: "13.1.4", heading: "Character references",
        quote: "<≤1 normative sentence>", quote_sha256: "…" }
```

Citations resolve against the **pinned snapshot** in the spec knowledge base
(`specs/catalog.yaml` + `snapshots/` + `sections/`,
[format-maturity.md §2.4](./format-maturity.md)) via
`scripts/format-ops/check-citations.mjs` — never the live network. PDF-only
specs resolve by quote-hash. When a watch run detects a moved anchor, the
relocation proposal arrives as a diff (the webref raw→patch→curated pattern,
https://github.com/w3c/webref). The `cite.url` fragment doubles as the
retrieval anchor for generation (§10).

## 7. Expectations live outside the corpus

WPT's decisive pattern: implementation-specific status never enters the
shared suite; a parallel metadata tree carries it, expected-FAIL tests still
run and report, and maintenance is regenerate-then-review
(https://web-platform-tests.org/tools/wptrunner/docs/expectation.html).

```
core/formats/expectations/<impl>/<format>.yaml      # impl ∈ native, bridge-1.48.0, wasm, …
  QF7K: { status: FAIL, kind: bridge-gap, reason: "fprm not re-loaded over gRPC", issue: 530 }
  R2NM: { status: UNSUPPORTED, reason: "no standalone dispatch (subfilter)" }
```

`kapi spec-test --update-expectations <impl>` regenerates the tree from run
logs; git diff is the review surface. This replaces the in-corpus
`expected_fail` / `divergence_kind` / `bridge_only` fields and unifies the two
disjoint annotation planes (spec.yaml xfails and
`parity-annotations.yaml` severity/skip) under one divergence taxonomy.

## 8. Result taxonomy, meta-schema, accept mode

**Results** classify per the Unicode DDT verifier taxonomy
(https://github.com/unicode-org/conformance):

| Outcome | Meaning | Today's analog |
|---|---|---|
| Success | All applicable views match. | `pass` |
| Failure | A view mismatches with no expectations entry. | `fail` |
| Error | Crash, timeout, protocol violation. | read-error → `fail` |
| Unsupported | The implementation declares the case out of scope. | `skip`, `bridge_only`, subfilter skip |
| Known-divergence | Mismatch matching an expectations entry, with attributed `kind`. | `expected_fail`, `parity_warn` |

Known-divergence `kind` keeps the existing `divergence_kind` vocabulary
verbatim (spec.go:178–192): `native-bug` ("the neokapi reader is wrong —
should be ~0"), `bridge-gap`, `okapi-bug`, `scope-diff`, `default-diff`,
`missing-filter`, `fixture`, `contract`. Under the expectations model the
`kind` is **mandatory** on every FAIL entry — closing today's gap of 3
explicit kinds against 140+ xfails attributed only by contract-audit's text
heuristic. `parity_warn` disappears as a status: bridge↔native representation
divergence is a Known-divergence entry in the bridge tree, because the
blocks-view structural diff is the strict comparison by default.

**Meta-schema.** The case files validate against a JSON Schema
(`core/format/spec/case-schema.json`), gated in CI — the JSON Schema Test
Suite's `test-schema.json` pattern with DDT's three checkpoints: after
generation, before run, after run (result objects validate too)
(https://github.com/json-schema-org/JSON-Schema-Test-Suite/blob/main/test-schema.json).
This is the precondition for trusting generated cases. `Load()`/`Validate()`
retain today's semantic checks (unknown config keys, unknown variants, input
shape, duplicate names) and add case-ID format/uniqueness and content-hash
dedup (§10).

**Accept mode.** `kapi spec-test -u <format>` rewrites `expected.*` views
from current engine output; git diff is the review surface — tree-sitter's
`-u` made thousands of community grammars maintainable solo
(https://tree-sitter.github.io/tree-sitter/creating-parsers/5-writing-tests.html).
The guard rail comes with it: **accept mode refuses to rewrite `class:
invalid` cases and any `expected.error` view** (tree-sitter's rule that `-u`
never rewrites tests containing ERROR/MISSING nodes), so error expectations
cannot be silently clobbered by a regressed reader that now "accepts"
malformed input.

## 9. Versioning and imported suites

Format-version applicability uses two complementary mechanisms, both from
toml-test (https://github.com/toml-lang/toml-test):

- `since:`/`until:` bounds on the case (inline, for the simple majority);
- **per-format-version manifests** — plain case-ID lists
  (`cases-xliff-2.0`, `cases-xliff-2.1`, `cases-xliff-2.2`, the
  `files-toml-1.0.0` pattern) generated from the bounds — one corpus, several
  spec versions, no duplicated cases. The runner takes
  `kapi spec-test xliff --format-version 2.1`.

If the corpus is ever shared outside the repo, releases are immutable and
dated (yaml-test-suite `data-YYYY-MM-DD`,
https://github.com/yaml/yaml-test-suite).

**Wholesale imports** are first-class: CommonMark `--dump-tests` JSON,
toml-test, yaml-test-suite data releases, the JSON Schema suite, and the
XLIFF `test-suite/` are directly consumable today. Imported cases carry

```yaml
origin: { kind: suite, suite: commonmark, version: "0.31.2", upstream_id: "65" }
```

so re-imports are **idempotent**: the importer keys on
`(suite, version, upstream_id)`, updates in place, and never duplicates. The
upstream section/number rides into `cite` (CommonMark dumps carry
`{section, number}` per example).

## 10. The generation loop (`case-gen`)

The pipeline behind format-ops ritual 9, grounded in iPanda's
section-anchored RAG, DiffSpec's implementations-as-mutual-oracles, and the
finding that LLM-generated conformance cases reach >90% oracle conformance
only with machine validation in the loop
(https://arxiv.org/pdf/2507.00378, https://arxiv.org/pdf/2410.04249,
https://arxiv.org/html/2510.23350v1):

1. **Anchor** — pick a spec section: one clause file from
   `specs/sections/<spec>/<version>/<anchor>.md` (the knowledge-base
   retrieval unit; section-anchored retrieval beats embedding chunks on
   standards text).
2. **Generate** — candidate cases, **positive and negative**, each citing the
   anchor (`cite.url#fragment` = the section), schema-shaped.
3. **Validate** — meta-schema (§8 checkpoint 1) plus `Validate()` semantics;
   a hallucinated config key dies before any reader runs. **Content-hash
   dedup**: a hash over `(normalized input, config, operation)` across the
   format's corpus rejects candidates identical to existing cases.
4. **Adjudicate** — the differential oracle: execute against neokapi *and*
   okapi-bridge via the §4.3 shims. Both agree → candidate-pass (lands
   `informational` until promoted). Disagree → divergence triage (an
   expectations entry with `kind`, or a bug). Both reject → the candidate
   becomes a `class: invalid` case if the rejection matches the cited clause,
   else discarded.
5. **Review** — the human sees only the diffs: triage items and the
   accept-mode delta. Assertion values are *observed* (filled by the runner),
   never model-predicted.
6. **Account** — per-section case counts update the coverage stats
   (CommonMark's `-s` per-section pass percentages; WPT's heading-id
   directories) — the ledger's `case-gen.watermarks.per_section_coverage`
   and a Knowledge-axis signal: coverage gaps make the next anchor choice.

**Implementation entrypoints (the surface the ritual drives, #847).** Step 4
(adjudicate) is `spec.RunNativeCase(spec, example, mergedConfig, newReader)
→ spec.CaseResult{BlockEvents, Extracted, Parts, Err}`
(`core/format/spec/oracle.go`): the native side of the §4.3 shim. The parity
`ParityRunner` (`cli/parity/spec`, build tag `parity`) consumes the **same**
`Spec`/`Example` and can call `spec.DumpBlockEvents` on the bridge parts, so
the two `CaseResult` dumps compare structurally — both-agree → candidate-pass,
disagree → divergence triage, both-reject → promote to `class: invalid`. Step 3
(validate) is `spec.Load`/`Spec.Validate` (`core/format/spec/validate.go`, the
§8 meta-schema gate). Step 5 (review) uses accept-mode
(`KAPI_SPEC_UPDATE=1`, `spectest.UpdateBlocksFixture`) whose tree-sitter guard
rail (`spectest.RefuseAcceptForCase`) refuses invalid/error-class cases. The
external-port / WASM shim is the same two calls (registry-resolved reader →
`ReadParts` → `DumpBlockEvents`); a thin `kapi spec-dump` CLI is its intended
packaging and the remaining scoped-out piece.

Generated cases carry full provenance and a promotion ladder:

```yaml
origin: { kind: generated, model: "<model-id>", date: 2026-07-01,
          verified_by: differential-oracle }   # → human, on promotion
informational: true   # reports to the dashboard, cannot fail CI until promoted
```

## 11. Migration mapping

The current `spec.yaml` header survives unchanged; the case model replaces
the `Example` layer. Field-precise mapping from `core/format/spec/spec.go`:

| Current (spec.go) | New | Notes |
|---|---|---|
| `Spec.Format`, `Kind`, `MimeType`, `Description`, `Variants[]`, `Config[]` (`ConfigKey{key, type, default, description, applies_to, okapi_param}`) | unchanged | The format header is not the case model's concern. |
| `Feature{ID, Name, Description, AppliesTo, Config}` | **section** (same fields) | Sections group cases and carry shared config; section IDs key the coverage stats. |
| `Feature.OkapiRefs`, `NativeRefs` | unchanged, on the section | Drift checks (`detectSpecRefDrift`) unaffected. |
| `Feature.SpecRefs` (free text) | `cite:` per case (§6) | Free-text refs may remain on sections as prose; the resolvable citation lives on the case. |
| `Example.Name` | `case.name` | |
| — | `case.id` | New: stable 4–6 char, assigned at migration, never reused. |
| `Example.InputFile` / `InputXML` / `InputBytes` | `input: {file: \| inline: \| bytes:}` | `okapi:` scheme unchanged; `corpus:` scheme added. |
| `Example.Variant`, `Config` | unchanged | |
| `Example.BridgeOnly` | expectations entry `native: UNSUPPORTED` | Out of the corpus. |
| `Example.ExpectedFail` | expectations entry `FAIL (reason)` | Still runs and reports. |
| `Example.DivergenceKind` | expectations entry `kind:` | Same 8-value enum; now mandatory on FAIL. |
| `Example.ParityStrict` | retired | Blocks-view structural diff is strict by default; representation divergence is a Known-divergence entry. |
| `Example.Origin` (free text: `authored:` / `okapi-fixture:` / `real-world:`) | `origin: {kind, …}` structured | Three existing kinds map 1:1; `suite`, `bug`, `generated` added. |
| `Assertions` (7 fields) | `expected.extracted` | **Vocabulary unchanged and valid as-is** — every existing example becomes a `class: valid` case with one view; `EvalAssertions` (helpers.go:153) is that view's evaluator. |
| — | `class:` | All existing examples are `class: valid`. |
| — | `expected.blocks` / `roundtrip` / `valid_by` / `error` | New views, adopted per Engine level (§5). |
| — | `since` / `until`, `tags` | New. |

The migration is mechanical (a script emits IDs, wraps assertions, moves
xfails into expectations trees), which is what makes the wholesale change
safe; the legacy `formatSpecs` parity table and `fixtures_*_generated.go`
fold in as `origin: {kind: okapi-fixture}` informational cases
(format-maturity.md Open question 3).

## 12. Companion artifact: vocabulary evidence binding

The per-format `vocabulary.yaml` (Vocabulary axis,
[format-maturity.md §2.2](./format-maturity.md)) binds every fidelity claim to
passing case IDs via its `evidence:` field — claims-must-bind-to-tests, WPT
`WEB_FEATURES.yml` style
(https://github.com/web-platform-tests/wpt/blob/master/css/css-transforms/WEB_FEATURES.yml);
construct rows seed from ITS 2.0's data categories and the XLIFF 2.x
module/inline-code vocabulary (https://www.w3.org/TR/its20/,
https://docs.oasis-open.org/xliff/xliff-core/v2.2/xliff-extended-v2.2-part2.html),
with separate read/write cells (the pandoc #4301/#4303 asymmetry lesson,
https://github.com/jgm/pandoc/issues/4301) and partial-never-counts scoring
(Baseline rule,
https://github.com/web-platform-dx/web-features/blob/main/docs/baseline.md).
The three nested round-trip evidence tiers map exactly onto this document's
views: text-content equality (`expected.extracted`) ⊂ canonical
construct-dump equality (`expected.blocks` — the ITS test-suite normalization
trick, https://github.com/w3c/its-2.0-testsuite) ⊂ byte equality
(`expected.roundtrip: byte_exact` — SCORE-Bench's "distinguish legitimate
representational variation from actual data loss",
https://unstructured.io/blog/introducing-score-bench-an-open-benchmark-for-document-parsing).
A vocabulary cell citing a case ID that asserts only the weaker tier does not
support the stronger claim — the audit resolves evidence at view granularity.
