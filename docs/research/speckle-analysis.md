# Neokapi as the Speckle of Localization: Architecture Analysis

## Executive Summary

[Speckle](https://speckle.systems/) is an open-source data platform for the AEC (Architecture, Engineering, Construction) industry that has solved a problem remarkably similar to what Neokapi faces in localization: extracting structured data from a vast number of proprietary native formats, processing it through a common model, and writing it back — all while an "official standard" (IFC for AEC, XLIFF for localization) exists but has failed to deliver true interoperability.

This report analyzes Speckle's architecture, contrasts it with Neokapi's current design, and identifies concrete gaps and opportunities for Neokapi to become the definitive open-source platform for localization data interoperability.

---

## 1. The Parallel: Two Industries, One Problem

### AEC's Interoperability Crisis

The AEC industry has dozens of authoring tools — Revit, Rhino, AutoCAD, ArchiCAD, Tekla, SketchUp, Blender — each with its own proprietary data model. IFC (Industry Foundation Classes) was supposed to solve this as the universal exchange standard, but after decades of development:

- IFC prioritizes **completeness over composability**, making it unapproachable
- Export/import through IFC is lossy — round-trips degrade data
- IFC is **file-based** in a world that needs real-time collaboration
- No two IFC implementations are fully compatible

Speckle's answer: bypass the standard-as-interchange model entirely. Instead, build **native connectors** that speak each application's language, convert through a **composable intermediate model**, and store in a **versioned object graph**.

### Localization's Interoperability Crisis

The localization industry has the same pattern. Content lives in:

- CMS platforms (WordPress, Drupal, Contentful, Strapi)
- Design tools (Figma, Sketch, Adobe XD)
- Development repos (JSON i18n, .properties, .strings, .resx)
- Documentation systems (Markdown, DITA, reStructuredText)
- Marketing tools (HubSpot, Marketo, Salesforce)
- Software UI (Android XML, iOS .strings, Flutter ARB)
- Subtitle/media (SRT, VTT, TTML)

XLIFF was supposed to be the universal exchange format. But:

- XLIFF 1.2 became "a victim of its own success" — so flexible that no two implementations were alike
- XLIFF 2.0 changed the DOM structure entirely, creating incompatibility with 1.2
- Round-tripping through XLIFF is lossy for rich formats (HTML context, metadata, comments)
- XLIFF focuses on **exchange** not **processing** — it's a file format, not a data platform
- TMS vendors each implement XLIFF differently, defeating the interoperability goal

**The parallel is exact.** Both industries have: (1) many proprietary native formats, (2) a standard that promised interoperability but delivered incomplete results, (3) a need for real-time data flow rather than file exchange.

---

## 2. Speckle's Architecture: Key Concepts

### 2.1 The Base Object: A Dynamic Foundation

At the core of Speckle is the `Base` class — the ancestor of all data in the system:

```
Base
├── id (hash-based, immutable)
├── applicationId (original app's ID)
├── speckle_type (type discriminator)
├── totalChildrenCount
├── [dynamic properties] (dictionary-like)
└── [typed properties] (via inheritance)
```

**Key design decisions:**
- **Hybrid typed/dynamic**: Inherits from C# `DynamicObject` — strongly typed when needed, falls back to dynamic properties. This means any data can flow through the system even without a pre-defined schema.
- **Immutable hashing**: Every object gets a content-based hash ID. Changes create new objects (Git-like semantics).
- **Detachable references**: The `@`-prefix on property names stores objects as references instead of nested copies, enabling deduplication.
- **Chunking**: Large arrays (e.g., mesh vertices) are split into manageable batches for serialization.

### 2.2 Kits: Schema + Conversion Bundled Together

A **Kit** is Speckle's unit of interoperability. It bundles:

1. **Object Model** — the schema classes (geometry, BIM elements, etc.)
2. **Converters** — translation routines to/from native application formats

The default "Objects Kit" provides:
- **Geometry layer**: Point, Line, Curve, Mesh, Surface, Brep — the basic building blocks
- **BuiltElements layer**: Wall, Floor, Beam, Room, Duct — domain-specific objects that compose geometry
- **Application extensions**: `Objects.BuiltElements.Revit` adds Revit-specific properties to generic elements

Kits are **hot-swappable** — you can replace the entire default object model with your own ("Bring Your Own Schema"). The system only enforces that objects inherit from `Base`.

### 2.3 Connectors: Living Inside Host Applications

Unlike traditional file-based exchange, Speckle **connectors** are plugins that run inside the authoring applications:

```
[Revit] ←→ [Revit Connector] ←→ [Kit Converter] ←→ [Transport] ←→ [Speckle Server]
[Rhino] ←→ [Rhino Connector] ←→ [Kit Converter] ←→ [Transport] ←→ [Speckle Server]
```

Each connector implements:
- `ConvertToSpeckle()` — native objects → Speckle Base objects
- `ConvertToNative()` — Speckle Base objects → native objects
- Selection UI for choosing what to send/receive
- Progress reporting and error handling

**Critical insight**: Connectors don't convert to a file format — they convert to an **object graph** that lives in a database with full version history. This is fundamentally different from "export to XLIFF, import from XLIFF."

### 2.4 Transport Layer: Abstracted Persistence

Speckle abstracts storage through **transports**:

| Transport | Use Case |
|-----------|----------|
| SQLite | Local cache, offline work |
| Server | Cloud storage, collaboration |
| Memory | Testing, temporary processing |
| MongoDB | Alternative backend |
| Disk | File-based persistence |

Writes can happen **in parallel to multiple transports** — e.g., local cache + remote server simultaneously. This decouples the data platform from any single storage backend.

### 2.5 Display Values: Universal Fallback

Every Speckle object can carry a `displayValue` — a simplified mesh/polyline representation. When a receiving application doesn't understand the native object type, it can still **display** it using this fallback. This ensures universal visibility even with partial schema support.

### 2.6 Speckle Automate: CI/CD for Data

Speckle Automate provides serverless functions that trigger on data changes — quality checks, compliance validation, report generation, clash detection. This turns the data platform into an **automation backbone**.

---

## 3. Neokapi's Current Architecture

### 3.1 The Content Model

Neokapi's model is a well-designed streaming pipeline:

```
Part (Type + Resource)
├── Layer    — structural grouping (document, embedded content)
├── Block    — translatable content (source segments + target segments per locale)
│   ├── Fragment — text with inline Spans (coded text with Unicode PUA markers)
│   └── Segment  — subdivision of a Block
├── Data     — non-translatable structure
├── Media    — binary content
├── Group    — logical grouping (start/end)
└── RawDocument — input document reference
```

### 3.2 The Format System (Filters)

Formats implement `DataFormatReader`/`DataFormatWriter` interfaces:

```go
// Reader: Opens a document, streams Parts through a channel
DataFormatReader.Open(ctx, doc) → Read(ctx) → <-chan PartResult

// Writer: Consumes Parts from a channel, reconstructs the document
DataFormatWriter.Write(ctx, <-chan *Part) → reconstructed output
```

15 built-in formats: HTML, XML, XLIFF, XLIFF2, JSON, YAML, PO, Properties, Plaintext, Markdown, CSV, SRT, VTT, TMX.

### 3.3 The Tool System

Tools implement `Process(ctx, in <-chan *Part, out chan<- *Part)` — they receive Parts, transform them, and pass them through. The `BaseTool` provides handler functions per Part type.

### 3.4 The Flow Executor

`Executor` orchestrates tool chains with goroutines and channels. Each tool runs in its own goroutine with buffered channels providing backpressure.

### 3.5 Connectors (Current)

Current "connectors" are **translation API wrappers** — DeepL, Google, Microsoft, MyMemory, ModernMT. They're implemented as Tools that call external APIs. They operate at the Block level, sending source text and receiving translations.

### 3.6 Plugin System

HashiCorp go-plugin + gRPC for external format readers/writers and tools, including a Java bridge for leveraging Okapi Framework filters.

---

## 4. Architectural Comparison

| Concept | Speckle (AEC) | Neokapi (Localization) | Gap? |
|---------|--------------|----------------------|------|
| **Fundamental unit** | Base object (dynamic + typed) | Part (typed enum + Resource interface) | Moderate |
| **Schema/model** | Kit (hot-swappable, BYO) | Fixed model (Part/Block/Fragment) | **Significant** |
| **Format reading** | Connector (lives in host app) | DataFormatReader (reads files) | **Significant** |
| **Format writing** | Connector (writes to host app) | DataFormatWriter (writes files) | **Significant** |
| **Conversion layer** | Kit converters (bidirectional) | Reader/Writer (tightly coupled) | Moderate |
| **Processing** | Speckle Automate (serverless) | Executor (goroutine pipeline) | Moderate |
| **Persistence** | Transport layer (multi-backend) | None (streaming only) | **Significant** |
| **Versioning** | Git-like object graph | None | **Significant** |
| **Collaboration** | Real-time, multi-user | None | **Significant** |
| **Inline formatting** | displayValue (mesh fallback) | Coded text (PUA markers) | Comparable |
| **Extensibility** | Custom kits, dynamic props | Plugin system (gRPC) | Moderate |
| **Standards support** | IFC import (but not central) | XLIFF read/write (first-class) | Comparable |

---

## 5. What Neokapi Can Learn: The Gaps

### Gap 1: From File Formats to Application Connectors

**Speckle's insight**: The real interoperability problem isn't converting between file formats — it's connecting to the **systems where content lives**. A Revit connector doesn't export to a file; it reaches into Revit's live object model, extracts what's needed, and pushes it to the data platform.

**For Neokapi**: The current format system (DataFormatReader/Writer) is file-centric. But much of the world's translatable content doesn't live in files:

| System Category | Examples | Integration Model |
|----------------|----------|-------------------|
| **CMS platforms** | WordPress, Drupal, Contentful, Strapi, Sanity | REST/GraphQL API connectors |
| **Design tools** | Figma, Sketch, Adobe XD | Plugin APIs |
| **Code repos** | GitHub, GitLab, Bitbucket | Git API + file format detection |
| **Marketing platforms** | HubSpot, Marketo, Salesforce | REST API connectors |
| **Documentation** | Confluence, Notion, Google Docs | REST/API connectors |
| **E-commerce** | Shopify, Magento, WooCommerce | REST API connectors |
| **Help centers** | Zendesk, Intercom, Freshdesk | REST API connectors |
| **Game engines** | Unity, Unreal | Plugin APIs |
| **Mobile apps** | App Store Connect, Google Play Console | API connectors |
| **Subtitle platforms** | YouTube Studio, Vimeo | API connectors |

**Recommendation**: Introduce a **Connector interface** distinct from `DataFormatReader`/`DataFormatWriter`:

```go
// Connector represents a bidirectional integration with an external system.
// Unlike DataFormatReader/Writer which operate on files, Connectors operate
// on live systems via APIs, plugins, or other integration mechanisms.
type Connector interface {
    Name() string

    // Pull extracts translatable content from the external system
    Pull(ctx context.Context, opts PullOptions) (<-chan *model.Part, error)

    // Push sends translated content back to the external system
    Push(ctx context.Context, parts <-chan *model.Part, opts PushOptions) error

    // List discovers available content in the external system
    List(ctx context.Context, opts ListOptions) ([]ContentRef, error)

    // Status checks connectivity and capabilities
    Status(ctx context.Context) (*ConnectorStatus, error)
}
```

The existing file-based formats remain as-is — they're the equivalent of Speckle's IFC import: useful but not the primary integration path. The new Connector interface opens the door to **live system integration**.

### Gap 2: Persistence and Versioning

**Speckle's insight**: Data should live in a **versioned object graph**, not just flow through a pipeline. Every change is tracked, every version is immutable, and the full history is queryable.

**For Neokapi**: The current architecture is purely streaming — Parts flow from reader through tools to writer. There's no persistence layer, no version history, no ability to compare what changed between processing runs.

**Recommendation**: Introduce an abstracted **Store** layer (analogous to Speckle's Transport):

```go
type Store interface {
    // Save persists a set of Parts (a "version") and returns a version ID
    Save(ctx context.Context, parts []*model.Part, meta VersionMeta) (VersionID, error)

    // Load retrieves a specific version as a Part stream
    Load(ctx context.Context, id VersionID) (<-chan *model.Part, error)

    // Diff compares two versions and returns changes
    Diff(ctx context.Context, from, to VersionID) ([]Change, error)

    // History lists versions for a given content reference
    History(ctx context.Context, ref ContentRef) ([]Version, error)
}
```

Implementations could include:
- **SQLite Store** — local persistence (like Speckle's SQLite transport)
- **Server Store** — cloud storage via the bowrain-server REST API
- **Memory Store** — for testing and temporary pipelines
- **Git Store** — version content alongside code (natural for i18n files)

This enables **incremental processing** — only re-translate what changed — which is critical for cost efficiency with LLM-based translation.

### Gap 3: A More Composable Object Model

**Speckle's insight**: The `Base` class is both typed and dynamic. You can define strongly-typed properties for known schemas, but any additional data passes through as dynamic properties. This means the system never loses data it doesn't understand.

**For Neokapi**: The current model is entirely statically typed. When an HTML reader encounters a `<div class="product-card" data-sku="ABC123">`, the class and data-sku attributes are preserved in the Skeleton but aren't semantically accessible to tools. If a Figma connector encounters component metadata, there's no natural place to put it in the Part model.

**Recommendation**: Extend the model with **open-ended metadata** inspired by Speckle's Base:

The `Properties map[string]string` on Block/Layer is a start, but it's limited to string values. Consider:

```go
// Properties with richer typing, analogous to Speckle's dynamic properties
type Properties map[string]any

// Annotations with an open type system
type Annotation interface {
    AnnotationType() string
}
```

Additionally, consider a concept like Speckle's `displayValue` — a **canonical text fallback** for rich content. When a Block comes from a Figma design with styled text, the Fragment's coded text is the canonical representation, but the original styling metadata could be preserved as a detachable property for round-trip fidelity. This is essentially what coded text already does, but formalizing the pattern would help with connector development.

### Gap 4: Kit-like Format Abstraction Layers

**Speckle's insight**: Kits bundle a **schema** with its **converters**. The default Objects Kit defines geometry + BIM elements and includes converters for every supported application. Custom kits can replace the entire schema.

**For Neokapi**: Format readers/writers are tightly coupled — each format has its own reader and writer that understand the format's specific structure. There's no intermediate "schema" layer that multiple formats could share.

**Recommendation**: Consider introducing **Format Families** — groups of formats that share common patterns:

| Family | Shared Patterns | Member Formats |
|--------|----------------|----------------|
| **Markup** | Tag-based structure, inline elements, attributes | HTML, XML, DITA, XHTML |
| **Structured Data** | Key-value hierarchies, arrays, nested objects | JSON, YAML, Properties, TOML |
| **Bilingual** | Source/target pairs, translation units, metadata | XLIFF, XLIFF2, PO, TMX, TBX |
| **Subtitle** | Timed text, cue sequences, styling | SRT, VTT, TTML, DFXP |
| **Rich Text** | Paragraphs, inline formatting, references | Markdown, reStructuredText, AsciiDoc |
| **Binary Document** | Embedded content, styles, structure | DOCX, PPTX, IDML, PDF |

Within a family, a shared **base converter** could handle common patterns (e.g., inline code handling for all markup formats), with format-specific overrides for unique features. This is analogous to how Speckle's Objects Kit has a geometry layer used by all BIM elements.

### Gap 5: The Automation Layer

**Speckle's insight**: Speckle Automate turns the data platform into a CI/CD system — functions trigger automatically on data changes, running quality checks, generating reports, and transforming data.

**For Neokapi**: The Executor is a powerful pipeline engine, but it's invoked explicitly. There's no concept of automated triggers, quality gates, or continuous processing.

**Recommendation**: Build on the existing Flow system to add **automation triggers**:

- **On-change hooks**: When a Connector detects content changes, automatically run a Flow
- **Quality gates**: Flows that validate translation quality before pushing back
- **Continuous sync**: Background processes that keep external systems synchronized
- **Webhook integration**: Trigger Flows from external events (CMS publish, Git push, etc.)

This would position Neokapi as a **localization automation platform**, not just a format conversion toolkit.

---

## 6. Which Systems Need "Connectors" vs "Formats"?

The key distinction is **where the content lives**:

### Keep as Formats (File-Based)

These are content types that naturally exist as files, often within version control or file systems:

- JSON i18n files (.json)
- Java/C# properties (.properties, .resx)
- iOS/Android strings (.strings, .xml)
- Gettext PO files (.po)
- Subtitle files (.srt, .vtt)
- Markup files (.html, .xml, .md)
- XLIFF exchange files (.xliff)
- Office documents (.docx, .pptx) — though these could also benefit from connectors

### New as Connectors (System-Based)

These need API-level integration because the content lives in a system, not a file:

| Priority | Connector | Why |
|----------|-----------|-----|
| **High** | **Figma** | Design-to-dev content flow; plugin API available; visual context |
| **High** | **Contentful/Strapi** | Headless CMS with structured content; REST/GraphQL APIs |
| **High** | **GitHub/GitLab** | Auto-detect localizable files in repos; PR-based translation workflow |
| **High** | **WordPress** | World's most popular CMS; REST API; huge market |
| **Medium** | **Shopify** | E-commerce localization; REST API; growing market |
| **Medium** | **Notion** | Knowledge base/docs; API available; growing in enterprises |
| **Medium** | **Google Docs/Sheets** | Collaborative documents; Drive API |
| **Medium** | **Zendesk** | Help center content; REST API |
| **Medium** | **HubSpot** | Marketing content; REST API |
| **Low** | **Salesforce** | CRM/UI content; complex but large market |
| **Low** | **Unity/Unreal** | Game localization; plugin APIs |

### Hybrid: Both Format and Connector

Some systems would benefit from both a file format reader AND a live connector:

- **Markdown**: File format reader exists, but a GitHub connector could manage the discovery/sync workflow
- **JSON i18n**: File format reader exists, but a Crowdin/Lokalise connector could sync with their platforms
- **XLIFF**: File format reader exists, but a TMS connector could push/pull from Trados, memoQ, etc.

---

## 7. The Internal Model: Should Neokapi Adopt Speckle's Approach?

### What Works Well in Neokapi Today

Neokapi's current model has genuine strengths:

1. **Streaming architecture**: Channel-based pipeline with backpressure is elegant and memory-efficient
2. **Coded text**: The Unicode PUA marker system for inline formatting is clever and battle-tested (inherited from Okapi's TextFragment)
3. **Layer nesting**: Embedded content support (HTML inside JSON) is clean
4. **Type discrimination**: The PartType enum is simple and efficient for dispatch

### Where Speckle's Model Could Improve Neokapi

1. **Richer metadata preservation**: Speckle's dynamic properties ensure no data loss during round-trips. Neokapi could lose format-specific metadata that doesn't map to Block fields.

2. **Content-addressable storage**: Speckle's hash-based IDs enable deduplication. In localization, this means identical segments across documents are stored once — critical for TM leverage.

3. **Detachable references**: Speckle's `@`-prefix pattern for shared objects could apply to shared terminology, TM matches, and style guides — objects referenced across many translation units.

4. **Universal display fallback**: Speckle's `displayValue` concept could map to a "preview representation" for each Block — how should this content be displayed to a translator in a UI, regardless of the source format?

### Proposed Evolution: Keep the Stream, Add the Graph

Rather than replacing Neokapi's streaming model with Speckle's object graph, **layer the graph on top of the stream**:

```
Streaming Layer (existing):
  Reader → chan *Part → Tool 1 → Tool 2 → ... → Writer

Graph Layer (new):
  Store ← Versioned snapshots of Part streams
  ↕
  Diff engine, query API, deduplication

Connector Layer (new):
  External System ←→ Connector ←→ Streaming Layer ←→ Graph Layer
```

The streaming layer remains the workhorse for processing. The graph layer adds persistence, versioning, and queryability. Connectors bridge to external systems.

---

## 8. Concrete Architectural Recommendations

### 8.1 Introduce ContentRef: Addressable Content

Like Speckle's content-addressed objects, give every piece of content a stable, hashable identity:

```go
type ContentRef struct {
    System    string    // "figma", "contentful", "file"
    Path      string    // system-specific path
    ContentID string    // content-addressed hash of the content
    Version   string    // version/revision identifier
}
```

### 8.2 Extend Properties to Support Rich Types

Move from `map[string]string` to `map[string]any` with JSON serialization, preserving format-specific metadata through the pipeline:

```go
type Block struct {
    // ... existing fields ...
    Properties  map[string]any      // rich metadata (Speckle-inspired)
    DisplayHint *DisplayHint        // how to present in UI (like displayValue)
    ContentRef  *ContentRef         // where this content came from
}
```

### 8.3 Define the Connector Interface

```go
type Connector interface {
    Name() string
    Description() string

    // Discovery
    ListContent(ctx context.Context, opts ListOptions) ([]ContentRef, error)

    // Reading (Pull)
    Pull(ctx context.Context, refs []ContentRef) (<-chan *model.Part, error)

    // Writing (Push)
    Push(ctx context.Context, parts <-chan *model.Part, locale model.LocaleID) error

    // Sync (bidirectional delta)
    Sync(ctx context.Context, opts SyncOptions) (*SyncResult, error)

    // Configuration
    Config() ConnectorConfig
    SetConfig(cfg ConnectorConfig) error
}
```

### 8.4 Add a Store Abstraction

```go
type Store interface {
    Save(ctx context.Context, ref ContentRef, parts []*model.Part) (VersionID, error)
    Load(ctx context.Context, ref ContentRef, version VersionID) (<-chan *model.Part, error)
    Diff(ctx context.Context, ref ContentRef, from, to VersionID) ([]Change, error)
    Query(ctx context.Context, q Query) ([]QueryResult, error)
}
```

### 8.5 Format Families with Shared Base Converters

```go
// MarkupConverter provides shared inline-code handling for HTML, XML, DITA
type MarkupConverter struct {
    InlineElements  []string  // elements that become Spans
    BlockElements   []string  // elements that become Blocks
    SkipElements    []string  // elements to pass through as Data
    AttributeRules  []AttrRule // which attributes are translatable
}

// Specific formats extend with their own rules
htmlConverter := &MarkupConverter{
    InlineElements: []string{"b", "i", "a", "span", "em", "strong"},
    BlockElements:  []string{"p", "div", "h1", "h2", "li", "td"},
    // ...
}
```

---

## 9. The "Speckle of Localization" Vision

Putting it all together, here's what Neokapi becomes:

### Open Source Platform Layer
- **Format readers/writers** (15+ built-in, extensible via plugins) — like Speckle's IFC import
- **System connectors** (CMS, design tools, repos, platforms) — like Speckle's native connectors
- **Versioned content store** — like Speckle's transport-backed object graph
- **Processing pipeline** (Flow executor with tools) — like Speckle Automate
- **REST API server** (bowrain-server) — like Speckle Server

### What Changes from Today
1. **Connectors become first-class** alongside formats
2. **Content is addressable and versioned**, not just streamed
3. **The model gets richer metadata** without losing streaming simplicity
4. **Format families** reduce duplication and improve consistency
5. **Automation hooks** turn the pipeline into a platform

### What Stays the Same
1. Channel-based streaming pipeline (the core engine)
2. Coded text with PUA markers (the inline formatting system)
3. Part/Block/Fragment/Layer model (the content abstraction)
4. Plugin system for external formats and tools
5. XLIFF as a first-class exchange format (but not the only path)

### The Positioning

| | XLIFF | Okapi | Neokapi (Today) | Neokapi (Vision) | Speckle (Analogy) |
|---|---|---|---|---|---|
| **Model** | Exchange format | Streaming events | Streaming Parts | Versioned Part graph | Versioned object graph |
| **Formats** | Is itself a format | 20+ file filters | 15+ file formats | Formats + Connectors | Kit converters + Connectors |
| **Standards** | Is the standard | Uses XLIFF, SRX, TMX | Reads/writes XLIFF | XLIFF as one path among many | IFC as one import among many |
| **Persistence** | Files | None | None | Multi-backend store | Transport layer |
| **Collaboration** | File sharing | None | None | API + versioning | Real-time multi-user |
| **Automation** | None | Pipeline runner | Flow executor | Event-driven automation | Speckle Automate |

---

## 10. Priority Roadmap

### Phase 1: Foundation (Near-term)
- Extend `Properties` to `map[string]any` for richer metadata
- Add `ContentRef` for addressable content
- Design the `Connector` interface
- Implement first connector: **GitHub** (file-based content in repos)

### Phase 2: Persistence (Medium-term)
- Implement `Store` interface with SQLite backend
- Add content-addressable hashing for deduplication
- Build version diffing for incremental processing
- Implement second connector: **Contentful** or **WordPress**

### Phase 3: Platform (Longer-term)
- Automation triggers (on-change hooks, quality gates)
- Format families with shared base converters
- Design tool connectors (Figma)
- Multi-user collaboration through bowrain-server
- TMS connectors (bidirectional sync with Trados, memoQ, Phrase)

---

## Sources

- [Speckle Architecture Documentation](https://speckle.guide/dev/architecture.html)
- [The Speckle Objects Kit](https://speckle.guide/dev/objects.html)
- [The Speckle Base Object](https://speckle.guide/dev/base.html)
- [Speckle Decomposition API](https://speckle.guide/dev/decomposition.html)
- [Speckle Transports](https://speckle.guide/dev/transports.html)
- [Speckle Custom Kits Development](https://speckle.guide/dev/kits-dev.html)
- [Speckle Developer Key Concepts](https://docs.speckle.systems/developers/key-concepts)
- [Why Speckle Doesn't Use IFC](https://speckle.community/t/why-doesnt-speckle-use-ifc-as-its-object-model/3790)
- [Speckle vs Autodesk Data Exchange](https://speckle.systems/blog/speckle-vs-autodesk-data-exchange/)
- [Speckle: Bridging AEC and Geospatial Data](https://www.speckle.systems/blog/speckle-bridging-aec-and-geospatial-data)
- [Speckle and IFC.js: Open Source Tools for BIM (AECbytes)](https://www.aecbytes.com/newsletter/2022/issue_113.html)
- [Speckle: The Open-Source Cloud Data Platform (AEC Magazine)](https://aecmag.com/features/speckle-the-open-source-cloud-data-platform/)
- [Speckle Next-Gen Connectors](https://speckle.systems/blog/speckles-next-gen-connectors-whats-in-it-for-you/)
- [Automate with Speckle](https://www.speckle.systems/blog/automate-with-speckle)
- [Speckle GitHub](https://github.com/specklesystems)
