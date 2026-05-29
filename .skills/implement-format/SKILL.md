---
name: implement-format
description: >-
  Implement a new neokapi document format (reader + writer + config + tests +
  registration), or port an Okapi Java filter, end to end. Use when the user
  asks to "add a format", "implement a format reader/writer", "port the okf_X
  filter", "support .XYZ files", or "extract translatable text from <file
  type>". NOT for tweaking an existing format (edit it in place) and NOT when an
  existing configurable JSON/YAML/XML reader can already cover the need —
  configure that instead.
---

# Implement a Format

Add a new format under `core/formats/<id>/` that faithfully extracts
translatable content as `model.Block`s of `model.Run`s and round-trips
everything else. Hold yourself to the maturity bar in
[`docs/internals/format-maturity.md`](../../docs/internals/format-maturity.md);
the architecture this skill assumes is in
[`docs/internals/format-engineering.md`](../../docs/internals/format-engineering.md).

## When to use

Use this skill to create a brand-new format package. Do **not** use it to:

- tweak an existing format → edit that package directly;
- handle a JSON/YAML/XML shape that the existing `json`/`yaml`/`xml` readers can
  match via config → configure the reader (see the
  `prefer-configured-readers` principle), don't fork a new format;
- emit a runtime/export artifact → that is a registered `DataFormatWriter` or a
  `//go:generate` step, not a new format and not a script.

## Step 0 — Port or harvest?

`grep`/`ls /Users/asgeirf/src/okapi/Okapi/okapi/filters/` for a matching
`okf_<format>` filter.

- **Found → port.** `spec.yaml` + parity are **mandatory**. The Okapi filter is
  ground truth; the format spec wins ties.
- **None → harvest** (e.g. a modern web/mobile catalog format). `spec.yaml` and
  parity are N/A. You instead owe `okapi_skip_test.go` (prose: why no
  counterpart) + `invariants_test` + `corpus_test` + (for catalogs) a tool-gated
  `acceptance_test`. Mirror `core/formats/xcstrings/` as the exemplar.

## Step 1 — (Port only) read the Okapi filter first

Read, in `okapi/filters/<format>/`:

- `<Format>Filter.java` — the `IFilter` event walk and the `GenericSkeleton`
  construction.
- `Parameters.java` — every key, the `reset()` defaults, and any
  `InlineCodeFinder` rules (properties ships **4** default rules; reproduce them
  or `getCodes().size()` parity breaks — code-count parity is load-bearing).
- `<Format>FilterTest.java` `@Test` methods + `src/test/resources/` fixtures.
  Note the exact assertions (block counts, code counts) — these become your
  `spec.yaml` examples and `okapi_refs`.

