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
CLI base, the `kapi` binary, Kapi Desktop, and the in-repo plugins.

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

## F — Foundations

| AD | Title | Scope |
| --- | --- | --- |
| [F-01](foundations/f-01-framework-and-modules.md) | The framework and its modules | the Go modules, `go.work`, the enforced dependency direction |
| [F-02](foundations/f-02-content-model.md) | The content model | Part and Resource, Block, Run, Overlay, the semantic vocabulary, Layers |
| [F-03](foundations/f-03-identity.md) | Identity | short ids, the durable content key, occurrences |
| [F-04](foundations/f-04-wire-schema.md) | The content-model wire schema | the canonical proto, `protoconvert`, frozen field numbers, the drift guard |

## E — Engine

| AD | Title | Scope |
| --- | --- | --- |
| [E-01](engine/e-01-processing-engine.md) | The processing engine | channel-based streaming, the Executor, parallel block tools, collectors |
| [E-02](engine/e-02-format-system.md) | The format system | readers and writers, detection, registries, skeletons, non-translatable context |
| [E-03](engine/e-03-tool-system.md) | The tool system | the Tool interface, locale cardinality, annotations, side effects, schemas |
| [E-04](engine/e-04-flows-and-io-binding.md) | Flows and I/O binding | source and sink bindings, process-only runs, ingest versus run transforms |
| [E-05](engine/e-05-plugin-system.md) | The plugin system | manifest-driven out-of-process plugins, the transport modes, presets |
| [E-06](engine/e-06-execution-trust.md) | Execution trust | the exec class, per-project consent keyed to the approved argv |
| [E-07](engine/e-07-model-providers.md) | Model and translation providers | the model provider interface, machine-translation backends, credentials |
| [E-08](engine/e-08-document-structure-tiers.md) | Document structure tiers | tagged structure versus geometric reconstruction, the native and browser readers |

## C — Context

| AD | Title | Scope |
| --- | --- | --- |
| [C-01](context/c-01-project-model.md) | The project model | the `kapi.yaml` recipe, the committed `.kapi/` layout, the store interface, `ProjectContext` |
| [C-02](context/c-02-coordinates-and-governance.md) | Coordinates and governance | the product × channel space and the declared axes, per-file resolution, validity windows |
| [C-03](context/c-03-context-store-and-graph.md) | The context store and graph | `.kapi/work/store.db`, the shared subsystem tables, the property graph and its query shapes |
| [C-04](context/c-04-unit-state-and-decisions.md) | Unit state and the decision record | `.kapi/state/`, the working set, document identity, `kapi commit`, basis and target-hash staleness |
| [C-05](context/c-05-freshness.md) | Freshness and the composite ref | one ref per stream, compare-and-swap per component, the staleness gate |
| [C-06](context/c-06-retrieval.md) | Context retrieval | by location and by content, on the CLI and over MCP |
| [C-07](context/c-07-voice-profiles.md) | Voice profiles | the profile model, starter packs, the vocabulary and voice checks, scoring |
| [C-08](context/c-08-terms.md) | Terms | the concept model, the committed source, tiered lookup, validity |
| [C-09](context/c-09-content-memory.md) | Content memory | tiered matching, entity generalization, version chains and governed reuse, the two-stage rebuild |
| [C-10](context/c-10-redaction.md) | Redaction and clearance | the placeholder model, the local vault, the three policy readers |

## S — Surfaces

| AD | Title | Scope |
| --- | --- | --- |
| [S-01](surfaces/s-01-kapi-cli.md) | The kapi CLI | the command tree, output formats, the credential store, exit codes |
| [S-02](surfaces/s-02-kapi-desktop.md) | Kapi Desktop | the desktop app, the flow editor, the runner, the plugin manager |
| [S-03](surfaces/s-03-agent-surfaces.md) | Agent surfaces: MCP and skills | the embedded skill, the curated MCP surface, the one write verb |
| [S-04](surfaces/s-04-toolbox.md) | Toolbox utilities | the multi-call binary, block-text projection, exit codes |
| [S-05](surfaces/s-05-i18n-runtime.md) | The i18n runtime for React | the runtime, build-time extraction, re-attach, in-context review |
| [S-06](surfaces/s-06-visual-editor.md) | The visual editor data model | the render projection, the shared preview kit, the edit round-trip |

## M — Multilingual

| AD | Title | Scope |
| --- | --- | --- |
| [M-01](multilingual/m-01-bilingual-interop.md) | Bilingual format interop | the extract and merge round trip, target alignment, exchange carriers |
| [M-02](multilingual/m-02-segmentation.md) | Segmentation | the stand-off overlay, the engine registry, per-project selection |
| [M-03](multilingual/m-03-multimodal-content.md) | Multimodal content | image, audio and video extraction, confidence-gated escalation, provenance |
| [M-04](multilingual/m-04-math-and-equations.md) | Math and equations | the equation converter, formula blocks, translatable prose inside math |
| [M-05](multilingual/m-05-prompts-and-batching.md) | Prompts and batching | the prompt library, batching, the placeholder protocol |
| [M-06](multilingual/m-06-content-packages.md) | Content packages | the block bundle and the project parcel |
| [M-07](multilingual/m-07-metadata-i18n.md) | Metadata in other languages | tool, format and plugin metadata, compiled catalogs |

## A — Assurance

| AD | Title | Scope |
| --- | --- | --- |
| [A-01](assurance/a-01-testing-and-documentation.md) | Testing and documentation | the test pyramid, the docs site, screenshots, recordings |
| [A-02](assurance/a-02-parity.md) | Parity with the Okapi Framework | the parity harness, the comparison dashboard, faithful output |
