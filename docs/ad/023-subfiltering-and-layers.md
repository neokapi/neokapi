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
  PartLayerStart (format="html", id="layer2", parentID="doc1")
    PartBlock ("Welcome to <b>our site</b>")
  PartLayerEnd (id="layer2")
  PartData (structural JSON)
PartLayerEnd (id="doc1")
```

**No native format reader currently emits child layers** — all emit only a
document-level root layer. The bridge protocol supports `LayerMessage` with
`parent_id`, but Okapi's subfiltered content arrives as a flat Part stream
with lost layer provenance. The flow executor passes layers through without
recursive processing.

## Decision

Implement subfiltering in three phases: (1) bridge subfilter roundtrip via
Java-side fix, (2) native Go reader/writer subfiltering via the Layer model,
(3) optional layer-aware flow processing for format-specific tool chains.

### Phase 1: Bridge subfilter roundtrip

**Problem.** Okapi's write phase needs a `FilterConfigurationMapper` to
reconstruct subfiltered content. The bridge protocol already sends
`filter_params` in both `OpenRequest` and `WriteHeader`, but the Java
bridge's Write handler doesn't reconstruct the mapper from those params.

**Fix.** Java-side only — in the bridge server's Write RPC handler,
reconstruct the `FilterConfigurationMapper` from `filter_params` before
calling the Okapi filter writer. The params already contain everything needed:
`global_cdata_subfilter`, `configFile` paths, element-level subfilter
mappings. No protocol changes, no Go-side changes.

**Verification.** Extend the existing read-only subfilter tests in
`okf_xmlstream/subfilter_test.go` with roundtrip assertions. Similarly for
`okf_json`, `okf_multiparsers`, and any other filter with subfilter config.

### Phase 2: Native subfiltering via Layers

#### 2a. SubfilterResolver interface

```go
// core/format/subfilter.go

// SubfilterResolver creates format readers/writers for embedded content.
// Format readers that support subfiltering receive this via SetSubfilterResolver.
type SubfilterResolver interface {
    ResolveReader(formatName string) (DataFormatReader, error)
    ResolveWriter(formatName string) (DataFormatWriter, error)
}
```

`FormatRegistry` naturally implements this via its existing `NewReader` /
`NewWriter` methods. The interface decouples format readers from the registry,
enabling test mocks and preventing circular imports.

#### 2b. SubfilterMapping configuration

Format configs gain optional subfilter mappings:

```go
// core/format/subfilter.go

// SubfilterMapping maps content locations to a format reader for subfiltering.
type SubfilterMapping struct {
    Pattern string // key path glob (JSON), element name (XML), column index (CSV)
    Format  string // format reader name: "html", "markdown", etc.
}
```

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
cdata_subfilter: html        # global: all CDATA → html
element_subfilters:
  - pattern: "content"
    format: markdown
```

These map directly to Okapi's existing subfilter config patterns but use
gokapi's format names, enabling any registered format (including bridge
formats and plugins) as a subfilter.

#### 2c. Reader: emit child Layers

When a format reader encounters content matching a subfilter pattern:

1. Emit `PartLayerStart` with child `Layer{ParentID, Format, ...}`
2. Create sub-reader via `SubfilterResolver.ResolveReader(format)`
3. Open sub-reader with embedded content as a `RawDocument`
4. Read all parts from sub-reader, emit on same channel
5. Emit `PartLayerEnd` for child layer
6. Close sub-reader

**Inline expansion.** The parent reader expands subfiltered content inline,
producing a single part stream with nested layer markers. This means:

- Existing tools work unchanged — they see all blocks from all nesting levels
- No flow executor changes needed for basic subfiltering to work
- Writers can reconstruct the original structure from layer boundaries

**Example for JSON reader** (`core/formats/json/reader.go`):

```go
// In walkValue, when emitting a string block for key "article.body":
if mapping := r.matchSubfilter(path); mapping != nil {
    r.emitSubfiltered(ctx, ch, value, path, mapping, blockCounter)
} else {
    r.emitBlock(ctx, ch, value, path, blockCounter)
}
```

