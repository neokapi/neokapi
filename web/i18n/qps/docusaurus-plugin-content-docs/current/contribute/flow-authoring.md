---
sidebar_position: 4
title: YAML Flow Authoring
description: How to write neokapi flows as YAML — the steps-based human-authored format, sequential and parallel branches, tool configuration, and how the YAML compiles to the internal nodes-and-edges graph.
keywords: [flow authoring, YAML flows, steps format, parallel steps, pipeline YAML, neokapi, flow definition]
---

# ▒ ÝÀḾĻ Ƒļöŵ Àüţĥöŕîñĝ ▒

▒ Ƒļöŵš đéƒîñé þŕöçéššîñĝ þîþéļîñéš àš ÝÀḾĻ ƒîļéš. Ţĥé šţéþš-ƃàšéđ ƒöŕḿàţ îš ţĥé ĥüḿàñ-àüţĥöŕéđ ŕéþŕéšéñţàţîöñ ţĥàţ çöḿþîļéš ţö àñ îñţéŕñàļ ĝŕàþĥ (ñöđéš + éđĝéš) ƒöŕ éẋéçüţîöñ. ▒

## ▒ Šţéþš ƒöŕḿàţ ▒

▒ À ƒļöŵ îš à ļîšţ öƒ šéǫüéñţîàļ šţéþš. Éàçĥ šţéþ ŕéƒéŕéñçéš à ţööļ ƃý ñàḿé àñđ öþţîöñàļļý þŕöṽîđéš çöñƒîĝüŕàţîöñ: ▒

```yaml
steps:
  - tool: pseudo-translate
    config:
      targetLocale: fr
      expansionPercent: 30
      prefix: "["
      suffix: "]"
```

### ▒ Šöüŕçé àñđ šîñķ ▒

▒ À ƒļöŵ çàŕŕîéš öñļý îţš šţéþš. Ŵĥéŕé çöñţéñţ éñţéŕš àñđ ļéàṽéš àŕé **ƃîñđîñĝš**
ŕéšöļṽéđ ŵĥéñ ţĥé ƒļöŵ ŕüñš — à ƒîļé, ţĥé þŕöĵéçţ šţöŕé, à `.ķþž` ŵöŕķšþàçé, àñ
îñţéŕçĥàñĝé îḿþöŕţ/éẋþöŕţ, öŕ `ñöñé` — ñöţ ƒîéļđš öƒ ţĥé ƒļöŵ đöçüḿéñţ. À ƒļöŵ
đéçļàŕéš à ƃîñđîñĝ öñļý ŵĥéñ îţ îš îñţŕîñšîç ţö ţĥé ƒļöŵ (é.ĝ. à çĥéçķ ƒļöŵ ţĥàţ
þŕöđüçéš ñö đöçüḿéñţ šéţš `šîñķ: ñöñé`), àñđ ñéṽéŕ à þàţĥ: ▒

```yaml
sink: none        # only when intrinsic; otherwise omit and let the run decide
steps:
  - tool: qa
```

▒ Àţ ţĥé ÇĻÎ ţĥé ƃîñđîñĝ çöḿéš ƒŕöḿ `-î` / `-ö` (à þļàîñ þàţĥ îš đéţéçţéđ, à
`šçĥéḿé:` îš éẋþļîçîţ), ţĥé þŕöĵéçţ / `.ķþž` ýöü àŕé îñ, öŕ àüţö-đéţéçţîöñ;
`ķàþî ŕüñ <ƒļöŵ> --éẋþļàîñ` šĥöŵš ţĥé ŕéšöļṽéđ `šöüŕçé → šîñķ`. Šéé
[É-04: Ƒļöŵ Î/Ö Ƃîñđîñĝ](architecture/engine/e-04-flows-and-io-binding). ▒

### ▒ Šţéþ ļàƃéļš ▒

▒ Àđđ à `ļàƃéļ` ƒöŕ ŕéàđàƃîļîţý îñ ţĥé ÜÎ ĝŕàþĥ ṽîéŵ: ▒

