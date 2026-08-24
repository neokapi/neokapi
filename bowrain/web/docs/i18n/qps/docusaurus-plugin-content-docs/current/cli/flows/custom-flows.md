---
sidebar_position: 2
title: Custom Flows
---

# ▒ Çŕéàţîñĝ Çüšţöḿ Ƒļöŵš ▒

▒ Đéƒîñé ýöüŕ öŵñ ţŕàñšļàţîöñ ŵöŕķƒļöŵš àš ÝÀḾĻ ƒîļéš îñ `.ķàþî/ƒļöŵš/`. ▒

## ▒ Ƒļöŵ Đéƒîñîţîöñ Ƒöŕḿàţ ▒

```yaml
name: my-flow
description: Brief description of what this flow does
steps:
  - tool: tool-name
    config:
      option1: value1
      option2: value2
  - tool: another-tool
    config:
      optionA: valueA
```

## ▒ Éẋàḿþļé Ƒļöŵš ▒

### ▒ Šîḿþļé ÀÎ Ţŕàñšļàţîöñ ▒

▒ `.ķàþî/ƒļöŵš/ţŕàñšļàţé-šîḿþļé.ýàḿļ`: ▒

```yaml
name: translate-simple
description: Basic AI translation without extras
steps:
  - tool: translate
    config:
      provider: anthropic
      model: claude-sonnet-4.5
```

### ▒ Ţŕàñšļàţîöñ ŵîţĥ Þŕé/Þöšţ Þŕöçéššîñĝ ▒

▒ `.ķàþî/ƒļöŵš/ƒüļļ-ţŕàñšļàţîöñ.ýàḿļ`: ▒

```yaml
name: full-translation
description: Complete translation workflow with all bells and whistles
steps:
  # 1. Look up terminology before translating
  - tool: term-lookup
    config:
      fuzzy_threshold: 85
  # 2. Pre-fill from content memory
  - tool: recycle
    config:
      fuzzy_threshold: 70
      provider: memory
  # 3. Translate untranslated blocks with AI
  - tool: translate
    config:
      provider: anthropic
      model: claude-sonnet-4.5
      temperature: 0.3
      skip_translated: true # Only translate empty targets
  # 4. Validate terminology compliance
  - tool: term-enforce
    config:
      required: true
      fail_on_violation: true
  # 5. Run quality checks
  - tool: qa
    config:
      rules:
        - whitespace
        - punctuation
        - placeholders
        - numbers
        - terminology
```

### ▒ Ḿüļţî-Þŕöṽîđéŕ Ţŕàñšļàţîöñ ▒

▒ `.ķàþî/ƒļöŵš/ḿüļţî-ḿţ.ýàḿļ`: ▒

```yaml
name: multi-mt
description: Try DeepL, fall back to Google, finally use AI
steps:
  - tool: translate
    config:
      provider: deepl
      skip_translated: true
  - tool: translate
    config:
      provider: google
      skip_translated: true
  - tool: translate
    config:
      provider: anthropic
      skip_translated: true
```

### ▒ ǪÀ-Öñļý Ƒļöŵ ▒

▒ `.ķàþî/ƒļöŵš/ǫà-öñļý.ýàḿļ`: ▒

```yaml
name: qa-only
description: Quality assurance checks without translation
steps:
  - tool: qa
    config:
      rules:
        - whitespace
        - punctuation
        - placeholders
        - case
        - spelling
  - tool: term-enforce
  - tool: qa
    config:
      provider: anthropic
      model: claude-sonnet-4.5
      checks:
        - fluency
        - accuracy
        - consistency
```

## ▒ Ţööļ Çöñƒîĝüŕàţîöñ ▒

▒ Éàçĥ ţööļ ĥàš îţš öŵñ çöñƒîĝüŕàţîöñ öþţîöñš. Çöḿḿöñ þàţţéŕñš: ▒

### ▒ ÀÎ Ţŕàñšļàţîöñ Ţööļš ▒

```yaml
- tool: translate
  config:
    provider: anthropic | openai | ollama
    model: claude-sonnet-4.5 | gpt-4o | llama3:70b
    temperature: 0.0-1.0 # Creativity (0 = deterministic)
    skip_translated: true # Only translate empty targets
```

