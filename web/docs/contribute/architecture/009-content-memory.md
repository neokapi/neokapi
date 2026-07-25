---
id: 009-content-memory
sidebar_position: 9
title: "AD-009: Content memory"
description: "Architecture decision: the content-memory library stores multilingual entries as Run sequences with inline markup and matches in three tiers (plain, structural, source-entity) to return the highest-quality match first."
keywords: [content memory, runs, multilingual, matching tiers, SQLite, architecture decision, neokapi]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# AD-009: Content memory

## Summary

Content memory is neokapi's built-in reuse store, living in the `memory/`
package. It stores multilingual entries as per-locale `[]model.Run`
sequences — preserving inline markup and entity metadata — rather than flat
strings, and uses a tiered matching pipeline (generalized exact, structural
exact, plain exact, fuzzy) — complemented by semantic retrieval for paraphrase
— to maximize reuse. The framework ships in-memory and SQLite backends; a
PostgreSQL backend can be supplied by a platform layer behind the same
interface.

Content memory is the project's **recycle** corpus: a pool of source→target
pairs reused to pre-fill and leverage future translation. It is not the carrier
of workflow **decisions** — whether a person has reviewed or signed off a
particular target lives in the project **state store**
([AD-008](008-project-model.md), `defaults.state`), not here. Adding a pair to
the memory (`kapi apply` with `kind:"tm"`) is recycle leverage; it does not
promote a unit to *reviewed*.

## Context

Reuse of previously produced content is a core multilingual primitive:
translated segments are recycled to maintain consistency and reduce cost.
Conventional translation-memory systems store flat source/target string pairs
and match on string similarity alone, which loses information that matters to
translators:

- **Inline codes** (bold, links, placeholders) are stripped before matching.
  A match is found but the codes do not transfer — the translator manually
  reinserts them.
- **Named entities** (people, products, dates) are treated as literal text.
  "John works at Acme" and "Alice works at Globex" score low despite being
  structurally identical; the only differences are substitutable entity
  values.
- **Pipeline context** (entity annotations, term matches, QA results)
  produced earlier in the flow is discarded.

A content-aware memory preserves Run sequences end-to-end, derives multiple
matching keys from a single entry, and returns matches with entity adaptation
information so translators receive pre-adapted targets.

## Decision

### Content-aware, multilingual storage

Content memory stores per-locale `[]model.Run` sequences — the same
inline-content representation used throughout the pipeline ([AD-002: Content
Model](002-content-model.md)) — rather than strings. An entry is
**multilingual**: each language is a peer variant in a `Variants` map, with no
authoritative "source" at the persistence layer. The lookup direction is
supplied at the call site. Each variant preserves inline-code runs (markup
codes) and the entry carries entity mappings.

