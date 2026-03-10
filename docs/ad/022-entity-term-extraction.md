---
id: 022-entity-term-extraction
sidebar_position: 22
title: "AD-022: Automated Entity & Term Extraction"
---
# AD-022: Automated Entity & Term Extraction

## Context

Bowrain aims to aid source-language creators before translation begins. A key part of this is automatic discovery of terminology and named entities — brands, UI labels, technical terms, person names, dates, measurements — so that translators start with a curated termbase rather than building one manually.

The content model already has `EntityAnnotation` ([AD-002](/docs/ad/002-content-model)) and `TermAnnotation` ([AD-010](/docs/ad/010-terminology)). The terminology notes describe `term-extract` and `entity-annotate` as pipeline tools. This AD defines the **automated extraction pipeline**: how extraction is triggered, where it runs, how results flow into a review queue, and how the mobile companion app enables rapid human review.

## Decision

### Hybrid Extraction: LLM + NER Providers

Two complementary extraction mechanisms:

**LLM-based extraction** via `ChatStructured()` ([AD-008](/docs/ad/008-ai-integration)) handles terminology and ambiguous entities. A single structured prompt identifies domain terms, classifies translatability (DNT, consistent, free), proposes definitions, and detects context-dependent entities — all in one call. This is the primary extraction path.

**NER provider-based extraction** via a new `NERProvider` interface handles high-volume, deterministic entity recognition. Cloud NER services (Azure: 45 entity types, 70+ languages; AWS Comprehend; Google NL) and local models (spaCy, Hugging Face) excel at fast, cheap detection of obvious entities: dates, currencies, measurements, person names, addresses, phone numbers. NER runs as a complementary layer — its results feed into the same annotation pipeline.

Both run through the same output path: entities become `EntityAnnotation`, term candidates become `TermCandidateAnnotation`, and all flow into the review queue.

### Why Both

| Concern | LLM | NER |
|---------|-----|-----|
| Terminology extraction | Excellent — understands domain context | Cannot do this |
| Obvious entities (dates, currencies) | Works but expensive at volume | Fast, cheap, deterministic |
| Context sensitivity ("Sprint" = agile vs. telecom) | Handles naturally | Cannot distinguish |
| Language coverage | Depends on model | Azure: 70+, spaCy: 23+ |
| Cost at scale | $$ per 1K blocks | ¢ per 1K blocks |
| Consistency | Probabilistic | Deterministic |

NER is an optimization layer: it handles the easy cases cheaply, freeing LLM capacity for the hard cases (terminology, ambiguous entities, classification decisions).

### NER Provider Interface

```go
// NERProvider detects named entities in text using ML models or cloud services.
type NERProvider interface {
    Name() string
    DetectEntities(ctx context.Context, req NERRequest) (*NERResponse, error)
    DetectEntitiesBatch(ctx context.Context, reqs []NERRequest) ([]NERResponse, error)
    SupportedLocales() []model.LocaleID
    Close() error
}

type NERRequest struct {
    Text   string
    Locale model.LocaleID
}

type NERResponse struct {
    Entities []DetectedEntity
}

type DetectedEntity struct {
    Text       string
    Type       model.EntityType
    Confidence float64
    Offset     int
    Length     int
}
```

Implementations: Azure Language Services, AWS Comprehend, Google Cloud NL, spaCy (via plugin bridge), Hugging Face (via plugin bridge). Each maps provider-specific entity types to `model.EntityType`.

### Extraction Pipeline

#### Trigger

Extraction runs **server-side** as a post-push automation. On `EventPushCompleted`, the automation engine fires the extraction flow for blocks where `ComputeIdentity` changed. On first push, all blocks are new — subsequent pushes only extract from changed blocks. Users can also trigger re-extraction manually via CLI or UI.

This is a server-side `StoredRule` in the database, registered as a built-in rule on project creation (not in `.bowrain/config.yaml`):

```go
StoredRule{
    Name:    "auto-extract-entities",
    Trigger: event.EventPushCompleted,
    Conditions: []AutomationCondition{
        {Field: "blocks_changed", Operator: "exists"},
    },
    Actions: []AutomationAction{
        {Type: "flow", Config: map[string]string{"flow": "entity-term-extract"}},
    },
    Builtin: true,
    Enabled: true,
}
```

#### Flow Stages

