---
id: 039-local-context-graph-store
sidebar_position: 39
title: "AD-039: The Local Context Graph Store"
description: "Architecture decision: a kapi project keeps one local database, .kapi/work/store.db — the local projection of the project's committed context and the substrate of its context graph. Every subsystem's tables share the file, a property graph relates them, and the same queries answer locally and on the server."
keywords: [store.db, context graph, graph_nodes, graph_edges, project store, local projection, coordinates, durable identity, neokapi, architecture decision]
---

# AD-039: The Local Context Graph Store

## Summary

A kapi project keeps **one** local database: `.kapi/work/store.db`. It is the
local **projection** of the project's committed context — never its truth — and
the substrate of the project's **context graph**.

Every subsystem's tables share that one file: the block cache, the terms store,
the content memory, the unit-state working set, and a property graph
(`graph_nodes` / `graph_edges`) relating them. One SQLite file, one connection,
one migration ledger per subsystem.

The point of the single file is the join. "Which blocks use this term, in which
collection, at which coordinate, and which of them are signed off?" is one query
here and is unanswerable across separate database files. It is the same question
a connected project asks bowrain, asked in the same shape with no server present.

## Context

Context is relational. A term occurs in blocks; blocks belong to collections and
sit at a [point in the context space](022-voice-profile.md#context-is-a-two-axis-space); a
a state record blesses a unit at a content hash; a memory entry recycles into a block.
Retrieval ([AD-037](037-context-retrieval-surface.md)) and governance
([AD-022](022-voice-profile.md)) both traverse those relations rather than reading
one store in isolation.

Bowrain answers such questions over a single Postgres spanning workspaces,
projects and streams. If a local project could not answer them at all, the two
halves of the product would differ in what they can be *asked*, not merely in
scale — and the framework's standing constraint is that kapi runs without the
platform ([AD-033](033-project-state-model.md)).

## Decision

### One file, many subsystems

`.kapi/work/store.db` holds:

| Tables | Subsystem | Derived from |
| --- | --- | --- |
| block cache | `core/blockstore` ([AD-008](008-project-model.md)) | the content files |
| terms | `terms/` ([AD-010](010-terminology.md)) | the committed `.terms.json` |
| content memory | `memory/` ([AD-009](009-content-memory.md)) | the committed target files plus an optional `.memory.json` seed |
| unit-state working set | `core/state` ([AD-033](033-project-state-model.md)) | the committed `.kapi/state/*.jsonl` shards |
| `graph_nodes`, `graph_edges` | `host/storage/graph` | the four above |

Each subsystem owns its own schema and its own migration ledger
(`storage.Migrate(db, "<subsystem>", …)`), so a subsystem evolves without
replaying anyone else's migrations. Sharing the file is a storage decision, not a
coupling: a subsystem still reaches its tables only through its own interface.

### The committed sources are the truth

`store.db` is an **index**, and every row in it is reconstructible:

- `.kapi/terms.json` — the terms source, bound by `defaults.terms_source`.
- `.kapi/memory/memory.json` — a content-memory seed, bound by
  `defaults.memory_source`.
- `.kapi/state/*.jsonl` — the committed unit-state record, one shard
  per document.
- the content files themselves — source and target.

Delete `store.db` and a re-run rebuilds it from those. Nothing authoritative
lives only in the database, which is why it sits under `.kapi/work/` — the one
gitignored path ([AD-008](008-project-model.md)) — while the sources it is built
from are committed one directory over.

### It sits at the top of `work/`, not under `cache/`

`.kapi/work/cache/` means *free to delete*: parse cache, extraction batches,
collection overlays, sync cache. `store.db` does not qualify, and by exactly one
margin. Between a review landing and `kapi commit` materializing it to
`.kapi/state/`, the working set inside `store.db` holds the **only**
copy of that staged state.

So the cost of losing the file is bounded and stated precisely: at most the
unit state staged since the last commit. Everything else rebuilds. `rm -rf
.kapi/work/cache` remains completely free, and keeping the two apart is what lets
that sentence stay true.

Deleting `.kapi/work` outright is a wider claim, because the redaction vault
([AD-020](020-redaction.md)) lives beside the database at `.kapi/work/vault/` and
holds the withheld originals. Those are local-only by design — never committed,
never synced — so nothing else has a copy, and losing them is not a rebuild but
a loss. `work/` groups the derived and the deliberately-local under one ignore
rule; only `cache/` inside it carries the unconditional promise.

### Presence is table-level

An empty subsystem inside an existing `store.db` behaves exactly as an absent
store does. The database's existence is not a signal, so nothing has to guard
against the file being there:

- the terminology gate reads the committed `.terms.json` directly on a fresh
  checkout, whether or not a database exists ([AD-010](010-terminology.md));
- `kapi up --plan` never creates state;
- `kapi pack` carries only non-empty parts ([AD-025](025-kbf-package.md)).

### The graph relates the subsystems

`graph_nodes` and `graph_edges` are a property graph — labelled nodes and
labelled, optionally time-bounded edges, both carrying JSON properties. What they
carry:

- **term ↔ block occurrence** — where a concept is actually used, filterable by
  collection and coordinate;
- **state ↔ unit** — which record blesses which unit, at which target hash;
- **coordinates** — the recipe's context axes as nodes, so governance resolution
  and retrieval traverse the same structure rather than re-deriving it.

Edges key on **durable identity** — the content key of a block
([AD-003](003-identity.md)), the unit key of a state record — not on a reader's
positional id, so a re-parse that renumbers a document does not orphan the graph.

**The tables exist and are attached; nothing writes them yet.** `App.ProjectGraph`
binds the property graph to the project store's own pool, so the `graph` ledger
migrates beside the other four. The relationships themselves are computed at
query time and handed back — `occurrence.Occurrence.Edge` is the term↔block edge,
fully formed — but not persisted. Materializing them waits on the identity
vocabulary settling: what a block node is called across a re-parse, how a
coordinate is addressed, and whether a concept node is per-project or
per-workspace. Edges written under a naming scheme we then change outlive the
change, which is worse than not writing them.

### Finding a term inside blocks

Term occurrence has to search text that lives inside a JSON payload no SQL can
read, so the block cache keeps `block_texts`: a row per block per locale — the
source under the empty locale, each target under its own — with a contentless
FTS5 trigram index over it.

The index is built **before a search, not during a write**. Maintaining it inside
every block write was measured first and cost roughly seven times the write:
extraction writes every block in the project, which is too much to levy for a
query that may never be asked. A write now only marks the block stale, and the
first search reconciles exactly what changed. The index narrows and never
decides — a trigram match is necessary, not sufficient — so matching is done in
Go, which is also how the browser build answers the same question by scanning.

`kapi terms occurrences` is the surface, and `kapi context search` carries a
usage count on each term it reports.

### Local and server converge in shape

Bowrain's Postgres spans workspaces, projects and streams. A local project answers
the **same query shapes** over `store.db` with those dimensions fixed to one
value. Term-occurrence retrieval by collection and coordinate works with no server
and no account; connecting one widens the scope rather than unlocking the
question.

This is what keeps the two surfaces honest with each other: a query added for the
server is expressible locally, and a local query does not have to be reinvented
when the project connects.

### Browser and wasm

There is no SQLite in the browser build. The model is unchanged and the backends
differ: in-memory content memory and terms, a path-keyed in-memory block store,
and a working set that persists to a JSON sidecar, `.kapi/work/store.json`. The same
sources rebuild it, and the same graph relations hold.

## Consequences

- **One connection per project.** Subsystems open the shared `storage.DB` rather
  than a file each; a single transaction can span a state record and the graph edges
  it implies.
- **Cross-subsystem queries are ordinary SQL.** Term coverage per collection,
  the blocks behind a coordinate, the units a term change puts at risk — joins,
  not application-level merges.
- **Store paths are not a user surface.** The recipe binds *sources*
  (`defaults.terms_source`, `defaults.memory_source`); it never names the derived
  database. Standalone stores outside a project keep their own selectors
  (`--termstore`, `--memory`) — those address a file the user owns, which is a
  different thing.
- **CI caches `.kapi/work/cache/`, not `store.db`.** The database carries staged
  unit state, and a cache key that restores stale state is worse than a cold
  rebuild ([Convergence in CI](/kapi/convergence-in-ci)).

## See also

- [AD-008: Project Model](008-project-model.md) — the project layout the store
  sits in.
- [AD-009: Content memory](009-content-memory.md) and
  [AD-010: Terminology](010-terminology.md) — the two subsystems whose
  source-versus-projection split this store implements.
- [AD-033: Project State Model](033-project-state-model.md) — the unit-state
  record and the working set inside `store.db`.
- [AD-037: The context retrieval surface](037-context-retrieval-surface.md) —
  the two primitives these tables answer.
- [The project store](/kapi/project-store) — the end-user view.
