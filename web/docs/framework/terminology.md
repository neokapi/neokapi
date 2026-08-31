---
sidebar_position: 11
title: Terminology
description: "neokapi's concept-oriented terminology system groups multi-locale terms under language-neutral concepts with lifecycle status and grammatical metadata, backing the kapi terms commands and the term-check pipeline tool."
keywords: [terminology, terms, TBX, concepts, term enforcement, quality checks]
---

import { PipelineDiagram, StreamDiagram } from "@neokapi/docs-shared";

# Terminology

neokapi manages terminology with a concept-oriented model inspired by the TBX
(TermBase eXchange) standard: language-neutral concepts group multi-locale
terms, each carrying a lifecycle status and optional grammatical metadata. The
same model backs the `kapi terms` commands, the `term-check`, `dnt-check` and
`term-extract` pipeline tools, and the `terms/` Go library.

## Concept-oriented model

A **concept** is a language-neutral knowledge unit. It carries a domain and a
definition, and groups **terms** across locales. Each term has a lifecycle
status, and a locale may hold several terms (a preferred form plus admitted
variants).

<StreamDiagram
  title='Concept: "cloud storage"'
  ariaLabel='Concept "cloud storage": its domain, its definition, and its terms in English, French, German and Japanese'
  items={[
    { kind: "Domain", detail: '"infrastructure"', role: "meta" },
    { kind: "Definition", detail: '"Remote file storage accessed via internet"', role: "meta" },
    { kind: "Term · en", detail: '"cloud storage"', role: "block", note: "preferred" },
    { kind: "Term · fr", detail: '"stockage cloud"', role: "block", note: "preferred" },
    { kind: "Term · fr", detail: '"stockage en nuage"', role: "block", note: "admitted" },
    { kind: "Term · de", detail: '"Cloud-Speicher"', role: "block", note: "preferred" },
    { kind: "Term · ja", detail: '"クラウドストレージ"', role: "block", note: "preferred" },
  ]}
/>

This differs from a flat list of source→target pairs and is what enables
multiple terms per locale, status-driven enforcement, and rich metadata
attached to a single language-neutral concept.

### Term lifecycle statuses

| Status       | Meaning                       | Usage                           |
| ------------ | ----------------------------- | ------------------------------- |
| `preferred`  | The recommended term          | Always suggest to translators   |
| `approved`   | Accepted for use              | Valid alternative               |
| `admitted`   | Allowed but not recommended   | Show with lower priority        |
| `deprecated` | Being phased out              | Warn when found in translations |
| `proposed`   | Under review, not yet approved | Show as suggestion with caveat |
| `forbidden`  | Must not be used              | Flag as error in QA             |

## Concept relations

