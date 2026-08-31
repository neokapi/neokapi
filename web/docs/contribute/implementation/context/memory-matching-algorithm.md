---
sidebar_position: 3
title: "Content Memory Matching Algorithm"
description: Implementation note for C-09. The three derived matching keys (plain, structural, generalized), how they are indexed in SQLite, the fuzzy match scoring and adaptation pipeline, and the version chain in the memory package.
keywords: [content memory matching, fuzzy match, memory, plain key, structural key, generalized key, version chain, implementation note, neokapi]
---

# Content Memory Matching Algorithm

This note provides implementation details for [C-09](/contribute/architecture/context/c-09-content-memory).

## Derived matching keys

Each content-memory entry stores its variants as `[]model.Run` sequences
(F-02). Several matching representations are derived from those runs at storage
time and indexed for fast lookup. The `memory` package computes them with the
framework's projection helpers in `core/model`:

- **plain**: `model.FlattenRuns(runs)` (normalized via `NormalizeText`). Keeps
  `Text` runs verbatim, renders `Ph` placeholders as `\{equiv\}` and `Sub` runs
  as `[equiv]`, emits paired-code (`PcOpen`/`PcClose`) inner content but not the
  wrappers, and takes the 'other' branch of plural/select constructs (or the
  first form if 'other' is absent). Enables matching against plain-text memories
  imported from other tools, and against unanalyzed content.
- **structural**: `model.RunsStructuralText(runs)`. Renders inline-code runs as
  positional placeholders: `PcOpen` as `\{1\}`, `PcClose` as `\{/1\}`, and `Ph`
  as `\{1/\}`. Enables matching with inline-code position awareness.
- **generalized**: `model.RunsGeneralizedText(runs)`. Renders entity `Ph` runs
  (whose `Type` is an entity type) as typed placeholders (`\{PERSON\}`,
  `\{PRODUCT\}`) and other inline-code runs as in the structural key. Maximum
  reuse: entities are interchangeable.

The generalized key reuses the most: "John works at Acme" and "Alice works at Globex" both generalize to `\{PERSON\} works at \{ORGANIZATION\}`, an exact match.

## Tiered Matching Pipeline

Lookup tries matching strategies in order of reuse potential:

1. generalized exact: score 1.0 (entities differ, structure identical)
2. structural exact: score 1.0 (inline codes match exactly)
3. plain exact: score 1.0 only when the structural key also matches;
   text-only equality across differing inline-code structure caps at
   `ScoreNearExact` (0.99, tag-mismatch penalty)
4. generalized fuzzy: Levenshtein on generalized keys
5. structural fuzzy: Levenshtein on structural keys
6. plain fuzzy: Levenshtein on plain keys

After the exact tiers, multiple full-score matches with *differing* target
texts are settled by where each answer was approved. `LookupOptions.Point`
names the point the caller is asking from; each entry carries the point its
answer was approved at (`Entry.Point`, the `point` column on `tm_entries`,
SQLite migration v4). `memory.PointDistance` compares the two as a prefix walk
down `profile → channel → collection`, and the nearest answer keeps score 1.0
while the rest demote to `ScoreNearExact`. A tie (two approvals at one point,
or two equally far) goes to the smaller target text (`memory.NearerAnswer`).

A caller that names no point settles nothing: the ambiguity rule runs instead
and every full-score match demotes to `ScoreNearExact` with `Match.Ambiguous`
set, so exact-only consumers (extract pre-fill, `fillTargetThreshold: 100`)
skip them rather than picking by storage order. Results order deterministically
by (score desc, match-type priority, entry ID).

The first match at or above the score threshold wins. A generalized exact match (different entity values, identical structure) is preferred over a plain fuzzy match (similar text, unknown structure). Levenshtein edit distance with a configurable threshold provides fuzzy matching; the `recycle` tool looks up at `fuzzyThreshold` (default 70) and fills at `fillTargetThreshold` (default 95), recording matches between the two as alternative-translation candidates.

## Entity Adaptation

When a generalized match is found, the match result carries adaptation information to substitute entity values from the current source into the stored target:

```go
type Match struct {
    Entry             Entry
    Score             float64 // 0.0-1.0 (1.0 = exact match, text AND structure)
    MatchType         MatchType
    ProjectID         string // provenance: project ID of the matched entry
    Ambiguous         bool   // an exact match demoted because no point was named
    EntityAdaptations []EntityAdaptation
}
```

The `recycle` tool ([E-03](/contribute/architecture/engine/e-03-tool-system)) applies adaptations automatically; translators receive pre-adapted targets with correct entity values.

## Version chains

An entry carries `Unit`, the durable block identity it was approved for (the
`unit` column on `tm_entries`, SQLite migration v5; empty for a seed, an import,
or an entry written before the column existed, and never backfilled). A block
whose source is rewritten writes a new entry beside the old one, and `Unit` is
what relates them.

