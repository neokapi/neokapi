---
sidebar_position: 6
title: Authoring Vocabularies
description: How to implement and extend neokapi vocabularies — the JSON type-definition format, category:name identifiers, rendering and display metadata, and registering a vocabulary with the framework.
keywords: [vocabularies, authoring, inline codes, semantic types, JSON, vocabulary file, neokapi]
---

# ▒ Àüţĥöŕîñĝ Ṽöçàƃüļàŕîéš ▒

▒ Ţĥîš ĝüîđé çöṽéŕš îḿþļéḿéñţîñĝ àñđ éẋţéñđîñĝ ṽöçàƃüļàŕîéš — ţĥé šéḿàñţîç ţýþé
šýšţéḿ ţĥàţ çļàššîƒîéš îñļîñé çöđéš. Ƒöŕ ŵĥàţ ṽöçàƃüļàŕîéš àŕé àñđ ŵĥý ţĥéý
éẋîšţ, šéé ţĥé çöñçéþţ þàĝé: [Ṽöçàƃüļàŕîéš](/framework/vocabularies). ▒

## ▒ Ṽöçàƃüļàŕý ƒîļé ƒöŕḿàţ ▒

▒ Éàçĥ ṽöçàƃüļàŕý îš à ĴŠÖÑ ƒîļé. Ţýþéš àŕé ķéýéđ ƃý à `çàţéĝöŕý:ñàḿé` îđéñţîƒîéŕ
àñđ çàŕŕý ŕéñđéŕîñĝ, đîšþļàý, çöļöŕ, àñđ çöñšţŕàîñţ ḿéţàđàţà: ▒

```json
{
  "name": "my-vocabulary",
  "version": "1.0",
  "extends": "common-formatting",
  "entity_prefix": "entity:",
  "types": {
    "category:type-name": {
      "category": "category-name",
      "label": "Human Readable Label",
      "html": {
        "open": "<tag>",
        "close": "</tag>",
        "placeholder": "<tag/>"
      },
      "display": {
        "open": "[TAG]",
        "close": "[/TAG]",
        "placeholder": "[TAG/]"
      },
      "chipLabel": {
        "open": "tag>",
        "close": "/tag",
        "placeholder": "tag"
      },
      "color": {
        "bg": "rgba(59,130,246,0.15)",
        "border": "rgba(59,130,246,0.5)",
        "text": "rgb(59,130,246)"
      },
      "equiv": "",
      "constraints": {
        "deletable": true,
        "cloneable": true,
        "reorderable": true
      }
    }
  },
  "fallback": {
    "html": { "open": "<span>", "close": "</span>", "placeholder": "<span/>" },
    "display": { "open": "[?]", "close": "[/?]", "placeholder": "[?/]" },
    "chipLabel": { "open": "?>", "close": "/?", "placeholder": "?" },
    "color": {
      "bg": "rgba(156,163,175,0.15)",
      "border": "rgba(156,163,175,0.5)",
      "text": "rgb(107,114,128)"
    },
    "constraints": { "deletable": true, "cloneable": true, "reorderable": true }
  }
}
```

### ▒ Ƒîéļđ ŕéƒéŕéñçé ▒

| Field           | Required | Description                                        |
| --------------- | -------- | -------------------------------------------------- |
| `name`          | Yes      | Unique vocabulary name                             |
| `version`       | Yes      | Semver version string                              |
| `extends`       | No       | Parent vocabulary name (types are merged)          |
| `entity_prefix` | No       | Prefix for entity-type inline codes (default `"entity:"`) |
| `types`         | Yes      | Map of type name → `SpanTypeInfo`                  |
| `fallback`      | No       | Default rendering for unknown types                |

### ▒ Ţýþé ñàḿé çöñṽéñţîöñ ▒

▒ Ţýþé ñàḿéš ƒöļļöŵ ţĥé `çàţéĝöŕý:ñàḿé` þàţţéŕñ: `ƒḿţ:ƃöļđ`, `ļîñķ:ĥýþéŕļîñķ`,
`çöđé:ṽàŕîàƃļé`, `šţŕüçţ:ƃŕéàķ`. ▒

### ▒ Çöñšţŕàîñţ šéḿàñţîçš ▒

| Constraint    | `true`                                | `false`                                   |
| ------------- | ------------------------------------- | ----------------------------------------- |
| `deletable`   | Translator may remove the tag         | Tag must appear in translation (enforced) |
| `cloneable`   | Translator may duplicate the tag      | Tag count must not exceed source count    |
| `reorderable` | Translator may rearrange tag position | Tag position relative to others is locked |

## ▒ Üšîñĝ ṽöçàƃüļàŕîéš îñ à ƒöŕḿàţ ŕéàđéŕ ▒

