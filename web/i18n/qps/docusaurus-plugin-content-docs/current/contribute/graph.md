---
sidebar_position: 12
title: Graph Store Library
description: The neokapi graph store library provides a backend-agnostic graph database abstraction for concept management, with a SQLite backend for local use and an interface designed for extension to server-side backends.
keywords: [graph store, SQLite, concept management, GraphStore interface, neokapi]
---

import { PhaseFlow } from "@neokapi/docs-shared";

# ▒ Ĝŕàþĥ Šţöŕé Ļîƃŕàŕý ▒

▒ Ţĥé ĝŕàþĥ šţöŕé ļîƃŕàŕý (`çöŕé/ĝŕàþĥ/`) þŕöṽîđéš à ƃàçķéñđ-àĝñöšţîç ĝŕàþĥ đàţàƃàšé àƃšţŕàçţîöñ ƒöŕ çöñçéþţ ḿàñàĝéḿéñţ. Ţĥé ƒŕàḿéŵöŕķ îñçļüđéš à ŠǪĻîţé ƃàçķéñđ (àđĵàçéñçý ţàƃļéš) ƒöŕ ļöçàļ àñđ ÇĻÎ üšé. Ţĥé îñţéŕƒàçé îš đéšîĝñéđ ƒöŕ éẋţéñšîöñ, šö šéŕṽéŕ đéþļöýḿéñţš çàñ àđđ ţĥéîŕ öŵñ ƃàçķéñđ ƃéĥîñđ ţĥé šàḿé îñţéŕƒàçé. ▒

## ▒ Àŕçĥîţéçţüŕé ▒

<PhaseFlow
  nodes={[
    { label: "GraphStore", sub: "backend-agnostic interface", role: "io" },
    {
      label: "SQLite backend",
      sub: "graphstore.NewSQLiteGraphStore",
      edge: "implemented by",
    },
    {
      label: "graph.db",
      sub: "graph_nodes · graph_edges",
      role: "io",
      edge: "reads and writes",
    },
  ]}
/>

▒ Ţŵö ŕéçöŕđ ţýþéš ţŕàṽéļ ţĥŕöüĝĥ ţĥàţ îñţéŕƒàçé. À **ñöđé** çàŕŕîéš àñ ÎĐ, à
ļàƃéļ, àñđ à ḿàþ öƒ þŕöþéŕţîéš. Àñ **éđĝé** ĵöîñš ţŵö ñöđéš üñđéŕ à ļàƃéļ àñđ
ḿàý çàŕŕý à **ṽàļîđîţý** — à `ṼàļîđƑŕöḿ`/`ṼàļîđŢö` îñţéŕṽàļ þļüš à šéţ öƒ ţàĝš —
ŵĥîçĥ îš ŵĥàţ ļéţš à ǫüéŕý àšķ ţĥé ĝŕàþĥ à ǫüéšţîöñ àţ à þöîñţ îñ ţîḿé àñđ
ŵîţĥîñ à šçöþé. Ƃöţĥ àŕé đéƒîñéđ îñ [Ķéý Ţýþéš](#key-types) ƃéļöŵ. ▒

## ▒ ĜŕàþĥŠţöŕé Îñţéŕƒàçé ▒

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

    // Cypher escape hatch (AGE backend only; SQLite returns ErrCypherNotSupported)
    CypherQuery(ctx context.Context, query string, params map[string]any) ([]*Node, error)
    CypherExec(ctx context.Context, query string, params map[string]any) error

    // Lifecycle
    Close() error
}
```

## ▒ Ķéý Ţýþéš ▒

### ▒ Ñöđé ▒

```go
type Node struct {
    ID         string            `json:"id"`
    Label      string            `json:"label"`
    Properties map[string]string `json:"properties"`
    CreatedAt  time.Time         `json:"created_at"`
    UpdatedAt  time.Time         `json:"updated_at"`
}
```

### ▒ Éđĝé ▒

```go
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
```

### ▒ Đîŕéçţîöñ ▒

```go
type Direction int
const (
    Outgoing Direction = iota  // source -> target
    Incoming                    // source <- target
    Both                        // either direction
)
```

### ▒ Þàţĥ ▒

```go
type Path struct {
    Nodes []Node `json:"nodes"`
    Edges []Edge `json:"edges"`
}
```

## ▒ Ţéḿþöŕàļ Ṽàļîđîţý ▒

▒ Éđĝéš çàñ çàŕŕý ţéḿþöŕàļ ƃöüñđš àñđ ţàĝ-ƃàšéđ šçöþîñĝ: ▒

```go
type Validity struct {
    ValidFrom *time.Time        `json:"valid_from,omitempty"`
    ValidTo   *time.Time        `json:"valid_to,omitempty"`
    Tags      map[string]string `json:"tags,omitempty"`
}
```

### ▒ Šçöþé Ḿàţçĥîñĝ ▒

▒ À `Šçöþé` ŕéþŕéšéñţš àñ éṽàļüàţîöñ þöîñţ: ▒

```go
type Scope struct {
    At   time.Time         `json:"at"`
    Tags map[string]string `json:"tags,omitempty"`
}
```

▒ Ḿàţçĥîñĝ ŕüļéš: ▒

- ▒ Ñîļ ṽàļîđîţý àļŵàýš ḿàţçĥéš (üñƃöüñđéđ éđĝé) ▒
- ▒ Ţîḿé: ĥàļƒ-öþéñ îñţéŕṽàļ `[ṼàļîđƑŕöḿ, ṼàļîđŢö)` ▒
- ▒ Ţàĝš: àļļ šçöþé ţàĝš ḿüšţ ƃé þŕéšéñţ îñ ṽàļîđîţý ţàĝš ŵîţĥ ḿàţçĥîñĝ ṽàļüéš ▒
- ▒ Éẋţŕà ṽàļîđîţý ţàĝš ñöţ îñ šçöþé àŕé îĝñöŕéđ (öþéñ-ŵöŕļđ àššüḿþţîöñ) ▒

```go
import "github.com/neokapi/neokapi/core/graph"