### ▒ ḾŢ Ţŕàñšļàţîöñ Ţööļš ▒

```yaml
- tool: translate
  config:
    provider: deepl | google | microsoft | modernmt | mymemory
    api_key: ${DEEPL_API_KEY} # Environment variable
    formality: formal | informal
    skip_translated: true
```

### ▒ Ŕéüšé ƒŕöḿ çöñţéñţ ḿéḿöŕý ▒

```yaml
- tool: recycle
  config:
    fuzzy_threshold: 70 # Match threshold (0-100)
    provider: memory | null
    tmx_path: ./my-tm.tmx # Optional TMX import
```

### ▒ ǪÀ Çĥéçķ ▒

```yaml
- tool: qa
  config:
    rules:
      - whitespace # Leading/trailing/double spaces
      - punctuation # Mismatched punctuation
      - placeholders # Missing/extra placeholders
      - numbers # Number consistency
      - case # Uppercase/lowercase consistency
      - spelling # Spell check (requires hunspell)
      - terminology # Term compliance (requires a bound terms store)
```

### ▒ Ţéŕḿîñöļöĝý ▒

```yaml
- tool: term-lookup
  config:
    fuzzy_threshold: 85
    domain: software # Filter by domain
- tool: term-enforce
  config:
    required: true # Block must use term if available
    fail_on_violation: true # Exit flow if violation found
```

## ▒ Ṽàŕîàƃļé Šüƃšţîţüţîöñ ▒

▒ Üšé éñṽîŕöñḿéñţ ṽàŕîàƃļéš îñ ƒļöŵ çöñƒîĝš: ▒

```yaml
- tool: translate
  config:
    provider: anthropic
    api_key: ${ANTHROPIC_API_KEY} # From environment
```

## ▒ Ŕüññîñĝ Çüšţöḿ Ƒļöŵš ▒

```bash
# List all flows (built-in + custom)
kapi flows
# Run your custom flow
kapi run my-flow
# Run with verbose output
kapi run my-flow --verbose
```

## ▒ Ƃéšţ Þŕàçţîçéš ▒

1. ▒ **Ñàḿé ƒļöŵš đéšçŕîþţîṽéļý**: `ţŕàñšļàţé-ŕéṽîéŵ-éẋþöŕţ` ñöţ `ḿý-ƒļöŵ` ▒
2. ▒ **Đöçüḿéñţ îñ đéšçŕîþţîöñ**: Éẋþļàîñ ŵĥàţ ţĥé ƒļöŵ đöéš àñđ ŵĥý ▒
3. ▒ **Üšé šķîþ_ţŕàñšļàţéđ**: Àṽöîđ ŕéţŕàñšļàţîñĝ éẋîšţîñĝ çöñţéñţ ▒
4. ▒ **Öŕđéŕ ḿàţţéŕš**: Þļàçé éẋþéñšîṽé ţööļš (ÀÎ) ļàšţ ▒
5. ▒ **Ţéšţ îñçŕéḿéñţàļļý**: Àđđ öñé ţööļ àţ à ţîḿé ▒
6. ▒ **Çöḿḿîţ ƒļöŵš ţö ĝîţ**: `.ķàþî/ƒļöŵš/*.ýàḿļ` šĥöüļđ ƃé ṽéŕšîöñéđ ▒
7. ▒ **Üšé ĥööķš ƒöŕ ĝàţéš**: Þŕé-þüšĥ ǪÀ þŕéṽéñţš ƃàđ üþļöàđš ▒

## ▒ Ñéẋţ Šţéþš ▒

- ▒ [Ƒļöŵ Ĥööķš](/cli/flows/hooks) ▒
- ▒ [Ŕüñ Çöḿḿàñđ](/cli/commands/run) ▒
- ▒ [Àṽàîļàƃļé Ƒöŕḿàţš](https://neokapi.github.io/formats) ▒
- ▒ [ÀÎ Ţŕàñšļàţîöñ](https://neokapi.github.io/framework/translation) ▒
