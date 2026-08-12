---
id: 037-context-retrieval-surface
sidebar_position: 37
title: "AD-037: The context retrieval surface"
---

# AD-037: The context retrieval surface

## Summary

A project's content context is retrieved through **two primitives, offered
identically on the CLI and over MCP**:

| Shape | Question | CLI | MCP |
| --- | --- | --- | --- |
| **By location** | *what applies here?* | `kapi context <path>` | `context://<path>` resource |
| **By content** | *what do we know about this?* | `kapi context search <query>` | `context_search` tool |

Every asset-shaped lookup folds into one of the two. There is no separate
retrieval command per store — not for voice, not for terms, not for content
memory — because "which store holds the answer" is an implementation detail the
caller should never have to know.

The two surfaces are held in parity by construction: the MCP tools and resources
are thin wrappers over the same host functions the CLI verbs call, and
conformance tests assert the retrieval surfaces agree.

## Context

### The surface was generated, not designed

`host/mcp_tools.go` exposed **every CLI-visible framework tool** over MCP. In
ad-hoc mode that produced 51 tools, of which roughly 12 were deliberately
authored. Adding a pipeline step silently added an agent-facing tool, so the
surface accumulated `whitespace-correct`, `encoding-detect`, `inline-codes-remove`
and `xml-validation` — flow steps no agent should call — alongside
`external-command` ("Executes an external command on block text") and `script`
("Run a JavaScript processing script"), which were exposed to any MCP client by
inheritance rather than by decision.

### Retrieval was asset-shaped on both surfaces, and the two disagreed

Before this AD, retrieval was a command per store, and MCP exposed an arbitrary
subset of what the CLI offered:

| CLI | MCP | |
| --- | --- | --- |
| `kapi voice guide` | `voice_guide` | both |
| `kapi voice show` | — | CLI only (and a duplicate of `voice guide`) |
| `kapi voice profiles` | — | CLI only |
| `kapi terms lookup` | `term_lookup` | both |
| `kapi terms search` | — | CLI only |
| `kapi memory lookup` | — | CLI only |
| `kapi memory search` | `tm_search` | both |

Six CLI retrieval verbs, three MCP tools, no stated rule for which got exposed.
The **agent skill drives the CLI**, so an assistant following the skill and an
assistant calling MCP tools learned two different models of what kapi holds.

### Why asset-shaped retrieval is the wrong shape

A caller does not have a store in mind. It has a question:

> Communication is contextual — the assistant asking *what applies here*, the
> check asking *does this rule fire*, and the engine asking *may this approved
> wording be reused* are one question asked by three callers.

Asset-shaped retrieval forces the caller to already know the answer's location
before asking, which inverts that. Worse, it returns **partial answers that read
as whole ones**: `voice_guide` renders a profile's own `PreferredTerms` /
`ForbiddenTerms`, so its output visibly contains terminology — while the
project's **terms store**, a separate source reaching the engine by a separate
path, went unread. A caller that writes against that output has terminology it
did not get. Silence would have been safer.

## Decision

### One question, two shapes

Retrieval is addressed **by location** or **by content**, never by store.

**By location** answers *what applies here*: the point the location resolves to,
the profile in force, its rendered guidance, the terms bound at that point, and
the governance windows around them. It resolves through
`project.ResolveGovernanceFor` — the seam a run, a check and a push resolve
through — so the voice a writer reads for a file is the voice a run applies to
it, including a content item's own `channel:` and a profile whose window has
closed.

**By content** answers *what do we know about this*: one query, every store the
scope holds — terms and concepts, content memory, profile examples.

### Two address forms, one thing

The by-location primitive is addressable two ways, because an ad-hoc run has no
path to resolve:

```
context://<path>            what applies at this location
context://profile/<name>    what a named profile holds (packs, ad-hoc, "as if")
```

CLI mirrors this as `kapi context <path>` and `kapi context --profile <name>`.

`profile/` is reserved under the scheme, which is what lets one address space
carry both forms. A name no recipe declares falls back to a voice profile of
that name — the local store, then a built-in pack — because otherwise the
by-name address would be unusable outside a project, which is the case it exists
for.

**Asking is a read, not a call.** The by-content primitive takes a query and is
a tool; the by-location primitive names something that already exists and is a
**resource**. That is not a cosmetic distinction: a resource is addressed, and
an address is what makes the rendering a property rather than a second tool.

### Rendering is a property, not a command

The by-location primitive renders as **prose for a model** (`text/markdown`) or
**structured for a program** (`application/json`). MCP resources carry a mime
type, so this is a property of the resource rather than a second entry point;
the CLI expresses it as `--json`.

On the wire the rendering rides on the address as `?format=json`, and the mime
type on the response states which was served. A format nobody recognises is an
error: a caller that asked for a shape it can parse must not be handed prose it
cannot.

This is what dissolves the `voice_guide` tool. It conflated three concerns —
voice only, by name only, markdown only — into one narrow point. Once content is
the whole context, addressing covers by-name, and rendering is a property,
nothing is left over.

### What folds in

The fold is on the **MCP surface**. An agent gets the two primitives and no
per-store retrieval tool:

| Gone from the agent surface | Folds into |
| --- | --- |
| `voice_guide` | the by-location primitive |
| `term_lookup` | `context_search` |
| `tm_search` | `context_search` |

The kapi MCP server exposes no voice-guide, term-lookup or memory-search tool,
and `host/mcp_tools_curation_test.go` guards the surface so a curated registry
tool cannot re-shadow the primitives.

