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
    Forms          []string         // other surface shapes in the term's language
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
`DoNotTranslate` set. The SQL backends keep it in the `do_not_translate` column
of `tb_concepts` (SQLite migration 5, Postgres migration 7), the bundle and the
JSON export as `do_not_translate`, the concept API and concept sync as
`do_not_translate`, and TBX as `<descrip type="x-doNotTranslate">true</descrip>`.

`Forms` are the term's other surface shapes in its own language (a plural, a
definite form, a case ending). `NormalizedConcept` trims them and drops blanks,
repeats and the term's own text before any backend writes them, so every backend
reads back the same list; `Term.Surfaces()` is the text followed by the forms,
which is what a scan matches. A single-term `Lookup` finds a term in its exact
tier by a declared form that is the whole query: each SQL backend returns the
terms with forms (`TermCandidateSource.Forms`), and `LookupTiered` keeps those
the shared matcher finds spanning the query. The SQL backends keep them as a JSON array in the
`forms` column of `tb_terms` (SQLite migration 4), the `.terms.json` bundle as a
`forms` array, and TBX as one `<termNote type="x-surfaceForm">` per form.

Progressive disclosure: CSV import auto-creates Concepts with a single preferred Term per locale, so nothing more is required of a user who wants a word list.

### Declaring and checking forms

`host.ProposeTermForms` selects the terms to expand (every language except the
source language unless `TermsExpandOptions.Locales` names some, skipping
do-not-translate concepts and terms that already declare forms), asks the
provider through `aitools.ExpandTermForms` in batches of 40 per language, and
drops a form spelled the same as another term in that language.
`host.ApplyFormsProposals` writes the survivors onto the concepts, and
`TermsFormsTarget` saves them to the bundle named on the command line, the
project's committed terms source, or the selected store, in that order.

`terms.ValidateConcepts(concepts, sourceLocale)` returns `Problem`s. Errors are a
missing or repeated concept id, a term with no text or locale, and an unknown
status. Warnings are a form spelled the same as another term in its language, and
a target term with no forms whose language `terms.LanguageInflects`, which is
false for languages whose nouns keep their written shape (Chinese, Japanese,
Korean, Vietnamese, Thai, Lao, Khmer, Burmese, Indonesian, Malay) and for
private-use and undetermined locales.

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
the matched text (for a store term, the form as the text spells it), the rule or
concept that declared it, its status or severity,
whether it is do-not-translate, byte offsets into the text, and a
`model.Anchor` into the runs (`model.RangeAnchorForBytes`). Rule hits come
first, then store matches, deduped across the candidate languages. An
occurrence is a use, not a verdict; the consuming gate decides which uses are
violations.

## Pipeline Tools

The terminology tools run as ordinary pipeline stages ([E-03](/contribute/architecture/engine/e-03-tool-system)). Every governed step takes its rules under one key, `term_rules:`, as `[]profile.TermRule`:

**`term-lookup`** (Enrich). Runs `terms.Locate` over the source with the store and the step's `term_rules:`, and attaches a `TermAnnotation` per occurrence as an overlay span whose `Range` is the occurrence's `model.Anchor`. Downstream tools (AI translate, checks) use these annotations for context.

**`translate`** (Produce, AI). Renders `term_rules:` into its prompt through two projections, because one line shape cannot carry both instructions: `profile.TermRuleMap` for the rules that name a replacement, and `profile.DoNotTranslateTerms` for the rules marked `DoNotTranslate`, which the map drops for want of one. Each is scoped to the text of the call, the renderings by `profile.ScopeTermRules` and the do-not-translate terms by `profile.ScopedDoNotTranslateTerms`, which matches containment on the raw text so a term written inside a command still reaches the prompt, as term-check demands it there. `profile.GovernanceContext` folds the unscoped projections into the context fingerprint stamped on the target, so setting or clearing the flag marks the content stale; an empty do-not-translate list contributes nothing to the hash, leaving a project that marks no concept with the fingerprints it has. The strings named by `--dnt` or the recipe are masked before the model instead (`dntMask`), a stronger guarantee that costs the batched path: a run configured with them translates one block per call.

