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

The native engine intentionally **does not** use the registered
`PseudoTranslate` tool — that tool always applies an accent map (à →
ã, e → é, …) which would diverge from bridge/tikal output. The
harness needs a deterministic wrap (`«source»`) every engine
produces identically.

### `BridgeEngine` (runs when okapi-bridge daemon is reachable)

Drives the okapi-bridge daemon's `Process` RPC in **read-write mode**
(`OutputRef` set in `ProcessHeader`). The daemon reads the input via
the requested `okf_<format>` filter, streams every Block back over
gRPC, this side overwrites the target (via the `Transform` hook on
`parity.BridgeRequest`), and the daemon's writer thread merges the
modified parts back into the output document at the configured
`OutputRef.Path`.

The transform hook is engine-agnostic: it receives a `*model.Block`
and is free to call `SetTargetText` / `SetTargetRuns`. The bridge
engine wires it to the same `applyPseudoToBlock` helper tikal uses
(in `cli/parity/roundtrip/pseudo.go`).

### `TikalEngine` (the comparator — hard-required)

Shells out to the upstream tikal CLI from the Okapi distribution:

  1. `tikal -x doc.X -sl en -tl fr` — extract to XLIFF (emits
     `doc.X.xlf` with empty `<target>` elements).
  2. **Go-side XLIFF rewrite**: walk the XLIFF, populate every
     `<target>` with `«<source-text>»`. Implementation in
     `cli/parity/roundtrip/tikal.go::fillXLIFFTargets`. Uses regex
     over the raw bytes rather than `encoding/xml` because Go's
     XML encoder mangles namespace declarations on roundtrip
     (re-emits `xmlns` on every element, rewrites `xmlns:foo` to
     `_xmlns:foo`) — both break tikal's merger.
  3. `tikal -m doc.X.xlf -od merged/` — merge the translated
     XLIFF back into the document at `merged/doc.X`.

When a fixture sets `tikalExtraArgs` (e.g. `["-fc", "okf_wiki"]`
because tikal's extension auto-routing misses), the same flag is
passed to both `tikal -x` and `tikal -m` — tikal needs the explicit
filter at merge time too.

We deliberately avoid `tikal -psd` (Okapi's built-in pseudo-translate
step). That step's accent map and pipeline ordering are tikal-specific
and can't be replicated by the other engines.

Tikal is the **comparator**, not a tested engine. The harness runs
tikal once per fixture to obtain the live reference output, then
byte-compares native and bridge against tikal's bytes. There is no
"tikal" subtest — asserting tikal-against-itself would be circular.

## The pseudo-translation transform

The single transform every engine applies, defined in
`cli/parity/roundtrip/pseudo.go`:

```go
func applyPseudoToBlock(b *model.Block, spec PseudoSpec) {
    if !b.Translatable { return }
    src := b.SourceText()
    if src == "" { return }
    b.SetTargetText(spec.TgtLocale(), spec.Wrap(src))
}
```

`spec.Wrap` defaults to `«…»`. The choice of guillemets is deliberate:

  - Survives every text-bearing format (no XML/JSON/CSV/YAML escape
    issues).
  - Single-rune so byte budgets stay predictable.
  - Visually distinguishable from skeleton text in failure dumps.
  - Locale-agnostic — no MT, no model dependency.

A test can override `PseudoSpec.Prefix` / `Suffix` to use different
markers if a particular format conflicts with `«…»` (none have so
far).

## Comparison strategy

For each fixture the harness runs **tikal end-to-end inline** to
obtain the live reference output (extract → fill XLIFF targets with
`«source»` → merge), then runs each tested engine (native, bridge)
and byte-compares its output against tikal's. Tikal is the
comparator; native and bridge are the engines under test. There are
no committed golden files — tikal's behavior IS the reference,
captured fresh every run, so there is no risk of "the golden is
from tikal v1.47, but we're on v1.48 now" drift.

For formats tikal can't open (no upstream filter, e.g.
srt/jsx-klf/versifiedtext/messageformat/epub) or where tikal has a
genuine merge bug (vtt/ttml mutate timestamps and split target text
across cues; txml merge NPEs; idml extract NPEs; rtf only ships as
`okf_tradosrtf` and silently extracts an empty XLIFF from plain
RTF), the row is marked `tikalUnsupported: true` and **the whole
case skips at run time** — no reference, no engine assertions. We
deliberately do not fall back to the native engine here: that would
make native judge itself, the exact circularity this whole approach
was built to remove. These rows are placeholders documenting that
the format exists; the case starts asserting once tikal can handle
the input (or someone wires a hand-authored reference).

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