See the Okapi→Run mapping in
[format-engineering.md §6](../../docs/internals/format-engineering.md#6-okapi-mapping).

## Step 2 — Scaffold the package

Follow the **published** tutorial `web/docs/docs/contribute/formats.md` (it shows
`Signature`/`Open`/`Read`/`Close`; the internal `implementing-formats.md` note
omits them and won't compile). Create `core/formats/<id>/{reader,writer,config}.go`,
name == id. Constructors set the `BaseFormatReader`/`BaseFormatWriter` fields,
keep a typed `cfg` alias, and call `cfg.Reset()`. Exemplars: a configurable
ported format → `core/formats/properties/`; a harvest format → `core/formats/xcstrings/`.

## Step 3 — Reader

`Signature()` → the `FormatSignature` (must match what `register.go` advertises).
`Open` only validates `doc != nil && doc.Reader != nil` and stashes `r.Doc`.
`Read` makes a cap-64 channel, spawns a goroutine that `defer`-closes it and
emits `PartLayerStart` → `PartBlock`/`PartData` → `PartLayerEnd`, using
`select(ch<-, ctx.Done())`. **Parse errors go on the channel as
`PartResult{Error}`, never returned from `Open`.** Linearize inline markup into
`Ph`/`PcOpen`+`PcClose` runs with verbatim `Data` and shared string IDs — never
leave markup in the text.

## Step 4 — Pick ONE round-trip strategy

1. **SkeletonStore** (coalescing buffer in the reader, `writeFromSkeleton` in the
   writer) — best for line/markup formats.
2. **Original-bytes + re-tokenize-and-splice** — best for structured catalogs
   (json/arb/xcstrings).
3. **DOM re-serialize** — accepts documented, spec-justified normalization
   (not byte-exact).

The writer resolves target-else-source and renders via
`model.RenderRunsWithData(runs)` (never `SourceText()`). **Never regex- or
byte-rewrite serialized output to patch a modeling gap** — fix the model or
canonicalize symmetrically in the parity comparator. The single sanctioned
exception (reproducing an Okapi byte transform on opaque bytes) must cite the
mirrored Java class/method.

## Step 5 — Config + schema

`ApplyMap` must **reject unknown keys** (a `default:` branch that errors, or
`format.ApplyMapViaJSON` with `DisallowUnknownFields`). Add `schema.go` so the
format surfaces in the CLI, UI, and reference docs.

## Step 6 — Register & wire

- `core/formats/register.go`: `RegisterReader` + `RegisterWriter` +
  `registerSchemaAndDecoder`.
- `ids.go`: add the id constant.
- `register_test.go`: update all **three** expected lists (the length assert
  fails *without naming* the missing format).
- Detection: for json/xml/zip overlap use a `Sniff` or a unique extension only;
  niche JSON formats must **not** advertise `application/json`.

## Step 7 — (Port only) spec.yaml + parity

- `spec.yaml`: `format: okf_<id>`, keys **1:1** with `ApplyMap` (+ `okapi_param`),
  features with `okapi_refs`/`native_refs`/`spec_refs`, ≥1 example with ≥1
  assertion that reflects **native** behavior. Prefer `input_file: okapi:...`
  real fixtures for binary formats. Do **not** declare inline-code config keys
  (contract-audit can't follow `$ref` into `$defs` → false drift).
- `spec_test.go` wires `spec.NativeRunner`.
- `transform.go`: register the Okapi config-kind transform in `init()`.
- `cli/parity/formats/<id>_spec_test.go`: the `ParityRunner` over the same
  `spec.yaml`; add `<id>_bridge_config.go` if the bridge's param shape differs.
- Run it: `make parity-sandbox` then
  `cd cli && go test -tags parity -run TestParity<Id>Spec ./parity/formats/`.
- A **pure default-only** native-vs-bridge difference is **not** an
  `expected_fail` — converge it with explicit config in `bridge_config`. Every
  real `expected_fail` gets a non-`default-diff` `divergence_kind` citing spec +
  Okapi.

## Step 8 — Tests (the maturity ladder)

Minimum for L1→L2: `reader_test`, `roundtrip_test` (or `skeleton_test`) that
**actually asserts byte/semantic equality** (not just "no error"), and a
`malformed_test` that asserts a clean `Error` + `NotPanics` on truncated/garbage
input (run with `-race`). Add `corpus_test`/`upstream_test` on real files;
`invariants_test`/`acceptance_test` for catalogs (`//go:build acceptance`, and
**SKIP — never FAIL — when the external validator is absent**). Import helpers
from `core/internal/testutil` (not `core/testutil`).

## Step 9 — Generate & verify

`make kapi-i18n-generate` and `make generate-reference-docs` (the `nativedocs`
sidecar must be named exactly the id — a typo silently no-ops). Then verify:
`go build ./...`, `make test`, the parity run, and that dashboards regenerate
cleanly. Confirm the target level against
[format-maturity.md](../../docs/internals/format-maturity.md) and state which
level you reached.

## Footguns

- `register_test.go` length asserts fail **without naming** the missing format —
  if it fails after you add a format, you forgot one of the three lists.
- A **nil config** silently skips schema + decoder registration.
- `spec.yaml` is **not gated by any test** unless `spec_test.go` exists — an
  untested spec rots.
- arb/i18next/xcstrings must **not** advertise `application/json` or a shared
  extension (it steals detection from `json`).
- Synthetic binary fixtures cause false bridge-bug null-derefs (#482) — prefer
  `okapi:` real fixtures.
- xliff2's DOM writer is **intentionally** non-byte-exact (#560); don't "fix" it
  to byte-equal.
- golangci-lint under-reports without `icu4c` on `PKG_CONFIG_PATH`; parity needs
  the `fts5` tag + the sandbox.

## References

- Architecture, spec grammar, parity, Okapi mapping, principles:
  [`docs/internals/format-engineering.md`](../../docs/internals/format-engineering.md)
- The bar + audit rubric:
  [`docs/internals/format-maturity.md`](../../docs/internals/format-maturity.md)
- Published tutorial: `web/docs/docs/contribute/formats.md`
- Exemplars: `core/formats/properties/` (ported, configurable),
  `core/formats/xcstrings/` (harvest, fullest test set)
- Round-trip harness: `docs/internals/roundtrip-testing.md`
