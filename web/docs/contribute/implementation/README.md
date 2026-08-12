---
sidebar_position: 0
title: Implementation Notes Overview
slug: index
description: Index of Implementation Notes for the neokapi framework — SQL schemas, wire protocols, algorithm pseudocode, Go interface signatures, and other tactical reference material alongside the Architecture Decisions.
keywords: [implementation notes, neokapi, reference, SQL schema, wire protocol, algorithms, Go interfaces]
---

# Implementation Notes — neokapi Framework

Implementation notes contain tactical details for the neokapi framework
(Apache-2.0): SQL schemas, wire protocols, algorithm pseudocode, Go interface
signatures, and other reference material. They exist alongside the
[Architecture Decisions](/contribute/architecture/index), which describe the design choices;
notes describe the implementation.

| Note                                                       | Parent AD                                      | Content                                             |
| ---------------------------------------------------------- | ---------------------------------------------- | --------------------------------------------------- |
| [Implementing Formats](implementing-formats.md)            | [E-02](/contribute/architecture/engine/e-02-format-system)      | Step-by-step guide for new format readers/writers   |
| [Skeleton Store](skeleton-store.md)                        | [E-02](/contribute/architecture/engine/e-02-format-system)      | SkeletonStore binary format, streaming HTML support, sub-skeleton |
| [Content-Fidelity Surfacing](content-fidelity.md)          | [E-02](/contribute/architecture/engine/e-02-format-system) | Surfacing non-translatable context: the inverted toggle, channels, parity force-off |
| [OMML Math Conversion](omml-math.md)                       | [M-04](/contribute/architecture/multilingual/m-04-math-and-equations) | core/math Exp AST, OMML reader, nor-splice algorithm, coverage ledger |
| [Flow Steps Format](flow-steps-format.md)                  | [E-03](/contribute/architecture/engine/e-03-tool-system)        | YAML step list, fan-out, script steps               |
| [Session-Scoped Tool Authoring](session-tool-authoring.md) | [E-03](/contribute/architecture/engine/e-03-tool-system)        | Guide for writing tools against BlockStore          |
| [Plugin Model](plugin-model.md)                            | [E-05](/contribute/architecture/engine/e-05-plugin-system)      | In-process registry contract for plugin binaries    |
| [Plugin protocol v1](plugin-protocol-v1.md)                | [E-05](/contribute/architecture/engine/e-05-plugin-system)      | The versioned plugin contract: manifest rules, the three transports, the Mode-C gRPC surface and wire format, and the conformance suite |
| [Kapi Project File](kapi-project-file.md)                  | [C-01](/contribute/architecture/context/c-01-project-model)      | `kapi.yaml` recipe schema and examples              |
| [Content Memory Matching Algorithm](memory-matching-algorithm.md) | [C-09](/contribute/architecture/context/c-09-content-memory) | Tiered matching, TMX mapping                        |
| [Terminology Data Model](terminology-data-model.md)        | [C-08](/contribute/architecture/context/c-08-terms)        | Go structs, Terminology interface                      |
| [MCP Tools Reference](mcp-tools-reference.md)              | [S-01](/contribute/architecture/surfaces/s-01-kapi-cli)           | Tool specs, input/output schemas                    |
| [CLI Conventions](cli-conventions.md)                      | [S-01](/contribute/architecture/surfaces/s-01-kapi-cli)           | Input/output/exit-code/project contracts, per-command surface table |
| [Tool & Data Model Rationale](tool-data-model-redesign.md) | [E-03](/contribute/architecture/engine/e-03-tool-system) · [F-02](/contribute/architecture/foundations/f-02-content-model) | Why stand-off overlays + annotations, a typed consumes/produces IO contract, a uniform unit iterator, and typed source/sink bindings |
| [Markdown in the UI](markdown-in-ui.md)                    | [E-03](/contribute/architecture/engine/e-03-tool-system) · [E-02](/contribute/architecture/engine/e-02-format-system) | Which metadata fields carry markdown, and the shared `Markdown` typeset primitive that renders them |
| [Content-Model Parity Over the Sync Wire](content-parity.md) | [F-04](/contribute/architecture/foundations/f-04-wire-schema) · [F-02](/contribute/architecture/foundations/f-02-content-model) | The lossless model↔proto↔store round-trip invariant, what must round-trip, and the extend-without-breaking checklist gated by the kitchen-sink conformance test |