**The CLI keeps its per-store verbs.** `kapi voice guide` and `kapi voice show`
render a resolved profile as a guide; `kapi terms lookup` / `terms search` and
`kapi memory lookup` / `memory search` query one store directly. They answer a
narrower question than `kapi context` — *what does this store hold* rather than
*what applies here* — which is a reasonable thing to ask of a store you opened on
purpose. What changes is what an agent is taught: the skill drives `kapi context
search`, so the narrow verbs stay available to a person without becoming the
model an assistant learns.

**Management verbs are not retrieval and do not fold.** `terms import/export/
stats`, `memory import/export/audit/sessions`, `voice new/validate/import/pack`
stay exactly as they are — they operate on a store deliberately, which is a
different act from asking a question. The same holds for the voice tools an agent
does get: `voice_check` and `voice_rewrite` judge or change a piece of text, so
neither is a retrieval question.

### Content memory is recycled, not searched

Recycling is the loop's job and is invisible by design: `up` pre-fills from
content memory as the structural cost control. So the retrieval surface does not
offer "search memory for prior translations" — that duplicates work already
done, and inviting a caller to do it by hand is inviting it to hand-crank the
loop. `recycle` and `diff-leverage` leave the agent surface for the same reason.

What *is* served is **precedent**: *how has this project said this before*, when
authoring source content. That is a different question from recycling, it is not
answered anywhere else, and it is what makes a caller's own writing sound like
the project. It reaches callers through `context search`, not a memory-specific
verb.

### Reach, not capability

The same call returns less locally, and says so. kapi resolves a terms store, a
content memory and a profile; bowrain resolves the full concept graph with
relations, revisions and market scoping. A local result set is **explicitly
scoped in its response** rather than silently thinner — a caller must be able to
tell "this project holds no answer" from "this scope cannot hold one".

Three reaches are named, and every answer carries one. `project` is the local
project's stores. `workspace` is a connected concept graph. `profile` is one
voice profile and nothing else — the by-name answer with no project behind it,
which has no terminology and no governance window to read. Reporting that third
case as an empty project scope would tell a caller the project holds no
terminology when no project was consulted at all.

### Results are grouped, never merged into one ranking

A term match and a memory match are not comparable scores. Results are grouped
by kind, each group ranked within itself. A single blended list would impose an
order that means nothing.

### Every answer reports its own freshness

An answer is a snapshot of a graph that other processes move, and a caller holds
one for the length of a task. So the first of an answer's `notes` says whether
the governing context, the terminology or the committed decisions moved since
this process last read them.

The comparison is against the freshness ref this project last observed of its
server, held on disk (`core/ref/refcache`). Two consequences follow from putting
it there rather than on the wire:

- **A retrieval costs no round trip.** It is a read path a caller hits
  repeatedly inside a single thought; phoning the server on each one would trade
  a note nobody waits for against latency on every question asked. Refreshing
  the cache belongs to the transport, at the cadence it already runs at.
- **The baseline is per process and advances on every read.** A first answer
  reports nothing — nothing has moved since a read that had not happened yet —
  and a change is reported once, to the answer that first spans it, rather than
  on every answer from then on.

It reports and never resolves. What a moved context means for work already
written is a judgement, and the retrieval surface is not in a position to make
it. `kapi status` carries the same fact for a person, on its governance axis;
`kapi check` is the enforcing half, where a target produced under a superseded
context fails the staleness gate.

### The generated surface becomes opt-in

MCP exposes a **curated** set by default. `KAPI_MCP_ALL_TOOLS=1` restores the
full generated surface for debugging and power use.

**`external-command` and `script` are not part of that flag.** "Show me every
tool" and "let a caller execute arbitrary commands and JavaScript" are different
classes of decision, and bundling them means enabling the first silently grants
the second. They are not exposed over MCP; `kapi exec` still runs them.

Cutting MCP exposure removes nothing from the CLI. `kapi exec <tool>` runs every
registry tool regardless.

## Consequences

- **The skill and the MCP client learn the same model.** The agent skill drives
  the CLI, so parity is what stops the highest-leverage prose in the repo from
  teaching a surface that MCP does not have.
- **Callers stop needing a map of the stores.** The question is the interface;
  where the answer lives is ours to change.
- **A stale answer is visible to the caller holding it.** Retrieval was
  previously a read with no memory, so an agent that retrieved once and wrote
  for an hour had no way to learn that the context under it had changed.
- **Partial answers stop reading as whole ones** — the failure that made
  `voice_guide` actively misleading rather than merely narrow.
- **A new registry tool no longer becomes an agent tool by accident.** Exposure
  is a decision with a name attached.
- **The by-location primitive resolves to the file, not yet to the passage.** A
  content item's own `channel:` is the finest declared point, so one file in a
  collection can answer differently from its neighbours. The migration-guide case
  — *the old name is permitted in these two paragraphs* — is what is left: it
  needs a point beneath the file, which nothing declares yet.
- **Parity is a test, not a convention.** The MCP tools and resources wrap the
  same host functions the CLI verbs call, and conformance tests assert the
  surfaces agree: `cli/context_path_test.go` pins the verb to the answer's own
  rendering, `host/mcp_context_resource_test.go` pins the resource body to the
  same bytes and its JSON to the same document, and
  `kapi/cmd/kapi/mcp_snapshot_test.go` locks both the tool names and the
  addresses as a contract. The drift that produced the table above cannot recur
  silently.
- **An address is a contract in the way a tool name is.** A caller writes
  `context://` into its own prompts and configuration, so the URIs may be added
  to and never renamed or dropped without an explicit decision — the same rule
  the tool surface already lived by, now extended to the addresses.
