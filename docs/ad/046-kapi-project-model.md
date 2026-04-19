---
id: 046-kapi-project-model
sidebar_position: 46
title: "AD-046: Kapi Project Model"
---

# AD-046: Kapi Project Model

- **Scope:** neokapi framework (Go), `.kapi` project folder, block store interface and providers, flow executor, tool interface. Bowrain integration appears as one provider.
- **Affects:** `core/project/`, `core/blockstore/`, `core/klf/`, `core/flow/`, `core/tool/`, `cli/`, `apps/kapi-desktop/`, bowrain sync path.
- **Related:** [AD-041](./041-kapi-desktop.md) (desktop + `.kapi` file), [AD-042](./042-project-context.md) (ProjectContext resolution).

## Summary

A kapi project is a folder with a **`{project}.kapi` recipe** at its
root and an adjacent **`.kapi/` state directory**. The recipe is the
user's authored YAML — project identity, collections, flows, store
declaration — and the file Kapi Desktop opens on double-click.
`.kapi/` holds kapi's working state: the block-store database, per-
collection sidecars (translation targets, term matches, QA findings,
skeletons), and a small bookkeeping manifest.

Sources stay outside `.kapi/` (under `src/`, `content/`, wherever the
user authored them). Generated translations land wherever the recipe
declares their writer output path (typically `i18n/` or
`public/locales/`). `.kapi/` contains kapi's working state only; it
never carries files users author or files runtime consumers load.

Flows and tools access block state through a `BlockStore` interface
with pluggable providers. Sharing a project works like sharing any
folder — git, tar, cp. There is no separate archive format.

## Context

Translation workflows need to persist more than the current in-flight
stream:

- Translators add targets over time, per locale.
- QA adds annotations. Term matching adds annotations. TM lookups add
  suggestions.
- Re-running a flow shouldn't re-translate blocks whose source hash
  hasn't changed.
- Multiple tools (term match + TM) should be able to run in parallel,
  each writing its own annotation layer.

The channel-based stream model (`Part → Tool → Part`) is a forward-
only transform — great for one-shot "read → translate → write" but
incapable of the "random access + append layers + incremental work"
story above.

A local SQLite block store inside the project folder gives the
substrate. The project folder itself is the unit users share, back
up, or commit. No additional archive format is needed.

## Decision

### 1. Project layout

Three ownership zones at the project root:

```
my-app/
├── my-app.kapi              ← RECIPE (user edits, click-to-open)
├── .kapi/                   ← WORKING STATE (kapi maintains)
│   ├── manifest.yaml        ← bookkeeping (block counts, fingerprints, timestamps)
│   ├── cache.db             ← block store (SQLite)
│   └── collections/         ← sidecar layers per collection
│       └── ui/
│           ├── targets/{fr,de}.json
│           ├── annotations/{terms,tm-matches,qa}.json
│           └── skeletons/
├── src/                     ← authored sources
│   └── **/*.tsx
└── i18n/                    ← generated translations (format writer output)
    └── {de,fr}.json
```

Ownership:

- **`{project}.kapi`** — the user's. Hand-edited YAML. The click-to-
  open handle for Kapi Desktop. Committed to git.
- **`.kapi/`** — kapi's. Managed by kapi tools. `cache.db` +
  `collections/**` hold the block state and sidecar layers;
  `manifest.yaml` is bookkeeping derived from the store. Safe to
  gitignore if blocks are re-extractable; opt in to commit when you
  want reproducibility across clones.
- **`src/**`** — the user's authored content. Referenced by the
  recipe; never moved into `.kapi/`.
- **Writer outputs** — e.g. `i18n/{locale}.json` — produced by
  format writers the recipe declares. Runtime consumes these; kapi
  doesn't.

The name pair mirrors git: `.gitignore` file + `.git/` folder at the
same root.

### 2. Recipe file

`{project}.kapi` is a YAML document with project identity,
collections, flows, and (optionally) a store declaration per
collection. Example:

```yaml
# my-app.kapi
version: v1
id: my-app
name: My App Localization
sourceLocale: en
targetLocales: [fr, de, qps]

content:
  - name: ui
    store:
      type: cache
      path: .kapi/cache.db
    items:
      - path: "src/**/*.{tsx,jsx}"
        format:
          name: exec
          config:
            command: "vp kapi-react extract --stream"
    writers:
      - format: json
        out: "i18n/{locale}.json"

flows:
  translate:
    steps:
      - ai-translate
      - qa
```

