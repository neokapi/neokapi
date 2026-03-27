---
sidebar_position: 0
title: Overview
slug: index
---
# Architecture Decisions

This directory contains Architecture Decisions (ADs) for neokapi —
the open localization platform. Each AD documents a significant architectural
choice, the context behind it, alternatives considered, and the consequences.
ADs are organized by architectural layer rather than chronologically.

Tactical implementation details (SQL schemas, API routes, algorithm pseudocode)
are separated into [Implementation Notes](/docs/notes/index).

## Layer 1: Identity and Data

| AD | Title | Scope |
|----|-------|-------|
| [001](001-vision.md) | Vision — The Open Localization Platform | Platform identity, principles, configuration, locale handling |
| [002](002-content-model.md) | Content Model | Part/Resource, Block identity, Fragments, Spans, semantic type vocabulary, Layers, subfiltering |
| [003](003-content-store.md) | Content Store and Versioning | Server-side versioned persistence, content addressing, KAZ snapshots |
| [016](016-kapi-project-model.md) | Bowrain Project Model | `.bowrain/` directories, file mappings, sync state, local automation |

## Layer 2: Processing and Integration

| AD | Title | Scope |
|----|-------|-------|
| [004](004-processing-engine.md) | Processing Engine | Channel-based streaming, FlowExecutor, flow definitions |
| [005](005-connector-system.md) | Connector System | Bidirectional connectors, FileConnector, format system |

## Layer 3: Extension Mechanisms

| AD | Title | Scope |
|----|-------|-------|
| [006](006-tool-system.md) | Tool System | BaseTool dispatch, categories, built-in tools |
| [007](007-plugin-system.md) | Plugin System and Okapi Bridge | go-plugin, gRPC, bridge protocol, plugin governance |

## Layer 4: Domain Intelligence

| AD | Title | Scope |
|----|-------|-------|
| [008](008-ai-integration.md) | AI Integration | LLM providers, AI tools, worker pool, batching |
| [009](009-translation-memory.md) | Translation Memory | Sievepen content-aware TM, tiered matching, entity adaptation |
| [010](010-terminology.md) | Terminology and Brand Management | Concept model, pipeline tools, streams, brand voice |
| [019](019-mt-providers.md) | Machine Translation Providers | MTProvider interface, provider registry, MTTranslateTool |

## Layer 5: Automation

| AD | Title | Scope |
|----|-------|-------|
| [011](011-automation.md) | Automation and Event System | Events, triggers, quality gates, continuous sync |
| [034](034-translator-workflow.md) | Translator Workflow | Push completion tracking, task fan-out, source review gate, agent integration |
| [035](035-automation-run-visibility.md) | Automation Run Visibility | GitHub Actions-style run/step/log model, real-time progress, SSE streaming |
| [036](036-distributed-event-bus.md) | Distributed Event Bus | ASB topics (Azure), NATS JetStream (local), replaces leader election |
| [037](037-async-content-ingestion.md) | Async Content Ingestion | Blob upload then job processing, bulk INSERT, rate limiting, pagination |

## Layer 6: Applications and Operations

| AD | Title | Scope |
|----|-------|-------|
| [012](012-bowrain.md) | Bowrain Desktop App | Wails v3, connector-driven UX, translation editor |
| [013](013-cli-and-server.md) | Bowrain CLI and Server | Bowrain CLI project commands, pull/push sync, REST/gRPC APIs |
| [014](014-testing-and-docs.md) | Testing, Documentation, and Website | Test pyramid, Docusaurus, VHS/Playwright demos |
| [015](015-auth-and-workspaces.md) | Authentication and Workspaces | OAuth/OIDC, PKCE, HttpOnly cookies, multi-tenancy |
| [017](017-cli-output-format.md) | CLI Output Format Flags | JSON/YAML/table/text output, machine-readable piping |
| [018](018-four-module-architecture.md) | Four-Module Monorepo Architecture | framework/platform/kapi/bowrain module separation |
| [020](020-collaborative-editor.md) | Collaborative Editor | EditorService gRPC, streaming presence, offline-first desktop |
| [021](021-mcp-integration.md) | MCP Integration | MCP servers for kapi and bowrain, tool specifications, stdio transport |
| [022](022-entity-term-extraction.md) | Entity & Term Extraction | Hybrid LLM+NER extraction, review queue, mobile companion app |
| [023](023-identity-system.md) | Identity System | Short base62 IDs, dual block identity (internal + source) |
