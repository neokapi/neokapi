---
id: 006-graph-concept-storage
sidebar_position: 6
title: "AD-006: Graph Concept Storage"
---

# AD-006: Graph Concept Storage

## Summary

Bowrain stores concept relationships, brand vocabulary networks, and
billing metadata links as a graph. The `GraphStore` interface has one
server backend: plain-SQL adjacency tables on standard PostgreSQL
(`SQLGraphStore`). The graph is optional and derived — a `GraphSyncer`
rebuilds it from relational events — and no consumer issues raw Cypher, so
the backend runs on stock PostgreSQL with no extension. Edges carry
temporal validity and tag-based scoping. (An opt-in Apache AGE backend
existed historically and was removed; see
[History](#history-the-apache-age-backend).)

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

### SQL Backend

The server backend, `SQLGraphStore` in `bowrain/graph/sql.go`, implements
`GraphStore` on standard PostgreSQL using plain relational adjacency tables
(`graph_nodes`, `graph_edges`) — no extension and no Cypher. It is a dialect
port of the framework's SQLite reference (`host/storage/graph/sqlite.go`):
the same node/edge CRUD, jsonb property containment, temporal
`Scope`/`Validity` filtering (evaluated in Go via `Validity.Matches`),
directional neighbours, and a bounded, cycle-safe recursive-CTE
`ShortestPath`.

Because the graph is optional and derived and needs no native traversal,
plain SQL means both local development and managed RDS/Aurora run on the
same stock `postgres:16` image. `CypherQuery` / `CypherExec` return
`core/graph.ErrCypherNotSupported`. `EnsureGraph` creates the schema at
wiring time (`bowrain/server/postgres.go`), under a transaction-scoped
advisory lock so replicas cold-starting against one fresh database serialise
their DDL rather than race. A schema-setup failure degrades to a nil
`GraphStore` (brand-graph features disabled) rather than failing server
startup.

A contract suite (`bowrain/graph/parity_test.go`) pins the operations
bowrain uses against a real PostgreSQL testcontainer. See
`bowrain/graph/NOTES.md` for behavior notes.

### History: the Apache AGE backend

An opt-in backend on PostgreSQL's Apache AGE extension (`AGEGraphStore`,
selected with `BOWRAIN_GRAPH_BACKEND=age`) existed alongside the SQL store,
with a `CypherStore` sub-interface for native Cypher, an `agtype` result
parser, and a `pgx` `AfterConnect` search-path hook. It was removed: AGE is
unavailable on managed RDS/Aurora, no consumer ever issued raw Cypher, and
carrying a second backend cost contract-suite and image maintenance with no
production user. If a workload ever needs native graph traversal (deep
multi-hop Cypher, path algorithms), a native graph engine is the upgrade
path — implemented behind the same `core/graph.GraphStore` interface, whose
`CypherQuery`/`CypherExec` escape hatch remains for such a backend to fill.

### SQLite reference (framework)

The `SQLGraphStore` schema is a dialect port of the framework's SQLite
reference, `host/storage/graph/sqlite.go`, which the CLI and single-instance
tooling use with no external database. Both implement the same `GraphStore`
with adjacency tables (`graph_nodes`, `graph_edges`), JSON/jsonb-encoded
properties, nullable RFC3339 validity bounds, Go-side tag filtering for scoped
queries, and a recursive-CTE cycle-safe `ShortestPath`. The one behavioural
difference: the SQL/Postgres backend cascades edge deletes
(`ON DELETE CASCADE`), whereas the SQLite reference errors on deleting a
node that still has incident edges.

### Event-Driven Graph Sync

The server-side `GraphSyncer` in `bowrain/graph/sync.go` subscribes to
the event bus and keeps the graph in sync with relational content
changes. It is backend-agnostic — it takes a `core/graph.GraphStore`:

| Event               | Action                                                      |
| ------------------- | ----------------------------------------------------------- |
| `EventBlockCreated` | Create Concept node with `project_id` and `name` properties |
| `EventBlockUpdated` | Update node properties                                      |
| `EventBlockDeleted` | Delete node (incident edges cascade via `ON DELETE CASCADE`) |

The syncer uses a 10-second context timeout per event and logs errors
without failing — graph inconsistency is recoverable; blocking event
processing is not. Because the graph is a derived projection, it can be
rebuilt from the relational stores at any time.

### Terminology Integration

The `ConceptRelation` type in the `terms` package bridges the terms store and
the graph:

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
concept relationships reflected in the graph without direct terms →
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
| `bowrain/graph/sql.go`         | `SQLGraphStore` — plain-SQL backend on PostgreSQL        |
| `bowrain/server/postgres.go`   | Store wiring (`EnsureGraph` at pool-open time)           |
| `bowrain/graph/sync.go`        | Event-driven graph sync                                  |
| `bowrain/graph/NOTES.md`       | Backend and behavior notes                               |

## Consequences

- Concept relationships are first-class graph edges; navigation,
  broader/narrower traversal, and shortest-path queries are natural.
- The SQL backend runs on stock PostgreSQL (including managed RDS/Aurora),
  so development and production share one image with no extension
  dependency.
- Temporal validity models relationships that change over time (term
  supersession, seasonal terminology, product lifecycle).
- SKOS-aligned labels ensure interoperability with standard terminology
  interchange formats.
- Event-driven sync keeps the graph consistent with relational data
  without manual intervention.
- Backend substitution stays open: callers depend only on
  `core/graph.GraphStore`, so a native graph engine can be introduced behind
  the same interface if scaling ever demands native traversal; the
  `parity_test.go` contract suite is the behavioral spec a new backend must
  pass.

## Related

- [AD-004: Content Store and Versioning](004-content-store.md)
- [AD-005: Streams](005-streams.md)
- [Framework terminology](https://neokapi.github.io/framework/terminology)
