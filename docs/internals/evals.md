# Tests and evals

The `/evals` page publishes evaluation evidence from `scripts/evalindex`.
This guide explains how to register evaluations and publish their results.

## Three bands

Evaluations are grouped by the subject under test.

| Band | Subject | Evidence | Gates CI |
| --- | --- | --- | --- |
| Engine and formats | kapi's own code | deterministic | yes |
| AI and context | model output under governance | sampled | no |
| Agent skills | an agent driving kapi | scenario-scored | no, never runs in CI |

Within each band, evaluations follow the six architecture decision series
(F, E, C, M, A, S). Each evaluation appears beside its relevant decision.
Series without evidence remain visible as coverage gaps.

## Adding an eval

1. Write the harness. It owns its own cadence and its own dataset.
2. Add a card to `scripts/evalindex/evals.go` and list its id under a layer in
   `registry.go`.
3. `make eval-index`, then `go test ./scripts/evalindex/`.

Run `make eval-index` after **re-running** an eval too, not only after adding
one. The index records each dataset's date, so a refresh makes the committed
index stale and its drift test fails. The format-maturity publish
(`scripts/format-ops/bootstrap-publish.mjs`) rebuilds the index as its last
step, and `make pre-push` runs `go test ./scripts/evalindex/` whenever a
dataset, the index, `scripts/evalindex/` or the Makefile changes.

Registration tests verify that each card references an existing Makefile target,
page and dataset. Judged evaluations must declare their validation status.
Use the `Misses` field to state limitations and unmeasured behavior.

## Datasets record their own date

`scripts/evalindex/freshness.go` reads timestamps from datasets. Give new
harness output a `generated` field so readers can assess its age. Do not
enter dates manually on cards or store computed ages in committed artifacts;
the page calculates age from the recorded date.

## Each card shows its number

`scripts/evalindex/headline.go` extracts a headline result from each dataset.
Register an extractor there instead of entering a result manually on the card.
`TestARegisteredHeadlineResolves` fails if a registered extractor returns no
result, including when the dataset schema changes.

Ensure comparisons use equivalent inputs. For example, comparing total engine
runtimes is misleading when the engines succeed on different files. A speed
comparison must use the files processed successfully by both engines.

## Dataset publication guards

Two CI guards apply to published datasets:

- **No absolute home paths** (`scripts/check-abs-paths.sh`). Scrub recorded
  paths before writing transcripts and benchmark errors. The guard permits
  placeholder user names `me`, `dev`, `demo`, `user`, `test` and `you` for
  path-scrubbing tests.
- **No downstream product references** (`scripts/check-docs-bowrain-clean.sh`).
  Content published under `web/src` must follow this rule, including scenario
  text and agent transcripts.

When a transcript contains a downstream product reference, `skilleval` omits
it from publication and records the omission. The result retains its verdict,
gate results, message counts and file changes. The report counts omissions so
readers can see what evidence is unavailable. This publication guard does not
apply to runs written elsewhere with `-out`.

## Evals that spend

Evaluations that call a model commit their datasets so builds can use the
results without making paid calls. Record sampling settings on the card and
set them explicitly in the harness:

```go
Temperature: new(0.0)   // request greedy decoding
```

`Config.Temperature` is a `*float64` so an explicit zero remains distinct from
an omitted setting. `TestEveryProviderSendsTemperature` verifies that providers
forward the configured value. A fixed temperature reduces sampling variation;
it does not guarantee identical results across runs.

## Validating a judge

A judged score cannot be trusted above the judge's measured agreement with a
person, and until that measurement exists the dashboard withholds the judged
dimension. Three steps, and the middle one needs a human:

```bash
make judge-candidates   # sweep and save every scored translation (costs calls)
make judge-label        # answer y/n per criterion, resumable, roughly 20 minutes
make judge-validate     # measure Cohen's kappa and record it in the history
```

Labeling hides the judge's verdict, producing model and experimental condition
to reduce bias. Items are shuffled with a fixed seed, preserving the order
when a session resumes. Record uncertain items as skipped; the report counts
them separately.

The minimum is 100 labeled items, with a target of 150 per session. Smaller
samples can produce confidence intervals too wide to assess agreement.

## Evals that drive an agent

`scripts/skilleval` runs the agent-skill and MCP scenarios. It never runs in CI:
it drives `claude -p` with local credentials and consumes account allowance.
Builds use the committed dataset and display its measurement date.

```bash
make skill-eval             # does the skill fire (cheap, 4-turn cap, 3 repeats)
make mcp-eval               # does an agent pick the right MCP tool
make skill-eval-completion  # does it finish the job (slow, 40-turn floor)
```

The same program runs the agent evaluation, which scores whether agents apply
and record a project's conventions against a generated fixture's answer key.
Its phases and budget are in the [agent evaluation runbook](agent-evaluation.md).

When adding a scenario:

- **Provide realistic fixtures.** Every file named in the prompt must exist.
  A cross-format task needs content in multiple formats; a Markdown-only task
  may be better served by native search tools.
- **Separate trigger and completion budgets.** Trigger mode uses a short turn
  cap to check skill selection. Completion mode needs enough turns to finish
  the task and run its gate.
- **Define a completion gate.** Scenarios without one receive `no gate` and
  are counted separately from passes.
- **Pass `-p .` to project commands in gates.** The isolation contract sets
  `KAPI_NO_PROJECT=1`, disabling automatic recipe discovery.

Run each gate against both incomplete and completed workspaces.
`TestAGateIsRedBeforeTheAgentRuns` verifies that every gate fails against its
initial fixture. Use `kapi voice validate` to assess whether a profile is
usable: a check alone can report no findings for an empty profile.
`TestEveryFixtureRecipeLoads` verifies fixture recipes through kapi's loader.

### The session is published, not summarised

Each session records assistant messages, tool calls and tool results. These
records let readers investigate outcomes beyond the dataset's counts and tool
list.

Per-scenario transcripts are staged under `web/static/skill-eval/transcripts/`
and published to the documentation CDN with the run artifacts. The page fetches
a transcript when its row is opened, keeping it out of the main dataset bundle.
Transcripts retain complete messages and tool results. `Run.record` scrubs
paths before storing events.

Pruning is scoped to the evaluated surface: `-only mcp` replaces MCP
transcripts while retaining skill transcripts. A run written elsewhere with
`-out` keeps events inline. Use `-transcripts <dir>` to write separate files.

## What the control arm found

The first fully gated sweep with the unaided control, over 17 scenarios:

```
kapi enabled 3, eased 1, hindered 3, neither 10
```

`hindered` means the agent with kapi failed where the unaided one passed. This outcome is counted separately from scenarios where both arms succeed or
both fail.

Two of the three are the same failure: **the kapi route extracts a catalog and
stops.** p09 produced `i18n/src/App.klf` and never touched `src/App.jsx`; p14
produced the catalog, edited `App.jsx`, and left `<h1>Welcome back, Alex</h1>`
in it. Neither app is translatable, and both look finished from the catalog
alone. The completion gate therefore checks both catalog creation and removal of
the hardcoded string from the component.

The third is #2227.

The counts are also not the whole comparison. The unaided arm was shorter on
most scenarios and often several times shorter, so the page reports the message
totals beside the outcome counts. In this sweep, some tasks required kapi, while others took fewer messages
without it.

## WP5: the paired evaluation of the edit contract

WP5 in the [edit model](edit-model.md) runs the paired agent evaluation before
kapi.change/v1 is frozen, and section 15.2 adds three measurements for the
neo/kapi question (D14). This section holds the study's design, the two offline
measurements, and the place its results go.

### Design

The study runs `scripts/skilleval` in paired mode over the manifest
`scripts/skilleval/testdata/paired-study.json`: two hosts, four arms, seven
task families and three repetitions, 2 × 4 × 7 × 3 = 168 live sessions. A
rerun's manifest, `paired-rerun.json`, names five arms and eleven tasks (the
fifth arm and the four variants below), 2 × 5 × 11 × 3 = 330 sessions; set
`PAIRED_EVAL_MANIFEST` to it. Each
host keeps one model and effort for every arm: Codex with `gpt-5.6-terra` at
medium effort, Claude Code with `claude-sonnet-5` at high effort. A session has
600 seconds and 40 turns. The schedule shuffles blocks of task, host and
repetition, then the four arms within each block, from the manifest's seed.

| Arm | kapi surface in the cell | Project |
| --- | --- | --- |
| `baseline` | none: no skill, no MCP server, no kapi name on PATH | none |
| `skill-cli` | the shipped kapi skill; `kapi` on PATH | the fixture's recipe, bound through `KAPI_PROJECT` |
| `mcp` | the kapi MCP server (its default writing set); `kapi` on PATH; no skill | the fixture's recipe, bound with `-p` |
| `project-free` | `kapi-files`, the same binary under a multi-call name (`cli.BusyboxRoot`) exposing `inspect`, `apply`, `formats` and the toolbox with discovery off, and a skill with the shipped skill's edit and toolbox guidance | none: the cell holds no recipe, no `STYLE.md` and no `.kapi` |
| `kapi-no-project` | the shipped kapi skill; `kapi` on PATH with `KAPI_NO_PROJECT=1` and no `KAPI_PROJECT` | none, as for `project-free` |

Every arm keeps the host's ordinary shell and file tools. A cell of
`baseline`, `skill-cli` or `mcp` holds the task's content, its `kapi.yaml`, a
`STYLE.md` and the project's `.kapi` context, and its store holds the same
context, imported before the session starts: a voice. A cell of
`project-free` or `kapi-no-project` holds the task's content alone, so what an
agent there does owes nothing to files a project put beside it. The gate task
adds one term rule while the session runs (below), which forbids `portal` and
names `overview page` in its place. No skill an arm installs and no MCP tool
description uses the word, which a test asserts, so no arm is primed for or
against it.

The project-free skill is the shipped skill with the names changed and what
needs a project left out. Its `SKILL.md` keeps the shipped one's description
and, where the shipped one lists its project habits, says that no voice, terms
or check applies. Both tell an agent to read `references/edit.md` before it
changes content inside a file, in the same sentence (the shipped skill adds
`kapi help edit`, which prints the topic where `kapi init` installs the skill
without its references), so an agent in either arm is one step from the edit
guidance. Its `edit.md` and `toolbox.md` are the shipped references with the
project, MCP, check and translation passages removed. A test asserts that both
skills carry that sentence, that every example the references give is one the
shipped references give, save the catalog example's second language, which a
file without a project refuses, and that they name nothing of the tasks.

kapi records every shell call in a cell as an agent's (`KAPI_ACTOR=agent`), so
the actor policy treats both hosts alike. The workspace is a git repository with
the project committed and a clean status, as a project an agent works in is, so
`git diff` and the skill's `kapi check --diff-against HEAD` work. The binary
under test is hard-linked into the cell, so the agent's commands never name a
path into the checkout, and Claude's sandbox denies reading the checkout, the
repository's main checkout when the study runs from a worktree of it, and the
attempt's own records.

Section 15.2 item 1 asks for the alias against kapi with and without a project.
The WP5 grid held four arms: `skill-cli` and `mcp` are kapi with a project, and
`project-free` is the binary without one. `kapi-no-project` is kapi's own CLI
and shipped skill with discovery off, the fifth arm a rerun of the question
adds: its edit commands are the code paths the alias runs, and what it
measures is the shipped skill's text and kapi's extra commands where no project
exists.

The tasks follow: the seven the WP5 grid ran, one per family, and four
variants of those families whose edit a unique string cannot reach, which the
rerun manifest adds. Every task fails when nothing changes, every fixture file
outside the edit must stay byte-identical, and no file may appear in the scoped
directories, or directly in the workspace root, except one the task creates.

