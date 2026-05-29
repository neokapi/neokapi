# Format Maturity — Rubric, Levels & Audit

This is the bar a neokapi format must clear, and how to measure where a format
sits today. It is the guardrail companion to
[format-engineering.md](./format-engineering.md) (how the engine works) and is
the contract the `implement-format` and `refresh-format-maturity` skills
(`.skills/`) operate against.

Two questions this document answers:

1. **Building** — what must be true before a new format is considered done?
2. **Auditing** — given an existing format, what level is it, and what is the
   ranked list of gaps to the next level?

## Maturity levels

A format sits at exactly **one** level: the highest level whose criteria are
*fully* met. A missing lower-tier requirement caps the level regardless of how
much higher-tier work exists.

| Level | Name | Entry criteria |
|---|---|---|
| **L0** | Experimental | Reader compiles and emits `LayerStart → Block → LayerEnd` for the happy path; registered in `register.go` + `ids.go` + the `register_test` lists so `make test` passes. May lack a writer or config validation. No round-trip guarantee, no `spec.yaml`, no parity. Use only behind an explicit experimental label. |
| **L1** | Readable + writable | L0 **plus**: writer applies target-else-source via `RenderRunsWithData`; one declared round-trip strategy; a `reader_test` **and** a `roundtrip_test` (or `skeleton_test`) prove read→write fidelity for core cases; `Config` rejects unknown keys; inline codes preserved as runs. |
| **L2** | Specified | L1 **plus**: `spec.yaml` + `spec_test.go` driving `spec.NativeRunner` green (keys 1:1 with `ApplyMap`, `okapi_param` recorded, `spec_refs`) **OR**, for a harvest format with no Okapi counterpart, `okapi_skip_test.go` + `invariants_test` + `corpus_test` in its place; `schema.go` present; a `malformed_test` asserts clean errors without panic. |
| **L3** | Parity-verified | L2 **plus** (Okapi counterpart): `cli/parity/formats/<id>_spec_test.go` passes head-to-head (`bridge_config` where param shapes differ); every divergence is an `expected_fail` with an explicit non-`native-bug` `divergence_kind` grounded in spec + Okapi citations (zero pure `default-diff` xfails); a corpus/upstream test exercises real files; reference docs + `nativedocs` sidecar + `metadata.json` wired with no drift. |
| **L4** | Rock-solid | L3 **plus**: byte-faithful round-trip proven across an edge-case matrix (encodings, line endings, unicode, malformed-but-recoverable) **and** over a real-world corpus; `schema_test` asserts schema == config struct; `transform_test` where a transform exists; bench/perf for perf-sensitive formats; zero `native-bug` xfails; any remaining divergence is a tracked, attributed, spec-justified faithful-class item; parity / contract-audit dashboards green and freshly regenerated. |

