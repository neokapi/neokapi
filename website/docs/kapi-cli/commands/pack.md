---
sidebar_position: 9.5
title: pack
---

# kapi pack

Pack NDJSON block records (from stdin) or a directory of `.klf` files into a single `.klz` archive.

## Synopsis

```bash
kapi pack --out <archive.klz> [--in <klf-dir>]
```

## Description

`kapi pack` is the standalone counterpart to [`kapi extract`](./extract) — a way to build a `.klz` from blocks produced outside of a `.kapi` project. Two input modes:

1. **NDJSON stdin** — default. Pipe block records in; one archive out.
2. **`.klf` directory** — pass `--in <dir>` to read every `*.klf` under that directory.

Useful pipelines:

```bash
# Pipe kapi-react straight into a .klz, no .kapi needed.
vp kapi-react extract --stream | kapi pack --out i18n/ui.klz

# Extract first to a debuggable directory, pack separately.
vp kapi-react extract --out i18n/klf/
kapi pack --in i18n/klf/ --out i18n/ui.klz

# A custom extractor written in any language — as long as stdout is
# NDJSON block records, `kapi pack` eats it.
python scripts/my-extractor.py | kapi pack --out i18n/custom.klz
```

## Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--out` | — | — | Output archive path (required). |
| `--in` | — | — | Directory of `.klf` files to pack. Omit to read NDJSON from stdin. |

## NDJSON format

One JSON record per line. Blank lines and lines that don't start with `{` are ignored (progress log noise is fine):

```json
scanning src/**/*.tsx
{"type": "block", "document": "src/App.tsx", "block": { ... }}
{"type": "block", "document": "src/Button.tsx", "block": { ... }}
```

See [AD-045](/docs/ad/045-klf-klz-spec) for the `block` shape.

## See also

- [`kapi extract`](./extract) — project-driven extraction via `.kapi`.
- [AD-045: KLF/KLZ format](/docs/ad/045-klf-klz-spec)
