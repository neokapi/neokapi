---
id: 010-terminology
sidebar_position: 10
title: "ADR-010: Terminology and Brand Management"
---
# ADR-010: Terminology and brand management system

## Context

Terminology management in localization ranges from simple glossaries (CSV with
source/target pairs) to concept-oriented termbases (TBX, MultiTerm) to full
brand governance platforms (Acrolinx, Writer.com). No existing tool integrates
concept-oriented terminology with a streaming localization pipeline as
first-class tools. gokapi needs progressive complexity: start simple (CSV),
grow into concept management and brand governance.

Key gaps in the market:

- No tool integrates concept-oriented terminology with a streaming pipeline
- Multi-dimensional context (domain x product x market x time) requires
  separate termbases in existing tools rather than dimensions within entries
- What-if experimentation for terminology changes does not exist
- No open-source system bridges terminology management and brand governance
- AI-assisted term extraction and enforcement are bolted on rather than native

Standards: **TBX** (ISO 30042:2019) is the universal interchange format for
concept-oriented terminological data. **CSV/TSV** provides simple glossary
import. TBX is used for import/export; native storage uses SQLite.

## Decision

### Architecture Overview

Progressive complexity model: Terminology Store (Phase 1) -> Concept
Management (Phase 2) -> Brand Governance (Phase 3).

Shared SQLite infrastructure with Bowrain Memory TM ([ADR-009](./009-translation-memory.md))
and Content Store ([ADR-003](./003-content-store.md)) via `internal/storage/`.

### Data Model: Concept-Oriented

The core data model is concept-oriented, following TBX principles. A Concept
groups terms across languages, each with context dimensions:

```go
type Concept struct {
    ID            string
    SubjectFields []string          // domain tags: "medical", "ui"
    Definition    string
    Notes         string
    Relations     []ConceptRelation // links to other concepts (Phase 2)
    Terms         []Term
    Properties    map[string]string
    Status        ConceptStatus     // draft, active, deprecated
    CreatedAt     time.Time
    UpdatedAt     time.Time
}

type Term struct {
    ID           string
    Text         string
    Locale       model.LocaleID
    Status       TermStatus  // proposed, approved, preferred,
                             // admitted, deprecated, forbidden
    Type         TermType    // fullForm, abbreviation, acronym, shortForm, variant
    PartOfSpeech string
    Context      TermContext
    CreatedAt    time.Time
}

type TermContext struct {
    Products   []string  // "Gokapi CLI", "Bowrain"
    Markets    []string  // "US", "EMEA", "JP"
    Audiences  []string  // "developer", "end-user"
    ValidFrom  time.Time // temporal validity start
    ValidUntil time.Time // temporal validity end
}
```

Progressive disclosure: CSV import auto-creates Concepts with a single
preferred Term per locale -- no extra complexity required.

### TermBase Interface

```go
type TermBase interface {
    AddConcept(concept Concept) error
    GetConcept(id string) (*Concept, error)
    UpdateConcept(concept Concept) error
    DeleteConcept(id string) error
    AddTerm(conceptID string, term Term) error
    LookupTerm(text string, locale model.LocaleID, opts LookupOptions) ([]TermMatch, error)
    Search(query SearchQuery) ([]Concept, error)
    Import(reader io.Reader, format ImportFormat) (ImportResult, error)
    Export(writer io.Writer, format ExportFormat) error
    Close() error
}
```

Backends: In-memory (CLI batch) and SQLite (persistent). Both use the shared
`internal/storage` layer from [ADR-003](./003-content-store.md).

### Term Lookup: Tiered Matching

Default pipeline: exact -> normalized -> fuzzy (Levenshtein). Opt-in:
stem matching (Snowball stemmers) and AI-assisted matching (LLM provider
from [ADR-008](./008-ai-integration.md)). The architecture is extensible via a
`Matcher` interface.

### Pipeline Tools

Six pipeline tools integrate terminology, entity annotation, and privacy
into the streaming pipeline ([ADR-006](./006-tool-system.md)):

