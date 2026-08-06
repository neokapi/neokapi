---
id: 021-brand-knowledge-graph
sidebar_position: 21
title: "AD-021: The context graph"
---

# AD-021: The context graph

> **Vocabulary.** This subsystem is the **context graph**. Brand is one
> **coordinate** on it — beside audience, surface, register, market and validity
> — not the frame. The identifiers below (`brand_voice_*` recipe keys, the
> `core/brand` package, the `kg_*` tables, this document's own slug) keep their
> spellings; the concepts they name are described in the settled vocabulary.

## Summary

Bowrain models a workspace's content context as a graph whose nodes are
**concepts** — the language-neutral units already used by the terms store — and
whose edges are typed, time- and market-scoped **relations** between them.
Vocabulary rules, terminology, observations about how others use a term,
discussion threads, and revision history all attach to the same concept, so a
single page can tell the whole story of a term: where it came from, what it
replaced, where it is banned, how it is said in every market, and what it would
cost to change. (Compliance scores are reported per project and locale on the
dashboard, not folded into an individual concept.)

A **profile** is the named bundle of coordinates a piece of content is written
under — *developer reference*, *end-user help*, *regulated disclosure* — and it
is what scopes a rule. The voice profile is the same shape one axis narrower.
Coordinates are declared at the broadest scope that is true and inherited from
there, so a rule set high up applies everywhere beneath it until something
overrides it, and can start and stop on a date. See
[The context graph](/getting-started/the-context-graph) for the reader-facing
account.

Changes to the graph follow a tiered governance model. Ordinary curation is
direct and audited. Governed transitions — banning a term, changing a preferred
term, merging an experiment — travel through a **change-set**: a named,
reviewable draft of graph and voice edits whose **blast radius** over existing
content is computed before anyone approves it. A change-set can optionally be
**piloted** on selected content streams before it merges, so a what-if becomes
a measured experiment rather than a leap.

The navigator surfaces as a unified **Brand** hub in the web and desktop apps
(Concepts, Voice, Experiments, Activity, Dashboard). Its Concepts section is a
searchable **concept list** that opens, per concept, a **dashboard** — terms,
geography, constraints, a local relations widget, a timeline, observations, and
discussion — not a whole-graph canvas. Governed terminology reaches a project's
local files through ordinary sync (`kapi pull`/`kapi push`), and assistants read
it through MCP tools, so CI gates and AI assistants consume the same governed
truth the hub shows.

## Context

A context decays through drift, not decisions: a competitor name slips into a
tagline, a renamed product survives in old docs, a market keeps a term the
company retired.

The questions a steward actually asks need a home: *what replaced this term, and
when?* *Is this banned everywhere or only in Germany?* *Who decided that, and
what did they discuss?* *If we rename this concept, how much published content
moves?* Each question touches identity (concepts), history (versions and audit),
scope (coordinates — market, surface, validity), and consequence (content
impact) at once — which is the shape of a governed graph, not of flat tables
holding terms, voice rules, and relations apart from one another.

This is also why a flat list of preferred and banned terms is the wrong shape. A
rename is one decision and five different rules — the help article changes, the
API parameter must not, the changelog keeps the old name, the migration guide
needs both, support recognises the old name without using it. Which rule applies
depends entirely on where the words sit, so rules are scoped by coordinates and
resolved per location, never applied globally.

## Decision

### One node type: the concept

The `terms` package's `Concept` is the single node of the brand knowledge graph. Brand
vocabulary is not a parallel system: a forbidden term is a concept whose term
carries `forbidden` status, and the brand profile's vocabulary rules reference
concepts by ID (`TermRule.ConceptID`). Concept-less rules exist only for
standalone kapi use, where a profile file travels without a workspace graph;
on the platform every rule — including rules promoted by the
correction-learning loop (AD-019) — is concept-backed.

Relations are persisted with the terms store as first-class records. The relation
vocabulary reuses the framework's SKOS-aligned labels: hierarchy (`BROADER`,
`NARROWER`, `PART_OF`, `HAS_PART`, `RELATED`), succession (`REPLACED_BY`),
guidance (`USE_INSTEAD`), cross-scheme equivalence (`EXACT_MATCH`,
`CLOSE_MATCH`), and brand stance (`COMPETITOR`). Each relation may carry a
`Validity`: a half-open time interval plus free tags evaluated against a
query-time scope. The graph can therefore be asked *as of* a date or *within*
a market, and the same edge can be true in one market and absent in another.

### Markets

