---
id: 026-flow-io-binding
sidebar_position: 26
title: "AD-026: Flow I/O Binding — Source → Flow → Sink"
description: "Architecture decision: a flow is a pure transformation over a stream of Blocks backed by a block-store session; where content enters (the source binding) and where results go (the sink binding) are resolved from invocation context rather than baked into the flow graph. The same flow runs unchanged over a file, a .klz workspace, the project block store, or an imported interchange file, and a sink is optional — a process-only run lands its work as overlays in the project/.klz and defers materialization to a later merge/export."
keywords: [flow, source, sink, binding, block store, klz, process-only, source-transform, redaction, segmentation, reader, writer, pipeline, architecture decision]
status: Proposed
---

# AD-026: Flow I/O Binding — Source → Flow → Sink

> **Status: Proposed.** This AD records the target architecture for flow I/O
> binding. It is being iterated on; the runtime executor already matches the
> model (it orchestrates tools over a block-store session), but the authoring
> graph and the file glue still assume file-in / file-out. Implementation is
> tracked in a GitHub issue.

## Summary

Three nouns divide the work cleanly: a **tool** is the unit of work, a **flow**
is a named reusable *composition* of tools, and a **binding** is an end — where
content enters and leaves. A flow is a **pure transformation over a stream of
Blocks** backed by a block-store session: it owns no I/O, and a single tool is
not a flow. *Where content enters* (the **source binding**) and *where results
go* (the **sink binding**) are resolved from invocation context, not encoded in
the flow graph. The same flow definition runs unchanged whether its
content comes from a file read, the `.klz` workspace cache, the project block
store, or an imported interchange file — and whether its results are written to
a file, committed as overlays to the store, or both.

A sink is **optional**. A *process-only* run lands its work as overlays in the
project / `.klz` and emits no file; materialization is a separate, later sink
operation (`merge` / `export` / `pack`). This makes the `.klz` lifecycle
first-class: `extract` (source → store), `run` / transform (store → store),
`merge` (store → file).

The leading **source-transform stage** ([AD-004](004-processing-engine.md))
splits into two kinds: *idempotent model-settling transforms* that run **once at
ingest** and persist to the store, and *round-trip-paired brackets*
(redact … unredact) that stay part of a **run's** source/sink wiring.

Bindings are named by one scheme vocabulary across the CLI, the flow document,
and the existing resource URIs. A concrete binding resolves by precedence —
explicit flag, then project / `.klz` context, then the flow's intent, then
auto-detection — and `kapi run --explain` always shows the resolved
`source → sink` so nothing is hidden. A flow declares only *intrinsic intent*
(`sink: none` for an analysis flow), never a path.

## Context

The runtime executor is **already** source/sink-agnostic. `DefaultExecutor`
(`core/flow/executor.go`) knows nothing about readers or writers — it wires
channels between tools, opens a `blockstore.Session`, and commits or rolls it
back. Reader and writer are attached *externally* by `FileRunner`
(`core/flow/filerunner.go`), which performs `read file → feed parts → executor →
drain → write file`.

The mismatch lives one layer up, in **authoring**:

- The flow graph bakes I/O into the model. `FlowDefinition` has explicit
  `NodeReader` / `NodeWriter` node types (`core/flow/definition.go`), and
  `StepsSpec` has `input:` / `output:` fields whose reader/writer nodes are
  *auto-inserted* around the steps (`core/flow/steps.go`, `StepsToGraph`). So the
  model a human (or the flow editor) authors against assumes a file is read and a
  file is written.

The `.klz` package ([AD-025](025-klf-package.md)) introduced content that does
**not** originate from a fresh file read:

- A workspace run reads existing blocks and prior overlays from a persistent
  per-source block store. Today `cli/klzworkspace.go` reuses `FileRunner.RunFile`
  with the cache's extracted source bytes *plus* a `Store` side-channel — the
  `reader` node still re-parses the original format each run while the real
  working state enters through a side door.
