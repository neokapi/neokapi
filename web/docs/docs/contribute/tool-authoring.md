---
sidebar_position: 3
title: Tool Authoring
---

# Creating Tools with Parameter Schemas

This guide covers how to create a tool with a parameter schema so that the UI and CLI can auto-generate configuration forms and validate user input.

## Tool basics

Every tool embeds `tool.BaseTool` and sets handler functions for the Part types it processes. Parts you don't handle pass through unchanged.

```go
package mytool

import (
    "context"
    "strings"

    "github.com/neokapi/neokapi/core/model"
    "github.com/neokapi/neokapi/core/tool"
)

type MyTool struct {
    tool.BaseTool
}

func NewMyTool(cfg *MyToolConfig) *MyTool {
    t := &MyTool{}
    t.BaseTool = tool.NewBaseTool("my-tool", "Does something useful")
    t.Cfg = cfg
    t.HandleBlockFn = t.handleBlock
    return t
}

func (t *MyTool) handleBlock(ctx context.Context, block *model.Block) (*model.Block, error) {
    cfg := t.Cfg.(*MyToolConfig)
    text := block.SourceText()
    if cfg.Uppercase {
        text = strings.ToUpper(text)
    }
    block.SetTargetText(model.LocaleID(cfg.TargetLocale), text)
    return block, nil
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

s := schema.FromStruct(&MyToolConfig{}, schema.ComponentMeta{
    ID:          "my-tool",
    Type:        "tool",
    Category:    "transform",
    DisplayName: "My Tool",
})
```

The function:

1. Iterates over exported struct fields
2. Maps Go types to JSON Schema types
3. Parses `schema` struct tags for metadata (description, default, enum, widget, etc.)
4. Extracts `group` tags to build `x-groups` for the UI
5. Uses `json` struct tags for field names (falls back to camelCase conversion)
6. Generates a `ComponentSchema` with `x-component` metadata

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
    return schema.FromStruct(cfg, schema.ComponentMeta{
        ID:          id,
        Type:        "tool",
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
    "context"
    "fmt"

    "github.com/neokapi/neokapi/core/model"
    "github.com/neokapi/neokapi/core/registry"
    "github.com/neokapi/neokapi/core/schema"
    "github.com/neokapi/neokapi/core/tool"
)

// Config
type WrapTextConfig struct {
    Prefix       string `json:"prefix"       schema:"description=Text prepended to each segment,default=["`
    Suffix       string `json:"suffix"       schema:"description=Text appended to each segment,default=]"`
    TargetLocale string `json:"targetLocale" schema:"description=Target locale,placeholder=en-US"`
    SourceOnly   bool   `json:"sourceOnly"   schema:"description=Wrap source text only,default=false"`
}

func (c *WrapTextConfig) ToolName() string { return "wrap-text" }
func (c *WrapTextConfig) Reset()           { c.Prefix = "["; c.Suffix = "]" }

// Tool
type WrapTextTool struct {
    tool.BaseTool
}

func NewWrapTextTool(cfg *WrapTextConfig) *WrapTextTool {
    t := &WrapTextTool{}
    t.BaseTool = tool.NewBaseTool("wrap-text", "Wraps segment text with prefix and suffix")
    t.Cfg = cfg
    t.HandleBlockFn = t.handleBlock
    return t
}

func (t *WrapTextTool) handleBlock(ctx context.Context, block *model.Block) (*model.Block, error) {
    cfg := t.Cfg.(*WrapTextConfig)
    wrapped := fmt.Sprintf("%s%s%s", cfg.Prefix, block.SourceText(), cfg.Suffix)
    if cfg.SourceOnly {
        block.SetSourceText(wrapped)
    } else {
        block.SetTargetText(model.LocaleID(cfg.TargetLocale), wrapped)
    }
    return block, nil
}

// Registration
func Register(reg *registry.ToolRegistry) {
    s := schema.FromStruct(&WrapTextConfig{}, schema.ComponentMeta{
        ID:          "wrap-text",
        Type:        "tool",
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
kapi wrap-text -i input.json --target-lang fr --prefix ">> " --suffix " <<"
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
