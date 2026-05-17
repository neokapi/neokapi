---
sidebar_position: 5
title: "Terminology Data Model"
---

# Terminology Data Model

This note provides implementation details for [AD-010](/architecture/010-terminology).

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

Import and export are standalone functions rather than interface methods: `ImportJSON`, `ExportJSON`, `ImportCSV`, `ExportCSV`. Framework backends: In-memory (CLI batch) and SQLite (persistent). The `TermBase` interface supports server-side backends for multi-user deployments.

## Fuzzy Matching and Search

Term lookup uses a tiered matching pipeline: exact -> normalized -> fuzzy. Fuzzy matching uses trigram-based candidate retrieval to avoid full table scans:

- **SQLite**: FTS5 `trigram` tokenizer on `text_lower` column, synced via triggers. Falls back to length-based pre-filtering if FTS5 is unavailable.
- **PostgreSQL**: pg_trgm GIN index on `text_lower` column, using the `%` similarity operator. Falls back to length-based pre-filtering.

Character-level Levenshtein scoring (on `[]rune`) is applied to ~200 trigram candidates. This is correct for all scripts including CJK (each character is a morpheme).

UI search uses ranked full-text search:

- **SQLite**: FTS5 `trigram` tokenizer for substring matching on term text.
- **PostgreSQL**: pg_trgm `similarity()` ranking on `text_lower`.

Text normalization applies Unicode NFC (`golang.org/x/text/unicode/norm`) via `NormalizeTerm()` before comparison, handling Arabic diacritics, Hangul jamo composition, and accented Latin characters.

## Pipeline Tools

Two pipeline tools integrate terminology into the streaming pipeline ([AD-006](/architecture/006-tool-system)):

**`term-lookup`** (Enrich) -- Scans source text for known terms, attaches `TermAnnotation` with `TextRange` character positions. Downstream tools (AI translate, QA) use these annotations for context.

**`term-enforce`** (Validate) -- Checks preferred term usage in target text. Reports forbidden terms, non-preferred variants, deprecated terms, and missing target counterparts.

Additional tools planned but not yet implemented:

**`term-extract`** (Enrich, AI) -- LLM extraction of candidate terms with `status: proposed`. Uses AI provider from [AD-011](/architecture/011-ai-providers).

**`entity-annotate`** (Enrich, AI) -- Named entity annotation (people, organizations, products, dates, locations). Serves multiple purposes: TM generalization in Sievepen ([AD-009](/architecture/009-translation-memory)), do-not-translate markers, localization hints, and terminology candidate discovery. Should run early in the pipeline -- before `tm-leverage`.

**`redact`** (Transform) -- Privacy tool replacing entity values with typed placeholders (e.g., "John" -> `\{PERSON\}`) before external services. Orthogonal to TM generalization, which handles matching natively via derived keys ([AD-009](/architecture/009-translation-memory)).

**`unredact`** (Transform) -- Restores original entity values after external processing. Paired with `redact`:
`reader -> entity-annotate -> redact -> [external MT] -> unredact -> writer`

## Concept Relations (Phase 2)

```go
type Stream struct {
    ID          string
    Name        string       // "Q2 Rebrand"
    Changes     []StreamChange
    Status      StreamStatus // draft, reviewing, promoted, discarded
}
```

## Terminology Streams (Phase 2)

Named what-if experiments for terminology changes. Streams isolate changes from the active termbase until explicitly promoted.stream terms applied to content. Promotion applies changes atomically.

## Content Model Extensions

- `TermAnnotation` -- matched term with concept, target terms, and position
- `EntityAnnotation` -- named entity with type, DNT flag, and position

These join `AltTranslation` as first-class annotations on Blocks ([AD-002](/architecture/002-content-model)).
