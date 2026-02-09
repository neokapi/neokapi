---
id: 005-connector-system
sidebar_position: 5
title: "ADR-005: Connector System"
---
# ADR-005: Bidirectional Connector System

## Context

Traditional localization tools treat file formats as the primary integration
mechanism. You export content from a CMS to XLIFF, translate it, and import it
back. This workflow is brittle, manual, and disconnected from the source system.
Changes in the CMS require re-export; translations sit in files until someone
remembers to re-import. There is no live connection between the content source
and the translation environment.

Connectors to native tools — pulling data directly into a versioned store —
create a fundamentally better workflow than file exchange. Instead of
exporting and importing, users connect their tools and data flows
bidirectionally through a unified platform.

Gokapi applies this pattern: connectors that pull content from CMS platforms,
design tools, code repositories, and marketing platforms directly into the
Content Store (ADR-003), and push translations back. File formats remain
important — Okapi's 40+ filters represent years of engineering — but they are
one connector type (the FileConnector), not the entire integration story.

The connector system must unify all content sources behind a single interface so
that the streaming pipeline (ADR-003), tools (ADR-007), and Bowrain UI
(ADR-012) work identically regardless of where content originates.

## Decision

### Connector Interface

Every connector implements a common interface for content operations, discovery,
synchronization, and lifecycle management:

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

`Pull` returns a channel of Parts — the same streaming unit used throughout the
pipeline (ADR-002). This means any connector's output feeds directly into tools,
translation memory, terminology lookup, and AI processing without adaptation
layers. `Push` consumes a Part channel to write translations back to the source
system.

`List` enables browsable discovery of content in the connected system. `Sync`
reports the connector's synchronization status — what has changed since the last
pull, what translations are pending push.

### Connector Categories

```go
type ConnectorCategory string

const (
    CategoryCMS       ConnectorCategory = "cms"
    CategoryDesign    ConnectorCategory = "design"
    CategoryCode      ConnectorCategory = "code"
    CategoryMarketing ConnectorCategory = "marketing"
    CategoryFile      ConnectorCategory = "file"
)
```

**CMS Connectors** — Pull content from headless CMS APIs (Contentful, Strapi,
WordPress REST API). Content items map to CMS entries and pages. Each field
becomes a Block with display hints carrying field metadata (max length, content
type, rich text vs. plain text). Translations push back via the same API,
updating the localized version of each entry. CMS connectors track content
versions to enable incremental pull — only changed entries are re-extracted.

**Design Connectors** — Pull text layers from design tools (Figma API, Sketch
files). Each text layer becomes a Block with display hints encoding visual
context: font size, position, bounding box dimensions, and maximum width
constraints. Translated text pushes back to create localized design variants.
Design connectors preserve layer hierarchy as nested Layers (ADR-002), so
translators see structure matching the original design.

**Code Connectors** — Pull i18n resource files from Git repositories (JSON,
YAML, Properties, PO files in a repo). Unlike the FileConnector which operates
on individual files, code connectors understand repository structure: they
discover resource bundles, track locale variants, and detect changes via Git
commits. Push creates pull requests or commits with translated files, preserving
the repository's i18n conventions.

**Marketing Connectors** — Pull email templates, landing pages, and ad copy from
marketing platforms (HubSpot, Marketo). Content items map to campaign assets.
Display hints carry platform constraints (subject line length limits, preview
text requirements). Push delivers localized variants ready for multi-language
campaigns.

**File Connector** — The bridge to the existing format system. Wraps
`DataFormatReader` / `DataFormatWriter` and the `FormatRegistry` behind the
Connector interface. This is the most mature connector category, inheriting 15
native formats plus the Okapi Java bridge (ADR-004).

### FileConnector: The Format System as a Connector

The FileConnector wraps the existing format system (ADR-004) to present
file-based processing through the Connector interface:

```go
type FileConnector struct {
    registry *format.FormatRegistry
}

func (c *FileConnector) Pull(ctx context.Context, opts PullOptions) (<-chan *model.Part, error) {
    reader, err := c.registry.NewReader(opts.Format, opts.Config)
    if err != nil {
        return nil, err
    }
    if err := reader.Open(ctx, opts.Document); err != nil {
        return nil, err
    }
    return reader.Read(ctx), nil
}

func (c *FileConnector) Push(ctx context.Context, parts <-chan *model.Part, opts PushOptions) error {
    writer, err := c.registry.NewWriter(opts.Format, opts.Config)
    if err != nil {
        return err
    }
    writer.SetOutput(opts.OutputPath)
    return writer.Write(ctx, parts)
}
```

This means existing format-centric workflows (`kapi convert`, `kapi translate`)
continue to work unchanged — they operate through the FileConnector. New
connector-centric commands (`kapi connect`, `kapi pull`, `kapi push`) work with
any connector type (ADR-013).

#### Three Implementation Tiers (from ADR-004)

The FileConnector inherits the format system's tiered architecture:

1. **Native formats** (Go): 15 built-in — HTML, XML, XLIFF, XLIFF 2, JSON,
   YAML, PO, Properties, Plaintext, Markdown, CSV, SRT, VTT, TMX.
2. **Plugin formats** (any language): External executables via gRPC (ADR-007).
3. **Java bridge formats** (Okapi): JVM subprocesses with NDJSON protocol
   (ADR-007).

All three tiers register into the same `FormatRegistry`. The FileConnector
unifies them behind the Connector interface, so the rest of the system does not
distinguish between a native Go HTML reader and a Java bridge DOCX filter.

#### Multi-Strategy Format Detection

