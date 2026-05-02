# Three-Engine Round-Trip Testing

This note describes the round-trip test harness in
`cli/parity/roundtrip/`, which exercises every neokapi format end-to-end
through three different filter engines and asserts they all agree on
the result.

## Why this exists

The `spec.yaml` runners in `core/format/spec/` and `cli/parity/spec/`
verify reader contracts: how many Blocks come out of an input, what
text each one carries, what their IDs are. They never run a writer.
That coverage proves "we can read these formats" but not "we can read,
modify, and write them back coherently".

Round-trip testing closes that gap. It's the canonical Okapi
integration-test pattern (cf. upstream's `IFilterRoundTripIt` /
`RoundTripUtils`) and is what "extract → translate → merge" reduces to
in practice. The harness exercises:

  - The reader (parses the source).
  - The model layer (carries Source + Target through the pipeline).
  - The writer (re-serializes the document with translations applied).
  - The skeleton path (preserves untranslatable structure verbatim).

Anything that breaks one of those — broken whitespace handling, a
writer that drops attributes, a skeleton that misses a region —
surfaces as a divergence between the engines or a re-extraction that
doesn't yield the expected text.

## The three engines

Three engines drive the same input through their own toolchain. Each
implements the `Engine` interface in `cli/parity/roundtrip/engine.go`:

```
Engine interface {
    Name() string
    Available() error
    RoundTrip(t *testing.T, in Input, spec PseudoSpec) []byte
}
```

### `NativeEngine` (always runs)

Wires the format's registered `DataFormatReader` through an inline
pseudo-translation goroutine into the format's registered
`DataFormatWriter`. All in-process. No subprocess, no gRPC, no
external binary. This is the same plumbing as `kapi pseudo-translate`
on the CLI.

### `BridgeEngine` (runs when okapi-bridge daemon is reachable)

Drives the okapi-bridge daemon's `Process` RPC in **read-write mode**
(`OutputRef` set in `ProcessHeader`). The daemon reads the input via
the requested `okf_<format>` filter, streams every Block back over
gRPC, this side overwrites the target (via the `Transform` hook on
`parity.BridgeRequest`), and the daemon's writer thread merges the
modified parts back into the output document at the configured
`OutputRef.Path`.

The transform hook is engine-agnostic: it receives a `*model.Block`
and is free to mutate `Targets`. The bridge engine wires it to the
same `applyPseudoToBlock` helper the native engine uses (in
`cli/parity/roundtrip/pseudo.go`).

### `OkapiEngine` (the comparator — hard-required)

Shells out to the okapi-bridge launcher's `pseudo` subcommand
(implemented at `bridge-core/.../PseudoCommand.java`), which composes
upstream Okapi's pipeline in-process:

```
RawDocumentToFilterEventsStep → TextModificationStep → FilterEventsToRawDocumentStep
   (input via okf_<format>)      (TYPE_EXTREPLACE,         (write via the same filter)
                                  SCRIPT_EXT_LATIN)
```

`TextModificationStep` skips inline-code marker chars during
substitution, so paired codes (`<bpt>`/`<ept>`), placeholders
(`<ph>`), and arbitrary inline runs survive the round-trip
unchanged. The Go side mirrors the same Latin-extended substitution
rune-by-rune in `pseudo.go::applyPseudoToBlock`, leaving
non-`Text` runs (Ph / PcOpen / PcClose / Sub / Plural / Select)
verbatim. Same transform, three engines.

When a fixture sets `okapiParamConfig` (e.g. VTT/TTML disabling
`mergeCaptions`), the harness passes it as `--fprm <content>` to the
launcher, where Okapi loads it via `IParameters.fromString()`.