| Task | Family | Files | Graded by |
| --- | --- | --- | --- |
| `edit-link-html-md` | wording and link | `site/help.html`, `docs/help.md` | byte diff against the reference; a FAQ link shares the old address as a prefix and a code block holds it verbatim |
| `edit-plural-branch` | plural branch | Flutter ARB `lib/l10n/app_en.arb` | byte diff; the edited `one` branch, the `=0` and `other` branches with the plural's syntax around them, and the `@inboxCount` metadata each checked byte for byte; the other branches also contain the words being replaced |
| `add-edition-markup` | new edition | `docs/en/welcome.md` to a new `docs/nb/welcome.md` | structure: headings, list items, bold spans, inline code and link addresses in order; every block translated; Norwegian Bokmål by its function words; byte equality with the reference reported only |
| `edit-po-context` | bilingual PO | `locales/nb/messages.po` | byte diff; two entries share `msgid "Book"` and differ by `msgctxt` |
| `add-json-key` | key added (`insert_block`) | German `i18n/de/messages.json`, in a project whose source language is German, unlike the shipped skill's English catalog example | the JSON leaves in document order, named by JSON pointer, so the key lands in `settings` after `importData`; layout byte equality reported only |
| `recover-stale-read` | stale recovery | `docs/en/upgrade.md` | byte diff of the agent's sentence and another editor's change to the same paragraph |
| `recover-gate-refusal` | gate refusal recovery | `docs/en/reports.md` | the first paragraph keeps its text and gains a sentence that names CSV and does not say `portal`; every other block unchanged; no `portal` anywhere; no override attempt in the transcript; naming the overview page reported only |
| `edit-plural-equal-branches` | plural branch | ARB `messages/reminders_en.arb` | byte diff; the `one` and `other` branches hold the same text on one line, and only `other` changes |
| `edit-nested-select` | plural branch | ARB `messages/invites_en.arb` | byte diff; a plural inside a select, where the `one` branch under `adviser` changes and the one under `other` stays |
| `edit-po-wrapped` | bilingual PO | `locales/nb/booking.po` | the catalog's entries, each string read with its continuation lines joined, against the reference, so the words changed across the `msgstr`'s line break count however the result is wrapped; the original wrapping reported only |
| `add-key-every-language` | key added (`insert_block`) | `app/i18n/en.json` and its translations `nb.json` and `de.json` | the JSON leaves of all three files, so the key lands in the same place in every language |

The plural task is an ICU plural in a Flutter ARB catalog, so it measures the
contract's branch selector. kapi reads the message as one block holding a
plural run (P5 in the edit model), and a read lists it under `structures` with
its path and the text of each branch:

```json
"structures": [{"path": [0], "kind": "plural", "pivot": "count",
  "branches": {"=0": "No new messages", "one": "<x id=\"p1/\"/> new message",
               "other": "<x id=\"p1/\"/> new messages"}}]
```

An edit reaches the `one` branch with `path` `[0, {"plural": "one"}]`, by
`replace_text` or `set_content`; `set_content` with the block's text, which
shows the `other` branch, is refused as a flattening guard. A `replace_text`
whose `find` is in the branches and not in the text around the plural is
refused `not_found` with the path of each branch that holds it. The text of a
`set_content` may spell the argument as `{count}`, as the prompt does, or send
its code. The task's reference route is that `replace_text`.
`TestPairedPluralRouteOnEverySurface` sends it, and a `set_content` of the
branch that types `{count}`, through `kapi apply` in the project, `apply_edits`
on the MCP server and `kapi-files apply` without a project; each result passes
the task's graders.
The other plural in the catalog and the `@inboxCount` placeholder declaration
are there to be left alone.

Two tasks act on the session while it runs. In `recover-stale-read` the runner
watches the host's stream and, once a tool result has shown the agent the text
"The upgrade keeps your appointments", changes it to "The upgrade keeps all
your appointments". The text sits on the line of the sentence the agent edits,
so a search that prints that line lands the change, and a patch made from the
old line meets a conflict. The prompt asks for the agent's sentence and says
nothing of another editor, so an agent learns of the change only from what its
tools report. A command that names the text without printing it does not land
it. A kapi arm's write against the old revision is refused `stale`.
Claude Code 2.1's Edit tool applies an edit to a file changed since it was read
and adds a note saying so, which the record counts as `host:stale`; Codex's
patch fails when its context lines changed (`host:patch_failed`). The record
says whether the change landed before the agent's write, after it, or never: an
agent that writes with `ksed` before reading never sees it, and the file is
then graded against the reference without the other editor's change. The
score report counts recovery over the attempts whose change landed before the
agent wrote, and separately over those that met a conflict signal.
`recover-gate-refusal` asks for a sentence "from the portal". The term rule
against `portal` is the task's late context: once a tool call has read the
project's context (a `kapi context`, `voice`, `terms` or `check` command, the
context MCP tools, or `STYLE.md`), or at the agent's first write if it reads
none, the runner adds the rule to the voice file the fixture imports and to
`STYLE.md`, and imports the context again, so the project's terms store holds
it, as when a person adds a rule while the agent works.
The agent's write then meets the commit check rather than a rule it already
followed. In the two arms with a project, `kapi apply` and `apply_edits`
refuse it `gate_failed` and name the replacement, and an agent's `--gate
report` is refused `not_permitted`. The baseline arm has no gate, so for it the
task measures whether the agent reads `STYLE.md` again; a cell with no project
holds no context for the rule to join. The grader asks for a sentence about
CSV downloads without the forbidden word; a sentence that names the overview
page is reported, and one that refers back to the overview page the paragraph
already names also passes. An attempt that changed no task file and ended on a
question to the person is scored as its own outcome, `asked`, beside passed,
failed and unchanged.

Beside the graders, each attempt records its status, duration, input and output
tokens (cache reads and writes kept apart), Claude's turn count, tool calls, the
refusal codes its tool results carried (kapi's own, `host:stale` or
`host:patch_failed` for a host tool's, `invalid:<pointer>` for a change set
that did not decode, with each array position as `*`, and
`write:not_read_back` for a write a format refused because the value would read
back as another message, such as an unquoted brace in an ARB branch), override attempts
(`--gate report`, a change set with `"gate": "report"`, `KAPI_ACTOR=person`,
`if_match: "*"`), kapi names and skills it tried that its cell does not hold,
and paths it named outside its cell. kapi merge's `not merged:` lines count
as refusals under `merge:<code>`. Each attempt also records its write route
(`contract` through `kapi apply`, `ksed -i` or `apply_edits`; `merge`;
`native` through the host's own edit, write or patch tools or a shell command
rewriting a task file; `none`), the files it wrote directly in the workspace
root, whether it kept them or not, and its writes to the context store. An mcp
attempt records what its session shows of the kapi tools the host gave the
model: the tools Claude's `system/init` declared, or for Codex, which declares
none, a kapi tool call; a declared list without them fails the attempt as
`mcp_absent`. For a Codex session the runner reads the session rollout, which
holds the rejected patches and `apply_patch` calls the exec stream leaves out,
and takes the call count, refusals and patch routes from it where the stream
showed fewer. The score report lists the decode errors per task, host and arm:
they are the names and shapes agents reach for that the contract does not take.
It gives the medians of each cell by write route too, counts the root and
context-store writes, and leaves an attempt whose host reported no token use
out of every median alike. The Bokmål check of `add-edition-markup` asks for a
form only Bokmål writes (`deg`, `inn`, `etter` and the like) and none only
Danish or Swedish writes, since both share most of its function words, and the
translation check fails on any four words of the English in a row, a link's
text included.

A session cut short by the service or the machine is an infrastructure failure
and runs again under `PAIRED_EVAL_RETRY=1` rather than being scored: a refused
or expired login, an overloaded API (a 529, "overloaded", "at capacity"), a
dropped network, or a disk that filled during the session, which the runner
finds in the transcript or standard error however the session ended. A login
failure or a full disk pauses the run.

`TestPairedSolutionsThroughKapi` sends each task's reference change sets
(`testdata/paired/solutions/`) through this tree's `bin/kapi` in the task's
project, draws the refusals the recovery tasks are built on, and checks that the
result passes the task's own graders; `TestPairedPluralRouteOnEverySurface`
does the same for the plural task through the CLI, the MCP server and the
project-free alias. A kapi arm's failure is therefore a finding about the agent
or the surface rather than about the task.

### Isolation proof

Before any inference, each cell's surface is read through the host itself,
with the executable, arguments, environment and directory the session uses,
and no model call:

- Claude: the `system/init` event of `claude --print`, with the model endpoint
  set to a closed local port and a placeholder credential, so the session ends
  before a request leaves the machine. Every Claude run passes
  `--setting-sources project`, `--strict-mcp-config` and an explicit MCP
  configuration, empty outside the MCP arm.
- Codex: `codex debug prompt-input`, which renders the model-visible skill list
  with each skill's root, and `codex mcp list --json`, both under the cell's own
  `CODEX_HOME`.

A cell fails the check when kapi's skill appears outside `skill-cli`, the
alias's skill outside `project-free`, any other skill but the host's system
skills is visible, an MCP server or `mcp__` tool outside the MCP arm, the edit
tools are missing from the MCP arm, the kapi names on its PATH differ from the
arm's, a plugin of the developer's own is visible, a skill root lies outside
the cell, or Codex's sandbox would let the agent write outside its workspace
and its own temporary directory. `make paired-eval-preflight` probes one cell
per host and arm and exits non-zero on any finding, and each live session
probes its own cell again before it starts. On 3 October 2026:

| Host | Arm | kapi skill | Skills visible | MCP servers | kapi names on PATH | Problems |
| --- | --- | --- | --- | --- | --- | --- |
| claude | baseline | none | 0 | none | none | 0 |
| claude | skill-cli | `kapi` | 1 | none | `kapi` | 0 |
| claude | mcp | none | 0 | `kapi` (10 tools) | `kapi` | 0 |
| claude | project-free | `kapi-files` | 1 | none | `kapi-files` | 0 |
| codex | baseline | none | 4 | none | none | 0 |
| codex | skill-cli | `kapi` | 5 | none | `kapi` | 0 |
| codex | mcp | none | 4 | `kapi` (10 tools) | `kapi` | 0 |
| codex | project-free | `kapi-files` | 5 | none | `kapi-files` | 0 |

The MCP column says the server answered the runner's own handshake
(`direct-server-discovery`). For Codex it does not show what the model was
given: an mcp attempt records that from its own session (Design).

Claude Code's bundled skills are off in every arm
(`CLAUDE_CODE_DISABLE_BUNDLED_SKILLS`, and a skill override in the workspace's
project settings for the two that switch leaves on), so each Claude arm shows
its own skill and nothing else. Codex shows the same four system skills from the
cell's `CODEX_HOME` in every arm. Each cell has a temporary directory of its
own, `TMPDIR` for both hosts and `CLAUDE_CODE_TMPDIR` for Claude Code, created
in `/tmp` because Claude Code puts sockets under it and falls back to the
shared `/tmp/claude-<uid>` above 44 bytes; the runner moves it into the cell's
`state/` when the session ends. Claude's sandbox denies reading `/tmp/claude`
and `/tmp/claude-<uid>`, and Codex's sandbox excludes `/tmp`: with the
workspace-write defaults, `codex debug prompt-input` lists `/private/tmp` among
the writable roots, and with the cell's configuration it lists only the
workspace and the cell's temporary directory.

### Running it

Run it in a plain terminal rather than from an agent's shell, which has a time
limit and a sandbox of its own, and from a worktree of `origin/main` made for
the study alone, beside the main checkout rather than inside it. The main
checkout is shared with other work and rebuilt during the day, and the
workflow harness creates and removes its agents' worktrees under
`.claude/worktrees/`; the study's worktree is touched by nothing else until the
study is scored. From the main checkout, once this change has merged:

```bash
git fetch origin
git worktree add ../neokapi-wp5-study origin/main
cd ../neokapi-wp5-study
make i18n-catalogs && vp install && make build
PAIRED_TEST_KAPI="$PWD/bin/kapi" go test -tags fts5 ./scripts/skilleval \
  -run 'PairedSolutions|PluralRoute|ProjectFreeAlias|WithBuiltKapi'
