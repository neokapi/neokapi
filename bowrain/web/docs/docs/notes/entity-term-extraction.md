---
sidebar_position: 6
title: "Entity & Term Extraction"
---

# Entity & Term Extraction

Implementation details for [AD-015](/architecture-decisions/015-server-ai-operations).

## LLM Extraction Schema

The `AIEntityExtractTool` uses `ChatStructured()` with this JSON schema:

```json
{
  "name": "extraction_result",
  "description": "Named entities and terminology candidates extracted from localization content",
  "strict": true,
  "schema": {
    "type": "object",
    "properties": {
      "blocks": {
        "type": "array",
        "items": {
          "type": "object",
          "properties": {
            "block_id": { "type": "string" },
            "entities": {
              "type": "array",
              "items": {
                "type": "object",
                "properties": {
                  "text": { "type": "string" },
                  "type": {
                    "type": "string",
                    "enum": [
                      "person",
                      "organization",
                      "product",
                      "location",
                      "date",
                      "time",
                      "currency",
                      "measurement",
                      "other"
                    ]
                  },
                  "dnt": { "type": "boolean" },
                  "offset": { "type": "integer" },
                  "length": { "type": "integer" },
                  "confidence": { "type": "number" }
                },
                "required": ["text", "type", "dnt", "offset", "length", "confidence"]
              }
            },
            "term_candidates": {
              "type": "array",
              "items": {
                "type": "object",
                "properties": {
                  "text": { "type": "string" },
                  "definition": { "type": "string" },
                  "category": {
                    "type": "string",
                    "enum": ["brand", "technical", "ui", "legal", "marketing", "general"]
                  },
                  "translatability": { "type": "string", "enum": ["dnt", "consistent", "free"] },
                  "confidence": { "type": "number" },
                  "offset": { "type": "integer" },
                  "length": { "type": "integer" }
                },
                "required": [
                  "text",
                  "definition",
                  "category",
                  "translatability",
                  "confidence",
                  "offset",
                  "length"
                ]
              }
            }
          },
          "required": ["block_id", "entities", "term_candidates"]
        }
      }
    },
    "required": ["blocks"]
  }
}
```

### System Prompt

```
You are a localization specialist analyzing source content for a translation project.

Given text blocks, identify:

1. Named entities: people, organizations, products, locations, dates, times, currencies,
   measurements. For each, indicate whether it should be marked do-not-translate (DNT).
   - Person names: usually DNT unless the project localizes names
   - Brand/product names: usually DNT
   - Dates/times/currencies/measurements: usually NOT DNT (they need locale-specific formatting)
   - Locations: context-dependent

2. Terminology candidates: domain-specific terms that should be translated consistently
   across the project. These are words/phrases that carry specific meaning in this context
   and would benefit from a termbase entry. Exclude common words.
   - "dnt" = never translate (brand names, acronyms that stay in source language)
   - "consistent" = translate, but the same way everywhere
   - "free" = translate naturally, no consistency requirement

Report character offsets relative to each block's text. Only report genuinely useful
entities and terms — quality over quantity.
```

### Batch Prompt Format

```
Analyze these {n} text blocks from a {source_locale} localization project:

Block (id: {block_id}):
"{block_text}"

Block (id: {block_id}):
"{block_text}"

...

Existing terms (do not re-propose): {known_terms}
```

## NER Provider Implementations

### Azure Language Services

```
POST {endpoint}/language/:analyze-text?api-version=2024-11-01
{
  "kind": "EntityRecognition",
  "analysisInput": {
    "documents": [
      { "id": "1", "language": "en", "text": "..." },
      { "id": "2", "language": "en", "text": "..." }
    ]
  }
}
```

Azure entity type mapping to `model.EntityType`:

