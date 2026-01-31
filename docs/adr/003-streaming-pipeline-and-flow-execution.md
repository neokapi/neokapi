# ADR-003: Streaming pipeline and flow execution

**Status:** Accepted

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
