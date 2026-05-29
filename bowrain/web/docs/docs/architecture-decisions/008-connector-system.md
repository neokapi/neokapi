---
id: 008-connector-system
sidebar_position: 8
title: "AD-008: Connector System"
---

# AD-008: Connector System

## Summary

Connectors are Bowrain's primary integration mechanism. They reach into live
systems — CMS, design tools, code repositories, marketing platforms, TMS
tools, and file systems — and move content bidirectionally into and out of
the ContentStore. Server-side connectors implement `IntegrationConnector`
(Fetch/Publish). Client-side connectors implement `SourceConnector`
(Push/Pull). The bowrain CLI is itself a File connector, managing local
files and syncing with the server via REST.

## Context

A localization platform is only as useful as the systems it integrates
with. Treating file formats as the primary integration mechanism produces
export/import workflows that are brittle, manual, and disconnected from
the source system. Changes in the CMS require re-export; translations sit
in files until someone remembers to re-import; no live connection exists
between the content source and the translation environment.

Connectors to native tools — pulling data directly into a versioned store
and publishing translations back — produce a fundamentally better
workflow. Content flows bidirectionally through a unified platform
instead of living in exchange files that drift out of sync.

Two concerns need separate interfaces:

1. **Server-side connectors** reach outward from Bowrain into external
   systems. Fetch content from Contentful; publish translations back to
   Contentful. The terminology is Bowrain's perspective.
2. **Client-side connectors** act from a source system's perspective.
   Push source files to Bowrain; pull translated files back. This is
   what the bowrain CLI does for local files, and what other source
   systems (code repository bots, design plugin bridges) do for their
   environments.

## Decision

### Two Interfaces, One Base

All connectors share a common identity and lifecycle base:

```go
type ConnectorBase interface {
    ID() string
    Name() string
    Category() Category
    Status(ctx context.Context) (*SyncStatus, error)
    Configure(config map[string]string) error
    Close() error
}
```

Server-side connectors implement `IntegrationConnector` — Bowrain reaches
into an external system:

```go
type IntegrationConnector interface {
    ConnectorBase

    // Fetch retrieves source content FROM the external system INTO Bowrain.
    Fetch(ctx context.Context, opts FetchOptions) ([]*ContentItem, error)

    // Publish sends translated content FROM Bowrain TO the external system.
    Publish(ctx context.Context, items []*ContentItem, opts PublishOptions) error
}
```

Client-side connectors implement `SourceConnector` — the source system
drives sync:

```go
type SourceConnector interface {
    ConnectorBase

    // Push sends source content FROM the source system TO Bowrain.
    Push(ctx context.Context, opts PushOptions) (*PushResult, error)

    // Pull retrieves translated content FROM Bowrain TO the source system.
    Pull(ctx context.Context, opts PullOptions) (*PullResult, error)
}
```

The terminology split resolves the ambiguity between "push from bowrain
CLI to server" and "push translations to WordPress" — the two operations
are the same word from opposite directions.

### Connector Categories

Six categories cover the integration space:

```go
type Category string

const (
    CategoryFile      Category = "file"      // Filesystem (bowrain CLI)
    CategoryCode      Category = "code"      // Git repositories, i18n resource bundles
    CategoryCMS       Category = "cms"       // Contentful, Strapi, WordPress
    CategoryDesign    Category = "design"    // Figma, Sketch
    CategoryMarketing Category = "marketing" // HubSpot, Marketo
    CategoryTMS       Category = "tms"       // External TMS integrations
)
```

Each category has characteristic behaviors: CMS connectors paginate
through entries and publish via content APIs; design connectors read
text layers and write back translated overlays; code connectors commit
to branches and open pull requests; marketing connectors sync campaigns
and assets across locales.

### Options and Status

