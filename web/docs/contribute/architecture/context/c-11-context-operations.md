---
id: c-11-context-operations
sidebar_position: 11
title: "C-11: Context operations"
description: "Architecture decision: every change to a project's context is an appended operation carrying an actor, a subject, the evidence behind it and the governance it was made against. Suggestions advise at neutral severity and can never fail a check; a person keeping one establishes the rule and writes it into the subsystem that already reads it."
keywords: [context operations, suggestion, keep, drop, withdraw, contested, revert, widen, evidence, policy, actor, operation log, workspace, architecture decision, neokapi]
---

import { CycleDiagram } from "@neokapi/docs-shared";

# C-11: Context operations

## Summary

Every change to a project's context is an **operation**: an actor, a kind, a
subject, the evidence behind it, the governance it was made against, and a
timestamp from Go's clock. Operations are appended to the workspace's operation
log ([C-03](c-03-context-store-and-graph.md)) and never edited. A status change
is a new operation naming the earlier one.

A suggested rule takes effect at once as **advice**. Checks report it with no
weight in the score, so a suggestion shows up wherever a rule would and can
never fail a check. A person **keeping** it establishes the rule, which then
fails a check unless it is marked advisory, and writes it
into the subsystem that already reads it: the terms store or the content
memory. The log is the history and the source of suggestions; established
rules live in the stores.

`contextop.PersonDecides` defines the permission policy. An agent or a tool
may observe, record a correction, and withdraw its own suggestion in the
session that recorded it. Only a person keeps, edits, drops, imports, reverts an
established rule, or widens one.

## Context

Projects accumulate context through observations and corrections made during
normal writing work. Agents can propose rules with evidence; people review
those proposals before checks enforce them.

<CycleDiagram
  steps={[
    { label: "Observe", sub: "a fact or a term, with evidence" },
    { label: "Suggest", sub: "advice reported by checks" },
    { label: "Keep", sub: "a person establishes the rule" },
    { label: "Enforce", sub: "kapi check" },
    { label: "Correct", sub: "record a wording change" },
  ]}
  caption="Observations and corrections become suggestions. A person reviews and keeps a suggestion before checks enforce it."
/>

Suggestions must not fail builds before review. Operations must also be
attributable and reversible. The design follows the unit-decision model in
`core/state` ([C-04](c-04-unit-state-and-decisions.md)): an append-only ledger
and a shared policy function for all writers.

## Decision

### An operation is the unit

`contextop.Record` is one recorded change:

```go
type Record struct {
    ID            string               // the operation id, the same in every log that holds it
    Short         string               // its first ten characters, as a log line shows it
    Project       workspace.ProjectKey
    Actor         Actor                // person | agent | tool, a name, an agent's session
    Kind          Kind                 // observe | correct | import | edit | keep | drop | withdraw | revert | widen
    Subject       Subject              // a term rule, a content-memory pair, a note
    Correction    *Correction          // the wording before and the wording after
    Evidence      []Evidence           // file, unit, quotation: where this was seen
    Basis         Basis                // the governance in force when it was recorded
    Scope         Scope                // how far it reaches, and at which coordinates
    Target        string               // the operation this one acts on
    TargetSession string               // the session a revert undoes
    Note          string               // why, in the recorder's words
    At            time.Time
    Status        Status               // folded from the log, never stored
    ContestedBy   []string             // the operations on the other side of a disagreement
    Established   bool                 // a person kept, imported or wrote it
}
```

The nine kinds, persisted as `context.<kind>`, divide into four that carry a
subject and five that act on one.

- `observe` records something somebody noticed. Stated in prose, it is a note,
  such as who the documents address. Stated as the form the project uses for a
  word and the forms it avoids, it is a term rule.
- `correct` records that someone changed wording from one form to another at a
  location, carries both wordings in `Correction`, and may carry the term rule
  the change implies as its subject.
- `import` records one context file a person read into the project's stores,
  and `edit` records a rule a person wrote directly, with `kapi apply` or by
  editing the voice profile. Both are established from the start.
