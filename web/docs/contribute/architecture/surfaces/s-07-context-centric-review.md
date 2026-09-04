---
id: s-07-context-centric-review
sidebar_position: 7
title: "S-07: The review model"
description: "A review decision is made at a point. Host assembles one review model per unit, from the retrieval primitives every other surface uses, and every review client renders it: the desktop queue, the CLI, the MCP tools, and any host that records a decision with an identity."
keywords: [neokapi, architecture decision, review, context, coordinates, voice profile, terms, neighbourhood, prior version, provenance, review queue, MCP]
---

# S-07: The review model

## Summary

A reviewer decides **at a point**. The unit under decision sits in a document,
at a coordinate, governed by a voice profile and a vocabulary, beside the blocks
that precede and follow it, after a version that was approved before it.

Review consumes the two retrieval primitives
[C-06](../context/c-06-retrieval.md) defines: *what applies here* for the unit's path, and *what do we know about
this* for its content. Host assembles one **review model** from those answers,
once per unit, and every client renders it. The desktop's detail pane and the
MCP `review_unit` tool receive the same object; a client that draws a subset
chooses it in its own projection, and every client receives the whole.

The bar the design is held to is an invariant:

> **A reviewer sees at least what the model was told.**

The reference is the tool configuration the translate call received for that
unit. `prompt.Context` (`core/ai/prompt/context_sections.go`) enumerates what a
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
the unit's status. The point is resolved on every request to select which
checkers run; the neighbourhood is read from disk to build the prompt; the
content-memory match is fetched to seed the draft. Each of these exists at the
moment the unit is translated and is gone by the time it is reviewed, unless
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
| History | the prior approved version (source and target, and whether the context it was approved under still governs); the content-memory match with its wording and score | the version chain, `memory.Lookup` |
| Judgement | check findings anchored to their run positions; the AI pre-review score, model and remarks when one has run | the checkers bound at the point, `core/state` |
| Provenance | origin of the current target; the decision in force, with identity, time and note; whether that decision was recorded against source wording that has since changed | `core/state` |

Provenance is named provenance. A card labelled Context that holds only this row
is mislabelled.

Provenance carries the decision **in force**. `core/state` keeps one record per
(scope, unit, variant) and `Put` overwrites it; the model exposes what the store
holds and invents no chain. A client that wants a chain wants a store change,
which is a [C-04](../context/c-04-unit-state-and-decisions.md) decision.

### One queue, and every unit in it belongs to a language

The queue is a single list of the units awaiting a person. Each row names the
language it belongs to (`language`), and the row whose language is the
project's source carries `isSource`. Listing every language lists the source
language's units among the translations; a language filter narrows the list, and
the result carries the pending count per language beside it, so a surface offers
the languages that have work rather than a lane switch.

A row's `status` is its rung on its own ladder: `translated` for a queued
translation, and the settled authoring rung for a source unit, with `held`
marking one the project's source gate is holding the fan-out on.

`host.App.ReviewQueue` derives it, merging the target derivation and the source
derivation over one project read. The listing is unified and the storage is not:
a source decision is recorded under the source locale variant and a target
decision under the target's, as [C-04](../context/c-04-unit-state-and-decisions.md)
defines them.

### The decision set is the same on every client

A reviewer has three verdicts on a target: **approve**, which promotes it to
`reviewed`; **sign off**, which promotes it to `signed-off`, the rung above;
and **reject**, which drops it to `draft` so the unit re-enters the work queue.
The rungs are the target ladder
[C-04](../context/c-04-unit-state-and-decisions.md) defines, and the ship gates
read them, so a client offering only two of the three leaves a rung that the
gates can require and nobody there can reach.

Every verdict is language-scoped: a reviewer decides the languages they hold
review permission for. Promotion also passes the workspace separation-of-duties
policy, which judges one thing, the author of the wording under decision.
Whoever last wrote a translation by hand may not approve it and may not sign it
off, unless the policy is off or set to warn. A target a run produced has no
human author, so one person decides it. Signing off a target already at
`reviewed` is a promotion like any other and is judged the same way; the policy
draws no second line between the approver and the signer.

### Every client renders the same object

The model is one Go type, `host.ReviewContext`, assembled by
`App.AssembleReviewContext` and attached to the unit (`ReviewUnitInfo.Context`)
when a client asks for a unit with its context. The point carries the language
it was resolved for, because a term rule resolves per language. The queue itself
stays a list of units; a file's point is resolved once per queue and shared by
its units. The clients are:

| Client | How it renders the model |
| --- | --- |
| Kapi Desktop ([S-02](s-02-kapi-desktop.md)) | the queue's detail pane: a point rail, the neighbourhood, the history, findings, and a provenance card; the document view opens at the unit with review state drawn as marks |
| `kapi status --review` ([S-01](s-01-kapi-cli.md)) | the queue as a table, `--lang` narrowing it to one or more languages, and as JSON with `--json` |
| MCP `review_unit` ([S-03](s-03-agent-surfaces.md)) | the model whole, as the read leg before `approve_unit`, `reject_unit` and `sign_off_unit`; `review_queue` lists the queue with its per-language counts |
| A review surface over the REST editor | the queue as a list with the focused unit beside it: the point rail, the neighbourhood, the anchored findings, the content-memory match and the provenance, with the three verdicts under them |

