---
id: c-11-context-operations
sidebar_position: 11
title: "C-11: Context operations"
description: "Architecture decision: every change to a project's context is an appended operation carrying an actor, a subject, the evidence behind it and the governance it was made against. Candidates advise at neutral severity and can never fail a check; a person's confirmation makes a rule binding and writes it into the subsystem that already reads it."
keywords: [context operations, candidate, confirm, revert, widen, evidence, policy, actor, operation log, workspace, architecture decision, neokapi]
---

# C-11: Context operations

## Summary

Every change to a project's context is an **operation**: an actor, a kind, a
subject, the evidence behind it, the governance it was made against, and a
timestamp from Go's clock. Operations are appended to the workspace's operation
log ([C-03](c-03-context-store-and-graph.md)) and never edited. A status change
is a new operation naming the earlier one.

A proposed rule takes effect at once as **advice**. Checks report it at
`neutral` severity, which carries no penalty and trips no gate threshold, so a
candidate shows up wherever a rule would and can never fail a check. A person's
**confirmation** makes it binding at the severity the rule carries, and writes
it into the subsystem that already reads it: the terms store, the voice
profile, or the content memory. The log is the history and the source of
candidates, not a second home for confirmed rules.

Who may do what is one function, `contextop.PersonDecides`. Today an agent or a
tool may observe, propose and record a correction; only a person confirms,
edits, discards another actor's work, withdraws a confirmed rule, or widens one.

## Context

A project starts with an empty context. It has no terms, no voice profile and
no content memory, and everything it comes to know accumulates out of ordinary
work: somebody notices how the product name is written, an agent proposes a rule
from a hundred documents that agree, a person corrects a translation, a check
enforces what was agreed, and the next correction is evidence for the rule after
that.

Two properties decide whether that loop is usable. The first is that a machine's
suggestion must never stop a person's work, because a project that fails its
build over an unreviewed guess is a project whose context people turn off. The
second is that every step must be attributable and reversible, because the loop
writes into the files and stores a project is governed by, and nobody accepts an
automated writer they cannot audit or undo.

`core/state` settled the same question for unit decisions
([C-04](c-04-unit-state-and-decisions.md)): an append-only ledger, a content
address, and one `Policy` function that every writer goes through. Context
operations take the same shape for the same reasons.

## Decision

### An operation is the unit

`contextop.Record` is one recorded change:

```go
type Record struct {
    ID       string               // the position the workspace log gave it
    Project  workspace.ProjectKey
    Actor    Actor                // person | agent | tool, a name, an agent's session
    Kind     Kind                 // observe | propose | correct | confirm | discard | revert | widen
    Subject  Subject              // a term rule, a voice rule, a content-memory pair, a note
    Evidence []Evidence           // file, unit, quotation: where this was seen
    Basis    Basis                // the governance in force when it was recorded
    Scope    Scope                // how far it reaches, and at which coordinates
    Target   string               // the operation this one acts on
    At       time.Time
    Status   Status               // folded from the log, never stored
}
```

The seven kinds divide into three that carry a subject and four that act on one.
`observe` records a fact with no rule implied, such as how a product name is
written or who the documents address. `propose` records a candidate rule with
its evidence. `correct` records that someone changed wording from one form to
another at a location, carries both wordings, and may carry the rule it implies.
`confirm`, `discard`, `revert` and `widen` name an earlier operation and say
what became of it.

Status is folded rather than stored. `contextop.Ledger` reads the log forward
and reports each subject-bearing operation at the status the operations naming
it left it: `candidate` until something acts on it, then `confirmed`,
`discarded` or `reverted`. Naming a decision rather than the rule it decided
reaches the rule, so `revert` on a confirmation undoes the confirmation's
subject.

### Candidates advise, confirmed rules bind

A candidate is projected into checks as an advisory rule set
(`profile.TermRuleSet.Advisory`). The matcher raises every hit against such a
set at `check.SeverityNeutral`, whatever severity the rule declares, and stamps
`Advisory` on the hit, the finding and the diagnostic. Neutral is the level the
framework already defines as carrying no penalty: it weighs zero in the score,
counts into `Summary.Neutral` alone, and trips none of the gate's limits. A
candidate therefore cannot fail a check under `--strict` or under any threshold
a project sets, and no gate had to be taught about it.

The flag is what a surface reads to show the finding as a proposal rather than a
broken rule, and `HitsToFindings` words the message accordingly: *Proposed rule
about "utilise", not yet confirmed*.

Candidates also run outside the vocabulary analyzer, before it. An analyzer's
findings are its verdict, and `kapi check` counts them, scores them and holds
the analyzer to a canary; a candidate settles none of that. The same stance
`core/check` takes with a `Warning`: reported beside the findings, read by no
gate. That is also what lets a project with no voice profile and no terms report
its candidates, which is the ordinary case on the day a project is created.

Confirming applies the rule's own severity, because confirming writes it into
the project's stores and every existing reader grades it from there.

