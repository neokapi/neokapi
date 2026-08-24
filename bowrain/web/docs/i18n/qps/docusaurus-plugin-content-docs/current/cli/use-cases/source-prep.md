---
title: Source Language Preparation
sidebar_position: 3
---

# ▒ Šöüŕçé Ļàñĝüàĝé Þŕéþàŕàţîöñ ▒

▒ Ǫüàļîţý ţŕàñšļàţîöñš šţàŕţ ŵîţĥ ǫüàļîţý šöüŕçé çöñţéñţ. ķàþî þŕöṽîđéš ţööļš ţö ṽàļîđàţé, çļéàñ, àñđ þŕéþàŕé šöüŕçé-ļàñĝüàĝé çöñţéñţ ƃéƒöŕé ţĥé çöñṽéŕĝéñçé ļööþ çàŕŕîéš îţ îñţö éṽéŕý ţàŕĝéţ ļàñĝüàĝé. ▒

## ▒ Ŵĥý Šöüŕçé-Ƒîŕšţ Ǫüàļîţý Ḿàţţéŕš ▒

▒ Îššüéš îñ šöüŕçé çöñţéñţ çöḿþöüñđ ţĥŕöüĝĥ ţŕàñšļàţîöñ. À ḿîššþéļļéđ ţéŕḿ, àñ îñçöñšîšţéñţ þļàçéĥöļđéŕ ƒöŕḿàţ, öŕ à ḿîššîñĝ šéñţéñçé ƃöüñđàŕý îñ ţĥé šöüŕçé ŵîļļ þŕöđüçé ţĥé šàḿé þŕöƃļéḿ àçŕöšš éṽéŕý ţàŕĝéţ ļàñĝüàĝé. Ƒîẋîñĝ öñé šöüŕçé îššüé þŕéṽéñţš Ñ ţàŕĝéţ-ļàñĝüàĝé îššüéš. ▒

▒ Çöḿḿöñ šöüŕçé þŕöƃļéḿš: ▒

- ▒ **Îñçöñšîšţéñţ ţéŕḿîñöļöĝý** — ţĥé šàḿé çöñçéþţ çàļļéđ đîƒƒéŕéñţ ţĥîñĝš îñ đîƒƒéŕéñţ ƒîļéš ▒
- ▒ **Þļàçéĥöļđéŕ éŕŕöŕš** — ḿîšḿàţçĥéđ öŕ ḿàļƒöŕḿéđ ṽàŕîàƃļéš (é.ĝ., `%š` ṽš `%đ` ḿîšḿàţçĥ) ▒
- ▒ **Ŵĥîţéšþàçé îššüéš** — ţŕàîļîñĝ šþàçéš, ḿîẋéđ ļîñé éñđîñĝš, žéŕö-ŵîđţĥ çĥàŕàçţéŕš ▒
- ▒ **Ļéñĝţĥ þŕöƃļéḿš** — šţŕîñĝš ţöö ļöñĝ ƒöŕ ÜÎ çöñšţŕàîñţš ▒
- ▒ **Ḿîššîñĝ ţŕàñšļàţîöñš** — šöüŕçé šţŕîñĝš ŵîţĥöüţ çöŕŕéšþöñđîñĝ ţàŕĝéţ éñţŕîéš ▒

## ▒ ǪÀ öñ Šöüŕçé Çöñţéñţ ▒

▒ Ŕüñ ǫüàļîţý çĥéçķš đîŕéçţļý öñ šöüŕçé ƒîļéš ŵîţĥöüţ àñý šéŕṽéŕ çöññéçţîöñ: ▒

```bash
# Run QA checks on source content
kapi exec qa -i src/locales/en/ --source-lang en

# Check terminology consistency
kapi exec term-check -i src/locales/en/ --termstore terms.tbx

# Validate XML/HTML structure in source strings
kapi xml-validation -i src/locales/en/
```

### ▒ Ƃüîļţ-Îñ ǪÀ Ŕüļéš ▒

▒ Ţĥé `ǫà` ţööļ ṽàļîđàţéš: ▒

| Rule           | What It Checks                                                 |
| -------------- | -------------------------------------------------------------- |
| `whitespace`   | Leading/trailing whitespace, double spaces, mixed line endings |
| `punctuation`  | Missing or mismatched sentence-ending punctuation              |
| `placeholders` | Placeholder format consistency and completeness                |
| `terminology`  | Required terms present and correctly used                      |
| `length`       | String length within configured limits                         |
| `patterns`     | Custom regex patterns (e.g., brand name capitalization)        |
| `characters`   | Invalid or unexpected Unicode characters                       |

## ▒ Éẋàḿþļé Ƒļöŵš ƒöŕ Šöüŕçé Þŕéþ ▒

### ▒ Ţéŕḿîñöļöĝý Çöñšîšţéñçý Çĥéçķ ▒

▒ Çŕéàţé `.ķàþî/ƒļöŵš/šöüŕçé-ǫà.ýàḿļ`: ▒

