---
id: 033-project-state-model
sidebar_position: 33
title: "AD-033: Project State Model"
description: "Architecture decision: a project's authored unit state — the review ladder, approvals, sign-off, parking — lives in a first-class core/state store, distinct from the derived caches and the recycle content memory. The committed, diff-friendly serialization under .kapi/state/ is the source of truth; a working set inside the project's one database stages changes until kapi commit publishes them."
keywords: [project state, state store, core/state, review, approval, convergence, kapi/units, working set, staged, commit, targetHash, architecture decision, neokapi]
---

# AD-033: Project State Model

## Summary

A kapi project carries three kinds of information, and they have three different
homes. The **recipe** is config. The **files** are the deliverable. Between them
sits the project's **work** — and that work is itself two kinds of thing:

- **Derived state** — parsed content, coverage percentages, the convergence
  ladder. Rebuildable from the files; it lives in the cache under `.kapi/work/cache/`
  and is gitignored. Delete it and a re-run reconstructs identical results.
- **Authored unit state** — a person approving a translation, signing it off,
  parking a unit, or recording who reviewed what. This is *not* derivable from
  anything; it must be **kept**.

Authored state needs a carrier that a plain target file (JSON, `.properties`)
cannot provide: such a file records that a target *exists*, but not that a human
*blessed* it. The `core/state` package is that carrier — a first-class,
format-independent, committed record of where each unit stands, distinct from
both the derived cache and the recycle content memory ([AD-009](009-content-memory.md)).

The end-user view of what this state *means* — the ladders, gates, and the
review queue derived from it — is [Convergence](/kapi/convergence) and
[the project store](/kapi/project-store).

## Context

