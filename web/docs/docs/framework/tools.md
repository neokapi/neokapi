---
sidebar_position: 6
title: Tools
description: Tools are the composable processing stages in a neokapi pipeline — each reads Parts from an input channel, transforms them, and writes to an output channel. Built-in tools cover translation, QA, segmentation, TM leverage, and more.
keywords: [tools, pipeline stage, processing, translation, QA, TM leverage, segmentation, composable]
---

import { ToolLab } from "@site/src/components/Lab/ToolLab";
import { ScriptLab } from "@site/src/components/Lab/ScriptLab";
import { PipelineDiagram } from "@neokapi/docs-shared";

# Tools

A **tool** is the unit of processing in neokapi. Where a [format](/framework/formats)
reader turns a document into a stream of [Parts](/framework/content-model) and a
writer turns the stream back into a document, a tool sits in between: it reads
Parts from an input channel, transforms them, and writes them to an output
channel. Tools are the neokapi analogue of an Okapi pipeline _Step_.

Because every tool speaks the same channel contract, tools compose freely. A
translation workflow is just a chain of tools — leverage from memory, look up
terminology, translate the remainder, check quality — each handling the Parts it
cares about and passing the rest through untouched. The category of work a tool
does is not fixed by the framework; the same interface backs analysis,
transformation, enrichment, and validation alike. The authoritative, generated
list of what ships in the current build is the [Tool Reference](/tools).

:::tip Try a tool on a file
Pick a tool, edit its configuration in the live form, and run it on a sample
file to see how each translatable [Block](/framework/content-model) changes —
source before, tool output after. The same form that drives the configuration
here is the one the visual editors and the [Tool Reference](/tools) render from
the tool's schema. This runs the real `kapi` engine in your browser via
WebAssembly.
:::

<ToolLab defaultSampleId="messages-json" />

## The Tool interface

A tool is anything that satisfies one small interface:

```go
type Tool interface {
    Name() string
    Description() string
    Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error
    Config() ToolConfig
    SetConfig(cfg ToolConfig) error
}
```

`Process` is the heart of it. A tool consumes Parts from `in`, does its work, and
emits Parts on `out`. It blocks until the input channel is closed (input
exhausted) or the context is cancelled. This is the lowest common denominator
that flow composition relies on: chaining tools means wiring one tool's `out`
channel to the next tool's `in` channel, so every tool must speak it.

The remaining methods carry the tool's identity (`Name`, `Description`) and its
configuration. Configuration is a small interface of its own — a tool config
knows which tool it belongs to, how to reset to defaults, and how to validate
itself:

```go
type ToolConfig interface {
    ToolName() string
    Reset()
    Validate() error
}
```

## Part-type dispatch with BaseTool

Most tools only care about one or two kinds of Part — usually
[Blocks](/framework/content-model) (translatable content). Writing the full
channel loop for every tool would be repetitive and error-prone, so the
framework provides `BaseTool`, an embeddable type that implements `Process` once
and dispatches each Part to a per-type handler:

```go
type PartHandler func(part *model.Part) (*model.Part, error)

type BaseTool struct {
    ToolName        string
    ToolDescription string
    Cfg             ToolConfig

    // Block handler — set exactly one. The view type bounds what it may write.
    Annotate  func(BlockView) error  // read-only: overlays / annotations / properties
    Translate func(TargetView) error // writes target
    Transform func(SourceView) error // rewrites source (and may write target)

    // Other Part types stay untyped.
    HandleDataFn       PartHandler
    HandleMediaFn      PartHandler
    HandleLayerStartFn PartHandler
    HandleLayerEndFn   PartHandler
    HandleGroupStartFn PartHandler
    HandleGroupEndFn   PartHandler
}
```

A concrete tool embeds `BaseTool` and sets only the handlers it needs.
`BaseTool.Process` reads each Part, switches on its `Type`, and calls the
matching handler. **Any handler left unset is a pass-through** — the Part flows
to the output channel unchanged. For Blocks, a tool sets one of three
capability-typed handlers and the view it receives decides what it may write
(the immutability model — see [the tool-system AD](/contribute/architecture/006-tool-system)):
`Annotate` reads source and target but writes only overlays, annotations, and
properties; `Translate` writes the target; `Transform` rewrites the source. The
forbidden writes simply aren't on the view, so a quality check can't accidentally
mutate the source.

The case-transform tool is a representative example. It can rewrite the source,
so it sets `Transform`:

```go
func NewCaseTransformTool(cfg *CaseTransformConfig) *tool.BaseTool {
    t := &tool.BaseTool{
        ToolName:        "case-transform",
        ToolDescription: "Transforms the case of source and/or target text",
        Cfg:             cfg,
    }
    t.Transform = func(v tool.SourceView) error {
        if !v.Translatable() {
            return nil // pass through
        }
        conf := t.Cfg.(*CaseTransformConfig)
        if conf.ApplySource {
            v.SetSourceText(transformCase(v.SourceText(), conf.Mode))
        }
        return nil
    }
    return t
}
```

