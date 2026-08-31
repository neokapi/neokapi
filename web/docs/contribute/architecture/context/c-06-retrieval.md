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
the profile in force, its rendered guidance, the terms bound at that point, and
the governance windows around them. It resolves through
`KapiProject.ResolveGovernanceFor` ([C-02](c-02-coordinates-and-governance.md)),
the seam a run, a check and a push resolve through, so the voice a writer reads
for a file is the voice a run applies to it, including a content item's own
`channel:` and a profile whose window has closed.

**By content** answers *what do we know about this*: one query, every store the
scope holds (terms and concepts, content memory, profile examples).

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

### Results are grouped, never merged into one ranking

A term match and a memory match are not comparable scores. Results are grouped by
kind, each group ranked within itself. A single blended list would impose an
order that means nothing.

### Every answer reports its own freshness

The first of an answer's notes says whether the governing context, the
terminology or the committed decisions moved since this process last read them.
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
`up_plan`), the write verb (`apply_edits`), the two offline voice tools, and
three registry tools that have no porcelain equivalent (`translate`,
`term-check`, `redact`). `kapi mcp --all-tools` restores the full generated
surface for debugging and power use.

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
