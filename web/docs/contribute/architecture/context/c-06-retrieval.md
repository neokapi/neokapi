---
id: c-06-retrieval
sidebar_position: 6
title: "C-06: Context retrieval"
description: "Architecture decision: a project's context is retrieved through two primitives, by location and by content, offered identically on the CLI and over MCP, with the rendering a property of the address and the scope stated on every answer."
keywords: [context retrieval, kapi context, context search, MCP, resources, staleness, scope, neokapi, architecture decision]
---

# C-06: Context retrieval

## Summary

A project's content context is retrieved through **two primitives, offered
identically on the CLI and over MCP**:

| Shape | Question | CLI | MCP |
| --- | --- | --- | --- |
| **By location** | *what applies here?* | `kapi context <path>` | `context_read` tool, or the `context://<path>` resource |
| **By content** | *what do we know about this?* | `kapi context search <query>` | `context_search` tool |

Every asset-shaped lookup folds into one of the two. There is no retrieval
command per store (none for voice, none for terms, none for content memory),
because *which store holds the answer* is an implementation detail the caller
should never have to know.

The two surfaces are held in parity by construction: the MCP tools and resources
are thin wrappers over the same host functions the CLI verbs call, and
conformance tests assert they agree.

A third surface needs no call at all: the **rules files**. kapi writes the
by-location answer for the project root and for every folder whose rules differ
into a section of the `AGENTS.md` and `CLAUDE.md` there, which an agent loads
before it reads anything ([below](#rules-files)).

## Context

A caller does not have a store in mind. It has a question. The assistant asking
*what applies here*, the check asking *does this rule fire*, and the engine
asking *may this approved wording be reused* are one question asked by three
callers.

Asset-shaped retrieval forces the caller to know the answer's location before
asking, which inverts that. Worse, it returns **partial answers that read as
whole ones**: a command that renders a voice profile's own preferred and
forbidden terms visibly contains terminology, while the project's terms store, a
separate source reaching the engine by a separate path, goes unread. A caller
writing against that output has terminology it did not get. Silence would have
been safer.

The same pressure applies across surfaces. The agent skill drives the CLI, so a
surface an assistant learns from the skill and a surface it reaches over MCP must
be the same surface, or the highest-leverage prose in the repository teaches a
model that does not exist.

## Decision

### One question, two shapes

Retrieval is addressed **by location** or **by content**, never by store.

**By location** answers *what applies here*: the point the location resolves to,
the profile in force, its rendered guidance, the terms bound at that point, the
suggestions nobody has decided on, and the governance windows around them. It
resolves through
`KapiProject.ResolveGovernanceFor` ([C-02](c-02-coordinates-and-governance.md)),
the seam a run, a check and a push resolve through, so the voice a writer reads
for a file is the voice a run applies to it, including a content item's own
`channel:` and a profile whose window has closed.

**By content** answers *what do we know about this*: one query, every store the
scope holds (terms and concepts, content memory, profile examples). Each term it
reports carries how often the extracted content uses it, read from the context
graph's `uses_term` edges ([C-03](c-03-context-store-and-graph.md)) rather than
computed when asked, so the number is the one the platform's concept page shows
and every surface labels it as of the last extraction.

### Two address forms, one thing

The by-location primitive is addressable two ways, because an ad-hoc caller has
no path to resolve:

```
context://{+path}{?format}          what applies at this location
context://profile/{name}{?format}   what a named profile holds
```

The CLI mirrors this as `kapi context <path>` and `kapi context --profile <name>`.
`kapi context <path> --comments`, and `comments` on the `context_read` tool,
answer for the point a file's comments sit at instead of its content.
The MCP server publishes the two as the resource templates
`context-at-location` and `context-for-profile`.

`profile/` is reserved under the scheme, which is what lets one address space
carry both forms. A name no recipe declares falls back to a voice profile of that
name (the local store, then a built-in pack), because otherwise the by-name
address would be unusable outside a project, which is the case it exists for.

**Asking is a read, not a call.** The by-content primitive takes a query and is a
tool; the by-location primitive names something that already exists and is a
**resource**. That is not cosmetic: a resource is *addressed*, and an address is
what makes the rendering a property rather than a second tool.

### Rendering is a property, not a command

The by-location primitive renders as **prose for a model** (`text/markdown`) or
**structured for a program** (`application/json`). MCP resources carry a mime
type, so this is a property of the resource rather than a second entry point; the
CLI expresses it as `--json`, and adds `--explain` for a person who wants the
prose with how it was reached. On the wire the rendering rides on the address as
`?format=json`, and the mime type on the response states which was served. A
format nobody recognises is an error: a caller that asked for a shape it can
parse must not be handed prose it cannot.

That is what dissolves a `voice_guide`-shaped tool. Such a tool conflates three
concerns (voice only, by name only, markdown only) into one narrow point. Once
content is the whole context, addressing covers by-name, and rendering is a
property, nothing is left over.

### The writing brief {#the-prose-is-the-task}

The Markdown response provides a writing brief:

- the **voice** in a sentence or two: its name, its description, and the tone
  and style fields the profile sets, with no line for a field it leaves unset,
  then its patterns, comment limits, examples and shared constraints
  (`profile.RenderVoiceBrief`);
- one list, **Say this, not that**, merging the terms in force at the point, the
  rules established and widened to the workspace, and the terms of a starter
  pack bound as the voice at render time, keyed by the wording to use, so a rule
  held in two places is one line naming where it is held (`from`)
  (`host/contextrules.go`); storage keeps the three apart;
- **Keep as it is**: every rule the same terms store holds at another place of
  the project (a collection item's files) and not at this point, as the wording
  that stays correct here, with the places the rule holds at
  (`host/contextelsewhere.go`). A rename kept for `help/` gives the answer for
  `api/workspaces.md` the line `"Workspace" is correct here; do not rename it to
  "Space". That rename holds only in help/.` Silence at an out-of-scope point
  reads to an agent as permission, and an agent told to rename everywhere
  renames there. The wording kept is the rule's old wording only: the new
  name's own spelling variants (`Space-Admin` for `Space Admin`) are wrong
  everywhere and never listed. `kapi check` holds the same line: the new
  wording in a file where the old one is correct is a `terms.vocabulary`
  finding there, which fails unless the concept is advisory
  (`checkTerms.keepAt`);
- the suggestions under **Suggested, not yet established**, less any the list
  already states;
- a closing line naming `context_note` (and `kapi context note`), for recording what the reader notices
  while working.

Notes appear in the prose only when a person or an agent must act before relying
on the answer: a checkout whose context nobody has imported into the store, a voice or terms binding that
failed to load, a governance change since the last read, or a location a profile
claims, where the rules listed are the ones that hold and a rule held elsewhere
does not apply. The JSON
carries them as `attention`. Everything else, the point, the binding field, the
scope, the provenance and every other note, is in the JSON and in `--explain`.

### Rules files {#rules-files}

The answer is also written where an agent reads it without asking
(`host/rulesfiles.go`). kapi resolves the answer at the project's default point
and at every place a collection item reads (one answer per distinct point), and
writes:

- the **root** `AGENTS.md` and `CLAUDE.md`: the voice brief, the rules that hold
  at the default point, one line per folder with rules of its own, the
  `kapi check` line, and the `kapi context note` line for a name or word the
  section does not list;
- a **folder's** `AGENTS.md` and `CLAUDE.md`, for each folder whose answer
  differs from the root's: the rules the root does not state, its voice when it
  is not the root's, the **Keep as it is** lines (a root rule that does not hold
  there, and every rule held elsewhere), and the `kapi check` line. A folder
  holding files at several points gets a heading per pattern. A folder whose
  section would repeat the nearest folder above it with rules of its own gets
  no file, because an agent loads the files of every folder above the one it
  works in.

A list that holds a translation's rules beside the source's names each rule's
language (`[nb]`), so an agent writing the source applies only the source's.

Each list is capped (the rules that rule a wording out first) with the command
that lists the rest, so a file stays short enough to load on every session.
The output is a pure function of the context and the recipe, ordered by folder
and rule, so a refresh that changes nothing changes no byte.

kapi owns one delimited section (`<!-- kapi:rules -->` to
`<!-- /kapi:rules -->`, `core/agentrules`) and nothing else in the file. The
section also replaces a `<!-- kapi:voice -->` pointer section where a file holds one. A
`CLAUDE.md` that imports `@AGENTS.md`, or is the same file, gets no section of
its own. A folder that no longer has rules of its own loses the section, and a
file that held nothing else is deleted. The rules files are never content
(`core/ignore` ignores both names), so a collection globbing `**/*.md` does not
read them.

The files are written by `kapi init` and by `kapi context sync --files-only`.
A project whose root section exists has them refreshed whenever the rules in
force change: a context call that writes a rule into a store or takes one out
(a keep, a drop, a choice, a widening, a reset, settling) refreshes when it
ends, a review round refreshes once at its end, and `kapi context sync`,
`kapi store import` and `kapi up` refresh after they run. Kapi Desktop's
decisions go through the same host calls, and a voice saved there writes the
files. A project without a root section was set up without them, and no
refresh creates them.

### What folds in, and what does not

The fold is on the **MCP surface**. An agent gets the two primitives and no
per-store retrieval tool: no voice-guide tool, no term-lookup tool, no
memory-search tool. `host/mcp_tools_curation_test.go` guards the surface so a
registry tool cannot re-shadow the primitives.

**The CLI keeps its per-store verbs.** `kapi voice show` renders a named
profile, a profile file or a starter pack as a guide; `kapi terms lookup` / `terms search` and
`kapi memory lookup` / `memory search` query one store directly. They answer a
narrower question than `kapi context` (*what does this store hold* rather than
*what applies here*), which is a reasonable thing to ask of a store you opened on
purpose. What an agent is taught differs: the skill drives `kapi context <path>` and
`kapi context search`, so the narrow verbs stay available to a person without becoming the
model an assistant learns.

**Management verbs are not retrieval and do not fold.** `terms
import/export/stats`, `memory import/export/audit`, `voice new/validate/import/pack`
operate on a store, which is a different act from asking a question. The same
holds for the voice tools an agent does get: `voice_check` and `voice_rewrite`
judge or change a piece of text, so neither is a retrieval question.

### Content memory is recycled, not searched

Recycling is the loop's job and is invisible by design: `kapi up` pre-fills from
content memory as the structural cost control. So the retrieval surface does not
offer *search memory for prior translations*. That duplicates work already done,
and inviting a caller to do it by hand is inviting it to hand-crank the loop.

What *is* served is **precedent**: *how has this project said this before*, when
authoring source content. That is a different question from recycling, it is not
answered anywhere else, and it is what makes a caller's own writing sound like
the project. It reaches callers through `context search`, not through a
memory-specific verb.

### Scope, not capability

The same call returns less at a narrower scope, and says so. Three scopes are
named (`host.ContextScope`), and every answer carries one in its `scope` field:

| Scope | What stands behind the answer |
| --- | --- |
| `project` | the local project's stores |
| `workspace` | a connected concept graph, with relations, revisions and market scoping |
| `profile` | one voice profile and nothing else: the by-name answer, with no project behind it |

A result set is **explicitly scoped in its response** rather than silently
thinner: a caller must be able to tell *this project holds no answer* from *this
scope cannot hold one*. Reporting the third case as an empty project scope would
tell a caller the project holds no terminology when no project was consulted at
all, so the by-name answer says in as many words that no recipe point stands
behind it.

Half an answer plus a statement of what was unreachable is more useful than an
answer that quietly omits a store it could not open.

### Coverage and suggestions {#an-answer-with-nothing-in-it-teaches}

Every response includes a `coverage` grade based on the kinds of context
available for that query:

| Primitive | What is counted |
| --- | --- |
| By location | applicable voice profile, terms, and established or workspace-wide rules |
| By content | matching terms and previously approved wording |

Two or more kinds produce `covered`, one produces `thin`, and none produces
`empty`. The grade describes the response, not the whole project. Unreviewed
suggestions raise `empty` to `thin` but cannot produce `covered`.

Suggestions appear separately from established context in a `suggestions`
list. Each entry includes its status, rule or note, actor, session, evidence and
operation id. Contested entries also identify the conflicting operations in
`contested_by`. A term suggestion carries its `standing`, the evidence for and
against it as plain counts (sessions that recorded it, corrections toward it,
uses in content, merges that carried it), and the Markdown line ends with the
same counts: `seen in 3 sessions · merged in #412`. Standing is never reduced
to a score. Markdown renders these under **Suggested, not yet established**.

By-location queries resolve suggestions through `App.ContextRulesAt`, the same
function used by checks. By-content queries match the query against term forms,
replacements and note text. Agents can use earlier observations as context,
and only a person's signal establishes them as rules
([C-11](c-11-context-operations.md)).

Thin and empty by-location responses explain the available coverage and invite
observations about project names, spelling and writing style. If suggestions
are pending, the note reports their count and review status. By-content
responses include this note only when the result is empty.

### Response provenance {#every-answer-says-what-it-read}

Every response includes `provenance`: the project identity, workspace operation
log revision and projection freshness. These fields appear in JSON and in the
CLI's `--explain` output.

The identity distinguishes projects. The revision identifies the workspace
state used for the response. Reopening a project records a new operation only
if its registration changed, preserving the revision for unchanged context.

Projection freshness is checked against extraction stamps using file metadata,
with a hash comparison when the metadata changes. It requires no directory
walk. A stale projection means term usage counts may describe an earlier
version of the content files.

Provenance describes the current read. The separate freshness notice
([C-05](c-05-freshness.md)) reports changes since the previous read by that
process. Neither check updates the underlying content.

Before the first extraction, the projection is reported as stale and term usage
counts are empty. Voice and terminology guidance remain available.

### Results are grouped, never merged into one ranking

A term match and a memory match are not comparable scores. Results are grouped by
kind, each group ranked within itself. A single blended list would impose an
order that means nothing.

### Every answer reports its own freshness

The first of an answer's notes says whether the governing context, the
terminology or the recorded decisions moved since this process last read them.
The comparison is against the freshness ref this project last observed, held on
disk ([C-05](c-05-freshness.md)), so a retrieval costs no round trip, and the
baseline is per process and advances on every read.

It reports and never resolves. What a moved context means for work already
written is a judgement, and the retrieval surface is not in a position to make
it. `kapi status` carries the same fact for a person on its governance axis;
`kapi check --ship` is the enforcing half, where a target produced under a
superseded context fails the staleness gate.

A governance window that closed is reported the same way: the by-location answer
carries the transition (which profile stopped governing, when, and what governs
in its place) as a note.

### The generated surface is opt-in

MCP serves **tool sets** ([S-03](../surfaces/s-03-agent-surfaces.md)), and the
writing set is the default: the two retrieval primitives with `context_read`,
the tool form of the by-location resource, then the context write tools and the
session read (`context_note`, `context_session_summary`,
[C-11](c-11-context-operations.md)), `check_file`, and the edit contract
(`read_blocks`, `apply_edits`, `describe_format`). The content, translation
and review sets carry the rest of the porcelain, with three registry tools that
have no porcelain equivalent (`translate`, `term-check`, `redact`). `kapi mcp
--all-tools` adds the full generated surface for debugging and power use.

**The tools that execute arbitrary commands and scripts are not part of that
flag.** *Show me every tool* and *let a caller run anything* are different classes
of decision, and bundling them means enabling the first silently grants the
second ([E-06](../engine/e-06-execution-trust.md)). They are not exposed over
MCP at all; `kapi exec` still runs them.

Cutting MCP exposure removes nothing from the CLI: `kapi exec <tool>` runs every
registry tool regardless.

## Consequences

- **The skill and the MCP client learn the same model**, which is what stops the
  repository's highest-leverage prose from teaching a surface that does not
  exist.
- **Callers stop needing a map of the stores.** The question is the interface;
  where the answer lives is ours to change.
- **A stale answer is visible to the caller holding it**, rather than being a
  read with no memory.
- **Empty responses are explicit.** They describe missing context and explain
  how to record observations.
- **Responses identify their source.** Project identity and workspace revision
  support comparisons between reads.
- **Partial answers stop reading as whole ones**, the failure that makes a
  store-shaped retrieval tool actively misleading rather than merely narrow.
- **A new registry tool does not become an agent tool by accident.** Exposure is
  a decision with a name attached.
- **The rules reach an agent that never calls a tool.** A comparison of the
  same rules delivered through retrieval and through a plain rules file found
  the file broke fewer rules at a quarter of the input, and lost scoped renames
  only where the answer was silent; the rules files carry the answer there, and
  `kapi check` stays the gate.
- **The rules files are committed output.** They change when the context
  changes, so a review that keeps a rule produces a diff a reviewer reads, and
  the dogfood loop regenerates this repository's own files.
- **The by-location primitive resolves to the file, not to the passage.** A
  content item's own `channel:` is the finest declared point, so one file in a
  collection can answer differently from its neighbours. A point beneath the file
  is not yet built ([C-02](c-02-coordinates-and-governance.md)).
- **Parity is a test, not a convention.** The CLI verb is pinned to the answer's
  own rendering, the MCP resource body is pinned to the same bytes and its JSON
  to the same document, and a snapshot test locks both the tool names and the
  addresses as a contract.
- **An address is a contract in the way a tool name is.** A caller writes
  `context://` into its own prompts and configuration, so the URIs may be added
  to and never renamed or dropped without an explicit decision.

## See also

- [C-02: Coordinates and governance](c-02-coordinates-and-governance.md): the
  resolution the by-location answer renders.
- [C-03: The context store and graph](c-03-context-store-and-graph.md): the
  tables and traversals these primitives read.
- [C-05: Freshness and the composite ref](c-05-freshness.md): the staleness note
  and the enforcing gate.
- [S-03: Agent Skills](../surfaces/s-03-agent-surfaces.md): the skill that
  drives these verbs.
- [MCP reference](/reference/mcp): the served tool and resource list.
