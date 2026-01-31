---
id: 002-content-model
sidebar_position: 2
title: "ADR-002: Content Model"
---

# ADR-002: Part-Resource content model with nested layers

**Status:** Accepted

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

## Alternatives Considered

- **Inheritance hierarchy** (Okapi style): idiomatic in Java but requires
  type casts in Go; deep trees are hard to navigate.
- **Interface per resource type** (`BlockHandler`, `DataHandler`): requires
  type assertions at runtime; less discoverable.
- **Flat START/END events for embedded content** (Okapi style): loses
  hierarchical structure; tools must track nesting depth manually.
- **Separate sub-document channel**: overcomplicates the pipeline; breaks the
  single-stream model.

## Consequences

- Type dispatch via `switch part.Type` instead of instanceof; compile-time
  exhaustiveness with linters
- Adding new resource types requires only a new PartType constant and a struct
  implementing `Resource`
- Tools that only handle Blocks can ignore all other Part types via BaseTool's
  pass-through behavior
- The Part stream remains a single ordered channel; no fan-out complexity
- Format readers that detect embedded content must emit child Layers with the
  correct format identifier
