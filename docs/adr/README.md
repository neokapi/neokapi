---
sidebar_position: 0
title: Overview
slug: index
---
# Architecture Decision Records

This directory contains Architecture Decision Records (ADRs) for gokapi —
the open localization platform. Each ADR documents a significant architectural
choice, the context behind it, alternatives considered, and the consequences.
ADRs are organized by architectural layer rather than chronologically.

## Layer 1: Identity and Data

| ADR | Title | Scope |
|-----|-------|-------|
| [001](001-vision.md) | Vision — The Open Localization Platform | Platform identity, principles, configuration, locale handling |
| [002](002-content-model.md) | Content Model | Part/Resource, Block identity, Fragments, Annotations, Properties |
| [003](003-content-store.md) | Content Store and Versioning | Versioned persistence, content addressing, diffing, KAZ serialization |

## Layer 2: Processing and Integration

| ADR | Title | Scope |
|-----|-------|-------|
| [004](004-processing-engine.md) | Processing Engine | Channel-based streaming, FlowExecutor, flow definitions |
| [005](005-connector-system.md) | Connector System | Bidirectional connectors, FileConnector, format system |

## Layer 3: Extension Mechanisms

| ADR | Title | Scope |
|-----|-------|-------|
| [006](006-tool-system.md) | Tool System | BaseTool dispatch, categories, built-in tools |
| [007](007-plugin-system.md) | Plugin System and Okapi Bridge | go-plugin, gRPC, Java bridge, plugin governance |

## Layer 4: Domain Intelligence

| ADR | Title | Scope |
|-----|-------|-------|
| [008](008-ai-integration.md) | AI Integration | LLM providers, AI tools, worker pool, batching |
| [009](009-translation-memory.md) | Translation Memory | Bowrain Memory content-aware TM, tiered matching, entity adaptation |
| [010](010-terminology.md) | Terminology and Brand Management | Concept model, pipeline tools, streams, brand voice |

## Layer 5: Automation

| ADR | Title | Scope |
|-----|-------|-------|
| [011](011-automation.md) | Automation and Event System | Events, triggers, quality gates, continuous sync |

## Layer 6: Applications and Operations

| ADR | Title | Scope |
|-----|-------|-------|
| [012](012-bowrain.md) | Bowrain Desktop App | Wails v3, connector-driven UX, translation editor |
| [013](013-cli-and-server.md) | Kapi CLI and Server | Cobra CLI, REST/gRPC server, CI/CD patterns |
| [014](014-testing-and-docs.md) | Testing, Documentation, and Website | Test pyramid, Docusaurus, VHS/Playwright demos |
