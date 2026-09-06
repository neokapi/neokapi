---
id: c-03-context-store-and-graph
sidebar_position: 3
title: "C-03: The context store and graph"
description: "Architecture decision: a kapi project keeps one local database, .kapi/work/store.db, the projection of its committed context and the substrate of its context graph. Every subsystem's tables share the file, a property graph relates them, and the same query shapes answer at any scope."
keywords: [store.db, context graph, graph_nodes, graph_edges, project store, projection, coordinates, durable identity, neokapi, architecture decision]
---

# C-03: The context store and graph

## Summary

A kapi project keeps **one** local database: `.kapi/work/store.db`. It is the
local **projection** of the project's committed context, never its truth, and
the substrate of the project's **context graph**.

Every subsystem's tables share that one file: the block cache, the terms store,
the content memory, the voice store, the unit-state working set, and a property
graph (`graph_nodes` / `graph_edges`) relating them. One file, one connection
pool, one migration ledger per subsystem.

The point of the single file is the join. *Which blocks use this term, in which
collection, at which coordinate, and which of them are signed off?* is one query
here and is unanswerable across separate database files.

## Context

Context is relational. A term occurs in blocks; blocks belong to collections and
sit at a point in the context space ([C-02](c-02-coordinates-and-governance.md));
a state record blesses a unit at a content hash; a memory entry recycles into a
block. Retrieval ([C-06](c-06-retrieval.md)) and governance
([C-02](c-02-coordinates-and-governance.md)) both traverse those relations rather
than reading one store in isolation.

A hosted layer answers such questions over one database spanning many projects.
If a local project could not answer them at all, the two halves would differ in
what they can be *asked*, not merely in scale, and the framework's standing
constraint is that kapi runs on its own.

## Decision

### One file, many subsystems

`.kapi/work/store.db` holds:

| Tables | Subsystem | Derived from |
| --- | --- | --- |
| block cache | `core/blockstore` ([C-01](c-01-project-model.md)) | the content files |
| terms | `terms/` ([C-08](c-08-terms.md)) | the committed terms source |
| content memory | `memory/` ([C-09](c-09-content-memory.md)) | the committed targets plus the `.memory.json` seeds |
| voice profiles | `voice/` ([C-07](c-07-voice-profiles.md)) | the committed `voice.yaml` files |
| unit-state working set | `core/state` ([C-04](c-04-unit-state-and-decisions.md)) | the committed `.kapi/state/*.jsonl` shards |
| `graph_nodes`, `graph_edges` | `host/storage/graph`, vocabulary in `core/contextgraph` | the five above, plus the recipe |

Each subsystem owns its own schema and its own migration ledger
(`storage.Migrate(db, "<subsystem>", …)`), so a subsystem evolves without
replaying anyone else's migrations. Sharing the file is a storage decision, not a
coupling: a subsystem still reaches its tables only through its own interface.
`core/projectdb` opens the pool and hands each subsystem its handle; a recipe's
`voice: profile: <name>` binding resolves against the pool's voice store, so the
same recipe resolves the same profile from any directory inside the project.

Sharing a file means sharing a writer. `core/storage` installs an in-process
**write gate** on the handle: one FIFO permit that every write transaction
passes through, so writers queue in arrival order instead of contending on
SQLite's `busy_timeout`, where a saturating writer can starve a small one
indefinitely. The gate is not reentrant, and a write transaction that tries to
open another reports `ErrWriteGateReentrant` rather than deadlocking.

### The committed sources are the truth

`store.db` is an **index**, and every row in it is reconstructible from:

- `.kapi/terms.json`, the terms source, bound by `defaults.terms_source`;
- `.kapi/memory/*.memory.json`, the content-memory seeds;
- `.kapi/voice.yaml` and `.kapi/profiles/*/voice.yaml`, the voice profiles;
- `.kapi/state/*.jsonl`, the committed unit-state record, one shard per
  document;
- the content files themselves, source and target.

Delete `store.db` and a re-run rebuilds it from those. Nothing authoritative
lives only in the database, which is why it sits under `.kapi/work/`, the one
ignored path ([C-01](c-01-project-model.md)), while the sources it is built from
are committed one directory over.

### It sits at the top of `work/`, not under `cache/`

`.kapi/work/cache/` means *free to delete*: the parse cache, extraction batches,
collection overlays. `store.db` does not qualify, and by exactly one margin.
Between a decision landing and `kapi commit` materializing it to `.kapi/state/`,
the working set inside `store.db` holds the **only** copy of that staged state.

So the cost of losing the file is bounded and stated precisely: at most the unit
state staged since the last commit. Everything else rebuilds. `rm -rf
.kapi/work/cache` remains completely free, and keeping the two apart is what lets
that sentence stay true.

Deleting `.kapi/work` outright is a wider claim, because the redaction vault
([C-10](c-10-redaction.md)) lives beside the database at `.kapi/work/vault/` and
holds withheld originals. Those are local-only by design, never committed and
never sent anywhere, so nothing else has a copy, and losing them is a loss
rather than a rebuild.

