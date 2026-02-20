---
sidebar_position: 4
title: "TM Matching Algorithm"
---
# TM Matching Algorithm

This note provides implementation details for [AD-009](/docs/ad/009-translation-memory).

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

The first match at or above the score threshold wins. A generalized exact match (different entity values, identical structure) is preferred over a plain fuzzy match (similar text, unknown structure). Levenshtein edit distance with a configurable threshold (default 75%) provides fuzzy matching.

## Entity Adaptation

When a generalized match is found, the match result carries adaptation information to substitute entity values from the current source into the stored target:

```go
type TMMatch struct {
    Entry             TMEntry
    Score             float64
    MatchType         MatchType
    EntityAdaptations []EntityAdaptation
}
```

The `tm-leverage` tool ([AD-006](/docs/ad/006-tool-system)) applies adaptations automatically -- translators receive pre-adapted targets with correct entity values.

## Storage Backends

1. **In-memory**: fast, ephemeral; for session-scoped leverage during batch processing.
2. **SQLite** (via `modernc.org/sqlite`): persistent; matching keys are pre-computed and indexed. Uses the shared `bowrain/storage/` infrastructure layer with TermBase ([AD-010](/docs/ad/010-terminology)) and Content Store ([AD-003](/docs/ad/003-content-store)). Pure Go with no CGo dependencies.

Generalized and structural exact matching is an indexed lookup -- fast even for large TMs. Fuzzy matching falls back to scanning with Levenshtein, which is acceptable because exact and near-exact matches dominate in localization workflows.

## TMX Element Mapping

The import/export layer maps between Fragment Spans and TMX inline elements:

| Fragment Span | TMX Element |
|---|---|
| `SpanPlaceholder` | `<ph>` |
| `SpanOpening` | `<bpt>` |
| `SpanClosing` | `<ept>` |

Entity metadata is carried as `<prop>` elements on the TMX `<tu>`. When importing legacy TMX files that contain only plain text (no inline codes), entries are stored with plain Fragments and no entity mappings. They participate in plain matching only.
