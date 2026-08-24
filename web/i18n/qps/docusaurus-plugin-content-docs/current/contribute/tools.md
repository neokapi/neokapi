---
sidebar_position: 4
title: Implementing a Tool
description: How to build a neokapi Tool — embedding BaseTool, setting handler functions for the Part types you care about, and passing unhandled Parts through the pipeline unchanged.
keywords: [tool implementation, BaseTool, Part, handler, pipeline, Go, neokapi, processing]
---

# ▒ Îḿþļéḿéñţîñĝ à Ñéŵ Ţööļ ▒

▒ Ţööļš þŕöçéšš Þàŕţš àš ţĥéý ƒļöŵ ţĥŕöüĝĥ à þîþéļîñé. Ḿöšţ ţööļš öñļý çàŕé àƃöüţ öñé öŕ ţŵö Þàŕţ ţýþéš (üšüàļļý Ƃļöçķš). ▒

## ▒ Üšîñĝ ƂàšéŢööļ ▒

▒ Ƃüîļđ à `ţööļ.ƂàšéŢööļ` àñđ šéţ ĥàñđļéŕ ƒüñçţîöñ ƒîéļđš ƒöŕ ţĥé Þàŕţ ţýþéš ýöü
ŵàñţ ţö þŕöçéšš. Þàŕţš ýöü đöñ'ţ ĥàñđļé þàšš ţĥŕöüĝĥ üñçĥàñĝéđ. Ţĥéŕé àŕé ţŵö
ƒàḿîļîéš öƒ ĥàñđļéŕ. ▒

▒ Ƒöŕ **Ƃļöçķ** þàŕţš, šéţ éẋàçţļý ÖÑÉ çàþàƃîļîţý-ţýþéđ ĥàñđļéŕ — ţĥé þàŕàḿéţéŕ
ţýþé ƃöüñđš ŵĥàţ ţĥé ţööļ ḿàý ŵŕîţé (îḿḿüţàƃîļîţý ḿöđéļ, É-03): ▒

- ▒ `Àññöţàţé(ţööļ.ƂļöçķṼîéŵ) éŕŕöŕ` — ŕéàđ-öñļý; ŵŕîţéš öñļý öṽéŕļàýš,
  àññöţàţîöñš, àñđ þŕöþéŕţîéš. ▒
- ▒ `Ţŕàñšļàţé(ţööļ.ṼàŕîàñţṼîéŵ) éŕŕöŕ` — ŕéàđš šöüŕçé, ŵŕîţéš ţàŕĝéţ. ▒
- ▒ `Ţŕàñšƒöŕḿ(ţööļ.ƂļöçķṼîéŵ) (ţööļ.ÉđîţÞļàñ, éŕŕöŕ)` — à ŕéàđ-öñļý éđîţ
  þŕöđüçéŕ: ŕéţüŕñš àñ éđîţ þļàñ, àñđ ţĥé ƒŕàḿéŵöŕķ àþþļîéŕ ŕéŵŕîţéš ţĥé
  šöüŕçé — ŕéƃàšîñĝ šüŕṽîṽîñĝ öṽéŕļàýš, ṽàüļţîñĝ šéçŕéţš, àñđ ƃöüñđš-çĥéçķîñĝ,
  àţöḿîçàļļý. Ţĥé ƒļöŵ'š þļàçéḿéñţ þàšš ṽàļîđàţéš ŵĥéŕé à ţŕàñšƒöŕḿéŕ ḿàý šîţ. ▒

▒ Ƒöŕ ţĥé ñöñ-Ƃļöçķ þàŕţš (Đàţà, Ḿéđîà, Ļàýéŕ/Ĝŕöüþ šţàŕţ/éñđ), šéţ ţĥé üñţýþéđ
`Ĥàñđļé*Ƒñ` ƒîéļđš, ŵĥîçĥ üšé `ţööļ.ÞàŕţĤàñđļéŕ` =
`ƒüñç(þàŕţ *ḿöđéļ.Þàŕţ) (*ḿöđéļ.Þàŕţ, éŕŕöŕ)`: ţĥéšé ŕéçéîṽé ţĥé šţŕéàḿîñĝ Þàŕţ
àñđ ţýþé-àššéŕţ ţĥé ŕéšöüŕçé ţĥéý çàŕé àƃöüţ. ▒

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

## ▒ Ţööļ Çàţéĝöŕîéš ▒

| Category      | Responsibility                  | Examples                                      |
| ------------- | ------------------------------- | --------------------------------------------- |
| **Transform** | Modify content in-place         | case change, search/replace, redaction        |
| **Enrich**    | Add metadata or overlays        | segmentation, content-memory leveraging, AI translation, terminology |
| **Validate**  | Check quality without modifying | QA checks, word count, spell check            |
| **Convert**   | Transform representations       | Encoding conversion, line break normalization |

## ▒ Öṽéŕŕîđîñĝ Þŕöçéšš ▒

▒ Îƒ ýöü ñééđ ƒüļļ çöñţŕöļ öṽéŕ ţĥé þŕöçéššîñĝ ļööþ (ƒöŕ éẋàḿþļé, ţö àççüḿüļàţé
šţàţé àçŕöšš ḿàñý Þàŕţš, öŕ ţö éḿîţ ḿöŕé Þàŕţš ţĥàñ ýöü çöñšüḿé), đéƒîñé à ñàḿéđ
ţýþé ţĥàţ éḿƃéđš `ţööļ.ƂàšéŢööļ` àñđ öṽéŕŕîđé `Þŕöçéšš` đîŕéçţļý: ▒

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

## ▒ Ŕéĝîšţŕàţîöñ ▒

