---
sidebar_position: 2
title: Custom Flows
---

# Creating Custom Flows

Define your own translation workflows as YAML files in `.kapi/flows/`.

## Flow Definition Format

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

## Example Flows

### Simple AI Translation

`.kapi/flows/translate-simple.yaml`:

```yaml
name: translate-simple
description: Basic AI translation without extras

steps:
  - tool: translate
    config:
      provider: anthropic
      model: claude-sonnet-4.5
```

### Translation with Pre/Post Processing

`.kapi/flows/full-translation.yaml`:

```yaml
name: full-translation
description: Complete translation workflow with all bells and whistles

steps:
  # 1. Look up terminology before translating
  - tool: term-lookup
    config:
      fuzzy_threshold: 85

  # 2. Pre-fill from translation memory
  - tool: tm-leverage
    config:
      fuzzy_threshold: 70
      provider: sievepen

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

### Multi-Provider Translation

`.kapi/flows/multi-mt.yaml`:

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

### QA-Only Flow

`.kapi/flows/qa-only.yaml`:

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

## Tool Configuration

Each tool has its own configuration options. Common patterns:

### AI Translation Tools

```yaml
- tool: translate
  config:
    provider: anthropic | openai | ollama
    model: claude-sonnet-4.5 | gpt-4o | llama3:70b
    temperature: 0.0-1.0 # Creativity (0 = deterministic)
    skip_translated: true # Only translate empty targets
```

### MT Translation Tools

```yaml
- tool: translate
  config:
    provider: deepl | google | microsoft | modernmt | mymemory
    api_key: ${DEEPL_API_KEY} # Environment variable
    formality: formal | informal
    skip_translated: true
```

### TM Leverage

```yaml
- tool: tm-leverage
  config:
    fuzzy_threshold: 70 # Match threshold (0-100)
    provider: sievepen | null
    tmx_path: ./my-tm.tmx # Optional TMX import
```

### QA Check

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
      - terminology # Term compliance (requires termbase)
```

### Terminology

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

## Variable Substitution

Use environment variables in flow configs:

```yaml
- tool: translate
  config:
    provider: anthropic
    api_key: ${ANTHROPIC_API_KEY} # From environment
```

## Running Custom Flows

```bash
# List all flows (built-in + custom)
kapi flows

# Run your custom flow
kapi run my-flow

# Run with verbose output
kapi run my-flow --verbose
```

## Best Practices

1. **Name flows descriptively**: `translate-review-export` not `my-flow`
2. **Document in description**: Explain what the flow does and why
3. **Use skip_translated**: Avoid retranslating existing content
4. **Order matters**: Place expensive tools (AI) last
5. **Test incrementally**: Add one tool at a time
6. **Commit flows to git**: `.kapi/flows/*.yaml` should be versioned
7. **Use hooks for gates**: Pre-push QA prevents bad uploads

## Next Steps

- [Flow Hooks](/cli/flows/hooks)
- [Run Command](/cli/commands/run)
- [Available Formats](https://neokapi.github.io/web/neokapi/docs/features/formats)
- [AI Translation](https://neokapi.github.io/web/neokapi/docs/features/ai-translation)
