# Format Engineering in neokapi — Knowledge Base

A neokapi format faithfully re-implements an Okapi (Java) filter: it extracts
translatable content as `model.Block`s of `model.Run`s, and round-trips
everything else byte-for-byte (or with explicitly documented, spec-justified
normalization). This document is the consolidated reference for *how the format
engine works* and *how to build and wire a format*. For the bar a format must
clear and how to audit it, see [format-maturity.md](./format-maturity.md). All
paths are relative to the repo root.

## Where the other docs live (read these too)

This hub deliberately cross-links rather than restates. The canonical companions:

| Topic | Document |
|---|---|
| Step-by-step "add a format" tutorial (reader/writer/Run handling) | `web/docs/docs/contribute/formats.md` (published, correct) |
| Format-system architecture decision | `web/docs/docs/contribute/architecture/005-format-system.md` |
| Bilingual format interop | `web/docs/docs/contribute/architecture/017-bilingual-format-interop.md` |
| Parity testing decision | `web/docs/docs/contribute/architecture/018-parity-testing.md` |
| Round-trip / three-engine test harness | `docs/internals/roundtrip-testing.md` |
| General testing conventions | `docs/internals/TESTING.md` |
| Interface signatures | `docs/internals/INTERFACES.md` |
| Skeleton store binary format | `web/docs/docs/contribute/notes-internal/skeleton-store.md` |
| Maturity rubric + levels + audit procedure | `docs/internals/format-maturity.md` |

> The older `web/docs/docs/contribute/notes-internal/implementing-formats.md`
> note carries valuable skeleton-store/writer-fallback detail but its reader
> snippet omits the three base-not-provided methods (`Signature`/`Open`/`Close`);
> treat `contribute/formats.md` as the authoritative tutorial.

## 1. Architecture & content model

Package `core/formats/<id>/` (directory name == format id) implements two
framework interfaces:

- `DataFormatReader` (`core/format/reader.go`): `Name` / `DisplayName` /
  `Signature()` / `Open(ctx, *RawDocument)` / `Read(ctx) <-chan PartResult` /
  `Close` / `Config` / `SetConfig`.
- `DataFormatWriter` (`core/format/writer.go`): `Name` / `SetOutput` /
  `SetOutputWriter` / `SetLocale` / `SetEncoding` / `Write(ctx, <-chan *Part)` /
  `Close`.

Concrete types embed `BaseFormatReader` / `BaseFormatWriter`
(`core/format/base_reader.go`, `base_writer.go`) for the boilerplate getters and
file plumbing. **`Signature()` is the one reader method the base does NOT
provide.** A reader therefore implements only `Signature`/`Open`/`Read`/`Close`;
a writer only `Write` (and `Close` if it needs cleanup).

**Streaming.** `Read(ctx)` returns a cap-64 buffered channel filled by a
goroutine that `defer`-closes it. It emits `PartLayerStart{*Layer}` (layer ID
`"doc1"`; multilingual catalogs set `Layer.IsMultilingual=true`) → one
`PartBlock` / `PartData` / `PartMedia` per item → `PartLayerEnd`. Cancellation is
a `select(ch<-, ctx.Done())`. **Parse errors go on the channel as
`PartResult{Error: ...}`, not returned from `Open`.** `Open` only validates
`doc != nil && doc.Reader != nil` and stashes `r.Doc`. (Consequence: a caller
that ignores `PartResult.Error` treats malformed input as empty — robustness
tests must assert the error is surfaced.)

**Content model** (`core/model/`):

- `Layer` — structural grouping; nests for embedded content.
- `Block` — `Source []Run`, `Targets map[VariantKey]*Target`, `Properties`,
  `Annotations`, stand-off `Overlays`. There is **no structural `Segment`** —
  segmentation is an opt-in overlay (AD-002).
- `Run` — a union: `Text` / `Ph` / `PcOpen`+`PcClose` (paired by a shared string
  ID) / `Sub` (→ child `Block`) / `Plural` / `Select`.

Inline markup lives **in runs** (verbatim in `Run.Data`), not in the text, so it
survives translation. Writers render with `model.RenderRunsWithData(runs)`,
never `SourceText()` (which drops codes). Notes land in
`block.Annotations["note"]`. Formats populate `Block` fields directly — they do
**not** use tool capability views (`Annotate`/`Translate`/`Transform`, AD-006);
those are for tools, not formats. Writer value-resolution, shared by every
writer: if `w.Locale` is set **and** `block.HasTarget(w.Locale)` →
`TargetRuns(w.Locale)`, else `SourceRuns()` / raw.

