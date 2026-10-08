---
id: c-11-context-operations
sidebar_position: 11
title: "C-11: Context operations"
description: "Architecture decision: every change to a project's context is an appended operation carrying an actor, a subject, the evidence behind it and the governance it was made against. Suggestions advise at neutral severity and can never fail a check; a person keeping one establishes the rule and writes it into the subsystem that already reads it, and a reset sets every later operation aside."
keywords: [context operations, suggestion, note, review, keep, drop, withdraw, contested, reset, widen, evidence, policy, actor, operation log, workspace, architecture decision, neokapi]
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
never fail a check. A suggestion is **established** when at least one signal
from a person backs it and nothing open contradicts it: a person keeping it, a
correction toward it, or the change reaching the default branch. Settling is a
function of the log alone, so every machine holding the same operations
reaches the same answer. An established rule fails a check unless it is marked
advisory, and is written into the subsystem that already reads it: the terms
store or the content memory. The log is the history and the source of
suggestions; established rules live in the stores.

`contextop.PersonDecides` defines the permission policy. An agent or a tool
may observe, record a correction, and withdraw its own suggestion in the
session that recorded it. Only a tool records evidence (`signal`) and the
establishments settling derives from it (`establish`). Only a person keeps,
edits, drops, imports, widens, or resets the context to an earlier point.

The command line splits along the same line. `kapi context` carries a
person's verbs (`<path>`, `search`, `review`, `reset`, `sync`) and lists the
two an assistant uses, `note` and `log`, under a heading of their own, *For
assistants*. Maintenance of the store itself (`import`, `export`, `rebuild`,
`locales`) is a separate command, `kapi store`.

## Context

Projects accumulate context through observations and corrections made during
normal writing work. Agents can propose rules with evidence; people review
those proposals before checks enforce them.

<CycleDiagram
  steps={[
    { label: "Note", sub: "a fact or a term, with evidence" },
    { label: "Suggest", sub: "advice reported by checks" },
    { label: "Review", sub: "a person keeps the rule" },
    { label: "Enforce", sub: "kapi check" },
    { label: "Note a change", sub: "what a person rewrote" },
  ]}
  caption="Notes about what the files do and what a person changed become suggestions. A person keeps a suggestion in review before checks enforce it."
/>

