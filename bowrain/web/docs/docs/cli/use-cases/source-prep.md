---
title: Source Language Preparation
sidebar_position: 3
---

# Source Language Preparation

Quality results start with quality source content. kapi provides tools to validate, clean, and prepare source-language content before the convergence loop carries it into every target language.

## Why Source-First Quality Matters

Issues in source content compound through translation. A misspelled term, an inconsistent placeholder format, or a missing sentence boundary in the source will produce the same problem across every target language. Fixing one source issue prevents N target-language issues.

Common source problems:

- **Inconsistent terms**: the same concept called different things in different files
- **Placeholder errors**: mismatched or malformed variables (for example `%s` vs `%d` mismatch)
- **Whitespace issues**: trailing spaces, mixed line endings, zero-width characters
- **Length problems**: strings too long for UI constraints
- **Missing translations**: source strings without corresponding target entries

## QA on Source Content

Run quality checks directly on source files without any server connection:

```bash
# Run QA checks on source content
kapi exec qa -i src/locales/en/ --source-lang en

# Check term consistency
kapi exec term-check -i src/locales/en/ --termstore terms.tbx

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
| `patterns`     | Custom regex patterns (for example brand name capitalization)  |
| `characters`   | Invalid or unexpected Unicode characters                       |

## Example Flows for Source Prep

### Term Consistency Check

Create `.kapi/flows/source-qa.yaml`:

Step config keys are the tool's own schema keys, in camelCase; see
[the tool reference](https://neokapi.github.io/reference/tools/qa).
An unrecognized key is silently ignored, so a misspelling costs you the check
rather than an error. The locale is not a config key either: it comes from the
run's `--target-lang`, so one flow serves every locale.

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

Run it, then gate on the findings: the flow annotates the content, and
`kapi check` is what turns findings into an exit code:

```bash
kapi run source-qa --target-lang fr
kapi check src/locales/en/*.json --target src/locales/fr/app.json \
  --target-lang fr --max-critical 0
```

### Scoping and Content Stats

Before starting a project, analyze the source content:

```bash
# Content stats (blocks, words, characters) across all source files
kapi stats src/locales/en/*.json

# Memory reuse, remaining work, and token estimate for the pending locales
kapi up --plan
```

### Cleanup

`case-transform` normalizes the source; `whitespace-correct` tidies the target
against it, so it runs after a target exists. Step config keys are camelCase,
the same spelling as the tool's schema, which
[the tool reference](https://neokapi.github.io/reference/tools/whitespace-correct)
lists.

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

Run source QA on every pull request that modifies content files:

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

- [Flows](/cli/flows/overview): available tools and flow configuration
- [Terms](https://neokapi.github.io/framework/terminology): managing terms stores
- [QA Checks](https://neokapi.github.io/framework/checks/qa-checks): rule-based quality checks
- [GitHub Actions](/cli/use-cases/github-actions): CI/CD integration