- `keep`, `drop`, `withdraw`, `revert` and `widen` name an earlier operation and
  say what became of it.

`contextop.Ledger` derives status by replaying the log in order and applying
subsequent operations to each subject:

| Status | What it means | Answers |
| --- | --- | --- |
| `suggested` | nothing has acted on it | advises |
| `established` | a person kept, imported or wrote it | fails a check unless advisory |
| `contested` | it disagrees with another rule, named in `ContestedBy` | advises |
| `withdrawn` | its author took it back in the session that recorded it | silent |
| `dropped` | a person set it aside | silent |
| `reverted` | somebody undid it, alone or with its session | silent |

Operations can target a decision indirectly: reverting a `keep` retracts the
rule that operation established.

### An observation states a term rule by its forms

`kapi context observe --term Quickcast --instead-of "Quick cast"` records the
form the project uses and a form it avoids. `contextop.AvoidedForms` derives
the rest deterministically, with no model: the `--instead-of` forms, then the
spacing, hyphen and case variants of a compound. The rule above avoids
`Quick cast`, `Quick-cast`, `QuickCast` and `quickcast`.

A compound's parts come from the term's own spaces, hyphens and case changes,
or, for a closed word such as `Quickcast`, from an `--instead-of` form that
breaks it. A lower-case phrase such as `content memory` yields only
`content-memory`, without generating a closed compound. A word with neither has no
parts to vary, and its rule avoids what `--instead-of` lists. The first avoided
form becomes the rule's term and the rest its forms. A rule whose avoided forms
differ from the term only in case matches as written, so the rule about
`quickcast` never fires on `Quickcast`.

### Suggestions advise, established rules bind

A suggestion or a contested rule is projected into checks as a suggested rule
set (`profile.TermRuleSet.Suggested`). The matcher raises every hit against such
a set with `Fails` false, whatever the rule declares, and stamps `Suggested` on
the hit, the finding and the diagnostic. A suggested finding weighs zero in the
score and counts into `Summary.Reporting`, so a suggestion cannot fail a check.

The flag is what a surface reads to show the finding as a suggestion rather than
a broken rule, and `HitsToFindings` words the message accordingly: *Suggested
rule about "utilise", not yet established*.

Suggested-rule checks run before and independently of the `terms` analyzer.
This allows projects without a voice profile or terms store to report
suggestions. Suggested findings have zero scoring weight and do not affect
gates or the analyzer's canary validation.

Keeping a rule writes it into the project's stores. Subsequent checks use the
rule's advisory setting: violations of an established rule fail unless it is
marked advisory.

### Disagreements are contested until a person chooses

Two rules disagree when they share a form to avoid and name different forms to
use, or when one avoids the form the other says to use. Rules at points that
cannot meet, in different projects or at different coordinates, never disagree.
The fold marks a disagreement `contested` and names the other side in
`ContestedBy`:

- Two suggestions about the same word are both contested, each naming the
  other. Both advise, and neither can be kept until a person drops one.
- A suggestion that contradicts an established rule is contested by the rule,
  and the rule stays in force.
- A person's correction that reverses an established rule contests the rule.
  The rule is taken out of the terms store and reports instead of failing,
  until the person keeps it again or drops or reverts the correction, so a
  person's own edit never fails their build.

`kapi context keep --session <id>` keeps everything one session suggested and
leaves each contested suggestion for later, naming the other side.

### Keeping writes through the existing appliers

`kapi apply` is the one write verb, and its asset entries write the project's
terms store or content memory ([C-08](c-08-terms.md),
[C-09](c-09-content-memory.md)). A word rule is a term, so keeping one writes a
concept, marked advisory when the rule is. Keeping a
rule builds the same change-set entry and runs the same applier, so retrieval,
checks, drafting, the governing fingerprint and `kapi context snapshot` all see
it with no second code path. A term rule with several forms to avoid lands each
form, and a form that differs from the form to use only in case stays out of
the store, which folds case.

Reverting an established rule reverses it against the same stores: the term is
deleted from the terms store, or the pair from the content memory. Other terms in the concept are preserved because deletion targets the term.

