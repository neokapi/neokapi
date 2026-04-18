---
sidebar_position: 11
title: sync
---

# kapi sync

Bring a [`.kapi` project](/docs/ad/041-kapi-desktop)'s translations up to date by running a translation tool for every (archive, missing-locale) pair.

## Synopsis

```bash
kapi sync -p <project.kapi> [--tool <tool-name>] [--dry-run]
```

## Description

`kapi sync` iterates every content collection that declares an `archive:` field and consults each archive against the project's declared target languages. For each locale whose coverage is incomplete, it runs the named tool (default: `ai-translate`) against the archive with `--target-lang <locale>`.

The `kapi` writer is locale-additive, so repeated sync runs accumulate new locales without clobbering existing targets. Pass `--dry-run` to preview the plan without executing.

`kapi sync` does **not** re-extract source content — run your extractor (`kapi-react extract`, etc.) first to refresh the archive. Sync is the translation half of the round-trip.

## Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--project` | `-p` | — | Path to the `.kapi` project file (required). |
| `--tool` | | `ai-translate` | Tool to invoke per missing locale. |
| `--dry-run` | | `false` | Print the plan and exit without executing. |

## Example

```bash
$ kapi sync -p translation.kapi --tool ai-translate --dry-run
translation.kapi: 2 step(s) to bring translations up to date
  ui [de] not translated → kapi ai-translate i18n/ui.klz --target-lang de
  ui [fr] 987/1007 translated → kapi ai-translate i18n/ui.klz --target-lang fr

--dry-run: not executing. Re-run without --dry-run to apply.
```

Without `--dry-run`, each planned command runs in sequence.

## Use with pseudo-translate

Swap `--tool` for a UI-QA pass:

```bash
kapi sync -p translation.kapi --tool pseudo-translate
```

## CI

Drop into CI as a post-extract step:

```yaml
- run: kapi-react extract --out i18n/ui.klz
- run: kapi sync -p translation.kapi --dry-run
- run: kapi sync -p translation.kapi
- run: kapi-react compile i18n/ui.klz --out public/translations
```

The `--dry-run` line gives a readable plan in the CI log before the actual run.

## See also

- [`kapi status`](./status) — read-only coverage report.
- [`kapi run`](./flow) — run a specific named flow from the project.
- [AD-041: `.kapi` project files](/docs/ad/041-kapi-desktop)
