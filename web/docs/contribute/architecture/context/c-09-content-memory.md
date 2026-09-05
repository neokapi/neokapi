---
id: c-09-content-memory
sidebar_position: 9
title: "C-09: Content memory"
description: "Architecture decision: the content memory stores multilingual entries as Run sequences with inline markup, matches in tiers (generalized, structural, plain, then fuzzy), keeps a governed version chain per block, and rebuilds from the committed seeds and the committed translations rather than living in version control."
keywords: [content memory, runs, multilingual, matching tiers, version chain, governed reuse, SQLite, recycle, TMX, architecture decision, neokapi]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# C-09: Content memory

## Summary

The content memory is the framework's reuse store, in the `memory/` package. It
stores multilingual entries as per-locale `[]model.Run` sequences, preserving
inline markup and entity metadata rather than flat strings, and uses a tiered
matching pipeline to maximize reuse. In-memory and SQLite backends ship; a wider
backend can be supplied behind the same interface.

The content memory is the project's **recycle** corpus: a pool of source→target
pairs reused to pre-fill and leverage future work. It is not the carrier of unit
state. Whether a person reviewed or signed off a particular target lives in the
unit-state record ([C-04](c-04-unit-state-and-decisions.md)). Adding a pair to
the memory (`kapi apply` with `kind:"memory"`) is recycle leverage; it does not
promote a unit to *reviewed*.

The corpus also holds each block's **version chain**: the answers approved for
that block over time, each carrying the governing context it was approved under.
A prior answer is offered to a producer as reference only under the governance
it was approved under, and the producer-facing contract for both questions is
one interface, `core/memory.Provider`.

## Context

Reuse of content produced before is a core multilingual primitive. A
conventional store keeps flat source/target string pairs and matches on string
similarity alone, which loses information that matters:

- **Inline codes** are stripped before matching. A match is found but the codes
  do not transfer, and someone reinserts them by hand.
- **Named entities** are treated as literal text. *John works at Acme* and *Alice
  works at Globex* score low despite being structurally identical; the only
  differences are substitutable values.
- **Pipeline context** (entity annotations, term matches, check results)
  produced earlier in the flow is discarded.

A content-aware memory preserves run sequences end to end, derives several
matching keys from one entry, and returns matches with adaptation information so
the caller receives pre-adapted targets.

Similarity is also the wrong tool for one common case. A block whose source is
edited writes a new entry beside the one that came before, keyed by the new
text, and a similarity score says nothing about whether the two are successive
answers for the same block or unrelated strings that happen to share a project.
Continuity for that block comes from its own history, not from a near-match.

## Decision

### Content-aware, multilingual storage

