---
sidebar_position: 18
title: "Note: Knowledge graph data model"
---

# Note: Knowledge graph data model

Implementation reference for [AD-021](../architecture-decisions/021-brand-knowledge-graph.md).
Schemas, Go types, REST routes, change-set op types, and permission mapping.

## Framework layer (Apache, `termbase/`)

### Persisted relations

`ConceptRelation` gains identity, a note, and validity, and is persisted by
every termbase backend:

```go
type ConceptRelation struct {
    ID           string          `json:"id"`
    SourceID     string          `json:"source_id"`
    TargetID     string          `json:"target_id"`
    RelationType string          `json:"relation_type"` // graph.Label* constants
    Note         string          `json:"note,omitempty"`
    Validity     *graph.Validity `json:"validity,omitempty"`
    CreatedAt    time.Time       `json:"created_at"`
}
```

`TermBase` interface additions:

```go
AddRelation(ctx, rel ConceptRelation) error            // upsert by ID
DeleteRelation(ctx, id string) error
RelationsOf(ctx, conceptID string, scope *graph.Scope) ([]ConceptRelation, error) // both directions
ListRelations(ctx, scope *graph.Scope) ([]ConceptRelation, error)
```

SQLite baseline schema (framework `termbase/sqlite.go` — part of the base
migration; the platform is not live, so the schema is authored as if designed
this way from the start):

```sql
CREATE TABLE tb_relations (
  id          TEXT PRIMARY KEY,
  source_id   TEXT NOT NULL REFERENCES tb_concepts(id) ON DELETE CASCADE,
  target_id   TEXT NOT NULL REFERENCES tb_concepts(id) ON DELETE CASCADE,
  relation    TEXT NOT NULL,
  note        TEXT NOT NULL DEFAULT '',
  valid_from  TEXT,            -- RFC3339, NULL = unbounded
  valid_to    TEXT,
  tags        TEXT NOT NULL DEFAULT '{}',  -- JSON object
  created_at  TEXT NOT NULL
);
CREATE INDEX idx_tb_relations_source ON tb_relations(source_id);
CREATE INDEX idx_tb_relations_target ON tb_relations(target_id);
```

### Term validity

`Term` gains `Validity *graph.Validity` (market/time scoping of a designation
or of a status). Stored as three columns on `tb_terms`
(`valid_from`, `valid_to`, `tags`) in SQLite and PostgreSQL. Lookup gains
`Scope *graph.Scope` in `LookupOptions`; a nil scope means "now, no tags".

### Status transition policy

`termbase.ValidateTransition(from, to model.TermStatus) error` plus
`termbase.IsGovernedTransition(from, to) bool`. Governed transitions (require a
change-set on the platform): any transition **to** `forbidden` or `preferred`,
and any transition **from** `forbidden`. Disallowed outright: none (history is
the guard, not a trap), but `forbidden → preferred` must pass through a
governed change-set even on the platform's direct-edit path.

### klftb

The file carries a `relations` array alongside `concepts`, emitted
deterministically (sorted by ID) under the existing schema version.

## Platform layer (AGPL)

### PostgreSQL termbase parity

`bowrain/termbase/postgres.go` carries the same relations table
(workspace-scoped: `PRIMARY KEY (workspace_id, id)`) and validity columns on
`tb_terms` as part of its baseline schema.

### New package `bowrain/knowledge`

A PostgreSQL store (`NewPostgresKnowledgeStore`, migration namespace
`knowledge_schema_migrations`), mirroring the brand store: bowrain-server runs
exclusively on PostgreSQL, and the knowledge graph is a server-side governance
subsystem with no standalone-kapi or desktop-cache use. The change-set state
machine, op validation, governed/ordinary classification, separation-of-duties
merge gate, and conflict detection are pure functions in
`bowrain/knowledge/changeset.go`, unit-tested without a database; the SQL layer
adds compile-time interface checks, scan round-trips, and `//go:build
integration` CRUD tests. Tables (`PRIMARY KEY (workspace_id, id)` where
workspace-scoped; `TIMESTAMPTZ` timestamps, `JSONB` for snapshot/payload/locales):

