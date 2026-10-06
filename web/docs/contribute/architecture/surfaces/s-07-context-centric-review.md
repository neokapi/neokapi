---
id: s-07-context-centric-review
sidebar_position: 7
title: "S-07: The review model"
description: "A review decision is made at a point. Host assembles one review model per block, from the retrieval primitives every other surface uses, and every review client renders it: the desktop queue, the CLI, the MCP tools, and any host that records a decision with an identity."
keywords: [neokapi, architecture decision, review, context, coordinates, voice profile, terms, neighbourhood, prior version, provenance, review queue, MCP]
---

# S-07: The review model

## Summary

A reviewer decides **at a point**. The block under decision sits in a document,
at a coordinate, governed by a voice profile and a vocabulary, beside the blocks
that precede and follow it, after a version that was approved before it.

Review consumes the two retrieval primitives
[C-06](../context/c-06-retrieval.md) defines: *what applies here* for the block's path, and *what do we know about
this* for its content. Host assembles one **review model** from those answers,
once per block, and every client renders it. The desktop's detail pane and the
MCP `review_block` tool receive the same object; a client that draws a subset
chooses it in its own projection, and every client receives the whole.

Content is reviewed block by block. What kapi learned about how a project writes
is reviewed differently: as discovery in a **digest** that reads as news and
never gates the work (below).

The bar the design is held to is an invariant:

> **A reviewer sees at least what the model was told.**

The reference is the tool configuration the translate call received for that
block. `prompt.Context` (`core/ai/prompt/context_sections.go`) enumerates what a
translate prompt carries about a block beyond the block: its key, the blocks
before it, the blocks after it, and the prior approved version. Each of those is
a field of the review model, and a reflection test in `host`
(`TestReviewContextAnswersPromptContext`) holds the model to the struct: a field
added to `prompt.Context` fails the test until the review model answers for it,
the way `check-run-projection.sh` holds every run projection to `RUN_KINDS`.

## Context

The engine has a context graph, coordinate axes, per-point governance,
run-anchored findings and a version chain. A review client addresses a
`(file, key, locale)` triple and, on its own, can reach only the two texts and
the block's status. The point is resolved on every request to select which
checkers run; the neighbourhood is read from disk to build the prompt; the
content-memory match is fetched to seed the draft. Each of these exists at the
moment the block is translated and is gone by the time it is reviewed, unless
something keeps it.

The review clients differ in who is behind them. The desktop queue is the
repository owner triaging their own project. The CLI is the same person in a
terminal or a CI log. The MCP tools are an assistant acting on a person's
behalf, recording decisions as `agent/<client>`. An assistant with only the two
texts approves against nothing; the same is true of a person, with a slower
failure.

## Decision

### One model, assembled in host

The review model binds a decision to its context. Host assembles it from
answers that already exist and serves it unchanged to every client.

| Layer | What it carries | Source |
| --- | --- | --- |
| Point | profile, channel, collection, coordinates, the voice in force with its rendered guidance, the terms in force, profile validity | `host.ContextAnswer`, resolved per file |
| Neighbourhood | the block's key, and the blocks before and after it in document order, each with its source and what the locale under review says there | the file's blocks as the reader returns them |
| History | the prior approved version (source and target, and whether the context it was approved under still governs); the content-memory match with its wording and score, and its answer's runs when the answer holds a code or a plural, which a surface that uses the match saves | the version chain, `memory.Lookup` |
| Judgement | check findings anchored to their run positions; the AI pre-review score, model and remarks when one has run | the checkers bound at the point, `core/state` |
| Provenance | origin of the current target; the decision in force, with identity, time and note; whether that decision was recorded against source wording that has since changed | `core/state` |

Provenance is named provenance. A card labelled Context that holds only this row
is mislabelled.

