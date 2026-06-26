---
id: 026-flow-io-binding
sidebar_position: 26
title: "AD-026: Flow I/O Binding — Source → Flow → Sink"
description: "Architecture decision: a flow is a pure transformation over a stream of Blocks backed by a block-store session; where content enters (the source binding) and where results go (the sink binding) are resolved from invocation context rather than baked into the flow graph. The same flow runs over a file, a .klz workspace, the project block store, or an imported interchange file, and a sink is optional — a process-only run lands its work as overlays in the project/.klz and defers materialization to a later merge/export."
keywords: [flow, source, sink, binding, block store, klz, process-only, transformer, redaction, segmentation, reader, writer, pipeline, architecture decision]
---

# AD-026: Flow I/O Binding — Source → Flow → Sink

## Summary

Three nouns divide the work cleanly: a **tool** is the unit of work, a **flow**
is a named reusable *composition* of tools, and a **binding** is an end — where
content enters and leaves. A flow is a **pure transformation over a stream of
Blocks** backed by a block-store session: it owns no I/O, and a single tool is
not a flow. *Where content enters* (the **source binding**) and *where results
go* (the **sink binding**) are resolved from invocation context, not encoded in
the flow graph. The same flow definition runs whether its content comes from a
file, the `.klz` workspace cache, the project block store, or an imported
interchange file — and whether its results are written to a file, committed as
overlays to the store, or both.

A sink is **optional**. A *process-only* run lands its work as overlays in the
project / `.klz` and emits no file; materialization is a separate, later sink
operation (`merge` / `export` / `pack`). This gives the `.klz` lifecycle a
first-class shape: `extract` (source → store), `run` / transform (store → store),
`merge` (store → file).

A binding can also **fan out**: a *container* source (a ZIP/TAR namespace, §6)
expands to one `file` run per inner entry — so a packaged format nested inside an
archive round-trips faithfully — and a *barrier* sink repacks the results,
copying untouched members byte-for-byte. The same enumerate→process→write-back
shape generalizes from files to **remote providers** (a CMS / API collection of
items, §7), which is how Bowrain's connectors fit the same abstraction.

**Transformers** ([AD-006](006-tool-system.md)) are of two kinds: *idempotent
model-settling transforms* that run **once at ingest** and persist to the store,
and *round-trip-paired brackets* (redact … unredact) that are part of a **run's**
source/sink wiring.

Bindings are named by one scheme vocabulary across the CLI, the flow document,
and the existing resource URIs. A concrete binding resolves by precedence —
explicit flag, then project / `.klz` context, then the flow's intent, then
auto-detection — and `kapi run --explain` always shows the resolved
`source → sink` so nothing is hidden. A flow declares only *intrinsic intent*
(`sink: none` for an analysis flow), never a path.

## Context

A localization pipeline runs at many origins and destinations. The same
translation flow processes a loose file on a laptop, the blocks already held in a
project's store, a `.klz` workspace, or content imported from an interchange
file; and its results land in a translated file, as overlays committed to the
store, or in an interchange file bound for a translator. The work the flow does —
leverage, translate, check — is the same in every case; only where the content
enters and leaves differs.

The processing engine is built around that fact. `DefaultExecutor`
(`core/flow/executor.go`) orchestrates tools over a `blockstore.Session` and has
no notion of files, readers, or writers — I/O lives at the edges, outside the
flow. This AD names those edges and settles two questions:

1. The flow's shape is `source → {flow} → sink`: the ends are context-wired
   bindings, not a fixed read → process → write baked into the graph.
2. A run need not produce a file: it can be **process-only**, landing its work in
   the store.

## Decision

### Three nouns: tool, flow, binding

I/O sits outside the flow, leaving three concepts, each with exactly one job:

- **Tool** — the unit of work. A single capability-typed transformation over the
  Part stream — `Annotate`, `Translate`, or `Transform`
  ([AD-006](006-tool-system.md)). A tool runs on its own; it needs no flow.
- **Flow** — a named, reusable **composition** of tools. A flow carries the
  ordering, the branching (`parallel:`, tee, batch), and the per-tool
  configuration — and nothing else. It is *the recipe*.
- **Binding** — the ends. Where content enters (source) and where results leave
  (sink). A binding belongs to neither the tool nor the flow; it is supplied by
  the invocation and the project (§1–§5).

