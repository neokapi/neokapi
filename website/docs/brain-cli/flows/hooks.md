---
sidebar_position: 3
title: Hooks
---

# Flow Hooks

Hooks are automatic tool chains that run before/after sync operations to enforce quality gates and process content.

## What Are Hooks?

Hooks are flows that run automatically during `brain push` and `brain pull`:

- **pre-push**: Run before uploading to server (quality gates)
- **post-pull**: Run after fetching from server (post-processing)

## Configuration

Define hooks in `.brain/config.yaml`:

```yaml
hooks:
  pre-push:
    - qa-check
    - term-enforce
  post-pull:
    - segmentation
```

## Pre-Push Hooks

Pre-push hooks run **before** `brain push` uploads content to Bowrain Server.

### Use Cases

- **Quality gates**: Block pushes with errors or violations
- **Terminology enforcement**: Ensure required terms are used
- **Style validation**: Check formatting, capitalization, spelling
- **Completeness checks**: Verify all blocks are translated

### Example

`.brain/config.yaml`:

```yaml
hooks:
  pre-push:
    - qa-check
    - term-enforce
```

When you run `brain push`, these tools run on local content:

```bash
$ brain push -m "Translate new features"

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
# .brain/flows/strict-qa.yaml
name: strict-qa
description: Strict QA checks that block push

steps:
  - tool: qa-check
    config:
      fail_on_error: true  # Exit with error code

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

Post-pull hooks run **after** `brain pull` fetches content from the server.

### Use Cases

- **Segmentation**: Split fetched source text into sentences
- **Formatting**: Normalize whitespace, capitalization
- **Extraction**: Extract terminology from new content
- **Caching**: Warm up local TM cache

### Example

`.brain/config.yaml`:

```yaml
hooks:
  post-pull:
    - segmentation
    - term-lookup
```

When you run `brain pull`:

```bash
$ brain pull

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
brain push --no-hooks

# Use --force to bypass quality gates but still run hooks
brain push --force
```

**Difference:**
- `--no-hooks`: Don't run any hooks
- `--force`: Run hooks but ignore errors

## Hook Execution

Hooks run as regular flows:

1. **Read files**: Load files matching `.brain/config.yaml` mappings
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
brain push --no-hooks

# Staging: medium
# .brain/config.yaml has qa-check only

# Production: strict
# Production branch has qa-check + term-enforce + spell-check
```

Or use environment-specific configs:

```yaml
# .brain/config.dev.yaml
hooks:
  pre-push: []

# .brain/config.prod.yaml
hooks:
  pre-push:
    - qa-check
    - term-enforce
    - spell-check
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

- [Custom Flows](/docs/brain-cli/flows/custom-flows)
- [Push Command](/docs/brain-cli/commands/push)
- [Pull Command](/docs/brain-cli/commands/pull)
- [QA Checks](/docs/features/qa-checks)
