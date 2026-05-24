---
sidebar_position: 7
title: Flows
---

import { DualExample } from "@site/src/components/curated";

# Flows

A **flow** is a named, ordered composition of [tools](/framework/tools). It is
the neokapi analogue of an Okapi _pipeline_: a recipe that says "read the
document, run these steps in this order, write the result." Where a single tool
does one thing, a flow assembles several into an end-to-end workflow — leverage
from memory, look up terminology, translate the remainder, check quality — and
gives it a name so it can be run, shared, and reused.

Flows separate _what to do_ from _how it runs_. A flow describes the chain; the
[pipeline executor](/framework/pipeline) decides how to run it concurrently. The
generated list of flows that ship in a given build is the `kapi flows` command
and the [Command Reference](/commands).

## A flow is a sequence of tools

At its simplest, a flow is a list of tools that Parts stream through in order.
The framework models this directly:

```go
type Flow struct {
    Name          string
    Tools         []tool.Tool   // for single-document / sequential execution
    ToolFactories []ToolFactory // for parallel: a fresh tool chain per document
}
```

The `Builder` provides a fluent API for assembling one in Go:

```go
f, err := flow.NewFlow("translate").
    AddTool(tools.NewTMLeverageTool(tmCfg)).
    AddTool(tools.NewTermLookupTool(termCfg)).
    AddTool(aitools.NewAITranslateTool(translateCfg)).
    AddTool(tools.NewQACheckTool(qaCfg)).
    Build()
```

A flow built this way holds concrete tool instances. For running the same flow
over many documents at once, a flow can instead hold **tool factories** — each
document then gets its own fresh tool chain, so per-document state never leaks
between concurrent runs.

## The two authored representations

A flow can be authored in two equivalent forms, and both compile to the same
executable graph.

### Steps — the human-authored form

The **steps** format is a YAML list of tools. It is the form people write by
hand:

```yaml
input: json
output: json
steps:
  - tool: tm-leverage
    label: Apply TM matches
    config:
      threshold: 0.7
  - tool: ai-translate
    label: Translate the rest
    config:
      provider: anthropic
  - tool: qa-check
    label: Quality checks
```

Each step names a tool and optionally configures it. `input` and `output` name
the formats to read and write; when omitted they default to `auto`, which means
detect from the file. Steps run sequentially: each tool's output channel feeds
the next tool's input channel.

A step can also fan out. A `parallel:` block runs several tools on the same
stream concurrently, each on its own branch:

```yaml
steps:
  - tool: create-target
    config: { copySource: true }
  - parallel:
      - tool: word-count
      - tool: qa-check
      - tool: chars-listing
```

### Graph — the canonical form

Internally a flow is a directed graph of **nodes** (a reader, tool nodes, a
writer) connected by **edges**:

```go
type FlowDefinition struct {
    ID    string
    Name  string
    Nodes []FlowNode // reader / tool / writer
    Edges []FlowEdge // directed connections
}
```

The graph is what the visual flow editor reads and writes, and it is the form
that survives to execution. Compilation from steps to graph is mechanical:
`StepsToGraph` creates a reader node from the `input` format, a tool node for
each step (chained by edges), a fan-out for each `parallel:` block, and a writer
node from the `output` format. A `parallel:` block becomes several tool nodes
all connected from the previous node; the step after it connects from every
branch endpoint (fan-in). Cycles are rejected — the executor runs nodes in
topological order.

Because both forms compile to the same graph, the steps you write by hand and
the graph you build in the editor are interchangeable: a hand-written flow opens
in the editor, and an editor-built flow runs from the CLI.

## Running a flow

A flow is run against one or more input files. The runner detects each file's
format, creates the reader and writer, instantiates the tool chain, and hands it
to the executor. Composed (multi-tool) flows run with `kapi run`; single-tool
flows are exposed directly as their tool's command:

```bash
# Run a composed flow (two or more tools) on a file
kapi run ai-translate-qa -i app.xliff --target-lang fr

# List the composed flows available in this build
kapi flows
```

The demo below runs the `pseudo-translate` flow — `reader → pseudo-translate →
writer`. Because it is a single-tool flow, it is invoked directly as `kapi
pseudo-translate`. The left pane is the CLI invocation; the right pane is the
framework's result, the same file with its source strings replaced by accented
look-alikes:

<DualExample
  command="kapi pseudo-translate messages.json -o out.json --target-lang fr"
  seed={["messages.json"]}
  result={{
    kind: "before-after",
    sample: "messages.json",
    command: "kapi pseudo-translate messages.json -o out.json --target-lang fr",
    outputPath: "out.json",
  }}
  caption="The pseudo-translate flow: read → pseudo-translate → write."
/>

## Built-in flows

The framework ships a set of built-in flows covering common workflows —
AI translation, AI translation with a quality pass, pseudo-translation for
layout testing, TM leverage, rule-based QA, and a redact-translate-restore flow
for [sensitive content](/framework/redaction). Rather than maintain a list here,
run `kapi flows` or see the [Command Reference](/commands), both generated from
the live flow set.

## Related reading

- [Tools](/framework/tools) — the units a flow composes.
- [Pipeline](/framework/pipeline) — how the executor runs a flow's graph concurrently.
- [Flow Authoring](/contribute/flow-authoring) — the full steps-format reference and more examples.
