---
sidebar_position: 4
title: "TM Matching Algorithm"
description: Implementation note for AD-009 — the three derived matching keys (plain, structural, generalized), how they are indexed in SQLite, and the fuzzy match scoring and adaptation pipeline in Sievepen.
keywords: [TM matching, fuzzy match, Sievepen, plain key, structural key, generalized key, implementation note, neokapi]
---

# TM Matching Algorithm

This note provides implementation details for [AD-009](/contribute/architecture/009-translation-memory).

## Derived matching keys

Each TM entry stores its variants as `[]model.Run` sequences (AD-002). Several
matching representations are derived from those runs at storage time and indexed
for fast lookup. Sievepen computes them with the framework's projection helpers
in `core/model`:

- **plain**: `model.FlattenRuns(runs)` (normalized via `NormalizeText`) -- keeps
  `Text` runs verbatim, renders `Ph` placeholders as `\{equiv\}` and `Sub` runs
  as `[equiv]`, emits paired-code (`PcOpen`/`PcClose`) inner content but not the
  wrappers, and takes the 'other' branch of plural/select constructs (or the
  first form if 'other' is absent). Enables matching against legacy TMs and
  unanalyzed content.
- **structural**: `model.RunsStructuralText(runs)` -- renders inline-code runs as
  positional placeholders: `PcOpen` as `\{1\}`, `PcClose` as `\{/1\}`, and `Ph`
  as `\{1/\}`. Enables matching with inline-code position awareness.
- **generalized**: `model.RunsGeneralizedText(runs)` -- renders entity `Ph` runs
  (whose `Type` is an entity type) as typed placeholders (`\{PERSON\}`,
  `\{PRODUCT\}`) and other inline-code runs as in the structural key. Maximum
  reuse -- entities are interchangeable.

The generalized key is the most powerful: "John works at Acme" and "Alice works at Globex" both generalize to `\{PERSON\} works at \{ORGANIZATION\}` -- an exact match.

## Tiered Matching Pipeline

Lookup tries matching strategies in order of reuse potential:

1. generalized exact -- score 1.0 (entities differ, structure identical)
2. structural exact -- score 1.0 (inline codes match exactly)
3. plain exact -- score 1.0 only when the structural key also matches;
   text-only equality across differing inline-code structure caps at
   `ScoreNearExact` (0.99, tag-mismatch penalty)
4. generalized fuzzy -- Levenshtein on generalized keys
5. structural fuzzy -- Levenshtein on structural keys
6. plain fuzzy -- Levenshtein on plain keys

After the exact tiers, the ambiguity rule runs: multiple full-score
matches with *differing* target texts all demote to `ScoreNearExact` and
flag `TMMatch.Ambiguous` -- exact-only consumers (extract pre-fill,
`fillTargetThreshold: 100`) skip them instead of picking by storage order.
Results order deterministically by (score desc, match-type priority,
entry ID).

The first match at or above the score threshold wins. A generalized exact match (different entity values, identical structure) is preferred over a plain fuzzy match (similar text, unknown structure). Levenshtein edit distance with a configurable threshold (default 70%) provides fuzzy matching.

## Entity Adaptation

When a generalized match is found, the match result carries adaptation information to substitute entity values from the current source into the stored target:

```go
type TMMatch struct {
    Entry             TMEntry
    Score             float64 // 0.0-1.0 (1.0 = exact match, text AND structure)
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
- **`tm_variant_search`**: word tokenizer via `storage.FTSWordTokenizer` on `text` -- resolves to `icu` under cgo builds and `unicode61` under the default pure-Go (modernc, no-cgo) and wasm builds; the ICU tokenizer is a cgo-only FTS5 extension. Used for ranked UI search (FTS5 BM25).

`BuildTrigramQuery()` constructs the FTS5 MATCH expression:

- **Multi-word text** (Latin, etc.): OR of individual words ≥3 chars as quoted substrings.
- **Single word / CJK**: Overlapping 4-character windows sampled at even intervals (max 6 windows).

Falls back to length-based pre-filtering (`LENGTH(plain) BETWEEN min AND max`) if FTS5 trigram is unavailable at runtime.

### PostgreSQL: pg_trgm + tsvector

- **pg_trgm extension**: GIN trigram indexes on `plain`, `struct_key`, `general_key`. Uses the `%` similarity operator for candidate retrieval with a low threshold to maximize recall. Final fuzzy scoring and threshold filtering happen in Go via `LevenshteinRatio` on the retrieved candidates -- not in SQL -- identical to the SQLite path.
- **tsvector column**: `search_tsv` is a `GENERATED ALWAYS AS (to_tsvector('simple', plain)) STORED` column (auto-maintained by Postgres, not by a trigger), indexed with a GIN index. UI text search matches via `search_tsv @@ plainto_tsquery('simple', ...)` with results ordered by recency (`updated_at DESC`, plus a stream-priority `CASE` when a stream chain is supplied), not via `ts_rank()`.

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

## TMX element mapping

The import/export layer maps between inline-code runs and TMX inline elements:

| Run       | TMX Element |
| --------- | ----------- |
| `Ph`      | `<ph>`      |
| `PcOpen`  | `<bpt>`     |
| `PcClose` | `<ept>`     |

Entity metadata is carried as `<prop>` elements on the TMX `<tu>`. When importing legacy TMX files that contain only plain text (no inline codes), entries are stored as `Text`-only run sequences with no entity mappings. They participate in plain matching only.