```go
type FetchOptions struct {
    Paths   []string
    Since   time.Time
    Filter  map[string]string
}

type PublishOptions struct {
    Locales []model.LocaleID
    DryRun  bool
}

type PushOptions struct {
    Paths  []string // Specific file paths to push (empty = all)
    Force  bool     // Push all blocks, ignoring sync cache
    DryRun bool     // Report what would be pushed without sending
}

type PullOptions struct {
    Locales []model.LocaleID // Target locales to pull (empty = all)
    Force   bool             // Overwrite local changes
    DryRun  bool             // Report what would be pulled without writing
}

type SyncStatus struct {
    ConnectorID string
    LastSync    time.Time
    ItemCount   int
    FileCount   int
    WordCount   int
    PendingPull int  // Items changed externally since last pull
    PendingPush int  // Items changed locally since last push
    Errors      []string
}

type ContentItem struct {
    ID          string
    Name        string
    Path        string
    Format      string
    Locale      model.LocaleID
    Blocks      []*model.Block
    Metadata    map[string]string
    LastChanged time.Time
}
```

`Status()` provides a lightweight check without performing a full
Fetch/Pull, powering the sync indicators in the connector management UI
and the `kapi status` command.

### The bowrain plugin as the file connector

The bowrain plugin is the primary `SourceConnector` implementation —
`BowrainSourceConnector`. It manages `.kapi` projects (a `<dir-name>.kapi`
recipe and sibling `.kapi/` state directory) and syncs local files with
Bowrain Server.

```
.kapi project (recipe + state dir)
     │
     ▼
  bowrain CLI (reads recipe content collections)
     │
     ▼
  FormatRegistry (HTML, JSON, XLIFF, Markdown, ...)
     │
     ▼
  Streaming Pipeline (Parts → Blocks)
     │
     ▼
  REST Client
     │
     ▼
  Bowrain Server (sync endpoints)
     │
     ▼
  ContentStore
```

The bowrain CLI is to Bowrain Server as git is to GitHub — a local tool
that syncs with a remote platform. It does not manage server-side
connectors; it does not access the ContentStore directly; it does not
run server-side automation. Its job is to extract Blocks from local
files, compute content hashes, and move them across the sync boundary
([AD-009: Sync Protocol](009-sync-protocol.md)).

### Server-Side Connector Architecture

Server-side connectors live in `bowrain/connector/`. They use the
framework's format and pipeline machinery to normalize external content
into the same Part stream that flows through the rest of the system.

```
┌─────────────────────────────────────────────────┐
│          Bowrain Server (Platform)              │
│  ┌──────────────────────────────────────────┐   │
│  │      ContentStore (AD-004)               │   │
│  └──────────────────────────────────────────┘   │
│    ▲        ▲         ▲         ▲         ▲     │
│    │        │         │         │         │     │
│  CMS    Design    Code     Marketing   File     │
│  Conn.   Conn.    Conn.     Conn.     (API)     │
└────┼────────┼─────────┼─────────┼─────────┼─────┘
     │        │         │         │         │
     ▼        ▼         ▼         ▼         ▼
Contentful Figma     GitHub   HubSpot    bowrain CLI
   CMS      Design     Repo    Marketing  (.kapi project)
```

Content from any connector flows into the same ContentStore and the same
streaming pipeline. Tools, TM, terminology, AI translation, and review
all operate identically regardless of origin.

### Registry

Server-side connectors register into a `Registry`:

```go
type Factory func(config map[string]string) (IntegrationConnector, error)

type Info struct {
    Name     string
    Category Category
}

type Registry struct {
    mu        sync.RWMutex
    factories map[string]Factory
    infos     map[string]Info
}

func (r *Registry) Register(name string, category Category, factory Factory)
func (r *Registry) NewConnector(name string, config map[string]string) (IntegrationConnector, error)
func (r *Registry) List() []Info
```

