---
id: 026-graph-concept-management
sidebar_position: 26
title: "AD-026: Graph Concept Management"
---

# AD-026: Graph-based concept management

## Context

Terminology management ([AD-010](./010-terminology.md)) uses concept-oriented data with relationships (broader/narrower, related, supersedes). These relationships form a graph that is natural to query, navigate, and visualize. A flat relational model can store relationships but makes traversal queries (find all narrower concepts, shortest path between concepts, scoped neighbors) unnecessarily complex.

Key requirements:

- Abstract graph storage behind a common interface (GraphStore)
- Support two backends: Apache AGE (PostgreSQL extension) for production and SQLite (adjacency tables) for CLI/development
- Model temporal validity on edges (relationships change over time)
- Use SKOS-aligned edge labels for terminology interoperability
- Sync graph state from relational events (content changes)

## Decision

### GraphStore Abstraction

The `GraphStore` interface in `core/graph/store.go` defines a backend-agnostic graph API:

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

    // Bulk operations
    BulkCreateNodes(ctx context.Context, nodes []*Node) error
    BulkCreateEdges(ctx context.Context, edges []*Edge) error

    Close() error
}
```

**Direction** supports `Outgoing`, `Incoming`, and `Both` for edge traversal.

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

Edges carry optional `Validity` (`core/graph/validity.go`) combining temporal bounds and tag-based scoping:

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

**Matching rules** (`Validity.Matches(Scope)`):

- Nil validity always matches (unbounded edge)
- Time: half-open interval `[ValidFrom, ValidTo)`
- Tags: all scope tags must be present in validity tags with matching values (open-world assumption -- extra validity tags are ignored)

Tag dimensions are workspace-configurable via `brand.TagDimension` -- the graph itself has no hard-coded dimensions.

Helper functions: `Now()`, `ScopeAt(t)`, `ScopeWithTags(tags)`, `IsExpired()`, `IsActive()`.

### SKOS Edge Labels

Edge labels in `core/graph/labels.go` are aligned with W3C SKOS vocabulary for terminology interoperability:

| Label         | SKOS/Semantic Origin | Purpose                           |
| ------------- | -------------------- | --------------------------------- |
| `BROADER`     | skos:broader         | Parent concept                    |
| `NARROWER`    | skos:narrower        | Child concept                     |
| `RELATED`     | skos:related         | Associative link                  |
| `PART_OF`     | meronymy             | Component of                      |
| `HAS_PART`    | holonymy             | Contains component                |
| `HAS_TERM`    | terminological       | Concept to term designation       |
| `USE_INSTEAD` | terminological       | Deprecated to preferred term      |
| `REPLACED_BY` | terminological       | Superseded concept to replacement |
| `EXACT_MATCH` | skos:exactMatch      | Cross-scheme equivalence          |
| `CLOSE_MATCH` | skos:closeMatch      | Approximate equivalence           |
| `FORBIDDEN`   | brand voice          | Brand to forbidden term           |
| `PREFERRED`   | brand voice          | Brand to preferred term           |
| `COMPETITOR`  | brand voice          | Brand to competitor term          |

`InverseLabel()` returns the inverse of directional labels (BROADER/NARROWER, PART_OF/HAS_PART).

### Apache AGE Backend (Server)

Server deployments can use an Apache AGE backend (`platform/graph/age.go`) that implements `GraphStore` using PostgreSQL's AGE extension. The AGE backend also implements a `CypherStore` sub-interface that adds native Cypher query support:

```go
// platform/graph/cypher.go
type CypherStore interface {
    graph.GraphStore
    CypherQuery(ctx context.Context, query string, params map[string]any) ([]*graph.Node, error)
    CypherExec(ctx context.Context, query string, params map[string]any) error
}
```

Cypher methods are not part of the core `GraphStore` interface — they are a server-specific extension. Callers type-assert to `CypherStore` when they need Cypher access. This keeps the framework interface clean and fully implementable by the SQLite backend.

See the [Graph Store Schema](/docs/notes/graph-store-schema) implementation note for AGE-specific details (agtype parsing, AfterConnect hook).

### SQLite Backend

`cli/storage/graph/sqlite.go` implements `GraphStore` using adjacency tables and recursive CTEs:

**Schema:**

```sql
CREATE TABLE graph_nodes (
    id TEXT PRIMARY KEY,
    label TEXT NOT NULL,
    properties TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE graph_edges (
    id TEXT PRIMARY KEY,
    source TEXT NOT NULL REFERENCES graph_nodes(id),
    target TEXT NOT NULL REFERENCES graph_nodes(id),
    label TEXT NOT NULL,
    properties TEXT NOT NULL DEFAULT '{}',
    valid_from TEXT,
    valid_to TEXT,
    tags TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);
```

Indexes on `source`, `target`, and `label` columns for efficient traversal.

**Key implementation details:**

- Properties stored as JSON, queried via `json_extract()`
- Validity filtering done in Go after edge retrieval (for scoped queries)
- `ShortestPath` uses a recursive CTE with BFS, tracking visited nodes to avoid cycles
- Bulk operations use transactions with prepared statements

### Event-Driven Graph Sync (Server)

Server deployments include a `GraphSyncer` (`platform/graph/sync.go`) that subscribes to the event bus ([AD-011](./011-automation.md)) and keeps the AGE graph in sync with relational content changes (block create/update/delete events).

### Terminology Integration

The `ConceptRelation` type in `termbase/termbase.go` bridges the termbase and graph systems:

```go
type ConceptRelation struct {
    SourceID     string `json:"source_id"`
    TargetID     string `json:"target_id"`
    RelationType string `json:"relation_type"` // Uses graph.Label* constants
}
```

`TermDesignation` pairs a `Term` with a `Validity` for the status-on-edge model, where term lifecycle status can be time-bounded.

## Alternatives Considered

- **Native graph database (Neo4j, DGraph)**: Adds deployment complexity and a separate database dependency. Apache AGE runs as a PostgreSQL extension on the existing database, requiring no additional infrastructure.

- **Graph operations in SQL only**: Recursive CTEs handle simple traversals but become unwieldy for complex graph patterns. The AGE backend's `CypherStore` sub-interface provides full graph query power when needed.

- **Hard-coded validity dimensions**: The tag-based model with workspace-configurable dimensions is more flexible than fixed fields like `market`, `product`, `channel`. It supports any scoping vocabulary without schema changes.

- **Separate graph sync service**: An event-driven syncer integrated into the existing event bus is simpler than a separate sync process. The syncer runs in-process with the server.

## Consequences

- Concept relationships are first-class graph edges, enabling natural navigation (broader/narrower hierarchies, related concept exploration, shortest path between concepts)
- Two backends serve different deployment needs: AGE for production with native Cypher queries, SQLite for CLI and development with no external dependencies
- Temporal validity enables modeling of relationships that change over time (term supersession, seasonal terminology, product lifecycle)
- SKOS-aligned labels ensure interoperability with standard terminology interchange formats
- The `CypherStore` sub-interface on the AGE backend allows power users to write complex graph queries directly, while the core `GraphStore` API covers common operations portably
- Event-driven sync keeps the graph consistent with relational data without manual intervention
- The `GraphStore` interface enables backend substitution -- a project can start with SQLite and migrate to AGE when needed
