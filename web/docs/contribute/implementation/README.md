---
sidebar_position: 0
title: Implementation Notes Overview
slug: index
description: Index of Implementation Notes for the neokapi framework — SQL schemas, wire protocols, algorithm pseudocode, Go interface signatures, and other tactical reference material, grouped by the architecture series each note details.
keywords: [implementation notes, neokapi, reference, SQL schema, wire protocol, algorithms, Go interfaces]
---

# Implementation Notes: neokapi Framework

Implementation notes contain tactical details for the neokapi framework
(Apache-2.0): SQL schemas, wire protocols, algorithm pseudocode, Go interface
signatures, and other reference material. They exist alongside the
[Architecture Decisions](/contribute/architecture/index), which describe the
design choices; notes describe the implementation.

The notes are grouped into the same series as the decisions (**F** foundations,
**E** engine, **C** context, **S** surfaces, **M** multilingual), so a note sits
under the heading of the AD it details. A final group holds the
notes that describe this repository's own infrastructure rather than a
subsystem of the engine.

## F: Foundations

| Note                                                                        | Parent AD                                                                                                                                 | Content                                                                                                                                        |
| --------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| [Content-Model Parity Across Wire Projections](foundations/content-parity.md) | [F-04](/contribute/architecture/foundations/f-04-wire-schema) · [F-02](/contribute/architecture/foundations/f-02-content-model)              | The lossless model↔proto↔store round-trip invariant, what must round-trip, and the extend-without-breaking checklist gated by a conformance test |

## E: Engine

| Note                                                              | Parent AD                                                                                                                  | Content                                                                                                                                          |
| ------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------- |
| [Implementing Formats](engine/implementing-formats.md)             | [E-02](/contribute/architecture/engine/e-02-format-system)                                                                    | Step-by-step guide for new format readers and writers                                                                                              |
| [Skeleton store and streaming](engine/skeleton-store.md) | [E-02](/contribute/architecture/engine/e-02-format-system)                                                                    | The skeleton store, its backings and writer fallbacks, and how the whole-document readers stream through it |
| [Content-Fidelity Surfacing](engine/content-fidelity.md)           | [E-02](/contribute/architecture/engine/e-02-format-system)                                                                    | Surfacing non-translatable context: the inverted toggle, channels, parity force-off                                                                |
| [Flow Steps Format](engine/flow-steps-format.md)                   | [E-04](/contribute/architecture/engine/e-04-flows-and-io-binding) · [E-03](/contribute/architecture/engine/e-03-tool-system)  | YAML step list, fan-out, script steps                                                                                                              |
| [Session-Scoped Tool Authoring](engine/session-tool-authoring.md)  | [E-03](/contribute/architecture/engine/e-03-tool-system)                                                                      | Guide for writing tools against BlockStore                                                                                                         |
| [Plugin Model](engine/plugin-model.md)                             | [E-05](/contribute/architecture/engine/e-05-plugin-system)                                                                    | In-process registry contract for plugin binaries                                                                                                   |
| [Plugin protocol v1](engine/plugin-protocol-v1.md)                 | [E-05](/contribute/architecture/engine/e-05-plugin-system)                                                                    | The versioned plugin contract: manifest rules, the three transports, the Mode-C gRPC surface and wire format, and the conformance suite             |

## C: Context

| Note                                                                     | Parent AD                                                        | Content                                                                                        |
| -------------------------------------------------------------------------- | ------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------ |
| [kapi.yaml Project File](context/kapi-project-file.md)                    | [C-01](/contribute/architecture/context/c-01-project-model)        | `kapi.yaml` recipe schema and examples                                                           |
| [Terminology Data Model](context/terminology-data-model.md)               | [C-08](/contribute/architecture/context/c-08-terms)                | Go structs, Terminology interface                                                                |
| [Content Memory Matching Algorithm](context/memory-matching-algorithm.md) | [C-09](/contribute/architecture/context/c-09-content-memory)       | Tiered matching, version chains, TMX mapping                                                     |

## S: Surfaces

| Note                                                    | Parent AD                                                            | Content                                                                                            |
| --------------------------------------------------------- | ---------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------- |
| [CLI Conventions](surfaces/cli-conventions.md)           | [S-01](/contribute/architecture/surfaces/s-01-kapi-cli)                | Input/output/exit-code/project contracts, per-command surface table                                  |
| [MCP Tools Reference](surfaces/mcp-tools-reference.md)   | [S-03](/contribute/architecture/surfaces/s-03-agent-surfaces)          | Where the MCP tool handlers live, how a tool reaches the server, and what shape its result takes     |
| [WASM Engine ABI](surfaces/wasm-engine-abi.md)           | [S-01](/contribute/architecture/surfaces/s-01-kapi-cli)                | The JS contract the browser build of the CLI exposes to `@neokapi/engine`, and its reverse bridges   |

## M: Multilingual

| Note                                                        | Parent AD                                                                     | Content                                                                                    |
| ------------------------------------------------------------- | ------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- |
| [Multimodal Content](multilingual/multimodal-content.md)     | [M-03](/contribute/architecture/multilingual/m-03-multimodal-content)           | The two axes of adaptation, and the timing/geometry/recognition annotations that carry them  |
| [OMML Math Conversion](multilingual/omml-math.md)            | [M-04](/contribute/architecture/multilingual/m-04-math-and-equations)           | core/math Exp AST, OMML reader, nor-splice algorithm, coverage ledger                        |

## Repo infrastructure

These two describe how this repository builds and presents itself rather than
how the engine works, so they answer to no architecture decision.

| Note                                        | Content                                                                                        |
| --------------------------------------------- | ------------------------------------------------------------------------------------------------ |
| [CDN asset offloading](repo/cdn-assets.md)   | Why the large immutable docs assets are served from S3 + CloudFront, and how they are published  |
| [Markdown in the UI](repo/markdown-in-ui.md) | Which metadata fields carry markdown, and the shared `Markdown` typeset primitive that renders them |
