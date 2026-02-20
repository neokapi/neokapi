---
id: 004-processing-engine
sidebar_position: 4
title: "AD-004: Processing Engine"
---
# AD-004: Channel-based processing engine

## Context

Okapi uses a synchronous pull-based iterator pattern (`IFilter.hasNext()` /
`next()`) where each pipeline step pulls the next event from the previous step.
This is simple but inherently single-threaded: only one step runs at a time,
and backpressure is implicit in the call stack. Processing multiple documents
requires external orchestration.

Go's goroutines and channels offer a natural alternative: each processing
stage runs concurrently in its own goroutine, connected by typed channels.
We needed both intra-document concurrency (tools running in parallel within
one document) and inter-document concurrency (multiple documents processed
simultaneously).

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

### Collectors

Collectors aggregate results across documents (word counts, QA reports,
terminology lists). They implement a `Collect(ctx, item, parts)` method
called after each document completes and a `Result()` method for the final
aggregate. Collectors must be thread-safe since multiple documents may
complete concurrently.

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
- `source: "project"` -- stored within a project's `.kaz` archive or
  project directory

Flow definitions enable visual editing in Bowrain
([AD-012](./012-bowrain.md)) through a drag-and-drop node
graph, and JSON serialization supports import/export and version control
of flow configurations.

## Alternatives Considered

- **Synchronous iterator** (Okapi style): simpler but no concurrency within
  a single document's pipeline; poor utilization on multi-core machines.
- **Actor model**: more complex; channels achieve the same fan-in/fan-out
  with less abstraction overhead.
- **Store-less pipeline**: simpler but loses incremental processing and
  version tracking. Every run must re-extract and re-process all content,
  even when only a few blocks have changed.
- **DAG-based execution** (Airflow-style): over-engineered for typical
  3-5 tool chains. The linear pipeline with optional collectors covers
  all current use cases.

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