```sql
kg_markets (
  workspace_id TEXT, id TEXT, name TEXT, description TEXT NOT NULL DEFAULT '',
  locales JSONB NOT NULL DEFAULT '[]',     -- array of locale IDs
  created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ,
  PRIMARY KEY (workspace_id, id)
)

kg_observations (
  workspace_id TEXT, id TEXT, concept_id TEXT,
  kind TEXT,                               -- competitor|customer|style_guide|regulatory|web|internal
  quote TEXT, source TEXT, url TEXT NOT NULL DEFAULT '',
  locale TEXT NOT NULL DEFAULT '', market TEXT NOT NULL DEFAULT '',
  note TEXT NOT NULL DEFAULT '',
  created_by TEXT, created_at TIMESTAMPTZ,
  PRIMARY KEY (workspace_id, id)
)

kg_comments (
  workspace_id TEXT, id TEXT, concept_id TEXT,
  parent_id TEXT NOT NULL DEFAULT '',      -- threaded; empty = top-level
  changeset_id TEXT NOT NULL DEFAULT '',   -- set when the thread belongs to a change-set
  body TEXT, author TEXT, created_at TIMESTAMPTZ,
  resolved BOOLEAN NOT NULL DEFAULT FALSE,
  PRIMARY KEY (workspace_id, id)
)

kg_concept_revisions (
  workspace_id TEXT, concept_id TEXT, rev BIGINT,
  snapshot JSONB,                          -- termbase.Concept + relations delta
  summary TEXT,                            -- human-readable change summary
  actor TEXT, changeset_id TEXT NOT NULL DEFAULT '',
  created_at TIMESTAMPTZ,
  PRIMARY KEY (workspace_id, concept_id, rev)
)

kg_changesets (
  workspace_id TEXT, id TEXT, name TEXT, description TEXT NOT NULL DEFAULT '',
  status TEXT NOT NULL DEFAULT 'draft',    -- draft|in_review|approved|merged|abandoned
  created_by TEXT, created_at TIMESTAMPTZ, updated_at TIMESTAMPTZ,
  submitted_at TIMESTAMPTZ, merged_at TIMESTAMPTZ, merged_by TEXT NOT NULL DEFAULT '',
  PRIMARY KEY (workspace_id, id)
)

kg_changeset_ops (
  workspace_id TEXT, changeset_id TEXT, seq BIGINT,
  op TEXT,                                 -- op type, see below
  payload JSONB,                           -- op-specific
  base_rev BIGINT NOT NULL DEFAULT 0,      -- concept revision the op was authored against
  created_by TEXT, created_at TIMESTAMPTZ,
  PRIMARY KEY (workspace_id, changeset_id, seq)
)

kg_changeset_reviews (
  workspace_id TEXT, changeset_id TEXT, reviewer TEXT,
  verdict TEXT,                            -- approve|reject
  comment TEXT NOT NULL DEFAULT '', created_at TIMESTAMPTZ,
  PRIMARY KEY (workspace_id, changeset_id, reviewer)
)

kg_pilots (
  workspace_id TEXT, changeset_id TEXT, project_id TEXT, stream TEXT,
  created_by TEXT, created_at TIMESTAMPTZ,
  PRIMARY KEY (workspace_id, changeset_id, project_id, stream)
)
```

### Change-set op types

`op` + JSON payload; each op is self-contained and re-validated at merge:

| op | payload |
|---|---|
| `concept.create` | full `termbase.Concept` |
| `concept.update` | `{concept_id, domain?, definition?, properties?}` |
| `concept.delete` | `{concept_id}` |
| `term.add` | `{concept_id, term}` |
| `term.update` | `{concept_id, locale, text, term}` (locale+text identify) |
| `term.remove` | `{concept_id, locale, text}` |
| `term.status` | `{concept_id, locale, text, from, to, validity?}` — governed when `IsGovernedTransition` |
| `relation.add` | full `ConceptRelation` — governed when relation is `REPLACED_BY` |
| `relation.remove` | `{relation_id}` |
| `voice.rule.add` | `{profile_id, list: preferred\|forbidden\|competitor, rule}` (rule carries `concept_id`) — governed |
| `voice.rule.remove` | `{profile_id, list, term}` — governed |

A change-set containing **any** governed op requires `in_review → approved`
(≥1 approval, approver ≠ author) before merge. A change-set with only ordinary
ops may merge directly from `draft` by its author.

**Merge** applies ops in sequence inside one transaction per backend store:
termbase ops via the workspace termbase, voice ops via the brand store
(version-bumping profiles exactly like AD-019 promotion). Before applying, each
op's `base_rev` is compared with the concept's current revision; a mismatch
marks the op conflicted and blocks the merge with a per-op conflict report
(re-basing is: reopen the op, re-validate, resubmit). Merge emits one revision
per touched concept (`changeset_id` set), then deletes pilot shadows.

**Pilot** copies the draft's resulting concepts into the project stream's
termbase shadow (`AddConceptWithStream`) and, for voice ops, sets the stream's
brand-voice binding property to a candidate profile built with
`brand.CandidateWithRule` semantics. Abandon/merge removes shadows and restores
stream properties.

### Blast radius

`knowledge.EvaluateChangeSet(ctx, ws, cs) (*ChangeSetImpact, error)` walks
stored blocks per project/stream (default `main` plus pilot streams), running
the before/after vocabulary + term-enforcement matchers, and returns:

```go
type ChangeSetImpact struct {
    TotalBlocks   int
    AffectedBlocks int
    NewViolations  int
    Resolved       int
    Words          int            // word count of affected blocks
    Projects       []ProjectImpact // per project → per collection → per locale counts
    Samples        []BlockSample   // capped sample of affected blocks
}
```

