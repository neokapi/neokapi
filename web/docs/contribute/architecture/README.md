---
sidebar_position: 0
title: Architecture Decisions Overview
slug: index
description: Index of Architecture Decisions for the neokapi open content and language intelligence framework — foundations, the engine, context, surfaces, multilingual content, and assurance.
keywords: [architecture decisions, neokapi, framework design, content model, context, plugin system, framework architecture]
---

# Architecture Decisions — neokapi

These are the Architecture Decisions for the **neokapi framework**, the open
content and language engine. Everything here is Apache-2.0 and describes the
framework modules: the repository root, the cobra-free host runtime, the shared
CLI base, the `kapi` binary, and Kapi Desktop.

Each decision describes the **current** state of its subsystem, not the history
of how it got there. When a subsystem evolves, its decision is updated in place;
the history lives in version control.

Tactical detail — SQL schemas, wire formats, algorithm pseudocode — is separated
into [Implementation Notes](/contribute/implementation/index).

## How the corpus is organized

Six series, by concern. A decision's identifier is its series letter and its
position within the series, and it does not change when a neighbour is added or
retired:

| Series | Concern |
| --- | --- |
| **F — Foundations** | what the framework is, what a unit of content is, and how it is identified and serialized |
| **E — Engine** | how content is read, processed, written, and extended |
| **C — Context** | what a project knows, where it keeps it, and what governs it |
| **S — Surfaces** | the CLI, the desktop app, agent surfaces, and the runtime libraries |
| **M — Multilingual** | what it takes for content to exist in more than one language |
| **A — Assurance** | how the framework proves it works |

Each series is a directory, and its decisions sort by `sidebar_position` within
it. The sidebar is generated from the directory: adding a decision means adding a
file, never editing a list.

## C — Context

What a project knows, where it keeps it, and what governs it.

| AD | Title | Scope |
| --- | --- | --- |
| [C-01](context/c-01-project-model.md) | The project model | the `kapi.yaml` recipe, the committed `.kapi/` layout, the store interface, `ProjectContext` |
| [C-02](context/c-02-coordinates-and-governance.md) | Coordinates and governance | the product × channel space, per-file resolution, validity windows |
| [C-03](context/c-03-context-store-and-graph.md) | The context store and graph | `.kapi/work/store.db`, the shared subsystem tables, the property graph and its query shapes |
| [C-04](context/c-04-unit-state-and-decisions.md) | Unit state and the decision record | `.kapi/state/`, the working set, `kapi commit`, target-hash staleness |
| [C-05](context/c-05-freshness.md) | Freshness and the composite ref | one ref per stream, compare-and-swap per component, the staleness gate |
| [C-06](context/c-06-retrieval.md) | Context retrieval | by location and by content, on the CLI and over MCP |
| [C-07](context/c-07-voice-profiles.md) | Voice profiles | the profile model, starter packs, the vocabulary and voice checks, scoring |
| [C-08](context/c-08-terms.md) | Terms | the concept model, the committed source, tiered lookup, validity |
| [C-09](context/c-09-content-memory.md) | Content memory | tiered matching, entity generalization, the two-stage rebuild |
| [C-10](context/c-10-redaction.md) | Redaction and clearance | the placeholder model, the local vault, the three policy readers |

## F — Foundations

| AD | Title | Scope |
| --- | --- | --- |
| [001](001-vision-and-modules.md) | Vision and Modules | Go modules, `go.work`, dependency boundaries |
| [002](002-content-model.md) | Content Model | Part/Resource, Block, Run, Overlay, semantic vocabulary, Layers |
| [003](003-identity.md) | Identity | Base62 IDs, dual block identity |
| [034](034-content-model-wire-schema.md) | Content-Model Wire Schema | the canonical proto, `protoconvert`, frozen field numbers |

## E — Engine

| AD | Title | Scope |
| --- | --- | --- |
| [004](004-processing-engine.md) | Processing Engine | channel-based streaming, Executor, parallel block tools, collectors |
| [005](005-format-system.md) | Format System | DataFormatReader/Writer, detection, registries, skeleton strategies |
| [006](006-tool-system.md) | Tool System | BaseTool, locale cardinality, annotations, side effects, schemas |
| [007](007-plugin-system.md) | Plugin System | manifest-driven out-of-process plugins, gRPC, presets |
| [011](011-ai-providers.md) | AI Providers | LLMProvider, streaming, batching, worker pool |
| [012](012-mt-providers.md) | MT Providers | MTProvider interface, built-in backends |
| [026](026-flow-io-binding.md) | Flow I/O Binding | source/sink bindings, process-only runs, ingest versus run transforms |
| [028](028-pdf-reader-plugin.md) | PDF Reader & Structure Tiers | native plugin plus browser WASM, geometry, structure tiers |
| [031](031-content-fidelity-surfacing.md) | Content-Fidelity Surfacing | readers surfacing non-translatable context |
| [038](038-execution-trust.md) | Execution Trust | exec-class tools and formats, per-project consent |

## S — Surfaces

| AD | Title | Scope |
| --- | --- | --- |
| [013](013-kapi-cli.md) | Kapi CLI | the standalone CLI, output formats, credential store, MCP server |
| [014](014-kapi-desktop.md) | Kapi Desktop | the Wails app, flow editor, runner, plugin manager |
| [019](019-i18n-react.md) | neokapi-i18n | the React runtime, build-time extraction, re-attach |
| [023](023-toolbox-utilities.md) | Toolbox Utilities | the busybox multi-call binary, block-text projection, exit codes |
| [024](024-agent-skills.md) | Agent Skills | embedded skill routers, install, the one write verb |
| [027](027-visual-editor-data-model.md) | Visual Editor | the render projection, shared preview kit, edit round-trip |
| [035](035-in-context-review.md) | In-Context Review | DOM stamping, write-back over a dev middleware, term and check painting |

## M — Multilingual

| AD | Title | Scope |
| --- | --- | --- |
| [016](016-metadata-i18n.md) | Metadata i18n | tool, format and plugin metadata in other languages |
| [017](017-bilingual-format-interop.md) | Bilingual Format Interop | XLIFF/PO/TMX round-trip, target alignment |
| [021](021-sat-segmenter-plugin.md) | SaT Segmenter Plugin | the ONNX segmentation model, native-stack isolation |
| [025](025-kbf-package.md) | Content Packages | the block bundle and the project parcel |
| [029](029-vision-and-image-adaptation.md) | Vision & Image Adaptation | image as an adaptable asset, OCR and layout |
| [030](030-multimodal-extraction-and-llm-refinement.md) | Multimodal Extraction | confidence-gated escalation, media anchors, provenance |
| [032](032-math-and-equations.md) | Math & Equations | the equation converter, formula blocks, translatable prose inside math |
| [036](036-llm-prompts-and-batching.md) | Prompts & Batching | the prompt library, batching, placeholder protocol |

## A — Assurance

| AD | Title | Scope |
| --- | --- | --- |
| [015](015-testing-and-documentation.md) | Testing and Documentation | the test pyramid, the docs site, screenshots, recordings |
| [018](018-parity-testing.md) | Parity Testing | the parity harness, the comparison dashboard |
