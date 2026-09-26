---
sidebar_position: 4
title: "Workspace storage"
description: Implementation note for C-03. The files a workspace keeps, the tables each pool holds, how a cross-file query is run, the three settings every pool is opened with, what the contention harness measured, and how a read-only workspace is opened.
keywords: [workspace, context store, projection, SQLite, ATTACH, WAL, advisory lock, write gate, contention, read-only, implementation note, neokapi]
---

# Workspace storage

This note provides implementation details for
[C-03](/contribute/architecture/context/c-03-context-store-and-graph).

## The files

A workspace is a directory of SQLite databases, created on first use
(`core/workspace.LocalBackend`):

```text
<DataDir>/workspaces/default/
  workspace.db                  the project registry, the context graph,
                                the operation log, widened rules, agent sessions
  projects/
    prj_9f2k…q7.db              one project's context store
    prj_4c8m…b1.db
```

`workspace.RegistryFileName` is `workspace.db` and `workspace.ProjectsDirName`
is `projects`. A project's file is `FileNameFor(key) + ".db"`: the key verbatim
when it is already a safe file name (1 to 64 characters of `a-z`, `0-9`, `-`
and `_`), so a directory listing reads as the projects it holds, and otherwise a
sanitized prefix of at most 32 bytes followed by the first eight bytes of the
key's SHA-256 in hex. An empty key files under `unidentified.db`.

A recipe that states neither an `id:` nor a `name:` takes
`workspace.KeyForCheckout(normalizedRoot)`: `at_` followed by the first ten
bytes of `SHA-256` over the slash-separated normalized checkout path, in
lowercase unpadded base32.

The projection stays in the checkout at `.kapi/work/store.db`
(`project.Layout.StorePath()`).

## The tables each pool holds

| Tables | Subsystem | Pool |
| --- | --- | --- |
| block cache, overlays | `core/blockstore` | projection |
| `store_meta` | `core/projectdb` | projection |
| `tb_*` | `terms/` | context |
| `tm_*` | `memory/` | context |
| `voice_*` | `voice/` | context |
| `unit_decision`, `unit_view`, `document`, `checkout`, `state_meta` | `core/state` | context |
| `projector_cursor` | `core/projector` | context |
| `graph_nodes`, `graph_edges` | `host/storage/graph` | workspace |
| `workspace_projects`, `workspace_checkouts` | `core/workspace` | workspace |
| `workspace_ops` (arrival `seq`, operation `id`, optional content `address`) | `core/workspace` | workspace |
| `workspace_blobs` (`sha256:` digest, size, gzip-compressed bytes) | `core/workspace` | workspace |
| `workspace_rules` | `core/workspace` | workspace |
| `workspace_agent_sessions` | `core/workspace` | workspace |

Each subsystem migrates its own schema under its own ledger table
(`storage.Migrate(db, "<subsystem>", …)`), so a subsystem evolves without
replaying anyone else's migrations, whichever pool it binds to. The rules and
sessions schemas migrate lazily on first use, so a workspace whose projects
widened nothing pays nothing for the table.

`core/projectdb` opens both project pools and hands each subsystem its handle.
Callers name a capability rather than a file: `Blocks()` and
`BlocksAutocommit()` come from the projection, and `Memory()`, `Terms()`,
`Voice()`, `Work()` and `Raw()` from the context store.

## The projector

