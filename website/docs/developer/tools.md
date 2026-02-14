---
sidebar_position: 4
title: Implementing a Tool
---

# Implementing a New Tool

Tools process Parts as they flow through a pipeline. Most tools only care about one or two Part types (usually Blocks).

## Using BaseTool

Create a type embedding `tool.BaseTool` and set handler function fields for the Part types you want to process. Parts you don't handle pass through unchanged.

```go
package mytool

import (
    "context"
    "strings"
    "github.com/gokapi/gokapi/core/model"
    "github.com/gokapi/gokapi/core/tool"
)

type UppercaseTool struct {
    tool.BaseTool
}

func NewUppercaseTool() *UppercaseTool {
    t := &UppercaseTool{}
    t.BaseTool = tool.NewBaseTool("uppercase", "Converts source text to uppercase")
    t.HandleBlockFn = t.handleBlock
    return t
}

func (t *UppercaseTool) handleBlock(ctx context.Context, block *model.Block) (*model.Block, error) {
    if !block.Translatable {
        return block, nil
    }
    text := strings.ToUpper(block.SourceText())
    block.SetTargetText(model.LocaleEnglish, text)
    return block, nil
}
```

## Tool Categories

| Category | Responsibility | Examples |
|---|---|---|
| **Transform** | Modify content in-place | Segmentation, case change, search/replace |
| **Enrich** | Add metadata | TM leveraging, AI translation, terminology |
| **Validate** | Check quality without modifying | QA checks, word count, spell check |
| **Convert** | Transform representations | Encoding conversion, line break normalization |

## Overriding Process

If you need full control over the processing loop (e.g., accumulating state across multiple Parts), override `Process` directly:

```go
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

Register your tool in the tool registry:

```go
registry.DefaultToolRegistry.Register("uppercase", func() tool.Tool {
    return NewUppercaseTool()
})
```

## Testing

```go
func TestUppercaseTool(t *testing.T) {
    tool := NewUppercaseTool()
    parts := []*model.Part{
        {Type: model.PartLayerStart, Resource: &model.Layer{ID: "doc1"}},
        {Type: model.PartBlock, Resource: &model.Block{
            ID: "b1", Translatable: true,
            Source: []*model.Segment{{ID: "s1", Content: model.NewFragment("hello")}},
        }},
        {Type: model.PartLayerEnd, Resource: &model.Layer{ID: "doc1"}},
    }

    results := testutil.RunToolOnParts(t, tool, parts)
    block := testutil.FindFirstBlock(results)
    assert.Equal(t, "HELLO", block.TargetText(model.LocaleEnglish))
}
```

## Built-in Tools

### Utility Tools (`lib/tools/`)

| Tool | Category | Description |
|------|----------|-------------|
| `wordcount` | Validate | Counts words and characters per locale, with per-locale properties |
| `charcount` | Validate | Counts characters per locale |
| `pseudo-translate` | Transform | Generates pseudo-translations for testing by applying character substitution |
| `search-replace` | Transform | Regex-based search and replace on block content |
| `segmentation` | Transform | SRX-like sentence segmentation with configurable regex rules |
| `qa-check` | Validate | Configurable quality checks: missing translations, whitespace mismatches, number mismatches |
| `tm-leverage` | Enrich | Pre-fills translations from Bowrain Memory translation memory with fuzzy matching |
| `term-lookup` | Enrich | Scans source text for terminology matches, attaches `TermAnnotation` entries |
| `term-enforce` | Validate | Checks translations for correct terminology usage, reports violations |

### Registering Built-in Tools

All built-in tools can be registered at once:

```go
import "github.com/gokapi/gokapi/lib/tools"

toolReg := registry.NewToolRegistry()
tools.RegisterTools(toolReg)
```

Individual tools can also be created directly:

```go
// Segmentation with default SRX-like rules
segTool := tools.NewSegmentationTool(&tools.SegmentationConfig{})

// QA check with specific checks enabled
qaTool := tools.NewQACheckTool(&tools.QACheckConfig{
    TargetLocale: "fr",
    Checks: []string{"missing-translation", "whitespace-mismatch", "number-mismatch"},
})

// TM leverage with custom threshold
tmTool := tools.NewTMLeverageTool(&tools.TMLeverageConfig{
    TargetLocale: "fr",
    Threshold: 0.8,
    TM: sievepenInstance,
})

// Term lookup — scans source text for terminology matches
termLookupTool := tools.NewTermLookupTool(&tools.TermLookupConfig{
    SourceLocale: "en",
    TargetLocale: "fr",
    TB: termbaseInstance,
})

// Term enforce — validates translations use correct terminology
termEnforceTool := tools.NewTermEnforceTool(&tools.TermEnforceConfig{
    SourceLocale: "en",
    TargetLocale: "fr",
    TB: termbaseInstance,
})
```
