---
sidebar_position: 10
title: Content memory
description: Content memory is neokapi's store of previously settled content. It holds multilingual entries as Run sequences with inline markup and matches them in three tiers — plain, structural, and source-entity — so high-quality matches are returned first.
keywords: [content memory, reuse, leverage, fuzzy matching, runs, inline markup, SQLite]
---

# ▒ Çöñţéñţ ḿéḿöŕý ▒

▒ ñéöķàþî'š **çöñţéñţ ḿéḿöŕý** ļîṽéš îñ ţĥé `ḿéḿöŕý/` þàçķàĝé. Üñļîķé šţöŕéš ţĥàţ
ķééþ þļàîñ šţŕîñĝš, îţ ŵöŕķš ŵîţĥ ţĥé ƒüļļ çöñţéñţ ḿöđéļ — éàçĥ éñţŕý ĥöļđš
ḿüļţîļîñĝüàļ ṽàŕîàñţš àš `Ŕüñ` šéǫüéñçéš (ţéẋţ þļüš îñļîñé ḿàŕķüþ) àñđ ḿàţçĥéš
ţĥéḿ îñ ţĥŕéé ţîéŕš ŵîţĥ éñţîţý-àŵàŕé àđàþţàţîöñ. Ţĥé šàḿé éñĝîñé ƃàçķš ţĥé
`ķàþî ḿéḿöŕý` çöḿḿàñđš, ţĥé `ŕéçýçļé` þîþéļîñé ţööļ, àñđ ţĥé Ĝö ļîƃŕàŕý. ▒

▒ Îñ ţĥé ÇĻÎ, çöñţéñţ ḿéḿöŕý îš ţĥé éñĝîñé üñđéŕ `ķàþî éẋéç ŕéçýçļé` — ţĥé
šîñĝļé-ţööļ ļéṽéŕàĝé þàšš — àñđ üñđéŕ ţĥé ƒîŕšţ šţéþ öƒ `ķàþî üþ`'š đéƒàüļţ ƒļöŵ,
ŵĥîçĥ ŕéçýçļéš ƒŕöḿ ḿéḿöŕý ƃéƒöŕé àñý ÀÎ ţŕàñšļàţîöñ ŕüñš. Šéé
[Üñđéŕšţàñđîñĝ ţĥé ÇĻÎ ļàýéŕš](/kapi/direct-execution-layer) ƒöŕ ĥöŵ ţĥé
šîñĝļé-ţööļ, ƒļöŵ, àñđ þŕöĵéçţ-ļööþ šüŕƒàçéš ŕéļàţé. ▒

## ▒ Çöñţéñţ-àŵàŕé ḿàţçĥîñĝ ▒

▒ Éàçĥ éñţŕý îš îñđéẋéđ üñđéŕ ţĥŕéé ķéýš, ţŕîéđ îñ öŕđéŕ, šö ţĥé ĥîĝĥéšţ-ǫüàļîţý
ḿàţçĥ îš ŕéţüŕñéđ ƒîŕšţ: ▒

| Tier | Match type      | Normalizes                          | Example                                    |
| ---- | --------------- | ----------------------------------- | ------------------------------------------ |
| 1    | **Generalized** | Named entities → typed placeholders | "Welcome, John" → "Welcome, \{PERSON\}"    |
| 2    | **Structural**  | Inline markup → normalized codes    | "Click **here**" → "Click \{1\}here\{/1\}" |
| 3    | **Plain**       | Nothing (raw text)                  | Levenshtein fuzzy matching                 |

