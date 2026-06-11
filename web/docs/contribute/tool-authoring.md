---
sidebar_position: 3
title: Tool Authoring with Parameter Schemas
description: How to build a neokapi tool with a JSON Schema parameter definition so the CLI and desktop UI can auto-generate configuration forms — embedding BaseTool, setting handler functions, and registering with the tool registry.
keywords: [tool authoring, BaseTool, parameter schema, JSON Schema, Go, neokapi, pipeline stage]
---

# Creating Tools with Parameter Schemas

This guide covers how to create a tool with a parameter schema so that the UI and CLI can auto-generate configuration forms and validate user input.

## Tool basics

Every tool is built on `tool.BaseTool`. For Blocks — the translatable unit — a
tool sets exactly one capability-typed handler, and the view it receives bounds
what it may write (AD-006): `Annotate(BlockView)` reads source and target and
writes only overlays, annotations, and properties; `Translate(TargetView)`
writes the target; `Transform(BlockView)` returns an edit plan that the
framework applier uses to rewrite the source. The wrong writes
are simply not on the view — an annotator has no target setter to call, and a
transformer holds no source setter at all. Other
Part types (Data, Media, Layer, Group) use the untyped `Handle*Fn` fields. Parts
you don't handle pass through unchanged; a handler returns an `error` (and may
call `v.Drop()` to remove the block from the stream).

```go
package mytool

import (
    "strings"

    "github.com/neokapi/neokapi/core/model"
    "github.com/neokapi/neokapi/core/tool"
)

func NewMyTool(cfg *MyToolConfig) *tool.BaseTool {
    t := &tool.BaseTool{
        ToolName:        "my-tool",
        ToolDescription: "Does something useful",
        Cfg:             cfg,
    }
    // A tool declares its capability by which block handler it sets — the
    // parameter type bounds what it may write (AD-006):
    //   Annotate(BlockView)  — read-only: overlays / annotations / properties
    //   Translate(TargetView) — writes the target; source stays read-only
    //   Transform(BlockView) — edit producer: returns an EditPlan the
    //                          framework applier applies to the source
    // This tool writes a target, so it sets Translate.
    t.Translate = func(v tool.TargetView) error {
        if !v.Translatable() {
            return nil // pass through
        }
        conf := t.Cfg.(*MyToolConfig)
        text := v.SourceText()
        if conf.Uppercase {
            text = strings.ToUpper(text)
        }
        v.SetTargetText(model.LocaleID(conf.TargetLocale), text)
        return nil
    }
    return t
}
```

## Declaring a parameter schema with struct tags

Define a config struct with exported fields. The `schema` struct tag controls how each field appears in the generated schema:

```go
type MyToolConfig struct {
    TargetLocale string `json:"targetLocale" schema:"description=Target locale for output"`
    Uppercase    bool   `json:"uppercase"    schema:"description=Convert text to uppercase,default=false"`
    MaxLength    int    `json:"maxLength"    schema:"description=Maximum output length (0 = unlimited),default=0"`
    Mode         string `json:"mode"         schema:"description=Processing mode,enum=fast|thorough|balanced,default=balanced"`
}
```

### Supported struct tag keys

| Key           | Example                     | Purpose                          |
| ------------- | --------------------------- | -------------------------------- |
| `description` | `description=Target locale` | Human-readable field description |
| `default`     | `default=true`              | Default value                    |
| `enum`        | `enum=fast\|thorough`       | Allowed values (pipe-separated)  |
| `min`         | `min=0`                     | Minimum numeric value            |
| `max`         | `max=100`                   | Maximum numeric value            |
| `widget`      | `widget=regexBuilder`       | UI widget hint                   |
| `placeholder` | `placeholder=en-US`         | Input placeholder text           |
| `group`       | `group=validation`          | Parameter group ID               |

### Go type to JSON Schema type mapping

| Go type                      | JSON Schema type |
| ---------------------------- | ---------------- |
| `bool`                       | `boolean`        |
| `string`                     | `string`         |
| `int`, `int64`, `uint`, etc. | `integer`        |
| `float32`, `float64`         | `number`         |
| `[]T`                        | `array`          |
| `map`, `struct`              | `object`         |

Interface, function, and channel fields are automatically skipped.

## How schema.FromStruct() works

The `schema.FromStruct()` function uses Go reflection to inspect a config struct and produce a `ComponentSchema`:

```go
import "github.com/neokapi/neokapi/core/schema"

s := schema.FromStruct(&MyToolConfig{}, schema.ToolMeta{
    ID:          "my-tool",
    Category:    "transform",
    DisplayName: "My Tool",
})
```

The function:

1. Iterates over exported struct fields
2. Maps Go types to JSON Schema types
3. Parses `schema` struct tags for metadata (description, default, enum, widget, etc.)
4. Extracts `group` tags to build `ui:groups` for the UI
5. Uses `json` struct tags for field names (falls back to camelCase conversion)
6. Generates a `ComponentSchema` with `toolMeta` metadata

