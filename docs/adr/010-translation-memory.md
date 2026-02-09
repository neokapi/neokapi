---
id: 010-translation-memory
sidebar_position: 10
title: "ADR-010: Translation Memory"
---
# ADR-010: Content-aware translation memory

## Context

Translation memory (TM) is essential for localization: previously translated
segments are reused to maintain consistency and reduce cost. Okapi's TM
support relies on external tools (Olifant, Trados). We wanted TM to be
built-in and usable from CLI, server, and desktop app without external
dependencies.

Traditional TM systems store plain text pairs and match on string
similarity alone. This loses critical information:

- **Inline codes** (bold, links, placeholders) are stripped before matching.
  A match is found but the codes don't transfer — the translator must
  manually reinsert them.
- **Named entities** (people, products, dates) are treated as literal text.
  "John works at Acme" and "Alice works at Globex" have low match scores
  despite being structurally identical — the only differences are
  substitutable entity values.
- **Translation context** (entity annotations, term matches, QA results)
  produced by the pipeline is lost when storing flat strings.

gokapi's TM is content-aware: it stores full Fragments with Spans and
entity metadata, derives multiple matching keys, and returns matches with
entity adaptation information.

## Decision

### Content-Aware Storage

The Sievepen TM library (`lib/sievepen/`) stores Fragments — the same
content model type used throughout the pipeline (ADR-002) — rather than
plain strings. Each TM entry preserves inline Spans (markup codes) and
entity mappings.

```go
// TMEntry stores a translated segment with full content model representation.
type TMEntry struct {
    ID           string
    Source       *model.Fragment             // coded text + inline spans
    Target       *model.Fragment             // coded text + inline spans
    SourceLocale model.LocaleID
    TargetLocale model.LocaleID
    Entities     []EntityMapping             // entity placeholders in this entry
    Annotations  map[string]model.Annotation // carried from original translation
    Properties   map[string]string
    CreatedAt    time.Time
    UpdatedAt    time.Time
}

// EntityMapping tracks a named entity and its values in source and target.
// This enables generalized matching: entities are replaced with typed
// placeholders in the matching key, so structurally identical segments
// match regardless of entity values.
type EntityMapping struct {
    PlaceholderID string          // "e1", "e2" — links source and target positions
    Type          EntityType      // person, product, organization, date, etc.
    SourceValue   string          // original value in source ("John")
    SourcePos     TextRange       // position in source fragment
    TargetValue   string          // original value in target ("John" or adapted form)
    TargetPos     TextRange       // position in target fragment
}
```

### Derived Matching Keys

Each TM entry has multiple matching representations derived from its
stored Fragment. These are computed at storage time and indexed for fast
lookup:

```
Stored source Fragment: "John is a great guy working on gokapi"
                         ^^^^                           ^^^^^^
                         Span{Type:"person"}            Span{Type:"product"}

Derived keys:
  plain:        "John is a great guy working on gokapi"
  structural:   "{1} is a great guy working on {2}"
  generalized:  "\{PERSON\} is a great guy working on \{PRODUCT\}"
```

| Key | How it's derived | What it enables |
|-----|-----------------|-----------------|
| `plain` | `Fragment.Text()` — strips all Span markers | Matching against legacy TMs and unanalyzed content |
| `structural` | Spans rendered as numbered placeholders (`{1}`, `{/1}`) | Matching with inline code position awareness |
| `generalized` | Entity Spans as typed placeholders, structural Spans as numbered | Maximum reuse — entities are interchangeable |

The `generalized` key is the most powerful: "John works at Acme" and
"Alice works at Globex" both generalize to "\{PERSON\} works at
\{ORGANIZATION\}" — an exact match.

### Tiered Matching Pipeline

Lookup tries matching strategies in order of reuse potential:

```
1. generalized exact   → 1.0  (entities differ, structure identical)
2. structural exact    → 1.0  (inline codes match exactly)
3. plain exact         → 1.0  (text-only exact match)
4. generalized fuzzy   → Levenshtein on generalized keys
5. structural fuzzy    → Levenshtein on structural keys
6. plain fuzzy         → Levenshtein on plain keys
```

The first match at or above the score threshold wins. This means a
generalized exact match (different entity values, identical structure) is
preferred over a plain fuzzy match (similar text, unknown structure).

```go
type MatchType string
const (
    MatchGeneralizedExact MatchType = "generalized-exact"
    MatchStructuralExact  MatchType = "structural-exact"
    MatchExact            MatchType = "exact"
    MatchGeneralizedFuzzy MatchType = "generalized-fuzzy"
    MatchStructuralFuzzy  MatchType = "structural-fuzzy"
    MatchFuzzy            MatchType = "fuzzy"
)
```

Levenshtein edit distance with a configurable threshold (default 75%)
provides fuzzy matching. The ratio is language-independent, which is
acceptable for localization where exact and near-exact matches dominate.

### Entity Adaptation on Match