Every one of these store writes goes through the projector
([C-03](c-03-context-store-and-graph.md#the-stores-are-projections-of-the-log)),
so the log holds each rule twice over: the context operation that decided it,
and the `terms.write`, `memory.write`, `voice.write` or `rules.write` operation
carrying the rows the decision wrote, whose origin names the operation it
carries out. The context operations fold into statuses; the store operations
replay into the stores.

`kapi apply` records one `edit` operation for each term or content-memory entry
it applies, established from the start and attributed to the person who ran the
command. An entry naming an agent as its actor is refused by the policy before
anything is written.

### Reading a checkout's context files is an operation too

`kapi context import` is the one command that opens a context file in a
checkout ([C-01](c-01-project-model.md)). What it reads it records: one
`import` operation per file, established from the start, with an actor of kind
person, carrying the
file's project-relative path and the SHA-256 of the bytes it read as evidence.
`kapi context log` then shows an import beside every other change to the
project's context, and a reader can tell which rules came from a file and which
from a decision somebody took. What the file held travels in the log as well:
each file is read as one projector batch, recorded as one store operation per
store it reaches, with the parsed concepts, entries or profile in a blob when
they are large. A second machine replaying the log therefore gets the imported
content, and needs neither the file nor its bytes.

A person runs it. The policy function below refuses an agent's import, naming
the command for the person to run, because reading a checkout's files puts them
in force for everyone working in the project. `kapi voice edit` records one
`edit` operation whose note says the profile was edited whole.

The skip stamps live in the workspace, keyed by the normalized checkout path and
the file's project-relative path, so two checkouts of one project never skip
each other's files and a second import of unmoved bytes reports that it read
nothing. `--force` reads regardless.

`host.ContextFilesUnread` detects a checkout with context files and an empty
project context store. It checks file metadata and whether the store contains
any concept, memory entry, voice profile or decision, without reading the
context files. It returns the file list and import command.

Every surface uses this result ([S-02](../surfaces/s-02-kapi-desktop.md),
[S-03](../surfaces/s-03-agent-surfaces.md)). `kapi check` reports it as a
configuration warning that affects neither scores nor verdicts.

### Scope is set by evidence, and widened deliberately

An operation is scoped to the point its evidence was seen at: the coordinates
`ResolveGovernanceFor` resolves for the file it was seen in
([C-02](c-02-coordinates-and-governance.md)), which for a new project with no
declared axes is that project. A rule answers where its scope covers the point
being checked, so a rule seen in one product's reference pages says nothing
about another product's tutorials.

Keeping is also the moment a person may **widen**, with `--widen-to`, or later
with `kapi context widen`. Naming an axis drops it
from the rule's point, so a rule learned at one mode answers at every mode of
its brand. Naming `workspace` puts the rule in force in every project of the
workspace.

Workspace-wide rules use `workspace.Rule` rows in `workspace.db`. Each row
contains an id, kind, source project and opaque JSON payload. This keeps
workspace storage independent of the rule type. Project-specific terms take
precedence over workspace rules for the same term.

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

`PersonDecides` is what ships. An agent or a tool may observe and record a
correction, and each takes effect as a suggestion. The author of a suggestion
may withdraw it in the session that recorded it, which is how a session cleans
up after itself, and an agent may revert its own suggestion. Everything that
turns advice into a rule, or sets another actor's advice aside, belongs to a
person: keeping, editing what another actor suggested, importing, writing a
rule directly, dropping, reverting an established rule or a whole session, and
widening.

`drop` and `withdraw` deactivate suggestions but have different permissions. A person drops a suggestion; asked to drop an established rule, kapi
refuses and names the `kapi context revert` that takes it out of the stores.
Its author withdraws a suggestion, and only while it is still one.

Widening is a property of the transition rather than of the verb, so the check
covers every route to it. `Ledger.Append` raises `Widening` for a `widen` and
equally for a `keep` that names the workspace, and a `Widening` transition is
refused for any actor but a person. Two sessions of one agent count as two
parties, so an agent acts on its own suggestion and on nobody else's.

Every writer goes through it: `kapi context`, `kapi apply`, the agent tools, the
desktop feed. Agent roles with wider rights are a change to this function rather
than to each call site.

### Reversibility is exact

Reverting a session marks its operations as reverted and retracts the rules
they established. With other state unchanged, checks produce the same findings,
summary and verdict as before the session. `host/contextops_test.go` verifies
this equality.

Nothing is erased. A reverted operation stays in the log with its evidence, so
the same suggestion is recognisable the next time it is made.

## Consequences

Agents can record suggestions during normal work without affecting build
results. People can review operations together, keep suggestions by session and
revert a session when necessary.

The log is folded on every read rather than indexed. The ledger selects the
`context.*` operations alone (`Backend.Select` with a kind prefix, answered from
an index on the kind), so whatever else shares the log costs a fold nothing, and
contest detection compares only rules that share a form. With 13,000 other
operations and 500 to 800 context operations in the log, sixteen concurrent
agents record an observation with a p99 of 32 ms. An implementation that needs
more adds an index behind `Ledger` without moving the model.

### Operation ids

An operation id (`workspace.NewOpID`) is 24 characters of lower-case Crockford
base32: eight for the moment the log accepted the operation, in milliseconds
since the start of 2026, and sixteen random ones. The time comes first, so ids
sort in the order operations were accepted, and the fold reads the log in id
order. A log never mints an id that sorts before one it already holds: where the
clock has not moved past the newest id, the time part advances one millisecond
past it, so an operation recorded after another was seen sorts after it, on
whichever machine either was recorded.

The id is the same in every log that holds the operation, so two logs merge by
union (`workspace.Merge`): recording an id the log already holds changes
nothing, and two logs merged into each other hold the same operations in the
same order, whichever was merged first. An operation may also carry a content
address; a log holds one operation per address, and where two machines recorded
one address under different ids the older id stands in both. The local backend
keeps a per-log arrival position beside the id. It is what `Head` returns and
what `Since` reads from, so a surface watching for change polls one number and
an operation merged in from elsewhere moves it like one recorded here.

A log line shows the first ten characters (`workspace.ShortOpID`): the time part
and two random characters, which one machine never repeats because it never
accepts two operations in one millisecond. Every verb resolves any prefix that
starts exactly one id; a prefix that starts several is refused with the
candidates listed (`workspace.AmbiguousOpIDError`).

Context exports and snapshots carry established rules without operation ids,
so the history stays in the workspace that recorded it.

## Surfaces

`kapi context observe`, `correct`, `log`, `keep`, `drop`, `withdraw`, `revert`
and `widen` are the command-line half. `kapi context keep` takes several ids, or
`--session <id>` for everything one session suggested that nothing contests;
`--use` changes the rule as it is kept, for one id only. `kapi context log
--status` filters by any of the six statuses, and a log line prints the kind
once and marks a contested entry `[contested by #0n79tw5k9s]`. The host API
(`host/contextops.go`) is typed requests and results with no flag sets, so the
agent tools and the desktop drive the same loop.

The agent surface records and reads, one tool per habit: `context_observe`,
`context_correct`, `context_withdraw` for what the same session recorded
wrongly, and `context_session_summary`, which reports what one session recorded
and what became of it and ends with the `kapi context keep --session` command a
person reviews it with. Each wraps one host call and adds no rule of its own.
Keeping, dropping, reverting and widening are reserved for people and are
excluded from the agent tool set ([S-03](../surfaces/s-03-agent-surfaces.md)).

The server assigns the actor identity. An MCP tool takes no actor
argument and refuses one: the kind is `agent`, the name comes from the client's
own `initialize`, and the session is minted once per server process, so
operations from one run can be read and reverted together. Shell commands use
environment-based actor detection to distinguish people from supported agent
hosts.

Evidence is required where a rule is stated. `context_observe` refuses a term
rule with no `path`, and `context_correct` declares the path as a required
argument and refuses a blank one, so recorded rules have traceable evidence.