# Copy both hosts where no upgrade reaches them, and run them from there.
hosts="$HOME/kapi-wp5-hosts"
mkdir -p "$hosts"
cp -R "$(dirname "$(realpath "$(command -v claude)")")" "$hosts/claude"
cp -R "$(dirname "$(dirname "$(realpath "$(command -v codex)")")")" "$hosts/codex"
export PATH="$hosts/claude:$hosts/codex/bin:$PATH"
make paired-eval-preflight PAIRED_EVAL_DIR="$HOME/kapi-wp5-study"
# The first stage: the plural task on both hosts and in every arm.
caffeinate -i make paired-eval-pilot PAIRED_EVAL_DIR="$HOME/kapi-wp5-study" \
  PAIRED_EVAL_MAX_ATTEMPTS=8 PAIRED_EVAL_CONCURRENCY=2 \
  PAIRED_EVAL_ARGS="-paired-sessions edit-plural-branch-claude-baseline-01,edit-plural-branch-claude-mcp-01,edit-plural-branch-claude-project-free-01,edit-plural-branch-claude-skill-cli-01,edit-plural-branch-codex-baseline-01,edit-plural-branch-codex-mcp-01,edit-plural-branch-codex-project-free-01,edit-plural-branch-codex-skill-cli-01"
# Read the eight transcripts. The second stage: the other 160.
caffeinate -i make paired-eval-pilot PAIRED_EVAL_DIR="$HOME/kapi-wp5-study" \
  PAIRED_EVAL_MAX_ATTEMPTS=168 PAIRED_EVAL_CONCURRENCY=2
