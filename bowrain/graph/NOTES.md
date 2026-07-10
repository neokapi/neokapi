# bowrain/graph — backend selection notes

The brand knowledge graph (`core/graph.GraphStore`) is **optional** (the server
runs with `GraphStore == nil`) and **derived** — `GraphSyncer` rebuilds it from
relational events. Its consumed interface is plain node/edge CRUD, neighbours,
scoped traversal, and a bounded `ShortestPath`; no consumer issues raw Cypher.
That lets the graph run on a plain-SQL implementation over **standard
PostgreSQL**.

## Two backends, one interface

Both backends implement the same `core/graph.GraphStore`:

| Backend | Type | Storage | Cypher |
|---|---|---|---|
| **SQL** (default) | `SQLGraphStore` (`sql.go`) | plain adjacency tables (`graph_nodes`, `graph_edges`) on stock PostgreSQL | not supported (`CypherQuery`/`CypherExec` return `core/graph.ErrCypherNotSupported`) |
| **AGE** (opt-in) | `AGEGraphStore` (`age.go`) | Apache AGE property graph via `ag_catalog.cypher()` | supported |

## Why SQL is the default (RDS)

The AWS launch runs PostgreSQL on managed **RDS/Aurora**, which do **not**
support the Apache AGE extension. Because the graph is optional + derived and
needs no native traversal, the default backend is the plain-SQL one, so dev and
production both run on the same standard `postgres:16` image. The AGE-only
`ag_catalog` search-path `AfterConnect` hook (`afterconnect.go`) is installed
**only** when the AGE backend is selected, so a stock/RDS Postgres works out of
the box.

## AGE is the documented future upgrade path

AGE stays available as an opt-in backend and is the upgrade path if the workload
ever needs native graph traversal (deep multi-hop Cypher, path algorithms).
Selecting it requires an AGE-enabled PostgreSQL — it is not available on managed
RDS/Aurora today.

## Selecting a backend

Server-side selection is by env var, read in `bowrain/server/postgres.go`:

- `BOWRAIN_GRAPH_BACKEND` unset or `sql` → `SQLGraphStore` (default). Standard
  Postgres; the AGE `AfterConnect` hook is **not** installed;
  `SQLGraphStore.EnsureGraph` creates the schema at wiring time.
- `BOWRAIN_GRAPH_BACKEND=age` → `AGEGraphStore`. Requires an AGE-enabled
  PostgreSQL; the `ag_catalog` `AfterConnect` hook is installed and the graph is
  created by `migrations/init-age.sql`.

`GraphSyncer` (`sync.go`) is backend-agnostic — it takes a
`core/graph.GraphStore` and needs no change.

## Parity

`parity_test.go` runs one behavioral contract suite (`graphContractSuite`)
against both backends to prove they behave identically for the operations
bowrain uses:

- `TestSQLGraphStoreContract` — the default backend, on standard PostgreSQL via
  the `pgtest` testcontainer; exercised by ordinary CI/local runs.
- `TestAGEGraphStoreContract` — gated on `AGE_TEST_DSN` (an AGE-enabled
  PostgreSQL); skipped by default so the standard run needs no AGE image.

The suite deliberately omits the two operations that legitimately differ between
backends (below).

### Known backend differences

- **Cypher** (`CypherQuery`/`CypherExec`): AGE-only escape hatch; SQL returns
  `ErrCypherNotSupported`. Asserted separately, not in the shared suite.
- **`FindNodesScoped`**: the SQL backend mirrors the SQLite reference (returns
  only nodes with at least one edge active under the scope); the AGE backend's
  implementation is currently a no-op filter (returns all label/property
  matches). Not covered by the shared suite.
- **`ShortestPath` with no path**: SQL returns `(nil, nil)` (matching the SQLite
  reference); AGE returns an empty, non-nil `*Path`. Callers treat both as "no
  path"; the shared suite only asserts the found-path case.
- **`DeleteNode` with incident edges**: the SQL backend cascades — its
  `graph_edges` foreign keys use `ON DELETE CASCADE`, so deleting a node also
  deletes its incident edges. This matches the AGE backend's `DETACH DELETE`.
  The SQLite framework reference (`host/storage/graph/sqlite.go`) instead uses
  plain foreign keys (no cascade) with `PRAGMA foreign_keys=ON`, so its
  `DeleteNode` *errors* on a node that still has edges. The cascade was chosen so
  the two server backends (SQL default + AGE opt-in) agree and no orphan edges
  survive to break `ShortestPath` reconstruction. The shared suite deletes only
  edge-less nodes, so both server arms stay identical.