Concepts are not islands. A terms store persists typed, directed **relations**
between concepts, so a renamed product points at its replacement and a
deprecated term points at the one to use instead. The relation vocabulary is
aligned with [SKOS](https://www.w3.org/2004/02/skos/):

| Category        | Labels                          | Meaning                              |
| --------------- | ------------------------------- | ------------------------------------ |
| Hierarchy       | `broader`, `narrower`           | A parent/child concept relationship  |
| Composition     | `part-of`, `has-part`           | A whole/component relationship       |
| Association     | `related`                       | A non-hierarchical association       |
| Succession      | `replaced-by`                   | A concept superseded by another      |
| Guidance        | `use-instead`                   | A discouraged term points at a preferred one |
| Cross-scheme    | `exact-match`, `close-match`    | Equivalence across schemes           |
| Stance          | `competitor`                    | A competitor's term                  |

Each edge is a concept pointing at another under one of those labels:

<PipelineDiagram
  channelLabel="use-instead"
  stages={[
    { label: "utilize", sub: "forbidden", role: "qa" },
    { label: "use", sub: "preferred", role: "io" },
  ]}
/>

<PipelineDiagram
  channelLabel="replaced-by"
  stages={[
    { label: "web store", sub: "deprecated", role: "qa" },
    { label: "marketplace", sub: "preferred", role: "io" },
  ]}
/>

<PipelineDiagram
  channelLabel="broader"
  stages={[{ label: "cloud storage" }, { label: "infrastructure" }]}
/>

A relation is a first-class record with an ID, a source and target concept, a
type from the vocabulary above, an optional note, and an optional validity
(below). The terms store validates that the type is known and that both concepts
exist before persisting an edge.

### Relation and term validity

A relation, and an individual term, may carry a **validity**: a half-open time
interval `[valid-from, valid-to)` plus a set of free-form tags. A query supplies
a **scope** (a point in time and a set of tags) and only edges and terms whose
validity matches the scope are returned. A nil validity is unbounded (it matches
every scope); a nil scope applies no filtering.

This makes the same terms store answer scope-dependent questions: which terms were
preferred *as of* last quarter, or which relations hold *within* a given market.
Tags are open-ended (the framework assigns them no meaning); a caller chooses a
tag vocabulary (for example a `market` key) and uses it consistently.

### Status transitions

A term's status changes over its lifetime. `ValidateTransition(from, to)`
accepts any transition between known statuses (history is the guard), while
`IsGovernedTransition(from, to)` reports whether a change is
consequential enough to deserve review: any transition **to** `forbidden` or
`preferred`, or any transition **from** `forbidden`. The framework only
classifies; a platform built on it decides what governance a governed transition
requires.

## Storage backends

Two backends ship in the `terms/` package, both thread-safe
(RWMutex-protected) and implementing the full `Terminology` interface:

1. **In-memory** (`terms.NewInMemoryStore`): fast and ephemeral, used
   for session-scoped batch processing.
2. **SQLite** (`terms.NewSQLiteStore`): persistent file-based storage
   for CLI workflows, with fuzzy matching via SQL-based Levenshtein distance.

The `Terminology` interface also accommodates server-side backends for multi-user
deployments with project scoping, terminology streams, and workspace isolation.

## CLI usage

### Resource location

All `kapi terms` commands (except `list`) accept these mutually exclusive flags:

| Flag            | Resolves to                         | Example                      |
| --------------- | ----------------------------------- | ---------------------------- |
| `--name <n>`    | `~/.config/kapi/terms/<n>.db`   | `--name project-terms`       |
| `--local`       | `./terms.db` (current directory) | `--local`                    |
| `--file <path>` | Explicit file path                  | `--file /shared/terms.db`    |
| _(no flag)_     | Same as `--local`                   |                              |

Databases are created on demand if they don't exist. `terms/` and `terms.db`
are the on-disk names for a named or loose terms store. `~/.config/kapi` is the
user config directory on Linux, and resolves to
`~/Library/Application Support/kapi` on macOS. `kapi config path` prints the
resolved location.

With no flag inside a project, the terms store is instead a set of tables in the
project's `.kapi/work/store.db`, compiled from the committed bundle the recipe binds
with `defaults.terms_source`; see
[Memory & terms storage](/kapi/recipes/memory-and-terms-storage).

```bash
# Import terms
kapi terms import terms.json --name project-terms

# Export terms
kapi terms export --name project-terms --format bundle -o terms.json

# Look up a term (exact, or --fuzzy)
kapi terms lookup "encryption" --name project-terms -s en -t fr
kapi terms lookup "authenticating users" -s en -t fr --fuzzy

# Search concepts, view statistics, list named terms stores
kapi terms search "auth" -s en --limit 50
kapi terms stats --name project-terms
kapi terms list

# Inside a project: where the extracted content uses a term or a concept
kapi terms occurrences "content memory"
kapi terms occurrences c-dashboard --locale nb --collection docs
```

The `kapi terms` commands cover import, export, lookup, search, occurrences,
statistics, and listing. Inside a project, a term decision also lands through
the one write verb: a `kapi apply` entry with `kind:"term"` (`op`, `term`,
`locale`, `status`, `replaces`) is written to the committed terms source the
recipe binds with `defaults.terms_source` and compiled into the project store,
so `git diff` is the review surface. Concept **relations** are authored
visually rather than from the command line: Kapi Desktop opens a per-concept
dashboard (the `@neokapi/concept-ui` component, which shows a concept's terms,
geography, constraints, a local relations widget, and a timeline) over a local
terms store, where an editor adds, retypes, scopes, and removes edges directly.
The relation data this produces is the same `ConceptRelation` records
persisted by the terms store and read through the [Go API](#go-library) below.

## Pipeline integration

Three pipeline tools bring terminology into the flow:

- **`term-check`** scans each Block's source text for the terms a store or a
  rule list carries, attaches the matches as `term` annotations (source term,
  target suggestions, positions, status), and, where a target exists, checks
  that it uses the expected rendering. Violations are reported as findings with
  expected-versus-actual detail, and the tool exits non-zero on them so it
  doubles as a gate. It takes its rules from the project's terms store
  (`--termstore` outside a project) or from `term_rules:` in a flow step's
  config: the same shape a voice profile's vocabulary uses (one `term`, its
  `replacement`, a `severity`, and optionally a `concept_id` that ties the rule
  to a concept here). A rule whose `severity` is `minor` warns; any other value,
  including unset, fails, because a rule resolved from a store carries no
  severity and must not be silently downgraded.
- **`dnt-check`** (do-not-translate) fails a target where a term listed under
  `--terms` (product names, trademarks, code identifiers) was translated,
  transliterated or dropped. The translate step masks those spans so the model
  never sees them.
- **`term-extract`** surfaces candidate terms from the source, for a curator to
  approve into the store.

Two further stages exist only in the Go library: `term-lookup` and
`term-enforce` (`terms/tool.go`, `NewTermLookupTool` and `NewTermEnforceTool`).
`term-lookup` scans a Block's source text and attaches the matches as
`TermAnnotation` entries; `term-enforce` checks a translated block against the
expected terminology and records the outcome as block properties
(`term-enforce-passed`, `term-enforce-errors`, `term-enforce-violations`).
Neither is a registered tool: a recipe cannot name them and `kapi tools` does
not list them. The runner appends both automatically behind any registered tool
whose schema declares that it requires terms, whenever a terms store is open,
so a flow step gets the lookup and the enforcement without naming either.

## Go library

### Interface

```go
type Terminology interface {
    AddConcept(concept Concept) error
    GetConcept(id string) (Concept, bool)
    DeleteConcept(id string) error
    Lookup(sourceText string, opts LookupOptions) []TermMatch
    LookupAll(sourceText string, opts LookupOptions) []TermMatch
    Search(query string, sourceLocale, targetLocale model.LocaleID, offset, limit int) ([]Concept, int)

    // Relations between concepts, optionally validity-scoped.
    AddRelation(rel ConceptRelation) error
    DeleteRelation(id string) error
    RelationsOf(conceptID string, scope *graph.Scope) []ConceptRelation // both directions
    ListRelations(scope *graph.Scope) []ConceptRelation

    Count() int
    Concepts() []Concept
    Close() error
}
```

(Every method, and the import/export helpers below, takes a `context.Context`
as its first argument and returns an `error` as its last in the real API; both
are elided here for readability.)

`Lookup` finds the best match for a single term. `LookupAll` scans running text
and returns every term occurrence with positions; this is what powers the
`term-check` tool, the `term-lookup` library stage, and editor suggestions. By
default `LookupAll` matches
case-insensitively (terminology should be recognized regardless of
capitalization); set `CaseSensitive` to override.

### Key types

```go
type Concept struct {
    ID         string
    Domain     string            // subject area (security, ui, marketing)
    Definition string            // language-neutral description
    Terms      []Term
    Properties map[string]string // extensible metadata
    CreatedAt  time.Time
    UpdatedAt  time.Time
}

type Term struct {
    Text         string
    Locale       model.LocaleID
    Status       model.TermStatus // preferred, approved, admitted, deprecated, proposed, forbidden
    PartOfSpeech string
    Gender       string
    Note         string
    Validity     *graph.Validity // optional time + tag scope (nil = unbounded)
}

type ConceptRelation struct {
    ID           string
    SourceID     string
    TargetID     string
    RelationType string          // a SKOS-aligned label: broader, use-instead, replaced-by, …
    Note         string
    Validity     *graph.Validity // optional time + tag scope (nil = unbounded)
    CreatedAt    time.Time
}

type TermMatch struct {
    Concept   Concept
    Term      Term                // the matched source term
    Score     float64             // 0.0-1.0
    MatchType model.MatchStrategy // exact, normalized, fuzzy
    Position  model.TextRange     // position in source text
}

type LookupOptions struct {
    SourceLocale  model.LocaleID
    TargetLocale  model.LocaleID
    CaseSensitive bool
    MinScore      float64             // minimum fuzzy score (default 0.8)
    MatchModes    []model.MatchStrategy
    Domains       []string            // restrict to specific domains
    StatusFilter  []model.TermStatus  // only return terms with these statuses
}
```

`Concept` helpers: `SourceTerm(locale)`, `TargetTerms(locale)`,
`PreferredTerm(locale)`.

### Example

```go
package main

import (
    "fmt"

    "github.com/neokapi/neokapi/core/model"
    "github.com/neokapi/neokapi/terms"
)

func main() {
    tb := terms.NewInMemoryStore()
    defer tb.Close()

    tb.AddConcept(terms.Concept{
        ID:         "c1",
        Domain:     "security",
        Definition: "Process of encoding information",
        Terms: []terms.Term{
            {Text: "encryption", Locale: "en", Status: model.TermPreferred},
            {Text: "chiffrement", Locale: "fr", Status: model.TermPreferred},
        },
    })

    matches := tb.LookupAll(
        "The encryption module handles end-to-end encryption",
        terms.LookupOptions{SourceLocale: "en", TargetLocale: "fr"},
    )
    for _, m := range matches {
        fmt.Printf("Found %q at [%d:%d] → %s (%s)\n",
            m.Term.Text, m.Position.Start, m.Position.End,
            m.Concept.TargetTerms("fr")[0].Text, m.Term.Status)
    }
}
```

### Import / export

```go
// JSON preserves the full concept-oriented structure
count, err := terms.ImportJSON(tb, reader)
err = terms.ExportJSON(tb, writer, "Project Terms")

// CSV is a flat source/target form with optional metadata
opts := terms.CSVImportOptions{
    SourceLocale: "en", TargetLocale: "fr", Domain: "general", HasHeader: true,
}
count, err = terms.ImportCSV(tb, reader, opts)
err = terms.ExportCSV(tb, writer, "en", "fr", true)
```

CSV columns are `source,target,domain` (domain optional). JSON carries the full
concept structure:

```json
{
  "name": "Project Terms",
  "version": "1.0",
  "concepts": [
    {
      "id": "c1",
      "domain": "security",
      "definition": "Encryption where only endpoints can decrypt",
      "terms": [
        { "text": "end-to-end encryption", "locale": "en", "status": "preferred" },
        { "text": "chiffrement de bout en bout", "locale": "fr", "status": "preferred" }
      ]
    }
  ]
}
```

## Terminology and content memory

Terminology and [content memory](/framework/content-memory) are
deliberately separate systems because they answer different questions:

- **Content memory**: "How was this sentence rendered before?" (segment pairs).
- **Terminology**: "What is the correct term for this concept?" (multi-locale
  knowledge units).

They share the `Block` annotation system as their integration point, so both
memory matches and term matches are available to any downstream tool or editor.

Terminology and [segmentation](/framework/segmentation) are run-anchored overlays
produced in the [content-preparation](/framework/content-preparation) pass that
readies a source before translation.