| Azure Type                                                                                                         | model.EntityType   |
| ------------------------------------------------------------------------------------------------------------------ | ------------------ |
| Person, PersonType                                                                                                 | EntityPerson       |
| Organization, OrganizationMedical, OrganizationSports, OrganizationStockExchange                                   | EntityOrganization |
| Product, ComputingProduct                                                                                          | EntityProduct      |
| Address, Airport, City, Continent, CountryRegion, GPE, Geological, Location, State, Structural                     | EntityLocation     |
| Date, DateTime, DateRange, DateTimeRange                                                                           | EntityDate         |
| Time, TimeRange                                                                                                    | EntityTime         |
| Currency                                                                                                           | EntityCurrency     |
| Age, Area, Dimension, Height, Length, Number, NumberRange, Ordinal, Percentage, Speed, Temperature, Volume, Weight | EntityMeasurement  |
| (all others)                                                                                                       | EntityOther        |

Batch: up to 25 documents per request, 5120 characters each.

### spaCy (via Plugin Bridge)

Uses the Java/Python plugin bridge ([Framework AD-007](https://neokapi.github.io/web/neokapi/docs/architecture/007-plugin-system)). spaCy NER models output:

| spaCy Label                          | model.EntityType   |
| ------------------------------------ | ------------------ |
| PERSON                               | EntityPerson       |
| ORG, NORP                            | EntityOrganization |
| PRODUCT, WORK_OF_ART                 | EntityProduct      |
| GPE, LOC, FAC                        | EntityLocation     |
| DATE                                 | EntityDate         |
| TIME                                 | EntityTime         |
| MONEY                                | EntityCurrency     |
| QUANTITY, PERCENT, CARDINAL, ORDINAL | EntityMeasurement  |

## Review Queue SQLite Schema

```sql
CREATE TABLE review_items (
    id            TEXT PRIMARY KEY,
    project_id    TEXT NOT NULL,
    type          TEXT NOT NULL,  -- 'term_candidate', 'entity_review'
    status        TEXT NOT NULL DEFAULT 'pending',  -- 'pending', 'assigned', 'approved', 'rejected'
    push_id       TEXT NOT NULL DEFAULT '',
    data          TEXT NOT NULL,  -- JSON: TermCandidateAnnotation or EntityAnnotation
    occurrences   TEXT NOT NULL DEFAULT '[]',
    assigned_to   TEXT NOT NULL DEFAULT '',
    decided_by    TEXT NOT NULL DEFAULT '',
    decided_at    TEXT NOT NULL DEFAULT '',
    comment       TEXT NOT NULL DEFAULT '',
    edits         TEXT NOT NULL DEFAULT '{}',
    confidence    REAL NOT NULL DEFAULT 0,
    locale        TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE
);

CREATE INDEX idx_review_items_project_status ON review_items(project_id, status);
CREATE INDEX idx_review_items_project_type ON review_items(project_id, type);
CREATE INDEX idx_review_items_assigned ON review_items(project_id, assigned_to);
CREATE INDEX idx_review_items_confidence ON review_items(project_id, confidence);

-- Track rejected terms to avoid re-proposing
CREATE TABLE rejected_terms (
    project_id    TEXT NOT NULL,
    term_text     TEXT NOT NULL,
    locale        TEXT NOT NULL,
    rejected_at   TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (project_id, term_text, locale)
);

-- DNT list (auto-approved entities + user-confirmed DNT terms)
CREATE TABLE dnt_entries (
    project_id    TEXT NOT NULL,
    text          TEXT NOT NULL,
    entity_type   TEXT NOT NULL DEFAULT '',
    locale        TEXT NOT NULL,
    source        TEXT NOT NULL DEFAULT '',
    created_at    TEXT NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (project_id, text, locale)
);
```

## Extraction Worker Flow

```
EventPushCompleted
    │
    ▼
AutomationEngine matches rule "auto-extract-entities"
    │
    ▼
ActionExecutor dispatches flow "entity-term-extract"
    │
    ▼
Worker picks up job
    │
    ├─ 1. Load changed blocks from push (by push_id)
    ├─ 2. Load existing terms (for dedup) + rejected terms (for skip list)
    │
    ├─ 3. NER pass (if configured)
    │   ├─ Batch blocks into NER requests (25 per batch, Azure limit)
    │   ├─ Map NER entities → EntityAnnotation
    │   ├─ Auto-approve configured types → write directly to block annotations
    │   └─ Queue remaining entities as ReviewItems
    │
    ├─ 4. LLM pass
    │   ├─ Batch blocks (configurable, default 10 per call)
    │   ├─ Concurrent calls (configurable, default 4)
    │   ├─ Exclude blocks fully covered by NER (optimization)
    │   ├─ Parse structured response → TermCandidateAnnotation + EntityAnnotation
    │   ├─ Dedup: skip terms already in termbase or rejected list
    │   └─ Apply confidence threshold: high → normal queue, low → low-confidence queue
    │
    ├─ 5. Merge
    │   ├─ Reconcile NER + LLM entities (prefer LLM classification for overlaps)
    │   ├─ Group identical term text → single ReviewItem with aggregated occurrences
    │   └─ Attach all annotations to blocks
    │
    └─ 6. Persist
        ├─ Write ReviewItems to review_items table
        ├─ Write auto-approved entities to block annotations
        ├─ Write auto-approved DNT entries to dnt_entries table
        └─ Emit EventExtractionCompleted (for downstream automation)
```

## bowrain-app API Contract

### Authentication

```
POST /api/v1/auth/device-code    → { device_code, user_code, verification_uri }
POST /api/v1/auth/token           → { access_token, refresh_token }
POST /api/v1/auth/refresh          → { access_token, refresh_token }
```

### Workspace & Project

```
GET /api/v1/workspaces                    → [{ id, slug, name }]
GET /api/v1/workspaces/:slug/projects     → [{ id, name, source_locale, target_locales }]
```

### Review Queue

```
GET /api/v1/projects/:id/review-queue
    ?type=term_candidate|entity_review
    &status=pending|assigned
    &confidence=high|low
    &assigned_to=me|unassigned
    &limit=50
    &cursor=...
    → { items: [ReviewItem], next_cursor, total, remaining }

POST /api/v1/projects/:id/review-queue/:item_id/decide
    { "decision": "approve"|"reject", "comment": "...", "edits": { "definition": "...", "category": "..." } }
    → { ok: true, concept_id?: "..." }  // concept_id returned on term approval

POST /api/v1/projects/:id/review-queue/batch-decide
    { "item_ids": [...], "decision": "approve"|"reject" }
    → { ok: true, decided: 5 }

POST /api/v1/projects/:id/review-queue/:item_id/assign
    { "user_id": "..." }

POST /api/v1/projects/:id/review-queue/:item_id/split
    { "occurrence_ids": ["..."] }
    → { original: ReviewItem, new_item: ReviewItem }
```

### Sync (offline-first)

```
POST /api/v1/projects/:id/review-queue/sync
    { "decisions": [{ "item_id": "...", "decision": "approve", "edits": {...}, "decided_at": "..." }] }
    → { synced: 5, conflicts: [] }
```

## Notification Schema

```sql
CREATE TABLE notifications (
    id            TEXT PRIMARY KEY,
    user_id       TEXT NOT NULL,
    type          TEXT NOT NULL DEFAULT 'general',  -- 'review.assigned', 'review.completed',
                                                    -- 'extraction.completed', 'general'
    title         TEXT NOT NULL,
    body          TEXT NOT NULL DEFAULT '',
    project_id    TEXT NOT NULL DEFAULT '',
    link_url      TEXT NOT NULL DEFAULT '',  -- deep link target
    read          INTEGER NOT NULL DEFAULT 0,
    created_at    TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX idx_notifications_user ON notifications(user_id, read, created_at DESC);
```

**Delivery layers (incremental):**

1. **Polling (v1):** `GET /api/v1/notifications?unread=true&limit=20` — simple, works everywhere
2. **WebSocket (v2):** Extend existing `ws_collab.go` with a notification channel per user — real-time bell badge updates
3. **Push (v3):** APNs/FCM for bowrain-app background notifications — device token registration via `POST /api/v1/notifications/devices`

**Notification API:**

```
GET    /api/v1/notifications?unread=true&limit=20&cursor=...
POST   /api/v1/notifications/:id/read
POST   /api/v1/notifications/read-all
DELETE /api/v1/notifications/:id
```

## Editor Entity Integration

### BlockInfoResponse Changes

```go
// In bowrain/server/editor.go
type BlockInfoResponse struct {
    // ... existing fields ...
    Entities []EntityInfoResponse `json:"entities,omitempty"`
}

type EntityInfoResponse struct {
    Key    string `json:"key"`              // annotation key (e.g. "entity:0")
    Text   string `json:"text"`
    Type   string `json:"type"`             // "person", "organization", "product", etc.
    Start  int    `json:"start"`            // character offset in source
    End    int    `json:"end"`
    DNT    bool   `json:"dnt"`
    Source string `json:"source,omitempty"` // "llm", "ner", "manual"
    Locale string `json:"locale,omitempty"`
}
```

### Entity Mutation Endpoints

```
POST   /api/v1/projects/:id/blocks/:block_id/entities
       { "text": "...", "type": "person", "dnt": true, "start": 5, "end": 15 }

PUT    /api/v1/projects/:id/blocks/:block_id/entities/:idx
       { "type": "organization", "dnt": false }

DELETE /api/v1/projects/:id/blocks/:block_id/entities/:idx

POST   /api/v1/projects/:id/blocks/:block_id/entities/:idx/promote
       → creates TermCandidateAnnotation from entity, routes to review queue
```

### Editor Component Hierarchy

```
VisualEditorLayout
├── FormattedSourceDisplay
│   └── EntityHighlight (inline, colored background per type)
│       └── EntityPopover (click: type picker, DNT toggle, promote-to-term)
├── ContextPanel (right sidebar)
│   ├── TerminologySection (existing)
│   └── EntitiesSection (NEW)
│       └── EntityListItem (type badge, text, DNT lock, source indicator)
└── EditorToolbar
    └── MarkEntityButton (select text → Cmd+E → type picker)
```

### Entity Color Tokens (CSS custom properties)

```css
--entity-person: hsl(210 80% 92%); /* blue tint */
--entity-organization: hsl(270 70% 92%); /* purple tint */
--entity-product: hsl(40 80% 90%); /* amber tint */
--entity-location: hsl(140 60% 90%); /* green tint */
--entity-date: hsl(220 15% 90%); /* slate tint */
--entity-time: hsl(220 15% 90%); /* slate tint */
--entity-currency: hsl(160 60% 90%); /* emerald tint */
--entity-measurement: hsl(190 70% 90%); /* cyan tint */
```

Dark mode variants shift to lower lightness with higher saturation.

## Implementation Sequence

### Track 1: Backend — Extraction Pipeline (bowrain server)

1. `TermCandidateAnnotation` in `core/model/`
2. `NERProvider` interface in `core/ai/ner/`
3. `AIEntityExtractTool` in `core/ai/tools/`
4. Azure NER provider in `core/ai/ner/azure/`
5. Review queue store in `bowrain/store/` (SQLite schema + CRUD)
6. Review queue API endpoints in `bowrain/server/`
7. Extraction automation rule + worker in `bowrain/event/`
8. Approval → termbase creation logic in `bowrain/service/`

### Track 2: Backend — Editor Entity Support

1. Add `entities` field to `BlockInfoResponse` in `bowrain/server/editor.go`
2. Entity mutation endpoints (create, update, delete, promote-to-term)
3. Entity highlighting in `FormattedSourceDisplay` and `HighlightedSource` (`packages/ui/`)
4. Entity popover component (type picker, DNT toggle, promote action)
5. Entities section in context panel (right sidebar)
6. Manual entity marking: text selection → Cmd+E → type picker
7. Code/source editor: colored underlines + sidebar entity list

### Track 3: Mobile App (bowrain-app)

1. Keycloak PKCE auth flow (device code + deep link callback)
2. Workspace selection screen
3. Project selection screen
4. Review queue screen with `SwipeCardStack`
5. `TermExtractionCard` wired to real API
6. `EntityReviewCard` (new card type for entity decisions)
7. Offline sync engine wired to `/sync` endpoint

### Track 4: Notifications (incremental)

1. Notification store (SQLite schema) + CRUD in `bowrain/store/`
2. Notification API endpoints (list, read, read-all, delete)
3. `"notify"` action executor in automation engine
4. Notification center UI component in `packages/ui/` (glass UI pattern)
5. Bell icon badge with unread count in `TopBar`
6. WebSocket notification channel (extend `ws_collab.go`)
7. Mobile push notifications via APNs/FCM (bowrain-app)
