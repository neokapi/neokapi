---
id: 010-terminology
sidebar_position: 10
title: "AD-010: Terminology"
description: "Architecture decision: neokapi's terminology system is concept-oriented, grouping multi-locale terms under language-neutral Concepts with per-term lifecycle status, part-of-speech, and domain metadata."
keywords: [terminology, Concept, TBX, terms store, concept-oriented, architecture decision, neokapi]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# AD-010: Terminology

## Summary

neokapi's terminology system is concept-oriented: a `Concept` groups terms
across locales with per-term metadata (status, part of speech, grammatical
gender). The `Terminology` interface (`terms/` package) supports in-memory
and SQLite backends, a tiered lookup pipeline, and TBX import/export.
Terminology flows through the streaming pipeline via first-class annotation
types whose positions are run-anchored (`RunRange`) for precise inline
highlighting that survives run-preserving edits.

## Context

Terminology management ranges from flat word lists (CSV with source/target
pairs) to concept-oriented stores (TBX, MultiTerm). A flat list does not
express that "bug", "defect", and "issue" are terms for the same concept in
different contexts, nor that "bug" can be preferred in engineering docs and
deprecated in customer-facing content.

The framework needs:

- Progressive complexity — start from a CSV word list, grow into concept
  management without rewriting data.
- Pipeline integration — terminology as streaming tools, not a separate
  service.
- Precise positions — run-anchored `RunRange` spans on matched terms (run
  index + intra-run rune offset) so downstream UIs can highlight within a
  Fragment.
- Annotation semantics — do-not-translate markers for entity names, locale
  formatting hints, and pending AI-proposed candidates distinct from
  curated entries.

TBX (TermBase eXchange, ISO 30042:2019) is the universal interchange format for
concept-oriented terminological data. Native storage uses SQLite for speed and
query flexibility; TBX handles import and export only.

## Decision

### Concept-oriented data model

A `Concept` groups terms across locales, each with context:

```go
// TermSource indicates whether a concept comes from traditional
// terminology or brand vocabulary.
type TermSource string

const (
    TermSourceTerminology     TermSource = "terminology"
    TermSourceBrandVocabulary TermSource = "brand_vocabulary"
)

type Term struct {
    Text           string
    Locale         model.LocaleID
    Status         model.TermStatus // proposed, approved, preferred,
                                     // admitted, deprecated, forbidden
    PartOfSpeech   string
    Gender         string
    Note           string
    CompetitorTerm bool             // marks a competitor brand term
}

type Concept struct {
    ID         string
    ProjectID  string
    Domain     string
    Definition string
    Source     TermSource // "terminology" or "brand_vocabulary"
    Terms      []Term
    Properties map[string]string
    CreatedAt  time.Time
    UpdatedAt  time.Time
}
```

Progressive disclosure: CSV import auto-creates Concepts with a single
preferred Term per locale. No extra complexity is imposed on users who want
a flat word list.

### Terminology interface

```go
type Terminology interface {
    AddConcept(concept Concept) error
    GetConcept(id string) (Concept, bool)
    DeleteConcept(id string) error
    Lookup(sourceText string, opts LookupOptions) []TermMatch
    LookupAll(sourceText string, opts LookupOptions) []TermMatch
    Search(query string, sourceLocale, targetLocale model.LocaleID,
        offset, limit int) ([]Concept, int)
    Count() int
    Concepts() []Concept
    Close() error
}
```

Import and export are standalone functions rather than interface methods:
`ImportTBX`, `ExportTBX`, `ImportCSV`, `ExportCSV`, `ImportJSON`,
`ExportJSON`.

### Backends

- **In-memory** (`terms/inmemory.go`) — fast, ephemeral; session-scoped
  batch processing.
- **SQLite** (`terms/sqlite.go`) — persistent file-based storage for CLI
  tools. Pure Go via `modernc.org/sqlite`.

A PostgreSQL backend with workspace isolation and terminology streams can be
supplied by a platform layer behind the same `Terminology` interface.

### Terminology is source; the store is a rebuildable read-cache

Terminology is **authored content, not derived state**: a human decides which
terms are do-not-translate and what the preferred translation is, and those
decisions belong in review and version control alongside the recipe and the
brand-voice profile ([AD-022](022-brand-voice.md)). So the split is source vs.
cache, not a two-way sync:

- the committed **`.terms.json` bundle is the source** — a diff-friendly,
  reviewable, mergeable JSON document bound by the recipe's
  `defaults.terms_source`,
  edited directly (`kapi apply` with `kind:"term"` writes the file first),
  reviewed in a PR, and versioned with the code. It is plain JSON under a
  compound suffix, so a reviewer reads it in a browser diff and `jq` reads it on
  the command line.
- the **terms store (`.kapi/terms.db`) is a rebuildable read-cache** over it,
  under the gitignored cache, rebuilt when the committed bundle changes
  (content-hash guarded). Discard it, rebuild from the bundle, lose nothing —
  **nothing authoritative ever lives only in the db**. Committing the binary
  SQLite would be git-hostile (opaque, conflict-prone) and would defeat
  interchange.

