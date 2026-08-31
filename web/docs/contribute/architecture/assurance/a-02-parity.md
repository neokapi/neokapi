---
id: a-02-parity
sidebar_position: 2
title: "A-02: Parity with the Okapi Framework"
description: "A build-tagged harness runs every Go port head-to-head against the Okapi Framework reference, through the bridge plugin and, where available, the tikal CLI, on a canonical projection that compares extracted text and code structure rather than byte-level markup."
keywords: [parity, Okapi Framework, format fidelity, test harness, canonical projection, tikal, conformance, architecture decision, neokapi]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# A-02: Parity with the Okapi Framework

## Summary

For every Okapi filter or step a format or tool here intends to match, the
**parity harness** runs both implementations against the same input and asserts
they produce equivalent output. The tests live under `cli/parity/`, gated by the
`parity` build tag. `make parity-test` builds a sandboxed `kapi` binary plus a
freshly built bridge plugin, spawns the bridge daemon, wires the tikal CLI as a
third reference corner when one is available, runs every case, and writes a JSON
report.

Parity is a **maintainer-facing fidelity check on Go ports**, not a product
surface and not a plugin contract. Its report is a local artifact inside a
gitignored sandbox. The distinct, durable, cross-repository guarantee, that a
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
it, and the bridge never runs. The harness exists to invert that, running both
implementations side by side on the same input and failing when they diverge.

## Parity is not conformance {#parity-is-not-conformance}

Two guarantees are easy to conflate, because both involve running the bridge.
Keeping them apart is what lets the bridge live in its own repository:

| | Parity | Protocol conformance |
| --- | --- | --- |
| Question | Do two implementations of the same filter produce the same output? | Does this plugin implement the plugin protocol? |
| Subject | This project's own Go readers, writers, and tools | Any plugin, in any language |
| Needs | a built bridge, an Okapi version pin, a JVM | only the plugin directory |
| Lives in | `cli/parity/`, build tag `parity` | `core/plugin/conformance/`, framework module, no build tag |
| Audience | maintainers of this repository | plugin authors, in their own repositories |
| Runs where | here, on the main branch and on pull requests touching the code involved | wherever the plugin lives, against a released `kapi` |
| Report | a local artifact under `.parity/` | a text transcript plus a stable JSON artifact |

Conformance is the cross-repository contract: it is what lets a plugin
repository answer "do I still work with kapi?" on its own, without a checkout of
this repository and without this repository running its tests.

Parity is the narrower question, and it points the other way: it is about *this*
implementation's fidelity, using the bridge as a reference. That is why parity is
not, and must not become, a gate on the bridge repository, and why the bridge's
own CI reports conformance, on a schedule, never gating this repository.

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
        { label: "bridge", sub: "daemon pool → JVM" },
        { label: "tikal", sub: "reference CLI, when built" },
      ],
    },
    { label: "canonical projection", sub: "fails on divergence", role: "qa" },
  ]}
/>

### The sandbox

The harness deliberately ignores user plugin directories, data-home overrides, and
any system-installed `kapi`. Without that discipline, a developer with an outdated
bridge would see a green run that does not reflect the code on disk. Instead,
`make parity-test`:

1. builds `kapi` from the current source tree into the sandbox;
2. builds the bridge plugin from a sibling checkout and unpacks it into the
   sandbox;
3. writes a launcher for the reference CLI when a built JAR is present in the
   Okapi checkout, and reports that the third corner will skip when it is not;
4. exports the sandbox path and runs `go test -tags "fts5,parity" ./parity/...`.

Both build tags are spelled in one comma-separated flag on purpose. Go does not
union repeated `-tags` (the last occurrence wins), so a second `-tags parity`
would silently drop the SQLite full-text capability and run the adjudicator for
every round-trip change in a build configuration no other target uses. Nothing
fails when that happens, because the dropped tag is a runtime capability rather
than a compile gate.

`cli/parity/env.go` resolves the sandbox from the environment, or auto-discovers a
locally built one by walking up from the working directory; it never falls back to
a system-installed binary. Tests go through `RequireSandbox`, which **fails** the
test when no sandbox is found. Skip-by-default was abandoned deliberately: silent
skips made local runs report parity green while CI failed. An explicit environment
variable opts into skipping instead.

The suite also keeps its hands off the machine it runs on. Several Okapi
analysis steps default to opening the report they just wrote in the desktop's
browser; the tool specs pass `autoOpen=false` through their step parameters, so
the steps still write their reports and a run never opens a window.

### Comparison

Two part streams are compared on a **canonical projection**
(`cli/parity/normalize.go`) that includes:

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
  not reachable, so the harness still passes on a machine with no Okapi build. It
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

