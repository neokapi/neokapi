---
id: s-03-agent-surfaces
sidebar_position: 3
title: "S-03: Agent surfaces: MCP and skills"
description: "An AI assistant reaches kapi two ways: a shipped Agent Skill that drives the CLI, and a curated MCP server for non-CLI clients. Content and asset edits are kapi.change/v1 change sets, applied whole or not at all by the change service, sent with kapi apply on the command line and with apply_edits over MCP, where an agent reads blocks with read_blocks and records a pre-review as a decide operation with outcome advise."
keywords: [neokapi, architecture decision, agent skill, SKILL.md, MCP, model context protocol, kapi apply, apply_edits, change set, hooks, progressive disclosure]
---

import { CycleDiagram } from "@neokapi/docs-shared";

# S-03: Agent surfaces: MCP and skills

## Summary

An AI assistant reaches kapi through two surfaces over one implementation. The
**Agent Skill**, one short `SKILL.md` sourced at `cli/skills/data/kapi/`,
teaches an assistant that runs shell commands *when* to reach for kapi and
*which* verb to run, and `kapi help <topic>` serves the reference files beside
it. The **MCP server**
(`kapi mcp`) serves clients that call tools rather than shell out, exposing a
deliberately curated set plus the `context://` resource space. Both converge on
the same asymmetry: **the assistant writes the content; kapi supplies the
context, applies edits through format-aware writers, and runs configured checks.** On the
command line, content and asset edits land through `kapi apply` as a
kapi.change/v1 change set ([E-09](../engine/e-09-the-change-contract.md)), the
contract the change service applies for every surface. On the MCP server an
agent reads a document's blocks with `read_blocks` and sends the same change
set with `apply_edits`. A pre-review is a `decide` operation with outcome
`advise` in such a change set, and a review decision stays a person's.

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
document describing every kapi command would crowd out the task. Hence a skill
that stays small and reference topics the assistant asks the binary for only
when the task matches.

## Decision

### A skill is a directory, and its source lives beside the CLI