`memory.VersionReader` is the optional capability of returning a block's prior
answers; both the in-memory and the SQLite backends implement it:

```go
type VersionQuery struct {
    Unit  string // required
    Point string // narrow to one point; empty = every point the block has sat at
    Limit int    // newest first; zero = DefaultVersionLimit (10)
}

type Version struct {
    Entry              Entry
    ContextFingerprint string // from the entry's most recent origin; empty if ungoverned
}

Versions(ctx, q VersionQuery, excludeID string) ([]Version, error)
```

`Version.GovernedBy(fingerprint)` is true only when both fingerprints are
non-empty and equal. `memory/leverage.PriorVersionFor` walks the chain with
`Limit: 1`, applies that gate, and returns the source and target texts of the
prior answer, or nothing.

The producer-facing contract is `core/memory.Provider` (`Lookup(ctx, Request)`
and `PriorVersion(ctx, VersionRequest)`), implemented by
`memory/leverage.Provider` over any `memory.ContentMemory`. A store that does not
implement `VersionReader` answers `PriorVersion` with false. The `.kmb` bundle
format carries `point` and `unit` per entry so a seed round-trips its chain.

## Fuzzy Candidate Retrieval

Tiers 4-6 (fuzzy matching) use trigram-based candidate retrieval, which reduces 100K entries to ~200 candidates before Levenshtein scoring. Scoring every entry in a locale pair instead would be an O(n) full table scan.

### Unicode NFC Normalization

`NormalizeText()` applies Unicode NFC normalization (`golang.org/x/text/unicode/norm`) before whitespace normalization. This fixes real edge cases: Arabic diacritics (tashkeel) as separate characters vs. combined, Hangul jamo vs. composed syllables, and accented Latin (e + combining acute vs. e).

### SQLite: FTS5 Trigram Tokenizer

Two FTS5 virtual tables backed by the `tm_variants` table, kept in sync on write:

- **`tm_variant_trigram`**: `tokenize='trigram'` on `plain`, `struct_key`, `general_key`. Used for fuzzy candidate retrieval.
- **`tm_variant_search`**: word tokenizer via `storage.FTSWordTokenizer` on `text`. Resolves to `icu` under cgo builds and `unicode61` under the default pure-Go (modernc, no-cgo) and wasm builds; the ICU tokenizer is a cgo-only FTS5 extension. Used for ranked UI search (FTS5 BM25).

`BuildTrigramQuery()` constructs the FTS5 MATCH expression:

- **Multi-word text** (Latin, etc.): OR of individual words ≥3 chars as quoted substrings.
- **Single word / CJK**: Overlapping 4-character windows sampled at even intervals (max 6 windows).

Falls back to length-based pre-filtering (`LENGTH(plain) BETWEEN min AND max`) if FTS5 trigram is unavailable at runtime.

### A second SQL dialect

The `memory/schema` package declares the tables once and emits them in two
dialects. Beside the SQLite definitions it carries a second SQL dialect whose
equivalents of the FTS tables are `pg_trgm` GIN indexes on `plain`,
`struct_key` and `general_key` and a generated `search_tsv` column with a GIN
index. The framework ships only the SQLite backend; a wider backend built on
the second dialect runs the same fuzzy scoring in Go over the candidates its
indexes retrieve.

### Performance

| Dataset      | Before (full scan) | After (trigram + Levenshtein) |
| ------------ | ------------------ | ----------------------------- |
| 1K entries   | ~5ms               | ~2ms                          |
| 10K entries  | ~50ms              | ~5ms                          |
| 100K entries | ~500ms+            | ~10-15ms                      |

## Storage Backends

1. **In-memory**: fast, ephemeral; for session-scoped leverage during batch processing.
2. **SQLite** (via `modernc.org/sqlite`): persistent; matching keys are pre-computed and indexed. FTS5 trigram indexes for fuzzy candidate retrieval; FTS5 word tokenizer for ranked UI search. Pure Go with no CGo dependencies in the default build.

Generalized and structural exact matching is an indexed lookup, fast even for large memories. Fuzzy matching uses trigram candidate retrieval to narrow the search space, then Levenshtein scoring on ~200 candidates.

## TMX element mapping

The import/export layer maps between inline-code runs and TMX inline elements:

| Run       | TMX Element |
| --------- | ----------- |
| `Ph`      | `<ph>`      |
| `PcOpen`  | `<bpt>`     |
| `PcClose` | `<ept>`     |

Entity metadata is carried as `<prop>` elements on the TMX `<tu>`. A TMX file carrying only plain text (no inline codes) imports as `Text`-only run sequences with no entity mappings; those entries participate in plain matching only.
