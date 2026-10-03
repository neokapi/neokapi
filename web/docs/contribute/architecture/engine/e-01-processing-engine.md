---
id: e-01-processing-engine
sidebar_position: 1
title: "E-01: The processing engine"
description: "Content is processed by a channel-based streaming pipeline: each tool runs in its own goroutine, connected by buffered channels, with errgroup for error propagation and context for cancellation."
keywords: [neokapi, architecture decision, processing engine, pipeline, goroutines, channels, errgroup, streaming, executor, flow definition]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# E-01: The processing engine

## Summary

Content is processed by a channel-based streaming pipeline. Each tool runs in
its own goroutine; tools are connected by buffered channels that provide
automatic backpressure. An `errgroup.Group` coordinates errors and propagates
context cancellation. Block parallelism within a tool, file and document
concurrency, and streaming observation each have their own control. A flow is
declared as a graph of nodes and edges or as a list of steps, and it runs as an
ordered chain of tools: the host that loads a definition derives that order,
and a parallel stage inside the chain admits a bounded number of Parts.

## Context

Go's goroutines and channels make it natural to structure a pipeline as
concurrent stages connected by typed channels. Such a pipeline mixes CPU-bound
work (format parsing, checks) with IO-bound work (LLM calls, memory lookups).
The same pipeline must also be driven at several scales: a single file on a
laptop; hundreds of files in a batch; a long-lived project with many documents
processed in parallel.

It must also support both declarative authoring (a visual flow editor,
human-readable YAML) and programmatic construction (the Go library) from one
data model.

## Decision

### Channel-based pipeline

Content flows through a channel-based concurrent pipeline:

<PipelineDiagram
  animated
  stages={[
    { label: "Source", sub: "binding", role: "io" },
    { label: "Tool 1", note: "goroutine" },
    { label: "Tool 2", note: "goroutine" },
    { label: "⋯" },
    { label: "Sink", sub: "binding", role: "io" },
  ]}
/>

Content enters through a **source** binding and leaves through a **sink**
binding ([E-04](e-04-flows-and-io-binding.md)). For the default `file` binding
these are a DataFormat reader and writer ([E-02](e-02-format-system.md)); a
project store, a `.kpz`, or an interchange file bind the same stream with no
reader or writer. Between the ends, each tool runs in its own goroutine.
Buffered channels provide backpressure. `errgroup.Group` coordinates error
handling across goroutines, and context cancellation propagates to all stages.

Parts carry typed resources ([F-02](../foundations/f-02-content-model.md)):
Blocks contain translatable content, Data carries structural markup, Layers
group nested content, Media holds binary assets. Tools declare which resource
types they handle; the rest pass through unchanged.

### Executor

`flow.DefaultExecutor` orchestrates tool chains using the goroutine-per-tool
model:

- Each tool in the chain runs in its own goroutine.
- Buffered channels (configurable size) connect adjacent tools.
- `errgroup` collects the first error and cancels the shared context.
- Parallel document processing is bounded by a semaphore worker pool.
- `ToolFactories` create fresh tool instances per document, so concurrent
  documents never share mutable tool state.

Configuration uses the functional-options pattern:

```go
executor := flow.NewExecutor(
    flow.WithMaxConcurrency(8),
    flow.WithChannelSize(128),
    flow.WithCollectors(wordCounter, checkReport),
)
```

`flow.WithImmutabilityCheck` turns on the per-dispatch content-immutability
backstop for a run ([E-03](e-03-tool-system.md)); it is dev/test tooling and is
off unless a caller asks for it.

### Parallel block processing

For IO-bound tools (LLM translation, remote checks), sequential per-part
processing underutilizes throughput. `tool.ParallelBlockTool` runs the block
handler of a `*tool.BaseTool` on a fixed pool of N workers while preserving
strict Part order. A wrapped tool of another concrete type, or one with no typed
block handler, runs through its own `Process`, and a wrapped `SessionTool` keeps
its `SessionProcess`:

<PipelineDiagram
  stages={[
    { label: "Input" },
    { label: "Dispatcher", sub: "seq numbers · N blocks admitted", role: "annotate" },
    {
      role: "translate",
      parallelLabel: "fixed pool · N workers",
      lanes: [{ label: "Worker 1" }, { label: "Worker 2" }, { label: "Worker N" }],
    },
    { label: "Reassembly", sub: "bounded ring · in order", role: "annotate" },
    { label: "Output" },
  ]}
/>

The dispatcher numbers every incoming Part and admits at most N blocks whose
results have not yet reached the output. A block keeps its place in that window
from admission until it is emitted, so a slow first block stops the dispatcher
reading further input even when later blocks have finished, and downstream
backpressure reaches the input. Non-Block Parts count against a separate window
of 4N, so N blocks stay in flight when a reader puts Data or Group Parts between
them. Reassembly holds the admitted Parts in a ring and emits them in input
order, so downstream tools see the same Part ordering whichever worker finished
first. A dropped Part advances the sequence and emits nothing.

When the inner tool sets a handler for a non-Block Part (Data, Media, Layer,
Group), that handler runs after every earlier block handler has finished and
before any later block starts, so a handler that updates layer or group state
never runs beside a block handler that reads it. A non-Block Part with no
handler passes through in order without that wait. Block handlers must be safe
to call concurrently for distinct blocks and must observe the context they
receive: on an error or cancellation the wrapper cancels its workers and joins
them before it returns.

Auto-parallelism is a **tool** property. Each tool declares
`ToolMeta.DefaultParallelBlocks` ([E-03](e-03-tool-system.md)); the runner takes
the maximum across the flow's tools and wraps every tool at that width. A
project may pin its own value, and `--parallel-blocks N` overrides both
(`--parallel-blocks 1` disables the wrapper). The tools that declare a default
are the LLM-backed ones, so an ordinary rules-only flow runs sequentially with
no configuration. The wrapper parallelizes the typed block handler of a plain
`*tool.BaseTool` only. The LLM-backed and MT tools embed `BaseTool` in types of
their own, so the wrapper runs each of them through its own `Process`, where AI
translation and entity extraction have their own batching and concurrency
settings. A declared default therefore widens the plain `BaseTool` steps that
share a flow with an LLM-backed tool.

### Batch executor

`flow.BatchExecutor` processes multiple pre-read files through a tool chain with
configurable file-level concurrency:

```go
type BatchConfig struct {
    FileConcurrency int         // max files processed in parallel (default: 1)
    ChannelSize     int         // per-pipeline channel buffer size (default: 64)
    SharedResources []io.Closer // resources shared across files (closed at end)
    FailFast        bool        // cancel remaining on first error (default: true)
}
```

Each file gets fresh tool instances from ToolFactory functions, preventing state
leakage between concurrent documents. Results are returned in input file order
regardless of completion order. Collectors are called with mutex protection for
thread-safe aggregation across files.

### Concurrency layering

Each execution surface has its own concurrency control:

| Layer             | Scope                  | Control                                 | Order                        |
| ----------------- | ---------------------- | --------------------------------------- | ---------------------------- |
| ParallelBlockTool | Blocks within one tool | N workers, at most N admitted blocks    | Strict Part order            |
| BatchExecutor     | Multiple files         | FileConcurrency semaphore               | File order preserved         |
| Executor          | Multiple documents     | MaxConcurrency semaphore                | Document order preserved     |
| TappingTool       | Observation            | Input and output interceptor goroutines | Sequential per tool boundary |

