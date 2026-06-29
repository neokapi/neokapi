---
id: 033-project-state-model
sidebar_position: 33
title: "AD-033: Project State Model"
description: "Architecture decision: a project's authored workflow decisions — the review ladder, approvals, sign-off, parking — live in a first-class core/state store, distinct from the derived cache and the recycle TM. The committed, diff-friendly serialization is the source of truth; an in-memory working set is transient until an explicit Export materializes it to a binding (a committed file in git mode, the server in bowrain mode)."
keywords: [project state, state store, core/state, review, approval, convergence, kapi-state.json, defaults.state, transient, export, targetHash, architecture decision, neokapi]
---

# AD-033: Project State Model

## Summary

A kapi project carries three kinds of information, and they have three different
homes. The **recipe** is config. The **files** are the deliverable. Between them
sits the project's **work** — and that work is itself two kinds of thing:

- **Derived state** — parsed content, coverage percentages, the convergence
  ladder. Rebuildable from the files; it lives in the cache under `.kapi/cache/`
  and is gitignored. Delete it and a re-run reconstructs identical results.
- **Authored decisions** — a person approving a translation, signing it off,
  parking a unit, or recording who reviewed what. These are *not* derivable from
  anything; they must be **kept**.

Authored decisions need a carrier that a plain target file (JSON, `.properties`)
cannot provide: such a file records that a target *exists*, but not that a human
*blessed* it. The `core/state` package is that carrier — a first-class,
format-independent, committed record of per-unit workflow decisions, distinct
from both the derived cache and the recycle translation memory ([AD-009](009-translation-memory.md)).

The end-user view of what this state *means* — the ladders, gates, and the
review queue derived from it — is [Convergence](/kapi/convergence) and
[the project store](/kapi/project-store).

## Context