`core/projector` is the only writer of `tb_*`, `tm_*`, `voice_profiles`,
`voice_profile_versions` and `workspace_rules`
([C-03](../../architecture/context/c-03-context-store-and-graph.md#the-stores-are-projections-of-the-log)).
A write is two transactions under one in-process mutex per context store: the
operation into `workspace_ops` (and its steps into `workspace_blobs` when they
pass 32 KiB), then the store calls, then the cursor. The mutex is keyed by the
context pool, so two projectors over one store in one process take turns; a
second process on the same store applies what the first recorded on its next
write, from the cursor.

A step stores content-memory entries in the `.memory.json` bundle's entry form
(`memory/kmb`), concepts and relations as the terms store's own JSON, and voice
profiles as `core/profile.VoiceProfile`. A step is applied with the store call
that made it, so `Bulk` steps go through `BulkAddWithStream` and single writes
through `AddWithStream`. A run of 32 or more single writes, in one batch or in
consecutive operations during a rebuild, goes through `ReplayWithStream`: the
same rows as `AddWithStream`, one transaction, and the two FTS5 tables rebuilt
once afterwards.

`Rebuild` deletes every row of the projection tables, skipping the FTS5 shadow
tables (emptying the virtual table empties them), resets their `sqlite_sequence`
entries so autoincrement ids repeat, narrows every rule whose origin is the
project, and replays the project's operations in id order.

Measured in process on an M-series laptop, 16 goroutine writers
(`KAPI_MEASURE_OPLOG=1 go test -tags fts5 ./core/projector -run Measure -v`):

| Measure | Result |
| --- | --- |
| rebuild: 13 000 entries from one batch, 2 000 single-entry operations, 1 000 single-concept operations | 2.8 s |
| a concept or an entry written through the projector, 16 writers, over a store of 13 000 entries | p50 141 ms, p99 283 ms |
| the same writes straight to the stores, no log | p50 138 ms, p99 342 ms |
| an agent's observation (`Ledger.Append`, which writes no store), 16 writers, 13 000 other operations | p50 21 ms, p99 31 ms |

The projector adds about a millisecond to a single write. The tail under 16
writers belongs to the content memory's row-by-row FTS5 maintenance described
below, which the direct writes show just the same.

## Joining across the two files

`projectdb.DB.Join` runs a function on one connection with both files visible:
the projection as `main` and the context store attached as `context`
(`projectdb.ContextSchema`).

```sql
SELECT DISTINCT bt.block_hash
FROM block_texts bt
JOIN context.tb_terms t ON instr(bt.text_lower, t.text_lower) > 0
WHERE t.concept_id = ?
```

The connection carries `PRAGMA query_only=ON` for the length of the call and
detaches on the way out, so the pooled connection goes back as it came. A join
reads. A write spanning both pools is two transactions, which is why the
decision ledger and the content memory sit in the same pool: the pairing that
must be atomic is a decision and the wording the content memory learns from it.

## The three settings every pool is opened with

`storage.ProjectOptions()` is `ImmediateTx`, `SerializeWrites` and
`CrossProcessWrites` together:

- **`BEGIN IMMEDIATE`** on every transaction. A deferred transaction that reads
  before it writes must upgrade its lock, and SQLite refuses a contended upgrade
  immediately, without consulting `busy_timeout`. Taking the write lock up front
  puts the wait somewhere the busy handler applies.
- **An in-process FIFO permit** (`core/storage.writeGate`) every write holds for
  its whole life. SQLite's busy backoff has no memory of who has waited longest:
  measured at dogfood scale, a drip of small unit-state writes completed 32 of
  2650 attempts against a saturating content-memory writer. The permit prevents this writer starvation.
- **A cross-process advisory lock** on `<database>.lock`, a separate file beside
  the database, taken inside the permit and released by the same `Commit` or
  `Rollback`. It is `flock` on Unix and `LockFileEx` on Windows, and a no-op on
  a build with neither. The separate file avoids interference with SQLite's locks on the database.

`core/storage/write.go` applies both: `ExecContext`, `Exec`, `BeginTx` and
`Begin` shadow the promoted `*sql.DB` methods and take the gate and then the
lock. Reads stay promoted and are never gated: under WAL a reader neither blocks
a writer nor waits for one.

Permits and locks are per file. A long block-store transaction locks only the
projection, allowing review decisions to be written concurrently to the context
store.

Two consequences at the call site:

- A block-store session from `Blocks()` is one transaction over a whole
  purge-and-refill, so it holds the projection's permit from `Begin` to
  `Commit`. A write to the same pool issued from the goroutine holding one is a
  deadlock, reported rather than hung: `storage.ErrWriteGateReentrant`.
- A second kapi process on the same project queues in the kernel rather than in
  SQLite's backoff, for both pools.

## What the contention harness measured

`scripts/contention-harness -mode=workspace` runs the shape the split creates:
16 agent-shaped writer processes, each in a checkout of its own, writing small
transactions into one shared context store; one desktop-shaped reader polling it
for change; and one CLI-shaped run holding a purge-and-refill transaction over
its own projection. Eighteen operating-system processes, because an in-process
permit can only order writers that share a handle.

Bar: zero failed writes across every stream, and p99 under 50 ms for the small
writes an observation and a decision make.

Measured over 120 s at dogfood scale (75 000 blocks, 20 000 content-memory
entries, 2 000 concepts, 75 000 units), 16 agents recording a decision once a
second and promoting wording into the content memory every tenth round:

| Workload | ops | failed | p50 ms | p95 ms | p99 ms | max ms |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| agent: decision put | 1895 | 0 | 0.68 | 4.80 | 30.19 | 76.99 |
| agent: register in workspace | 16 | 0 | 0.33 | 0.53 | 0.53 | 0.53 |
| agent: content-memory promotion | 198 | 0 | 33.19 | 48.78 | 90.98 | 105.36 |
| desktop: poll for change | 599 | 0 | 16.36 | 25.44 | 29.89 | 33.47 |
| CLI: projection purge+refill | 11 | 0 | 74.81 | 93.26 | 93.26 | 93.26 |
| CLI: context write | 11 | 0 | 32.13 | 44.07 | 44.07 | 44.07 |

The measured projection transactions took 75–93 ms, while agent decision writes
met the p99 target. Content-memory promotion and projection writes are reported
separately from the latency gate for small context writes.

**Content-memory writes have additional indexing costs.**
`memory.Add` maintains the FTS5 tables row by row, and it grows with the corpus:
on this store, with a single writer and nobody else on the file, it costs 32 ms
at the median and 47 ms at the 99th percentile. The sixteen-process run above
measures 33 ms and 89 ms, so contention adds about a millisecond to the median.
Holding that write to a 50 ms bar would measure FTS5 rather than the workspace.
A converge worker with entries in hand should use `BulkAddWithStream`, which is
one transaction for the batch; at the decision rate (`-memory-every=1`) the
store saturates and the tail goes with it.

**Cross-process locking reduces tail latency.** With
the in-process permit alone, sixteen agents recorded zero failed writes but a
66 ms p99 and a 778 ms maximum on a write whose median was 0.45 ms. The tail was
`busy_timeout` rather than the work: a writer that loses the race sleeps a fixed
step of the backoff ladder (1, 2, 5, 10, 15, 20, 25, 25, 25, 50, 50, 100 ms) and
wakes to try again, so a lock that freed a millisecond later stays untaken for
the rest of the step. Replacing the sleep with a kernel wait took that p99 to
21 ms and the maximum to 57 ms on the same configuration.

## Reading from a write-restricted sandbox

A check may run where it can read the workspace and not write it: a restricted
sandbox, a read-only mount, a workspace owned by another account. Under WAL that
is a write, because SQLite creates the write-ahead log's shared-memory index
beside the database on the first connection.

`storage.OpenReadOnly` answers it in two steps. It first opens the database
where it is, with the journal-mode switch skipped and every connection carrying
`PRAGMA query_only=ON`. If SQLite still cannot open it, the database and its log
are copied to a writable temporary directory and the copy is opened, and the
copy is deleted when the handle closes. The workspace then reports itself
read-only, answers reads, and refuses a write with `workspace.ErrReadOnly`
rather than losing it somewhere the caller cannot see.

## A synchronized folder is refused

`workspace.CloudSyncedDir` matches a workspace root a path segment at a time,
case-insensitively, against the folder names iCloud Drive, Dropbox, OneDrive and
Google Drive use. A prefix match has to end at a space, a hyphen or an
underscore, so `OneDrive - Contoso` matches and `OneDriveProjects` does not. The
message names the folder, the product and the way out, which is to set
`KAPI_DATA_DIR` to a directory outside it.

## Adoption out of the embedded layout

`projectdb.Open` asks whether the context store holds any user table at all
before it binds the subsystems, because binding creates those tables. A workspace
supplied with an empty context store and a projection that still carries context
tables is adopted.

Every context row moves, because nothing reproduces one: the terms, the content
memory, the voice profiles and every record in the decision ledger are read out
of the old store and recorded in the new one. The old store's own migration runs
first, so a project that predates the ledger has its rows carried into one before
they are read.

What is dropped afterwards is computed rather than listed. An empty projection is
built in memory, its tables are what a projection is entitled to hold, and every
other table in the file belonged to a subsystem that has moved out. Virtual
tables are dropped first, since they take their shadow tables with them. The
whole pass is best-effort: a project that cannot be adopted keeps both copies and
works from the context store, which `kapi context import` fills from the
checkout's own files.

## Related

- [C-03: The context store and graph](/contribute/architecture/context/c-03-context-store-and-graph):
  the decision this note implements.
- [C-04: Unit state and the decision record](/contribute/architecture/context/c-04-unit-state-and-decisions):
  the ledger in the context pool.
