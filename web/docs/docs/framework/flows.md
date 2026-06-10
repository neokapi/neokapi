---
sidebar_position: 8
title: Flows
description: A flow is a named, ordered composition of tools — the neokapi equivalent of an Okapi pipeline. Flows are defined in YAML, can be embedded in a project file or stored separately, and run with a single kapi command.
keywords: [flows, pipeline, YAML, tool composition, kapi run, localization workflow]
---

import { DualExample } from "@site/src/components/curated";
import { FlowBuilderRunner } from "@site/src/components/Lab/FlowBuilderRunner";

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
    AddTool(termbase.NewTermLookupTool(tb, termCfg)).
    AddTool(aitools.NewAITranslateTool(provider, translateCfg)).
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

Each step names a tool and optionally configures it. Steps run sequentially: each
tool's output channel feeds the next tool's input channel. A flow carries only
its steps — *where content comes from and goes to* is a binding decided when you
run it, not part of the flow (see [Source and sink](#source-and-sink-the-flows-ends)).

A [check](/framework/checks) such as `qa-check` is just a read-only step: it
attaches findings to each block as annotations rather than rewriting content, so
it typically sits last and a CI gate reads its result.

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

### Transformers

Some tools rewrite the **source** itself — redaction replacing sensitive spans
with placeholders, a simplifier rephrasing for clarity, a normalizer. These
**transformers** are ordinary steps in the same ordered list as everything
else:

```yaml
steps:
  - tool: redact          # rewrites the source in place
  - tool: ai-translate    # translates the redacted source
  - tool: qa-check
```

A transformer does not edit the block directly: it is a read-only **edit
producer** that returns an edit plan, and a single framework-owned **applier**
performs the rewrite — applying the edits, rebasing surviving run-anchored
overlays (segments, term and entity spans) onto the new runs, vaulting any
secrets fail-closed, and bounds-checking the result, atomically. Because the
applier mutates inline and in order, each transformer settles the source
before later steps observe it.

Ordering safety is a **placement pass** that validates every flow at build and
load time. It rejects a transformer placed after a step that produces a
committed target (rewriting source would orphan the targets — `unredact` is
exempt because it rewrites both sides), rejects a recoverable transformer such
as `redact` placed after a step that sends source to a remote service (except
a step producing an input the transformer's configuration requires), and warns
when a transformer sits later than its earliest valid slot, since every
overlay present at apply time must be rebased. See
[the tool system AD](/contribute/architecture/006-tool-system) for the model.

A flow that declares the removed `source_transforms:` field is rejected at
load with a migration error: list the transformers as ordered steps instead.

### Graph — the canonical form

Internally a flow is a directed graph of **tool nodes** connected by **edges**:

```go
type FlowDefinition struct {
    ID    string
    Name  string
    Nodes []FlowNode // tool nodes
    Edges []FlowEdge // directed connections
}
```

The graph is what the visual flow editor reads and writes, and it is the form
that survives to execution. Compilation from steps to graph is mechanical:
`StepsToGraph` creates a tool node for each step (chained by edges) and a fan-out
for each `parallel:` block. A `parallel:` block becomes several tool nodes all
connected from the previous node; the step after it connects from every branch
endpoint (fan-in). Cycles are rejected — the executor runs nodes in topological
order. The flow's ends — where content enters and leaves — are not nodes; they
are bindings, covered next.

Because both forms compile to the same graph, the steps you write by hand and
the graph you build in the editor are interchangeable: a hand-written flow opens
in the editor, and an editor-built flow runs from the CLI.

:::tip Build a flow, then run it
Assemble a flow in the same node editor the desktop app uses — add, remove, and
reorder tool nodes — then press **Run flow** to execute it on a file and step
through the result. The graph is serialized to a `.kapi` recipe and run with the
real `kapi` engine in your browser via WebAssembly, so the flow you build is the
flow that runs.
:::

<FlowBuilderRunner defaultSampleId="support-reply" />

## Source and sink: the flow's ends

A flow processes a stream of blocks; *where that stream comes from and where the
result goes* are its **source** and **sink** bindings. The same flow runs over a
loose file, the blocks already in a project, a `.klz` workspace, or content
imported from an interchange file — only the binding changes:

| Binding | As source | As sink |
| --- | --- | --- |
| `file` (default) | read + parse a file | write a file (round-trip via skeleton) |
| `store` / `klz` | existing blocks + overlays | commit overlays — no file |
| interchange | import from XLIFF / PO / a bilingual `.klz` | emit interchange for a translator |
| `none` | — | discard (analysis / checks only) |

Bindings are resolved when you run the flow, by precedence: an explicit `-i` / `-o`
flag, then the project or `.klz` you're in, then the flow's own intent, then
auto-detection from the path. A plain path is detected (`.klz` → workspace,
`.xliff` → interchange, a document → `file`); a `scheme:` is explicit
(`-o store:`, `-o xliff:hand.xliff`). `kapi run <flow> --explain` prints the
resolved `source → sink` without running anything.

A flow only ever declares *intrinsic* intent — a check flow that produces no
document sets `sink: none` — never a path. Inside a project, a run with no `-o`
lands its work in the store (process-only); `kapi merge` materializes files when
you're ready. See [AD-026](/contribute/architecture/026-flow-io-binding) for the
full model.

## Running a flow

A flow is run against a source. The runner resolves the source and sink bindings,
instantiates the tool chain, and hands it to the executor. Composed (multi-tool)
flows run with `kapi run`; single-tool flows are exposed directly as their tool's
command:

```bash
# Run a composed flow (two or more tools) on a file
kapi run ai-translate-qa -i app.xliff --target-lang fr

# List the composed flows available in this build
kapi flows
```

The demo below runs the `pseudo-translate` flow — `source → pseudo-translate →
sink`. Because it is a single-tool flow, it is invoked directly as `kapi
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
  caption="The pseudo-translate flow over the file binding: file → pseudo-translate → file."
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
