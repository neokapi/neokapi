---
sidebar_position: 0
title: Overview
slug: index
---

# Implementation Notes — Bowrain Platform

Implementation notes contain tactical details for the bowrain platform
(AGPL-3.0): SQL schemas, wire protocols, algorithm pseudocode, Go interface
signatures, and other reference material. They exist alongside the
[Architecture Decisions](/bowrain/architecture-decisions/index), which describe
the design choices; notes describe the implementation.

Framework-level notes (formats, plugin bridge, TM matching, etc.) live in the
framework's [Implementation Notes](/docs/notes/index).

| Note                                                      | Parent AD                                                 | Content                                                     |
| --------------------------------------------------------- | --------------------------------------------------------- | ----------------------------------------------------------- |
| [Content Store Schema](content-store-schema.md)           | [AD-004](../architecture-decisions/004-content-store)     | PostgreSQL schema, migrations                               |
| [Connector Interfaces](connector-interfaces.md)           | [AD-008](../architecture-decisions/008-connector-system)  | IntegrationConnector signatures                             |
| [Sync Protocol](sync-protocol.md)                         | [AD-009](../architecture-decisions/009-sync-protocol)     | Wire format, Merkle negotiation, chunking                   |
| [Graph Store Schema](graph-store-schema.md)               | [AD-006](../architecture-decisions/006-graph-concept-storage) | Table layout, Cypher mapping                           |
| [Media Asset Storage](media-asset-storage.md)             | [AD-007](../architecture-decisions/007-media-and-blob-storage) | Upload flows, variant pipeline                       |
| [Automation Run Visibility](automation-run-visibility.md) | [AD-013](../architecture-decisions/013-automation-engine) | Run/step/log model, SSE, REST API                           |
| [Translator Workflow](translator-workflow.md)             | [AD-014](../architecture-decisions/014-translator-workflow) | Task fan-out, source review, MCP tools                    |
| [Translation Job Queue](translation-job-queue.md)         | [AD-015](../architecture-decisions/015-server-ai-operations) | Job model, worker algorithm, quota schema               |
| [Brand Voice Data Model](brand-voice-data-model.md)       | [AD-015](../architecture-decisions/015-server-ai-operations) | VoiceProfile, scoring dimensions                        |
| [Entity & Term Extraction](entity-term-extraction.md)     | [AD-015](../architecture-decisions/015-server-ai-operations) | NER pipeline, review queue schema                       |
| [Bravo Agent Implementation](bravo-agent-implementation.md) | [AD-016](../architecture-decisions/016-bravo-agent)     | ZeroClaw integration, MCP cloud tools                     |
| [CLI Commands Reference](cli-commands-reference.md)       | [AD-010](../architecture-decisions/010-bowrain-cli-and-project-model) | Command tree, REST routes                   |
| [Admin Control Plane](admin-control-plane.md)             | [AD-017](../architecture-decisions/017-bowrain-apps)      | Realm separation, impersonation audit                       |
