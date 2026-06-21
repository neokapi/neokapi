---
sidebar_position: 9
title: Pipeline
description: The neokapi pipeline is the concurrent execution engine that runs flows — each tool stage runs in its own goroutine, connected by buffered channels of Parts, with context cancellation and error propagation via errgroup.
keywords: [pipeline, goroutines, channels, concurrent, streaming, errgroup, execution engine]
---

import { FlowLab } from "@site/src/components/Lab/FlowBuilderRunner";
import { PipelineDiagram } from "@neokapi/docs-shared";

# Pipeline

The **pipeline** is how neokapi runs a [flow](/framework/flows). Where a flow
says _what_ tools to run in _what_ order, the pipeline is the concurrent
machinery that actually runs them: a [format reader](/framework/formats), a chain
of [tools](/framework/tools), and a format writer, each running in its own
goroutine and connected by buffered channels of [Parts](/framework/content-model).

<PipelineDiagram
  animated
  stages={[
    { label: "RawDocument" },
    { label: "Reader", sub: "DataFormat", role: "io" },
    { label: "Tool 1", note: "goroutine" },
    { label: "Tool 2", note: "goroutine" },
    { label: "⋯" },
    { label: "Writer", sub: "DataFormat", role: "io" },
    { label: "Output" },
  ]}
/>

This is the neokapi analogue of Okapi's _PipelineDriver_. It is built on Go's
native concurrency: goroutines for the stages, channels for the connections, and
`errgroup` for coordination.

The reader and writer shown here are the **file binding** — the default way
content enters and leaves the pipeline. The same tool stream can instead be bound
to a project store, a `.klz` workspace, or an interchange file, with no reader or
writer ([flows: source and sink](/framework/flows#source-and-sink-the-flows-ends)).

:::tip Watch it run, step by step
Run a file through a pipeline and drive it with **Next** — each step advances the
stream by one event, so you can watch Parts move out of the reader, through the
tools, and into the writer, inspecting how each Part changes at every stage. This
runs the real `kapi` engine in your browser via WebAssembly.
:::

<FlowLab withRecordedTraces defaultScenarioId="pseudo" defaultSampleId="messages-json" />

## Streaming, not batching

A pipeline does not load a document into memory, transform it, and write it out
in three phases. Instead, the reader emits Parts as it parses, and those Parts
flow downstream while the reader is still working. Each tool processes a Part as
soon as it arrives and forwards it, so the writer can begin emitting output
before the reader has finished reading. Memory use stays bounded by the size of
the channel buffers and the Parts in flight, not by the size of the document.

This streaming model is why the [content model](/framework/content-model) is
shaped the way it is: a `Part` is the indivisible unit that flows through, and a
document is a stream of Parts (layer starts, blocks, data, layer ends) rather
than a single tree.

## Channels and backpressure

Adjacent stages are connected by **buffered channels** — by default a buffer of
64 Parts. The buffer decouples the stages so a fast reader does not have to wait
on a slow tool for every single Part, but it is bounded: when the buffer fills,
the upstream stage blocks on its send until the downstream stage catches up.
That blocking _is_ the backpressure. A slow tool — an AI translation step
waiting on a network call, say — naturally throttles the reader feeding it,
without any explicit rate limiting or queue management.

Each tool runs `Process(ctx, in, out)` in its own goroutine. The executor wires
stage `i`'s output channel to stage `i+1`'s input channel, launches a goroutine
per tool, and closes each output channel when its tool returns. Channel close is
the end-of-stream signal: a tool's `Process` loop exits when its input channel is
closed and drained, then closes its own output, which signals the next tool, and
so on down to the writer.

## Error handling and cancellation

The stages are coordinated by an `errgroup.Group`. If any tool's `Process`
returns an error, the group cancels a shared context derived from the caller's
context. Every stage selects on `ctx.Done()` in its channel operations, so
cancellation propagates promptly to all goroutines — a stage blocked on a send or
a receive wakes up and returns. The pipeline tears down cleanly rather than
leaking goroutines on a partial failure, and the first error is reported to the
caller.

Because cancellation flows from the caller's context, a pipeline is also
cancellable from the outside — closing a CLI run, a request timeout, or a desktop
"stop" button all cancel the same context and unwind every stage.

## Layers of concurrency

The single tool chain is only one of several independent concurrency layers, and
they compose without interfering:

| Layer                      | What runs concurrently                                          |
| -------------------------- | --------------------------------------------------------------- |
| **Stage (tool) pipeline**  | Each tool in the chain runs in its own goroutine, the default.  |
| **Intra-tool blocks**      | A block-handling tool can fan its work across N goroutines while preserving Part order (see [Tools](/framework/tools)). |
| **Batch documents**        | The executor processes many input files in parallel, bounded by a concurrency limit. |

Document-level batching is controlled on the executor. `MaxConcurrency` bounds
how many documents run at once — `1` is sequential, `0` means use the number of
CPUs — and a semaphore enforces the bound. With fail-fast enabled (the default),
the first document error cancels the remaining work; with it disabled, the
executor runs every document and reports errors together. Each document gets its
own tool chain (via the flow's [tool factories](/framework/flows)) so concurrent
documents never share tool state.

## Configuring the executor

The executor is created with functional options:

```go
executor := flow.NewExecutor(
    flow.WithMaxConcurrency(4), // documents in parallel; 0 = NumCPU, 1 = sequential
    flow.WithChannelSize(64),   // inter-tool channel buffer
    flow.WithFailFast(true),    // cancel remaining documents on first error
)
err := executor.Execute(ctx, f, items)
```

With no options it runs sequentially, with a channel buffer of 64 and fail-fast
on. `Execute` takes a built flow and a slice of items (each an input document, an
output path, and a target locale) and runs the whole batch.

For callers that want to feed Parts in and read results out directly — rather
than reading and writing files — the executor can also expose the chain's input
and output channels, wiring the same goroutine-per-tool pipeline but leaving the
ends open for the caller to drive.

## Observation

Because the pipeline is a stream of Parts, work can be observed without
disturbing it. The executor accepts **collectors** that are fed the output Parts
of each document as it completes, which is how cross-document analysis — scoping
reports, repetition analysis across a batch — accumulates results. Observation is
a separate concurrency concern from the tool chain itself: a collector reads the
finished stream and does not sit inside it.

## Related reading

- [Flows](/framework/flows) — the graph the pipeline executes.
- [Tools](/framework/tools) — the stages that run in the pipeline.
- [Content Model](/framework/content-model) — the Part that streams through it.
- [AD-004: Processing Engine](/contribute/architecture/004-processing-engine) — the design rationale.
