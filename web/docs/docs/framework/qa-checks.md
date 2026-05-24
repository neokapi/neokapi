---
sidebar_position: 6
title: QA Checks
---

# Quality Assurance Checks

Automated quality checks help ensure translation accuracy and consistency.

## What Are QA Checks?

QA checks are validation rules that run automatically on your translations to catch:

- **Formatting errors** — missing punctuation, whitespace issues
- **Terminology violations** — incorrect or inconsistent terms
- **Completeness issues** — untranslated segments
- **Number mismatches** — different numbers in source vs target
- **Tag mismatches** — missing or incorrect inline markup

## Running QA Checks

### In Flows

Include the `qa-check` tool in your flows:

```yaml
# flows/translate-and-check.yaml
name: translate-and-check
description: Translate with AI and run QA

steps:
  - tool: ai-translate
    config:
      provider: anthropic
      model: claude-sonnet-4.5

  - tool: qa-check
    config:
      fail_on_error: false
      rules:
        - missing-punctuation
        - number-mismatch
        - term-violation
```

## Available QA Rules

| Rule                  | Description                            | Example                                     |
| --------------------- | -------------------------------------- | ------------------------------------------- |
| `missing-punctuation` | Target missing punctuation from source | Source ends with `.`, target doesn't        |
| `number-mismatch`     | Different numbers in source vs target  | Source: "5 items", Target: "3 items"        |
| `term-violation`      | Incorrect terminology                  | Using "login" instead of required "sign in" |
| `tag-mismatch`        | Missing or incorrect inline tags       | Source has `<b>`, target doesn't            |
| `whitespace-mismatch` | Leading/trailing whitespace differs    | Source has trailing space, target doesn't   |

## Implementation Status

:::warning Work in Progress

QA check framework is under development. Current status:

- ✅ QA check tool interface defined
- ✅ Basic validation rules designed
- 🚧 Rule execution engine (in progress)
- 🚧 Integration with flows (in progress)
- ❌ Custom rule plugins (planned)

:::

## Next Steps

- [Terminology](/framework/terminology)
- [Implementing a Tool](/contribute/tools)
