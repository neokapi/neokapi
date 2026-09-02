---
sidebar_position: 2
title: "Terminology Data Model"
description: Implementation note for C-08. The Go struct layout for Concept, Term, and their context dimensions, the Terminology interface, the SQLite terms store, the pipeline tools, and the import pipeline from TBX and CSV sources.
keywords: [terminology data model, Concept, Term, SQLite, TBX import, CSV, term rules, implementation note, neokapi]
---

# Terminology Data Model

This note provides implementation details for [C-08](/contribute/architecture/context/c-08-terms).

## Data Model: Concept-Oriented

The core data model is concept-oriented, following TBX principles. A Concept groups terms across languages, each with context dimensions:

```go
type Term struct {
    Text           string           // the term text
    Locale         model.LocaleID   // language/locale
    Status         model.TermStatus // lifecycle status (proposed, approved, preferred,
                                    // admitted, deprecated, forbidden)
    PartOfSpeech   string           // noun, verb, adjective, etc.
    Gender         string           // grammatical gender (if applicable)
    Note           string           // usage note or context
    CompetitorTerm bool             // true if this is a competitor's term
    Validity       *graph.Validity  // time/tag scoping; nil = always valid
}

type Concept struct {
    ID             string            // unique concept identifier
    ProjectID      string            // project scope (empty = workspace-scoped)
    Domain         string            // subject field (software, medical, legal, etc.)
    Definition     string            // language-neutral definition
    Source         TermSource        // "terminology" or "brand_vocabulary"
    Terms          []Term            // terms across locales
    DoNotTranslate bool              // the source term travels into every target unchanged
    Properties     map[string]string // extensible metadata
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

`TermSource` distinguishes traditional terminology
(`TermSourceTerminology`) from voice vocabulary (`TermSourceBrandVocabulary`,
whose persisted value is `"brand_vocabulary"`), so the two populations can share
one terms store while staying filterable.

`DoNotTranslate` marks a concept whose source term is the same string in every
locale (a product name, a trademark, a format acronym). It is independent of
whether a target term exists, and it reaches the tools as a `TermRule` with
`DoNotTranslate` set.

Progressive disclosure: CSV import auto-creates Concepts with a single preferred Term per locale, so nothing more is required of a user who wants a word list.

## Terminology Interface

```go
type Terminology interface {
    AddConcept(ctx context.Context, concept Concept) error
    GetConcept(ctx context.Context, id string) (Concept, bool, error)
    DeleteConcept(ctx context.Context, id string) error
    Lookup(ctx context.Context, sourceText string, opts LookupOptions) ([]TermMatch, error)
    LookupAll(ctx context.Context, sourceText string, opts LookupOptions) ([]TermMatch, error)
    Search(ctx context.Context, query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]Concept, int, error)
    Count(ctx context.Context) (int, error)
    Concepts(ctx context.Context) ([]Concept, error)

    AddRelation(ctx context.Context, rel ConceptRelation) error
    DeleteRelation(ctx context.Context, id string) error
    RelationsOf(ctx context.Context, conceptID string, scope *graph.Scope) ([]ConceptRelation, error)
    ListRelations(ctx context.Context, scope *graph.Scope) ([]ConceptRelation, error)

    Close() error
}
```

Import and export are standalone functions rather than interface methods:
`ImportJSON`/`ExportJSON`, `ImportCSV`/`ExportCSV`, and `ImportTBX`/`ExportTBX`
(the ISO TBX interchange format, with `TBXImportOptions`/`TBXExportOptions`).
Framework backends: in-memory (CLI batch) and SQLite (persistent). A backend
with wider isolation can be supplied behind the same interface.

`TermMatch` is what a lookup returns: the concept, the matched term, a score, a
match strategy, and a `Position model.TextRange` (a character range into the
searched text).

## Fuzzy Matching and Search

Term lookup uses a tiered matching pipeline: exact, then normalized, then fuzzy. Fuzzy matching uses trigram-based candidate retrieval to avoid full table scans:

- **SQLite**: a contentless FTS5 `trigram` index (`tb_terms_trigram`) over the `text_lower` column, populated by insert/update/delete triggers. Falls back to length-based pre-filtering if FTS5 is unavailable.

Character-level Levenshtein scoring (on `[]rune`) is applied to ~200 trigram candidates. This is correct for all scripts including CJK (each character is a morpheme).

UI search uses the same FTS5 `trigram` index for substring matching on term text, ranked.

Text normalization applies Unicode NFC (`golang.org/x/text/unicode/norm`) via `NormalizeTerm()` before comparison, handling Arabic diacritics, Hangul jamo composition, and accented Latin characters.

The `terms/schema` package declares the tables once and emits them in two SQL dialects. The second dialect's equivalent of the FTS index is a `pg_trgm` GIN index on `text_lower` and a `search_tsv` column; the framework ships only the SQLite backend.

## Locating declared terms

`terms.Locate(ctx, LocateRequest)` is the one pass that finds every declared
term in a text. A `LocateRequest` carries the text and its runs, the
`profile.TermRuleSet`s the caller holds (a voice profile's vocabulary through
`profile.VocabularyRuleSets`, a tool's `term_rules:` as its own set, or both),
the bound `Terminology` store, the locale, and the domains, minimum score and
validity scope passed through to the store lookup. It returns `Occurrence`s:
the matched text, the rule or concept that declared it, its status or severity,
whether it is do-not-translate, byte offsets into the text, and a
`model.Anchor` into the runs (`model.RangeAnchorForBytes`). Rule hits come
first, then store matches, deduped across the candidate languages. An
occurrence is a use, not a verdict; the consuming gate decides which uses are
violations.

## Pipeline Tools

The terminology tools run as ordinary pipeline stages ([E-03](/contribute/architecture/engine/e-03-tool-system)). Every governed step takes its rules under one key, `term_rules:`, as `[]profile.TermRule`:

**`term-lookup`** (Enrich). Runs `terms.Locate` over the source with the store and the step's `term_rules:`, and attaches a `TermAnnotation` per occurrence as an overlay span whose `Range` is the occurrence's `model.Anchor`. Downstream tools (AI translate, checks) use these annotations for context.

**`term-check`** (Validate). Holds the target to the renderings its `term_rules:` mandate. Both sides match on containment rather than word boundaries, so an inflected form of a lemma counts; a rule's severity sorts a violation into `term-check-errors` or `term-check-warnings`, and a rule with no replacement is skipped.

**`term-enforce`** (Validate). For each source term the store knows, checks that an acceptable target-locale rendering is present and reports the block where it is missing. A forbidden or deprecated source concept is redirected through its `use-instead` or `replaced-by` relation (`resolveReplacement`), so the expected rendering is the replacement concept's preferred term.

**`dnt-check`** (Validate). Checks that do-not-translate terms survive verbatim into the target. `DNTCheckConfig.EffectiveTerms` unions the strings named directly with every rule in `term_rules:` marked `DoNotTranslate`, which is how the store's do-not-translate concepts reach it; a store is not required.

Related AI and redaction tools (registered in `core/ai/tools/` and
`core/tools/`):

**`term-extract`** (Enrich, AI). LLM extraction of candidate terms. Uses an AI provider from [E-07](/contribute/architecture/engine/e-07-model-providers).

**`entity-extract`** (Enrich, AI). Named entity annotation (people, organizations, products, dates, locations). Serves multiple purposes: content-memory generalization ([C-09](/contribute/architecture/context/c-09-content-memory)), do-not-translate markers, translation hints, and terminology candidate discovery. Should run early in the pipeline, before `recycle`.

**`redact`** (Transform). Privacy tool replacing entity values with typed placeholders (e.g., "John" -> `\{PERSON\}`) before external services. See [C-10](/contribute/architecture/context/c-10-redaction).

**`unredact`** (Transform). Restores original entity values after external processing. Paired with `redact`:
`reader -> entity-extract -> redact -> [external MT] -> unredact -> writer`

## Concept relations

Concepts are linked by persisted, typed, directed edges. A `ConceptRelation`
records the edge with an identity, an optional note, and an optional validity:

```go
type ConceptRelation struct {
    ID           string          // edge identity (caller-assigned; required)
    SourceID     string          // origin concept ID
    TargetID     string          // target concept ID
    RelationType string          // a graph.Label* constant
    Note         string          // optional human note
    Validity     *graph.Validity // optional time + tag scope (nil = unbounded)
    CreatedAt    time.Time
}
```

`RelationType` draws its values from the `graph.Label*` constants, so relation
edges share the vocabulary used by the rest of the graph layer.
`KnownRelationType` and `ValidateRelation` reject an unknown type or a missing
ID before a write. The relation methods on `Terminology` (above) persist and
query the edges: `AddRelation` upserts by ID, `RelationsOf` returns both
directions, and both read methods filter by scope when one is given.

## Temporal and tag validity

A `Term` and a `ConceptRelation` each carry an optional `*graph.Validity`: a
half-open `[ValidFrom, ValidTo)` interval plus a `map[string]string` of tags.
`LookupOptions.Scope` and the relation read methods take a `*graph.Scope` (a
time plus tags); a term or edge is returned only when its validity matches the
scope (a nil validity always matches; a nil scope filters nothing). Tags are
open-ended: a caller picks a vocabulary, such as a `market` key.

## Status transitions

`ValidateTransition(from, to model.TermStatus) error` accepts any transition
between known statuses (it rejects only unknown statuses), and
`IsGovernedTransition(from, to) bool` reports whether a transition is
consequential: any transition to `forbidden` or `preferred`, or from
`forbidden`. The framework classifies; it imposes no review workflow.

## Content model extensions

- `TermAnnotation`: matched term with concept, target terms, status, score and
  match type; its position is the `Range` of the overlay span that carries it.
- `EntityAnnotation`: named entity with type and do-not-translate flag.

These join `AltTranslation` as first-class annotations on Blocks ([F-02](/contribute/architecture/foundations/f-02-content-model)).
