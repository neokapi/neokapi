---
id: c-08-terms
sidebar_position: 8
title: "C-08: Terms"
description: "Architecture decision: terminology is concept-oriented (a Concept groups terms across locales with per-term status, part of speech and validity), the project's terms store is authoritative and .terms.json bundles support exchange, and one pass locates every declared term in a text."
keywords: [terms, terminology, Concept, TBX, terms store, concept-oriented, validity, term rules, architecture decision, neokapi]
---

import { PipelineDiagram } from "@neokapi/docs-shared";

# C-08: Terms

## Summary

Terminology is **concept-oriented**: a `Concept` groups terms across locales with
per-term metadata (status, part of speech, grammatical gender, validity). The
`Terminology` interface (`terms/`) supports in-memory and SQLite backends, a
tiered lookup pipeline, and TBX import and export.

The project's terms store is authoritative. `.terms.json` bundles provide a
portable representation for exchange and review. Terms flow through the streaming pipeline as first-class annotation types
whose positions are run-anchored, so a match survives run-preserving edits. One
pass, `terms.Locate`, finds every declared term in a text, whether it was
declared in the store, in a tool's `term_rules:` or in a voice file's `terms:`.
Every "write this, not that" rule is a term: a voice profile holds none
([C-07](c-07-voice-profiles.md)).

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
    Forms          []string         // other surface shapes in the term's language
}