▒ Ŕéĝîšţéŕ ýöüŕ ţööļ îñ à `ŢööļŔéĝîšţŕý`, ḿàþþîñĝ à ñàḿé ţö à ƒàçţöŕý: ▒

```go
reg := registry.NewToolRegistry()
reg.Register("uppercase", func() tool.Tool {
    return NewUppercaseTool()
})
```

▒ Üšé `ŔéĝîšţéŕŴîţĥŠçĥéḿà` îñšţéàđ ţö àţţàçĥ à þàŕàḿéţéŕ šçĥéḿà — šéé
[Ţööļ Àüţĥöŕîñĝ](/contribute/tool-authoring). ▒

## ▒ Ƃüîļţ-îñ Ţööļš ▒

▒ Ţĥé ƒŕàḿéŵöŕķ'š ƃüîļţ-îñ ţööļš àŕé ŕéĝîšţéŕéđ ŵîţĥ ţĥéîŕ þàŕàḿéţéŕ šçĥéḿàš. Ţĥé
àüţĥöŕîţàţîṽé, ĝéñéŕàţéđ ļîšţ öƒ ŵĥàţ šĥîþš îñ ţĥé çüŕŕéñţ ƃüîļđ — éṽéŕý ţööļ'š
ñàḿé, đéšçŕîþţîöñ, àñđ þàŕàḿéţéŕš — îš ţĥé [Ţööļ Ŕéƒéŕéñçé](/tools), ŕéñđéŕéđ
ƒŕöḿ ţĥöšé šçĥéḿàš šö îţ àļŵàýš ḿàţçĥéš ţĥé ƃüîļđ. Ţĥîš ĝüîđé đéļîƃéŕàţéļý đöéš
ñöţ ŕéšţàţé îţ; ƒöŕ ĥöŵ ţĥé ƃüîļţ-îñš ḿàþ ţö ţĥé ķîñđš öƒ ŵöŕķ àƃöṽé, šéé
[Ţööļš](/framework/tools). ▒

### ▒ Šçĥéḿà-Đŕîṽéñ ÇĻÎ Ƒļàĝš ▒

▒ Àļļ ƃüîļţ-îñ ţööļš üšé šçĥéḿà-đŕîṽéñ ÇĻÎ ƒļàĝš. Ţööļ çöñƒîĝ šţŕüçţš üšé `šçĥéḿà:"..."` ţàĝš ţö àüţö-ĝéñéŕàţé ƒļàĝš ƒŕöḿ ţĥé šţŕüçţ ƒîéļđš. Üšé `šçĥéḿà:"-"` ţö éẋçļüđé à ƒîéļđ ƒŕöḿ ƒļàĝ ĝéñéŕàţîöñ. Ţĥé `ÑéŵŢööļƑŕöḿÇöñƒîĝ` þàţţéŕñ àļļöŵš ţĥé ƒļöŵ éñĝîñé ţö îñšţàñţîàţé ţööļš ƒŕöḿ ÝÀḾĻ çöñƒîĝüŕàţîöñ ƃý ḿàþþîñĝ çöñƒîĝ ķéýš ţö šţŕüçţ ƒîéļđš àüţöḿàţîçàļļý. ▒

### ▒ Ŕéĝîšţéŕîñĝ Ƃüîļţ-îñ Ţööļš ▒

▒ Àļļ ƃüîļţ-îñ ţööļš çàñ ƃé ŕéĝîšţéŕéđ îñţö à ŕéĝîšţŕý àţ öñçé, éàçĥ ŵîţĥ îţš
þàŕàḿéţéŕ šçĥéḿà: ▒

```go
import (
    "github.com/neokapi/neokapi/core/registry"
    "github.com/neokapi/neokapi/core/tools"
)

toolReg := registry.NewToolRegistry()
tools.RegisterAll(toolReg)
```

▒ Îñđîṽîđüàļ ţööļš çàñ àļšö ƃé çöñšţŕüçţéđ đîŕéçţļý. Éàçĥ ţàķéš à çöñƒîĝ šţŕüçţ
(šéé ţĥé [Ţööļ Ŕéƒéŕéñçé](/tools) ƒöŕ éṽéŕý ƒîéļđ): ▒

```go
// Segmentation with default SRX-like rules
segTool := tools.NewSegmentationTool(&tools.SegmentationConfig{})

// QA check — configured via per-rule flags on QACheckConfig
qaTool := tools.NewQACheckTool(tools.NewQACheckConfig(model.LocaleID("fr")))

// Content-memory leverage with a custom fuzzy threshold and a memory provider
memoryTool := tools.NewMemoryLeverageTool(&tools.MemoryLeverageConfig{
    TargetLocale:   "fr",
    FuzzyThreshold: 80, // 0-100
    Provider:       memoryProvider,
})
```

▒ Ţĥé ţéŕḿîñöļöĝý ţööļš ļîṽé îñ ţĥé `ţéŕḿš` þàçķàĝé àñđ ţàķé à `Ţéŕḿîñöļöĝý`
àļöñĝšîđé ţĥéîŕ çöñƒîĝ: ▒

```go
import "github.com/neokapi/neokapi/terms"

// Term lookup — scans source text and attaches terminology annotations
termLookupTool := terms.NewTermLookupTool(tb, terms.TermLookupConfig{
    SourceLocale: "en",
    TargetLocale: "fr",
})

// Term enforce — verifies translations use the preferred terminology
termEnforceTool := terms.NewTermEnforceTool(tb, terms.TermEnforceConfig{
    SourceLocale: "en",
    TargetLocale: "fr",
})
```