> **Harvest formats** (no Okapi counterpart: androidxml, applestrings, arb,
> designtokens, i18next, mdx, resx, xcstrings) cannot reach L3/L4 via the parity
> bridge. Their ceiling is defined by the L2-substitute path (okapi_skip +
> invariants + corpus + acceptance). Whether to also put them on the native-only
> `spec.yaml` engine is an [open question](#open-questions).

## The rubric (9 dimensions)

Score each dimension `none` / `partial` / `complete` / `na`. Weight reflects how
much a gap there undermines "rock-solid."

| # | Dimension | Weight | What "complete" means | How to check |
|---|---|---|---|---|
| 1 | **Reader** | critical | `Signature`/`Open`/`Read`/`Close`; streams `LayerStart→Block/Data→LayerEnd` on a cap-64 channel; parse errors as `PartResult.Error` (not an `Open` return); inline non-translatable spans become `Ph`/`PcOpen`/`PcClose` runs with verbatim `Data`. | `reader.go` exists; `go test ./core/formats/<id>/ -run TestRead`; grep `reader.go` for the cap-64 channel + `PartLayerStart`; `reader_test` asserts `SourceRuns()` / code preservation. |
| 2 | **Writer / round-trip fidelity** | critical | Renders via `RenderRunsWithData`, target-else-source resolution, reconstructs unchanged content byte-exact (or documented spec-justified normalization); one declared round-trip strategy. | `writer.go` exists; `go test ./core/formats/<id>/ -run 'TestRoundTrip\|TestByteFaithful\|TestSkeleton'`; **read the assertions** — do they prove byte/semantic equality or merely "no error"? normalization commented + justified. |
| 3 | **Config + Schema** | high | `DataFormatConfig` with `ApplyMap` rejecting unknown keys; `schema.go` for CLI/UI/reference docs; ideally a `schema_test` asserts schema matches the struct. | `config.go` has `FormatName`/`Reset`/`Validate`/`ApplyMap` with an unknown-parameter default branch; `find -name schema.go`; full credit needs `schema_test.go` (only html/json today). |
| 4 | **spec.yaml** | high | Executable spec: `format` (`okf_` id), `mime_type`, keys 1:1 with `ApplyMap` + `okapi_param`, features with `okapi_refs`/`native_refs`/`spec_refs` and ≥1 example with ≥1 assertion reflecting native behavior. | `find -name spec.yaml`; `go test ./core/formats/<id>/ -run TestSpec`; verify `okapi_param` + `spec_refs`. N/A only when there is no Okapi counterpart (then `okapi_skip_test.go`). |
| 5 | **Parity coverage** | high | `cli/parity/formats/<id>_spec_test.go` runs the same `spec.yaml` through native **and** the okapi-bridge daemon; `bridge_config` where param shapes differ; divergences tracked with `expected_fail` + a non-`default-diff` `divergence_kind`. | `ls cli/parity/formats/<id>_spec_test.go`; `make parity-sandbox` then `cd cli && go test -tags parity -run TestParity<Id>Spec ./parity/formats/`; no pure default-diff xfails. N/A for harvest. |
| 6 | **Malformed / robustness** | high | Broken/truncated input yields a clean `Error` on the channel with `NotPanics`; nil doc rejected by `Open`. | `find -name malformed_test.go`; run with `-race`; only arb/resx/xcstrings have one today, so absence is a real gap. |
| 7 | **Corpus breadth** | medium | Real-world/upstream files (not just synthetic) for stable counts + byte-exact round-trip; prefer Okapi upstream fixtures for binary formats. | `find -name corpus_test.go -o -name upstream_test.go`; `testdata/` has real files or `spec.yaml` uses `input_file: okapi:...`; corpus test green. |
| 8 | **Detection** | medium | `FormatSignature` detects without stealing a generic format's ext/MIME; collisions handled via `Sniff` or unique-extension-only; distinct MIME for ZIP containers. | `register.go` `RegisterReader` + reader `Signature()` in sync; for json/xml/zip overlap confirm a `Sniff` or no shared MIME; `register_test` detection cases pass. |
| 9 | **Docs + reference wiring** | medium | `ids.go` constant, `register_test` lists updated, `metadata.json` regenerated, reference-data generated with a correctly-named `nativedocs` sidecar, `transform.go` if Okapi-mapped. | grep `ids.go`; `go test ./core/formats/ -run TestRegister`; `make kapi-i18n-generate` + `make generate-reference-docs` produce no git diff; sidecar filename == id exactly. |

## How to score a format (audit procedure)

The `refresh-format-maturity` skill automates this. By hand:

1. **Identify** `core/formats/<id>/` and whether it has an Okapi counterpart
   (`ls /Users/asgeirf/src/okapi/Okapi/okapi/filters/`). Exclude
   `exec`/`jsx`/`memorytest`.
2. **Score the 9 dimensions** by file existence **and targeted test runs** —
   open the `*_test.go` and judge whether assertions truly prove fidelity, not
   just absence of error. Run `go test ./core/formats/<id>/...` and, if parity
   applies, `make parity-sandbox` + the tagged parity test.
3. **Assign L0–L4** from the strictest unmet criterion; list the missing items
   blocking the next tier, ranked by rubric weight.
4. **Audit divergences:** open `spec.yaml` + `parity-annotations.yaml`; for every
   `expected_fail` confirm (a) `divergence_kind` is set, (b) it is **not** a
   `native-bug` (if it is — fix, don't document), (c) it is **not** a pure
   `default-diff` (converge instead), (d) it cites spec + Okapi. Flag any
   `expected_fail` whose runner now logs "assertions pass."
5. **Check the Okapi tracker** for upstream fixes since the pinned 1.48.0 — see
   the issue-tracker recipe in
   [format-engineering.md §6](./format-engineering.md#6-okapi-mapping).
6. **Drift:** `make contract-audit` for the filter; confirm dashboard caches are
   fresh (regenerate — a regression can hide behind a stale cache).

## Maturity scoring quick-reference (file signals)

A coarse first pass from file presence alone (deeper scoring still requires
reading assertions):

```
L1 floor : reader.go + writer.go + config.go(ApplyMap rejects unknown) + (roundtrip_test|skeleton_test)
L2 floor : + (spec.yaml+spec_test) OR (okapi_skip_test+invariants_test+corpus_test) ; + schema.go ; + malformed_test
L3 floor : + cli/parity/formats/<id>_spec_test.go ; + corpus_test|upstream_test ; + reference wiring
L4 floor : + edge-case matrix + schema_test + zero native-bug xfails + fresh dashboards
```

<!-- BEGIN: gap-analysis report (generated) -->
## Maturity report (snapshot: 2026-05-30)

Point-in-time scoring of all 49 real formats against the rubric above, one
analysis agent per format reading the package, its tests' assertions, its
`spec.yaml`/parity wiring, and its Okapi counterpart. Regenerate it with the
`refresh-format-maturity` skill (per format) or the `format-maturity-gap-analysis`
workflow (all at once). `splicedlines` was hand-scored (its agent did not return
structured output).

**Distribution: L0: 1 · L1: 28 · L2: 7 · L3: 13 · L4: 0 (n=49).**

### Headline

- **No format is L4 (rock-solid).** The best formats are L3 — either
  parity-verified against the Okapi bridge (html, openxml, xliff, regex, csv,
  xml-stream) or harvest formats with the full self-contained ladder
  (xcstrings, arb, resx, androidxml, applestrings, designtokens, i18next, mdx).
  None clear the L4 bar (edge-case matrix + `schema_test` + zero `native-bug`
  xfails + fresh dashboards).
- **The two generations have complementary, non-overlapping strengths.** The
  *harvest* cohort all reached L3 on robustness + corpus + invariants but has no
  cross-implementation check. Much of the older *parity* cohort (json,
  properties, yaml, ts, idml, mif, …) is stuck at **L1** despite the okapi-bridge
  cross-check, because it lacks `malformed_test` and (often) `schema.go`. Raising
  the floor means giving each generation what the other already has.
- **The single highest-leverage gap is `malformed_test`.** 45/48 analyzed
  formats were flagged for robustness gaps; it is the L1→L2 blocker for ~24
  formats. (The `TestRobustnessCoverage` advisory in
  `core/formats/maturity_test.go` tracks the live count — 46/49.)

### Per-format levels

| Format | Level | Type | Top gap to next level |
|---|---|---|---|
| `mo` | L0 | harvest | `Config.ApplyMap` does not reject unknown keys (no-op) + no fidelity/malformed test — genuinely thin |
| `doxygen` | L1 | parity | add `malformed_test.go` |
| `dtd` | L1 | parity | add `malformed_test.go` |
| `epub` | L1 | parity | add `malformed_test.go` (corrupt ZIP / missing container.xml) |
| `fixedwidth` | L1 | parity | add `malformed_test.go` |
| `icml` | L1 | parity | add `malformed_test.go` |
| `idml` | L1 | parity | add `schema.go` + `malformed_test.go` |
| `json` | L1 | parity | add `malformed_test.go` |
| `messageformat` | L1 | parity | add `malformed_test.go` |
| `mif` | L1 | parity | add `malformed_test.go` |
| `mosestext` | L1 | parity | add `malformed_test.go` |
| `odf` | L1 | parity | add `schema.go` + `malformed_test.go` |
| `paraplaintext` | L1 | parity | add `malformed_test.go` |
| `phpcontent` | L1 | parity | add `malformed_test.go` |
| `plaintext` | L1 | parity | add `malformed_test.go` |
| `properties` | L1 | parity | add `malformed_test.go` |
| `rtf` | L1 | parity | add `malformed_test.go` |
| `splicedlines` | L1 | internal | add `schema.go` + `malformed_test.go` (has spec+parity) |
| `tex` | L1 | parity | add `malformed_test.go` |
| `transtable` | L1 | parity | add `malformed_test.go` |
| `ts` | L1 | parity | add `malformed_test.go` |
| `ttml` | L1 | parity | add `malformed_test.go` |
| `ttx` | L1 | parity | add `malformed_test.go` (UTF-16/BOM cases) |
| `txml` | L1 | parity | add `schema.go` + `malformed_test.go` |
| `versifiedtext` | L1 | harvest | add `malformed_test.go` |
| `vignette` | L1 | parity | add `schema.go` |
| `vtt` | L1 | parity | make `spec.yaml` keys 1:1 with `ApplyMap`; add `malformed_test.go` |
| `xliff2` | L1 | parity | **writer RED**: `TestRoundTrip_ByteEqualUntouched` fails on 22 upstream fixtures (#560) |
| `yaml` | L1 | parity | add `malformed_test.go` |
| `markdown` | L2 | parity | add `malformed_test.go`; parity for L3 |
| `pdf` | L2 | read-only | add `malformed_test.go` (read-only: writer N/A) |
| `po` | L2 | parity | add `malformed_test.go` |
| `srt` | L2 | parity | add `malformed_test.go`; close the parity gap or mark N/A |
| `tmx` | L2 | parity | convert non-asserting `TestInvalidXml` to assert; parity |
| `wiki` | L2 | parity | add `malformed_test.go` |
| `xml` | L2 | parity | add `malformed_test.go` |
| `androidxml` | L3 | harvest | `schema_test` + encoding/CRLF/BOM edge-case matrix (L4) |
| `applestrings` | L3 | harvest | `schema_test` + edge-case matrix (L4) |
| `arb` | L3 | harvest | `schema_test` (L4) |
| `csv` | L3 | parity | add `malformed_test.go` (L4 robustness) |
| `designtokens` | L3 | harvest | encoding/CRLF/BOM edge-case matrix (L4) |
| `html` | L3 | parity | dedicated `malformed_test.go` (L4) |
| `i18next` | L3 | harvest | `schema_test` (L4) |
| `mdx` | L3 | harvest | `schema_test` (L4) |
| `openxml` | L3 | parity | `malformed_test.go` (corrupt ZIP); RunFonts xfail |
| `regex` | L3 | parity | `malformed_test.go` (uncompilable pattern) |
| `resx` | L3 | harvest | encoding edge-case matrix (L4) |
| `xcstrings` | L3 | harvest | byte-fidelity edge-case matrix (L4) |
| `xliff` | L3 | parity | `malformed_test.go` (L4) |

### Systemic gaps (frequency across the 48 analyzed)

| Theme | Formats flagged | Remediation |
|---|---|---|
| Malformed / robustness untested | 45/48 | a `malformed_test.go` asserting clean `Error` + `NotPanics` on truncated/garbage/nil input, run with `-race` |
| Byte-faithful round-trip not fully proven | 43/48 | round-trip tests that assert byte/semantic **equality**, not "no error" |
| Encoding / BOM / CRLF blind spot | 34/48 | an edge-case fixture matrix (non-UTF-8, BOM, CRLF, all-Unicode) |
| Schema drift (no `schema_test`) | 34/48 | a `schema_test` asserting `Schema()` keys == `ApplyMap` keys (only html/json have it) |
| Synthetic-only corpus | 25/48 | vendor/upstream real files (or `spec.yaml input_file: okapi:…`) |
| Detection collision concerns | 24/48 | confirm `Sniff`/unique-ext; niche JSON must not advertise `application/json` |
| xfail / default-diff hygiene | 23/48 | converge default-only diffs; attribute every `expected_fail` |

### Prioritized remediation roadmap

1. **Fix the two specific defects first.** `mo` (L0): make `ApplyMap` reject
   unknown keys and add a fidelity test, or formally retire it as a stub.
   `xliff2`: the writer's `TestRoundTrip_ByteEqualUntouched` is **RED** on 22
   upstream fixtures (#560) — resolve or formally classify as a tracked
   faithful-class divergence.
2. **Robustness wave (biggest lever).** Add `malformed_test.go` across the L1/L2
   formats. This alone lifts ~24 formats off the L1 floor and closes the largest
   systemic gap. The `implement-format` skill now makes this mandatory for new
   formats, and `core/formats/maturity_test.go` tracks the burn-down.
3. **Schema wave.** Add `schema.go` where missing (idml, odf, txml, vignette,
   splicedlines, …) and a reusable `schema_test` helper asserting
   schema==`ApplyMap`; adopt it across the 34 schema-drift formats.
4. **Fidelity & encoding wave.** Promote re-parse/"no-error" round-trip tests to
   byte/semantic equality, and add the encoding/BOM/CRLF edge-case matrix —
   the remaining L3→L4 work for the strong formats.
5. **Corpus & xfail hygiene.** Replace synthetic-only corpora with real files;
   sweep `parity-annotations.yaml` for stale/unattributed/default-diff xfails
   (use the `refresh-format-maturity` skill per format).
<!-- END: gap-analysis report -->

## Open questions

These are genuine design decisions that shape where the bar sits. They are
recorded here rather than silently resolved:

1. **Harvest formats and `spec.yaml`.** Should the 8 harvest formats
   (androidxml/applestrings/arb/designtokens/i18next/mdx/resx/xcstrings) be put
   on the native-only `spec.yaml` engine (no parity bridge) so the
   single-source-of-truth + contract-audit coverage extends to them — or is the
   separate acceptance/corpus/invariants taxonomy their intended permanent
   shape? This decides whether L2 *requires* `spec.yaml` for harvest formats.
2. **Byte-exactness at L4.** Is byte-exact round-trip a hard L4 requirement, or
   is xliff2's intentionally-normalizing DOM writer (idempotent but not
   byte-equal, ~21 tracked fails #560) an accepted L4-compatible faithful-class
   case? This rubric currently treats faithful-class divergence as
   L4-compatible.
3. **Retiring the legacy `formatSpecs` parity table.** Should
   `cli/parity/formats/spec.go` be retired in favor of the `spec.yaml`-driven
   `ParityRunner` everywhere, or do they intentionally coexist? Affects how
   Parity coverage is scored where both exist with conflicting skips.
4. **Round-trip assertions in the spec vocabulary.** Worth adding a
   round-trip/writer-output assertion type to the spec `Assertions` vocabulary
   (promised in spec headers but unimplemented), or is delegating round-trip to
   native `TestRoundTrip_*` the intended permanent split?
5. **Reporting denominator.** Is maturity reported over 49 real formats, or all
   52 dirs including `exec`/`jsx`/`memorytest`?
6. **Step parity.** Step parity is stability-only (~120 steps) with no
   head-to-head correctness and a hand-pinned 1.48.0 step list — is investing in
   real step-parity in scope?
