---
id: 023-subfiltering-and-layers
sidebar_position: 23
title: "AD-023: Subfiltering and Nested Layers"
---
# AD-023: Subfiltering and nested layers for embedded content

## Context

Documents frequently contain embedded content in a different format: HTML
strings inside JSON values, HTML in CDATA sections of XML, Markdown in CSV
columns, HTML in XLIFF notes. These need format-aware extraction — a JSON
reader that only sees `"<p>Hello <b>world</b></p>"` as a flat string misses
the inline formatting and produces inferior translation results.

Okapi Framework solves this with **subfilters**: a filter can delegate content
to another filter via `FilterConfigurationMapper`. The parent filter fires
`START_SUBDOCUMENT` / `END_SUBDOCUMENT` events, and the subfilter produces
TextUnits within that boundary. This works but has limitations:

- Subfilters must be Okapi filters registered in the same Java process
- Configuration is format-specific (each filter has its own subfilter params)
- The subfilter mechanism is invisible to the pipeline — steps don't know
  content came from a subfilter and can't apply format-specific processing

gokapi's content model already defines the right abstraction: **nested Layers**
([AD-002](./002-content-model.md)). A Layer carries a `Format` identifier and
a `ParentID` linking it to its enclosing layer. This creates a recursive tree:

```
PartLayerStart (format="json", id="doc1")
  PartBlock (key: "title", text: "Hello")
  PartLayerStart (format="html", id="sf1", parentID="doc1")
    PartBlock ("Welcome to <b>our site</b>")
  PartLayerEnd (id="sf1")
  PartData (structural JSON)
PartLayerEnd (id="doc1")
```

## Decision

### SubfilterResolver interface

```go
// core/format/subfilter.go

type SubfilterResolver interface {
    ResolveReader(formatName string) (DataFormatReader, error)
    ResolveWriter(formatName string) (DataFormatWriter, error)
}
```

`FormatRegistry` implements this via its existing `NewReader` / `NewWriter`
methods. The interface decouples format readers from the registry, preventing
circular imports and enabling test mocks.

Readers and writers that support subfiltering implement the `SubfilterAware`
marker interface:

```go
type SubfilterAware interface {
    SetSubfilterResolver(r SubfilterResolver)
}
```

The resolver is injected before `Open` / `Write` is called. Any registered
format (including bridge formats and plugins) can serve as a subfilter.

### SubfilterMapping configuration

Format configs declare subfilter mappings that bind content locations to a
format reader:

```go
type SubfilterMapping struct {
    Pattern string // content location pattern (format-specific syntax)
    Format  string // format name: "html", "markdown", etc.
}
```

Pattern syntax varies by parent format:

- **JSON:** key path glob — `"*.body"`, `"translations.*.html"`
- **XML:** element path — `"root.body"`, `"root.*.content"`

Patterns use `filepath.Match` semantics with `.` as the path separator.

**JSON example:**
```yaml
subfilters:
  - pattern: "*.body"
    format: html
  - pattern: "*.description"
    format: markdown
```

**XML example:**
```yaml
subfilters:
  - pattern: "root.*.body"
    format: html
```

### Reader: emit child Layers

When a format reader encounters content matching a subfilter pattern:

1. Emit `PartLayerStart` with child `Layer{ParentID, Format, ...}`
2. Create sub-reader via `SubfilterResolver.ResolveReader(format)`
3. Open sub-reader with embedded content as a `RawDocument`
4. Read all parts from sub-reader, emit on same channel
5. Emit `PartLayerEnd` for child layer
6. Close sub-reader

The parent reader stores subfilter provenance in layer properties:
`subfilter.source` (parent format) and `subfilter.keyPath` or
`subfilter.elementPath` (the matched pattern location).

**Inline expansion.** The parent reader expands subfiltered content inline,
producing a single part stream with nested layer markers. This means:

- Existing tools work unchanged — they see all blocks from all nesting levels
- Writers can reconstruct the original structure from layer boundaries
- No flow executor changes needed for basic subfiltering

If the sub-reader cannot be resolved (format not registered), the reader falls
back to emitting a plain block with the raw string.

**Recursion.** The sub-reader itself may have subfilter config. Since each
reader gets its own `SubfilterResolver`, nesting is naturally recursive:
HTML-in-JSON-in-YAML is three reader instances, each emitting child layers.

### Writer: reconstruct from child Layers

When a format writer encounters `PartLayerStart` with `layer.IsEmbedded()`:

1. Buffer all parts until matching `PartLayerEnd`
2. Create sub-writer via `SubfilterResolver.ResolveWriter(layer.Format)`
3. Write buffered parts through sub-writer to an in-memory buffer
4. Use buffer contents as the string value in the parent format

For example, the JSON writer encountering `PartLayerStart(format="html")`
buffers the child parts, writes them through an HTML writer, and inserts the
resulting HTML string as the JSON value.

### LayerProcessor tool

The `layer-processor` tool ([AD-006](./006-tool-system.md)) applies
format-specific tool chains to child layers. This enables different processing
for different embedded formats without changing the flow executor:

```go
type LayerProcessorConfig struct {
    Pipelines map[string][]tool.Tool // format name → tool chain
}
```

When it encounters a `PartLayerStart` with `layer.IsEmbedded()`:

1. Buffer all child parts until `PartLayerEnd`
2. Look up the pipeline for the layer's format
3. If found, run buffered parts through the tool chain sequentially
4. Emit the processed (or unchanged) parts bracketed by layer markers

Parts outside child layers pass through unchanged. Layers whose format has
no configured pipeline also pass through unchanged. This is opt-in — flows
that don't need format-specific processing simply omit the tool.

**Use cases:**

- HTML-specific QA rules applied only to HTML content embedded in JSON
- Different MT prompts for code comments vs. UI strings
- Format-specific terminology enforcement per embedded layer

### Bridge subfilter roundtrip

Bridge filters use Okapi's internal `FilterConfigurationMapper` for
subfiltering. The bridge protocol carries `filter_params` in both
`OpenRequest` and `WriteHeader`, which contain subfilter configuration
(global CDATA subfilter, element-level subfilter mappings, config file
paths). The Java bridge reconstructs the mapper from these params for
both read and write operations.

### Format support

JSON and XML readers/writers implement `SubfilterAware` and support
subfilter mappings. Additional formats (CSV, Markdown, YAML, PO) follow
the same pattern: add `Subfilters []SubfilterMapping` to the format config,
implement `SubfilterAware`, and delegate to the resolved sub-reader/writer
when patterns match.

## Consequences

**Positive:**
- Any registered format can be a subfilter for any other — not limited to
  Okapi's predefined filter configurations
- Layers nest recursively with no special cases — the content model handles it
- Existing tools work unchanged with inline expansion
- LayerProcessor enables format-aware tool chains without changing the
  content model or flow executor
- Bridge filters continue to work via Okapi's internal subfilter mechanism

**Negative:**
- Subfilter support adds complexity to format readers (sub-reader lifecycle,
  pattern matching, fallback on resolution failure)
- Writer-side buffering uses memory proportional to embedded content size —
  acceptable since embedded content is typically small
- LayerProcessor buffers entire child layers in memory for pipeline processing

**Neutral:**
- No content model changes — the existing Layer, Part, and Block types suffice
- `SubfilterResolver` interface prevents coupling between format and registry
  packages
