---
id: 018-parity-testing
sidebar_position: 18
title: "AD-018: Parity testing against Okapi"
description: "Architecture decision: a parity harness runs every neokapi format and tool against Okapi Framework reference outputs to surface divergences — categorizing results as faithful, okapi-faithful, divergent, or new."
keywords: [parity testing, Okapi Framework, format parity, test harness, divergence, architecture decision, neokapi]
---

# AD-018: Parity testing against Okapi

## Summary

neokapi (Go) is an in-progress port of Okapi Framework (Java). For every
filter and step the Go side intends to match, the **parity harness**
runs both implementations against the same input and asserts that they
produce equivalent output. Tests live under `cli/parity/`, gated by the
`parity` build tag. `make parity-test` builds a sandboxed kapi binary
and a freshly built okapi-bridge plugin, spawns the bridge daemon, runs
every parity case, and writes a JSON report consumed by the docs-site
parity dashboard.

This is the load-bearing safety net for v1.0.0 onward — without it,
Go-port refactors can silently diverge from the Java reference.

## Context

The codebase has two independent stacks that must agree on output:

- **neokapi (Go)** — native readers, writers, and tools embedded in the kapi
  binary.
- **okapi-bridge** — a Java plugin distributed as `okapi-bridge`, built
  from the Okapi Framework JARs. Spawned as a Mode-C daemon on demand,
  speaks gRPC over a Unix socket.

When a Go port and a bridge filter both claim to read `okf_html`, kapi
prefers the Go port (`format_factory.go` only registers a daemon-backed
reader when no native reader exists). That preference is correct for
end users — native is faster — but it means a regression in the Go port
is invisible: the bridge would have caught it, but the bridge never
runs. The parity harness exists to invert that: it explicitly runs both
implementations side by side, on the same input, and fails when their
outputs diverge.

## Design

### Architecture

```
TestParityHTML ─┐
                ├─→ RunNative ─► html.NewReader (in-process)
TestParityJSON  │       │
                │   []model.Part
                │       ▼
                │   CompareBlockText(t, native, bridge)
                │       ▲
                │   []model.Part
                │       │
                └─→ RunBridge ─► DaemonPool.Acquire("okapi-bridge")
                          │
                          ▼
                    kapi-okapi-bridge daemon (JVM, gRPC over Unix socket)
                          │
                          ▼
                    BridgeService.Process / .ProcessStep
```

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

`cli/parity/env.go::LoadSandbox` enforces the contract: tests
SkipNow if `KAPI_PARITY_SANDBOX` is unset, never silently fall
back to a system install.

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

Inline-code data is intentionally hidden from the default comparison.
Different implementations represent paired codes differently — Okapi
serializes them as display markers like `[#$dp2]`, the Go HTML reader
emits the raw markup `<a href="…">`. Both are valid; neither is
"wrong"; comparing them byte-for-byte would mask the meaningful parity
bar of "same translatable text + same code structure".

For tests that DO want byte-level fidelity, `CompareBytes` is
available — typically used against the round-trip output of a writer.

### Reporting

Each parity test reports one row via `parity.Report` with `Kind`
(`format` or `step`), `ID` (the Okapi short id), `Mode`
(`head-to-head`, `bridge-only`, or `byte`), and the test outcome.
`parity.FlushReport` from each package's `TestMain` writes the
accumulated rows to `$REPO/.parity/test-comparison.json`. The
`parity.yml` CI workflow uploads that JSON as an artifact; the
[parity dashboard](/parity) on the docs site renders it as a
per-filter / per-step status table.

## Consequences

- **Regressions in Go ports surface immediately**. A change to the
  HTML reader that drops a paragraph break shows up the next time
  `parity.yml` runs on `main`.
- **Bridge-only filters remain validated**. Even when a Go port does
  not exist (e.g. `okf_archive`, `okf_idml`), the parity test asserts
  that the bridge produces stable output against a fixed input. New
  Okapi releases that break a filter become visible without anyone
  needing to invoke that filter from production.
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
2. Add a test under `cli/parity/formats/<name>_test.go` (or
   `cli/parity/tools/<name>_test.go`) following the
   `TestParityHTML` / `TestParityWordCount` template.
3. Set `Mode: "head-to-head"` if there is a Go port to compare against,
   or `Mode: "bridge-only"` if the test is a stability snapshot.
4. Run `make parity-test` locally; iterate until green.

## How the dashboard is wired

`scripts/testcompare/main.go` reads `.parity/test-comparison.json` (the
raw report written by the `cli/parity/` test packages) and emits a
narrower per-row published shape at
`web/docs/static/data/parity-report.json`. The
[`/parity`](/parity) page (`web/docs/src/pages/parity/index.tsx`)
imports that JSON at build time and renders one row per filter / step
with its current status, mode, and skip detail. Run
`make parity-publish` to refresh both files locally.

The output path is deliberately separate from the legacy
`/test-comparison` page's data file (`web/docs/static/data/test-comparison.json`),
which is kept temporarily so that page's per-test-class view still
works.

## Pre-release gate

The `release.yml` workflow blocks tagging if the `parity.yml` workflow
has not concluded as `success` for the tagged commit. The `parity-gate`
job queries the GitHub Actions API for the parity workflow's
conclusion against `${{ github.sha }}` and fails closed on absent /
in-progress / failed runs. `goreleaser` and `build-bowrain` (the two
top-level independent release jobs) then `needs: parity-gate`, so the
entire downstream release pipeline inherits the gate.

## References

- Issue: [#448 — Restore full parity coverage](https://github.com/neokapi/neokapi/issues/448)
- PR: [#447 — Retire core/plugin/bridge](https://github.com/neokapi/neokapi/pull/447) (the deletion that #448 reverses on top of Mode-C dispatch)
- Bridge proto sync: [#450](https://github.com/neokapi/neokapi/issues/450) — closed by okapi-bridge `b0ee4d5`
- Short-id resolution: [#451](https://github.com/neokapi/neokapi/issues/451) — closed by okapi-bridge `b0ee4d5`