### Locales are keyed canonically

Every subsystem keys its rows by the canonical BCP-47 locale beside the text:
the content memory's variants ([C-09](c-09-content-memory.md)), the terms
store's terms ([C-08](c-08-terms.md)), the block cache's `targets/<locale>`
overlays and its `block_texts` rows. The stores apply `locale.Normalize` on
every write, read and list, so a producer writing `targets/nb_NO` and a reader
asking for `targets/nb-NO` address one overlay. Rows a store wrote before it
normalized locales are keyed by whatever spelling the recipe used then, and no
lookup finds them again; `projectdb.NonCanonicalLocales` reports them, and
`kapi status` and `kapi up` print the report once with the rebuild named. The
store is a projection, so the remedy is to write staged decisions with `kapi
commit`, delete `store.db` and let the next `kapi up` derive it again.

### Presence is table-level

An empty subsystem inside an existing `store.db` behaves exactly as an absent
store does. The database's existence is not a signal, so nothing has to guard
against the file being there: the terminology gate reads the committed terms
source directly on a fresh checkout whether or not a database exists
([C-08](c-08-terms.md)), `kapi up --plan` never creates state, and `kapi pack`
carries only non-empty parts
([M-06](../multilingual/m-06-content-packages.md)). `projectdb.DB.DetectStoreDrift`
reads "store missing" as *the block cache holds no blocks*, not as *the file is
absent*, precisely because the file exists from the first open of any subsystem.

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
projection: each pass clears what it is entitled to and rebuilds it, so a delete
of the store loses nothing the recipe, the terms, the blocks and the committed
state cannot rebuild. Occurrence edges come from a term search over the block
cache (`core/occurrence`), where repeated uses of one term in one block fold into
a `count` property rather than into separate edges, and the term and the locale
are the edge discriminators; `governed_by` comes from resolving each named
collection's governance; `blesses` joins the unit-state working set against the
block cache, so a record whose block no longer exists keeps its node and loses
its edge. A unit state names its document by the durable key the working set
records ([C-04](c-04-unit-state-and-decisions.md)); the graph writer turns that
key back into the path the block cache files blocks under before joining.

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

**Dimensions are fields, not containment edges.** Every node carries the
non-empty components of its scope tuple as properties as well as in its id, so
slicing a view by project or stream is a filter rather than a traversal. An empty
dimension is written as an *absent* key rather than an empty string, because a
property filter compares against a value and absence is not the empty string.
Locally that means the scope properties are not there, which is correct: a
project on its own has no workspace to name. There is no project→project edge and
there will not be one: projects relate by co-occurrence through the vocabulary
they share.

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
the same pass: no row survives under the old key. The project dimension is the
recipe's project name, the only identity a project on its own has, so a rename
re-keys the whole projection, which is the rebuild each pass performs anyway.
Where a project dimension is issued rather than authored, a rename does not touch
it: the display name rides as the `project_name` property and the rule costs
nothing.

### Finding a term inside blocks

Term occurrence has to search text that lives inside a JSON payload no SQL can
read, so the block cache keeps `block_texts`: a row per block per locale (the
source under the empty locale, each target under its own) with a contentless
FTS5 trigram index over it.

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
and a working set that persists to a JSON sidecar, `.kapi/work/store.json`.
Operations that genuinely need the database report `projectdb.ErrNoStore`, which
callers whose feature is optional there match and degrade on. The same sources
rebuild it, and the same graph relations hold.

## Consequences

- **One connection per project.** Subsystems open the shared `storage.DB` rather
  than a file each, so one transaction can span a state record and the graph
  edges it implies.
- **Cross-subsystem questions are ordinary SQL.** Term coverage per collection,
  the blocks behind a coordinate, the units a term change puts at risk: joins,
  not application-level merges.
- **Store paths are not a user surface.** The recipe binds *sources*
  (`defaults.terms_source`, `defaults.memory_source`); it never names the derived
  database. Standalone stores outside a project keep their own selectors
  (`--termstore`, `--memory`); those address a file the user owns, which is a
  different thing.
- **CI caches `.kapi/work/cache/`, not `store.db`.** The database carries staged
  unit state, and a cache key that restores stale state is worse than a cold
  rebuild ([Convergence in CI](/kapi/convergence-in-ci)).

## See also

- [C-01: The project model](c-01-project-model.md): the layout the store sits in.
- [C-04: Unit state and the decision record](c-04-unit-state-and-decisions.md):
  the working set inside `store.db`.
- [C-08: Terms](c-08-terms.md) and [C-09: Content memory](c-09-content-memory.md):
  the two subsystems whose source-versus-projection split this store
  implements.
- [C-06: Context retrieval](c-06-retrieval.md): the primitives these tables
  answer.
- [The project store](/kapi/project-store): the end-user view.
