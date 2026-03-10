---
id: 004-processing-engine
sidebar_position: 4
title: "AD-004: Processing Engine"
---
# AD-004: Channel-based processing engine

## Context

gokapi's processing engine uses Go's goroutines and channels to run each
pipeline stage concurrently in its own goroutine, connected by typed channels.
The engine provides both intra-document concurrency (tools running in parallel
within one document) and inter-document concurrency (multiple documents
processed simultaneously).

The processing engine now operates on content from the Content Store
([AD-003](./003-content-store.md)), not just raw files. Flows read versioned
blocks, apply transformations, and persist results back to the store. This
decouples extraction from processing from delivery and enables incremental
workflows where only changed blocks are re-processed.

## Decision

### Channel-Based Pipeline

Content flows through a channel-based concurrent pipeline:

```
Store -> DataFormatReader -> [Tool 1] -> [Tool 2] -> ... -> DataFormatWriter -> Store
                                 |            |
                            chan *Part    chan *Part
```

Each tool runs in its own goroutine. Buffered channels (default size 64)
provide backpressure. `errgroup.Group` coordinates error handling across
goroutines. Context cancellation propagates to all stages.

Parts carry typed resources through the pipeline (see
[AD-002](./002-content-model.md)): Blocks contain translatable content,
Data carries structural markup, Layers group nested content, and Media
holds binary assets. Tools declare which resource types they handle; the
rest pass through unchanged.

### FlowExecutor

`FlowExecutor` orchestrates tool chains using the goroutine-per-tool model:

- Each tool in the chain runs in its own goroutine
- Buffered channels (configurable size) connect adjacent tools
- `errgroup` collects the first error and cancels the shared context
- Parallel document processing via a semaphore-bounded worker pool
- `ToolFactories` create fresh tool instances per document to avoid shared
  state between concurrent documents

Configuration uses the functional options pattern:

```go
executor := flow.NewFlowExecutor(
    flow.WithMaxConcurrency(8),
    flow.WithChannelSize(128),
    flow.WithCollectors(wordCounter, qaReport),
)
```

### Store Integration

Flows can read from and write to the Content Store
([AD-003](./003-content-store.md)):

- **Extract flow**: Connector -> Format Reader -> Store (blocks persisted
  with content hashes)
- **Process flow**: Store -> Tool chain -> Store (blocks enriched,
  translations added)
- **Merge flow**: Store -> Format Writer -> Connector (translated content
  pushed back)

This decouples extraction from processing from delivery. A connector
extracts content once, and multiple flows can process the same stored
content. Content-addressable hashing (see [AD-002](./002-content-model.md))
means unchanged blocks are skipped on re-extraction, and incremental
processing flows only touch blocks whose source content has changed.

### Parallel Block Processing

The default pipeline processes Parts sequentially within each tool. For
IO-bound tools (AI translation, MT calls), this underutilizes available
throughput. `ParallelBlockTool` wraps any tool to fan out Block processing
across N goroutines while preserving strict Part ordering.

```
Input Channel → Dispatcher → [Worker 1] → Reassembly (min-heap) → Output Channel
                            → [Worker 2] →
                            → [Worker N] →
```

The dispatcher assigns monotonic sequence numbers to all incoming Parts.
Block Parts are dispatched to a semaphore-bounded worker pool; non-Block
Parts (Data, Media, Layer) pass through the inner tool sequentially.
A min-heap reassembly buffer collects results and emits them in strict
sequence order, so downstream tools see the same Part ordering regardless
of which worker finished first.

Auto-parallel is applied by the CLI for IO-bound flows:

| Flow | Default Parallel Blocks |
|------|------------------------|
| `ai-translate`, `ai-translate-qa` | 5 |
| All other flows | 1 (sequential) |

Users override with `--parallel-blocks N` or disable with
`--parallel-blocks 1`.

### Batch Executor

`BatchExecutor` processes multiple pre-read files through a tool chain with
configurable file-level concurrency:

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
file order regardless of completion order. Collectors are called with mutex
protection for thread-safe aggregation across files.

### Concurrency Layering

Four independent concurrency layers compose without interference:

| Layer | Scope | Control | Order |
|-------|-------|---------|-------|
| ParallelBlockTool | Blocks within one tool | N goroutines per tool | Strict Part order |
| BatchExecutor | Multiple files | FileConcurrency semaphore | File order preserved |
| FlowExecutor | Multiple documents | MaxConcurrency semaphore | Document order preserved |
| TappingTool | Observation | Inline (no extra goroutine) | Sequential |

### Collectors and Streaming Collectors

Collectors aggregate results across documents (word counts, QA reports,
terminology lists). They implement a `Collect(ctx, item, parts)` method
called after each document completes and a `Result()` method for the final
aggregate. Collectors must be thread-safe since multiple documents may
complete concurrently.

`StreamingCollector` extends `Collector` with an `Observe(part)` method
for inline observation without additional pipeline stages. `TappingTool`
wraps a tool and its streaming collector: output Parts are intercepted
and passed to `Observe()` synchronously before forwarding downstream.
This enables real-time metrics (e.g., streaming word counts) without
buffering the entire result set.

