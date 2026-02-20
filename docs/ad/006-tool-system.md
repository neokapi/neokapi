---
id: 006-tool-system
sidebar_position: 6
title: "AD-006: Tool System"
---
# AD-006: Tool system with BaseTool dispatch

## Context

Most tools in a localization pipeline only care about one or two Part types.
A translation tool processes Blocks; a word counter reads Blocks; a binary
extractor handles Media. Requiring every tool to implement the full
`Process(ctx, in, out)` method with a type switch over all Part types leads
to repetitive boilerplate.

The streaming pipeline ([AD-004](./004-processing-engine.md)) delivers Parts
through channels. Tools sit between reader and writer stages, processing Parts
as they flow through. The content model ([AD-002](./002-content-model.md))
defines the Part types that tools operate on: Blocks for translatable content,
Data for structural elements, and Media for binary content.

## Decision

### BaseTool with Function Fields

Provide a `BaseTool` struct with optional function fields for each Part type:

```go
type BaseTool struct {
    HandleBlockFn func(ctx context.Context, block *Block) (*Block, error)
    HandleDataFn  func(ctx context.Context, data *Data) (*Data, error)
    HandleMediaFn func(ctx context.Context, media *Media) (*Media, error)
}
```

`BaseTool.Process` reads Parts from the input channel, dispatches to the
appropriate handler function if set, and passes unhandled Parts through to the
output channel unchanged. Concrete tools embed `BaseTool` and set only the
handlers they need.

Tools that need access to the full stream (e.g., segmentation across Blocks)
can override `Process` directly.

### Tool Categories

| Category | Responsibility | Examples |
|---|---|---|
| **Transform** | Modify content in-place | Segmentation, case change, search/replace |
| **Enrich** | Add metadata via annotations | TM leveraging, AI translation, terminology lookup |
| **Validate** | Check quality without modifying | QA checks, word count, character count |
| **Convert** | Transform representations | Encoding conversion, line break normalization |

### Built-in Tool Inventory

| Tool | Category | Description |
|---|---|---|
| `wordcount` | Validate | Count words per block (also exposed as `Block.WordCount()`) |
| `charcount` | Validate | Count characters per block |
| `pseudo-translate` | Transform | Generate pseudo-translations for i18n testing |
| `search-replace` | Transform | Regex-based search and replace in content |
| `segmentation` | Transform | Split blocks into sentence segments using SRX-like rules |
| `qa-check` | Validate | Check translations (missing, whitespace, numbers) |
| `tm-leverage` | Enrich | Pre-fill translations from Sievepen TM ([AD-009](./009-translation-memory.md)) |
| `term-lookup` | Enrich | Scan source text for known terms from TermBase ([AD-010](./010-terminology.md)) |
| `term-enforce` | Validate | Validate preferred term usage in target text ([AD-010](./010-terminology.md)) |
| `entity-annotate` | Enrich | Annotate named entities (people, places, dates) ([AD-010](./010-terminology.md)) |
| `redact` | Transform | Replace entity values with placeholders for privacy |
| `unredact` | Transform | Restore original values after external processing |

### Annotation Flow Between Tools

Tools communicate through annotations on Blocks. A typical pipeline:

```
reader → entity-annotate → term-lookup → tm-leverage → ai-translate → term-enforce → qa-check → writer
```

- `entity-annotate` adds `EntityAnnotation` with named entities
- `term-lookup` adds `TermAnnotation` with matched terminology
- `tm-leverage` reads entity annotations for generalized matching, adds `AltTranslation`
- `ai-translate` reads term and entity annotations for context-aware translation
- `term-enforce` validates terminology consistency in targets
- `qa-check` validates translation quality

Annotations are attached to Blocks as typed metadata. Each tool reads the
annotations it cares about and adds its own, keeping tools loosely coupled
through a shared data model rather than direct dependencies.

### Registration

Tools register into a `ToolRegistry` with a name and factory function,
mirroring the format registry pattern ([AD-004](./004-processing-engine.md)).
The CLI and flow executor look up tools by name. The `RegisterTools(reg)`
function auto-registers all built-in utility tools with default configurations;
users can customize tool settings via `gokapi.yaml`.

Plugin tools ([AD-007](./007-plugin-system.md)) use the same `Tool` interface
via gRPC translation, so plugin-provided tools and built-in tools are
interchangeable from the pipeline's perspective.

## Alternatives Considered

- **Interface per Part type** (e.g., `BlockHandler`, `DataHandler`): requires
  type assertions at runtime; less discoverable.
- **Visitor pattern**: idiomatic in Java but awkward in Go; more abstraction
  than needed.
- **Full Process override only**: always available as an escape hatch but too
  much boilerplate for the common case.

## Consequences

- Implementing a new tool requires only embedding `BaseTool` and setting one
  function field
- Unhandled Part types pass through automatically; no risk of accidentally
  dropping Parts
- Function fields are set at construction time; no interface satisfaction
  ceremony
- Categories guide tool design and set expectations for idempotency and
  ordering
- Annotation-based inter-tool communication keeps tools loosely coupled
- Plugin tools ([AD-007](./007-plugin-system.md)) use the same interface via
  gRPC translation, so the pipeline treats all tools uniformly