**`term-lookup`** (Enrich) -- Scans source text for known terms, attaches
`TermAnnotation` with `TextRange` character positions. Downstream tools
(AI translate, QA) use these annotations for context.

**`term-enforce`** (Validate) -- Checks preferred term usage in target text.
Reports forbidden terms, non-preferred variants, deprecated terms, and
missing target counterparts.

**`term-extract`** (Enrich, AI) -- LLM extraction of candidate terms with
`status: proposed`. Uses AI provider from [ADR-008](./008-ai-integration.md).

**`entity-annotate`** (Enrich, AI) -- Named entity annotation (people,
organizations, products, dates, locations). Serves multiple purposes:
TM generalization in Bowrain Memory ([ADR-009](./009-translation-memory.md)),
do-not-translate markers, localization hints, and terminology candidate
discovery. Should run early in the pipeline -- before `tm-leverage`.

**`redact`** (Transform) -- Privacy tool replacing entity values with typed
placeholders (e.g., "John" -> `\{PERSON\}`) before external services.
Orthogonal to TM generalization, which handles matching natively via
derived keys ([ADR-009](./009-translation-memory.md)).

**`unredact`** (Transform) -- Restores original entity values after external
processing. Paired with `redact`:
`reader -> entity-annotate -> redact -> [external MT] -> unredact -> writer`

### Concept Relations (Phase 2)

broader/narrower, related, supersedes, see-also. Enables concept graph
navigation in Bowrain ([ADR-012](./012-bowrain.md)).

### Terminology Streams (Phase 2)

Named what-if experiments for terminology changes:

```go
type Stream struct {
    ID          string
    Name        string       // "Q2 Rebrand"
    Changes     []StreamChange
    Status      StreamStatus // draft, reviewing, promoted, discarded
}
```

Streams isolate changes from the active termbase until explicitly promoted.
Side-by-side preview in Bowrain: current terms vs. stream terms applied
to content. Promotion applies changes atomically.

### Brand Voice (Phase 3)

Brand voice rules (tone, style) with a `brand-voice-check` pipeline tool
using LLM analysis ([ADR-008](./008-ai-integration.md)). Positions gokapi as
the only open-source system bridging terminology and brand governance.

### KAZ Integration

KAZ archives embed a read-only terminology snapshot (`terms/concepts.json`)
for offline/sharing use. Master termbase is managed externally. On project
open, Bowrain checks freshness and offers to refresh the snapshot.

### Content Model Extensions

Two annotation types implement the `Annotation` interface with character-level
`TextRange` positions for precise inline highlighting in Bowrain
([ADR-012](./012-bowrain.md)):

- `TermAnnotation` -- matched term with concept, target terms, and position
- `EntityAnnotation` -- named entity with type, DNT flag, and position

These join `AltTranslation` as first-class annotations on Blocks
([ADR-002](./002-content-model.md)).

## Alternatives Considered

**Embed in Bowrain Memory (TM)**: Terminology has fundamentally different data
requirements (concept-orientation, lifecycle, relations). Separate systems
sharing SQLite infrastructure is the right balance.

**External terminology server**: Adds deployment complexity and defeats the
single-binary goal.

**TBX as native format**: Verbose, hard to query, lacks performance for
real-time lookup. TBX for import/export only.

**Git-like branching**: Too complex. Streams provide essential what-if
capability without merge conflicts.

## Consequences

- Terminology is first-class in the pipeline, not a bolt-on
- Progressive complexity: CSV glossary to concept management to brand governance
- Shared SQLite infrastructure with TM ([ADR-009](./009-translation-memory.md))
  and Content Store ([ADR-003](./003-content-store.md))
- Character-level annotation positions enable precise Bowrain highlighting
  ([ADR-012](./012-bowrain.md))
- Entity annotation drives both terminology and TM generalization
  ([ADR-009](./009-translation-memory.md))
- Terminology streams enable rebranding workflows with content preview
- TBX import/export provides interoperability with all major localization tools