- `merge` has **no incoming content at all** — it hydrates `targets/<locale>`
  overlays. It is implemented as a synthetic `"merge"` flow whose only tool is a
  `hydrateTargetsTool` (`cli/klzhydrate.go`). The `NodeReader` concept is
  conceptually wrong here: the input *is* the cached blocks.

Two questions this AD settles:

1. Is read → process → write the right shape, or should source and sink be
   **context-wired ends** — `Start → {flow} → End`?
2. Must every run produce an output, or can a run be **process-only**, landing
   its work in the store?

## Decision

### Three nouns: tool, flow, binding

Separating I/O from the flow leaves three concepts, each with exactly one job:

- **Tool** — the unit of work. A single capability-typed transformation over the
  Part stream — `Annotate`, `Translate`, or `Transform`
  ([AD-006](006-tool-system.md)). A tool runs on its own; it needs no flow.
- **Flow** — a named, reusable **composition** of tools. A flow carries the
  ordering, the branching (`parallel:`, tee, batch), the per-tool configuration,
  and the settle/main phase split — and nothing else. It is *the recipe*.
- **Binding** — the ends. Where content enters (source) and where results leave
  (sink). A binding belongs to neither the tool nor the flow; it is supplied by
  the invocation and the project (§1–§5).

A flow is **composition, and only composition.** It owns no I/O, and a single
tool is not a flow: a lone tool is invoked directly as a tool command, and
`kapi flows` lists only the compositions. The flow noun earns its place by
carrying the four things a flat list of tool names cannot:

- **Configuration** — a flow pins each tool's settings, so it is a *configured*
  recipe (`tm-leverage{fuzzy:75}` → `ai-translate{provider:anthropic}` →
  `qa-check`), not merely an ordered set of tool names.
- **Topology** — a flow is a DAG. `parallel:` fan-out, `tee`, and `batch` are
  graph shapes a sequence cannot express.
- **Identity and reuse** — a flow has a name and a source (built-in, user,
  project). A project's `flows:` block is its vocabulary of named operations,
  versioned with the recipe and shared like any other artifact. Because a flow is
  portable, declarative intent and owns no I/O, it travels in a project's portable
  twin — the `.klz` package — like any other recipe field
  ([AD-025](025-klf-package.md) §6).
- **Phase structure** — the leading settle stage and the round-trip brackets
  (§4) are a typed two-phase shape, not a flat run of tools.

What a flow is **not**: it is not an I/O harness (that is the binding), it is not
a runtime primitive beyond an ordered tool chain over a session
([AD-004](004-processing-engine.md)), and it is never required to run one tool.

### 1. The flow is the middle; source and sink are bindings

A flow operates only on a stream of Parts backed by a session. The endpoints are
a small, separate **binding** vocabulary, resolved from invocation context:

| Binding | Source role (in) | Sink role (out) |
| --- | --- | --- |
| `file` | `DataFormatReader` over file bytes | `DataFormatWriter` + skeleton round-trip ([AD-005](005-format-system.md)) |
| `store` / `klz` | existing blocks + overlays from a persistent store | commit overlays — no materialization |
| `import` / `export` | overlays landed from an interchange file ([AD-017](017-bilingual-format-interop.md)) | emit interchange (XLIFF / PO / TMX / TBX) |
| `none` | — | discard (observation/metrics only) |

The defining property: **a flow definition is identical across bindings.** The
same `ai-translate-qa` flow runs in the file CLI, against a `.klz` workspace, and
against a project — only the binding differs.

### 2. Reader and writer stop being graph nodes

`input:` / `output:` are promoted **out of** `StepsSpec.steps` into a top-level
`source:` / `sink:` spec, and `NodeReader` / `NodeWriter` cease to be tool-graph
nodes. The `file` binder remains the **default**, so an unqualified
`kapi run flow -i file.json -o out.json` is unchanged — it is sugar for
`source: file`, `sink: file`. `FileRunner` becomes **one binder among several**
behind a common binder interface; the klz workspace's store wiring and the
synthetic merge flow collapse into `source: store` + `sink: file`.