### Reporting

Each pass reports one row: a `Kind` discriminating format, round-trip, reference
CLI, fixture, feature, or step; an `ID` (the Okapi short id); a `Name`; a
`Status`; a `Mode` describing what was actually verified (head-to-head,
bridge-only, byte, or reference CLI); and a short detail on failure or skip.
`FlushReport`, called from each package's `TestMain`, writes the accumulated rows
to the sandbox as JSON, and the parity workflow uploads that JSON as an artifact.

A second tool reads those raw rows and emits a narrower per-row summary: one row
per filter or step with its current status, mode, and skip detail. `make
parity-publish` refreshes both files locally.

Both files are maintainer telemetry inside the gitignored sandbox: a per-filter
status table for whoever is working on a port. The row results are not
published. What the public [format-maturity dashboard](/format-maturity) reads
from parity is narrower: whether a format has a parity case at all is one
signal on its engine axis, and the maturity ledger records the watermark of the
last parity run (its report timestamp and the main-branch commit it ran on), so
the dashboard can say how fresh that signal is. No product claim is derived
from a parity row.

## Consequences

- **Regressions in Go ports surface immediately.** A change to a reader that drops
  a paragraph break shows up the next time the parity workflow runs on the main
  branch.
- **Bridge-only filters stay validated.** Where no Go port exists and a textual
  fixture can be supplied, the test asserts that the bridge produces stable output
  against a fixed input, so a new Okapi release that breaks a filter becomes
  visible without anyone invoking that filter in production.
- **Binary-container filters appear as gaps rather than false greens.** Formats
  with no committed binary corpus carry an explicit skip, sourced from the
  format's own spec where it has one and inline where it does not, so they show
  as gap rows in the summary instead of silently asserting nothing.
- **Cross-repository schema sync is enforced.** A schema change the bridge does
  not mirror trips parity immediately. That is the intent: the schema is the
  contract.
- **A full run costs wall-clock time.** It includes a Maven JAR build and an
  application-image step, several minutes in total. The sandbox is cached between
  runs, so iterating on a single test stays fast.

## Adding a parity case

1. Identify the Okapi filter id (`okf_<name>`) or step id from the bridge
   manifest in the sandbox.
2. **For a format:** add or extend a `spec.yaml` under `core/formats/<name>/`,
   then add a spec test under `cli/parity/formats/` that loads it and runs the
   spec runner. Wire the native reader for a head-to-head comparison, or leave it
   unset for a bridge-only stability snapshot. The same `spec.yaml` also drives the
   always-on native test in `core/formats/<name>/`: one source of truth, and the
   place where the bridge filter class, the reference-CLI corner, and the parity
   skips are declared.
   **For a tool:** add a row to the tool spec table in `cli/parity/tools/`; the
   single table-driven test picks it up automatically. There are no per-tool test
   files.
3. `Mode` is derived by the runner (head-to-head when a native reader or tool is
   wired, bridge-only otherwise) and emitted with the report row. It is never
   assigned by hand.
4. Run `make parity-test` locally and iterate until green.

The harvested fixtures under `cli/parity/formats/` are extracted from the upstream
Okapi Java tests by a scanner and are not regenerated by an ordinary test run;
refresh them with the dedicated make target and review the diff.

## Release gate

The release workflow blocks tagging unless the parity workflow has concluded
successfully for the tagged commit. The gate job queries the Actions API for every
parity run against that commit and passes if any of them succeeded, robust to a
second, manually dispatched run being in flight while an earlier one already
passed. It fails closed on absent, in-progress, or failed runs, and the
independent release jobs depend on it, so the whole downstream pipeline inherits
the gate.

## Where the harness ends up

*Not yet built.* The harness and its specs are intended to move to the bridge
repository, which owns the Okapi version matrix and builds the JARs, consuming
released modules for the native side and leaving this repository a scheduled,
non-blocking conformance smoke against the released bridge.

## See also

- [A-01: Testing and documentation](a-01-testing-and-documentation.md): the pyramid this harness sits above
- [F-02: The content model](../foundations/f-02-content-model.md): the fields the canonical projection excludes
- [F-04: The content-model wire schema](../foundations/f-04-wire-schema.md): the schema the two repositories share
- [E-02: The format system](../engine/e-02-format-system.md): readers, defaults, and surfacing
- [E-05: The plugin system](../engine/e-05-plugin-system.md): the daemon dispatch the bridge uses
- [Plugin protocol v1](/contribute/implementation/engine/plugin-protocol-v1): the conformance contract
- [Format maturity](/format-maturity): the published quality story