▒ À ƒöŕḿàţ ŕéàđéŕ îñîţîàļîžéš à `ṼöçàƃüļàŕýŔéĝîšţŕý` àñđ üšéš îţ ţö þöþüļàţé
îñļîñé-çöđé ḿéţàđàţà àš îţ ƃüîļđš à Ƃļöçķ'š `[]ḿöđéļ.Ŕüñ` šéǫüéñçé: ▒

```go
package myformat

import "github.com/neokapi/neokapi/core/model"

type Reader struct {
    vocab *model.VocabularyRegistry
}

func NewReader() *Reader {
    vocab := model.NewVocabularyRegistry()
    _ = vocab.LoadDefaults() // common-formatting + rich-html + rich-jsx + code-tokens
    return &Reader{vocab: vocab}
}
```

▒ Îñļîñé çöñţéñţ îš à ƒļàţ `[]ḿöđéļ.Ŕüñ` (šéé
[Ƒ-02: Çöñţéñţ Ḿöđéļ](/contribute/architecture/foundations/f-02-content-model)). Àñ
öþéñîñĝ ţàĝ ƃéçöḿéš à `ÞçÖþéñŔüñ`, îţš ḿàţçĥîñĝ çļöšé à `ÞçÇļöšéŔüñ` ŵîţĥ ţĥé
šàḿé `ÎĐ`, àñđ à šéļƒ-çļöšîñĝ çöñšţŕüçţ à `ÞļàçéĥöļđéŕŔüñ`. Ŵĥéñ ƃüîļđîñĝ öñé,
ļööķ üþ ţĥé ṽöçàƃüļàŕý éñţŕý àñđ þöþüļàţé ţĥé ŕéñđéŕîñĝ àñđ çöñšţŕàîñţ ƒîéļđš —
ḿîŕŕöŕîñĝ ţĥé þéŕ-ƒöŕḿàţ `ŕüñƂüîļđéŕ` ĥéļþéŕš (`çöŕé/ƒöŕḿàţš/*/ŕüñ_ƃüîļđéŕ.ĝö`): ▒

```go
// openRun builds the opening half of a paired code, e.g. <b> / <a href="…">.
func (r *Reader) openRun(semType, subType, id, nativeMarkup string) model.Run {
    info := r.vocab.LookupOrFallback(semType)
    return model.Run{PcOpen: &model.PcOpenRun{
        ID:      id,            // shared with the matching PcClose
        Type:    semType,       // "fmt:bold"
        SubType: subType,       // "html:b" or "md:strong"
        Data:    nativeMarkup,  // original markup for roundtrip
        Disp:    info.Display.Open,    // "[B]"
        Equiv:   info.Equiv,           // "" (or "\n" for struct:break)
        Constraints: &model.RunConstraints{
            Deletable:   info.Constraints.Deletable,
            Cloneable:   info.Constraints.Cloneable,
            Reorderable: info.Constraints.Reorderable,
        },
    }}
}

// closeRun builds the matching close. PcCloseRun shares the opener's ID and
// replays its own native markup; it inherits the opener's constraints.
func (r *Reader) closeRun(semType, subType, id, nativeMarkup string) model.Run {
    info := r.vocab.LookupOrFallback(semType)
    return model.Run{PcClose: &model.PcCloseRun{
        ID:      id,
        Type:    semType,
        SubType: subType,
        Data:    nativeMarkup,  // "</b>"
        Equiv:   info.Equiv,
    }}
}

// phRun builds a self-closing placeholder, e.g. <br/> or a variable token.
func (r *Reader) phRun(semType, subType, id, nativeMarkup string) model.Run {
    info := r.vocab.LookupOrFallback(semType)
    return model.Run{Ph: &model.PlaceholderRun{
        ID:      id,
        Type:    semType,
        SubType: subType,
        Data:    nativeMarkup,
        Disp:    info.Display.Placeholder, // "[BR/]"
        Equiv:   info.Equiv,               // "\n" for struct:break
        Constraints: &model.RunConstraints{
            Deletable:   info.Constraints.Deletable,
            Cloneable:   info.Constraints.Cloneable,
            Reorderable: info.Constraints.Reorderable,
        },
    }}
}
```

### ▒ Ḿàþþîñĝ ñàţîṽé éļéḿéñţš ţö šéḿàñţîç ţýþéš ▒

▒ Éàçĥ ƒöŕḿàţ ḿàþš îţš ñàţîṽé çöñšţŕüçţš ţö šéḿàñţîç ţýþéš. Ţĥé ĤŢḾĻ ŕéàđéŕ ķéýš à
ñàḿé → ţýþé ḿàþ öñ ţĥé éļéḿéñţ ñàḿé: ▒

