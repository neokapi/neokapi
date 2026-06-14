---
sidebar_position: 0
title: Architecture Decisions Overview
slug: index
description: Index of Architecture Decisions for the neokapi open-source localization framework — covering the content model, processing engine, format and tool systems, plugin architecture, TM, terminology, AI providers, and more.
keywords: [architecture decisions, neokapi, framework design, content model, plugin system, localization architecture]
---

# Architecture Decisions — neokapi Framework

This directory contains the Architecture Decisions for the **neokapi framework**
— the open localization engine. All content here is Apache-2.0 licensed and
describes modules at the repository root (`github.com/neokapi/neokapi`), the
shared CLI base (`github.com/neokapi/neokapi/cli`), the kapi CLI
(`github.com/neokapi/neokapi/kapi`), and Kapi Desktop
(`github.com/neokapi/neokapi/kapi-desktop`).

Tactical implementation details (SQL schemas, wire protocols, algorithm
pseudocode) are separated into [Implementation Notes](/contribute/notes-internal/index).

## Foundation

| AD                                 | Title                | Scope                                                             |
| ---------------------------------- | -------------------- | ----------------------------------------------------------------- |
| [001](001-vision-and-modules.md)   | Vision and Modules   | Go modules, `go.work`, dependency boundaries, license gradient    |
| [002](002-content-model.md)        | Content Model        | Part/Resource, Block, Run, Overlay, semantic vocabulary, Layers   |
| [003](003-identity.md)             | Identity             | Base62 IDs, dual block identity                                   |

## Processing

| AD                              | Title             | Scope                                                               |
| ------------------------------- | ----------------- | ------------------------------------------------------------------- |
| [004](004-processing-engine.md) | Processing Engine | Channel-based streaming, Executor, parallel block tools, collectors |
| [005](005-format-system.md)     | Format System     | DataFormatReader/Writer, detection, registries, skeleton strategies |
| [006](006-tool-system.md)       | Tool System       | BaseTool, locale cardinality, annotations, side effects, schemas    |
| [007](007-plugin-system.md)     | Plugin System     | manifest-driven out-of-process plugins, gRPC, presets, Okapi bridge |
| [021](021-sat-segmenter-plugin.md) | SaT Segmenter Plugin | in-process ONNX SaT model, stdin/stdout protocol, native-stack isolation, `sat` engine |
| [026](026-flow-io-binding.md)   | Flow I/O Binding  | source/sink bindings, file·store·klz·import/export, process-only runs, ingest vs run transforms |

## Project Model

| AD                          | Title         | Scope                                                                |
| --------------------------- | ------------- | -------------------------------------------------------------------- |
| [008](008-project-model.md) | Project Model | `.kapi` recipe, `.kapi/` state, BlockStore interface, ProjectContext |

## Intelligence

| AD                               | Title              | Scope                                                         |
| -------------------------------- | ------------------ | ------------------------------------------------------------- |
| [009](009-translation-memory.md) | Translation Memory | Sievepen, tiered matching, generalized matching with entities |
| [010](010-terminology.md)        | Terminology        | Concept model, TermBase, tiered lookup                        |
| [011](011-ai-providers.md)       | AI Providers       | LLMProvider, streaming, batching, worker pool                 |
| [012](012-mt-providers.md)       | MT Providers       | MTProvider interface, built-in backends                       |
| [022](022-brand-voice.md)        | Brand Voice        | VoiceProfile, starter packs, vocab/voice checks, scoring, command + MCP surface |

## Applications

| AD                         | Title        | Scope                                                        |
| -------------------------- | ------------ | ------------------------------------------------------------ |
| [013](013-kapi-cli.md)     | Kapi CLI     | Standalone CLI, output formats, credential store, MCP server |
| [014](014-kapi-desktop.md) | Kapi Desktop | Wails v3 app, flow editor, runner, plugin manager            |
| [019](019-kapi-react.md)   | Kapi React   | React i18n runtime, build-time extraction, `__tx` re-attach  |
| [023](023-toolbox-utilities.md) | Toolbox Utilities | kcat/kgrep/ksed busybox multi-call, block-text projection, exit codes |
| [024](024-agent-skills.md) | Agent Skills | embedded SKILL.md routers, `.claude/skills` install, kapi-*/bowrain-* split |
| [027](027-visual-editor-data-model.md) | Visual Editor | ContentTree→RenderDoc projection, vocabulary/overlay rendering, shared preview kit, edit→commit round-trip |

## Cross-Cutting

| AD                                      | Title                     | Scope                                                   |
| --------------------------------------- | ------------------------- | ------------------------------------------------------- |
| [015](015-testing-and-documentation.md) | Testing and Documentation | Test pyramid, Docusaurus, screenshots, recordings       |
| [016](016-metadata-i18n.md)             | Metadata i18n             | Tool/format/plugin metadata translation via MO catalogs |
| [017](017-bilingual-format-interop.md)  | Bilingual Format Interop  | XLIFF/PO/TMX bilingual round-trip, target alignment     |
| [018](018-parity-testing.md)            | Parity Testing            | Okapi parity harness, test-comparison dashboard, faithful% |
| [020](020-redaction.md)                 | Content Redaction         | Placeholder model, local vault, rule/entity detection, secure-translate, extract/merge |