A **market** is a workspace-defined scope — a name plus the locales it covers
(for example `dach` covering `de-DE`, `de-AT`, `de-CH`). Markets give the free
validity tags a stable vocabulary: terms and relations scoped with
`market: dach` are interpreted, filtered, and rendered consistently across
every surface. A term's status may differ per market by carrying market-scoped
validity; a concept page renders the per-market truth side by side.

### The concept story

Every concept has a **story**: a single chronological timeline merging its
revision history (every edit produces an immutable revision snapshot whose
summary records the term-status transitions and the relations gained or lost),
the observations recorded against it, its comment threads, and the change-sets
that affected it. The story answers "how did this term get here" without
archaeology across systems. Revisions are the per-concept slice of the
hash-chained audit trail (AD-020); compliance scores are not part of the story —
those are aggregated per project and locale on the dashboard.

### Observations — what others say

An **observation** attaches external evidence to a concept: a competitor's
phrasing, customer language from support tickets, a style-guide citation, a
regulatory requirement. Observations carry a kind, a quote, a source, an
optional locale and market, and provenance. They are evidence, not rules: they
inform proposals and appear in the story, but they enforce nothing.

### Comments

Concepts carry threaded comments with @mentions, reusing the platform's
mention-notification machinery. Discussion lives where the decision will be
made — on the concept or on the change-set under review — and resolved threads
remain part of the story.

### Experiments: change-sets and pilots

