---
sidebar_position: 18
title: "Graph Store Schema"
---

# Graph Store Schema

This note provides implementation details for [AD-006](/architecture-decisions/006-graph-concept-storage).

## Framework: SQLite Adjacency Table DDL

```sql
-- cli/storage/graph/sqlite.go
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

## Server: Apache AGE Cypher DDL

AGE uses a property graph model accessed through Cypher queries via `ag_catalog.cypher()`:

```sql
-- Graph creation (bowrain/graph/age.go)
SELECT * FROM ag_catalog.create_graph('bowrain_graph');
```

**Node creation:**

```sql
SELECT * FROM ag_catalog.cypher('bowrain_graph', $$
    CREATE (n:Concept {id: 'c1', name: 'encryption', created_at: '2024-01-01T00:00:00Z', updated_at: '2024-01-01T00:00:00Z'})
    RETURN n
$$) as (v agtype);
```

**Edge creation with validity:**

```sql
SELECT * FROM ag_catalog.cypher('bowrain_graph', $$
    MATCH (a {id: 'c1'}), (b {id: 'c2'})
    CREATE (a)-[r:BROADER {id: 'e1', source: 'c1', target: 'c2',
        valid_from: '2024-01-01T00:00:00Z',
        tags: '{"market":"us"}',
        created_at: '2024-01-01T00:00:00Z',
        updated_at: '2024-01-01T00:00:00Z'}]->(b)
    RETURN r
$$) as (e agtype);
```

**Traversal with direction:**

```sql
-- Outgoing neighbors
SELECT * FROM ag_catalog.cypher('bowrain_graph', $$
    MATCH (n {id: 'c1'})-[r:BROADER]->(m) RETURN m
$$) as (v agtype);

-- Incoming neighbors
SELECT * FROM ag_catalog.cypher('bowrain_graph', $$
    MATCH (n {id: 'c1'})<-[r:BROADER]-(m) RETURN m
$$) as (v agtype);

-- Both directions, multiple labels
SELECT * FROM ag_catalog.cypher('bowrain_graph', $$
    MATCH (n {id: 'c1'})-[r:BROADER|NARROWER]-(m) RETURN m
$$) as (v agtype);
```

**Scoped traversal (temporal + tags):**

```sql
SELECT * FROM ag_catalog.cypher('bowrain_graph', $$
    MATCH (n {id: 'c1'})-[r]->(m)
    WHERE (r.valid_from IS NULL OR r.valid_from <= '2024-06-01T00:00:00Z')
    AND (r.valid_to IS NULL OR r.valid_to > '2024-06-01T00:00:00Z')
    AND r.tags CONTAINS '"market":"us"'
    RETURN m
$$) as (v agtype);
```

**Shortest path:**

```sql
SELECT * FROM ag_catalog.cypher('bowrain_graph', $$
    MATCH p = shortestPath((a {id: 'c1'})-[*..10]-(b {id: 'c2'}))
    RETURN p
$$) as (p agtype);
```

## Server: agtype Parsing

AGE returns results as `agtype`, a custom PostgreSQL type. The parser in `bowrain/graph/agtype.go` handles three formats:

### Vertex format

```
{"id": 123, "label": "Concept", "properties": {"id": "c1", "name": "encryption"}}::vertex
```

- Strip `::vertex` suffix
- Parse JSON body into `agtypeVertex` struct
- Extract application-level `id` from properties (fall back to AGE internal `id`)
- Convert `map[string]any` properties to `map[string]string`

### Edge format

```
{"id": 456, "label": "BROADER", "start_id": 123, "end_id": 789, "properties": {"id": "e1", "source": "c1", "target": "c2", "valid_from": "2024-01-01T00:00:00Z"}}::edge
```

- Strip `::edge` suffix
- Parse JSON body into `agtypeEdge` struct
- Extract `source`/`target` from properties (fall back to `start_id`/`end_id`)
- Reconstruct `Validity` from `valid_from`, `valid_to`, `tags` properties

### Path format

```
[{...}::vertex, {...}::edge, {...}::vertex]::path
```

- Strip `::path` suffix
- Split array elements by tracking brace depth (commas inside JSON objects are skipped)
- Parse alternating vertex (even indices) and edge (odd indices) elements
- Return assembled `Path` struct

### Scalar values

`ParseScalar` handles: `null`, `true`/`false`, integers, floats, quoted strings.

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
| `EventBlockDeleted` | Delete node (AGE: DETACH DELETE cascades edges)             |

The syncer uses a 10-second context timeout per event and logs errors without failing.

## Implementation Files

### Framework (`core/`, `cli/`)

| File                          | Purpose                           |
| ----------------------------- | --------------------------------- |
| `core/graph/types.go`         | Node, Edge, Path, Direction types |
| `core/graph/store.go`         | GraphStore interface              |
| `core/graph/validity.go`      | Validity, Scope, matching logic   |
| `core/graph/labels.go`        | SKOS-aligned edge label constants |
| `cli/storage/graph/sqlite.go` | SQLite adjacency table backend    |

### Server (`bowrain/`)

| File                             | Purpose                                                  |
| -------------------------------- | -------------------------------------------------------- |
| `bowrain/graph/cypher.go`       | CypherStore sub-interface                                |
| `bowrain/graph/age.go`          | Apache AGE backend (implements GraphStore + CypherStore) |
| `bowrain/graph/agtype.go`       | agtype parser (vertex, edge, path, scalar)               |
| `bowrain/graph/time.go`         | Time formatting helpers for AGE                          |
| `bowrain/graph/afterconnect.go` | pgx AfterConnect hook for AGE extension                  |
| `bowrain/graph/factory.go`      | GraphStore factory                                       |
| `bowrain/graph/sync.go`         | Event-driven graph sync                                  |