### Confirmation writes through the existing appliers

`kapi apply` is the one write verb, and its asset entries write into the
committed source the recipe binds before the existing importer compiles that
source into the store ([C-08](c-08-terms.md),
[C-07](c-07-voice-profiles.md), [C-09](c-09-content-memory.md)). Confirming a
rule builds the same change-set entry and runs the same applier. The rule
therefore lands in `.kapi/terms.json`, `.kapi/voice.yaml` or
`.kapi/memory/memory.json`, `git diff` shows the decision, and retrieval,
checks, drafting, the governing fingerprint and `kapi context snapshot` all see
it with no second code path.

Withdrawing reverses it: the term is removed from the committed source, the
concept is deleted from the store and re-imported from what the source still
declares, and the voice profile is rewritten and re-imported. The re-import
rather than an in-place edit is what keeps a concept's other terms standing when
the confirmed rule had joined an existing concept.

`kapi apply` records the operation pair its own asset entries produce, a
`propose` and the `confirm` of it, both by the person who ran the command.
An entry naming an agent as its actor is refused by the policy before anything
is written.

### Scope is set by evidence, and widened deliberately

An operation is scoped to the point its evidence was seen at: the coordinates
`ResolveGovernanceFor` resolves for the file it was seen in
([C-02](c-02-coordinates-and-governance.md)), which for a new project with no
declared axes is that project. A rule answers where its scope covers the point
being checked, so a rule seen in one product's reference pages says nothing
about another product's tutorials.

Confirming is also the moment a person may **widen**. Naming an axis drops it
from the rule's point, so a rule learned at one mode answers at every mode of
its brand. Naming `workspace` puts the rule in force in every project of the
workspace.

A workspace-wide rule has no project store to live in, so the workspace holds
it: `workspace.Rule`, a row in `workspace.db` carrying an id, a kind, the
project its evidence came from, and the rule as opaque JSON. The workspace never
reads inside the payload, which keeps it ignorant of term rules and of whatever
is widened next. Resolution puts these beneath what the project itself says: a
term the project's own store declares hides the workspace-wide rule about it,
and the most specific answer wins.

### One policy function

```go
type Transition struct {
    Actor    Actor
    Kind     Kind
    Subject  SubjectKind
    Target   Record
    Targeted bool
    Widening bool
    Editing  bool
}

type Policy func(Transition) error
```

`PersonDecides` is what ships. An agent or a tool may observe, propose and
record a correction, and may discard or revert its own candidate, which is how a
session cleans up after itself. Everything that turns advice into a rule belongs
to a person: confirming, editing another actor's proposal, discarding one,
withdrawing a confirmed rule, reverting a whole session, and widening.

Every writer goes through it: `kapi context`, `kapi apply`, the agent tools, the
desktop feed. Agent roles with wider rights are a change to this function rather
than to each call site.

### Reversibility is exact

Reverting a session marks every operation it recorded as reverted and retracts
whatever they put in force. What a check says before a session and after that
session is reverted are the same findings, the same summary and the same gate
result, not a similar answer. `host/contextops_test.go` asserts the equality
rather than a resemblance.

Nothing is erased. A reverted operation stays in the log with its evidence, so
the same suggestion is recognisable the next time it is made.

## Consequences

An agent can write into a project's context continuously without any risk of
stopping a build, because everything it writes is advice until a person says
otherwise. A person reviewing a week of that work reads one list, decides in one
verb, and can undo a whole session in another.

The log is folded on every read rather than indexed. A workspace holds one
operation per context change, which stays small enough to read whole; an
implementation that needs an index adds one behind `Ledger` without moving the
model.

Operation ids are workspace-local positions, so they are not portable between
workspaces. Nothing needs them to be: confirmed rules travel in the committed
sources and the context bundles, which carry no operation ids.

## Surfaces

`kapi context observe`, `propose`, `correct`, `log`, `confirm`, `discard`,
`revert` and `widen` are the command-line half. The host API
(`host/contextops.go`) is typed requests and results with no flag sets, so the
agent tools and the desktop drive the same loop.

The agent surface is the first three and a read, one tool per habit:
`context_observe`, `context_propose`, `context_correct` and
`context_session_summary`, which reports what one session recorded and what
became of it. Each wraps one host call and adds no rule of its own. There is no
tool for confirming, discarding, reverting or widening, because those are a
person's and a tool that is always refused is one an assistant keeps trying
([S-03](../surfaces/s-03-agent-surfaces.md)).

The actor rides on the call rather than in it. An MCP tool takes no actor
argument and refuses one: the kind is `agent`, the name comes from the client's
own `initialize`, and the session is minted once per server process, so
everything one run recorded reads back and reverts together. A command line
records as the person running it, which is what a command line is.

Evidence is required where a rule is stated. `context_propose` and
`context_correct` declare the path as a required argument and refuse a blank
one, so a rule nobody can check never reaches the log.
