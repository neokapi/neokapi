---
id: 005-connector-system
sidebar_position: 5
title: "AD-005: Connector System"
---
# AD-005: Connector System

## Context

Traditional localization tools treat file formats as the primary integration mechanism. You export content from a CMS to XLIFF, translate it, and import it back. This workflow is brittle, manual, and disconnected from the source system. Changes in the CMS require re-export; translations sit in files until someone remembers to re-import. There is no live connection between the content source and the translation environment.

Connectors to native tools — pulling data directly into a versioned store — create a fundamentally better workflow than file exchange. Instead of exporting and importing, users connect their tools and data flows bidirectionally through a unified platform.

**This AD establishes the role separation:**
- **Kapi** is the **file connector** — it handles local file processing and syncs files with Bowrain Server
- **Bowrain Server** hosts **integration connectors** — CMS, design tools, code repositories, marketing platforms

File formats remain important — Okapi's 40+ filters represent years of engineering. But they are **Kapi's domain**, not the entire connector story. Bowrain Server orchestrates integrations with external systems; Kapi is one such integration (the file-based one).

## Decision

### Connector Architecture

The connector system has two layers:

**1. Bowrain Server Connectors** — Server-side integrations with external systems:
- **CMS Connectors** (Contentful, Strapi, WordPress)
- **Design Connectors** (Figma, Sketch)
- **Code Connectors** (Git repositories, i18n resource bundles)
- **Marketing Connectors** (HubSpot, Marketo)

**2. Kapi as the File Connector** — CLI tool that syncs local files with Bowrain Server:
- Reads/writes files via FormatRegistry (15+ native formats, plugins, Okapi bridge)
- Maps local paths to remote project items
- Syncs with server via REST API (`pull`/`push`)
- Runs file-based processing flows

```
┌─────────────────────────────────────────────────┐
│          Bowrain Server (Platform)              │
│  ┌──────────────────────────────────────────┐   │
│  │         ContentStore (AD-003)           │   │
│  └──────────────────────────────────────────┘   │
│    ▲        ▲         ▲         ▲         ▲     │
│    │        │         │         │         │     │
│  CMS    Design    Code     Marketing   File    │
│  Conn.   Conn.    Conn.     Conn.     (API)    │
└────┼────────┼─────────┼─────────┼─────────┼─────┘
     │        │         │         │         │
     │        │         │         │         │ REST API
     ▼        ▼         ▼         ▼         ▼
Contentful Figma     GitHub   HubSpot    Kapi
   CMS      Design     Repo    Marketing  (.kapi/ projects)
```

### Connector Interfaces

The connector system uses two distinct interfaces: `IntegrationConnector` (server-side, Fetch/Publish terminology) and `SourceConnector` (client-side, Push/Pull terminology), sharing a common `ConnectorBase` for identity and lifecycle. The terminology split resolves the ambiguity between "push from Kapi to server" vs. "push translations to WordPress."

See [Connector Interfaces](/docs/notes/connector-interfaces) for Go struct definitions, method signatures, and ConnectorRegistry code.

### Connector Categories

Six connector categories are defined: CMS, Design, Code, Marketing, File (Kapi), and TMS. Server-side connectors include CMS (Contentful, Strapi, WordPress), Design (Figma, Sketch), Code (Git repositories), and Marketing (HubSpot, Marketo).

See [Connector Interfaces](/docs/notes/connector-interfaces) for the full `ConnectorCategory` enum and `ContentItem` struct definitions.

**Kapi as the file connector:**

- Kapi is **not a server-side connector**. It is a CLI tool that acts as the file connector for Bowrain Server.
- Kapi operates on local file systems with `.kapi/` project directories ([AD-016](./016-kapi-project-model.md))
- `kapi pull/push` syncs local files with Bowrain Server via REST API
- Kapi uses the FormatRegistry to read/write files (15+ native formats + plugins + Okapi bridge)

### Kapi: The File Connector

Kapi's role in the connector ecosystem:

**What Kapi does:**
- Reads local files via FormatRegistry (HTML, JSON, XLIFF, Markdown, etc.)
- Extracts Blocks from file content (streaming Parts → Blocks)
- Computes content hashes (`BlockIdentity` from [AD-002](./002-content-model.md))
- Syncs with Bowrain Server via REST API (`/api/v1/workspaces/:ws/projects/:id/pull|push`)
- Writes remote blocks back to local files via FormatRegistry
- Runs file-based flows (pseudo-translate, QA, segmentation, etc.)

