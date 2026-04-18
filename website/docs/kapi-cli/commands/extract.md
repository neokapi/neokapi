---
sidebar_position: 9
title: extract
---

# kapi extract

Populate every archive-declared collection in a [`.kapi` project](/docs/ad/041-kapi-desktop) by running the declared format's extractor.

## Synopsis

```bash
kapi extract -p <project.kapi> [--timeout <duration>]
```

## Description

For each [content collection](/docs/ad/041-kapi-desktop) that declares an `archive:` path, `kapi extract` looks at each item's `format:` declaration and runs the extractor. Today the only source-extraction format that generates blocks from arbitrary source is `exec`:

```yaml
content:
  - name: ui
    archive: i18n/ui.klz
    items:
      - path: "src/**/*.tsx"
        format:
          name: exec
          config:
            command: "vp kapi-react extract --stream"
```

The declared command is invoked once per collection with every matched file path on stdin (NUL-separated). It should emit NDJSON block records on stdout — see [AD-045](/docs/ad/045-klf-klz-spec) for the protocol. `kapi extract` packs the streamed blocks into the collection's `.klz`.

The developer picks the package manager in the `command` string (`vp`, `pnpm`, `npm`, `yarn`, or a direct binary path). kapi runs it verbatim.

## Flags

| Flag | Short | Default | Description |
|---|---|---|---|
| `--project` | `-p` | — | Path to the `.kapi` project file (required). |
| `--timeout` | | `5m` | Maximum runtime per extractor subprocess. |

## Environment

| Var | Purpose |
|---|---|
| `KAPI_EXEC_OVERRIDE` | Replace the project-declared command for this run. Useful in CI to inject a coverage wrapper or vendored binary without touching the `.kapi`. |

## Example

```bash
$ kapi extract -p translation.kapi
  ui → vp (186 file(s))
  i18n/ui.klz ← 1007 block(s) across 147 document(s)
```

## Alternative: standalone pipe (no `.kapi`)

For ad-hoc or single-collection projects, skip `.kapi` entirely:

```bash
vp kapi-react extract --stream | kapi pack --out i18n/ui.klz
```

See [`kapi pack`](./pack) for the pipe-in command.

## See also

- [`kapi pack`](./pack) — pack NDJSON or KLF directory into a `.klz`.
- [`kapi status`](./status) — read-only coverage report.
- [`kapi sync`](./sync) — translate the extracted archive.
- [AD-045: KLF/KLZ format](/docs/ad/045-klf-klz-spec) — the exec-format protocol contract.