```go
var htmlSemanticTypes = map[string]string{
    "b": "fmt:bold", "strong": "fmt:bold",
    "i": "fmt:italic", "em": "fmt:italic",
    "u": "fmt:underline", "s": "fmt:strikethrough",
    "a": "link:hyperlink", "code": "fmt:code",
    "br": "struct:break", "img": "media:image",
    "sub": "fmt:subscript", "sup": "fmt:superscript", "mark": "fmt:highlight",
}
```

▒ Ţĥé Ḿàŕķđöŵñ ŕéàđéŕ ĥàš ñö šüçĥ ḿàþ. Îţ šŵîţçĥéš öñ ĝöļđḿàŕķ ÀŠŢ ñöđé ţýþéš àñđ
àššîĝñš ţĥé šéḿàñţîç ţýþé þéŕ ñöđé ƃéƒöŕé çàļļîñĝ `ŕ.ṽöçàƃ.ĻööķüþÖŕƑàļļƃàçķ(…)`,
ŕéšöļṽîñĝ ţö ţĥé šàḿé ṽöçàƃüļàŕý ţýþéš: ▒

| Markdown construct                  | Semantic type    |
| ----------------------------------- | ---------------- |
| strong emphasis (`ast.Emphasis` level 2) | `fmt:bold`       |
| emphasis (`ast.Emphasis` level 1)   | `fmt:italic`     |
| inline code                         | `fmt:code`       |
| link                                | `link:hyperlink` |
| image                               | `link:image`     |

▒ À šöƒţ ļîñé ƃŕéàķ îš ñöţ à ŕüñ: îţ îš éḿîţţéđ àš îñļîñé ţéẋţ çöñţîñüàţîöñ (šéé
`šöƒţƂŕéàķÇöñţîñüàţîöñ`), ñöţ à `šţŕüçţ:ƃŕéàķ` þļàçéĥöļđéŕ. ▒

### ▒ ŠüƃŢýþé çöñṽéñţîöñš ▒

▒ Ţĥé `ŠüƃŢýþé` ƒîéļđ ŕéçöŕđš ƒöŕḿàţ-šþéçîƒîç þŕöṽéñàñçé üšîñĝ à þŕéƒîẋ
çöñṽéñţîöñ: `ĥţḿļ:` (`ĥţḿļ:ƃ`, `ĥţḿļ:šþàñ`), `ḿđ:` (`ḿđ:šţŕöñĝ`), `ẋļƒ:`
(`ẋļƒ:ṽàŕ`), `đöçẋ:` (`đöçẋ:ŵ:ƃ`). Çüšţöḿ ƒöŕḿàţš šĥöüļđ üšé à ŕéṽéŕšé-đöḿàîñ
þŕéƒîẋ: `çöḿ.àçḿé:çüšţöḿ-ţàĝ`. ▒

## ▒ Çŕéàţîñĝ à çüšţöḿ ṽöçàƃüļàŕý ▒

### ▒ 1. Çŕéàţé ţĥé ĴŠÖÑ ƒîļé ▒

▒ Çŕéàţé à ĴŠÖÑ ƒîļé üñđéŕ `çöŕé/ḿöđéļ/ṽöçàƃüļàŕîéš/`: ▒

```json
{
  "name": "my-domain",
  "version": "1.0",
  "extends": "common-formatting",
  "types": {
    "domain:widget": {
      "category": "domain",
      "label": "Widget",
      "html": { "placeholder": "<span class=\"widget\"/>" },
      "display": { "placeholder": "[WIDGET]" },
      "chipLabel": { "placeholder": "wgt" },
      "color": {
        "bg": "rgba(168,85,247,0.15)",
        "border": "rgba(168,85,247,0.5)",
        "text": "rgb(168,85,247)"
      },
      "equiv": "",
      "constraints": { "deletable": false, "cloneable": false, "reorderable": true }
    }
  }
}
```

### ▒ 2. Ļöàđ îţ îñţö ţĥé ŕéĝîšţŕý ▒

▒ `ĻöàđĐéƒàüļţš()` ļöàđš ţĥé éḿƃéđđéđ ṽöçàƃüļàŕîéš. Ţö àđđ öñé àţ ŕüñţîḿé: ▒

```go
vocab := model.NewVocabularyRegistry()
vocab.LoadDefaults()

customData, _ := os.ReadFile("my-domain.json")
vocab.Load(customData)
```

### ▒ 3. Ḿàþ îţ îñ ýöüŕ ŕéàđéŕ ▒

▒ Àđđ ţĥé ñéŵ ţýþé ţö ýöüŕ ƒöŕḿàţ ŕéàđéŕ'š šéḿàñţîç ţýþé ḿàþþîñĝ: ▒

```go
var myFormatSemanticTypes = map[string]string{
    "widget": "domain:widget",
}
```

## ▒ ŠþàñÇļàššîƒý ţööļ ▒