A flow is **composition, and only composition.** It owns no I/O, and a single
tool is not a flow: a lone tool is invoked directly as a tool command, and
`kapi flows` lists only the compositions. The flow noun earns its place by
carrying the four things a flat list of tool names cannot:

- **Configuration** — a flow pins each tool's settings, so it is a *configured*
  recipe (`recycle{fuzzy:75}` → `translate{provider:anthropic}` →
  `qa`), not merely an ordered set of tool names.
- **Topology** — a flow is a DAG. `parallel:` fan-out, `tee`, and `batch` are
  graph shapes a sequence cannot express.
- **Identity and reuse** — a flow has a name and a source (built-in, user,
  project). A project's `flows:` block is its vocabulary of named operations,
  versioned with the recipe and shared like any other artifact. A flow is
  portable, declarative intent and owns no I/O, so it travels in a project's
  portable twin — the `.klz` package — like any other recipe field
  ([AD-025](025-klf-package.md) §6).
- **Transformer roles** — ingest-time settlers and the round-trip brackets (§4)
  are distinct transformer roles, validated by the placement pass
  ([AD-006](006-tool-system.md)), not a flat run of tools.

What a flow is **not**: it is not an I/O harness (that is the binding), it is not
a runtime primitive beyond an ordered tool chain over a session
([AD-004](004-processing-engine.md)), and it is never required to run one tool.

### 1. The flow is the middle; source and sink are bindings

A flow operates only on a stream of Parts backed by a session. The endpoints are
a small, separate **binding** vocabulary, resolved from invocation context:

| Binding | Source role (in) | Sink role (out) |
| --- | --- | --- |
| `file` | `DataFormatReader` over file bytes | `DataFormatWriter` + skeleton round-trip ([AD-005](005-format-system.md)) |
| `container` | a ZIP/TAR namespace fanned out to one `file` source per inner entry (§6) | a *barrier* sink that repacks — replaced entries spliced in, every other member copied byte-for-byte |
| `store` / `klz` | existing blocks + overlays from a persistent store | commit overlays — no materialization |
| `import` / `export` | overlays landed from an interchange file ([AD-017](017-bilingual-format-interop.md)) | emit interchange (bilingual `.klz`, XLIFF / PO / TMX / TBX) |
| `none` | — | discard (observation/metrics only) |

The defining property: **a flow definition is identical across bindings.** The
same `translate-qa` flow runs in the file CLI, against a `.klz` workspace, and
against a project — only the binding differs.

Each binding also advertises the **ports** it provides ([AD-002](002-content-model.md),
[AD-006](006-tool-system.md)): a plain `file` source carries source content
only; a bilingual interchange source adds a committed `target`, segmentation
and alignment; the content store adds every persisted stand-off layer. The flow
loader uses this to validate the contract end to end — a flow whose first tool
needs a port the source cannot supply, with no upstream tool to produce it, is rejected
at build (`FlowDefinition.ValidateDataFlow`). So `qa` (which requires a
`target`) is valid against a bilingual source or after a translate step, but
rejected against a plain monolingual `file` source on its own.

### 2. Reader and writer are bindings, not graph nodes

The flow document carries only its steps. Where content enters and leaves is a
top-level `source:` / `sink:` spec, not a node in the tool graph. The `file`
binding is the default, so an unqualified
`kapi run flow -i file.json -o out.json` is `source: file`, `sink: file`. A
`.klz` workspace is `source: store`; `merge` is `source: store` with
`sink: file`. A single binder interface backs them all, so the engine never
special-cases an origin.

```yaml
apiVersion: v1
kind: FlowDefinition
metadata:
  name: Production Pipeline
spec:
  source: file        # default; or `store`, `klz`, `import:xliff`
  sink: store         # process-only: commit overlays, emit nothing
  steps:
    - tool: recycle
    - tool: translate
    - tool: qa
```

### 3. Sink is optional → process-only runs

A run whose `sink` is `store` (or absent) commits its overlays to the project /
`.klz` block store and **emits no file**. Materialization is a distinct sink
operation — `merge` (store → file via skeleton), `export` (store → interchange),
or `pack` (store → `.klz`). This separates *doing the work* from *handing it
out*, and gives the workspace lifecycle its natural grain:

| Command | Source | Sink | |
| --- | --- | --- | --- |
| `extract` | file | store | ingest sources into the store |
| `run` / transform | store | store | **process-only** — commit overlays, emit nothing |
| `merge` | store | file | materialize via the skeleton |

Because the block store is append-only and content-addressed, a process-only run
is **idempotent and resumable**: re-running skips work whose overlay already
exists, anchored to the current block hashes ([AD-025](025-klf-package.md) §5).
The store *is* the workspace.

### 4. Transformers: settlers and brackets

Transformers ([AD-006](006-tool-system.md)) are ordinary ordered steps; the
framework applier rewrites the source inline, so each transformer settles the
model before the steps that follow it, and the placement pass validates the
ordering. At the binding level their two uses are distinct:

- **Ingest-time settlers** — *idempotent, model-settling* transforms
  (segmentation, normalization) belong to **bringing content into the store**,
  not to each flow. They run **once at ingest** and persist as overlays; later
  flows see the settled model and never recompute it. This avoids redundant
  per-run work and the drift hazard of re-settling the canonical model on every
  run.
- **Run brackets** — *paired, policy-bearing* transforms (redact … unredact,
  [AD-020](020-redaction.md)) bracket a single run and may vary per run or
  provider. They are part of the **run's** source/sink wiring: the `Start`
  redacts the source binding, the `End` restores in the sink binding. The
  built-in `secure-translate` flow (redact · translate · unredact) is exactly
  this `Start(redact) → {translate} → End(unredact)` shape.

A transform that is genuinely both (idempotent *and* recoverable) may be declared
at ingest; the run-bracket form is for transforms whose restore must happen
inside the run.

### 5. Resolving a binding across the CLI and flow surfaces

A binding is named by the same small scheme vocabulary (§1) on every surface —
the CLI, the flow document, and the resource URIs the tool resolver understands
(`tm:`, `termbase:`, `srx:` in `core/flow/resolve.go`). This follows two
conventions a user already knows: *detect-by-extension with an explicit override*
(as in format-converting tools) and *scheme-prefixed endpoints* (as in file-sync
tools).

**Precedence.** A concrete binding resolves from the first source that names one,
in order: an explicit CLI flag, the project / `.klz` context, the flow's declared
intent, then auto-detection. `kapi run --explain` prints the resolved
`source → sink` and executes nothing, so the chosen binding is always visible.

**The CLI carries the locator; bare paths are detected, schemes are explicit.**
`-i` / `-o` accept either a plain path or a `scheme:` locator. A plain path is
bound by detection — its extension or kind decides it (`.klz` → the workspace
store, `.xliff` / `.po` → interchange, a plain document → `file`, a directory
inside a project → the project store). A `scheme:` locator forces the binding and
removes any ambiguity: `-o store:` is the block store, while `-o l10n/` is a
directory of files. `file:` forces a path that would otherwise read as a scheme.
Each example shows the resolved `source → sink`:

```bash
kapi run translate -i a.json -o b.json          # file(a.json)    → file(b.json)
kapi run translate -i a.json                     # file(a.json)    → store        (in a project: process-only)
kapi run translate -i work.klz                   # store(work.klz) → store        (.klz transformed in place)
kapi run translate -i work.klz --pack            # store(work.klz) → store, then ejected to the .klz
kapi run translate -i store: -o xliff:hand.xliff # store           → interchange(hand.xliff)
kapi run qa  -i a.json -o none             # file(a.json)    → none         (analysis; report only)
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
produces no document, so it declares `sink: none`; a flow that only makes sense
over an existing workspace may declare `source: store`.

```yaml
# A translate flow: binding-agnostic. The ends come from where it is run.
spec:
  steps:
    - tool: recycle
    - tool: translate
    - tool: qa
```

```yaml
# A QA flow: intrinsically process-only. It never emits a document, anywhere.
spec:
  sink: none
  steps:
    - tool: qa