```yaml
steps:
  - tool: pseudo-translate
    label: Generate test translations
    config:
      targetLocale: fr
```

## ▒ Šéǫüéñţîàļ šţéþš ▒

▒ Šţéþš éẋéçüţé îñ öŕđéŕ. Ţĥé öüţþüţ çĥàññéļ öƒ öñé ţööļ ƒééđš îñţö ţĥé îñþüţ çĥàññéļ öƒ ţĥé ñéẋţ: ▒

```yaml
steps:
  - tool: create-target
    config:
      targetLocale: fr
      copySource: true

  - tool: search-replace
    config:
      pairs:
        - search: "TODO"
          replace: ""
      target: true

  - tool: qa
    config:
      targetLocale: fr
```

▒ Ţĥîš çŕéàţéš à ţĥŕéé-šţéþ þîþéļîñé: çŕéàţé ţĥé ţàŕĝéţ, çļéàñ üþ þļàçéĥöļđéŕ ţéẋţ, ţĥéñ ŕüñ ǫüàļîţý çĥéçķš. ▒

## ▒ Þàŕàļļéļ ƃļöçķš ƒöŕ ƒàñ-öüţ ▒

▒ Üšé `þàŕàļļéļ:` ţö ŕüñ ḿüļţîþļé ţööļš çöñçüŕŕéñţļý öñ ţĥé šàḿé šţŕéàḿ öƒ Þàŕţš. Éàçĥ ƃŕàñçĥ ŕéçéîṽéš à çöþý öƒ ţĥé îñþüţ àñđ þŕöđüçéš îñđéþéñđéñţ öüţþüţ: ▒

```yaml
steps:
  - tool: create-target
    config:
      targetLocale: fr
      copySource: true

  - parallel:
      - tool: qa
        label: Quality checks
        config:
          targetLocale: fr
      - tool: term-check
        label: Terminology checks
        config:
          targetLocale: fr
      - tool: xml-validation
        label: XML validation
```

▒ Àļļ ţĥŕéé çĥéçķ ţööļš ŕüñ àţ ţĥé šàḿé ţîḿé, éàçĥ îñ îţš öŵñ ĝöŕöüţîñé. ▒

## ▒ Ţŕàñšƒöŕḿéŕš ▒

▒ Ţööļš ţĥàţ ŕéŵŕîţé ţĥé šöüŕçé — ŕéđàçţîöñ, ŵĥîţéšþàçé/ḿàŕķüþ ñöŕḿàļîžàţîöñ, à
šöüŕçé-ḿüţàţîñĝ `šçŕîþţ` — àŕé öŕđîñàŕý šţéþš îñ ţĥé šàḿé öŕđéŕéđ ļîšţ àš
éṽéŕýţĥîñĝ éļšé. À ţŕàñšƒöŕḿéŕ ŕéţüŕñš àñ éđîţ þļàñ; ţĥé ƒŕàḿéŵöŕķ àþþļîéŕ
þéŕƒöŕḿš ţĥé ŕéŵŕîţé îñļîñé àñđ îñ öŕđéŕ, ŕéƃàšîñĝ šüŕṽîṽîñĝ ŕüñ-àñçĥöŕéđ
öṽéŕļàýš (šéĝḿéñţàţîöñ, ţéŕḿš) öñţö ţĥé ñéŵ ŕüñš, šö éàçĥ ţŕàñšƒöŕḿéŕ šéţţļéš
ţĥé šöüŕçé ƃéƒöŕé ļàţéŕ šţéþš öƃšéŕṽé îţ. ▒

```yaml
steps:
  - tool: redact
    config:
      detectors: [rules]
      rulesPath: redaction-rules.yaml

  - tool: translate
    config:
      targetLocale: fr
```

▒ À **þļàçéḿéñţ þàšš** (`çöŕé/ƒļöŵ/þļàçéḿéñţ.ĝö`) ṽàļîđàţéš ţŕàñšƒöŕḿéŕ
þöšîţîöñš ƃéšîđé đàţà-ƒļöŵ ṽàļîđàţîöñ àţ éṽéŕý ƒļöŵ ƃüîļđ àñđ ļöàđ: ▒

