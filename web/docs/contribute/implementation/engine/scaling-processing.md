---
sidebar_position: 10
id: scaling-processing
title: "Note: Scaling the processing primitive"
description: How the same processing primitive runs in-process, as a queue-backed worker, and as a gRPC daemon, the seams that let it become an independently scalable service, and the direction for the first such services.
keywords: [scale-out, processing primitive, RunOnParts, tool service, gRPC daemon, queue worker, context graph, autoscaling, implementation note]
---

# Scaling the processing primitive

This implementation note describes how the one processing primitive runs at
several scales today, the seams that let it become an independently scalable
service, and the direction the first such services take. It is grounded in the
engine architecture: the tool contract ([E-03](../../architecture/engine/e-03-tool-system)),
the streaming executor ([E-01](../../architecture/engine/e-01-processing-engine)),
flows and I/O binding ([E-04](../../architecture/engine/e-04-flows-and-io-binding)),
the plugin transports ([E-05](../../architecture/engine/e-05-plugin-system)), and
the context store and graph ([C-03](../../architecture/context/c-03-context-store-and-graph)).

The phased delivery is tracked in the scale-out epic
([#2392](https://github.com/neokapi/neokapi/issues/2392)) and its per-phase
issues; this note is the architecture those issues realize.

## The primitive

A tool is a single streaming stage over the content model: it reads `*Part`,
writes `*Part`, and communicates only through overlays and annotations on
blocks, never tool to tool. The smallest reusable contract is
`tool.RunOnParts(ctx, t, parts) ([]*Part, error)` (`core/tool`): it needs only a
tool and a `[]Part`. That pair, the tool identity and its config plus a slice of
parts, is the natural payload for a remote call, and `core/plugin/protoconvert`
already translates a `Part` to and from its protobuf form.

A flow is composition only: a graph of tool nodes, or a step list compiled to
that graph. A flow owns no I/O; where content enters and leaves is a binding
resolved at invocation (a file, the project store, an interchange file, or none).
The same flow, and the same primitive under it, runs over any binding.

## How it runs today

The primitive is dispatched two ways:

- **In-process.** Built-in and model-backed tools run inside the calling
  process, driven by the streaming executor's four composable concurrency
  layers (intra-tool block parallelism, batch file concurrency, document-level
  concurrency, streaming observation). This drives a single file on a laptop, a
  batch, and a long-lived project alike.
- **Over a gRPC daemon.** A plugin can run as a long-lived daemon over a
  Unix-domain socket and gRPC (the plugin system's Mode C). This is a working
  tool-over-gRPC model in production: the Okapi bridge is a JVM daemon exposing
  dozens of filters this way, and `plugins/pdfium` and `plugins/sourcecode` are
  first-party daemons. The gRPC service definitions live in `core/plugin/proto`,
  the pool and dispatch in `host/pluginhost`.

One gap sits inside this second mode: the manifest transport for a plugin that
contributes a full **block-processing tool** over the daemon is declared but not
yet built. Only segmenters cross the daemon boundary today, over a dedicated
`Segment` RPC. The protocol surface exists; the general tool RPC is the missing
piece.

A third scale is a **queue-backed worker**: a headless process that consumes a
message queue and runs the same framework tools over content loaded from the
shared stores, writing results back. A worker is a stateless consumer of shared
state (the content store, the content memory, terms, voice profiles, the blob
store, an event bus), so more than one can run against the same queue. The claim
is lease and epoch guarded and apply is idempotent and atomic, so concurrent
consumers are safe.

## The shared substrate

Processing connects content through the context graph. `core/contextgraph`
defines the node and edge vocabulary, the `(workspace, project, stream)` scope
tuple, and the id scheme; the same vocabulary is written into the local store and
the server-side store, so one query runs against either. The graph is a
projection, derived and rebuilt when the committed sources change, never
authoritative. `core/graph.GraphStore` is the CRUD and traversal interface, with
SKOS-aligned edge labels, over plain adjacency tables so it runs on a stock
relational store.

A service that reads blocks and their overlays and writes stand-off annotations
plus graph nodes and edges fits this substrate directly, scoped by the same
tuple and triggered by the same content-changed event that drives the projection
rebuild.

## The seams that already support scale-out

- **The primitive payload.** `RunOnParts` plus `Part` to and from protobuf is a
  ready remote contract for a tool service.
- **The daemon boundary.** The Mode-C gRPC daemon is a working precedent for a
  tool running as its own process.
- **The queue boundary.** A stateless worker consuming a queue against shared
  state, with lease and epoch guarded claims and idempotent apply, already runs
  the framework tools.
- **The shared stores.** The content store, content memory, terms, voice
  profiles, blob store, and event bus are shared state that workers consume
  rather than own.

## Direction

The scale-out builds independently scalable services on the framework side of
the licence line, over these seams.

- **One primitive-run contract.** A uniform "run this primitive over this scope"
  entry both the synchronous and the asynchronous execution paths route through,
  carrying the tool identity, its config, and the parts, replacing the paths that
  hand-roll a fixed tool ordering. Its payload is `RunOnParts` plus the existing
  part-to-proto translation.
- **First services on framework tools.** The first two services already exist as
  framework tools and need assembly, not new mechanism: a term tagger runs
  `term-lookup` and `term-extract` (`terms/tool.go`) to write term overlays and
  candidate concepts; a graph analyzer reads blocks and their term and entity
  overlays and writes the connective edges into the graph store. Each is its own
  queue-backed service, triggered by the content-changed event.
- **Incremental graph analysis.** The graph writer reprojects a whole scope's
  subgraph per change today. A finer-grained analyzer keyed on the change's
  declared tree replaces the whole-scope rebuild.
- **The general tool daemon.** Finishing the not-yet-built Mode-C path for a full
  block-processing tool lets a term tagger or graph analyzer run as a separately
  deployed, separately scaled gRPC daemon that both the CLI and a server dispatch
  to identically, the same shape the Okapi bridge already takes.
- **Horizontal worker scaling.** Nothing in the code blocks more than one worker
  against a queue; running several is a deployment configuration, not an
  architectural change.

## The licence line decides where a service is built

The licence boundary is directory containment under a `LICENSE` file. The
framework (the root module), `host/`, `cli/`, and `kapi/` are Apache-2.0. A
service built only on the framework tools (`core/tool`, `core/flow`,
`core/ai/tools`, `core/mt/tools`, `terms/`, `core/plugin/*`) plus a thin gRPC
shell is Apache-clean and reusable by the CLI and any server alike, exactly as
the first-party plugins already demonstrate. `make audit-modules` enforces that
no Apache module reaches the platform tree. Building the first services on the
framework side of the line is therefore the decision: it keeps them reusable and
independently deployable, where a service carved from the platform's own job
code could be neither.
