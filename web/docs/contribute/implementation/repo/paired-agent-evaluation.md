---
sidebar_position: 5
title: Paired agent evaluation
description: Compare the same content tasks through ordinary file tools, the kapi CLI skill, MCP and the edit commands without a project, with explicit model identities and bounded subscription usage.
---

# Paired agent evaluation

The paired mode in `scripts/skilleval` measures the contribution of each kapi
integration within a fixed agent and model. It runs identical tasks with ordinary
file tools, the shipped CLI skill, MCP, and the edit commands without a project.
Each condition receives the same files and starting content. The comparison
covers each integration as a package, including its instructions and tool
interface.

The existing `skill-eval`, `skill-eval-completion` and `mcp-eval` targets measure
their own scenario sets. Use the paired targets for a comparison across the same
tasks. See [S-03](../../architecture/surfaces/s-03-agent-surfaces.md) for the
interfaces and [A-01](../../architecture/assurance/a-01-testing-and-documentation.md)
for the evidence policy.

## Conditions

| Condition | Available kapi integration | Ordinary file tools |
| --- | --- | --- |
| `baseline` | None | Available |
| `skill-cli` | Shipped skill and CLI, without kapi MCP | Available |
| `mcp` | kapi MCP and the CLI on PATH, without the skill | Available |
| `project-free` | The same binary under an evaluation-only multi-call name that offers `inspect`, `apply`, `formats` and the toolbox with project discovery off, and a skill that carries the shipped skill's edit and toolbox guidance with the names changed and the project parts left out | Available |

The manifest identifies each agent host, model and effort setting. Comparisons
keep these settings fixed within each host. Effort labels across providers do
not represent equivalent computation. Preserve separate results for different
models and document families.

Each attempt has a private command path, fresh agent configuration and a
temporary directory of its own, and kapi records every shell call in a cell as
an agent's. The workspace is a git repository with the project committed, and
the binary under test is linked into the cell, so no command the agent sees
names a path into the checkout. The transcript audit invalidates observed use
of the wrong kapi interface: an MCP tool outside the MCP condition, or a kapi
binary run by a path outside the cell, which would run a build other than the
one under test. It reads which programs a shell command runs, so a command that
only names a kapi path, such as a listing of the condition's own skill folder,
is no route. A bare kapi name the cell's PATH does not hold cannot run there,
and a skill the condition does not install loads nothing; the audit records
both as attempts rather than ending the session. It also records each path a
tool call names outside the cell: another attempt or the study's records, the
checkout, the home directory or a shared temporary directory. A reviewer reads
such an attempt's transcript before counting it. This controls accidental
mixing of integrations; it is not a boundary against hostile code. Keep hidden
evaluation artifacts outside the agent's workspace and inspect smoke
transcripts before a scored study.

Codex's built-in MCP resource helpers can appear under the host name `codex`
when listing resources without a server argument. The MCP condition permits
those discovery calls and reads explicitly targeting `kapi`. It still rejects
foreign server targets and resource-helper use in other conditions.

The CLI wrapper binds `KAPI_PROJECT` to the fixture's absolute recipe path and
clears `KAPI_NO_PROJECT` for that invocation. Explicit environment binding resolves
before directory discovery. This supports commands without a recipe flag,
such as `ksed`, which finds the project of the files it edits. The surrounding
agent environment retains `KAPI_NO_PROJECT=1`, isolated configuration and plugin
discovery. MCP binds the recipe through its explicit `-p` argument. The
project-free condition links the binary under its multi-call name, which turns
discovery off and refuses `-p`.