```

A flow's only binding is intrinsic intent, so there is no per-flow output path to
surprise a reader; the same flow document runs over a loose file, a `.klz`
workspace, or a project, and `--explain` shows where a given run's content lands.

**In a project, a run lands in the store.** When a `.kapi` recipe is in scope, a
run with no explicit sink commits its work as overlays to the project block store
and emits no document. Materializing the localized files is a separate, explicit
step (`kapi merge`). The store is the working copy: a re-run reuses the overlays
already present and recomputes only what changed
([AD-025](025-klf-package.md) §5).

### 6. Container bindings: a source that fans out, a sink that repacks

Some inputs are not one document but a **namespace of documents**: a ZIP, a TAR,
a `.tar.gz`. These are **not formats** — a format is the implementation of the
`file` binding for a *single* document ([AD-005](005-format-system.md)). A
container is a *binding*: it decides where content enters and leaves, and it
expands to many `file` bindings.

This is the same shape AD-026 already endorses for `kapi extract src/*.json -o
work.klz` — `file(glob) → store`, a source that fans out to N documents. A
container source is that pattern with the namespace *inside* a container instead
of on the filesystem. The decisive property follows for free: **each inner entry
is a real, standalone `file` run**, so it inherits the whole file machinery —
per-entry format detection, per-entry configuration, and the `file` sink's
**skeleton round-trip**. A packaged, skeleton-bound format (DOCX, PPTX, EPUB,
ODF, IDML) *inside* the container therefore round-trips faithfully, because it is
processed by its own reader and writer, not flattened into a parent document.

- **Source (fan-out).** Enumerate the container's regular-file entries; each is
  resolved and run as its own `file` source. An entry whose format is binary
  (image/audio/video), a bilingual interchange file, a nested container, or
  unrecognised is **not** processed — it is carried to the sink untouched.
  Enumeration is recursive in principle (a ZIP inside a TAR), bounded by the
  shared zip-bomb / size / entry-count guards.
- **Sink (barrier repack).** Unlike a folder sink, which writes N independent
  files, a container sink must emit **one valid container atomically**. It is a
  *barrier*: it buffers the processed entries, then rebuilds the container from
  the **original bytes**, splicing in only the entries that were processed and
  copying every other member — structure, entry order, metadata, compression,
  binaries — byte-for-byte.

The fan-out and repack are a small, provider-agnostic substrate (`core/container`:
`Enumerate` / `Repack`) with no dependency on the format registry or the flow
engine; the per-entry *processing* is injected by the caller. A read-only
`archive` reader is kept as the **inspection** face only — it surfaces each
entry's content so `kapi inspect bundle.zip` shows what is inside — but it has no
writer, because localizing a container is the binding above, not a format
round-trip.

**Addressing.** A container fits the locator vocabulary (§5): a bare `.zip` /
`.tar` / `.tgz` / `.tar.gz` path detects as a container, and a single inner
entry is addressed with the JAR-style bang separator — `release.zip!docs/x.md` —
repeatable for nesting (`outer.tar.gz!inner.zip!file.json`). No new URL scheme is
introduced: kapi inputs stay paths, so a scheme prefix is reserved for genuine
remote endpoints (§7).

**Per-entry configuration.** Because each entry is an ordinary `file` run, the
recipe's existing per-format config and presets apply to inner content the same
way they apply to loose files; there is no parallel "entries" configuration
language. (Matching recipe *content-items* to inner entry paths — so a glob like
`**/*.docx` can pin an entry's format/config — is an additive refinement on top
of this, not a new mechanism.)

### 7. Beyond files: provider sources/sinks

The container shape — *enumerate a collection into independent items, process
each, write the results back as a batch* — is not specific to archives. It is the
same shape a **remote provider** has: a CMS, a headless API, or a SaaS connector
exposes a *collection* (a space, a project, a content type) whose *items* are
individual documents. In Bowrain's connector model these are the WordPress,
Figma, and HubSpot connectors (server- and desktop-side; the local `file` / `git`
connectors are the filesystem instance of the same idea).

The binding vocabulary generalizes cleanly:

| | container (file) | provider (remote/CMS) |
| --- | --- | --- |
| source fan-out | enumerate archive entries | list collection items (paginated) via the API |
| per-item run | inner `file` run + skeleton round-trip | item run; the connector supplies the format (often a rich-text JSON or HTML body) |
| sink | barrier repack into one archive | batch write-back: PATCH/PUT each changed item, or one bulk call |
| addressing | `release.zip!path` | `wordpress://site/posts/123`, `figma://file/KEY/node/1:2` |
| identity for resume | entry path | the provider's stable item id + revision |

Two differences matter, and they are properties of the provider, not of the
binding contract:

1. **The sink is rarely byte-exact and rarely atomic.** A filesystem container is
   rebuilt wholesale from original bytes; a remote sink writes each item back
   through the provider's API, item-by-item, and "everything else preserved" is
   the provider's responsibility, not a byte copy. So a provider sink is an
   *incremental* barrier (write each processed item; leave the rest) rather than a
   *whole-artifact* one.
2. **Identity is provider-defined and must support resume.** An archive entry is
   addressed by path; a remote item by a stable id + revision/etag. This is
   exactly what the `store` binding already wants — content-addressed,
   resumable, process-only overlays ([AD-025](025-klf-package.md) §5) — so the
   natural pattern is **provider source → store** (extract once, resumable) and a
   later **store → provider** publish, mirroring `extract` / `merge` for files.

So the recommendation is to treat remote/CMS connectors as the *provider* family
of the same source/sink abstraction: reuse the `Enumerate → process → write-back`
contract and the `store` binding for incremental state, and let each connector
supply enumeration, per-item format, item identity, and write-back. The
`core/container` substrate is the file instance; a connector is the remote
instance. What is **not** shared is the byte-exact whole-artifact repack — that is
a property of a self-contained file, and a remote provider substitutes its own
API write-back.

## Consequences

- A flow definition is portable across origins: the same flow runs in the file
  CLI, a `.klz` workspace, and a project, because it only ever sees a session of
  Blocks.
- A `.klz` workspace, `extract`, and `merge` are ordinary `source` / `sink`
  bindings, not special cases.
- Process-only runs make incremental, resumable workflows the default; a file is
  materialized only when a sink asks for it.
- `kapi run flow -i file.json -o out.json` is the `file` binding on both ends —
  the zero-ceremony common case.
- Ingest-time settling avoids per-run segmentation/normalization recomputation
  and keeps the canonical model stable across a project's lifetime.
- The flow editor surfaces source/sink as **endpoint pickers** (file · store ·
  import/export · none) rather than reader/writer nodes; transformer placement
  ([AD-006](006-tool-system.md)) and overlay capability
  ([AD-002](002-content-model.md)) are independent of bindings.
- The executor binds nothing: it orchestrates tools over a session, and the
  bindings sit outside it.
- A container (ZIP/TAR) is a binding, not a format: it fans out to one `file` run
  per entry, so per-entry detection, config, and skeleton round-trip come for
  free — a DOCX/EPUB *inside* an archive round-trips faithfully — and a barrier
  sink repacks, preserving untouched members byte-for-byte. The fan-out/repack is
  a provider-agnostic substrate (`core/container`); a read-only `archive` reader
  remains only as the inspection face.
- The same enumerate→process→write-back shape extends to remote/CMS providers
  (§7): a connector supplies enumeration, per-item format, item identity, and an
  incremental API write-back, while reusing the `store` binding for resumable
  state. The byte-exact whole-artifact repack is the file-only specialization.
- The flow noun means *composition*: with I/O at the edges, a flow carries
  configuration, topology, identity, and phase structure. A single tool is a
  tool, not a flow — the concept is load-bearing where it is used and absent where
  it would be overkill.
- One scheme vocabulary spans the CLI locator, the flow document, and the tool
  resolver, so a binding reads the same wherever it appears. Bare paths keep the
  zero-ceremony common case; `scheme:` is the unambiguous escape hatch.
- A documented precedence plus `--explain` keeps the resolved binding visible, so
  layered defaults (flow intent under project context under an explicit flag) are
  never hidden configuration.

## Related

- [AD-004: Processing Engine](004-processing-engine.md) — the executor is the pure
  middle; this AD names its endpoints.
- [AD-005: Format System](005-format-system.md) — readers/writers are the `file`
  binding's implementation; skeleton round-trip is a sink concern.
- [AD-008: Project Model](008-project-model.md) — the project block store as a
  source and sink.
- [AD-025: KLF Family and the .klz Package](025-klf-package.md) — the `.klz`
  workspace; process-only = the `store` sink; content-derived cached resume.
- [AD-020: Content Redaction](020-redaction.md) — redact/unredact as run brackets
  rather than ingest settlers.
- [AD-002: Content Model](002-content-model.md) — overlays are the unit a sink
  commits and a source rehydrates.
