---
id: 009-translation-memory
sidebar_position: 9
title: "ADR-009: Translation Memory"
---
# ADR-009: Content-aware translation memory

## Context

Translation memory (TM) is essential for localization: previously translated
segments are reused to maintain consistency and reduce cost. Okapi's TM support
relies on external tools (Olifant, Trados). We wanted TM to be built-in and
usable from CLI, server, and desktop app without external dependencies.

Traditional TM systems store plain text pairs and match on string similarity
alone. This loses critical information:

- **Inline codes** (bold, links, placeholders) are stripped before matching.
  A match is found but the codes don't transfer -- the translator must manually
  reinsert them.
- **Named entities** (people, products, dates) are treated as literal text.
  "John works at Acme" and "Alice works at Globex" have low match scores
  despite being structurally identical -- the only differences are substitutable
  entity values.
- **Translation context** (entity annotations, term matches, QA results)
  produced by the pipeline is lost when storing flat strings.

gokapi's TM is content-aware: it stores full Fragments with Spans and entity
metadata, derives multiple matching keys, and returns matches with entity
adaptation information. TM now persists within the Content Store ecosystem
([ADR-003](./003-content-store.md)) and is portable via KAZ archives.

## Decision

### Content-Aware Storage

The Bowrain Memory TM library (`lib/sievepen/`) stores Fragments -- the same content
model type used throughout the pipeline ([ADR-002](./002-content-model.md)) --
rather than plain strings. Each TM entry preserves inline Spans (markup codes)
and entity mappings.

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

### Derived Matching Keys

Each TM entry has multiple matching representations derived from its stored
Fragment. These are computed at storage time and indexed for fast lookup:

- **plain**: `Fragment.Text()` -- strips all Span markers. Enables matching
  against legacy TMs and unanalyzed content.
- **structural**: Spans rendered as numbered placeholders (`\{1\}`, `\{/1\}`).
  Enables matching with inline code position awareness.
- **generalized**: Entity Spans as typed placeholders (`\{PERSON\}`,
  `\{PRODUCT\}`). Maximum reuse -- entities are interchangeable.

The generalized key is the most powerful: "John works at Acme" and "Alice works
at Globex" both generalize to `\{PERSON\} works at \{ORGANIZATION\}` -- an
exact match.

### Tiered Matching Pipeline

Lookup tries matching strategies in order of reuse potential:

1. generalized exact -- score 1.0 (entities differ, structure identical)
2. structural exact -- score 1.0 (inline codes match exactly)
3. plain exact -- score 1.0 (text-only exact match)
4. generalized fuzzy -- Levenshtein on generalized keys
5. structural fuzzy -- Levenshtein on structural keys
6. plain fuzzy -- Levenshtein on plain keys

The first match at or above the score threshold wins. A generalized exact match
(different entity values, identical structure) is preferred over a plain fuzzy
match (similar text, unknown structure). Levenshtein edit distance with a
configurable threshold (default 75%) provides fuzzy matching.

### Entity Adaptation

When a generalized match is found, the match result carries adaptation
information to substitute entity values from the current source into the stored
target:

```go
type TMMatch struct {
    Entry             TMEntry
    Score             float64
    MatchType         MatchType
    EntityAdaptations []EntityAdaptation
}
```

The `tm-leverage` tool ([ADR-006](./006-tool-system.md)) applies adaptations
automatically -- translators receive pre-adapted targets with correct entity
values.

### Lookup Interface

```go
type TranslationMemory interface {
    Add(entry TMEntry) error
    Lookup(source *model.Block, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error)
    Delete(id string) error
    Count() int
    Close() error
}
```

`Lookup` takes a `*model.Block` instead of a `string`. This gives the TM access
to the Block's entity annotations for computing the generalized key, as well as
Spans for the structural key. No separate pre-processing step is needed.

### Storage Backends

1. **In-memory**: fast, ephemeral; for session-scoped leverage during batch
   processing.
2. **SQLite** (via `modernc.org/sqlite`): persistent; matching keys are
   pre-computed and indexed. Uses the shared `internal/storage/` infrastructure
   layer with TermBase ([ADR-010](./010-terminology.md)) and Content Store
   ([ADR-003](./003-content-store.md)). Pure Go with no CGo dependencies.

Generalized and structural exact matching is an indexed lookup -- fast even for
large TMs. Fuzzy matching falls back to scanning with Levenshtein, which is
acceptable because exact and near-exact matches dominate in localization
workflows.

### Content Store Integration

TM persists alongside the Content Store ([ADR-003](./003-content-store.md)).
When a project version is created, TM entries relevant to that project are
snapshotted. KAZ archives embed TM entries as `tm/entries.json` for portable
sharing -- a team member can import a KAZ file and get both the project content
and TM in one step.

After translation (human or AI), Blocks are saved to TM with their full
Fragment representation and entity mappings. The save-to-TM step extracts
entity annotations from the Block and stores them as `EntityMapping` entries.
This means the TM accumulates richer data over time as more content passes
through the pipeline with entity analysis.

### TMX Import/Export

The import/export layer maps between Fragment Spans and TMX inline elements:

| Fragment Span | TMX Element |
|---|---|
| `SpanPlaceholder` | `<ph>` |
| `SpanOpening` | `<bpt>` |
| `SpanClosing` | `<ept>` |

Entity metadata is carried as `<prop>` elements on the TMX `<tu>`. When
importing legacy TMX files that contain only plain text (no inline codes),
entries are stored with plain Fragments and no entity mappings. They participate
in plain matching only.

## Alternatives Considered

- **Plain text TM**: loses inline code information and entity metadata; no
  generalized matching. Dramatically lower reuse rates.
- **External TM server** (Moses, Trados): adds deployment complexity; defeats
  the single-binary goal. gokapi's TM must work out of the box.
- **BoltDB / BadgerDB**: key-value stores lack query flexibility for tiered
  matching with multiple indexed keys.
- **PostgreSQL**: overkill for local TM; requires external service. SQLite
  provides indexed queries with zero deployment overhead.
- **`mattn/go-sqlite3`**: CGo dependency breaks cross-compilation. Chose pure
  Go `modernc.org/sqlite` instead.

## Consequences

- TM stores rich content (Fragments with Spans and entity metadata), not flat
  strings
- Generalized matching turns entity variation from a fuzzy penalty into an exact
  match -- "John works at Acme" matches "Alice works at Globex" at 100%
- Entity adaptation provides pre-adapted targets with correct entity values
- Inline codes are preserved through TM storage and matching, reducing manual
  tag reinsertion
- The tiered matching pipeline (generalized, structural, plain) maximizes reuse
  while falling back gracefully for legacy content
- TMX roundtrip preserves inline codes via standard TMX elements
- Shared SQLite infrastructure with TermBase ([ADR-010](./010-terminology.md))
  and Content Store ([ADR-003](./003-content-store.md)) reduces code duplication
- TM is portable via KAZ archives for offline sharing
