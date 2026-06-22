---
sidebar_position: 1
title: Architecture
description: An overview of the neokapi framework architecture — the streaming pipeline, content model, format readers and writers, composable tools, and the multi-module Go structure.
keywords: [neokapi, architecture, streaming pipeline, content model, content engine, localization, go modules]
---

import { ArchitectureDiagram } from "@neokapi/docs-shared";

# neokapi: Architecture

neokapi is an open-source, format-aware **content engine** built in Go. It parses
any format into one unified content model, edits the content inside it, checks it,
and writes it back — byte-for-byte. Get your content right — edit it, check it,
keep it on brand — and the same engine makes it work in every language. It also
serves AI ingestion and programmatic editing. It provides format-aware document parsing,
composable processing tools, and a concurrent streaming pipeline. The [`kapi` CLI
and desktop app](/kapi/overview) and [Kapi React](/react/introduction) are
surfaces built on top of this engine — but the content model, format readers and
writers, tools, and pipeline are equally a Go library you can import and drive
directly. If you want to start with running code, jump to the
[Go quickstart](/framework/go-quickstart); for the reasoning behind each major
design choice, see the [Architecture Decisions](/contribute/architecture/001-vision-and-modules).

## Processing Pipeline

<ArchitectureDiagram />

The edges are the flow's **source** and **sink** — bindings that decide where
content enters and leaves. The default, shown above, is the **file binding**: a
[reader](/framework/formats) turns source files of any format into a stream of
[Parts](/framework/content-model) and a [writer](/framework/formats) turns the
stream back into files, byte-for-byte. The same flow can instead bind to the project
store, a `.klz` workspace, or an interchange file — with no reader or writer
([flows: source and sink](/framework/flows#source-and-sink-the-flows-ends)).
Between the edges runs a [flow](/framework/flows): a serial chain of
[tools](/framework/tools) connected by buffered channels of Parts. The tools divide by capability — **annotators** attach stand-off
[overlays and annotations](/framework/content-model#two-ways-to-annotate-a-block)
(segmentation, terminology, entities, QA findings, analysis results),
**translators** fill in targets, and **QA** tools check and enforce — while
[translation memory](/framework/translation-memory) and the
[termbase](/framework/terminology) feed the relevant stages.

Concurrency runs at three levels at once: each stage is its own goroutine joined
by channels with automatic backpressure; a block-handling stage such as AI
translation can **fan out** across N goroutines with an ordered fan-in; and the
executor runs many documents in parallel, bounded by `MaxConcurrency`. Context
cancellation propagates to every stage. Readers, writers, and tools can be
supplied by [plugins](/contribute/notes-internal/plugin-model) — the Java
[Okapi Bridge](/contribute/architecture/007-plugin-system), the `kapi-sat`
segmenter, the [`kapi-pdfium`](/contribute/architecture/028-pdf-reader-plugin) PDF
reader, or any remote plugin — dispatched as subprocesses over gRPC. See
[AD-001](/contribute/architecture/001-vision-and-modules) and
[AD-004](/contribute/architecture/004-processing-engine).

## Package Layout

```
neokapi/
├── go.mod                           # module github.com/neokapi/neokapi
├── go.work                          # coordinates the framework + CLI + app modules
│
├── core/                            # Platform-agnostic framework packages
│   ├── model/                       # Part, Block, Layer, Run, Target, Overlay, Data, Media
│   ├── format/                      # DataFormatReader/Writer interfaces, detection
│   ├── tool/                        # Tool interface, BaseTool dispatch
│   ├── flow/                        # Executor, Builder, FlowDefinition
│   ├── registry/                    # FormatRegistry, ToolRegistry
│   ├── encoding/                    # Text encoding utilities
│   ├── locale/                      # BCP-47 locale handling
│   ├── editor/                      # Block index serialization and preview generation
│   ├── version/                     # Build version info
│   ├── formats/                     # Built-in format implementations
│   │   └── …                        # one package each (reader.go, writer.go, config.go)
│   ├── ai/                          # AI pipeline tools, NER, prompt assembly
│   ├── mt/                          # Machine-translation pipeline tools
│   ├── brand/                       # Brand voice profiles, scoring, starter packs
│   ├── tools/                       # Utility tools (wordcount, pseudo, segmentation, …)
│   ├── storage/                     # Shared SQLite infrastructure (Open, Migrate)
│   ├── project/                     # .kapi project file format (Load, Save, Validate)
│   ├── plugin/                      # Plugin system (gRPC, loader, bridge, registry)
│   └── testutil/                    # Shared test helpers
│
├── sievepen/                        # Translation memory (interface, in-memory, SQLite)
├── termbase/                        # Terminology (interface, in-memory, SQLite)
├── providers/
│   ├── ai/                          # package aiprovider — LLM backends
│   └── mt/                          # package mtprovider — MT backends
│
├── cli/                             # Shared CLI base (module: …/cli)
├── kapi/                            # Kapi standalone CLI (module: …/kapi)
├── apps/kapi-desktop/          # Kapi Desktop (Wails v3; module: …/kapi-desktop)
├── packages/
│   ├── ui/                          # @neokapi/ui-primitives — shared shadcn/ui primitives
│   └── flow-editor/                 # @neokapi/flow-editor — shared React flow editor
└── docs/                            # Architecture decisions, notes
```

The framework module (repo root) stays platform-agnostic. `sievepen/`,
`termbase/`, and `providers/` are top-level framework packages — not nested
under `core/`. Front-ends such as the CLI and the desktop app, and any other
consumer, attach through the plugin and extension registries rather than by
direct imports, so the framework never depends on a particular platform.

## The framework concepts

To see these concepts working together in a few lines of Go — register the
formats, read a file into the content model, run a tool, and write the result —
start with the [Go quickstart](/framework/go-quickstart). The framework rests on
a few concepts, each with its own page:

- **[Content Model](/framework/content-model)** — the format-independent
  representation. A document becomes a stream of `Part`s carrying layers, blocks,
  fragments, spans, data, and media. Embedded content (HTML inside JSON, CDATA in
  XML) is modeled as nested layers, each with its own format.
- **[Formats](/framework/formats)** — paired readers and writers that produce and
  consume the content model. The neokapi analogue of an Okapi _filter_.
- **[Tools](/framework/tools)** — the processing units. Each reads Parts from a
  channel, transforms them, and writes them out. The analogue of an Okapi _step_.
- **[Flows](/framework/flows)** — named, ordered compositions of tools. The
  analogue of an Okapi _pipeline_.
- **[Pipeline](/framework/pipeline)** — the concurrent executor that runs a flow:
  goroutines, buffered channels, and context-driven cancellation. The analogue of
  the Okapi _PipelineDriver_.

For the concrete Go interfaces and method signatures behind these concepts, see
the [Interface Reference](/contribute/interfaces). For the design rationale, see
the [Architecture Decisions](/contribute/architecture/001-vision-and-modules).