The channel capacities and each parallel stage's windows bound the Parts in
flight inside one tool chain, except inside a tool that buffers its input. AI
translation and entity extraction in their batched mode, and media refine, read
a whole document's Parts before they emit any; the layer processor holds one
child layer at a time. A run's working set also depends on what surrounds the
chain: `Execute` collects each document's output Parts, `BatchExecutor`
takes pre-read Part slices and returns output slices, and a file run feeds its
reader concurrently only when the reader is a streaming reader and no pre-read
content is supplied, with a streaming skeleton only when the writer streams too.
The bounds count Parts, so a single large Block or a retained skeleton can still
dominate memory.

### The write stage

A file run ends in the format writer, and the writer's bytes reach the file
through the **file home** of the change contract
([E-09](e-09-the-change-contract.md)), the place every change set commits too.
The file runner (`flow.FileRunner`) digests the destination before it reads
anything, streams the writer's output into a file staged beside the
destination with the destination's mode (`filehome.Home.Produce`), and commits
it: under the destination's advisory lock, the staged file is renamed onto the
destination only while the destination still holds what it held when the run
began. A destination that changed while the run worked (a person saved it, a
change set or another run committed first) keeps what it holds, and the run
reports it as moved (`filehome.ErrMoved`). A destination that already holds the
bytes the run produced is left as it is. A tool or writer error discards the
staged file, so a failed run leaves no partial destination.

