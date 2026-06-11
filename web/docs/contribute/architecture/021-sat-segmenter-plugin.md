---
id: 021-sat-segmenter-plugin
sidebar_position: 21
title: "AD-021: SaT Segmenter Plugin"
description: "Architecture decision: the kapi-sat plugin runs the wtpsplit Segment-any-Text ONNX models in-process behind a line-delimited JSON stdin/stdout protocol, isolating its native ML stack from the portable kapi binary while exposing a sat segment engine."
keywords: [SaT, segmentation, wtpsplit, ONNX, plugin, segment engine, stdin stdout protocol, native isolation, architecture decision, neokapi]
---

# AD-021: SaT Segmenter Plugin

## Summary

`kapi-sat` is a standalone plugin binary that runs the
[SaT / wtpsplit](https://github.com/segment-any-text/wtpsplit) *Segment any
Text* sentence-segmentation models **in-process** and exposes them to a host
over a tiny line-delimited JSON protocol on stdin/stdout. The heavy native
dependencies — the ONNX Runtime shared library and the XLM-RoBERTa tokenizer —
live only in this plugin, gated behind the `onnx` build tag, and are never
linked into the portable `kapi` binary. The CLI registers a `sat` segment
engine ([AD-002](002-content-model.md)) that discovers the installed plugin,
spawns it once in `serve` mode, and drives it with a cgo-free protocol client.
The plugin is a distinct shape from the manifest-driven Mode A/B/C transports
([AD-007](007-plugin-system.md)): a long-lived subprocess speaking a
purpose-built segmentation protocol rather than gRPC.

## Context

Segmentation in neokapi is a pluggable, run-anchored stand-off overlay
([AD-002](002-content-model.md)). The framework ships rule- and
Unicode-baseline engines (`srx`, `uax29`) and an AI-provider chunker (`llm`);
the segment registry in `core/segment` mirrors the AI/MT provider registries so
an engine is selected by a short name and is simply absent when the binary that
would supply it is not linked. A high-quality ML segmenter — wtpsplit's SaT
models, which segment any of the languages XLM-RoBERTa covers without
per-language rules — is valuable, but it carries requirements the rest of the
framework deliberately avoids:

- **A native ML stack.** SaT inference needs ONNX Runtime (a native shared
  library) and the HuggingFace `tokenizers` Rust crate (linked via cgo). Linking
  either into `kapi` would force every install of the portable CLI to carry the
  ONNX ABI, defeat pure-Go cross-compilation, and inflate the binary for a
  capability most invocations never use.
- **Large model assets.** The `*-sm` models download hundreds of megabytes on
  first use. That is a runtime concern of the segmenter, not of `kapi`.
- **A warm process.** Loading an ONNX session per block is prohibitively slow;
  the model must load once and stay resident across an entire run.

The native-stack isolation rationale matches the one behind the Okapi bridge in
[AD-007](007-plugin-system.md) — keep `kapi` Apache-2.0, pure-Go, and small;
let a separate binary own its heavyweight runtime. SaT differs from the bridge
in transport: it is not a gRPC daemon serving formats/tools/connectors, but a
single segmenter answering a stream of segment requests, so it warrants its own
minimal protocol rather than the Mode-C contract.

## Decision

### Module layout and native isolation

The plugin is its own Go module, `github.com/neokapi/neokapi/plugins/sat`,
isolated so its native dependencies never enter any other module's build graph:

```
plugins/sat/
├── go.mod                      module github.com/neokapi/neokapi/plugins/sat
├── manifest.json               plugin manifest (registry distribution)
├── cmd/kapi-sat/main.go        entry point: serve | version | command
├── satproto/                   PURE-GO protocol (host imports this; no cgo)
│   └── satproto.go             Request/Response, Serve loop, Client driver
└── internal/
    ├── model/                  PURE-GO model + tokenizer download & XDG cache
    └── sat/                    inference
        ├── algo.go             PURE-GO blocks/recombine/sigmoid/rune mapping
        ├── float16.go          PURE-GO IEEE-754 half-precision ↔ float32
        ├── engine.go           Engine interface + ErrNoONNX
        ├── engine_stub.go      //go:build !onnx  (default; no native deps)
        ├── engine_onnx.go      //go:build onnx   (cgo: onnxruntime + tokenizer)
        └── ort_onnx.go         //go:build onnx   (runtime init + KAPI_SAT_ORT_LIB)
```

Two build configurations cover the same module:

- **Default build** (`go build ./...`) selects `engine_stub.go`, links no native
  libraries, and is CI-safe on any machine. Its `Segment` returns `ErrNoONNX`;
  `ping` and `info` still answer, so a host can detect "installed but no ONNX
  backend" gracefully.
- **ONNX build** (`-tags onnx`) selects `engine_onnx.go` and `ort_onnx.go`,
  linking the daulet/tokenizers static archive at build time and loading the
  ONNX Runtime shared library at runtime. This is the configuration shipped in
  release tarballs.

Because `satproto` and everything reachable from the default build are pure Go,
the protocol and inference-algorithm tests build and run with no native
dependency. The native code is reachable only under `-tags onnx`.

### Wire protocol — `satproto`

The plugin speaks a deliberately tiny, dependency-free protocol defined in the
`satproto` package: one JSON object per line, in both directions, over the
plugin's stdin (host→plugin) and stdout (plugin→host). The package imports
nothing beyond the standard library — no cgo, no ONNX, no tokenizer — so a host
can import it to talk to the plugin without inheriting its native build
requirements.

A `Request` carries an `op` (`segment`, `ping`, or `info`), an optional
host-chosen `id` the plugin echoes back, and for `segment` the `text`, optional
`model` and `lang` hints, and a boundary-probability `threshold`. A `Response`
carries the matching `id` and exactly one result shape:

| Request | Response |
|---|---|
| `{"id":1,"op":"segment","text":"…","lang":"en","model":"sat-3l-sm","threshold":0.25}` | `{"id":1,"boundaries":[13,…]}` |
| `{"op":"ping"}` | `{"ok":true,"version":"…"}` |
| `{"op":"info"}` | `{"version":"…","models":[{"name":"sat-3l-sm","loaded":true,"default":true},…]}` |
| any failure | `{"id":N,"error":"…"}` |

`boundaries` are **interior sentence boundaries as rune offsets** into the exact
request text — a boundary at rune offset `i` means a new sentence begins at
`text[i]`. Offsets `0` and the rune length are never emitted; results are
strictly ascending and de-duplicated. The host reconstructs segments by slicing
the original runes at these offsets.

The process stays alive across requests: models load lazily on first use and
are cached in memory, so a single spawned process serves an unbounded sequence
of segment requests until its stdin closes. Errors are reported in-band (a
`Response` with a non-empty `error` field) and do not terminate the loop, so the
host can keep issuing requests. `satproto.Serve` is the plugin-side
read/dispatch/write loop; `satproto.Client` is the host-side driver, which is
safe for concurrent use and serializes round-trips by id.

### Host integration — the `sat` segment engine

The CLI registers the `sat` engine into `core/segment` at init
(`cli/segment_sat.go`). The engine implements `segment.Segmenter`: it lazily
dials the plugin on first `Segment`, flattens the run sequence to plain text
with the shared mask options, sends one `segment` request, and maps the returned
boundary rune offsets back to run-anchored spans. The subprocess is started
under an engine-scoped context (`context.WithCancel(context.Background())` in
`dial`), deliberately decoupled from any per-run pipeline context, so the warm
model survives across blocks and files; the per-call `ctx` is used only for a
pre-flight cancellation check. The process is torn down only by `satEngine.Close`
(which cancels the engine context and kills/waits the child) or, as a backstop, a
`runtime.AddCleanup` finalizer when `Close` is never called — not by completion
or cancellation of an individual run.

Two properties of this path are deliberate:

- **The host knows the protocol but does not import the plugin module.** To keep
  `kapi` free of the heavy native build, `cli/segment_sat.go` defines its own
  small `satRequest`/`satResponse` structs that mirror `satproto`'s wire format,
  rather than importing `plugins/sat/satproto`. The protocol is the contract;
  the duplication is the cost of the isolation.
- **Discovery reuses the plugin host.** The engine locates the installed plugin
  by name through `cli/pluginhost`'s discovery (the same fixed location search
  as [AD-007](007-plugin-system.md)) and runs its `BinaryPath` with `serve`. An
  explicit `PluginPath` in the segment config overrides discovery (used by tests
  and dev builds). When the plugin is not found, the engine returns an
  actionable error pointing at `kapi plugins install sat` or the local
  `make build-sat-plugin-onnx` build.

