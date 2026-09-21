---
id: s-03-agent-surfaces
sidebar_position: 3
title: "S-03: Agent surfaces: MCP and skills"
description: "An AI assistant reaches kapi two ways: a shipped Agent Skill that drives the CLI, and a curated MCP server for non-CLI clients. Content and asset edits land through one write verb, kapi apply, whose typed change-set carries them through a single reviewed path; review decisions have verbs of their own on both surfaces."
keywords: [neokapi, architecture decision, agent skill, SKILL.md, MCP, model context protocol, kapi apply, change-set, hooks, progressive disclosure]
---

import { CycleDiagram } from "@neokapi/docs-shared";

# S-03: Agent surfaces: MCP and skills

## Summary

An AI assistant reaches kapi through two surfaces over one implementation. The
**Agent Skill**, a `SKILL.md` router plus progressive-disclosure reference
files sourced at `cli/skills/data/kapi/`, teaches an assistant that runs shell
commands *when* to reach for kapi and *which* verb to run. The **MCP server**
(`kapi mcp`) serves clients that call tools rather than shell out, exposing a
deliberately curated set plus the `context://` resource space. Both converge on
the same asymmetry: **the assistant writes the content; kapi supplies the
context, applies edits through format-aware writers, and runs configured checks.** Content and
asset edits land through one write verb, `kapi apply`, as a typed JSONL
change-set, which is also the MCP `apply_edits` tool. A review decision has
its own verbs on both surfaces, sharing the host's decision path.

## Context

The assistant already writes the prose and the code. What it lacks is the
project's own context (what a thing is called here, what wording is approved,
what tone applies at this location) and the ability to write an edit back into
a `.docx` or an XLIFF without corrupting it. Those are exactly what kapi holds.

In the attended workflow, the assistant authors the text and uses scoped
findings to repair it. Format readers and writers determine the supported
round-trip behavior; the resulting diff remains reviewable. Provider tools also
support unattended work. A completed check gate covers its configured analyzers.
Applicable guidance that requires semantic judgment is explicitly unassessed
when no analyzer implements it.

The connective tissue has to be cheap. An assistant's context is finite, and a
document describing every kapi command would crowd out the task. Hence a router
that stays small and reference files that load only when the task matches.

## Decision

### A skill is a directory, and its source lives beside the CLI

```
cli/skills/data/kapi/
├── SKILL.md            the router: frontmatter (name, description) + a short
│                       body that decides scope and points at references
└── references/         progressive-disclosure how-to, loaded on demand
    ├── edit.md         read → edit → write → verify
    ├── create.md       author → parse → check → revise
    ├── voice.md        retrieve guidance, score a draft, fix it
    ├── translate.md    translation and terminology
    ├── project.md      the project model
    ├── context.md      ask what applies, and notice when it moves
    ├── check.md        the report, its exit codes, the release gates
    ├── growing-context.md   the everyday calls, discovery, and the refresh path
    ├── toolbox.md
    ├── i18n.md         routes by detected stack into…
    └── i18n/           …per-ecosystem playbooks + a machine-readable registry
```

`SKILL.md` leads with **four habits** an assistant keeps inside other work: ask
what applies at the file before writing it, record what it notices while reading
the project, record the wording the person changes, and check what it changed
before reporting the work done and saying what the session recorded. Each habit
is a few lines and one command, and everything past them is a map of the
references. The body is a router: it triages the request and points at one
reference, and the references carry the task detail, one per concern.
Terminology folds into the voice and translate references rather than standing
alone, because a term is something you apply while writing or translating, not a
task you set out to do.

The `growing-context` reference is the second and third habits at their other
speed. Everyday growth and a deliberate discovery session are one mechanism:
both record operations that produce candidates, and neither writes a governance
file, because confirming is what writes one ([C-11](../context/c-11-context-operations.md)).
The deliberate half covers two visits. On the first, the assistant assembles a
project's context from the user's material. On a later one it diffs new material
against what the project already holds and proposes a **refresh**: candidates
the user confirms one at a time, or a change-set the user approves
(`kapi apply refresh.jsonl`) where the decisions are already made. Nothing is
rewritten behind the user's back either way.

