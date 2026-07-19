---
title: Source Language Preparation
sidebar_position: 3
---

# Source Language Preparation

Quality translations start with quality source content. kapi provides tools to validate, clean, and prepare source-language content before it enters the translation pipeline.

## Why Source-First Quality Matters

Issues in source content compound through translation. A misspelled term, an inconsistent placeholder format, or a missing sentence boundary in the source will produce the same problem across every target language. Fixing one source issue prevents N target-language issues.

Common source problems:

- **Inconsistent terminology** — the same concept called different things in different files
- **Placeholder errors** — mismatched or malformed variables (e.g., `%s` vs `%d` mismatch)
- **Whitespace issues** — trailing spaces, mixed line endings, zero-width characters
- **Length problems** — strings too long for UI constraints
- **Missing translations** — source strings without corresponding target entries

## QA on Source Content

Run quality checks directly on source files without any server connection:

```bash
# Run QA checks on source content
kapi exec qa -i src/locales/en/ --source-lang en

# Check terminology consistency
kapi exec term-check -i src/locales/en/ --termbase glossary.tbx

# Validate XML/HTML structure in source strings
kapi xml-validation -i src/locales/en/
```

### Built-In QA Rules

The `qa` tool validates:

| Rule           | What It Checks                                                 |
| -------------- | -------------------------------------------------------------- |
| `whitespace`   | Leading/trailing whitespace, double spaces, mixed line endings |
| `punctuation`  | Missing or mismatched sentence-ending punctuation              |
| `placeholders` | Placeholder format consistency and completeness                |
| `terminology`  | Required terms present and correctly used                      |
| `length`       | String length within configured limits                         |
| `patterns`     | Custom regex patterns (e.g., brand name capitalization)        |
| `characters`   | Invalid or unexpected Unicode characters                       |

## Example Flows for Source Prep

### Terminology Consistency Check

Create `.kapi/flows/source-qa.yaml`:

```yaml
name: source-qa
description: Validate source content quality before translation

steps:
  - tool: term-check
    config:
      target_locale: en-US

  - tool: qa
    config:
      rules:
        - whitespace
        - placeholders
        - patterns
      checkTargetInconsistency: true
      fail_on_error: true
```

Run it:

```bash
kapi run source-qa
```

### Scoping and Content Stats

Before starting a translation project, analyze the source content:

```bash
# Content stats (blocks, words, characters) across all source files
kapi stats src/locales/en/*.json

# TM leverage, remaining work, and token estimate for the pending locales
kapi up --plan
```

### Source Cleanup

Normalize source content before translation:

```yaml
name: source-cleanup
description: Normalize source content

steps:
  - tool: whitespace-correct
    config:
      normalize_spaces: true
      match_source_whitespace: true
      remove_zero_width_chars: true

  - tool: case-transform
    config:
      mode: title
      apply_source: true
```

## CI Integration

### Pre-Push QA Gate

Add source QA to your CI pipeline so content that fails validation never reaches the server. Declare local automation rules at the top level of your `kapi.yaml` recipe:

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

### GitHub Actions

Run source QA on every pull request that modifies localization files:

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

This catches source-language issues at the PR stage, before they propagate to translations.

## Related

- [Translation Flows](/cli/flows/overview) — available tools and flow configuration
- [Terminology](https://neokapi.github.io/web/neokapi/framework/terminology) — managing termbases
- [QA Checks](https://neokapi.github.io/web/neokapi/framework/checks/qa-checks) — rule-based quality checks
- [GitHub Actions](/cli/use-cases/github-actions) — CI/CD integration