Every Claude run reads only the `project` setting source, which is where a
skill installed in the workspace is discovered, with a strict, explicit MCP
configuration that is empty outside the MCP condition. Conditions without a
skill also disable skill loading. Claude Code's bundled skills are off in every
condition, so the conditions differ in kapi's skill alone; Codex shows its own
system skills alike in every condition. Claude Code otherwise gives every
session the same temporary directory, and Codex's workspace-write sandbox lets
every session write `/tmp`, so a file one session left there could reach the
next. Each cell's `TMPDIR` is therefore a directory of its own, which Claude
Code also uses for its own temporary files, and the Codex sandbox writes only
the workspace and that directory. The isolated Codex MCP condition sets the
fixture server's `default_tools_approval_mode` to `approve` while retaining an
overall approval policy of `never`. A tool that still requires a prompt cannot
execute under that overall policy. This setting applies only to the prepared
fixture server; the runner does not modify the maintainer's configuration. See
the host docs for [Claude skill discovery](https://code.claude.com/docs/en/agent-sdk/skills)
and [Codex MCP tool policy](https://learn.chatgpt.com/docs/extend/mcp?surface=cli).

## Build and preflight

Build kapi once before preparing a study:

```sh
make build
make paired-eval-preflight
```

The study runs `bin/kapi` from the checkout and never a kapi found on PATH; it
refuses to start when that binary is missing or was built from another commit
than the checkout's HEAD. When a study starts, it copies the binary and the
shipped skill into the study directory, and every session takes them from
there, so a rebuild or an edit in the checkout during the study changes nothing
it measures.

Check the CLI wrapper against that binary without launching an agent:

```sh
PAIRED_TEST_KAPI="$PWD/bin/kapi" go test ./scripts/skilleval -run TestPairedCLIWrapper
```

Preflight prepares configurations without inference. The default manifest is
`scripts/skilleval/testdata/paired-study.json`; output defaults to
`kapi-paired-eval` under the system temporary directory. Override them with
`PAIRED_EVAL_MANIFEST` and `PAIRED_EVAL_DIR`. The cells live in the output
directory, so the runner refuses one that lies inside a checkout or under any
instruction file, host configuration or kapi recipe an agent would find by
walking up from its cell: the checkout's `CLAUDE.md`, `AGENTS.md` and skills
would reach every condition, and a host lists no instruction file it loaded.
macOS removes temporary files a few days after their last use, so copy the
evidence somewhere lasting once a study is scored.

For MCP, preparation also starts the isolated kapi server, initializes its
protocol and lists its tools and context resources. Missing edit tools, a
missing check tool or failed discovery block inference.

Preparation then reads each cell's surface through the agent host, with the
executable, arguments, environment and directory the session uses and no model
call. For Claude it reads the `system/init` event of `claude --print` with the
model endpoint pointed at a closed local port and a placeholder credential, so
the session ends before a request leaves the machine. For Codex it reads
`codex debug prompt-input`, which lists the skills the model would see with
each skill's root and the folders its sandbox may write, and `codex mcp list`.
A cell fails when it shows another condition's skill, MCP server or kapi
command, lacks its own, shows any skill beyond its own and the host's system
skills, shows a plugin or MCP server of the developer's own, or lets the agent
write outside its workspace and temporary directory. Preflight probes one cell per host
and condition and exits non-zero on any finding, and each live session probes
its own cell again before it starts. These records establish what the host
exposes; whether the model used it is a separate observation from the live
transcript.

A study's fingerprint covers its manifest, its corpus, the runner's code and its
copies of kapi and the skill, and every run checks the copies before each
session. A run of a study whose fingerprint differs is refused rather than
silently combining incompatible attempts. To resume after the checkout moved on,
run the pilot again from a checkout of the commit the study started from; it
needs no build, because the study runs its own copy of kapi.

## Subscription batches

Live runs use signed-in subscriptions. They consume the account's allowance;
token counts or locally estimated dollar figures do not establish the remaining
subscription quota. Check account usage before commissioning a batch. The runner
does not enable extra credits or switch to API billing.

Input-token totals include reported cache reads and writes, with those counts
also retained separately. Host tokenization and subscription weighting differ,
so these observations do not establish equivalent allowance consumption.

The prepared environment excludes API keys and endpoint overrides. Claude uses
the existing subscription credential in memory, with token redaction on recorded
output. A cell receives the token as a fixed value it cannot refresh, so a
keychain token must stay valid for a session's limit and fifteen minutes more,
or the session is not started; a long-lived token from `claude setup-token`,
exported as `CLAUDE_CODE_OAUTH_TOKEN`, avoids the question for a long study.
Codex uses the signed-in account from a fresh configuration directory.
Prepared reports omit credential-bearing environment values. Review local
transcripts before sharing them even when automatic redaction passes.

```sh
make paired-eval-smoke
make paired-eval-score
```

The default persistent ceiling is six started attempts, including failures.
Review that batch before increasing `PAIRED_EVAL_MAX_ATTEMPTS`. Re-running a
command does not erase attempts already recorded under the study directory.
Diagnostic, smoke and pilot phases share the ceiling. Each live phase prints
its planned session count before it starts one.

The pilot target runs the manifest's whole grid of tasks, hosts, conditions and
repetitions:

```sh
make paired-eval-pilot
```

`PAIRED_EVAL_CONCURRENCY` sets how many sessions run at once, spread evenly
over the hosts: the default of 2 runs one session per subscription at a time,
for the whole run. A rate limit pauses only the host it reaches, and so does a
refused or expired login, or two sessions in a row that end within a minute
without completing. Running the same command again resumes the phase without
repeating a started attempt. There are no automatic retries:
`PAIRED_EVAL_RETRY=1` runs again the attempts a rate limit, an interrupt, a
failed launch or an infrastructure failure (a refused login, an overloaded
API, a lost network) cut short, keeps the first record as superseded, and
counts both against the ceiling. The summary leaves such attempts out until
they run again. A paired run has no overall deadline unless `-timeout` names
one in `PAIRED_EVAL_ARGS`.

The pilot remains subject to the same ceiling. Raising the ceiling authorizes
additional subscription use; select it deliberately after reviewing the
preceding batch. `PAIRED_EVAL_ARGS` passes additional paired-runner options.
Run the evaluator's help for the available flags.

## Tasks and scoring

The corpus holds one task for each family the edit contract's evaluation names:
a wording and link edit in HTML and Markdown, a plural branch, a new edition in
another language that keeps the markup, a bilingual PO edit, a key added to a
JSON catalog, recovery from a stale read, and recovery from a refusal by the
check at commit. Every task's project binds one voice whose terms forbid one
word, and a style guide in the workspace states the same rule.

Two tasks act on the session while it runs. In the stale-read task the runner
watches the host's stream and, once a tool result has shown the agent the text
another editor changes, changes it in the paragraph the agent edits. The
record says whether the change landed before the agent's write, after it, or
never, and the file is graded against the reference with the other editor's
change when it landed and without it when it did not. The score report counts
recovery over the attempts whose change landed before the agent wrote, and over
those that met a conflict signal. In the refusal task the request names the
forbidden word, so the conditions with a project meet the check, and the
attempt fails if its transcript shows an attempt to land the edit over it.

Independent validators read the output files with parsers of their own, never
kapi's readers. Every task fails when nothing changes; every fixture file
outside the edit must stay byte-identical; and no file may appear in the task's
directories except one it creates. The task's criteria then compare bytes with
a reference output, compare JSON leaves in document order, or compare a
Markdown page's structure with its source. A criterion may be informational:
reported, never deciding the outcome. Each task's reference change sets, run
through `bin/kapi` by a test, reach the reference output and pass the task's
criteria, so a failure in a kapi condition is a finding about the agent or the
surface rather than about the task.

Each attempt also records its duration, tokens, Claude's turn count, tool
calls, the refusal codes its tool results carried (a change set that does not
decode is counted by the field kapi names), override attempts, kapi names it
tried that its cell does not hold, and paths it named outside its cell. The
score report summarizes these by task, host and condition. Each verdict is the
one recorded when its attempt finished; the report says beside it whether the
graded files still match, so review copies of the workspaces.

### Integration use and interpretation

Natural task prompts measure end-to-end behavior, including discovery. Record
failure to use an available integration as an outcome. Explicitly instructed
diagnostic runs answer a separate execution question and retain their own labels.

For each attempt, inspect the transcript for skill loading, context retrieval
and actual kapi invocations. A completed session can use ordinary file tools
throughout. Preserve these attempts in the assigned condition: excluding them
would hide discovery failures. Report observed use alongside completion and
artifact checks.

`make paired-eval-diagnostic` explicitly instructs each configured host to use
its condition's integration on the smoke task: the skill, MCP, or the
project-free commands. It omits the baseline condition. Each diagnostic asks
for the condition's read and write route and for every refusal to be reported.
Missing capabilities must be reported rather than repaired inside the attempt.
Results retain `phase: "diagnostic"`; completion and artifact criteria still
require transcript inspection to establish integration use.

Select particular attempts with `-paired-sessions`, using the task, host,
condition and repetition from their schedule IDs:

```sh
make paired-eval-diagnostic PAIRED_EVAL_ARGS='-paired-sessions recover-stale-read-claude-skill-cli-01,recover-stale-read-codex-mcp-01'
```

Selection preserves schedule order and the shared attempt count. It does not
authorize retries. Run the evaluator directly with `-paired-phase diagnostic`
and without `-paired-live` to prepare those diagnostics without inference.
Each preparation saves its exact `prompt.txt` beside the transcript, outside the
agent's workspace. Natural prompts remain unchanged by diagnostic instructions.

A passing artifact check establishes its declared conditions. Human reviewers
assess meaning, suitability and acceptance using a task rubric; absent labels
remain pending. The evaluator's own checks cannot establish general writing
quality or reviewer time savings.

Keep failed, interrupted and capped attempts in the record. Read paired outcomes
within each host and task family, and use document-level uncertainty for scored
studies. Repeated runs on one document do not create independent documents.

## Evidence handling

Keep raw transcripts, generated artifacts and private review records in ignored
local output. Inspect and scrub evidence before publication. The study records
its manifest, implementation and input identities so incompatible runs can be
distinguished. Scoring reads saved attempts without launching an agent.

Model-backed checking is a separate intervention. Record the checker model and
prompt independently from the authoring agent, include its resource use, and
compare against an equivalent additional review pass when attributing benefit.
