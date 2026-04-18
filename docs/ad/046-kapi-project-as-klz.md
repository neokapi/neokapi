---
id: 046-kapi-project-as-klz
sidebar_position: 46
title: "AD-046: Kapi Project as KLZ + BlockStore Providers"
---

# AD-046: Kapi Project as KLZ + BlockStore Providers

- **Scope:** neokapi framework (Go), `.kapi` project model, `core/klz` cache, flow executor, tool interface. Bowrain integration appears as one provider but its server-side storage is out of scope (it implements the interface however it wants).
- **Affects:** `core/project/`, `core/klz/`, `core/klf/`, `core/flow/`, `core/tool/`, `cli/extract.go`, `packages/kapi-format/`, `apps/kapi-desktop/`, and the bowrain push/pull path.
- **Related:** [AD-041](./041-kapi-desktop.md) (desktop + `.kapi` file), [AD-042](./042-project-context.md) (ProjectContext resolution), [AD-044](./044-klf-klz-integration.md), [AD-045](./045-klf-klz-spec.md) (KLF/KLZ spec).

## Summary

Collapse three currently-separate concepts — the `.kapi` project, the `.klz` archive, and the in-flight block stream — into a single coherent model, while keeping the user-facing click-to-open experience intact:

- A kapi project has two physical parts side-by-side: a **`{project}.kapi` file** at the project root (the recipe — what users edit, what they double-click to open in Kapi Desktop, what they commit to git as the project identity), and a **`.kapi/` folder** adjacent to it (kapi's working state — manifest bookkeeping, block store, sidecars).
- The block state is accessed through a `BlockStore` interface with pluggable providers: `memory`, `klzdb` (SQLite in `.kapi/cache.db`), `zip` (read-only `.klz`), and `bowrain` (REST against a bowrain-server). Flows and tools operate against the interface, not directly against channels.
- Sources (authored content) live outside `.kapi/`. Translated outputs (generated) live outside too, at paths the manifest declares. `.kapi/` contains kapi's working state only; it never carries files users author or runtime consumers load.
- A `.klz` snapshot is a zip of the `{project}.kapi` file **plus** the adjacent `.kapi/` folder, not a byte-for-byte mirror of one or the other. The relationship between project and KLZ is logical (same content, content-addressed) rather than structural (same zip root layout).

The streaming model becomes the `memory` provider. KLZ becomes one provider *and* the portable snapshot format. `{project}.kapi` gains a single `store:` declaration per collection (default `klzdb`).

## Context

Today three concepts sit in overlapping layers without a clean factoring:

1. **`.kapi` project file** — declarative YAML, lists collections + flows + formats. Runs via `kapi extract -p project.kapi`. No notion of where block state lives; implicitly in-memory for the duration of the run.
2. **`.klz` archive** — ZIP with a manifest + KLF files + sidecars. Optional output produced by `kapi pack` or an `archive:` field on a collection. After a flow finishes, block state is lost unless a `.klz` was written.
3. **Streaming pipeline** — tools exchange `*Part` over channels. Annotations accumulate on the Part struct during the flow. Random access by hash is impossible; re-runs repeat work.

This leaves `.kapi` projects "missing out on" the capabilities KLZ + `klzdb` could provide: random access by block hash, append-only sidecars, incremental re-runs, concurrent tools writing different annotations, and partial resume after failure. The bowrain push/pull path is a fourth, parallel universe that has to marshal kapi's internal state onto an ad-hoc REST API because there is no common block-store contract.

The root cause is that persistence is treated as an end-of-pipeline opt-in ("pack this at the end"), not as the substrate flows run against.

## Decision

### 1. A `BlockStore` interface

Flows and tools read and write blocks + sidecars through `BlockStore`, not through raw channels. The streaming contract is preserved as one capability among several.

```go
// Package core/blockstore
type BlockStore interface {
    Begin(ctx context.Context) (Session, error)
    Close() error
    Capabilities() Capabilities
}

type Session interface {
    // Streaming read (every provider supports this)
    Blocks(filter ...BlockFilter) iter.Seq2[*Block, error]

    // Random access (optional — Capabilities.RandomAccess)
    GetBlock(hash string) (*Block, error)

    // Block writes
    PutBlock(b *Block) error

    // Sidecars — append-only layers keyed by block hash
    GetSidecar(kind, hash string) (Sidecar, error)
    PutSidecar(kind, hash string, s Sidecar) error
    ListSidecars(kind string) iter.Seq2[Sidecar, error]

    // Commit / rollback semantics
    Commit() error
    Rollback() error
}

type Capabilities struct {
    RandomAccess bool   // GetBlock / ListSidecars by hash are O(log n) or better
    Concurrent   bool   // multiple sessions can write different sidecars in parallel
    Remote       bool   // provider is network-backed; prefer batched ops
}
```

Tools that only need forward-only streaming iterate `session.Blocks()`. Tools that need random access probe `Capabilities.RandomAccess` and use `GetBlock` when available. Tools that can't work without random access declare it in their manifest and the executor refuses to schedule them against a `memory` store.

### 2. Providers

Four providers land in `core/blockstore/`:

| Provider | Backing | Cap: RandomAccess | Cap: Concurrent | Cap: Remote | Use case |
|---|---|---|---|---|---|
| `memory` | Go maps | yes | no (single goroutine) | no | ephemeral flows, tests, ad-hoc CLI invocations |
| `klzdb` | SQLite file `.kapi/cache.db` | yes | yes (SQLite WAL) | no | default for kapi projects, long-lived local work |
| `zip` | `.klz` file on disk | yes (read) / no (append-only) | no | no | read-only snapshot consumption; write path produces a new zip at `Commit()` |
| `bowrain` | REST against bowrain-server | yes (indexed) | yes (server ACID) | yes | multi-user / cloud projects |

A fifth provider, `format-reader`, wraps any `format.DataFormatReader` as a read-only store. Useful for ad-hoc commands (`kapi ai-translate -i file.xliff`) — the reader becomes a store, the flow runs, a writer emits the result. Equivalent to today's streaming behaviour but expressed uniformly.

### 3. Project layout

A kapi project has three ownership zones with a clear boundary:

```
my-app/
├── my-app.kapi              ← RECIPE (user edits, click-to-open handle)
├── .kapi/                   ← WORKING STATE (kapi maintains)
│   ├── manifest.yaml        ← block bookkeeping — counts, checksums, times
│   ├── cache.db             ← klzdb provider (default store)
│   ├── collections/         ← sidecars per collection
│   │   └── ui/
│   │       ├── targets/
│   │       │   ├── fr.json
│   │       │   └── de.json
│   │       ├── annotations/
│   │       │   ├── terms.json
│   │       │   ├── tm-matches.json
│   │       │   └── qa.json
│   │       └── skeletons/
│   └── cache/               ← disposable; gitignore'd
├── src/                     ← user-authored sources
│   └── **/*.tsx
├── i18n/                    ← generated translations
│   ├── de.json              ← format writer output; runtime consumes these
│   └── fr.json
└── package.json
```

Ownership:

- **`{project}.kapi`** — the user's. Hand-edited YAML. The click-to-open handle for Kapi Desktop. Committed to git. See §4.
- **`.kapi/`** — kapi's. Managed by kapi tools. `cache.db` and `collections/**` hold the block state and sidecars; `manifest.yaml` records bookkeeping derived from that state. Safe to gitignore if blocks are re-extractable; opt in to commit when you want reproducibility.
- **`src/**`** — the user's authored content. Referenced by the recipe; never moved into `.kapi/`.
- **Writer outputs** — e.g. `i18n/{locale}.json` — produced by format writers the recipe declares. Runtime (kapi-react, the app) consumes these; kapi doesn't.

The name pair (`{project}.kapi` file + `.kapi/` folder) mirrors git's pattern: `.gitignore` file + `.git/` folder at the same root. Different slot, shared prefix.

**`.klz` snapshot shape.** A `.klz` is a zip of `{project}.kapi` + `.kapi/` together. The zip root has a single `{project}.kapi` file and a `.kapi/` directory — unzipping into an empty folder reproduces a working project. The zip is a snapshot container, not a semantic mirror of any one of them; the relationship between project and KLZ is logical (same content, same content-addressing) rather than a byte-for-byte folder-is-the-zip equivalence. That frees us to evolve either side independently.

**File association.** Kapi Desktop registers `.kapi` as a file extension. Double-clicking `my-app.kapi` launches the app with that path; the app reads the recipe, looks for a sibling `.kapi/` folder, and either loads state from it or treats the project as fresh (creating `.kapi/` on first persist). Opening a `.klz` unzips to a chosen location and opens the contained `.kapi` file the same way.

### 4. Two manifests, one source of truth each

The recipe and the block bookkeeping are kept in separate files so each has a single clear author:

**`{project}.kapi`** — human-authored recipe. The project identity, collections, flows, store choices. This is the file users edit and commit.

```yaml
# my-app.kapi
schemaVersion: 1
kind: kapi-project

id: my-app
sourceLocale: en
targetLocales: [de, fr, qps]

collections:
  - name: ui
    store:
      type: klzdb                 # default; omittable
      path: .kapi/cache.db        # default; omittable
    items:
      - src: src/**/*.{tsx,jsx}
        format:
          name: exec
          config:
            command: vp kapi-react extract --stream
    writers:
      - format: json
        out: i18n/{locale}.json

flows:
  prepare-handoff:
    steps:
      - extract
      - termbase-match: { termbase: terms.db }
      - tm-match: { tm: tm.db, threshold: 75 }
```

**`.kapi/manifest.yaml`** — kapi-maintained bookkeeping. Block counts, content hashes, generator identity, last-extract timestamps. Derived from the store. Users never edit it by hand; kapi recomputes it whenever the store changes.

```yaml
# .kapi/manifest.yaml
schemaVersion: 1
kind: kapi-state
generator: { id: kapi, version: 0.5.0 }
project: { id: my-app, path: ../my-app.kapi }

blocks:
  ui:
    count: 1007
    sha256: 3f8a…                 # hash of the sorted block index
    sources:                      # per-source hash for staleness detection
      - path: src/components/App.tsx
        sha256: 7e12…
        blocks: 14

updatedAt: 2026-04-18T15:00:00Z
```

The `.kapi/manifest.yaml` is content-addressed bookkeeping: every entry keyed by block hash or source file hash, safe to regenerate from the `cache.db` if deleted or corrupted. `kapi manifest recompute` rebuilds it.

**Tool discovery.** Walk up from the current directory looking for either:

1. A single `*.kapi` file at a given directory level (that's the recipe; the adjacent `.kapi/` folder is the state), or
2. A `.kapi/` folder (from which the recipe path is read via the `project.path` entry in `.kapi/manifest.yaml`).

In practice users always land on (1) because the recipe file is the project identity. Case (2) is a fallback when tools are invoked from inside `.kapi/` itself (e.g. an automation running against the state folder).

If a directory contains multiple `*.kapi` files (monorepo with several projects side-by-side), kapi requires `-p <path>` to disambiguate. The `.kapi/` folder is one-per-recipe; two recipes in the same directory means two adjacent `.kapi/` folders, which we disallow (use subdirectories instead).

### 5. Flow executor operates against `Session`

The executor opens a `Session` against the collection's declared store at flow start, passes it to each tool, commits at flow end:

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

The `tool.Tool` interface gains a new method (or an adapter wraps the old streaming interface):

```go
type Tool interface {
    Manifest() ToolManifest           // declares required capabilities
    Process(ctx context.Context, s Session) error
}
```

Tools that still want streaming iterate `s.Blocks()`. Tools that need random access use `s.GetBlock()` and `s.GetSidecar()`. Tools that need to know what's already done consult `s.ListSidecars("targets")` before translating.

### 6. Snapshot + handoff verbs

Three verbs, clearly separated:

- **`kapi snapshot`** — zip the recipe (`{project}.kapi`) plus `.kapi/` into a `.klz`. Portable full-state export. Default excludes `.kapi/cache/**` (rebuildable). Optional `--include-sources` copies the project's declared source globs (`src/**`) into the zip for fully-self-contained handoff.
- **`kapi open <file.klz>`** — inverse of snapshot. Unzip into a chosen directory; the recipe file lands at the new directory's root and `.kapi/` lands beside it.
- **`kapi run <flow>`** — run a flow; writer outputs land wherever the recipe's `writers:` declare (typically outside `.kapi/`).

CAT-tool handoff is unchanged: a writer step in the flow emits `.xliff`/`.po`/etc. at a declared path. KLZ is not used for this — it's for kapi-to-kapi transport only.

**Zip layout of a `.klz`:**

```
my-app.klz:
├── my-app.kapi              ← recipe
└── .kapi/
    ├── manifest.yaml
    ├── cache.db
    └── collections/**
```

`my-app.kapi` lives at the zip root so the opening logic is trivial: list zip members, find the single `*.kapi` entry, adjacent `.kapi/` is the state folder. No index file or special header needed in the zip.

### 7. Bowrain push/pull through `BowrainStore`

`bowrain push` becomes content-addressed copy from a local `BlockStore` to a `BowrainStore`:

1. Hash every block locally (already content-addressed).
2. Ask bowrain-server which hashes it's missing.
3. Push only the missing blocks + any sidecars not on the server.

`bowrain pull` is the reverse. A `.bowrain/` directory replaces `.kapi/cache.db` with a `BowrainStore` descriptor, but the manifest.yaml and sidecar layout are identical. The kapi CLI and the bowrain CLI run the same flow code against the same interface; the only difference is which provider backs the session.

## Consequences

### What becomes possible

- **Incremental work.** Translate only blocks whose source hash isn't already in the target sidecar. Re-run QA without re-translating. Resume after failure.
- **Concurrent flows.** Terminology matching and TM lookup can run simultaneously — they write different sidecars.
- **Multi-pass tools.** Compute vocabulary statistics across the whole store, then use them in a second pass to prime AI prompts per block.
- **Partial dispatches.** Translate a subset: `kapi run translate --filter locale=fr --filter collection=ui`. The filter pushes into `session.Blocks()`.
- **Bowrain collaboration without a second model.** Web UI edits and CLI flows write the same sidecar shapes.
- **Kapi Desktop project lifecycle.** Open a `.klz` → get a working project. Save → snapshot back to `.klz`. Same mental model as opening a document in any editor.

### What it costs

- **Interface refactor.** Every tool's `Process` signature changes. The streaming helpers stay; they wrap `session.Blocks()`. Estimated scope: `core/tool/`, `core/flow/`, plus all built-in tools in `core/tools/`, `providers/ai/`, `providers/mt/`. Manageable but non-trivial.
- **Capability negotiation.** The executor has to match a tool's declared capabilities against the session's provider. Not hard, but it's new validation logic.
- **Transaction semantics.** `Commit`/`Rollback` are easy for `klzdb` (SQLite txn) and `bowrain` (server ACID). They're a no-op for `memory`. `zip` builds the new zip in-memory and writes it atomically on `Commit`.
- **Remote latency.** `BowrainStore` sessions need batched reads / writes. Tools that naïvely call `GetBlock` per-block will be slow over the network. Mitigation: a `session.Batch(hashes)` helper; encourage streaming iteration where possible; document the pattern in the tool-authoring guide.
- **Spec migration.** AD-045's KLZ spec gets revised: no standalone `manifest.json`; `manifest.yaml` with both `project:` and `manifest:` sections; `.kapi/`-folder-is-the-zip-root rule. Since there are no production KLZ files, this is a rewrite, not a migration path.

### What's explicitly not in scope

- **bowrain-server storage internals.** The server implements `BlockStore` however it wants (Postgres tables, SQLite, content-addressed blob store). The contract is the interface.
- **Multi-user conflict resolution.** Two humans editing the same block via bowrain-server is a bowrain concern, not a framework concern. The interface says "last write wins at the sidecar level" and higher-level semantics are outside.
- **Versioning / history.** KLZ stays a snapshot; `klzdb` stays current-state. A future AD can add a `History` capability to providers that support it (probably `bowrain` only, for audit and collaborative diffs).
- **Source files in the zip.** Default is sources live outside `.kapi/` and outside the snapshot. `--include-sources` is a convenience, not the default.

## Alternatives considered

### A. Leave streaming as the primary model; promote `pack` steps

Keep today's architecture. Encourage users to insert a `pack` step wherever they want persistence, and to read back from the archive for the next phase. This is what exists now. Rejected: it keeps three overlapping concepts in play without uniting them, and leaves bowrain integration a separate universe.

### B. KLZ-only (no klzdb, no memory, no bowrain providers)

Every flow run reads/writes a `.klz` on disk. Simple and consistent, but slow (zip random-writes are painful) and forces every project — including CI's one-shot `ai-translate` — to materialize an archive. Rejected: zip is the wrong working format; klzdb / bowrain are needed.

### C. Keep `.kapi` and `.klz` separate; add BlockStore without the folder unification

Land `BlockStore` + providers, but leave `.kapi` projects at the root file and `.klz` as a standalone archive. Rejected on symmetry grounds: it leaves the project-as-container idea unimplemented, and "zip the project to hand it off" becomes an ad-hoc operation rather than a first-class verb. The folder + file pair is cheap (it's mostly a spec decision) and removes an entire layer of mental model.

### D. Single manifest — fold the recipe into `.kapi/manifest.yaml`

Earlier draft of this AD put everything (recipe + bookkeeping) in `.kapi/manifest.yaml` with no top-level file, git-style. Rejected once the desktop file-association UX was factored in: users clicking a file to open a project is a strong, concrete need. Kapi Desktop needs a specific file to register for the `.kapi` extension; `.kapi/manifest.yaml` buried in a hidden folder can't serve that. Splitting into `{project}.kapi` (recipe) + `.kapi/manifest.yaml` (bookkeeping) costs one file but gives:

- A click-to-open handle that works like `.code-workspace`, `.sln`, `.xcworkspace`.
- A clean author boundary — recipe is user-owned, bookkeeping is kapi-owned, neither overwrites the other.
- A more natural zip shape (recipe file at zip root, state folder beside it) than nesting everything in `.kapi/`.

### E. Sources inside `.kapi/`

A fully self-contained `.kapi/` including a copy of `src/**`. Rejected: sources are authored artifacts that belong next to the rest of the codebase (git, IDE, build tools). Putting them inside `.kapi/` either duplicates them or implies `.kapi/` is the project root — both worse than keeping them separate.

### F. Byte-for-byte "zip the `.kapi/` folder = KLZ"

An earlier framing tried to make the project folder and the KLZ zip be byte-identical trees (`.kapi/` folder contents at zip root). Rejected once the top-level recipe file was kept: the `.klz` zip now needs the recipe at its root and `.kapi/` alongside, which is not a literal mirror of either physical form. That's fine — the relationship is **logical** (same content, same content-addressing, round-trip lossless) rather than **structural**. It lets recipe and state evolve independently without spec breakage.

## Rollout

Backward compat is not a priority (no production consumers). The rollout is a rewrite across a few PRs:

1. **`BlockStore` interface + `memory` + `klzdb` providers.** Port the executor and one or two tools (e.g. `extract`, `pseudo-translate`) to operate against `Session`. Validate the interface.
2. **Rewrite `.kapi` loader + KLZ reader/writer.** New manifest schema, `.kapi/` folder layout, `snapshot` / `open` verbs. Delete old standalone `project.kapi` handling and old `klz/manifest.json` pathway.
3. **Port remaining built-in tools** and the Kapi Desktop + kapi-react side to the new interface.
4. **`BowrainStore`.** Reshape `bowrain push`/`bowrain pull` to be `BlockStore`-to-`BlockStore` copies. Bowrain-server gains the block+sidecar REST surface; its internal storage is untouched.
5. **`format-reader` adapter + ad-hoc CLI flows** (`kapi ai-translate -i file.xliff`) through the same pipeline.
6. **Docs + examples.** Update AD-041, AD-042, AD-044, AD-045 to reference this decision; rewrite the "extract / translate / compile" walkthrough around the new project layout; provide example `.kapi/manifest.yaml` files for each provider.
