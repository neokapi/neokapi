---
sidebar_position: 4
title: Implementing a Tool
description: How to build a neokapi Tool — embedding BaseTool, setting handler functions for the Part types you care about, and passing unhandled Parts through the pipeline unchanged.
keywords: [tool implementation, BaseTool, Part, handler, pipeline, Go, neokapi, processing]
---

# Implementing a New Tool

Tools process Parts as they flow through a pipeline. Most tools only care about one or two Part types (usually Blocks).

## Using BaseTool

Build a `tool.BaseTool` and set handler function fields for the Part types you
want to process. Parts you don't handle pass through unchanged. There are two
families of handler.

For **Block** parts, set exactly ONE capability-typed handler — the parameter
type bounds what the tool may write (immutability model, AD-006):

- `Annotate(tool.BlockView) error` — read-only; writes only overlays,
  annotations, and properties.
- `Translate(tool.TargetView) error` — reads source, writes target.
- `Transform(tool.BlockView) (tool.EditPlan, error)` — a read-only edit
  producer: returns an edit plan, and the framework applier rewrites the
  source — rebasing surviving overlays, vaulting secrets, and bounds-checking,
  atomically. The flow's placement pass validates where a transformer may sit.

For the non-Block parts (Data, Media, Layer/Group start/end), set the untyped
`Handle*Fn` fields, which use `tool.PartHandler` =
`func(part *model.Part) (*model.Part, error)`: these receive the streaming Part
and type-assert the resource they care about.

```go
package mytool

import (
    "strings"

    "github.com/neokapi/neokapi/core/model"
    "github.com/neokapi/neokapi/core/tool"
)

func NewUppercaseTool() *tool.BaseTool {
    t := &tool.BaseTool{
        ToolName:        "uppercase",
        ToolDescription: "Converts source text to uppercase",
    }
    // Writes a target, so it sets Translate (the view bounds it to target writes).
    t.Translate = func(v tool.TargetView) error {
        if !v.Translatable() {
            return nil
        }
        v.SetTargetText(model.LocaleEnglish, strings.ToUpper(v.SourceText()))
        return nil
    }
    return t
}
```

## Tool Categories

| Category      | Responsibility                  | Examples                                      |
| ------------- | ------------------------------- | --------------------------------------------- |
| **Transform** | Modify content in-place         | case change, search/replace, redaction        |
| **Enrich**    | Add metadata or overlays        | segmentation, TM leveraging, AI translation, terminology |
| **Validate**  | Check quality without modifying | QA checks, word count, spell check            |
| **Convert**   | Transform representations       | Encoding conversion, line break normalization |

## Overriding Process

If you need full control over the processing loop (for example, to accumulate
state across many Parts, or to emit more Parts than you consume), define a named
type that embeds `tool.BaseTool` and override `Process` directly:

```go
type MyTool struct {
    tool.BaseTool
}

func (t *MyTool) Process(ctx context.Context, in <-chan *model.Part, out chan<- *model.Part) error {
    for {
        select {
        case <-ctx.Done():
            return ctx.Err()
        case part, ok := <-in:
            if !ok {
                return nil
            }
            // Custom processing logic
            out <- part
        }
    }
}
```

## Registration

Register your tool in a `ToolRegistry`, mapping a name to a factory:

```go
reg := registry.NewToolRegistry()
reg.Register("uppercase", func() tool.Tool {
    return NewUppercaseTool()
})
```

Use `RegisterWithSchema` instead to attach a parameter schema — see
[Tool Authoring](/contribute/tool-authoring).

## Built-in Tools

The framework's built-in tools are registered with their parameter schemas. The
authoritative, generated list of what ships in the current build — every tool's
name, description, and parameters — is the [Tool Reference](/tools), rendered
from those schemas so it always matches the build. This guide deliberately does
not restate it; for how the built-ins map to the kinds of work above, see
[Tools](/framework/tools).

### Schema-Driven CLI Flags

All built-in tools use schema-driven CLI flags. Tool config structs use `schema:"..."` tags to auto-generate flags from the struct fields. Use `schema:"-"` to exclude a field from flag generation. The `NewToolFromConfig` pattern allows the flow engine to instantiate tools from YAML configuration by mapping config keys to struct fields automatically.

### Registering Built-in Tools

All built-in tools can be registered into a registry at once, each with its
parameter schema:

```go
import (
    "github.com/neokapi/neokapi/core/registry"
    "github.com/neokapi/neokapi/core/tools"
)

toolReg := registry.NewToolRegistry()
tools.RegisterAll(toolReg)
```

Individual tools can also be constructed directly. Each takes a config struct
(see the [Tool Reference](/tools) for every field):

```go
// Segmentation with default SRX-like rules
segTool := tools.NewSegmentationTool(&tools.SegmentationConfig{})

// QA check — configured via per-rule flags on QACheckConfig
qaTool := tools.NewQACheckTool(tools.NewQACheckConfig(model.LocaleID("fr")))

// TM leverage with a custom fuzzy threshold and a TM provider
tmTool := tools.NewTMLeverageTool(&tools.TMLeverageConfig{
    TargetLocale:   "fr",
    FuzzyThreshold: 80, // 0-100
    Provider:       tmProvider,
})
```

The terminology tools live in the `termbase` package and take a `TermBase`
alongside their config:

```go
import "github.com/neokapi/neokapi/termbase"

// Term lookup — scans source text and attaches terminology annotations
termLookupTool := termbase.NewTermLookupTool(tb, termbase.TermLookupConfig{
    SourceLocale: "en",
    TargetLocale: "fr",
})

// Term enforce — verifies translations use the preferred terminology
termEnforceTool := termbase.NewTermEnforceTool(tb, termbase.TermEnforceConfig{
    SourceLocale: "en",
    TargetLocale: "fr",
})
```