A caller that keeps a record of what a run changed follows each document
through `FileRunnerConfig.Documents`: it sees every block as it leaves the
reader and as it reaches the writer, and it commits the staged document itself.
The kapi host records each document a flow writes in a project as one
`content.edit`, applies the run's changes again through the change service when
the destination moved, and, under `--print-ops`, commits through a home that
writes nothing and prints the change set the run would apply
([E-09](e-09-the-change-contract.md#flows)). The kapi host passes the home its
change service commits through, so a flow and a change set writing one file
take turns under one lock.

### Collectors and streaming collectors

Collectors aggregate results across documents (word counts, check reports, term
lists). They implement `Collect(ctx, item, parts)`, called after each document
completes, and `Result()` for the final aggregate. Collectors must be
thread-safe since multiple documents may complete concurrently.

`StreamingCollector` extends `Collector` with `Observe(part)` for inline
observation without adding a pipeline stage. `TappingTool` wraps a tool and its
streaming collector: output Parts are intercepted and passed to `Observe()`
synchronously before forwarding downstream. This enables real-time metrics
without buffering the entire result set. The metrics, tracing and tapping
wrappers share one interceptor: its input side stops waiting for input once the
context is cancelled or the inner tool returns, its output side keeps draining
the inner tool so a send the inner tool does not guard still completes, and
both are joined before the wrapper returns. A cancelled context surfaces as the
context error when the inner tool reports none.

### Flow tracing and visualization

`flow.TraceRecorder` captures timestamped events during flow execution.
`flow.TracingTool` wraps each tool in the chain and records enter/exit events
with Part snapshots. The `--trace path/to/trace.json` flag on `kapi run` enables
tracing. The output is a `FlowTrace` JSON file containing:

- **Nodes**: the tool chain with concurrency metadata
- **Events**: timestamped enter/exit events per Part
- **Part snapshots**: Part state before and after each node
- **Duration**: total flow execution time in microseconds

A browser-based visualization renders the trace as an animated flow diagram with
particles moving through nodes, channel fill indicators, and worker lane
separation for parallel tools. The playback engine supports variable-speed replay
and seeking.

### Observation seam

Tracing to a file is one consumer of a more general seam. `core/observe`
defines a `Tracer` / `Span` interface with a no-op default and no dependency on
any telemetry library, so it costs nothing until a host registers a tracer at
startup; `kapi` and the desktop register none. `flow.WrapWithSpans` wraps each
tool in a chain so that one span opens per tool invocation (`flow.tool`), and
the file runner opens one per format read and write (`format.read`,
`format.write`). A `SessionTool` keeps its session path through the wrapper, so
a resumable run still resumes. Model provider calls carry their own span through
the same seam ([E-07](e-07-model-providers.md)). One span per tool rather than
per Part keeps the cost proportional to the handful of tools a flow has, not the
thousands of Parts it moves.

### Flow definitions

`flow.FlowDefinition` is a JSON/YAML-serializable struct that captures a flow
graph (nodes + edges) and the tool configurations needed to reconstruct a
runnable flow. This separates the declarative description of a flow from its
runtime execution. The runtime `flow.Flow` is narrower than the graph: it holds
an ordered `[]tool.Tool` and connects each tool to the next, so a host turns a
definition into that order before anything runs.

Each `FlowNode` has:

- **ID**: unique identifier within the definition
- **Type**: `flow.NodeTool` for a processing step. `NodeReader` and
  `NodeWriter` remain in the `NodeType` vocabulary for the editor's graph, but
  every built-in and steps-compiled flow carries tool nodes only, because the
  ends are bindings ([E-04](e-04-flows-and-io-binding.md))
- **Name**: the registered name of the tool (e.g. `"pseudo-translate"`)
- **Label**: optional display label for UI rendering
- **Config**: optional key-value configuration map
- **Position**: x/y coordinates for visual layout in the flow editor

> **Bindings ([E-04](e-04-flows-and-io-binding.md)).** A flow's source and sink
> are bindings resolved from invocation context (file, the project store, a
> `.kpz`, interchange import/export, or none), so the same flow runs over any
> origin. Every built-in flow graph carries tool nodes only. The graph is
> composition; a single tool is invoked directly, not wrapped in a one-tool flow.

Each `FlowEdge` connects a source node to a target node. `TopologicalOrder()`
orders the nodes with Kahn's algorithm and reports a cycle as an error;
`Validate()` checks node identifiers, node types and edge endpoints. Each host
path derives the chain in its own way:

- The engine service (`kapi engine serve`) refuses a definition in which a node
  has more than one incoming or outgoing edge, then runs the topological order
  as one chain.
- The built-in flow path checks data flow and transformer placement, then
  orders the tool nodes by their canvas X position (`orderedToolNodes`).
- The project flow runner builds one tool per step, in step order, and refuses
  a flow holding a `parallel:` step before it builds any.
- A Go caller that fills `Flow.Tools` supplies the order itself.

No path schedules branches concurrently or joins their results, so a flow meant
to run is a linear chain.

The built-in flow catalog is a product concern and lives in the host module
(`host/flowdef.BuiltInFlows`), not in the engine. It covers translation with
guardrails, checks, memory reuse, pseudo-translation, the media-to-subtitle
paths, and the redaction-bracketed flows; the authoritative list with each
flow's steps is `kapi flows` and the generated
[command reference](/reference/commands/flows).

`kapi flows` lists only the *composed* (multi-tool) built-in flows, because
single-tool definitions are surfaced as top-level tool commands rather than as
flows. `host/flowdef.FlowStore` persists user-created flow definitions as JSON
files on disk, distinguished by source:

- `built-in`: ships with neokapi, immutable
- `user`: created by the user, stored in the user's config directory

A project's own named flows are declared in its recipe's `flows:` block rather
than stored as files ([E-04](e-04-flows-and-io-binding.md)).

### Steps-based YAML format

A human-friendly steps format is the primary authoring surface for flows in YAML
([E-03](e-03-tool-system.md)):

```yaml
apiVersion: v1
kind: FlowDefinition
metadata:
  name: Production Pipeline
spec:
  steps:
    - tool: recycle
      config: { fuzzyThreshold: 75 }
    - tool: translate
      config: { provider: anthropic }
    - tool: qa
```

Steps run in the order listed. The parser auto-detects the shape (steps vs
graph), and `StepsToGraph` compiles steps to a chain of nodes and edges. It also
accepts a `parallel:` block and compiles it to several nodes fed from the same
predecessor; that shape exists in the definition model only, and none of the
host paths above executes it as concurrent branches.

The steps carry only the composition. A flow's source and sink are bindings
resolved at invocation (file, the project store, a `.kpz`, interchange, or none;
[E-04](e-04-flows-and-io-binding.md)) rather than fields of the flow document.

### Fan-out and batching

`tool.Tee()` is a channel helper for a Go caller: it forwards the same Part
pointer to every output channel, takes backpressure from each receiver, and
closes every output once its input closes or its context is cancelled. The
executor never wires it from graph edges. A caller that uses it owns the
branches' lifecycle, and concurrent receivers share each mutable Block, so they
must coordinate access to it.

The `batch` tool is a stage in a linear chain. It holds blocks until it has the
configured number, then forwards them one by one; a non-block Part passes
through at once, ahead of any blocks it holds. Grouping blocks into one request
for batch-capable remote APIs and LLM prompts happens inside the tools that call
them ([M-05](../multilingual/m-05-prompts-and-batching.md)).

### Script step

The `script` tool runs user-provided JavaScript (ES5) via the goja runtime. Each
tool instance owns its own `goja.Runtime`, which is safe because `ToolFactory`
gives one instance per goroutine. The JS API exposes `part`, `emit()`, `skip()`,
and `log()` for filtering and transforming parts: lightweight custom
transformations without Go code. `script` is in the **exec class**: a recipe
cannot arm it silently ([E-06](e-06-execution-trust.md)).

### Terminology: Okapi → neokapi

For readers familiar with the Okapi Framework, the engine maps to Okapi concepts
as follows:

| Okapi (Java)                    | neokapi (Go)               |
| ------------------------------- | -------------------------- |
| Filter                          | DataFormat (Reader/Writer) |
| Step                            | Tool                       |
| Pipeline                        | Flow                       |
| PipelineDriver                  | Executor                   |
| Event                           | Part                       |
| TextUnit                        | Block                      |
| TextFragment                    | Run sequence (`[]Run`)     |
| Code                            | Run                        |
| StartSubDocument/StartSubFilter | Child Layer                |

## Consequences

- Each tool runs concurrently; multi-core CPUs are utilized within a single
  document's pipeline.
- Multiple documents process in parallel, bounded by `MaxConcurrency`.
- Backpressure is automatic: a slow tool causes its input channel to fill, which
  blocks the upstream tool without manual coordination.
- Context cancellation cleanly propagates through the entire chain.
- ToolFactories ensure no shared mutable state between parallel documents.
- Collectors provide cross-document aggregation without breaking the streaming
  model.
- Tool authors do not manage goroutines; the executor handles lifecycle, and
  `ParallelBlockTool` supplies intra-tool parallelism without any concurrency
  code in the tool. A slow block holds back at most N admitted blocks and 4N
  other Parts, so a parallel stage stays bounded when its workers finish out of
  order.
- `StreamingCollector` enables real-time observation of pipeline output without
  modifying the Part stream or adding buffering stages.
- Flow tracing enables post-hoc debugging and visualization, helping users
  understand tool behaviour and identify bottlenecks.
- A host that orders a definition with `TopologicalOrder` reports a cycle before
  it builds any tool.
- JSON and YAML serialization supports import/export and version control of flow
  configurations.
- Steps-based YAML makes flow authoring accessible without Go; the visual editor
  and the YAML share one graph representation, and what runs is the ordered
  chain a host derives from it.

## Related

- [F-02: The content model](../foundations/f-02-content-model.md): the Part types that stream
- [E-02: The format system](e-02-format-system.md): readers that emit Parts, writers that consume them
- [E-03: The tool system](e-03-tool-system.md): the tools that make up a flow
- [E-04: Flows and I/O binding](e-04-flows-and-io-binding.md): reader/writer become source/sink bindings; a flow is composition only
- [E-05: The plugin system](e-05-plugin-system.md): plugin tools use the same executor contract