make paired-eval-score PAIRED_EVAL_DIR="$HOME/kapi-wp5-study"
```

The test line sends each task's reference route through the worktree's build
before any session is spent, and the plural task's through each arm's surface.
The preflight prints the checkout and host versions a study started then would
pin, and where each host runs from; it warns of a host in a Homebrew cask or
formula directory, which an upgrade replaces. For a long Claude run, export a
long-lived `CLAUDE_CODE_OAUTH_TOKEN` from `claude setup-token` first: a keychain
token must stay valid for a session's limit and fifteen minutes more, or the
session is not started.

Both hosts are Homebrew casks on the study machine, and an upgrade run from any
shell on it, another agent's included, replaces a cask's version directory. The
copies in `$HOME/kapi-wp5-hosts` keep the version the study pins whatever an
upgrade does to the casks: the Claude Code cask is one executable, and Codex
keeps its resources beside its `bin`, so each copy is the whole version
directory. Every shell that runs or resumes the study needs the same `PATH`.

The first stage is eight sessions of the grid chosen by name: the plural task's
first repetition on both hosts and in all four arms, so the ARB task, both
hosts and every arm have a live session, one host beside the other, before the
rest is committed. Without the selection the runner would take the first eight
of the seeded schedule, which are all Claude, on `add-json-key` and
`edit-po-context`, one after another. The ceiling counts every attempt the
study has started, the first stage's included, so the second stage's ceiling
of 168 runs the other 160 and nothing more. A rerun under `PAIRED_EVAL_RETRY=1`
counts against the ceiling too, so raise it deliberately, by the number of
reruns approved, when they are needed.

The cells live in the study directory, so it sits outside the checkout: the
runner refuses a directory with a checkout, an instruction file, host
configuration or a kapi recipe above it, which an agent in a cell would find.
A directory in the home folder survives a restart, which `/tmp` does not.

When the study starts it copies `bin/kapi` and the shipped skill into
`inputs/` in the study directory, and every session runs those copies. Its
`study.json` records, and its fingerprint covers, the checkout it runs from,
the commit, the hash of its copy of kapi and each host's `--version`. To resume
after an interrupt, a rate limit or a paused host, export the same `PATH` and
run only the `make paired-eval-pilot` line again from the same worktree; it
needs no build and no new copy of the hosts.
The runner refuses a run from another checkout, naming the one the study runs
from, and a run with `claude` or `codex` at another version than the study
started with, naming that version and where it ran from: a session on another
version would measure another agent, and putting the copy back first on `PATH`
resumes it. Each session's own version check uses the same pin, so a host
upgraded before its first session is refused too. A new directory would run
every session again.

The pilot phase is the manifest's whole grid. It prints the planned session
count before it starts one and runs one session per subscription at a time. A
host pauses on a rate limit, on a refused or expired login, and after two
sessions in a row that ended within a minute without completing; the other host
goes on. `PAIRED_EVAL_RETRY=1` runs again the attempts a rate limit, an
interrupt, a failed launch or an infrastructure failure (a refused login, an
overloaded API, a lost network) cut short, and keeps the first record as
superseded; the summary leaves such attempts out until they run again.

Each host runs its 84 sessions one after another, beside the other host. In
the smoke runs on 3 October 2026 the stale-read task took 10 to 37 seconds in
most sessions and 324 seconds in one, and preparing and probing a cell, with
the check of the study's copy of kapi, takes about ten seconds more. The other
tasks ask for more (a translation, three files), so an average of one to one
and a half minutes per session puts the study at about two to three hours; if
every session ran to its 600-second limit, it would take 14 hours.

### What the edit engine costs an embedder (15.2, item 2)

`examples/go-apply` applies a change set with `core/change` and the file home
over a directory, with every built-in format registered and nothing of the
host. Measured on 3 October 2026 with Go 1.27.1 on darwin/arm64:

| Measure | Value |
| --- | --- |
| `go list -deps` | 269 packages; 124 outside the standard library: 75 neokapi, 49 third-party |
| Third-party modules | beevik/etree, gabriel-vasile/mimetype, pmezard/go-difflib, yuin/goldmark, golang.org/x/image, x/net, x/sync, x/sys, x/text, gopkg.in/yaml.v3 |
| Packages from `host`, `cli`, `kapi` or `bowrain` | none |
| Packages with cgo files | none; `CGO_ENABLED=0` builds the same program |
| ICU | not linked; the binary links only libSystem and libresolv |
| Binary size | 18.6 MB (12.9 MB stripped); `bin/kapi` is 91.3 MB |

On the link task it writes the same bytes as `kapi apply`. A native,
project-free embedding of the edit engine needs neither cgo nor ICU, so the
audience for which 15.3 would build a separate artifact is already served by
the Go module.

### What a second artifact costs (15.2, item 3)

| Channel | Multi-call alias | Second artifact |
| --- | --- | --- |
| Release matrix (darwin/arm64, linux/amd64, linux/arm64, windows/amd64, windows/arm64) | nothing | five more builds per release |
| macOS signing and notarization | nothing | one more signed and notarized binary |
| Windows Authenticode, signed out of band by hand | nothing | two more executables to sign each release |
| cosign | nothing | signatures for five more archives |
| nfpm `.deb`/`.rpm` and the apt/yum repository | one symlink entry in `packaging/nfpm.yaml` | a second package or a second binary in `kapi-cli`, for two architectures and two formats |
| Homebrew | one `install_symlink` in `scripts/gen-brew-formula.sh` | a second formula in homebrew-tap |
| winget | no symlink on Windows: a `kapi` subcommand, or a second portable alias of the same executable | a new package identifier and a komac submission each release |
| `cli.json` self-update index | nothing: the update replaces the binary the link names | a second entry, and an updater that replaces two binaries |
| setup-kapi | one link step | download, checksum and cache logic for a second binary |
| Docker (`docker/kapi/Dockerfile`) | one `ln -s` | a second binary per architecture |
| Docs | a page | an install page, a reference and a product name |

Name availability, checked on 3 October 2026 against registry.npmjs.org, the
PyPI JSON API, Debian's archive (api.ftp-master.debian.org) and Ubuntu's
(api.launchpad.net), the winget source index (cdn.winget.microsoft.com) and
formulae.brew.sh:

| Name | npm | PyPI | Debian and Ubuntu | winget | Homebrew |
| --- | --- | --- | --- | --- | --- |
| `neo` | taken (a Geonames parser, 1.0.2) | taken (electrophysiology data, 0.14.5) | taken as a source package (python-neo; binary `python3-neo`) | free as a moniker and a command; `BrowserOS.BrowserOS.Neo` exists, and a name search for `neo` matches many packages | free |
| `kapi-files` | free | free | free | free | free |

### Results

The study ran on 3 October 2026 against `bin/kapi` `v1.3.0-rc3-21-g4adeb612f`
(origin/main at `4adeb612f`), with Claude Code 2.1.288 running
`claude-sonnet-5` at high effort and Codex CLI 0.160.0 running `gpt-5.6-terra`
at medium effort. All 168 sessions ran, and the score report is
`score-20261003T170129.321490000Z.md` in the study directory. Every count below
comes from the automatic graders, the result records and the transcripts; the
human review of the attempts is still pending for all 168.

Two sessions ended on a host failure before the agent could finish, and the
tables below leave them out, so two cells hold two attempts:

- `add-edition-markup-claude-skill-cli-02` stopped after 25 tool calls, part
  way through its recovery, with `write …/transcript.jsonl: no space left on
  device` (status `malformed_stream`).
- `recover-stale-read-codex-mcp-01` stopped after 3.9 seconds and no tool call
  with `agent error: Selected model is at capacity. Please try a different
  model.` (status `agent_failed`).

The study's score report counts both as attempts that did not pass, since its
runner retried neither. The runner classes both as infrastructure failures,
which `PAIRED_EVAL_RETRY=1` runs again (Design).

`add-edition-markup-claude-skill-cli-01` ran out of turns (41) with a file that
passes the graders. It is recorded `agent_failed` and counted here as passed,
as the score report counts it.

#### Outcome

Each cell gives attempts passed over valid attempts, then the medians of input
tokens, tool calls and seconds. Input tokens include cache reads, so they
compare arms within a host and not one host with the other.

| Task | Host | baseline | skill-cli | mcp | project-free |
| --- | --- | --- | --- | --- | --- |
| `edit-link-html-md` | claude | 3/3, 76k, 4, 12 s | 3/3, 285k, 11, 38 s | 3/3, 145k, 12, 18 s | 3/3, 411k, 14, 43 s |
| `edit-link-html-md` | codex | 3/3, 39k, 3, 21 s | 3/3, 205k, 9, 68 s | 3/3, 42k, 3, 23 s | 3/3, 159k, 8, 47 s |
| `edit-plural-branch` | claude | 3/3, 56k, 2, 9 s | 3/3, 371k, 13, 54 s | 3/3, 281k, 9, 27 s | 3/3, 333k, 12, 45 s |
| `edit-plural-branch` | codex | 3/3, 38k, 3, 19 s | 3/3, 185k, 10, 68 s | 3/3, 41k, 3, 19 s | 3/3, 135k, 8, 44 s |
| `add-edition-markup` | claude | 3/3, 75k, 3, 14 s | 2/2¹, 1.55M, 45, 248 s | 3/3, 217k, 11, 40 s | 3/3, 217k, 7, 48 s |
| `add-edition-markup` | codex | 3/3, 48k, 4, 28 s | 3/3, 333k, 13, 113 s | 3/3, 52k, 4, 32 s | 3/3, 73k, 5, 36 s |
| `edit-po-context` | claude | 3/3, 75k, 3, 14 s | 3/3, 267k, 10, 30 s | 3/3, 245k, 10, 27 s | 3/3, 207k, 8, 31 s |
| `edit-po-context` | codex | 3/3, 37k, 3, 22 s | 3/3, 137k, 7, 46 s | 3/3, 41k, 3, 19 s | 3/3, 71k, 5, 32 s |
| `add-json-key` | claude | 3/3, 74k, 3, 11 s | 3/3, 209k, 8, 27 s | 3/3, 156k, 7, 23 s | 3/3, 110k, 4, 16 s |
| `add-json-key` | codex | 3/3, 57k, 5, 26 s | 3/3, 100k, 6, 38 s | 3/3, 61k, 5, 26 s | 3/3, 95k, 6, 27 s |
| `recover-stale-read` | claude | 3/3, 115k, 5, 23 s | 3/3, 253k, 9, 46 s | 3/3, 131k, 6, 23 s | 2/3, 264k, 10, 32 s |
| `recover-stale-read` | codex | 3/3, 37k, 3, 19 s | 3/3, 118k, 7, 39 s | 2/2¹, 41k, 3, 18 s | 3/3, 127k, 9, 46 s |
| `recover-gate-refusal` | claude | 0/3, 57k, 2, 9 s | 3/3, 243k, 10, 31 s | 0/3, 80k, 3, 13 s | 0/3, 164k, 6, 19 s |
| `recover-gate-refusal` | codex | 0/3, 38k, 3, 21 s | 3/3, 124k, 7, 67 s | 0/3, 41k, 3, 17 s | 0/3, 74k, 5, 32 s |

¹ One attempt left out for a host failure. The score report's medians for these
cells differ, because it keeps the failed attempts' tool calls and seconds (25
calls and 94 s; 0 calls and 4 s) while leaving out their tokens. For Claude
skill-cli on `add-edition-markup` it reports 37 calls and 181 s against 45 and
248 s here; for Codex mcp on `recover-stale-read` the medians agree (3 calls,
18 s).

Over all seven tasks, with the uncached part of the input beside the total:

| Host | Arm | Passed | Input tokens | Uncached input | Tool calls | Seconds |
| --- | --- | --- | --- | --- | --- | --- |
| claude | baseline | 18/21 | 75k | 7k | 3 | 13 |
| claude | skill-cli | 20/20 | 260k | 19k | 10 | 33 |
| claude | mcp | 18/21 | 173k | 14k | 9 | 23 |
| claude | project-free | 17/21 | 207k | 13k | 8 | 32 |
| codex | baseline | 18/21 | 38k | 6k | 3 | 22 |
| codex | skill-cli | 21/21 | 137k | 20k | 8 | 55 |
| codex | mcp | 17/20 | 41k | 5k | 3 | 20 |
| codex | project-free | 18/21 | 93k | 13k | 6 | 37 |

No attempt tried an override: no `--gate report`, no `"gate": "report"`, no
`KAPI_ACTOR=person` and no `if_match: "*"` in any of the 168 transcripts.

#### Which route wrote the file

The score report does not record how the graded file was written, and the arm
does not tell it, so each attempt was classified from its transcript (and, for
Codex, from the session rollout, which holds calls the transcript leaves out).
`contract` means the edit reached the file through `kapi apply`, `kapi-files
apply` or `apply_edits`, including a file first copied natively and then
edited through the contract. `merge` means `kapi merge` alone. `native` means
the host's own edit tools (Claude's Edit and Write, Codex's `apply_patch`).

| Task | Host | baseline | skill-cli | mcp | project-free |
| --- | --- | --- | --- | --- | --- |
| `edit-link-html-md` | claude | native 3 | contract 2, native 1 | native 3 | contract 3 |
| `edit-link-html-md` | codex | native 3 | contract 3 | native 3 | contract 3 |
| `edit-plural-branch` | claude | native 3 | contract 3 | contract 3 | contract 2, native 1 |
| `edit-plural-branch` | codex | native 3 | contract 3 | native 3 | contract 3 |
| `add-edition-markup` | claude | native 3 | contract 1, merge 1 | native 3 | contract 1, native 2 |
| `add-edition-markup` | codex | native 3 | contract 2, native 1 | native 3 | contract 1, native 2 |
| `edit-po-context` | claude | native 3 | contract 3 | native 3 | contract 3 |
| `edit-po-context` | codex | native 3 | contract 2, native 1 | native 3 | contract 3 |
| `add-json-key` | claude | native 3 | contract 3 | native 3 | contract 2, native 1 |
| `add-json-key` | codex | native 3 | contract 3 | native 3 | contract 3 |
| `recover-stale-read` | claude | native 3 | contract 2, native 1 | native 3 | contract 3 |
| `recover-stale-read` | codex | native 3 | contract 3 | native 2 | contract 3 |
| `recover-gate-refusal` | claude | native 3 | contract 3 | no write 3 | contract 2, native 1 |
| `recover-gate-refusal` | codex | native 3 | contract 3 | native 3 | contract 3 |

Of the 124 valid attempts in the three kapi arms, 74 wrote through the contract
and 68 of those passed. Five of the six that did not are project-free attempts
on `recover-gate-refusal`, which wrote "from the portal" through `kapi-files
apply` as the prompt asked; the alias holds no terms, by design. The sixth,
`recover-stale-read-claude-project-free-02`, landed its edit through the
contract and then undid the other editor's change with a native Edit (see the
stale results below).

#### What the arms show

**Six of the seven families do not separate the arms.** Leaving out
`recover-gate-refusal`, baseline passed 36 of 36, skill-cli 35 of 35, mcp 35 of
35 and project-free 35 of 36. In `edit-link-html-md`, `edit-plural-branch`,
`edit-po-context` and `add-json-key`, all 24 final files of each family are the
same bytes, whichever arm and route wrote them (`shasum` over every attempt's
graded file); in `recover-stale-read`, 22 of the 23 valid attempts are, the
exception being `recover-stale-read-claude-project-free-02`. The traps the
fixtures set were all beaten by an edit anchored on a unique string. Claude's Edit needs an
`old_string` that occurs once, and the baseline agents chose one that did:
`edit-plural-branch-claude-baseline-01` replaced the whole line
`"inboxCount": "{count, plural, =0{No new messages} one{{count} new message} other{{count} new messages}}",`,
and every `edit-po-context` baseline edit included the `msgctxt "button"` line
that sits beside the `msgstr`. Codex's `apply_patch` carries context lines that
do the same. The fixtures therefore measure what the contract costs, and give no
evidence of what it prevents: no fixture holds an edit that a unique string
cannot target safely.

**Native editing was as good and cheaper.** Against baseline, the kapi arms
took 1.1 to 7 times the input tokens on a task, outside one outlier (Claude
skill-cli on `add-edition-markup`, 20 times, caused by the merge defect in the
friction list) and outside Codex's mcp arm, which never used kapi. Three things
make up the difference. The skill's project habits cost a median of three tool
calls per skill-cli session (`kapi context` before writing, `kapi check` and
`kapi context log` after). Composing a change set costs a read, often a schema
read, and a file or a pipe. Recovering from a refused change set costs a round
trip per refusal.

**When agents used the contract, it wrote what they meant.** Every change set
that applied wrote the edit the task asked for and left the bytes around it as
they were. `set_attribute` changed the `href` in both files and kept
`class="cta"` in the HTML (11 of 11 link attempts through the contract). Edits
to the ARB plural left the `=0` and `other` branches byte-identical in 14 of 14.
Every PO change set (11) was the reference operation exactly: `set_content` at
`{doc, block: "button/Book", edition: "nb"}` with `if_match`
`r:6cde96fe004bf39f`. All 11 `insert_block` change sets landed first time, and
the five `kapi apply` and `kapi-files apply` change sets in
`add-edition-markup` landed first time too. Agents copied `ref` and `rev` from
reads without constructing either: the only `stale` refusals in the study are
the nine `recover-stale-read` planned. Outside the link and plural families, no
change set in 49 contract-route attempts was refused `invalid`, `not_found` or
`ambiguous`.

**The MCP arm reached the contract in one family.** The server's instructions
(`host/mcp_instructions.go` at `4adeb612f`) name `context_read`,
`context_observe`, `context_correct`, `context_withdraw`, `check_file` and
`context_session_summary`, and no edit tool. Claude Code defers MCP tool
schemas, and in 17 of 21 sessions it loaded only tools the instructions name:
15 of those then edited natively, and 2 (`recover-gate-refusal-claude-mcp-02`
and `-03`) wrote nothing. An example of the loading is
`select:mcp__kapi__context_read,mcp__kapi__check_file,mcp__kapi__context_observe,mcp__kapi__context_session_summary`
in `edit-link-html-md-claude-mcp-01`. The exception is the ARB plural, where
all three sessions loaded `read_blocks` and `apply_edits` by name
(`select:mcp__kapi__context_read,mcp__kapi__read_blocks,mcp__kapi__apply_edits,mcp__kapi__check_file`
in `edit-plural-branch-claude-mcp-01`) and landed the reference route,
`replace_text` with `edits[].path` `[0, {"plural": "one"}]`.
`recover-gate-refusal-claude-mcp-01` also loaded `apply_edits`, then asked the
person instead of writing. Codex made no MCP call and never ran `kapi` in any
of its 20 valid mcp sessions. Whether the kapi tools reached the model is
unknown. A Codex session rollout records no tool declarations at all, for
kapi's tools or the host's own. The preparation's tool list comes from the
runner's own handshake with the server (`direct-server-discovery`), `codex mcp
list --json` shows only that the server is configured, and the preparation
records say `agent_host_exposure: "unverified"`. Codex's base instructions also say "Use `apply_patch` for local
file edits", a prior toward native editing in every Codex arm. In effect
Codex's mcp arm is a second baseline: its input tokens are 1.08 to 1.09 times
baseline's on every task.

**Only the gate family separates the arms, and through the context read.**
skill-cli passed 6 of 6; every other cell passed 0 of 3. No agent met a gate
refusal (below). The skill's first habit runs `kapi context` on the file, which
printed `overview page, not "portal": Harbor Help has no portal …`
(`recover-gate-refusal-claude-skill-cli-01`), so every skill-cli agent wrote
"from the overview page" before the commit check could refuse anything.

#### Stale recovery

The other editor's change from "five minutes" to "ten minutes" landed before
the agent wrote in 18 of the 23 valid sessions. It landed after the write in
all five valid Codex baseline and mcp sessions, because their `rg` printed only
line 8, where the agent's sentence is, and the trigger text sits on line 9; the
closing `git diff` fired the change (`agent_wrote_first: true` in
`recover-stale-read-codex-baseline-01`). Those five passes say nothing about
recovery.

| Signal | Sessions | Passed | Told the user about the other change |
| --- | --- | --- | --- |
| kapi `stale` refusal | 9 (claude skill-cli 2, claude project-free 3, codex skill-cli 1, codex project-free 3) | 8 | 5 |
| Claude Code's note that the file changed since it was read | 7 (claude baseline 3, mcp 3, skill-cli 1) | 7 | 6 |
| None: printed the changed file before taking a revision | 2 (codex skill-cli 2) | 2 | 1 |
| Codex patch that failed to apply | 0 | | |

The two sessions without a conflict signal, `recover-stale-read-codex-skill-cli-02`
and `-03`, printed the whole file (`sed -n '1,220p'` and `'1,160p'`) after the
change landed and before reading a revision, so their writes held. `-02` told
the user ("An existing change to the upgrade duration was already present and
was left untouched."); `-03` did not.

Every kapi `stale` refusal was recovered on the first resend, and each resend
differed from the refused operation only in `if_match`. Six of the nine agents
took the new revision from the refusal's `current`; three re-ran `inspect`
first. The refusal reads `refused: stale: edition en is at r:4247d44c3e210651,
not r:546353b6762dc81a` with the current text, and leaves the agent to work
out where the difference came from. `recover-stale-read-claude-project-free-02`
recovered correctly, then read `git diff` and decided: "a change I didn't make;
it must have been modified externally between my two reads. I'll revert that
part while keeping only the requested edit." It changed "ten minutes" back to
"five minutes" with a native Edit, the study's only stale failure. Four other
kapi-route sessions kept the change and did not mention it
(`recover-stale-read-claude-project-free-01` ended "Nothing else was changed.").
Claude Code's own note ("the file contains other changes not in your context")
led six of seven sessions to tell the user.

#### Gate refusal recovery

The commit check never refused an edit in the study: no `gate_failed`, no
`not_permitted` and no override attempt in 24 sessions. The family measured
whether agents learned the term before writing:

- skill-cli, 6 of 6: learned it from `kapi context` and wrote "overview page".
  Each reported the substitution, for example "using "overview page" rather
  than "portal" since the project's style guide notes Harbor Help has no
  portal" (`recover-gate-refusal-claude-skill-cli-01`).
- Claude mcp, 0 of 3: read the same rule with `context_read`, then asked the
  person and changed nothing, citing the term and the voice guideline "Do not
  invent service capabilities": "Can you confirm the CSV download feature
  actually exists (so I'm not inventing a capability)?"
  (`recover-gate-refusal-claude-mcp-01`). The grader scores this like writing
  "portal".
- Codex mcp, baseline and project-free, 0 of 9, and Claude baseline and
  project-free, 0 of 6: all 15 wrote "portal". None of the 24 sessions opened
  `STYLE.md`.

The refusal itself was exercised offline instead. On a scratch copy of
`recover-gate-refusal-claude-mcp-01/workspace`, `kapi apply --json` and
`apply_edits` both refused the "portal" sentence with `gate_failed` and the
finding `Forbidden term "portal" found: use: overview page`, and refused an
agent's `--gate report` as `not_permitted`. The shape has two faults the
adjustments list: a refused operation reports an `after` revision for content
never written, and the replacement appears only inside the message prose. The
findings also sit on the operation, where section 2.5 puts them on `docs[]`.

#### Frictions in the kapi arms, by the attempts they cost

Ranked by the number of valid attempts that spent at least one call on the
friction. The cause column says where each one sits: the contract
(kapi.change/v1, its results and its discovery), the skill text, the host, the
check, or the merge path. Only the contract rows (#2, #3, #6, #8, #9, #12, #13)
bear on the freeze. Frictions no attempt met, found by reading results or by
probes on scratch copies, follow the table.

| # | Friction | Cause | Attempts | What it cost | Evidence |
| --- | --- | --- | --- | --- | --- |
| 1 | The change set is read from a file or stdin, and every skill example passes a file; sandboxes refused the paths agents chose | host (sandboxes, Claude Code's `.claude` write protection); skill (examples) | 24 (18 Claude, 6 Codex) | 1 to 4 calls each | `edit-link-html-md-claude-skill-cli-01`: `open /tmp/kpe-2364253385/claude-502/bash-edit-diff/contact-edits.json: operation not permitted`, then `.claude/.cc-writes/contact-edits.json which is a sensitive file`; `edit-po-context-codex-skill-cli-01`: `patch rejected: writing outside of the project`; `edit-plural-branch-codex-project-free-01`: `zsh:1: can't create temp file for here document: operation not permitted`. Every pipe agents tried reached kapi (`printf '%s' '…' \| kapi apply -`). |
| 2 | `apply --schema` is the only reference for an operation's fields, and it is 41,508 bytes | contract (discovery) | 18 printed it | 1 to 2 calls each; one truncation led to a wrong guess | `edit-link-html-md-claude-project-free-02`: `Output too large (40.5KB)`, then a guessed `attrs` refused; `edit-plural-branch-claude-project-free-02`: `head -100` ended inside `set_content`, so it put `path` on `replace_text` and was refused |
| 3 | A plural branch's `path` sits on the operation for `set_content` and inside each `edits[]` entry for `replace_text` | contract | 9 | 11 `invalid` refusals (6 with `path` in `at`, 5 with it on a `replace_text` operation); none of the messages led straight to the fix | `edit-plural-branch-claude-skill-cli-02`: `invalid at /ops/0/at/path: unknown field "path"; it takes doc, block, edition`; `edit-plural-branch-codex-skill-cli-01`: `invalid at /ops/0/path: unknown field "path"; replace_text takes at, edits, if_match, op` |
| 4 | The skill's verify step does not run on a PO edit or a new edition | check | 10 | 1 call each | `edit-po-context-claude-skill-cli-01`: `blocks tu1#msgid and tu1 are adjacent with no text between them to divide their span`, verdict `did_not_run`, exit 4; `add-edition-markup-codex-skill-cli-01`: `docs/nb/welcome.md` `out_of_scope`, "not content kapi.yaml declares" |
| 5 | `kapi help translation`, named in `SKILL.md`, does not exist (the topic is `translate`) | skill | 5 | 1 call each | all five valid `add-edition-markup` skill-cli sessions (and the excluded `-claude-skill-cli-02`): `no help topic or command "translation"` |
| 6 | `find` refuses the `<x id="p1/"/>` form that reads, `text` and `structures.branches` all show | contract | 5 | 7 `not_found` refusals; 3 attempts gave up on `replace_text` | `edit-plural-branch-claude-skill-cli-02`: `refused: not_found: "<x id=\"p1/\"/> new message" is not in the text`; `edit-plural-branch-codex-skill-cli-01` then tried `"{count} new message"`, also `not_found` |
| 7 | The skill's translate route goes through XLIFF, which holds Markdown inline markup as text, so `kapi merge` refuses the targets | merge; skill (route) | 4 | the largest single cost: Claude skill-cli median 1.55M input tokens against 75k for baseline | every merge: `block tu2 not merged: gate_failed: the edit introduces 4 failing finding(s) in docs/en/welcome.md#welcome-to-harbor-help/p@nb: Inline code pc-close:1 is missing from the nb target (dropped 1×)`, a copy of the source refused alike (`add-edition-markup-claude-skill-cli-01`); the excluded `-claude-skill-cli-02` met it too |
| 8 | The stale refusal does not say the difference is another editor's change to keep | contract (message) | 5 | 1 failure, 4 silent rebases | `recover-stale-read-claude-project-free-02` (quoted above) |
| 9 | `set_attribute` takes `name` and `value`, while reads and `mark` give attributes as an `attrs` map | contract (discovery) | 4 | 1 `invalid` refusal each | `edit-link-html-md-claude-skill-cli-01` and `-03`: `invalid at /ops/1/attr: unknown field "attr"; set_attribute takes at, code, if_match, name, op, value`; `-codex-skill-cli-02` sent `attribute`; `-claude-project-free-02` sent `attrs` after its schema print was cut off. Three of the four kept the name and value shape and missed only the key. All 7 attempts that read the schema first were right first time. |
| 10 | The skill says "one operation per block you changed", but a link edit needs two on one block | skill | 3 | an extra change set each, and a re-read in two | `edit-link-html-md-codex-skill-cli-01` sent `set_attribute`, re-ran `inspect`, then sent `replace_text` with `if_match` `r:42ce604d55e366f3`; `-codex-project-free-03` sent its second change set against the first result's `after` without a re-read. None of the three said why it split the edit; the skill sentence is a likely cause (inference). |
| 11 | The skill gives two recoveries from `stale`: use `current`, or re-read | skill | 3 | 1 call each | `recover-stale-read-claude-project-free-03`: "Re-read block to get current revision" |
| 12 | Under `--json`, a change set that does not decode prints `{"error": …, "code": "usage"}` instead of a refused result with `invalid` and a pointer | contract (result) | 3 met it | none for these agents, who read the prose; a program cannot branch on it | `edit-link-html-md-codex-skill-cli-02`: `{"error": "apply: invalid at /ops/1/attribute: …", "code": "usage"}`, exit 2 |
| 13 | Over MCP, the whole change set was sent as a string in `ops` | contract (MCP input) | 1 | 1 call | `edit-plural-branch-claude-mcp-01`: `{"code":"invalid","pointer":"/ops","message":"must be an array of operations"}` |