```
Changed Blocks
    │
    ├─ Stage 1: NER Detection (if NER provider configured)
    │   → Fast entity detection (dates, currencies, names, etc.)
    │   → Attach EntityAnnotation to blocks
    │   → Auto-approve obvious entities (dates, currencies, measurements, emails)
    │     based on project auto-approval config
    │
    ├─ Stage 2: LLM Extraction (always)
    │   → ChatStructured() with extraction schema
    │   → Identifies domain terms, ambiguous entities, translatability
    │   → Dedup against existing termbase (skip known terms)
    │   → Produces TermCandidateAnnotation for new candidates
    │
    ├─ Stage 3: Merge & Dedup
    │   → Reconcile NER entities with LLM entities (prefer LLM classification)
    │   → Group identical terms across blocks (one review item per unique term)
    │   → Aggregate occurrences
    │
    └─ Stage 4: Queue
        → Auto-approved items → termbase / entity store directly
        → Pending items → review queue (routed by locale/domain)
```

#### Multi-Locale Extraction

Extraction runs on **all locales** — source and targets. Source extraction discovers terms and entities. Target extraction catches locale-specific entities (date formats, transliterated names, locale-specific abbreviations) and feeds into QA checks. Target extraction results can flag inconsistencies: if source has "Dashboard" marked as a term but the French translation uses two different renderings, that's a QA issue.

### Provider Configuration

Extraction settings are **server-side project configuration**, managed through the Bowrain UI or REST API — not `.bowrain/config.yaml`. The client project directory only knows about file mappings and server connection details; it pushes content and the server decides what post-push processing to apply.

The project's configured LLM provider is the default for extraction. An override is available per project for teams that want a cheaper model (e.g., Haiku for extraction, Sonnet for translation):

```go
// ExtractionConfig is server-side project configuration stored in the database.
type ExtractionConfig struct {
    Enabled             bool              // enable auto-extraction on push
    ProviderOverride    string            // LLM provider ID, empty = use project default
    ModelOverride       string            // model override, empty = use provider default
    NERProvider         string            // "azure", "spacy", "" = none
    NERProviderConfig   map[string]string // provider-specific settings (endpoint, key ref, etc.)
    AutoApproveTypes    []string          // entity types to auto-approve as DNT
    ConfidenceThreshold float64           // below this → low-confidence queue (default 0.4)
    BatchSize           int               // blocks per LLM call (default 10)
    Concurrency         int               // concurrent LLM calls (default 4)
}
```

Default auto-approve types: `entity:date`, `entity:time`, `entity:currency`, `entity:measurement`, `entity:email`.

```
PUT /api/v1/projects/:id/settings/extraction
{
  "enabled": true,
  "provider_override": "",
  "model_override": "claude-haiku-4-5-20251001",
  "ner_provider": "azure",
  "auto_approve_types": ["entity:date", "entity:time", "entity:currency", "entity:measurement"],
  "confidence_threshold": 0.4
}
```

### Content Model: TermCandidateAnnotation

A new annotation type for proposed terms awaiting review. Distinct from `TermAnnotation` (which represents a confirmed match against an existing termbase entry):

```go
type TermCandidateAnnotation struct {
    Text            string          // the term text as found
    Definition      string          // AI-proposed definition
    Category        string          // "brand", "technical", "ui", "legal", "marketing"
    Translatability string          // "dnt", "consistent", "free"
    Confidence      float64         // extraction confidence [0,1]
    Position        TextRange       // character offset in source
    Locale          LocaleID        // locale where found
    Source          ExtractionSource // "llm", "ner", "manual"
    Status          CandidateStatus // "pending", "approved", "rejected"
}

type ExtractionSource string

const (
    ExtractionSourceLLM    ExtractionSource = "llm"
    ExtractionSourceNER    ExtractionSource = "ner"
    ExtractionSourceManual ExtractionSource = "manual"
)

type CandidateStatus string

const (
    CandidateStatusPending  CandidateStatus = "pending"
    CandidateStatusApproved CandidateStatus = "approved"
    CandidateStatusRejected CandidateStatus = "rejected"
)
```

`ExtractionSourceManual` covers the case where translators or content creators manually mark terms/entities — the same review and approval workflow applies regardless of whether a human or AI proposed the candidate.

### Auto-Approval

Configurable per project. By default, obvious entity types skip the review queue:

- **Auto-approve (default):** `entity:date`, `entity:time`, `entity:currency`, `entity:measurement`, `entity:email`
- **Always review:** `entity:person`, `entity:organization`, `entity:product`, `entity:location`, all term candidates

Auto-approved entities are written directly to blocks as `EntityAnnotation` with `DNT: true`. The auto-approve list is configurable — teams can add or remove types based on their domain (e.g., a legal team might auto-approve `entity:location` but always review `entity:date` because date formats are legally significant).

### Review Queue

#### Data Model

