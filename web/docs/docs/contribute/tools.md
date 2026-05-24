---
sidebar_position: 4
title: Implementing a Tool
---

# Implementing a New Tool

Tools process Parts as they flow through a pipeline. Most tools only care about one or two Part types (usually Blocks).

## Using BaseTool

Build a `tool.BaseTool` and set handler function fields for the Part types you
want to process. Parts you don't handle pass through unchanged. A handler has the
signature `func(part *model.Part) (*model.Part, error)`: it receives the
streaming Part and type-asserts the resource it cares about.

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
    t.HandleBlockFn = func(part *model.Part) (*model.Part, error) {
        block, ok := part.Resource.(*model.Block)
        if !ok || !block.Translatable {
            return part, nil
        }
        text := strings.ToUpper(block.SourceText())
        block.SetTargetText(model.LocaleEnglish, text)
        return part, nil
    }
    return t
}
```

## Tool Categories

| Category      | Responsibility                  | Examples                                      |
| ------------- | ------------------------------- | --------------------------------------------- |
| **Transform** | Modify content in-place         | Segmentation, case change, search/replace     |
| **Enrich**    | Add metadata                    | TM leveraging, AI translation, terminology    |
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

The framework's built-in tools are registered with their parameter schemas; the
authoritative, generated list of what ships in the current build is the
[Tool Reference](/tools). The categories below are a way of thinking about the
kinds of work tools do — see [Tools](/framework/tools) for the concept.

### Analysis & Reporting

| Tool                  | Category | Description                                                                                                      |
| --------------------- | -------- | ---------------------------------------------------------------------------------------------------------------- |
| `word-count`          | Validate | Counts words per locale, stores in block properties                                                              |
| `char-count`          | Validate | Counts characters (with/without spaces) per locale                                                               |
| `segment-count`       | Validate | Counts source and target segments                                                                                |
| `repetition-analysis` | Validate | Tracks repeated source segments across the pipeline, tags first-occurrence vs repetition with group keys         |
| `scoping-report`      | Validate | Classifies blocks into scoping categories (new, repetition, exact-match, fuzzy-match) based on upstream analysis |
| `chars-listing`       | Validate | Accumulates all unique characters and frequencies for font subsetting                                            |

### Content Manipulation

| Tool                  | Category  | Description                                                       |
| --------------------- | --------- | ----------------------------------------------------------------- |
| `create-target`       | Transform | Creates target segment containers, optionally copying source text |
| `remove-target`       | Transform | Removes target segments for a specific locale or all locales      |
| `inline-codes-remove` | Transform | Strips inline span markers to produce clean plain text            |
| `properties-set`      | Transform | Sets key-value properties on blocks programmatically              |

### Text Processing

| Tool                 | Category  | Description                                                                   |
| -------------------- | --------- | ----------------------------------------------------------------------------- |
| `pseudo-translate`   | Transform | Generates pseudo-translations with accented characters and expansion padding  |
| `search-replace`     | Transform | Regex or literal search-and-replace on block content                          |
| `case-transform`     | Transform | Transforms text to upper, lower, or title case                                |
| `linebreak-convert`  | Convert   | Normalizes line endings (LF, CRLF, CR)                                        |
| `bom-convert`        | Convert   | Controls Unicode BOM presence on Layer resources                              |
| `fullwidth-convert`  | Convert   | Converts between half-width and full-width characters (CJK)                   |
| `uri-convert`        | Convert   | Encodes or decodes URI escape sequences                                       |
| `whitespace-correct` | Convert   | Normalizes whitespace, removes zero-width characters, matches source patterns |
| `encoding-convert`   | Convert   | Tags blocks with target encoding for downstream writers                       |
| `external-command`   | Transform | Executes external CLI programs on block text                                  |

### Segmentation

| Tool             | Category  | Description                                                  |
| ---------------- | --------- | ------------------------------------------------------------ |
| `segmentation`   | Transform | SRX-like sentence segmentation with configurable regex rules |
| `xslt-transform` | Transform | Regex-based tag transformation with backreference support    |

### Quality Assurance

| Tool                     | Category | Description                                                                  |
| ------------------------ | -------- | ---------------------------------------------------------------------------- |
| `qa-check`               | Validate | Checks whitespace, empty targets, target-same-as-source, span constraints    |
| `length-check`           | Validate | Verifies character count, word count, and target/source length ratio         |
| `chars-check`            | Validate | Detects forbidden characters, mojibake corruption, control characters        |
| `pattern-check`          | Validate | Validates regex patterns in translations (e.g., printf placeholders)         |
| `inconsistency-check`    | Validate | Flags same source with different targets (or vice versa) across the pipeline |
| `translation-comparison` | Validate | Compares translations across two target locales                              |
| `xml-validation`         | Validate | Validates XML structure in source and/or target text                         |

### Translation & Leverage

| Tool              | Category  | Description                                                               |
| ----------------- | --------- | ------------------------------------------------------------------------- |
| `tm-leverage`     | Enrich    | Pre-fills translations from Sievepen TM with fuzzy matching               |
| `diff-leverage`   | Enrich    | Preserves translations from previous document versions for unchanged text |
| `term-lookup`     | Enrich    | Scans source text for terminology matches, attaches annotations           |
| `term-enforce`    | Validate  | Checks translations for correct terminology usage                         |
| `term-check`      | Validate  | Term glossary checking with source→target mapping                         |
| `tag-protect`     | Transform | Protects tags matching regex patterns from modification                   |
| `span-classify`   | Transform | Reclassifies markup spans into semantic vocabulary types                  |
| `layer-processor` | Transform | Applies format-specific tool chains to child layers                       |

### AI & MT Tools

| Tool                   | Category | Description                                                      |
| ---------------------- | -------- | ---------------------------------------------------------------- |
| `ai-translate`         | Enrich   | LLM-powered translation via Anthropic, OpenAI, Gemini, or Ollama |
| `ai-qa`                | Validate | LLM-powered quality checks (terminology, fluency, accuracy)      |
| `ai-review`            | Validate | LLM-powered translation review with explanations                 |
| `ai-terminology`       | Enrich   | LLM-powered terminology extraction                               |
| `{provider}-translate` | Enrich   | MT translation via DeepL, Google, Microsoft, ModernMT, MyMemory  |

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