Met by no attempt:

- A PO catalog read without `--target-lang` lists no translation, and a
  `set_content` at that ref without `at.edition` rewrites the `msgid` and
  reports `previewed` (reproduced on a copy of an `edit-po-context` workspace).
  Every agent followed the skill's sentence on `--target-lang`.
- A read never names a monolingual file's own edition, which `insert_block`'s
  `editions` map needs. All 11 `insert_block` change sets used `en`, as the
  shipped catalog example does for a file of the same name (see Limits), which
  was right here; no read would have told them.
- The schema about to freeze carries the retired word `unit`
  (`evidence[].unit`, "the block key inside the file") and offers `if_match`
  `"*"` "for whatever is there", which an agent is refused.
- A refused operation echoes `after` and `resolved` for content never written
  (`"resolved": [{"start": {"run": 0}, "end": {"run": 0}}]` in
  `edit-plural-branch-codex-skill-cli-01`), and a refused change set lists
  `"docs": []` where section 2.5 lists each document with `written: false`.
- `resolved` drops a zero `offset`, and writes the end of a match as the start
  of the run after it (`"end": {"run": 2}` for a two-run branch in
  `edit-plural-branch-claude-mcp-01`). That is `RangeAnchor`'s attribution,
  which section 3.3 names, while section 2.5's example writes the end inside
  the run with `offset` printed.
- An `insert_block` anchored in another object is refused `unsupported`, the
  code for a format that lacks the operation.
- `not_found` carries no candidates, where section 2.6 promises up to three.

#### Limits

- **Size.** Three repetitions per cell, two hosts, one model and effort per
  host. A difference of one attempt in a cell is within what reruns would
  change. The study says nothing about other models or about agents with a
  longer history in the project.
- **Ceiling.** In six families every arm passed all its attempts or all but
  one, so their pass rates compare nothing; they give cost and friction only.
  A discriminating variant needs an
  edit a unique string cannot reach: equal text in two plural branches on one
  line, a nested select, a wrapped `msgstr`, or a catalog where the new key
  must also land in every translation file.
- **The MCP arm.** Codex never called an MCP tool in 20 valid sessions, and the
  preflight proves only that the server is configured (`codex mcp list
  --json`), not that the model sees its tools. Claude reached `apply_edits` in
  3 of 21. The arm measures the context tools and the server instructions far
  more than the edit contract.
- **The gate family.** The refusal never happened in a live session, so
  recovery from `gate_failed` is unmeasured. A rerun should add the term after
  the agent's first context read, as `recover-stale-read` changes the file
  after a read. "Asked, changed nothing" deserves an outcome of its own.
- **The stale family.** In the study the other editor's change fired only when
  a tool result showed the agent the text "it takes about five minutes", on
  the line after the agent's sentence, so the five Codex baseline and mcp
  sessions, whose `rg` printed one line, wrote before the change landed. Those
  five passes are uninformative about recovery. The trigger now sits on the
  agent's own line (Design).
- **The two host failures.** Both are counted as attempts that did not pass in
  the study's score report and are left out here. Neither was rerun.
- **Grader heuristics.** In the study `md_translated` failed only when a whole
  source sentence of four or more words survived, so
  `add-edition-markup-claude-mcp-01` passed with `Les mer i [Preparing for a
  video appointment](…)`. `json_ordered` joined keys with dots and could not
  tell a nested key from a flat dotted one. The scope criteria covered the
  task's directories only. 25 attempts (19 Codex, 6 Claude) wrote a change-set file
  at the workspace root and deleted it before finishing; one left behind would
  have passed. Writes to the context store were not recorded, including
  observations with no ground in the files
  (`add-json-key-codex-skill-cli-02` recorded `term "Harbor Help", not
  "HarborHelp"` for `locales/en.json`, which holds no "Harbor"). The refusal
  counter missed `kapi merge`'s `not merged:` lines, so the merge refusals
  four valid `add-edition-markup` sessions met (five with the excluded
  `-claude-skill-cli-02`) are absent from the score report. Each of these is a criterion, a record or a
  count of the runner now (Design).