```go
type ReviewItem struct {
    ID            string
    ProjectID     string
    Type          ReviewItemType      // "term_candidate", "entity_review"
    Status        ReviewItemStatus    // "pending", "assigned", "approved", "rejected"
    Candidate     *TermCandidateAnnotation // for term candidates
    Entity        *EntityAnnotation        // for entity reviews
    Occurrences   []Occurrence        // all blocks where this term/entity appears
    AssignedTo    *string             // user ID, nil = unassigned
    DecidedBy     *string
    DecidedAt     *time.Time
    Comment       string
    Edits         map[string]string   // user edits (definition, category, etc.)
    CreatedAt     time.Time
}

type Occurrence struct {
    BlockID  string
    FileID   string
    FilePath string
    Position TextRange
    Context  string // surrounding text snippet
}

type ReviewItemType string

const (
    ReviewItemTermCandidate ReviewItemType = "term_candidate"
    ReviewItemEntityReview  ReviewItemType = "entity_review"
)
```

#### Grouping

Identical terms across blocks are grouped into a single `ReviewItem` with multiple `Occurrences`. One approval decision applies to all occurrences. Users can split a grouped item if occurrences have different meanings (e.g., "bank" as financial institution vs. river bank).

#### Routing

Items are routable to specific reviewers by locale or domain. Initial implementation: assignable per item. Future: rule-based auto-assignment (e.g., "all Japanese entities → reviewer X", "all legal terms → reviewer Y").

#### Confidence Tiers

- **High confidence (>= threshold):** Normal review queue
- **Low confidence (< threshold):** Separate "low confidence" queue — review if you want, ignore if busy. Items here don't block workflows.

Default threshold: 0.4 (configurable in project config).

### Approval Lifecycle

On **approve**:
- **Term candidate:** Creates a Concept in the termbase with the extracted definition, category, and a Term entry for the source locale. In projects with role-based permissions, the behavior is configurable:
  - **Direct create** (default for small teams): approval creates an active Concept immediately
  - **Draft create** (enterprise): approval creates a Concept with `status: draft`, requiring a terminologist to promote to `active`
- **Entity:** Adds `EntityAnnotation` to all occurrence blocks. If DNT, the entity text is added to a project-level DNT list for future auto-detection.

On **reject:**
- Item marked rejected with optional comment. The same term/entity won't be re-proposed on future pushes unless the source text changes.

On **manual mark** (translator/creator marks a term in the editor or CLI):
- Creates a `TermCandidateAnnotation` with `Source: "manual"` and routes to the review queue with the same approval workflow. Manual candidates can be auto-approved if the user has the appropriate role/permission.

### Review Queue API

```
GET    /api/v1/projects/:id/review-queue
       ?type=term_candidate|entity_review
       &status=pending|assigned
       &locale=en-US
       &confidence=high|low
       &assigned_to=me|unassigned|:user_id

GET    /api/v1/projects/:id/review-queue/:item_id

POST   /api/v1/projects/:id/review-queue/:item_id/decide
       { "decision": "approve"|"reject", "comment": "...", "edits": {...} }

POST   /api/v1/projects/:id/review-queue/:item_id/assign
       { "user_id": "..." }

POST   /api/v1/projects/:id/review-queue/:item_id/split
       { "occurrence_ids": ["..."] }  // split into separate review item

POST   /api/v1/projects/:id/review-queue/batch-decide
       { "item_ids": [...], "decision": "approve"|"reject" }
```

### Mobile Companion App (bowrain-app)

The bowrain-app provides rapid review of the extraction queue through a swipe-based interface. Full authentication flow:

```
Keycloak Login → Workspace Selection → Project Selection → Review Queue
```

#### Card Types

Three card types map to review item types:

1. **TermExtractionCard** — AI-proposed term candidates. Shows term, definition, category, translatability, confidence, top 3 occurrences. Expanded sheet: all occurrences, editable definition/category/translatability.

2. **EntityReviewCard** — Entities needing human judgment (person names, product names, organizations). Shows entity text, type, DNT suggestion, occurrences. Expanded sheet: context per occurrence, DNT toggle.

3. **TermReviewCard** — Existing terms with proposed translations (from translation flow). Shows source term, proposed target, domain, POS/gender.

#### Interaction

- **Swipe right** = approve (with haptic feedback)
- **Swipe left** = reject (with haptic feedback)
- **Tap** = expand for details + inline editing
- Offline-first: decisions persist locally, sync on reconnect
- Queue shows remaining count + progress

### Existing Content

No backfill on feature enable. Extraction runs incrementally on future pushes only. Users who want to bootstrap can trigger a manual "Extract All" via the UI or CLI, but this is an explicit action — not automatic.

### Notifications (Planned)

Review queue changes need to reach users across all surfaces: desktop app, web app, and mobile companion. The architecture must support this from the start even if the full notification system is built incrementally.

