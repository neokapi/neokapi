---
id: 016-terminology-and-brand-management
sidebar_position: 16
title: "ADR-016: Terminology and Brand Management"
---
# ADR-016: Terminology and brand management system

## Context

Terminology management in localization ranges from simple glossaries (CSV with
source/target pairs) to concept-oriented termbases (TBX, MultiTerm) to full
brand governance platforms (Acrolinx, Writer.com, Bynder). No existing tool
spans this entire spectrum well, and none integrates deeply with a localization
pipeline.

gokapi currently has a minimal `ai-terminology` tool that extracts terms from
Blocks via LLM and stores them as JSON in `Block.Properties["terminology"]`.
This is useful for discovery but provides no persistent term storage,
lifecycle management, concept modeling, enforcement, or UI.

### Industry Landscape

**Translation-focused tools** (MultiTerm, memoQ, Phrase, crossTerm) offer
concept-oriented termbases with lifecycle workflows and deep CAT tool
integration, but lack brand voice management and treat terminology in isolation
from broader content strategy.

**Brand management platforms** (Acrolinx, Writer.com, Grammarly Business,
Bynder, Frontify) excel at brand voice enforcement, tone management, and visual
asset governance, but lack multilingual terminology depth and translation
workflow integration.

**Key gaps in the market** (opportunities for gokapi):

- No tool integrates concept-oriented terminology with a streaming localization
  pipeline as first-class pipeline tools
- Time-based term validity (term valid 2020-2024, then superseded) is rarely
  supported
- Multi-dimensional context (domain x product x market x time) requires
  separate termbases in existing tools rather than dimensions within entries