A **change-set** is a named draft of edits to the graph and to brand voice
vocabulary: create or update concepts, add or remove terms, change a term's
status, add or remove relations, add or remove concept-backed voice rules. Ops
accumulate in a draft; nothing touches the live graph until merge. A change-set
moves through `draft → in_review → approved → merged`, or is `abandoned`; every
transition is audited, lands in the activity feed, and — on submit — summons the
reviewers who can act on it (see [The summons](#the-summons)).

Two capabilities make a change-set an experiment rather than paperwork:

- **Blast radius.** At any point, the platform evaluates the draft against
  stored content across the workspace: which blocks in which projects,
  collections, streams, and locales would be newly flagged or resolved, with
  word counts as a proxy for re-translation effort and sample blocks for
  inspection. This generalizes the single-rule preview of AD-019 to arbitrary
  graph edits. Nothing is persisted by the preview.
- **Pilots.** A change-set may be bound to one or more content streams as a
  pilot. While the pilot is active, term lookups, enforcement, and brand checks
  in those streams resolve through the draft (a stream-scoped shadow over the
  workspace graph, using the terms store's existing stream inheritance), so real
  content and real checks exercise the proposal. Merging the change-set applies
  the ops to the workspace graph and retires the shadows; abandoning it retires
  the shadows untouched.

### Tiered governance

Edits are classified as **ordinary** or **governed**. Ordinary edits —
definitions, notes, observations, comments, proposed terms, non-status term
metadata — apply directly, produce a revision, and land in the audit chain.
Governed edits — setting a term's status to `forbidden` or `preferred`,
removing such a status, `REPLACED_BY` relations, concept-backed voice rule
changes, and merging any change-set — require a change-set with at least one
approval from someone other than its author (separation of duties, enforced
with the same machinery as AD-020). Term status transitions are additionally
validated against a transition policy in the framework — for example, a
forbidden term cannot silently return to preferred without passing through a
governed transition.

The pending-approval queue is deliberately simple: a list of proposals, each
with a human-readable summary, a blast radius, and an approve/reject decision.
This is the contract a future stakeholder review app consumes (see below).

### The summons

Separation of duties means a submitted change-set is, by construction, work for
someone who did not ask for it. So submitting does not merely record a state: it
reaches the people who can approve it, over every channel the workspace has.

Eligibility is computed from the same permission the approve and reject gates
check — the workspace members holding `manage_brand`, minus the author. The reach
can therefore never disagree with the gate, and a summons that reached only the
author would be a summons to nobody.

On submit, a change-set:

- opens one **review task** (`review_terms`) in the workspace queue, assigned to
  an eligible reviewer, or unassigned when there is none, so the work is visible
  either way. Approve, reject, abandon and merge close it again.
- creates a **notification** for every eligible reviewer, delivered live over the
  existing web socket.
- sends a **review-request email** to each of them. Governed review is task work,
  so it routes on the `task` notification category, which ships with email
  enabled: send-on-submit is the default, and a reviewer who wants no mail turns
  the category off in notification preferences.
- records an **activity** entry, alongside the audit-chain entry every knowledge
  event already produced.

Sending is best-effort. The change-set is already in review by the time the
summons runs, so a task store that refuses an insert or an unconfigured mailer is
logged against the change-set and never turned into a failed submit.

The link every channel hands out is the change-set's review page,
`/:workspace/context/changes/:id`.

### Blast radius as a first-class query

Beyond change-sets, every concept exposes *where it is used*: occurrences of
its terms across stored blocks, grouped by project, collection, stream, and
locale. This powers the "consequences" half of the navigator — a steward sees
the footprint of a concept before proposing anything.

### Surfaces

- **Web and desktop** present one **Brand** hub with five sections: Concepts
  (a searchable concept list that opens a per-concept dashboard, with the
  changes waiting on a review named above it — a proposed concept is not in the
  list, so without that the page reads as though nothing had happened), Voice
  (profiles and the correction loop), Experiments (change-sets, reviews,
  pilots), Activity (the brand-scoped event timeline), and Dashboard
  (compliance, drift, coverage, pending decisions). There is no whole-graph
  visualization: a concept's relations are read and edited as a **local
  widget** on its dashboard — the concept plus the concepts one hop away, with
  large families collapsed to an "N related" summary and a click to navigate —
  so the surface never has to lay out or guard an unbounded graph. The desktop
  app remains a working copy of the server: it proxies the same REST surface,
  and its views stay fresh through React Query's stale-time and
  refetch-on-focus; concepts and relations are edited inline against the
  server, never authored offline.
- **CLI and CI.** Governed terminology rides ordinary project sync through the
  bowrain plugin. `kapi pull` snapshots the workspace's governed concepts and
  their relations into the project's local terms store, and — decisive for CI —
  `kapi check --ship --terms` then gates offline against the same truth the hub
  shows. `kapi push` sends local concept edits back: ordinary edits apply
  directly through the concept endpoints, while governed edits (a banned or
  preferred term, an un-forbidding, a `REPLACED_BY` relation, a concept delete)
  are bundled into one submitted change-set proposal, so the separation of
  duties holds whether an edit originates in the hub or from a project.
- **MCP.** Assistants navigate the workspace's concepts through MCP read tools
  (concept search, concept story, experiment status), complementing the
  existing brand and terminology tools.

### Framework and platform split

The framework (Apache) owns what any terminology system needs: the concept and
term model, persisted relations with validity, the status-transition policy,
matching and enforcement tools, and import/export. Bowrain (AGPL) owns
governance and collaboration: markets, observations, comments, revisions,
change-sets, reviews, pilots, blast radius over stored content, events, audit,
and every surface. The framework remains free of platform types; the platform
consumes framework types directly.

## Consequences

- The terms store is the system of record for brand vocabulary; brand
  profiles keep enforcement semantics but delegate identity to concepts, so
  vocabulary and enforcement cannot drift apart silently.
- Relations and validity make queries scope-dependent: every consumer of term
  data must decide its scope (time, market, stream). Defaults are "now,
  everywhere, main".
- Change-sets introduce a second write path to the graph. Direct writes and
  merges serialize through the same store, and merge re-validates ops against
  current revisions so stale drafts conflict loudly instead of clobbering.
- Pilot shadows add stream-scoped state that must be cleaned up on merge and
  abandon; the implementation owns that lifecycle, not the user.
- Blast radius over a whole workspace is bounded work but not free; it runs
  on demand and reports per-project partial progress rather than blocking.

## Future: the stakeholder review app (design only)

The governed-proposal queue is the natural seam for a lightweight stakeholder
surface: a phone-first app where a brand owner reviews one card per proposal —
the summary in plain words, the blast radius as one number and one chart, the
discussion highlights — and swipes to approve or reject, with an optional
"discuss" gesture that drops a comment into the thread. Everything it needs
exists in this decision's API: list pending change-sets, read summary and
blast radius, post a review verdict, post a comment. Scoped session grants
(AD-016) bound such an app to review-only permissions. Push notifications map
to the existing notification dispatcher. No additional server concepts are
required, which is the point: the swipe app is a view over governance, not a
second governance system. It is intentionally not implemented now.

## Cross-references

- AD-006 — graph concept storage (the dormant store this decision activates as
  a projection target).
- AD-019 — the correction-learning loop; promotions now create concept-backed
  rules.
- AD-020 — governance, audit, and rollback; change-set approvals reuse SoD and
  the audit chain.
- AD-005 — streams; pilots bind change-sets to content streams.
- Implementation detail (schemas, routes, op types):
  [knowledge graph data model](../notes/knowledge-graph-data-model.md).
