---
title: push
sidebar_position: 5
---

# kapi push

Send local file changes to Bowrain Server. Only transfers modified blocks
(incremental sync using content hashing).

## Usage

```bash
kapi push [paths...] [flags]
```

## Examples

```bash
# Push all local changes to server
kapi push

# Push specific files
kapi push src/locales/en/

# Show what would be pushed without uploading
kapi push --dry-run

# Push with a commit message
kapi push -m "Add new welcome messages"

# Bypass quality gates (use with caution!)
kapi push --force

# Skip pre-push hooks
kapi push --no-hooks

# Example output:
# Pushing to: https://bowrain.example.com
# Project: abc123
#
# Running pre-push hooks: [qa-check, term-enforce]
# ✓ qa-check: 0 issues found
# ✓ term-enforce: 2 terminology violations (blocking)
#
# Error: Quality gates failed
# - Block 'error_message': missing required term "username" (use "nom d'utilisateur" in French)
#
# Fix issues or use --force to bypass
```

## Options

| Flag | Short | Description | Default |
|------|-------|-------------|---------|
| `--force` | | Bypass quality gates | `false` |
| `--dry-run` | | Show what would be pushed | `false` |
| `--message` | `-m` | Commit message for this push | `""` |
| `--no-hooks` | | Skip pre-push hooks | `false` |

## What Happens

1. **Read local files** via FormatRegistry
2. **Compute block hashes** using `BlockIdentity`
3. **Compare with `.kapi/.state.json`** to identify changed blocks
4. **Run pre-push hooks** if configured (unless `--no-hooks`)
   - Hooks may **block push** if quality gates fail
   - Use `--force` to bypass (not recommended)
5. **Verify auth** token (`.kapi/.server-token` or environment variable)
6. **Call server API** `POST /api/v1/workspaces/:ws/projects/:id/push`
   - Send changed blocks + item mappings + message
   - Server may **reject** if quality gates fail
7. **Update `.kapi/.state.json`** with new sync state

## Content Hashing

Kapi uses content-addressed blocks for efficient sync:

```
block_hash = sha256(block_id + source_text + context)
```

Only blocks with changed hashes are transferred. Large projects sync in seconds.

## Pre-Push Hooks

Hooks run before uploading to catch quality issues early. Configure in `.kapi/config.yaml`:

```yaml
hooks:
  pre-push:
    - qa-check         # Rule-based quality checks
    - term-enforce     # Validate required terminology
```

Hooks are tool chains that process content before push:

- **qa-check**: Detects common errors (whitespace, punctuation, placeholders)
- **term-enforce**: Ensures required terms are used correctly
- Custom tools: Any tool in the registry can be a hook

### Hook Failure

If hooks find **blocking issues**, the push is aborted:

```
Running pre-push hooks: [qa-check, term-enforce]
✓ qa-check: 0 issues found
✗ term-enforce: 2 violations (blocking)

Block 'login_button':
  Missing required term "sign in" (found "log in")

Block 'password_field':
  Incorrect translation of "password" → use "mot de passe" not "code secret"

Push aborted. Fix issues or use --force to bypass.
```

Use `--force` to push anyway (bypasses local hooks but **not** server-side gates).

## Quality Gates

The server may also enforce quality gates:

- **Terminology compliance**
- **Translation memory fuzzy match threshold**
- **Custom validation rules**

These **cannot** be bypassed with `--force`. Contact your workspace admin to adjust gate settings.

## Commit Messages

Use `-m` to document why changes were made:

```bash
kapi push -m "Translate new error messages for v2.0 release"
```

Messages appear in the server's change history for the project. Required by some workspace policies.

## File Mappings

`.kapi/config.yaml` defines how local files map to server items:

```yaml
mappings:
  - local: src/locales/**/*.json
    remote: ui/strings/{path}
    format: json
```

Templates expand during push:

| Local File | Remote Item |
|------------|-------------|
| `src/locales/en/buttons.json` | `ui/strings/en/buttons` |
| `src/locales/fr/buttons.json` | `ui/strings/fr/buttons` |

This allows flexible file organization while maintaining clean server structure.

## Implementation Status

:::warning Work in Progress

`kapi push` is currently a **placeholder**. Full implementation requires:

- Server API endpoint: `POST /api/v1/workspaces/:ws/projects/:id/push`
- Block hash computation
- FormatRegistry integration for reading files
- Hook execution framework

Current behavior: prints a message indicating the feature is not yet implemented.

:::

## Exit Codes

- `0` — Success (changes pushed)
- `1` — Quality gates failed (fix issues or use `--force`)
- `2` — Error (auth failed, server rejected, network error, etc.)

## Authentication

`kapi push` requires a valid server auth token:

1. **Environment variable**: `KAPI_SERVER_TOKEN`
2. **Token file**: `.kapi/.server-token` (auto-gitignored)
3. **Interactive login**: `kapi auth login` (stores token in file)

If no token is found, you'll be prompted to authenticate.

## Related Commands

- [`kapi pull`](/docs/kapi-cli/commands/pull) — Fetch changes from server
- [`kapi status`](/docs/kapi-cli/commands/status) — Show what will be pushed
- [`kapi diff`](/docs/kapi-cli/commands/diff) — Show detailed changes

## When to Use

Push to Bowrain Server to:

- **Share translations** with your team
- **Trigger workflows** (AI translation, QA, terminology extraction)
- **Backup content** to the server
- **Integrate with CI/CD** pipelines

Think of it as `git push` for localization content.

## Best Practices

1. **Run `kapi status`** before pushing to see what changed
2. **Use commit messages** (`-m`) to document your changes
3. **Let hooks run** — don't use `--no-hooks` without good reason
4. **Pull first** if working with a team to avoid conflicts
5. **Use `--dry-run`** when unsure about what will be uploaded
