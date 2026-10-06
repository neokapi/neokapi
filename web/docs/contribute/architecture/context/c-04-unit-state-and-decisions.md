---
id: c-04-unit-state-and-decisions
sidebar_position: 4
title: "C-04: Unit state and the decision record"
description: "Architecture decision: a project's authored unit state (the review ladder, approvals, parking) lives in an append-only, content-addressed decision ledger in core/state. An entry applies where the revisions of the translation and the source it blessed match the content (its hashes, for an entry recorded before revisions), so one ledger serves every checkout of a project; the .kapi/state/ shards carry one checkout's view of it as text, which kapi context import reads."
keywords: [project state, decision ledger, core/state, review, approval, convergence, append-only, content-addressed, commit, revision, basis, targetHash, architecture decision, neokapi]
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
- **Authored unit state**: a person approving a translation, parking a unit, or recording who reviewed what. This is *not* derivable from
  anything; it must be **kept**.

Authored state needs a carrier a plain target file cannot provide: such a file
contains target text but no record of its approval. `core/state` is
that carrier, a first-class, format-independent record of where each unit
stands, distinct from both the derived cache and the recycle content memory
([C-09](c-09-content-memory.md)).

The end-user view of what this state *means* (the ladders, the gates, and the
review queue derived from it) is [Convergence](/kapi/convergence) and
[the project store](/kapi/project-store).

## Context