▒ Éàçĥ ţîéŕ ýîéļđš éẋàçţ (100%) öŕ ƒüžžý ḿàţçĥéš. Ŵĥéñ à ĝéñéŕàļîžéđ éẋàçţ ḿàţçĥ
îš ƒöüñđ, éñţîţý ṽàļüéš ƒŕöḿ ţĥé çüŕŕéñţ šöüŕçé àŕé àđàþţéđ îñţö ţĥé šţöŕéđ
ţàŕĝéţ — šö "Ŵéļçöḿé, Ƃöƃ" → "Ƃîéñṽéñüé, Ƃöƃ" àđàþţš ţö "Ŵéļçöḿé, Àļîçé" →
"Ƃîéñṽéñüé, Àļîçé" àţ 100%. Ţĥîš öŕđéŕîñĝ ḿîŕŕöŕš ĥöŵ à ţŕàñšļàţöŕ éṽàļüàţéš
ḿàţçĥéš: éñţîţý đîƒƒéŕéñçéš ḿàţţéŕ ļéšš ţĥàñ šţŕüçţüŕàļ öñéš, ŵĥîçĥ ḿàţţéŕ ļéšš
ţĥàñ ţéẋţüàļ çĥàñĝéš. ▒

▒ Ţĥé ţýþéđ þļàçéĥöļđéŕš ţĥé ĝéñéŕàļîžéđ ţîéŕ ķéýš öñ (`{PERSON}`, `{PRODUCT}`, …)
çöḿé ƒŕöḿ éñţîţý đéţéçţîöñ — à ƒàšţ ļöçàļ ḿöđéļ öŕ àñ ĻĻḾ ţĥàţ ŕéçöĝñîžéš ţĥé
ñàḿéđ ţĥîñĝš îñ à ƃļöçķ. Ýöü đöñ'ţ ŕüñ đéţéçţîöñ àš à šéþàŕàţé ţàšķ: îţ ĥàþþéñš
àš þàŕţ öƒ þŕéþàŕîñĝ çöñţéñţ, àñđ ţĥé šàḿé đéţéçţîöñ àļšö þöŵéŕš
[ŕéđàçţîöñ](/framework/redaction). Àññöţàţé éñţîţîéš öñçé àñđ ƃöţĥ ĝéñéŕàļîžéđ
ḿéḿöŕý ŕéüšé àñđ ŕéđàçţîöñ ƒöļļöŵ. ▒

## ▒ Šţöŕàĝé ƃàçķéñđš ▒

▒ Ţŵö ƃàçķéñđš šĥîþ îñ ţĥé `ḿéḿöŕý/` þàçķàĝé, ƃöţĥ îḿþļéḿéñţîñĝ ţĥé
`ÇöñţéñţḾéḿöŕý` îñţéŕƒàçé ŵîţĥ ƒüļļ ţîéŕ šüþþöŕţ: ▒

1. ▒ **Îñ-ḿéḿöŕý** (`ḿéḿöŕý.ÑéŵÎñḾéḿöŕýŠţöŕé`) — ƒàšţ àñđ éþĥéḿéŕàļ, üšéđ ƒöŕ
   šéššîöñ-šçöþéđ ƃàţçĥ þŕöçéššîñĝ. ▒
2. ▒ **ŠǪĻîţé** (`ḿéḿöŕý.ÑéŵŠǪĻîţéŠţöŕé`) — þéŕšîšţéñţ ƒîļé-ƃàšéđ šţöŕàĝé ƒöŕ ÇĻÎ
   ŵöŕķƒļöŵš. ▒

▒ Ţĥé îñţéŕƒàçé àļšö àççöḿḿöđàţéš šéŕṽéŕ-šîđé ƃàçķéñđš ƒöŕ ḿüļţî-üšéŕ
đéþļöýḿéñţš ŵîţĥ þŕöĵéçţ šçöþîñĝ, šţŕéàḿš, àñđ ŵöŕķšþàçé îšöļàţîöñ. Ƒüžžý
ḿàţçĥîñĝ üšéš Ļéṽéñšĥţéîñ éđîţ đîšţàñçé ŵîţĥ à çöñƒîĝüŕàƃļé ţĥŕéšĥöļđ (đéƒàüļţ
0.70); ŕéšüļţš àŕé šöŕţéđ ƃý šçöŕé àñđ ţĥéñ ƃý ţîéŕ. ▒

## ▒ ÇĻÎ üšàĝé ▒

### ▒ Ŕéšöüŕçé ļöçàţîöñ ▒