### Flow Tracing and Visualization

`TraceRecorder` captures timestamped events during flow execution for
debugging and visualization. `TracingTool` wraps each tool in the chain
and records enter/exit events with Part snapshots.

The `--trace path/to/trace.json` CLI flag enables tracing. The output is a
`FlowTrace` JSON file containing:

- **Nodes** -- the tool chain with concurrency metadata
- **Events** -- timestamped enter/exit/pool-acquire events per Part
- **Part snapshots** -- Part state before and after each node
- **Duration** -- total flow execution time in microseconds

A browser-based visualization renders the trace as an animated flow diagram
with particles moving through nodes, channel fill indicators, and worker
lane separation for parallel tools. The playback engine supports
variable-speed replay and seeking.

### Flow Definitions

`FlowDefinition` is a JSON-serializable struct that captures a visual flow
graph (nodes + edges) and tool configurations needed to reconstruct a
runnable Flow. This separates the declarative description of a flow from
its runtime execution.

Each `FlowNode` has:

- **ID** -- unique identifier within the definition
- **Type** -- one of `"tool"`, `"reader"`, or `"writer"`
- **Name** -- the registered name of the tool/format (e.g.,
  `"pseudo-translate"`, `"html"`)
- **Label** -- optional display label for UI rendering
- **Config** -- optional key-value configuration map
- **Position** -- x/y coordinates for visual layout in the flow editor

Each `FlowEdge` connects a **Source** node to a **Target** node.
`TopologicalOrder()` computes the execution order using Kahn's algorithm,
returning an error if a cycle is detected to prevent invalid flow graphs
from reaching the runtime executor.

Five built-in flow definitions are provided:

| Name                  | Description                                         |
|-----------------------|-----------------------------------------------------|
| `ai-translate`        | AI-powered translation using configured provider    |
| `ai-translate-qa`     | AI translation followed by QA validation            |
| `pseudo-translate`    | Pseudo-translation for internationalization testing |
| `qa-check`            | Quality assurance checks on existing translations   |
| `tm-leverage`         | Translation memory leveraging from Sievepen TM      |

`FlowStore` persists user-created flow definitions as JSON files on disk.
Flow definitions are distinguished by source:

- `source: "built-in"` -- shipped with gokapi, immutable
- `source: "user"` -- created by the user, stored in the user's config
  directory
- `source: "project"` -- stored within a project directory

Flow definitions enable visual editing in Bowrain
([AD-012](./012-bowrain.md)) through a drag-and-drop node
graph, and JSON serialization supports import/export and version control
of flow configurations.

## Alternatives Considered

- **Synchronous iterator**: simpler but no concurrency within a single
  document's pipeline; poor utilization on multi-core machines.
- **Actor model**: more complex; channels achieve the same fan-in/fan-out
  with less abstraction overhead.
- **Store-less pipeline**: simpler but loses incremental processing and
  version tracking. Every run must re-extract and re-process all content,
  even when only a few blocks have changed.
- **DAG-based execution** (Airflow-style): over-engineered for typical
  3-5 tool chains. The linear pipeline with optional collectors covers
  all current use cases.
- **Per-Part goroutines**: Spawning a goroutine per Part is wasteful for
  small, fast operations. The semaphore-bounded worker pool in
  ParallelBlockTool provides the same throughput with bounded resource use.
- **Separate tracing service**: Adding an external tracing backend (Jaeger,
  Zipkin) is too heavy for a CLI tool. In-process recording with JSON
  export and a static browser viewer is simpler and requires no
  infrastructure.

## Consequences

- Each tool runs concurrently; multi-core CPUs are utilized within a single
  document's processing pipeline
- Multiple documents can be processed in parallel, bounded by MaxConcurrency
- Backpressure is automatic: a slow tool causes its input channel to fill,
  which blocks the upstream tool
- Context cancellation cleanly propagates through the entire chain
- Store integration enables incremental processing -- only changed blocks
  are re-processed across extraction cycles
- ToolFactories ensure no shared mutable state between parallel documents
- Collectors provide cross-document aggregation without breaking the
  streaming model
- Tool authors do not manage goroutines; the executor handles lifecycle
- Flow definitions enable visual flow editing in Bowrain
  ([AD-012](./012-bowrain.md))
- TopologicalOrder validation catches cycles before runtime, providing fast
  feedback during flow authoring
- JSON serialization supports import/export and version control of flow
  configurations
- Connectors ([AD-005](./005-connector-system.md)) integrate naturally: extract
  once to the store, process with multiple flows, merge back when ready
- ParallelBlockTool provides intra-tool parallelism for IO-bound tools
  without requiring tool authors to manage concurrency
- BatchExecutor provides file-level parallelism with isolated tool
  instances, preventing state leakage between concurrent documents
- StreamingCollector enables real-time observation of pipeline output
  without modifying the Part stream or adding buffering stages
- Flow tracing enables post-hoc debugging and visualization of pipeline
  execution, helping users understand tool behavior and identify bottlenecks
- The four concurrency layers (parallel blocks, batch files, flow documents,
  streaming observation) compose independently -- each can be enabled or
  disabled without affecting the others
