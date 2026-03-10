---
sidebar_position: 5
title: "Terminology Data Model"
---
# Terminology Data Model

This note provides implementation details for [AD-010](/docs/ad/010-terminology).

## Data Model: Concept-Oriented

The core data model is concept-oriented, following TBX principles. A Concept groups terms across languages, each with context dimensions:

```go
type Term struct {
    Text         string           // the term text
    Locale       model.LocaleID   // language/locale
    Status       model.TermStatus // lifecycle status (proposed, approved, preferred,
                                  // admitted, deprecated, forbidden)
    PartOfSpeech string           // noun, verb, adjective, etc.
    Gender       string           // grammatical gender (if applicable)
    Note         string           // usage note or context
}

type Concept struct {
    ID         string            // unique concept identifier
    ProjectID  string            // project scope (empty = workspace-scoped)
    Domain     string            // subject field (software, medical, legal, etc.)
    Definition string            // language-neutral definition
    Terms      []Term            // terms across locales
    Properties map[string]string // extensible metadata
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

Progressive disclosure: CSV import auto-creates Concepts with a single preferred Term per locale -- no extra complexity required.

## TermBase Interface

```go
type TermBase interface {
    AddConcept(concept Concept) error
    GetConcept(id string) (Concept, bool)
    DeleteConcept(id string) error
    Lookup(sourceText string, opts LookupOptions) []TermMatch
    LookupAll(sourceText string, opts LookupOptions) []TermMatch
    Search(query string, sourceLocale, targetLocale string, offset, limit int) ([]Concept, int)
    Count() int
    Concepts() []Concept
    Close() error
}
```

Import and export are standalone functions rather than interface methods: `ImportJSON`, `ExportJSON`, `ImportCSV`, `ExportCSV`. Backends: In-memory (CLI batch) and SQLite (persistent). Both use the shared `bowrain/storage` layer from [AD-003](/docs/ad/003-content-store).

## Pipeline Tools

Two pipeline tools integrate terminology into the streaming pipeline ([AD-006](/docs/ad/006-tool-system)):

**`term-lookup`** (Enrich) -- Scans source text for known terms, attaches `TermAnnotation` with `TextRange` character positions. Downstream tools (AI translate, QA) use these annotations for context.

**`term-enforce`** (Validate) -- Checks preferred term usage in target text. Reports forbidden terms, non-preferred variants, deprecated terms, and missing target counterparts.

Additional tools planned but not yet implemented:

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