Built-in connectors register via `init()`. Plugin connectors register at
runtime via gRPC discovery — see
[AD-framework-007: Plugin System](https://neokapi.github.io/web/neokapi/docs/architecture/007-plugin-system).

The bowrain CLI does not interact with the server-side registry. It is
a file-based `SourceConnector` that syncs via REST.

### Credential Flow

Workspace-scoped credentials are stored encrypted in the database. When
a connector needs OAuth authorization (e.g. Figma, HubSpot), the
server initiates an authorization flow:

1. User clicks "Connect Figma" in the connector management UI.
2. Server redirects to Figma's OAuth endpoint with a workspace-scoped
   state parameter.
3. Figma redirects back to `/connectors/:id/oauth/callback`.
4. Server exchanges the code for access + refresh tokens and encrypts
   them with the workspace's encryption key.
5. Subsequent Fetch/Publish calls load and decrypt the tokens on demand.

For destructive or high-privilege connector operations (Publish that
would mutate production content, re-authorize with elevated scopes),
the server issues a step-up prompt via the permission system — see
[AD-003: Permissions and Access Control](003-permissions.md).

### Connector Management UI

The connector management panel in `bowrain/apps/bowrain/` and
`bowrain/apps/web/` provides:

- **Configuration.** API keys, endpoints, authentication for each
  connector. Connector-specific settings are rendered dynamically based
  on the connector's `Configure()` schema.
- **Content browser.** Tree/list view of remote content items returned
  by `Fetch()` with content type icons and locale indicators.
- **Sync status.** Last pull/push timestamps, pending changes count,
  change indicators from `Status()`. Badges alert when source content
  has changed since the last pull.
- **Visual diff.** Compare local ContentStore state against remote
  source to identify what has changed before pulling or pushing.

### Format System Integration

Both server-side and client-side connectors use the framework's
three-tier format system — see
[AD-framework-005: Formats](https://neokapi.github.io/web/neokapi/docs/architecture/005-format-system):

1. **Native formats** (Go): built-in implementations — HTML, XML,
   XLIFF, XLIFF 2, JSON, YAML, PO, Properties, Markdown, CSV, SRT, VTT,
   TMX, and more (see the format reference for the current set).
2. **Plugin formats** (any language): external executables via gRPC.
3. **Bridge formats** (Okapi): subprocess-hosted filters via the
   gRPC bridge protocol.

All three tiers register into the framework's `FormatRegistry`.
Connectors use the registry to read and write content based on the item
format detected from MIME type, file extension, magic bytes, or content
sniffing.

### Embedded Translation

For design tools and CMS platforms, a lightweight Bowrain panel can be
embedded within the host application. The embedded UI is a WebView
rendering a subset of the translation editor, connected to the host
application's connector via a bidirectional message channel.

When a translator edits a translation in the embedded panel, the change
propagates through the connector and the sync protocol to update the
ContentStore. When source content changes in the host application, the
embedded panel updates to reflect the new content. The connector
abstraction extends beyond data exchange to become a full in-context
translation experience within native tools.

## Consequences

- Content flows bidirectionally between live systems and the
  ContentStore — no export/import shuffling.
- Role separation is clear: server-side connectors handle external
  integrations; the bowrain CLI handles local files.
- The bowrain CLI is the File connector and stays focused: it reads
  and writes local files and syncs via REST. It never manages
  server-side connectors or accesses the ContentStore directly.
- Plugin connectors register at runtime via gRPC, so new integrations
  ship without rebuilding the server.
- Connectors are the primary integration mechanism; file formats are a
  single connector category (File), not the whole story.
- The connector interface uses streaming Parts, the same unit used
  throughout the pipeline — any connector's output feeds directly into
  tools, TM, terminology, and AI processing without adaptation layers.
- Credentials are workspace-scoped, encrypted at rest, and subject to
  step-up prompts for high-privilege operations.

## Related

- [AD-001: Bowrain Vision and Module Architecture](001-vision-and-modules.md)
- [AD-003: Permissions and Access Control](003-permissions.md)
- [AD-004: Content Store and Versioning](004-content-store.md)
- [AD-007: Media and Blob Storage](007-media-and-blob-storage.md)
- [AD-009: Sync Protocol](009-sync-protocol.md)
- [AD-framework-005: Formats](https://neokapi.github.io/web/neokapi/docs/architecture/005-format-system)
- [AD-framework-007: Plugin System](https://neokapi.github.io/web/neokapi/docs/architecture/007-plugin-system)
