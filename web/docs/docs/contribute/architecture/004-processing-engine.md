---
id: 004-processing-engine
sidebar_position: 4
title: "AD-004: Processing Engine"
description: "Architecture decision: the processing engine is a channel-based streaming pipeline where each tool runs in its own goroutine, connected by buffered channels, with errgroup for error propagation and context for cancellation."
keywords: [processing engine, pipeline, goroutines, channels, errgroup, streaming, architecture decision]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# AD-004: Processing Engine

## Summary

Documents are processed by a channel-based streaming pipeline. Each tool
runs in its own goroutine; tools are connected by buffered channels
(default 64) that provide automatic backpressure. An `errgroup.Group`
coordinates errors and propagates context cancellation. Four independent
concurrency layers — intra-tool block parallelism, batch file concurrency,
document-level concurrency, and streaming observation — compose without
interference. Flows are declared as either a graph of nodes and edges or a
sequential list of steps (with explicit `parallel:` blocks for fan-out),
both compiled to the same executable representation.

## Context

Go's goroutines and channels make it natural to structure a pipeline as
concurrent stages connected by typed channels. A localization pipeline has
a mixture of CPU-bound (format parsing, QA checks) and IO-bound (AI
translation, MT calls, TM lookups) stages. The same pipeline must also be
driven at multiple scales: a single file on a laptop; hundreds of files in
a batch; a long-lived project with many documents processed in parallel.

Additionally, the pipeline must support both declarative authoring (visual
flow editor, human-readable YAML) and programmatic construction (Go
library) from the same data model.

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

Content enters through a **source** binding and leaves through a **sink** binding
([AD-026](026-flow-io-binding.md)). For the default `file` binding these are a
DataFormat reader and writer ([AD-005](005-format-system.md)); a project store, a
`.klz`, or an interchange file bind the same stream with no reader or writer.
Between the ends, each tool runs in its own goroutine. Buffered channels (default
size 64) provide backpressure. `errgroup.Group` coordinates error handling across
goroutines. Context cancellation propagates to all stages.

Parts carry typed resources ([AD-002: Content Model](002-content-model.md)):
Blocks contain translatable content, Data carries structural markup, Layers
group nested content, Media holds binary assets. Tools declare which
resource types they handle; the rest pass through unchanged.

### Executor