▒ Àļļ `ķàþî ḿéḿöŕý` çöḿḿàñđš (éẋçéþţ `ļîšţ`) àççéþţ ţĥéšé ḿüţüàļļý éẋçļüšîṽé ƒļàĝš: ▒

| Flag            | Resolves to                   | Example                    |
| --------------- | ----------------------------- | -------------------------- |
| `--name <n>`    | `~/.config/kapi/memory/<n>.db`    | `--name project-memory`    |
| `--local`       | `./memory.db` (current directory) | `--local`                  |
| `--file <path>` | Explicit file path            | `--file /shared/memory.db` |
| _(no flag)_     | Same as `--local`             |                            |

▒ Đàţàƃàšéš àŕé çŕéàţéđ öñ đéḿàñđ îƒ ţĥéý đöñ'ţ éẋîšţ. `ḿéḿöŕý/` àñđ `ḿéḿöŕý.đƃ`
àŕé ţĥé öñ-đîšķ ñàḿéš ƒöŕ à ñàḿéđ öŕ ļööšé çöñţéñţ ḿéḿöŕý. `~/.çöñƒîĝ/ķàþî` îš
ţĥé üšéŕ çöñƒîĝ đîŕéçţöŕý öñ Ļîñüẋ, àñđ ŕéšöļṽéš ţö
`~/Ļîƃŕàŕý/Àþþļîçàţîöñ Šüþþöŕţ/ķàþî` öñ ḿàçÖŠ. `ķàþî çöñƒîĝ þàţĥ` þŕîñţš ţĥé
ŕéšöļṽéđ ļöçàţîöñ. ▒

▒ Ŵîţĥ ñö ƒļàĝ îñšîđé à þŕöĵéçţ, ţĥé çöñţéñţ ḿéḿöŕý îš îñšţéàđ à šéţ öƒ ţàƃļéš îñ
ţĥé þŕöĵéçţ'š `.ķàþî/ŵöŕķ/šţöŕé.đƃ` — šéé
[Ḿéḿöŕý & ţéŕḿš šţöŕàĝé](/kapi/recipes/memory-and-terms-storage). ▒

```bash
kapi memory import translations.memory.json --name project-memory
kapi memory export --name project-memory -o project.memory.json
kapi memory lookup "Welcome to our platform" --name project-memory -s en -t fr
kapi memory search "welcome" --name project-memory -s en
kapi memory stats --name project-memory
kapi memory list
```

## ▒ Þîþéļîñé îñţéĝŕàţîöñ ▒

▒ Ţĥé `ŕéçýçļé` ţööļ ǫüéŕîéš çöñţéñţ ḿéḿöŕý ƒöŕ éàçĥ Ƃļöçķ'š šöüŕçé šéĝḿéñţš àñđ
àþþļîéš ḿàţçĥéš. Éṽéŕý ḿàţçĥ — éẋàçţ öŕ ƒüžžý — îš ŕéçöŕđéđ àš àñ
`ÀļţŢŕàñšļàţîöñ` àññöţàţîöñ (ḿàţçĥéđ šöüŕçé/ţàŕĝéţ ŕüñš, šçöŕé, ḿàţçĥ ţýþé, àñđ
ţĥé `ţḿ` öŕîĝîñ ķîñđ), àñđ à ƒîļļéđ ţàŕĝéţ îš çöḿḿîţţéđ ŵîţĥ þŕöṽéñàñçé
(`Öŕîĝîñ{Kind: "tm", Tool: "recycle"}`), îţš šçöŕé, àñđ `đŕàƒţ` šţàţüš, šö
ţĥé ļéṽéŕàĝé îš àüđîţàƃļé ŕàţĥéŕ ţĥàñ àñ öþàǫüé öṽéŕŵŕîţé. Éẋàçţ ḿàţçĥéš šķîþ ÀÎ
ţŕàñšļàţîöñ, ŕéđüçîñĝ çöšţ àñđ ļàţéñçý. ▒