The segment tool surfaces this through its `engine: sat` config, with `satModel`
and `threshold` parameters; the CLI resolves the plugin path and passes it into
the engine's `segment.Config`.

### Manifest shape

`manifest.json` registers the plugin under the unified model
([AD-007](007-plugin-system.md)) by declaring a single Mode-A command (`sat`, an
interactive self-check that constructs the engine and lists supported models).
The segment contract itself does not fit any manifest capability slot — there is
no first-class "subprocess segmenter" capability — so it lives under a top-level
`metadata.segment` block describing the protocol name
(`satproto-line-json-v1`), the transport (`stdin-stdout`), the invocation
(`["kapi-sat","serve"]`), the protocol package import path, the boundary units
(rune offsets), the operations, and the supported models. The manifest root
permits extra keys, so the parser accepts it. If a first-class segment
capability is later added to `core/plugin/manifest`, this block migrates into
it.

### Models, runtime resolution, and distribution

Models are not bundled. Each `*-sm` model and its XLM-RoBERTa tokenizer download
from HuggingFace on first use and are cached on disk under `$KAPI_SAT_CACHE`,
`$XDG_CACHE_HOME/kapi/models/sat/<model>/`, or `~/.cache/kapi/models/sat/…` in
precedence order; concurrent downloads are serialized by a per-file lock with
atomic writes so a reader never sees a partial model.

