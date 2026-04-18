---
sidebar_position: 10
title: status
---

# kapi status

Show translation state for a [`.kapi` project](/docs/ad/041-kapi-desktop).

## Synopsis

```bash
kapi status -p <project.kapi>
```

## Description

For each content collection that declares an `archive:` field, `kapi status` opens the declared `.klz` and reports:

- total source-block count
- per-locale coverage (translated blocks / total)
- whether the archive is present on disk
- whether each declared target language has any translations

Collections without `archive:` are listed with a reminder that they use file-based flows and have no `.klz` state to inspect. The command is stateless — coverage is re-derived from the archive on every invocation, and the archive itself is the state store.

## Flags

| Flag | Short | Description |
|---|---|---|
| `--project` | `-p` | Path to the `.kapi` project file (required). |

## Example

```bash
$ kapi status -p translation.kapi
translation.kapi (My App Localization)

  ui → i18n/ui.klz
    1007 blocks
    de:      not translated
    fr:      987/1007 translated
    ja:      1007/1007 translated (complete)

  legacy
    (no archive — file-based flow)
```

## See also

- [`kapi sync`](./sync) — run translations for every (archive, missing-locale) pair in one command.
- [AD-041: `.kapi` project files](/docs/ad/041-kapi-desktop)
- [AD-045: KLF/KLZ format specification](/docs/ad/045-klf-klz-spec)
