---
id: 007-tool-system
sidebar_position: 7
title: "ADR-007: Tool System"
---

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
| **Validate**  | Check quality without modifying   | QA checks, word count, spell check        |
| **Convert**   | Transform representations         | Encoding conversion, line break normalization |

### Registration

Tools register into a `ToolRegistry` with a name and factory function,
mirroring the format registry pattern. The CLI and flow executor look up
tools by name.

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
