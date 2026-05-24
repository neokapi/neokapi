---
id: 009-translation-memory
sidebar_position: 9
title: "AD-009: Translation Memory (Sievepen)"
description: "Architecture decision: Sievepen is neokapi's TM library — it stores full Fragments with inline Spans and matches in three tiers (plain, structural, source-entity) to return the highest-quality match first."
keywords: [Sievepen, translation memory, Fragment, matching tiers, SQLite, architecture decision, neokapi]
---

# AD-009: Translation Memory (Sievepen)

## Summary

Sievepen is neokapi's built-in translation memory library, living in
`sievepen/`. It stores full Fragments with inline Spans and entity metadata
rather than plain strings, and uses a tiered matching pipeline (generalized
exact, structural exact, plain exact, fuzzy) to maximize reuse. The framework
ships in-memory and SQLite backends; a PostgreSQL backend is provided by the

## Context

Translation memory is a core localization primitive: previously translated
segments are reused to maintain consistency and reduce cost. Existing TM
systems store flat source/target string pairs and match on string similarity
alone, which loses information that matters to translators:

- **Inline codes** (bold, links, placeholders) are stripped before matching.
  A match is found but the codes do not transfer — the translator manually
  reinserts them.
- **Named entities** (people, products, dates) are treated as literal text.
  "John works at Acme" and "Alice works at Globex" score low despite being
  structurally identical; the only differences are substitutable entity
  values.
- **Pipeline context** (entity annotations, term matches, QA results)
  produced earlier in the flow is discarded.

A content-aware TM preserves Fragments end-to-end, derives multiple matching
keys from a single entry, and returns matches with entity adaptation
information so translators receive pre-adapted targets.

## Decision

### Content-aware storage

Sievepen stores `model.Fragment` values — the same type used throughout the
pipeline ([AD-002: Content Model](002-content-model.md)) — rather than
strings. Each TM entry preserves inline Spans (markup codes) and entity
mappings.

```go
type TMEntry struct {
    ID           string
    Source       *model.Fragment
    Target       *model.Fragment
    SourceLocale model.LocaleID
    TargetLocale model.LocaleID
    Entities     []EntityMapping
    Annotations  map[string]model.Annotation
    Properties   map[string]string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}
```

An `EntityMapping` records the typed entity that appeared at a position in
the source, alongside its translation in the target.

### Derived matching keys

Each entry is stored with three derived keys, pre-computed at write time and
indexed:

- **plain** — `Fragment.Text()` with all Span markers stripped. Enables
  matching against legacy TMs and unanalyzed content.
- **structural** — Spans rendered as numbered placeholders (`{1}`, `{/1}`).
  Preserves inline code position awareness.
- **generalized** — entity Spans rendered as typed placeholders (`{PERSON}`,
  `{PRODUCT}`). Maximum reuse; entities become interchangeable.

"John works at Acme" and "Alice works at Globex" both generalize to
`{PERSON} works at {ORGANIZATION}` — an exact match at the generalized tier.

### Tiered matching pipeline

Lookup tries strategies in order of reuse potential:

1. generalized exact — score 1.0 (entities differ, structure identical)
2. structural exact — score 1.0 (inline codes match exactly)
3. plain exact — score 1.0 (text-only exact match)
4. generalized fuzzy — Levenshtein on generalized keys
5. structural fuzzy — Levenshtein on structural keys
6. plain fuzzy — Levenshtein on plain keys

The first match at or above the configured score threshold wins. A
generalized exact match (different entity values, identical structure) is
preferred over a plain fuzzy match (similar text, unknown structure).
Levenshtein edit distance with a configurable threshold (default 70%)
controls fuzzy matching.

### Entity adaptation

When a generalized match is found, the result carries adaptation information
that substitutes entity values from the current source into the stored
target:

```go
type TMMatch struct {
    Entry             TMEntry
    Score             float64
    MatchType         MatchType
    ProjectID         string
    EntityAdaptations []EntityAdaptation
}
```

The `tm-leverage` tool applies these adaptations automatically, so
translators receive pre-adapted targets with the correct entity values
already substituted.

### Lookup interface

```go
type TranslationMemory interface {
    Add(entry TMEntry) error
    Lookup(source *model.Block, sourceLocale, targetLocale model.LocaleID,
        opts LookupOptions) ([]TMMatch, error)
    LookupSegment(source *model.Block, segmentIdx int,
        sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error)
    Delete(id string) error
    Count() int
    Close() error
}
```

