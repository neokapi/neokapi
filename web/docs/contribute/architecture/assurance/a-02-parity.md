---
id: a-02-parity
sidebar_position: 2
title: "A-02: Parity with the Okapi Framework"
description: "The parity suite lives in the okapi-bridge repository, compiles against a released neokapi module, and runs every Go port head-to-head against the Okapi Framework reference on a weekly schedule, on a canonical projection that compares extracted text and code structure rather than byte-level markup. It reports; it gates nothing here."
keywords: [parity, Okapi Framework, format fidelity, test harness, canonical projection, tikal, conformance, architecture decision, neokapi]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# A-02: Parity with the Okapi Framework

## Summary

For every Okapi filter or step a format or tool here intends to match, the
**parity suite** runs both implementations against the same input and asserts
they produce equivalent output. The suite lives in the
[okapi-bridge repository](https://github.com/neokapi/okapi-bridge), under
`parity/`, as its own Go module. It requires a **released**
`github.com/neokapi/neokapi` module, installs a released `kapi`, builds the
bridge plugin from that repository's checkout, runs every case, and writes a
JSON report. A scheduled workflow there runs it weekly and on demand and
publishes the report as a workflow artifact.

Parity is a **maintainer-facing fidelity check on Go ports**, not a product
surface and not a plugin contract. Nothing in this repository builds, tests or
gates on the bridge: no workflow here checks out the bridge, and no release
waits on a parity run. The distinct, durable, cross-repository guarantee, that a
plugin implements the plugin protocol at all, is
[protocol conformance](#parity-is-not-conformance), which lives in the
framework module and needs no bridge checkout.

## Context

Two independent stacks must agree on output:

- **The Go implementation**: native readers, writers, and tools compiled into the
  `kapi` binary.
- **The bridge plugin**: a Java plugin built from the Okapi Framework JARs,
  developed in its own repository and spawned as a daemon on demand over a local
  socket. It is not part of the product surface; it is the reference
  implementation of a third-party plugin in a non-Go language, and, for parity, a
  convenient second opinion on what an Okapi filter produces.

When a Go port and a bridge filter both claim to read the same format, kapi
prefers the Go port: a daemon-backed reader is registered only where no native
reader exists. That preference is right for users, because native is faster, but
it means a regression in the Go port is invisible: the bridge would have caught
it, and the bridge never runs. The suite exists to invert that, running both
implementations side by side on the same input and reporting when they diverge.

## Parity is not conformance {#parity-is-not-conformance}

Two guarantees are easy to conflate, because both involve running the bridge.
Keeping them apart is what lets the bridge, and the suite, live in another
repository:

| | Parity | Protocol conformance |
| --- | --- | --- |
| Question | Do two implementations of the same filter produce the same output? | Does this plugin implement the plugin protocol? |
| Subject | This project's Go readers, writers, and tools, at a released version | Any plugin, in any language |
| Needs | a built bridge, an Okapi version pin, a JVM, a released `kapi` | only the plugin directory |
| Lives in | `okapi-bridge/parity/`, build tag `parity` | `core/plugin/conformance/`, framework module, no build tag |
| Audience | maintainers of the Go ports | plugin authors, in their own repositories |
| Runs where | the bridge repository, weekly and on dispatch, against the latest release | wherever the plugin lives, against a released `kapi` |
| Report | a workflow artifact | a text transcript plus a stable JSON artifact |

Conformance is the cross-repository contract: it is what lets a plugin
repository answer "do I still work with kapi?" on its own, without a checkout of
this repository and without this repository running its tests.

Parity is the narrower question, and it points the other way: it is about *this*
implementation's fidelity, using the bridge as a reference. It consumes released
modules and binaries, so it measures a change here after that change ships. It
is a report in both directions: it is not a gate on the bridge repository's
pull requests, and it is not a gate on anything here.

## Design

### Architecture

<PipelineDiagram
  channelLabel="[]Part"
  caption="One input, two implementations, one canonical projection. A third corner joins when the reference CLI is reachable."
  stages={[
    { label: "one spec, one input", sub: "spec.yaml + fixtures", role: "io" },
    {
      parallelLabel: "run the implementations side by side",
      lanes: [
        { label: "native", sub: "in-process Go reader" },
        { label: "bridge", sub: "daemon → JVM" },
        { label: "tikal", sub: "reference CLI, when built" },
      ],
    },
    { label: "canonical projection", sub: "reports divergence", role: "qa" },
  ]}
/>

### One module version

The suite's `go.mod` pins a released `github.com/neokapi/neokapi` and requires
nothing from the `host` or `cli` modules. The native side of every comparison
is the reader compiled from that module, and the format specs
(`core/formats/<id>/spec.yaml`) that declare each format's bridge filter class,
reference-CLI corner and parity skips are read from the same module's source
directory. The specs and the readers under test therefore always come from one
version.

The daemon launcher is the plugin protocol's Mode C handshake, implemented in
the suite: start the launcher with `daemon`, read the socket from the first
stdout line, dial it over gRPC. See
[Plugin protocol v1](/contribute/implementation/engine/plugin-protocol-v1).

### The sandbox

The suite deliberately ignores user plugin directories, data-home overrides, and
any system-installed plugin. Its sandbox holds:

1. the `kapi` binary under test, copied from a release (in CI, installed by
   `setup-kapi`);
2. the bridge plugin, built from the bridge checkout and unpacked;
3. the published `okapi-testdata` corpus, for binary fixtures the round-trip
   pass reads;
4. a launcher for the reference CLI when a built JAR is present, with the third
   corner skipping when it is not.

Tests resolve the sandbox from the environment, or by walking up from the
working directory to a built one, and **fail** when there is none. Skip-by-default
was abandoned deliberately: silent skips made runs report parity green while
nothing had been compared. An explicit environment variable opts into skipping
instead.

Every build tag the suite needs is spelled in one `-tags` flag. Go does not
union repeated `-tags` (the last occurrence wins), so a second `-tags parity`
would silently drop the SQLite full-text capability, and nothing would fail,
because the dropped tag is a runtime capability rather than a compile gate.

The suite also keeps its hands off the machine it runs on. Several Okapi
analysis steps default to opening the report they just wrote in the desktop's
browser; the tool specs pass `autoOpen=false` through their step parameters, so
the steps still write their reports and a run never opens a window.

### Comparison

Two part streams are compared on a **canonical projection** that includes:

- the sequence of part types: block, layer, group, data, media;
- block IDs and the translatable flag;
- source text rendered with **structural placeholders** for inline codes
  (`{<id}`, `{>id}`, `{ph:id}`), not the format-specific code data verbatim;
- target locale text in the same shape;
- layer, group, data, and media identity fields.

The projection deliberately **excludes** several fields, which makes them
**parity-safe carriers**: a native reader may populate them with richer
information than the bridge emits without ever tripping parity. Those are the
placeholder `Equiv` and `Disp` fields (which carry portable equation renderings,
[M-04](../multilingual/m-04-math-and-equations.md)), the block's structure and
geometry annotations, dynamic properties, and stand-off annotations
([F-02](../foundations/f-02-content-model.md)). Anything a reader wants to add for
downstream consumers (retrieval ingestion, the editor, cross-format export)
rides one of these carriers precisely because the comparator does not look at it.

Inline-code data is intentionally hidden from the default comparison. Different
implementations represent paired codes differently: the reference serializes them
as display markers, the Go HTML reader emits the raw markup. Both are valid,
neither is wrong, and comparing them byte-for-byte would mask the meaningful bar,
which is "same extracted text plus same code structure". For tests that *do*
want byte-level fidelity, a byte comparator is available, typically used against a
writer's round-trip output.

### Three passes per format

A format spec drives up to three passes, each reporting its own row so the
per-filter summary can show them side by side without one masking another:

- **Read**: the native reader against the bridge reader on the same input,
  compared on the canonical projection.
- **Round-trip**: read then write, run only when both implementations declare a
  writer, compared on bytes.
- **Reference CLI**: the native round-trip output against an extract-then-merge
  run of the Okapi CLI. A divergence here says the native side reads or writes the
  format differently from the canonical reference; agreement between the CLI and
  the bridge, where both are populated, says the bridge plumbing is faithful even
  where the native side diverges. This pass skips automatically when the CLI is
  not reachable, so the suite still passes on a machine with no Okapi build. It
  also skips under non-default reader parameters rather than running at defaults,
  which would silently compare unlike things.

### Same semantic config, same results

Parity is faithfulness to the reference **under the same semantic configuration**,
not faithfulness to the reference's *defaults*. A native reader is free to pick a
richer default when the matching configuration still reproduces the reference's
output: the contract is "same semantic config → same results", not "same
defaults".

The instance that matters most is content-fidelity surfacing. Native readers
default to surfacing non-modifiable content (code, captions, formulas) as
`Block{Translatable: false}` for model and retrieval ingestion, content the
reference has no notion of and keeps in skeleton, so a head-to-head with surfacing
on would diverge by construction. The spec runner therefore forces the matching
configuration: it duck-types the reader config's surfacing setter and turns it
**off** before reading, so the native stream is byte-identical to the reference.
Surfacing is an opt-in ingestion convenience layered on top of a parity-faithful
core, never a divergence from it. A new format option that changes *defaults*
rather than *semantics* must offer the same off-switch so parity can pin it. See
[E-02: The format system](../engine/e-02-format-system.md).

### Known divergences

A divergence that is understood and accepted is recorded where it belongs: an
`expected_fail` with a `divergence_kind` on the example in the format's
`spec.yaml` here, or a per-fixture annotation in the suite's
`annotations/<format>.yaml` for the round-trip corpus, carrying severity, the
tracking issue and the spec citation. The suite reports these as
`expected_fail` or `parity_warn`, never as regressions, and reports an expected
failure that now passes so the record can be retired.

### Reporting

Each pass reports one row: a `Kind` discriminating format, round-trip, reference
CLI, fixture, feature, or step; an `ID` (the Okapi short id); a `Name`; a
`Status`; a `Mode` describing what was actually verified (head-to-head,
bridge-only, byte, or reference CLI); and a short detail on failure or skip.
A second tool folds those rows into a per-filter summary: one row per filter or
step with its current status, mode, and skip detail. The scheduled workflow
uploads both files as an artifact.

The row results are not published. What the
[format-maturity dashboard](/format-maturity) reads from parity is narrower:
whether a format has a parity case at all is one signal on its engine axis.
No product claim is derived from a parity row.

## Consequences

- **Regressions in Go ports surface after release, on a schedule.** A change to
  a reader that drops a paragraph break shows up in the first weekly run after
  the release that carries it. The always-on native spec tests in
  `core/formats/<id>/` are what catches it before merge.
- **No bridge bytes here.** Nothing in this repository's CI checks out, builds
  or installs the bridge, and no release depends on a parity run. The bridge's
  Okapi version matrix, its JVM and its build time stay in the bridge repository.
- **Bridge-only filters stay validated.** Where no Go port exists and a textual
  fixture can be supplied, the suite asserts that the bridge produces stable output
  against a fixed input, so a new Okapi release that breaks a filter becomes
  visible without anyone invoking that filter in production.
- **Binary-container filters appear as gaps rather than false greens.** Formats
  with no committed binary corpus carry an explicit skip, sourced from the
  format's own spec where it has one and inline where it does not, so they show
  as gap rows in the summary instead of silently asserting nothing.
- **Schema drift between the repositories is visible, not fatal.** A schema
  change the bridge does not mirror shows up in the next report.

## Adding a parity case

1. Identify the Okapi filter id (`okf_<name>`) or step id from the bridge
   manifest.
2. **For a format:** add or extend the `spec.yaml` under `core/formats/<name>/`
   here. The same `spec.yaml` drives the always-on native test in that package:
   one source of truth, and the place where the bridge filter class, the
   reference-CLI corner, and the parity skips are declared. Then, in the bridge
   repository, add a spec test under `parity/formats/` that loads it and runs the
   spec runner, wiring the native reader for a head-to-head comparison or leaving
   it unset for a bridge-only stability snapshot.
   **For a tool:** add a row to the tool spec table in `parity/tools/`; the
   single table-driven test picks it up automatically. There are no per-tool test
   files.
3. `Mode` is derived by the runner (head-to-head when a native reader or tool is
   wired, bridge-only otherwise) and emitted with the report row. It is never
   assigned by hand.
4. Run the suite in the bridge repository (`make parity-test`) and iterate until
   green. The spec test reads the `spec.yaml` from the released module the suite
   pins, so a spec change here is picked up by the suite once a release carries
   it, or locally by pointing the suite at a checkout.

The harvested fixtures under `parity/formats/` are extracted from the upstream
Okapi Java tests by a scanner in the suite and are not regenerated by an
ordinary test run.

## See also

- [A-01: Testing and documentation](a-01-testing-and-documentation.md): the pyramid this suite sits above
- [F-02: The content model](../foundations/f-02-content-model.md): the fields the canonical projection excludes
- [F-04: The content-model wire schema](../foundations/f-04-wire-schema.md): the schema the two repositories share
- [E-02: The format system](../engine/e-02-format-system.md): readers, defaults, and surfacing
- [E-05: The plugin system](../engine/e-05-plugin-system.md): the daemon dispatch the bridge uses
- [Plugin protocol v1](/contribute/implementation/engine/plugin-protocol-v1): the conformance contract
- [Format maturity](/format-maturity): the published quality story
