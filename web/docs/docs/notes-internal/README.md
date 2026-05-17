---
sidebar_position: 0
title: Overview
slug: index
---

# Implementation Notes — neokapi Framework

Implementation notes contain tactical details for the neokapi framework
(Apache-2.0): SQL schemas, wire protocols, algorithm pseudocode, Go interface
signatures, and other reference material. They exist alongside the
[Architecture Decisions](/architecture/index), which describe the design choices;
notes describe the implementation.

| Note                                                       | Parent AD                                      | Content                                             |
| ---------------------------------------------------------- | ---------------------------------------------- | --------------------------------------------------- |
| [Implementing Formats](implementing-formats.md)            | [AD-005](/architecture/005-format-system)      | Step-by-step guide for new format readers/writers   |
| [Skeleton Store](skeleton-store.md)                        | [AD-005](/architecture/005-format-system)      | SkeletonStore binary format, streaming HTML support |
| [Flow Steps Format](flow-steps-format.md)                  | [AD-006](/architecture/006-tool-system)        | YAML step list, fan-out, script steps               |
| [Session-Scoped Tool Authoring](session-tool-authoring.md) | [AD-006](/architecture/006-tool-system)        | Guide for writing tools against BlockStore          |
| [Plugin Bridge Protocol](plugin-bridge-protocol.md)        | [AD-007](/architecture/007-plugin-system)      | gRPC protocol, bridge descriptor                    |
| [Kapi Project File](kapi-project-file.md)                  | [AD-008](/architecture/008-project-model)      | `.kapi` recipe schema and examples                  |
| [TM Matching Algorithm](tm-matching-algorithm.md)          | [AD-009](/architecture/009-translation-memory) | Tiered matching, TMX mapping                        |
| [Terminology Data Model](terminology-data-model.md)        | [AD-010](/architecture/010-terminology)        | Go structs, TermBase interface                      |
| [MCP Tools Reference](mcp-tools-reference.md)              | [AD-013](/architecture/013-kapi-cli)           | Tool specs, input/output schemas                    |
