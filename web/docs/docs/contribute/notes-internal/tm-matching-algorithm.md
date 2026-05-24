---
sidebar_position: 4
title: "TM Matching Algorithm"
description: Implementation note for AD-009 — the three derived matching keys (plain, structural, source-entity), how they are indexed in SQLite, and the fuzzy match scoring and adaptation pipeline in Sievepen.
keywords: [TM matching, fuzzy match, Sievepen, plain key, structural key, source-entity, implementation note, neokapi]
---

# TM Matching Algorithm

This note provides implementation details for [AD-009](/contribute/architecture/009-translation-memory).

## Derived Matching Keys

Each TM entry has multiple matching representations derived from its stored Fragment. These are computed at storage time and indexed for fast lookup:

- **plain**: `Fragment.Text()` -- strips all Span markers. Enables matching against legacy TMs and unanalyzed content.
- **structural**: Spans rendered as numbered placeholders (`\{1\}`, `\{/1\}`). Enables matching with inline code position awareness.
- **generalized**: Entity Spans as typed placeholders (`\{PERSON\}`, `\{PRODUCT\}`). Maximum reuse -- entities are interchangeable.

The generalized key is the most powerful: "John works at Acme" and "Alice works at Globex" both generalize to `\{PERSON\} works at \{ORGANIZATION\}` -- an exact match.

## Tiered Matching Pipeline

Lookup tries matching strategies in order of reuse potential:

1. generalized exact -- score 1.0 (entities differ, structure identical)
2. structural exact -- score 1.0 (inline codes match exactly)
3. plain exact -- score 1.0 (text-only exact match)
4. generalized fuzzy -- Levenshtein on generalized keys
5. structural fuzzy -- Levenshtein on structural keys
6. plain fuzzy -- Levenshtein on plain keys

The first match at or above the score threshold wins. A generalized exact match (different entity values, identical structure) is preferred over a plain fuzzy match (similar text, unknown structure). Levenshtein edit distance with a configurable threshold (default 70%) provides fuzzy matching.

## Entity Adaptation

When a generalized match is found, the match result carries adaptation information to substitute entity values from the current source into the stored target:

```go
type TMMatch struct {
    Entry             TMEntry
    Score             float64 // 0.0-1.0 (1.0 = exact match)
    MatchType         MatchType
    ProjectID         string // provenance: project ID of the matched entry
    EntityAdaptations []EntityAdaptation
}
```

The `tm-leverage` tool ([AD-006](/contribute/architecture/006-tool-system)) applies adaptations automatically -- translators receive pre-adapted targets with correct entity values.

## Fuzzy Candidate Retrieval

Tiers 4-6 (fuzzy matching) previously scanned all entries for a locale pair and computed Levenshtein distance for each -- O(n) full table scan. This was replaced with trigram-based candidate retrieval that reduces 100K entries to ~200 candidates before Levenshtein scoring.

### Unicode NFC Normalization

`NormalizeText()` applies Unicode NFC normalization (`golang.org/x/text/unicode/norm`) before whitespace normalization. This fixes real edge cases: Arabic diacritics (tashkeel) as separate characters vs. combined, Hangul jamo vs. composed syllables, and accented Latin (e + combining acute vs. e).

### SQLite: FTS5 Trigram Tokenizer

Two FTS5 virtual tables backed by the `tm_variants` table, kept in sync on write:

- **`tm_variant_trigram`**: `tokenize='trigram'` on `plain`, `struct_key`, `general_key`. Used for fuzzy candidate retrieval.
- **`tm_variant_search`**: `tokenize='icu'` on `text`. Used for ranked UI search (FTS5 BM25).

`BuildTrigramQuery()` constructs the FTS5 MATCH expression:

- **Multi-word text** (Latin, etc.): OR of individual words ≥3 chars as quoted substrings.
- **Single word / CJK**: Overlapping 4-character windows sampled at even intervals (max 6 windows).

Falls back to length-based pre-filtering (`LENGTH(source_plain) BETWEEN min AND max`) if FTS5 trigram is unavailable at runtime.

### PostgreSQL: pg_trgm + fuzzystrmatch

- **pg_trgm extension**: GIN trigram indexes on source_plain, source_struct, source_general. Uses the `%` similarity operator for candidate retrieval with a low threshold to maximize recall.
- **fuzzystrmatch extension**: Provides `levenshtein_less_equal()` for optional threshold-based filtering in SQL.
- **tsvector column**: `search_tsv` with `to_tsvector('simple', ...)` populated via BEFORE INSERT/UPDATE trigger. `ts_rank()` provides BM25-like ranking for UI search.

Falls back to length-based pre-filtering if pg_trgm is unavailable.

### Performance

| Dataset      | Before (full scan) | After (trigram + Levenshtein) |
| ------------ | ------------------ | ----------------------------- |
| 1K entries   | ~5ms               | ~2ms                          |
| 10K entries  | ~50ms              | ~5ms                          |
| 100K entries | ~500ms+            | ~10-15ms                      |

## Storage Backends

1. **In-memory**: fast, ephemeral; for session-scoped leverage during batch processing.
2. **SQLite** (via `modernc.org/sqlite`): persistent; matching keys are pre-computed and indexed. FTS5 trigram indexes for fuzzy candidate retrieval; FTS5 unicode61 for ranked UI search. Pure Go with no CGo dependencies.
3. **PostgreSQL**: persistent; same matching logic with pg_trgm GIN indexes for fuzzy candidate retrieval and tsvector/tsquery for ranked UI search. Workspace-scoped isolation via `workspace_id` column.

Generalized and structural exact matching is an indexed lookup -- fast even for large TMs. Fuzzy matching uses trigram candidate retrieval to narrow the search space, then Levenshtein scoring on ~200 candidates.

## TMX Element Mapping

The import/export layer maps between Fragment Spans and TMX inline elements:

| Fragment Span     | TMX Element |
| ----------------- | ----------- |
| `SpanPlaceholder` | `<ph>`      |
| `SpanOpening`     | `<bpt>`     |
| `SpanClosing`     | `<ept>`     |

Entity metadata is carried as `<prop>` elements on the TMX `<tu>`. Note that inline element mapping (`<ph>`, `<bpt>`, `<ept>`) is handled by the full TMX format reader (`core/formats/tmx/`), not by the sievepen TM import/export layer. The TM module's TMX import (`sievepen/tmx_import.go`) handles plain text and entity properties only. When importing legacy TMX files that contain only plain text (no inline codes), entries are stored with plain Fragments and no entity mappings. They participate in plain matching only.