## 2. Canonical file layout

Minimum: `reader.go`, `writer.go`, `config.go` (name == id). Of 52 dirs, ~49 are
real formats — `exec`, `jsx` (klf-rename alias stub), and `memorytest` are
thin/internal.

| File | Responsibility |
|---|---|
| `reader.go` | `Reader{BaseFormatReader + *Config + state}`; `NewReader()` seeds base fields + a `cfg` alias + `cfg.Reset()`. |
| `writer.go` | Drains parts into `map[blockID]*Block` (capturing the root `Layer` / original bytes), reconstructs on channel close. |
| `config.go` | `DataFormatConfig` (`FormatName`/`Reset`/`Validate`/`ApplyMap`); `ApplyMap` is a key switch whose `default` **rejects unknown keys** (or `format.ApplyMapViaJSON` with `DisallowUnknownFields`). |
| `schema.go` (27 formats) | `Schema() *FormatSchema` with `Groups` + `Properties` + conditional `Visible(ConditionExpr)`. Feeds CLI introspection, UI forms, and generated reference docs. |
| `transform.go` (14 formats) | Okapi config-kind transformer registered in `init()`; maps Okapi param names → native config keys. Only formats with an Okapi-bridge counterpart. |
| `spec.yaml` + `spec_test.go` (41) | Executable spec — see §3. |
| `scanner.go`/`parse.go`/`catalog.go`/`encode.go`/`rewrite.go`/`path.go` | The byte-faithful tokenizer family used by re-tokenize formats (xcstrings/arb/json). |

**Three round-trip strategies (the main axis of divergence):**

1. **SkeletonStore binary stream** (`core/format/skeleton.go`: `[type:1B][len:4B
   BE][data]` of `SkeletonText`/`SkeletonRef`/`SkeletonLang`; reader implements
   `SkeletonStoreEmitter`, writer `SkeletonStoreConsumer`) — properties, html
   (token), xliff2 (streaming), openxml, idml.
2. **Original-bytes-on-layer + re-tokenize-and-splice** — xcstrings (via
   `unsafe.String` sharing the read buffer), arb, json.