`Lookup` takes a `*model.Block` rather than a string. The Block carries the
entity annotations needed to compute the generalized key and the Spans
needed for the structural key; no separate pre-processing step is required.
By default `Lookup` keys on the block's _first_ segment, which is correct
when segmentation is off (one segment per Block — the verbatim lookup case).

`LookupSegment` selects a specific segment by index for the
sentence-level TM leverage path used by `kapi extract` when the
project's recipe sets `segmentation.source: true` (see
[AD-017](017-bilingual-format-interop.md)).

### Backends

The framework provides two tiers:

- **In-memory** (`sievepen/memory/`) — fast, ephemeral; session-scoped
  leverage during batch processing.
- **SQLite** (`sievepen/sqlite/`) — persistent file-based storage for CLI
  tools. Same matching algorithm as the in-memory tier, with FTS5 indexes
  for fuzzy candidate retrieval. Uses `modernc.org/sqlite` (pure Go, no CGo)
  for cross-compilation.

A PostgreSQL backend with workspace-scoped isolation, project scoping, and

### Fuzzy candidate retrieval

Fuzzy matching uses trigram-based candidate retrieval to avoid full table
scans. The candidate set (target ~200 entries) is then scored with
character-level Levenshtein in Go.

- **SQLite** — an FTS5 virtual table with `tokenize='trigram'` indexes
  source_plain, source_struct, and source_general. Triggers keep it in sync
  with the base table. Falls back to length-based pre-filtering if FTS5
  trigram is unavailable at runtime.
- **SQLite UI search** — a separate FTS5 `unicode61` table with BM25
  ranking, used by the CLI and desktop UI for ranked full-text search.

`buildTrigramQuery()` constructs the FTS5 MATCH expression differently for
multi-word Latin text (OR of quoted substrings ≥3 characters) and for
single-word or CJK text (overlapping 4-character windows sampled at even
intervals).

### Unicode normalization

All matching keys are passed through `NormalizeText()`, which applies
Unicode NFC (`golang.org/x/text/unicode/norm`) before whitespace
normalization. This handles real edge cases: Arabic tashkeel as separate
characters vs. combined, Hangul jamo vs. composed syllables, and accented
Latin (e + combining acute vs. é).

### TMX import and export

Sievepen imports and exports TMX files for interchange with external
tooling. The element mapping:

| Fragment Span     | TMX Element |
| ----------------- | ----------- |
| `SpanPlaceholder` | `<ph>`      |
| `SpanOpening`     | `<bpt>`     |
| `SpanClosing`     | `<ept>`     |

Entity metadata travels as `<prop>` elements on the TMX `<tu>`. Inline
element mapping is handled by the full TMX format reader in
`core/formats/tmx/`; the TM module's TMX import (`sievepen/tmx_import.go`)
handles plain text and entity properties only. Legacy plain-text TMX
imports produce entries with plain Fragments and no entity mappings;
they participate in plain matching only.

### Pipeline integration

The `tm-leverage` tool consumes `*model.Block` parts, queries the TM, and
attaches `TMSuggestion` annotations with match score and entity adaptations.
Downstream tools — `ai-translate`, UI review, QA — read these annotations
as context.

A typical flow:

```
Reader → entity-annotate → tm-leverage → ai-translate → qa-check → Writer
```

After translation (human or AI), Blocks are written to TM with their full
Fragment representation and entity mappings. The save step extracts entity
annotations and stores them as `EntityMapping` entries, so the TM
accumulates richer data over time.

## Consequences

- TM stores rich content (Fragments with Spans and entity metadata), not
  flat strings.
- Generalized matching turns entity variation from a fuzzy penalty into an
  exact match at the top tier.
- Entity adaptation provides pre-adapted targets with the correct entity
  values, reducing manual editing.
- Inline codes survive TM storage and matching, reducing manual tag
  reinsertion.
- The SQLite backend uses pure-Go `modernc.org/sqlite`, preserving cross-
  compilation and the single-binary distribution goal.
- Matching on Blocks (not strings) makes TM a streaming pipeline stage that
  composes naturally with other tools.
- Trigram candidate retrieval keeps fuzzy lookup fast even for 100K-entry
  TMs.

## Related

- [AD-002: Content Model](002-content-model.md) — Fragment, Span, entity
  annotations
- [AD-006: Tool System](006-tool-system.md) — `tm-leverage` tool
- [AD-010: Terminology](010-terminology.md) — shares matching infrastructure
- [TM Matching Algorithm](/contribute/notes-internal/tm-matching-algorithm) — trigram
  construction, performance table, TMX element mapping