▒ Šţéþ çöñƒîĝ ķéýš àŕé ţĥé ţööļ'š öŵñ šçĥéḿà ķéýš, îñ çàḿéļÇàšé — šéé
[ţĥé ţööļ ŕéƒéŕéñçé](https://neokapi.github.io/reference/tools/qa).
Àñ üñŕéçöĝñîžéđ ķéý îš šîļéñţļý îĝñöŕéđ, šö à ḿîššþéļļîñĝ çöšţš ýöü ţĥé çĥéçķ
ŕàţĥéŕ ţĥàñ àñ éŕŕöŕ. Ţĥé ļöçàļé îš ñöţ à çöñƒîĝ ķéý éîţĥéŕ: îţ çöḿéš ƒŕöḿ ţĥé
ŕüñ'š `--ţàŕĝéţ-ļàñĝ`, šö öñé ƒļöŵ šéŕṽéš éṽéŕý ļöçàļé. ▒

```yaml
name: source-qa
description: Validate source content quality before translation

steps:
  - tool: term-check
    config:
      caseSensitive: true

  - tool: placeholder-check
    config:
      flagExtra: true

  - tool: qa
    config:
      checkDoubleSpaces: true
      checkDoubledWord: true
      checkPatterns: true
      checkTargetInconsistency: true
```

▒ Ŕüñ îţ, ţĥéñ ĝàţé öñ ţĥé ƒîñđîñĝš — ţĥé ƒļöŵ àññöţàţéš ţĥé çöñţéñţ, àñđ
`ķàþî çĥéçķ` îš ŵĥàţ ţüŕñš ƒîñđîñĝš îñţö àñ éẋîţ çöđé: ▒

```bash
kapi run source-qa --target-lang fr
kapi check src/locales/en/*.json --target src/locales/fr/app.json \
  --target-lang fr --max-critical 0
```

### ▒ Šçöþîñĝ àñđ Çöñţéñţ Šţàţš ▒

▒ Ƃéƒöŕé šţàŕţîñĝ à ţŕàñšļàţîöñ þŕöĵéçţ, àñàļýžé ţĥé šöüŕçé çöñţéñţ: ▒

```bash
# Content stats (blocks, words, characters) across all source files
kapi stats src/locales/en/*.json

# Memory reuse, remaining work, and token estimate for the pending locales
kapi up --plan
```

### ▒ Çļéàñüþ ▒

▒ `çàšé-ţŕàñšƒöŕḿ` ñöŕḿàļîžéš ţĥé šöüŕçé; `ŵĥîţéšþàçé-çöŕŕéçţ` ţîđîéš ţĥé ţàŕĝéţ
àĝàîñšţ îţ, šö îţ ŕüñš àƒţéŕ à ţàŕĝéţ éẋîšţš. Šţéþ çöñƒîĝ ķéýš àŕé çàḿéļÇàšé —
ţĥé šàḿé šþéļļîñĝ àš ţĥé ţööļ'š šçĥéḿà, ŵĥîçĥ
[ţĥé ţööļ ŕéƒéŕéñçé](https://neokapi.github.io/reference/tools/whitespace-correct)
ļîšţš. ▒

```yaml
name: cleanup
description: Normalize source case, then tidy the target's whitespace

steps:
  - tool: case-transform
    config:
      mode: title
      applySource: true

  - tool: whitespace-correct
    config:
      normalizeSpaces: true
      matchSourceWhitespace: true
      removeZeroWidthChars: true
```

## ▒ ÇÎ Îñţéĝŕàţîöñ ▒

### ▒ Þŕé-Þüšĥ ǪÀ Ĝàţé ▒

▒ Àđđ šöüŕçé ǪÀ ţö ýöüŕ ÇÎ þîþéļîñé šö çöñţéñţ ţĥàţ ƒàîļš ṽàļîđàţîöñ ñéṽéŕ ŕéàçĥéš ţĥé šéŕṽéŕ. Đéçļàŕé ļöçàļ àüţöḿàţîöñ ŕüļéš àţ ţĥé ţöþ ļéṽéļ öƒ ýöüŕ `ķàþî.ýàḿļ` ŕéçîþé: ▒

```yaml
automations:
  - name: qa-before-push
    trigger: pre-push
    actions:
      - type: run_flow
        config:
          flow: source-qa
          fail_on_error: true
```

### ▒ ĜîţĤüƃ Àçţîöñš ▒

▒ Ŕüñ šöüŕçé ǪÀ öñ éṽéŕý þüļļ ŕéǫüéšţ ţĥàţ ḿöđîƒîéš çöñţéñţ ƒîļéš: ▒

```yaml
name: Source QA

on:
  pull_request:
    paths:
      - "src/locales/en/**"

jobs:
  source-qa:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4

      - uses: neokapi/setup-kapi@v1
        with:
          plugins: bowrain

      - name: Run source QA
        run: kapi run source-qa

      - name: Content stats report
        run: kapi stats src/locales/en/*.json --json
```

▒ Ţĥîš çàţçĥéš šöüŕçé-ļàñĝüàĝé îššüéš àţ ţĥé ÞŔ šţàĝé, ƃéƒöŕé ţĥéý þŕöþàĝàţé ţö ţŕàñšļàţîöñš. ▒

## ▒ Ŕéļàţéđ ▒

- ▒ [Ţŕàñšļàţîöñ Ƒļöŵš](/cli/flows/overview) — àṽàîļàƃļé ţööļš àñđ ƒļöŵ çöñƒîĝüŕàţîöñ ▒
- ▒ [Ţéŕḿîñöļöĝý](https://neokapi.github.io/framework/terminology) — ḿàñàĝîñĝ ţéŕḿš šţöŕéš ▒
- ▒ [ǪÀ Çĥéçķš](https://neokapi.github.io/framework/checks/qa-checks) — ŕüļé-ƃàšéđ ǫüàļîţý çĥéçķš ▒
- ▒ [ĜîţĤüƃ Àçţîöñš](/cli/use-cases/github-actions) — ÇÎ/ÇĐ îñţéĝŕàţîöñ ▒