Suggestions must not fail builds before review. Operations must also be
attributable and reversible. The design follows the unit-decision model in
`core/state` ([C-04](c-04-block-state-and-decisions.md)): an append-only ledger
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
    Kind          Kind                 // observe | correct | import | edit | keep | drop | withdraw | reset | widen | signal | establish
    Subject       Subject              // a term rule, a content-memory pair, a note
    Correction    *Correction          // the wording before and the wording after
    Evidence      []Evidence           // file, unit, quotation: where this was seen
    Basis         Basis                // the governance in force when it was recorded
    Scope         Scope                // how far it reaches, and at which coordinates
    Target        string               // the operation this one acts on
    Before        string               // the first operation a reset sets aside
    Note          string               // why, in the recorder's words
    At            time.Time
    Status        Status               // folded from the log, never stored
    ContestedBy   []string             // the operations on the other side of a disagreement
    Established   bool                 // kept, imported, written, or settled on a person's signal
    Signal        *Signal              // the evidence a signal carries
    Because       []string             // the signals an establishment rested on
    Standing      *Standing            // the counts for and against, folded from the log
}
```

The eleven kinds, persisted as `context.<kind>`, divide into four that carry a
subject and seven that act on one.

- `observe` records something somebody noticed. Stated in prose, it is a note,
  such as who the documents address. Stated as the form the project uses for a
  word and the forms it avoids, it is a term rule.
- `correct` records that someone changed wording from one form to another at a
  location, carries both wordings in `Correction`, and may carry the term rule
  the change implies as its subject.
- `import` records one context file a person read into the project's stores,
  and `edit` records a rule a person wrote directly, with `kapi apply` or by
  editing the voice profile. Both are established from the start.
- `keep`, `drop`, `withdraw` and `widen` name an earlier operation and say what
  became of it.
- `reset` names a point in the log, the first operation it sets aside, and
  rewinds the project's context to how it stood there
  ([below](#a-reset-goes-back-to-an-earlier-point)).
- `signal` records evidence about a suggestion that nobody wrote down by hand,
  and `establish` records that settling established one
  ([below](#suggestions-settle-by-evidence)).

`contextop.Ledger` derives status by replaying the log in order and applying
subsequent operations to each subject:

| Status | What it means | Answers |
| --- | --- | --- |
| `suggested` | nothing has acted on it | advises |
| `established` | a person kept, imported or wrote it, or a person's signal settled it | fails a check unless advisory |
| `contested` | it disagrees with another rule, named in `ContestedBy` | advises |
| `withdrawn` | its author took it back in the session that recorded it | silent |
| `dropped` | a person set it aside, as a suggestion or as a rule in force | silent |
| `reset` | a reset set it aside | silent |

### An observation states a term rule by its forms

`kapi context note --term Quickcast --instead-of "Quick cast"` records the
form the project uses and a form it avoids. `contextop.AvoidedForms` derives
the rest deterministically, with no model: the `--instead-of` forms, then the
spacing, hyphen and capitalisation variants of a compound. The rule above
avoids `Quick cast`, `Quick-cast` and `QuickCast`.

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

### Suggestions settle by evidence

`contextop.settle` runs inside the fold, after every person's decision has been
applied and before disagreements are marked. It groups the suggestions that
state one rule (the same form to use, a form to avoid in common, at points that
meet) and reads the evidence for the group from the log:

| Signal | Recorded as | Effect |
| --- | --- | --- |
| A person keeps it | `keep` | Establishes it at once |
| A correction toward it, by a person or by an agent recording what the person changed | `correct` | Person signal |
| The change reached the default branch | `signal` with source `merge` | Person signal |
| Another session records the same rule | `observe` | Standing only |
| The content writes the preferred form | `signal` with source `usage` | Standing only |
| An agent's applied edit writes the preferred form | `signal` with source `applied` | Standing only |
| A correction away from it | `correct` | Against |
| Its author withdraws it | `withdraw` | Against |
| The content moves to a rejected form after it was recorded | `signal` with source `usage` | Against |

The group shares one standing, and each member is settled on the person
signals seen **where that member holds**. A correction counts for a member whose
scope covers the point the correction was recorded at (`seenWithin`), and a
merge signal names the member whose scope the change's files sat in, so it
counts for that member alone. Suggestions at a point with no coordinates meet
every other point, so a group can span the help pages and the legal terms
through one of them; the evidence is still read per member, and a correction
in the help pages establishes the help pages' suggestion and leaves the legal
terms' suggestion as it was.

A member with a person signal and nothing against it is established. A member
with a person signal and a signal against it, or a rival rule that disagrees
with it, is contested and waits for a person. A member without a person signal
stays a suggestion whatever its standing, and time alone settles nothing. The
correction a suggestion was drawn from is not evidence for that suggestion
alone. A person dropping the rule sets aside what was recorded before the
drop, together with the evidence gathered before it.

An agent's suggestion applies only where it was seen until a person widens it
in review (`placed`). One that names no file it was seen in has no place, so
settling never establishes it; a person keeps it and chooses where it holds.
Settling records an `establish` with the suggestion's own scope and never
changes it: evidence establishes a rule where it was seen and widens nothing.

When the log supports establishing a suggestion that no `establish` names yet,
`Ledger.Settle` records one: actor `tool settle`, the suggestion as its target,
and the person signals it rested on in `Because`. Its content address is
derived from the target and that evidence, so two machines settling the same
log record one operation. The host lands the rule the way a keep does, and a
later signal against it takes it back out of the store through the same
reconciliation a contesting correction uses. Because the fold reads the
evidence again on every read, an `establish` whose evidence no longer holds
stops binding, and the outcome is the same whatever order two logs were merged
in.

`Record.Standing` carries the counts behind each suggestion, and every surface
shows them as counts, never as a score: `seen in 3 sessions · 14 of 15 uses in
docs/ · merged in #412`.

