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
terms, the voice profiles, the content memory, and the unit decision ledger
([C-04](c-04-unit-state-and-decisions.md)). It is authored rather than derived,
written a little at a time, and true wherever the project is checked out. Two
checkouts of one project, a second clone and a git worktree, share it, and each
keeps its own view of the one ledger.

The workspace also holds what spans projects: the **project registry** and the
**context graph**, whose node ids already carry the project they belong to.

A question that reaches across the two files is one query. `projectdb.DB.Join`
opens the context store beside the projection on one read-only connection, so
*which blocks use this term, in which collection, at which coordinate, and which
of them are signed off?* is answered in one pass.

## Context

Context is relational. A term occurs in blocks; blocks belong to collections and
sit at a point in the context space ([C-02](c-02-coordinates-and-governance.md));
a state record blesses a unit at a content hash; a memory entry recycles into a
block. Retrieval ([C-06](c-06-retrieval.md)) and governance
([C-02](c-02-coordinates-and-governance.md)) both traverse those relations rather
than reading one store in isolation.

Context is also durable in a way a parse is not. A voice profile, a term, an
approved wording and a recorded decision are what a person put there. A parsed
block, an overlay and a stamp are what the last run computed, and the next run
computes them again. Keeping both in one file inside the checkout made the
durable half inherit the disposable half's lifetime: a second clone started with
no memory of what had been approved, a git worktree recorded decisions the main
checkout never saw, and `rm -rf .kapi/work` was a sentence with two very
different halves.

A hosted layer answers such questions over one database spanning many projects.
If a local project could not answer them at all, the two halves would differ in
what they can be *asked*, not merely in scale, and the framework's standing
constraint is that kapi runs on its own.

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

The alternative was one database per workspace with a project column on every
context table. A directory won, for four reasons:

- The `terms/`, `memory/` and `voice/` schemas move out of the checkout
  unchanged. A project column on every table would have meant a scoping change
  inside three subsystems, each with its own migration ledger and its own
  hosted counterpart to stay in step with.
- Write contention is bounded to the project that caused it. SQLite locks a
  file, so a converge run teaching one project's content memory would otherwise
  queue every other project's writers behind it.
- So is corruption. A truncated write costs the project that was being written
  rather than every project on the machine.
- Exporting a project, copying one, or deleting one is a file operation.