The FileConnector preserves the format system's detection cascade:

1. **MIME type** — explicit type declaration
2. **File extension** — `.html`, `.xliff`, `.json`, etc.
3. **Magic bytes** — binary signatures (BOM, XML declaration)
4. **Content sniffing** — heuristic analysis of file content

The registry stores a `FormatSignature` per format with the applicable detection
strategies.

#### Skeleton Strategies

Format readers use two skeleton strategies for roundtrip fidelity:

- **Fragment-based** (HTML, XML, XLIFF): The reader builds an interleaved
  skeleton of `SkeletonText` (non-translatable markup) and `SkeletonRef`
  (pointers to translatable blocks). The writer reconstructs the output by
  walking the skeleton and inserting translated content at each reference.

- **Re-parse** (Plaintext, JSON, YAML, PO): The writer re-opens the source
  document and replaces translatable content in place, preserving all original
  formatting. No skeleton is stored; the source document itself serves as the
  template.

### ContentItem for Discovery

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

`List()` returns available content items from the connected system. For a CMS
connector, these are pages and entries. For a design connector, artboards and
text layers. For the FileConnector, files matching supported format signatures.

Bowrain displays content items as a browsable tree, allowing selective pull.
Users can choose specific pages from a CMS, specific artboards from Figma, or
specific files from a repository (ADR-012).

### PullOptions and PushOptions

```go
type PullOptions struct {
    Format   string            // for FileConnector: format ID
    Document *model.RawDocument // for FileConnector: source document
    Items    []string          // content item IDs to pull (empty = all)
    Locales  []model.LocaleID  // source locales to pull
    Config   map[string]any    // connector-specific configuration
}

type PushOptions struct {
    Format     string
    OutputPath string
    Items      []string          // content item IDs to push (empty = all)
    Locales    []model.LocaleID  // target locales to push
    Config     map[string]any
}
```

The options structs accommodate both file-specific parameters (format ID, source
document, output path) and connector-generic parameters (item selection, locale
filtering). Connectors ignore fields that do not apply to their category.

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

`Sync()` provides a lightweight status check without performing a full pull.
Bowrain uses this to show sync indicators in the connector management panel and
to alert users when source content has changed.

### Connector Registry

```go
type ConnectorRegistry struct {
    connectors map[string]ConnectorFactory
}

type ConnectorFactory func(config map[string]any) (Connector, error)

func (r *ConnectorRegistry) Register(id string, category ConnectorCategory, factory ConnectorFactory)
func (r *ConnectorRegistry) NewConnector(id string, config map[string]any) (Connector, error)
func (r *ConnectorRegistry) List() []ConnectorInfo
```

Connectors register into a `ConnectorRegistry` analogous to the format system's
`FormatRegistry`. Built-in connectors register via `init()`. Plugin connectors
register at runtime via gRPC discovery (ADR-007). The registry provides a
`List()` method for Bowrain to populate the connector management UI.

### Embedded Translation UI

For design tools and CMS platforms, connectors can provide an embedded
translation experience — a lightweight Bowrain panel that appears within the
host application. This enables translators to work in context without switching
tools.

The embedded UI is a WebView rendering a subset of the Bowrain translation
editor (ADR-012), connected to the host application's connector via a
bidirectional message channel. When a translator edits a translation in the
embedded panel, the change propagates through the connector to update the
Content Store. When the source content changes in the host application, the
embedded panel updates to reflect the new content.

This approach extends the connector concept beyond data exchange to become a
full in-context translation experience within native tools.

## Alternatives Considered

- **File-only integration** (traditional Okapi approach): Export to XLIFF,
  translate, re-import. Works for batch workflows but is manual, disconnected,
  and provides no live sync. Changes in the source system go undetected until
  the next export cycle.

- **Webhook-only integration**: Reactive approach where source systems notify
  Gokapi of changes. Handles push well but does not support browsing, selective
  pull, or initial content discovery. Webhooks complement connectors but cannot
  replace them.

- **XLIFF as universal exchange format**: XLIFF is a format, not an integration
  mechanism. It standardizes the file representation of translatable content but
  says nothing about how content gets from a CMS to a translation tool and back.
  Connectors are richer — they handle discovery, change tracking, and
  bidirectional sync.

- **Per-system custom integrations**: Building bespoke integrations for each CMS
  and design tool does not scale. The Connector interface provides consistency
  across all content sources, and the shared Part model (ADR-002) means
  downstream tools work identically regardless of origin.

## Consequences

- Connectors are the primary integration mechanism, positioning Gokapi as a
  localization platform rather than a file processing tool.
- The FileConnector preserves all existing format support — 15 native formats
  plus the Okapi Java bridge — as a first-class connector.
- New connectors can be added as built-in packages or as plugins via the gRPC
  plugin system (ADR-007).
- Content from any connector flows into the same Content Store and streaming
  pipeline (ADR-003), processed by the same tools (ADR-007) regardless of
  origin.
- The `kapi connect`, `kapi pull`, and `kapi push` commands provide
  connector-centric workflows alongside existing format-centric commands
  (ADR-013).
- Bowrain shows a connector management UI with browse, select, pull, and push
  workflows (ADR-012).
- Format detection, skeleton strategies, and reader/writer separation are
  preserved as FileConnector internals, invisible to the rest of the system.
- The embedded translation concept opens the door to in-context translation
  within native design and CMS tools, a significant differentiator.
- The `ConnectorRegistry` parallels the `FormatRegistry`, maintaining the
  established pattern of factory-based registration with runtime discovery.