Concept-level "where used" (`GET /concepts/:cid/blast-radius`) is the same
walk filtered to one concept's terms, without a candidate side.

### Events

New event types (wired through the platform event bus → audit chain,
notifications, SSE, desktop watch):
`concept.created`, `concept.updated`, `concept.deleted`,
`concept.term.status_changed`, `concept.relation.added`,
`concept.relation.removed`, `observation.added`, `concept.comment.added`,
`changeset.created`, `changeset.submitted`, `changeset.approved`,
`changeset.rejected`, `changeset.merged`, `changeset.abandoned`,
`pilot.started`, `pilot.stopped`.

## REST API

Workspace-scoped, AD-011 conventions. `/:ws/concepts` is the terminology API:
it replaces the former `/:ws/terms` routes, and every consumer (web, desktop,
Pulse, MCP) uses it.

```
GET    /:ws/concepts                      ?q&status&domain&market&locale&source&offset&limit
POST   /:ws/concepts                      (ordinary create; governed parts rejected with hint)
GET    /:ws/concepts/:cid
PUT    /:ws/concepts/:cid                 (ordinary edits only; governed → 409 + change-set hint)
DELETE /:ws/concepts/:cid                 (governed)
GET    /:ws/concepts/:cid/story           merged timeline (revisions, observations, comments, change-sets)
GET    /:ws/concepts/:cid/relations       ?as_of&market
POST   /:ws/concepts/:cid/relations
DELETE /:ws/concepts/:cid/relations/:rid
GET    /:ws/concepts/:cid/blast-radius    where-used
GET    /:ws/concepts/:cid/observations
POST   /:ws/concepts/:cid/observations
DELETE /:ws/concepts/:cid/observations/:oid
GET    /:ws/concepts/:cid/comments
POST   /:ws/concepts/:cid/comments        {body, parent_id?}
POST   /:ws/concepts/:cid/comments/:id/resolve
DELETE /:ws/concepts/:cid/comments/:id

GET    /:ws/graph                         viz payload {nodes, edges} ?as_of&market&domain&status&focus&depth
GET    /:ws/markets    POST /:ws/markets    PUT/DELETE /:ws/markets/:mid

GET    /:ws/changesets                    ?status
POST   /:ws/changesets
GET    /:ws/changesets/:id                includes ops, reviews, pilots
PATCH  /:ws/changesets/:id                name/description (draft only)
POST   /:ws/changesets/:id/ops            append op
DELETE /:ws/changesets/:id/ops/:seq       (draft only)
POST   /:ws/changesets/:id/submit         draft → in_review
POST   /:ws/changesets/:id/approve        {comment?}   (SoD: reviewer ≠ author)
POST   /:ws/changesets/:id/reject         {comment?}
POST   /:ws/changesets/:id/merge
POST   /:ws/changesets/:id/abandon
GET    /:ws/changesets/:id/blast-radius
POST   /:ws/changesets/:id/pilots         {project_id, stream}
DELETE /:ws/changesets/:id/pilots/:project/:stream
```

### Permissions

| Action | Permission |
|---|---|
| Read concepts/graph/story/changesets | `view_content` (workspace member) |
| Ordinary concept edits, observations, comments | `manage_terms` |
| Markets CRUD | `manage_terms` |
| Create/edit own change-set, pilots | `manage_terms` |
| Approve/reject change-set | `manage_brand`, and reviewer ≠ author |
| Merge governed change-set | `manage_brand` after approval |
| Voice ops inside change-sets | `manage_brand` |

## CLI / MCP (bowrain plugin)

```
kapi concepts list|show <id>|story <id>     server-backed, --json
kapi experiments list|show <id>|blast-radius <id>
kapi terms pull                              snapshot workspace concepts+relations → .kapi/termbase.db
```

MCP tools: `concept_search`, `concept_story`, `experiment_status`.

`kapi terms pull` writes through the framework termbase API so the snapshot is
identical in shape to a local termbase; `kapi verify` then gates offline in CI
against governed terminology.

## Frontend

One `BrandHub` shell (web + desktop, shared in `bowrain/packages/ui`) with
sub-navigation: Concepts (list, graph canvas, concept story), Voice (existing
pages re-homed), Experiments (list, detail with op diff + blast radius +
reviews + pilots, what-if wizard), Activity (brand-scoped event feed),
Dashboard (scores, drift, coverage, pending decisions). Graph canvas renders
with React Flow using a force-directed layout; every new component has a story
and vitest coverage; the desktop app proxies all routes through Wails bindings
(`governance.go` pattern). The workspace-scoped hub has no project to watch, so
freshness is React Query's own refetch (per-hook `staleTime` + refetch-on-focus)
plus the mutation-driven invalidation the brand hooks already do on every write;
there are no dedicated `concept-changed` / `changeset-changed` Wails events.
When a project *is* being watched, its existing `brand-voice-changed` /
`termbase-changed` events invalidate the hub's query keys for cross-client
freshness, so no new bindings (and no binding regen) are needed.