```go
type Entry struct {
    ID          string
    ProjectID   string
    Variants    map[model.LocaleID][]model.Run // peer language variants
    HintSrcLang model.LocaleID                 // locale the author treated as canonical
    Entities    []EntityMapping
    Properties  map[string]string
    Origins     []Origin
    Note        string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

`HintSrcLang` records which locale the author treated as canonical (e.g. the
TMX header `srclang`, or the locale a translator started from); it is used for
display and entity-direction purposes only. An `EntityMapping` records a typed
entity across all variants (`Values map[LocaleID]EntityValue`) with its
per-locale value and position. `Entry` helpers project a single variant:
`Variant(locale)` returns its runs, `VariantText` / `VariantStructural` /
`VariantGeneralized` return the corresponding text keys.

### Derived matching keys

Each variant is indexed under three keys, derived from its Run sequence and
pre-computed at write time:

- **plain** — `model.FlattenRuns(runs)` with inline-code runs contributing
  their text equivalents. Enables matching against legacy memories and
  unanalyzed content.
- **structural** — `model.RunsStructuralText(runs)`: inline-code runs rendered
  as numbered placeholders (`{1}`, `{/1}`). Preserves inline-code position
  awareness.
- **generalized** — `model.RunsGeneralizedText(runs)`: entity `Ph` runs
  rendered as typed placeholders (`{PERSON}`, `{PRODUCT}`). Maximum reuse;
  entities become interchangeable.

"John works at Acme" and "Alice works at Globex" both generalize to
`{PERSON} works at {ORGANIZATION}` — an exact match at the generalized tier.

### Tiered matching pipeline

Lookup tries strategies in order of reuse potential:

1. generalized exact — score 1.0 (entities differ, structure identical)
2. structural exact — score 1.0 (inline codes match exactly)
3. plain exact — score 1.0 only when the inline-code structure also
   matches; a text-only match across *differing* structure (a bare heading
   against a markup-wrapped entry) is capped at `ScoreNearExact` (0.99) —
   the industry "tag mismatch" penalty. A 100% match means text *and*
   structure.
4. generalized fuzzy — Levenshtein on generalized keys
5. structural fuzzy — Levenshtein on structural keys
6. plain fuzzy — Levenshtein on plain keys

Two cross-cutting rules apply to the exact tiers:

- **Ambiguity demotion.** When several entries match at full score but
  disagree on the target text, none of them is *the* translation: all are
  demoted to `ScoreNearExact` and flagged `Match.Ambiguous`. Full-score
  policies (`MinScore: 1.0` lookups, `fillTargetThreshold: 100` leverage,
  extract pre-fill) therefore get nothing rather than a coin flip; the
  choice surfaces for review. Identical targets at full score are not
  ambiguous — the pick doesn't matter.
- **Deterministic ordering.** Results sort by score, then match-type
  priority, then entry ID. Before this, equal candidates inherited
  incidental storage order — re-importing a memory could silently flip
  which of two exact matches won (the failure mode that leaked a desktop
  UI markup token into a docs page).

The first match at or above the configured score threshold wins. A
generalized exact match (different entity values, identical structure) is
preferred over a plain fuzzy match (similar text, unknown structure).
Levenshtein edit distance with a configurable threshold (default 70%)
controls fuzzy matching.

One data-hygiene corollary: entries must keep inline markup as code runs,
not literal text. An entry whose target text embeds another format's
markup tokens behind a plain-text source defeats the structural tier and
can leak those tokens into any surface that shares the text —
`kapi memory import` warns when variants disagree on their markup-token sets.

### Entity adaptation

When a generalized match is found, the result carries adaptation information
that substitutes entity values from the current source into the stored
target:

```go
type Match struct {
    Entry             Entry
    Score             float64
    MatchType         MatchType
    ProjectID         string
    EntityAdaptations []EntityAdaptation
    Ambiguous         bool // several full-score exacts with differing targets
}
```

The `recycle` tool applies these adaptations automatically, so
translators receive pre-adapted targets with the correct entity values
already substituted.

### Lookup interface

```go
type ContentMemory interface {
    Add(entry Entry) error
    Lookup(source *model.Block, sourceLocale, targetLocale model.LocaleID,
        opts LookupOptions) ([]Match, error)
    LookupSegment(source *model.Block, segmentIdx int,
        sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]Match, error)
    Delete(id string) error
    Count() int
    Close() error
}
```

`Lookup` takes a `*model.Block` rather than a string. The Block carries the
entity annotations needed to compute the generalized key and the inline-code
runs needed for the structural key; no separate pre-processing step is
required. By default `Lookup` keys on the block's whole content — the verbatim
lookup case when no segmentation overlay is present. Matches are found among
entries whose `Variants[sourceLocale]` exists and matches the source;
`Match.Entry.Variant(targetLocale)` is the translation.

`LookupSegment` keys on a single segment span — `segmentIdx` indexes the
block's segmentation overlay ([AD-002](002-content-model.md)) — for the
sentence-level leverage path used by `kapi extract` when the project's
recipe sets `segmentation.source: true` (see
[AD-017](017-bilingual-format-interop.md)).

### Backends

The framework provides two tiers:

- **In-memory** (`memory/inmemory.go`) — fast, ephemeral; session-scoped
  leverage during batch processing.
- **SQLite** (`memory/sqlite.go`) — persistent file-based storage for CLI
  tools. Same matching algorithm as the in-memory tier, with FTS5 indexes
  for fuzzy candidate retrieval. Uses `modernc.org/sqlite` (pure Go, no CGo)
  for cross-compilation.

A PostgreSQL backend with workspace-scoped isolation and project scoping can
be supplied by a platform layer, reusing the same matching algorithm behind
the same `ContentMemory` interface.

### Content memory is derived state, not committed source

Content memory is **accumulated machine memory** — every translated segment
becomes leverage for the next — not authored, reviewed content. So unlike the
terms store ([AD-010](010-terminology.md)), which is *source*, the memory is
**state, kept out of git scope** — the same posture as Terraform state:

- its home is a store **outside git**: `.kapi/cache/` locally, the CI job cache
  (the `actions/cache` idiom — restore at job start, save at job end) in CI, and
  the bowrain platform database for a team (the shared, authoritative "remote
  backend"). One continuum, larger backend.
- it is **rebuildable**, which makes it softer than Terraform state: the leverage
  reconstructs from the committed translations (the per-locale `i18n/{lang}/`
  target catalogs) plus an optional human-curated, **read-only** committed
  `.memory.json` *seed*
  bound by `defaults.tm_source`. A cold or clobbered cache is a performance hit,
  not data loss.
- because it is additive and rebuildable it needs **no locking**: it tolerates
  last-write-wins or per-branch cache keys, unlike Terraform state, which must be
  locked because it is irreplaceable.

Consequently **CI never commits the memory**: it restores the store from the
cache, leverages and accumulates during the run, and saves it back to the cache —
the translation *output* (`i18n/{lang}/`) is what gets committed to git. This is
why "a new `.terms.json` arrives while a memory is in play" is not a reconciliation
problem: no store lives in git to conflict. Committing the binary SQLite would be
git-hostile and defeat interchange in any case; the `.memory.json` bundle
remains for explicit human snapshot / seed / transfer (deterministic, lossless),
not as an auto-grown git artifact.

A project accumulates **many** memory bundles, not one — this repository's own
dogfood commits a bundle per content surface under `l10n/tm/*.memory.json` — so
the suffix, not the location, is what identifies a seed. This is why content
memory has no conventional-location fallback where the terms store does
([AD-010](010-terminology.md)): a project has exactly one glossary, so
`<root>/terms.json` has a single obvious answer, while a conventional
`memory.json` would force a project with a bundle per surface to nominate one of
them arbitrarily. A seed is named by `defaults.tm_source` or by an explicit
path; there is nothing sensible to guess.

> **State, not source.** Contrast the terms store
> ([AD-010](010-terminology.md)), which is *authored source* committed to git and
> reviewed. Content memory is *state* — derived, out of git, rebuildable — and
> where an accumulating store must be shared and authoritative across a team,
> that is the server's job, not git's.

### Fuzzy candidate retrieval

Fuzzy matching uses trigram-based candidate retrieval to avoid full table
scans. The candidate set (target ~200 entries) is then scored with
character-level Levenshtein in Go.

- **SQLite** — an FTS5 virtual table with `tokenize='trigram'` indexes
  `plain`, `struct_key`, and `general_key`. Because these are not `content=`
  external-content FTS tables, no SQL triggers are wired; the index is kept in
  sync manually — explicit DELETE/INSERT into `tm_variant_trigram` on each
  upsert/delete, plus `RebuildFuzzyIndex()`/`RebuildSearchIndex()` for set-based
  repopulation after bulk imports. Falls back to length-based pre-filtering if
  FTS5 trigram is unavailable at runtime.
- **SQLite UI search** — a separate FTS5 `unicode61` table with BM25
  ranking, used by the CLI and desktop UI for ranked full-text search.

`BuildTrigramQuery()` constructs the FTS5 MATCH expression differently for
multi-word Latin text (OR of quoted substrings ≥3 characters) and for
single-word or CJK text (overlapping 4-character windows sampled at even
intervals).

### Hybrid leverage: exact tiers plus semantic retrieval

The tiers above are *exact and fuzzy on normalized keys* — strong for
repetition and near-repetition, blind to paraphrase. The intended direction is
**hybrid**: the deterministic exact/structural/generalized tiers stay the
high-confidence path (and back locked 100% / ICE leverage), complemented by
**semantic retrieval** — embedding the source content and ranking candidates by
vector similarity — for suggestions where no exact or close fuzzy match exists.
Exact keys and embeddings derive from the same stored `[]Run` on demand; the
whole block, and per-span when a segmentation overlay is present, feed both
paths. Semantic matches surface as scored suggestions, never as silent
auto-fill.

### Unicode normalization

All matching keys are passed through `NormalizeText()`, which applies
Unicode NFC (`golang.org/x/text/unicode/norm`) before whitespace
normalization. This handles real edge cases: Arabic tashkeel as separate
characters vs. combined, Hangul jamo vs. composed syllables, and accented
Latin (e + combining acute vs. é).

### TMX import and export

Content memory imports and exports TMX (Translation Memory eXchange) files for
interchange with external tooling. The element mapping (TMX inline element ↔
`model.Run` kind):

| TMX element | Run kind     |
| ----------- | ------------ |
| `<ph>`      | `Ph`         |
| `<bpt>`     | `PcOpen`     |
| `<ept>`     | `PcClose`    |

Entity metadata travels as `<prop>` elements on the TMX `<tu>`. Legacy
plain-text TMX imports produce entries whose variants are a single `TextRun`
with no entity mappings; they participate in plain matching only.

### Pipeline integration

The `recycle` tool is a `Translate`-capability tool
([AD-006: Tool System](006-tool-system.md)): it reads each block's source,
queries the memory (exact, then fuzzy above the configured threshold), and, when
a match clears the fill threshold, writes the translated target via
`SetTargetText`. It records the outcome on `Block.Properties` —
`tm-match-score` (0–100) and `tm-match-type` (`exact` or `fuzzy`). Downstream
tools — `translate`, UI review, QA — read those properties as context (for
example, `translate` can skip blocks the memory already filled at a high
score).

A typical flow:

<PipelineDiagram
  caption="recycle uses entity annotations for generalized-tier match."
  stages={[
    { label: "Source", sub: "binding", role: "io" },
    { label: "entity-extract", sub: "LLM/NER", role: "annotate" },
    { label: "recycle", sub: "memory", role: "translate" },
    { label: "translate", sub: "LLM/MT provider", role: "translate" },
    { label: "qa", sub: "LLMProvider", role: "qa" },
    { label: "Sink", sub: "binding · optional", role: "io" },
  ]}
/>

After translation (human or AI), Blocks are written to the memory with their full
Run representation and entity mappings. The save step extracts entity
annotations and stores them as `EntityMapping` entries, so the memory
accumulates richer data over time.

## Consequences

- The memory stores rich content (Run sequences with inline-code runs and entity
  metadata), not flat strings.
- Generalized matching turns entity variation from a fuzzy penalty into an
  exact match at the top tier.
- Entity adaptation provides pre-adapted targets with the correct entity
  values, reducing manual editing.
- Inline codes survive storage and matching, reducing manual tag
  reinsertion.
- The SQLite backend uses pure-Go `modernc.org/sqlite`, preserving cross-
  compilation and the single-binary distribution goal.
- Matching on Blocks (not strings) makes the memory a streaming pipeline stage
  that composes naturally with other tools.
- Trigram candidate retrieval keeps fuzzy lookup fast even for 100K-entry
  memories.

## Related

- [AD-002: Content Model](002-content-model.md) — Run sequences, inline-code
  runs, entity annotations
- [AD-006: Tool System](006-tool-system.md) — `recycle` tool
- [AD-010: Terminology](010-terminology.md) — shares matching infrastructure
- [Memory matching algorithm](/contribute/implementation/memory-matching-algorithm) — trigram
  construction, performance table, TMX element mapping
