---
sidebar_position: 5
title: "Terminology Data Model"
description: Implementation note for AD-010 — the Go struct layout for Concept, Term, and their context dimensions, plus the SQL schema for the SQLite termbase and the import pipeline from TBX and CSV sources.
keywords: [terminology data model, Concept, Term, SQLite, TBX import, CSV, implementation note, neokapi]
---

# Terminology Data Model

This note provides implementation details for [AD-010](/contribute/architecture/010-terminology).

## Data Model: Concept-Oriented

The core data model is concept-oriented, following TBX principles. A Concept groups terms across languages, each with context dimensions:

```go
type Term struct {
    Text           string           // the term text
    Locale         model.LocaleID   // language/locale
    Status         model.TermStatus // lifecycle status (proposed, approved, preferred,
                                    // admitted, deprecated, forbidden)
    PartOfSpeech   string           // noun, verb, adjective, etc.
    Gender         string           // grammatical gender (if applicable)
    Note           string           // usage note or context
    CompetitorTerm bool             // true if this is a competitor brand term
}

type Concept struct {
    ID         string            // unique concept identifier
    ProjectID  string            // project scope (empty = workspace-scoped)
    Domain     string            // subject field (software, medical, legal, etc.)
    Definition string            // language-neutral definition
    Source     TermSource        // "terminology" or "brand_vocabulary"
    Terms      []Term            // terms across locales
    Properties map[string]string // extensible metadata
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

`TermSource` distinguishes traditional terminology
(`TermSourceTerminology`) from brand vocabulary (`TermSourceBrandVocabulary`),
so the two populations can share one termbase while staying filterable.

Progressive disclosure: CSV import auto-creates Concepts with a single preferred Term per locale -- no extra complexity required.

## TermBase Interface

```go
type TermBase interface {
    AddConcept(concept Concept) error
    GetConcept(id string) (Concept, bool)
    DeleteConcept(id string) error
    Lookup(sourceText string, opts LookupOptions) []TermMatch
    LookupAll(sourceText string, opts LookupOptions) []TermMatch
    Search(query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]Concept, int)
    Count() int
    Concepts() []Concept
    Close() error
}
```

Import and export are standalone functions rather than interface methods:
`ImportJSON`/`ExportJSON`, `ImportCSV`/`ExportCSV`, and `ImportTBX`/`ExportTBX`
(the ISO TBX interchange format, with `TBXImportOptions`/`TBXExportOptions`).
Framework backends: in-memory (CLI batch) and SQLite (persistent). The
`TermBase` interface supports server-side backends for multi-user deployments.

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

Two pipeline tools integrate terminology into the streaming pipeline ([AD-006](/contribute/architecture/006-tool-system)):

**`term-lookup`** (Enrich) -- Scans source text for known terms, attaches `TermAnnotation` with `TextRange` character positions. Downstream tools (AI translate, QA) use these annotations for context.

**`term-enforce`** (Validate) -- Checks preferred term usage in target text. Reports forbidden terms, non-preferred variants, deprecated terms, and missing target counterparts.

Related AI and redaction tools (registered in `core/ai/tools/` and
`core/tools/`):

**`ai-terminology`** (Enrich, AI) -- LLM extraction of candidate terms. Uses an AI provider from [AD-011](/contribute/architecture/011-ai-providers).

**`ai-entity-extract`** (Enrich, AI) -- Named entity annotation (people, organizations, products, dates, locations). Serves multiple purposes: TM generalization in Sievepen ([AD-009](/contribute/architecture/009-translation-memory)), do-not-translate markers, localization hints, and terminology candidate discovery. Should run early in the pipeline -- before `tm-leverage`.

**`redact`** (Transform) -- Privacy tool replacing entity values with typed placeholders (e.g., "John" -> `\{PERSON\}`) before external services. See [AD-020](/contribute/architecture/020-redaction).

**`unredact`** (Transform) -- Restores original entity values after external processing. Paired with `redact`:
`reader -> ai-entity-extract -> redact -> [external MT] -> unredact -> writer`

## Concept relations

Concepts can be linked for graph import and export. A `ConceptRelation` records
a typed, directed edge between two concepts:

```go
type ConceptRelation struct {
    SourceID     string // origin concept ID
    TargetID     string // target concept ID
    RelationType string // uses graph.Label* constants
}
```

`RelationType` draws its values from the `graph.Label*` constants, so relation
edges share the vocabulary used by the rest of the graph layer.

## Content model extensions

- `TermAnnotation` -- matched term with concept, target terms, and position
- `EntityAnnotation` -- named entity with type, DNT flag, and position

These join `AltTranslation` as first-class annotations on Blocks ([AD-002](/contribute/architecture/002-content-model)).