```yaml
apiVersion: v1
kind: FlowDefinition
metadata:
  name: Production Pipeline
spec:
  source: file        # default; or `store`, `klz`, `import:xliff`
  sink: store         # process-only: commit overlays, emit nothing
  steps:
    - tool: tm-leverage
    - tool: ai-translate
    - tool: qa-check
```

### 3. Sink is optional → process-only runs

A run whose `sink` is `store` (or absent) commits its overlays to the project /
`.klz` block store and **emits no file**. Materialization becomes a distinct sink
operation — `merge` (store → file via skeleton), `export` (store → interchange),
or `pack` (store → `.klz`). This separates *doing the work* from *handing it
out*, and makes the workspace lifecycle the natural grain:

```
extract  sources → store        (source: file,  sink: store)
run/xfm  store   → store         (source: store, sink: store)   ← process-only
merge    store   → files         (source: store, sink: file)
```

Because the block store is append-only and content-addressed, a process-only run
is **idempotent and resumable**: re-running skips work whose overlay already
exists, anchored to the current block hashes ([AD-025](025-klf-package.md) §5).
The store *is* the workspace.

### 4. Split the source-transform stage

The leading source-transform stage ([AD-004](004-processing-engine.md)) settles
a single canonical model before the main tools run. In a workspace world its two
uses pull apart:

- **Ingest-time settlers** — *idempotent, model-settling* transforms
  (segmentation, normalization) belong to **bringing content into the store**,
  not to each flow. They run **once at ingest** and persist as overlays; later
  flows see the settled model and never recompute it. This removes redundant
  per-run work and the drift hazard of re-settling the canonical model on every
  run.
- **Run brackets** — *paired, policy-bearing* transforms (redact … unredact,
  [AD-020](020-redaction.md)) bracket a single run and may vary per run or
  provider. They stay part of the **run's** source/sink wiring: the `Start`
  redacts the source binding, the `End` restores in the sink binding. The
  built-in `secure-translate` flow (redact · ai-translate · unredact) is exactly
  this `Start(redact) → {translate} → End(unredact)` shape.

A transform that is genuinely both (idempotent *and* recoverable) may be declared
at ingest; the run-bracket form is reserved for transforms whose restore must
happen inside the run.

### 5. Resolving a binding across the CLI and flow surfaces

A binding is named by the same small scheme vocabulary (§1) on every surface —
the CLI, the flow document, and the resource URIs the tool resolver already
understands (`tm:`, `termbase:`, `srx:` in `core/flow/resolve.go`). This mirrors
two conventions a user already knows: *detect-by-extension with an explicit
override* (as in format-converting tools) and *scheme-prefixed endpoints* (as in
file-sync tools).

**Precedence.** A concrete binding is resolved from the first source that names
one, in order: an explicit CLI flag, the project / `.klz` context, the flow's
declared intent, then auto-detection. `kapi run --explain` prints the resolved
`source → sink` and executes nothing, so the chosen binding is always visible
rather than inferred.

**The CLI carries the locator; bare paths are detected, schemes are explicit.**
`-i` / `-o` accept either a plain path or a `scheme:` locator. A plain path is
bound by detection — its extension or kind decides it (`.klz` → the workspace
store, `.xliff` / `.po` → interchange, a plain document → `file`, a directory
inside a project → the project store). A `scheme:` locator forces the binding and
removes any ambiguity: `-o store:` is the block store, while `-o l10n/` is a
directory of files. `file:` forces a path that would otherwise read as a scheme.
Each example below shows the resolved `source → sink`:

```bash
kapi run translate -i a.json -o b.json          # file(a.json)    → file(b.json)
kapi run translate -i a.json                     # file(a.json)    → store        (in a project: process-only)
kapi run translate -i work.klz                   # store(work.klz) → store        (.klz transformed in place)
kapi run translate -i work.klz --pack            # store(work.klz) → store, then ejected to the .klz
kapi run translate -i store: -o xliff:hand.xliff # store           → interchange(hand.xliff)
kapi run qa-check  -i a.json -o none             # file(a.json)    → none         (analysis; report only)
kapi extract src/*.json -o work.klz              # file(glob)      → store(work.klz)
kapi merge -o l10n/{lang}/{name}.{ext}           # store           → file(template)
```