We deliberately avoid `tikal -psd` (Okapi's built-in pseudo-translate
configured through tikal's CLI). That step has its own driver
parameters and the tikal CLI's two-pass extract→XLIFF→merge
architecture mishandles inline `<ph>`/`<bpt>` markers when the
harness rewrites XLIFF targets by hand. Driving
`TextModificationStep` directly via the bridge launcher is one
in-process pipeline with no XLIFF round-trip in between.

The okapi engine is the **comparator**, not a tested engine. The
harness runs the okapi engine once per fixture to obtain the live
reference output, then byte-compares native and bridge against its
bytes. There is no "okapi" subtest — asserting okapi-against-itself
would be circular.

## The pseudo-translation transform

The single transform every engine applies, defined in
`cli/parity/roundtrip/pseudo.go`. For each translatable Block, the
target is built as a copy of the source's segments where every
`TextRun` has its text rune-substituted via the Latin-extended
table; non-text runs (Ph / PcOpen / PcClose / Sub / Plural / Select)
are copied as-is so inline codes survive intact.

The substitution table mirrors Okapi's `TextModificationStep`
(`SCRIPT_EXT_LATIN`):

```
A→À a→à B→ß b→ƀ C→Ć c→ć D→Ď d→ď E→Ē e→ē F→Ƒ f→ƒ
G→Ĝ g→ĝ H→Ĥ h→ĥ I→Ĩ i→ĩ J→ĵ j→Ĵ K→Ķ k→ķ L→Ĺ l→ĺ
N→Ń n→ń O→Ō o→ō P→Ƥ p→ƥ Q→Ǫ q→ǫ R→Ŕ r→ŕ S→Ś s→ś
T→Ţ t→ţ U→Ũ u→ũ W→Ŵ w→ŵ Y→Ŷ y→ŷ Z→Ź z→ź
```

(M, V, X have no entries and pass through unchanged — same quirks as
upstream `oldChars[0]`.)

The accented variants are deliberate:

  - Survives every text-bearing format (no XML/JSON/CSV/YAML escape
    issues — every replacement codepoint is in the Latin-Extended
    Unicode block).
  - Visually distinct from the source so failure dumps clearly show
    "this is the translated text".
  - Locale-agnostic — no MT, no model dependency, deterministic
    bytes.

## Comparison strategy

For each fixture the harness runs the **okapi engine end-to-end
inline** to obtain the live reference output (Okapi's pipeline:
read → `TextModificationStep` → write), then runs each tested
engine (native, bridge) and byte-compares its output against the
okapi reference. The okapi engine is the comparator; native and
bridge are the engines under test. There are no committed golden
files — upstream Okapi's behavior IS the reference, captured fresh
every run, so there is no risk of "the golden is from Okapi v1.47,
but we're on v1.48 now" drift.

For formats where the okapi engine itself can't produce a usable
reference (intentionally-malformed test fixtures, files referencing
sibling resources that aren't copied to the harness's tempdir,
read-only filters like `okf_xliff2`), per-file `skip` entries with
`Engines: ["okapi"]` skip the whole sub-test — no reference, no
engine assertions. We deliberately do not fall back to the native
engine here: that would make native judge itself, the exact
circularity this whole approach was built to remove. These rows are
placeholders documenting that the format exists; the case starts
asserting once the upstream pipeline can handle the input.

Compound zip formats (idml, openxml, epub) compare per-entry:
byte-equal across uncompressed entry contents, ignoring zip
metadata (mtime, central-directory order, compression level) that
two correct round-trippers can legitimately differ on. Mark a
fixture with `isZip: true` to opt into per-entry comparison.

This approach avoids the trap of using one engine's reader as the
judge of another engine's writer. Every engine is held to the same
external yardstick. A divergence is concrete and actionable: "your
writer produced different bytes from the reference at offset 251."

## Hard requirements

The **parity sandbox** is the only mandatory dependency — there is
no "skip if missing" path:

  - The `okapi-bridge` launcher binary inside the sandbox is checked
    at `TestMain` (it backs both the bridge daemon engine AND the
    okapi reference engine via the `pseudo` subcommand).
  - The bridge daemon process itself is acquired through
    `parity.AcquireBridgeDaemon`, which `t.Fatal`s if it can't
    spawn.

Build the sandbox from the repo root:

```bash
make parity-test
# or to force a rebuild including a fresh okapi-bridge plugin tarball:
PARITY_FORCE=1 make parity-test
```

Failing fast at startup means a missing dependency surfaces as one
loud error, not a swarm of identical "engine unavailable" skips.

## Test wiring

All test invocations come from `coverage_test.go`, which scans
upstream Okapi fixtures and constructs one `RunThreeWay` call per
file. The shape of one call:

```go
roundtrip.RunThreeWay(t, roundtrip.Case{
    Name:     base,            // file basename
    FormatID: "plaintext",     // neokapi registry key
    Input:    roundtrip.Input{Bytes: body, Filename: base},
},
    &roundtrip.NativeEngine{FormatID: "plaintext"},
    &roundtrip.BridgeEngine{FilterClass: "okf_plaintext"},
    &roundtrip.OkapiEngine{FilterClass: "okf_plaintext"},
)
```

`RunThreeWay` runs each engine in its own sub-test. Engines listed
in `c.ExpectedSkipped` are recorded as `t.Skip`; the remaining
engines must produce output equivalent to the okapi reference. The
harness reports every disagreement at once instead of bailing on the
first.

For formats where the bridge filter expects different parameter names
than neokapi's canonical config, supply `BridgeEngine.FilterParams`
in already-translated form — same approach as the spec runner's
`BridgeConfig` hook (see `cli/parity/formats/csv_bridge_config.go`).
For Okapi-side parameter overrides on the reference engine (e.g.
disabling `mergeCaptions` on subtitle filters), supply
`OkapiEngine.ParamConfig` as raw `.fprm` content.

## File layout

```
cli/parity/roundtrip/
├── doc.go                 # Package overview
├── engine.go              # Engine interface, Input, Result, PseudoSpec
├── pseudo.go              # applyPseudoToBlock — shared Latin-extended substitution
├── native.go              # NativeEngine — in-process reader → transform → writer
├── bridge.go              # BridgeEngine — gRPC read-write with Transform hook
├── okapi.go               # OkapiEngine — bridge launcher's `pseudo` subcommand (the comparator)
├── compare.go             # Byte / per-zip-entry comparators + Divergence
├── harness.go             # RunThreeWay — runs the okapi engine as comparator
├── main_test.go           # TestMain — hard-requires the okapi-bridge launcher
├── coverage_test.go       # Per-format scans (formatScan) + discovery loop
└── coverage_skips_test.go # Per-file bridge skip maps (idml, openxml, html, markdown, po, csv, ts, mif, icml)
```

All files build-tagged `parity` to match the surrounding harness.

The `pseudo` subcommand itself lives in okapi-bridge:
`bridge-core/src/main/java/neokapi/bridge/PseudoCommand.java`. It's
wired into `OkapiBridgeServer.main`'s subcommand dispatcher
(alongside `daemon`, `version`, `--list-filters`, …).

## Build and run

Build the parity sandbox once from the repo root:

```bash
make parity-test
# or, after editing okapi-bridge sources:
PARITY_FORCE=1 make parity-test
```

Then run the suite:

```bash
KAPI_PARITY_SANDBOX="$(pwd)/.parity" \
  go test -tags parity ./cli/parity/roundtrip/ -v
```

Without the sandbox, `TestMain` aborts the binary immediately with
a clear error — no test runs. The bridge daemon is acquired through
the same `parity.AcquireBridgeDaemon` used by the spec runner; a
single daemon process is shared across the whole `go test` run and
torn down in `TestMain`.

## Adding a new format

The harness mirrors upstream Okapi: each `formatScan` points at the
same resource directory the corresponding `RoundTrip<X>IT.java` uses,
and every file with a matching extension becomes one sub-test.

1. Add a `formatScan` entry to `coverage_test.go::coverageScans()`:

   ```go
   {
       formatID:    "myformat",       // neokapi registry key
       filterClass: "okf_myformat",   // upstream Okapi filter class
       sources:     []string{"integration-tests/okapi/src/test/resources/myformat"},
       extensions:  []string{".myext"},
   },
   ```

   The harness scans `sources` for files matching `extensions` and
   creates one sub-test per file (`TestRoundTrip_Coverage/myformat/<basename>`).
   Use `explicitFiles` instead of `sources` when the format's
   fixtures are mixed in with sibling-filter fixtures and need to be
   cherry-picked (see splicedlines / paraplaintext / tsv / fixedwidth
   for examples).

2. If the bridge filter takes parameters with different names than
   neokapi's spec config, populate `bridgeParams` (see
   `cli/parity/formats/csv_bridge_config.go` for the translation
   pattern).