The ONNX Runtime shared library *is* bundled, at `lib/<name>` beside the binary
in the release tarball. When `KAPI_SAT_ORT_LIB` is unset the plugin
auto-resolves the library from beside the executable, so a registry-installed
plugin needs no environment configuration; `KAPI_SAT_ORT_LIB` remains an
override, needed only for a from-source/dev build where no bundled library sits
next to the binary. The tokenizer archive is linked at build time and is not a
runtime artifact.

Release tarballs are built per platform on native runners (cgo, `-tags onnx`),
cosign-signed, and indexed in the registry, so `kapi plugins install sat`
verifies the download exactly as for any other plugin
([AD-007](007-plugin-system.md)).

## Consequences

- The portable `kapi` binary stays pure-Go, small, and cross-compilable; the
  ONNX/tokenizer native stack is confined to a separately built, separately
  installed plugin.
- A high-quality, language-agnostic ML segmenter is available behind the same
  `engine:` selector as the rule and baseline engines, with no impact on builds
  that do not use it — selecting `sat` without the plugin yields a clear,
  actionable error rather than a build failure.
- The protocol is intentionally narrower than the gRPC Mode-C contract, matching
  a single warm segmenter. The cost is a small struct duplication in the host so
  the CLI need not import the plugin module.
- First-segment latency includes a large one-time model download; integrators
  warm the model with an explicit segment or surface the plugin's stderr
  progress.

## Related

- [AD-002: Content Model](002-content-model.md) — segmentation as a run-anchored
  stand-off overlay; the `core/segment` engine registry and the `sat` engine
  value
- [AD-007: Plugin System and Okapi Bridge](007-plugin-system.md) — manifest
  discovery, native-stack isolation rationale, signed registry distribution
- [AD-013: Kapi CLI](013-kapi-cli.md) — the CLI that registers and drives the
  `sat` engine
- [`plugins/sat/`](https://github.com/neokapi/neokapi/tree/main/plugins/sat) —
  plugin module, protocol, inference, and README