`extract`, `merge`, and `pack` are named presets for the bindings their names
imply; `run` is the general form. All resolve through the same precedence and
report the same `--explain` line.

**The flow declares intent, never a location.** A flow document carries a binding
only when it is *intrinsic to what the flow is*, and then only the *kind* — never
a path or a concrete store. A translation flow materializes, so it leaves its
sink unset and lets the invocation place the result; an analysis or QA flow
inherently produces no document, so it declares `sink: none`; a flow that only
makes sense over an existing workspace may declare `source: store`.

```yaml
# A translate flow: binding-agnostic. The ends come from where it is run.
spec:
  steps:
    - tool: tm-leverage
    - tool: ai-translate
    - tool: qa-check
```

```yaml
# A QA flow: intrinsically process-only. It never emits a document, anywhere.
spec:
  sink: none
  steps:
    - tool: qa-check
```

Because a flow's only binding is intrinsic intent, there is no per-flow output
path for a reader to be surprised by; the same flow document runs over a loose
file, a `.klz` workspace, or a project without edit, and `--explain` shows where
a given run's content actually lands.

**In a project, a run lands in the store.** When a `.kapi` recipe is in scope, a
run with no explicit sink commits its work as overlays to the project block store
and emits no document. Materializing the localized files is a separate, explicit
step (`kapi merge`). The store is the working copy: a re-run reuses the overlays
already present and recomputes only what changed
([AD-025](025-klf-package.md) §5).

## Consequences

- A flow definition is portable across origins: the same flow runs in the file
  CLI, a `.klz` workspace, and a project, because it only ever sees a session of
  Blocks.
- The synthetic `"merge"` flow, the `hydrateTargetsTool`, and the `Store`
  side-channel in `cli/klzworkspace.go` collapse into ordinary `source` / `sink`
  bindings.
- Process-only runs make incremental, resumable workflows the default rather than
  a special case; materialization is deferred and explicit.
- `kapi run flow -i file.json -o out.json` is unchanged — the `file` binder is
  the default, so there is no CLI regression.
- Ingest-time settling removes per-run segmentation/normalization recomputation
  and keeps the canonical model stable across a project's lifetime.
- The flow editor surfaces source/sink as **endpoint pickers** (file · store ·
  import/export · none), not arbitrary reader/writer nodes; capability gating for
  the source-transform stage ([AD-002](002-content-model.md) overlays,
  segmentation) is unaffected.
- The runtime executor is unchanged — it already binds nothing. This is a model,
  naming, and glue refactor, not an engine rewrite.
- The flow noun sharpens to mean *composition*: removing I/O strips the one job
  that diluted it, leaving configuration, topology, identity, and phase
  structure. A single tool is a tool, not a flow — so the concept is load-bearing
  exactly where it is used and absent where it was overkill.
- One scheme vocabulary spans the CLI locator, the flow document, and the tool
  resolver, so a binding reads the same wherever it appears. Bare paths keep the
  zero-ceremony common case; `scheme:` is the unambiguous escape hatch.
- A documented precedence plus `--explain` keeps the resolved binding visible, so
  layered defaults (flow intent under project context under an explicit flag) do
  not become hidden configuration.

## Related

- [AD-004: Processing Engine](004-processing-engine.md) — the executor is already
  the pure middle; this AD names the endpoints it was always agnostic to.
- [AD-005: Format System](005-format-system.md) — readers/writers become the
  `file` binding's implementation; skeleton round-trip is a sink concern.
- [AD-008: Project Model](008-project-model.md) — the project block store as a
  source and sink.
- [AD-025: KLF Family and the .klz Package](025-klf-package.md) — the `.klz`
  workspace; process-only = the `store` sink; content-derived cached resume.
- [AD-020: Content Redaction](020-redaction.md) — redact/unredact as run
  brackets rather than ingest settlers.
- [AD-002: Content Model](002-content-model.md) — overlays are the unit a sink
  commits and a source rehydrates.
</content>
</invoke>