**Foundation already in place:**
- The automation engine defines a `"notify"` action type — no executor yet
- WebSocket infrastructure exists for collaborative editing (`ws_collab.go`) — can extend for notification streaming
- The `TopBar` component has a placeholder bell icon ready for a notification center

**Notification architecture:**

1. **Server-side notification store.** Persistent notifications per user with read/unread state, stored alongside review items (SQLite or PostgreSQL, depending on the deployment — see [AD-003](./003-content-store.md)). Notification types: `queue_item_added`, `queue_item_assigned`, `queue_item_decided`, `extraction_completed`.

2. **Real-time delivery.** Extend the existing WebSocket infrastructure with a notification channel. Clients subscribe on connect; the server pushes notification events as they occur. Falls back to polling for clients without WebSocket support (mobile in background).

3. **Notification center UI.** Build on the glass UI notification center pattern (stacked cards with timestamps, read/unread state, action buttons). The bell icon badge shows unread count. Clicking opens a slide-out panel. Shared component in `packages/ui/` used by both web and desktop apps.

4. **Mobile push notifications.** Future: integrate with APNs/FCM via the bowrain-app. The server stores device tokens and dispatches push notifications for high-priority events (new items assigned to you).

**Notification triggers for extraction:**
- Extraction completed → notify project admins with summary (N entities, M terms found)
- New items assigned → notify the assignee
- Item decided → notify the original proposer (for manual candidates)
- Queue empty → notify team ("all caught up!")

### Translation Editor Entity Support

Entities must be first-class citizens in the translation editor — both the visual editor and the code/source view. The existing character-offset `TextRange` mechanism used for terminology highlighting works identically for entities.

**API changes:**

`BlockInfoResponse` gains an `entities` field alongside the existing terminology data:

```go
type BlockEntityResponse struct {
    Text     string `json:"text"`
    Type     string `json:"type"`      // "person", "date", "currency", etc.
    DNT      bool   `json:"dnt"`
    Start    int    `json:"start"`     // character offset
    End      int    `json:"end"`
    Locale   string `json:"locale"`
    Source   string `json:"source"`    // "llm", "ner", "manual"
}
```

**Visual editor rendering:**

Entities render as inline highlights with distinct styling per entity type:

| Entity Type | Style | Icon |
|------------|-------|------|
| Person | Blue background tint | User icon |
| Organization | Purple background tint | Building icon |
| Product | Amber background tint | Package icon |
| Location | Green background tint | MapPin icon |
| Date/Time | Slate background tint | Calendar/Clock icon |
| Currency | Emerald background tint | Currency icon |
| Measurement | Cyan background tint | Ruler icon |

DNT entities get an additional lock badge overlay. Hovering shows a tooltip with entity type, DNT status, and extraction source. Clicking opens an inline popover to:
- Toggle DNT
- Change entity type (correct misclassification)
- Promote to term candidate (creates a `TermCandidateAnnotation` with `Source: "manual"`)
- Remove annotation

**Manual entity marking:**

Users can select text in the source editor and mark it as an entity via:
- Right-click context menu → "Mark as Entity" → type picker
- Keyboard shortcut (Cmd/Ctrl+E) → type picker popover
- These create `EntityAnnotation` with `Source: "manual"` or route through the review queue depending on the user's role

**Code/source editor:**

In the non-visual editor, entities display as colored underlines with type-coded colors matching the visual editor. A sidebar panel lists all entities in the current block with edit controls.

**Context panel:**

The existing context panel (right sidebar) gains an "Entities" section below the terminology section, listing all entities in the current block grouped by type. Each entry shows the entity text, type badge, DNT status, and source (AI/manual).

### Existing Content

No backfill on feature enable. Extraction runs incrementally on future pushes only. Users who want to bootstrap can trigger a manual "Extract All" via the UI or CLI, but this is an explicit action — not automatic.

## Consequences

- **New framework types:** `TermCandidateAnnotation`, `NERProvider` interface, `AIEntityExtractTool`, `AITermExtractTool`
- **New bowrain components:** review queue store (SQLite and PostgreSQL), review queue API endpoints, extraction automation rule, extraction worker, notification store + WebSocket delivery
- **New platform types:** `ReviewItem`, `ReviewItemType`, `ReviewItemStatus` in `platform/store/`
- **New editor components:** entity highlighting in `FormattedSourceDisplay` and `HighlightedSource`, entity context panel, manual entity marking, entity popover editor
- **bowrain-app:** Full auth flow + workspace/project selection + review queue integration
- **Cost consideration:** LLM extraction has per-block cost. Batch mode (10+ blocks per call) and NER fast-path for obvious entities keep costs manageable. The confidence threshold filters noise without losing signal.
- **Manual + AI parity:** The same review queue handles both AI-extracted and manually-marked candidates, giving a unified workflow regardless of source.