Provenance carries the decision **in force**. `core/state` records each
decision as an entry in an append-only, content-addressed ledger, keyed by the
block, the edition and the pairing it blessed (the source and translation
revisions), and a checkout's view points at the entry for the pairing its files
hold; `Put` appends an entry. The model exposes the entry that applies and
invents no chain over the others
([C-04](../context/c-04-block-state-and-decisions.md)). Every write to the
edition sits in the block history, which the Review page lists beside it
([S-02](s-02-kapi-desktop.md)).

### One queue, and every block in it belongs to a language

The queue is a single list of the blocks awaiting a person. Each row names the
language it belongs to (`language`), and the row whose language is the
project's source carries `isSource`. Listing every language lists the source
language's blocks among the translations; a language filter narrows the list, and
the result carries the pending count per language beside it, so a surface offers
the languages that have work rather than a lane switch.

A row's `status` is its rung on its own ladder: `translated` for a queued
translation, and the settled authoring rung for a source block, with `held`
marking one the project's `translate_after` level is holding the fan-out on.

`host.App.ReviewQueue` derives it, merging the target derivation and the source
derivation over one project read. The listing is unified and the storage is not:
a source decision is recorded under the source locale variant and a target
decision under the target's, as [C-04](../context/c-04-block-state-and-decisions.md)
defines them.

### The decision set is the same on every client

A reviewer has two verdicts on a target: **approve**, which establishes it,
and **reject**, which drops it to `draft` so the block re-enters the work queue.
The rungs are the target ladder
[C-04](../context/c-04-block-state-and-decisions.md) defines, and the ship gates
read them. There is one human rung, and no second rung above it. An agent reviews
ahead of the person with a score and its reasons, which the queue shows and
which never count as a decision.

Every verdict is language-scoped: a reviewer decides the languages they hold
review permission for. Promotion also passes the workspace separation-of-duties
policy, which judges one thing, the author of the wording under decision.
Whoever last wrote a translation by hand may not approve it, unless the policy
is off or set to warn. A target a run produced has no human author, so one
person decides it.

The author is read from the block history, which records every writer: a
person through `kapi apply`, ksed, Kapi Desktop or the browser engine, an agent
through the MCP tools, a flow, a pull, a merge, and an edit made outside kapi,
which a read records as observed with no author. The author of a translation
is the writer of the change that left it as it is (`history.Store.Wrote`), so
a branch switch brings back the author of what the branch holds. On the venue
the policy reads the venue's own block history, where its editor and its
change route record the person sending each change and an agent's change is
its person's. Content written on a checkout reaches it through a push, which
says for each translation who wrote it. A translation a person or an agent
wrote by hand on the checkout (not one a pull or a merge brought in, nor one
observed) is the pusher's. The venue keeps that author for the translation's
revision whether or not it holds the translation (`edition_writers`), names
the author in the block history row a push writes for a translation it holds,
and judges an approval of it, in that push or a later one, as the author's.
The first pusher of a revision stays its author when another checkout that
pulled the record sends the same write, and a write of another revision by
anybody else ends the claim. A translation a flow produced, or one a pull, a
merge or an edit outside kapi brought in, names no author, and one person
decides it.

### Every client renders the same object

The model is one Go type, `core/review.Context`, which `host` names
`ReviewContext`. `App.AssembleReviewContext` assembles it and attaches it to
the block (`ReviewUnitInfo.Context`) when a client asks for a block with its
context. The point carries the language it was resolved for, because a term
rule resolves per language. The queue itself stays a list of blocks; a file's
point is resolved once per queue and shared by its blocks.

The type sits in the framework, below the licence line, because two hosts
assemble it. The platform's REST review context is the same struct embedded
whole, with the rows only the platform holds beside it: the block's own address
there, the positioned term hits its document surface marks, the block's notes,
and the block's voice score against its profile's bar. Where the two venues
hold the same fact, one spelling and one scale carry it: the memory match is an
integer percent on both, a neighbour carries its rung on both, and the decision
in force carries its rung on both. The conversions the venues share (the match
percent, the prior version judged against the fingerprint of the context the
current target was produced under, the term rules led by the ones bearing on
the wording) are functions in `core/review`, and a test in the server holds
its assembler to the host's over one block.