```
cli/skills/data/kapi/
├── SKILL.md            four habits, each in its CLI and MCP form, under 365 words
└── references/         the topics `kapi help <topic>` serves
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

`SKILL.md` is the **four habits** an assistant keeps inside other work: ask
what applies at the file before writing it, record what the project does every
time while reading it, record the wording the person changes, and check what it
changed before reporting the work done and saying what the session recorded.
Each habit is its CLI command and its MCP tool: `kapi context <path>` and
`context_read`, `kapi context note` and `context_note` for both kinds of
recording, and `kapi check` then `kapi context log --session this`, or
`check_file` then `context_session_summary`. The skill teaches only the verbs
`kapi context` lists for assistants. Review, reset and sync are a person's,
and an assistant at most tells the person to run `kapi context review`. One
line names the edit topic,
`references/edit.md` (`kapi help edit`), for an assistant about to change
content inside a file, so the edit guidance is one step from the skill; the
references folder ships with the skill in a plugin install, and `kapi help
edit` prints it where `kapi init` writes `SKILL.md` alone. A last line says that
`kapi help` lists the topics for everything else. The recording habit also names what
to record (names as written, the spelling variety, a word chosen over a common
alternative) and what to leave alone (a word the project writes two ways, an
interface label, the wording of the task), because an assistant that reaches
kapi from the shell reads the skill and no tool description. The skill's description stays under the 1,024
characters agent hosts load at startup, and `cli/skills/skills_test.go` holds
both limits.

The references carry the task detail, one per concern, and the binary serves
them: `kapi help <topic>` prints one, with links between references rewritten
as the `kapi help` command that serves each ([S-01](s-01-kapi-cli.md)). A
reference therefore always matches the binary the assistant is running.
Terminology folds into the voice and translate references rather than standing
alone, because a term is something you apply while writing or translating, not a
task you set out to do.

The `growing-context` reference covers routine observations and dedicated
context discovery. Both record suggestions for review
([C-11](../context/c-11-context-operations.md)). During initial discovery, the
assistant proposes context from the user's material. During a refresh, it
compares new material with existing context and proposes changes. The user can
keep individual suggestions in `kapi context review` or approve a prepared
change set with `kapi apply refresh.jsonl`.

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

The following targets distribute copies from one source tree:

| Target | What it produces |
| --- | --- |
| `make plugin-bundle` | the Claude Code plugin bundle under `packages/kapi-claude-plugin` |
| `make publish-plugin` | mirrors that bundle to the `neokapi-plugins` marketplace repo |
| `make publish-skill` | mirrors the portable skill into the agent-skills collection, for any `SKILL.md`-aware tool |
| `make dev-skills` | copies `SKILL.md` into this repo's own `.claude/skills` for dogfooding |
| `cli/skills` (`go:embed`) | the `SKILL.md` `kapi init` writes into a project (see [the wiring below](#kapi-init-wires-an-agent-up)), and the topics `kapi help` serves |

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
| Claude Code | `.mcp.json` (`mcpServers`), `.claude/skills/kapi/SKILL.md` | [project MCP file](https://code.claude.com/docs/en/mcp), [project skills](https://code.claude.com/docs/en/skills) |
| Cursor | `.cursor/mcp.json` (`mcpServers`) | [Cursor MCP](https://cursor.com/docs/context/mcp) |
| VS Code | `.vscode/mcp.json` (`servers`) | [MCP configuration reference](https://code.visualstudio.com/docs/agents/reference/mcp-configuration) |
| Codex | `.codex/config.toml` (`mcp_servers`) | read once the person trusts the repository |
| Cross-client | `.agents/skills/kapi/SKILL.md` | [Agent Skills client guide](https://agentskills.io/client-implementation/adding-skills-support) |

Host configuration follows each client's documented format.
`host/agentwiring_test.go` verifies differences such as the `servers` and
`mcpServers` keys.

Claude Code is wired unconditionally, because its MCP file sits at the project
root and its skills directory is one kapi creates, so there is nothing to
detect. The others are wired where the project already keeps their directory.
`--agents` takes a list, `all`, or `none`; `kapi init` on a project that
already has a recipe is how an existing project gains the same wiring.

Five properties hold for everything written:

- **Project scope only.** Every path is under the project root. Nothing under
  the user's home directory and nothing machine-wide is read or written.
- **A command, and nothing else.** The entry carries the binary, the `mcp`
  verb, the project it answers for and, when the recipe declares target
  languages, `--tools writing,translation`. No shell, no environment, no
  credential: these files are committed, shared, and loaded by a program that
  runs what they say.
- **The entry names the project.** `kapi mcp --project kapi.yaml`, so the
  server binds this project rather than whichever one is above the directory
  the host happened to start it in. The path is relative, because the file is
  shared with everyone on the project and an absolute one resolves on one
  machine.
- **An entry someone else wrote is left alone.** An entry called kapi that is
  exactly what kapi writes, differing only in its tool sets, is kapi's own and
  follows the recipe. Any other entry called kapi is read and not written.
- **The skill directory holds what this binary ships.** `SKILL.md` is refreshed.
  A file an earlier kapi copied there (a reference file from before the skill
  was one file) is recognised by its content, against the embedded tree and the
  closed list in `cli/skills/retired.sha256`, and removed. Anything else stays,
  and `kapi init` names it.

### The MCP server introduces itself

`initialize` carries an `instructions` string to every client, ahead of the
tool list and whether or not the host loads a skill. It is the only text a
client with no skill support ever reads about kapi, so it states the task in
about a hundred words: call `context_read` before changing a file; record with
`context_note` what the files do every time (names as written, the spelling
variety, a word chosen over a common alternative) and leave alone a word they
write two ways; record the person's changes with `context_note` too (`from`,
`to`, `path`); take back a mistake with its `withdraw` field; run `check_file`
on each changed file; and end with `context_session_summary`.

It points at `context_read` rather than the `context://<path>` resource. Some
clients list only concrete resources and never a resource template, so a model
there reaches the answer through the tool, which returns the same text. The
instructions name the writing set's tools, so a server that does not serve that
set sends none. `host/mcp_instructions_test.go` holds the text to the names the
writing set serves and to a word budget.

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
file tools, the CLI skill, MCP, or the edit commands with no project. The skill
condition excludes the kapi MCP server; the MCP condition excludes the skill
and keeps kapi on PATH, as an installation has it; the project-free condition
runs the same binary with discovery off, so nothing governs its edits.
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

**Editing existing content.** `kapi inspect` is the read leg: it reads any
format through the change service into one read record per block: the block's
reference (`ref`: document, block key and edition) and the revision of its text
(`rev`), the text with inline codes rendered as `<x id="…"/>` placeholders so an
edit can round-trip, each code with the attributes an edit may change, its
plurals and selects with the path to each branch, its other editions, the
operations it accepts, and its structural role and nesting level. The assistant
rewrites the text and returns a change set whose operations copy `ref` into
`at` and `rev` into `if_match`; `kapi apply` writes it.

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