**What Kapi doesn't do:**
- Kapi does **not** manage server-side connectors (no `kapi connect add contentful`)
- Kapi does **not** access the ContentStore directly (it's a REST API client)
- Kapi does **not** run server-side automation or event-driven workflows

**Architecture:**

```
.kapi/ Project Directory
     |
     v
  Kapi CLI (reads config.yaml, mappings)
     |
     v
  FormatRegistry (HTML, JSON, XLIFF, etc.)
     |
     v
  Streaming Pipeline (Parts → Blocks)
     |
     v
  REST API Client
     |
     v
  Bowrain Server (/api/v1/.../pull, /api/v1/.../push)
     |
     v
  ContentStore
```

Kapi is to Bowrain Server as **git is to GitHub** — a local tool that syncs with a remote platform.

### Format System Integration

Kapi inherits the **three-tier format system** from [AD-004](./004-processing-engine.md):

1. **Native formats** (Go): 15 built-in — HTML, XML, XLIFF, XLIFF 2, JSON, YAML, PO, Properties, Plaintext, Markdown, CSV, SRT, VTT, TMX
2. **Plugin formats** (any language): External executables via gRPC ([AD-007](./007-plugin-system.md))
3. **Bridge formats** (Okapi): Subprocess-hosted filters via gRPC bridge protocol ([AD-007](./007-plugin-system.md))

All three tiers register into the `FormatRegistry`. Kapi uses this registry to read/write files based on `.kapi/config.yaml` mappings.

**Format detection cascade:**
1. **MIME type** — explicit type declaration
2. **File extension** — `.html`, `.xliff`, `.json`, etc.
3. **Magic bytes** — binary signatures (BOM, XML declaration)
4. **Content sniffing** — heuristic analysis of file content

**Skeleton strategies:**
- **SkeletonStore streaming** (HTML): Temp-file-backed binary store where the
  reader writes non-translatable bytes and block references during extraction;
  the writer reads entries sequentially to reconstruct the document with
  byte-exact fidelity. See [Skeleton Store](/docs/notes/skeleton-store) for
  the binary format, interfaces, and wiring details.
- **Re-parse** (JSON, YAML, PO, Plaintext): Re-open source document and
  replace content in place during writing.
- **Fragment-based** (XML, XLIFF): Interleaved skeleton of non-translatable
  markup + references to translatable blocks carried on the Block/Data
  resources.

These strategies ensure roundtrip fidelity when Kapi writes translated files
back to disk. The SkeletonStore approach is preferred for new formats because
it produces byte-exact output with ~100KB peak memory per document, compared
to the re-parse approach which requires holding the full document in memory
twice (once for parsing, once for writing).

### Server-Side Connector Registration

Server-side connectors register into a `ConnectorRegistry` via factory functions. Built-in connectors register via `init()`, plugin connectors register at runtime via gRPC discovery. Bowrain Server manages connectors through its admin UI (add instances, browse content, trigger sync, view status). Kapi does not interact with the ConnectorRegistry.

See [Connector Interfaces](/docs/notes/connector-interfaces) for PullOptions, PushOptions, SyncStatus, ContentItem structs, and ConnectorRegistry code.

## Alternatives Considered

- **File-only integration** (traditional Okapi approach): Export to XLIFF, translate, re-import. Works for batch workflows but is manual, disconnected, and provides no live sync. Changes in the source system go undetected until the next export cycle.

- **Kapi as a server-side connector framework**: Would require Kapi to run as a daemon, manage connector lifecycles, and coordinate with the server. This overcomplicates Kapi's role. Kapi is a CLI tool for files; the server orchestrates connectors.

- **All connectors in Kapi**: Would require Kapi to have API credentials for CMS, design tools, etc. This mixes concerns — Kapi is for local file work, not managing integrations. Connectors belong server-side where they can be shared across teams and managed centrally.

- **XLIFF as universal exchange format**: XLIFF is a format, not an integration mechanism. It standardizes the file representation of translatable content but says nothing about how content gets from a CMS to a translation tool and back. Connectors are richer — they handle discovery, change tracking, and bidirectional sync.

## Consequences

- **Clear role separation**: Kapi handles files, Bowrain Server handles integrations.

- **Kapi is the file connector** for Bowrain Server — it reads/writes local files and syncs with the server via REST API. It does not manage server-side connectors.

- Server-side connectors (CMS, design, code, marketing) live in Bowrain Server and write extracted content into the ContentStore ([AD-003](./003-content-store.md)).

- The FormatRegistry (15+ native formats, plugins, Okapi bridge) is Kapi's domain. All file processing goes through Kapi's format system.

- `.kapi/` project directories ([AD-016](./016-kapi-project-model.md)) define file mappings (local paths ↔ remote items), enabling `kapi pull/push` to sync with the server.

- Connectors are the primary integration mechanism for Bowrain Server, positioning it as a localization platform rather than a file processing tool.

- Content from any connector flows into the same ContentStore and streaming pipeline ([AD-004](./004-processing-engine.md)), processed by the same tools ([AD-006](./006-tool-system.md)) regardless of origin.

- The `ConnectorRegistry` parallels the `FormatRegistry`, maintaining the established pattern of factory-based registration with runtime discovery.

- Kapi does not expose connector management commands (`kapi connect add/list`) — these belong in Bowrain Server's admin UI or API.

- The connector interface uses streaming Parts, the same unit used throughout the pipeline ([AD-002](./002-content-model.md)). This means any connector's output feeds directly into tools, TM, terminology, and AI processing without adaptation layers.

- Format detection, skeleton strategies, and reader/writer separation are preserved as Kapi internals, invisible to the server and other connectors.
