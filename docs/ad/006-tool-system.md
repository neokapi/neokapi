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

All built-in tools are registered via `RegisterAll()` in `core/tools/register.go`.

**Transform tools** — modify content in-place:

| Tool | Description |
|---|---|
| `pseudo-translate` | Generate pseudo-translations with accent marks and prefix/suffix wrapping |
| `search-replace` | Regex-based search and replace in content |
| `segmentation` | Split blocks into sentence segments using SRX-like rules |
| `case-transform` | Transform the case of source and/or target text |
| `create-target` | Create target segment containers for blocks |
| `remove-target` | Remove target segments from blocks |
| `inline-codes-remove` | Strip inline codes/spans from fragment content |
| `properties-set` | Set or modify properties on blocks programmatically |
| `whitespace-correct` | Normalize and fix whitespace issues in translations |
| `span-classify` | Reclassify `code:markup` spans into semantic vocabulary types |
| `tag-protect` | Identify and mark tags and placeholders for protection |
| `xslt-transform` | Apply regex-based tag/text transformations to block text |

**Enrich tools** — add metadata via annotations:

| Tool | Description |
|---|---|
| `tm-leverage` | Pre-fill translations from Sievepen TM ([AD-009](./009-translation-memory.md)) |
| `diff-leverage` | Compare blocks against previous version, preserve translations for unchanged text |
| `term-lookup` | Scan source text for known terms from TermBase ([AD-010](./010-terminology.md)) |
| `repetition-analysis` | Analyze source text repetitions across blocks in the pipeline |

**Validate tools** — check quality without modifying:

| Tool | Description |
|---|---|
| `word-count` | Count words per block |
| `char-count` | Count characters per block |
| `segment-count` | Count source and target segments in blocks |
| `qa-check` | Rule-based quality checks (missing translations, whitespace, numbers, span constraints) |
| `term-check` | Verify terminology usage in translations against a glossary |
| `term-enforce` | Validate preferred term usage in target text ([AD-010](./010-terminology.md)) |
| `inconsistency-check` | Check for translation inconsistencies across blocks |
| `length-check` | Verify translation length constraints (characters, words, ratio) |
| `chars-check` | Check for invalid or unexpected characters in translations |
| `pattern-check` | Validate regex patterns in translations (placeholders, variables) |
| `translation-comparison` | Compare translations across two target locales and report differences |
| `xml-validation` | Validate XML well-formedness of block text |
| `chars-listing` | List all unique characters used in content (for font subsetting) |
| `scoping-report` | Classify blocks into scoping categories based on repetition and match status |

**Convert tools** — transform representations:

| Tool | Description |
|---|---|
| `encoding-convert` | Convert character encoding of text content |
| `encoding-detect` | Detect encoding characteristics of block text |
| `linebreak-convert` | Normalize line endings in source and/or target text |
| `bom-convert` | Add or remove the Unicode BOM marker on document layers |
| `fullwidth-convert` | Convert between half-width and full-width characters |
| `uri-convert` | Encode or decode URI escape sequences in text |

**Pipeline tools** — operate on the part stream:

| Tool | Description |
|---|---|
| `layer-processor` | Apply format-specific tool chains to child layers ([AD-002](./002-content-model.md)) |
| `external-command` | Execute an external command on block text |
| `script` | Run user-provided JavaScript (ES5 via goja) on each part — filter, transform, or enrich |
| `batch` | Collect blocks into configurable batches for downstream batch processing |

### AI and MT Tools

AI and MT tools are instantiated on demand with a provider, not auto-registered.
They use the same `Tool` interface and work identically in flows.

**AI tools** (`core/ai/tools/`):

| Tool | Description |
|---|---|
| `ai-translate` | Translate blocks using an LLM provider (batch + concurrent) |
| `ai-qa` | Check translation quality using an LLM provider |
| `ai-review` | Review translations with explanations using an LLM |
| `ai-terminology` | Extract terminology from blocks using an LLM |
| `ai-entity-extract` | Extract named entities and term candidates using AI + optional NER |

**MT tools** (`core/mt/tools/`):

| Tool | Description |
|---|---|
| `{provider}-translate` | Translate blocks using an MT provider (DeepL, Google, Microsoft, ModernMT, MyMemory) |

**Terminology tools** (`core/termbase/`):

| Tool | Description |
|---|---|
| `term-lookup` | Annotate blocks with matching terms from a TermBase ([AD-010](./010-terminology.md)) |
| `term-enforce` | Verify correct terminology usage in translations ([AD-010](./010-terminology.md)) |

**TM tools** (`core/sievepen/`):

| Tool | Description |
|---|---|
| `tm-leverage` | Content-aware TM leverage with generalized, structural, and plain matching ([AD-009](./009-translation-memory.md)) |

### Annotation Flow Between Tools

Tools communicate through annotations on Blocks. A typical pipeline:

```
reader → ai-entity-extract → term-lookup → tm-leverage → ai-translate → term-enforce → qa-check → writer
```

- `ai-entity-extract` adds `EntityAnnotation` with named entities
- `term-lookup` adds `TermAnnotation` with matched terminology
- `tm-leverage` reads entity annotations for generalized matching, adds `AltTranslation`
- `ai-translate` reads term and entity annotations for context-aware translation
- `term-enforce` validates terminology consistency in targets
- `qa-check` validates translation quality

Annotations are attached to Blocks as typed metadata. Each tool reads the
annotations it cares about and adds its own, keeping tools loosely coupled
through a shared data model rather than direct dependencies.

### Tool Parameter Schemas

Tools declare their parameter schemas via the `tool.SchemaProvider` interface
([AD-040](./040-tool-parameter-schemas.md)). The `core/schema/` package provides
`ComponentSchema` — a generalized JSON Schema subset used by both tools and
format filters:

```go
type SchemaProvider interface {
    Schema() *schema.ComponentSchema
}
```

`schema.FromStruct(cfg, meta)` generates schemas from Go config structs via
reflection. The `ToolRegistry` stores schemas alongside factories via
`RegisterWithSchema()`. All 35+ built-in tools register auto-generated schemas.

Schema-driven features:
- **CLI flags**: `cli.RegisterSchemaFlags()` auto-generates cobra flags from schemas
- **Flow editor**: Schema-driven config panels for tool nodes in the visual editor
- **Validation**: `ComponentSchema.Validate()` checks parameter values against the schema
- **JSON export**: `kapi tools schema <name>` prints the schema for any tool

### Registration

Tools register into a `ToolRegistry` with a name, factory function, and optional
parameter schema, mirroring the format registry pattern
([AD-004](./004-processing-engine.md)). The CLI and flow executor look up tools
by name. `RegisterAll(reg)` in `core/tools/register.go` auto-registers all
built-in tools with default configurations and auto-generated schemas. AI, MT,
and terminology tools are instantiated on demand with their respective providers
and registered into the flow's tool set at configuration time.

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