▒ **Šéĝḿéñţ-àŵàŕé ļéṽéŕàĝé.** Ŵĥéñ à ƃļöçķ çàŕŕîéš à ḿüļţî-šéĝḿéñţ
[šéĝḿéñţàţîöñ](/framework/segmentation) öṽéŕļàý (à þŕöšé þàŕàĝŕàþĥ šþļîţ îñţö
šéñţéñçéš), `ŕéçýçļé` ļööķš üþ çöñţéñţ ḿéḿöŕý **þéŕ šéñţéñçé**. Ţĥîš ŕéçöṽéŕš
ļéṽéŕàĝé ƒöŕ ḿüļţî-šéñţéñçé ƃļöçķš ţĥàţ ŵöüļđ ñéṽéŕ ḿàţçĥ ţĥé šéñţéñçé-ķéýéđ
ḿéḿöŕý àš à šîñĝļé üñîţ. À šîñĝļé-šéĝḿéñţ ƃļöçķ — ḿöšţ ÜÎ šţŕîñĝš — ţàķéš ţĥé
ŵĥöļé-ƃļöçķ þàţĥ üñçĥàñĝéđ. ▒

▒ Ţĥé ŕéšüļţ îš ŕéçöŕđéđ šö îţ îš **àüđîţàƃļé, ñöţ ƃļîñđ**: ▒

- ▒ Éàçĥ ḿàţçĥîñĝ šéñţéñçé îš àţţàçĥéđ àš àñ `ÀļţŢŕàñšļàţîöñ` àññöţàţîöñ (ḿàţçĥéđ
  šöüŕçé àñđ ţàŕĝéţ ŕüñš, šçöŕé, éẋàçţ/ƒüžžý ḿàţçĥ ţýþé, `ţḿ` öŕîĝîñ ķîñđ) — ķéþţ
  ŵĥéţĥéŕ öŕ ñöţ ţĥé ƃļöçķ ţàŕĝéţ îš ƒîļļéđ, šö **þàŕţîàļ** ļéṽéŕàĝé (šöḿé
  šéñţéñçéš ḿàţçĥéđ, šöḿé ñéŵ) îš þŕéšéŕṽéđ ƒöŕ à ŕéṽîéŵéŕ öŕ à ļàţéŕ
  ţŕàñšļàţîöñ šţàĝé ŕàţĥéŕ ţĥàñ đîšçàŕđéđ. ▒
- ▒ Ţĥé ƃļöçķ ŕéçöŕđš `ţḿ-šéĝḿéñţ-ḿàţçĥéš` (é.ĝ. `3/5`) ƒöŕ ǫüîçķ ĝàţîñĝ. ▒
- ▒ Ţĥé ƃļöçķ ţàŕĝéţ îš ƒîļļéđ öñļý ŵĥéñ **éṽéŕý** šéñţéñçé ḿàţçĥéđ àñđ ţĥé
  šéĝḿéñţš àŕé çöñţîĝüöüš; ŵĥéñ îţ îš, ţĥé çöḿḿîţţéđ ţàŕĝéţ çàŕŕîéš
  þŕöṽéñàñçé (`Öŕîĝîñ{Kind: "tm", Tool: "recycle"}`), ţĥé ŕöļļ-üþ `Šçöŕé`,
  àñđ `đŕàƒţ` šţàţüš — à ŕéṽîéŵàƃļé þŕé-ƒîļļ, ñöţ à šîĝñéđ-öƒƒ ţŕàñšļàţîöñ. ▒

▒ Ŕüñ [šéĝḿéñţàţîöñ](/framework/segmentation) ƃéƒöŕé `ŕéçýçļé` ţö éñàƃļé ţĥîš. ▒

```bash
kapi exec recycle input.html -o output.html --source-lang en --target-lang fr --memory project-memory
```

```yaml
steps:
  - tool: recycle
    config:
      fuzzyThreshold: 70 # minimum score for fuzzy matches (0-100)
      fillTarget: true # copy the best candidate into the target
      fillTargetThreshold: 95 # minimum score required to fill the target
```

## ▒ Ĝö ļîƃŕàŕý ▒

