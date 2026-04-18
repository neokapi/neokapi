---
sidebar_position: 12
title: show
---

# kapi show

Look up a single block by its hash across every archive declared in a [`.kapi` project](/docs/ad/041-kapi-desktop) and pretty-print its source, per-locale targets, and properties.

## Synopsis

```bash
kapi show <hash> -p <project.kapi>
```

## Description

Useful when a hash surfaces in a translator complaint, QA log, or warning trace and you want to see what the block actually says — without cracking open the archive with zip tools.

## Flags

| Flag | Short | Description |
|---|---|---|
| `--project` | `-p` | Path to the `.kapi` project file (required). |

## Example

```bash
$ kapi show aB3xZ -p translation.kapi
aB3xZ (ui)
  archive:  i18n/ui.klz
  document: src/App.tsx
  element:  h1
  jsxPath:  div > h1
  source:   Welcome to Acme
  targets:
    de:     Willkommen bei Acme
    fr:     Bienvenue chez Acme
    ja:     Acme へようこそ
```

## Exit codes

- `0` — block found and printed.
- `1` — hash not found in any declared archive.

## See also

- [`kapi status`](./status) — per-project coverage overview.
- [`kapi sync`](./sync) — run translations for missing locales.