`kapi context sync --merged <range>` records the merge signal
(`host.App.SettleContext`, which `SyncProjectContext` runs between its pull and
its push, and alone in a project that shares its context nowhere). It reads the
diff the range made inside the project, and for each suggestion whose scope
covers a changed file it counts the added lines that write the preferred form
and the removed lines that held a rejected one. A suggestion with either count
above zero gets a `signal` naming the commit, the pull request (read from a
squash or merge commit's subject when not given) and the merger. The signal's
address covers the suggestion, the commit and the counts, so settling one range
twice records nothing new. CI runs it after a merge or a push to the default
branch ([Convergence in CI](/kapi/convergence-in-ci)).

The other signals are recorded where the content is read. A whole-project
`kapi check` and a `kapi up` run count, for each suggestion and each
established rule whose scope covers the content, the uses of its preferred form
and of the forms it avoids in the source they read, and record a `usage` signal
as tool `check`; a check of named files or of a diff records none, because part
of the content says nothing about how the project writes. A rule's counts are
recorded only when they differ from the latest recorded for it, so a run over
unchanged content adds nothing to the log. For a suggestion the
count is standing. For an established rule, a later count that writes a
rejected form more often than the count taken when the rule came into force is
drift, which the review digest reports ([S-07](../surfaces/s-07-context-centric-review.md)).
An agent's content edit applied through `kapi apply` or the `apply_edits` tool
records an `applied` signal as tool `apply` for each suggestion whose preferred
form the edit writes, naming the agent's session.

### Disagreements are contested until a person chooses

Two rules disagree when they share a form to avoid and name different forms to
use, or when one avoids the form the other says to use. Rules at points that
cannot meet, in different projects or at different coordinates, never disagree.
The fold marks a disagreement `contested` and names the other side in
`ContestedBy`:

- Two suggestions about the same word are both contested, each naming the
  other. Both advise, and neither can be kept until a person chooses one:
  `host.App.ChooseContextSide` (`kapi context review --choose`) drops the
  rivals and keeps the chosen side, one operation per step.
- A suggestion that contradicts an established rule is contested by the rule,
  and the rule stays in force.
- A person's correction that reverses an established rule contests the rule,
  and the rule contests the correction. The rule is taken out of the terms
  store and reports instead of failing, until a person keeps it again or drops
  the correction, so a person's own edit never fails their build. Keeping the
  correction is a choice like any other side's: `review --keep` refuses it and
  names the rule, and `review --choose` drops the rule and keeps the
  correction.

`kapi context review --session <id> --keep all` keeps everything one session
suggested and leaves each contested suggestion for later, naming the other
side.

### Review is where a person decides

`host.App.DecideContextReview` takes one round of decisions
(`ContextReviewRequest`: `Keep`, `Drop`, `Choose`, `Session`, `WidenTo`,
`Replacement`, `Advisory`, `Note`) and records them in a fixed order: the
choice, then the drops, then the keeps, so a keep never meets a rival the same
round drops. `kapi context review` reaches it two ways. In a terminal it walks
the digest one item at a time and asks; elsewhere it prints the digest, and
the decisions arrive as flags. Both record the same operations.

A digest item says what a person may do with it: `Droppable` holds for a
suggestion and for a rule in force, and `Widenable` for a rule in force that
can apply more widely. Dropping a rule in force records a `drop` and takes the
rule back out of the stores keeping wrote it to
(`host.App.DropContextOperation`). There is no undo for a single decision:
changing your mind about a rule is a new decision, recorded beside the one it
replaces.

### Keeping writes through the existing appliers

`kapi apply` is the one write verb, and its asset operations write the project's
terms store or content memory ([C-08](c-08-terms.md),
[C-09](c-09-content-memory.md)). A word rule is a term, so keeping one writes a
concept, marked advisory when the rule is. Keeping a
rule builds the same change-set entry and runs the same applier, so retrieval,
checks, drafting, the governing fingerprint and every machine that pulls all see
it with no second code path. A term rule with several forms to avoid lands each
form, and a form that differs from the form to use only in case stays out of
the store, which folds case.

Dropping an established rule reverses it against the same stores: the term is
deleted from the terms store, or the pair from the content memory. Other terms in the concept are preserved because deletion targets the term.

Every one of these store writes goes through the projector
([C-03](c-03-context-store-and-graph.md#the-stores-are-projections-of-the-log)),
so the log holds each rule twice over: the context operation that decided it,
and the `terms.write`, `memory.write`, `voice.write` or `rules.write` operation
carrying the rows the decision wrote, whose origin names the operation it
carries out. The context operations fold into statuses; the store operations
replay into the stores.

A call that lands a rule in a store or takes one out also refreshes the
project's rules files when it ends, so the `AGENTS.md` and `CLAUDE.md` an agent
loads state the rules now in force ([C-06](c-06-retrieval.md#rules-files)).

`kapi apply` records one `edit` operation for each term or content-memory
operation it applies, established from the start and attributed to the person
who ran the command. The transport stamps the actor and a change set has no
field for one: `kapi
apply` stamps the actor the environment names, as every `kapi context` command
does, and `apply_edits` stamps the calling agent and the server's session. The
policy refuses a term, content-memory or recipe entry from an agent before
anything is written, so an agent records a note instead.

### Reading a checkout's context files is an operation too

`kapi store import` is the one command that opens a context file in a
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

A rule lands in the terms store at its point too. Keeping or settling a rule
whose evidence was seen where a profile governs writes its concept with that
profile in `terms.PropProfile` and the point's coordinates in
`terms.PropCoordinates` (`channel=app,product=quickcast`). `kapi store import`
scopes the word rules in a profile's own voice or terms file under
`.kapi/profiles/<name>/` to that profile. Both writes go through the projector
like every other store write. `projectConcepts` keeps, at a point, the concepts
that hold there (`terms.AtPoint`): scoped to no profile or to the profile
governing there, and to no coordinates or to coordinates the point sits at. A
rule kept from an app's strings therefore says nothing about the same
product's documentation, and checks, retrieval and the translation tools all
read the same filter. The project-wide answer, asked with no point, lists every
concept.

An answer for a point names the rules held at the project's other places and
not there, as the wording that stays correct
([C-06](c-06-retrieval.md#the-prose-is-the-task)). Without that line an agent
told to rename something everywhere renames it at a point the rule never
reached, and its note of the rename there would then be a suggestion at that
point too.

Keeping is also the moment a person may **widen**, with
`kapi context review --keep <id> --widen-to <where>`; the same flags on a rule
already in force record a `widen`. The steps nest:

| `--widen-to` | What the rule's scope drops | Where it then holds |
| --- | --- | --- |
| an axis, such as `channel` | that axis; `product` drops the profile too | every value of the axis, at the rest of the point |
| `project` | the profile and the axes it derives (`product`, `channel`) | every point of the project |
| `workspace` | the profile, its axes and the project | every project of the workspace |

An axis no profile derives, such as a brand the recipe declares, stays through
`project` and `workspace`, so a rule widened from one brand's project holds
wherever that brand does. Widening moves the rule's concept with it: the
concept a keep wrote is rescoped to the wider point in the same write.

Workspace-wide rules use `workspace.Rule` rows in `workspace.db`. Each row
contains an id, kind, source project and opaque JSON payload. This keeps
workspace storage independent of the rule type. Project-specific terms take
precedence over workspace rules for the same term, and `kapi context search`
reports a workspace rule beside the project's terms, marked as holding across
the workspace.

A widened rule is written through the projector as a `rules.write` operation
recorded under the project that decided it, so it travels like every other
operation of the project: a push carries it, a pull or an import applies it to
the receiving machine's workspace, and a checkpoint holds the project's widened
rules. Narrowing it, by dropping the rule or contesting it, travels the same
way.

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
correction, and each takes effect as a suggestion. A tool records evidence and
the establishments settling derives from it; an agent may record neither,
because an agent's statement is a suggestion and never evidence for one. The author of a suggestion
may withdraw it in the session that recorded it, which is how a session cleans
up after itself. Everything that turns advice into a rule, or sets another
actor's advice aside, belongs to a person: keeping, editing what another actor
suggested, importing, writing a rule directly, dropping a suggestion or a rule,
widening, and resetting.

`drop` and `withdraw` both set something aside, with different permissions. A
person drops a suggestion or a rule in force, and a dropped rule leaves the
stores. Its author withdraws a suggestion, and only while it is still one.

Widening is a property of the transition rather than of the verb, so the check
covers every route to it. `Ledger.Append` raises `Widening` for a `widen` and
equally for a `keep` that names the workspace, and a `Widening` transition is
refused for any actor but a person. Two sessions of one agent count as two
parties, so an agent acts on its own suggestion and on nobody else's.

Every writer goes through it: `kapi context`, `kapi apply`, the agent tools, the
desktop feed. Agent roles with wider rights are a change to this function rather
than to each call site.

### A reset goes back to an earlier point

`host.App.ResetContext` (`kapi context reset --before <point>`) rewinds a
project's context to how it stood at a point in its history: before an agent
session's first operation, before a date or an instant, or before one
operation, named by id or by an unambiguous start of one. The request resolves
to the first operation set aside, and the `reset` operation records it in
`Before`. `ContextResetScope` answers the same request without recording
anything, which is `--dry-run`.

`workspace.SetAside` decides what a reset sets aside, and both readers of the
log call it, so they agree. It reads a project's operations newest first, in id
order. A reset still in force sets aside every operation of a context kind in
its project from the operation it names up to itself: the `context.*`
operations, and the `terms.write`, `memory.write`, `voice.write` and
`rules.write` operations that fill the stores. Reading resumes below that
point, so a reset inside the range a later reset covers is itself set aside
and has no effect. A document's edits, adoptions and review decisions are the
project's content, and a reset leaves them alone.

What a reset sets aside stays in the log:

- The ledger folds every set-aside record to status `reset`. It stops
  answering, and `kapi context log --status reset` lists it.
- A projector rebuild skips the set-aside operations, so the stores stand as
  they did before the point. `ResetContext` runs that rebuild once the reset is
  recorded.
- A rebuild starts from no checkpoint that a reset was received after, because
  the reset changes what the operations before it apply. It starts from an
  older checkpoint, or from an empty store.
- A catch-up that meets a reset, or an operation that sorts before a reset the
  log already holds, rebuilds instead of applying operations one by one. This
  covers an operation merged in from another machine that falls inside what a
  reset set aside.

A reset is itself an operation, so a later reset to before it sets it aside
and brings back what it set aside: `kapi context reset --before <reset-id>`
takes one back. `ContextResetResult` reports the subject-bearing operations set
aside, a count of the decisions set aside with them, and the ones an earlier
reset had set aside that answer again.

A reset is for going back to a known good state. A single decision has no
undo: changing your mind about a rule is a new decision in review.

Nothing is erased. A set-aside operation stays in the log with its evidence, so
the same suggestion is recognisable the next time it is made.

## Consequences

Agents can record suggestions during normal work without affecting build
results. People can review operations together, keep suggestions by session,
and reset the context to before a session when necessary.

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

## Surfaces

The command line divides by audience:

| Audience | Commands | What they do |
| --- | --- | --- |
| A person | `kapi context <path>`, `search` | read what applies |
| A person | `kapi context review` | keep, drop, choose a side, apply more widely |
| A person | `kapi context reset --before <point>` | go back to an earlier point |
| A person | `kapi context sync` | pull, settle a merge, push |
| An assistant (*For assistants*) | `kapi context note`, `log` | record, and read back |
| Maintenance | `kapi store import`, `export`, `rebuild`, `locales` | the store itself |

`kapi context note` records an `observe` for something noticed (`--term` with
`--instead-of`, or a fact in prose), a `correct` for a person's change
(`--from`, `--to`, and `--suggest` for the rule it implies), and a `withdraw`
for a note of the caller's own (`--withdraw <id>`). `kapi context log
--status` filters by any of the six statuses, and `--session this` reads back
what the calling session recorded. A log line prints the kind once, marks a
contested entry `[contested by #0n79tw5k9s]` and ends a suggestion's line with
its standing. The host API (`host/contextops.go`, `host/contextreview.go`,
`host/contextreset.go`) is typed requests and results with no flag sets, so the
agent tools and the desktop drive the same loop.

Operations are what travel between machines. `host.App.SyncProjectContext`
(`kapi context sync`) pulls another machine's operations from the backend
`context.backend` in `kapi.yaml` declares and merges them by id, settles a
merge when `--merged` names one, and pushes this machine's operations, unless
`--no-push` asks it only to read. The recipe is the only place the backend is
set. `kapi store export` carries the
operations in one file, and `kapi store import` merges such a file the same
way ([C-03](c-03-context-store-and-graph.md)). A suggestion recorded on one
machine and kept on another is therefore one history, whichever pulled first.

The agent surface records and reads with two tools: `context_note`, which
records what the session noticed, a person's change (`from`, `to`, `suggest`),
or the withdrawal of the session's own note (`withdraw`), and
`context_session_summary`, which reports what one session recorded and what
became of it and ends with the `kapi context review --session <id>` command a
person reviews it with. Each wraps a host call and adds no rule of its own.
Keeping, dropping, widening and resetting are reserved for people and are
excluded from the agent tool set ([S-03](../surfaces/s-03-agent-surfaces.md)).

The server assigns the actor identity. An MCP tool takes no actor
argument and refuses one: the kind is `agent`, the name comes from the client's
own `initialize`, and the session is minted once per server process, so
operations from one run can be read together, and the context reset to before
them. Shell commands use
environment-based actor detection to distinguish people from supported agent
hosts. A person is named by their git identity (`user.email`, or `user.name`
when no email is set), so two people on two machines of one team are two
actors: `kapi context log --actor` tells them apart, and the digest shows a
teammate's operation under the teammate's name and the reader's own as "you".
A machine with no git identity records the person unnamed.

Evidence is required where a rule is stated. `context_note` refuses a term
rule or a person's change that names neither a `path` nor a `quote`, so recorded rules have
traceable evidence.
