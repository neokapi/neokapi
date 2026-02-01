# ADR-007: Tool system with BaseTool dispatch

**Status:** Accepted

## Context

Most tools in a localization pipeline only care about one or two Part types.
A translation tool processes Blocks; a word counter reads Blocks; a binary
extractor handles Media. Requiring every tool to implement the full
`Process(ctx, in, out)` method with a type switch over all Part types leads
to repetitive boilerplate.

## Decision

Provide a `BaseTool` struct with optional function fields for each Part type:

```go
type BaseTool struct {
    HandleBlockFn func(ctx, *Block) (*Block, error)
    HandleDataFn  func(ctx, *Data) (*Data, error)
    HandleMediaFn func(ctx, *Media) (*Media, error)
}
```

The `BaseTool.Process` method reads Parts from the input channel, dispatches
to the appropriate handler function if set, and passes unhandled Parts through
to the output channel unchanged. Concrete tools embed `BaseTool` and set only
the handlers they need.

Tools that need access to the full stream (e.g., segmentation across Blocks)
can override `Process` directly.

### Tool Categories

| Category      | Responsibility                    | Examples                                  |
|---------------|-----------------------------------|-------------------------------------------|
| **Transform** | Modify content in-place           | Segmentation, case change, search/replace |
| **Enrich**    | Add metadata                      | TM leveraging, AI translation, terminology |
| **Validate**  | Check quality without modifying   | QA checks, word count, character count     |
| **Convert**   | Transform representations         | Encoding conversion, line break normalization |

### Registration

Tools register into a `ToolRegistry` with a name and factory function,
mirroring the format registry pattern. The CLI and flow executor look up
tools by name.

### Built-in Action Tools

Three built-in action tools provide core localization pipeline functionality:

**Segmentation** (`segmentation`) — Category: Transform

Splits Block source text into sentence segments using SRX-like regex rules.
Each `SegmentationRule` defines a `BeforeBreak` regex, an `AfterBreak` regex,
and an `IsBreak` flag indicating whether the match point is a segment boundary.
Default rules handle:

- Common abbreviations (Mr., Mrs., Dr., etc.) — non-breaking
- Single-letter initials (e.g., "J.") — non-breaking
- Sentence-ending punctuation (`.`, `?`, `!`) followed by whitespace and a
  capital letter — breaking

Sets the `segment-count` property on each processed Block.

**QA Check** (`qa-check`) — Category: Validate

Validates translations with configurable checks:

- **Missing translations** — source text present but target is empty
- **Mismatched leading/trailing whitespace** — whitespace differs between
  source and target
- **Mismatched numbers** — numeric values in source not found in target

Returns per-block annotations with issue details (check name, severity,
message). Supports configurable check names to enable or disable individual
checks.

**TM Leverage** (`tm-leverage`) — Category: Enrich

Pre-fills translations from translation memory. Searches the Pensieve TM
for exact and fuzzy matches above a configurable threshold (default 0.7).
Exact matches (score >= 0.99) and fuzzy matches are distinguished. Sets
`translation-origin: tm` property on matched Blocks, allowing downstream
tools and editors to identify TM-sourced translations.

### Tool Registry

The `RegisterTools(reg)` function auto-registers all built-in utility tools
into the provided `ToolRegistry`. Default tool configurations are used;
users can customize tool settings via `gokapi.yaml`.

Registered built-in tools:

| Tool               | Category   |
|--------------------|------------|
| `wordcount`        | Validate   |
| `charcount`        | Validate   |
| `pseudo-translate` | Transform  |
| `search-replace`   | Transform  |
| `segmentation`     | Transform  |
| `qa-check`         | Validate   |
| `tm-leverage`      | Enrich     |

## Alternatives Considered

- **Interface per Part type** (e.g., `BlockHandler`, `DataHandler`): requires
  type assertions at runtime; less discoverable.
- **Visitor pattern**: idiomatic in Java but awkward in Go; more abstraction
  than needed.
- **Full Process override only**: always available as an escape hatch but
  too much boilerplate for the common case.

## Consequences

- Implementing a new tool requires only embedding BaseTool and setting one
  function field
- Unhandled Part types pass through automatically; no risk of accidentally
  dropping Parts
- Function fields are set at construction time; no interface satisfaction
  ceremony
- Categories guide tool design and set expectations for idempotency and
  ordering
