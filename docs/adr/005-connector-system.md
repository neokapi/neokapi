---
id: 005-connector-system
sidebar_position: 5
title: "ADR-005: Connector System"
---
# ADR-005: Connector System

## Context

Traditional localization tools treat file formats as the primary integration mechanism. You export content from a CMS to XLIFF, translate it, and import it back. This workflow is brittle, manual, and disconnected from the source system. Changes in the CMS require re-export; translations sit in files until someone remembers to re-import. There is no live connection between the content source and the translation environment.

Connectors to native tools — pulling data directly into a versioned store — create a fundamentally better workflow than file exchange. Instead of exporting and importing, users connect their tools and data flows bidirectionally through a unified platform.

**This ADR establishes the role separation:**
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
│  │         ContentStore (ADR-003)           │   │
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

### Connector Interface (Server-Side)

Every **Bowrain Server connector** implements a common interface:

```go
type Connector interface {
    // Identity
    ID() string
    Name() string
    Category() ConnectorCategory

    // Content operations
    Pull(ctx context.Context, opts PullOptions) (<-chan *model.Part, error)
    Push(ctx context.Context, parts <-chan *model.Part, opts PushOptions) error

    // Discovery
    List(ctx context.Context, opts ListOptions) ([]ContentItem, error)

    // Status
    Sync(ctx context.Context) (*SyncStatus, error)

    // Lifecycle
    Configure(config map[string]any) error
    Close() error
}
```

**Key methods:**
- **`Pull`** — Returns a channel of Parts (streaming content extraction)
- **`Push`** — Consumes a Part channel (streaming content delivery)
- **`List`** — Browsable discovery of content in the connected system
- **`Sync`** — Lightweight status check (what changed, pending push)

These connectors live **server-side** and write extracted content into the ContentStore ([ADR-003](./003-content-store.md)).

### Connector Categories

```go
type ConnectorCategory string

const (
    CategoryCMS       ConnectorCategory = "cms"
    CategoryDesign    ConnectorCategory = "design"
    CategoryCode      ConnectorCategory = "code"
    CategoryMarketing ConnectorCategory = "marketing"
    CategoryFile      ConnectorCategory = "file"  // Kapi's category
    CategoryTMS       ConnectorCategory = "tms"   // External TMS integrations
)
```

**Server-side connectors:**

- **CMS Connectors** — Pull content from headless CMS APIs (Contentful, Strapi, WordPress REST API). Content items map to CMS entries and pages. Each field becomes a Block. Translations push back via the same API, updating the localized version of each entry.

- **Design Connectors** — Pull text layers from design tools (Figma API, Sketch files). Each text layer becomes a Block with display hints encoding visual context: font size, position, bounding box dimensions. Translated text pushes back to create localized design variants.

- **Code Connectors** — Pull i18n resource files from Git repositories (JSON, YAML, Properties, PO files in a repo). Unlike Kapi (which operates on local files), code connectors understand repository structure: they discover resource bundles, track locale variants, and detect changes via Git commits. Push creates pull requests or commits with translated files.

- **Marketing Connectors** — Pull email templates, landing pages, and ad copy from marketing platforms (HubSpot, Marketo). Display hints carry platform constraints (subject line length limits, preview text requirements). Push delivers localized variants ready for multi-language campaigns.

**Kapi as the file connector:**

- Kapi is **not a server-side connector**. It is a CLI tool that acts as the file connector for Bowrain Server.
- Kapi operates on local file systems with `.kapi/` project directories ([ADR-016](./016-kapi-project-model.md))
- `kapi pull/push` syncs local files with Bowrain Server via REST API
- Kapi uses the FormatRegistry to read/write files (15+ native formats + plugins + Okapi bridge)

### Kapi: The File Connector

Kapi's role in the connector ecosystem:

**What Kapi does:**
- Reads local files via FormatRegistry (HTML, JSON, XLIFF, Markdown, etc.)
- Extracts Blocks from file content (streaming Parts → Blocks)
- Computes content hashes (`BlockIdentity` from [ADR-002](./002-content-model.md))
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

Kapi inherits the **three-tier format system** from [ADR-004](./004-processing-engine.md):

1. **Native formats** (Go): 15 built-in — HTML, XML, XLIFF, XLIFF 2, JSON, YAML, PO, Properties, Plaintext, Markdown, CSV, SRT, VTT, TMX
2. **Plugin formats** (any language): External executables via gRPC ([ADR-007](./007-plugin-system.md))
3. **Java bridge formats** (Okapi): JVM subprocesses with NDJSON protocol ([ADR-007](./007-plugin-system.md))

All three tiers register into the `FormatRegistry`. Kapi uses this registry to read/write files based on `.kapi/config.yaml` mappings.

**Format detection cascade:**
1. **MIME type** — explicit type declaration
2. **File extension** — `.html`, `.xliff`, `.json`, etc.
3. **Magic bytes** — binary signatures (BOM, XML declaration)
4. **Content sniffing** — heuristic analysis of file content

**Skeleton strategies:**
- **Fragment-based** (HTML, XML, XLIFF): Interleaved skeleton of non-translatable markup + references to translatable blocks
- **Re-parse** (Plaintext, JSON, YAML, PO): Re-open source document and replace content in place

