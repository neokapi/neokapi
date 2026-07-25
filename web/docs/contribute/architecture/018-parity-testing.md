---
id: 018-parity-testing
sidebar_position: 18
title: "AD-018: Parity testing against Okapi"
description: "Architecture decision: a parity harness runs every neokapi format and tool against Okapi Framework reference outputs to surface divergences — categorizing results as faithful, okapi-faithful, divergent, or new."
keywords: [parity testing, Okapi Framework, format parity, test harness, divergence, architecture decision, neokapi]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# AD-018: Parity testing against Okapi

## Summary

For every Okapi filter and step a neokapi format or tool intends to
match, the **parity harness** runs both implementations against the same
input and asserts they produce equivalent output. Tests live under
`cli/parity/`, gated by the `parity` build tag. `make parity-test` builds
a sandboxed kapi binary plus a freshly built okapi-bridge plugin, spawns
the bridge daemon, runs every parity case, and writes a JSON report.

Parity is a **maintainer-facing fidelity check on Go ports**, not a
product surface and not a plugin contract. Its report is a local artifact
under the gitignored `.parity/` sandbox; nothing parity-related is
published. The distinct, durable, cross-repo guarantee — that a plugin
implements the plugin protocol at all — is
[protocol conformance](#parity-is-not-conformance), which lives in the
framework module and needs no bridge checkout.

Without parity, a Go-port refactor can silently diverge from the Java
reference. That risk is real and worth a harness; it is simply a
different risk from "is this plugin still speaking the protocol?".

## Context

Two independent stacks must agree on output:

- **neokapi (Go)** — native readers, writers, and tools embedded in the kapi
  binary.
- **okapi-bridge** — a Java plugin built from the Okapi Framework JARs,
  developed in [its own repository](https://github.com/neokapi/okapi-bridge)
  and spawned as a Mode-C daemon on demand over a Unix socket. It is not
  part of the product surface; it is the reference implementation of a
  third-party plugin in a non-Go language, and — for parity — a
  convenient second opinion on what an Okapi filter produces.

When a Go port and a bridge filter both claim to read `okf_html`, kapi
prefers the Go port (`format_factory.go` only registers a daemon-backed
reader when no native reader exists). That preference is correct for
end users — native is faster — but it means a regression in the Go port
is invisible: the bridge would have caught it, but the bridge never
runs. The parity harness exists to invert that: it explicitly runs both
implementations side by side, on the same input, and fails when their
outputs diverge.

## Parity is not conformance

Two guarantees are easy to conflate because both involve running the
bridge. Keeping them apart is what lets the bridge live in its own
repository:

|                | Parity                                                        | Protocol conformance                                                        |
| -------------- | ------------------------------------------------------------- | --------------------------------------------------------------------------- |
| Question       | Do two implementations of the same filter produce the same output? | Does this plugin implement the plugin protocol?                             |
| Subject        | neokapi's own Go readers, writers, and tools                   | Any plugin, in any language                                                 |
| Needs          | a built okapi-bridge, an Okapi version pin, a JVM              | only the plugin directory                                                   |
| Lives in       | `cli/parity/` (build tag `parity`)                             | `core/plugin/conformance/` (framework module, no build tag)                  |
| Audience       | neokapi maintainers                                            | plugin authors, in their own repositories                                    |
| Runs where     | this repository, gated on `main`                               | wherever the plugin lives, against a *released* kapi                        |
| Report         | a local artifact under `.parity/`                              | `conformance.Report` — text transcript plus a stable JSON artifact           |

Conformance is the load-bearing cross-repo contract: it is what makes a
plugin repository able to answer "do I still work with kapi?" on its own,
without a checkout of this repository and without this repository running
its tests. See
[Plugin protocol v1](../implementation/plugin-protocol-v1.md) and
[AD-007](007-plugin-system.md).

Parity is the narrower question, and it points the other way: it is about
neokapi's fidelity, using the bridge as a reference. That is why parity
is not, and must not become, a gate on the bridge repository — and why
the bridge's own CI reports *conformance*, on a schedule, never gating
this repository.

## Design

### Architecture

<PipelineDiagram
  channelLabel="[]Part"
  stages={[
    { label: "TestParityHtmlSpec / TestParityJSONSpec", sub: "same input", role: "io" },
    {
      parallelLabel: "run both implementations side by side",
      lanes: [
        { label: "RunNative", sub: "html.NewReader (in-process)" },
        { label: "RunBridge", sub: "DaemonPool → JVM daemon (gRPC)" },
      ],
    },
    { label: "BlockTexts equality", sub: "text-only; fails on divergence", role: "qa" },
  ]}
/>

### Sandbox

The harness deliberately ignores `~/.local/share/kapi/plugins/`,
`$XDG_DATA_HOME`, and any system-installed `kapi`. Without that
discipline, a developer with an outdated bridge would see a
green parity run that doesn't reflect the code on disk. Instead,
`make parity-test`:

1. Builds `bin/kapi` from the current source tree into `.parity/bin/kapi`.
2. Runs `make plugin-v2 V=1.48.0` in `../okapi-bridge` and unpacks the
   tarball into `.parity/plugins/okapi-bridge/`.
3. Exports `KAPI_PARITY_SANDBOX=$REPO/.parity` and runs
   `go test -tags parity ./cli/parity/...`.

`cli/parity/env.go::LoadSandbox` resolves the sandbox from
`$KAPI_PARITY_SANDBOX`, or auto-discovers a locally built `.parity/`
by walking up from cwd for `.parity/bin/kapi`; it never falls back to
a system-installed `kapi`. Tests go through `RequireSandbox`, which
enforces the contract by FAILING the test (`t.Fatalf`) when no sandbox
is found — set `KAPI_PARITY_SKIP=1` to skip instead. Skip-by-default
was deliberately abandoned because silent skips made local agent runs
report parity green while CI failed.

### Comparison

Two part streams are compared on a **canonical projection**
(`cli/parity/normalize.go::CanonicalPart`) that includes:

- Sequence of `PartType` values (block / layer / group / data / media).
- Block IDs and translatable flag.
- Source text rendered with **structural placeholders** for inline
  codes (`{<id}`, `{>id}`, `{ph:id}`) — not the format-specific code
  data verbatim.
- Target locale text in the same shape.
- Layer / group / data / media identity fields.

The projection deliberately **excludes** several fields, which makes them
**parity-safe carriers** — a native reader may populate them with richer
information than the bridge emits without ever tripping parity: placeholder
`Equiv`/`Disp` (used to carry portable LaTeX/MathML for equations,
[AD-032](032-math-and-equations.md)), the block `SemanticRole` /
`StructureAnnotation`, dynamic `Properties`, and stand-off `Annotations`
([AD-002](002-content-model.md)). Anything a reader wants to add for
downstream consumers — ingestion, the editor, cross-format export — rides one
of these carriers precisely because the comparator does not look at it.

Inline-code data is intentionally hidden from the default comparison.
Different implementations represent paired codes differently — Okapi
serializes them as display markers like `[#$dp2]`, the Go HTML reader
emits the raw markup `<a href="…">`. Both are valid; neither is
"wrong"; comparing them byte-for-byte would mask the meaningful parity
bar of "same translatable text + same code structure".

For tests that DO want byte-level fidelity, `CompareBytes` is
available — typically used against the round-trip output of a writer.

### Same semantic config → same results

Parity is faithfulness to the bridge **under the same semantic
configuration**, not faithfulness to the bridge's *defaults*. A native reader
is free to pick a richer default than Okapi when the matching configuration
still reproduces the bridge's output — the contract is "same semantic config →
same results", not "same defaults".

The load-bearing instance is content-fidelity surfacing
([AD-031](031-content-fidelity-surfacing.md)): native readers default
`extractNonTranslatableContent` ON, surfacing code, captions, formulas, and
other non-translatable context as `Block{Translatable:false}` for LLM/RAG
ingestion — content the bridge has no notion of. The bridge keeps that content
in skeleton, so a head-to-head with surfacing on would diverge by construction.
The spec runner therefore forces the matching config: `runNative`
(`cli/parity/spec/runner.go`) duck-types the reader config's
`SetExtractNonTranslatableContent(bool)` setter and sets it **false** before
reading, so the native stream is byte-identical to the bridge.
Surfacing is an opt-in ingestion convenience layered on top of a parity-faithful
core, never a divergence from it. New format options that change *defaults*
rather than *semantics* must offer the same off-switch so parity can pin them.

### Reporting

Each parity test reports one row via `parity.Report` with `Kind`
(`format` or `step`), `ID` (the Okapi short id), `Mode`
(`head-to-head`, `bridge-only`, or `byte`), and the test outcome.
`parity.FlushReport` from each package's `TestMain` writes the
accumulated rows to `$REPO/.parity/test-comparison.json`, and the
`parity.yml` CI workflow uploads that JSON as an artifact. It is
maintainer telemetry: a per-filter / per-step status table for whoever is
working on a port. It is not published, and no product claim is derived
from it — `/format-maturity` carries the public quality story.

## Consequences

- **Regressions in Go ports surface immediately**. A change to the
  HTML reader that drops a paragraph break shows up the next time
  `parity.yml` runs on `main`.
- **Bridge-only filters remain validated**. When no Go port exists and
  a textual fixture can be supplied (e.g. `okf_multiparsers`, wired with
  `NewReader: nil` and an inline CSV input in
  `cli/parity/formats/spec.go`), the parity test asserts that the bridge
  produces stable output against a fixed input, so new Okapi releases
  that break a filter become visible without anyone needing to invoke
  that filter from production. Binary-container filters such as
  `okf_idml` (which has a full `core/formats/idml` reader) and
  `okf_archive` are currently `Skip: SKIP_BINARY` — no committed binary
  corpus — so they appear as gap rows on the dashboard rather than
  asserting bridge output until a corpus ships via okapi-bridge
  `testdata/`.
- **Cross-repo proto sync becomes load-bearing**. A neokapi proto
  change that the bridge doesn't mirror trips parity immediately.
  This is what we want: the proto IS the contract.
- **Sandbox build adds wall-clock time**. A full parity run includes a
  Maven JAR build and a `jpackage` app-image step, totalling several
  minutes. The sandbox is cached locally between runs (set
  `PARITY_FORCE=1` to rebuild) so iterating on a single test stays
  fast.

## How to add a new parity case

1. Identify the Okapi filter id (`okf_<name>`) or step id from the
   bridge manifest at `~/.local/share/kapi/plugins/okapi-bridge/manifest.json`.
2. **For a format:** add (or extend) a `spec.yaml` under
   `core/formats/<name>/`, then add a `TestParity<Name>Spec` in
   `cli/parity/formats/<name>_spec_test.go` that loads it via
   `parityspec.LoadSpec` and runs a `parityspec.ParityRunner` — set
   `NewReader` to the native reader for a head-to-head comparison, or
   leave it `nil` for a bridge-only stability snapshot. The same
   `spec.yaml` also drives the always-on native test in
   `core/formats/<name>/spec_test.go` — one source of truth.
   **For a step/tool:** add a `ToolSpec` row to the `toolSpecs` table in
   `cli/parity/tools/spec.go`; the single table-driven `TestParityTools`
   (`cli/parity/tools/spec_test.go`) picks it up automatically — there
   are no per-tool `<name>_test.go` files.
3. `Mode` is derived by the runner (head-to-head when a native
   reader / tool is wired, bridge-only otherwise) and emitted via
   `parity.Report` — it is not assigned by hand in the test.
4. Run `make parity-test` locally; iterate until green.

## How the summary is wired

`scripts/testcompare/main.go` reads `.parity/test-comparison.json` (the
raw report written by the `cli/parity/` test packages) and emits a
narrower per-row summary at `.parity/parity-report.json`, one row per
filter / step with its current status, mode, and skip detail. Run
`make parity-publish` to refresh both files locally.

Both files are local maintainer artifacts inside the (gitignored) parity
sandbox. Nothing parity-related is published to the documentation site:
the `/parity` dashboard was retired when the bridge left the product
surface, and `/format-maturity` carries the public quality story.

## Pre-release gate

The `release.yml` workflow blocks tagging if the `parity.yml` workflow
has not concluded as `success` for the tagged commit. The `parity-gate`
job queries the GitHub Actions API for the parity workflow's
conclusion against `${{ github.sha }}` and fails closed on absent /
in-progress / failed runs. The top-level independent release jobs (such
as `build-cli`) then `needs: parity-gate`, so the entire downstream
release pipeline inherits the gate.

## Where the harness ends up

The harness's dependencies point the wrong way for a repository that no
longer ships the bridge: running it here means this repository needs an
okapi-bridge checkout, an Okapi version pin, a Maven build, and a JVM —
for a check about *neokapi's* fidelity, whose reference implementation is
maintained elsewhere.

The end state inverts that. The parity harness and its specs move to the
bridge repository, which already owns the Okapi version matrix and builds
the JARs, and consumes released `neokapi` modules to obtain the native
side. This repository keeps no bridge build, no sandbox script, and no
PR-blocking bridge job; what remains is a scheduled, non-blocking
conformance smoke against the released bridge. The bridge repository runs
both parity and protocol conformance against released kapi versions on
its own schedule.

What makes that inversion possible is exactly the split in
[Parity is not conformance](#parity-is-not-conformance): the durable
contract between the two repositories is the protocol, and the protocol
is verifiable from a released module. Parity then becomes an ordinary
consumer of that released module rather than a reason for the two
repositories to share a build. Tracked in
[#1073](https://github.com/neokapi/neokapi/issues/1073).

## References

- Issue: [#448 — Restore full parity coverage](https://github.com/neokapi/neokapi/issues/448)
- Issue: [#1073 — De-couple okapi-bridge from the monorepo](https://github.com/neokapi/neokapi/issues/1073)
- PR: [#447 — Retire core/plugin/bridge](https://github.com/neokapi/neokapi/pull/447) (the deletion that #448 reverses on top of Mode-C dispatch)
- Bridge proto sync: [#450](https://github.com/neokapi/neokapi/issues/450) — closed by okapi-bridge `b0ee4d5`
- Short-id resolution: [#451](https://github.com/neokapi/neokapi/issues/451) — closed by okapi-bridge `b0ee4d5`
- [Plugin protocol v1](../implementation/plugin-protocol-v1.md) — the contract the two repositories share
