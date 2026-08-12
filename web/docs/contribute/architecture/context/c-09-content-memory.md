---
id: c-09-content-memory
sidebar_position: 9
title: "C-09: Content memory"
description: "Architecture decision: the content memory stores multilingual entries as Run sequences with inline markup and matches in tiers — generalized, structural, plain, then fuzzy — and rebuilds from the committed seeds and the committed translations rather than living in version control."
keywords: [content memory, runs, multilingual, matching tiers, SQLite, recycle, TMX, architecture decision, neokapi]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# C-09: Content memory

## Summary

The content memory is the framework's reuse store, in the `memory/` package. It
stores multilingual entries as per-locale `[]model.Run` sequences — preserving
inline markup and entity metadata — rather than flat strings, and uses a tiered
matching pipeline to maximize reuse. In-memory and SQLite backends ship; a wider
backend can be supplied behind the same interface.

The content memory is the project's **recycle** corpus: a pool of source→target
pairs reused to pre-fill and leverage future work. It is not the carrier of unit
state — whether a person reviewed or signed off a particular target lives in the
unit-state record ([C-04](c-04-unit-state-and-decisions.md)). Adding a pair to
the memory (`kapi apply` with `kind:"memory"`) is recycle leverage; it does not
promote a unit to *reviewed*.

## Context

Reuse of previously produced content is a core multilingual primitive. A
conventional store keeps flat source/target string pairs and matches on string
similarity alone, which loses information that matters:

- **Inline codes** are stripped before matching. A match is found but the codes
  do not transfer, and someone reinserts them by hand.
- **Named entities** are treated as literal text. *John works at Acme* and *Alice
  works at Globex* score low despite being structurally identical; the only
  differences are substitutable values.
- **Pipeline context** — entity annotations, term matches, check results —
  produced earlier in the flow is discarded.

A content-aware memory preserves run sequences end to end, derives several
matching keys from one entry, and returns matches with adaptation information so
the caller receives pre-adapted targets.

## Decision

### Content-aware, multilingual storage

An entry stores per-locale `[]model.Run` sequences — the same inline-content
representation used throughout the pipeline
([AD-002](../002-content-model.md)). An entry is **multilingual**:
each language is a peer variant in a map, with no authoritative source at the
persistence layer; the lookup direction is supplied at the call site.