These strategies ensure roundtrip fidelity when Kapi writes translated files back to disk.

### Server-Side Connector Registration

Server-side connectors register into a `ConnectorRegistry`:

```go
type ConnectorRegistry struct {
    connectors map[string]ConnectorFactory
}

type ConnectorFactory func(config map[string]any) (Connector, error)

func (r *ConnectorRegistry) Register(id string, category ConnectorCategory, factory ConnectorFactory)
func (r *ConnectorRegistry) NewConnector(id string, config map[string]any) (Connector, error)
func (r *ConnectorRegistry) List() []ConnectorInfo
```

**Built-in connectors** register via `init()`. **Plugin connectors** register at runtime via gRPC discovery ([ADR-007](./007-plugin-system.md)).

**Bowrain Server manages connectors** through its admin UI:
- Add new connector instances (configure API keys, URLs, etc.)
- Browse content items from each connector
- Trigger pull/push operations
- View sync status

**Kapi does not interact with the ConnectorRegistry** — it is a file-based tool that syncs with the server via API.

### ContentItem for Discovery

Server-side connectors expose browsable content:

```go
type ContentItem struct {
    ID           string
    ExternalID   string            // ID in the source system
    Name         string
    Path         string            // hierarchical path in source system
    ContentType  string            // "page", "entry", "text-layer", "file"
    Locale       model.LocaleID
    LastModified time.Time
    Metadata     map[string]string // connector-specific metadata
}
```

`List()` returns available content items from the connected system. For a CMS connector, these are pages and entries. For a design connector, artboards and text layers.

Bowrain displays content items as a browsable tree, allowing selective pull.

### PullOptions and PushOptions

```go
type PullOptions struct {
    Items   []string          // content item IDs to pull (empty = all)
    Locales []model.LocaleID  // source locales to pull
    Config  map[string]any    // connector-specific configuration
}

type PushOptions struct {
    Items   []string          // content item IDs to push (empty = all)
    Locales []model.LocaleID  // target locales to push
    Message string            // commit message / changelog
    Config  map[string]any
}
```

### SyncStatus for Change Tracking

```go
type SyncStatus struct {
    ConnectorID   string
    LastSynced    time.Time
    ItemsChanged  int       // items modified since last pull
    ItemsNew      int       // new items in source system
    ItemsDeleted  int       // items removed from source system
    PendingPush   int       // translated items awaiting push
}
```

`Sync()` provides a lightweight status check without performing a full pull. Bowrain uses this to show sync indicators in the connector management panel.

## Alternatives Considered

- **File-only integration** (traditional Okapi approach): Export to XLIFF, translate, re-import. Works for batch workflows but is manual, disconnected, and provides no live sync. Changes in the source system go undetected until the next export cycle.

- **Kapi as a server-side connector framework**: Would require Kapi to run as a daemon, manage connector lifecycles, and coordinate with the server. This overcomplicates Kapi's role. Kapi is a CLI tool for files; the server orchestrates connectors.

- **All connectors in Kapi**: Would require Kapi to have API credentials for CMS, design tools, etc. This mixes concerns — Kapi is for local file work, not managing integrations. Connectors belong server-side where they can be shared across teams and managed centrally.

- **XLIFF as universal exchange format**: XLIFF is a format, not an integration mechanism. It standardizes the file representation of translatable content but says nothing about how content gets from a CMS to a translation tool and back. Connectors are richer — they handle discovery, change tracking, and bidirectional sync.

## Consequences

- **Clear role separation**: Kapi handles files, Bowrain Server handles integrations.

- **Kapi is the file connector** for Bowrain Server — it reads/writes local files and syncs with the server via REST API. It does not manage server-side connectors.

- Server-side connectors (CMS, design, code, marketing) live in Bowrain Server and write extracted content into the ContentStore ([ADR-003](./003-content-store.md)).

- The FormatRegistry (15+ native formats, plugins, Okapi bridge) is Kapi's domain. All file processing goes through Kapi's format system.

- `.kapi/` project directories ([ADR-016](./016-kapi-project-model.md)) define file mappings (local paths ↔ remote items), enabling `kapi pull/push` to sync with the server.

- Connectors are the primary integration mechanism for Bowrain Server, positioning it as a localization platform rather than a file processing tool.

- Content from any connector flows into the same ContentStore and streaming pipeline ([ADR-004](./004-processing-engine.md)), processed by the same tools ([ADR-006](./006-tool-system.md)) regardless of origin.

- The `ConnectorRegistry` parallels the `FormatRegistry`, maintaining the established pattern of factory-based registration with runtime discovery.

- Kapi does not expose connector management commands (`kapi connect add/list`) — these belong in Bowrain Server's admin UI or API.

- The connector interface uses streaming Parts, the same unit used throughout the pipeline ([ADR-002](./002-content-model.md)). This means any connector's output feeds directly into tools, TM, terminology, and AI processing without adaptation layers.

- Format detection, skeleton strategies, and reader/writer separation are preserved as Kapi internals, invisible to the server and other connectors.