The convergence model derives a project's per-locale standing: per `(unit,
locale)`, a monotone ladder (`draft → translated → established`) and a source
ladder (`written → established`). The lower rungs are derivable from content:
an absent target is below the ladder, a present non-empty target is at least
*translated*, and present source is *written*. The top rung is not: whether a
person reviewed *this exact translation* and let it stand is something someone
did, and it has to be stored somewhere. There is one such rung, and the
ladder has no second human rung above it.

The source ladder is not merely reported: it gates the loop symmetrically with
the target ladder. Just as a target below its ship gate cannot ship, source below
the project's `defaults.translate_after` level (`model.DefaultTranslateAfter`,
*written*) is not translated. The `translate-after` leading stage settles the
source before the fan-out and holds each under-ready block rather than
translating it ([Convergence, source first](/kapi/convergence#source-first)).

The model already expresses these facts (`model.TargetStatus`,
`model.SourceStatus`, `model.Origin`). What they need is a *persistence*
independent of the deliverable format. The danger is overloading an existing
store, in particular the content memory, which is content-keyed leverage, not
project state. Conflating *have we ever translated this string?* (recycle,
content-keyed) with *is this unit established, by whom?* (state, unit-keyed) is a
category error: the two have different keys and different lifecycles.

## Decision

### Two kinds of state, two homes

The invariant *"delete the cache and lose nothing"* holds precisely because the
two are separated:

| Kind | Examples | Home | Authoritative? |
| --- | --- | --- | --- |
| Derived | parsed blocks, coverage, rungs reachable from content, the overlays a producer serves a draft from | `.kapi/work/cache/`, and the checkout's projection at `.kapi/work/store.db` | no: rebuildable, ignored |
| Authored unit state | approvals, parking, reviewer, notes | the decision ledger (`core/state`) in the project's context store | yes |
| A parked locale's drafts, and the edits made to them | the text of translations whose files a gate withholds | the workspace home (`core/workhome`) in the project's context store | yes, until a delivery writes them to their files |

The cache may *mirror* authored state in transit, but it never *owns* it. A
decision is durable in the ledger the moment it is recorded, and every checkout
of the project reads it from there. A parked locale's drafts are text, so they
have a home of their own: the workspace home keeps them in the operation log
([C-03](c-03-context-store-and-graph.md#the-workspace-home)), and the coverage,
the review queue and the checks read them there while their files do not exist.
A decision on such a draft binds to the revision the workspace home holds, as a
decision on a delivered translation binds to the revision its file holds.

### Content memory is recycle, not the state carrier

The content memory ([C-09](c-09-content-memory.md)) is the **recycle corpus**: a
content-keyed pool of source→target pairs reused to pre-fill and leverage future
work. It does not record review outcomes. Adding a pair to the memory (a
`memory` operation through `kapi apply`) is recycle leverage; approving a unit
(a `decide` operation through `kapi apply`) writes the state store. An approved pair may *also* land in
the memory as leverage, but that is a side effect, not where the record lives.

### The ledger is the authority

Decisions are content-addressed entries in an **append-only ledger**. Updates
and withdrawals append new entries. Each entry contains the decision record,
actor, origin and a timestamp assigned by Go.

The entry's key identifies the unit **and its source/target pairing**: `(document, unit
identity, variant, basis, revision)`, beside the source and target hashes, which
an entry recorded before revisions carries alone. That is what makes the ledger
answerable across checkouts, and it is the same fact the `blesses` edge carries
([C-03](c-03-context-store-and-graph.md)).

The address is the SHA-256 of the record, the actor and the withdrawal flag, so
two parties that reach the same decision about the same pairing write the same
entry. Recording one the ledger already holds leaves it alone, which is what
lets a project read its shards in, pull the same venue ledger twice and replay a
change feed without the ledger growing. Recording the entry that already
answers for its pairing is the same no-op; recording an older entry again
moves its moment forward so it answers once more.

### The ledger is a projection of the operation log

In a workspace, every ledger write is an operation in the workspace's log
before it is a row ([C-03](c-03-context-store-and-graph.md#the-stores-are-projections-of-the-log)).
`state.WorkStore` records through a journal the host binds when it opens the
project store (`WorkStore.SetJournal`, implemented by `projector.Decisions`):
each entry becomes one `decision.record` operation carrying the record, the
actor, the origin and the moment, and the projector writes the row with
`state.ApplyEntries`. The operation's content address is
`decision:<project>:<entry id>`, so the entry's own address carries over: one
decision recorded twice, on one machine or on two whose logs are merged, is one
operation. A re-assertion of an older entry is a new event and carries no
address.

`kapi context rebuild` therefore rebuilds the ledger with the other stores, and
a second machine that merges the log receives the decisions with it. Each
checkout's view stays out of the log: it is a reading of that checkout's files,
written by the store beside the journal. The embedded layout a test opens has
no log, and its store writes the ledger directly.

A change to the text itself is a separate record. An edit that reaches the
edit recorder is a `content.edit` operation, projected into the block history,
which answers who changed an edition, from which revision to which and through
which surface
([C-03](c-03-context-store-and-graph.md#edits-are-recorded-as-content-edit)). A
decision says a person stands behind a pairing; the block history says how the
text in that pairing came to be.

The ledger holds decisions only. An entry decides something when it carries a
review state (an approval or a rejection, of a translation or of source
wording), a rung above translated, a parked unit, an assignee, a note, or an
agent's pre-review, which is a `decide advise` (`state.UnitState.Decides`).
`WorkStore` refuses an entry that decides nothing with
`state.ErrDecidesNothing`: a translation, the source it was made from and the
rung its producer put it on belong to the block history. A shard line or a log
entry that decides nothing, which only an earlier release wrote, is left out on
import and on projection, so a rebuild holds decisions only too, and so does a
predecessor store a newer build carries forward: an entry that decides nothing
is left behind, and the decisions after it come across. An import reports the
decisions it recorded. Each writer follows from that. A pull records the
venue's decisions and leaves out its records of what it produced, whose
translations arrive as content and whose write the pull records in the block
history, with the source the venue's record names as its basis where that is
the source the checkout holds. A governing fingerprint read back from a
content-memory bundle lands only on a decision. A verdict a venue refused is
withdrawn (a revocation entry) unless the record still decides something else,
and an approval the venue kept over a withdrawal the project made is recorded
back from the record the venue sent.

### Document keys are recorded in the log

A decision is filed under its document's key, so every checkout of a project
has to give a document the same key. Each one a checkout resolves is a
`document.adopt` operation (`state.Adoption`, recorded through the same
journal): the key, the path the document was read at, the content hash of each
block it held there, a digest over those hashes, and the id of the adoption the
key held before (`Prev`). Its id (`state.AdoptionID`) is the SHA-256 over the
key, the path, the digest and that previous id, and its content address is
`adopt:<project>:` followed by the id, so two checkouts that see one document
move from one state to the same path and content record it once, and a
document that returns to a path or a content it held before is adopted again. A
checkout records an adoption only when the project does not hold it yet: a key
never adopted, or one now read at another path or with other content.

The projector writes `document_adoption`, one row per key: where it was most
recently read, what it held there, the adoption's id and when the key was
first adopted, the same rows whatever order the operations arrive in.
`WorkStore.AdoptDocuments` matches a read against the documents this checkout
has read first, so a checkout keeps the keys it has, and then against the
project's adoptions (`WorkStore.AdoptedDocuments`), the earliest adopted first.
A checkout reading a document for the first time therefore takes the key the
project already uses, at the path another checkout recorded or by what the file
holds, before it mints one from the path. A key derived from a path that
another document already holds, one that moved away from that path, is taken
whether or not that document is in the read, and the new document gets the
next ordinal (`d-…-2`). `host.DocumentIndex` answers a path the checkout has
not resolved from the same adoptions, so a fresh checkout names a renamed
document correctly before its first extraction.

Two checkouts that mint different keys for one document before either's log
reaches the other keep both: each prefers the key it holds, and the decisions
filed under each stay with it.

### Applicability is a lookup

`WorkStore.Lookup` returns the decision for the source and the translation a
checkout holds (`state.Reading`): an entry recorded with revisions where they
are the content's, and one recorded before revisions where its hashes are,
until an entry with revisions is recorded for the same hashes. From then on the
later entry speaks for that text, whether or not its revisions are still the
content's, and of the entries that answer the latest does. A later withdrawal is
therefore never undone by an older entry for the same text, even after an
inline code moves under it. Different
translations of a unit have separate ledger entries, two that differ in an
inline code alone among them. Checkouts with identical source and translation
share the same decision, regardless of branch.

The ledger and each checkout's view carry the revisions beside the hashes (store
migration 8). A store written before revisions gains the two columns empty, and
nothing rewrites an entry to add them: it answers by its hashes until the next
decision on its unit records the revisions.

Each checkout maintains a derived **view** of its current unit pairings. Content
changes, including branch switches, rebuild this view without modifying the
ledger. `WorkStore.ClearView` clears only the checkout's view; decisions remain
available if their source and target pairing appears again.

### Recording is durable; the shards are an artifact

`Put` and `RecordEntry` append to the ledger and are durable at once.
No separate publish or commit step is required.

The log is the source of the decisions, and the shards under `.kapi/state/`
are one input to it: an import records each line it takes as a
`decision.record` operation. Decisions move between machines as operations, through a context
backend or a transfer file ([C-03](c-03-context-store-and-graph.md)).

**Import.** `kapi context import` reads the shards into the ledger, and a
   person runs it ([C-11](c-11-context-operations.md)). A line the
   ledger already holds costs nothing, and a line older than the entry in force at
   its pairing (by the `Updated` stamp both ends write) is left out, which is the
   same last-writer-wins rule a venue pull follows. A fresh clone picks up the
   project's decisions this way when its store has never held them. A record read
   in this way is recorded in the order that leaves this
   checkout's view where it was: the lines whose pairing it already holds go
   last, so a record carrying several branches' answers for one unit cannot
   repoint it. `state.CommittedDigest`, over each shard's name and bytes and stamped
   per checkout in `state_meta`, is the fast path for an import that would find
   nothing: the same key-and-value shape the block cache uses for its extraction
   stamps ([C-03](c-03-context-store-and-graph.md)).

**One line per unit, sharded by document**, rather than one JSON array. A single
indented document means one approval rewrites every byte of the file: the diff
for a one-word change is the whole project, two branches touching unrelated
documents conflict on sight, and a run approving many units moves orders of
magnitude more bytes than it writes. A line per unit makes an approval a one-line
diff; a shard per document keeps a documentation edit from churning the shard
holding the interface strings.

The ledger itself lives in the project's context store
([C-03](c-03-context-store-and-graph.md)), beside the content memory, which
gives a decision and the wording the content memory learns from it one
transaction on one connection pool. That store sits in the workspace rather than
in the checkout, so every checkout of the project records into one ledger and
each answers from its own view of it. Committing a binary database as the
reviewable record would be hostile to review (opaque, conflict-prone) and would
defeat exchange, so the shards stay text.

The browser build keeps the ledger and the view in the same tables, on its
SQLite WebAssembly driver ([C-03](c-03-context-store-and-graph.md)).

### Who may record what is policy

Every write goes through one function (`state.Policy`), which sees the pairing,
the actor, the origin and what applies at that pairing now, and either records or
refuses. The default records everything: a single-player project's decisions are
the person's, and a connected venue enforces its own permissions on its side and
reports what it refused. An actor class with narrower or wider rights is a change
in that one function rather than at each call site, which is why actor and origin
ride on every entry.

### Unit state is unit-keyed and bound to the pairing it records {#unit-state-is-unit-keyed-and-bound-to-the-pairing-it-blessed}

State is keyed by the **unit**: `(document, unit identity, variant)`, not by
content. The unit is a block, named by the identity its reader gives it, and the
variant is an edition key (`model.EditionKey`): the locale plus any further
qualification. The surfaces that show a decision name the same three as the
document, the block and the edition.

The document is identity, not a label beside it. A unit id is unique inside its
document and nowhere wider: a reader names blocks by what the format gives it,
and for prose those names follow position, so every page of a documentation
collection carries an `h`, a `p` and an `fm_title`. Keyed on less, one page's
decision is the collection's decision: the reviewer's approvals are accepted,
reported applied, and all but the last document's discarded, and the pages that
lost theirs then read as stale against a source nobody edited.

The document's identity is a **durable key**, not its path. When extraction
reads a document, the checkout's view records what the document held as well as
where it lives (`project.DocumentAdopter`, implemented by the project pool), and
`WorkStore.AdoptDocuments` matches each read against the documents the project
already knows, moving the decisions filed under an address onto the identity
([document keys are recorded in the log](#document-keys-are-recorded-in-the-log)).
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

- `revision`: the revision of the specific translation it blesses
  (`model.EditionRevision`, the token an `if_match` names), which covers the
  translation's inline codes and their attributes.
- `basis`: the revision of the source wording it blessed that translation
  *for*, the authoritative edition's revision: the token a change set's `basis`
  carries and a derived edition's derivation names (`model.Derivation.Rev`).
- `targetHash` and `contentHash`: the content hashes of the two halves
  (`state.TargetHash`, and `state.SourceHash`, which is
  `model.ComputeContentHash`, the same normalization `core/reconcile` matches
  identity on, so a unit's basis by hash and its identity signal are one
  number).

A record is graded by its revisions where it carries them, and by its hashes
where it was recorded before revisions (`state.Reading`). The two answer alike
except where an inline code alone moved: a changed link target, or a link
removed, retires a decision recorded with revisions and leaves one recorded
before them standing.

A revision covers the key its edition is filed under, and the readers of one
document do not file its source alike: the change service keeps a language the
file declares, a project read files every block under the project's source
language, and a reader that declares none leaves the block with none. The source
half is therefore matched under every key a reader of the document gives it
(`model.Block.SourceRevisions`), and every project read that files a block under
the project's language keeps the language its reader declared on the block
(`model.PropReadSourceLocale`), the workspace home's reading of a parked
locale's drafts included. The content is the same under every key, so the
match finds exactly the source as it stands. A derivation's standing
(`model.Block.BasisStanding`) and the record absorber's recovery of the source
at a basis match the same keys.

A connected venue names the pairing by revision alone: a decision travels the
sync protocol as its two revisions, without the hashes, and the venue grades its
ledger by them. It stamps every block it stores with the revision of its source
under the project's source language (`venue.SourceRevision`), a function of the
source's runs and that language alone, so a write that carries some of a
block's translations and not others stamps the same revision. A decision is
current there while its basis is that revision and its revision is the
translation's. The status projection, the grouped tally, the draft mark, the
review context's stale flag and the governance a push is put to all read the
pairing that way. A decision that raises a translation to *established* needs a
current basis to project; a rejection or a withdrawal projects unless its basis
is stale, so a rejection of a translation written outside kapi, which names no
basis, lowers the translation on the venue as it does on the checkout. A push
decides what to send by a transfer hash that folds the same source revision
(`venue.RecordHash`), so a change to an inline code alone reaches the venue, and
it retires a decision there as it does on the checkout.

The venue takes every revision under the one key, and a checkout records a basis
under whichever key its reader filed the source by. A push therefore sends each
basis it carries, of a decision or of an edition write, as the venue's revision
of the same source (`venue.Basis`): where the basis is among the block's source
revisions, it travels as `venue.SourceRevision` of the block the push reads, and
any other basis travels as it is and reads stale on both sides. The venue's
revision is among the source revisions a checkout accepts, so a record pulled
from the venue reads current on a checkout holding the same source. A project's
source language cannot change once its venue holds content, since every stamped
revision and every basis is taken under it.

A record is **stale** when either half no longer matches what the project holds.
Editing an approved translation drops the unit back below *established*; rewriting
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

A scope's verdict therefore takes one of three ship states, decided once in the
coverage rollup and read by every surface that reports one. A scope is
**shippable** when a ship gate matches it, it clears the gate, and nothing
withholds it. It is **withheld** when it is short of a matching gate or a
withhold applies: stale wording, a rejected translation, a failing check, or a
unit the terms govern with no terminology result. It is **not gated** when no gate
matches and nothing withholds. A not-gated scope carries no shippable claim:
`kapi status` and `kapi up` name it not gated, and `ship.json` records
`state: not_gated`. It keeps the two-field reading `gated: false`,
`shippable: true`, so a consumer that reads only `shippable` offers it. A locale
spread over several collections takes the weakest state among them.

**Stale is work, not only a report.** The convergence fan-out treats a
basis-stale unit exactly as it treats one with no translation at all: it is in
the pending set on any scope (gated or not, since the `draft` tally would
otherwise read an ungated scope as complete), it is priced in `kapi up --plan` on
the same recycle-versus-AI split, and the pass produces a translation of the
source the project has now. The server venue derives the answer from its
ledger by revision: one grouped query grades every recorded basis against the
revision of the current source, and a stale unit is withheld from the produced count until a pass has
drafted it, so a run started by a source change has pending work and produces.
For a unit nobody has decided, the venue's ledger row carries the basis of the
latest draft: one its own run made, or one a run on a checkout made, which the
push carries beside the decisions
([S-07](../surfaces/s-07-context-centric-review.md#a-push-carries-decisions-the-venue-decides)).
A basis record from a push carries the basis and nothing else: it never
replaces a decision, and it leaves an undecided row's rung, note and assignee
as they were. A translation a checkout pulled from the venue sends no basis,
since the venue's own record of it already holds one.

**Only an approval re-stamps the basis.** What clears a stale unit is the next
decision on it, and one kind of decision: a reviewer looking at the re-drafted
translation and saying it stands, on the source in front of them and under the
governance in force where they are deciding. That verdict binds both halves of
the basis, the source and the governing fingerprint, and the readers that
grade the unit compare against it, so the unit reads current on the source axis
and clears the staleness gate on the governance axis
([C-05](c-05-freshness.md)). The never-over-a-decision rule holds: the approval
IS the decision.

A **rejection** records the verdict, the rejected translation and the reviewer.
It preserves the basis recorded by the last approval or by the run that
produced the translation, so the unit's staleness remains unchanged. A
translation the loop produced and nobody has decided has its basis in the
block history, as the flow's write that left it
([C-03](c-03-context-store-and-graph.md#edits-are-recorded-as-content-edit)), and
a decision on it starts from that write: its basis, by revision and by hash, and
the producer's stamp. Coverage grades the write's basis by revision. Withdrawing
an approval has the same effect.
On the server, rejection also clears the draft mark, scheduling a new draft.

**A rejection is work whatever the basis says.** Comparing the two halves of the
pairing answers whether a translation renders the source the project holds. It
says nothing about whether the project stands behind that translation, and a
reviewer turning down a translation of the source in front of them moves neither
half, so
a reader grading the basis alone sees a settled record over a unit sitting at
`draft` with a refusal on it. The verdict is therefore read beside the basis: a
rejected unit is owed a draft until the venue has drafted it again since the
rejection.

Each venue answers "since the rejection" from what it already keeps. On the
server it is the draft mark, which the rejection clears and the next pass
stamps, so one rejection buys exactly one draft and a second rejection buys one
more. Locally it is the decision's target half: the rejection names the
translation it refused, a pass that drafts something else moves the unit off
that pairing, and the decision stops applying. Neither venue can loop, and a
pass that reproduces the refused wording word for word leaves the unit exactly
where it was, held out of shipping rather than quietly delivered.

The count is published beside the stale split rather than inside it. A
rejection on an unmoved source belongs to neither half of that split, and
folding it in would break the subset relation the derive depends on. It is
`convergence.LocaleCoverage.RejectedAwaitingDraft` locally,
`rejected_awaiting_draft_blocks` on the dashboard stats, and
`DecisionBasisTally.RejectedOwed` in the ledger's grouped tally, where it holds
the rejections `Stale` does not, so the two are disjoint. A convergence pass
owes a draft for `Owed + RejectedOwed` units, which is the number the derive
withholds from the produced count and the number the worker's own predicate
partitions out. Such a unit holds its scope out of shipping on both venues,
exactly as a stale one does.

**A decided unit is re-drafted once per source change.** The re-draft cannot
decide, so a stale decision stays stale until a person re-reviews, and a loop
that read only the decision would draft the unit again on every pass. Each
venue keeps its own record of what it last drafted. Locally, the content memory
absorbs the re-drafted pairing and the next pass recycles it rather than paying
for it again. On the server, the ledger row carries the source the platform
last drafted the unit against beside the decision (`unit_decisions.draft_basis`),
written by the worker for every target it produces and never over the decision
itself. A stale unit whose mark names the current source is owed nothing by the
loop: it counts as produced again, the run converges, and the unit waits on a
reviewer with its ship state withheld. A source rewritten again moves away from
the mark, and the unit is owed once more.

**The stale count splits by what the unit waits on.** A stale unit the loop has
not yet drafted against the source the project holds now is owed a convergence
pass; one it has drafted is owed a person's attention. Both are stale and both
withhold the scope, and the work is different, so the count is published as its
two halves as well as its total (`convergence.LocaleCoverage.StaleAwaitingDraft`
and `StaleAwaitingReview` locally, `stale_awaiting_draft_blocks` and
`stale_awaiting_review_blocks` on the dashboard stats). Each venue reads the
split from what it already holds: locally, whether the record still describes
the translation on disk; on the server, whether the row's draft mark names the
block's current source, which is the `Owed` half of the grouped tally. `Owed`
stays a subset of `Stale`, which is what lets the derive subtract it from a
produced count the stale units are already inside.

Staleness is one reason a produced unit is work, and the plan carries the others
on their own axis. What a pass spends a provider call on is decided by the
content memory, not by a target file: the pipeline reads the source documents,
`recycle` fills what the corpus answers, and `translate` drafts the remainder. So
`kapi up --plan` asks the corpus about every unit it counts, reading the
project's content memory without writing anything, so a dry run prices the
recycling a run would do without creating the state a dry run must not. A
produced unit the record does not pair with its source (a rewrite, an
identical pair no approval stands behind, a pair refused for asymmetric inline
codes) is reported as **unanswered** and priced. It is kept apart from `stale`:
stale means a decision's basis moved, which also drives the review worklist and
shipping, and merging the two would make the plan and the run summary disagree.
The plan prices only the languages the run's first pass works on, chosen from
coverage derived as the run derives it before that pass (`localesNeedingPass`,
with the bound checks unless the run skips them). A pass drafts every
unanswered unit of a language it works on, and a language with nothing
missing, stale, rejected, failing a check or short of its gate gets no pass, so
its unanswered units are no work and its exact lookups are not asked.
Whether the price is a provider call is the drafting step's own question. The
step serves a stored draft when the project block store holds a translation of
the same source made under its current configuration fingerprint and the
governing context in force (`blockstore.TargetOverlay.ReusableFor`), and the
plan puts that question to a producer built the way a pass builds one
(`tool.StoredTargetReuser`), so the two answer it from one function. A unit the
step would serve this way is counted as a **stored draft** at no tokens; a
parked locale's whole draft set reads this way on the run after the one that
drafted it. The block store is a cache: the workspace home keeps each parked
draft with the stamp its producer serves it by (the overlay's key,
configuration fingerprint and source stamp), and a run writes the overlays it
lacks back before its first pass, so a checkout whose `.kapi/work/` was deleted
serves its parked drafts without a provider call. A draft a person or an agent
edited since is restored from the latest draft a producer wrote for it, which
the log keeps.
The plan judges a produced unit only once the record absorber has read its
committed target at the bytes on disk (the digest stamps of
[C-03](c-03-context-store-and-graph.md)); before that the corpus is unfinished,
its silence means "not asked", and the plan says so rather than quoting either a
free run or a provider call per translation the run will recycle. A produced
unit with no file on disk is a parked locale's draft, read out of the
workspace home; nothing is left for the absorber to read, so the plan judges it
at once. What the loop
cannot do is decide, so the re-draft never restores the withdrawn approval: the
unit returns at its presence baseline, in the review worklist, and the scope
stays withheld until someone reviews the new pairing.

The record absorber (`host/recordabsorb.go`) follows one rule for a pairing the
project's own record contradicts. A committed target is absorbed against the
source it *does* translate: the wording recovered by the decision's basis (its
revision, or its hash for a decision recorded before revisions) for a locale
that holds a decision, or the wording the block store held when the pass last
read the working tree for a source rewrite that affects every locale. That
wording is the text at the basis revision, so the content memory pairs the
translation with it, and a later lookup classifies the source in hand against it
(`edit.Classify`).
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
the pairing is what makes an approval unable to silently outlive the text it
approved, and it is the same fact the graph's `blesses` edge carries
([C-03](c-03-context-store-and-graph.md)), which carries both halves, by
revision and by hash, for that reason, so *which decision covers this unit, at
which basis* is answerable by traversal as well as by lookup. A connected venue
applies the same basis rule from its side, by revision: a record that names no
basis is unknown there too, counted and never stale.

### What governed the decision

A record also carries **what governed the answer it is about**:
`governingFingerprint`, the fingerprint of the voice guidance and the term rules
in force at the unit's governance point for its locale
([C-05](c-05-freshness.md)). It is the value every translation producer stamps
on a target's `Origin.ContextFingerprint`, computed by the one function all of
them share (`core/profile.GovernanceContext`), so a decision and a produced
target are comparable against the same context.

Two writers set it. A **decision** (`kapi apply`, the desktop's approve action,
an agent's pre-review, an approval made in a connected venue)
resolves the context in force where the decider is deciding and records it
beside the verdict. Each decider resolves it from what it has: a project reads
the recipe's bindings at the unit's point, a venue reads the voice profile its
own ladder resolves and the term rules its workspace holds, and both fold the
result with the one function every producer stamps with, so the two are
comparable. The same verdict on the same pairing under a moved context is a new
decision, and is recorded again. A **basis** the convergence loop writes for
its own output records the producer's stamp: the run that wrote the target is
the run recording it. A hand-typed translation records none, because nothing
vouches for the context it was written under. The venue ledger carries the same
column, so a push and a pull agree on it.

The fingerprint is a different quantity from the record's identity signals, and
neither is derivable from the other. `contextHash` says *which block* this is
(`model.ComputeContextHash` over the block's name, type and properties) and moves
when the block's surroundings move. `governingFingerprint` says *what governed*
the answer and moves when the voice or the terminology moves. Either changes
with the other holding still.

The record is the durable carrier of this value. A bilingual format keeps the
producer's stamp beside the words, but most delivered formats hold strings and
nothing else: a JSON catalog, a `.properties` file. A reader pairing such a
file with its source finds what governed the translation in the record for the
unit or nowhere. That is what the record absorber reads, in this order: the
target's own stamp where the format carries one, then the record row for the
unit and locale, while that row still describes the translation on disk. The
value travels into the content memory beside the answer
([C-09](c-09-content-memory.md)), and compiling a content-memory bundle back
into the store writes it onto the record row for the unit the answer was
recorded for, where that row holds none and is about that translation. A record
written before the field existed reads through the producer's stamp on its
`origin`, which is all it has to say.

### Who decided

A `Decision` records a person's outcome and, where a hosted surface knows it,
who reached it. Every decision is a `decide` operation sent to the change
service ([E-09](../engine/e-09-the-change-contract.md)), and an agent or a model
never records one: the service refuses a `decide` other than `advise` from an
agent as `not_permitted`, whichever surface carries it (`kapi apply` run from an
agent's shell, MCP `apply_edits`). An agent pre-reviews instead: it stores a
score and its reasons on the unit (`state.AIReview`, a `decide` operation with
outcome `advise`, the desktop pre-review), bound to the translation it judged,
and the person reviewing reads it in the queue. Its judgement never counts as a
person's.

An `AIReview` is a third thing again: an advisory annotation carrying a score
and findings, bound to the translation it judged so that an edit invalidates it
(`AIReview.Fresh`). It never moves a unit on the ladder.

### A venue is authoritative for what it accepts

The project stores decisions locally; each venue enforces its own review policy.
A push sends the record whole, so it can carry an approval the venue declines:
the pusher may hold no review permission for that language, or the workspace may
refuse a verdict on work its author wrote. The venue keeps such a record as the
basis it carries, with no rung above translated and no decider, and reports what
it refused.

The project follows that answer rather than restating its own. A refused verdict
is recorded locally as the same basis, with the venue as the entry's origin,
because the venue is who reached it. Both
ends compute the same record, so the decision component of the freshness ref
agrees again and the next push has nothing to send. Without that step the two
folds differ for good, and every push re-sends the same refused approvals.

The venue declines the other direction on the same terms. A push whose record
takes back an established unit the venue holds, over the same translation of the same
source, is a withdrawal, and the venue applies it only for a pusher holding
review permission for the language. A refused withdrawal keeps the venue's
record, and the report carries that record back; the project writes it into
its own ledger with the venue as the origin, so the two agree again with
no pull between them.

A rejection is bound to the translation it judged in the same way. A pushed
rejection of a translation the venue has since replaced changes nothing the venue
holds: the venue keeps the unit's record, reports the rejection it did not apply
and carries its record back. The project takes that record, or keeps the basis
without the rejection where the venue holds none.

A venue also keeps a decision after the content it judged is removed. A file
leaving a checkout says nothing about the record, which still holds the decision,
and a project sends its record again only when that record's fold moves. A venue
that dropped the decision would have nothing to bring it back, so it keeps the
decision and whatever entry the decision hangs from, and content arriving at that
path again lands on that entry, with the decision still recorded against it. What
the venue reports as content leaves that entry out, so a project is never asked
to account for a file it does not have.

### The shards' location is fixed

A checkout's decision shards sit under `state/` in its `.kapi/` directory
(`project.ExportLayout.UnitStateDir`), and `kapi context import <dir>` reads the
same layout under the directory it is given. The recipe has no separate binding
for this directory. When an import, a pull or a content-memory bundle records
decisions into a checkout, the store writes the checkout's view back to its
shards (`WorkStore.Commit`), one shard per document, and removes each shard whose
document the view does not name.

Getting the record *out* of kapi's own layout is a job for exchange rather than
relocation (`kapi merge`, XLIFF `<target state=…>`, the `.kpz` bilingual
profile), which converts the record into a format a third party can read instead
of moving it.

### Local decision records {#why-a-project-keeps-its-own-record-whatever-else-exists}

A hosted layer can coordinate review: concurrent reviewers, assignment, queues,
and a place for reviews done by people with no checkout. That is coordination
*around* the record rather than a replacement for it, and a project's own record
keeps properties no live service can:

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

The decision ledger, a checkout's view of it, and the convergence *model* (the
ladder types and the per-block rung helpers) live in `core/state` and
`core/convergence`, so every surface agrees on what the rungs mean. The
*orchestration* that reads files and computes a report stays with its IO.

## Consequences

- **`core/state`** holds `UnitState` (status, source status, origin, the
  pairing by revision and by hash, governing fingerprint, decision, updated), a
  `Key`, a `Pairing`, a `Reading` and the `Stale`/`SourceStale`/`Fresh`/
  `Established` helpers that grade a record against it, and `WorkStore`, the ledger and this
  checkout's view of it (`Lookup`/`Get`/`Put`/`RecordEntry`/`Delete`/
  `All`/`Priors`/`Entries`, `Ledger` for the whole of it, `Commit` and
  `RecordDiff` for writing the shards, `Import` and `CommittedDigest` for
  reading them,
  `ClearView` for emptying this checkout's view while every entry stands,
  `Documents`, `AdoptDocuments` and `AdoptedDocuments` for document identity,
  and `SetPolicy` for who may record what), and `Adoption`, the document
  adoption the log carries.
- **Edits and decisions are two records.** `core/history` holds the block
  history `content.edit` operations project to; the ledger holds what was
  decided about a pairing ([C-03](c-03-context-store-and-graph.md)).
- **A transfer file carries the log, the shards carry a view.**
  `kapi context export` writes the project's operations, so it carries every
  `decision.record` whichever checkout recorded it
  ([C-03](c-03-context-store-and-graph.md)): a project whose branches answer one
  unit differently has decided both, and a backup built from one checkout's view
  would drop the rest. `Ledger` answers with the entry in force at every
  pairing, and `OpenLedger` reaches it with no checkout in hand. The `.kapi/`
  shards carry one checkout's view, because they are what that checkout
  evaluates from.
- **Approvals flow through one verb.** A `decide` operation through
  `kapi apply` records the unit state in the project store, addressed by the
  source document, the block and the edition `kapi status --review` lists, and
  bound to the revision the person read ([E-09](../engine/e-09-the-change-contract.md)).
  The desktop's approve action and the CLI verb share one path.
- **Coverage derives from the state store plus the target files**, never from
  content-memory properties.
- **Exchange and parcels carry state**, so a hand-off does not drop it.
- **Storage locations are independent of governance bindings.** The recipe
  binds governance by name; the shards' location is fixed by the project layout, and the
  database holding the ledger is fixed by the workspace
  ([C-03](c-03-context-store-and-graph.md)).

## See also

- [C-01: The project model](c-01-project-model.md): where the snapshot's shards
  sit among the ownership zones.
- [C-03: The context store and graph](c-03-context-store-and-graph.md): the
  database the ledger sits in, and the `blesses` edge.
- [C-09: Content memory](c-09-content-memory.md): the recycle corpus this store
  is not.
- [M-01: Bilingual Format Interop](../multilingual/m-01-bilingual-interop.md):
  the exchange that carries state across a hand-off.
- [Convergence](/kapi/convergence) and [the project store](/kapi/project-store):
  the end-user model derived from this state.