Every deliberate, reviewed change to content or to an asset is an operation of
a kapi.change/v1 change set, and on the command line every one lands through
`kapi apply`, which hands the set to the change service
([E-09](../engine/e-09-the-change-contract.md)):

| Operation | What it edits | How it lands |
| --- | --- | --- |
| `set_content`, `replace_text` | an edition of a block, named by `at` and guarded by the revision read in `if_match` | the file home's byte-faithful round-trip, refused when the edition moved or an inline code or plural would be lost |
| `set_content` on a code comment | one comment's prose in a source file, keyed as `kapi check` reports it, its revision taken from the comment's fingerprint | the comment write path: the file must still parse and the language's formatter must agree, and what was written is checked again |
| `decide` | a review outcome on the edition revision a person read | the decision ledger ([C-04](../context/c-04-block-state-and-decisions.md)) |
| `term` | a term, including every word rule (`advisory`, `competitor`) | the terms tables of the project store, with a context operation recorded |
| `memory` | a content-memory pair | the memory tables of the project store, with a context operation recorded |
| `recipe` | an allowlisted recipe field | the `kapi.yaml` recipe, via project load and save |

Three properties make this one verb.

**A change set lands whole or not at all.** The service applies every content
operation in memory and stages every document before it writes one; when any
operation is refused, nothing is written, the refused operation names its error
code, and every other operation reports `not_applied`. Decisions and asset
operations land after the content they refer to. A change set edits code
comments or documents, never both, because a comment is rewritten through its
language's comment layer rather than a format's writer.

**An edit carries its own guards.** Each content operation names the revision
it read; if the edition moved since, the operation is refused `stale` with the
current revision and text, so the sender can rebase without another read. An
edit that drops, invents, or unbalances an inline code is refused `guard`, and
so is flat text in place of a plural or select, whose branches an edit reaches
by `path`. A reference to no document or block is `not_found`; a block the
format reads as not translatable, such as a code block, lists no operations and
refuses an edit `unsupported`. A refusal exits 3 so the fix loop re-reads and
retries; a change set that does not decode exits 2. A change set resent after
it landed writes nothing: the revisions it names have moved, so it is refused
`stale` with the current text. An operation whose `if_match` still holds and
whose content the edition already holds reports `unchanged`.

**An asset edit writes the project's store and records what it did.** The edit
lands in the terms store or the content memory, and the same call appends a
context operation carrying the actor and the evidence
([C-11](../context/c-11-context-operations.md)). Each store therefore has
exactly one writer, and `kapi context log` is the uniform review surface for
every kind. The command line stamps the actor its environment names; a change
set has no field that could name another.

`kapi apply --dry-run` computes and checks a change set and prints a diff per
document without writing. `--print-ops` echoes the decoded set, and
`ksed --print-ops` prints the change set a substitution compiles to, so the
path a person takes and the path an agent takes share one format.

A review decision belongs to a person. An agent pre-reviews: `review_block`
reads a queued block with the reference and revision of the edition under
review, and `apply_edits` records a `decide` operation at that reference with
outcome `advise`, a score from 0 to 100 and its reasons. The pre-review is
bound to the revision the agent read and recorded as `agent/<client>`, and the
person working the queue reads it beside the block. An agent's `establish`,
`reject` or `withdraw` is refused as `not_permitted`. A person's decision
reaches the decision ledger through the desktop, `kapi apply` run as a person,
or a hosted review session.

### Governance at commit

The change service (`core/change`) checks the editions a change set changes
before it writes anything, through two hooks a host supplies: a commit check
and an actor policy. The package defines both and knows nothing about
projects. kapi's host implements them as `App.CommitCheck` and
`ChangePolicy`.

