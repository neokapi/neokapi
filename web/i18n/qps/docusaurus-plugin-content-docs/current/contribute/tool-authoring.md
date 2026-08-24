---
sidebar_position: 3
title: Tool Authoring with Parameter Schemas
description: How to build a neokapi tool with a JSON Schema parameter definition so the CLI and desktop UI can auto-generate configuration forms — embedding BaseTool, setting handler functions, and registering with the tool registry.
keywords: [tool authoring, BaseTool, parameter schema, JSON Schema, Go, neokapi, pipeline stage]
---

# ▒ Çŕéàţîñĝ Ţööļš ŵîţĥ Þàŕàḿéţéŕ Šçĥéḿàš ▒

▒ Ţĥîš ĝüîđé çöṽéŕš ĥöŵ ţö çŕéàţé à ţööļ ŵîţĥ à þàŕàḿéţéŕ šçĥéḿà šö ţĥàţ ţĥé ÜÎ àñđ ÇĻÎ çàñ àüţö-ĝéñéŕàţé çöñƒîĝüŕàţîöñ ƒöŕḿš àñđ ṽàļîđàţé üšéŕ îñþüţ. ▒

## ▒ Ţööļ ƃàšîçš ▒

▒ Éṽéŕý ţööļ îš ƃüîļţ öñ `ţööļ.ƂàšéŢööļ`. Ƒöŕ Ƃļöçķš — ţĥé ţŕàñšļàţàƃļé üñîţ — à
ţööļ šéţš éẋàçţļý öñé çàþàƃîļîţý-ţýþéđ ĥàñđļéŕ, àñđ ţĥé ṽîéŵ îţ ŕéçéîṽéš ƃöüñđš
ŵĥàţ îţ ḿàý ŵŕîţé (É-03): `Àññöţàţé(ƂļöçķṼîéŵ)` ŕéàđš šöüŕçé àñđ ţàŕĝéţ àñđ
ŵŕîţéš öñļý öṽéŕļàýš, àññöţàţîöñš, àñđ þŕöþéŕţîéš; `Þŕöđüçé(ṼàŕîàñţṼîéŵ)`
ŵŕîţéš ţĥé ţàŕĝéţ; `Ţŕàñšƒöŕḿ(ƂļöçķṼîéŵ)` ŕéţüŕñš àñ éđîţ þļàñ ţĥàţ ţĥé
ƒŕàḿéŵöŕķ àþþļîéŕ üšéš ţö ŕéŵŕîţé ţĥé šöüŕçé. Ţĥé ŵŕöñĝ ŵŕîţéš
àŕé ñöţ öñ ţĥé ṽîéŵ — àñ àññöţàţöŕ ĥàš ñö ţàŕĝéţ šéţţéŕ ţö çàļļ, àñđ à
ţŕàñšƒöŕḿéŕ ĥöļđš ñö šöüŕçé šéţţéŕ àţ àļļ. Öţĥéŕ
Þàŕţ ţýþéš (Đàţà, Ḿéđîà, Ļàýéŕ, Ĝŕöüþ) üšé ţĥé üñţýþéđ `Ĥàñđļé*Ƒñ` ƒîéļđš. Þàŕţš
ýöü đöñ'ţ ĥàñđļé þàšš ţĥŕöüĝĥ üñçĥàñĝéđ; à ĥàñđļéŕ ŕéţüŕñš àñ `éŕŕöŕ` (àñđ ḿàý
çàļļ `ṽ.Đŕöþ()` ţö ŕéḿöṽé ţĥé ƃļöçķ ƒŕöḿ ţĥé šţŕéàḿ). ▒

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
    // parameter type bounds what it may write (E-03):
    //   Annotate(BlockView)  — read-only: overlays / annotations / properties
    //   Produce(VariantView) — writes the target; source stays read-only
    //   Transform(BlockView) — edit producer: returns an EditPlan the
    //                          framework applier applies to the source
    // This tool writes a target, so it sets Translate.
    t.Produce = func(v tool.VariantView) error {
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

## ▒ Đéçļàŕîñĝ à þàŕàḿéţéŕ šçĥéḿà ŵîţĥ šţŕüçţ ţàĝš ▒

▒ Đéƒîñé à çöñƒîĝ šţŕüçţ ŵîţĥ éẋþöŕţéđ ƒîéļđš. Ţĥé `šçĥéḿà` šţŕüçţ ţàĝ çöñţŕöļš ĥöŵ éàçĥ ƒîéļđ àþþéàŕš îñ ţĥé ĝéñéŕàţéđ šçĥéḿà: ▒

```go
type MyToolConfig struct {
    TargetLocale string `json:"targetLocale" schema:"description=Target locale for output"`
    Uppercase    bool   `json:"uppercase"    schema:"description=Convert text to uppercase,default=false"`
    MaxLength    int    `json:"maxLength"    schema:"description=Maximum output length (0 = unlimited),default=0"`
    Mode         string `json:"mode"         schema:"description=Processing mode,enum=fast|thorough|balanced,default=balanced"`
}
```

### ▒ Šüþþöŕţéđ šţŕüçţ ţàĝ ķéýš ▒

| Key           | Example                     | Purpose                          |
| ------------- | --------------------------- | -------------------------------- |
| `description` | `description=Target locale` | Human-readable field description |
| `default`     | `default=true`              | Default value                    |
| `enum`        | `enum=fast\|thorough`       | Allowed values (pipe-separated)  |
| `min`         | `min=0`                     | Minimum numeric value            |
| `max`         | `max=100`                   | Maximum numeric value            |
| `widget`      | `widget=regexBuilder`       | UI widget hint                   |
| `placeholder` | `placeholder=en`            | Input placeholder text           |
| `group`       | `group=validation`          | Parameter group ID               |

### ▒ Ĝö ţýþé ţö ĴŠÖÑ Šçĥéḿà ţýþé ḿàþþîñĝ ▒

| Go type                      | JSON Schema type |
| ---------------------------- | ---------------- |
| `bool`                       | `boolean`        |
| `string`                     | `string`         |
| `int`, `int64`, `uint`, etc. | `integer`        |
| `float32`, `float64`         | `number`         |
| `[]T`                        | `array`          |
| `map`, `struct`              | `object`         |

▒ Îñţéŕƒàçé, ƒüñçţîöñ, àñđ çĥàññéļ ƒîéļđš àŕé àüţöḿàţîçàļļý šķîþþéđ. ▒

## ▒ Ĥöŵ šçĥéḿà.ƑŕöḿŠţŕüçţ() ŵöŕķš ▒

▒ Ţĥé `šçĥéḿà.ƑŕöḿŠţŕüçţ()` ƒüñçţîöñ üšéš Ĝö ŕéƒļéçţîöñ ţö îñšþéçţ à çöñƒîĝ šţŕüçţ àñđ þŕöđüçé à `ÇöḿþöñéñţŠçĥéḿà`: ▒

```go
import "github.com/neokapi/neokapi/core/schema"

s := schema.FromStruct(&MyToolConfig{}, schema.ToolMeta{
    ID:          "my-tool",
    Category:    "transform",
    DisplayName: "My Tool",
})
```

▒ Ţĥé ƒüñçţîöñ: ▒

1. ▒ Îţéŕàţéš öṽéŕ éẋþöŕţéđ šţŕüçţ ƒîéļđš ▒
2. ▒ Ḿàþš Ĝö ţýþéš ţö ĴŠÖÑ Šçĥéḿà ţýþéš ▒
3. ▒ Þàŕšéš `šçĥéḿà` šţŕüçţ ţàĝš ƒöŕ ḿéţàđàţà (đéšçŕîþţîöñ, đéƒàüļţ, éñüḿ, ŵîđĝéţ, éţç.) ▒
4. ▒ Éẋţŕàçţš `ĝŕöüþ` ţàĝš ţö ƃüîļđ `üî:ĝŕöüþš` ƒöŕ ţĥé ÜÎ ▒
5. ▒ Üšéš `ĵšöñ` šţŕüçţ ţàĝš ƒöŕ ƒîéļđ ñàḿéš (ƒàļļš ƃàçķ ţö çàḿéļÇàšé çöñṽéŕšîöñ) ▒
6. ▒ Ĝéñéŕàţéš à `ÇöḿþöñéñţŠçĥéḿà` ŵîţĥ `ţööļḾéţà` ḿéţàđàţà ▒

### ▒ Þàŕàḿéţéŕ ĝŕöüþš ▒

▒ Ƒîéļđš ŵîţĥ à `ĝŕöüþ` ţàĝ àŕé öŕĝàñîžéđ îñţö çöļļàþšîƃļé šéçţîöñš îñ ţĥé ÜÎ: ▒

```go
type QAConfig struct {
    CheckLeadingWS  bool `schema:"description=Check leading whitespace,default=true,group=whitespace"`
    CheckTrailingWS bool `schema:"description=Check trailing whitespace,default=true,group=whitespace"`
    CheckEmptyTarget bool `schema:"description=Check empty translations,default=true,group=content"`
}
```

▒ Ţĥîš þŕöđüçéš ţŵö çöļļàþšîƃļé ĝŕöüþš ("Ŵĥîţéšþàçé" àñđ "Çöñţéñţ") îñ ţĥé ĝéñéŕàţéđ ƒöŕḿ. ▒

## ▒ Ŕéĝîšţéŕîñĝ ŵîţĥ ŔéĝîšţéŕŴîţĥŠçĥéḿà() ▒

▒ Üšé `ŔéĝîšţéŕŴîţĥŠçĥéḿà()` îñšţéàđ öƒ `Ŕéĝîšţéŕ()` ţö îñçļüđé ţĥé šçĥéḿà îñ ţĥé ŕéĝîšţŕý: ▒

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

▒ Öñçé ŕéĝîšţéŕéđ ŵîţĥ à šçĥéḿà: ▒

- ▒ `ķàþî ţööļš` šĥöŵš ţĥé ţööļ ŵîţĥ îţš đéšçŕîþţîöñ àñđ çàţéĝöŕý ▒
- ▒ Ţĥé ŵéƃ ÜÎ ŕéñđéŕš à đýñàḿîç çöñƒîĝüŕàţîöñ ƒöŕḿ (ṽîà `ƑîļţéŕÇöñƒîĝÉđîţöŕ` / `ŠçĥéḿàÇöñƒîĝÉđîţöŕ`) ▒
- ▒ Ţĥé ÇĻÎ çàñ ṽàļîđàţé ţööļ çöñƒîĝ ƃéƒöŕé éẋéçüţîöñ ▒
- ▒ `ŕéĝ.ĜéţŠçĥéḿà("ḿý-ţööļ")` ŕéţüŕñš ţĥé šçĥéḿà ƒöŕ þŕöĝŕàḿḿàţîç àççéšš ▒

## ▒ Ƒüļļ éẋàḿþļé: çŕéàţîñĝ à çüšţöḿ ţööļ ▒

▒ Ĥéŕé îš à çöḿþļéţé éẋàḿþļé öƒ à þŕéƒîẋ/šüƒƒîẋ ŵŕàþþîñĝ ţööļ ŵîţĥ à þàŕàḿéţéŕ šçĥéḿà: ▒

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
    TargetLocale string `json:"targetLocale" schema:"description=Target locale,placeholder=en"`
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

▒ Üšé ţĥé ţööļ ƒŕöḿ ţĥé ÇĻÎ: ▒

```bash
kapi wrap-text input.json --target-lang fr --prefix ">> " --suffix " <<"
```

▒ Öŕ îñ à ÝÀḾĻ ƒļöŵ: ▒

```yaml
steps:
  - tool: wrap-text
    config:
      prefix: ">> "
      suffix: " <<"
      targetLocale: fr
```