The TypeScript both frontends read is generated from the same structs
(`packages/contract-types/src/review.gen.ts`, by `make generate-contract-types`,
drift-gated in CI), so a field added to the model reaches every client or
fails to compile in the one that ignores it. The clients are:

| Client | How it renders the model |
| --- | --- |
| Kapi Desktop ([S-02](s-02-kapi-desktop.md)) | the queue's detail pane: the five shared cards over the model, and the document view opening at the block with review state drawn as marks |
| `kapi status --review` ([S-01](s-01-kapi-cli.md)) | the queue as a table, `--lang` narrowing it to one or more languages, and as JSON with `--json` |
| MCP `review_block` ([S-03](s-03-agent-surfaces.md)) | the model whole, with the reference and revision of the edition under review, as the read leg before a pre-review sent through `apply_edits`; `review_queue` lists the queue with its per-language counts and each row's reference |
| A review surface over the REST editor | the queue as a list with the focused block beside it: the same five cards over the same model, the findings anchored on the target, with the three verdicts under them |

A host that records a review decision with an identity is a client of this
model by shape: the layers are the contract, whatever renders them.

The rendering is shared. One card per layer lives in `@neokapi/ui-primitives`
(`packages/ui/src/components/review/`: `PointCard`, `NeighbourhoodCard`,
`HistoryCard`, `JudgementCard` with the AI pre-review inside it, and
`ProvenanceCard`, each on the folding `LayerCard`), and each card takes its
layer of the generated model as its prop. A review shell hands its layers to
the cards and keeps what is its own: Kapi Desktop's `ReviewPage` owns the
verdict bar, the AI actions and the source-block pane; the REST review surfaces
own the target editor, the anchored marks on the target, the re-check and the
term and voice-rule dialogs, and pass the platform's own rows (the term hits,
the score against the bar, the latest block note, the check issues on the
error and warning scale) as the cards' extra props. A row a venue leaves empty
draws the card's own empty case, so the same block reads the same way on either
surface. The origin kinds, the decision states, the tone a finding takes and
the neutral chip a term rule takes are all named once, in the cards.

### The AI actions inherit the point

An AI action taken from a review client builds its tool through the same
configuration assembly the flow runner uses, `App.ToolConfigForUnit` in `host`,
never from a hand-written map. Eight fields carry context into the translate tool (term
rules, profile, memory, point, reuse, DNT, context, context window), and an
equality test holds the review path to the flow path over all eight. The AI
pre-review judge scores against the same assembly, so it judges the block against
the voice and vocabulary in force rather than against a bare pair of strings.

### Source review is review, in the source language

Judging the author's wording and judging a translation of it are the same act on
different content, at rungs of the two ladders
[C-04](../context/c-04-block-state-and-decisions.md) defines. Both render the
same review model, so a reviewer approving source wording sees the voice it is
approved against, and a source decision is recorded with the same identity a
target decision carries.

The source language is therefore a language of the queue rather than a mode of
it. A decision on either kind is a `decide` operation sent to the change service
([E-09](../engine/e-09-the-change-contract.md)) with the revision the reviewer
read: `kapi apply`, an agent's pre-review and Kapi Desktop's Review page all send
it. An approval of source wording is recorded as an `establish`, bound to the
wording the reviewer read.

### What a source change does to an undecided target