An entry stores per-locale `[]model.Run` sequences, the same inline-content
representation used throughout the pipeline
([F-02](../foundations/f-02-content-model.md)). An entry is **multilingual**:
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
    Point       string                         // where this answer was approved
    Unit        string                         // the block this answer was approved for
    CreatedAt   time.Time
    UpdatedAt   time.Time
}
```

`HintSrcLang` records which locale the author treated as canonical, and is used
for display and entity direction only. An entity mapping records a typed entity
across every variant with its per-locale value and position. `Point` records
where the answer was approved (see [Where an answer was
approved](#where-an-answer-was-approved)) and `Unit` which block it was approved
for (see [Version chains and governed reuse](#version-chains-and-governed-reuse)).
Each `Origin` carries a `ContextFingerprint`, the governing context in force
when that answer was produced, so an answer absorbed into the corpus keeps the
statement about what governed it; it is empty for an import, a seed, or a
producer that ran ungoverned.

The store normalizes the locale beside the text. Every variant key, entity
value key and lookup locale is held in canonical BCP-47 form
(`locale.Normalize`, the lenient form of the canonicalization every locale
crosses at a recipe boundary), on the same write and the same statement that
normalize the text. An entry written under `nb_NO` and a lookup asking for
`nb-NO` meet at one row, and `Entry.Variant` answers for either spelling. The
entry id `kapi apply` mints for a correction, `apply:<source>:<target>:<hash>`,
embeds both locales in that form, so one pair has one entry whichever way a
change entry spelled it. A store written before the locale was normalized may
hold rows under another spelling; `kapi status` and `kapi up` report them and
name the rebuild, because no lookup will find them again.

### Derived matching keys

Each variant is indexed under three keys, derived from its run sequence and
computed at write time:

- **plain**: the flattened runs, with inline-code runs contributing their text
  equivalents. This is what matches against plain-text memories imported from
  other tools, and against unanalyzed content.
- **structural**: inline-code runs rendered as numbered placeholders, which
  preserves position awareness.
- **generalized**: entity runs rendered as typed placeholders. Maximum reuse:
  entities become interchangeable.

*John works at Acme* and *Alice works at Globex* both generalize to
`{PERSON} works at {ORGANIZATION}`, an exact match at the generalized tier.

### The tiered pipeline

`memory.TieredLookup` tries strategies in order of reuse potential:

1. generalized exact: entities differ, structure identical
2. structural exact: inline codes match exactly
3. plain exact: full score **only** when the inline-code structure also matches;
   a text-only match across *differing* structure is capped at near-exact, the
   industry tag-mismatch penalty. A 100% match means text *and* structure.
4. generalized fuzzy → 5. structural fuzzy → 6. plain fuzzy: Levenshtein on the
   corresponding keys, with a boost for entries from the same project.

Two cross-cutting rules apply to the exact tiers:

- **Nearest approval wins.** When several entries match at full score but
  disagree on the target text, the one approved nearest the point the caller is
  asking from is the one that governs there, and the others demote out of its
  way. See [Where an answer was approved](#where-an-answer-was-approved).
- **Ambiguity demotion.** A caller that names no point cannot prefer one
  approval over another, so none of them is *the* translation: all are demoted
  to near-exact and flagged ambiguous. Full-score policies (a strict lookup, a
  100-threshold leverage step, extract pre-fill) therefore get nothing rather
  than a coin flip, and the choice surfaces for review. Identical targets at full
  score are not ambiguous, because the pick does not matter.
- **Deterministic ordering.** Results sort by score, then match-type priority,
  then entry id, so equal candidates never inherit incidental storage order.
  Without it, re-importing a memory can silently flip which of two exact matches
  wins.

The first match at or above the configured threshold wins, so a generalized exact
match is preferred over a plain fuzzy one.

**The fill policy sits above the matcher.** The `recycle` tool looks up at its
`fuzzyThreshold` (default 70) and fills a target at or above its
`fillTargetThreshold` (default 95); a match between the two floors is recorded
as an alternative-translation candidate and read by nothing. The fill floor
catches the cosmetic edits an author actually makes (a trailing period, an added
comma, a capitalised word), which score between 96 and 98 at realistic sentence
length. The lookup floor below it is inert. A recipe that wants exact-only fill
sets `fillTargetThreshold: 100`, which is what the dogfood recipe does. A
sub-exact fill is a pricing mechanism from a time when a person edited rather
than wrote, and it is retiring in favour of the version chain below: a
near-match makes a reader diff the source against a stored target to find what
moved, which is harder than reading a clean draft.

One data-hygiene corollary: entries must keep inline markup as code runs, not as
literal text. An entry whose target embeds another format's markup tokens behind
a plain-text source defeats the structural tier and can leak those tokens into
any surface that shares the text. `kapi memory import` warns when variants
disagree on their markup-token sets.

### Entity adaptation

When a generalized match is found, the result carries adaptation information that
substitutes entity values from the current source into the stored target. The
`recycle` tool applies these automatically, so what arrives is a pre-adapted
target with the correct values already in place.

### Where an answer was approved

A project answers one source string more than once, and the answers do not have
to agree. Two collections reviewed apart (a CLI's help catalog and an engine's
metadata catalog) can each carry a reviewed Norwegian wording for the same
English sentence, and both are correct where they were approved. The corpus
absorbs both, so it has to be able to hand back the right one.

**A decision is qualified by where it was made.** Each entry the record teaches
the corpus carries the **context point** its answer was approved at, the
coordinate [C-02](c-02-coordinates-and-governance.md) resolves, rendered as the
containment ladder:

```
profile → channel → collection
```

**Nearest is a prefix comparison down that ladder.** Two points are 0 apart when
they name the same collection, 1 apart when they share a channel, 2 apart when
they share only a product, and maximally apart when they share nothing.
Containment is what makes the comparison meaningful: two files in one collection
ship on one channel by construction, so a match at a fine rung that disagreed at
a coarse one would not describe anything. An entry bound to no location (a seed,
an import, an ad-hoc addition) sits at the project's default point, which is
maximally far from every answer that names a product. That is the right reading
rather than a missing value: such an entry was never approved anywhere in
particular.

The ladder stops at the collection because that is the finest place a fill can
honestly name itself at. A project flow resolves its governance once and bakes it
into the tool chain before any content is read, and the chain is shared by every
file of a binding group ([C-02](c-02-coordinates-and-governance.md), *One run,
one resolution per collection*), so the point a `recycle` step asks from is its
group's product and channel.

**A genuine tie is broken by the answer's own text, smallest byte sequence
first.** Two approvals at one point, or two points equally far from the asker,
are a disagreement the ladder cannot separate, and the fallback must not be
anything that moves when the rest of the corpus moves. A winner decided by how
often each spelling happens to appear flips when an unrelated string is added or
removed, so a rebuild stops reproducing the wording it started from. The
answer's own text is arbitrary in meaning and exact in behaviour: a function of
the two answers alone. A tie is also **reported**, because two approvals at one
point is a question about the project's governance that the corpus cannot
answer.

Every contested source is named where the absorption is reported (the source,
each answer, and the point that approved it), because both candidates are real
translations a reader approved, so no gate tells them apart on quality and a
count alone leaves nobody able to look at the disagreement.

### Version chains and governed reuse

The corpus accumulates versions. A block whose source is rewritten writes a new
entry beside the one that came before, because the key is the text and the text
moved. `Entry.Unit` is what says the two are successive answers for one block:
the framework's own durable block identity (`model.Block.Unit`), matched by
reconciliation rather than named, so it survives an edit that rewrites the
source and a reorder that moves it. It is the same key a decision is filed
under. A version chain is therefore a query rather than a new store:
`memory.VersionReader.Versions` returns a block's prior answers, newest first,
narrowed to one point or across every point the block has sat at.

A chain is not a matcher. A lookup asks *what has been approved for something
like this, near here* and ranks candidates; a chain asks *what did this block
say before*, and the answer carries no score. Ranking it would invite a caller
to treat a prior version as a match, which is what fuzzy matching gets wrong.

A prior answer is only reusable under the rules it was approved under, and the
gate is inside the lookup rather than in front of it.
`memory/leverage.PriorVersionFor` returns the last answer approved for a block,
and the source it was approved for, only when that answer's
`ContextFingerprint` equals the fingerprint in force now; an empty fingerprint on
either side is refusal rather than agreement. A gate a caller applies is a gate a
caller forgets, and the failure is silent: a translation steered by wording
approved under superseded rules, stamped with today's fingerprint and looking
fresh. The pair is returned, source and target, because either half alone is
worse than neither: a target with no source is an anchor with no explanation,
and a source with no target teaches wording that must not be reused.

The record absorber ([C-04](c-04-unit-state-and-decisions.md)) writes both
fields as it learns a committed target: the block's identity, and the governing
context the answer stands under. The context is read from the target's own
stamp where the format keeps one, and otherwise from the decision record for
the unit and locale, which is where a JSON catalog's or a `.properties` file's
translations have it recorded: the context the decider approved the unit under,
or the producer's stamp the loop recorded when it wrote the target. The `.kmb`
bundle carries `point`, `unit` and each origin's `contextFp`, so a seed
exported and re-imported keeps its chain and its governance. Compiling a bundle
into the store also writes each entry's fingerprint onto the record row for the
unit it answers, where the row holds none and is about that translation, so the
absorber that runs after the compile finds it there. Nothing backfills a unit
onto entries approved before chains existed; those entries answer lookups as
before and belong to no chain, and an answer produced under no governing
context, or before anything recorded one, carries no fingerprint.

**One interface for producers.** `core/memory.Provider` is everything a producer
may ask a content memory, in two methods with struct requests: `Lookup` (what is
approved for this content, at this point, above this score, verbatim or
adapted) and `PriorVersion` (what this block said before, under this
governance). A provider that cannot answer returns false rather than omitting
the method, so a caller cannot tell a store that keeps no chains from a block
that has no history, and should not behave differently on them.
`memory/leverage` is the one adapter from the `memory` package to that contract,
and `core/memory.NullProvider` answers nothing for a run with no corpus. A host
grants a corpus through one config key, `core/memory.ConfigKey`, to every tool
whose schema declares it: `recycle` *requires* a memory, and `translate`
*accepts* one (`schema.Accepts`) and still builds in a project with no store.

The `translate` tool offers a block's prior answer to the model as reference,
gated as above, under `reuse: prior` (the default); `reuse: none` sends no
reference, which is what a run wants when it is re-translating everything
after a voice change the old wording should not anchor, or does not want to
pay for the lookup. The reference is per block and travels beside the block in a batch,
never in the shared preamble, so one block's history is not offered to another.
It is part of the prompt's context digest, so a translation cached under one
prior version is not served after the chain moves.

### The lookup interface

```go
type ContentMemory interface {
    Add(ctx context.Context, entry Entry) error
    Lookup(ctx context.Context, source *model.Block, sourceLocale, targetLocale model.LocaleID,
        opts LookupOptions) ([]Match, error)
    LookupSegment(ctx context.Context, source *model.Block, segmentIdx int,
        sourceLocale, targetLocale model.LocaleID, opts LookupOptions) ([]Match, error)
    LookupText(ctx context.Context, source string, sourceLocale, targetLocale model.LocaleID,
        opts LookupOptions) ([]Match, error)
    Delete(ctx context.Context, id string) error
    Count(ctx context.Context) (int, error)
    Close() error
}
```

`Lookup` takes a `*model.Block` rather than a string. The block carries the
entity annotations needed to compute the generalized key and the inline-code runs
needed for the structural key, so no separate pre-processing step is required. By
default it keys on the block's whole content, the verbatim case when no
segmentation overlay is present. `LookupSegment` keys on a single segment span
for the sentence-level leverage path
([M-01](../multilingual/m-01-bilingual-interop.md)). `LookupText` is the
flattened form, for entries a block form cannot reach; a producer uses it after
the block form finds nothing. A store that keeps version chains also implements
`VersionReader`.

### The memory is state, not source

The content memory is **accumulated machine memory** (every translated segment
becomes leverage for the next), not authored, reviewed content. So unlike the
terms source ([C-08](c-08-terms.md)), which is *source*, the memory is **state,
kept out of version control**:

- its home is a store outside the working tree: locally the memory tables inside
  `.kapi/work/store.db` ([C-03](c-03-context-store-and-graph.md)); in CI whatever
  the job restores; and a shared backend where a team needs one authoritative,
  accumulating store. One continuum, larger backend. The `memory/schema` package
  carries the table definitions in a second SQL dialect beside SQLite's, so such
  a backend builds the same tables from the same declaration.
- it is **rebuildable**, which makes it softer than most machine state: the
  leverage reconstructs from the committed translations plus the human-curated,
  **read-only** committed seeds. A cold or clobbered store is a performance hit,
  not data loss.
- because it is additive and rebuildable it needs **no locking**: it tolerates
  last-write-wins and per-branch cache keys.

Consequently **CI never commits the memory**: it builds the tables from the
committed sources, leverages and accumulates during the run, and discards them.
The translation *output* is what is committed. This is why a new terms source
arriving while a memory is in play is not a reconciliation problem: no store
lives in version control to conflict.

A project accumulates **many** memory bundles, not one (one per content surface
under `.kapi/memory/*.memory.json`), so the suffix, not the location, identifies
a seed. That is why the memory has no conventional-location fallback where the
terms source does: a conventional single name would force a project with a bundle
per surface to nominate one of them arbitrarily. A seed is named by
`defaults.memory_source` or by an explicit path; there is nothing sensible to
guess.

### The rebuild, in two stages

Both stages run on the read path, keyed by content digest, so an unchanged input
costs a read and no writes:

1. **The seeds**: every committed bundle under `.kapi/memory/`, not only the
   primary one bound by `defaults.memory_source`.
2. **The committed translations**: each collection's per-locale target document,
   paired with its source through the collection's own binding (the same reader
   and format config on both sides) and absorbed as source→target pairs.

The second stage is the half that carries wording no bundle holds. A translation
approved somewhere reaches version control as the target artifact, and without
reading it back the reviewed wording lives in exactly one place nothing in the
pipeline can see. The absorption reports what it did: pairs seen, documents read,
pairs learned, pairs reconciled, pairs contested, pairs refused.

The record is absorbed **after** the seeds, so on the pass that compiles both (a
fresh checkout) the committed translation supersedes the accelerant. Afterwards
each input has had its say and the digest decides: only an artifact whose bytes
moved is applied again, which is what lets a later seed edit, a pulled decision
or an approval stand rather than being overwritten by an unchanged file. A run
stamps the targets it writes itself, so a convergence never reads its own output
back as if a person had committed it.

Two rules keep the absorbed record honest. A pair whose target does not carry its
source's inline codes is refused rather than stored, the same predicate
`recycle` fills by. And where the record answers one source string more than one
way, each answer keeps an entry of its own under the point that approved it, so
the fill resolves between them by asking from where it is (see [Where an answer
was approved](#where-an-answer-was-approved)). A source answered the same way
wherever it appears stays one entry bound to no point: an answer every point
agrees on is not a decision about a place, and giving it one would put a copy of
it in the store per collection that carries the string.

Because the absorber's entries are reproducible from the committed translations
by construction (that is what this stage does), a store written under an older
entry identity is **re-learned rather than migrated**: the pass forgets the
entries it minted and the stamps that would let it skip a document, and reads the
record again. What the corpus learned elsewhere is not the record's to forget.

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

The trigram query is constructed differently for multi-word Latin text (an OR of
quoted substrings) and for single-word or CJK text, where overlapping character
windows are sampled at even intervals.

### Unicode normalization

Every matching key passes through Unicode NFC normalization before whitespace
normalization. This handles the real edge cases: Arabic tashkeel as separate
characters versus combined, Hangul jamo versus composed syllables, and accented
Latin composed two ways.

### TMX interchange

The memory imports and exports TMX for interchange with external tooling, mapping
the inline elements onto run kinds: `<ph>` to a placeholder run, `<bpt>`/`<ept>`
to the open and close halves of a paired code. Entity metadata travels as
properties on the translation unit. Legacy plain-text imports produce entries
whose variants are a single text run with no entity mappings, and participate in
plain matching only.

### Pipeline integration

The `recycle` tool is a translate-class tool
([E-03](../engine/e-03-tool-system.md)): it reads each block's source, queries the
memory at the run's point, and where a match clears the fill threshold writes the
target. It records the outcome on the block's properties (the match score and
match type), which downstream tools read as context, so a translate step can
skip what the memory already filled at a high score. A filled target is stamped
with the context governing the collection now, so a recycled target falls stale
when that governance moves exactly as a freshly translated one does.

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

- The memory stores rich content (run sequences with inline-code runs and entity
  metadata), not flat strings.
- One source string can carry a different reviewed answer per collection, and a
  fill that knows where it is gets the local one. A rebuild therefore reproduces
  the wording it started from instead of moving with the corpus.
- A block's previous answer reaches the model as reference, and only under the
  governance it was approved under.
- Every producer reaches the corpus through one contract, so a second tool
  cannot reach it by a side channel the first one did not.
- Generalized matching turns entity variation from a fuzzy penalty into an exact
  match at the top tier.
- Inline codes survive storage and matching, reducing manual tag reinsertion.
- Matching on blocks rather than strings makes the memory a streaming pipeline
  stage that composes with other tools.
- Trigram candidate retrieval keeps fuzzy lookup fast at scale.
- The pure-Go SQLite driver preserves cross-compilation and the single-binary
  distribution goal.

## See also

- [C-02: Coordinates and governance](c-02-coordinates-and-governance.md): the
  context space an approval point names, and the resolver that answers it.
- [C-04: Unit state and the decision record](c-04-unit-state-and-decisions.md):
  the state carrier this store is not, and the absorber that teaches it.
- [C-05: Freshness and the composite ref](c-05-freshness.md): the context
  fingerprint a prior answer is gated on.
- [C-08: Terms](c-08-terms.md): shared matching infrastructure and the
  source-versus-state contrast.
- [F-02: Content Model](../foundations/f-02-content-model.md): run
  sequences, inline-code runs, entity annotations.
- [Memory matching algorithm](../../implementation/context/memory-matching-algorithm.md):
  trigram construction, the schema, the TMX element mapping.