### ▒ Îñţéŕƒàçé ▒

```go
type ContentMemory interface {
    Add(entry Entry) error
    Lookup(source *model.Block, sourceLocale, targetLocale model.LocaleID,
        opts LookupOptions) ([]Match, error)
    LookupText(source string, sourceLocale, targetLocale model.LocaleID,
        opts LookupOptions) ([]Match, error)
    Delete(id string) error
    Count() int
    Close() error
}
```

▒ (Ḿéţĥöđš, àñđ ţĥé ŢḾẊ ĥéļþéŕš ƃéļöŵ, ţàķé à `çöñţéẋţ.Çöñţéẋţ` àš ţĥéîŕ ƒîŕšţ
àŕĝüḿéñţ îñ ţĥé ŕéàļ ÀÞÎ; îţ îš éļîđéđ ĥéŕé ƒöŕ ŕéàđàƃîļîţý.) ▒

▒ `Ļööķüþ` ţàķéš à ƒüļļ `*ḿöđéļ.Ƃļöçķ` àñđ üšéš îţš Ŕüñ çöñţéñţ (àñđ éñţîţý
àññöţàţîöñš) ƒöŕ ţîéŕéđ ḿàţçĥîñĝ; `ĻööķüþŢéẋţ` ţàķéš à þļàîñ šţŕîñĝ àñđ
þéŕƒöŕḿš þļàîñ-ţîéŕ ḿàţçĥîñĝ öñļý. `ĻööķüþŠéĝḿéñţ` ḿàţçĥéš à šîñĝļé šéĝḿéñţ öƒ
à ƃļöçķ ƒöŕ šéñţéñçé-ļéṽéļ ļéṽéŕàĝé. Ƃöţĥ ŠǪĻîţé àñđ îñ-ḿéḿöŕý ƃàçķéñđš àļšö
îḿþļéḿéñţ `ÉñţŕýÞŕöṽîđéŕ` (`Éñţŕîéš()`), ŵĥîçĥ îš ĥöŵ ŢḾẊ éẋþöŕţ éñüḿéŕàţéš à
šţöŕé, àñđ öƒƒéŕ þàĝîñàţéđ `ŠéàŕçĥÉñţŕîéš(...)` ƒöŕ ƃŕöŵšîñĝ. ▒

### ▒ Ķéý ţýþéš ▒

```go
type Entry struct {
    ID          string
    ProjectID   string
    Variants    map[model.LocaleID][]model.Run // peer language variants
    HintSrcLang model.LocaleID                 // locale the author treated as canonical
    Entities    []EntityMapping                // entity placeholders
    Properties  map[string]string
    Origins     []Origin
    Note        string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}

type Match struct {
    Entry             Entry
    Score             float64 // 0.0-1.0
    MatchType         MatchType
    ProjectID         string             // provenance of the matched entry
    EntityAdaptations []EntityAdaptation // entity value substitutions
}

type LookupOptions struct {
    MinScore     float64      // minimum match score (default 0.7)
    MaxResults   int          // max results to return (default 10)
    MatchModes   []MatchMode  // which tiers to use (default: all)
    ProjectID    string       // project context for scoring boost
    ProjectScope ProjectScope // project filtering mode (default: all)
}
```