- ▒ **Éŕŕöŕ** — à ţŕàñšƒöŕḿéŕ ƒöļļöŵš à šţéþ ţĥàţ þŕöđüçéš à çöḿḿîţţéđ ţàŕĝéţ
  (ŕéŵŕîţîñĝ šöüŕçé öŕþĥàñš ţĥé ţàŕĝéţš). À ţŕàñšƒöŕḿéŕ ţĥàţ þŕöđüçéš ţàŕĝéţš
  îţšéļƒ, šüçĥ àš `üñŕéđàçţ`, îš éẋéḿþţ. ▒
- ▒ **Éŕŕöŕ** — à ŕéçöṽéŕàƃļé ţŕàñšƒöŕḿéŕ (`ŕéđàçţ`) ƒöļļöŵš à šţéþ ŵîţĥ ţĥé
  ŕéḿöţé-šöüŕçé-éĝŕéšš šîđé éƒƒéçţ, éẋçéþţ à šţéþ þŕöđüçîñĝ àñ îñþüţ ţĥé
  ţŕàñšƒöŕḿéŕ'š çöñƒîĝ-ŕéšöļṽéđ çöñţŕàçţ ŕéǫüîŕéš (à çļöüđ ÑÉŔ šţéþ ƒééđîñĝ
  éñţîţý-đŕîṽéñ ŕéđàçţîöñ îš ţĥé đöçüḿéñţéđ đéţéçţîöñ ţŕàđé-öƒƒ,
  [Ç-10](/contribute/architecture/context/c-10-redaction)). ÀÎ ţööļš çöñƒîĝüŕéđ ŵîţĥ
  à ļöçàļ þŕöṽîđéŕ (öļļàḿà, đéḿö) çàŕŕý ñö éĝŕéšš éƒƒéçţ. ▒
- ▒ **Ŵàŕñîñĝ** — à ţŕàñšƒöŕḿéŕ þļàçéđ ļàţéŕ ţĥàñ îţš éàŕļîéšţ ṽàļîđ šļöţ, šîñçé
  éṽéŕý öṽéŕļàý þŕéšéñţ àţ àþþļý ţîḿé ḿüšţ ƃé ŕéƃàšéđ. ▒

▒ À ƒļöŵ ţĥàţ đéçļàŕéš ţĥé ŕéḿöṽéđ `šöüŕçé_ţŕàñšƒöŕḿš:` ƒîéļđ îš ŕéĵéçţéđ àţ
ļöàđ ŵîţĥ à ḿîĝŕàţîöñ éŕŕöŕ đîŕéçţîñĝ ýöü ţö ļîšţ ţĥé ţŕàñšƒöŕḿéŕš àš öŕđéŕéđ
šţéþš. Šéé ţĥé [ţööļ-šýšţéḿ ÀĐ](/contribute/architecture/engine/e-03-tool-system) ƒöŕ
ţĥé îḿḿüţàƃîļîţý ḿöđéļ àñđ ţĥé þŕöđüçéŕ/àþþļîéŕ šþļîţ. ▒

## ▒ Ĥöŵ šţéþš çöḿþîļé ţö ţĥé ĝŕàþĥ ▒

▒ Ţĥé `ŠţéþšŢöĜŕàþĥ()` ƒüñçţîöñ ţŕàñšƒöŕḿš à `ŠţéþšŠþéç` îñţö `ƑļöŵÑöđé` àñđ `ƑļöŵÉđĝé` šļîçéš: ▒

1. ▒ Éàçĥ šéǫüéñţîàļ šţéþ ƃéçöḿéš à **ţööļ** ñöđé, çĥàîñéđ ƃý éđĝéš ▒
2. ▒ À `þàŕàļļéļ:` ƃļöçķ çŕéàţéš ḿüļţîþļé ţööļ ñöđéš, àļļ çöññéçţéđ ƒŕöḿ ţĥé þŕéṽîöüš ñöđé (ƒàñ-öüţ) ▒
3. ▒ Àƒţéŕ à þàŕàļļéļ ƃļöçķ, šüƃšéǫüéñţ šţéþš çöññéçţ ƒŕöḿ àļļ ƃŕàñçĥ éñđþöîñţš (ƒàñ-îñ) ▒