`Executor` orchestrates tool chains using the goroutine-per-tool model:

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
    flow.WithCollectors(wordCounter, qaReport),
)
```

### Parallel block processing

For IO-bound tools (AI translation, MT calls), sequential per-part
processing underutilizes throughput. `ParallelBlockTool` wraps any tool to
fan out Block processing across N goroutines while preserving strict Part
ordering:

<PipelineDiagram
  stages={[
    { label: "Input" },
    { label: "Dispatcher", sub: "seq numbers", role: "annotate" },
    {
      role: "translate",
      parallelLabel: "fan-out · N goroutines (semaphore-bounded)",
      lanes: [{ label: "Worker 1" }, { label: "Worker 2" }, { label: "Worker N" }],
    },
    { label: "Reassembly", sub: "min-heap · in order", role: "annotate" },
    { label: "Output" },
  ]}
/>

The dispatcher assigns monotonic sequence numbers to all incoming Parts.
Block Parts are dispatched to a semaphore-bounded worker pool; non-Block
Parts (Data, Media, Layer) pass through the inner tool sequentially. A
min-heap reassembly buffer collects results and emits them in strict
sequence order, so downstream tools see the same Part ordering regardless
of which worker finished first.

The CLI applies auto-parallelism to IO-bound flows:

| Flow                              | Default Parallel Blocks |
| --------------------------------- | ----------------------- |
| `ai-translate`, `ai-translate-qa` | 5                       |
| All other flows                   | 1 (sequential)          |

Users override with `--parallel-blocks N` or disable with
`--parallel-blocks 1`.

### Batch executor

`BatchExecutor` processes multiple pre-read files through a tool chain
with configurable file-level concurrency:

```go
type BatchConfig struct {
    FileConcurrency int         // max files processed in parallel (default: 1)
    ChannelSize     int         // per-pipeline channel buffer size (default: 64)
    SharedResources []io.Closer // resources shared across files (closed at end)
    FailFast        bool        // cancel remaining on first error (default: true)
}
```

Each file gets fresh tool instances from ToolFactory functions, preventing
state leakage between concurrent documents. Results are returned in input
file order regardless of completion order. Collectors are called with
mutex protection for thread-safe aggregation across files.

### Concurrency layering

Four independent concurrency layers compose without interference:

| Layer             | Scope                  | Control                     | Order                    |
| ----------------- | ---------------------- | --------------------------- | ------------------------ |
| ParallelBlockTool | Blocks within one tool | N goroutines per tool       | Strict Part order        |
| BatchExecutor     | Multiple files         | FileConcurrency semaphore   | File order preserved     |
| Executor          | Multiple documents     | MaxConcurrency semaphore    | Document order preserved |
| TappingTool       | Observation            | Inline (no extra goroutine) | Sequential               |

### Collectors and streaming collectors

Collectors aggregate results across documents (word counts, QA reports,
terminology lists). They implement `Collect(ctx, item, parts)` — called
after each document completes — and `Result()` for the final aggregate.
Collectors must be thread-safe since multiple documents may complete
concurrently.

`StreamingCollector` extends `Collector` with `Observe(part)` for inline
observation without adding a pipeline stage. `TappingTool` wraps a tool and
its streaming collector: output Parts are intercepted and passed to
`Observe()` synchronously before forwarding downstream. This enables
real-time metrics (e.g., streaming word counts) without buffering the
entire result set.

### Flow tracing and visualization

`TraceRecorder` captures timestamped events during flow execution.
`TracingTool` wraps each tool in the chain and records enter/exit events
with Part snapshots. The `--trace path/to/trace.json` CLI flag enables
tracing. The output is a `FlowTrace` JSON file containing:

- **Nodes** — the tool chain with concurrency metadata
- **Events** — timestamped enter/exit events per Part
- **Part snapshots** — Part state before and after each node
- **Duration** — total flow execution time in microseconds

A browser-based visualization renders the trace as an animated flow
diagram with particles moving through nodes, channel fill indicators, and
worker lane separation for parallel tools. The playback engine supports
variable-speed replay and seeking.

### Flow definitions

`FlowDefinition` is a JSON/YAML-serializable struct that captures a
visual flow graph (nodes + edges) and tool configurations needed to
reconstruct a runnable Flow. This separates the declarative description
of a flow from its runtime execution.

Each `FlowNode` has:

- **ID** — unique identifier within the definition
- **Type** — `tool` (a processing step). A flow's I/O ends are not nodes; they
  are `source` / `sink` **bindings** ([AD-026](026-flow-io-binding.md))
- **Name** — the registered name of the tool (e.g., `"pseudo-translate"`)
- **Label** — optional display label for UI rendering
- **Config** — optional key-value configuration map
- **Position** — x/y coordinates for visual layout in the flow editor

> **Bindings ([AD-026](026-flow-io-binding.md)).** A flow's source and sink are
> bindings resolved from invocation context — file, the project store, a `.klz`,
> interchange import/export, or none — so the same flow runs over any origin. The
> graph is composition; a single tool is invoked directly, not wrapped in a
> one-tool flow.

Each `FlowEdge` connects a source node to a target node.
`TopologicalOrder()` computes the execution order using Kahn's algorithm,
returning an error if a cycle is detected so invalid flow graphs never
reach the runtime executor.

Built-in flow definitions include:

| Name               | Description                                              |
| ------------------ | ------------------------------------------------------- |
| `ai-translate`     | AI-powered translation using configured provider        |
| `ai-translate-qa`  | AI translation followed by QA validation                |
| `pseudo-translate` | Pseudo-translation for internationalization testing     |
| `qa-check`         | Quality assurance checks on existing translations       |
| `tm-leverage`      | Translation memory leveraging from Sievepen TM          |
| `secure-translate` | Redact sensitive content, AI-translate, then restore the originals locally ([AD-020](020-redaction.md)) |

`kapi flows` lists only the *composed* (multi-tool) built-in flows —
`ai-translate-qa` and `secure-translate` — because single-tool definitions
(`ai-translate`, `pseudo-translate`, `qa-check`, `tm-leverage`) are surfaced as
top-level tool commands rather than as flows.

`FlowStore` persists user-created flow definitions as JSON files on disk.
Flow definitions are distinguished by source:

- `source: "built-in"` — ships with neokapi, immutable
- `source: "user"` — created by the user, stored in the user's config directory
- `source: "project"` — stored within a project directory

### Steps-based YAML format

A human-friendly steps format is the primary authoring surface for flows in
YAML ([AD-006: Tool System](006-tool-system.md)):

```yaml
apiVersion: v1
kind: FlowDefinition
metadata:
  name: Production Pipeline