`emitSubfiltered` creates a child layer, delegates to the resolved sub-reader,
and emits the child's parts bracketed by `PartLayerStart` / `PartLayerEnd`.

**Recursion.** The sub-reader itself may have subfilter config. Since each
reader gets its own `SubfilterResolver`, nesting is naturally recursive:
HTML-in-JSON-in-YAML is three reader instances, each emitting child layers.

#### 2d. Writer: reconstruct from child Layers

When a format writer encounters `PartLayerStart` with a child format:

1. Buffer all parts until matching `PartLayerEnd`
2. Create sub-writer via `SubfilterResolver.ResolveWriter(layer.Format)`
3. Write buffered parts through sub-writer to an in-memory buffer
4. Use buffer contents as the string value in the parent format

**Example for JSON writer:** encountering `PartLayerStart(format="html")`
inside a JSON key, it buffers the child parts, writes them through an HTML
writer, and inserts the resulting HTML string as the JSON value.

### Phase 3: Layer-aware flow processing (future)

Phase 2 gives format-level subfiltering: the reader fully expands embedded
content, tools see a flat block stream, and the writer reconstructs it. This
already exceeds Okapi's capabilities (any format as a subfilter).

Phase 3 goes further: **format-specific tool chains per layer**. Instead of
applying the same tools to all blocks regardless of their enclosing format,
the executor applies different tool configurations to different layers:

- HTML-specific QA rules only to HTML content
- JSON schema validation only to JSON structure
- Different MT prompts for code comments vs. UI strings

**LayerProcessor tool:**

```go
// A tool that intercepts child layers and applies a sub-pipeline.
type LayerProcessor struct {
    tool.BaseTool
    resolver  format.SubfilterResolver
    pipelines map[string][]tool.Tool  // format name → tool chain
}
```

When it encounters `PartLayerStart(format="html")`, it buffers parts until
`PartLayerEnd`, runs them through the HTML-specific tool chain, and emits
the processed parts.

**Executor option:**

```go
func WithLayerProcessing(resolver format.SubfilterResolver) ExecutorOption
```

This is additive — the base executor continues to work for all existing
use cases. Layer-aware processing is opt-in via flow configuration.

## Implementation order

```
Phase 1 ─── Java bridge Write handler fix (independent)
             └─ Subfilter roundtrip tests pass

Phase 2a ── SubfilterResolver interface
Phase 2b ── SubfilterMapping config type
Phase 2c ── JSON reader: first native subfilter implementation (HTML-in-JSON)
Phase 2d ── JSON writer: reconstruct HTML-in-JSON
             └─ Full native roundtrip for HTML-in-JSON

Phase 2e ── XML reader/writer subfiltering (CDATA, PCDATA)
Phase 2f ── Additional formats: CSV, Markdown, YAML, PO

Phase 3 ─── LayerProcessor tool + executor option (future)
```

Phases 1 and 2 are independent. Phase 2a–2d form the core — once JSON works,
other formats follow the same pattern (2e, 2f). Phase 3 is an enhancement
with no prerequisites beyond 2a.

## Consequences

**Positive:**
- Any registered format can be a subfilter for any other — not limited to
  Okapi's predefined filter configurations
- Layers nest recursively with no special cases — the content model handles it
- Existing tools work unchanged for Phase 2 (inline expansion)
- Phase 3 enables format-aware tool chains without changing the content model
- Bridge filters continue to work via Okapi's internal subfilter mechanism

**Negative:**
- Phase 2c adds complexity to format readers (subfilter detection, sub-reader
  lifecycle management)
- Writer-side buffering (Phase 2d) uses memory proportional to embedded content
  size — acceptable since embedded content is typically small
- Phase 3's LayerProcessor adds pipeline complexity — but it's opt-in

**Neutral:**
- No content model changes — the existing Layer, Part, and Block types suffice
- No bridge protocol changes — Phase 1 is Java-only, Phase 2 is Go-only
- `SubfilterResolver` interface prevents coupling between format and registry
  packages
