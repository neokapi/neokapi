---
id: c-03-context-store-and-graph
sidebar_position: 3
title: "C-03: The context store and graph"
description: "Architecture decision: a project keeps two databases. The projection of its working tree stays in the checkout at .kapi/work/store.db; its authored context lives in a workspace outside every checkout, one database per project, beside the workspace's project registry and context graph."
keywords: [workspace, context store, store.db, context graph, graph_nodes, graph_edges, project registry, projection, backend, neokapi, architecture decision]
---

# C-03: The context store and graph

## Summary

A project keeps **two** databases, split by what produces the rows.

The **projection** stays in the checkout, at `.kapi/work/store.db`: the block
cache, the overlays a flow wrote, the extraction stamps. Everything in it is
derived from the working tree by long transactions that rewrite large parts of
it, so it belongs beside the tree it describes and a second checkout of the same
project keeps one of its own.

The **context store** lives in a **workspace**, outside every checkout: the
terms, the voice profiles, the content memory, the decision ledger and the
document adoptions ([C-04](c-04-unit-state-and-decisions.md)), the block
history of every recorded edit, and the project's settings: team
choices about the project that are neither content nor governance, such as the
saved filters a team shares (`projectdb.Setting`). It is authored rather than derived,
written a little at a time, and true wherever the project is checked out. Two
checkouts of one project, a second clone and a git worktree, share it, and each
keeps its own view of the one ledger.

The workspace also holds what spans projects: the **project registry**, the
**context graph**, whose node ids already carry the project they belong to, and
the **operation log**.

Every change to a project's terms, voice profiles, content memory and decision
ledger, every document adoption and every recorded edit, and every change to the
rules widened to the whole workspace, is an operation in that log carrying the
rows it wrote. Those stores are **projections** of the log:
`core/projector` is their only writer, and `kapi context rebuild` empties them
and replays the log into the same rows, starting from the latest checkpoint.

A question that reaches across the two files is one query. `projectdb.DB.Join`
opens the context store beside the projection on one read-only connection, so
*which blocks use this term, in which collection, at which coordinate, and which
of them are established?* is answered in one pass.

## Context

Context is relational. A term occurs in blocks; blocks belong to collections and
sit at a point in the context space ([C-02](c-02-coordinates-and-governance.md));
a state record approves a unit at a content hash; a memory entry recycles into a
block. Retrieval ([C-06](c-06-retrieval.md)) and governance
([C-02](c-02-coordinates-and-governance.md)) both traverse those relations rather
than reading one store in isolation.

Authored context must survive checkout deletion and remain available across
clones and worktrees. Parsed blocks, overlays and extraction stamps belong to a
specific working tree and can be rebuilt. Separate databases give these data
sets independent lifetimes and write locks.

Both local and hosted storage support queries across context relationships.
The local implementation provides these capabilities without a server.

## Decision

### The workspace