Discovery is git-style: kapi tools walk up from the current directory
until they find a `*.kapi` file. Multiple recipes at the same
directory level require an explicit `-p <path>`.

### 3. State manifest

`.kapi/manifest.yaml` is kapi's bookkeeping — block counts, per-
source SHA-256 fingerprints for staleness detection, generator
identity, last-updated timestamps. Users don't hand-edit it. It is
safe to delete and rebuild from `cache.db`; nothing authoritative
lives here that isn't also in the store.

### 4. `BlockStore` interface

Flows and tools read and write blocks + sidecars through
`BlockStore`, not through raw channels. The streaming contract is
preserved as one capability among several.

```go
// Package core/blockstore
type BlockStore interface {
    Begin(ctx context.Context) (Session, error)
    Capabilities() Capabilities
    Close() error
}

type Session interface {
    Blocks(filter BlockFilter) iter.Seq2[*Block, error]
    GetBlock(hash string) (*Block, error)
    PutBlock(collection string, b *Block) error
    GetSidecar(kind, blockHash string) (Sidecar, error)
    PutSidecar(s Sidecar) error
    ListSidecars(kind string) iter.Seq2[Sidecar, error]
    Commit() error
    Rollback() error
    Close() error
}

type Capabilities struct {
    RandomAccess bool
    Concurrent   bool
    Remote       bool
    Writable     bool
}
```

### 5. Providers

| Provider | Backing | Use case |
|---|---|---|
| `memory` | Go maps | ephemeral flows, tests, ad-hoc CLI invocations |
| `cache` | SQLite at `.kapi/cache.db` | default for kapi projects, long-lived local work |
| `bowrain` | REST against bowrain-server | multi-user / cloud projects |

Tools never open `cache.db` directly — they read from the session.

### 6. Flow executor operates against `Session`

```go
session, err := store.Begin(ctx)
if err != nil { return err }
defer session.Close()

for _, tool := range flow.Tools {
    if err := tool.Process(ctx, session); err != nil {
        return session.Rollback()
    }
}
return session.Commit()
```

The existing channel-based `Tool` interface stays. The new
`SessionTool` extension (see `core/tool/session.go`) is for tools
that want random access.

### 7. Sharing projects

A project is a folder. Sharing means sharing the folder — git,
tarball, rsync. Kapi doesn't prescribe a bundling format. For
collaborative / server-backed work, swap the `cache` provider for
`bowrain` without changing the recipe or the flow code.

### 8. CLI

- **`kapi init`** — scaffold a new project (`{name}.kapi` +
  `.kapi/manifest.yaml`). Refuses to overwrite existing files.
- **`kapi run <flow> -p project.kapi`** — run a declared flow.
- **Tool commands** (`kapi ai-translate`, `kapi pseudo-translate`,
  …) — run against the project's store when `-p` is passed, or
  against file inputs directly.

## Consequences

### What becomes possible

- Incremental work: translate only blocks whose source hash isn't
  already in `targets/<locale>`.
- Concurrent flows: term match + TM lookup run in parallel, each
  writing its own sidecar.
- Multi-pass tools: compute stats across the whole store, then use
  them in a second pass.
- Bowrain collaboration without a second model — same interface,
  different provider.

### What it costs

- Transaction semantics per provider (SQLite txn for `cache`, server
  ACID for `bowrain`, no-op for `memory`).
- Remote latency for `BowrainStore`: needs batched reads/writes;
  tools calling `GetBlock` per-block will be slow.

### Out of scope

- bowrain-server storage internals.
- Multi-user conflict resolution (bowrain-level concern).

## Alternatives considered

### A. Leave streaming as the primary model

Keep the channel-based executor; add persistence via ad-hoc per-tool
caches. Rejected: each tool reinvents persistence, cross-tool state
has no coherent home.

### B. Ship a KLZ archive format

An earlier design proposed a `.klz` zip containing the project's
state in a canonical layout. Rejected: a project folder is already
its own unit; users can tar or zip it with standard tools. Adding
an archive format means two layouts to reason about and transforms
in both directions for no real gain.

### C. Sources inside `.kapi/`

A fully self-contained `.kapi/` including `src/**`. Rejected:
sources belong next to the rest of the codebase (git, IDE, build
tools). Duplicating them into `.kapi/` makes the project harder to
reason about.