3. **DOM patch / re-serialize** — xliff2's **default** etree writer
   *intentionally* normalizes formatting / attribute order / namespaces
   (idempotent but **not** byte-equal, ~21 tracked fails, #560); html's DOM path
   when no skeleton. openxml additionally strips WML elements via regex on
   serialized bytes (fragile — the source of its remaining bugs).

## 3. Config / Schema / spec.yaml wiring

- **Config** = the runtime surface.
- **`schema.go`** = presentation metadata → CLI introspection, UI forms, and
  generated reference docs (`make generate-reference-docs` →
  `packages/reference-data/data` + a `nativedocs` sidecar named **exactly** the
  id; a typo silently no-ops).
- **`spec.yaml`** (`core/format/spec/spec.go`) = the single-source-of-truth
  executable description.

**`spec.yaml` grammar:** `format` (`okf_` id, required), `kind`
(`top_level`|`subfilter`), `mime_type`, `description`, `variants`, `config[]`,
`features[]` (required). A `ConfigKey{key, type, default, description,
applies_to?, okapi_param?}` maps its `key` **1:1** to an `ApplyMap` key;
`okapi_param` is the upstream Java field name (used for drift detection). A
`Feature{id, name, description, config?, examples (≥1), okapi_refs?
(Class#method), native_refs? (pkg.TestFunc), spec_refs? (RFC/W3C/Java-SE)}`. Each
example carries one input (`input_xml` inline / `input_file` relative or
`okapi:<path>` / `input_bytes` base64) plus `variant`, `bridge_only`,
`expected_fail` (downgrades a failure to a log; warns if it *unexpectedly
passes*), `divergence_kind`
(`native-bug`|`bridge-gap`|`okapi-bug`|`scope-diff`|`default-diff`|`missing-filter`|`fixture`|`contract`),
`parity_strict`, `origin`, and inline `Assertions` (`block_count`/`_min`/`_max`,
`first_block_text`, `block_texts` exact-ordered, `has_block_with_text`
substring, `no_block_with_text`). **Assertions see only translatable block
source text — there is no round-trip / writer-output assertion type.**

**Three consumers of `spec.yaml`:**

1. **Native runner** (`core/format/spec/runner.go`, always-on via
   `spec_test.go`): `Load` + `NewReader`; skips `bridge_only`; cleanly skips
   unavailable `okapi:` fixtures; `MergeConfig`; reads `src=en trg=fr`;
   `expected_fail` → `t.Logf`.
2. **Parity runner** (`cli/parity/spec/runner.go`, build tag `parity`): the same
   examples through native **and** the okapi-bridge daemon;
   bridge-satisfies / native-satisfies / bridge==native. Optional `BridgeConfig`
   translates keys to the bridge's parameter shape.
3. **contract-audit** (`scripts/contract-audit/main.go`): checks `okapi_refs`
   exist in the pinned Surefire output and that config keys (by `okapi_param`)
   exist in the bridge JSON schema. The walker does **not** follow `$ref` into
   `$defs`, so specs must **not** declare inline-code config keys (they would
   read as false drift). `nativeFilterAliases` bridges `xmlstream`→`xml`.

**`bridge_config` translators** (`cli/parity/formats/<fmt>_bridge_config.go`:
csv/fixedwidth/ttml/regex/xmlstream): csv/fixedwidth render the whole config to
one Okapi `ParametersString` under the reserved `fprmContent` (per-key
`setString` never re-runs `Parameters.load`, #530); regex uses `regexRulesJson`
(RE2 parity).

## 4. Test taxonomy

Three overlapping families: (A) legacy unit tests, (B) the harvest "acceptance"
family (web/mobile catalog formats with no Okapi counterpart), (C) the
spec/parity engine. Kinds observed (approx. counts):

| Kind | Purpose | Detect |
|---|---|---|
| `reader_test` (~37) | extraction: block count/text/code preservation | `reader_test.go` |
| `spec_test` (40) | drives `spec.NativeRunner` over `spec.yaml` — **mandatory if an Okapi counterpart exists** | `spec_test.go` + `spec.yaml` |
| `skeleton_test` (~30) | byte-exact read→write edge cases | `skeleton_test.go` |
| `roundtrip_test` (~9) | byte/semantic equality — effectively mandatory | `roundtrip_test.go` |
| `writer_test` | writer-specific (mo/openxml/po/yaml) | `writer_test.go` |
| `transform_test` (8) | Okapi config-kind mapping | `transform_test.go` |
| `config_test` (mif/rtf/ttx/txml) | config validation | `config_test.go` |
| `schema_test` (**only html/json**) | schema matches the config struct | `schema_test.go` |
| `subfilter_test` (epub/json/odf/xml) | embedded content | `subfilter_test.go` |
| `upstream_test` (csv/icml/idml/odf) | real vendor files | `upstream_test.go` |
| `bench`/`perf_test` (html/json/openxml/wiki) | performance | `*_bench`/`perf_test.go` |
| `acceptance_test` (7 harvest) | external validator, `//go:build acceptance`, **SKIP if tool absent, FAIL only on rejection** | `acceptance_test.go` |
| `corpus_test` (harvest + idml/mif) | real-world files; caught 2 androidxml bugs | `corpus_test.go` |
| `invariants_test` (harvest) | translate→re-read: leaf/placeholder/state/CLDR preserved | `invariants_test.go` |
| `malformed_test` (**only arb/resx/xcstrings**) | clean Error + NotPanics — **should be mandatory** | `malformed_test.go` |
| `okapi_stubs_test` (json/properties/xliff/xliff2/yaml) | porting markers `// okapi: Class#method` | `okapi_stubs_test.go` |
| `okapi_skip_test` (8 harvest) | prose why there is no Okapi counterpart | `okapi_skip_test.go` |
| `okapi_test`/`okapi_parity_test` (mosestext/ttx/markdown) | bespoke parity | — |
| `parity_spec_test` (38) | `cli/parity/formats/<id>_spec_test.go` head-to-head | — |

**Two parity layers coexist.** The older `formatSpecs` table
(`cli/parity/formats/spec.go`, pinned manifest 1.48.0) with `Skip` constants +
generated fixtures, **and** the newer `spec.yaml` `ParityRunner`. Some table
`Skip` rows (csv/regex `SKIP_DIVERGENCE_453`) are stale versus now-working specs.

**Generated fixtures** (`fixtures_*_generated.go`, tag `parity`, "DO NOT EDIT")
come from `scripts/okapi-test-scan` (regex, not AST) harvesting `String snippet =
"..."`. **Regeneration is manual** (no `//go:generate`, no Makefile target):
`go run ./scripts/okapi-test-scan -src <okapi-java>/.../src/test/java -class
<Class> -package formats -out cli/parity/formats/fixtures_<x>_generated.go`.

**Test helpers** import from `core/internal/testutil` (NOT `core/testutil` —
CLAUDE.md is stale on this): `RawDocFromString`, `RawDocFromReader`,
`CollectBlocks`, `CollectParts`, `PartsToChannel`.

## 5. The parity harness

`cli/parity/`, build tag `parity`. `RunNative` (`native.go`): in-process
`NewReader`+`ApplyMap`, drain `Read`. `RunBridge` (`bridge.go`): bidirectional
`Process` gRPC to a pooled singleton okapi-bridge JVM daemon (cap one), spawned
via `exec.Command` with the daemon arg (**not** `CommandContext`, so the JVM
survives startup-context cancel), stdout handshake for the Unix socket; one
`ProcessHeader` then drain until `Complete`. `AcquireBridgeDaemon` discovers
plugins **only** from the sandbox plugins dir. `CompareEvents` uses a normalized
canonical projection (drops skeleton ids/uris/counters, codes→placeholders,
collapses whitespace); `CompareBytes` is byte-exact. Tikal extract-merge is an
optional third corner and skips when absent. The `roundtrip` sub-package has
tiers; **native defaults to `TierDivergent` (observation only), so native
regressions never fail CI unless a per-format `MinTier` is set.**

**Run parity for one format:**

```bash
make parity-sandbox        # builds .parity sandbox: kapi + okapi-bridge plugin
                           # needs a JRE + the okapi clone at OKAPI_VERSION=1.48.0
                           # (/Users/asgeirf/src/okapi/Okapi)
cd cli && KAPI_PARITY_SANDBOX=<p> KAPI_PARITY_REPORT=<p> \
  go test -tags parity -count=1 -run TestParityHtmlSpec ./parity/formats/
```

`make parity-test` builds the sandbox + exports the env; a missing sandbox
**hard-fails** via `RequireSandbox` unless `KAPI_PARITY_SKIP=1`.
`make parity-publish` runs `scripts/testcompare` for the dashboard JSON. Local
runs need `icu4c` on `PKG_CONFIG_PATH` + the `fts5` tag.

**Proves:** same input + same semantic config → native and the Okapi Java filter
extract the same translatable block text (byte-equal serialization when
`parity_strict`). **Limits:** `CompareEvents` discards whitespace /
code-serialization / skeleton ids (finer fidelity needs the byte rig, which
skips without Tikal/Okapi reference); native round-trip never fails CI without a
`MinTier`; step parity is stability-only (~120 steps, `cli/parity/tools/spec.go`);
**dashboards are stale caches** (regenerate after any change — a bridge that is
byte-stable while the tier moved means native regressed; dump via
`PARITY_DUMP`); `testcompare` ignores `parity_warn`/`expected_fail`.

## 6. Okapi mapping

Okapi (Java) lives at `/Users/asgeirf/src/okapi/Okapi/okapi/filters/` — one Maven
module per filter, v1.48.0. The filter id is `okf_<format>` from `getName()`.

A filter module contains: `<Format>Filter.java` (an `IFilter` event pull-parser:
`START_DOCUMENT`/`TEXT_UNIT`/`DOCUMENT_PART`/`START`-`END_GROUP`/`START`-`END_SUBFILTER`/`END_DOCUMENT`;
builds a `GenericSkeleton` of literal bytes + `addContentPlaceholder`);
`Parameters.java` (extends `StringParameters`, key constants + `reset()`
defaults; many implement `ISimplifierRulesParameters` and embed an
`InlineCodeFinder` — properties ships **4 default rules**; reproduce them for
exact `getCodes().size()` parity); and `IEditorDescriptionProvider` (SWT GUI
metadata, **non-load-bearing — do not port**).

**Find the test corpus:**

1. `<X>FilterTest.java` `@Test` methods → `okapi_refs` `Class#method`; inline
   `String snippet =` are the smallest fixtures.
2. `src/test/resources/` holds full files + matching `okf_<format>@TestNN.fprm`
   loaded via `InputDocument(path, paramFile)`. `out/` is generated, not a
   fixture.
3. `testDoubleExtraction()` enumerates the canonical corpus.
4. Deeper merge fixtures live in `integration-tests/okapi` (okapi-it-okapi).
   `RoundTripComparison` is `@Deprecated` upstream (double extraction misses
   merge bugs); the fuller contract is **EXTRACT → MERGE → compare-against-original**,
   not the per-filter `src/test`.

**Model → Run.** Okapi's `TextFragment` is a coded `String` with 2-char PUA
markers (`OPENING` 0xE101 / `CLOSING` 0xE102 / `ISOLATED` 0xE103 / `CHARBASE`
0xE110+index) plus a parallel `List<Code>`. `OPENING` → `PcOpenRun`; `CLOSING`
(same int id) → `PcCloseRun` (shared string ID); `PLACEHOLDER` →
`PlaceholderRun`; `TYPE_SUB`/`StartSubfilter` → `SubRun` → child `Block`; ICU
plural/select → `PluralRun`/`SelectRun`. `Code.type` → `Run.Type`, `Code.data` →
`Run.Data`, `Code.outerData` → `Run.Equiv`/`Disp`, `CLONEABLE`/`DELETEABLE` →
`Run.Constraints`. A faithful port **linearizes the coded-text walk into an
ordered `[]Run` with stable ID remapping** — the most correctness-sensitive
step. `outerData` / `MERGED` / `HASREF` refs / move-codes-to-skeleton have no
automatic equivalent — model them or record an accepted divergence.

**Issue tracker.** `https://gitlab.com/okapiframework/Okapi/-/issues` — **GitLab,
not GitHub** (`gh` does not apply; declared in `superpom/pom.xml`). Query:

```bash
# web filter:  ?search=...&state=all   (issues at /-/issues/<iid>)
# REST (public, no auth; project 62298414 == okapiframework/Okapi):
curl -s "https://gitlab.com/api/v4/projects/62298414/issues?search=openxml&state=all&per_page=50"
curl -s "https://gitlab.com/api/v4/projects/62298414/issues?labels=bug&state=opened&order_by=updated_at"
# or:  glab issue list -R okapiframework/Okapi --search "openxml"
```

Issue numbers are embedded in source comments and fixture names
(`issue_NNN.properties` / `.fprm`) — grep the checkout, then open `/-/issues/NNN`.

## 7. Hard-won principles & failure modes

- **Faithful by default; prove equivalence in the comparator.** Native is
  byte-exact from a coalescing-buffer skeleton, so the canon gap is usually
  Okapi *re-serializing* (native is the *more* faithful side). Canonicalize
  **symmetrically** at compare time on **both** `got` and `ref`
  (idml `MergeAdjacentCSRs`); never mutate writer output. openxml deleted ~5800
  LOC of WSO machinery; xliff Okapi-compat is opt-in.
- **Never regex/byte-rewrite serialized writer output to fix a modeling gap.**
  The one exception — reproducing an Okapi transform on opaque bytes — must cite
  the mirrored class/method (openxml's DrawingML default-run hoist; flagged
  fragile).
- **Ground every change in BOTH the format spec AND the Okapi Java
  class/method (v1.48.0); the spec wins ties.** Argue "your filter violates
  <cited spec>," don't normalize-hack.
- **Parity is semantic-config equivalence, not default-matching.** A pure
  default-only divergence is **not** an `expected_fail` — converge with explicit
  config (translated in `bridge_config`). csv #530 was recorded with converging
  examples.
- **xfail / divergence model.** Every `expected_fail` is tracked + attributed
  (`divergence_kind`). Only `native-bug` is alarming (≈0). Native never judges
  itself — an `engine: okapi` example means the reference can't produce output,
  so the sub-test skips. #616 triaged 151 xfails. The "assertions now pass —
  remove the tag" log is the only safety net catching stale xfails.
- **faithful% tiers:** byte / canon / divergent; `CanonClass` faithful = do not
  chase to byte, closeable = native loses info, unclassified = conservative;
  headline ≈97.5%.
- **Detection** (`core/format/detect.go`): MIME → ext → content (Sniff, magic,
  container `mimetype`, ambiguous magic). Niche formats sharing an ext/MIME with
  a generic one (resx/xcstrings/androidxml/designtokens/vignette) claim no
  extension or use a `Sniff`; arb/i18next do **not** advertise
  `application/json`. Never advertise a shared MIME for a niche format; use a
  distinct MIME for ZIP containers.
- **Harvest recipe** (no Okapi counterpart): byte-faithful like json + generated
  reference-data; the test ladder is corpus + invariants + tool-gated
  acceptance; detection is native-first; registered writers, not scripts;
  configure the JSON/YAML readers rather than fork; KLF is exchange-only;
  `//go:generate`, not runtime export.
- **Never gloss a FAIL as "pre-existing."** Tracked-open: pdf #617, xliff2 #560
  (~21 byte-equal fails), archive #504, openxml RunFonts. The 42% dashboard was
  once a generator bug.
- **Recurring failure modes:** dashboard caches go stale (a mif regression once
  hid); `parity-annotations.yaml` has been stale/inverted; golangci-lint
  under-reports without `icu4c` on `PKG_CONFIG_PATH`; the internal
  `implementing-formats.md` reader snippet won't compile (omits
  `Signature`/`Open`/`Close`) while `contribute/formats.md` is correct.
- **Not coverage holes:** pdf is read-only by design; `mo`/`exec` are stub
  readers that error on `Open`; `exec`/`jsx`/`memorytest` are thin/internal.

## 8. Integration / wiring checklist

1. **Package + files** (`reader.go`/`writer.go`/`config.go`, name == id); embed
   the bases; constructors set base fields + a `cfg` alias + `cfg.Reset()`.
2. **Reader:** `Signature` / `Open` (validate + stash) / `Read` (cap-64 channel +
   goroutine + ctx-aware emit; `LayerStart` → `Block`/`Data` → `LayerEnd`; errors
   on the channel) / `Close`; linearize inline markup into `Ph`/`PcOpen`+`PcClose`
   runs with verbatim `Data` + shared string IDs.
3. **Writer:** target-else-source + `RenderRunsWithData`.
4. **Round-trip strategy:** pick **one**; implement `SkeletonStoreEmitter` /
   `Consumer` if using skeletons; document any non-byte-exact normalization.
5. **Config** `ApplyMap` rejects unknown keys.
6. **`schema.go`** (recommended).
7. **Register:** `core/formats/register.go`
   (`RegisterReader`+`RegisterWriter`+`registerSchemaAndDecoder`), the `ids.go`
   constant, the three `register_test.go` expected lists; `Signature()` must
   match the registered `FormatSignature`; use `Sniff`/unique-ext for
   json/xml/zip overlap.
8. **`spec.yaml` + `spec_test.go`** (mandatory if there is an Okapi counterpart):
   `okf_<id>`, keys 1:1 with `ApplyMap` + `okapi_param`, features with refs,
   assertions reflecting **native** behavior; prefer `okapi:` fixtures for
   binary; do **not** declare inline-code keys.
9. **`transform.go`** `init()` registration (if Okapi-mapped).
10. **Parity:** `cli/parity/formats/<id>_spec_test.go` (`ParityRunner`, same
    `spec.yaml`) + `<id>_bridge_config.go` if the param shapes differ;
    `make parity-sandbox` then the tagged test.
11. **Tests:** round-trip + malformed minimum; corpus/upstream;
    invariants/acceptance for catalogs; annotate `// okapi:` / `// okapi-skip:`;
    a prose `okapi_skip_test` if there is no counterpart.
12. `make kapi-i18n-generate` (a git-diff gate).
13. `make generate-reference-docs` + a `nativedocs` sidecar named exactly the id.
14. Verify `go build ./...`, `make test`, parity, and the dashboards.

**Silent-failure traps:** a nil config silently skips the schema + decoder; a
`register_test.go` length assert fails **without naming** the missing format;
`spec.yaml` is **not gated by any test** unless `spec_test.go` exists; a sidecar
filename typo no-ops; metadata/reference regen only fails in CI drift gates.