spec:
  steps:
    - tool: tm-leverage
      config: { fuzzyThreshold: 75 }
    - tool: ai-translate
      config: { provider: anthropic }
    - tool: qa-check
```

Steps are sequential by default. `parallel:` blocks provide fan-out. The
parser auto-detects the format (steps vs graph) and compiles steps to
nodes+edges via `StepsToGraph()`. Both formats produce the same runnable
executor.

The steps carry only the composition. A flow's source and sink are bindings
resolved at invocation — file, the project store, a `.klz`, interchange, or none
([AD-026: Flow I/O Binding](026-flow-io-binding.md)) — not fields of the flow
document.

### Fan-out and batching

`tool.Tee()` copies parts to N output channels, enabling fan-out topologies
where one node feeds multiple parallel branches. A `batch` tool collects
blocks into configurable batches before forwarding, useful for batch MT
APIs and LLM prompts that benefit from multiple inputs per request.

### Script step

A `script` tool runs user-provided JavaScript (ES5) via the goja runtime.
Each tool instance owns its own `goja.Runtime` (safe: one goroutine per
tool instance via `ToolFactory`). The JS API exposes `part`, `emit()`,
`skip()`, and `log()` for filtering and transforming parts — lightweight
custom transformations without Go code.

### Terminology: Okapi → neokapi

For readers familiar with the Okapi Framework, neokapi's engine maps to
Okapi concepts as follows:

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
- Backpressure is automatic: a slow tool causes its input channel to fill,
  which blocks the upstream tool without manual coordination.
- Context cancellation cleanly propagates through the entire chain.
- ToolFactories ensure no shared mutable state between parallel documents.
- Collectors provide cross-document aggregation without breaking the
  streaming model.
- Tool authors do not manage goroutines; the executor handles lifecycle.
- ParallelBlockTool provides intra-tool parallelism for IO-bound tools
  without requiring tool authors to manage concurrency.
- StreamingCollector enables real-time observation of pipeline output
  without modifying the Part stream or adding buffering stages.
- Flow tracing enables post-hoc debugging and visualization, helping users
  understand tool behavior and identify bottlenecks.
- `TopologicalOrder` validation catches cycles before runtime, giving fast
  feedback during flow authoring.
- JSON and YAML serialization supports import/export and version control
  of flow configurations.
- Steps-based YAML makes flow authoring accessible to non-developers; the
  visual editor and YAML stay in sync because both compile to the same
  graph.

## Related

- [AD-002: Content Model](002-content-model.md) — the Part types that stream
- [AD-005: Format System](005-format-system.md) — readers that emit Parts, writers that consume them
- [AD-006: Tool System](006-tool-system.md) — the tools that make up a Flow
- [AD-007: Plugin System and Okapi Bridge](007-plugin-system.md) — plugin tools use the same executor contract
- [AD-026: Flow I/O Binding](026-flow-io-binding.md) — reader/writer become source/sink bindings; a flow is composition only