The commit check holds each changed edition to what `kapi check` holds it to,
resolved the same way: the voice and the terms at the point its document sits
at ([C-02](../context/c-02-coordinates-and-governance.md)), and for a
translation the term rules `gateTermRules` gives for its language
([C-08](../context/c-08-terms.md#the-terms-gate)). The authoritative edition,
and any edition in its language, meets the hygiene analyzer, the terms analyzer
and the voice's pattern rules and constraints. A translation meets the
placeholder check against the authoritative edition and the term rules for its
language. Only deterministic analyzers run. Each edition is checked alone, so
the rules that read a whole document (a voice's required patterns) and the
model-backed analyzers run in `kapi check`. Each call resolves the project,
its source language and its governance for itself, so one long-lived host
checks change sets for several projects at once.

The check reports findings on each edition before and after the change, and
the service refuses a change set as `gate_failed` only for a failing finding
the edit introduces. A violation the edition already had is reported beside
the edit and lands with it, so a typo fix in a paragraph with an old term
violation is written. Findings carry `fails` and `suggested` as the check
report does: an advisory rule, and a suggested rule nobody has kept, report
and never refuse. A source edit is held to the source analyzers alone; the
translations it leaves behind are pending work for the loop. Before the
change, a translation is checked against the source it was written against,
so a change set that edits a source and its translation together is held to
the rules the new source demands. An edition the change set removed, alone or
with its deleted block, has nothing after the change and introduces nothing.

The check returns the governance fingerprint it resolved, and the record
stores it. Each document and language the change set touches sits under the
governance the staleness gate recomputes a fingerprint for
(`profile.GovernanceContext` over the voice and the term rules there), so a
later reader can tell whether the governance an edit was made under still
holds. A change set under one governance carries its fingerprint. One under
several carries `tool.OverlayConfigFingerprint` over their distinct
fingerprints in sorted order, which a reader recomputes from the documents and
languages of the record's transitions.

`ChangePolicy` extends the context policy
([C-11](../context/c-11-context-operations.md)) to the operations of a change
set. A person may send every operation, write over whatever an edition holds,
and choose the `report` gate to land an edit over its findings. An agent may
change content and may pre-review (`decide` with `advise`). Its `term`,
`memory` and `recipe` operations, any other decision, a blind write
(`if_match: "*"`) and the `report` gate are refused as `not_permitted`, with
what to do instead: record a suggestion, ask a person, or read the edition and
send its revision. A tool in a flow may choose `report`, because its drafts
meet the ship gates later, and only a tool records how it produced an edition.
A tool decides only with `advise`, as an agent does; a pull or a merge that
carries a person's decision sends it as that person. The transport names the
sender, and a sender of any other kind, or of none, is refused every
operation.

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

**Call state is isolated.** The handler passes the resolved recipe through its
command object. Concurrent calls for different projects do not change the
server's default project.

**Each project gets its own store handle.** `App.ProjectDB` memoizes one handle
per project root, and `Shutdown` closes all of them, so a second project opens a
second connection pool and neither outlives the server.

**Source language resolves per call.** Each run uses the resolved project's
source language for exact locale matching in term lookups. An explicit server
source-language setting takes precedence over the recipe. Calls without a
project use the server default.

**The tool list is fixed at startup.** A client reads `tools/list` once, so
which tools exist is fixed when the server starts. A per-call project decides
what governs the content, and for a registry tool it decides the target-language
default; it does not add or remove tools.

`kapi mcp` starts a stdio JSON-RPC server. Its surface is a **decision with a
name attached**, never a consequence of a tool being CLI-visible. Exposing every
registry tool produced an agent surface nobody chose, most of it pipeline steps
(`whitespace-correct`, `encoding-detect`, `xml-validation`) that no caller
should be assembling by hand, plus verbs like recycling that the catch-up loop
does automatically and invisibly.

The surface is served in **tool sets**, because an assistant reads every
description it is offered and a writing session needs a handful of tools.
`--tools <set>[,<set>...]` names them, and `writing` is served when it names
none:

| Set | Serves |
| --- | --- |
| `writing` | the `context://` resources, `context_read`, `context_search`, `context_note`, `context_session_summary`, `check_file`, `read_blocks`, `apply_edits`, `describe_format` |
| `content` | `check_text`, `voice_check`, `voice_rewrite`, `term-check`, `detect_format`, `redact` |
| `translation` | `translate`, `up`, `up_plan`, `stats` |
| `review` | `review_queue`, `review_block`, `apply_edits` |
| `all` | every set |

The sets are one table in `host/mcp_sets.go`, and a tool may sit in more than
one: `apply_edits` is in the review set too, because an agent records its
pre-review through it. Every factory registers its tools, and the server then
removes each tool that no selected set lists; the surface snapshot fails when a
tool belongs to no set. A name that is not a set
fails startup with the list. `kapi init` writes `--tools writing,translation`
into the MCP entry of a project that declares target languages. The writing
set holds the edit contract, so an agent in a project `kapi init` wired
reaches it whichever sets the entry names. The listing
helpers and `pseudo_translate` sit behind `--all-tools`, the flow-running verbs
behind `--all-flows`, and `--all` serves every set and both. `list_flows`
lists what `kapi flows` lists for the call's project and `run_flow` resolves
a name the way `kapi run` does ([E-04](../engine/e-04-flows-and-io-binding.md)). `translate`,
`term-check` and `redact` are the registry tools on a set: they produce
something a caller cannot produce itself or check something with no porcelain
equivalent. The full generated list is in the [MCP reference](/reference/mcp).

Three curation rules are asserted by tests rather than remembered:

- **Nothing that executes caller-supplied code is ever agent-facing**, not even
  under `--all-tools`. "Show me every tool" and "let a caller run arbitrary
  commands and JavaScript" are different classes of decision, and bundling them
  would mean enabling the first silently grants the second. Neither is removed
  from the CLI: `kapi exec` still runs both.
- **No curated tool shadows a porcelain one.** Two names for one job means the
  caller picks wrong half the time.
- **Nothing a person decides is agent-facing.** `context_note` records what an
  agent may record, each as a suggestion: what it noticed, or a person's
  change (`from`, `to`, `suggest`). With `withdraw` it takes back what the same
  session recorded wrongly and nothing else. Keeping, dropping, widening,
  resetting and sharing are a person's, and the surface carries no tool for
  them at all. The policy
  ([C-11](../context/c-11-context-operations.md)) would refuse such a call
  before modifying context.
  The server assigns the actor identity: kind `agent`, the name from
  `initialize`, and a session minted once per server process, so a caller cannot
  claim to be a person.

In project mode the set narrows further to tools whose source the recipe
declares, and the project's first target language becomes the default.

### The edit contract over MCP

Three tools carry the change contract ([E-09](../engine/e-09-the-change-contract.md))
to an agent, each over the change service the host builds for the call's
project:

| Tool | Takes | Returns |
| --- | --- | --- |
| `read_blocks` | `doc`, optionally `blocks`, `editions`, `cursor` and `limit` | a page of blocks, each with its `ref` and `rev`, its text with inline codes as `<x id="…"/>` placeholders, its codes and their attributes, its plural and select branches, its other editions and the operations it accepts, and `next` for the following page |
| `apply_edits` | a `kapi.change/v1` change set: its input schema is the change-set schema with the per-call `project` added | a `kapi.change-result/v1` result |
| `describe_format` | `format`, or `doc` for the format kapi reads it in | what each operation supports in that format, or null where the format refuses it |

The read and the write go through one service and one reader for a document,
so a reference and a revision `read_blocks` reports are the ones `apply_edits`
resolves. The service reads each project in that project's source language,
resolved for the call from its recipe and any language the server was started
with, so an edition is a translation whichever project the server started in
or answered last. A refused change set writes nothing; a partial one landed in
part, after an interrupted write or a decision refused once the content was
written. Either is an error result carrying the same structured result, so a
client that reads only `isError` still learns that nothing, or not
everything, landed. The results are written with HTML escaping off, so the
placeholders a block's text holds reach the agent as written.

Code comments are written by `kapi apply`, whose `set_content` on a comment
takes the comment write path outside the change service. A read or an edit of
a source code file kapi reads for its comments is refused as `unsupported` with
the capability `comment`, and an agent edits the comment in the file and runs
`check_file` on it.

The transport stamps the actor: every change set `apply_edits` applies is the
calling agent's, named by its `initialize` name, in the server's session. The
context policy refuses an agent's `term`, `memory` and `recipe` operations, and
the change set holding one is refused whole; an agent records a term rule as a
suggestion instead. An applied edit that writes the form a suggestion prefers
adds to that suggestion's standing as an agent signal
([C-11](../context/c-11-context-operations.md)).

### Asking a location is a resource

Context retrieval is split by the shape of the question
([C-06](../context/c-06-retrieval.md)). Asking what a *word* means is a call with
arguments, so it is a **tool** (`context_search`). Asking what applies at a
*location* is reading something that already exists at an address, so it is a
**resource**, served in the `context://` space:

| Address | Answers |
| --- | --- |
| `context://{+path}{?format,project}` | what applies at a project-relative location: the voice in force, the words to use and to avoid there, and what has been suggested but not yet established |
| `context://profile/{name}{?format,project}` | the same, addressed by governance profile name, for a caller with no file in hand |

Both render markdown by default, the brief a writer reads and nothing about how
it was reached; `?format=json` returns the structured shape, which adds the
point, the full voice guide, the terms, the governance windows and the
provenance.
Making the rendering a property of the read, a MIME type, is what avoids a
second entry point for the same question. One reserved path prefix carries the
by-name form, so a single scheme carries both address forms. `?project=` names
the project the read acts on, the way the tools take a `project` argument.

The server offers both addresses as resource templates, and a client lists no
resources from a template, so an agent that works from its tool list would never
find one. `context_read` is the same read as a tool: it takes `path`, `format`
and `project` and returns exactly the text `context://<path>` returns.

Both MCP primitives are thin wrappers over the same host functions the `kapi
context` verbs call. The skill drives the CLI, so a capability that existed on
only one surface would teach an assistant a kapi the other half does not have.

### CLI and MCP conformance {#parity-is-a-suite-not-a-claim}

A conformance suite starts the real `kapi mcp` server over stdio against
isolated projects. It exercises `initialize`, `tools/list`, `tools/call` and
`resources/read`, then compares results with `kapi check`, `kapi exec` or
`kapi context` on the same input.

Every check tool has both passing and failing fixtures. Bilingual fixtures
include target content so the suite detects tools that skip translation checks
or return an empty result for invalid content. The edit contract is driven the
way an agent drives it: `read_blocks`, then `apply_edits` with the reference
and revision it reported, the same change set refused as `stale` with the text
the block holds now, and an agent's decision refused as `not_permitted`.

The suite lives in `kapi/e2e` and runs in the `Kapi CLI E2E` job on pull requests
that change `cli/`, `host/` or `kapi/`.

### The skill drives the CLI

The bundled skill issues shell commands and can use the CLI's full set of
checks, credential management and project operations. MCP provides structured
access for clients without shell execution. Both use the shared host runtime.

The skill relies on the [exit-code contract](s-01-kapi-cli.md): a failing check
has a distinct code from an operational error, allowing the assistant to choose
between revising content and troubleshooting the command.

## Consequences

- One source tree feeds the plugin bundle, the portable skill, the binary's
  embedded copy and the in-repo dogfood by copy, and it sits beside the CLI it
  documents, so a command change and its skill update are one reviewed change.
- A project is wired for the agents that work in it by the command that creates
  it, so the first hour needs no install step and no hand-written JSON.
- Progressive disclosure keeps the router cheap and loads detail only on a match.
- The attended loops call no provider: the assistant writes, kapi round-trips,
  refuses an edit whose block moved since it was read, and gates.
- One write verb covers content and asset edits, a change set lands whole or not
  at all, and `git diff` is the uniform review surface for all of it.
- Tool sets mean the agent-facing tool list is a reviewed decision per kind of
  work, and a writing session reads a handful of descriptions rather than every
  tool; the code-execution exclusion is a test, so widening the surface can
  never silently grant shell access.
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
- [S-07: The review model](s-07-context-centric-review.md): the object `review_block` returns whole
- [C-11: Context operations](../context/c-11-context-operations.md): the operations the write tools record, and the policy that decides who may record which
- [E-09: The change contract](../engine/e-09-the-change-contract.md): the operations, revisions, results and error codes `kapi apply` and `apply_edits` speak, and the service that applies them
- [E-02: The format system](../engine/e-02-format-system.md): the writer capabilities behind `editable` / `round_trip` / `generative`
- [E-06: Execution trust](../engine/e-06-execution-trust.md): why code-executing tools stay off the agent surface
- [C-04: Block state and the decision record](../context/c-04-block-state-and-decisions.md): what a `decide` operation records
- [C-06: Context retrieval](../context/c-06-retrieval.md): the two questions, and why one is a resource
- [C-07: Voice profiles](../context/c-07-voice-profiles.md): the voice an assistant writes in
- [M-01: Bilingual interop](../multilingual/m-01-bilingual-interop.md): the `extract`/`merge` round-trip that `inspect`/`apply` mirror on the monolingual side
- [A-01: Testing and documentation](../assurance/a-01-testing-and-documentation.md): the eval band the skill and MCP measurements belong to
- [MCP reference](/reference/mcp): the generated tool and resource surface
- [`kapi inspect`](/reference/commands/inspect) and [`kapi apply`](/reference/commands/apply): the read and write legs, per-flag
