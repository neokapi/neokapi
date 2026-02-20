---
id: 002-content-model
sidebar_position: 2
title: "AD-002: Content Model"
---
# AD-002: Part-Resource content model with content-addressable identity

## Context

Okapi's content model uses a deep inheritance hierarchy rooted in `IResource`
with subtypes like `TextUnit`, `DocumentPart`, `StartDocument`, etc. Each event
type carries different fields, leading to type casts and instanceof checks
throughout filter and step code. Embedded content (HTML inside JSON, CDATA in
XML) is handled with flat `START_SUBDOCUMENT` / `END_SUBDOCUMENT` events where
nesting depth is tracked implicitly.

Go does not have class inheritance. The content model needed to work with Go's
composition and interface system while remaining type-safe, extensible, and
able to represent recursive embedded content naturally.

Beyond structural representation, real-world localization workflows demand
stable content identity across extraction cycles for incremental processing,
dynamic properties for extensible metadata, and display hints that guide UI
rendering without coupling the model to any particular frontend.

## Decision

### Part and Resource

Use a single `Part` struct carrying a `PartType` enum discriminator and a
`Resource` interface:

```go
type Part struct {
    Type     PartType
    Resource Resource  // Block, Layer, Data, or Media
}

type Resource interface {
    ResourceID() string
}
```

PartType values: `PartLayerStart`, `PartLayerEnd`, `PartBlock`, `PartData`,
`PartMedia`, `PartCustom`.

Resource types:

- **Layer** -- structural grouping (document, section, embedded content)
- **Block** -- translatable content with Source segments and Target segments
  per locale
- **Fragment** -- text with inline Spans using coded text (Unicode PUA markers
  replace inline markup)
- **Data** -- non-translatable structure (skeleton, metadata)
- **Media** -- binary content (images, embedded files)

The `PartResult{Part, Error}` tuple carries both content and errors on the
same channel, allowing tools to decide how to handle errors (skip, retry,
fail) without separate error channels.

### Block Identity

Content-addressable identity provides stable block identification across
extraction cycles. Rather than assigning opaque UUIDs that break when content
is re-extracted, identity is derived from the content itself:

```go
type BlockIdentity struct {
    ContentHash  string  // SHA-256 of normalized source text
    ContextHash  string  // Hash of surrounding context (prev/next blocks)
    SourcePath   string  // XPath, JSON path, or line number (display hint)
    SequenceNum  int     // Order in document
}
```

The `ContentHash` is computed from normalized source text (whitespace-normalized,
inline codes stripped). Combined with `ContextHash`, this provides stable
identity even when surrounding content changes -- same content always produces
the same identity.

This enables incremental extraction: only blocks whose identity has changed
since the last extraction cycle need reprocessing. It also enables
deduplication across documents -- identical blocks share the same
`ContentHash`, allowing translation memory and AI tools to avoid redundant
work (see [AD-003](./003-content-store.md)).

### Nested Layers for Embedded Content

Embedded content is modeled as nested Layers. A Layer carries its own
DataFormat identifier. When a format reader encounters embedded content (e.g.,
an HTML string inside a JSON value), it emits a child Layer with
`Format: "html"` containing the parsed HTML Blocks, nested between the parent
Layer's Parts:

```
PartLayerStart (format="json")
  PartBlock (key: "title")
  PartLayerStart (format="html")   <- embedded HTML
    PartBlock ("Hello <b>world</b>")
  PartLayerEnd (format="html")
  PartData (structural JSON)
PartLayerEnd (format="json")
```

Each Layer can be independently processed by format-aware tools. Layers nest
recursively: HTML in JSON in YAML is three levels deep with no special cases.

### Fragment and Coded Text

Text with inline formatting is represented as Fragments using coded text.
Unicode Private Use Area (PUA) markers replace inline markup (bold, italic,
links, etc.) within the text stream. Each marker maps to a `Span` that records
the original markup. This allows tools and translation engines to process plain
text with positional codes, reconstructing the original markup on output.
Fragment is the text-with-markup representation used throughout the system.

### Dynamic Properties

```go
type Block struct {
    ID          BlockIdentity
    Source      []Segment
    Targets     map[LocaleID][]Segment
    Annotations map[string]Annotation
    Properties  map[string]any  // extensible metadata
}
```

Properties carry arbitrary key-value metadata that tools and connectors attach
to blocks as they flow through the pipeline. Examples:

