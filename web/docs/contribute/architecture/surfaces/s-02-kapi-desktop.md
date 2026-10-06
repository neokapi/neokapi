---
id: s-02-kapi-desktop
sidebar_position: 2
title: "S-02: Kapi Desktop"
description: "Kapi Desktop uses the shared host runtime for project management, context review and content processing, with a Wails v3 backend and React frontend."
keywords: [neokapi, architecture decision, Kapi Desktop, Wails, desktop app, module isolation, workspace, context hub, context operations, point map, governance, review, credential vault]
---

import { SwimlaneDiagram } from "@neokapi/docs-shared";

# S-02: Kapi Desktop

## Summary

Kapi Desktop is a Wails v3 application at `apps/kapi-desktop/` (module
`github.com/neokapi/neokapi/kapi-desktop`). It opens on the **workspace**: every
project this machine account has run kapi in, whichever surface ran it
([C-03](../context/c-03-context-store-and-graph.md)), beside a feed of what
people and agents have recorded about their context
([C-11](../context/c-11-context-operations.md)). Projects open from there as
tabs, several at a time, and each gets a visual surface: a project home that
opens on what the project stands at and a map of its coordinate points, a
Context hub that opens on the digest of what kapi learned about how the project
writes, beside the graph explorer, the voice profile, the terms and the content
memory, a checks panel, a
review queue, governance editing on the recipe, a runner that brings the project
up to date through the same venue the CLI uses, a tool reference, and an
OS-keychain credential vault. It depends on the framework and `host` only. It
links neither Cobra nor the `cli` module, and no package from the platform
layer, which `make audit-modules` asserts by inspecting the transitive package
list of `./backend/...`.

## Context

Everything the desktop does is available from the CLI. What it adds is the
affordance: the whole resolved voice profile on one page instead of a YAML file
to read, a map of where each collection is governed instead of a recipe to
trace, a form driven by a tool's JSON Schema instead of a remembered flag, a
progress view instead of a log tail, and a place to paste an API key that is
not shell history.

Nothing about that needs a second implementation. `host` is cobra-free.
Command threading is the `host.Command` interface, which `*cobra.Command`
satisfies natively and which `host.EnvCommand` satisfies for embedded runs, so
the desktop calls the same functions the CLI does, with a different thing on
the other end of them.

The constraint that shapes the module boundary is the reverse direction: Wails
brings a large native toolchain, and the CLI must never acquire it. So the
desktop is its own Go module, and the boundary is asserted rather than trusted.

## Decision

### Stack and module boundary

- **Backend**: Go, with Wails v3 generating TypeScript bindings from the
  exported service methods.
- **Frontend**: React 19, Vite, Tailwind CSS 4, shadcn-based primitives.
- **License**: Apache-2.0, matching the framework.

The module depends on the framework, `host` and Wails, and on no package from
the platform layer. The licence boundary has no exception: a type both the
desktop and the platform need lives below the line, in the framework or in
`host`.

```
apps/kapi-desktop/
├── go.mod                   # module github.com/neokapi/neokapi/kapi-desktop
├── main.go                  # Wails entry point, application menu, embedded assets
├── backend/                 # one flat Go package: the bound service
├── frontend/                # React + Vite; bindings/ is Wails-generated
└── build/                   # per-platform build configuration
```

Four assertions run in `make audit-modules`: an isolated `GOWORK=off` build of
`./backend/...` (so the module resolves against its own `go.mod`, not the
workspace overlay), a tidiness check that fails on any stale or
boundary-crossing requirement, an import-level check that the transitive
package list contains no Cobra and no `cli` package, and the same check for any
platform package.

### One bound service, many open projects

`backend.App` is the single Wails service. The frontend calls its exported
methods; there is no second RPC layer and no hand-written binding code. Within
that one service the concerns are split across files: project lifecycle and
status, the project's coordinate points, the context graph and the voice
profile per point, governance edits on the recipe, flow CRUD, the runner, the
catch-up loop and its venue, checks, the review queue, inspection, content
memory, terms, media, outputs, the sample project, plugins, credentials and
model detection, locales, settings, the workspace home and its watcher, the file
watcher, and the in-app updater.

