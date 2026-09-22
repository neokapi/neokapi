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
| **By location** | *what applies here?* | `kapi context <path>` | `context://<path>` resource |
| **By content** | *what do we know about this?* | `kapi context search <query>` | `context_search` tool |

Every asset-shaped lookup folds into one of the two. There is no retrieval
command per store (none for voice, none for terms, none for content memory),
because *which store holds the answer* is an implementation detail the caller
should never have to know.

The two surfaces are held in parity by construction: the MCP tools and resources
are thin wrappers over the same host functions the CLI verbs call, and
conformance tests assert they agree.

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
candidates nobody has decided on, and the governance windows around them. It
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
CLI expresses it as `--json`. On the wire the rendering rides on the address as
`?format=json`, and the mime type on the response states which was served. A
format nobody recognises is an error: a caller that asked for a shape it can
parse must not be handed prose it cannot.

That is what dissolves a `voice_guide`-shaped tool. Such a tool conflates three
concerns (voice only, by name only, markdown only) into one narrow point. Once
content is the whole context, addressing covers by-name, and rendering is a
property, nothing is left over.

### What folds in, and what does not

The fold is on the **MCP surface**. An agent gets the two primitives and no
per-store retrieval tool: no voice-guide tool, no term-lookup tool, no
memory-search tool. `host/mcp_tools_curation_test.go` guards the surface so a
registry tool cannot re-shadow the primitives.

**The CLI keeps its per-store verbs.** `kapi voice guide` and `kapi voice show`
render a resolved profile as a guide; `kapi terms lookup` / `terms search` and
`kapi memory lookup` / `memory search` query one store directly. They answer a
narrower question than `kapi context` (*what does this store hold* rather than
*what applies here*), which is a reasonable thing to ask of a store you opened on
purpose. What changes is what an agent is taught: the skill drives `kapi context
search`, so the narrow verbs stay available to a person without becoming the
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

### An answer with nothing in it teaches

Most projects meet kapi with no voice profile and no terms, and the honest
answer for a location in one is short. It was once so short that an assistant
read it as *there is nothing to do here* and worked without the project's
context for the rest of the session.

So every answer carries a `coverage` grade, counting the kinds of material in
force behind it:

| Primitive | What is counted |
| --- | --- |
| By location | the voice profile in force at the point; the terms bound there and in force; the rules confirmed and widened at it ([C-11](c-11-context-operations.md)) |
| By content | the terms the query matched; the prior wording it found |

Two or more is `covered`, one is `thin`, none is `empty`. Three grades, because
the grade is read by a model deciding how much to lean on the answer, and a
finer scale would be a number nobody could act on differently. The grade
describes **the answer**, so a search for a word the project has never written
about is empty whatever else the project holds.

**A candidate counts for less than anything in force.** A proposal nobody has
decided on holds no content to anything, so it never makes an answer `covered`.
It does lift `empty` to `thin`, because a candidate is evidence that someone
looked here, and `empty` then means what it says: nothing at all has been
recorded at this point. The by-location answer reads them through
`App.ContextRulesAt`, the seam a check resolves them with, so a candidate an
answer mentions is a candidate a check reports.

**Candidates are listed, apart from everything in force.** The answer carries a
`candidates` list beside `voice` and `terms`: each entry says it is a candidate,
what it proposes, who recorded it, in which session, and the evidence behind it,
with the operation id a person confirms it by. The rules in it are the ones the
resolution holds at the point, so the list and a check cannot disagree, and the
operation log supplies the provenance a reader judges one by. The prose
rendering puts them under a heading that says they are not decided.

A second agent reading a location therefore builds on what the first one
recorded rather than working the same facts out again, and cannot mistake either
for a rule in force. Neither can it act on them: deciding is a person's
([C-11](c-11-context-operations.md)).

A thin or empty by-location answer adds one note: that this project records
nothing here yet, and what is worth noticing while the work is done (the names
the project gives its own things, the spellings it keeps to, who the text
addresses, how formal it is). Where candidates stand behind a thin answer, the
note counts them and says they are waiting to be confirmed or discarded. It
**states no rule**, because none is in force, and handing a writer a proposal
the project has not agreed to is the one outcome worse than saying nothing.

The by-content answer carries that note when it found nothing at all and on no
other grade. A search graded thin found the word and answered the question
asked, so the same sentence would be a lecture delivered on a successful call.

### Every answer says what it read

An answer is quoted, acted on, and sometimes committed, often an hour after it
was read and by a process that has since asked the same question of another
project. So every answer carries a `provenance`: the project's stable identity
([C-01](c-01-project-model.md)), the position the workspace's operation log had
reached, and whether the content kapi holds still matches the files on disk.

The three are one fact each, and each is needed for a different reason. The
identity says which project answered, for a caller that moves between them. The
**revision** is a position: two answers carrying one revision were read from
one state of the context, which is what makes them comparable. Opening a
project is the most frequent thing that happens to a workspace, so it records
an operation only when the registration changed, and a re-open leaves the
position where it was.

The **projection's state** compares the extract-time stamps the block store
already holds against the files on disk, one stat each and a hash only where
the stat moved. It costs no walk of the tree, which matters on a path an agent
hits repeatedly inside one thought, and it is what tells a caller that a term's
use count describes the files as they were rather than as they are.

This reports position on every answer. The staleness note beside it
([C-05](c-05-freshness.md)) reports movement, once, to the answer that first
spans it. Neither resolves.

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

MCP exposes a **curated** set by default: the two retrieval primitives, the check
tools (`check_text`, `check_file`), `stats`, the convergence verbs (`up`,
`up_plan`), the write verb (`apply_edits`), the two offline voice tools, the
three context write tools and the session read (`context_observe`,
`context_propose`, `context_correct`, `context_session_summary`,
[C-11](c-11-context-operations.md)), and three registry tools that have no
porcelain equivalent (`translate`, `term-check`, `redact`). `kapi mcp
--all-tools` restores the full generated surface for debugging and power use.

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
- **A project that has recorded nothing is still worth asking.** The first hour
  of a project gets an answer that says it is empty and what to watch for,
  rather than one an assistant reads as permission to stop asking.
- **Two answers can be compared without either having watched the other**,
  because each carries the project and the revision it was read at.
- **Partial answers stop reading as whole ones**, the failure that makes a
  store-shaped retrieval tool actively misleading rather than merely narrow.
- **A new registry tool does not become an agent tool by accident.** Exposure is
  a decision with a name attached.
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