- `"translation-origin": "tm"` -- how the translation was produced
- `"segment-count": 3` -- number of segments in the block
- `"word-count": 42` -- word count from the wordcount tool
- `"cms-path": "/en/blog/post-1"` -- source location in a CMS

Properties are serialized in the Content Store and carried through the
pipeline. Tools can read and write properties without any content model
changes. This replaces the pattern of adding dedicated fields for every
new piece of metadata.

### Display Hints

```go
type DisplayHint struct {
    Preview     string // HTML preview for editor rendering
    Context     string // surrounding content for translator context
    MaxLength   int    // character limit for target text
    ContentType string // "heading", "button", "paragraph", "alt-text"
}
```

Display hints guide [Bowrain's](./012-bowrain.md) rendering
without coupling the content model to UI concerns. Connectors populate
display hints with metadata from the source system (e.g., a CMS connector
sets `MaxLength` from a field character limit). Format readers can set
`ContentType` based on structural analysis (an `<h1>` element becomes
`"heading"`, a `<button>` becomes `"button"`).

Display hints are advisory -- Bowrain uses them when available but renders
sensibly without them. This keeps the content model independent of any
particular frontend.

### ContentRef

```go
type ContentRef struct {
    ConnectorID string    // which connector produced this content
    ExternalID  string    // ID in the source system
    ExternalURL string    // link back to the source item
    SyncedAt    time.Time
}
```

ContentRef links blocks back to their origin system, enabling bidirectional
sync. When a connector pulls content, it sets the ContentRef. When pushing
translations back, the connector uses the ContentRef to locate the target
in the external system. This is central to the connector architecture
described in [AD-005](./005-connector-system.md).

### Annotation System

Blocks carry an `Annotations` map (`map[string]Annotation`) for attaching
metadata produced by pipeline tools. Each annotation type implements the
`Annotation` interface:

```go
type Annotation interface {
    AnnotationType() string
}
```

Built-in annotation types:

| Annotation          | Type Key           | Producer              | Purpose                               |
|---------------------|--------------------|-----------------------|---------------------------------------|
| `AltTranslation`    | `alt-translation`  | TM leverage, AI tools | Alternative translations with scores  |
| `TermAnnotation`    | `term`             | term-lookup tool      | Matched terminology with target terms |
| `EntityAnnotation`  | `entity`           | entity-annotate tool  | Named entities (people, places, dates)|

Annotations are keyed by type and instance (e.g., `"term:0"`, `"term:1"`)
to support multiple annotations of the same type per Block. Annotations
carry character-level positions via `TextRange` (start/end offsets within
source text). This enables precise inline highlighting in Bowrain without
re-detecting boundaries at render time. See [AD-010](./010-terminology.md)
for the full annotation data models.

## Alternatives Considered

- **Inheritance hierarchy** (Okapi style): idiomatic in Java but requires
  type casts in Go; deep trees are hard to navigate.
- **Flat START/END events for embedded content** (Okapi style): loses
  hierarchical structure; tools must track nesting depth manually.
- **UUID-based block identity**: doesn't survive re-extraction; content hash
  is stable across extraction cycles and enables deduplication.
- **Schema-driven properties**: over-constrains extensibility;
  `map[string]any` is simpler and sufficient for pipeline metadata.
- **Separate sub-document channel**: overcomplicates the pipeline; breaks the
  single-stream model that enables simple tool composition.

## Consequences

- Type dispatch via `switch part.Type` instead of instanceof; compile-time
  exhaustiveness with linters
- Adding new resource types requires only a new PartType constant and a struct
  implementing `Resource`
- Tools that only handle Blocks can ignore all other Part types via BaseTool's
  pass-through behavior
- The Part stream remains a single ordered channel; no fan-out complexity
- Content-addressable identity enables incremental extraction and deduplication
  ([AD-003](./003-content-store.md))
- Dynamic properties let tools and connectors carry metadata without content
  model changes
- Display hints guide UI rendering in
  [Bowrain](./012-bowrain.md) without coupling
- ContentRef enables bidirectional connector sync
  ([AD-005](./005-connector-system.md))
- The Annotation interface is open for extension -- new annotation types can
  be added by tools without modifying the content model
- `LocaleID` fields on Blocks and Layers hold BCP-47 tags validated by the
  `locale` package (see [AD-001](./001-vision.md))
- Format readers that detect embedded content must emit child Layers with the
  correct format identifier
  ([AD-001](./001-vision.md))