Projects open as **tabs**. Every project-scoped method takes a `tabID`, so the
open set is real state in the backend. Each tab carries its own loaded recipe,
project context, and resolved stores, and closing one releases them. A file
watcher refreshes the tab when the recipe changes, including edits made through
the CLI or git.

### Every edit is a change set

The desktop changes content through the change service and nothing else
([E-09](../engine/e-09-the-change-contract.md)). Four bindings carry the
contract: `Read`, `Apply`, `Describe` and `History`, each taking and returning
its JSON as a string that the frontend types with `@neokapi/contract-types`. A
string, because an operation is a union that marshals itself, and a model Wails
generated from the Go struct would describe its fields instead. The backend
builds the service for the tab's project with `desktop` as the origin, so the
commit check governs every edit and the block history records it, and it sends
every change set as the person at the keyboard.

A surface that changes content reads the block it shows through `Read` and sends
what the person does with the revision it read:

- The Review page edits a translation, or a source row's wording, in the
  inline-code editor: each code is a chip the person types beside, and the text
  goes back in the placeholder form the read showed, so a code the edit kept is
  the code the edition holds. A plural or select is edited a branch at a time, each branch in
  the same inline-code editor (inside `PluralTargetEditor` for a plural), so
  every code keeps its id whatever it displays, and each changed branch is a
  `set_content` addressed by the path the read lists. An AI proposal is accepted as the same
  `set_content`.
- Approve and Reject send `decide` with the outcome `establish` or `reject`, Reject with the reviewer's note on the
  change set. A parked draft, which reads as `absent`, is decided as `absent`.
  Approving the clean blocks of a view reads each one as it approves it and runs
  its checks again; a block whose text is no longer what its row lists, or that
  now trips a check, stops the batch with its row brought up to date.
- A check finding whose rule names a replacement carries its fix, the
  `replace_text` `kapi check` reports, and the Checks panel sends it as it is.
  A finding whose words have an inline code among them carries none.
- The document view puts a clicked block in focus and edits its source or a
  translation under the document ([S-06](s-06-visual-editor.md)). A translation
  is edited with its own plurals and codes, which the read lists beside the
  block.

When the content changed after the surface read it, the change service refuses
the change with the edition as it stands. The surface shows that text under
"Changed since you opened it" and asks before sending the change again over it,
with what the editor holds at that moment. When a rule in force fails on the
wording a person saves, the change service refuses the change as `gate_failed`
with the findings. The editor shows them under "A rule in force fails on this
wording" and offers "Save anyway", which sends the change set it refused again
under `gate: report`: the edit lands with its findings, and the record of it
lists them as overridden ([E-09](../engine/e-09-the-change-contract.md)).
Changing the wording puts the prompt away, and the next save is checked again,
so an override only ever carries findings the person was shown. The desktop
sends every change set as the person at the keyboard, and the change service
refuses `gate: report` from an agent. A change set naming one translation is
read and written with that translation's language, as kapi apply edits it: a
PO catalog that holds its translation is edited in place, and the French of a
catalog whose collection names a target lands in the file the target names
(`po/fr.po`), never in the source catalog. A check's fix sent again finds its
words by the text the finding quotes, and is offered only while those words are
plain text in the text as it stands. The
Review page lists the edition's recorded changes under its provenance, read from
the block history through `History`, and its provenance names the person or the
agent whose recorded change produced the text in force.

### The app opens on the workspace

The first screen reads the workspace's project registry
([C-03](../context/c-03-context-store-and-graph.md)): every project this machine account
has run kapi in, whichever surface ran it. A project registers the first time
kapi opens it, so the desktop lists a repository set up from a terminal without
being told about it. The app registers too, from `OpenProject` and `NewProject`,
through `workspace.Register` rather than through the project store, because
opening a project to look at it is no reason to create `.kapi/work/store.db`
inside somebody's checkout.

A registration carries the project's identity, its display name, the checkout
paths it has been seen at on this machine and its last-active time. Checkouts
are a list rather than a field, so two worktrees or two clones of one repository
are one row with two places to open it from. A checkout whose recipe has gone is
marked missing and kept: the project and its context are untouched by a folder
disappearing, and dropping the row would erase the only record of where it was.
A project with no checkout here opens as a **context-only tab**: it carries no
recipe and no files, its terms and content memory are bound straight to the
workspace's context store as borrowed handles, and the Context hub drops the
sections that read a checkout.

