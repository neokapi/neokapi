---
id: c-08-terms
sidebar_position: 8
title: "C-08: Terms"
description: "Architecture decision: terminology is concept-oriented (a Concept groups terms across locales with per-term status, part of speech and validity), the committed JSON source is the truth while the store is a rebuildable projection, and one pass locates every declared term in a text."
keywords: [terms, terminology, Concept, TBX, terms store, concept-oriented, validity, term rules, architecture decision, neokapi]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# C-08: Terms

## Summary

Terminology is **concept-oriented**: a `Concept` groups terms across locales with
per-term metadata (status, part of speech, grammatical gender, validity). The
`Terminology` interface (`terms/`) supports in-memory and SQLite backends, a
tiered lookup pipeline, and TBX import and export.

The committed JSON source is the truth and the store is a rebuildable projection
of it. Terms flow through the streaming pipeline as first-class annotation types
whose positions are run-anchored, so a match survives run-preserving edits. One
pass, `terms.Locate`, finds every declared term in a text, whether it was
declared in a voice profile, in a tool's `term_rules:` or in the store.

## Context

Terminology management ranges from flat word lists to concept-oriented stores. A
flat list cannot express that *bug*, *defect* and *issue* are terms for one
concept in different contexts, nor that *bug* is preferred in engineering
documentation and deprecated in customer-facing content.

The framework needs progressive complexity (start from a word list, grow into
concept management without rewriting data), pipeline integration rather than a
separate service, precise run-anchored positions so a UI can highlight inside a
fragment, and annotation semantics that distinguish do-not-translate markers,
locale formatting hints, and model-proposed candidates from curated entries.

TBX (TermBase eXchange, ISO 30042:2019) is the interchange format for
concept-oriented terminological data. Native storage is SQLite for speed and
query flexibility; TBX handles import and export only.

## Decision

### The concept model