// Query with current time, no tag constraints
scope := graph.Now()

// Query at a specific point in time
scope := graph.ScopeAt(time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC))

// Query with tag constraints
scope := graph.ScopeWithTags(map[string]string{"market": "us", "product": "enterprise"})

// Check if validity is currently active
edge.Validity.IsActive()

// Check if validity has expired
edge.Validity.IsExpired()
```

## ▒ Éđĝé Ļàƃéļš ▒

▒ Ļàƃéļš àŕé àļîĝñéđ ŵîţĥ Ŵ3Ç ŠĶÖŠ ṽöçàƃüļàŕý ƒöŕ ţéŕḿîñöļöĝý îñţéŕöþéŕàƃîļîţý: ▒

```go
// Hierarchical (SKOS)
graph.LabelBroader   // "BROADER"  — parent concept
graph.LabelNarrower  // "NARROWER" — child concept

// Associative (SKOS)
graph.LabelRelated   // "RELATED"  — associative link

// Compositional
graph.LabelPartOf    // "PART_OF"  — component of
graph.LabelHasPart   // "HAS_PART" — contains component

// Terminological
graph.LabelHasTerm     // "HAS_TERM"     — concept → term
graph.LabelUseInstead  // "USE_INSTEAD"  — deprecated → preferred
graph.LabelReplacedBy  // "REPLACED_BY"  — superseded → replacement

// Equivalence (SKOS)
graph.LabelExactMatch  // "EXACT_MATCH" — cross-scheme equivalence
graph.LabelCloseMatch  // "CLOSE_MATCH" — approximate equivalence

// Voice profile
graph.LabelForbidden   // "FORBIDDEN"  — voice → forbidden term
graph.LabelPreferred   // "PREFERRED"  — voice → preferred term
graph.LabelCompetitor  // "COMPETITOR" — voice → competitor term
```

▒ `ÎñṽéŕšéĻàƃéļ()` ŕéţüŕñš ţĥé îñṽéŕšé öƒ đîŕéçţîöñàļ ļàƃéļš (é.ĝ., `ƂŔÖÀĐÉŔ` -> `ÑÀŔŔÖŴÉŔ`). ▒

## ▒ ŠǪĻîţé Ƃàçķéñđ ▒

```go
import (
    "github.com/neokapi/neokapi/core/storage"
    graphstore "github.com/neokapi/neokapi/cli/storage/graph"
)

