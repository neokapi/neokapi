---
id: c-04-unit-state-and-decisions
sidebar_position: 4
title: "C-04: Unit state and the decision record"
description: "Architecture decision: a project's authored unit state (the review ladder, approvals, sign-off, parking) lives in a first-class core/state store. The committed, diff-friendly serialization under .kapi/state/ is the source of truth; a working set inside the project's one database stages changes until kapi commit publishes them."
keywords: [project state, decision record, core/state, review, approval, convergence, working set, staged, commit, targetHash, architecture decision, neokapi]
---

# C-04: Unit state and the decision record

## Summary

A kapi project carries three kinds of information with three different homes. The
**recipe** is config. The **files** are the deliverable. Between them sits the
project's **work**, and that work is itself two kinds of thing:

- **Derived state**: parsed content, coverage percentages, the rungs of the
  ladder reachable from content. Rebuildable; it lives in the cache under
  `.kapi/work/cache/` and is ignored. Delete it and a re-run reconstructs
  identical results.
- **Authored unit state**: a person approving a translation, signing it off,
  parking a unit, or recording who reviewed what. This is *not* derivable from
  anything; it must be **kept**.

Authored state needs a carrier a plain target file cannot provide: such a file
records that a target *exists*, not that anyone *blessed* it. `core/state` is
that carrier, a first-class, format-independent, committed record of where each
unit stands, distinct from both the derived cache and the recycle content memory
([C-09](c-09-content-memory.md)).

The end-user view of what this state *means* (the ladders, the gates, and the
review queue derived from it) is [Convergence](/kapi/convergence) and
[the project store](/kapi/project-store).

## Context

