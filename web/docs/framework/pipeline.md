---
sidebar_position: 9
title: Pipeline
description: The neokapi pipeline is the concurrent execution engine that runs flows: each tool stage runs in its own goroutine, connected by buffered channels of Parts, with context cancellation and error propagation via errgroup.
keywords: [pipeline, goroutines, channels, concurrent, streaming, errgroup, execution engine]
---

import { FlowLab } from "@site/src/components/Lab/FlowBuilderRunner";
import { PipelineDiagram } from "@neokapi/docs-shared";

# Pipeline

The **pipeline** is how neokapi runs a [flow](/framework/flows). A flow names
the tools and their order. The pipeline runs them concurrently: a
[format reader](/framework/formats), a chain of [tools](/framework/tools), and a
format writer, each in its own goroutine and connected by buffered channels of
[Parts](/framework/content-model).

<PipelineDiagram
  animated
  stages={[
    { label: "RawDocument" },
    { label: "Reader", sub: "DataFormat", role: "io", note: "drains first", channel: "" },
    { label: "Tool 1", note: "goroutine" },
    { label: "Tool 2", note: "goroutine" },
    { label: "⋯" },
    { label: "Writer", sub: "DataFormat", role: "io", note: "goroutine" },
    { label: "Output" },
  ]}
/>

The executor is built on Go's native concurrency: goroutines for the stages,
channels for the connections, and `errgroup` for coordination.

The reader and writer shown here are the **file binding**, the default way
content enters and leaves the pipeline. The same tool stream can instead be bound
to a project store, a `.kpz` workspace, or an interchange file, with no reader or
writer ([flows: source and sink](/framework/flows#source-and-sink-the-flows-ends)).

:::tip Watch it run, step by step
Run a file through a pipeline and drive it with **Next**. Each step advances the
stream by one event, so you can watch Parts move out of the reader, through the
tools, and into the writer, inspecting how each Part changes at every stage. This
runs the real `kapi` engine in your browser via WebAssembly.
:::

<FlowLab withRecordedTraces defaultScenarioId="pseudo" defaultSampleId="messages-json" />

## Streaming, not batching

The reader emits Parts as it parses, and those Parts flow downstream while the
reader is still working. Each tool processes a Part as
soon as it arrives and forwards it, so the writer can begin emitting output
before the reader has finished reading. Memory use stays bounded by the size of
the channel buffers and the Parts in flight, not by the size of the document.

The [content model](/framework/content-model) follows from this: a `Part` is the
indivisible unit that flows through, and a document is a stream of Parts (layer
starts, blocks, data, layer ends).

## Channels and backpressure

Adjacent stages are connected by **buffered channels**, by default a buffer of
64 Parts. The buffer decouples the stages so a fast reader does not have to wait
on a slow tool for every single Part, but it is bounded: when the buffer fills,
the upstream stage blocks on its send until the downstream stage catches up.
That blocking is the backpressure. A slow tool, such as an AI translation step
waiting on a network call, throttles the reader that feeds it, with no explicit
rate limiting or queue management.

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
cancellation propagates promptly to all goroutines: a stage blocked on a send or
a receive wakes up and returns. On a partial failure the pipeline tears down
with no leaked goroutines, and the caller receives the first error.

Because cancellation flows from the caller's context, a pipeline is also
cancellable from the outside. Closing a CLI run, a request timeout, and a desktop
"stop" button all cancel the same context and unwind every stage.

## Layers of concurrency

The tool chain is one of three independent concurrency layers:

| Layer                      | What runs concurrently                                          |
| -------------------------- | --------------------------------------------------------------- |
| **Stage (tool) pipeline**  | Each tool in the chain runs in its own goroutine, the default.  |
| **Intra-tool blocks**      | A block-handling tool can fan its work across N goroutines while preserving Part order (see [Tools](/framework/tools)). |
| **Batch documents**        | The executor processes many input files in parallel, bounded by a concurrency limit. |

Document-level batching is controlled on the executor. `MaxConcurrency` bounds
how many documents run at once (`1` is sequential, `0` means use the number of
CPUs) and a semaphore enforces the bound. With fail-fast enabled (the default),
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

A caller that feeds Parts in and reads results out directly, with no files, can
ask the executor for the chain's input and output channels. The executor wires
the same goroutine-per-tool pipeline and leaves the ends open for the caller.

## Observation

Because the pipeline is a stream of Parts, work can be observed without
disturbing it. The executor accepts **collectors** that are fed the output Parts
of each document as it completes. Cross-document analysis (scoping reports,
repetition analysis across a batch) accumulates its results this way. A
collector reads the finished stream from outside the tool chain.

## Related reading

- [Flows](/framework/flows): the graph the pipeline executes.
- [Tools](/framework/tools): the stages that run in the pipeline.
- [Content Model](/framework/content-model): the Part that streams through it.
- [E-01: Processing Engine](/contribute/architecture/engine/e-01-processing-engine): the design rationale.