A decision records the source it was taken against, and coverage grades a
decided block stale once the source moves away from that basis. An undecided
translation gets the same anchor from the loop itself: every document a run
writes is recorded as the flow's `content.edit`, and the block history keeps,
for each translation the run produced, the revision and the hash of the source
it translated and the revision of the target it left ([E-09](../engine/e-09-the-change-contract.md#flows)).
The record lives in the project's context log, so a checkout that reads the
project's context reads it, and it is not a decision, so loop output is never
counted as a person's pending decision. A fresh clone holds the translations
the repository carries and none of that record until it runs
`kapi context pull`; `kapi up` and `kapi status` say so in one line while the
translations exist on disk and the history records nothing.

Coverage grades the basis of both classes alike, by the revision of the
source, so a changed link counts as a source change as much as a changed word.
A source change under an undecided target grades the block stale, the plan counts
it, and the next pass re-drafts it with the old wording still on disk. Only a decision moves an edition on
its ladder. A target that no longer holds the revision the flow left was taken
over by a person; the next read records that change as observed, with no
author and no basis, and the block grades as basis unknown and is left alone and
reported. No host clears targets to force the loop's attention. A venue's own
worker records the basis of the drafts it writes, and a push carries the basis
a run on a checkout recorded (see below), so the venue grades both alike. The
server venue grades by the revision of the source as well, taken under the
project's source language, so a changed link reads stale there as it does in
coverage on a checkout ([C-04](../context/c-04-block-state-and-decisions.md#unit-state-is-unit-keyed-and-bound-to-the-pairing-it-blessed)).

The server's translation worker reads the same ledger. A target whose recorded
basis is stale is owed a draft, a target the ledger has no record of is left
alone, and a decided block is drafted once per source change: the worker marks
the row with the source it drafted against, beside the decision it may not
replace, and the next pass counts the block as awaiting review rather than as
work ([C-04](../context/c-04-block-state-and-decisions.md)).

### A push carries decisions; the venue decides

A working copy holds its own decision record, and `kapi push` sends it with the
content it judges. A decision names the translation and the source it blesses by
revision, as the project's record does, and a block whose source changed in an
inline code alone is sent as a changed block, so the venue grades the decision
against the source the checkout holds. The checkout records a basis under the
key its reader filed the source by, which for a file that declares its own
language is that language, and the venue takes every revision under the
project's source language, so the push sends each basis as the venue's revision
of the same source (`venue.Basis`). Beside the decisions it sends how each translation of the
documents it reads came to be (`venue.EditionWrite`): the write the block
history records as having left the translation the checkout holds, with its
revision, the source it was made from (sent the same way), the writer (person,
agent, tool, or external) and the surface. A pulled translation's write names no source: the
venue's own record holds it. Each write goes until the venue has applied a
push that carried it, and again when it changes. The venue records them after
the decisions, reading each item's blocks, translations and ledger rows once:
on a block nobody has decided, the basis becomes the block's ledger record, as
the basis of one of its own drafts does, so a translation a run on the
checkout produced is graded stale once its source moves; a tool's write from a
recorded source marks the block drafted against it; a write about another
translation than the one the venue holds leaves the basis and the draft mark
alone; and a write by hand records its author.

The venue is authoritative for what has been approved in it, so it holds every
rung above translated and every approval a push
carries to the gate its own review surfaces pass: the pusher's review permission
for that language in that project, and the workspace separation-of-duties
policy with the pusher as the decider. One function answers for every caller,
so the review endpoint, the bulk routes and the ingest worker cannot drift
apart.

A verdict made in the venue records what governed it, the way a verdict made in
a project does: the voice profile the venue's own ladder resolves for the
block's collection and locale, and the term rules its workspace holds, folded by
the function every producer stamps with
([C-04](../context/c-04-block-state-and-decisions.md)). It reaches the ledger,
the content memory the approval promotes to, and the project's record on the
next pull, so a decision made in either place answers the staleness question
against one definition of the context in force.

A verdict that fails the gate is withheld, not the content: the translation
lands at translated, the verdict is kept as the basis it carries, and the
refusal is counted per language and reason and reported back on the push status.
The pusher is the decider recorded for a verdict that passes, whatever decider
the payload named. The project's own record then retires the refused verdicts to
the same basis, which is what stops the next push sending them again.

A rejection is held to the translation it names before the gate is asked. One
whose translation the venue has since replaced is dropped: the block keeps its
rung, its ledger record and its draft mark, the refusal is counted as a demotion
the venue did not apply, and the venue's record travels back for the project to
take. A rejection of the translation the venue holds lands, and clears the
platform's mark that it has drafted the block, so the next run drafts it again.

The other direction is held to one question. A push that lowers a target the
venue holds at `established`, keeping the translation and the source the
decision blessed, is withdrawing it, and the review surfaces let an
un-review or a rejection do that only for a caller holding review permission
for the language. The ingest worker asks the same: a withdrawal from a pusher
without it keeps the venue's rung and ledger record, is counted as a demotion
the venue did not apply, and travels back with the record the venue kept, which
the project's own record is restored to. The separation-of-duties policy is not
asked, because a withdrawal blesses nothing. A pushed target that changes the
translation or arrives with a moved source is an edit and lands at
`translated`, as an edit in the editor does.

### Context is reviewed as discovery, in a digest

A context suggestion (a term rule, a note about how the project writes, a
wording pair) advises agents and checks from the moment it is recorded, and an
established rule is what fails a check ([C-11](../context/c-11-context-operations.md)).
A suggestion nobody answers keeps advising. Reviewing context is therefore
reading what kapi learned and answering the few things that need a person;
nothing waits on it, and there is no queue to clear.

`host.App.ContextDigest` (`host/contextdigest.go`) assembles one project's
digest from the operation log, and every surface reads it: the desktop's
Learned section ([S-02](s-02-kapi-desktop.md)) and `kapi context digest`, with
`--json` for a program. Its sections come in the order a person reads them:
conflicts (`contested` records, each with the rules on its other side), rules
established and on what evidence, suggestions grouped by theme and then by
collection, drift away from an established rule, and the project in numbers.
Every item carries the id the context verbs take, its rule as a sentence, the
quotation it was seen in, its standing as plain counts and who noticed it.

The actions are the context operations a person already has, and each is an
operation in the log: keep, keep with a changed form (`--use`), drop, revert,
and choosing a side of a conflict (`host.App.ChooseContextSide`, `kapi context
keep --choose`), which drops each rival suggestion, reverts a rival established
rule and keeps the chosen one, one operation per step.

"Since you last looked" belongs to the reader. It is a marker per project in the
machine account's config (`host.ContextDigestMarkerPath`), never in the log,
and only a person moves it: `kapi context digest` moves it after printing unless
`--peek` is given or the actor is an agent. The digest flags each item as new or
not and keeps the ones already seen, so a surface shows them under "Earlier".

The digest is shaped for the surfaces that read it later: the request names a
project by recipe path or workspace key and an optional instant to read from,
and the answer carries no rendering beyond the sentences and the text form.

## Consequences

The context graph gains its first reader on a decision surface. The model is
retrieval over stores that exist, plus one new fact per written target (its
basis), which is a state record; what governs a point is still read from the
graph.

Rendering stays with each host. The model sits in the framework
(`core/review`), which every client already depends on, so no module above
the licence line imports from below it, which `make audit-modules` asserts for
Go and for TypeScript.

The invariant is enforced. The reflection
test holds the model to `prompt.Context`; the equality test holds the review AI
path to the flow path; and the MCP tool returns the model whole, so the client
with the least screen has the same facts as the one with the most.

## Related

- [C-02: Coordinates and governance](../context/c-02-coordinates-and-governance.md): the point a decision is made at
- [C-04: Block state and the decision record](../context/c-04-block-state-and-decisions.md): what a decision records
- [C-06: Context retrieval](../context/c-06-retrieval.md): the two primitives Review consumes
- [S-01: The kapi CLI](s-01-kapi-cli.md): `kapi status --review` and `kapi apply`
- [S-02: Kapi Desktop](s-02-kapi-desktop.md): the queue, the document view and the digest
- [C-11: Context operations](../context/c-11-context-operations.md): the operations the digest reads and the actions it offers
- [S-03: Agent surfaces](s-03-agent-surfaces.md): the MCP review tools
- [S-06: The visual editor data model](s-06-visual-editor.md): the kit the document view is built on