### Parameter groups

Fields with a `group` tag are organized into collapsible sections in the UI:

```go
type QAConfig struct {
    CheckLeadingWS  bool `schema:"description=Check leading whitespace,default=true,group=whitespace"`
    CheckTrailingWS bool `schema:"description=Check trailing whitespace,default=true,group=whitespace"`
    CheckEmptyTarget bool `schema:"description=Check empty translations,default=true,group=content"`
}
```

This produces two collapsible groups ("Whitespace" and "Content") in the generated form.

## Registering with RegisterWithSchema()

Use `RegisterWithSchema()` instead of `Register()` to include the schema in the registry:

```go
func RegisterAll(reg *registry.ToolRegistry) {
    reg.RegisterWithSchema("my-tool", func() tool.Tool {
        return NewMyTool(&MyToolConfig{})
    }, toolSchema(&MyToolConfig{}, "my-tool", "My Tool", "transform"))
}

// Helper to reduce boilerplate
func toolSchema(cfg any, id, displayName, category string) *schema.ComponentSchema {
    return schema.FromStruct(cfg, schema.ToolMeta{
        ID:          id,
        Category:    category,
        DisplayName: displayName,
    })
}
```

Once registered with a schema:

- `kapi tools` shows the tool with its description and category
- The web UI renders a dynamic configuration form (via `FilterConfigEditor` / `SchemaConfigEditor`)
- The CLI can validate tool config before execution
- `reg.GetSchema("my-tool")` returns the schema for programmatic access

## Full example: creating a custom tool

Here is a complete example of a prefix/suffix wrapping tool with a parameter schema:

```go
package wraptext

import (
    "fmt"

    "github.com/neokapi/neokapi/core/model"
    "github.com/neokapi/neokapi/core/registry"
    "github.com/neokapi/neokapi/core/schema"
    "github.com/neokapi/neokapi/core/tool"
)

// Config
type WrapTextConfig struct {
    Prefix       string `json:"prefix"       schema:"description=Text prepended to each block,default=["`
    Suffix       string `json:"suffix"       schema:"description=Text appended to each block,default=]"`
    TargetLocale string `json:"targetLocale" schema:"description=Target locale,placeholder=en-US"`
    SourceOnly   bool   `json:"sourceOnly"   schema:"description=Wrap source text only,default=false"`
}

func (c *WrapTextConfig) ToolName() string { return "wrap-text" }
func (c *WrapTextConfig) Reset()           { c.Prefix = "["; c.Suffix = "]" }

// Tool
func NewWrapTextTool(cfg *WrapTextConfig) *tool.BaseTool {
    t := &tool.BaseTool{
        ToolName:        "wrap-text",
        ToolDescription: "Wraps block text with prefix and suffix",
        Cfg:             cfg,
    }
    // It can rewrite the source (SourceOnly), so it sets Transform. A
    // transformer is a read-only edit producer: it returns an EditPlan and the
    // framework applier performs the rewrite — applying the edits and rebasing
    // surviving run-anchored overlays. The structured Edits (here two pure
    // insertions) are what lets the applier rebase rather than drop overlays.
    t.Transform = func(v tool.BlockView) (tool.EditPlan, error) {
        conf := t.Cfg.(*WrapTextConfig)
        text := v.SourceText()
        wrapped := fmt.Sprintf("%s%s%s", conf.Prefix, text, conf.Suffix)
        var plan tool.EditPlan
        if conf.SourceOnly {
            n := len([]rune(text))
            plan.NewRuns = []model.Run{{Text: &model.TextRun{Text: wrapped}}}
            plan.Edits = []model.RunEdit{
                {Start: 0, End: 0, NewLen: len([]rune(conf.Prefix))}, // insert prefix
                {Start: n, End: n, NewLen: len([]rune(conf.Suffix))}, // append suffix
            }
        } else {
            plan.SetTarget(model.LocaleID(conf.TargetLocale),
                []model.Run{{Text: &model.TextRun{Text: wrapped}}})
        }
        return plan, nil
    }
    return t
}

// Registration
func Register(reg *registry.ToolRegistry) {
    s := schema.FromStruct(&WrapTextConfig{}, schema.ToolMeta{
        ID:          "wrap-text",
        Category:    "transform",
        DisplayName: "Wrap Text",
    })
    reg.RegisterWithSchema("wrap-text", func() tool.Tool {
        return NewWrapTextTool(&WrapTextConfig{Prefix: "[", Suffix: "]"})
    }, s)
}
```

Use the tool from the CLI:

```bash
kapi wrap-text input.json --target-lang fr --prefix ">> " --suffix " <<"
```

Or in a YAML flow:

```yaml
steps:
  - tool: wrap-text
    config:
      prefix: ">> "
      suffix: " <<"
      targetLocale: fr
```