The convergence model derives a project's per-locale standing: per `(unit,
locale)`, a monotone ladder (`draft → translated → reviewed → signed-off`) and a
symmetric source ladder (`authored → checked → approved`). The lower rungs are
derivable from content: an absent target is below the ladder, a present
non-empty target is at least *translated*. The **higher** rungs are not: whether
a person reviewed *this exact translation* is something someone did, and it has
to be stored somewhere.

The source ladder is not merely reported: it gates the loop symmetrically with
the target ladder. Just as a target below its ship gate cannot ship, source below
the project's source gate (`model.DefaultSourceGate`, *checked*) is not
translated. The `source-gate` leading stage settles and gates the source before
the fan-out, and holds under-ready source rather than translating it
([Convergence, source first](/kapi/convergence#source-first)).

The model already expresses these facts (`model.TargetStatus`,
`model.SourceStatus`, `model.Origin`). What they need is a *persistence*
independent of the deliverable format. The danger is overloading an existing
store, in particular the content memory, which is content-keyed leverage, not
project state. Conflating *have we ever translated this string?* (recycle,
content-keyed) with *is this unit signed off, by whom?* (state, unit-keyed) is a
category error: the two have different keys and different lifecycles.

## Decision

### Two kinds of state, two homes

The invariant *"delete the cache and lose nothing"* holds precisely because the
two are separated:

| Kind | Examples | Home | Authoritative? |
| --- | --- | --- | --- |
| Derived | parsed blocks, coverage, rungs reachable from content | `.kapi/work/cache/`, and the derived tables of `.kapi/work/store.db` | no: rebuildable, ignored |
| Authored unit state | approvals, sign-off, parking, reviewer, notes | `.kapi/state/` (`core/state`) | yes: committed |

The cache may *mirror* authored state in transit, but it never *owns* it. The
durable home is the committed record under `.kapi/state/`, and the only window in
which the record exists nowhere else is between writing it and `kapi commit`.

### Content memory is recycle, not the state carrier

The content memory ([C-09](c-09-content-memory.md)) is the **recycle corpus**: a
content-keyed pool of source→target pairs reused to pre-fill and leverage future
work. It does not record review outcomes. Adding a pair to the memory (`kapi
apply` with `kind:"memory"`) is recycle leverage; approving a unit (`kapi apply`
with `kind:"review"`) writes the state store. An approved pair may *also* land in
the memory as leverage, but that is a side effect, not where the record lives.

### The committed serialization is the truth; the working set is an index

State has two representations, and conflating them is the trap to avoid:

1. **Source of truth: a committed, diff-friendly serialization.** JSON Lines
   under `.kapi/state/`, one shard per document, committed: mergeable, reviewable
   in a diff, exchangeable to XLIFF (`<target state=…>`, notes, phase and owner,
   [M-01](../multilingual/m-01-bilingual-interop.md)), carried by a `.kpz`
   parcel's bilingual profile. This is what a fresh checkout restores from. Each
   line is one `UnitState` object; lines are sorted within a shard so a file's
   bytes depend only on its contents, and there is no envelope, version field or
   header.
2. **Working set: tables in the project's one database.** `core/state.WorkStore`
   inside `.kapi/work/store.db` ([C-03](c-03-context-store-and-graph.md)) is the
   fast random-access model with transactions and hash lookups. Derived from the
   record: seeded from it when empty, materialized back by `Commit`.

   Being a database rather than memory is what makes the record durable the
   moment it is written, so committing is about *publishing* it rather than about
   not losing it. That is what lets committing be explicit without inventing a
   way to lose work. It is an index in every respect but one: deleting `store.db`
   costs nothing already committed, and it costs exactly the unit state staged
   since. That bounded exposure is why the database sits at the top of
   `.kapi/work/` and not under `cache/`.

   In the browser, where there is no SQLite, the working set persists to a JSON
   sidecar, `.kapi/work/store.json`; the model is unchanged.

Committing a binary database as the authoritative store would be hostile to
review (opaque, conflict-prone) and would defeat exchange, so the durable home
is the text serialization and the database is only a working index over it.
Discard the working index, reopen from the committed record, lose nothing beyond
what was staged.

**One line per unit, sharded by document**, rather than one JSON array. A single
indented document means one approval rewrites every byte of the file: the diff
for a one-word change is the whole project, two branches touching unrelated
documents conflict on sight, and a run approving many units moves orders of
magnitude more bytes than it writes. A line per unit makes an approval a one-line
diff; a shard per document keeps a documentation edit from churning the shard
holding the interface strings.

### Recording is implicit; publishing is explicit

Mutations to the working set are **not published until an explicit `Commit`**,
the mental model of staged changes:

- `Put` / `Delete` mutate the working set.
- `Pending()` reports how many unit-state changes are staged. `kapi status`
  surfaces the count and names the command that publishes them, staying silent
  when there are none, on the habit that a clean project should read clean.
- `Commit()` materializes the working set to the durable home in one
  auditable step, rather than churning a write on every approval. `kapi commit`
  is the verb.

Recording and publishing are different acts. A run of automated approvals should
not land in the tracked record before anyone has looked at it, and an explicit
commit is the moment at which someone can.

Because the working set is a database rather than memory, staged state survives
the process. The tiers are *staged* and *committed*, not *in transit* and
*durable*.

### Unit state is unit-keyed and bound to the pairing it blessed

State is keyed by the **unit**: `(document, unit identity, variant)`, where the
variant is the locale plus any further qualification, not by content.

The document is identity, not a label beside it. A unit id is unique inside its
document and nowhere wider: a reader names blocks by what the format gives it,
and for prose those names follow position, so every page of a documentation
collection carries an `h`, a `p` and an `fm_title`. Keyed on less, one page's
decision is the collection's decision: the reviewer's approvals are accepted,
reported applied, and all but the last document's discarded, and the pages that
lost theirs then read as stale against a source nobody edited.

The document's identity is a **durable key**, not its path. When extraction
reads a document, the working set records what the document held as well as
where it lives (`project.DocumentAdopter`, implemented by the project pool), and
`WorkStore.AdoptDocuments` matches each read against the documents the project
already knows, moving the decisions filed under an address onto the identity.
A run resolves the scope of every decision it records or reads through one
`host.DocumentIndex`, read once from the store. A file that moves keeps its
approvals. The path is the document's *address*: it is what a decision is scoped
by where the project holds no key (a fresh checkout, a build with no store),
and `host.DecisionScope` is that one fallback definition, so every party names
an unresolved document the same way. It is also the name the connector gives the
item, so a decision travels the sync protocol scoped to the item it was made in.

A decision is not about a translation; it is about a **pairing**: this rendering,
*of this source*. Each record therefore carries both halves, computed by the one
definition every party uses:

- `targetHash`: the content hash of the specific translation it blesses
  (`state.TargetHash`).
- `contentHash`: the **basis**, the content hash of the source wording it
  blessed that translation *for* (`state.SourceHash`, which is
  `model.ComputeContentHash`, the same normalization `core/reconcile` matches
  identity on, so a unit's basis and its identity signal are one number).

A record is **stale** when either half no longer matches what the project holds.
Editing an approved translation drops the unit back below *reviewed*; rewriting
its source does the same. Binding only the target is the half-measure that lets a
reviewer's blessing outlive the sentence it blessed: the translation stays
`translated`, stays approved, and ships wording for text the project no longer
has, reporting nothing.

Staleness is **derived on every read, never stored**. A decision is history and is
never rewritten; what a source edit changes is not the record but whether it still
describes the project. So the demotion happens where the state is read (coverage,
the review queue, the ship gate, the convergence plan), and a restored source
converges back onto the decision already on record, with nobody re-reviewing
anything.

A stale unit tallies at `draft`: a committed target exists, so it is not below the
ladder, but it is not a translation of the current source either. It withholds its
scope from shipping **whether or not a ship gate applies**. A coverage bar is a
threshold on quantity, and no threshold makes a translation of a rewritten
sentence shippable. An ungated project is precisely the one with nothing else to
catch it.

**Stale is work, not only a report.** The convergence fan-out treats a
basis-stale unit exactly as it treats one with no translation at all: it is in
the pending set on any scope (gated or not, since the `draft` tally would
otherwise read an ungated scope as complete), it is priced in `kapi up --plan` on
the same recycle-versus-AI split, and the pass produces a translation of the
source the project has now.

Staleness is one reason a produced unit is work, and the plan carries the others
on their own axis. What a pass spends a provider call on is decided by the
content memory, not by a target file: the pipeline reads the source documents,
`recycle` fills what the corpus answers, and `translate` drafts the remainder. So
`kapi up --plan` asks the corpus about every unit it counts, including on a
checkout whose store does not exist yet, where the committed content-memory
bundles are compiled into a corpus that lives only for that call, so a dry run
prices the recycling a run would do without creating the state a dry run must
not. A produced unit the record does not pair with its source (a rewrite, an
identical pair no approval stands behind, a pair refused for asymmetric inline
codes) is reported as **unanswered** and priced. It is kept apart from `stale`:
stale means a decision's basis moved, which also drives the review worklist and
shipping, and merging the two would make the plan and the run summary disagree.
Whether the price is a provider call is the drafting step's own question. The
step serves a stored draft when the project block store holds a translation of
the same source made under its current configuration fingerprint and the
governing context in force (`blockstore.TargetOverlay.ReusableFor`), and the
plan puts that question to a producer built the way a pass builds one
(`tool.StoredTargetReuser`), so the two answer it from one function. A unit the
step would serve this way is counted as a **stored draft** at no tokens; a
parked locale's whole draft set reads this way on the run after the one that
drafted it.
The plan judges a produced unit only once the record absorber has read its
committed target at the bytes on disk (the digest stamps of
[C-03](c-03-context-store-and-graph.md)); before that the corpus is unfinished,
its silence means "not asked", and the plan says so rather than quoting either a
free run or a provider call per translation the run will recycle. A produced
unit with no file on disk is a parked locale's draft, read out of the store;
nothing is left for the absorber to read, so the plan judges it at once. What the loop
cannot do is decide, so the re-draft never restores the withdrawn approval: the
unit returns at its presence baseline, in the review worklist, and the scope
stays withheld until someone reviews the new pairing.

The record absorber (`host/recordabsorb.go`) follows one rule for a pairing the
project's own record contradicts. A committed target is absorbed against the
source it *does* translate: the wording recovered by the decision's basis hash
for a locale that holds a decision, or the wording the block store held when the
pass last read the working tree for a source rewrite that affects every locale.
When that wording is unrecoverable the pair is not written at all, so the corpus
never learns a translation of a sentence the project no longer has. Once the
target has moved as well, the record describes neither half of what is on disk
and the pair is absorbed like any undecided one, which is what lets a person who
rewrites a sentence and its translation together keep the pairing they authored.

**An identical translation is a decision or it is nothing.** A target equal to
its source is dropped: unapproved, the identity is far more often a catalog
carrying its untranslated leaves verbatim than a translation that happens to
coincide, and absorbing one would fill the unit from its own source and take it
away from the AI step for good. Carrying an approval it is absorbed like any
other pair: a person read the pairing and said this wording is right, which is
what proper nouns, product names and short labels look like when they are
correct.

**A decision settles the check that guesses at it.** The rule
`target-same-as-source` is a heuristic for "nobody translated this", and an
approval is a person having read that exact pairing and answered the question it
asks, so an approved identical target is not reported and does not fail the ship
gate. The project's terms settle it the same way, for an entry whose target is
its source. Both are one rule (`host.identicalTargetRule`) because two surfaces
consult it: the gate `kapi check` fails on, and the check exclusions that
demote a unit below `translated` during `kapi up`. Only that one finding is
settled; a dropped placeholder on an approved unit is still a defect, and an
approval licenses nothing about it.

**A missing basis is unknown, not stale.** A record with no basis says nothing
about the source it blessed, and reading that silence as drift would demote
every such decision a project holds. Such a unit keeps its rung, ships as it
does, and is *counted*: `kapi status` reports how many decisions rest on an
unrecorded basis, so the assumption is visible rather than silent. It clears
itself: the next decision on the unit records a basis.

A content-keyed index structurally cannot express any of this. Unit-keying plus
the two hashes is what makes an approval unable to silently outlive the text it
approved, and it is the same fact the graph's `blesses` edge carries
([C-03](c-03-context-store-and-graph.md)), which carries both hashes for that
reason, so *which decision covers this unit, at which basis* is answerable by
traversal as well as by lookup. A connected venue applies the same basis rule
from its side.

### Who decided

A `Decision` records the authored outcome and who reached it. Two distinctions in
the identity string matter:

- An identity prefixed `ai/` (`state.AIIdentityPrefix`) marks a decision reached
  autonomously by a model. Such decisions count toward the reviewed and
  signed-off gate thresholds **only** when the gate's approver class is `any`
  (`core/gate`), so a project can require that a person be in the loop by
  configuration rather than by hope.
- An `agent/…` identity is **not** an AI decision: an agent acts on a person's
  behalf, and `state.IsAIDecision` says so.

An `AIReview` is a third thing again: an advisory annotation carrying a score
and findings, bound to the translation it judged so that an edit invalidates it
(`AIReview.Fresh`). It never moves a unit on the ladder.

### A venue is authoritative for what it accepts

The committed record is the project's. What a venue does with it is the venue's.
A push sends the record whole, so it can carry an approval the venue declines:
the pusher may hold no review permission for that language, or the workspace may
refuse a verdict on work its author wrote. The venue keeps such a record as the
basis it carries, with no rung above translated and no decider, and reports what
it refused.

The project follows that answer rather than restating its own. A refused verdict
is retired locally to the same basis, recorded rather than staged, because the
venue's answer about published work is not a person's pending decision. Both
ends compute the same record, so the decision component of the freshness ref
agrees again and the next push has nothing to send. Without that step the two
folds differ for good, and every push re-sends the same refused approvals.

The venue declines the other direction on the same terms. A push whose record
takes back a sign-off the venue holds, over the same translation of the same
source, is a withdrawal, and the venue applies it only for a pusher holding
review permission for the language. A refused withdrawal keeps the venue's
record, and the report carries that record back; the project writes it into
its committed record, recorded rather than staged, so the two agree again with
no pull between them.

### The committed location is fixed

The record lives at `.kapi/state/`, derived from the project layout: inside the
committed context, beside the terms and the content memory it makes claims about.
`kapi status` and `kapi commit` take no path, and the recipe binds nothing: the
record is a directory whose contents kapi owns and prunes, so pointing it at an
arbitrary location would invite a project to aim it somewhere kapi deletes from.
That is the difference from `terms_source` and `memory_source`, which bind any
path because a person authors those files.

Getting the record *out* of kapi's own layout is a job for exchange rather than
relocation (`kapi merge`, XLIFF `<target state=…>`, the `.kpz` bilingual
profile), which converts the record into a format a third party can read instead
of moving it.

### Why the committed record stays, whatever else exists

A hosted layer can coordinate review: concurrent reviewers, assignment, queues,
and a place for reviews done by people with no checkout. That is coordination
*around* the record rather than a replacement for it, and the committed record
keeps properties no live database can:

- **kapi runs on its own.** If unit state required a service, a plain kapi
  project could not converge at all.
- **The record belongs to the change that caused it.** Source drift happens in a
  pull request, and the state change belongs in that same diff, where a reviewer
  sees that an edit invalidated twelve approvals. A live database is *now*, not
  *at this commit*.
- **A checkout at a past commit must converge identically.** State versioned
  alongside the source gives that.
- **Local-first holds elsewhere too.** Redaction
  ([C-10](c-10-redaction.md)) exists so content can be withheld from a named
  destination. A design where unit state has to round-trip that destination
  contradicts it.

### Layering: the model in `core/`, the IO with its surface

The state record, its working set, and the convergence *model* (the ladder
types and the per-block rung helpers) live in `core/state` and
`core/convergence`, so every surface agrees on what the rungs mean. The
*orchestration* that reads files and computes a report stays with its IO. The CLI
re-exports the core types through aliases so downstream code sees one import.

## Consequences

- **`core/state`** holds `UnitState` (status, source status, origin, target hash,
  basis, decision, updated), a `Key`, the `Stale`/`Fresh`/`Reviewed` ladder
  helpers, and `WorkStore`, the working set over the sharded committed record
  (`Get`/`Put`/`Delete`/`All`/`Pending`/`Commit`, plus `Documents` and
  `AdoptDocuments` for document identity).
- **Approvals flow through one verb.** `kapi apply` with `kind:"review"` records
  the unit state in the project store, addressed by `(file, id, locale)` exactly
  as `kapi status --review` lists it. The desktop's approve action and the CLI
  verb share one path.
- **Coverage derives from the state store plus the target files**, never from
  content-memory properties.
- **Exchange and parcels carry state**, so a hand-off does not drop it.
- **The recipe stays clean.** It binds sources, never a derived artifact; the
  state record and the database that stages it are both fixed by the layout.

## See also

- [C-01: The project model](c-01-project-model.md): where `.kapi/state/` sits
  among the ownership zones.
- [C-03: The context store and graph](c-03-context-store-and-graph.md): the
  working set and the `blesses` edge.
- [C-09: Content memory](c-09-content-memory.md): the recycle corpus this store
  is not.
- [M-01: Bilingual Format Interop](../multilingual/m-01-bilingual-interop.md):
  the exchange that carries state across a hand-off.
- [Convergence](/kapi/convergence) and [the project store](/kapi/project-store):
  the end-user model derived from this state.