Both **tikal** and **okapi-bridge** are mandatory dependencies for
this suite — there is no "skip if missing" path:

  - Tikal is checked at `TestMain`. If not found via
    `$OKAPI_TIKAL`, `$OKAPI_HOME`, or `$PATH`, the test binary
    aborts with a single clear error.
  - Bridge is hard-required by `parity.AcquireBridgeDaemon`, which
    `t.Fatal`s if the daemon can't be spawned.

Failing fast at startup means a missing dependency surfaces as one
loud error, not a swarm of identical "engine unavailable" skips.

## Test wiring

A round-trip test looks like this (`plaintext_test.go`):

```go
func TestRoundTrip_Plaintext(t *testing.T) {
    roundtrip.RunThreeWay(t, roundtrip.Case{
        Name:     "three_lines",
        FormatID: "plaintext",
        Input: roundtrip.Input{
            Bytes:    []byte("Hello world\nAnother line\n"),
            Filename: "doc.txt",
        },
    },
        &roundtrip.NativeEngine{FormatID: "plaintext"},
        &roundtrip.BridgeEngine{FilterClass: "okf_plaintext"},
        &roundtrip.TikalEngine{},
    )
}
```

`RunThreeWay` runs each engine in its own sub-test:

  - Engines that report `Available() != nil` `t.Skip` rather than
    failing — tikal isn't installed everywhere.
  - Engines that run produce a Divergence on disagreement; the
    sub-test fails but the harness keeps going so a single run
    surfaces every disagreement at once.

For formats where the bridge filter expects different parameter names
than neokapi's canonical config, supply `BridgeEngine.FilterParams`
in already-translated form — same approach as the spec runner's
`BridgeConfig` hook (see `cli/parity/formats/csv_bridge_config.go`).

## File layout

```
cli/parity/roundtrip/
├── doc.go              # Package overview
├── engine.go           # Engine interface, Input, Result, PseudoSpec
├── pseudo.go           # applyPseudoToBlock — shared transform
├── native.go           # NativeEngine — in-process reader → transform → writer
├── bridge.go              # BridgeEngine — gRPC read-write with Transform hook
├── tikal.go               # TikalEngine — extract → XLIFF rewrite → merge (the comparator)
├── compare.go             # Byte / per-zip-entry comparators + Divergence
├── harness.go             # RunThreeWay — runs tikal as comparator
├── main_test.go           # TestMain — hard-requires tikal up front
├── coverage_test.go       # Per-format scans (formatScan) + discovery loop
└── coverage_skips_test.go # idmlBridgeSkips / openxmlBridgeSkips — long per-file maps
```

All files build-tagged `parity` to match the surrounding harness.

## Build and run

Tikal is a hard requirement, so set one of `$OKAPI_TIKAL`,
`$OKAPI_HOME`, or put `tikal` on `$PATH` before running:

```bash
OKAPI_HOME=/opt/okapi \
  go test -tags parity ./cli/parity/roundtrip/ -v

# Or with an explicit tikal launcher path:
OKAPI_TIKAL=/opt/okapi/tikal.sh \
  go test -tags parity ./cli/parity/roundtrip/ -v
```

Without tikal, `TestMain` aborts the binary immediately with a clear
error — no test runs. The bridge daemon is acquired through the same
`parity.AcquireBridgeDaemon` used by the spec runner; a single
daemon process is shared across the whole `go test` run and torn
down in `TestMain`.

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

3. If tikal needs an explicit `-fc` flag because the file extension
   doesn't auto-route to the desired filter (`.wiki`, `.tsv`, etc.),
   set `tikalExtraArgs: []string{"-fc", "okf_xxx"}`. The same flag
   is passed to both extract and merge.

4. If tikal needs non-default filter parameters (e.g. disabling
   `mergeCaptions` on subtitle filters), set `tikalParamConfig` to
   the `.fprm` content and use the `@variant` form in
   `tikalExtraArgs`:

   ```go
   tikalExtraArgs: []string{"-fc", "okf_ttml@nomerge"},
   tikalParamConfig: `#v1
   mergeCaptions.b=false
   `,
   ```

   The harness writes the `.fprm` to a temp dir and appends `-pd
   <dir>` so tikal can resolve the variant. See `okapi/filters/`
   in the Okapi source for which fields each filter accepts.