The convergence model derives a project's per-locale state — per `(unit,
locale)`, a monotone ladder (`draft → translated → reviewed → signed-off`) and a
symmetric source ladder (`authored → checked → approved`). The lower rungs are
derivable from content: an absent target is below the ladder; a present, non-empty
target is at least *translated*. The **higher** rungs are not derivable — whether
a person reviewed *this exact translation* is something someone did, and it has
to be stored somewhere.

The source ladder is **not** merely reported: it gates the loop symmetrically
with the target ladder. Just as a target below its `ship_gate` cannot ship,
source below the project's `source_gate` (default `checked`) is not translated —
server-side convergence settles and gates the source *before* the fan-out and
holds under-ready source rather than translating it
([Convergence — source first](/kapi/convergence#source-first)). So the source
rungs recorded here are load-bearing, not informational.

The model already expresses these facts (`model.TargetStatus`,
`model.SourceStatus`, `model.Origin`). What was missing was a *persistence* for
them that is independent of the deliverable format. The danger is to overload an
existing store — in particular the `.memory.json` content memory, which is
content-keyed leverage, not project state. Conflating "have we ever translated
this string?" (recycle, content-keyed) with "is *this* unit signed off, by whom?"
(state, unit-keyed) is a category error: the two have different keys and
different lifecycles. AD-033 gives unit state its own engine.

## Decision

### Two kinds of state, two homes

The invariant *"delete the cache and lose nothing"* holds precisely because the
two kinds of state are separated:

| Kind | Examples | Home | Authoritative? |
|---|---|---|---|
| Derived | parsed blocks, coverage %, ladder rungs reachable from content | `.kapi/work/cache/`, and the derived tables of `.kapi/work/store.db` | no — rebuildable, gitignored |
| Authored unit state | approvals, sign-off, parking, reviewer, notes | `.kapi/state/` (`core/state`) | yes — committed |

`.kapi/work/cache/` may *mirror* authored state in transit, but it never
*owns* it; `rm -rf .kapi/work/cache` is free. Its durable home is
the committed record under `.kapi/state/`, and the only window in
which the record exists nowhere else is between writing it and `kapi commit` —
see the working set below.

### Content memory is recycle, not the state carrier

The `.memory.json` content memory ([AD-009](009-content-memory.md)) is the
**recycle corpus** — a content-keyed pool of source→target pairs reused to
pre-fill and leverage future translation. It does **not** record review
outcomes. Adding a pair to the memory (`kapi apply` with `kind:"memory"`) is recycle
leverage; approving a unit (`kapi apply` with `kind:"review"`) writes the state
store. An approved pair may *also* land in the memory as leverage, but that is a
side effect, not where the state record lives.

### The committed serialization is the truth; the working set is an index

State has two representations, and conflating them is the trap to avoid:

1. **Source of truth — a committed, diff-friendly serialization.** JSON Lines
   under `.kapi/state/`, one shard per document, committed to git:
   mergeable, reviewable in a `git diff`, exchangeable to XLIFF
   (`<target state=…>`, notes, phase/owner — [AD-017](017-bilingual-format-interop.md)),
   carried by a `.kpz` parcel's bilingual profile. This is what a clone or
   checkout restores from.
2. **Working set — tables in the project's one database** (`core/state.WorkStore`
   inside `.kapi/work/store.db`, [AD-039](039-local-context-graph-store.md)), the fast
   random-access model with transactions and hash lookups. **Derived** from #1;
   seeded from it when empty, materialized back by `Commit`.

   Being a database rather than memory is what makes the record durable the
   moment it is written, so committing is about *publishing* it rather than
   about not losing it. That is what lets committing be explicit without
   inventing a way to lose work. It is an index in every respect but one:
   deleting `store.db` costs nothing already committed, and it costs exactly the
   unit state staged since. That bounded exposure is why `store.db` sits at the
   top of `.kapi/work/` and not under `cache/`.

   In the browser, where there is no SQLite, the working set persists to a JSON
   sidecar, `.kapi/work/store.json`; the model is unchanged.

Committing a binary SQLite as the authoritative store would be git-hostile
(opaque, conflict-prone) and would defeat exchange, so the durable home is the
text serialization and the database is only a working index over it. The
invariant is preserved: discard the working index, reopen from the committed
record, lose nothing beyond what was staged.

**One line per unit, sharded by document**, rather than one JSON array. A single
indented document meant one approval rewrote every byte of the file:
the diff for a one-word change was the whole project, two branches touching
unrelated documents conflicted on sight, and a run approving many units moved
orders of magnitude more bytes than it wrote. A line per unit makes an approval a
one-line diff; a shard per document keeps a documentation edit from churning the
shard holding the interface strings.

> **Server variant.** In bowrain (server mode) the platform database *is* the
> authoritative store — git is not in the loop. Same model, different backend:
> file mode → committed serialization is truth; server mode → the server DB is
> truth; a desktop working copy mirrors the server.

#### Does the git record survive bowrain?

Yes, and the two are not alternatives competing for the same job. The
relationship is the one bowrain has generally — git is to bowrain what git is to
a hosting platform.

The committed record stays because it carries properties a server cannot:

- **kapi runs without bowrain.** The framework is standalone and never imports
  the platform. If unit state required a server, a plain kapi project could not
  converge at all.
- **The record belongs to the change that caused it.** Source drift happens in a
  pull request; the state change belongs in that same diff, where a reviewer can
  see that an edit invalidated twelve approvals. Server state is *now*, not *at
  this commit*.
- **A clone at a past commit must converge identically.** State versioned
  alongside the source gives that; a live database does not.
- **Local-first is load-bearing elsewhere.** Redaction ([AD-020](020-redaction.md))
  exists so content can be withheld from any named destination, bowrain included.
  A design where unit state must round-trip a server contradicts it.

What the server adds is what git is bad at: concurrent review by several people,
assignment and queues, and a place for reviews done by people who do not have
the repository checked out. That is coordination *around* the record, not a
replacement for it — so the server syncs with the committed record rather than
superseding it, and a project that never connects one loses nothing.

### Recording is implicit; publishing is explicit

Mutations to the working set are **not published until an explicit `Commit`**.
This is deliberately the git/bowrain mental model: unit state is like staged
changes you commit (or push) on purpose.

- `Put` / `Delete` mutate the working set.
- `Pending()` reports how many unit-state changes are staged; `kapi status` surfaces it
  and names the command that publishes them, staying silent when there are none.
- `Commit()` materializes the working set to the durable home in one deliberate,
  auditable step, rather than churning a write on every approval. `kapi commit`
  is the verb.

Recording and publishing are different acts. A run of automated approvals should
not land in the tracked record before anyone has looked at it, and an explicit
commit is the moment at which someone can.

Because the working set is a database rather than memory, staged state
survives the process. That is a deliberate departure from the git analogy: staged
work here is not "uncommitted changes you might lose", it is recorded work not
yet published. The tiers are *staged* and *committed*, not *in-transit* and
*durable*.

### The committed location is fixed

The record lives at `.kapi/state/`, derived from the project layout —
inside the committed context graph, beside the terms and the content memory it
makes claims about. `kapi status` and `kapi commit` take no path, and the recipe
binds nothing: the record is a directory whose contents kapi owns and prunes, so
pointing it at an arbitrary location would invite a project to aim it somewhere
kapi deletes from. That is the difference from `terms_source` and
`memory_source`, which bind any path because a person authors those files.

Getting the record *out* of kapi's own layout is a job for exchange rather than
relocation — `kapi merge`, XLIFF `<target state=…>`, the `.kpz` bilingual
profile — which converts the record into a format a third party can read instead
of moving it.

In server mode the same verbs point at a different remote: publishing is
`kapi push`, and `kapi pull` imports server state into the working set. This
rides the existing source/sink binding model rather than introducing a new one.

### Unit state is unit-keyed and content-hash-bound

State is keyed by the **unit** — `(unit identity, variant)` where variant is
locale plus optional tone/channel — not by content. Each record additionally
records a `targetHash`: the content hash of the *specific* translation it
blesses. A record is **stale** when the current translation's hash differs from
the one it was written against, so editing an approved translation drops the unit
back below *reviewed* on its own. A content-keyed index (the shape the old
memory-based hack used) structurally cannot express this — unit-keying plus
`targetHash` is what
makes an approval unable to silently outlive the text it approved.

### Layering — the model in `core/`, the IO with its surface

The state record, its store interface, and the convergence *model* (the ladder
types and per-block rung helpers) live in `core/` (`core/state`,
`core/convergence`), so every surface agrees on what the rungs mean. The
*orchestration* that reads files and computes a report stays with its IO — the
CLI's file-IO derivation in `cli/`, a future server's derivation against its own
store. The CLI re-exports the core types via aliases so downstream code sees one
import. This is the same "state in core, both surfaces agree" unification the
project model relies on elsewhere.

## Consequences

- **`core/state`** holds `UnitState` (status, sourceStatus, origin, targetHash,
  decision, updated), a `Key`, the `Stale`/`Reviewed` ladder helpers, and a
  `Store` interface over the sharded committed record
  (`Open`/`Get`/`Put`/`Delete`/`All`/`Pending`/`Commit`). The on-disk form is
  schema-versioned (`SchemaVersion`, `Kind = "kapi-project-state"`).
- **Approvals flow through one verb.** `kapi apply` with `kind:"review"` records
  the unit state in the project store, addressed by `(file, id, locale)` exactly as
  `kapi status --review` lists it. The desktop "approve" action and the CLI verb
  share the same `ApproveReviewUnit` path.
- **Coverage derives from the state store + target files**, never from
  content-memory properties. The memory is recycle-only.
- **Exchange and parcels carry state.** The committed serialization maps to XLIFF
  for third-party exchange and rides inside a `.kpz` parcel's bilingual profile
  for hand-off ([AD-017](017-bilingual-format-interop.md)).
- **The recipe stays clean.** The recipe carries no state and binds no derived
  artifact. It binds *sources* — `defaults.terms_source`,
  `defaults.memory_source` ([AD-008](008-project-model.md)) — while the state
  record and the database that stages it are both fixed by the project layout.

## See also

- [AD-008: Project Model](008-project-model.md) — the project layout and where
  `.kapi/state/` sits among the ownership zones.
- [AD-039: The Local Context Graph Store](039-local-context-graph-store.md) —
  `.kapi/work/store.db`, which holds the working set alongside every other subsystem.
- [AD-009: Content memory](009-content-memory.md) — the recycle corpus
  this state store is deliberately *not*.
- [AD-017: Bilingual Format Interop](017-bilingual-format-interop.md) — XLIFF /
  `.kpz` exchange that carries state across a hand-off.
- [Convergence](/kapi/convergence) and [the project store](/kapi/project-store) —
  the end-user model derived from this state.