3. If the okapi reference engine needs non-default filter
   parameters (e.g. disabling `mergeCaptions` on subtitle filters),
   set `okapiParamConfig` to the raw `.fprm` content:

   ```go
   okapiParamConfig: `#v1
   mergeCaptions.b=false
   `,
   ```

   The harness forwards it to the `pseudo` subcommand as
   `--fprm <content>`, where Okapi loads it via
   `IParameters.fromString()` against the configured filter. See
   `okapi/filters/` in the Okapi source for which fields each
   filter accepts.

4. If the format is a zip-based compound (idml, openxml, epub, …),
   set `isZip: true` to switch the comparator to per-entry mode.

5. Run the test. The first run will surface every divergence as a
   real failure. For each one, characterize the cause and add a
   `skip` entry:

   - **Format-default skip** (most cases): if the same root cause
     affects most/all files, set `formatDefaultSkip` with the
     engines + a one-line reason. Per-file overrides extend it.
   - **Per-file skip**: keyed by basename in the `skip` map. Use
     `Engines: ["okapi"]` to mark a file where the okapi reference
     engine can't produce a usable output (intentionally-malformed
     test fixture, missing linked resource, upstream parser bug);
     the whole sub-test then skips with the reason.
   - For long per-file maps (idml: 46 entries, openxml: 124),
     extract them into a helper in `coverage_skips_test.go` to
     keep `coverage_test.go` readable — see `idmlBridgeSkips()`.

   Treat every skip the way `expected_fail:` is treated in
   spec.yaml — a tracked, real divergence, not a workaround for a
   config you forgot to set.

