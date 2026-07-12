---
id: 006-graph-concept-storage
sidebar_position: 6
title: "AD-006: Graph Concept Storage"
---

# AD-006: Graph Concept Storage

## Summary

Bowrain stores concept relationships, brand vocabulary networks, and
billing metadata links as a graph. The `GraphStore` interface has two
server backends: plain-SQL adjacency tables on standard PostgreSQL
(`SQLGraphStore`, the default) and Apache AGE (a PostgreSQL extension, an
opt-in upgrade). The graph is optional and derived — a `GraphSyncer`
rebuilds it from relational events — and no consumer issues raw Cypher, so
the default backend runs on stock PostgreSQL. Edges carry temporal validity
and tag-based scoping. The AGE backend additionally extends `GraphStore`
with a `CypherStore` sub-interface for native Cypher queries. The backend is
selected with `BOWRAIN_GRAPH_BACKEND` (unset or `sql` → SQL; `age` → AGE);
see [Backend selection](#backend-selection).

## Context

Terminology is conceptually a graph. Concepts relate to broader and
narrower concepts, to related concepts, to deprecated synonyms, to
competitor terms, and to terms designated for specific brand voices.
Brand voice rules form another graph: brand → preferred terms, brand →
forbidden terms, brand → competitor terms. Billing metadata links
workspaces, plans, feature flags, and quotas through relationship chains.

A flat relational model can store edges, but traversal queries (find all
narrower concepts, shortest path between concepts, scoped neighbors at a
specific point in time) become unnecessarily complex. A native graph
representation makes these queries natural and fast.

The graph also needs temporal semantics: relationships change over time
(term supersession, seasonal terminology, product lifecycle), and they
scope to tag dimensions (market, product line, channel). A single model
has to support both dimensions without hard-coding vocabulary.

## Decision

### GraphStore Interface

The `GraphStore` interface in `core/graph/store.go` is a
backend-agnostic graph API:

```go
type GraphStore interface {
    // Node CRUD
    CreateNode(ctx context.Context, node *Node) error
    GetNode(ctx context.Context, id string) (*Node, error)
    UpdateNode(ctx context.Context, node *Node) error
    DeleteNode(ctx context.Context, id string) error

    // Node queries
    FindNodes(ctx context.Context, label string, properties map[string]string) ([]*Node, error)
    FindNodesScoped(ctx context.Context, label string, properties map[string]string, scope Scope) ([]*Node, error)

    // Edge CRUD + queries
    CreateEdge(ctx context.Context, edge *Edge) error
    GetEdge(ctx context.Context, id string) (*Edge, error)
    UpdateEdge(ctx context.Context, edge *Edge) error
    DeleteEdge(ctx context.Context, id string) error
    FindEdges(ctx context.Context, label string, properties map[string]string) ([]*Edge, error)

    // Traversal
    Neighbors(ctx context.Context, nodeID string, direction Direction, labels ...string) ([]*Node, error)
    NeighborsScoped(ctx context.Context, nodeID string, direction Direction, scope Scope, labels ...string) ([]*Node, error)
    EdgesOf(ctx context.Context, nodeID string, direction Direction, labels ...string) ([]*Edge, error)
    ShortestPath(ctx context.Context, fromID, toID string, maxDepth int) (*Path, error)

    // Bulk
    BulkCreateNodes(ctx context.Context, nodes []*Node) error
    BulkCreateEdges(ctx context.Context, edges []*Edge) error

    Close() error
}
```

`Direction` supports `Outgoing`, `Incoming`, and `Both` for edge traversal.

### Data Types

```go
type Node struct {
    ID         string            `json:"id"`
    Label      string            `json:"label"`
    Properties map[string]string `json:"properties"`
    CreatedAt  time.Time         `json:"created_at"`
    UpdatedAt  time.Time         `json:"updated_at"`
}

type Edge struct {
    ID         string            `json:"id"`
    Source     string            `json:"source"`
    Target     string            `json:"target"`
    Label      string            `json:"label"`
    Properties map[string]string `json:"properties"`
    Validity   *Validity         `json:"validity,omitempty"`
    CreatedAt  time.Time         `json:"created_at"`
    UpdatedAt  time.Time         `json:"updated_at"`
}

type Path struct {
    Nodes []Node `json:"nodes"`
    Edges []Edge `json:"edges"`
}
```

### Temporal Validity

Edges carry optional `Validity` combining temporal bounds and tag-based
scoping:

```go
type Validity struct {
    ValidFrom *time.Time        `json:"valid_from,omitempty"`
    ValidTo   *time.Time        `json:"valid_to,omitempty"`
    Tags      map[string]string `json:"tags,omitempty"`
}

type Scope struct {
    At   time.Time         `json:"at"`
    Tags map[string]string `json:"tags,omitempty"`
}
```

Matching rules (`Validity.Matches(Scope)`):

- Nil validity always matches (unbounded edge).
- Time: half-open interval `[ValidFrom, ValidTo)`.
- Tags: all scope tags must be present in validity tags with matching
  values (open-world — extra validity tags are ignored).

Tag dimensions are workspace-configurable via `brand.TagDimension`. The
graph itself has no hard-coded dimensions, so customers can introduce
`market`, `product`, `channel`, `locale-family`, or any other axis
without schema changes.

Helper functions: `Now()`, `ScopeAt(t)`, `ScopeWithTags(tags)`,
`IsExpired()`, `IsActive()`.

### SKOS-Aligned Edge Labels

Edge labels in `core/graph/labels.go` align with W3C SKOS vocabulary for
terminology interoperability:

| Label         | SKOS / Semantic Origin | Purpose                           |
| ------------- | ---------------------- | --------------------------------- |
| `BROADER`     | skos:broader           | Parent concept                    |
| `NARROWER`    | skos:narrower          | Child concept                     |
| `RELATED`     | skos:related           | Associative link                  |
| `PART_OF`     | meronymy               | Component of                      |
| `HAS_PART`    | holonymy               | Contains component                |
| `HAS_TERM`    | terminological         | Concept to term designation       |
| `USE_INSTEAD` | terminological         | Deprecated to preferred term      |
| `REPLACED_BY` | terminological         | Superseded concept to replacement |
| `EXACT_MATCH` | skos:exactMatch        | Cross-scheme equivalence          |
| `CLOSE_MATCH` | skos:closeMatch        | Approximate equivalence           |
| `FORBIDDEN`   | brand voice            | Brand to forbidden term           |
| `PREFERRED`   | brand voice            | Brand to preferred term           |
| `COMPETITOR`  | brand voice            | Brand to competitor term          |

`InverseLabel()` returns the inverse of directional labels
(BROADER/NARROWER, PART_OF/HAS_PART) so callers can navigate in either
direction.

### SQL Backend (Default)

The default server backend, `SQLGraphStore` in `bowrain/graph/sql.go`,
implements `GraphStore` on standard PostgreSQL using plain relational
adjacency tables (`graph_nodes`, `graph_edges`) — no Apache AGE and no
Cypher. It is a dialect port of the framework's SQLite reference
(`host/storage/graph/sqlite.go`): the same node/edge CRUD, jsonb property
containment, temporal `Scope`/`Validity` filtering (evaluated in Go via
`Validity.Matches`), directional neighbours, and a bounded, cycle-safe
recursive-CTE `ShortestPath`.

Because the graph is optional and derived and needs no native traversal, SQL
is the default so both local development and managed RDS/Aurora — which do
not ship the AGE extension — run on the same stock `postgres:16` image.
`CypherQuery` / `CypherExec` return `core/graph.ErrCypherNotSupported` on
this backend. `EnsureGraph` creates the schema at wiring time, under a
transaction-scoped advisory lock so replicas cold-starting against one fresh
database serialise their DDL rather than race.

### Apache AGE Backend (Opt-In)

The opt-in backend, `AGEGraphStore` in `bowrain/graph/age.go`, implements
`GraphStore` using PostgreSQL's Apache AGE extension, and is the upgrade path
for workloads that eventually need native graph traversal (deep multi-hop
Cypher, path algorithms). It requires an AGE-enabled PostgreSQL (for example
the `apache/age` image); managed RDS/Aurora do not support it today. The AGE
backend also implements a `CypherStore` sub-interface that adds native Cypher
query support:

```go
type CypherStore interface {
    graph.GraphStore
    CypherQuery(ctx context.Context, query string, params map[string]any) ([]*graph.Node, error)
    CypherExec(ctx context.Context, query string, params map[string]any) error
}
```

Cypher methods are not part of the core `GraphStore` interface — they
are a backend-specific extension. Callers type-assert to `CypherStore`
when they need Cypher access. This keeps `GraphStore` fully
implementable by the SQLite backend while letting power users on AGE
write complex graph queries directly.

AGE graphs are created once per deployment:

```sql
SELECT * FROM ag_catalog.create_graph('bowrain_graph');
```

Queries route through `ag_catalog.cypher()`:

```sql
SELECT * FROM ag_catalog.cypher('bowrain_graph', $$
    MATCH (n {id: 'c1'})-[r:BROADER]->(m) RETURN m
$$) as (v agtype);
```

A `pgx` `AfterConnect` hook loads the AGE extension and sets the search
path when pooled connections are first established. This hook is installed
**only** when the AGE backend is selected, so a stock (or RDS) PostgreSQL
running the SQL backend works out of the box.

### agtype Parsing

AGE returns results as `agtype`, a custom PostgreSQL type. The parser in
`bowrain/graph/agtype.go` handles three formats:

- **Vertex.** `{"id": 123, "label": "Concept", "properties": {...}}::vertex`
- **Edge.** `{"id": 456, "label": "BROADER", "start_id": 123, "end_id": 789, "properties": {...}}::edge`
- **Path.** `[<vertex>::vertex, <edge>::edge, <vertex>::vertex]::path`

Vertex and edge IDs prefer application-level `id` in properties, falling
back to AGE's internal `id`. Properties convert from `map[string]any` to
`map[string]string`. Paths are parsed by tracking brace depth so commas
inside JSON objects don't split elements prematurely.

### Backend selection

Server-side selection is by environment variable, read in
`bowrain/server/postgres.go`:

- `BOWRAIN_GRAPH_BACKEND` unset or `sql` → `SQLGraphStore` (default). Standard
  PostgreSQL; the AGE `AfterConnect` hook is **not** installed, and
  `EnsureGraph` creates the schema at wiring time.
- `BOWRAIN_GRAPH_BACKEND=age` → `AGEGraphStore`. Requires an AGE-enabled
  PostgreSQL; the `ag_catalog` `AfterConnect` hook is installed and the graph
  is created via `ag_catalog.create_graph`.

The graph is optional and derived, so a schema-setup failure degrades to a
nil `GraphStore` (brand-graph features disabled) rather than failing server
startup. A `parity_test.go` contract suite runs the operations bowrain uses
against both backends to prove they behave identically. The two operations
that legitimately differ — raw Cypher and unbounded-scope `FindNodesScoped` —
are asserted separately. See `bowrain/graph/NOTES.md`.

### SQLite reference (framework)

The `SQLGraphStore` schema is a dialect port of the framework's SQLite
reference, `host/storage/graph/sqlite.go`, which the CLI and single-instance
tooling use with no external database. Both implement the same `GraphStore`
with adjacency tables (`graph_nodes`, `graph_edges`), JSON/jsonb-encoded
properties, nullable RFC3339 validity bounds, Go-side tag filtering for scoped
queries, and a recursive-CTE cycle-safe `ShortestPath`. The one behavioural
difference: the SQL/Postgres backend and AGE cascade edge deletes
(`ON DELETE CASCADE` / `DETACH DELETE`), whereas the SQLite reference errors
on deleting a node that still has incident edges.

### Event-Driven Graph Sync

The server-side `GraphSyncer` in `bowrain/graph/sync.go` subscribes to
the event bus and keeps the graph in sync with relational content
changes. It is backend-agnostic — it takes a `core/graph.GraphStore` and
works with either backend:

| Event               | Action                                                      |
| ------------------- | ----------------------------------------------------------- |
| `EventBlockCreated` | Create Concept node with `project_id` and `name` properties |
| `EventBlockUpdated` | Update node properties                                      |
| `EventBlockDeleted` | Delete node (edges cascade: SQL `ON DELETE CASCADE`, AGE `DETACH DELETE`) |

The syncer uses a 10-second context timeout per event and logs errors
without failing — graph inconsistency is recoverable; blocking event
processing is not. Because the graph is a derived projection, it can be
rebuilt from the relational stores at any time.

### Terminology Integration

The `ConceptRelation` type in the termbase package bridges termbase and
graph systems:

```go
type ConceptRelation struct {
    SourceID     string `json:"source_id"`
    TargetID     string `json:"target_id"`
    RelationType string `json:"relation_type"` // graph.Label* constants
}
```

`TermDesignation` pairs a `Term` with a `Validity` for the
status-on-edge model, where term lifecycle status (approved, pending,
deprecated) can be time-bounded or tag-scoped.

Terminology updates emit events consumed by the `GraphSyncer`, keeping
concept relationships reflected in the graph without direct termbase →
graph coupling.

### Brand Voice and Billing

Brand voice vocabulary projects onto the graph: preferred terms
(`PREFERRED`), forbidden terms (`FORBIDDEN`), and competitor mentions
(`COMPETITOR`) are edges from brand nodes. Scoped queries (Scope with
market/product tags) resolve the effective brand vocabulary at a point in
time. The relational brand store remains the source of truth; the graph is
the derived traversal projection.

Billing metadata uses the graph to link workspaces to plans, plans to
feature flags, and features to quotas. Temporal validity on edges models
plan transitions without destructive updates.

### Implementation Files

| File                           | Purpose                                                  |
| ------------------------------ | -------------------------------------------------------- |
| `core/graph/types.go`          | Node, Edge, Path, Direction types                        |
| `core/graph/store.go`          | `GraphStore` interface                                   |
| `core/graph/validity.go`       | `Validity`, `Scope`, matching logic                      |
| `core/graph/labels.go`         | SKOS-aligned edge label constants                        |
| `host/storage/graph/sqlite.go` | SQLite adjacency-table reference (framework / CLI)       |
| `bowrain/graph/sql.go`         | `SQLGraphStore` — default plain-SQL backend on PostgreSQL |
| `bowrain/graph/cypher.go`      | `CypherStore` sub-interface                              |
| `bowrain/graph/age.go`         | `AGEGraphStore` — opt-in Apache AGE backend              |
| `bowrain/graph/agtype.go`      | agtype parser                                            |
| `bowrain/graph/afterconnect.go` | pgx `AfterConnect` hook for AGE extension loading       |
| `bowrain/server/postgres.go`   | Backend selection (`BOWRAIN_GRAPH_BACKEND`)              |
| `bowrain/graph/sync.go`        | Event-driven graph sync                                  |
| `bowrain/graph/NOTES.md`       | Backend selection and parity notes                       |

## Consequences

- Concept relationships are first-class graph edges; navigation,
  broader/narrower traversal, and shortest-path queries are natural.
- The default SQL backend runs on stock PostgreSQL (including managed
  RDS/Aurora), so development and production share one image with no
  extension dependency. AGE remains an opt-in upgrade for workloads that
  need native Cypher queries and path algorithms.
- Temporal validity models relationships that change over time (term
  supersession, seasonal terminology, product lifecycle).
- SKOS-aligned labels ensure interoperability with standard terminology
  interchange formats.
- The `CypherStore` sub-interface on the AGE backend enables power users
  to write arbitrary graph queries; core `GraphStore` operations remain
  portable.
- Event-driven sync keeps the graph consistent with relational data
  without manual intervention.
- Backend substitution: a deployment runs on the SQL backend by default and
  can move to AGE (`BOWRAIN_GRAPH_BACKEND=age`) when scaling demands native
  traversal, without changing callers; a `parity_test.go` contract suite
  proves both backends behave identically for the operations bowrain uses.

## Related

- [AD-004: Content Store and Versioning](004-content-store.md)
- [AD-005: Streams](005-streams.md)
- [Framework terminology](https://neokapi.github.io/web/neokapi/framework/terminology)