The `i18n` concern is itself a tree. `references/i18n.md` detects the stack and
routes into `references/i18n/`, driven by a machine-readable framework registry
(`frameworks.yaml`) carrying detection signals, catalog layouts, kapi presets,
and a maintenance-cost grade per framework.

The source lives in `cli/skills/data` because the skill names specific commands
and flags. A verb change and its skill update are then one reviewed change, and
the reviewer sees both.

Skill content is **agent-actionable only**: when to trigger, which command,
what footgun to avoid. Architecture and implementation belong in these
documents, not in a file an assistant loads into a live context window.

### The skill is a copy, never a second tree

One source tree, copied. Four make targets and the binary itself produce the
copies, so they cannot diverge:

| Target | What it produces |
| --- | --- |
| `make plugin-bundle` | the Claude Code plugin bundle under `packages/kapi-claude-plugin` |
| `make publish-plugin` | mirrors that bundle to the `neokapi-plugins` marketplace repo |
| `make publish-skill` | mirrors the portable skill into the agent-skills collection, for any `SKILL.md`-aware tool |
| `make dev-skills` | copies it into this repo's own `.claude/skills` for dogfooding |
| `cli/skills` (`go:embed`) | the copy `kapi init` writes into a project (see [the wiring below](#kapi-init-wires-an-agent-up)) |

The embedded copy is the one a release can make a promise about. A plugin
cannot pin a CLI version, so a marketplace skill that named an unreleased
command would break an up-to-date plugin against a released binary; the
embedded copy is the skill the running binary was built with, and the commands
and flags it names are the ones that binary has.

The marketplace and collection repos are **generated distribution artifacts**,
like a package-manager tap: never hand-edited. Publication is on kapi release,
not on merge.

### `kapi init` wires an agent up {#kapi-init-wires-an-agent-up}

A project has a voice, terms and a check gate long before anyone tells an
assistant they exist. The voice pointer says so in prose, in `CLAUDE.md` or
`AGENTS.md`. The wiring says so in the files an agent host reads as
configuration, so an agent opened in the project finds kapi with nothing else
installed:

| Host | What is written | Convention |
| --- | --- | --- |
| Claude Code | `.mcp.json` (`mcpServers`), `.claude/skills/kapi/` | [project MCP file](https://code.claude.com/docs/en/mcp), [project skills](https://code.claude.com/docs/en/skills) |
| Cursor | `.cursor/mcp.json` (`mcpServers`) | [Cursor MCP](https://cursor.com/docs/context/mcp) |
| VS Code | `.vscode/mcp.json` (`servers`) | [MCP configuration reference](https://code.visualstudio.com/docs/agents/reference/mcp-configuration) |
| Cross-client | `.agents/skills/kapi/` | [Agent Skills client guide](https://agentskills.io/client-implementation/adding-skills-support) |

Each host is supported where its convention was read from that host's own
documentation. `servers` and `mcpServers` differ between two of them, and a key
the host does not read is inert with nothing to notice it, so
`host/agentwiring_test.go` asserts each spelling rather than trusting one.

Claude Code is wired unconditionally, because its MCP file sits at the project
root and its skills directory is one kapi creates, so there is nothing to
detect. The others are wired where the project already keeps their directory.
`--agents` takes a list, `all`, or `none`; `kapi init` on a project that
already has a recipe is how an existing project gains the same wiring.

Four properties hold for everything written:

- **Project scope only.** Every path is under the project root. Nothing under
  the user's home directory and nothing machine-wide is read or written.
- **A command, and nothing else.** The entry carries the binary, the `mcp`
  verb, and the project it answers for. No shell, no environment, no
  credential: these files are committed, shared, and loaded by a program that
  runs what they say.
- **The entry names the project.** `kapi mcp --project kapi.yaml`, so the
  server binds this project rather than whichever one is above the directory
  the host happened to start it in. The path is relative, because the file is
  shared with everyone on the project and an absolute one resolves on one
  machine.
- **An existing entry is left alone.** A configuration file that already names
  a server called kapi is read and not written. The skill directory is kapi's
  own, so the files the binary ships are refreshed there and anything else in
  it stays.

### The MCP server introduces itself

`initialize` carries an `instructions` string to every client, ahead of the
tool list and whether or not the host loads a skill. It is the only text a
client with no skill support ever reads about kapi, so it carries the same four
habits the skill leads with, one paragraph each.
`host/mcp_instructions_test.go` holds it to the names the server actually
serves and to a length budget, so a fifth paragraph is a decision rather than a
drift.

The server also writes one row into the workspace saying that an agent is at
work: the session id it records operations under, the project, the client's own
name from `initialize`, when the session opened and when it was last seen. A
receiving middleware moves the row forward on every request, so a session that
is only reading still says it is here, and the desktop can show it
([S-02](s-02-kapi-desktop.md)). Nothing has to be cleaned up when a process
exits: a row that stops moving ages out on the next write.

The skill's `description` is the sole triggering lever, and it is loaded at
startup by every `SKILL.md`-aware tool. Whether it fires on the right tasks is
measured rather than remembered: `scripts/skilleval` drives a real assistant
session (`claude -p`) per scenario in a throwaway workspace and records what
the agent did. `make skill-eval` scores triggering over positive and negative
prompts, `make skill-eval-completion` drives each positive scenario to a green
gate, and `make mcp-eval` measures whether an agent picks the right MCP tool.
The results and the whole transcripts are published on the
[skill eval page](/skill-eval) ([A-01](../assurance/a-01-testing-and-documentation.md));
none of the three runs in CI, because they spend and need local credentials.

The paired study runs identical tasks through each agent host with ordinary
file tools, the CLI skill, or MCP. The skill condition excludes the kapi MCP
server; the MCP condition excludes the skill and direct kapi CLI execution.
Independent artifact checks assess completion, while natural prompts measure
whether the agent discovers the available integration. Results retain failed
attempts and distinguish these outcomes from human judgments of the content.
The [paired evaluation runner](../../implementation/repo/paired-agent-evaluation.md)
records the model and integration configuration for each attempt.

### Two hooks, and a protocol for failing open

The Claude Code plugin ships two project-scoped hooks that drive kapi rather
than being kapi:

- a **Stop** hook running `kapi hook stop`, which runs the project's ship gates
  and keeps the assistant working until they are green;
- a **PreToolUse** hook running `kapi hook pre-edit`, which blocks direct
  hand-edits of files the project generates as translation targets.

Both **fail open**. A guard that cannot evaluate must never block work that has
nothing wrong with it: failing closed on an unparseable payload would stop an
unrelated edit, or trap an assistant that has finished.

What fail-open needs is a way to *say so*, because the zero value of a hook
payload is indistinguishable from a legitimate one. An empty file path reads as
"nothing to guard"; an empty working directory lets the project walk run from
wherever the hook process started. Both allow, and neither leaves a trace, so
without the protocol below a guard that never ran looks exactly like a guard
that passed.

| Situation | stdout | Exit |
| --- | --- | --- |
| Guard evaluated, verdict negative | the decision shape, with a reason | 0 |
| Guard evaluated, nothing to report | nothing | 0 |
| Guard could not run | `{"systemMessage":"…"}`, and the same warning on stderr, naming the hook | 0 |
| Guard evaluated, gates did not run | `{"systemMessage":"…"}` naming the `did_not_run_cause`, and the same warning on stderr | 0 |

Three consequences follow.

**The verdict travels in the JSON, never in the exit code.** A non-zero exit
reads as a broken hook, so carrying a denial there converts an enforced decision
into a hook error. A denial is exit 0 with the verdict in the payload. In
particular it never uses `ExitGate` (3): that code belongs to a human or a CI
step running the gate directly ([S-01](s-01-kapi-cli.md)). The hook *drives*
the gate; it is not the gate.

**A payload carrying only `systemMessage` carries no decision**, so the normal
permission flow is untouched and fail-open is preserved exactly.

**The warning goes to stderr as well**, naming the hook, because a hand-run or CI
invocation has no session to surface a `systemMessage` into.

The discriminating case: **no payload at all is not a failure**. When stdin is a
character device, the command was run by hand with nothing piped in; there is
nothing to read and nothing to report. Likewise "there is no kapi project here"
is nothing to gate, not a guard that failed. Warning on either would make every
session outside a project noisy, which is how a guard gets uninstalled.

These are the assistant-integration hooks. They are unrelated to a recipe's
`hooks:` block, a separate lifecycle mechanism.

### The two loops the skill drives

<CycleDiagram
  steps={[
    { label: "Read", sub: "kapi inspect" },
    { label: "Edit", sub: "the assistant writes" },
    { label: "Write", sub: "kapi apply" },
    { label: "Check", sub: "kapi check <path>" },
  ]}
  caption="The edit loop: the assistant supplies the words; kapi reads, applies and checks the content. Findings guide revisions, while unsupported guidance remains for review."
/>

**Editing existing content.** `kapi inspect` is the read leg: it parses any
editable format into one record per block, carrying the text with inline codes
rendered as `<x id="…"/>` placeholders so an edit can round-trip, the block's
structural role and nesting level, a stable `id`, and a `content_hash`, the
canonical block identity ([F-03](../foundations/f-03-identity.md)). The
assistant rewrites the text and returns a typed change-set; `kapi apply` writes
it.

**Creating new content.** With no frozen source, the assistant authors in a
*generative* format, one whose writer can produce a document from the content
model alone, and uses `kapi inspect` or `kapi stats` to parse it back as the
first check, then `kapi check` as the voice-and-terminology gate, revising until
green. Binary office formats are editable but not generative: authored
elsewhere, edited in place.

Both loops are provider-free by default. The assistant is the writer; kapi is the
format engine and the checker.

The ordinary authoring loop uses `kapi context <path>` before editing and
`kapi check <path>` afterwards. The file path determines the applicable voice
channel and terms. The assistant reads analyzer coverage alongside findings;
a passing score establishes only the checks that ran. In a repository the loop
checks the change rather than the project: `kapi check --diff-against HEAD`
checks each content block the edit touched, whole, and reports the lines it
spans, so the check costs one read per changed file and runs after every edit.
`kapi check --ship` enforces project release policy when that is part of the
task.

For MCP, context retrieval uses the existing `context://<path>` resource.
`check_file` checks saved content in its project scope. A draft can be checked
with `check_text` and `context_path`, the intended project-relative destination.
This binds the same voice and terms without reading the destination file.
Explicit profile overrides belong to unscoped snippet checks and cannot be
combined with `context_path`. A bound-context failure is an operation error.

### `kapi apply`, the write verb for content and assets

Every deliberate, reviewed change to content or to an asset is one typed JSONL
entry discriminated by `kind`, and every one lands through `kapi apply`:

| `kind` | What it edits | How it lands |
| --- | --- | --- |
| `content` | a block's text in a named `file` | byte-faithful round-trip, drift- and inline-code guarded |
| `comment` | one comment's prose in a named `file`, pinned by `comment_sha256` | the same round-trip, through the collection that governs comments |
| `term` | a term | the committed terms source → import → the terms tables of the project store |
| `memory` | a content-memory pair | the committed memory source → import → the memory tables of the project store |
| `voice` | a voice vocabulary rule | the committed voice profile → voice-store import ([C-07](../context/c-07-voice-profiles.md)) |
| `review` | a unit's review outcome | appended to the decision ledger, exported by `kapi commit` ([C-04](../context/c-04-unit-state-and-decisions.md)) |
| `recipe` | an allowlisted recipe field | the `kapi.yaml` recipe, via project load and save |

Two properties make this one verb rather than six.

**An asset edit writes the committed source, then compiles the projection.** The
edit lands in the git-tracked artifact the recipe binds (the terms or memory
bundle, the voice profile, the recipe) and the *existing* importer refreshes the
gitignored database from it. The backing store therefore has exactly one writer,
`git diff` is the uniform review surface for every kind, and the operation is
idempotent, so re-running a partly-applied change-set is safe.

**A content edit carries its own guards.** Each `content` entry pins a
`content_hash`; if the block drifted since it was inspected, the edit is *stale*
and skipped. An edit that drops, invents, or unbalances an inline code is
*rejected* by the fidelity guard rather than written as broken markup. Either
outcome exits non-zero so the fix loop re-inspects and retries.

A mixed change-set (a content fix plus the `term` or `voice` rule that justifies
it) lands atomically, so the draft and the rule that governs future drafts move
together.

A review decision is the one write that also has verbs of its own. On MCP,
`approve_unit`, `reject_unit` and `sign_off_unit` record a unit's outcome
through the same host decision path the CLI uses, with the agent's identity
attached; `apply_edits` with a `review` entry reaches the same record. Both
append to the decision ledger, and `kapi commit` exports it.

### Format editability is declarative

A skill needs to know, before it edits, whether a format can be written back.
[`kapi formats`](/reference/commands/formats) carries an **Edit** column, and its JSON adds `editable`,
`round_trip`, and `generative`. A format is *editable* when it has a reader and a
writer and is not a bilingual interchange format, **including binary office
formats**, because the faithful round-trip is precisely what makes editing a
binary container safe. *Round-trip* means the writer reconstructs from a
skeleton, so an edit changes only the edited text. *Generative* gates authoring
from scratch. All three resolve declaratively, without loading a plugin
([E-02](../engine/e-02-format-system.md)).

### The MCP server is curated

An MCP session binds to an explicit recipe (`kapi mcp -p project.kapi.yaml`) or
an implicitly discovered project. An explicit path wins when discovery is
disabled. Invalid bound context fails startup or the affected check operation;
it cannot silently become an ungoverned successful result. File checks retain
the recipe path the call resolved and resolve the file's effective profile at
that path. CLI and MCP reports share finding, gate and execution semantics.
Their transport and timing boundaries remain distinct.

### The project is an argument of the call

An assistant works in more than one project, and an MCP server outlives any one
of them. So every project-scoped tool takes an optional `project`, and the
`context://` resource takes a `?project=` parameter. The value names the
project's `kapi.yaml`, its root directory, or any path inside it: a call can
pass the file it is editing.

Resolution runs per call, through the seam the CLI uses (`host.ResolveProjectPath`
→ `core/project.ResolveRecipePath` → `core/project.ResolveLayout`), so a call and
a `kapi -p …` invocation reach the same recipe. `KAPI_NO_PROJECT` is honoured
exactly as it is on the CLI: it disables discovery, and an explicit path still
resolves. A path that holds no project is refused with a typed error naming the
path, rather than falling back to whichever project the server's working
directory sits in.

The project the server resolved at start remains the default, so a client
configured for one project keeps the behaviour it had.

Four properties follow from resolving per call rather than per process.

**Nothing about a call lands on the server.** The resolved recipe travels on the
command the handler builds, which is what every embedded surface already does,
so two calls for two projects run at once without interfering.

**Each project gets its own store handle.** `App.ProjectDB` memoizes one handle
per project root, and `Shutdown` closes all of them, so a second project opens a
second connection pool and neither outlives the server.

**Content is read in the call's own source language.** A term lookup matches the
locale exactly, so content read in another project's language is held to no
vocabulary at all. The language is settled from the recipe each call resolved
and travels with the run. A source language named when the server started still
outranks a recipe, as an explicit `--source-lang` outranks one on the command
line, and the server's own language answers a call that resolved no project.

**The tool list stays the start project's.** A client reads `tools/list` once, so
which tools exist is fixed when the server starts. A per-call project decides
what governs the content, and for a registry tool it decides the target-language
default; it does not add or remove tools.

`kapi mcp` starts a stdio JSON-RPC server. Its surface is a **decision with a
name attached**, never a consequence of a tool being CLI-visible. Exposing every
registry tool produced an agent surface nobody chose, most of it pipeline steps
(`whitespace-correct`, `encoding-detect`, `xml-validation`) that no caller
should be assembling by hand, plus verbs like recycling that the catch-up loop
does automatically and invisibly.

The default surface is therefore the hand-authored porcelain (reading and sizing
content with `extract_content`, `detect_format` and `stats`, checking text or a
file, voice scoring and offline rewriting, context search, the three context
write tools and the session read, the catch-up verbs and their dry run, the
review-queue verbs, and `apply_edits`) plus a short curated list of registry
tools that produce something a caller cannot produce itself or check something
with no porcelain equivalent: `translate`, `term-check` and `redact`. The
listing helpers and `pseudo_translate` sit behind `--all-tools`, the
flow-running verbs behind `--all-flows`, and `--all` is the shorthand for both.
The full generated list is in the [MCP reference](/reference/mcp).

Three curation rules are asserted by tests rather than remembered:

- **Nothing that executes caller-supplied code is ever agent-facing**, not even
  under `--all-tools`. "Show me every tool" and "let a caller run arbitrary
  commands and JavaScript" are different classes of decision, and bundling them
  would mean enabling the first silently grants the second. Neither is removed
  from the CLI: `kapi exec` still runs both.
- **No curated tool shadows a porcelain one.** Two names for one job means the
  caller picks wrong half the time.
- **Nothing a person decides is agent-facing.** `context_observe`,
  `context_propose` and `context_correct` record what an agent may record;
  confirming, discarding another actor's work, reverting and widening are a
  person's, and the surface carries no tool for them at all. The policy
  ([C-11](../context/c-11-context-operations.md)) would refuse such a call
  anyway, and a tool that is always refused is one an assistant keeps trying.
  The actor rides on the call rather than in it: kind `agent`, the name from
  `initialize`, and a session minted once per server process, so a caller cannot
  claim to be a person.

In project mode the set narrows further to tools whose source the recipe
declares, and the project's first target language becomes the default.

### Asking a location is a resource

Context retrieval is split by the shape of the question
([C-06](../context/c-06-retrieval.md)). Asking what a *word* means is a call with
arguments, so it is a **tool** (`context_search`). Asking what applies at a
*location* is reading something that already exists at an address, so it is a
**resource**, served in the `context://` space:

| Address | Answers |
| --- | --- |
| `context://{+path}{?format,project}` | what applies at a project-relative location: the voice profile in force with its guidance, the terms bound there, the candidates nobody has decided on, and the governance windows around them |
| `context://profile/{name}{?format,project}` | the same, addressed by governance profile name, for a caller with no file in hand |

Both render markdown by default; `?format=json` returns the structured shape.
Making the rendering a property of the read, a MIME type, is what avoids a
second entry point for the same question. One reserved path prefix carries the
by-name form, so a single scheme carries both address forms. `?project=` names
the project the read acts on, the way the tools take a `project` argument.

Both MCP primitives are thin wrappers over the same host functions the `kapi
context` verbs call. The skill drives the CLI, so a capability that existed on
only one surface would teach an assistant a kapi the other half does not have.

### Parity is a suite, not a claim

Sharing an implementation makes the two surfaces agree in principle. What
establishes it in practice is a conformance suite that drives the real
`kapi mcp` over stdio the way a client does (`initialize`, `tools/list`,
`tools/call`, `resources/read`) against scratch projects, and compares each
answer with what `kapi check`, `kapi exec` or `kapi context` reports for the
same input.

Every check-type tool carries a fixture that must pass and a fixture that must
fail. The must-fail half is the point: the harness that builds a block from a
snippet built it source-only, so every bilingual check reached over MCP returned
a clean result whatever the translation said, and the CLI path was correct the
whole time. A suite that only asserts a clean result on clean content cannot see
that.

The suite lives in `kapi/e2e` and runs pre-merge in the `Kapi CLI E2E` job on
every pull request touching `cli/`, `host/` or `kapi/`.

### The skill drives the CLI

The bundled skill issues shell commands. The CLI is the richer surface (the
model-backed checks, the credential store, project resolution, the full
toolbox), and an assistant that can run shell already has it. MCP exists for
clients that cannot, and the shared host implementation is what keeps the two
honest.

Because a skill issues commands, it consumes the exit-code contract
([S-01](s-01-kapi-cli.md)): a distinct gate code lets a loop branch on "the draft
scored below the bar, rewrite it" versus "the command failed" without parsing
output.

## Consequences

- One source tree feeds the plugin bundle, the portable skill, the binary's
  embedded copy and the in-repo dogfood by copy, and it sits beside the CLI it
  documents, so a command change and its skill update are one reviewed change.
- A project is wired for the agents that work in it by the command that creates
  it, so the first hour needs no install step and no hand-written JSON.
- Progressive disclosure keeps the router cheap and loads detail only on a match.
- The attended loops call no provider: the assistant writes, kapi round-trips,
  drift-checks, and gates.
- One write verb covers content and asset edits, a mixed change-set lands
  atomically, and `git diff` is the uniform review surface for all of it.
- A curated MCP surface means the agent-facing tool list is a reviewed decision;
  the code-execution exclusion is a test, so widening the surface can never
  silently grant shell access.
- One long-lived server serves an assistant that moves between projects, because
  the project is an argument of the call rather than a property of the process.
- A check tool that stops reporting a violation over MCP fails a pre-merge job,
  rather than surviving until someone notices a clean result on broken content.
- Splitting retrieval into a tool and a resource lets rendering be a property of
  the read rather than a second address, and keeps both forms of the question in
  one address space.
- Whether the skill fires, and whether an agent picks the right tool, is a
  measured number with a transcript behind it rather than a checklist someone
  remembers to run.
- A project's context grows out of the work an assistant was doing anyway,
  because the habits are in the two places an assistant reads before it acts:
  the skill body and the server's instructions. What it records advises and
  never binds, so a session that records the wrong thing costs a person one
  decision rather than a bad rule in force.

## Related

- [S-01: The kapi CLI](s-01-kapi-cli.md): the command surface the skill drives and the exit-code contract it consumes
- [S-04: Toolbox utilities](s-04-toolbox.md): the format-aware utilities a skill reaches for; `kapi apply` is the deliberate, reviewed sibling of `ksed`'s regex substitution
- [S-07: The review model](s-07-context-centric-review.md): the object `review_unit` returns whole
- [C-11: Context operations](../context/c-11-context-operations.md): the operations the write tools record, and the policy that decides who may record which
- [F-03: Identity](../foundations/f-03-identity.md): the `content_hash` a change-set pins as its drift anchor
- [E-02: The format system](../engine/e-02-format-system.md): the writer capabilities behind `editable` / `round_trip` / `generative`
- [E-06: Execution trust](../engine/e-06-execution-trust.md): why code-executing tools stay off the agent surface
- [C-04: Unit state and decisions](../context/c-04-unit-state-and-decisions.md): what a `review` entry records
- [C-06: Context retrieval](../context/c-06-retrieval.md): the two questions, and why one is a resource
- [C-07: Voice profiles](../context/c-07-voice-profiles.md): the profile a `voice` entry edits
- [M-01: Bilingual interop](../multilingual/m-01-bilingual-interop.md): the `extract`/`merge` round-trip that `inspect`/`apply` mirror on the monolingual side
- [A-01: Testing and documentation](../assurance/a-01-testing-and-documentation.md): the eval band the skill and MCP measurements belong to
- [MCP reference](/reference/mcp): the generated tool and resource surface
- [`kapi inspect`](/reference/commands/inspect) and [`kapi apply`](/reference/commands/apply): the read and write legs, per-flag
