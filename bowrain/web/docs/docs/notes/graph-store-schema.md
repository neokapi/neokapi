---
sidebar_position: 18
title: "Graph Store Schema"
---

# Graph Store Schema

This note provides implementation details for [AD-006](/architecture-decisions/006-graph-concept-storage).

The server runs the plain-SQL backend on standard PostgreSQL
(`SQLGraphStore`), a port of the framework's SQLite adjacency-table
reference. See `bowrain/graph/NOTES.md` for behavior notes.

## Framework: SQLite Adjacency Table DDL

```sql
-- host/storage/graph/sqlite.go
CREATE TABLE IF NOT EXISTS graph_nodes (
    id TEXT PRIMARY KEY,
    label TEXT NOT NULL,
    properties TEXT NOT NULL DEFAULT '{}',
    created_at TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS graph_edges (
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

CREATE INDEX IF NOT EXISTS idx_graph_edges_source ON graph_edges(source);
CREATE INDEX IF NOT EXISTS idx_graph_edges_target ON graph_edges(target);
CREATE INDEX IF NOT EXISTS idx_graph_edges_label ON graph_edges(label);
CREATE INDEX IF NOT EXISTS idx_graph_nodes_label ON graph_nodes(label);
```

**Design notes:**

- Properties stored as JSON, queried via `json_extract(properties, '$.key')`
- Validity fields (`valid_from`, `valid_to`) are nullable TEXT (RFC3339 format)
- Tags stored as JSON object, parsed in Go for scope matching
- Foreign keys enforce referential integrity on edges
- Indexes on source, target, label enable efficient traversal queries

## Server: SQL Adjacency Tables on PostgreSQL

The server backend, `SQLGraphStore` (`bowrain/graph/sql.go`), ports the
SQLite reference above to standard PostgreSQL: `graph_nodes` / `graph_edges`
with `jsonb` properties (queried via containment), nullable RFC3339 validity
bounds, Go-side tag filtering, and a recursive-CTE `ShortestPath`. It needs
no extension, so it runs on stock PostgreSQL and managed RDS/Aurora.
`EnsureGraph` creates the schema at wiring time under an advisory lock, and
`graph_edges` foreign keys use `ON DELETE CASCADE`. Raw Cypher is not
supported (`CypherQuery`/`CypherExec` return
`core/graph.ErrCypherNotSupported`).

## History: Apache AGE Cypher DDL and agtype parsing

The removed opt-in AGE backend accessed a property graph through Cypher via
`ag_catalog.cypher()` and parsed AGE's custom `agtype` results (vertex, edge,
path, scalar). Its DDL, query patterns, and parser details live in git history
(`bowrain/graph/age.go`, `agtype.go`, `cypher.go`, `afterconnect.go`).

## Framework: SQLite Shortest Path (Recursive CTE)

The SQLite backend implements `ShortestPath` using BFS via a recursive CTE:

```sql
WITH RECURSIVE bfs(node, depth, path_nodes, path_edges) AS (
    SELECT ?, 0, ?, ''
    UNION ALL
    SELECT
        CASE WHEN e.source = bfs.node THEN e.target ELSE e.source END,
        bfs.depth + 1,
        bfs.path_nodes || ',' || CASE WHEN e.source = bfs.node THEN e.target ELSE e.source END,
        CASE WHEN bfs.path_edges = '' THEN e.id ELSE bfs.path_edges || ',' || e.id END
    FROM bfs
    JOIN graph_edges e ON (e.source = bfs.node OR e.target = bfs.node)
    WHERE bfs.depth < ?
      AND instr(bfs.path_nodes, CASE WHEN e.source = bfs.node THEN e.target ELSE e.source END) = 0
)
SELECT path_nodes, path_edges FROM bfs WHERE node = ? LIMIT 1
```

**Algorithm:**

1. Start from the source node (depth 0)
2. Expand to neighbors via joined edges, tracking visited nodes via string concatenation
3. Cycle detection via `instr(path_nodes, ...)` check
4. Stop at max depth or when target is found
5. Result is comma-separated node IDs and edge IDs, resolved to full objects after the CTE query completes

## Server: Event-Driven Graph Sync

`GraphSyncer` in `bowrain/graph/sync.go` subscribes to the event bus and maintains graph consistency:

| Event               | Action                                                      |
| ------------------- | ----------------------------------------------------------- |
| `EventBlockCreated` | Create Concept node with `project_id` and `name` properties |
| `EventBlockUpdated` | Update node properties                                      |
| `EventBlockDeleted` | Delete node (incident edges cascade via `ON DELETE CASCADE`) |

The syncer is backend-agnostic (it takes a `core/graph.GraphStore`), uses a
10-second context timeout per event, and logs errors without failing. The
graph is a derived projection, so it can be rebuilt from the relational
stores at any time.

## Implementation Files

### Framework (`core/`, `cli/`)

| File                          | Purpose                           |
| ----------------------------- | --------------------------------- |
| `core/graph/types.go`         | Node, Edge, Path, Direction types |
| `core/graph/store.go`         | GraphStore interface              |
| `core/graph/validity.go`      | Validity, Scope, matching logic   |
| `core/graph/labels.go`        | SKOS-aligned edge label constants |
| `host/storage/graph/sqlite.go` | SQLite adjacency-table reference   |

### Server (`bowrain/`)

| File                             | Purpose                                                  |
| -------------------------------- | -------------------------------------------------------- |
| `bowrain/graph/sql.go`          | `SQLGraphStore` — plain-SQL backend on PostgreSQL        |
| `bowrain/server/postgres.go`    | Store wiring (`EnsureGraph` at pool-open time)           |
| `bowrain/graph/sync.go`         | Event-driven graph sync                                  |
| `bowrain/graph/NOTES.md`        | Backend and behavior notes                               |