What the split costs is a cross-project query, which is exactly what the
workspace database absorbs: the graph's node ids carry the project dimension
already ([Node identity carries the scope tuple](#node-identity-carries-the-scope-tuple)),
so *which projects use this concept* is a traversal there. A question the graph
does not hold is a fan-out over the project databases, which is a loop over
files a directory listing gives.

### Where the authoritative copy lives is a backend

`core/workspace.Backend` is where a workspace's authoritative copy lives:

| Method | What it answers |
| --- | --- |
| `Describe` | which backend this is, where the workspace is, whether it can be written |
| `Registry` | the workspace-wide database: registry, graph, operation log, widened rules, agent sessions |
| `Project` | one project's context store, created on first use |
| `Forget` | drop one project's context store |
| `Record` | append operations, assigning each a sequence number |
| `Since` | read operations back from a position |
| `Head` | the position the log stands at |
| `Close` | release every handle the backend owns |

`Registry` and `Project` answer with handles the backend owns: a caller reads
and writes through one and closes the backend, never a handle.

One adapter ships: `workspace.Local`, the directory of SQLite files above. The
interface is drawn so an adapter keeping the authoritative copy elsewhere fits
behind it without the callers changing. `Registry` and `Project` hand back
handles the adapter has materialized locally, which a remote adapter would
hydrate; `Record` and `Since` carry the operations such an adapter would
exchange with its authority, and which a background synchronizer would read to
learn what this machine did offline. Neither is built.

`core/workspace/workspacetest.RunConformance` is the suite every adapter passes:
one table of behaviours, driven against whatever backend a factory hands back.
It states what the layers above are entitled to assume, in a form an adapter
author and a reviewer can both run.

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
| terms | `terms/` ([C-08](c-08-terms.md)) | context | the committed terms source |
| content memory | `memory/` ([C-09](c-09-content-memory.md)) | context | the committed targets plus the `.memory.json` seeds |
| voice profiles | `voice/` ([C-07](c-07-voice-profiles.md)) | context | the committed `voice.yaml` files |
| unit decision ledger, and one view per checkout | `core/state` ([C-04](c-04-unit-state-and-decisions.md)) | context | the committed `.kapi/state/*.jsonl` shards, plus what each checkout has recorded since |
| `graph_nodes`, `graph_edges` | `host/storage/graph`, vocabulary in `core/contextgraph` | workspace | the rows above, plus the recipe |
| `workspace_projects`, `workspace_checkouts` | `core/workspace` | workspace | what has been opened |
| `workspace_ops` | `core/workspace` | workspace | its own log |
| `workspace_rules` | `core/workspace` | workspace | the rules a person widened ([C-11](c-11-context-operations.md)) |
| `workspace_agent_sessions` | `core/workspace` | workspace | which agents are at work ([S-03](../surfaces/s-03-agent-surfaces.md)) |

Each subsystem owns its own schema and its own migration ledger, so a subsystem
evolves without replaying anyone else's migrations, whichever pool it binds to.

`core/projectdb` opens both project pools and hands each subsystem its handle.
Callers name a capability rather than a file: `Blocks()` and
`BlocksAutocommit()` come from the projection, `Memory()`, `Terms()`, `Voice()`,
`Work()` and `Raw()` from the context store, and nothing above has to know which
is which. The table-by-table layout is in
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

### Adoption

The first open of a project WITH a workspace, where the context store is new and
the projection still carries context tables, carries the project across.

Only what no committed source reproduces moves. Everything else in those tables
is a projection of a file under `.kapi/`, and the next command derives it again
into the context store. Between a decision being recorded and `kapi commit`
writing it to `.kapi/state/`, the ledger holds its only copy, so those records
are read out of the old store and recorded in the new one first, and nothing is
dropped unless that succeeded.

What is dropped afterwards is computed rather than listed: an empty projection
is built in memory, its tables are what a projection is entitled to hold, and
every other table in the file belonged to a subsystem that has moved out. A list
would be a second copy of four subsystems' schemas with nothing keeping it
current.

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

A sync client and a SQLite database in WAL mode disagree about what a file is.
SQLite keeps a database, a write-ahead log and a shared-memory index consistent
with each other through byte-range locks the operating system enforces; a sync
client copies each of the three whenever it notices a change, takes no lock, and
replaces a file under an open handle when a second machine writes. The result is
a database whose log describes a state the main file is not in, reported as
corruption long after the copy, in a process that did nothing wrong. The cost of
the rule is a false positive on a directory that merely carries one of those
names.

### The committed sources are the truth

Both pools are **indexes**, and every row in them is reconstructible from:

- `.kapi/terms.json`, the terms source, bound by `defaults.terms_source`;
- `.kapi/memory/*.memory.json`, the content-memory seeds;
- `.kapi/voice.yaml` and `.kapi/profiles/*/voice.yaml`, the voice profiles;
- `.kapi/state/*.jsonl`, the committed unit-state record, one shard per
  document;
- the content files themselves, source and target.

Delete either database and a re-run rebuilds it from those. One thing stands
outside that rule and lives in the workspace: between a decision being recorded
and `kapi commit` materializing it, the ledger holds its only copy.

### `.kapi/work/` is free to delete

Everything under `.kapi/work/` is derived from the working tree: the parse
cache, extraction batches, collection overlays, and `store.db` itself. Deleting
the whole of it costs a re-extraction.

The one thing that exists in a single place is the decision ledger, and it sits
in the workspace: between a decision being recorded and `kapi commit` writing it
to `.kapi/state/`, the ledger holds the only copy. So deleting
`<DataDir>/workspaces/` costs every decision recorded since the last `kapi
commit`, in every project.

The redaction vault ([C-10](c-10-redaction.md)) at `.kapi/work/vault/` is the
one thing under `work/` that is still a loss rather than a rebuild: it holds
withheld originals, local-only by design, never committed and never sent
anywhere, so nothing else has a copy.

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
so writing the decision record with `kapi commit`, deleting the database and
running `kapi up` derives every row in it again. The context store holds terms,
approved wording and voice profiles that exist there and nowhere else, so
`projectdb.RekeyContextLocales` keys its rows canonically where they stand, in
one transaction over the context store, and `kapi context locales --fix` is the
verb that calls it. It deletes nothing: a row whose canonical spelling is free
takes it, a row saying exactly what the canonical row says folds into it and is
reported as merged, and a row whose canonical spelling already answers
differently stays where it is and is reported, because choosing between two
approvals is not a repair. The content memory's search indexes are rebuilt
afterwards, since they carry the locale beside each variant they were built
from.

### Presence is table-level

An empty subsystem inside an existing database behaves exactly as an absent
store does. A database's existence is not a signal, so nothing has to guard
against a file being there: the terminology gate reads the committed terms
source directly on a fresh checkout whether or not a database exists
([C-08](c-08-terms.md)), `kapi up --plan` never creates state, and `kapi pack`
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
| `blesses` | unit state → block | the pairing the decision was written against: the target hash, and the source basis |

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
exactly what changed. The index narrows and never decides (a trigram match is
necessary, not sufficient), so matching is done in Go, which is also how the
browser build answers the same question by scanning.

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

There is no SQLite in the browser build. The model is unchanged and the backends
differ: in-memory content memory and terms, a path-keyed in-memory block store,
and a decision ledger that persists to a JSON sidecar, `.kapi/work/store.json`.
Operations that genuinely need a database report `projectdb.ErrNoStore`, which
callers whose feature is optional there match and degrade on. The same sources
rebuild it, and the same graph relations hold.

There is no workspace there either, and no need of one: the browser holds one
project and nothing outlives the tab. So the browser build keeps the **embedded
layout**, and the host layer says so rather than failing: a workspace that
cannot be opened because this build has no file-backed SQLite driver
(`storage.ErrNoSQLite`) is not an error, and the project opens with its context
tables beside its projection.

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
into the developer's own context and read it back on the next run. A released
binary is not a test binary, which is why every in-repo surface that launches one
sets `$KAPI_DATA_DIR` as part of the isolation contract.

## Consequences

- **A clone starts where the project is, not where the checkout is.** A second
  clone of a project with an `id:` opens with its terms, its voice profiles, its
  content memory and its decisions already in force, and re-extracts its own
  projection.
- **A git worktree is a checkout, not a second project.** Decisions recorded in
  one are in force in the other, and switching branches moves no authored
  context.
- **Cross-subsystem questions are one query within a pool, and one join across
  them.** Term coverage per collection, the blocks behind a coordinate, the
  units a term change puts at risk.
- **One transaction still covers a decision and the wording it blesses.** Both
  are in the context pool, which is why they are in the same file.
- **Store paths are not a user surface.** The recipe binds *sources*
  (`defaults.terms_source`, `defaults.memory_source`); it names neither derived
  database and carries no workspace binding. Standalone stores outside a project
  keep their own selectors (`--termstore`, `--memory`); those address a file the
  user owns, which is a different thing.
- **CI caches `.kapi/work/cache/docs`, and nothing else under `work/`.** The
  parse cache is keyed by content, configuration and build, so a restored entry
  changes no result, and `setup-kapi` carries it by default. The remaining
  entries under `cache/` belong to the checkout that wrote them
  ([Convergence in CI](/kapi/convergence-in-ci)).
- **A machine's context is one directory.** Backing up
  `<DataDir>/workspaces/default/` backs up every project's authored context;
  deleting it costs every decision recorded since the last `kapi commit`.

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
