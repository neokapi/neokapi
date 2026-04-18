---
sidebar_position: 9
title: extract
---

# kapi extract

Populate every archive-declared collection in a [`.kapi` project](/docs/ad/041-kapi-desktop) by dispatching matched files to the right extractor.

## Synopsis

```bash
kapi extract -p <project.kapi> [--timeout <duration>]
```

## Description

For each [content collection](/docs/ad/041-kapi-desktop) that declares an `archive:` field, `kapi extract` walks the collection's patterns, groups files by extension, and dispatches each group to an extractor. Resulting blocks are packed into the collection's `.klz`.

Dispatch resolves in three stages:

1. The collection's explicit `extractor: { exec: [...] }` wins.
2. Otherwise, the file's extension is matched against every `kapi-plugin.json` descriptor discovered in the project's `node_modules` tree.
3. Otherwise, the extension has no registered extractor — the command exits with an error listing the uncovered types.

Extractors implement a lightweight contract: NUL-separated paths on stdin, NDJSON block records on stdout.

## Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--project` | `-p` | — | Path to the `.kapi` project file (required). |
| `--timeout` | | `5m` | Maximum runtime per extractor subprocess. |

## Example

```bash
$ kapi extract -p translation.kapi
  ui → @neokapi/kapi-react (186 file(s))
  i18n/ui.klz ← 1007 block(s) across 147 document(s)
  marketing → markdown (23 file(s))
  i18n/marketing.klz ← 412 block(s) across 23 document(s)
```

## Auto-discovery of extractors

A package declares itself as an extractor plugin by adding a `kapi-plugin` field to its `package.json`:

```json title="packages/kapi-react/package.json"
{
  "name": "@neokapi/kapi-react",
  "kapi-plugin": {
    "extensions": [".tsx", ".jsx"],
    "extract": {
      "exec": ["npx", "--no-install", "kapi-react", "extract", "--blocks-stream"],
      "stdin": "paths-nul-separated"
    }
  }
}
```

Any project that has the package in `node_modules` gets the extractor dispatched automatically — no `kapi plugins install` step.

## Pinning dispatch in `.kapi`

When auto-discovery picks the wrong plugin, or when you want the dispatch documented in the project file, use `extractor:` on the collection:

```yaml title="translation.kapi"
content:
  - name: ui
    archive: i18n/ui.klz
    extractor:
      exec: ["vp", "run", "kapi-react", "extract", "--blocks-stream"]
    items:
      - path: "src/**/*.{tsx,jsx}"
```

## See also

- [`kapi status`](./status) — coverage report after extract.
- [`kapi sync`](./sync) — translate the extracted archive.
- [AD-045: KLF/KLZ format](/docs/ad/045-klf-klz-spec) — the extractor protocol contract.