type Concept struct {
    ID             string
    ProjectID      string
    Domain         string
    Definition     string
    Source         TermSource // terminology, or brand_vocabulary
    Terms          []Term
    DoNotTranslate bool       // the source term travels into every target unchanged
    Advisory       bool       // a use of its forbidden, competitor or retired terms reports without failing
    Properties     map[string]string
    CreatedAt      time.Time
    UpdatedAt      time.Time
}
```

`DoNotTranslate` marks a concept whose source term is the same string
everywhere: a product name, a trademark, a format acronym. It is independent of
whether a target term exists, because an untranslated term needs no entry per
locale. Every backend stores the flag with the concept, the bundle and the JSON
export carry it as `do_not_translate`, and TBX export writes it as a private
`x-doNotTranslate` concept descrip, which `ImportTBX` reads back and other TBX
readers skip.

`Forms` lists the other surface shapes a term takes in its own language: the
Norwegian plural *varsler* for *varsel*, the German *Liegeplätze* for
*Liegeplatz*. A form is a spelling of the term and carries no status. An
alternative word for the concept is a term of its own, with a status. Forms are
declared rather than derived, because the shapes a word takes are per-language
knowledge. Every backend stores them normalized (trimmed, without repeats or the
term's own text), the bundle carries them as a `forms` array, and TBX export
writes each as a private `x-surfaceForm` term note, which `ImportTBX` reads back
and other TBX readers skip.

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

A term's locale is held in canonical BCP-47 form, the way its text is held
case-folded beside it: every backend normalizes `Term.Locale` on write
(`locale.Normalize`) and every lookup, search and concept accessor asks in the
same form, so a term recorded under `en_US` is the term a check running in
`en-US` finds. The concept id `kapi apply` mints for a term decided outside any
concept, `term:<locale>:<slug>`, embeds the locale in that form.

### Authoritative storage and bundle exchange {#the-terms-store-is-the-truth-a-bundle-is-how-it-travels}

Terminology is **authored content, not derived state**. A person decides which
terms are do-not-translate and what the preferred wording is, and those
decisions have to be kept. They are kept in the project's terms store, in the
user's workspace ([C-03](c-03-context-store-and-graph.md)), where every
checkout, branch and worktree of the project reads them:

- every read goes there. The terminology gate, `kapi terms lookup`, the
  retrieval an agent calls and the governing fingerprint all ask the same store
  and get the same answer, whichever branch the checkout is on;
- every write goes there too. `kapi apply` with `kind:"term"` writes the store
  and records a context operation ([C-11](c-11-context-operations.md)), so
  `kapi context log` carries the change and the evidence behind it.

A **terms bundle** (`kind: "kapi-terms"`) serializes terms as JSON for review,
merging and command-line processing.
`kapi context snapshot` writes one, `kapi context import` reads one, and
`kapi context export` packs the same content into a `.kpz`. A team that wants
its vocabulary reviewable in a pull request keeps the snapshot committed; a team
that does not backs the store up instead, and both govern identically.

An import with no path reads `.kapi/terms.json`, the conventional place, and an
import of a directory reads the `terms.json` in it. The recipe names no terms
file: the project's own terms govern with nothing bound, and a profile binds a
standalone store by name with `termstore:`. That single convention works
because **a project has exactly one set of terms**. The content memory has no
equivalent: a project accumulates *many* memory bundles, one per content surface
([C-09](c-09-content-memory.md)), so an import reads every bundle in a directory
rather than one well-known name.

Presence is table-level, so a project whose terms tables are empty enforces
nothing, whether or not a database file exists
([C-03](c-03-context-store-and-graph.md)). A checkout carrying a terms bundle
with an empty context store triggers a notice directing the user to
`kapi context import` ([C-11](c-11-context-operations.md)). The bundle affects governance only after a person imports it.

### The return leg: reviewed decisions come home

Terminology is often decided where the reviewers are, which may not be a working
tree. Those decisions have to reach the project or they govern nothing locally.
After a concept pull, the reviewed decisions among the pulled concepts are
**merged** into the project's terms store, and the pull records a context
operation whose actor is the venue.

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

1. **Exact**: the term's text, case folded unless the call asks for case
   sensitivity, or a form the term declares that is the whole query. A query
   is read against a term's forms by the matcher described below, so a lookup
   for `alerts` finds `alert` when the term lists `alerts`, and names `alert`.
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
what it means for a text to use a term. The word-rule check, the
do-not-translate check and the occurrence graph scan with the same matcher, so a
word is a hit for the whole gate or for none of it. A store term is found under
its text and under each form it declares, and the match names the term, so
"Two alerts" is a use of `alert` when the term lists `alerts`.

`term-check` holds a translation to what its rules require, and it reads the
two sides differently. On the source side it asks whether the text uses a rule's
term, with the whole-word matcher above. An English source also finds a term's
regular inflections, the endings -s, -es, -ed and -ing and -d after a final e,
so "Two new alerts" uses `alert`. A derivation is a word of its own:
"translation" uses `translation`, and a hyphenated compound such as
"pseudo-translate" is one word. A source in any other language finds a term as
written or as a form the term declares. Placeholders are syntax on both sides
(`check.PlaceholderText`), so `{vessel}` holds no term. The source side also
reads program syntax as syntax (`check.TermText`): inline code and fenced
blocks, a kapi command in quotes such as 'kapi check --staged', an indented
example command line up to its shell comment, and a flag name such as
`--diff-range`. Code keeps its words in a translation, so a term written in the
source's code owes no rendering whatever the rule's scope, and the same word in
the prose beside it still does. A source word rule reports where a term is
written rather than what a translation owes, so it reads code and leaves it out
only when its `scope` is `prose`. A
do-not-translate rule reads the source with only its placeholders as syntax, so
a product name inside a command is still held to being kept. Where the terms of two rules cover the same
words, only the longer one is demanded.

On the target side a demanded rule is satisfied when the text contains the
preferred rendering, any admitted or approved term of the concept, or a declared
form of one of them. Containment keeps Norwegian and German compounds working,
because "kaiplassene" and "Liegeplatzplan" contain their terms. It also accepts
a different word that happens to contain a rendering, such as "tilleggskrav" for
`tillegg`, and that is the accepted cost of keeping compounds. A form that
changes the word, such as "varsler" for `varsel`, is recognised once the term
declares it, and a finding for a rule with no forms in a language that inflects
says so. The gate records, per target language, which matching it used: English
inflection or whole words for the source, and containment or containment plus
declared forms for the target. `scripts/contexteval` pins the Norwegian case
that would regress first, and `scripts/termeval` measures the matching over the
project's reviewed Norwegian content and the samples.

The whole-word rule is Unicode-aware: an underscore continues a word, so
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

A term is declared in two places. The terms store holds the concepts the
project has decided, which is where `kapi apply`, `kapi context keep` and
`kapi terms import` write. A caller holds rules of its own: a tool's
`term_rules:` lists the wording a piece of content is held to, a bound starter
pack carries its terms beside its voice, and the rules established across the
workspace apply to every project in it. Both are the same kind of statement
about the same words, so a gate that asks them separately is two gates that can
disagree about whether a word is in use.

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
gate's policy and lives there: the word-rule gate objects to a
competitor's name, a forbidden term and a retired one, and `term-lookup`
annotates all of them because context is what it is for. A pass that filtered to
one caller's three statuses would be a pass only that caller could use.

The matcher is rule-shaped rather than profile-shaped: `MatchTermRules` takes
term rule *sets*, each carrying the kind of violation a hit is and, for a set
the store did not declare, where it comes from (`From`). The store contributes
the word rules it imposes on source content through `terms.SourceWordRules`; a
voice file contributes the terms it carries through `CarriedRuleSets`; a tool
contributes its own. A hit fails unless its rule is marked `advisory`. Each rule's `MatchesCase` decides whether it matches in its
own casing ([C-07](c-07-voice-profiles.md)): case sensitively when the preferred
form is capitalised or differs from the rejected one only in case, regardless
of case otherwise, and as `case_sensitive` says when the rule sets it. That is what lets one match
run cover every source a caller holds, and what keeps a rule-carrying tool from
being a second-class citizen of the word-rule gate.

A set may be marked `Suggested`, which is how the candidates a project has
accumulated reach the same pass ([C-11](c-11-context-operations.md)). Every hit
against such a set reports and never fails, whatever its rule declares, and
carries `Suggested` through the hit, the finding and the diagnostic. A rule
nobody has confirmed is therefore reported wherever a decided term would be,
weighs nothing in the score, and fails no check.

What a consumer does with an occurrence is its own business. The word-rule
gate raises a finding, presenting it through `HitsToFindings`, the mapping every
check surface shares (`kapi check`, the `check_text` and `check_file` MCP
tools, the desktop panel). Every finding has one message shape
(`Forbidden term "x" found`, `Competitor term "x" found`, `Retired term "x"
found`), and one whose rule the store did not declare carries `metadata.from`
naming the source (`pack technical-docs`, a workspace rule), so a writer knows
where to argue with the decision. `kapi check` reports these findings under the
`terms` analyzer with rule id `terms.vocabulary`, wherever the rule is held.
Locating is the part the consumers share.

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

### Word rules and advisory concepts

Every word rule is a term, so the terms store holds one word list. A term
carries a competitor flag for a rival's name, and a concept carries an
`advisory` marking. The word-rule gate (`voice-vocab-check`) surfaces competitor
and forbidden terms found in source text as failing findings and a retired
(deprecated) term as one that reports. On an advisory concept every such use
reports without failing. A person marks a concept advisory with
`kapi context keep --advisory`, `kapi terms import --advisory` (which marks every
imported concept), an `x-advisory` descrip in TBX, an `advisory` column in CSV,
or `"advisory": true` on a `kind: term` change-set entry, which also takes
`"competitor": true`.

### Pipeline tools

The framework ships terminology tools as ordinary pipeline stages:

- **`term-lookup`** (enrich): records where the source uses a declared term,
  as term annotations with run-anchored positions. It reads both sources through
  `terms.Locate`: the concepts in the store and the rules a project carries under
  `term_rules:`, so terminology declared in a recipe counts as much as
  terminology decided in the store. Downstream tools use these for context.
  An occurrence is where a reader sees a term written, so both sources read
  code as written, with only placeholder names masked.
- **`term-check`** (validate): holds a target to the renderings its
  `term_rules:` require, finding source terms by word and renderings by
  containment as described above. A do-not-translate rule names no rendering:
  the target keeps its term verbatim, as the term, a declared form or the
  source's own occurrence, in that casing, with placeholder names masked on both
  sides. A violation of a rule fails, and one of a rule marked `advisory`
  reports; the verify gate reports both and fails only on the first. The
  bilingual comparison matches a rule in its own casing only when the tool or
  the rule sets `case_sensitive`, because the replacement is written in another
  language and its capitalisation says nothing about the source term. It probes canaries
  under its own configuration: a target with the rendering deleted, one holding
  a word that opens like the rendering and ends differently, the shape of
  "Kaiplan" for `kaiplass`, and a target that does not keep a do-not-translate
  term. The ship terminology gate, `kapi check` with a target in a project, the
  loop checks behind `kapi status` and `ship.json`, and the platform's
  compliance predicate all decide through it.
- **`term-enforce`** (validate): for each known source term, checks that an
  acceptable target-locale translation is present, and flags blocks where it is
  missing. A source term whose concept is forbidden or deprecated redirects
  through its *use-instead* or *replaced-by* relation, so the expected rendering
  is the replacement's. Forbidden, deprecated and competitor detection in the
  source belongs to `voice-vocab-check`.
- **`dnt-check`** (validate): checks that do-not-translate terms survive
  verbatim into the target. It takes `term_rules:` like every governed step and
  unions the rules marked do-not-translate with the strings a recipe, `--terms`
  or `kapi check --dnt` names directly. A store is not required: a recipe may
  name its terms alone.
- **`term-extract`** (enrich, model-assisted): extraction of candidate terms
  with a proposed status.
- **`entity-extract`** (enrich, model-assisted): named-entity annotation. Should
  run early, before `recycle`.
- **`redact`** / **`unredact`** ([C-10](c-10-redaction.md)): the pair that
  replaces entity values with typed placeholders before an external service and
  restores them afterwards.

Terminology reaches generation as well as validation, by two routes of
different strength.

A string a recipe or `--dnt` names is **masked**: the translate step replaces
each occurrence with a sentinel before the model sees the text and restores the
original afterwards, so the term cannot be translated, transliterated or
reworded. The lock costs the batched path, because a per-span sentinel cannot
be tracked across a packed multi-segment generation, so a run configured with
masked terms translates one block per call.

A concept the terms store marks do-not-translate is **instructed and checked**:
the translate step renders it as a term to keep verbatim, and `term-check` fails
a target that does not keep it. An instruction alone is not a guarantee, and the
check is what makes it one. Concepts are instructed rather than masked so that a
store holding a handful of product names does not pin every run to one block per
call.

The rules that name a replacement are rendered as the renderings to use, each
projection scoped to the text of the call. Both enter the context fingerprint
stamped on what the step writes, so marking a concept do-not-translate makes the
content it governs stale rather than leaving targets that were drafted without
it.

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
`stats`, `expand`, `validate` and `list`. The store selector is **`--termstore`**: `--terms` is already
taken as the boolean gate on `kapi exec dnt-check`, and the asymmetry with
`--memory` is guarded by a test. The recipe follows the flag: a profile binds a
standalone store with `profiles.<name>.termstore`, by the name `--termstore
<name>` takes and never by a path, and `terms` names contents (the concepts, and
dnt-check's list of strings), never a store. A `termstore:` that is a path or a
file name fails to load.

`kapi terms occurrences` reports where a concept is actually used, reading the
occurrence index in the block cache ([C-03](c-03-context-store-and-graph.md)).
It searches each term under its text and every declared form, because the block
cache's text search matches one string and a form such as *varsler* does not
contain *varsel*.

`kapi terms expand` fills in forms at authoring time. It asks a model for the
forms of each term in the term's own language, one language at a time and in
batches, and keeps a proposal only when it opens with the term's first three
characters and is spelled differently from every other term in that language.
What survives is written onto the terms in the store, where a
reviewer reads it in the diff, so the matching that consumes the forms stays
deterministic and free of model calls. By default it asks about target terms and
leaves terms that already declare forms alone. `kapi terms validate` reports the
structural errors a store refuses, and warns about a target term with no forms in
a language that inflects and about a form spelled the same as another term.

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
- One word list: a forbidden, competitor or retired term is decided once, in
  the store, and every check reads it there.
- The same storage backends as the content memory keep the dependency footprint
  small and cross-compilation simple.

## See also

- [C-01: The project model](c-01-project-model.md): where a bundle sits in the
  layout.
- [C-03: The context store and graph](c-03-context-store-and-graph.md): where
  the projection lives and how occurrence is indexed.
- [C-07: Voice profiles](c-07-voice-profiles.md): the `TermRule` shape, the
  voice files that carry terms, and the word-rule gate.
- [C-09: Content memory](c-09-content-memory.md): shared matching
  infrastructure, and the source-versus-state contrast.
- [E-03: Tool System](../engine/e-03-tool-system.md): the pipeline-tool
  pattern.
- [Terminology data model](../../implementation/context/terminology-data-model.md): the
  full Go structs, the tool catalog and the relation vocabulary.