A host that records a review decision with an identity is a client of this
model by shape: the layers are the contract, whatever renders them.

### The AI actions inherit the point

An AI action taken from a review client builds its tool through the same
configuration assembly the flow runner uses, `App.ToolConfigForUnit` in `host`,
never from a hand-written map. Eight fields carry context into the translate tool (term
rules, profile, memory, point, reuse, DNT, context, context window), and an
equality test holds the review path to the flow path over all eight. The AI
pre-review judge scores against the same assembly, so it judges the unit against
the voice and vocabulary in force rather than against a bare pair of strings.

### Source review is review, in the source language

Judging the author's wording and judging a translation of it are the same act on
different content, at rungs of the two ladders
[C-04](../context/c-04-unit-state-and-decisions.md) defines. Both render the
same review model, so a reviewer approving source wording sees the voice it is
approved against, and a source decision is recorded with the same identity a
target decision carries.

The source language is therefore a language of the queue rather than a mode of
it. What differs is what a client can do next: `kapi apply` records
target-language decisions, and source wording is approved through
`App.ApproveSourceUnit`, which Kapi Desktop's Review page calls.

### What a source change does to an undecided target

A decision records the source it was taken against, and coverage grades a
decided unit stale once the source moves away from that basis. An undecided
translation gets the same anchor from the loop itself: for every unit a run
writes a target for, it records a decision-less state entry carrying the hash of
the source it translated and the hash of the target it produced. The record is
committed state, so a fresh clone carries it, and it is written unstaged, so
loop output is never counted as a person's pending decision.

Coverage derives the basis for both classes alike. A source change under an
undecided target grades the unit stale, the plan counts it, and the next pass
re-drafts it with the old wording still on disk. Only a decision moves a unit on
its ladder. A target that no longer matches its recorded hash was taken over by
a person; it grades as basis unknown and is left alone and reported. No host
clears targets to force the loop's attention, and the records travel with the
decisions on push, so a venue receives the same basis the loop worked from.

The server's translation worker reads the same ledger. A target whose recorded
basis is stale is owed a draft, a target the ledger has no record of is left
alone, and a decided unit is drafted once per source change: the worker marks
the row with the source it drafted against, beside the decision it may not
replace, and the next pass counts the unit as awaiting review rather than as
work ([C-04](../context/c-04-unit-state-and-decisions.md)).

### A push carries decisions; the venue decides

A working copy holds its own decision record, and `kapi push` sends it with the
content it judges. The venue is authoritative for what has been approved in it,
so it holds every rung above translated and every approval or sign-off a push
carries to the gate its own review surfaces pass: the pusher's review permission
for that language in that project, and the workspace separation-of-duties
policy with the pusher as the decider. One function answers for every caller,
so the review endpoint, the bulk routes and the ingest worker cannot drift
apart.

A verdict that fails the gate is withheld, not the content: the translation
lands at translated, the verdict is kept as the basis it carries, and the
refusal is counted per language and reason and reported back on the push status.
The pusher is the decider recorded for a verdict that passes, whatever decider
the payload named. The project's own record then retires the refused verdicts to
the same basis, which is what stops the next push sending them again.

The other direction is held to one question. A push that lowers a target the
venue holds at `signed-off`, keeping the translation and the source the
sign-off blessed, is withdrawing that sign-off, and the review surfaces let an
un-review or a rejection do that only for a caller holding review permission
for the language. The ingest worker asks the same: a withdrawal from a pusher
without it keeps the venue's rung and ledger record, is counted as a demotion
the venue did not apply, and travels back with the record the venue kept, which
the project's own record is restored to. The separation-of-duties policy is not
asked, because a withdrawal blesses nothing. A pushed target that changes the
translation or arrives with a moved source is an edit and lands at
`translated`, as an edit in the editor does. Taking back an approval at
`reviewed` is translation work on every surface and passes ungated.

## Consequences

The context graph gains its first reader on a decision surface. The model is
retrieval over stores that exist, plus one new fact per written target (its
basis), which is a state record; what governs a point is still read from the
graph.

Rendering stays with each host. The model crosses no licence line: `host` is
Apache-2.0 and every client already depends on it. No file moves between
licence subtrees, and no module above the line imports from below it, which
`make audit-modules` asserts for Go and for TypeScript.

The invariant is enforced. The reflection
test holds the model to `prompt.Context`; the equality test holds the review AI
path to the flow path; and the MCP tool returns the model whole, so the client
with the least screen has the same facts as the one with the most.

## Related

- [C-02: Coordinates and governance](../context/c-02-coordinates-and-governance.md): the point a decision is made at
- [C-04: Unit state and decisions](../context/c-04-unit-state-and-decisions.md): what a decision records
- [C-06: Context retrieval](../context/c-06-retrieval.md): the two primitives Review consumes
- [S-01: The kapi CLI](s-01-kapi-cli.md): `kapi status --review` and `kapi apply`
- [S-02: Kapi Desktop](s-02-kapi-desktop.md): the queue and the document view
- [S-03: Agent surfaces](s-03-agent-surfaces.md): the MCP review tools
- [S-06: The visual editor data model](s-06-visual-editor.md): the kit the document view is built on