A project that binds nothing still resolves, through a fallback ladder. The
ordering follows from the bundle being *committed source*, so read it that way
rather than as an exception to the "state lives in `.kapi/`" rule:
**`.kapi/` is gitignored** (this repository's own `.gitignore` carries `/.kapi/`,
and it is inside kapi's default ignore set), so a glossary kept there would never
be committed, never appear in a diff, and never reach review — which is the one
thing the terms source exists to do. The repository root therefore comes first:

1. `<root>/terms.json` — the committed glossary.
2. `<root>/.kapi/terms.json` — second, and only so a project that deliberately
   treats its glossary as local, uncommitted state still resolves.

An explicit `defaults.terms_source` wins over both. This is the same ladder
shape the brand-voice profile uses ([AD-022](022-brand-voice.md)), and the same
distinction the recipe draws in its own comments: `termbase_source` is the
committed glossary, `terms` is the gitignored cache it compiles into. Confusing
the two — treating the portable JSON *bundle* as if it were the SQLite *store* —
is what puts a glossary under `.kapi/` in the first place.

A ladder works here because **a project has exactly one glossary**. Content
memory has no equivalent convention, and deliberately so — a project accumulates
*many* memory bundles, one per content surface ([AD-009](009-content-memory.md)),
so there is no single bundle for a fallback to name. That asymmetry is a
consequence of what each store is, not an omission to be tidied away.

Read-only consumers read the committed bundle directly — the terminology check
gate decodes it without materializing the cache, which is why it holds on a fresh
CI checkout where the gitignored `.db` is absent. The cache earns its keep only
for the heavy indexed lookups (fuzzy, FTS) during translation. **CI reads the
terms; it never writes them back** — humans author them through git; a pulled new
bundle just rebuilds the read-cache, so there is nothing to reconcile.

In bowrain (server mode) terminology is managed in the platform database and
edited through the app; git is not in the loop.

> **Source, not state.** Contrast content memory
> ([AD-009](009-content-memory.md)), which is *derived state* — a rebuildable
> leverage cache kept out of git scope. Terminology is *source*: authored,
> reviewed, committed. And unlike the project *state* store
> ([AD-033](033-project-state-model.md)), whose interactive review decisions
> warrant a deferred `Pending()`/`Export` discipline, the terms source is simply
> edited-in-place and cached.

### Tiered lookup

Term lookup follows a cascading pipeline:

1. **Exact** — case-sensitive match on normalized term text.
2. **Normalized** — Unicode NFC + case folding + whitespace collapse.
3. **Fuzzy** — trigram candidate retrieval + Levenshtein scoring on the
   ~200 closest candidates.
4. **AI-assisted** (opt-in) — LLM proposes candidate term mappings that
   produce `TermCandidateAnnotation` entries for human review.

The fuzzy tier uses the same SQLite FTS5 trigram tokenizer as content memory
([AD-009: Content memory](009-content-memory.md)), keeping lookup cost
sub-linear in the size of the terms store. Text is normalized with Unicode
NFC via `NormalizeTerm()` before comparison. Character-level Levenshtein (on
`[]rune`) is correct for all scripts including CJK.

Which tiers run is selected per call through `LookupOptions.MatchModes`
(`[]model.MatchStrategy`) on `Terminology.Lookup`/`LookupAll`, alongside
`CaseSensitive`, `MinScore`, and scope filters — so a caller can request, for
example, exact-only or exact-plus-fuzzy without changing the pipeline.

### UI search

Distinct from lookup, the `Search` method powers the terms browser in
the CLI and desktop UI. It uses an FTS5 tokenizer to support substring
search ranked by match quality, rather than unranked `LIKE '%...%'`
queries.

### Annotations

Three annotation types — `TermAnnotation`, `TermCandidateAnnotation`, and
`EntityAnnotation` — implement the `Annotation` interface with run-anchored
`RunRange` positions for precise inline highlighting. (The term lookup
itself returns a character-level `TextRange` offset into the source text,
which the pipeline tool converts to a `RunRange` when it writes the
annotation onto the block.)

- **`TermAnnotation`** — a matched term from the terms store, carrying
  concept ID, target term options, status, and position.
- **`TermCandidateAnnotation`** — AI-proposed term not yet in the
  terms store. Carries a `status: proposed` marker so UI reviewers can
  accept, reject, or defer.

An **`EntityAnnotation`** type carries named entities (people,
organizations, products, dates, locations) with run-anchored `RunRange`
positions and optional DNT (do-not-translate) flags. Entity annotations
serve multiple purposes:

- Input to content-memory generalization ([AD-009: Content memory](009-content-memory.md)).
- Do-not-translate markers consumed by AI translation.
- Locale formatting hints (dates, numbers) for downstream tools.
- Terminology candidate discovery.

These annotations join `AltTranslation` as first-class annotations on
Blocks.

### Concept relations

The terms store persists typed, directed `ConceptRelation` edges between concepts.
Each edge has an ID, a source and target concept, a type drawn from the
SKOS-aligned vocabulary, an optional note, and an optional validity:

- **broader** / **narrower** — taxonomic relationships
  (`skos:broader` / `skos:narrower`).
- **part-of** / **has-part** — compositional meronymy/holonymy.
- **related** — the associative relationship (`skos:related`).
- **replaced-by** — a superseded concept points to its replacement.
- **use-instead** — a discouraged term points at the preferred one.
- **exact-match** / **close-match** — cross-scheme equivalence
  (`skos:exactMatch` / `skos:closeMatch`).
- **competitor** — a competitor's term.

`KnownRelationType` and `ValidateRelation` gate writes: a relation is rejected
unless its type is in the vocabulary and both concepts exist. The interface
exposes `AddRelation`, `DeleteRelation`, `RelationsOf` (both directions), and
`ListRelations`; the read methods take an optional `*graph.Scope` and return
only edges whose validity matches. Relations enable graph navigation in UIs and
deprecation workflows where a superseded concept's terms are flagged in new
content; the `term-enforce` tool resolves `use-instead` / `replaced-by` to name
the replacement.

### Term and relation validity

A term and a relation each carry an optional `*graph.Validity` — a half-open
`[valid-from, valid-to)` interval plus free-form tags. `LookupOptions.Scope`
and the relation read methods accept a `*graph.Scope` (a point in time plus
tags) and return only the terms and edges active at that scope. This is how the
terms store answers as-of-time and within-a-tag-scope (for example, per-market)
questions; the framework assigns tags no meaning, leaving the vocabulary to the
caller.

### Status transitions

`ValidateTransition(from, to)` accepts any transition between known statuses,
and `IsGovernedTransition(from, to)` flags the consequential ones — any
transition to `forbidden` or `preferred`, or from `forbidden`. The framework
classifies transitions; it does not impose a review workflow, leaving that to a
platform built on it.

### Competitor terms

Terms carry a `CompetitorTerm` boolean flag marking competitor brand
terms. The `brand-vocab-check` tool surfaces competitor terms found in
source text as critical-severity brand-voice findings (and forbidden terms
as major-severity), supporting brand voice governance using the store's
brand-vocabulary term source.

### Pipeline tools

The framework ships built-in terminology tools as ordinary pipeline stages:

- **`term-lookup`** (enrich) — scans source text for known terms, attaches
  `TermAnnotation` with run-anchored `RunRange` positions. Downstream tools
  (AI translate, QA) use these annotations for context.
- **`term-enforce`** (validate) — for each known source term, checks that
  an acceptable target-locale translation (preferred/approved by default,
  configurable via `CheckStatuses`) is present in the target text; flags
  blocks where the expected translation is missing. Forbidden-, deprecated-,
  and competitor-term detection is handled by `brand-vocab-check`, which
  scans source text — not `term-enforce`.
- **`term-extract`** (AI-assisted enrich) — LLM extraction of candidate
  terms with `status: proposed`. Uses a provider from
  [AD-011: AI Providers](011-ai-providers.md).
- **`entity-extract`** (AI-assisted enrich) — LLM-based named entity
  annotation (with optional NER). Should run early in the pipeline, before
  `recycle`.
- **`redact`** and **`unredact`** (transform) — pair that replaces entity
  values with typed placeholders before external services and restores
  them afterwards.

A full pipeline looks like:

<PipelineDiagram
  stages={[
    { label: "Source", sub: "binding", role: "io" },
    { label: "entity-extract", sub: "LLM/NER", role: "annotate" },
    { label: "term-lookup", role: "annotate" },
    { label: "recycle", sub: "memory", role: "translate" },
    { label: "translate", sub: "LLM/MT", role: "translate" },
    { label: "term-enforce", role: "qa" },
    { label: "Sink", sub: "binding · optional", role: "io" },
  ]}
/>

### TBX import and export

TBX (TermBase eXchange, ISO 30042:2019) is the interchange format. Import maps
TBX entries to Concepts and populates per-locale Terms. Export preserves concept
relations, term status, and context fields.

## Consequences

- Terminology is a first-class pipeline citizen, not a bolt-on
  post-processing step.
- Run-anchored annotation positions enable precise inline UI
  highlighting without re-detecting term boundaries at render time.
- Entity annotations drive both terminology extraction and content-memory
  generalization — a single annotation pass serves multiple consumers.
- Concept relations give UIs a graph substrate for browsing terminology
  without requiring a separate graph database in the framework.
- `CompetitorTerm` gives the framework a minimal hook for brand guardrails
  without depending on the full brand module.
- The same storage backends as content memory (in-memory, SQLite) keep the CLI
  dependency footprint small and cross-compilation simple.

## Related

- [AD-002: Content Model](002-content-model.md) — annotations on Blocks
- [AD-006: Tool System](006-tool-system.md) — pipeline tool pattern
- [AD-009: Content memory](009-content-memory.md) — shared
  matching infrastructure, entity annotation input
- [AD-011: AI Providers](011-ai-providers.md) — LLM-based term extraction
  and entity annotation
- [Terminology Data Model](/contribute/implementation/terminology-data-model) — full Go
  struct definitions, pipeline tool catalog, relations
