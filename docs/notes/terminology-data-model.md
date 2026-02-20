---
sidebar_position: 5
title: "Terminology Data Model"
---
# Terminology Data Model

This note provides implementation details for [AD-010](/docs/ad/010-terminology).

## Data Model: Concept-Oriented

The core data model is concept-oriented, following TBX principles. A Concept groups terms across languages, each with context dimensions:

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

Progressive disclosure: CSV import auto-creates Concepts with a single preferred Term per locale -- no extra complexity required.

## TermBase Interface

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

Backends: In-memory (CLI batch) and SQLite (persistent). Both use the shared `bowrain/storage` layer from [AD-003](/docs/ad/003-content-store).

## Pipeline Tools

Six pipeline tools integrate terminology, entity annotation, and privacy into the streaming pipeline ([AD-006](/docs/ad/006-tool-system)):

**`term-lookup`** (Enrich) -- Scans source text for known terms, attaches `TermAnnotation` with `TextRange` character positions. Downstream tools (AI translate, QA) use these annotations for context.

**`term-enforce`** (Validate) -- Checks preferred term usage in target text. Reports forbidden terms, non-preferred variants, deprecated terms, and missing target counterparts.

**`term-extract`** (Enrich, AI) -- LLM extraction of candidate terms with `status: proposed`. Uses AI provider from [AD-008](/docs/ad/008-ai-integration).

**`entity-annotate`** (Enrich, AI) -- Named entity annotation (people, organizations, products, dates, locations). Serves multiple purposes: TM generalization in Sievepen ([AD-009](/docs/ad/009-translation-memory)), do-not-translate markers, localization hints, and terminology candidate discovery. Should run early in the pipeline -- before `tm-leverage`.

**`redact`** (Transform) -- Privacy tool replacing entity values with typed placeholders (e.g., "John" -> `\{PERSON\}`) before external services. Orthogonal to TM generalization, which handles matching natively via derived keys ([AD-009](/docs/ad/009-translation-memory)).

**`unredact`** (Transform) -- Restores original entity values after external processing. Paired with `redact`:
`reader -> entity-annotate -> redact -> [external MT] -> unredact -> writer`

## Concept Relations (Phase 2)

broader/narrower, related, supersedes, see-also. Enables concept graph navigation in Bowrain ([AD-012](/docs/ad/012-bowrain)).

```go
type Stream struct {
    ID          string
    Name        string       // "Q2 Rebrand"
    Changes     []StreamChange
    Status      StreamStatus // draft, reviewing, promoted, discarded
}
```

## Terminology Streams (Phase 2)

Named what-if experiments for terminology changes. Streams isolate changes from the active termbase until explicitly promoted. Side-by-side preview in Bowrain: current terms vs. stream terms applied to content. Promotion applies changes atomically.

## Content Model Extensions

Two annotation types implement the `Annotation` interface with character-level `TextRange` positions for precise inline highlighting in Bowrain ([AD-012](/docs/ad/012-bowrain)):

- `TermAnnotation` -- matched term with concept, target terms, and position
- `EntityAnnotation` -- named entity with type, DNT flag, and position

These join `AltTranslation` as first-class annotations on Blocks ([AD-002](/docs/ad/002-content-model)).