When a generalized match is found, the TM entry's target contains the
*stored* entity values (e.g., "Bob", "Okapi"). The match result carries
adaptation information that maps stored values to current values:

```go
type TMMatch struct {
    Entry             TMEntry
    Score             float64
    MatchType         MatchType
    EntityAdaptations []EntityAdaptation
}

// EntityAdaptation describes how to substitute an entity value
// in the matched target to produce a translation for the current source.
type EntityAdaptation struct {
    PlaceholderID string     // which entity ("e1")
    Type          EntityType // person, product, etc.
    StoredValue   string     // value in the TM target ("Bob")
    CurrentValue  string     // value in the current source ("John")
    TargetPos     TextRange  // where to substitute in the target
}
```

The `tm-leverage` tool applies adaptations automatically:

```
TM source:   "Bob is a great guy working on Okapi"
TM target:   "Bob est un mec super qui travaille sur Okapi"
Current src: "John is a great guy working on gokapi"

Match type: generalized-exact (1.0)
Adaptations: Bob→John, Okapi→gokapi

Adapted target: "John est un mec super qui travaille sur gokapi"
```

The translator receives a pre-adapted target — no manual entity
substitution needed.

### Lookup Interface

```go
type TranslationMemory interface {
    // Add inserts or updates a TM entry with full Fragment representation.
    Add(entry TMEntry) error

    // Lookup searches for matches using tiered matching (generalized → structural → plain).
    // The source Block's entity annotations are used to compute the generalized key.
    Lookup(source *model.Block, sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]TMMatch, error)

    // Delete removes an entry by ID.
    Delete(id string) error

    // Count returns the total number of entries.
    Count() int

    // Close releases resources.
    Close() error
}

type LookupOptions struct {
    MinScore   float64    // minimum match score (default 0.7)
    MaxResults int        // maximum results to return (default 10)
    MatchModes []MatchMode // which key types to use (default: all)
}

type MatchMode string
const (
    MatchModeGeneralized MatchMode = "generalized" // entity-aware matching
    MatchModeStructural  MatchMode = "structural"  // inline-code-aware matching
    MatchModePlain       MatchMode = "plain"        // text-only matching
)
```

Note that `Lookup` takes a `*model.Block` instead of a `string`. This
gives the TM access to the Block's entity annotations for computing the
generalized key, as well as Spans for the structural key.

### Storage Backends

1. **In-memory**: fast, ephemeral; for session-scoped leverage during
   batch processing. Matching keys are computed on the fly.
2. **SQLite** (via `modernc.org/sqlite`): persistent; matching keys are
   pre-computed and indexed. Uses the shared `internal/storage/`
   infrastructure layer (ADR-016). Pure Go with no CGo dependencies.

#### SQLite Schema

```sql
CREATE TABLE tm_entries (
    id              TEXT PRIMARY KEY,
    source_coded    TEXT NOT NULL,    -- serialized Fragment (JSON)
    target_coded    TEXT NOT NULL,    -- serialized Fragment (JSON)
    source_plain    TEXT NOT NULL,    -- derived: Fragment.Text()
    source_struct   TEXT NOT NULL,    -- derived: structural key
    source_general  TEXT NOT NULL,    -- derived: generalized key
    source_locale   TEXT NOT NULL,
    target_locale   TEXT NOT NULL,
    entities        TEXT,             -- JSON array of EntityMapping
    annotations     TEXT,             -- JSON map of carried annotations
    created_at      TEXT NOT NULL,
    updated_at      TEXT NOT NULL
);
CREATE INDEX idx_tm_general ON tm_entries(source_general, source_locale, target_locale);
CREATE INDEX idx_tm_struct  ON tm_entries(source_struct, source_locale, target_locale);
CREATE INDEX idx_tm_plain   ON tm_entries(source_plain, source_locale, target_locale);
```

Generalized and structural exact matching is an indexed lookup — fast
even for large TMs. Fuzzy matching falls back to scanning with
Levenshtein, which is acceptable because exact/near-exact matches
dominate in localization workflows.

### Pipeline Integration

The `tm-leverage` tool queries the TM for each Block's source content.
Because `Lookup` takes a full Block, the tool simply passes the Block
through — the TM reads entity annotations and Spans to compute matching
keys internally.

```go
func (t *TMLeverageTool) handleBlock(part *model.Part) (*model.Part, error) {
    block := part.Resource.(*model.Block)

    matches, _ := t.tm.Lookup(block, t.cfg.SourceLocale, t.cfg.TargetLocale, t.cfg.LookupOpts)
    if len(matches) == 0 {
        return part, nil
    }

    best := matches[0]

    // For exact matches (including generalized-exact), apply target directly.
    if best.Score >= 0.99 {
        adapted := applyEntityAdaptations(best.Entry.Target, best.EntityAdaptations)
        block.SetTargetFragment(t.cfg.TargetLocale, adapted)
    }

    // Attach match as AltTranslation annotation for downstream tools.
    block.Annotations["alt-translation"] = &model.AltTranslation{
        Source:    best.Entry.Source,
        Target:    best.Entry.Target,
        Locale:    t.cfg.TargetLocale,
        Origin:    "tm:sievepen",
        Score:     best.Score,
        MatchType: string(best.MatchType),
    }

    return part, nil
}
```