`Workspace.Forget` deletes the project registration, checkout records and
context store after confirmation. The confirmation identifies the database to
delete and the checkout folders to preserve. Closing a tab, deleting a checkout
or resetting a sample leaves the registration and context intact.

The desktop detects changes made by the CLI and MCP server through the shared
workspace operation log. A goroutine started in `ServiceStartup` polls
`workspace.Head` once per second and emits `workspace:changed` when the revision
changes, triggering a frontend refetch. Reopening an unchanged registration
leaves the revision unchanged. This requires one indexed query per second and
avoids interpreting filesystem events from SQLite database and WAL files.

### The project home and the point map

A project tab opens on the project's **standing**, in the two-axis shape
`kapi status` prints: content on one row, governance on the other, and the
venue and stream on the identity line. Below it, the point map lists every
coordinate point the recipe declares ([C-02](../context/c-02-coordinates-and-governance.md)),
the collections at each and the voice profile governing there. A row opens the
Context hub standing at that point. A single-point project gets one row rather
than no map.

Between the standing and the point map, the project lists the **conflicts**
`kapi status` reports, when there are any, block by block with both wordings
(`GetKeptConflicts`). One kind is an edit another machine made to a draft the
workspace keeps, which a pull merged and which could not land: the workspace
holds one wording and that edit another. The other is wording a person kept in
the workspace whose translation's file has appeared since without it: the file
holds one wording and the workspace another
([C-03](../context/c-03-context-store-and-graph.md#the-workspace-home)). The
person keeps either side or writes a new wording, and each choice that writes
is a `set_content` sent through `Apply` with the revision the conflict shows as
held, so a block that moved meanwhile is refused stale and shown again. Keeping
the wording the workspace holds is recorded as the person's decision, and a
translation whose file exists has the workspace's copy released once the file
holds the chosen wording (`ReleaseKeptWording`).

A third kind is a version of a document a KPZ in the project carries that was
written from an older version than the document holds now, such as a `.kpz`
replaced on disk while its cache held unpacked edits
([E-09](../engine/e-09-the-change-contract.md#homes)). It lists no blocks at
first. The person rebases it (`RebaseKeptDocument`), which carries its changes
over onto the document through the change service and lists each block both
versions changed, decided as above, or discards it (`DiscardKeptDocument`),
which keeps the document as it stands. A rebase the change service refuses,
for a stale block or a failing gate, shows the reason and settles nothing.

The workspace home lists the same kind for every `.kpz` on this machine whose
working cache holds edits, inside a project or outside every project
(`GetWorkspaceDocumentConflicts`). The caches are the user's own, one per
`.kpz` and keyed by its absolute path, which their metadata records, so the
list needs no project open, and each document is named by that path. Rebasing,
discarding and deciding a block (`RebaseWorkspaceDocument`,
`DiscardWorkspaceDocument`, `ApplyWorkspaceDocument`) go through the change
service of the project the `.kpz` sits in, or of its directory when it sits in
none.

The backend resolves the declared cross product, the project's own point and
then each profile's channels, and lists a collection where it is actually
governed: a collection whose profile's window has closed appears at the point
that governs it, not where it was written.

### The Context hub

The stores are sections of one hub, beside the graph:

| Section | What it shows |
| --- | --- |
| **Learned** | the digest of what kapi learned about how the project writes since the person last looked (below), where a person reviews it |
| **Explorer** | the context graph, through `@neokapi/context-explorer`; the governs pane renders the same guide the retrieval surface serves ([C-06](../context/c-06-retrieval.md)) |
| **Agent View** | the resolved context for one file as an agent receives it: `AgentContextAt` calls the desktop's `ContextAt` and renders the answer's own `FormatText`, so the body on screen is the `context://` resource body rather than a second rendering of it |
| **Voice** | the whole resolved profile per point: tone, style patterns with severities and rates, term rules, examples, and the locale, channel and persona overrides as authored ([C-07](../context/c-07-voice-profiles.md)); the profile is edited here |
| **Terms** | the terms store: search, facets, provenance, concept relations |
| **Content Memory** | the content memory: search, facets, activity, provenance, the languages gate |

The voice section resolves every point the recipe declares twice: as declared,
and at the governance instant. A binding whose window excluded the instant is
drawn as a skipped rung with its boundary date and what governs in its place.

The terms and memory pages browse and audit. Moving store contents in and out
as files is the CLI's job (`kapi memory`, `kapi terms`), so the desktop offers
no import or export control, and `scripts/check-desktop-interchange.sh` keeps
one from returning. The view ids the store tabs had before the hub stay
routable so a
persisted view still lands, and the ad-hoc rail keeps both stores, having no
project to file them under.

### The feed of recorded context operations

The app is meant to sit open beside an agent that is working. The agent runs in
another process, through the CLI or its MCP server, and records what it learns
as context operations in this machine account's workspace log
([C-11](../context/c-11-context-operations.md)). The desktop reads that log and
shows it in two places: as a feed of every project's operations on the
workspace home, and as each project's digest in its Context hub.

The feed groups by session. An agent session is the id its operations carry, so
one run is one card however long it lasts. A person at a terminal and a tool
record no session, so their operations group by actor and local day, which is
the span a person recognises as their own work. Each card carries the counts a
finished session reads out in one line (recorded, corrected, kept and dropped),
and says "still working" until nothing has been added for two minutes.

Each operation shows its kind, its actor with the machine an agent ran on, the
rule or wording it is about, its status (`suggested`, `established`,
`contested`, `withdrawn`, `dropped` or `reverted`, with a contested entry
reading "Contested by #n"), and its evidence: the file, the block and the
quotation. Evidence is on the card rather than behind a disclosure for
anything awaiting a decision, so reviewers can assess the supporting text.

**Deciding goes through the host API and nothing else.** Keep, keep with an
edit, drop, revert one operation, revert a session and widen are the backend's
`KeepContextSuggestion`, `DropContextSuggestion` and their neighbours, which
call `host.App.KeepContextOperation` and the host's other context operations.
The host owns the policy about who may do what; the desktop re-implements none of it. The acting actor is
`contextop.Actor{Kind: ActorPerson}` with no name, because the app holds no
account and the person at the keyboard is the one acting. Reading is
`contextop.Ledger` over the workspace, because the host's reads resolve a
project from a recipe path and the home screen spans projects, some of which
have no checkout on this machine. A project with no readable checkout is read
here and not decided on, and the card says so.

Widening requires confirmation. The preview lists affected projects for
workspace scope, or newly covered recipe points when removing an axis
restriction. It explicitly states that affected content has not been counted;
the backend does not provide that count.

The keys are the review session's, so the two decision surfaces feel the same:
`j`/`k` and the arrows move over the suggestions, `a` keeps, `r` drops, `e`
opens the edit, and a field with focus keeps its own keys. A contested
suggestion can be dropped at once and kept only after a person drops the other
side. The workspace watcher is what keeps the feed current, so a suggestion
recorded elsewhere appears within a second. The previous answer stays mounted through the refetch, which is what
keeps the scroll position and a half-typed edit through an agent recording in
the middle of it.

Beside each project on the home sits what its digest holds that the person has
not seen, "4 new since Tuesday" (`host.App.ContextNews`, one fold of the log
for every project). A project with nothing new shows nothing: the home carries
no count of work waiting.

### The digest

A project's Context hub opens on **Learned**, the digest of what kapi learned
about how the project writes. Review there is discovery, never a gate: a
suggestion advises agents and checks from the moment it is recorded, and one
nobody answers keeps advising, so the digest reads as news and carries no count
of unread work. `host.App.ContextDigest` assembles it from the operation log,
the same call `kapi context digest` prints, and the desktop renders it in five
sections, in this order:

| Section | What a person does |
| --- | --- |
| **Needs you** | the conflicts: two rules that disagree about one word, or a rule the evidence turned against. Choosing a side keeps it and sets the others aside (`ChooseContextSide`) |
| **Established** | the rules that came into force, with what they rest on ("merged in #412", "your correction in docs/billing.md", "kept by you"). Revert takes one back out |
| **Suggested** | suggestions grouped by theme (names and spellings, words to avoid, how the project writes, wording in other languages), then by collection where they sit in more than one. Keep, change then keep, drop, or keep a whole group (`KeepContextGroup`) |
| **Drift** | an established rule whose latest usage count (from a whole-project `kapi check` or `kapi up`) writes a rejected form more often than the count taken when the rule came into force, or the first count after it |
| **Numbers** | "kapi knows 23 rules for Fernwell; 4 are new this week" |

Each item states its rule as a sentence ("Write Quickcast, not Quick cast or
QuickCast"), the quotation it was seen in with a link that opens the explorer at
that file, its standing as plain counts, and who noticed it. Where a check has
counted the rule's forms, the item carries the project's own words back to it:
`docs/ says "studio" 41 times and "business" twice`.

"Since you last looked" is a marker per project in this machine account's kapi
config (`context-digest.json` under the config root), never in the log. The
panel reads the digest from the marker once when it opens, holds that instant
for as long as it stays open, and moves the marker to now, so what was new when
the person arrived stays marked new until they leave. Items the person has seen
stay in the digest under "Earlier". A digest with nothing new says "Nothing new
since Tuesday" beside the numbers.

The keys act on the item under the cursor: `j`/`k` move, `a` keeps (or chooses
a side), `c` changes the form to write and keeps, `d` drops, `g` keeps the
group, `u` reverts a rule in force, and `o` opens the file. A project with no
checkout on this machine shows its digest without the decisions.

### Governance editing

Project Settings carries a Governance section: the voice profile bound as
`defaults.voice`, the declared axes of the project's default point, the default
flow, and the paths a content scan skips. The collection editor sets the
channel a collection names and the axes it declares there, and a collection row
shows the point it resolves to.

The refusal for the structural axes has one home. `product` and `channel` are
derived from a collection's channel, so the setter calls
`project.DeclarableAxis` and the desktop serves the same error the CLI does;
the editor cannot offer an axis `kapi apply` would reject, or word the refusal
differently ([C-02](../context/c-02-coordinates-and-governance.md)).

### Runs go through the up venue

"Bring up to date" calls `host.App.RunUpDispatch`, which resolves where the run
executes through `host.App.ResolveUpVenue`: locally, or at the convergence
venue the recipe binds. The CLI and the MCP `up` tool take the same route, so a
project bound to a venue converges there from every face and the server keeps
the state a team reviews from.

Live progress survives the dispatch. The venue route reads the plumbing's
NDJSON line by line as it arrives, folding framing records into the result and
handing the run's own events to the run view; a buffered document and a
streamed one are read by the same fold. The shared engine discovers plugins
once, and a run borrows that engine, so the venue a run resolves is the one the
tab's other surfaces see.

<SwimlaneDiagram
  actors={[
    { label: "Frontend", sub: "React", role: "io" },
    { label: "backend.App", sub: "Wails service" },
    { label: "Executor", sub: "host + framework", role: "translate" },
  ]}
  messages={[
    { from: 0, to: 1, label: "RunFlow(tabID, flow, inputs, locales)" },
    { from: 1, to: 2, label: "execute", detail: "flow spec + project context" },
    { from: 2, to: 1, label: "trace records", detail: "step, block, log, error" },
    { from: 1, to: 0, label: "flow:event", detail: "emitted + buffered for reconnect" },
    { from: 0, to: 1, label: "CancelRun()" },
    { from: 1, to: 2, label: "context cancel" },
  ]}
  caption="A run is one call in and a stream of events out. The backend buffers the stream so a reloaded window replays the run rather than losing it."
/>

A desktop window can reload mid-run, and a run that only existed as a live
event stream would vanish. The backend keeps the events, collapsing consecutive
metric snapshots so a long run does not grow the buffer at the sampling rate.
The runner asks for a target language only when the flow produces one.

### The toolbox is a reference; flows run through the runner

The Toolbox page describes each registry tool: what it does, its schema, what it
consumes and produces, its reference page, and the command that runs it
(`kapi exec <tool>`, with `kapi tools schema <tool>` for its options). Tools
execute through the CLI and through flows; the desktop runs no tool on its
own, because a third path would have to re-solve project isolation and AI
consent for little.

A project's flows page lists what `kapi flows` lists for the project: the
recipe's inline flows, then the files in its `flows_dir:` that no inline flow
shadows, a file that will not load shown with its problem. `RunFlow`,
`GetFlow` and the preview resolve a name the way `kapi run` does
(`host.ResolveProjectFlow`, [E-04](../engine/e-04-flows-and-io-binding.md)).
The editor saves inline flows into the recipe and opens a flow file read-only,
since the file is where that flow is edited.

The flow editor's Run action goes through the same `RunFlow` path the
collections table and the project home use. It appears in project mode, where
there is a project for a run to act on, and stays absent in ad-hoc mode.

The flow editor's graph edits **composition** only. Its two ends are endpoint
pickers (file, store, interchange, or none), not draggable reader and writer
nodes, because the binding is a property of the run rather than of the flow
([E-04](../engine/e-04-flows-and-io-binding.md)).

### The sample project

A first-run sample project is scaffolded from `backend/sample/`: a governed
recipe, and context read into the project's stores from a `.terms.json` bundle,
a voice profile and a `.memory.json` bundle, the native serializations
([M-06](../multilingual/m-06-content-packages.md)), loaded with the same reader
the rest of the tree uses. Seeding bulk-loads the entries and then rebuilds the
search and fuzzy side-tables, so the sample's memory answers search as well as
exact lookup.

### Three faces, one record

kapi answers the same questions from the CLI, over MCP, and from the desktop.
A face parity suite holds the three to one recorded answer: `host/facetest`
writes one fixture from one description and embeds one set of answers, and
each face's own suite builds the fixture and compares its reply, the desktop's
through its backend methods. The shapes are projections rather than the faces'
own structs. A known gap between two faces is pinned by a test that asserts
it, so closing the gap fails the test that describes it.

### The frontend consumes the workspace packages

The desktop is one consumer of the monorepo's shared frontend packages rather
than the owner of any of them: the shadcn primitives and the preview kit from
`@neokapi/ui-primitives` ([S-06](s-06-visual-editor.md)), the xyflow-based
`@neokapi/flow-editor`, the graph views in `@neokapi/context-explorer`, the lab
explorers in `@neokapi/kapi-lab`, plus the contract types, reference data,
status views, concept views, and grid editor packages. All resolve through the
single root pnpm workspace.

Its palette comes from the same place. The logo-derived palette is one file,
`packages/ui/src/styles/kapi-colors.css`, which the desktop imports rather than
declaring the values itself, and which in turn imports the semantic tokens that
say what a colour means: the judgement colours and one hue per coordinate axis.
The Storybook renders through the desktop's stylesheet, so a story and the app
read the same values, and the documentation site's Infima variables and the
diagram kit's defaults are computed from that one file. Ivory surfaces, cocoa
accents and turquoise highlights follow the existing logo; dark mode uses warm
charcoal and ochre with dark primary-button text. See
[Brand tokens and the documentation palette](../../implementation/repo/docs-palette.md).

It also consumes `@neokapi/i18n-react` ([S-05](s-05-i18n-runtime.md)) for its
own interface languages. The desktop's UI strings go through the same
extraction and runtime the framework offers to any React application, and the
target-language catalogs are build artefacts, regenerated rather than authored.

### Configuration lives in two roots

Provider configuration and installed plugins live under the kapi config home,
shared with the CLI: `providers.json` there, API keys in the OS keychain under
the `kapi` service name, and plugins under `plugins/` ([S-01](s-01-kapi-cli.md)).
The CLI and the desktop both take an alternative plugin discovery root from
`KAPI_PLUGINS_DIR`, and both honour `KAPI_PLUGINS_DIR_ONLY` to skip the
user/system roots entirely, the same isolation contract dev, CI, and the
harness recorder already rely on.

The desktop's own preferences (theme, interface language, hidden and custom
locales, telemetry consent, the session's open tabs) live in a *separate* root,
`<UserConfigDir>/kapi-desktop`, overridable with `KAPI_DESKTOP_CONFIG_DIR`. The
split is deliberate: a preference about a window is not a setting the CLI
should read, and resetting one should not disturb the other.

### It reuses framework primitives rather than re-deriving them

Tool listings come from the same registry the CLI reads, and each tool's
schema form is generated from its JSON Schema
([E-03](../engine/e-03-tool-system.md)), so a new tool appears in the desktop
with no desktop change beyond registration. Formats, plugins, providers, block
stores, the context graph, and the executor are likewise the framework's,
consumed rather than mirrored. A project declaring a different block store opens
without the desktop knowing which one: resolution goes through the `BlockStore`
interface in the project machinery.

Differences from the CLI are presentational: dynamic forms, event streaming,
live progress, tabbed state. There is no desktop fork of framework behaviour.

### Distribution

The release workflow builds the desktop across a platform matrix covering
macOS, Windows, and Linux on both common architectures where the toolchain
allows it. macOS ships as a Homebrew cask from the tap, Windows and Linux as
archives attached to the GitHub release. A signed appcast feeds the in-app
updater, so an installed desktop can update itself without going back through
the package manager.

## Consequences

- The desktop reaches every framework capability through `host`, so it can
  drift from the CLI only on presentation, and the face parity record catches
  drift on the answers themselves.
- Wails and the native toolchain stay out of the CLI module, which keeps the CLI
  cross-compilable and small.
- The cobra-free and platform-free assertions are mechanical: a reintroduced
  `cli` or platform import fails `make audit-modules`, not a review.
- A project bound to a convergence venue converges there from the desktop as
  from the CLI, so the team's state has one home.
- Tabs support several open projects. Each project-scoped method requires a
  `tabID` to route operations to the correct project.
- `kapi.yaml` recipes stay shareable workflow documents: open, edit, save,
  commit. No hidden state travels with the recipe, and a governance edit made
  in the desktop is a recipe edit `git diff` shows.
- Any new tool, format, or provider registered in the framework appears in the
  desktop with no backend change.
- The app lists projects registered by any local kapi surface and checks for
  updates with one indexed query per second.
- Every desktop edit passes the commit check and lands in the project's block
  history with the desktop as its origin, and a change made elsewhere since the
  desktop read the content is shown to the person before anything overwrites
  it.

## Related

- [F-01: The framework and its modules](../foundations/f-01-framework-and-modules.md): the module isolation contract and the license boundary
- [E-01: The processing engine](../engine/e-01-processing-engine.md): the executor and its trace events
- [E-03: The tool system](../engine/e-03-tool-system.md): the registry and schemas the forms are generated from
- [E-04: Flows and I/O binding](../engine/e-04-flows-and-io-binding.md): why the graph's ends are endpoint pickers
- [E-05: The plugin system](../engine/e-05-plugin-system.md): the plugin manager's model
- [E-09: The change contract](../engine/e-09-the-change-contract.md): the change sets every desktop edit and decision is
- [C-01: The project model](../context/c-01-project-model.md): the recipe and the `.kapi/` sources a tab loads
- [C-03: The context store and graph](../context/c-03-context-store-and-graph.md): the workspace the home screen reads and the operation log it follows
- [C-11: Context operations](../context/c-11-context-operations.md): the operations the feed shows and the policy behind its decisions
- [C-02: Coordinates and governance](../context/c-02-coordinates-and-governance.md): the coordinate points the home maps and the axes the settings edit
- [C-04: Block state and the decision record](../context/c-04-block-state-and-decisions.md): what the review queue records
- [C-06: Context retrieval](../context/c-06-retrieval.md): the guide the explorer renders
- [C-07: Voice profiles](../context/c-07-voice-profiles.md): the profile the Voice page resolves and edits
- [S-01: The kapi CLI](s-01-kapi-cli.md): the shared credential store and config home
- [S-03: Agent surfaces](s-03-agent-surfaces.md): the MCP face the parity record also holds
- [S-05: The i18n runtime for React](s-05-i18n-runtime.md): the runtime the desktop's own interface uses
- [S-06: The visual editor data model](s-06-visual-editor.md): the preview kit the desktop hosts
- [S-07: The review model](s-07-context-centric-review.md): the model the queue's detail pane and the document view render
- [Kapi Desktop overview](/kapi/desktop/overview): the user-facing guide
