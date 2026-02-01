---
id: 003-streaming-pipeline-and-flow-execution
sidebar_position: 3
title: "ADR-003: Streaming Pipeline"
---

# ADR-003: Streaming pipeline and flow execution

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

## Decision

### Channel-Based Pipeline

Documents flow through a channel-based concurrent pipeline:

```
RawDocument -> DataFormatReader -> [Tool 1] -> [Tool 2] -> ... -> DataFormatWriter -> Output
                                      |            |
                                 chan *Part    chan *Part
```

Each tool runs in its own goroutine. Buffered channels (default size 64)
provide backpressure. `errgroup.Group` coordinates error handling across
goroutines. Context cancellation propagates to all stages.

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

### Collectors

Collectors aggregate results across documents (word counts, QA reports,
terminology lists). They implement a `Collect(ctx, item, parts)` method
called after each document completes and a `Result()` method for the final
aggregate. Collectors must be thread-safe.

### Flow Definitions

`FlowDefinition` is a JSON-serializable struct that captures a visual flow graph
(nodes + edges) and tool configurations needed to reconstruct a runnable Flow.
This separates the declarative description of a flow from its runtime execution.

Each `FlowNode` has:

- **ID** — unique identifier within the definition
- **Type** — one of `"tool"`, `"reader"`, or `"writer"`
- **Name** — the registered name of the tool/format (e.g., `"pseudo-translate"`, `"html"`)
- **Label** — optional display label for UI rendering
- **Config** — optional key-value configuration map
- **Position** — x/y coordinates for visual layout in the flow editor

Each `FlowEdge` has:

- **ID** — unique identifier
- **Source** — node ID of the upstream node
- **Target** — node ID of the downstream node

`TopologicalOrder()` computes the execution order of nodes using Kahn's
algorithm. It returns an error if a cycle is detected, preventing invalid
flow graphs from reaching the runtime executor.

`ToolNodeNames()` returns tool names in topological order, providing the
ordered list needed to build the runtime tool chain for `FlowExecutor`.

Five built-in flow definitions are provided:

| Name                  | Description                                      |
|-----------------------|--------------------------------------------------|
| `ai-translate`        | AI-powered translation using configured provider |
| `ai-translate-qa`     | AI translation followed by QA validation         |
| `pseudo-translate`    | Pseudo-translation for internationalization testing |
| `qa-check`            | Quality assurance checks on existing translations |
| `tm-leverage`         | Translation memory leveraging from Pensieve TM   |

`FlowStore` persists user-created flow definitions as JSON files on disk.
Flow definitions are distinguished by source:

- `source: "built-in"` — shipped with gokapi, immutable
- `source: "user"` — created by the user, stored in the user's config directory
- `source: "project"` — stored within a project's `.kaz` archive or project directory

## Alternatives Considered

- **Synchronous iterator** (Okapi style): simpler but no concurrency within
  a single document's pipeline; poor utilization on multi-core machines.
- **Actor model**: more complex; channels achieve the same fan-in/fan-out
  with less abstraction overhead.
- **Separate error channel**: complicates select statements and ordering
  guarantees; `PartResult` tuple is simpler.
- **Thread pool with work stealing**: over-engineered for typical 3-5 tool
  chains.
- **Single goroutine with tool loop**: loses inter-tool concurrency.

## Consequences

- Each tool runs concurrently; multi-core CPUs are utilized within a single
  document's processing pipeline
- Multiple documents can be processed in parallel, bounded by MaxConcurrency
- Backpressure is automatic: a slow tool causes its input channel to fill,
  which blocks the upstream tool
- Context cancellation cleanly propagates through the entire chain
- Channel buffer size (default 64) is a tuning knob for memory vs. latency
- Tool authors do not manage goroutines; the executor handles lifecycle
- ToolFactories ensure no shared mutable state between parallel documents
- Collectors provide cross-document aggregation without breaking the
  streaming model
- Flow definitions enable visual flow editing in Bowrain (drag-and-drop
  node graph)
- TopologicalOrder validation catches cycles before runtime, providing
  fast feedback during flow authoring
- JSON serialization supports import/export and version control of flow
  configurations