db, _ := storage.Open("graph.db")
store, _ := graphstore.NewSQLiteGraphStore(db)
defer store.Close()
```

▒ Üšéš àđĵàçéñçý ţàƃļéš (`ĝŕàþĥ_ñöđéš`, `ĝŕàþĥ_éđĝéš`) ŵîţĥ ĴŠÖÑ þŕöþéŕţîéš. Šĥöŕţéšţ þàţĥ üšéš ŕéçüŕšîṽé ÇŢÉ ŵîţĥ ƂƑŠ. Šçöþéđ ǫüéŕîéš ƒîļţéŕ éđĝéš îñ Ĝö àƒţéŕ ŕéţŕîéṽàļ. ▒

▒ Ţĥé ŠǪĻîţé ƃàçķéñđ ĥàš ñö ñàţîṽé Çýþĥéŕ šüþþöŕţ, šö `ÇýþĥéŕǪüéŕý` àñđ `ÇýþĥéŕÉẋéç` ŕéţüŕñ ţĥé šéñţîñéļ `ĝŕàþĥ.ÉŕŕÇýþĥéŕÑöţŠüþþöŕţéđ`. À šéŕṽéŕ-šîđé đéþļöýḿéñţ çàñ šüþþļý à ƃàçķéñđ ŵîţĥ ñàţîṽé Çýþĥéŕ šüþþöŕţ ƃéĥîñđ ţĥé šàḿé îñţéŕƒàçé. ▒

▒ Ţĥé `ĜŕàþĥŠţöŕé` îñţéŕƒàçé îš đéšîĝñéđ ƒöŕ éẋţéñšîöñ — šéŕṽéŕ đéþļöýḿéñţš çàñ àđđ ţĥéîŕ öŵñ ƃàçķéñđ ƃéĥîñđ ţĥé šàḿé îñţéŕƒàçé. ▒

## ▒ Üšàĝé Éẋàḿþļéš ▒

### ▒ Ƃüîļđîñĝ à Çöñçéþţ Ĥîéŕàŕçĥý ▒

```go
store, _ := graphstore.NewSQLiteGraphStore(db)

// Create concept nodes
store.CreateNode(ctx, &graph.Node{ID: "animal", Label: "Concept", Properties: map[string]string{"name": "Animal"}})
store.CreateNode(ctx, &graph.Node{ID: "mammal", Label: "Concept", Properties: map[string]string{"name": "Mammal"}})
store.CreateNode(ctx, &graph.Node{ID: "dog", Label: "Concept", Properties: map[string]string{"name": "Dog"}})

// Create hierarchy edges
store.CreateEdge(ctx, &graph.Edge{ID: "e1", Source: "mammal", Target: "animal", Label: graph.LabelBroader})
store.CreateEdge(ctx, &graph.Edge{ID: "e2", Source: "dog", Target: "mammal", Label: graph.LabelBroader})

// Navigate: what is broader than "dog"?
parents, _ := store.Neighbors(ctx, "dog", graph.Outgoing, graph.LabelBroader)
// parents = [mammal]

// Navigate: what is narrower than "animal"?
children, _ := store.Neighbors(ctx, "animal", graph.Incoming, graph.LabelBroader)
// children = [mammal]

// Find path from dog to animal
path, _ := store.ShortestPath(ctx, "dog", "animal", 10)
// path.Nodes = [dog, mammal, animal]
// path.Edges = [e2, e1]
```

### ▒ Ţéḿþöŕàļ Éđĝéš ▒

```go
start := time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC)
end := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC)

store.CreateEdge(ctx, &graph.Edge{
    ID: "e3", Source: "old-term", Target: "new-term", Label: graph.LabelReplacedBy,
    Validity: &graph.Validity{
        ValidFrom: &start,
        ValidTo:   &end,
        Tags:      map[string]string{"market": "us"},
    },
})

// Query with scope — only returns edges active at the given time with matching tags
scope := graph.Scope{At: time.Date(2024, 6, 1, 0, 0, 0, 0, time.UTC), Tags: map[string]string{"market": "us"}}
neighbors, _ := store.NeighborsScoped(ctx, "old-term", graph.Outgoing, scope, graph.LabelReplacedBy)
```

▒ Üšé `ƑîñđÑöđéš`, `Ñéîĝĥƃöŕš`, àñđ `ŠĥöŕţéšţÞàţĥ` ƒöŕ þöŕţàƃļé ǫüéŕîéš ţĥàţ ŵöŕķ àçŕöšš àļļ ƃàçķéñđš. ▒
