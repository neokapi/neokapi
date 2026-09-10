---
sidebar_position: 1
title: Architecture
description: "An overview of the neokapi framework architecture: the streaming pipeline, content model, format readers and writers, composable tools, and the multi-module Go structure."
keywords: [neokapi, architecture, streaming pipeline, content model, content engine, multilingual content, go modules]
---

import { ArchitectureDiagram } from "@neokapi/docs-shared";

# neokapi: Architecture

neokapi is an open content engine in Go. It reads supported formats into a
shared content model, runs editing and checking tools, and writes content back
through the format's writer. The [`kapi` project tool](/kapi/overview), desktop
app and agent interfaces use this engine. Applications can also import the Go
library directly.

The engine separates content processing from the project's applicable terms,
writing guidance and check policy. A check result names the analysis performed;
a successful gate does not establish factual accuracy or other qualities that
were not assessed. Multilingual content, AI ingestion and programmatic editing
use the same format-aware processing interfaces.

Fidelity depends on the format and operation. No-op byte preservation,
preservation outside an edit and semantic round-trip fidelity are distinct
properties. Consult the [format evidence](/format-maturity) for the applicable
reader and writer. Start with the [Go quickstart](/framework/go-quickstart), or
read the [Architecture Decisions](/contribute/architecture/foundations/f-01-framework-and-modules)
for the system's design.

## Processing Pipeline

<ArchitectureDiagram />

The edges are the flow's **source** and **sink**: bindings that decide where
content enters and leaves. The default, shown above, is the **file binding**: a
[reader](/framework/formats) turns supported source files into a stream of
[Parts](/framework/content-model) and a [writer](/framework/formats) turns the
stream back into files with the fidelity its format supports. The same flow can instead bind to the project
store, a `.kpz` workspace, or an interchange file, with no reader or writer
([flows: source and sink](/framework/flows#source-and-sink-the-flows-ends)).
Between the edges runs a [flow](/framework/flows): a serial chain of
[tools](/framework/tools) connected by buffered channels of Parts. The tools divide by capability: **annotators** attach stand-off
[overlays and annotations](/framework/content-model#two-ways-to-annotate-a-block)
(segmentation, terminology, entities, check findings, analysis results),
**translators** fill in targets, and **validators** check and enforce, while
[content memory](/framework/content-memory) and the
[terms store](/framework/terminology) feed the relevant stages.

Concurrency runs at three levels at once: each stage is its own goroutine joined
by channels with automatic backpressure; a block-handling stage such as
translation can **fan out** across N goroutines with an ordered fan-in; and the
executor runs many documents in parallel, bounded by `MaxConcurrency`. Context
cancellation propagates to every stage. Readers, writers, and tools can be
supplied by [plugins](/contribute/implementation/engine/plugin-model) (the
[`kapi-sat`](/contribute/architecture/multilingual/m-02-segmentation) segmenter, the
[`kapi-pdfium`](/contribute/architecture/engine/e-08-document-structure-tiers) PDF
reader, or any remote plugin), dispatched as subprocesses over gRPC. See
[F-01](/contribute/architecture/foundations/f-01-framework-and-modules) and
[E-01](/contribute/architecture/engine/e-01-processing-engine).

## Package Layout

The tree below is trimmed to the packages the rest of these pages refer to;
`ls core` in the repository lists the rest.

```
neokapi/
├── go.mod                           # module github.com/neokapi/neokapi
├── go.work                          # coordinates the framework, host, CLI, app and plugin modules
│
├── core/                            # Platform-agnostic framework packages
│   ├── model/                       # Part, Block, Layer, Run, Target, Overlay, Anchor, Data, Media
│   ├── format/                      # DataFormatReader/Writer interfaces, detection
│   ├── formats/                     # Built-in format implementations, one package each
│   ├── tool/                        # Tool interface, BaseTool dispatch, EditPlan
│   ├── tools/                       # Built-in tools (pseudo-translate, qa, segmentation, …)
│   ├── flow/                        # Executor, Builder, FlowDefinition, placement pass
│   ├── registry/                    # FormatRegistry, ToolRegistry
│   ├── ai/                          # LLM-backed tools and prompt assembly
│   ├── check/                       # Report and Finding
│   ├── project/                     # kapi.yaml recipe, context axes, gates
│   ├── profile/                     # Voice profiles and term rules
│   ├── kbf/                         # Content bundle (.kbf.json) and its validator
│   ├── plugin/                      # Plugin manifests and the gRPC protocol
│   └── …                            # segmentation, redaction, graph, state, storage, and the rest
│
├── memory/                          # Content memory
├── terms/                           # Terms store
├── kpz/                             # Project archive (.kpz)
├── providers/                       # LLM provider backends
│
├── host/                            # Cobra-free runtime + services (module: …/host)
├── cli/                             # Thin Cobra shell over host (module: …/cli)
├── kapi/                            # The kapi binary (module: …/kapi)
├── apps/kapi-desktop/               # Kapi Desktop (Wails v3; module: …/kapi-desktop)
├── plugins/                         # In-repo plugins, each its own module
├── packages/                        # Shared TypeScript packages (@neokapi/ui-primitives, @neokapi/flow-editor, …)
└── docs/                            # Repo internals (format ops, testing, runbooks)
```

The framework module (repo root) stays platform-agnostic. `memory/`,
`terms/`, `kpz/` and `providers/` are top-level framework packages rather than
nested under `core/`. Front-ends such as the CLI and the desktop app, and any
other consumer, attach through the plugin and extension registries rather than
by direct imports, so the framework never depends on a particular platform.

## The framework concepts

To see these concepts working together in a few lines of Go (register the
formats, read a file into the content model, run a tool, and write the result),
start with the [Go quickstart](/framework/go-quickstart). The framework rests on
a few concepts, each with its own page:

- **[Content Model](/framework/content-model)**: the format-independent
  representation. A document becomes a stream of `Part`s carrying layers, blocks,
  runs, overlays, data, and media. Embedded content (HTML inside JSON, CDATA in
  XML) is modeled as nested layers, each with its own format.
- **[Formats](/framework/formats)**: paired readers and writers that produce and
  consume the content model.
- **[Tools](/framework/tools)**: the processing units. Each reads Parts from a
  channel, transforms them, and writes them out.
- **[Flows](/framework/flows)**: named, ordered compositions of tools.
- **[Pipeline](/framework/pipeline)**: the concurrent executor that runs a flow:
  goroutines, buffered channels, and context-driven cancellation.

For the concrete Go interfaces and method signatures behind these concepts, see
the [Interface Reference](/contribute/interfaces). For the design rationale, see
the [Architecture Decisions](/contribute/architecture/foundations/f-01-framework-and-modules).
