---
sidebar_position: 3
title: Hooks
---

# Flow Hooks

Hooks are automatic tool chains that run before/after sync operations to enforce quality gates and process content.

## What Are Hooks?

Hooks are flows that run automatically during `kapi push` and `kapi pull`:

- **pre-push**: Run before uploading to server (quality gates)
- **post-pull**: Run after fetching from server (post-processing)

## Configuration

Define hooks at the top level of your `<dir-name>.kapi` recipe:

```yaml
hooks:
  pre-push:
    - qa-check
    - term-enforce
  post-pull:
    - segmentation
```

## Pre-Push Hooks

Pre-push hooks run **before** `kapi push` uploads content to Bowrain Server.

### Use Cases

- **Quality gates**: Block pushes with errors or violations
- **Terminology enforcement**: Ensure required terms are used
- **Style validation**: Check formatting, capitalization, spelling
- **Completeness checks**: Verify all blocks are translated

### Example

`my-app.kapi`:

```yaml
hooks:
  pre-push:
    - qa-check
    - term-enforce
```

When you run `kapi push`, these tools run on local content:

```bash
$ kapi push -m "Translate new features"

Running pre-push hooks: [qa-check, term-enforce]
ok qa-check: 0 issues found
FAIL term-enforce: 2 violations (blocking)

Block 'login_button':
  Missing required term "sign in" (found "log in")

Block 'password_field':
  Incorrect translation of "password" -> use "mot de passe"

Push aborted. Fix issues or use --force to bypass.
```

### Blocking vs. Non-Blocking

Hooks can be configured to block or warn:

```yaml
# Create a flow that fails on violations
# .kapi/flows/strict-qa.yaml
name: strict-qa
description: Strict QA checks that block push

steps:
  - tool: qa-check
    config:
      fail_on_error: true # Exit with error code

  - tool: term-enforce
    config:
      fail_on_violation: true
```

Then reference in hooks:

```yaml
hooks:
  pre-push:
    - strict-qa
```

## Post-Pull Hooks

Post-pull hooks run **after** `kapi pull` fetches content from the server.

### Use Cases

- **Segmentation**: Split fetched source text into sentences
- **Formatting**: Normalize whitespace, capitalization
- **Extraction**: Extract terminology from new content
- **Caching**: Warm up local TM cache

### Example

`my-app.kapi`:

```yaml
hooks:
  post-pull:
    - segmentation
    - term-lookup
```

When you run `kapi pull`:

```bash
$ kapi pull

Pulling from: https://bowrain.example.com
Project: abc123

Fetching changes...
ok 3 files updated

Running post-pull hooks: [segmentation, term-lookup]
ok Segmented 42 blocks into 128 segments
ok Extracted 15 new terms

Pull complete.
```

## Bypassing Hooks

Skip hooks with `--no-hooks`:

```bash
# Skip all hooks
kapi push --no-hooks

# Use --force to bypass quality gates but still run hooks
kapi push --force
```

**Difference:**

- `--no-hooks`: Don't run any hooks
- `--force`: Run hooks but ignore errors

## Hook Execution

Hooks run as regular flows:

1. **Read files**: Load files matching the recipe's `content:` collections
2. **Process blocks**: Run each tool in the hook sequence
3. **Check results**: If any tool exits with error, abort operation
4. **Write files**: Save processed content back to local files

## Best Practices

### Pre-Push Hooks

**Do:**

- Enforce terminology compliance
- Check for formatting errors
- Validate required translations
- Block on critical errors

**Don't:**

- Run expensive operations (AI translation)
- Make network calls (MT services)
- Modify content (hooks should validate, not transform)

### Post-Pull Hooks

**Do:**

- Segment new source text
- Extract terminology
- Warm up caches
- Format content consistently

**Don't:**

- Make network calls
- Run expensive analysis
- Modify source blocks

## Example Configurations

### Minimal Quality Gates

```yaml
hooks:
  pre-push:
    - qa-check
```

### Strict Enforcement

```yaml
hooks:
  pre-push:
    - qa-check
    - term-enforce
    - spell-check

  post-pull:
    - segmentation
```

### Development vs. Production

Use different hooks per environment:

```bash
# Development: lenient
kapi push --no-hooks

# Staging: medium
# Recipe has qa-check only

# Production: strict
# Production branch has qa-check + term-enforce + spell-check
```

Or use environment-specific recipes (committed alongside the canonical one) and pass them with `-p`:

```bash
kapi push -p my-app.dev.kapi    # lenient
kapi push -p my-app.prod.kapi   # strict
```

## Implementation Status

:::warning Work in Progress

Hook execution is currently a **placeholder**. Full implementation requires:

- Hook execution framework
- Error aggregation and reporting
- File state management
- Integration with push/pull commands

Current behavior: hooks are documented in config but not executed.

:::

## Next Steps

- [Custom Flows](/cli/flows/custom-flows)
- [Push Command](/cli/commands/push)
- [Pull Command](/cli/commands/pull)
- [QA Checks](https://neokapi.github.io/web/neokapi/docs/features/qa-checks)