The convergence model derives a project's localization state — per `(unit,
locale)`, a monotone ladder (`draft → translated → reviewed → signed-off`) and a
symmetric source ladder (`authored → checked → approved`). The lower rungs are
derivable from content: an absent target is below the ladder; a present, non-empty
target is at least *translated*. The **higher** rungs are not derivable — whether
a person reviewed *this exact translation* is a decision someone made, and it has
to be stored somewhere.

The model already expresses these facts (`model.TargetStatus`,
`model.SourceStatus`, `model.Origin`). What was missing was a *persistence* for
them that is independent of the deliverable format. The danger is to overload an
existing store — in particular the `.klftm` translation memory, which is
content-keyed leverage, not project state. Conflating "have we ever translated
this string?" (recycle, content-keyed) with "is *this* unit signed off, by whom?"
(decision, unit-keyed) is a category error: the two have different keys and
different lifecycles. AD-033 gives decisions their own engine.

## Decision

### Two kinds of state, two homes

The invariant *"delete the cache and lose nothing"* holds precisely because the
two kinds of state are separated:

| Kind | Examples | Home | Authoritative? |
|---|---|---|---|
| Derived | parsed blocks, coverage %, ladder rungs reachable from content | `.kapi/cache/` (block store, doc cache) | no — rebuildable, gitignored |
| Authored decision | approvals, sign-off, parking, reviewer, notes | the **state store** (`core/state`) | yes — committed |

The cache may *mirror* an authored decision in transit, but it never *owns* one.
Every decision's durable home is the committed state store.

### The TM is recycle, not the state carrier

The `.klftm` translation memory ([AD-009](009-translation-memory.md)) is the
**recycle corpus** — a content-keyed pool of source→target pairs reused to
pre-fill and leverage future translation. It does **not** record review
decisions. Adding a pair to the TM (`kapi apply` with `kind:"tm"`) is recycle
leverage; approving a unit (`kapi apply` with `kind:"review"`) writes the state
store. An approved pair may *also* land in the TM as leverage, but that is a side
effect, not where the decision lives.

### The committed serialization is the truth; the working set is an index

State has two representations, and conflating them is the trap to avoid:

1. **Source of truth — a committed, diff-friendly serialization.** A text
   document (`kind: kapi-project-state`, schema-versioned JSON) committed to git:
   mergeable, reviewable in a `git diff`, exchangeable to XLIFF
   (`<target state=…>`, notes, phase/owner — [AD-017](017-bilingual-format-interop.md)),
   carried by a `.klz` parcel's bilingual profile. This is what a clone or
   checkout restores from.
2. **Working set — a transient in-memory store** (`core/state.FileStore`), the
   fast random-access model for a session. **Derived** from #1; rebuilt by
   `Open`, materialized back by `Export`.

Committing a binary SQLite as the authoritative store would be git-hostile
(opaque, conflict-prone) and would defeat exchange, so the durable home is the
text serialization and any database is only a working index over it. The
invariant is preserved: discard the working index, re-`Open` from the committed
file, lose nothing.

> **Server variant.** In bowrain (server mode) the platform database *is* the
> authoritative store — git is not in the loop. Same model, different backend:
> file mode → committed serialization is truth; server mode → the server DB is
> truth; a desktop working copy mirrors the server.

### State is explicitly transient; export is explicit

Mutations to the working set are **not durable until an explicit `Export`**. This
is deliberately the git/bowrain mental model: decisions are like staged changes
you commit (or push) on purpose.

- `Put` / `Delete` mutate the transient working set.
- `Pending()` reports whether un-exported decisions exist — so nothing is lost
  silently; a `status` surface can report "N un-exported decisions".
- `Export()` materializes the working set to the durable home in one deliberate,
  auditable step (one clean diff), rather than churning an auto-commit on every
  approval.

This sharpens the invariant into two tiers: un-exported decisions are in-transit
(lose them like uncommitted git changes — expected); exported state is the source
of truth ("delete the cache, lose nothing" applies here).

### The committed location is a binding, not a fixed path

Because export targets a *binding*, there is no fixed location to hard-code. The
binding is the unification of the file and server worlds:

- **git mode** — export writes the committed state file (`defaults.state`,
  default `.kapi-state.json`, beside the recipe), mirroring how `tm_source` binds
  the committed `.klftm`. CI (or the user) commits it.
- **server mode** — export is `kapi push`; `kapi pull` imports server state into
  the working set.

Same verbs, different remote. This rides the existing source/sink binding model
rather than introducing a new one.

### Decisions are unit-keyed and content-hash-bound

State is keyed by the **unit** — `(unit identity, variant)` where variant is
locale plus optional tone/channel — not by content. Each decision additionally
records a `targetHash`: the content hash of the *specific* translation it
blesses. A decision is **stale** when the current translation's hash differs from
the one the decision recorded, so editing an approved translation drops the unit
back below *reviewed* on its own. A content-keyed index (the shape the old TM hack
used) structurally cannot express this — unit-keying plus `targetHash` is what
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
  `Store` interface with a committed-file implementation (`FileStore`:
  `Open`/`Get`/`Put`/`Delete`/`All`/`Pending`/`Export`). The on-disk form is
  schema-versioned (`SchemaVersion`, `Kind = "kapi-project-state"`).
- **Approvals flow through one verb.** `kapi apply` with `kind:"review"` records
  a decision in the state store, addressed by `(file, id, locale)` exactly as
  `kapi status --review` lists it. The desktop "approve" action and the CLI verb
  share the same `ApproveReviewUnit` path.
- **Coverage derives from the state store + target files**, never from TM
  properties. The TM is recycle-only.
- **Exchange and parcels carry state.** The committed serialization maps to XLIFF
  for third-party exchange and rides inside a `.klz` parcel's bilingual profile
  for hand-off ([AD-017](017-bilingual-format-interop.md)).
- **The recipe stays clean.** The recipe carries no state; it *binds* the state
  artifact via `defaults.state`, just as it binds `tm_source` / `termbase_source`
  ([AD-008](008-project-model.md)).

## See also

- [AD-008: Project Model](008-project-model.md) — the project layout and where
  `.kapi-state.json` sits among the ownership zones.
- [AD-009: Translation Memory](009-translation-memory.md) — the recycle corpus
  this state store is deliberately *not*.
- [AD-017: Bilingual Format Interop](017-bilingual-format-interop.md) — XLIFF /
  `.klz` exchange that carries state across a hand-off.
- [Convergence](/kapi/convergence) and [the project store](/kapi/project-store) —
  the end-user model derived from this state.