▒ Àñ éñţŕý îš ḿüļţîļîñĝüàļ: ţĥéŕé îš ñö àüţĥöŕîţàţîṽé šöüŕçé àţ ţĥé þéŕšîšţéñçé
ļàýéŕ — éàçĥ ļàñĝüàĝé îš à þééŕ `Ṽàŕîàñţš[ļöçàļé]` Ŕüñ šéǫüéñçé, àñđ ţĥé ļööķüþ
đîŕéçţîöñ îš šüþþļîéđ àţ ţĥé çàļļ šîţé. `ḾàţçĥŢýþé` ŕàñĝéš ƒŕöḿ
`ĝéñéŕàļîžéđ-éẋàçţ` (ĥîĝĥéšţ ŕéüšé) ţĥŕöüĝĥ `šţŕüçţüŕàļ-éẋàçţ`, `éẋàçţ`, ţĥé
çöŕŕéšþöñđîñĝ ƒüžžý ṽàŕîàñţš, đöŵñ ţö `ƒüžžý`. `Éñţŕý` ĥéļþéŕš:
`Ṽàŕîàñţ(ļöçàļé)`, `ṼàŕîàñţŢéẋţ(ļöçàļé)`, `ṼàŕîàñţŠţŕüçţüŕàļ(ļöçàļé)`,
`ṼàŕîàñţĜéñéŕàļîžéđ(ļöçàļé)`. Ţĥé `ÉñţîţýÀđàþţàţîöñš` ƒîéļđ öñ à ḿàţçĥ ļîšţš
éàçĥ šüƃšţîţüţîöñ ŵîţĥ îţš þöšîţîöñ šö çöñšüḿéŕš çàñ àþþļý àđàþţàţîöñš
þŕéçîšéļý. ▒

### ▒ Éẋàḿþļé ▒

```go
package main

import (
    "fmt"

    "github.com/neokapi/neokapi/core/model"
    "github.com/neokapi/neokapi/memory"
)

func main() {
    tm := memory.NewInMemoryStore()
    defer tm.Close()

    tm.Add(memory.Entry{
        ID: "e1",
        Variants: map[model.LocaleID][]model.Run{
            "en": {{Text: &model.TextRun{Text: "Welcome to our platform"}}},
            "fr": {{Text: &model.TextRun{Text: "Bienvenue sur notre plateforme"}}},
        },
        HintSrcLang: "en",
    })

    block := model.NewBlock("b1", "Welcome to our platform")
    matches, err := tm.Lookup(block, "en", "fr", memory.DefaultLookupOptions())
    if err != nil {
        panic(err)
    }
    for _, m := range matches {
        fmt.Printf("Score: %.0f%% Type: %s Target: %s\n",
            m.Score*100, m.MatchType, m.Entry.VariantText("fr"))
    }
}
```

### ▒ ŢḾẊ îḿþöŕţ / éẋþöŕţ ▒

```go
// Importing keeps only the named bilingual pair; ImportTMXLocalePairs keeps an
// arbitrary set of locales, and an empty set keeps every <tuv> present.
count, err := memory.ImportTMXWithOptions(tm, reader, "en", "fr",
    memory.ImportTMXOptions{OriginKey: "corpus.tmx"})

err = memory.ExportTMXBilingual(tm, writer, "en", "fr") // src/tgt pair
// or, for a set of locales held in the store:
err = memory.ExportTMX(tm, writer, []model.LocaleID{"en", "fr", "de"})
```

▒ Îḿþöŕţ ŕéǫüîŕéš à ƃàçķéñđ ţĥàţ šüþþöŕţš îḿþöŕţ šéššîöñš — ţĥàţ îš, öñé
îḿþļéḿéñţîñĝ ţĥé þéŕšîšţéñţ `Šţöŕé` îñţéŕƒàçé. ▒

## ▒ Çöñţéñţ ḿéḿöŕý àñđ ţéŕḿîñöļöĝý ▒

▒ Çöñţéñţ ḿéḿöŕý àñđ [ţéŕḿîñöļöĝý](/framework/terminology) àŕé đéļîƃéŕàţéļý
šéþàŕàţé šýšţéḿš ŵîţĥ đîƒƒéŕéñţ đàţà šĥàþéš — ḿéḿöŕý šţöŕéš šéĝḿéñţ þàîŕš,
ţéŕḿîñöļöĝý šţöŕéš ḿüļţî-ļöçàļé çöñçéþţš. Ţĥéý šĥàŕé ţĥé `Ƃļöçķ` àññöţàţîöñ
šýšţéḿ àš ţĥéîŕ îñţéĝŕàţîöñ þöîñţ, šö ƃöţĥ ķîñđš öƒ ḿàţçĥ àŕé àṽàîļàƃļé ţö àñý
đöŵñšţŕéàḿ ţööļ öŕ éđîţöŕ. ▒
