---
sidebar_position: 0
title: Overview
slug: index
---

# Architecture Decisions — Bowrain Platform

This directory contains the Architecture Decisions for the **bowrain platform**
— the hosted, collaborative, SaaS localization platform built on neokapi.
All content here is AGPL-3.0 licensed.

Bowrain builds on the neokapi framework. Framework-level decisions
(content model, processing pipeline, tools, plugins, AI/MT provider interfaces,
the kapi project model) live in the framework's
[Architecture Decisions](https://neokapi.github.io/web/neokapi/docs/architecture/index). This directory describes
bowrain-specific choices: the content store, sync protocol, multi-tenant auth,
connectors, event bus, server-side AI operations, agent runtime, apps, and
billing.

Tactical implementation details (SQL schemas, algorithm pseudocode, wire
protocols specific to bowrain) live in [Implementation Notes](/notes/index).

## Foundation

| AD                                              | Title                            | Scope                                                                    |
| ----------------------------------------------- | -------------------------------- | ------------------------------------------------------------------------ |
| [001](001-vision-and-modules.md)                | Vision and Module Architecture   | Bowrain identity, AGPL-3.0 boundary, three-module layout, apps inventory |
| [002](002-authentication-and-workspaces.md)     | Authentication and Workspaces    | OIDC federation, PKCE, device flow, workspace multi-tenancy              |
| [003](003-permissions.md)                       | Permissions and Access Control   | Capability envelope model, token scopes, session grants                  |

## Data Layer

| AD                                               | Title                   | Scope                                                    |
| ------------------------------------------------ | ----------------------- | -------------------------------------------------------- |
| [004](004-content-store.md)                      | Content Store           | PostgreSQL-backed versioned block store                   |
| [005](005-streams.md)                            | Streams                 | Git-like branching at the content layer                   |
| [006](006-graph-concept-storage.md)              | Graph Concept Storage   | Apache AGE / SQLite graph backends                        |
| [007](007-media-and-blob-storage.md)             | Media and Blob Storage  | Content-addressed BlobStore, asset metadata               |

## Connectivity

| AD                                        | Title            | Scope                                                  |
| ----------------------------------------- | ---------------- | ------------------------------------------------------ |
| [008](008-connector-system.md)            | Connector System | IntegrationConnector, categories, registry            |
| [009](009-sync-protocol.md)               | Sync Protocol    | Chunked, resumable, direct-to-storage                  |
| [010](010-bowrain-cli-and-project-model.md) | Bowrain CLI      | `.kapi` projects with `server:` block, hooks, MCP     |
| [011](011-rest-api.md)                    | REST API         | Slug-based hierarchy, route taxonomy                   |

## Events & Automation

| AD                                             | Title                    | Scope                                                     |
| ---------------------------------------------- | ------------------------ | --------------------------------------------------------- |
| [012](012-distributed-event-bus.md)            | Distributed Event Bus    | Azure Service Bus (prod), NATS JetStream (dev)            |
| [013](013-automation-engine.md)                | Automation Engine        | Rules, quality gates, run visibility, SSE                 |
| [014](014-translator-workflow.md)              | Translator Workflow      | Tasks, activities, notifications, source review           |

## Intelligence

| AD                                      | Title                         | Scope                                                                |
| --------------------------------------- | ----------------------------- | -------------------------------------------------------------------- |
| [015](015-server-ai-operations.md)      | Server-Side AI Operations     | Translation jobs, entity extraction, brand voice governance          |
| [016](016-bravo-agent.md)               | Bravo Agent                   | ZeroClaw runtime, scoped tokens, identity delegation, MCP cloud      |

## Applications

| AD                                    | Title         | Scope                                                              |
| ------------------------------------- | ------------- | ------------------------------------------------------------------ |
| [017](017-bowrain-apps.md)            | Bowrain Apps  | Desktop, collaborative editor, Pulse, Admin Control Plane          |
| [018](018-billing-and-plans.md)       | Billing       | Stripe plans, weekly credits, feature matrix, middleware guards    |

## Governance

| AD                                          | Title                          | Scope                                                                       |
| ------------------------------------------- | ------------------------------ | --------------------------------------------------------------------------- |
| [019](019-correction-learning-loop.md)      | The correction-learning loop   | Corrections promoted into versioned regression-test checks                  |
| [020](020-governance-audit-rollback.md)     | Governance, audit, and rollback | Groups, deny rules, role overrides, status ABAC, separation of duties, tamper-evident audit log, rollback |