5. If the format is a zip-based compound (idml, openxml, epub, …),
   set `isZip: true` to switch the comparator to per-entry mode.

6. Run the test. The first run will surface every divergence as a
   real failure. For each one, characterize the cause and add a
   `skip` entry:

   - **Format-default skip** (most cases): if the same root cause
     affects most/all files, set `formatDefaultSkip` with the
     engines + a one-line reason. Per-file overrides extend it.
   - **Per-file skip**: keyed by basename in the `skip` map. Use
     `Engines: ["tikal"]` to mark a file where tikal can't produce
     a usable reference (intentionally-malformed test fixture,
     missing linked resource, upstream parser bug); the whole sub-
     test then skips with the reason.
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
| Engines             | Native + Bridge                   | Native + Bridge (tikal is the inline comparator)     |
| What it asserts     | Block count / text / IDs          | Engine output matches tikal's output byte-for-byte   |
| Driver format       | Per-format `spec.yaml` (declarative) | Per-format Go test row (programmatic)                  |
| Failure granularity | Per-example                       | Per-engine                                           |
| Catches             | Reader bugs, parity-of-reads      | Writer bugs, skeleton bugs, target-merge bugs        |

Both should pass before a format is considered shipped. A format that
passes spec parity but fails round-trip almost certainly has a writer
or skeleton bug.

## Coverage status

The harness mirrors upstream's `RoundTrip<X>IT.java` set: **30
formats × ~25 files each ≈ 1145 sub-tests** in ~16 minutes. With
tikal installed and the bridge daemon up the suite is fully green
(0 fail, 887 engine assertions pass, 1258 documented engine skips).

Highlights of the upstream-mirroring discovery:

  - **html** runs against all 69 fixtures from
    `integration-tests/okapi/src/test/resources/html/` — same set as
    upstream's `RoundTripHtmlIT`.
  - **json** 70, **idml** 70, **openxml** 185 — same fixtures
    upstream roundtrips with their own `RoundTrip<X>IT`.
  - **xml** uses the ITS-based `okf_xml` filter (it lives in
    `okapi/filters/its/.../XMLFilter.java`, not a dedicated `xml`
    filter directory) and roundtrips the 23 files from
    `integration-tests/okapi/src/test/resources/xml/`.
  - **paraplaintext** / **splicedlines** / **mosestext** —
    sub-filters of `okf_plaintext` with no dedicated upstream
    integration-test directory; cherry-picked via `explicitFiles`
    against the canonical fixtures referenced by their unit tests
    (`combined_lines.txt`, `test_paragraphs1.txt`, `Test01.txt`).
  - **vtt** / **ttml** use `tikalParamConfig` to disable
    `mergeCaptions` (matching the bridge's `mergeCaptions:false`
    knob), avoiding the upstream default that mangles cue text on
    round-trip.

Formats intentionally **omitted** from coverage:

  - `txml`, `rtf`, `epub` — tikal can't produce a usable reference
    (upstream merge bugs / read-only filters / no `okf_epub` in this
    distribution).
  - `jsx-klf`, `versifiedtext`, `messageformat` — neokapi-only
    formats with no upstream Okapi peer to compare against.
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
    where tikal also fails, or where bridge has a different bug).
  - `Engines: ["tikal"]` on a per-file entry means tikal itself
    can't produce a usable reference for that file — the whole
    sub-test skips. Common causes: intentionally-malformed test
    fixtures (`tmx/code_fail.tmx`), files referencing sibling
    resources tikal can't find when invoked on a single file
    (`xml/Translate2.xml`, `xliff/lqiTest.xlf`), upstream parser
    rejects (`yaml/Test03.yml` `!!timestamp`).

Common bug classes the skips encode:

  - **Native writer byte-shape divergence** (most formats):
    serialization choices differ from tikal — XML attribute order,
    YAML quoting style, properties escape case, trailing newlines.
  - **Native skips target on merge** (html, openxml): writer doesn't
    splice the translated text back into the document.
  - **Bridge inline-code marker divergence** (xliff, xml, idml,
    openxml): the bridge daemon emits different inline-code wrappers
    than tikal does for the same upstream filter.
  - **Bridge read-only filters** (xliff2): the upstream filter
    doesn't implement merge.
  - **Native YAML reader infinite loop** (4 files with self-
    referencing anchors): a real reader bug worth fixing.

When an engine bug gets fixed, the corresponding skip entry can be
deleted — the next run will then assert against tikal byte-for-byte,
catching any regression.

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
