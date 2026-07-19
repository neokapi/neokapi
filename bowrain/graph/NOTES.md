# bowrain/graph — backend notes

The brand knowledge graph (`core/graph.GraphStore`) is **optional** (the server
runs with `GraphStore == nil`) and **derived** — `GraphSyncer` rebuilds it from
relational events. Its consumed interface is plain node/edge CRUD, neighbours,
scoped traversal, and a bounded `ShortestPath`; no consumer issues raw Cypher.
That lets the graph run on a plain-SQL implementation over **standard
PostgreSQL**.

## One backend: plain SQL

`SQLGraphStore` (`sql.go`) is the only backend: plain adjacency tables
(`graph_nodes`, `graph_edges`) on stock PostgreSQL. Cypher is not supported —
`CypherQuery`/`CypherExec` return `core/graph.ErrCypherNotSupported`.

The AWS deployment runs PostgreSQL on managed **RDS/Aurora**, and because the
graph is optional + derived and needs no native traversal, plain SQL means dev
and production both run on the same standard `postgres:16` image with no
extension requirements. `SQLGraphStore.EnsureGraph` creates the schema at
wiring time (`bowrain/server/postgres.go`).

`GraphSyncer` (`sync.go`) is backend-agnostic — it takes a
`core/graph.GraphStore`.

## History: the removed Apache AGE backend

An opt-in **Apache AGE** backend (`AGEGraphStore`, with an `ag_catalog`
search-path `AfterConnect` hook and an `init-age.sql` bootstrap) existed
alongside the SQL store, selected with `BOWRAIN_GRAPH_BACKEND=age` and proven
interchangeable by running the shared contract suite against both arms. It was
removed as part of the surface cut: AGE is unavailable on managed RDS/Aurora,
no consumer ever needed Cypher, and carrying a second backend cost parity-test
and image maintenance with no production user. If the workload ever needs
native graph traversal (deep multi-hop Cypher, path algorithms), a native graph
engine is the upgrade path — reintroduce it behind the same
`core/graph.GraphStore` interface and revive the second contract-suite arm from
git history.

## Contract suite

`parity_test.go` runs the behavioral contract suite (`graphContractSuite`)
against the SQL backend on standard PostgreSQL via the `pgtest` testcontainer
(`TestSQLGraphStoreContract`), exercised by ordinary CI/local runs. The
Cypher-not-supported escape hatch is asserted separately
(`TestSQLGraphStoreCypherUnsupported`).

### Behavior notes

- **`FindNodesScoped`**: mirrors the SQLite reference (returns only nodes with
  at least one edge active under the scope).
- **`ShortestPath` with no path**: returns `(nil, nil)`, matching the SQLite
  reference.
- **`DeleteNode` with incident edges**: cascades — `graph_edges` foreign keys
  use `ON DELETE CASCADE`, so deleting a node also deletes its incident edges
  (Cypher's `DETACH DELETE` semantics). The SQLite framework reference
  (`host/storage/graph/sqlite.go`) instead uses plain foreign keys (no cascade)
  with `PRAGMA foreign_keys=ON`, so its `DeleteNode` *errors* on a node that
  still has edges. The cascade ensures no orphan edges survive to break
  `ShortestPath` reconstruction.