- What-if experimentation for terminology changes (e.g., "preview the impact
  of renaming product X across all content") does not exist
- No open-source system bridges the gap between terminology management and
  brand governance
- AI-assisted term extraction, enforcement, and evolution tracking are bolted
  on rather than natively integrated

### Standards

- **TBX** (ISO 30042:2019): XML format for concept-oriented terminological
  data. The universal interchange format — virtually all serious terminology
  tools support TBX import/export. Concept-oriented by design.
- **OLIF**: Open Lexicon Interchange Format. Designed for computational
  lexicons and MT systems. Less widely adopted than TBX; development stagnant.
- **CSV/TSV**: Simple tabular glossary format. Universal compatibility but no
  concept modeling, limited metadata, no standardization.

## Decision

### Architecture Overview

Introduce a terminology and brand management subsystem alongside Sievepen (TM).
The system follows a **progressive complexity** model: simple to start (import a
CSV glossary) but capable of growing into a full brand management solution.

```
                    ┌─────────────────────┐
                    │   Brand Governance   │  ← Phase 3
                    │ voice, tone, style   │
                    ├─────────────────────┤
                    │  Concept Management  │  ← Phase 2
                    │ lifecycle, relations,│
                    │ streams, history     │
                    ├─────────────────────┤
                    │  Terminology Store   │  ← Phase 1
                    │ terms, translations, │
                    │ context, annotations │
                    └──────────┬──────────┘
                               │
           ┌───────────────────┼───────────────────┐
           │                   │                   │
    Pipeline Tools      Bowrain Module       CLI / API
    (lookup, enforce,   (browse, edit,      (import, export,
     annotate, extract)  review, approve)    query, validate)
```

### Shared Storage Infrastructure

TermBase and Sievepen (TM) share a common internal SQLite infrastructure
layer (`internal/storage/`) that provides connection management, migration
framework, and query helpers. Each system defines its own schema and query
logic on top of this shared foundation. This reduces code duplication and
ensures consistent behavior (connection pooling, WAL mode, vacuum policy)
across both systems.

```
internal/storage/
    sqlite.go           # connection pool, WAL mode, shared pragmas
    migrate.go          # migration runner (schema versioning)
    query.go            # common query helpers
lib/sievepen/
    persistent.go       # TM schema + queries (uses internal/storage)
lib/termbase/
    persistent.go       # TermBase schema + queries (uses internal/storage)
```

### Phase 1: Terminology Store and Pipeline Tools

#### Data Model: Concept-Oriented with Progressive Disclosure

The core data model is concept-oriented, following TBX principles but
simplified for practical use. A Concept groups terms across languages, each
with context dimensions.

```go
// Concept is the central unit — an idea or meaning that has
// linguistic representations (Terms) in one or more languages.
type Concept struct {
    ID            string
    SubjectFields []string          // domain/subject tags: "medical", "ui"
    Definition    string            // language-independent definition
    Notes         string            // usage guidance
    Relations     []ConceptRelation // links to other concepts
    Terms         []Term            // linguistic representations
    Properties    map[string]string // extensible metadata
    CreatedAt     time.Time
    UpdatedAt     time.Time
    CreatedBy     string
    Status        ConceptStatus     // draft, active, deprecated
}

// Term is a linguistic representation of a Concept in a specific locale.
type Term struct {
    ID           string
    Text         string            // the term itself
    Locale       LocaleID
    Status       TermStatus        // proposed, approved, preferred,
                                   // admitted, deprecated, forbidden
    Type         TermType          // fullForm, abbreviation, acronym,
                                   // shortForm, variant
    PartOfSpeech string            // noun, verb, adjective, etc.
    Gender       string            // masculine, feminine, neuter (if applicable)
    Note         string            // usage note
    Source       string            // where term was sourced from
    Context      TermContext       // multi-dimensional context
    CreatedAt    time.Time
    UpdatedAt    time.Time
    CreatedBy    string
}

// TermContext provides multi-dimensional scoping for when/where a term applies.
// Context dimensions are flat tags in Phase 1. Phase 2 may introduce first-class
// entities (Brand, Product) that organize these tags into navigable hierarchies.
type TermContext struct {
    Products   []string  // product names: "Gokapi CLI", "Bowrain"
    Markets    []string  // market/region: "US", "EMEA", "JP"
    Audiences  []string  // target audience: "developer", "end-user"
    ValidFrom  time.Time // temporal validity start (zero = always)
    ValidUntil time.Time // temporal validity end (zero = no expiry)
}

type TermStatus string
const (
    TermProposed   TermStatus = "proposed"
    TermApproved   TermStatus = "approved"
    TermPreferred  TermStatus = "preferred"   // use this term
    TermAdmitted   TermStatus = "admitted"    // acceptable alternative
    TermDeprecated TermStatus = "deprecated"  // was valid, now outdated
    TermForbidden  TermStatus = "forbidden"   // never use this term
)

type ConceptStatus string
const (
    ConceptDraft      ConceptStatus = "draft"
    ConceptActive     ConceptStatus = "active"
    ConceptDeprecated ConceptStatus = "deprecated"
)

type ConceptRelation struct {
    TargetID string       // ID of related concept
    Type     RelationType // broader, narrower, related, supersedes
}
```

**Progressive disclosure**: A user importing a CSV glossary gets Concepts
auto-created with a single preferred Term per locale — no extra complexity.
Users who need concept modeling, lifecycle management, or context dimensions
opt in incrementally.

#### Storage: TermBase with Backend Abstraction

Follow the Sievepen pattern with an interface and multiple backends, sharing
the common SQLite infrastructure layer:

```go
type TermBase interface {
    // Concept CRUD
    AddConcept(concept Concept) error
    GetConcept(id string) (*Concept, error)
    UpdateConcept(concept Concept) error
    DeleteConcept(id string) error

    // Term operations
    AddTerm(conceptID string, term Term) error
    UpdateTerm(conceptID string, term Term) error
    DeleteTerm(conceptID, termID string) error

    // Lookup
    LookupTerm(text string, locale LocaleID, opts LookupOptions) ([]TermMatch, error)
    Search(query SearchQuery) ([]Concept, error)

    // Bulk
    Import(reader io.Reader, format ImportFormat) (ImportResult, error)
    Export(writer io.Writer, format ExportFormat) error

    // Metadata
    Count() int
    Locales() []LocaleID
    SubjectFields() []string
    Close() error
}
```

**Backends**:

1. **In-memory**: Fast, session-scoped. For CLI batch processing.
2. **SQLite**: Persistent, using the shared `internal/storage` layer. For
   project and organization termbases. Pure Go via `modernc.org/sqlite`.

**Import/export formats** (Phase 1):
- CSV/TSV (simple glossary)
- TBX (ISO standard, concept-oriented)
- JSON (native format)

#### KAZ Archive: Snapshot + External Reference

KAZ archives embed a read-only terminology snapshot (`termbase.json`) for
offline and sharing use cases. The master termbase is managed externally
(SQLite file or server). On project open, Bowrain checks whether the
snapshot is stale compared to the master and offers to refresh it.

```
project.kaz
├── manifest.yaml
├── blocks/
├── preview/
├── items/
└── termbase.json       # read-only snapshot of the project's termbase
```

The snapshot contains all concepts and terms relevant to the project's
source/target locales and subject fields. This makes archives
self-contained for sharing via email, cloud storage, or version control,
while the external master termbase remains the source of truth for ongoing
work.

#### Term Lookup: Tiered Matching with Extensible Strategies

```go
type LookupOptions struct {
    MinScore       float64       // fuzzy match threshold (default 0.85)
    MaxResults     int
    IncludeStatuses []TermStatus // filter by status
    SubjectFields  []string      // filter by domain
    Products       []string      // filter by product context
    Markets        []string      // filter by market context
    Strategies     []MatchStrategy // override default matching pipeline
}

type MatchStrategy string
const (
    MatchExact      MatchStrategy = "exact"      // exact string match
    MatchNormalized MatchStrategy = "normalized"  // case, whitespace, diacritics
    MatchFuzzy      MatchStrategy = "fuzzy"       // Levenshtein edit distance
    MatchStem       MatchStrategy = "stem"        // linguistic stemming (opt-in)
    MatchAI         MatchStrategy = "ai"          // LLM-assisted matching (opt-in)
)

type TermMatch struct {
    Concept     *Concept
    SourceTerm  *Term   // matched source term
    TargetTerms []Term  // target terms for the requested locale
    Score       float64
    MatchType   MatchStrategy
}
```

The default matching pipeline is: exact → normalized → fuzzy. This covers
the dominant use cases without language-dependent complexity.

Two additional strategies are available as opt-in:

- **Stem matching**: Uses linguistic stemmers (e.g., Snowball) for major
  languages. Enabled per-termbase or per-lookup via `Strategies` option.
  Adds a dependency on stemmer libraries but improves recall for inflected
  forms (matching "running" when the term is "run").

- **AI-assisted matching**: Uses the configured LLM provider to determine
  matches in ambiguous cases. Slowest and most expensive strategy — suitable
  for batch processing or edge cases where exact/fuzzy matching fails.
  Enabled explicitly via `Strategies` option, never part of the default
  pipeline.

The architecture is extensible: new matching strategies can be added by
implementing a `Matcher` interface and registering with the lookup pipeline.

#### Pipeline Tools

Six pipeline tools integrate terminology, entity annotation, and privacy
into the streaming pipeline:

**`term-lookup`** — Category: Enrich

Scans Block source text for known terms. Attaches `TermAnnotation`
annotations to Blocks where terms are found, including the matched concept,
available target terms, and character-level position ranges within the
source text. Downstream tools (AI translate, QA) use these annotations for
context.

```go
// TermAnnotation carries a matched term with its position in the source text.
// Implements the Annotation interface.
type TermAnnotation struct {
    SourceTerm  string        // as found in source text
    ConceptID   string
    TargetTerms []TermRef     // preferred translations per locale
    Status      TermStatus    // preferred, admitted, forbidden, etc.
    Position    TextRange     // character offset range in source text
    Score       float64       // match confidence (1.0 for exact)
    MatchType   MatchStrategy // how it was matched
}

// TextRange represents a character offset range within a text.
type TextRange struct {
    Start int // inclusive start offset
    End   int // exclusive end offset
}
```

**`term-enforce`** — Category: Validate

Checks that preferred terms are used consistently in target text. Reports
issues when:
- A forbidden term appears in the target
- A preferred term has a non-preferred variant in the target
- A deprecated term is used
- A source term is found but its target counterpart is missing

**`term-extract`** — Category: Enrich (AI-powered)

Enhanced version of the current `ai-terminology` tool. Uses LLM to extract
candidate terms from source Blocks. Instead of storing raw JSON in
properties, creates proper TermAnnotation entries with `status: proposed`.
Extracted terms can be reviewed and promoted to approved/preferred via
Bowrain or CLI.

**`entity-annotate`** — Category: Enrich (AI-powered)

Annotates named entities in source text: people, organizations, products,
dates/times, places, currencies, measurements. These annotations serve
multiple purposes:
- **TM generalization**: Sievepen's content-aware TM (ADR-010) reads entity
  annotations to compute generalized matching keys, enabling "John works at
  Acme" to match "Alice works at Globex" at 100% with automatic entity
  adaptation in the target
- Do-not-translate markers for proper names
- Localization hints for date/number formats
- Context for AI translation
- Source for terminology candidate discovery

The `entity-annotate` tool is the single source of entity information for
both TM generalization and terminology management. It should run early in
the pipeline — before `tm-leverage` — so that entity annotations are
available for generalized matching.

```go
// EntityAnnotation carries a named entity with its position in source text.
// Implements the Annotation interface.
type EntityAnnotation struct {
    Text     string
    Type     EntityType  // person, organization, product, location,
                         // date, time, currency, measurement, other
    Position TextRange   // character offset range in source text
    Locale   LocaleID    // locale-specific formatting hint
    DNT      bool        // do-not-translate flag
}

type EntityType string
const (
    EntityPerson       EntityType = "person"
    EntityOrganization EntityType = "organization"
    EntityProduct      EntityType = "product"
    EntityLocation     EntityType = "location"
    EntityDate         EntityType = "date"
    EntityTime         EntityType = "time"
    EntityCurrency     EntityType = "currency"
    EntityMeasurement  EntityType = "measurement"
    EntityOther        EntityType = "other"
)
```

**`redact`** — Category: Transform

Replaces entity values in Block source text with typed placeholders (e.g.,
"John" → `{PERSON}`), using entity annotations produced by
`entity-annotate`. This is a **privacy tool** — it prevents sensitive
information (personal names, addresses, etc.) from reaching external
services like AI translation or MT providers. The redacted content uses
`SpanPlaceholder` Spans in the Fragment's coded text representation.

Redaction is orthogonal to TM generalization. Sievepen's content-aware TM
(ADR-010) achieves generalized matching natively by computing derived
matching keys from entity annotations — it does not need or use redacted
content. The `redact` tool exists purely for cases where entity values must
not leave the local environment.

**`unredact`** — Category: Transform

Restores original entity values from placeholders after external processing
is complete. Uses the same entity annotations and `SpanPlaceholder` Spans
to reverse the substitution. Typically paired with `redact` at the
opposite end of a pipeline:

```
reader → entity-annotate → redact → [external MT] → unredact → writer
```

#### Content Model Extensions

Two new annotation types implement the `Annotation` interface, both carrying
character-level `TextRange` positions for precise inline highlighting:

```go
func (ta *TermAnnotation) AnnotationType() string   { return "term" }
func (ea *EntityAnnotation) AnnotationType() string  { return "entity" }
```

These join `AltTranslation` as first-class annotations on Blocks. The
`Block.Annotations` map holds multiple annotations keyed by type and
instance (e.g., `"term:0"`, `"term:1"`, `"entity:0"`).

Character-level positions (`TextRange`) enable Bowrain to render precise
inline highlighting without re-detecting term boundaries at render time.
Pipeline tools produce positions when they detect terms; the positions
persist through the pipeline and into KAZ block indices.

#### Package Layout

```
internal/storage/
    sqlite.go            # shared SQLite connection pool, WAL mode, pragmas
    migrate.go           # shared migration runner (schema versioning)
    query.go             # common query helpers
lib/termbase/
    termbase.go          # TermBase interface, types, options
    concept.go           # Concept, Term, TermContext types
    memory.go            # in-memory backend
    persistent.go        # SQLite backend (uses internal/storage)
    lookup.go            # tiered matching pipeline
    matcher_exact.go     # exact + normalized matching
    matcher_fuzzy.go     # Levenshtein fuzzy matching
    matcher_stem.go      # linguistic stemming (opt-in)
    matcher_ai.go        # LLM-assisted matching (opt-in)
    import_csv.go        # CSV import
    import_tbx.go        # TBX import
    export_csv.go        # CSV export
    export_tbx.go        # TBX export
    export_json.go       # JSON export
lib/tools/
    termlookup.go        # term-lookup pipeline tool
    termenforce.go       # term-enforce pipeline tool
    redact.go            # entity redaction for privacy (Transform)
    unredact.go          # entity restoration after external processing
ai/tools/
    terminology.go       # enhanced term-extract (update existing)
    entityannotate.go    # entity-annotate tool
```

### Phase 2: Concept Management and Lifecycle

Build on Phase 1 with richer concept management for teams that need it.

#### Concept Relations and Hierarchies

Concepts can relate to each other:

- **broader/narrower**: "database" is broader than "SQL database"
- **related**: "authentication" is related to "authorization"
- **supersedes**: "cloud-native" supersedes "SaaS" in this product's
  terminology
- **see-also**: cross-reference without hierarchy

This enables a concept graph that can be browsed and navigated in Bowrain.

#### Term Lifecycle Workflow

```
proposed → under-review → approved → preferred
                                   → admitted
                                   → deprecated → archived
                       → rejected

forbidden (can be set directly, bypassing the approval flow)
```

Each transition records who, when, and why (comment). The lifecycle is
configurable — small teams can skip review steps and go directly from
proposed to preferred.

Term suggestions can be created inline during translation in Bowrain
(right-click on selected text → "Suggest term") AND managed in the
dedicated terminology panel. Inline suggestions feed into the same
lifecycle workflow, appearing in the moderation queue alongside terms
created through the terminology module or AI extraction. This dual-path
approach ensures terminology needs are captured at the moment of
discovery during translation while maintaining a centralized management
interface.

#### Version History

Every change to a Concept or Term is recorded with timestamp, author, and
change description. This enables:
- Audit trail for regulatory compliance
- Rollback to previous versions
- Diff view showing what changed and why

#### Terminology Streams

Streams are named what-if experiments for terminology changes. They enable
scenarios like:

- "What if we rename Product X to Product Y across all markets?"
- "What if we change these 15 medical terms for the EU market starting
  March 2026?"
- "Preview the impact of deprecating these legacy terms"

```go
// Stream represents a named what-if experiment over a termbase.
// Changes within a stream are isolated from the active termbase until
// explicitly promoted.
type Stream struct {
    ID          string
    Name        string            // "Q2 Rebrand", "EU Medical Terms v2"
    Description string
    TargetDate  time.Time         // planned activation date (informational,
                                  // not auto-triggered)
    Changes     []StreamChange    // term additions, modifications, removals
    Status      StreamStatus      // draft, reviewing, promoted, discarded
    CreatedAt   time.Time
    CreatedBy   string
}

type StreamStatus string
const (
    StreamDraft     StreamStatus = "draft"
    StreamReviewing StreamStatus = "reviewing"
    StreamPromoted  StreamStatus = "promoted"  // changes applied to active termbase
    StreamDiscarded StreamStatus = "discarded"
)

type StreamChange struct {
    Type      ChangeType // add, modify, deprecate, remove
    ConceptID string
    TermID    string     // empty for concept-level changes
    Before    *Term      // nil for additions
    After     *Term      // nil for removals
}
```

**Stream mechanics**:

- A stream starts as a draft — users make term changes within the stream
  without affecting the active termbase.
- Streams have an optional target date (e.g., "activate March 1, 2026") that
  serves as a planning aid and reminder. The target date is informational
  only — promotion always requires explicit human action.
- Multiple people can collaborate on the same stream.
- Bowrain provides a side-by-side preview: "show me how Block X translates
  with current terms vs. with the Q2 Rebrand stream active." This enables
  stakeholder review and sign-off before promotion.
- When promoted, stream changes are applied to the active termbase as a
  single atomic operation. The stream is marked `promoted` and retained for
  audit purposes.
- Discarded streams are archived but not deleted, preserving the history of
  what was considered.

**Bowrain stream preview**:

The Bowrain translation editor supports a split preview mode for streams:

```
┌──────────────────────────────────────────────────┐
│  Stream: Q2 Rebrand (target: March 1, 2026)      │
├────────────────────┬─────────────────────────────┤
│  Current Terms     │  With Stream Applied         │
├────────────────────┼─────────────────────────────┤
│  "Platform X" →    │  "CloudSuite" →              │
│  "プラットフォームX" │  "クラウドスイート"            │
├────────────────────┼─────────────────────────────┤
│  Block 42:         │  Block 42:                   │
│  "Connect to       │  "Connect to                 │
│   Platform X..."   │   CloudSuite..."             │
└────────────────────┴─────────────────────────────┘
```

This preview shows both term-level diffs and content-level impact,
enabling informed promotion decisions.

#### Context Dimension Evolution

Phase 1 uses flat tags for context dimensions (products, markets,
audiences). Phase 2 evaluates whether to introduce first-class entities
(Brand, Product) that organize these tags into navigable hierarchies.
The decision depends on real-world usage patterns observed during Phase 1.

Possible evolution:
- If users naturally create consistent tag conventions (e.g.,
  "product:bowrain", "product:kapi-cli"), first-class entities are
  likely valuable.
- If tag usage remains ad-hoc and varied, flat tags with search/filter
  are sufficient.

The Phase 1 flat-tag design explicitly avoids premature abstraction while
keeping the door open for entity modeling.

### Phase 3: Brand Governance

Extend terminology into broader brand management. Brand voice and tone
rules are part of the gokapi vision, providing a unique differentiation
as the only open-source system that integrates terminology management
with brand governance in a localization pipeline.

#### Brand Voice and Tone Rules

Beyond individual terms, manage brand voice characteristics:

```go
// BrandVoice defines voice and style rules for a particular context
// (e.g., developer documentation, marketing copy, UI strings).
type BrandVoice struct {
    ID          string
    Name        string            // "Developer Documentation", "Marketing"
    Tones       []ToneRule        // formal/casual, technical/accessible
    StyleRules  []StyleRule       // sentence length, active voice, etc.
    Terminology *TermBase         // associated termbase
    Products    []string          // applicable products
    Markets     []string          // applicable markets
    Locales     []LocaleID        // applicable languages
}

type ToneRule struct {
    Dimension string   // "formality", "technicality", "empathy"
    Target    string   // "formal", "technical", "empathetic"
    Examples  []string // example sentences demonstrating the tone
    Guidance  string   // explanation for translators/writers
}

type StyleRule struct {
    Name       string  // "max-sentence-length", "active-voice"
    Value      string  // "25", "preferred"
    Severity   string  // "error", "warning", "suggestion"
}
```

A `brand-voice-check` pipeline tool validates content against brand voice
rules using LLM analysis, reporting tone/style issues as annotations.
This integrates naturally with the existing AI tool architecture (ADR-009)
and the pipeline (ADR-003).

#### Product/Initiative Context (Brand Concept Management)

For companies with multiple products, initiatives, and campaigns, terminology
needs to be managed in the context of:

- **Products**: Each product may have its own terminology (e.g., feature names)
- **Initiatives**: Rebranding campaigns, product launches introduce new terms
  and deprecate old ones (modeled as streams in Phase 2)
- **Markets**: Terms may differ by market (US vs. UK English, France vs.
  Canada French)
- **Time**: Campaign-specific terms have start/end dates

The `TermContext` in Phase 1 provides the data model hooks via flat tags.
Phase 3 adds UI for managing these dimensions and reporting on consistency
across them. If Phase 2 usage patterns indicate the need for entity
hierarchies, Phase 3 introduces first-class Brand/Product entities.

### Bowrain Terminology Module

A dedicated Bowrain module for terminology management (see ADR-012):

**Browse and Search**
- Faceted search by locale, domain, product, status
- Concept graph visualization (broader/narrower/related)
- Term concordance: show all Blocks where a term appears

**Edit and Review**
- Create/edit concepts and terms
- Inline term suggestion during translation (right-click → suggest term)
- Moderation queue for proposed terms
- Approval workflow with comments

**Import and Export**
- CSV, TBX, JSON import with field mapping UI
- Export in any supported format
- Merge imported terms with existing termbase (conflict resolution)

**Analytics**
- Term usage statistics across project content
- Coverage: what percentage of source terms have approved translations
- Consistency: how often are preferred terms used vs. variants
- Freshness: terms not reviewed since a configurable date

**Real-Time Term Recognition**

The translation editor highlights recognized terms inline in real-time as
the user navigates between Blocks. An in-memory term index is loaded from
the termbase on project open (or from the KAZ snapshot when working
offline). Recognized terms are rendered with underline or colored background;
hovering shows definition, preferred translation, and status. This is always
on — passive discovery of terminology is essential for translator awareness.

The in-memory index is optimized for fast prefix and substring lookup using
an Aho-Corasick automaton or similar multi-pattern matcher, enabling
efficient scanning of Block source text against potentially thousands of
terms.

**Stream Preview**

The editor supports a side-by-side split view comparing current terminology
against a stream's terms applied to actual project content. This enables
stakeholder sign-off on rebranding or terminology evolution before
promotion.

### CLI Integration

```bash
# Import/export
kapi termbase import glossary.csv --format csv --source en --target fr
kapi termbase import terms.tbx --format tbx
kapi termbase export --format tbx --output terms.tbx

# Query
kapi termbase lookup "cloud computing" --locale en --domain technology
kapi termbase search --status preferred --locale ja

# Pipeline with entity-aware TM leverage (ADR-010)
kapi flow run --flow "reader -> entity-annotate -> term-lookup -> tm-leverage -> ai-translate -> term-enforce -> writer"

# Entity annotation only
kapi flow run --flow "reader -> entity-annotate -> writer"

# Pipeline with privacy redaction for external MT
kapi flow run --flow "reader -> entity-annotate -> redact -> mt-translate -> unredact -> writer"

# Statistics
kapi termbase stats --locale en --domain technology

# Streams (Phase 2)
kapi termbase stream create "Q2 Rebrand" --target-date 2026-03-01
kapi termbase stream preview "Q2 Rebrand" --project project.kaz
kapi termbase stream promote "Q2 Rebrand"
```

## Comparison with Existing Solutions

| Capability | MultiTerm | memoQ | crossTerm | Acrolinx | Writer.com | **gokapi (planned)** |
|---|---|---|---|---|---|---|
| Concept-oriented | Yes | Yes | Yes | Basic | Basic | **Yes** |
| Term lifecycle | Basic | Moderation | Excellent | Governance | Style evolution | **Configurable workflow** |
| Multi-dimensional context | Domains only | Domains | Instances | Context-aware | Team-based | **Domain x product x market x time** |
| AI extraction | AI extract | Limited | Limited | Excellent | Excellent | **LLM-native, pipeline-integrated** |
| Entity annotation | No | No | No | No | No | **Yes (names, dates, places)** |
| Pipeline integration | Trados only | memoQ only | Across only | Standalone | Standalone | **First-class pipeline tools** |
| What-if streams | No | No | No | No | No | **Yes (side-by-side preview)** |
| Brand voice | No | No | No | Excellent | Excellent | **Phase 3 (tone + style rules)** |
| Version control | Last modified | Last modified | Workflow chain | Governance trail | Limited | **Full history, rollback** |
| Forbidden terms | QA flag | QA flag | Workflow | Content check | Style rules | **Pipeline enforcement tool** |
| Open source | No | No | No | No | No | **Yes** |
| TBX support | Export | Import/Export | Export | No | No | **Import/Export** |
| Offline-capable | Desktop | Desktop/Server | Server | Cloud | Cloud | **Desktop + CLI** |

**Key differentiators**:

1. **Pipeline-native**: gokapi is the only system where terminology is both
   a standalone management system AND composable pipeline tools. Terms flow
   through the same channel-based pipeline as translations.

2. **Terminology streams**: No existing tool supports what-if experiments
   with side-by-side content preview. This is uniquely powerful for
   rebranding and terminology evolution decisions.

3. **Entity annotation**: No localization framework offers named entity
   annotation as a pipeline tool with DNT flags, localization hints, and
   native TM generalization (ADR-010) driven by the same annotations.

4. **Open-source brand governance**: No open-source system bridges
   terminology management and brand voice/tone rules.

## Alternatives Considered

### Embed terminology in Sievepen (TM)

Reuse the TM storage for terminology entries. Rejected because terminology
has fundamentally different data requirements: concept-orientation, lifecycle
management, multi-dimensional context, and relations between concepts. Sharing
a data model would over-constrain both systems. However, the systems share
underlying SQLite infrastructure (`internal/storage/`) to avoid code
duplication.

### Use an external terminology server

Integrate with MultiTerm Server, SDL Cloud, or a dedicated terminology API.
Rejected because it adds deployment complexity and defeats the single-binary
goal. However, a future connector layer could integrate with external
termbases as a sync target.

### Start with brand governance (Phase 3 first)

Build brand voice management from the start. Rejected because terminology
is the foundation — brand voice depends on having a solid term management
system. Progressive complexity ensures each phase is independently useful.

### Use TBX as the native storage format

Store terminology directly in TBX XML. Rejected because TBX is an
interchange format, not a working format — it's verbose, hard to query, and
lacks performance characteristics needed for real-time lookup during pipeline
processing. TBX is supported for import/export only.

### Redact/unredact pipeline tools for TM generalization

Use separate pipeline tools to replace entities with placeholders before
TM lookup, and restore them after. Rejected because it conflates privacy
redaction with TM matching optimization. Making the TM content-aware
(ADR-010) handles generalization internally — the right layer for this
intelligence. Redaction for privacy remains a separate, orthogonal pipeline
tool in this ADR.

### Git-like branching for terminology

Full git-style branching with merge conflict resolution. Rejected in favor
of the simpler streams model. Streams avoid git-level complexity
(merge conflicts, branch management) while providing the essential
capability: isolated what-if experiments with preview and atomic promotion.

## Consequences

- Terminology becomes a first-class citizen in the gokapi pipeline, not a
  bolt-on
- The progressive complexity model means users can start with a CSV glossary
  and grow into full concept management with streams and brand governance
- Shared SQLite infrastructure between TermBase and Sievepen reduces code
  duplication and ensures consistent database behavior
- The TermBase interface enables future backends (PostgreSQL for server
  deployments, cloud storage for distributed teams)
- TBX import/export provides interoperability with the entire localization
  tool ecosystem
- Character-level annotation positions enable precise inline highlighting
  in Bowrain without re-detection at render time
- Entity annotation is a novel capability that no existing localization
  framework offers as a pipeline tool — and the same annotations drive
  Sievepen's content-aware TM generalization (ADR-010), making
  `entity-annotate` the single source of entity information for both
  terminology and translation memory
- Privacy redaction (`redact`/`unredact` tools) is orthogonal to TM
  generalization — the TM handles generalization natively via derived
  matching keys, while redaction exists purely for preventing sensitive
  data from reaching external services
- The tiered matching pipeline (exact → normalized → fuzzy, with opt-in
  stemming and AI) balances speed with flexibility — fast by default,
  linguistically rich when needed
- Terminology streams enable rebranding and terminology evolution workflows
  that no existing tool supports, with side-by-side content preview for
  stakeholder sign-off
- Inline term suggestion during translation captures terminology needs at
  the moment of discovery, feeding the same lifecycle workflow as the
  dedicated terminology module
- Real-time term highlighting in Bowrain provides passive terminology
  awareness for translators without requiring explicit lookup actions
- Brand voice and tone rules (Phase 3) position gokapi as the only
  open-source system bridging terminology management and brand governance
