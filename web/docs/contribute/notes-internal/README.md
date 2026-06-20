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
| [Implementing Formats](implementing-formats.md)            | [AD-005](/contribute/architecture/005-format-system)      | Step-by-step guide for new format readers/writers   |
| [Skeleton Store](skeleton-store.md)                        | [AD-005](/contribute/architecture/005-format-system)      | SkeletonStore binary format, streaming HTML support, sub-skeleton |
| [Content-Fidelity Surfacing](content-fidelity.md)          | [AD-031](/contribute/architecture/031-content-fidelity-surfacing) | Surfacing non-translatable context: the inverted toggle, channels, parity force-off |
| [OMML Math Conversion](omml-math.md)                       | [AD-032](/contribute/architecture/032-math-and-equations) | core/math Exp AST, OMML reader, nor-splice algorithm, coverage ledger |
| [Flow Steps Format](flow-steps-format.md)                  | [AD-006](/contribute/architecture/006-tool-system)        | YAML step list, fan-out, script steps               |
| [Session-Scoped Tool Authoring](session-tool-authoring.md) | [AD-006](/contribute/architecture/006-tool-system)        | Guide for writing tools against BlockStore          |
| [Plugin Model](plugin-model.md)                            | [AD-007](/contribute/architecture/007-plugin-system)      | In-process registry contract for plugin binaries    |
| [Plugin Bridge Protocol](plugin-bridge-protocol.md)        | [AD-007](/contribute/architecture/007-plugin-system)      | gRPC protocol, bridge descriptor                    |
| [Kapi Project File](kapi-project-file.md)                  | [AD-008](/contribute/architecture/008-project-model)      | `.kapi` recipe schema and examples                  |
| [TM Matching Algorithm](tm-matching-algorithm.md)          | [AD-009](/contribute/architecture/009-translation-memory) | Tiered matching, TMX mapping                        |
| [Terminology Data Model](terminology-data-model.md)        | [AD-010](/contribute/architecture/010-terminology)        | Go structs, TermBase interface                      |
| [MCP Tools Reference](mcp-tools-reference.md)              | [AD-013](/contribute/architecture/013-kapi-cli)           | Tool specs, input/output schemas                    |
| [Tool & Data Model Rationale](tool-data-model-redesign.md) | [AD-006](/contribute/architecture/006-tool-system) · [AD-002](/contribute/architecture/002-content-model) | Why stand-off overlays + annotations, a typed consumes/produces IO contract, a uniform unit iterator, and typed source/sink bindings |