- **Accounting.** Input tokens include cache reads (for example 329,688 of
  352,872 in `edit-link-html-md-claude-skill-cli-01`). Codex's
  `transcript.jsonl` leaves out rejected patches and `apply_patch` bodies, so
  the study's tool-call counts and its count of paths outside the cell differ
  from the session rollout, which the runner now reads. In seven of eight mismatches the score is one call low
  (`add-json-key-codex-project-free-03` has 7 calls in its rollout and 6 in the
  score, the missing one a refused write to `/private/tmp`); in
  `recover-gate-refusal-codex-skill-cli-01` the score counts 14 against 9 in
  the rollout.
- **Fixture overlap.** The shipped `edit.md` catalog example names
  `locales/en.json`, a dotted anchor and the `en` edition, and all 11
  `insert_block` change sets are that example with the names replaced.
  `add-json-key` measured copying more than discovery; it now adds a key to a
  German catalog at another path.
- **Sandboxes.** The temporary-file refusals in friction 1 come from the hosts'
  sandboxes as configured for the study. They fall only on the kapi arms,
  because native edits need no intermediate file.

#### Decision

The contract's shapes held where agents used them. No applied change set wrote
something other than what its sender meant, and every contract refusal was
recovered, the `stale` ones on the first resend. (The `kapi merge` refusals in
`add-edition-markup` sit outside the contract and were not all recovered:
`add-edition-markup-claude-skill-cli-01` rewrote its XLIFF targets and ended at
its turn limit.) The contract rows of the friction table are a field placement
agents could not infer (#3), a `find` that does not read as `text` does (#6), a
key agents guessed without the schema in view (#9), a schema reference too
large to read (#2), and refusal shapes and wording (#8, #12, #13). The rest sit
in the skill text, the hosts' sandboxes, `kapi check` and `kapi merge`.

v1 is frozen after the changes in the freeze specification. The schema changes
are three: `find` read as placeholder text, with a token the match held
following rule 2 of section 2.4; an optional `path` on the `replace_text`
operation, which removes 5 of the 11 path refusals and leaves the 6 placements
in `at` to a better message and example; and `evidence[].unit` renamed
`evidence[].block`. The results change to match sections 2.5 and 2.6: refused
operations without a revision, `invalid` on every transport, findings on
`docs[]` whenever the check ran, and `offset` printed on every position in
`resolved`.
`set_attribute` keeps `name` and `value`: three of its four wrong guesses kept
that shape and missed only the key, and all seven agents that read the schema
were right, so the fix is discovery (a per-operation schema, an example, a
message), not a new shape.

On D14, the project-free arm passed no task that kapi with a project failed,
and its gate-task losses follow from having no gate. Its lower cost came from
the skill text it ran rather than from the binary: on Codex from the governance
steps it leaves out (0.71 of skill-cli's median input on the five tasks other
than `add-edition-markup` and `recover-gate-refusal`), and on both hosts from
skipping the shipped skill's translation detour (0.14 and 0.22 on
`add-edition-markup`). On Claude, outside those two tasks, the two arms cost
the same (0.98). The recommendation is no separate artifact and no second name
in 1.3.0 (see the neo/kapi section).

### The neo/kapi question (D14)

Section 15.2 of the [edit model](edit-model.md) names the condition for a
split: the project-free arm beats `kapi` materially on success or cost for
agents, or an audience needs a native binary without cgo or ICU that the WASM
build does not serve. Section 15.3 adds that a project-free name, if it proves
useful, is a multi-call alias of the same binary. Three measurements answer
both. The second and third are recorded above (what the edit engine costs an
embedder; what a second artifact costs). The first is the `project-free` arm
of the paired study.

#### Agents: the project-free arm against kapi's arms

`project-free` ran `kapi-files`, the same binary under a multi-call name that
exposes `inspect`, `apply`, `formats` and the toolbox with discovery off, and a
skill that is the shipped one with the project, MCP, check and translation
passages removed. `skill-cli` and `mcp` are kapi with a project. Every
project-free cell of the study held the task's `kapi.yaml` and `STYLE.md` as
every other cell did, and the alias never read them, so the arm measured the
alias beside a project rather than agents working where no project exists. A
rerun's project-free cells hold neither (Design). Two attempts are
excluded (host failures, one in `skill-cli` and one in `mcp`); every other cell
has three.

| Host | Arm | Passed | Passed without the gate task | Wrote through the contract | Input tokens | Tool calls |
| --- | --- | --- | --- | --- | --- | --- |
| claude | skill-cli | 20/20 | 17/17 | 17 | 260k | 10 |
| claude | mcp | 18/21 | 18/18 | 3 | 173k | 9 |
| claude | project-free | 17/21 | 17/18 | 16 | 207k | 8 |
| codex | skill-cli | 21/21 | 18/18 | 19 | 137k | 8 |
| codex | mcp | 17/20 | 17/17 | 0 | 41k | 3 |
| codex | project-free | 18/21 | 18/18 | 19 | 93k | 6 |

`mcp` hardly used the contract (Claude in one family, Codex never), so the
comparison that bears on D14 is `project-free` against `skill-cli`: the same
edit commands, with and without a project. Per task, project-free's median
input tokens as a share of skill-cli's:

| Task | Claude | Codex |
| --- | --- | --- |
| `edit-link-html-md` | 1.44 | 0.78 |
| `edit-plural-branch` | 0.90 | 0.73 |
| `add-edition-markup` | 0.14 | 0.22 |
| `edit-po-context` | 0.78 | 0.52 |
| `add-json-key` | 0.53 | 0.95 |
| `recover-stale-read` | 1.05 | 1.07 |
| `recover-gate-refusal` | 0.67 | 0.59 |

**Success.** project-free passed no task that kapi with a project failed.
skill-cli passed all 41 of its attempts. project-free's losses are the six
`recover-gate-refusal` attempts and `recover-stale-read-claude-project-free-02`,
which reverted another editor's change after a correct `stale` recovery. The
gate losses follow from the arm's construction: the alias has no gate and holds
no terms (its skill says "It holds no project, so no voice, terms or check
applies"), the prompt asks for "from the portal", and the study's design gives
the baseline and project-free arms no gate. They say nothing about whether a
project-free name is useful. Outside the gate task, project-free passed 35 of
36 and skill-cli 35 of 35.

**Cost, per host.**

- **Codex.** project-free took 0.68 of skill-cli's median input over the seven
  tasks (93k against 137k) and 0.71 over the five tasks other than
  `add-edition-markup` and `recover-gate-refusal` (98k against 137k, 15
  attempts each). On those five tasks every skill-cli session ran `kapi
  context` and 13 of 15 ran `kapi check`, and no project-free session ran
  either. The saving on Codex is the governance steps the alias's skill leaves
  out. That is an inference from the call pattern: the two arms run the same
  edit code and differ in their skill text and the commands it names.
- **Claude.** project-free took 0.80 of skill-cli's median input over the seven
  tasks (207k against 260k) but 0.98 over the same five (262k against 267k).
  Claude's overall saving comes from two tasks. On `add-edition-markup` (0.14)
  the shipped skill sent agents through extract, XLIFF and merge into the merge
  defect, while the alias's skill has no translation passage. On
  `recover-gate-refusal` (0.67) the context read and check that skill-cli
  spent are what passed the task. On the other five, Claude's skill-cli
  sessions also ran the governance steps (15 of 15 `kapi context`, 14 of 15
  `kapi check`), and they did not show as a cost difference; project-free
  sessions read the schema more often (5 of 15 against 2 of 15).
- **Both hosts.** Where both arms wrote through the contract, they sent the
  same change sets: in `edit-po-context` all 11 contract change sets, from
  both arms, were the same operation, and in four families every final file in
  every arm is the same bytes.
- **One cell.** Claude's project-free link edits took 1.44 times skill-cli's
  tokens, but that skill-cli median includes a native attempt
  (`edit-link-html-md-claude-skill-cli-02`, 157k). On the plural project-free
  took 0.90 (Claude) and 0.73 (Codex).

So the cost difference between the arms comes from the skill text each ran: the
governance habits (Codex) and the translation detour (both hosts). The binary
and the edit path are the same.

**Use.** Agents reached the contract through the alias as readily as through
kapi: project-free wrote through the contract in 35 of 42 attempts, skill-cli in
36 of 41. Nothing in the study suggests the name changed whether agents used
the contract.

**What the arm also showed.** At the run the alias's surface drifted from what
it is, which the freeze fixed: `kapi-files apply --help` read "Apply a change set: content edits, review
decisions, terms and recipe fields" (`add-json-key-codex-project-free-02`).
`kapi-files --help` says "no check at commit", and a dropped `%d` was
refused `gate_failed` (probe). Every JSON catalog's own edition was taken to
be `en` (probe); a document outside a project now holds the language its file
or directory names. In `add-edition-markup` no project-free attempt sent an edition
operation: four wrote the Norwegian file natively and two copied the English
file and edited the copy through the contract. The agents treated a file that
did not exist yet as outside the edit loop ("docs/nb doesn't exist yet, so this
is creating a new standalone file rather than editing one",
`add-edition-markup-claude-project-free-01`), and the alias's skill has no
translation passage to say otherwise. Had one sent it, a probe showed the
operation refused `unsupported` ("has nowhere to live") with no next step;
`apply --out FILE` now writes such an edition, and the refusal names it.

**Not measured.** kapi's own CLI outside a project was not an arm of the
study, and no cell lacked a project; `kapi-no-project` and the project-free
cells without a recipe are the rerun's answer. Whether the shipped skill's project habits would run there,
and what they would cost, is unknown. The MCP comparison rests on Claude alone.

#### Embedders

`examples/go-apply` builds the edit engine with the file home and every
built-in format and nothing of the host: 269 packages, no cgo, ICU not linked,
18.6 MB (12.9 MB stripped) against 91.3 MB for `bin/kapi`, writing the same
bytes as `kapi apply` on the link task. A native, project-free embedding needs
neither cgo nor ICU, so the audience for which section 15.3 would build a
separate artifact is served by the Go module, and the WASM build serves the
web.

#### Distribution

A multi-call alias costs a symlink entry in nfpm, the Homebrew formula,
setup-kapi and the Docker image, and on Windows either a `kapi` subcommand or a
second portable alias, since Windows has no symlink to install. A second
artifact costs five more builds per release, another signed and notarized
binary, two more executables to sign by hand on Windows, cosign signatures, a
second package or formula on every channel, a second winget identifier with a
submission each release, an updater that replaces two binaries, and a product
name. `neo` is taken on npm, PyPI and Debian; `kapi-files` is free on every
registry checked.

#### Recommendation

No separate artifact, and no second name in 1.3.0.

- **No separate artifact.** Section 15.2's first condition is not met. On
  success the project-free arm matched kapi and beat it nowhere. On cost it was
  cheaper on Codex, and on Claude only in two tasks, and in both cases the
  saving came from the skill text it ran, which a second artifact would not
  change and kapi's own skill can. The second condition is answered by the
  embedder measurement: the Go module is the project-free engine for a native
  embedder, without cgo or ICU, and the WASM build is the engine for the web.
  A second artifact would add the release, signing and packaging work listed
  above for no audience the measurements found.
- **No `kapi-files` alias.** Section 15.3 makes the alias depend on a
  project-free name proving useful, and the study shows no use the name
  provided. Agents reached the contract through it as often as through kapi,
  and its lower cost came from the skill text it carried. The arm ran beside a
  project, so it says nothing about agents with no project, the audience the
  name would be for. Against it: at the run its help named operations that need a
  project and said there is no check at commit while a dropped `%d` was
  refused `gate_failed`, and it took every JSON catalog's language as `en`;
  shipping it costs a link on four channels and a separate answer on Windows. The multi-call case in `cli/toolbox_files.go` stays as it is,
  installed by no build or channel and documented nowhere, as the surface a
  rerun measures. An agent outside a project can run `kapi inspect` and `kapi
  apply` on any file, since those are the code paths the alias runs; that is
  the design's inference, which the study did not measure.
- **What the cost evidence asks of kapi instead.** The skill-cli arm paid for
  steps that bought nothing on these fixtures outside the gate task. Three
  changes in the freeze specification address that without a second name: a
  commit result that says the check ran and what it found, together with skill
  text and an `apply_edits` description that stop asking for a second check of
  what the commit already checked; a diff-scoped check that names, for a PO
  catalog and a recipe target file it cannot place, the command that checks
  the file whole, in place of the fallback call ten attempts worked out; and
  `translate.md` putting the edition route ahead of extract and merge, which
  removes the detour behind the largest gap on both hosts.
- **What would reopen it.** An audience asking for a native binary without the
  host; or a rerun whose project-free cells hold no project, with kapi under
  discovery off as a fifth arm and fixtures that separate the arms, in which
  agents without a project succeed where kapi's fail, or spend materially less
  than kapi does for the same work.

## Where the gaps are

See the generated `/evals` page for current coverage gaps.

Building the last four evals turned up five bugs, four of them since fixed, and they are
worth keeping here because in each case the eval's first result was about kapi
rather than about the thing the eval set out to measure:

- **`kapi exec voice-check` and `voice-infer` write nothing and exit 0**, under
  every provider, profile and input tried, while sibling tools on the same file
  print results ([#2225](https://github.com/neokapi/neokapi/issues/2225)). Both
  declare `model.AnnoVoice` output, which appears nowhere under `cli/` or
  `host/`. So two of the three authoring evals had no output to score, and
  `voice-infer-quality` is `blocked` rather than absent: its comparison is
  written and runs the moment there is a draft.
- **`kapi apply` rejects every edit to a block with paired inline codes**, even
  one whose codes are byte-identical to the source's
  ([#2227](https://github.com/neokapi/neokapi/issues/2227)). The reader numbers
  a close as its open's id plus one and the guard requires them to match, so
  bold, italic and hyperlink spans in a .docx cannot be edited through the
  `inspect | edit | apply` path the help advertises. 14 of 26 coded blocks
  across the repo's own fixtures; `simple.docx` is entirely uneditable. It was
  found because a scenario failed with kapi and passed without it, and the
  transcript showed the agent producing a correct ten-block change-set and
  then spending a 40-turn budget on the one rejection.
- **A forbidden term matches no inflection**, so a profile forbidding `utilize`
  passes "the platform utilizes your data"
  ([#2226](https://github.com/neokapi/neokapi/issues/2226)). That was the single
  term-mechanism miss in the authoring corpus, and the fix took two attempts;
  see [below](#fixing-the-inflection-miss-took-two-attempts).
- **An empty voice profile scores 100/100** and reports the text as on brand
  ([#2224](https://github.com/neokapi/neokapi/issues/2224)).
- **The engine benchmark wrote 844 files where nobody looked for them.** A
  repo-relative `-output` resolved against each engine's scratch `cmd.Dir`, so
  every file scored "no output written" and the run reported 0/844 with real
  timings ([#2221](https://github.com/neokapi/neokapi/issues/2221) tracks the
  republished dataset).

Four of the five are the same shape: a surface reporting a refusal, or an empty
result, so quietly that the caller reads it as success. An eval is the only
thing that notices, because each failure is invisible from the outside.

The fifth had a different cause. Nothing about
`apply` looked wrong from the inside: it has a faithfulness guard, the guard
fires, and it reports what it refused. What surfaced it was the control arm:
the scenario failed with kapi and passed without it, and that comparison is the
only signal that pointed at a tool doing its job correctly and uselessly.

## Fixing the inflection miss took two attempts

The obvious fix for #2226 is to derive the forms from the term: add `s`, `d`,
`ing`, handle a trailing `e`. That is what shipped first, and the corpus went
from 12 of 13 terms to 13 of 13.

Then the same check ran against nb, which this repo actually publishes in. It
caught the bare stem and nothing else, missing utnytter, utnyttet, løsningen,
løsninger and løsningene, while generating løsninges and utnytted, which are not
words. It had also needed a floor on term length, because `Go` matched inside
"going" and a test in `core/profile` says it must not. That floor took the forms
away from `use`.

The English suffix rules were not an approximation of morphology. They were
morphology for one language, applied silently to every locale, in a gate.

The tools that do this at scale split along one line, and it is not
mechanical against AI:

| | how a term is matched | what it costs |
| --- | --- | --- |
| Vale | declared strings, exact | nothing, and no morphology at all |
| Lucene, Snowball | stemmed at index and query time | a stemmer per language, and conflations a search can absorb |
| LanguageTool, Acrolinx | full morphology | a linguistic pack per language, which is the product |
| Grammarly, MS Editor | a model reads the sentence | a model call per document, and a gate that varies run to run |

Stemming is the tempting middle and it belongs to search: Snowball folds
"universe" and "university" together, which costs a search engine a place in a
ranking and costs a check an accusation against text that broke no rule.

So the axis is *when* the language knowledge is applied. A model has the
morphology for every language, and asking it once, at authoring time, puts the
answer on the term where a person reads it in a diff. `kapi terms expand`
does that, and the check that consumes the result stays exact, free,
deterministic and language-neutral. Norwegian goes from 0 of 5 to 5 of 5 and the
English corpus holds at 13 of 13.

The reason this is in the eval notes rather than in an architecture note: the
first fix passed the corpus that found the bug. It had to be run against a
second language before it was visibly wrong, and this repo had one to hand only
because it publishes in it.

## The authoring evals

```bash
make authoring-eval          # all three: checks, infer, the voice guide
make authoring-eval-checks   # the checks alone, free and offline
```

The corpus is synthesized and says so in the data rather than only in the prose
around it. Two of the three questions need ground truth no repository carries,
a profile a person wrote from a known corpus and prose whose every violation is
marked, and labelling real material to that standard is the eval rather than
preparation for it.

Recall is measured over the documents written against the profile, where every
violation is marked, and false positives over the documents written to it, where
the right answer is silence. Neither half substitutes for the other. An
off-profile document contains violations beyond the marked ones, so counting
unmarked findings there measures how complete the marking is rather than how
good the check is; the first version pooled them into one precision figure and
reported 61%, none of which was about kapi.

Split recall by which of the three mechanisms a profile states a rule through,
because the answer differs completely between them: terms and patterns are
matched offline, and `active_voice`, `person_pov` and the rest of the enum
fields are not evaluated by anything offline at all. They reach the guide and
the LLM check. A profile saying `active_voice: true` scores dense passive prose
100/100 offline, and a reader who does not know which mechanism a rule uses
cannot tell a clean document from an unchecked one.

## Reconciling a read against the block history

```bash
KAPI_MEASURE_RECONCILE=1 go test -tags fts5 ./host -run TestMeasureReconcileOnRead -v
```

The edit model (`docs/internals/edit-model.md`, section 3.5) proposed running
`reconcile.Blocks` on every local read, against priors from the block history
and cached per document revision, so a block's key survives a sibling insertion
locally the way it does on a push. The condition was a cost under 10% of read
time.

The harness reads the repository's documentation (`web/docs`: 408 files and
13,703 blocks, with the mdx reader the dogfood recipe uses), gives every block a
recorded change in `block_history`, and times each part over five passes.
Measured on 2 October 2026 on an M-series laptop; three runs agreed within a
percentage point.

| Per pass over the corpus | Time | Share of the read |
| --- | ---: | ---: |
| read with the mdx reader | 190 ms | |
| priors from `block_history` (`history.Priors`) | 48 ms | 25% |
| `reconcile.Blocks` | 25 ms | 13% |
| the history head, which a cache hit still reads | 8 ms | 4% |

A read at a new revision pays for the priors and the reconciliation: 38% of the
read. Reconciliation alone is 13%, over the bar even with the priors free. A
cache hit costs 4%, but every read after an edit is at a new revision, so the
agent's read, edit and read loop gains least from the cache.

Reads therefore keep the keys the format reports. Every `content.edit`
transition and every `block_history` row carries the block's key, content hash
and context hash, the signals `core/reconcile` matches on, so a later pass can
re-attach history after a reorder and pay the cost once.

## Comparing against other tools

```bash
make conversion-eval   # every converter installed, over the parity corpus
```

`scripts/conversioneval` compares document converters, and the hard part is
ground truth. Scoring against pandoc's output would measure agreement with
pandoc. OOXML avoids that: the spec designates which elements carry text, so
each document states its own contents and no converter stands in for the answer.

Three things that comparison has to get right, and each was wrong first:

- **Ask each tool only for what it claims.** `--convert-to txt` has no Impress
  target, so LibreOffice was scored eight failures for a capability it does not
  offer, and it looked broken across two thirds of the corpus.
- **Weight by content, not by file.** The corpus holds two-word fixtures;
  averaging per-file recall gives one of those the same vote as a full report,
  and every converter scored 0% on the same two-word document.
- **Then check what the weight lands on.** Weighting by content makes one large
  document the score: `large.xlsx` carries 99% of the spreadsheet ground truth,
  so the .xlsx row is one workbook wearing a corpus's clothes. The dataset now
  records the top file's share and the page says so above 50%.
- **Read the cells, not the string table.** `xl/sharedStrings.xml` holds each
  distinct string once and the sheets refer to it by index, so a string in five
  hundred cells appeared once in the truth and five hundred times in the output.
  Recall is min(output, truth)/truth, so undercounting the truth made every
  score easier, and both converters returned exactly 100.0% on .xlsx. A metric
  that cannot fail looks precisely like a good result.
- **Compare within a format.** The tools accept different ones, so a single
  column ranks them by what they declined.

The corpus is the okapi-testdata tree the parity harness already downloads. It
matters that it was collected by another project for another purpose.

## What the commit check costs

```bash
make bench-commit-check
```

The change service runs the host's commit check on every edition a change set
changes, before it writes ([S-03](../../web/docs/contribute/architecture/surfaces/s-03-agent-surfaces.md#governance-at-commit)).
`BenchmarkCommitCheck` (`host/commitcheck_cost_test.go`) measures it on this
repository's own documentation: every Markdown and MDX file under `web/docs`,
408 documents and 13,804 translatable blocks. Four change sets append a word
to each block they name:

- **paragraph**: one block;
- **document**: every block of the largest document, `reference/project-file.mdx`
  (510 blocks);
- **translations**: the Norwegian translation of each of those 510 blocks;
- **docs sweep**: every block of every document.

A warm run keeps one App, as Kapi Desktop and the MCP server do. A cold run
starts each change set on a new App, as one `kapi apply` does.

`make bench-commit-check` measures two governances. The first copies the
documentation into a scratch project with the Tidewatch sample's context
(`samples/tidewatch-docs/context`: one voice with two constraints, five
concepts). The second checks the documentation in place under this
repository's `kapi.yaml`, with the context `make import-dogfood-context` pulls
from `refs/kapi/context` into the isolated data root: the project's terms, its
context log, and the documentation voice at the points the docs collections
declare. The benchmark reads that root from `KAPI_COMMIT_COST_DATA_DIR`,
because the host package's tests clear `KAPI_DATA_DIR`.

Per change set, on an Apple M1 Max, the faster of two runs of five. Other
agents' builds shared the machine (load average 9 to 32 during the runs), so
the figures are upper bounds, and a cold run that beats its warm run shows the
noise:

| Change set | Editions | Sample, warm | Sample, cold | Dogfood, warm | Dogfood, cold |
| --- | ---: | ---: | ---: | ---: | ---: |
| paragraph | 1 | 2.1 ms | 4.2 ms | 16.5 ms | 19.0 ms |
| document | 510 | 39 ms | 39 ms | 169 ms | 127 ms |
| translations | 510 | 31 ms | 36 ms | 209 ms | 340 ms |
| docs sweep | 13,804 | 1.21 s | 1.48 s | 5.98 s | 4.46 s |

Two costs add up:

- **Resolution, once per change set.** The check loads the recipe once,
  opens the project's stores, resolves the voice and the terms at each point,
  reads the context log for suggested and widened rules, and resolves the
  governance fingerprint. On a one-paragraph edit that is most of the cost. It
  grows with the project's context: the dogfood project, with a larger terms
  store and a context log of thousands of operations, pays several times what
  the sample does. A new App adds 2 ms to 3 ms for opening the stores.
- **The analyzers, per edition.** About 0.08 ms under the sample's governance
  and 0.25 ms to 0.4 ms under the project's own, mostly the terms analyzer
  locating every concept in the text and the voice's prohibited patterns, so
  the cost follows the size of the governance. Each analyzer runs over each
  block in a goroutine of its own (`RunCheckTool`), and the hand-off is a
  visible share of a large change set. A sweep allocates about 41 KB per
  edition under the sample and 139 KB under the project's own.

Resolution is shared wherever it can be. `governFile` resolves a file's point
once for both of its halves and both of its points, because each resolution
reads the project's ignore rules from disk; documents that sit under one
governance resolve its fingerprint once; and a translation pass resolves its
term rules once for both sides of the change.

A one-paragraph edit costs milliseconds, small beside the model call that
usually produced it. A sweep over every block of the docs takes seconds, which
is the shape of a flow rather than an agent's edit, and a flow checks once per
document.

## What a steady kapi up costs

A `kapi up` with nothing to draft should cost about what reading the project
does. Before the changes below, a steady pass over 120 Markdown pages took two
minutes once the content memory held entries, and the plan it printed named
thousands of units to draft while the run drafted none. The measurements were
taken with `bin/kapi` built from the commit before the changes (`da3f15c64`)
and after them, under the isolation contract, on three scratch projects outside
the repository:

- **synthetic**: 120 Markdown pages of 25 paragraphs (3,120 blocks), one target
  language, a flow of `pseudo-translate` alone;
- **docs**: a copy of `web/docs` (Markdown read as MDX, and MDX) and every
  `harness/demos/*/demo.yaml`, 444 files and 15,628 blocks, into `nb` with the
  same flow, under a recipe written for it;
- **KapiMart**: `samples/mart/src`, two source files into three languages.

Each project runs a first pass, a steady pass, a pass after a source edit on
every page, and two steady passes after that, then `kapi up --plan`,
`kapi status` and `kapi extract --no-memory --force`. `KAPI_CPUPROFILE=<file>`
writes a CPU profile of any kapi command. Other agents' builds shared the
machine (load average 18 to 60), so CPU seconds (user and system) are the
figures to compare; wall seconds follow them loosely.

| Run | Synthetic, before | Synthetic, after | Docs, before | Docs, after |
| --- | ---: | ---: | ---: | ---: |
| steady pass after the edit | 161 s CPU, 145 s wall | 1.8 s CPU, 1.8 s wall | 238 s CPU, 156 s wall | 24.6 s CPU, 13.3 s wall |
| the same, `--no-checks` (no pass) | | | 124 s CPU, 114 s wall | 3.3 s CPU, 2.2 s wall |
| `kapi up --plan` | 159 s CPU | 0.6 s CPU | 123 s CPU | 9.1 s CPU (1.1 s with `--no-checks`) |
| `kapi status`, for scale | 1.3 s CPU | 1.2 s CPU | 4.4 s CPU | 4.7 s CPU |
| `kapi extract` | 0.9 s CPU | 0.6 s CPU | 5.0 s CPU | 2.6 s CPU |

The docs project's language fails 47 bound checks, which the pseudo flow cannot
fix, so every run there passes over it and drafts the 1,535 units the content
memory does not answer; with `--no-checks` no pass runs and the run costs about
what `kapi status` does. On KapiMart every run takes under 0.4 s before and
after; a steady pass went from 0.26 s to 0.08 s of CPU.

What the time went to, and what changed:

- **Exact lookups scored the fuzzy pool.** The plan asks the content memory for
  an exact answer (`MinScore` 1.0) for every unit it prices. A lookup whose
  exact tiers found nothing went on to fetch and Levenshtein-score the fuzzy
  candidate pool, though no fuzzy score reaches 1.0 for a key the exact tiers
  did not already compare. With a few thousand entries that was 10 ms a unit.
  `memory.TieredLookup` now ends an exact-only lookup at the exact tiers.
- **Each exact tier loaded the entry again.** An entry that answers exactly
  answers under its generalized, structural and plain keys alike, and each tier
  loaded it with four queries. `SQLiteStore` loads it once per lookup.
- **The plan priced work no pass would do.** A pass drafts every unit of a
  language it works on that the corpus does not answer, but the loop passes over
  a language only while its coverage holds work (`localesNeedingPass`). The
  plan now derives that selection the way the run does and prices those
  languages alone, which also skips their lookups. The MCP `up_plan` tool takes
  the `no_checks` the `up` tool takes, so it prices the run that follows it.
- **A flow resolved the project's document index per block.** The follower
  named each document by loading every document and adoption the project knows,
  with their content, once for every block it compared, and the edit recorder
  did the same per record. `WorkStore.DocumentKey` answers for one path from
  two indexed rows. On the docs project this was most of a steady pass (38 s of
  its CPU).

A read of the change service, which every follower, `kapi extract` and every
editing surface makes, changed too:

| Benchmark | Before | After |
| --- | ---: | ---: |
| `BenchmarkSQLiteMemory_LookupExactMiss` (3,000 entries) | 10.2 ms | 16 µs |
| `BenchmarkSQLiteMemory_LookupBlockExactHit` | 160 µs | 73 µs |
| `BenchmarkChangeRead_TranslatedDocument` (401 blocks, a French basis on each) | 18.5 ms | 6.3 ms |
| `BenchmarkInterchangeRevisions` (the same document, as `kapi extract` reads it) | 10.8 ms | 6.5 ms |

```bash
go test -tags fts5 ./memory -run XXX -bench 'LookupExact|LookupBlockExactHit'
go test -tags fts5 ./host -run XXX -bench 'ChangeRead|InterchangeRevisions'
```

- **Bases were read one block at a time.** A read asked the block history for
  each derived edition of each block (one query apiece). A read now tells
  `EditionStates` how many blocks it shows at most. A short read (a page of up
  to 100 blocks, or the blocks it names) looks each one up, so it costs what it
  shows whatever the document's history holds. A longer read asks
  `history.Store.Latest` once for the editions its blocks hold, which finds each
  edition's latest change in one pass over the document's entries in the
  primary key and reads that row by a seek. Answering every read from the whole
  document's history in every language, through a subquery per history row, made
  a review pane's read of one block cost more than a read of the document had.
- **A joined read parsed the document twice and spooled skeletons nobody
  read.** A read with an edition joined from its own file read the document
  once to index it for the join and again to stream it, and every pass of a
  read gave its reader a temporary skeleton file, as did each markdown span of
  an MDX document. A joined read now parses the document once, a read-only pass
  wires a skeleton store that keeps nothing, and an MDX span keeps its own
  skeleton in memory. `kapi extract` gives a source's later language pairs the
  same store. This is the read extract makes of every pair. On the docs project
  it was about 70% of an extract (3.1 s of wall time with it, 1.1 s with the
  read skipped), and the whole extract now takes 2.6 s of CPU against 5.0 s.

The bases were read per document in one query (`Latest` over every edition,
with a subquery per history row) before the read said what it shows. Against
that, on a document of 400 blocks translated into five languages and rewritten
three times, read in French:

| Benchmark | One query per read | Shown blocks |
| --- | ---: | ---: |
| `BenchmarkChangeRead_MultilingualHistory/one-block` | 13.5 ms | 3.0 ms |
| `BenchmarkChangeRead_MultilingualHistory/page-of-100` | 16.0 ms | 6.1 ms |
| `BenchmarkChangeRead_MultilingualHistory/whole-document` | 18.5 ms | 6.8 ms |

The block history alone, for a document of 2,000 blocks in ten languages with
five changes to each translation (`BenchmarkLatest`,
`BenchmarkDocumentStates`):

| Read | Before | After |
| --- | ---: | ---: |
| `Latest`, every edition | 141 ms | 88 ms |
| `Latest`, one edition | | 13 ms |
| the bases of one block | 141 ms | 0.03 ms |
| the bases of a page of 100 blocks | 141 ms | 2.5 ms |
| the bases of the whole document in one language | 141 ms | 14 ms |

```bash
go test -tags fts5 ./core/history -run XXX -bench Latest
go test -tags fts5 ./host -run XXX -bench 'DocumentStates|ChangeRead_Multilingual|FlowRecord'
```

`BenchmarkFlowRecord_ManyDocuments` holds the follower's record to one
lookup of the document's key: a run over one document of 300 strings in a
project whose store knows 200 documents, every translation one the file
already held, records in 5.2 ms, and in 412 ms with the key resolved from the
whole document index for each string.

On the docs and synthetic projects the timings of the table above did not move
with the read's hint (docs steady pass 24.0 s of CPU against 24.2 s; synthetic
1.8 s against 1.9 s).

Three costs were measured and left:

- **Extract's second parse of a source.** `kapi extract` reads each source
  with its own reader, which captures the skeleton merge splices into, and the
  change service reads it again with the translation joined for the revisions
  each unit carries. On the docs project the second read is 0.19 s of an
  extract's 2.4 s of CPU samples. Reading once would take the change service
  handing out the blocks and the skeleton of its joined read.
- **The follower's second read.** A flow reads each document it follows through
  the change service before the run and, when the commit wrote the file, again
  after, so the recorded revision is the one a later read of the written bytes
  finds (a writer may normalize what it writes). On the docs project's rewrite
  pass the second read was 0.24 s of CPU across the documents it wrote, and the
  first 0.63 s, against about 50 s for the run. When the commit leaves the file
  as the run read it, the record already uses the first read.
- **Row iteration with a cancellable context.** The SQLite driver runs each
  `Rows.Next` of a query made under a cancellable context in a goroutine of its
  own, and the hand-off is a visible share of every profile. Stripping
  cancellation from reads saved 10% to 20% of a steady pass on the docs
  project, at the cost of a long query no longer stopping on Ctrl-C, and was
  not adopted.

The absorber, which reads the committed translations into the content memory
before a pass, was the largest cost of the docs project's rewrite pass (about
27 s of CPU, nearly all of it SQLite writing memory entries and their trigram
index) and is unchanged.

Bowrain's commit check, ship pass and review queue resolve the term gate from
an in-memory snapshot of the workspace's terms. Reading 1,000 concepts of three
terms from PostgreSQL for it took 22 ms on every call. Every write through the
terms store now replaces a per-workspace revision in its own transaction, and
the server keeps one snapshot per workspace under the revision it read first,
so a call while nobody writes the terms costs 0.36 ms
(`TestTermSnapshotCache_ReadsPostgresTermsOncePerRevision` logs both). The
migration that adds the revision gives every workspace already holding terms a
first one, so a deployed server keeps their snapshots from the first call. A
server keeps snapshots for at most 32 workspaces and drops the one used least
recently.
