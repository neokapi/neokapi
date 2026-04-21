---
id: 015-server-ai-operations
sidebar_position: 15
title: "AD-015: Server-Side AI Operations"
---

# AD-015: Server-Side AI Operations

## Summary

Three server-side AI subsystems run on top of the framework's
`LLMProvider` interface
([AD-framework-011: AI Providers](/docs/ad/011-ai-providers)):
asynchronous **translation jobs** with per-workspace quotas, **entity
and term extraction** combining LLM reasoning with NER detection, and
**brand voice governance** with MQM-inspired scoring. All three surface
through the same automation engine, activity feed, and task system.

## Context

Translation at workspace scale cannot run synchronously — jobs may
process thousands of blocks and take minutes. Extraction must discover
terminology and entities as source content arrives, without blocking
the push. Brand voice compliance extends terminology enforcement into
tone and style. Each subsystem has its own data model and lifecycle,
but they share the same provider abstraction, the same event bus, and
the same human review surface, so unifying them at the architecture
level keeps the platform coherent.

## Decision

### 1. Translation jobs

Server-side translation is asynchronous, quota-metered, and
automation-triggered.

**Components.**

| Component     | Responsibility                                                                             |
| ------------- | ------------------------------------------------------------------------------------------ |
| `JobStore`    | Persists jobs with status, progress, and token usage (SQLite or PostgreSQL)                |
| `JobQueue`    | Abstract dispatch; implementations: in-memory channels (dev), Azure Service Bus, NATS       |
| `Worker`      | Dequeues jobs, resolves providers, processes blocks in chunks of 50, records usage         |
| `QuotaStore`  | Tracks token usage per workspace with a monthly limit (default 10M tokens)                 |

**Job model.**

```go
type TranslationJob struct {
    ID               string
    WorkspaceSlug    string
    ProjectID        string
    ItemName         string
    TargetLocale     string
    ProviderConfigID string   // empty or "platform" = managed identity
    Model            string
    PushID           string   // links to originating push event
    Status           JobStatus
    Progress         int
    TotalBlocks      int
    DoneBlocks       int
    BatchSize        int      // blocks per LLM call (default 20)
    Concurrency      int      // parallel batch calls (default 5)
    TokensUsed       int
    Error            string
    StepID           string   // links to parent automation step (AD-013)
    CreatedAt, UpdatedAt time.Time
}
```

**Worker algorithm.**

```
1. Dequeue job ID from queue
2. Load job from JobStore; skip if status != "queued"
3. Check quota via QuotaStore
4. Mark status = "processing"
5. Load project and blocks from ContentStore
6. Resolve provider (see below)
7. Create AITranslateTool with batch/concurrency config
8. Process blocks in chunks of 50:
   a. Run tool on chunk
   b. Record token usage in QuotaStore
   c. Update progress in JobStore and emit log lines
9. Store translated blocks in ContentStore
10. Mark status = "completed" with total token count
```

**Provider resolution.**

- **Platform provider** — Azure OpenAI with Managed Identity. No API
  keys; workers acquire Entra ID tokens automatically. Enabled when
  `BOWRAIN_OPENAI_ENDPOINT` is set.
- **Workspace-configured provider** — Anthropic, OpenAI, Azure OpenAI
  (explicit key), Ollama. Credentials live in the credential store.

Jobs prefer workspace configuration and fall back to platform default.
`IsPlatformProvider()` on a job returns true when
`ProviderConfigID` is empty or `"platform"`.

**Per-workspace quota.** `QuotaStore` enforces a monthly token budget
before each job starts. The quota aggregates every AI operation —
translation, QA, extraction, Bravo — and drains the same pool. Exceeding
quota returns a dedicated error that the automation engine surfaces as
a `quota.warning` notification.

**API.**

```
POST   /api/v1/:ws/jobs/translate    # create async job (202 Accepted)
GET    /api/v1/:ws/jobs/:id          # poll status and progress
GET    /api/v1/:ws/ai-usage           # quota summary (used, remaining, period)
```

Automation rules dispatch jobs by `push_id` so the
`PushCompletionTracker` ([AD-014](014-translator-workflow.md)) can
correlate completion across items and locales.

### 2. Entity & term extraction

Extraction runs as a post-push automation rule (`auto-extract`) and
combines two complementary mechanisms.

**LLM-based extraction** via `ChatStructured` handles terminology and
ambiguous entities — domain terms, translatability classification,
proposed definitions. A single structured prompt answers all of them.

