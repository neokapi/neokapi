---
id: 010-terminology
sidebar_position: 10
title: "AD-010: Terminology"
description: "Architecture decision: neokapi's terminology system is concept-oriented, grouping multi-locale terms under language-neutral Concepts with per-term lifecycle status, part-of-speech, and domain metadata."
keywords: [terminology, Concept, TBX, termbase, concept-oriented, architecture decision, neokapi]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# AD-010: Terminology

## Summary

neokapi's terminology system is concept-oriented: a `Concept` groups terms
across locales with per-term metadata (status, part of speech, grammatical
gender). The `TermBase` interface (`termbase/` package) supports in-memory
and SQLite backends, a tiered lookup pipeline, and TBX import/export.
Terminology flows through the streaming pipeline via first-class annotation
types with character-level positions for precise inline highlighting.

## Context

Terminology management in localization ranges from simple glossaries
(CSV with source/target pairs) to concept-oriented termbases (TBX,
MultiTerm). A flat glossary does not express that "bug", "defect", and
"issue" are terms for the same concept in different contexts, nor that
"bug" can be preferred in engineering docs and deprecated in customer-
facing content.

The framework needs:

- Progressive complexity — start from a CSV glossary, grow into concept
  management without rewriting data.
- Pipeline integration — terminology as streaming tools, not a separate
  service.
- Precise positions — character-level ranges on matched terms so downstream
  UIs can highlight within a Fragment.
- Annotation semantics — do-not-translate markers for entity names, locale
  formatting hints, and pending AI-proposed candidates distinct from
  curated entries.

TBX (ISO 30042:2019) is the universal interchange format for concept-
oriented terminological data. Native storage uses SQLite for speed and
query flexibility; TBX handles import and export only.

## Decision

### Concept-oriented data model

A `Concept` groups terms across locales, each with context:

```go
type Term struct {
    Text         string
    Locale       model.LocaleID
    Status       model.TermStatus // proposed, approved, preferred,
                                   // admitted, deprecated, forbidden
    PartOfSpeech string
    Gender       string
    Note         string
}

type Concept struct {
    ID         string
    ProjectID  string
    Domain     string
    Definition string
    Terms      []Term
    Properties map[string]string
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

Progressive disclosure: CSV import auto-creates Concepts with a single
preferred Term per locale. No extra complexity is imposed on users who want
a flat glossary.

### TermBase interface

```go
type TermBase interface {
    AddConcept(concept Concept) error
    GetConcept(id string) (Concept, bool)
    DeleteConcept(id string) error
    Lookup(sourceText string, opts LookupOptions) []TermMatch
    LookupAll(sourceText string, opts LookupOptions) []TermMatch
    Search(query string, sourceLocale, targetLocale string,
        offset, limit int) ([]Concept, int)
    Count() int
    Concepts() []Concept
    Close() error
}
```

Import and export are standalone functions rather than interface methods:
`ImportTBX`, `ExportTBX`, `ImportCSV`, `ExportCSV`, `ImportJSON`,
`ExportJSON`.

### Backends

- **In-memory** (`termbase/memory.go`) — fast, ephemeral; session-scoped
  batch processing.
- **SQLite** (`termbase/sqlite.go`) — persistent file-based storage for CLI
  tools. Pure Go via `modernc.org/sqlite`.

A PostgreSQL backend with workspace isolation and terminology streams can be
supplied by a platform layer behind the same `TermBase` interface.

### Tiered lookup

Term lookup follows a cascading pipeline:

1. **Exact** — case-sensitive match on normalized term text.
2. **Normalized** — Unicode NFC + case folding + whitespace collapse.
3. **Fuzzy** — trigram candidate retrieval + Levenshtein scoring on the
   ~200 closest candidates.
4. **AI-assisted** (opt-in) — LLM proposes candidate term mappings that
   produce `TermCandidateAnnotation` entries for human review.

The fuzzy tier uses the same SQLite FTS5 trigram tokenizer as Sievepen
([AD-009: Translation Memory](009-translation-memory.md)), keeping lookup
cost sub-linear in termbase size. Text is normalized with Unicode NFC via
`NormalizeTerm()` before comparison. Character-level Levenshtein (on
`[]rune`) is correct for all scripts including CJK.

Which tiers run is selected per call through `LookupOptions.MatchModes`
(`[]model.MatchStrategy`) on `TermBase.Lookup`/`LookupAll`, alongside
`CaseSensitive`, `MinScore`, and scope filters — so a caller can request, for
example, exact-only or exact-plus-fuzzy without changing the pipeline.

### UI search

Distinct from lookup, the `Search` method powers the termbase browser in
the CLI and desktop UI. It uses an FTS5 tokenizer to support substring
search ranked by match quality, rather than unranked `LIKE '%...%'`
queries.

### Annotations

Two annotation types implement the `Annotation` interface with
character-level `TextRange` positions for precise inline highlighting:

- **`TermAnnotation`** — a matched term from the termbase, carrying
  concept ID, target term options, status, and position.
- **`TermCandidateAnnotation`** — AI-proposed term not yet in the
  termbase. Carries a `status: proposed` marker so UI reviewers can
  accept, reject, or defer.

An **`EntityAnnotation`** type carries named entities (people,
organizations, products, dates, locations) with character positions and
optional DNT (do-not-translate) flags. Entity annotations serve multiple
purposes:

- Input to Sievepen TM generalization ([AD-009: Translation Memory](009-translation-memory.md)).
- Do-not-translate markers consumed by AI translation.
- Locale formatting hints (dates, numbers) for downstream tools.
- Terminology candidate discovery.

These annotations join `AltTranslation` as first-class annotations on
Blocks.

### Concept relations

Concepts carry relations to other concepts:

- **broader** / **narrower** — taxonomic relationships.
- **related** — associative relationships.
- **supersedes** — this concept replaces another.
- **see-also** — cross-reference.

Relations enable graph navigation in UIs and support terminology
deprecation workflows where a superseded concept's terms are
automatically flagged in new content.

### Competitor terms

Terms carry a `CompetitorTerm` boolean flag marking competitor brand
terms. Competitor terms flow through `term-enforce` as critical-severity
violations, supporting brand voice governance without requiring the full
brand module.

### Pipeline tools

The framework ships built-in terminology tools as ordinary pipeline stages:

- **`term-lookup`** (enrich) — scans source text for known terms, attaches
  `TermAnnotation` with `TextRange` positions. Downstream tools (AI
  translate, QA) use these annotations for context.
- **`term-enforce`** (validate) — checks preferred term usage in target
  text. Reports forbidden terms, non-preferred variants, deprecated
  terms, and missing target counterparts.
- **`ai-terminology`** (AI-assisted enrich) — LLM extraction of candidate
  terms with `status: proposed`. Uses a provider from
  [AD-011: AI Providers](011-ai-providers.md).
- **`ai-entity-extract`** (AI-assisted enrich) — LLM-based named entity
  annotation (with optional NER). Should run early in the pipeline, before
  `tm-leverage`.
- **`redact`** and **`unredact`** (transform) — pair that replaces entity
  values with typed placeholders before external services and restores
  them afterwards.

A full pipeline looks like:

<PipelineDiagram
  stages={[
    { label: "Reader", role: "io" },
    { label: "ai-entity-extract", role: "annotate" },
    { label: "term-lookup", role: "annotate" },
    { label: "tm-leverage", role: "translate" },
    { label: "ai-translate", role: "translate" },
    { label: "term-enforce", role: "qa" },
    { label: "Writer", role: "io" },
  ]}
/>

### TBX import and export

TBX (ISO 30042:2019) is the interchange format. Import maps TBX entries
to Concepts and populates per-locale Terms. Export preserves concept
relations, term status, and context fields.

## Consequences

- Terminology is a first-class pipeline citizen, not a bolt-on
  post-processing step.
- Character-level annotation positions enable precise inline UI
  highlighting without re-detecting term boundaries at render time.
- Entity annotations drive both terminology extraction and TM
  generalization — a single annotation pass serves multiple consumers.
- Concept relations give UIs a graph substrate for browsing terminology
  without requiring a separate graph database in the framework.
- `CompetitorTerm` gives the framework a minimal hook for brand guardrails
  without depending on the full brand module.
- The same storage backends as TM (in-memory, SQLite) keep the CLI
  dependency footprint small and cross-compilation simple.

## Related

- [AD-002: Content Model](002-content-model.md) — annotations on Blocks
- [AD-006: Tool System](006-tool-system.md) — pipeline tool pattern
- [AD-009: Translation Memory](009-translation-memory.md) — shared
  matching infrastructure, entity annotation input
- [AD-011: AI Providers](011-ai-providers.md) — LLM-based term extraction
  and entity annotation
- [Terminology Data Model](/contribute/notes-internal/terminology-data-model) — full Go
  struct definitions, pipeline tool catalog, relations