A typical pipeline with entity-aware TM:

```
reader → entity-annotate → tm-leverage → ai-translate → term-enforce → writer
```

The `entity-annotate` tool (ADR-016) runs first and attaches entity
annotations to Blocks. The `tm-leverage` tool reads those annotations
to compute generalized keys. AI translation only runs for Blocks
without TM matches — generalized matching dramatically increases the
exact match rate, reducing AI translation cost.

### Saving to TM

After translation (human or AI), Blocks are saved to TM with their
full Fragment representation and entity mappings. The save-to-TM step
extracts entity annotations from the Block and stores them as
`EntityMapping` entries on the TMEntry.

```
translated Block
  ├── source Fragment (with entity Spans)
  ├── target Fragment (with entity Spans)
  ├── entity annotations (from entity-annotate tool)
  └── other annotations (term matches, QA results)
       ↓
  TMEntry {
      Source: source Fragment,
      Target: target Fragment,
      Entities: [EntityMapping from annotations],
      Annotations: [carried context],
  }
```

This means the TM accumulates richer data over time as more content
passes through the pipeline with entity analysis.

### TMX Import/Export

TMX supports inline codes natively via `<ph>` (placeholder), `<bpt>`/
`<ept>` (begin/end paired tags), and `<it>` (isolated tags). The
import/export layer maps between Fragment Spans and TMX inline elements:

| Fragment Span | TMX Element |
|---|---|
| `SpanPlaceholder` | `<ph>` |
| `SpanOpening` | `<bpt>` |
| `SpanClosing` | `<ept>` |

Entity metadata is carried as `<prop>` elements on the TMX `<tu>`:
```xml
<tu tuid="entry-1">
  <prop type="entity:e1">person:John</prop>
  <prop type="entity:e2">product:gokapi</prop>
  <tuv xml:lang="en">
    <seg><ph type="person" x="e1">John</ph> is a great guy working on
         <ph type="product" x="e2">gokapi</ph></seg>
  </tuv>
  <tuv xml:lang="fr">
    <seg><ph type="person" x="e1">John</ph> est un mec super qui travaille sur
         <ph type="product" x="e2">gokapi</ph></seg>
  </tuv>
</tu>
```

When importing legacy TMX files that contain only plain text (no inline
codes), entries are stored with plain Fragments and no entity mappings.
They participate in plain matching only. Over time, as content is
re-processed through the entity-annotate pipeline, TM entries can be
enriched with entity information.

## Alternatives Considered

### Plain text TM (original design)

Store `Source string` and `Target string`. Simple and compatible with
any TM system. Rejected because it loses inline code information, cannot
generalize on entities, and misses the opportunity for content-aware
matching that dramatically increases reuse rates.

### External TM server (e.g., Moses, Trados)

Adds deployment complexity; defeats the single-binary goal. gokapi's TM
must work out of the box.

### BoltDB / BadgerDB

Key-value stores lack the query flexibility needed for tiered matching
with multiple indexed keys.

### PostgreSQL

Overkill for local TM; requires external service. SQLite provides indexed
queries with zero deployment overhead.

### `mattn/go-sqlite3`

CGo dependency; breaks cross-compilation. Chose `modernc.org/sqlite`
(pure Go) instead.

### Redact/unredact pipeline tools for TM generalization

Use separate pipeline tools to replace entities with placeholders before
TM lookup, and restore them after. Rejected because it conflates privacy
redaction with TM matching optimization. Making the TM content-aware
handles generalization internally — the right layer for this intelligence.
Redaction for privacy remains a separate, orthogonal pipeline tool
(ADR-016).

## Consequences

- TM stores rich content (Fragments with Spans and entity metadata), not
  flat strings
- Generalized matching turns entity variation from a fuzzy penalty into
  an exact match — "John works at Acme" matches "Alice works at Globex"
  at 100%
- Entity adaptation on match application means translators receive
  pre-adapted targets with correct entity values
- Inline codes are preserved through TM storage and matching, reducing
  manual tag reinsertion
- The tiered matching pipeline (generalized → structural → plain)
  maximizes reuse while falling back gracefully for legacy content
- TMX import/export roundtrips inline codes via standard TMX elements
- Legacy plain-text TM entries (from TMX import) work seamlessly via
  plain matching
- `Lookup` takes a `*model.Block` instead of a string, giving the TM
  direct access to annotations — no separate pre-processing step needed
- Shared SQLite infrastructure with TermBase (ADR-016) via
  `internal/storage/` reduces code duplication
- The `entity-annotate` tool (ADR-016) is the single source of entity
  information for both TM generalization and terminology management
- TM entry locale columns store BCP-47 tags in canonical form (see ADR-017)