**NER provider-based extraction** via a `NERProvider` interface handles
high-volume, deterministic detection of dates, currencies,
measurements, person names, emails. Implementations: Azure Language
Services, AWS Comprehend, Google Cloud NL, spaCy.

```go
type NERProvider interface {
    Name() string
    DetectEntities(ctx context.Context, req NERRequest) (*NERResponse, error)
    DetectEntitiesBatch(ctx context.Context, reqs []NERRequest) ([]NERResponse, error)
    SupportedLocales() []model.LocaleID
    Close() error
}
```

**Four stages.**

```
Changed blocks
  ├─ Stage 1: NER detection (if NER provider configured)
  │     → fast entity annotation, auto-approve obvious types
  │
  ├─ Stage 2: LLM extraction
  │     → ChatStructured() produces term candidates + ambiguous entities,
  │       dedup against existing termbase
  │
  ├─ Stage 3: Merge & dedup
  │     → reconcile NER with LLM (prefer LLM classification),
  │       group identical terms across blocks
  │
  └─ Stage 4: Queue
        → auto-approved → termbase / entity store directly
        → pending → review queue (routed by locale/domain)
```

**Review items.**

```go
type ReviewItem struct {
    ID            string
    ProjectID     string
    Type          ReviewItemType      // "term_candidate" | "entity_review"
    Status        ReviewItemStatus    // pending | assigned | approved | rejected
    Candidate     *TermCandidateAnnotation
    Entity        *EntityAnnotation
    Occurrences   []Occurrence
    AssignedTo    *string
    DecidedBy     *string
    DecidedAt     *time.Time
    Comment       string
    Edits         map[string]string
    CreatedAt     time.Time
}
```

Identical terms across blocks group into a single `ReviewItem` with
multiple occurrences — one decision applies to all. Grouped items can
be split when occurrences have different meanings.

**Confidence tiers.** High-confidence items go to the normal queue;
low-confidence items (below the configurable threshold, default 0.4)
land in a separate "low confidence" queue that never blocks workflows.

**Auto-approval.** Obvious entity types skip review by default:
`entity:date`, `entity:time`, `entity:currency`, `entity:measurement`,
`entity:email`. Configurable per project — legal teams might auto-approve
`entity:location` but always review `entity:date`.

**Review queue API.**

```
GET    /:ws/:proj/review-queue/:ref           # filter by type/status/locale/confidence
GET    /:ws/:proj/review-queue/:ref/:itemId
POST   /:ws/:proj/review-queue/:ref/:itemId/decide    { decision, comment, edits }
POST   /:ws/:proj/review-queue/:ref/:itemId/assign    { user_id }
POST   /:ws/:proj/review-queue/:ref/:itemId/split     { occurrence_ids }
POST   /:ws/:proj/review-queue/:ref/batch-decide      { item_ids, decision }
```

**Mobile companion app.** The bowrain-app (Pulse's mobile companion,
[AD-017](017-bowrain-apps.md)) renders extraction queue items as swipe
cards: right = approve, left = reject, tap = expand for inline editing.
Decisions persist locally and sync on reconnect.

**Inline editor rendering.** `BlockInfoResponse` carries an `entities`
field. The translation editor renders entities as inline highlights
with entity-type-specific styling (Person = blue tint, Organization =
purple, Product = amber, …), a lock badge for DNT, and an inline
popover to toggle DNT, reclassify, promote to term candidate, or
remove.

### 3. Brand voice governance

Brand voice extends terminology with tone, style, and vocabulary
governance.

**VoiceProfile.**

```go
type VoiceProfile struct {
    ID          string
    Name        string
    Description string
    Tone        ToneProfile
    Style       StyleRules
    Vocabulary  VocabularyRules
    Examples    []VoiceExample
    Locales     map[string]LocaleOverride
    Channels    map[string]ChannelOverride
    WorkspaceID string
    Version     int
    CreatedAt, UpdatedAt time.Time
    CreatedBy   string
}
```

`ToneProfile` captures personality, formality (`casual`, `neutral`,
`formal`, `technical`), emotion, humor, and free-text guidelines.
`StyleRules` defines writing constraints including regex-based
prohibited and required patterns with severity. `VocabularyRules`
defines preferred terms, forbidden terms, competitor terms, and
approved abbreviations.

**Locale and channel overrides.** `LocaleOverride` adjusts formality,
humor, and vocabulary for a market. `ChannelOverride` replaces tone
and style entirely for content channels (docs vs social vs email).
`ResolveProfile()` applies both to produce the effective profile for a
(locale, channel) pair.

**MQM-inspired scoring.**