▒ Ţĥé ĝŕàþĥ îš ţööļ ñöđéš öñļý. Ţĥé ƒļöŵ'š šöüŕçé àñđ šîñķ àŕé ƃîñđîñĝš šüþþļîéđ àţ
ŕüñ ţîḿé ([É-04](architecture/engine/e-04-flows-and-io-binding)), ñöţ ñöđéš îñ ţĥé ĝŕàþĥ. ▒

▒ Ţĥé ŕéšüļţîñĝ ĝŕàþĥ îš ŵĥàţ ţĥé `Éẋéçüţöŕ` ŕüñš -- éàçĥ ñöđé ƃéçöḿéš à ĝöŕöüţîñé çöññéçţéđ ƃý ƃüƒƒéŕéđ çĥàññéļš. ▒

## ▒ Éẋàḿþļé ƒļöŵš ▒

### ▒ Ţŕàñšļàţîöñ þîþéļîñé ▒

▒ À ţýþîçàļ ţŕàñšļàţîöñ ƒļöŵ ŵîţĥ çöñţéñţ-ḿéḿöŕý ļéṽéŕàĝé, ÀÎ ţŕàñšļàţîöñ ƒöŕ ñéŵ ƃļöçķš, àñđ ǫüàļîţý çĥéçķš: ▒

```yaml
steps:
  - tool: create-target
    config:
      targetLocale: fr
      copySource: false

  - tool: recycle
    label: Apply memory matches
    config:
      targetLocale: fr
      fuzzyThreshold: 75

  - tool: translate
    label: Translate remaining
    config:
      targetLocale: fr
      provider: anthropic

  - tool: qa
    label: Quality checks
    config:
      targetLocale: fr
```

### ▒ Ƒàñ-öüţ àñàļýšîš ▒

▒ Ŕüñ ḿüļţîþļé àñàļýšîš ţööļš îñ þàŕàļļéļ àƒţéŕ þšéüđö-ţŕàñšļàţîöñ: ▒

```yaml
steps:
  - tool: pseudo-translate
    config:
      targetLocale: qps-ploc
      expansionPercent: 30

  - parallel:
      - tool: term-check
        config:
          targetLocale: qps-ploc
      - tool: qa
        config:
          targetLocale: qps-ploc
          checkAbsoluteMaxCharLength: true
          absoluteMaxCharLength: 200
```

### ▒ Šçŕîþţ ƒîļţéŕîñĝ ▒

▒ Üšé ţĥé ĴàṽàŠçŕîþţ šçŕîþţ šţéþ ţö ƒîļţéŕ öŕ ţŕàñšƒöŕḿ þàŕţš þŕöĝŕàḿḿàţîçàļļý: ▒

```yaml
steps:
  - tool: script
    label: Skip short blocks
    config:
      code: |
        if (part.type === 'block') {
          var text = part.block.source[0].content.text;
          if (text.length < 3) {
            skip();
          }
        }

  - tool: pseudo-translate
    config:
      targetLocale: fr
```

## ▒ Ŕüññîñĝ ƒļöŵš ▒

### ▒ Ƒŕöḿ ţĥé ÇĻÎ ▒

```bash
# Run a built-in composed flow
kapi run translate-qa -i input.xliff --target-lang fr

# Run a flow defined in a .kapi project file
kapi run my-flow -p kapi.yaml -i input.json

# List available flows
kapi flows
```

### ▒ Þŕöĝŕàḿḿàţîçàļļý ▒

```go
spec := &flow.StepsSpec{
    Input: "json",
    Steps: []flow.FlowStep{
        {Tool: "pseudo-translate", Config: map[string]any{
            "targetLocale": "fr",
            "expansionPercent": 30,
        }},
        {Tool: "qa", Config: map[string]any{
            "targetLocale": "fr",
        }},
    },
}

nodes, edges, err := flow.StepsToGraph(spec)
// Build and execute with Executor...
```