When a tool needs full control of the loop — for example to accumulate state
across many Parts, or to emit more Parts than it consumes — it can implement
`Process` directly instead of using the handler fields.

## How tools compose

The streaming contract is what makes composition trivial. Three Parts — a layer
start, a block, a layer end — flowing through a two-tool chain look like this:

<PipelineDiagram
  animated
  stages={[
    { label: "reader", role: "io" },
    { label: "segmentation", role: "annotate", note: "handles Block · passes Layer*" },
    { label: "ai-translate", role: "translate", note: "handles Block · passes Layer*" },
    { label: "writer", role: "io" },
  ]}
/>

Each tool runs in its own goroutine, connected by buffered channels. A tool that
does not handle layer markers simply relays them, so structural context survives
the whole chain even though only some stages act on it. Ordering is preserved:
the segmentation tool's output for a block reaches the translation tool before
the next block does. The mechanics of that concurrency — goroutines, buffered
channels, backpressure, error propagation — are covered in
[Pipeline](/framework/pipeline); how chains are described and built is covered in
[Flows](/framework/flows).

### Wrapping tools

Because a tool is just an interface, one tool can wrap another to add behavior
without the inner tool knowing. The framework uses this for **intra-tool block
parallelism**: `ParallelBlockTool` wraps a block-handling tool and fans its
block handler out across N goroutines while preserving Part order, which is
valuable for IO-bound tools such as AI or MT translation where each block is an
independent network call. The wrapper presents the same `Tool` interface, so the
rest of the flow is unaffected.

## Categories of work

The framework does not enforce tool categories — the interface is the same
whether a tool transforms, enriches, or validates. As a way of thinking about
what a tool does, the built-in tools fall into a few broad kinds:

| Kind          | What it does                              | Examples                                            |
| ------------- | ----------------------------------------- | --------------------------------------------------- |
| **Transform** | Modify content in place                   | case change, search/replace, redact                 |
| **Enrich**    | Attach matches or metadata to content     | segmentation, TM leverage, terminology lookup, AI translation |
| **Validate**  | Check content without modifying it        | QA checks, length checks, terminology enforcement    |
| **Analyze**   | Accumulate statistics across the stream   | word count, repetition analysis, character inventory |
| **Convert**   | Adjust representation                     | encoding conversion, line-break normalization        |

Enrich and validate tools commonly use the [Block annotation
system](/framework/content-model) rather than rewriting text: a TM-leverage tool
attaches candidate matches, a QA tool attaches findings, and downstream tools or
an editor read those annotations. This shared annotation channel is how
[translation memory](/framework/translation-memory),
[terminology](/framework/terminology), and [brand voice](/framework/checks/brand-voice)
results all reach the same consumer without colliding.

## Configuration and schemas

Tools that expose configuration declare it as a struct with `schema:"…"` field
tags. The framework derives a JSON-Schema-style descriptor from that struct by
reflection, which is what drives auto-generated CLI flags, validation, and the
configuration forms in the visual editors. A tool that opts into this advertises
its schema through an optional interface; the generated [Tool
Reference](/tools) renders each tool's parameters from exactly these schemas, so
it always matches the build.

## Scripting — write a transform in JavaScript

Not every transform deserves its own Go tool. The built-in `script` tool runs a
small JavaScript program against each Part. Define `process(part)`, edit
`part.block.source` or `part.block.targets`, and return the part to keep it (or
`null` to drop it) — or omit the function and write top-level code against the
global `part`, calling `emit(part)` / `skip()`. It is the quickest way to
prototype a one-off rule, and it runs anywhere the engine runs — including the
browser, via the embedded interpreter.

:::tip Write a script, run it on your file
Edit the JavaScript below — `process(part)` runs once per Part, with full
autocomplete for the `part` API — or load an example, then run it on a sample or
your own file and read the per-Block before/after of source and target.
:::

<ScriptLab defaultSampleId="messages-json" />

## Where tools come from

Built-in tools live in the framework and are registered into a `ToolRegistry`,
which maps a tool name to a factory. Tools can also be supplied by
[plugins](/contribute/plugins) — discovered at runtime and dispatched as
subprocesses over gRPC — so the available toolset can extend beyond what is
compiled into a given binary without changing the interface tools satisfy.

## Related reading

- [Tool Reference](/tools) — the generated list of built-in tools and their parameters.
- [Flows](/framework/flows) — composing tools into a named pipeline.
- [Pipeline](/framework/pipeline) — the streaming executor that runs the chain.
- [Implementing a Tool](/contribute/tools) and [Tool Authoring](/contribute/tool-authoring) — writing your own.
