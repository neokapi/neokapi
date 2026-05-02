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

### `TikalEngine` (runs when tikal is on PATH)

Shells out to the upstream tikal CLI from the Okapi distribution:

  1. `tikal -x doc.X -sl en -tl fr` — extract to XLIFF (emits
     `doc.X.xlf` with empty `<target>` elements).
  2. **Go-side XLIFF rewrite**: walk the XLIFF, populate every
     `<target>` with `«<source-text>»`. Implementation in
     `cli/parity/roundtrip/tikal.go::fillXLIFFTargets`. Uses
     `encoding/xml` rather than string substitution so we don't
     accidentally rewrite text inside `<alt-trans>` or `<note>`
     elements.
  3. `tikal -m doc.X.xlf -od merged/` — merge the translated
     XLIFF back into the document at `merged/doc.X`.

We deliberately avoid `tikal -psd` (Okapi's built-in pseudo-translate
step). That step's accent map and pipeline ordering are tikal-specific
and can't be replicated by the other engines.

When tikal isn't installed (no `OKAPI_TIKAL`, no `OKAPI_HOME`, not on
`PATH`), the engine returns an error from `Available()` and the
harness records it as `t.Skip`.

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

Byte-equal comparison across engines is not workable: each filter
emits subtly different whitespace, attribute order, and XML
declaration quoting. Even between two runs of the same engine,
non-determinism (e.g. attribute serialization order) can drift.

The harness compares **Block streams re-extracted through the native
reader**:

  1. Extract source texts from the *input* through the native reader
     → `expected[i] = spec.Wrap(source[i])`.
  2. For each engine's *output*, extract again through the native
     reader. For each translatable block, take the target if
     populated for `spec.TgtLocale()`, otherwise the source. (PO,
     XLIFF, TMX populate target; plaintext, HTML, properties
     overwrite source.) Call this `actual[i]`.
  3. Compare `expected` vs `actual` element-by-element. Any
     difference is a Divergence reported per-engine.

Re-extracting through the native reader is intentional: every engine
gets evaluated against the same yardstick, so the test surfaces real
behavior differences instead of encoding noise.

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
├── bridge.go           # BridgeEngine — gRPC read-write with Transform hook
├── tikal.go            # TikalEngine — extract → XLIFF rewrite → merge
├── compare.go          # extractedBlocks, expectedTargets, Divergence
├── harness.go          # RunThreeWay — test entry point
├── main_test.go        # TestMain — daemon shutdown
├── plaintext_test.go   # First-format proof
└── coverage_test.go    # Multi-format coverage table
```

All files build-tagged `parity` to match the surrounding harness.

## Build and run

```bash
# Run the round-trip suite (auto-spawns the bridge daemon).
go test -tags parity ./cli/parity/roundtrip/ -v

# With tikal installed:
OKAPI_TIKAL=/opt/okapi/tikal.sh \
  go test -tags parity ./cli/parity/roundtrip/ -v

# Or via OKAPI_HOME:
OKAPI_HOME=/opt/okapi \
  go test -tags parity ./cli/parity/roundtrip/ -v
```

The bridge daemon is acquired through the same
`parity.AcquireBridgeDaemon` used by the spec runner — a single
daemon process is shared across the whole `go test` run and torn down
in `TestMain`.

## Adding a new format

1. Pick a small but multi-block fixture for the format.
2. Add a row to `coverage_test.go::TestRoundTrip_Coverage`:

   ```go
   {
       name:        "myformat_two_blocks",
       formatID:    "myformat",       // neokapi registry key
       filterClass: "okf_myformat",   // upstream Okapi filter class
       filename:    "doc.myext",
       body:        []byte("..."),
   },
   ```

3. If the bridge filter takes parameters with different names than
   neokapi's spec config, populate `BridgeEngine.FilterParams` (see
   `cli/parity/formats/csv_bridge_config.go` for the translation
   pattern).

4. If a particular engine is known to fail (e.g. tikal lacks the
   filter, or the bridge filter has a known bug), add the engine name
   to `ExpectedSkipped`. Treat this exactly the way `expected_fail:`
   is treated in spec.yaml — only for genuine divergence, not for
   default-mismatch you can fix with explicit config.

## Relationship to the spec runner

The spec runner (`spec.yaml` + `NativeRunner` / `ParityRunner`) and
the round-trip harness are complementary, not redundant:

| Aspect              | Spec runner                       | Round-trip harness                                   |
| ------------------- | --------------------------------- | ---------------------------------------------------- |
| Phase covered       | Read only                         | Read → modify → write → re-read                      |
| Engines             | Native + Bridge                   | Native + Bridge + Tikal                              |
| What it asserts     | Block count / text / IDs          | Re-extracted Block stream matches expected wrapped form |
| Driver format       | Per-format `spec.yaml` (declarative) | Per-format Go test row (programmatic)                  |
| Failure granularity | Per-example                       | Per-engine                                           |
| Catches             | Reader bugs, parity-of-reads      | Writer bugs, skeleton bugs, target-merge bugs        |

Both should pass before a format is considered shipped. A format that
passes spec parity but fails round-trip almost certainly has a writer
or skeleton bug.

## Coverage status

The harness currently exercises **34 formats** out of the 43 with
registered reader+writer pairs. Per-engine pass counts (when the
bridge daemon is up and tikal is not installed):

| Engine | Pass | Skip | Fail |
| ------ | ---- | ---- | ---- |
| native |   30 |    4 |    0 |
| bridge |   17 |   17 |    0 |
| tikal  |    0 |   34 |    0 |

The intentional skips encode known divergences — each is documented
inline in `coverage_test.go` with a one-line reason. The pattern:

  - **Native skips** (4): yaml writer reorders mapping keys; phpcontent
    writer emits output the reader can't re-extract; openxml/idml
    writers don't merge target text into the binary container; mif/rtf
    writers produce malformed output.
  - **Bridge skips** (17): per-format upstream issues — okf_xliff2 is
    read-only; okf_ts emits malformed XML on merge; okf_txml hangs on
    write; okf_vtt and okf_ttml-default merge adjacent cues; okf_wiki,
    okf_tex, okf_doxygen segment differently than native; okf_csv /
    okf_tabseparatedvalues / okf_fixedwidthcolumns / okf_splicedlines
    have default-divergence that needs a per-format BridgeConfig
    translator like the spec runner has; okf_idml/okf_mif throw
    Java-level errors on round-trip; okf_rtf is read-only;
    okf_transtable writer drops the wire-format header.
  - **Tikal skips** (all 34 locally): not installed; runs in CI when
    `OKAPI_TIKAL`/`OKAPI_HOME` is set.

These skips are working failure-tracking, not silent passes. As each
underlying issue is fixed, drop the corresponding `skip:` entry and
the round-trip harness immediately starts asserting on it.

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
