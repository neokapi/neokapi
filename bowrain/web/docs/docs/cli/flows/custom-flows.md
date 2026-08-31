---
sidebar_position: 2
title: Custom Flows
---

# Creating Custom Flows

Define your own workflows as YAML files in `.kapi/flows/`.

## Flow definition format

```yaml
name: my-flow
description: Brief description of what this flow does

steps:
  - tool: tool-name
    config:
      optionA: value

  - tool: another-tool
    config:
      optionB: value
```

Step config keys are the tool's own schema keys, in camelCase, exactly as the
[tool reference](https://neokapi.github.io/tools) lists them. An unrecognized
key is ignored rather than rejected, so a misspelling costs you the option
rather than an error. The locale is never a config key: it comes from the
run's `--target-lang`, or from the recipe inside a project, so one flow serves
every locale. `kapi tools` lists the tools your installation provides,
plugins included.

## Example flows

### Simple AI drafting

`.kapi/flows/translate-simple.yaml`:

```yaml
name: translate-simple
description: AI drafting with the project's default provider settings

steps:
  - tool: translate
    config:
      provider: anthropic
      model: claude-sonnet-5
```

The `provider` and `model` keys accept any provider the
[translate tool](https://neokapi.github.io/reference/tools/translate)
supports; omit them to use the project's defaults.

### Reuse first, then draft, then check

`.kapi/flows/full-translation.yaml`:

```yaml
name: full-translation
description: Reuse from memory, draft the remainder, check terms and quality

steps:
  # 1. Pre-fill from content memory before spending any credits
  - tool: recycle

  # 2. Draft what memory could not fill
  - tool: translate
    config:
      provider: anthropic
      model: claude-sonnet-5

  # 3. Check the terms in force at each block's point
  - tool: term-check
    config:
      caseSensitive: true

  # 4. Run quality checks
  - tool: qa
    config:
      checkDoubleSpaces: true
      checkDoubledWord: true
      checkPatterns: true
      checkTargetInconsistency: true
```

The `recycle` and `term-check` steps read the content memory and the terms the
recipe binds (`defaults.memory_source`, `defaults.terms_source`, or a
profile's own `termstore:`), so they take no path of their own. See the
[recycle](https://neokapi.github.io/reference/tools/recycle) and
[term-check](https://neokapi.github.io/reference/tools/term-check) references
for their options.

### Checks only

`.kapi/flows/checks-only.yaml`:

```yaml
name: checks-only
description: Quality and term checks without drafting

steps:
  - tool: placeholder-check
    config:
      flagExtra: true

  - tool: term-check

  - tool: qa
    config:
      checkDoubleSpaces: true
      checkPatterns: true
```

Run it, then let `kapi check` turn the findings into an exit code; see
[Source language preparation](/cli/use-cases/source-prep).

### Source cleanup

`.kapi/flows/cleanup.yaml`:

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

## The wording constraint

`term-check`, `translate` and `recycle` take one key for the wording
constraint, `term_rules:`, a list of rules naming a term, what to use instead,
and how hard the rule bites. Inside a project you rarely write it by hand: the
voice profile's `vocabulary:` and the bound terms store supply the rules in
force at each block's point. See [Project model](/cli/project-model#tool-configuration).

## Variable substitution

Use environment variables in flow configs:

```yaml
- tool: translate
  config:
    provider: anthropic
    apiKey: ${ANTHROPIC_API_KEY} # From environment
```

## Running custom flows

```bash
# List all flows (built-in + custom)
kapi flows

# Run your custom flow
kapi run my-flow
```

## Best practices

1. **Name flows descriptively**: `translate-review-export` rather than `my-flow`
2. **Document in description**: explain what the flow does and why
3. **Reuse before you draft**: put `recycle` ahead of `translate`
4. **Order matters**: place expensive tools (AI) last
5. **Test incrementally**: add one tool at a time
6. **Commit flows to git**: `.kapi/flows/*.yaml` should be versioned
7. **Gate in CI**: `kapi check --ship` is the enforcement point, not the flow

## Next steps

- [Flow hooks](/cli/flows/hooks)
- [Run command](/cli/commands/run)
- [Available formats](https://neokapi.github.io/formats)
- [Tool reference](https://neokapi.github.io/tools)