▒ Ƒöŕ ƒöŕḿàţš ţĥàţ đö ñöţ þéŕƒöŕḿ ƒüļļ šéḿàñţîç çļàššîƒîçàţîöñ (ƒöŕ éẋàḿþļé, ŵĥéñ
çöñţéñţ àŕŕîṽéš ṽîà ţĥé Öķàþî ƃŕîđĝé), ţĥé `šþàñ-çļàššîƒý` ţööļ ŕéçļàššîƒîéš
ĝéñéŕîç `çöđé:ḿàŕķüþ` îñļîñé-çöđé ŕüñš (`Þĥ` / `ÞçÖþéñ` / `ÞçÇļöšé`) îñţö
þŕöþéŕ šéḿàñţîç ţýþéš: ▒

```go
tool := tools.NewSpanClassifyTool(&tools.SpanClassifyConfig{})
```

▒ Îţ àþþļîéš šţŕàţéĝîéš îñ öŕđéŕ: çĥéçķ ţĥé ŕüñ'š `ŠüƃŢýþé` àĝàîñšţ ķñöŵñ Öķàþî
ţýþé šţŕîñĝš, þàŕšé `Đàţà` ƒöŕ àñ ĤŢḾĻ éļéḿéñţ ñàḿé, ļööķ ţĥàţ ñàḿé üþ îñ ţĥé
šéḿàñţîç ţýþé ḿàþ, àñđ öţĥéŕŵîšé ļéàṽé ţĥé ŕüñ àš `çöđé:ḿàŕķüþ`. Ţĥé ţööļ ñàḿé
îš ŕéţàîñéđ ƒöŕ ƃàçķŵàŕđš çöḿþàţîƃîļîţý ŵîţĥ éẋîšţîñĝ ƒļöŵ đéƒîñîţîöñš. ▒

## ▒ Ţéšţîñĝ ṽöçàƃüļàŕîéš ▒

```go
func TestMyVocabulary(t *testing.T) {
    vocab := model.NewVocabularyRegistry()
    require.NoError(t, vocab.LoadDefaults())

    info := vocab.Lookup("fmt:bold")
    require.NotNil(t, info)
    assert.Equal(t, "formatting", info.Category)
    assert.True(t, info.Constraints.Deletable)

    unknown := vocab.LookupOrFallback("custom:unknown")
    require.NotNil(t, unknown)
    assert.True(t, unknown.Constraints.Deletable) // fallback rendering
}
```

## ▒ Ƃéšţ þŕàçţîçéš ▒

1. ▒ **Üšé éẋîšţîñĝ ţýþéš ŵĥéñ þöššîƃļé.** Ḿàþ ţö `ƒḿţ:ƃöļđ` ŕàţĥéŕ ţĥàñ çŕéàţîñĝ `ḿý-ƒöŕḿàţ:ƃöļđ`. ▒
2. ▒ **Šéţ çöñšţŕàîñţš çöñšéŕṽàţîṽéļý.** Ḿàŕķ çöđé ţöķéñš ñöñ-đéļéţàƃļé; ƒöŕḿàţţîñĝ ƒüļļý ƒļéẋîƃļé. ▒
3. ▒ **Ķééþ ṽöçàƃüļàŕîéš šḿàļļ.** Öñļý àđđ ţýþéš ŵîţĥ đîšţîñçţ ŕéñđéŕîñĝ öŕ çöñšţŕàîñţ ñééđš. ▒
4. ▒ **Ţéšţ ŕöüñđţŕîþ ƒîđéļîţý.** Ṽöçàƃüļàŕý ţýþéš àƒƒéçţ ŕéñđéŕîñĝ, ƃüţ éàçĥ ŕüñ'š `Đàţà` đŕîṽéš öüţþüţ — ṽéŕîƒý ƃöţĥ. ▒
5. ▒ **Éẋţéñđ ŕàţĥéŕ ţĥàñ ŕéþļàçé.** Üšé `éẋţéñđš` ţö ƃüîļđ öñ `çöḿḿöñ-ƒöŕḿàţţîñĝ`. ▒

## ▒ Ŕéļàţéđ ŕéàđîñĝ ▒

- ▒ [Ṽöçàƃüļàŕîéš](/framework/vocabularies) — ţĥé çöñçéþţ àñđ ƃüîļţ-îñ ṽöçàƃüļàŕîéš. ▒
- ▒ [Îḿþļéḿéñţîñĝ à Ƒöŕḿàţ](/contribute/formats) — ƃüîļđîñĝ ŕéàđéŕš àñđ ŵŕîţéŕš. ▒
- ▒ [Îñļîñé Ƒöŕḿàţţîñĝ](/framework/inline-formatting) — ţĥé îñļîñé-çöđé ḿöđéļ îñ ţĥé çöñţéñţ ḿöđéļ. ▒
