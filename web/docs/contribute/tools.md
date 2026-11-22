---
sidebar_position: 4
title: Implementing a Tool
description: "How to build a neokapi Tool: embedding BaseTool, setting handler functions for the Part types you care about, and passing unhandled Parts through the pipeline unchanged."
keywords: [tool implementation, BaseTool, Part, handler, pipeline, Go, neokapi, processing]
---

# Implementing a New Tool

A tool reads the Parts of a run and acts on the ones it handles, usually
Blocks. You build it on `tool.BaseTool`, set a handler for each Part type you
want, and register it.

## Using BaseTool

Build a `tool.BaseTool` and set handler function fields for the Part types you
want to process. Parts you don't handle pass through unchanged. There are two
families of handler.

For **Block** parts, set exactly ONE capability-typed handler: the parameter
type bounds what the tool may write (immutability model, E-03):

- `Annotate(tool.BlockView) error`: read-only; writes only overlays,
  annotations, and properties.
- `Produce(tool.VariantView) error`: reads source, writes target.
- `Transform(tool.BlockView) (tool.EditPlan, error)`: a read-only edit
  producer: returns an edit plan, and the framework applier rewrites the
  source, rebasing surviving overlays, vaulting secrets, and bounds-checking,
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
    // Writes a target, so it sets Produce (the view bounds it to target writes).
    t.Produce = func(v tool.VariantView) error {
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
| **Enrich**    | Add metadata or overlays        | segmentation, content-memory leveraging, AI translation, terminology |
| **Validate**  | Check quality without modifying | rule-based checks, word count, spell check    |
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

Use `RegisterWithSchema` instead to attach a parameter schema, and
`SetConfigFactory` so a YAML step's `config:` reaches the tool; see
[Tool Authoring with Parameter Schemas](/contribute/tool-authoring).

## Built-in Tools

The framework's built-in tools are registered with their parameter schemas. The
[Tool Reference](/tools) is generated from those schemas and lists every tool
in the current build with its name, description and parameters. For how the
built-ins map to the kinds of work above, see [Tools](/framework/tools).

### Schema-Driven CLI Flags

Built-in tools take their CLI flags from their config structs: each field with a
`schema:"..."` tag becomes a flag, and `schema:"-"` excludes a field. The
`NewToolFromConfig` pattern lets the flow engine build a tool from YAML
configuration by mapping config keys to struct fields.

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

// Rule-based check: configured via per-rule flags on RuleCheckConfig
checkTool := tools.NewRuleCheckTool(tools.NewRuleCheckConfig(model.LocaleID("fr")))

// Content-memory leverage with a custom fuzzy threshold and a memory provider
memoryTool := tools.NewMemoryLeverageTool(&tools.MemoryLeverageConfig{
    TargetLocale:   "fr",
    FuzzyThreshold: 80, // 0-100
    Memory:         memoryProvider,
})
```

The `terms` package offers two library tools for embedding in Go, each taking a
`Terminology` alongside its config. Neither is a registered flow tool: the tool a
YAML flow or `kapi exec` names is `term-check`, and when a tool's schema
declares that it requires a terms store, the runner appends `term-lookup` and
`term-enforce` after it with the run's locales.

```go
import "github.com/neokapi/neokapi/terms"

// Term lookup: scans source text and attaches terminology annotations
termLookupTool := terms.NewTermLookupTool(tb, terms.TermLookupConfig{
    SourceLocale: "en",
    TargetLocale: "fr",
})

// Term enforce: verifies translations use the preferred terminology
termEnforceTool := terms.NewTermEnforceTool(tb, terms.TermEnforceConfig{
    SourceLocale: "en",
    TargetLocale: "fr",
})
```

## Related

- [Tool Authoring with Parameter Schemas](/contribute/tool-authoring): declaring parameters and registering with a schema.
- [E-03: The tool system](/contribute/architecture/engine/e-03-tool-system): handler views, placement and side effects.
- [Interface Reference](/contribute/interfaces): the `Tool` interface and `BaseTool`.