## Relationship to the spec runner

The spec runner (`spec.yaml` + `NativeRunner` / `ParityRunner`) and
the round-trip harness are complementary, not redundant:

| Aspect              | Spec runner                       | Round-trip harness                                   |
| ------------------- | --------------------------------- | ---------------------------------------------------- |
| Phase covered       | Read only                         | Read → modify → write → re-read                      |
| Engines             | Native + Bridge                   | Native + Bridge (okapi engine is the inline comparator) |
| What it asserts     | Block count / text / IDs          | Engine output matches okapi reference byte-for-byte  |
| Driver format       | Per-format `spec.yaml` (declarative) | Per-format Go test row (programmatic)                  |
| Failure granularity | Per-example                       | Per-engine                                           |
| Catches             | Reader bugs, parity-of-reads      | Writer bugs, skeleton bugs, target-merge bugs        |

Both should pass before a format is considered shipped. A format that
passes spec parity but fails round-trip almost certainly has a writer
or skeleton bug.

## Coverage status

The harness mirrors upstream's `RoundTrip<X>IT.java` set: **30
formats × ~25 files each ≈ 1100+ sub-tests** in ~10 minutes. With
the parity sandbox built the suite is fully green (0 fail, 765
engine assertions pass, ~990 documented engine skips).

After the okapi-bridge per-field code-hydrate fix (the daemon now
clones source `Code` metadata across the wire — `outerData`,
`originalId`, `referenceFlag` — instead of rebuilding from the
FragmentDTO), inline-code-bearing formats fully recovered: idml
70/70 bridge, openxml 185/185, mif 41/41, icml 9/9, xml 199/199.
The remaining 51 known divergences cluster in three buckets: PO/TS
property round-trip + target-vs-source pseudo (26), CSV
segmented-cell handling (8), HTML/markdown code-id reconstruction
on the way back (17). See `coverage_skips_test.go` for the per-file
list.

Highlights of the upstream-mirroring discovery:

  - **html** runs against all 69 fixtures from
    `integration-tests/okapi/src/test/resources/html/` — same set as
    upstream's `RoundTripHtmlIT`.
  - **json** 70 (all pass bridge), **idml** 70, **openxml** 185 —
    same fixtures upstream roundtrips with their own `RoundTrip<X>IT`.
    json got bumped from "all skipped" to "all bridge passes" as a
    direct result of switching the comparator to the in-process
    okapi pipeline.
  - **xml** uses the ITS-based `okf_xml` filter (it lives in
    `okapi/filters/its/.../XMLFilter.java`, not a dedicated `xml`
    filter directory) and roundtrips the 22 files from
    `integration-tests/okapi/src/test/resources/xml/`. Bridge passes
    14 of 22, with 8 inline-code marker divergences flagged
    per-file.
  - **paraplaintext** / **splicedlines** / **mosestext** —
    sub-filters of `okf_plaintext` with no dedicated upstream
    integration-test directory; cherry-picked via `explicitFiles`
    against the canonical fixtures referenced by their unit tests
    (`combined_lines.txt`, `test_paragraphs1.txt`, `Test01.txt`).
  - **vtt** / **ttml** use `okapiParamConfig` to disable
    `mergeCaptions` (matching the bridge's `mergeCaptions:false`
    knob), avoiding the upstream default that mangles cue text on
    round-trip.