```go
type Entry struct {
    ID          string
    ProjectID   string
    Variants    map[model.LocaleID][]model.Run // peer language variants
    HintSrcLang model.LocaleID                 // the locale the author treated as canonical
    Entities    []EntityMapping
    Properties  map[string]string
    Origins     []Origin
    Note        string
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

`HintSrcLang` records which locale the author treated as canonical, and is used
for display and entity direction only. An entity mapping records a typed entity
across every variant with its per-locale value and position.

### Derived matching keys

Each variant is indexed under three keys, derived from its run sequence and
computed at write time:

- **plain** — the flattened runs, with inline-code runs contributing their text
  equivalents. This is what matches against plain-text memories imported from
  other tools, and against unanalyzed content.
- **structural** — inline-code runs rendered as numbered placeholders, which
  preserves position awareness.
- **generalized** — entity runs rendered as typed placeholders. Maximum reuse:
  entities become interchangeable.

*John works at Acme* and *Alice works at Globex* both generalize to
`{PERSON} works at {ORGANIZATION}` — an exact match at the generalized tier.

### The tiered pipeline

`memory.TieredLookup` tries strategies in order of reuse potential:

1. generalized exact — entities differ, structure identical
2. structural exact — inline codes match exactly
3. plain exact — full score **only** when the inline-code structure also matches;
   a text-only match across *differing* structure is capped at near-exact, the
   industry tag-mismatch penalty. A 100% match means text *and* structure.
4. generalized fuzzy → 5. structural fuzzy → 6. plain fuzzy — Levenshtein on the
   corresponding keys, with a boost for entries from the same project.

Two cross-cutting rules apply to the exact tiers:

- **Ambiguity demotion.** When several entries match at full score but disagree
  on the target text, none of them is *the* translation: all are demoted to
  near-exact and flagged ambiguous. Full-score policies — a strict lookup, a
  100-threshold leverage step, extract pre-fill — therefore get nothing rather
  than a coin flip, and the choice surfaces for review. Identical targets at full
  score are not ambiguous, because the pick does not matter.
- **Deterministic ordering.** Results sort by score, then match-type priority,
  then entry id, so equal candidates never inherit incidental storage order.
  Without it, re-importing a memory can silently flip which of two exact matches
  wins.

The first match at or above the configured threshold wins, so a generalized exact
match is preferred over a plain fuzzy one.

One data-hygiene corollary: entries must keep inline markup as code runs, not as
literal text. An entry whose target embeds another format's markup tokens behind
a plain-text source defeats the structural tier and can leak those tokens into
any surface that shares the text — `kapi memory import` warns when variants
disagree on their markup-token sets.

### Entity adaptation

When a generalized match is found, the result carries adaptation information that
substitutes entity values from the current source into the stored target. The
`recycle` tool applies these automatically, so what arrives is a pre-adapted
target with the correct values already in place.

### The lookup interface

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

`Lookup` takes a `*model.Block` rather than a string. The block carries the
entity annotations needed to compute the generalized key and the inline-code runs
needed for the structural key, so no separate pre-processing step is required. By
default it keys on the block's whole content — the verbatim case when no
segmentation overlay is present. `LookupSegment` keys on a single segment span
for the sentence-level leverage path
([AD-017](../017-bilingual-format-interop.md)).

### The memory is state, not source

The content memory is **accumulated machine memory** — every translated segment
becomes leverage for the next — not authored, reviewed content. So unlike the
terms source ([C-08](c-08-terms.md)), which is *source*, the memory is **state,
kept out of version control**:

- its home is a store outside the working tree: locally the memory tables inside
  `.kapi/work/store.db` ([C-03](c-03-context-store-and-graph.md)); in CI whatever
  the job restores; and a shared backend where a team needs one authoritative,
  accumulating store. One continuum, larger backend.
- it is **rebuildable**, which makes it softer than most machine state: the
  leverage reconstructs from the committed translations plus the human-curated,
  **read-only** committed seeds. A cold or clobbered store is a performance hit,
  not data loss.
- because it is additive and rebuildable it needs **no locking**: it tolerates
  last-write-wins and per-branch cache keys.

Consequently **CI never commits the memory**: it builds the tables from the
committed sources, leverages and accumulates during the run, and discards them.
The translation *output* is what is committed. This is why a new terms source
arriving while a memory is in play is not a reconciliation problem — no store
lives in version control to conflict.

A project accumulates **many** memory bundles, not one — one per content surface
under `.kapi/memory/*.memory.json` — so the suffix, not the location, identifies
a seed. That is why the memory has no conventional-location fallback where the
terms source does: a conventional single name would force a project with a bundle
per surface to nominate one of them arbitrarily. A seed is named by
`defaults.memory_source` or by an explicit path; there is nothing sensible to
guess.

### The rebuild, in two stages

Both stages run on the read path, keyed by content digest, so an unchanged input
costs a read and no writes:

1. **The seeds** — every committed bundle under `.kapi/memory/`, not only the
   primary one bound by `defaults.memory_source`.
2. **The committed translations** — each collection's per-locale target document,
   paired with its source through the collection's own binding (the same reader
   and format config on both sides) and absorbed as source→target pairs.

The second stage is the half that carries wording no bundle holds. A translation
approved somewhere reaches version control as the target artifact, and without
reading it back the reviewed wording lives in exactly one place nothing in the
pipeline can see. The absorption reports what it did: pairs seen, documents read,
pairs learned, pairs reconciled, pairs contested, pairs refused.

The record is absorbed **after** the seeds, so on the pass that compiles both — a
fresh checkout — the committed translation supersedes the accelerant. Afterwards
each input has had its say and the digest decides: only an artifact whose bytes
moved is applied again, which is what lets a later seed edit, a pulled decision
or an approval stand rather than being overwritten by an unchanged file. A run
stamps the targets it writes itself, so a convergence never reads its own output
back as if a person had committed it.

Two rules keep the absorbed record honest. A pair whose target does not carry its
source's inline codes is refused rather than stored — the same predicate
`recycle` fills by. And where the record answers one source string more than one
way, the answer the corpus repeats most often wins: no text-keyed store can hold
both, and holding both makes every occurrence unfillable through the ambiguity
rule above.

Whether the loop materializes context on its own is a recipe decision:
`defaults.materialize` is `manual` by default and `on-converge` opts the project
into compiling its committed sources on the convergence path.

### Fuzzy candidate retrieval

Fuzzy matching uses trigram candidate retrieval to avoid full table scans, then
scores the candidate set with rune-level Levenshtein in Go.

The SQLite backend indexes the plain, structural and generalized keys in an FTS5
virtual table with a trigram tokenizer. Because these are not external-content
FTS tables, no SQL triggers are wired: the index is kept in sync explicitly on
each upsert and delete, with set-based rebuild functions for bulk imports. Where
the trigram tokenizer is unavailable at runtime, matching falls back to
length-based pre-filtering. A separate full-text table with relevance ranking
backs the ranked search the CLI and the desktop app use.

The trigram query is constructed differently for multi-word Latin text — an OR of
quoted substrings — and for single-word or CJK text, where overlapping character
windows are sampled at even intervals.

### Hybrid leverage

The tiers above are exact and fuzzy on normalized keys: strong for repetition and
near-repetition, blind to paraphrase. The direction is **hybrid** — the
deterministic tiers stay the high-confidence path and back locked full-score
leverage, complemented by **semantic retrieval**, embedding the source content
and ranking candidates by vector similarity, for suggestions where no exact or
close fuzzy match exists. Exact keys and embeddings derive from the same stored
runs on demand, and both the whole block and each span under a segmentation
overlay feed both paths. Semantic matches surface as scored suggestions, never as
silent auto-fill.

### Unicode normalization

Every matching key passes through Unicode NFC normalization before whitespace
normalization. This handles the real edge cases: Arabic tashkeel as separate
characters versus combined, Hangul jamo versus composed syllables, and accented
Latin composed two ways.

### TMX interchange

The memory imports and exports TMX for interchange with external tooling, mapping
the inline elements onto run kinds — `<ph>` to a placeholder run, `<bpt>`/`<ept>`
to the open and close halves of a paired code. Entity metadata travels as
properties on the translation unit. Legacy plain-text imports produce entries
whose variants are a single text run with no entity mappings, and participate in
plain matching only.

### Pipeline integration

The `recycle` tool is a translate-class tool
([AD-006](../006-tool-system.md)): it reads each block's source, queries the
memory, and where a match clears the fill threshold writes the target. It records
the outcome on the block's properties — the match score and match type — which
downstream tools read as context, so a translate step can skip what the memory
already filled at a high score.

<PipelineDiagram
  caption="recycle uses entity annotations for a generalized-tier match."
  stages={[
    { label: "Source", sub: "binding", role: "io" },
    { label: "entity-extract", sub: "model/NER", role: "annotate" },
    { label: "recycle", sub: "memory", role: "translate" },
    { label: "translate", sub: "provider", role: "translate" },
    { label: "qa", role: "qa" },
    { label: "Sink", sub: "binding · optional", role: "io" },
  ]}
/>

After translation, blocks are written back with their full run representation and
entity mappings, so the memory accumulates richer data over time.

## Consequences

- The memory stores rich content — run sequences with inline-code runs and entity
  metadata — not flat strings.
- Generalized matching turns entity variation from a fuzzy penalty into an exact
  match at the top tier.
- Inline codes survive storage and matching, reducing manual tag reinsertion.
- Matching on blocks rather than strings makes the memory a streaming pipeline
  stage that composes with other tools.
- Trigram candidate retrieval keeps fuzzy lookup fast at scale.
- The pure-Go SQLite driver preserves cross-compilation and the single-binary
  distribution goal.

## See also

- [C-04: Unit state and the decision record](c-04-unit-state-and-decisions.md) —
  the state carrier this store is deliberately not.
- [C-08: Terms](c-08-terms.md) — shared matching infrastructure and the
  source-versus-state contrast.
- [AD-002: Content Model](../002-content-model.md) — run
  sequences, inline-code runs, entity annotations.
- [Memory matching algorithm](../../implementation/memory-matching-algorithm.md)
  — trigram construction, the performance table, the TMX element mapping.