**`term-check`** (Validate). Holds the target to the renderings its `term_rules:` require, deciding each block through `TermCheckViolations`. `terms.RulesFromConcepts` derives the rules every surface uses: a concept's head term with its forms as `Term` and `Forms`, its preferred target term with its forms as `Replacement` and `ReplacementForms`, and every other admitted, approved or unmarked target term as an entry in `Accepted`. The source side reads `check.TermText` of the block's text, which overwrites placeholders, inline code and fenced blocks, quoted kapi commands, indented example command lines up to a shell comment, and flag names byte for byte, because code keeps its words in a translation and a term written there owes no rendering whatever the rule's scope; a do-not-translate rule reads `check.PlaceholderText` instead, so a term inside a command is still demanded. It finds each rule's term with `check.FindEnglishInflectionsIn` when `SourceLocale` is English and the rule declares no forms, and with `check.FindTermFormsIn` over the term and its forms otherwise; `check.KeepLongestDeclared` then drops a term covered by a longer declared one. A demanded rule is satisfied when the target contains any of `TermRule.Renderings()` or a declared form of one. A rule marked `DoNotTranslate` names no rendering and is satisfied when `check.PlaceholderText` of the target contains, in its own casing, the term, a declared form, or the text of one of the source's occurrences (`keptVerbatim`). A rule's severity sorts a violation into `term-check-errors` or `term-check-warnings`, and any other rule with no replacement demands nothing. `TermCheckMatching` summarizes the mode for `check.Execution.TermMatching`, counting do-not-translate rules among the rules applied, and `TermCheckCanaries` builds a deleted-rendering canary and a clipped-word canary (`check.ClipWord`) for the first rule with a replacement, and a not-kept canary for the first do-not-translate rule.

**`term-enforce`** (Validate). For each source term the store knows, checks that an acceptable target-locale rendering is present and reports the block where it is missing. A forbidden or deprecated source concept is redirected through its `use-instead` or `replaced-by` relation (`resolveReplacement`), so the expected rendering is the replacement concept's preferred term.

**`dnt-check`** (Validate). Checks that do-not-translate terms survive verbatim into the target. `DNTCheckConfig.EffectiveTerms` unions the strings named directly with every rule in `term_rules:` marked `DoNotTranslate`; a store is not required. The gates, the loop checks (`host.computeLoopCheckExclusions`) and `kapi check --target` hold the store's do-not-translate concepts through `term-check`, so one decision answers for every rule the terms bind; `dnt-check` serves the strings a recipe or `--dnt` names.

Related AI and redaction tools (registered in `core/ai/tools/` and
`core/tools/`):

**`term-extract`** (Enrich, AI). LLM extraction of candidate terms. Uses an AI provider from [E-07](/contribute/architecture/engine/e-07-model-providers).

**`entity-extract`** (Enrich, AI). Named entity annotation (people, organizations, products, dates, locations). Serves multiple purposes: content-memory generalization ([C-09](/contribute/architecture/context/c-09-content-memory)), do-not-translate markers, translation hints, and terminology candidate discovery. Should run early in the pipeline, before `recycle`.

**`redact`** (Transform). Privacy tool replacing entity values with typed placeholders (e.g., "John" -> `\{PERSON\}`) before external services. See [C-10](/contribute/architecture/context/c-10-redaction).

**`unredact`** (Transform). Restores original entity values after external processing. Paired with `redact`:
`reader -> entity-extract -> redact -> [external MT] -> unredact -> writer`

## Measuring term matching

`scripts/termeval` runs term-check over reviewed translations and compares three
modes: substring matching of the term and its preferred rendering, term-check
with no declared forms, and term-check with the forms the terms store declares
plus the reviewed forms in `scripts/termeval/testdata/forms.json`. The corpora
are the project's Norwegian content memory, the compass catalogs and the
tidewatch memory, and the rules come
from `terms.RulesFromConcepts`. For each corpus, language and mode it reports the
demands, the fails, the false fails among true uses, the demands whose source
match is not a use of the term, and the constructed negatives (a rendering deleted
or clipped) the mode passes. The labels in `testdata/labels.json` were made by
agents and are pending a person's review. `go run ./scripts/termeval` prints the
table, `-forms` and `-labels` run it against another file, and
`go test ./scripts/termeval` pins that no stage mode passes a negative and that
every demand and fail is labelled.

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