Formats intentionally **omitted** from coverage:

  - `txml`, `rtf`, `epub` — upstream Okapi can't produce a usable
    reference (txml NPE on merge, rtf only ships as `okf_tradosrtf`,
    no `okf_epub` in this distribution).
  - `jsx-klf`, `versifiedtext`, `messageformat` — neokapi-only
    formats with no upstream Okapi peer to compare against.
  - `srt` — needs SRT-specific regex rules loaded as a sizable
    `.fprm` against the bridge's `okf_regex` filter; wire that in
    when there's a real signal worth catching.
  - `odf` — no test fixtures committed in the framework yet.

These are covered by per-format unit tests under `core/formats/<x>/`
instead.

### Reading the skip map

Each `formatScan.skip` entry is **one tracked engine bug**, not a
workaround. The skip mechanism has three layers:

  - `formatDefaultSkip` applies to every file in the scan. Use it
    when the same root cause affects most/all files (e.g. native
    yaml writer doesn't preserve quoting style → every yaml file
    skips native).
  - Per-file `skip` entries extend the format default with file-
    specific engines/reasons. Use them for outliers (a single file
    where the okapi reference also fails, or where bridge has a
    different bug).
  - `Engines: ["okapi"]` on a per-file entry means the okapi
    reference engine itself can't produce a usable reference for
    that file — the whole sub-test skips. Common causes:
    intentionally-malformed test fixtures (`tmx/code_fail.tmx`),
    files referencing sibling resources Okapi can't find when
    invoked on a single file (`xml/Translate2.xml`,
    `xliff/lqiTest.xlf`), upstream parser rejects (`yaml/Test03.yml`
    `!!timestamp`), read-only filters (`okf_xliff2`).

Common bug classes the skips encode:

  - **Native writer byte-shape divergence** (most formats):
    serialization choices differ from upstream Okapi — XML
    attribute order, YAML quoting style, properties escape case,
    trailing newlines.
  - **Native skips target on merge** (html, openxml): writer doesn't
    splice the translated text back into the document.
  - **Bridge inline-code marker divergence** (xml, xliff, html,
    markdown, po, mif, …): the bridge daemon's
    `StreamingTranslationApplier` rebuilds `TextFragment` from Go-
    side fragments via `OkapiCodeConverter`, and the result diverges
    from what an in-process Okapi pipeline would have written for
    the same source — usually around inline-code id assignment,
    paired-code restoration, or alt-trans/extension element
    placement.
  - **Native YAML reader infinite loop** (4 files with self-
    referencing anchors): a real reader bug worth fixing.

When an engine bug gets fixed, the corresponding skip entry can be
deleted — the next run will then assert against the okapi reference
byte-for-byte, catching any regression.

## Formats not yet covered

Some registered formats don't fit the generic harness today:

  - **regex** — needs an explicit rule set; native + bridge configs
    don't line up enough to share a fixture.
  - **vignette** — requires the `<importContentInstance>` shape with
    a paired source/target instance via `SOURCE_ID` / `LOCALE_ID`,
    which is too verbose for inline fixtures and doesn't work without
    config aliasing.
  - **ttx** — UTF-16-encoded; the harness's re-extraction pass uses
    Go's `encoding/xml` without a CharsetReader.
  - **odf** — no `core/formats/odf/testdata/*.odt` fixture committed
    to the framework yet.
  - **pdf** — no native writer (extract-only).
  - **mo** — write-only stub on the read side.

Adding any of these is a follow-up — usually one fixture row plus,
for the harder cases, a small bit of per-format wiring (charset
reader for ttx, BridgeConfig hook for regex, fixture commit for odf).