Every machine account has one implicit **default workspace**, created on first
use under the data root ([host.DataDir](#where-the-workspace-lives)) at
`<DataDir>/workspaces/default/`. A recipe carries no binding to it. A project is
keyed by `project.KapiProject.Identity()`: the stable `id:` the recipe carries,
or its `name:` where it carries none.

Consequences of that key, both intended:

- Two checkouts of a project with an `id:` reach one context store, wherever
  they sit and whatever they are called on disk.
- Two unrelated projects with no `id:` and the same `name:` would reach one
  store, so a project stating neither an id nor a name is keyed by the checkout
  it was found at instead. `kapi init` mints an `id:`, which is what makes the
  first case the normal one.

### A directory per workspace, a database per project

```text
<DataDir>/workspaces/default/
  workspace.db                  the project registry, the context graph,
                                the operation log
  projects/
    prj_9f2k…q7.db              one project's context store
    prj_4c8m…b1.db
```

A separate database per project preserves the terms, content-memory and voice
schemas without adding project columns to each table. It also limits write
contention and file corruption to one project and supports file-level export,
copying and deletion.

Cross-project graph queries use `workspace.db`, where node identities include
project scope ([Node identity carries the scope tuple](#node-identity-carries-the-scope-tuple)).
Queries for data outside the graph read the relevant project databases.

### Where the authoritative copy lives is a backend

`core/workspace.Backend` is where a workspace's authoritative copy lives:

| Method | What it answers |
| --- | --- |
| `Describe` | which backend this is, where the workspace is, whether it can be written |
| `Registry` | the workspace-wide database: registry, graph, operation log, widened rules, agent sessions |
| `Project` | one project's context store, created on first use |
| `Forget` | drop one project's context store |
| `Record` | append operations, minting an id for each that arrives without one; an id or content address the log holds is not written twice |
| `Since` | read operations back from a local arrival position |
| `Select` | read one project's operations, or one kind's, from a position |
| `PutBlob`, `Blob` | keep the bytes an operation names under the digest of those bytes, up to 64 MiB each |
| `Head` | the arrival position of the last operation, recorded here or merged in |
| `Close` | release every handle the backend owns |

`Registry` and `Project` answer with handles the backend owns: a caller reads
and writes through one and closes the backend, never a handle.

One adapter ships: `workspace.Local`, the directory of SQLite files above.
Every machine keeps its workspace locally; a project whose context is shared
exchanges operations with a remote, below, rather than reading through one.

`core/workspace/workspacetest.RunConformance` is the suite every adapter passes:
one table of behaviours, driven against whatever backend a factory hands back.
It states what the layers above are entitled to assume, in a form an adapter
author and a reviewer can both run.

### A project's context is shared through a remote

A recipe declares where a project's context is shared (`context.backend`:
`local`, `file`, `git` or `s3`), and a person can choose another on one machine
(`kapi context backend`, kept in the machine configuration under the project's
id). Every shared backend is a `workspace.Remote` holding the same layout:

| Path | Holds |
| --- | --- |
| `log/<writer>/<first-op-id>.jsonl` | the operations one workspace pushed, one JSON object a line, in id order |
| `blobs/<sha256>` | the payloads those operations name |
| `checkpoints/<op-id>.kpz` | the projections as of one operation (`kpz.KindCheckpoint`) |

`<writer>` is an id each workspace mints once (`Workspace.WriterID`), so a
machine only adds files under its own name, two machines never write one object,
and no lock is taken. A remote owes three things, stated by
`workspacetest.RunRemoteConformance`: list a directory, read an object, and
create an object that is not there (a second create of the same bytes is
nothing; of other bytes, `ErrObjectExists`).

| Adapter | Create-only write | Access |
| --- | --- | --- |
| `FileRemote` | a temporary file linked into place | the filesystem's |
| `GitRemote` | one commit per push on the ref, pushed without force; a push that loses a race fetches the new tip, lays its files over it and pushes again | the repository's remote |
| `host/s3remote` | `PutObject` with `If-None-Match: *` | the AWS credential chain |
| `MemoryRemote` | a map, packed into a transfer file | none |

`workspace.Sync` moves one project's operations. A **pull** lists `log/`, reads
the segments this workspace has not seen, merges their operations into the log
by id (`workspace.Merge`) and hands them to the projector: a catch-up when every
merged operation sorts after the ones applied, a rebuild in id order when one
sorts before. A **push** writes the project's operations the remote is not
known to hold as new segments, with the blobs they name, and a checkpoint once
the remote has gained `CheckpointEvery` operations since its last. An operation
names its blobs in its payload's top-level `blob` field and `blobs` list
(`workspace.BlobRefs`), so an edit's kept runs travel with it. A first pull
into a log that holds nothing of the project starts from the newest checkpoint
whose segment list covers every operation up to it, and still merges every
segment, so the history travels. The kinds `projector.LocalKinds` names never
leave the machine: the project's registration, its checkpoints, and rules a
person widened to the whole workspace.

What the workspace knows about a remote is kept in `workspace.db`, keyed by the
project and the remote: the operation ids the remote holds, the segments read
and whether they are merged, and the last contact. That is what the sync line
every context answer and `kapi status` carry is counted from (`N to push, M to
pull`), without reaching the remote. A remote that cannot be reached is
`ErrRemoteUnreachable`, and the CLI exits with status 5.

A **transfer file** is the same layout in one `.kpz` (`kpz.KindContext`):
`kapi context export` pushes the project into a `MemoryRemote` and packs it, and
`kapi context import <file>.kpz` unpacks one and pulls from it.

### No daemon

Every process opens the local databases directly: the CLI, an agent's MCP
server, the desktop. There is no broker in between, and nothing has to be
running for a project to be opened.

### Projects register on first use

Opening a project records it in `workspace.db`: its key, the display name the
recipe carries, the checkout paths it has been seen at on this machine, and when
it was last active. Re-opening replaces the display name (a `name:` is a label a
person edits), adds the checkout rather than replacing the list, and moves the
last-active time forward.

Checkout paths are hints, machine-local and normalized through
`host.NormalizeCheckoutPath`, so a directory reached through a symlinked parent
and the same directory reached directly are one entry. They are what a surface
listing projects offers to open; a checkout that has moved stays in the list
until the project is opened somewhere else.

### What is in which file

| Tables | Subsystem | Pool | Derived from |
| --- | --- | --- | --- |
| block cache, overlays | `core/blockstore` ([C-01](c-01-project-model.md)) | projection | the content files |
| `store_meta` | `core/projectdb` | projection | the last extraction |
| terms | `terms/` ([C-08](c-08-terms.md)) | context | the operation log, through `core/projector` |
| content memory | `memory/` ([C-09](c-09-content-memory.md)) | context | the operation log, through `core/projector` |
| voice profiles | `voice/` ([C-07](c-07-voice-profiles.md)) | context | the operation log, through `core/projector` |
| `projector_cursor` | `core/projector` | context | the position of the last operation applied |
| decision ledger, document adoptions | `core/state` ([C-04](c-04-unit-state-and-decisions.md)) | context | the operation log, through `core/projector` |
| one view of the ledger and of the documents read, per checkout | `core/state` | context | the checkout's files |
| block history | `core/history` | context | the operation log, through `core/projector` |
| `edition_head`, `edition_subject_head`: the workspace home | `core/workhome` | context | the operation log, through `core/projector` |
| `graph_nodes`, `graph_edges` | `host/storage/graph`, vocabulary in `core/contextgraph` | workspace | the rows above, plus the recipe |
| `workspace_projects`, `workspace_checkouts` | `core/workspace` | workspace | what has been opened |
| `workspace_ops` | `core/workspace` | workspace | its own log |
| `workspace_blobs` | `core/workspace` | workspace | the large payloads operations name |
| `workspace_rules` | `core/workspace` | workspace | the operation log: the rules a person widened ([C-11](c-11-context-operations.md)) |
| `workspace_agent_sessions` | `core/workspace` | workspace | which agents are at work ([S-03](../surfaces/s-03-agent-surfaces.md)) |

Each subsystem owns its own schema and its own migration ledger, so a subsystem
evolves without replaying anyone else's migrations, whichever pool it binds to.

`core/projectdb` opens both project pools and hands each subsystem its handle.
Callers name a capability rather than a file: `Blocks()` and
`BlocksAutocommit()` come from the projection, `Memory()`, `Terms()`, `Voice()`,
`Work()`, `History()`, `Heads()` and `Raw()` from the context store, and nothing
above has to know which is which. The table-by-table layout is in
[Note: Workspace storage](../../implementation/context/workspace-storage.md).

### Opening without a workspace

`projectdb.Open` with no workspace option puts the context tables beside the
projection in `.kapi/work/store.db`, which is the **embedded layout**. The
workspace location is an explicit option the host layer supplies
(`host/projectstore.go`, from `host.DataDir()`), and the framework holds no
default for it.

That is what keeps `core/` hermetic: a framework test opens a project in a
temporary directory and cannot reach a real workspace, because there is no path
inside the framework that names one.

### Joining across the two files

`projectdb.DB.Join` runs a function on one connection with both files visible:
the projection and the context store, the second attached under the schema name
`context`. The connection is read-only for the length of the call.

A join reads. A write spanning both pools is two transactions, and the pairing
that must be atomic, a decision and the wording the content memory learns from
it, is why the decision ledger and the content memory are in the same pool.

### Writes are ordered; reads are never gated

Every pool is opened with `storage.ProjectOptions()`: an immediate transaction,
an in-process FIFO permit, and a cross-process advisory lock on a file beside
the database. The first puts the wait where SQLite's busy handler applies, the
second orders writers that share a handle, and the third orders the several
processes that write one context store after it leaves the checkout.

The permit and the lock are per file, which is what the split buys back: a block
session's long transaction holds the projection's and leaves the context store's
alone, so a review loop recording decisions runs beside an extraction rather
than behind it. Reads take neither, because under WAL a reader neither blocks a
writer nor waits for one.

The settings, the measurements behind them and the deadlock a reentrant write
reports are in
[Note: Workspace storage](../../implementation/context/workspace-storage.md).

### Reading from a write-restricted sandbox

A check may run where it can read the workspace and not write it: a restricted
sandbox, a read-only mount, a workspace owned by another account. Under WAL that
counts as a write, because SQLite creates the write-ahead log's shared-memory
index beside the database on the first connection.

`storage.OpenReadOnly` answers it: the database is opened where it is with
writes disabled, and where SQLite refuses that, it and its log are copied to a
writable temporary directory that is deleted when the handle closes. The
workspace then reports itself read-only, answers reads, and refuses a write with
`workspace.ErrReadOnly` rather than losing it somewhere the caller cannot see.

### A synchronized folder is refused

A workspace inside a folder a desktop sync client keeps is refused at open, by
path pattern: iCloud Drive, Dropbox, OneDrive and Google Drive, matched a path
segment at a time. The message names the folder, the product and the way out,
which is to set `KAPI_DATA_DIR` to a directory outside it.

SQLite keeps a database, a write-ahead log and a shared-memory index consistent
with each other through byte-range locks the operating system enforces; a sync
client copies each of the three whenever it notices a change, takes no lock, and
replaces a file under an open handle when a second machine writes. The result is
a database whose log describes a state the main file is not in, reported as
corruption long after the copy, in a process that did nothing wrong. The cost of
the rule is a false positive on a directory that merely carries one of those
names.

### One pool is derived and one is authored

The **projection** is an index. Every row in it is a reading of the content
files, source and target, so deleting it costs a re-extraction and nothing else.

The **context store** holds the project's terms ([C-08](c-08-terms.md)), its
voice profiles ([C-07](c-07-voice-profiles.md)), its content memory
([C-09](c-09-content-memory.md)), its decision ledger and document adoptions
([C-04](c-04-unit-state-and-decisions.md)), its block history and the
editions the workspace home keeps, each a projection of the workspace's
operation log, described below. No read path opens a file in the
checkout to answer for any of them. A checkout may carry a terms bundle or a
voice profile a person authored under `.kapi/`; `kapi context import` is the one
command that reads them ([C-11](c-11-context-operations.md)). The store moves between
machines through a context backend or a transfer file, below.

Branches use the current context store even when they contain older snapshot
files. Governance is therefore independent of whether a team commits snapshots.

### The stores are projections of the log

A write to the terms, the content memory, the voice profiles or the widened
rules goes through `core/projector`, which does two things under one lock per
context store. It records an operation carrying the rows the write puts or
removes: `terms.write`, `memory.write`, `voice.write` or `rules.write`, with the
steps in the order the store calls made them. Then it applies every operation
the store has not yet seen, in the order the log received them, and moves the
store's cursor (`projector_cursor`) past them. A write another process made, or
an operation merged in from another machine, is applied by the next write, and
a projector opening a store applies whatever the log holds beyond the cursor.
`Workspace.Forget` records a `project.forget` operation when it removes a
project's context store, and a store that has applied nothing starts after the
project's latest one, so a project registered again begins with an empty
context. A rebuild starts there too.

Each step carries its rows with every timestamp the store would take from the
clock filled in from the operation's instant, and a write that would leave the
store as it is records nothing. An operation larger than 32 KiB keeps its steps
in a blob it names; a batch of entries or concepts is split so each blob stays
well inside the 64 MiB bound. A pass that writes many rows (an import, a
convergence run's absorbed record) collects them into one batch, recorded as one
operation per store.

`Projector.Rebuild`, behind `kapi context rebuild`, empties the projection tables
and the rules the project widened, and replays the project's operations in id
order. On a log one machine wrote, the rows it leaves equal the rows the writes
left; the voice store stamps an edit and the version it archives from the
clock, so those two columns are the exception. A run of single content-memory
writes is replayed in one transaction with the search indexes rebuilt once,
which keeps a dogfood-sized log (16,000 entries, 1,000 concepts) to about three
seconds.

The decision ledger records through the same projector: `state.WorkStore`
takes a journal (`projector.Decisions`), and each entry is a `decision.record`
operation addressed by the entry's own content address, so a decision recorded
twice is one operation ([C-04](c-04-unit-state-and-decisions.md#the-ledger-is-a-projection-of-the-operation-log)).
Each document a checkout resolves is a `document.adopt` operation carrying its
key, its path and what it held, which every other checkout of the project
consults before minting a key of its own
([C-04](c-04-unit-state-and-decisions.md#document-keys-are-recorded-in-the-log)).
Each checkout's view of the ledger, and its own list of the documents it has
read, stay readings of its files.

A **checkpoint** keeps a rebuild short. `Projector.Checkpoint`, behind
`kapi context rebuild --checkpoint`, writes every projection table as it stands
(rows, their rowids and the AUTOINCREMENT numbering), with the project's widened
rules, into a `.kpz` of kind `kapi-checkpoint`, stores it as a blob and records
a `checkpoint.write` operation naming it and the last operation it includes. A
table whose rows run past 1 MiB, the block history above all, travels in parts
instead: blobs of at most 16 MiB the checkpoint names (`kpz.CheckpointPart`),
so no file a checkpoint is made of grows with the project's history. A push
writes the parts to the remote's `blobs/` before the checkpoint, and a first
pull fetches them before it installs the checkpoint. A
rebuild loads the newest checkpoint that still stands and replays only the
operations after it. A checkpoint stops standing when the log receives, after
it was taken, an operation whose id sorts before its last one, which is what a
merge of an older operation from another machine does; the rebuild then falls
back to an earlier checkpoint or to the whole log. A checkpoint that cannot be
loaded, a part of it missing, costs a replay of the whole log, and the rebuild
reports it.

A log written before an operation kind was retired still holds operations of
that kind, because the project resets data rather than migrating it. A rebuild
leaves them out and counts them (`RebuildReport.Retired`, from
`projector.RetiredKinds`), and `kapi context rebuild` says how many it left
out: `unit.record`, the ledger's entries before they were `decision.record`,
is one, and `kapi context import` reads the decisions in again from the shards.

Callers reach the stores through the projector: `App.Projector` in host hands
out `projector.Terms`, `projector.Memory` and `projector.Voice`, which answer
reads from the projection and record every write. Code that only reads takes a
view (`projector.TermsView`, `MemoryView`, `VoiceView`) whose writes are
refused. `make check-projection-writes` type-checks the Apache modules and
fails on a store write, or a store handed to an interface that can write it,
anywhere outside the projector, `history.Store.Put` and the workspace home's
`workhome.Store.Apply` and `Replace` among them. A write method
reached through a type that
embeds the store counts as a direct call, so a method a projector store leaves
to its embedded store is caught too. The guard also reads the store packages:
an exported method that writes the database must be in its list of writes or
in its list of writes the log does not project (a search-index rebuild, the
workspace's project registry). The few functions that open a store a person
named on the command line are listed with the reason.

### Edits are recorded as content.edit {#edits-are-recorded-as-content-edit}

Text lives in its home: the file in a checkout. What happened to it lives in
the log. An applied edit is a `content.edit` operation per document it
changed, and the projector writes it into the **block history**
(`core/history`, the `block_history` table): one row per edition the edit
changed, with the edition revisions before and after, the basis a derived
edition was made from, the block's key, content hash and context hash, the
kinds of the operations that changed it and, for a flow, the tool that changed
it, who made the change (person, agent or tool, with a name and a session, or
`external` for an edit made outside kapi) and through which surface (`apply`,
`ksed`, `mcp`, `browser`, `desktop`, `flow:<name>`, `merge`, `pull`,
`observed`). `history.Store.Wrote` answers who wrote the content an edition
holds, for every writer: the change that left the edition at that revision,
a recorded write before an observed one. `LastWrite` and `Latest` read the
most recent change to one edition or to every edition of a document (or to
the editions `Latest` is given) in one pass over the document's history, and
`Edition` and `Document` read the changes back, most recent first. A read of
the change service that shows at most a page of blocks asks `LastWrite` for
each edition it shows, so its cost follows what it shows; a longer read asks
`Latest` once for the editions its blocks hold. Either way a read asks
`Wrote` only for an edition whose most recent change did not leave the
revision it holds. The file stays the only copy of its text and the log keeps
facts about it, keyed by revisions that hold on every branch where the content
matches, so a branch switch moves no record: the history is shared by every
branch of a checkout, and the change that wrote what a branch holds answers
for it whatever was recorded on another branch since.

`core/change` defines the hook an applier of change sets calls once the homes
committed (`change.Recorder`), and `host.App.EditRecorder` is the project's
recorder: it records every document of a change set through one
`Projector.RecordEdits` call. Every flow the host runs records each document it
writes through the same recorder, as one `content.edit` with the actor
`tool:<flow>` ([E-09](../engine/e-09-the-change-contract.md#flows)); its
transitions carry each translation's basis, the kind of operation the run's
change to the edition comes to (as `change.Diff` would send it), the tool that
made it (the tool's stamp, or the run's only tool, the counter a convergence
pass adds left out), and, as the row's producer,
the stamp the producing tool left (provider, model, and the governing context
fingerprint), which a file of strings has nowhere to keep. The block history is
therefore where the loop's basis lives: coverage grades an undecided
translation by the tool's write that left it, a decision on such a translation
starts from that write, the review context names the producer of a translation
a flow wrote, and the staleness gate reads the producer from the latest write
([C-05](c-05-freshness.md)). The basis is the source the run read before it
ran, so a source edited while the run worked reads as drift. A run that
reproduces a person's or an agent's wording records nothing over their write.
`kapi pull` records a venue's translation with a basis only where the venue's
record says it was made from the source the checkout holds
(`host.WithStatedBases`); any other pulled translation is made from wording
this checkout does not hold, and is recorded with none.
Kapi Desktop, `kapi apply`, ksed, the MCP edit tools and the browser engine
edit content through the change service, so their edits reach the same
recorder under their own origins. The decision ledger holds decisions only
([C-04](c-04-unit-state-and-decisions.md#the-ledger-is-a-projection-of-the-operation-log)):
what a producer made, and from which source, is in the block history and
nowhere else.

The block history is part of the workspace log, so a checkout of a project
that shares its context through a backend reads it with `kapi context pull`,
and a fresh clone holds the translations the repository carries and none of the
record of how they were written. Until it pulls, a translation whose source was
edited before the clone's first pass is not graded stale there. `kapi up` and
`kapi status` say so in one line (`host.HistoryNotPulledNote`) when the project
names a backend, translations exist on disk and the history records nothing. A
push to a venue carries the same record for the documents it reads
([S-07](../surfaces/s-07-context-centric-review.md#a-push-carries-decisions-the-venue-decides)).

An edit made outside kapi records nothing when it happens. The next read
through the change service finds it: the service shows each block it reads to
its observer (`change.Observer`), and the host's (`host/changes_observe.go`)
compares every edition with the changes the history records for it. An
edition whose revision no recorded change left was changed by something that
recorded nothing, and the read records it as one hash-only `content.edit` for
the document, with the actor `external` and the origin `observed`, starting
from the revision the history last recorded. A revision an earlier change left
is explained, so a checkout of another branch records nothing. The read asks
when it ends rather than when it starts (`history.Store.Reached`), so a writer
in another process that recorded its own change while the read ran explains
it; and an observed change that lands after such a writer's record never takes
the revision from it. An edition the history has never recorded is left alone.
A history shows an observed change with no actor, since nobody knows who made
it, and it carries no basis, so the translation reads as made from no recorded
source. A flow's read of the document it has just committed is left
unobserved (`change.Unobserved`), because the flow records that change itself,
and so is every read of a run that prints its change set instead of writing
it.

A person's or an agent's edit keeps the runs around each change and the change
set as sent, in blobs the operation names; a tool's edit keeps the revisions
and hashes only, because the file holds the text and a flow writes thousands of
them. A write to the workspace home keeps the edition it leaves, whoever made
it, because the log is that home (below). A project that declares redaction
([C-10](c-10-redaction.md)) keeps no withheld value in a record: the recorder
redacts the runs it keeps and the note with the project's rules, the originals
going to the project vault, and leaves the change set out. The workspace home
redacts what it records the same way, and puts the originals back from the
vault where it reads an edition, so a kept draft reads whole on the machine
that withheld its values and with placeholders anywhere else. A policy that
detects entities needs a read's entity annotations, which a record's runs do
not carry, so under one a record keeps revisions and hashes only, and the
workspace home keeps no edition at all: a change to a translation with no file
is refused, and a gated run leaves its parked drafts in the producer's cache.

An operation is addressed by the document, the actor and each transition with
the address of the operation it extends, the one that most recently left the
edition at the revision the transition starts from (`history.Store.Reached`,
which looks up only those revisions). Every log holding an operation agrees on
its address, so one edit recorded twice, by a retry or by two machines noticing
one change made outside kapi, is one operation, and so is every change that
extends it, while the same change made again after an undo is another. The
history keeps each operation's address beside its rows (`block_history_op`):
when two logs that recorded one edit under different ids merge, the log keeps
the older id, and its rows take the place of the ones the newer id projected,
as a rebuild writes them.

The identity evidence on every transition is what lets history re-attach after
a reorder. A read reports the keys the format gives, and reconciliation against
the history's priors (`history.Store.Priors`, `reconcile.Blocks`) finds a
recorded block among siblings whose positions moved.

### The workspace home keeps the editions that have no file {#the-workspace-home}

An edition with no file has the log as its home. Under
`defaults.materialize: on-converge` a locale's files appear only when it clears
its ship gate, so a parked locale's drafts, the edits a person or an agent makes
to them, and the drafts a gated run produced that `kapi merge` has not
delivered all live in the **workspace home** (`core/workhome`), outside the
checkout, until a delivery writes them to the files the recipe names.

Each write to such an edition is one `content.edit` that carries the result:
the runs, status and origin of each block's edition it leaves, the basis it was
made from, the block's identity signals, and the stamp its producer serves a
draft again by. Its block history rows name the kinds of the operations that
made each change and the tool, as a write to a file's do: the kinds a change
set sent, the kind a flow's draft comes to (`set_content`, or `replace_text`
for text moved around the same codes) with the tool the flow names, and
`remove_edition` for a removal or a delivery's release. The operation names its **subject**, the document's key and the
edition (`workspace.Op.Subject`, indexed), and the writer appends it with a
**conditional record** (`workspace.Backend.RecordIf`): inside the IMMEDIATE
transaction that appends, the subject's last local position must still be the
one the writer read, or nothing is appended. The projector's lock is per
process, so this check is what makes two processes writing one edition (Kapi
Desktop and an agent's MCP server) take turns. The head a writer expects is read
from the log (`SubjectHead`), never from a projection, because a position is
local to one log. A head that moves after a change set settled, and before its
commit, refuses the change set `doc_changed` with nothing landed.

The projector folds each subject's writes into two tables of the context
store: `edition_head`, one row per block of each kept edition (revision, runs
inline up to 16 KiB and in their blob above, status, origin, basis, stamp), and
`edition_subject_head`, one row per subject (the operation its head is at, the
latest operation folded, and the writes that did not advance it). The fold reads
a subject's writes in id order. A write advances the head when it was staged on
the head it finds there (its base); otherwise it is **divergent** and changes
nothing. Two machines that wrote one edition from one head therefore reach the
same head after their logs merge, whichever log received which write first: a
write that arrives with an id before the latest one folded makes the projector
fold the subject again from every write the log holds for it. A pull then
**rebases** (`Projector.RebaseWorkspace`, called by the projector's syncer
after it applies what a pull merged): a divergent write each of whose blocks
still holds the revision it started from, the common case for writes to
different blocks, is applied onto the head as a new `content.edit` naming it as
its cause, addressed so that two machines rebasing one write onto one head make
one operation. A divergent write that changed a block the head has moved since
is a **conflict**, which `kapi status` names. A person's or an agent's write
that advances the head settles the conflict for every block it writes, since it
was made with the conflict in view, and a rebase carries over only the blocks a
conflict still lists. A delivery's release of the whole edition settles every
conflict on it.

The recipe picks an edition's home. The file home reaches the workspace home
through a keeper (`filehome.Keeper`): under `on-converge` a translation whose
file does not exist lives in the workspace home whether or not the workspace
holds anything of it yet, so a change set never delivers a file the ship gate
withholds; under `manual` it is written to its file, unless the workspace still
keeps a draft of it. Its reads and change sets go through the keeper with
`if_match` like any edition ([E-09](../engine/e-09-the-change-contract.md#homes)).
Once the edition's file exists, the file is its home and the workspace's copy
is never read or written there, and a gated run keeps no draft of a
translation whose file exists, so the workspace never becomes a second copy of
file-backed content.

Every delivery (`kapi merge`, or `kapi up` for a locale that cleared its gate)
writes the file from what the workspace keeps, with the basis each draft was
made from, and then releases from the workspace home what it read there, a
`content.edit` that removes each block. The release expects the head the
delivery read, so an edit that lands in between is delivered by reading again
rather than released unread. A run that delivers a locale first replaces the
drafts the workspace kept from an earlier run with its own, so what it writes
on top of the run's drafts is the wording a person or an agent gave the
locale, never a draft of a source that has changed since.

A translation's file can also appear by another path: a person writes it,
`kapi pull` or a checkout brings it, or a recipe switched to `manual` has its
pass write it from the flow's drafts. The end of every run **settles** such an
edition: the workspace releases each block the file already holds and each
draft a tool made, which the file supersedes, and keeps the wording a person or
an agent wrote that the file does not hold. `kapi status` lists that wording
as a conflict, and `kapi merge` writes it into the file.

### Deleting derived data {#kapiwork-is-free-to-delete}

The parse cache, extraction batches, collection overlays and `store.db` can be
rebuilt from content files. A parked locale's drafts are kept in the workspace
home, so deleting `.kapi/work/` loses none of them: the next `kapi up` writes the
overlays a producer serves them from back into the block store from the stamps
the workspace home keeps, and the pass calls no provider for them. A draft a
person or an agent has edited since is restored from the latest draft a
producer wrote for it, which the log keeps, so the producer serves that draft
and the edit stays kept. The redaction vault at `.kapi/work/vault/` is an
exception: it contains withheld originals that are neither committed nor sent
to a service. Deleting it loses those values, in kept drafts too
([C-10](c-10-redaction.md)).

The context store has a separate lifetime in the workspace. Deleting
`<DataDir>/workspaces/` removes the terms, voice profiles, content memory,
decisions, block history and kept drafts of every local project that no
backend holds. Push each project to
its backend, or write it to a transfer file with `kapi context export`, before
deleting the workspace.

### Locales are keyed canonically

Every subsystem keys its rows by the canonical BCP-47 locale beside the text:
the content memory's variants ([C-09](c-09-content-memory.md)), the terms
store's terms ([C-08](c-08-terms.md)), the block cache's `targets/<locale>`
overlays and its `block_texts` rows. The stores apply `locale.Normalize` on
every write, read and list, so a producer writing `targets/nb_NO` and a reader
asking for `targets/nb-NO` address one overlay. Rows a store wrote before it
normalized locales are keyed by whatever spelling the recipe used then, and no
lookup finds them again; `projectdb.NonCanonicalLocales` reports them across both
pools with the pool each row is in, and `kapi status` and `kapi up` print the
report once with the remedy for that pool named.

The two pools take different remedies, because a row in one is derived and a
row in the other is authored. The projection is a reading of the working tree,
so deleting the database and running `kapi up` derives every row in it again.
The context store is a projection of the operation log, and the stores
normalize as they write, so `kapi context rebuild` writes its rows again under
the canonical spelling.

### Presence is table-level

An empty subsystem inside an existing database behaves exactly as an absent
store does. A database's existence is not a signal, so nothing has to guard
against a file being there: the terminology gate enforces nothing on a project
whose terms tables are empty ([C-08](c-08-terms.md)), `kapi up --plan` never
creates state, and `kapi pack`
carries only non-empty parts
([M-06](../multilingual/m-06-content-packages.md)). `projectdb.DB.DetectStoreDrift`
reads "store missing" as *the block cache holds no blocks*, not as *the file is
absent*.

### The graph relates the subsystems

`graph_nodes` and `graph_edges` are a property graph: labelled nodes and
labelled, optionally time-bounded edges, both carrying JSON properties.
`core/contextgraph` owns the vocabulary: the labels, the scope tuple, the id
scheme, and the node and edge constructors every writer calls.

Node labels:

| Label | What it is | Scope |
| --- | --- | --- |
| `block` | a unit of source content, keyed by its content key | instance |
| `collection` | a content collection, keyed by its label | instance |
| `unit_state` | one unit's state in one document, in one locale variant | instance |
| `concept` | a terminology concept | vocabulary |
| `coordinate` | a point on the structural axes, a `(profile, channel)` pair | vocabulary |

The coordinate node is the structural pair only. A collection's declared axes
(brand, mode, and whatever else a recipe names under `coordinates:`) travel on
its context entry over the wire and are folded into that entry's hash
([C-02](c-02-coordinates-and-governance.md)); they are not properties of the
coordinate node, and the graph's coordinate queries take the profile and the
channel.

Edge labels:

| Label | Relates | Carries |
| --- | --- | --- |
| `uses_term` | block → concept | the term used, its status, the locale, the document, a use count, and the term's own validity window |
| `in_collection` | block → collection | membership |
| `governed_by` | collection → coordinate | the governing profile's validity window |
| `blesses` | unit state → block | the pairing the decision was written against: the translation and the source basis, by revision where the decision carries them and by hash |

`host.MaterializeContextGraphInDB` writes all four on the convergence path,
after extraction commits its block-write transaction. The subgraph is a pure
projection: each pass clears what it is entitled to, keyed by its own scope
tuple, and rebuilds it, so several projects' subgraphs coexist in one
`workspace.db` without a pass reaching another project's rows. Occurrence edges
come from a term search over the block cache (`core/occurrence`), where repeated
uses of one term in one block fold into a `count` property rather than into
separate edges, and the term and the locale are the edge discriminators;
`governed_by` comes from resolving each named collection's governance; `blesses`
joins the unit decision ledger against the block cache, so a record whose block
no longer exists keeps its node and loses its edge. A unit state names its
document by the durable key the ledger's view records
([C-04](c-04-unit-state-and-decisions.md)); the graph writer turns that key back
into the path the block cache files blocks under before joining.

The term is a discriminator because a concept holds several spellings and a
project reaches for one of them: a block still saying the deprecated word is a
different finding from one saying the preferred word, and folding both onto one
edge would leave the graph with a status to choose between. So the edge records
the status of the term it names, and carries that term's own window, which is
what makes *is this discouraged* answerable as *is it discouraged here*. The
standing is a property of the concept at a coordinate, never of the word: the
same block answers differently before and after a deprecation date, and inside
the market a deprecation reaches versus outside it.

The `governed_by` edge is where governance stops being re-derived. It carries the
same half-open validity window the recipe declares
([C-02](c-02-coordinates-and-governance.md)), so *what governed here on that
date* is answered by the graph under the same temporal model, not by a second
implementation of the ladder.

### Node identity carries the scope tuple

An id is `<label>:<workspace>/<project>/<stream>:<local>`, with the separators
percent-escaped inside each component so two different tuples can never render to
one id. The scope segment is always three fields, so it says which dimensions the
node is qualified by rather than leaving that to be inferred from the label.

Two kinds of node, and the split decides what can be asked:

- **Vocabulary nodes** (concepts, coordinates) drop the instance dimensions.
  One concept is one node however many projects use it, which is what makes
  *which projects use this concept* a two-hop traversal instead of a join across
  project boundaries. A coordinate is vocabulary for the same reason: a set of
  projects binds to one coordinate vocabulary rather than each inventing its own.
- **Instance nodes** (blocks, collections, unit states) carry `(project,
  stream)`. Two projects' `docs` collections are two nodes. Two projects holding
  identical wording hold two block nodes carrying the same content key, and *same
  wording* is a content-key equality query: one shared node would say the two
  instances are governed together, and an instance sits somewhere.

This is what lets one `workspace.db` hold every project's subgraph: the project
dimension is in the id, so the pass that rebuilds one project's rows names them
and reaches nothing else.

**Dimensions are fields, not containment edges.** Every node carries the
non-empty components of its scope tuple as properties as well as in its id, so
slicing a view by project or stream is a filter rather than a traversal. An empty
dimension is written as an *absent* key rather than an empty string, because a
property filter compares against a value and absence is not the empty string.
Locally the workspace dimension is absent, which is correct: the implicit
workspace is this machine account's and has no name to carry. There is no
project→project edge and there will not be one: projects relate by co-occurrence
through the vocabulary they share.

Within a scope, identity is **durable**: a block is its content key
([F-03](../foundations/f-03-identity.md)), and a unit state is its document,
unit and variant, not a reader's positional id, so a re-parse that renumbers a
document rewrites the same rows rather than orphaning them. The document is part
of a unit state's identity for the reason
[C-04](c-04-unit-state-and-decisions.md) gives: a unit id is unique inside its
document and nowhere wider, so without it two pages of one collection are one
node and the decision written last answers for both.

Changing a scope value changes the id, so **a rename is a deterministic
re-key**. That is safe because a writer clears the scope it is about to write in
the same pass: no row survives under the old key. Where the project dimension is
the recipe's `id:`, a rename does not touch it and the display name rides as the
`project_name` property.

### Finding a term inside blocks

Term occurrence has to search text that lives inside a JSON payload no SQL can
read, so the block cache keeps `block_texts`: a row per block per locale (the
source under the empty locale, each target under its own) with a contentless
FTS5 trigram index over it. Both live in the projection, beside the blocks they
describe.

The index is built **before a search, not during a write**. Maintaining it inside
every block write costs roughly seven times the write: extraction writes every
block in the project, which is too much to levy for a query that may never be
asked. A write only marks the block stale, and the first search reconciles
exactly what changed. The index selects candidate matches; Go code verifies them. The browser build
uses the same matcher with a scan instead of an index.

`kapi terms occurrences` is the surface that lists uses live from this index,
with their positions. The usage count is a different reading: `kapi context
search`, the `context_search` tool, the desktop explorer's search and relations
panes and the platform's concept page all report how often a term is used, and
all of them read it from the `uses_term` edges through `contextgraph.UsesByProject`
rather than joining the terms store against the block cache when asked. One
producer means one number wherever the question is asked, and because that
producer is extraction, the number is as of the last extraction rather than of
the working tree. Each surface says so beside the count. The passage a use sits
in is read from the block cache by the content key the edge names, so a block
the cache no longer holds costs the snippet and never the count.

### The query shapes are written once

A store spanning many projects answers the same questions with the dimensions
free; a local project answers them with the dimensions pinned to one value. That
is enforced rather than described. The queries live in `core/contextgraph/query.go`
against two narrow read interfaces, `EdgeReader` and `NodeFinder`, that both
backing stores satisfy, so there is one implementation rather than two agreeing
by convention:

| Query | Question |
| --- | --- |
| `Uses` | term → blocks → collection, by traversal |
| `ProjectsUsingConcept` | which projects use this concept |
| `UsesByProject` | how much of it sits in each, in which words, and how it stands there at this point |
| `CollectionsAtCoordinate` | what is governed at this point, at this instant |
| `BlessingsOfBlock` | which decision covers this unit, at which basis |
| `BlocksWithContentKey` | who else holds this same wording |

`core/contextgraph/graphtest` is the shared query-shape suite: one fixture of two
projects sharing a concept, one table of expected answers, run against every
store that claims to hold this vocabulary. A query added for a wider scope is
expressible at a narrower one, and a local query does not have to be reinvented
when the scope widens.

Scope filtering is one predicate, `Scope.Contains`, treating an empty dimension
as *any*, which is how the same call serves both a project-scoped surface and a
rollup across projects.

### Browser and wasm

The browser build runs the same stores. `core/storage` has a driver for every
build, and the browser's (`core/storage/driver_js.go`) runs the SQL on the
official SQLite WebAssembly build, reached through a bridge the page installs
before the engine starts. The workspace, its operation log, the projector, the
content memory, the terms store, the voice store, the decision ledger, the
context graph and the block cache are the native code there, a lab's writes
are operations in the log, and the projector is their one writer. The workspace
sits under the engine's data root, `/.kapi-data`.

The driver declares what it gives a store in a profile
(`storage.DriverProfile`): one connection per file, no WAL, no lock another
process honours, and no durability, since the databases live in the module's
memory and nothing outlives the tab. Two rules follow for every store, natively
too. No correctness rule depends on a reader running beside a writer, and no
code holds a transaction or open rows on a pool and then waits for a second
session on the same pool; `make test-stores-oneconn` runs the store suites
natively with every pool held to one connection and WAL off to keep it so. A
database file belongs to its driver, so code asks `storage.Exists`,
`storage.Remove`, `storage.Rename` and `storage.List` about one rather than the
file system. A SQLite file the page's file system holds is read into memory the
first time a store opens its path. FTS5
word search uses `unicode61` there, the module having no ICU tokenizer.
`make test-wasm-stores` runs the store suites under `GOOS=js` over the same
driver. The [WASM Engine ABI](../../implementation/surfaces/wasm-engine-abi.md)
note has the mechanics.

### Where the workspace lives

The data root is `host.DataDir()`, resolved in order:

```text
$KAPI_DATA_DIR            the whole path, used as given
$XDG_DATA_HOME/kapi       on every platform, when the variable is set
macOS                     ~/Library/Application Support/kapi
Linux and the rest        ~/.local/share/kapi
Windows                   %LocalAppData%\kapi
```

Inside a `go test` binary the resolution is different: unless `$KAPI_DATA_DIR`
names a root outright, the answer is a directory of that process's own under the
system temporary directory. A test that opened a project would otherwise write
into the developer's own context and read it back on the next run. The first
resolution in a test binary removes the roots of test binaries that are no
longer running. A root is named by its process id, and on macOS, where ids wrap
at 99999, a root last written before the process now holding its id started
is removed as well. A package whose `TestMain` runs through
`devenvtest.Main(m, host.RemoveTestDataDir)` removes its own root when its tests
finish, so test runs do not accumulate them. A released
binary is not a test binary, which is why every in-repo surface that launches one
sets `$KAPI_DATA_DIR` as part of the isolation contract.

## Consequences

- **Clones share project context.** A second
  clone of a project with an `id:` opens with its terms, its voice profiles, its
  content memory and its decisions already in force, and re-extracts its own
  projection.
- **Worktrees share the decision ledger.** Decisions recorded in
  one are in force in the other, and switching branches moves no authored
  context.
- **Cross-subsystem questions are one query within a pool, and one join across
  them.** Term coverage per collection, the blocks behind a coordinate, the
  units a term change puts at risk.
- **One transaction still covers a decision and the wording it approves.** Both
  are in the context pool, which is why they are in the same file.
- **Store paths are not a user surface.** The recipe binds what governs a point
  by name and names no database or context file, and it carries no workspace
  binding. Standalone stores outside a project keep
  their own selectors (`--termstore`, `--memory`); those address a file the user
  owns, which is a different thing.
- **CI caches `.kapi/work/cache/docs`, and nothing else under `work/`.** The
  parse cache is keyed by content, configuration and build, so a restored entry
  changes no result, and `setup-kapi` carries it by default. The remaining
  entries under `cache/` belong to the checkout that wrote them
  ([Convergence in CI](/kapi/convergence-in-ci)).
- **A machine's context is one directory.** Backing up
  `<DataDir>/workspaces/default/` backs up every project's authored context, and
  `kapi context export` writes one project's as one file. Deleting
  it costs every project's terms, voice profiles, content memory, decisions
  and block history.

## See also

- [C-01: The project model](c-01-project-model.md): the layout the projection
  sits in.
- [C-04: Unit state and the decision record](c-04-unit-state-and-decisions.md):
  the decision ledger inside the context store.
- [C-08: Terms](c-08-terms.md) and [C-09: Content memory](c-09-content-memory.md):
  the two subsystems whose source-versus-projection split this store
  implements.
- [C-06: Context retrieval](c-06-retrieval.md): the primitives these tables
  answer.
- [C-11: Context operations](c-11-context-operations.md): the operation log and
  the widened rules the workspace holds.
- [Note: Workspace storage](../../implementation/context/workspace-storage.md):
  the files, the tables, the write settings and the contention measurements.
- [The project store](/kapi/project-store): the end-user view.