| Dimension          | What it measures                                      |
| ------------------ | ----------------------------------------------------- |
| `tone`             | Personality, formality, emotion alignment             |
| `style`            | Writing rules (active voice, sentence length, POV)    |
| `vocabulary`       | Preferred/forbidden/competitor term usage             |
| `clarity`          | Readability and comprehension                         |
| `brand_compliance` | Overall alignment                                     |

Severity weights: `neutral` = 0, `minor` = 1, `major` = 5,
`critical` = 25. Dimension score = 100 − sum of penalties (clamped).
Overall score = 100 − total penalties across dimensions.

**AI-powered compliance checking.** The `brand-voice-check` tool
([AD-framework-011: AI Providers](/docs/ad/011-ai-providers))
uses `ChatStructured` with a JSON Schema specifying dimension,
severity, message, and suggestion. Results become `BrandVoiceAnnotation`
entries and `brand-voice-score` / `brand-voice-findings` block
properties.

**Hierarchical binding.** Profiles are workspace-owned but bindable at
multiple levels. Each binding stores a profile ID, and resolution
picks the most specific:

```
Workspace  (Workspace.BrandVoiceProfileID)
  └─ Project  (Properties["brand_voice_profile_id"])
       └─ Stream  (Properties["brand_voice_profile_id"])
            └─ Collection  (ConnectorConfig["brand_voice_profile_id"])
```

`ResolveProfileFromContext()` performs the inheritance chain lookup,
channel resolution, and locale/channel override application in a single
call.

**Terminology integration.** Brand vocabulary flows through the same
pipeline as standard terminology
([AD-framework-010: Terminology](/docs/ad/010-terminology)).
`TermSource` distinguishes `"terminology"` from `"brand_vocabulary"`.
`CompetitorTerm` marks competitor brand terms. `SourceFilter` on
`LookupOptions` filters lookups by source, so the same `term-lookup`
and `term-enforce` tools enforce both concept sets without parallel
systems.

**Profile versioning and tags.** Every `UpdateProfile()` archives the
previous state as an immutable `ProfileVersion`. `ProfileTag` names a
version (e.g., `"v1.0-launch"`, `"pre-rebrand"`). `StoredScore` gains a
`ProfileVersion` field so historical scores are traceable to the exact
profile that produced them.

**Evaluation.** The `BrandVoiceEvaluation` compares two stream+profile
pairs — current vs. historical, experimental vs. baseline — producing
aggregate scores (overall, min, max, median, distribution), blast
radius (affected blocks, improvement/regression counts, critical
violations), per-dimension comparison, and top findings with source
snippets for in-context review.

**Starter packs.** Five `go:embed`-bundled YAML profiles ship with
bowrain:

| Pack               | Formality | Personality                    | Use case                |
| ------------------ | --------- | ------------------------------ | ----------------------- |
| `professional-b2b` | formal    | knowledgeable, authoritative   | Enterprise software     |
| `friendly-dtc`     | casual    | friendly, approachable         | Consumer products       |
| `technical-docs`   | technical | precise, informative           | Developer documentation |
| `marketing-blog`   | neutral   | engaging, creative             | Content marketing       |
| `customer-support` | neutral   | empathetic, helpful            | Support interactions    |

## Consequences

- Translation scales to workspace-sized batches without blocking
  clients; quota metering prevents unbounded spend in multi-tenant
  deployments.
- Azure Managed Identity removes API key management for the default
  deployment while keeping key-based providers available.
- Extraction discovers terminology and entities as a side effect of
  pushing content — translators start with a curated termbase.
- Hybrid LLM + NER keeps extraction cost predictable: NER handles
  obvious types cheaply, LLM handles domain classification.
- Brand voice governance reuses the terminology pipeline rather than
  running a parallel enforcement system.
- Profile versioning and evaluation make voice changes measurable and
  reviewable before rollout.

## Related

- [AD-framework-011: AI Providers](/docs/ad/011-ai-providers) — `LLMProvider` interface
- [AD-framework-010: Terminology](/docs/ad/010-terminology) — terminology pipeline
- [AD-013: Automation Engine](013-automation-engine.md) — triggers for translation and extraction
- [AD-014: Translator Workflow](014-translator-workflow.md) — review queue, tasks, notifications
- [AD-016: Bravo Agent](016-bravo-agent.md) — MCP consumer of brand voice and extraction tools
- [Translation Job Queue](/bowrain/notes/translation-job-queue) — worker and quota details
- [Entity & Term Extraction](/bowrain/notes/entity-term-extraction) — extraction pipeline internals
- [Brand Voice Data Model](/bowrain/notes/brand-voice-data-model) — schema and resolution