```go
type Term struct {
    Text           string
    Locale         model.LocaleID
    Status         model.TermStatus // proposed, approved, preferred,
                                    // admitted, deprecated, forbidden
    PartOfSpeech   string
    Gender         string
    Note           string
    CompetitorTerm bool
    Validity       *graph.Validity
}

type Concept struct {
    ID             string
    ProjectID      string
    Domain         string
    Definition     string
    Source         TermSource // terminology, or voice vocabulary
    Terms          []Term
    DoNotTranslate bool       // the source term travels into every target unchanged
    Properties     map[string]string
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

`DoNotTranslate` marks a concept whose source term is the same string
everywhere: a product name, a trademark, a format acronym. It is independent of
whether a target term exists, because an untranslated term needs no entry per
locale.

Progressive disclosure: a CSV import auto-creates concepts with a single
preferred term per locale, so nothing is imposed on a user who wants a word list.

The `Terminology` interface carries the concept methods (`AddConcept`,
`GetConcept`, `DeleteConcept`, `Lookup`, `LookupAll`, `Search`, `Count`,
`Concepts`, `Close`) and the relation methods (`AddRelation`, `DeleteRelation`,
`RelationsOf`, `ListRelations`); every method takes a context. Import and
export are standalone functions rather than interface methods, so a backend does
not have to implement a file format to be a backend.

Two backends ship: an in-memory one for session-scoped batch processing, and a
SQLite one for persistent local work, built on the pure-Go driver so
cross-compilation and single-binary distribution are unaffected. A backend with
wider isolation and terminology streams can be supplied by a layer above, behind
the same interface.

### Terms are source; the store is a rebuildable projection

Terminology is **authored content, not derived state**. A person decides which
terms are do-not-translate and what the preferred wording is, and those decisions
belong in review and version control alongside the recipe and the voice profile.
So the split is source versus projection, not a two-way sync:

- the committed **terms source is the truth**: a diff-friendly, mergeable JSON
  document (`kind: "kapi-terms"`) bound by `defaults.terms_source`, edited
  directly (`kapi apply` with `kind:"term"` writes the file first), reviewed in a
  pull request, and versioned with the code. It is plain JSON under a compound
  suffix, so a reviewer reads it in a browser diff and `jq` reads it on the
  command line.
- the **terms tables inside `.kapi/work/store.db` are a rebuildable projection**
  of it ([C-03](c-03-context-store-and-graph.md)), ignored by version control and
  rebuilt when the committed source changes, guarded by its content digest.
  Discard them, rebuild from the source, lose nothing: **nothing authoritative
  ever lives only in the database.** Committing the binary database would be
  hostile to review and would defeat interchange in any case.

A project that binds nothing still resolves, through a fallback ladder:
`.kapi/terms.json`, the conventional home inside the committed context, then
`terms.json` at the project root for a project that keeps its terms there. An
explicit `defaults.terms_source` wins over both and binds any path. The
conventional home comes first because that is where the rest of the project's
context lives, and terms are one node of it rather than a loose file beside it.
Both rungs are committed and both reach review, which is the one thing the terms
source exists to do.

That ladder works here because **a project has exactly one set of terms**. The
content memory has no equivalent convention: a project accumulates *many* memory
bundles, one per content surface ([C-09](c-09-content-memory.md)), so there is
no single bundle for a fallback to name. The asymmetry is a consequence of what
each store is.

Read-only consumers read the committed source directly. The terminology gate
decodes it without materializing any tables, which is why it holds on a fresh CI
checkout. Presence is table-level, so the gate behaves identically whether the
project's database is absent, present with empty terms tables, or fully populated
([C-03](c-03-context-store-and-graph.md)); the tables earn their keep only for
the heavy indexed lookups during translation.

**The projection is rebuilt on the read path, not only by an edit.** The
convergence loop compiles the bound source into the store before it runs, keyed
by the file's content digest, so a fresh checkout converges against the committed
vocabulary and a pulled change to the source reaches the store with no explicit
import. An unchanged source costs a read and no writes.

### The return leg: reviewed decisions come home

Terminology is often decided where the reviewers are, which may not be a working
tree. Decisions have to travel back into version control or they exist only in a
store nobody diffs. After a concept pull, the reviewed decisions among the pulled
concepts are **merged** into the committed source.

Two properties make that safe to run unattended:

- **Upsert-only, at every level.** A concept the decision set does not mention
  survives; a term it does not mention survives; nothing is removed. A
  whole-store export over the authored file would be the opposite: it would
  delete every concept the decision source has not adopted. Removal stays an
  edit an author makes by hand.
- **Byte-stable.** The merged document is compared against the bytes on disk and
  an identical serialization is not rewritten, so a run with no new decisions
  writes nothing at all.

What counts as reviewed is the term's own status. A layer that admits *forbidden*
and *preferred* only from a reviewed change-set, refusing them on the direct
write path, makes a term resting at either carry the evidence that a reviewer
approved it. A concept whose terms are all ungoverned is ordinary working state
and stays out of version control until a decision touches it. The same rule
selects the one governed relation kind.

The projection lands in the working tree; publishing it is a reviewable pull
request, never a push to the default branch.

### Tiered lookup

Lookup is a cascading pipeline (`terms.LookupTiered`, with `LookupAllTiered` for
occurrence scanning):

1. **Exact**: case-sensitive match on normalized term text.
2. **Normalized**: Unicode NFC, case folding, whitespace collapse.
3. **Fuzzy**: trigram candidate retrieval plus Levenshtein scoring over the
   closest candidates, inside a length window.
4. **Model-assisted** (opt-in): a provider proposes candidate mappings that
   produce term-candidate annotations for human review.

Each tier stops early once it has an answer. The fuzzy tier uses the same FTS5
trigram tokenizer as the content memory ([C-09](c-09-content-memory.md)), keeping
lookup cost sub-linear in the size of the store. Text is normalized with Unicode
NFC before comparison, and Levenshtein runs over runes rather than bytes, which
is correct for every script including CJK.

Which tiers run is selected per call through the lookup options, alongside case
sensitivity, a minimum score, and the status and validity filters, so a caller
can request exact-only, or exact-plus-fuzzy, without changing the pipeline.

### Scanning a text: one matcher, one rule

`LookupAllTiered` asks a different question (*which declared terms does this
passage use*) and answers it with `check.TermMatcher`, the single definition of
what it means for a text to use a term. The voice-profile vocabulary rules, the
do-not-translate check and the occurrence graph scan with the same matcher, so a
word is a hit for the whole gate or for none of it.

`term-check` is the exception. A mandate names a lemma while content inflects
it: a source reading "Two new alerts" uses the term `alert`, and an obedient
Norwegian rendering of `alarmmelding` is `alarmmeldinger`. So both its sides
match on containment rather than on word boundaries. The cost is real (a mandate
on `use` also fires inside `user`), and no boundary rule separates the two
cases, because `user` is a word that starts with `use` exactly as `alerts` is a
word that starts with `alert`. Telling an inflection from a coincidence needs
stemming or explicitly declared forms, not a stricter matcher, which is what a
term rule's `forms` are for ([C-07](c-07-voice-profiles.md));
`scripts/contexteval` pins the Norwegian case that would regress first. The
whole-word rule is Unicode-aware: an underscore continues a word, so
`mooring_id` is one token rather than a use of `mooring`; scripts written
without word separators take no boundary rule at all; and a multi-word term
matches across any run of whitespace.

Where two declared terms cover the same characters, the longer one is reported
and the shorter suppressed. A project that has declared `mooring_id` a concept of
its own has said those characters are not a use of the retired name inside them,
and reporting both would be the graph contradicting itself.

Distinct from lookup, `Search` powers the terms browser in the CLI and the
desktop app. It uses a full-text tokenizer with relevance ranking rather than
unranked substring queries.

### Locating declared terms: one pass, two sources

A term is declared in two places. A voice profile lists the words a product
forbids and a competitor's names it must not print, and a tool's `term_rules:`
lists the wording a piece of content is held to; the terms store holds the
concepts the project has decided, which is where `kapi apply` writes. Both are
the same kind of statement about the same words, so a gate that asks them
separately is two gates that can disagree about whether a word is in use.

`terms.Locate` asks once. It takes the rules the caller holds and the bound
store, matches the rules through `profile.MatchTermRules` and the store through
`LookupAll`, and returns **occurrences**: the matched surface text, the
declaration that governs it, the concept it denotes, and a `model.Anchor`
positioning it in the block's runs. Store matches are deduped across the
candidate languages, because a term recorded in both `en-GB` and `en` is one
decision about one word. A `LocateRequest` also carries the domains, minimum
score and validity scope passed through to the store lookup; those narrow which
declarations are consulted, which is a different question from which uses
matter.

An occurrence is a **use**, not a verdict. The pass reports every declared term
it finds, including the preferred and approved ones, and says nothing about
whether any of them is a problem. Which uses are violations is the consuming
gate's policy and lives there: the voice vocabulary gate objects to a
competitor's name, a forbidden term and a retired one, and `term-lookup`
annotates all of them because context is what it is for. A pass that filtered to
one caller's three statuses would be a pass only that caller could use.

The matcher is rule-shaped rather than profile-shaped: `MatchTermRules` takes
term rule *sets*, each carrying the kind of violation a hit is and the severity
a rule that names none of its own takes. A voice profile contributes two sets
(forbidden terms at major, a competitor's at critical) through
`VocabularyRuleSets`; a tool contributes its own. That is what lets one match
run cover every source a caller holds, and what keeps a rule-carrying tool from
being a second-class citizen of the vocabulary gate.

What a consumer does with an occurrence is its own business. The voice
vocabulary gate raises a finding, presenting it through `HitsToFindings`, the
mapping every check surface shares (`kapi check`, the `check_text` and
`check_file` MCP tools, the desktop panel). It names the terms store when the
store is what declared the term, because "forbidden by the profile" and
"forbidden in terms" send a writer to different places to argue with the
decision. Locating is the part they share.

### Annotations

Three annotation types implement the annotation interface. Each is written onto
a block as an overlay span whose range is a `model.Anchor`, so a UI highlights
precisely without re-detecting term boundaries at render time. The lookup itself
returns a character-level range into the source text, which `terms.Locate`
converts to an anchor once, so two callers cannot convert it differently.

- **`TermAnnotation`**: a matched term from the store, carrying the concept id,
  target-term options, status, score and match type.
- **`TermCandidateAnnotation`**: a proposed term not yet in the store, carrying
  a proposed marker so a reviewer can accept, reject or defer.
- **`EntityAnnotation`**: named entities (people, organizations, products,
  dates, locations) with optional do-not-translate flags. Entity annotations
  feed content-memory generalization ([C-09](c-09-content-memory.md)),
  do-not-translate handling in translation, locale formatting hints, and
  term-candidate discovery: one annotation pass serving several consumers.

### Concept relations

The store persists typed, directed relation edges between concepts. Each has an
id, a source and target concept, a type from the SKOS-aligned vocabulary, an
optional note and an optional validity: broader/narrower, part-of/has-part,
related, replaced-by, use-instead, exact-match/close-match, and competitor.

Writes are gated: a relation is rejected unless its type is in the vocabulary and
both concepts exist. The read methods take an optional scope and return only
edges whose validity matches. Relations give UIs a graph substrate for browsing
terminology without a separate graph database, and drive deprecation workflows:
the `term-enforce` tool resolves *use-instead* and *replaced-by* to name the
replacement.

### Validity

A term and a relation each carry an optional half-open `[valid-from, valid-to)`
interval plus free-form tags. Lookup options and the relation read methods accept
a scope (a point in time plus tags) and return only what is active there. This
is how the store answers as-of-time and within-a-tag-scope questions; the
framework assigns tags no meaning, leaving the vocabulary to the caller.

It is the **same temporal model** a governing profile's window uses
([C-02](c-02-coordinates-and-governance.md)) and the same one the context graph's
edges carry ([C-03](c-03-context-store-and-graph.md)), so *what was in force
then* reads identically wherever it is asked.

### Status transitions

`ValidateTransition` accepts any transition between known statuses, and
`IsGovernedTransition` flags the consequential ones: any transition to
*forbidden* or *preferred*, or away from *forbidden*. The framework classifies
transitions; it does not impose a review workflow, which is left to a layer
above.

### Competitor terms

A term carries a competitor flag. `voice-vocab-check` surfaces competitor terms
found in source text as critical-severity voice findings, and forbidden terms as
major-severity, using the store's voice-vocabulary term source
([C-07](c-07-voice-profiles.md)). This gives the framework a minimal hook for
voice guardrails without depending on the whole voice module.

### Pipeline tools

The framework ships terminology tools as ordinary pipeline stages:

- **`term-lookup`** (enrich): records where the source uses a declared term,
  as term annotations with run-anchored positions. It reads both sources through
  `terms.Locate`: the concepts in the store and the rules a project carries under
  `term_rules:`, so terminology declared in a recipe counts as much as
  terminology decided in the store. Downstream tools use these for context.
- **`term-check`** (validate): holds a target to the renderings its
  `term_rules:` mandate, with the containment matcher described above. A rule's
  severity sorts a violation into an error or a warning, and the verify gate
  reports both while failing only on the first.
- **`term-enforce`** (validate): for each known source term, checks that an
  acceptable target-locale translation is present, and flags blocks where it is
  missing. A source term whose concept is forbidden or deprecated redirects
  through its *use-instead* or *replaced-by* relation, so the expected rendering
  is the replacement's. Forbidden, deprecated and competitor detection is
  `voice-vocab-check`'s job, not this one's.
- **`dnt-check`** (validate): checks that do-not-translate terms survive
  verbatim into the target. It takes `term_rules:` like every governed step and
  unions the rules marked do-not-translate, which is how the store's
  `DoNotTranslate` concepts reach it, with the strings a recipe or `--terms`
  names directly. A store is not required: a recipe may name its terms alone.
- **`term-extract`** (enrich, model-assisted): extraction of candidate terms
  with a proposed status.
- **`entity-extract`** (enrich, model-assisted): named-entity annotation. Should
  run early, before `recycle`.
- **`redact`** / **`unredact`** ([C-10](c-10-redaction.md)): the pair that
  replaces entity values with typed placeholders before an external service and
  restores them afterwards.

<PipelineDiagram
  stages={[
    { label: "Source", sub: "binding", role: "io" },
    { label: "entity-extract", sub: "model/NER", role: "annotate" },
    { label: "term-lookup", role: "annotate" },
    { label: "recycle", sub: "memory", role: "translate" },
    { label: "translate", role: "translate" },
    { label: "term-enforce", role: "qa" },
    { label: "Sink", sub: "binding · optional", role: "io" },
  ]}
/>

### The command surface

`kapi terms` carries `import`, `export`, `lookup`, `search`, `occurrences`,
`stats` and `list`. The store selector is **`--termstore`**: `--terms` is already
taken as the boolean gate on `kapi exec dnt-check`, and the asymmetry with
`--memory` is guarded by a test. The recipe follows the flag: a profile binds a
standalone store with `profiles.<name>.termstore`, and `terms` names contents
(the concepts, and dnt-check's list of strings), never a store.

`kapi terms occurrences` reports where a concept is actually used, reading the
occurrence index in the block cache ([C-03](c-03-context-store-and-graph.md)).

## Consequences

- Terminology is a first-class pipeline citizen rather than a post-processing
  step.
- Run-anchored positions enable precise inline highlighting without re-detecting
  boundaries at render time.
- One pass locates every declared term, so the gates that consume it cannot
  disagree about whether a word is in use.
- Entity annotations drive both term extraction and content-memory
  generalization.
- Concept relations give UIs a graph substrate without a separate graph database
  in the framework.
- The competitor flag gives voice guardrails a hook without a dependency on the
  voice module.
- The same storage backends as the content memory keep the dependency footprint
  small and cross-compilation simple.

## See also

- [C-01: The project model](c-01-project-model.md): `defaults.terms_source`.
- [C-03: The context store and graph](c-03-context-store-and-graph.md): where
  the projection lives and how occurrence is indexed.
- [C-07: Voice profiles](c-07-voice-profiles.md): the `TermRule` shape and the
  vocabulary gate.
- [C-09: Content memory](c-09-content-memory.md): shared matching
  infrastructure, and the source-versus-state contrast.
- [E-03: Tool System](../engine/e-03-tool-system.md): the pipeline-tool
  pattern.
- [Terminology data model](../../implementation/context/terminology-data-model.md): the
  full Go structs, the tool catalog and the relation vocabulary.
