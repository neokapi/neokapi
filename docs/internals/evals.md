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
task families and three repetitions, 2 × 4 × 7 × 3 = 168 live sessions. Each
host keeps one model and effort for every arm: Codex with `gpt-5.6-terra` at
medium effort, Claude Code with `claude-sonnet-5` at high effort. A session has
600 seconds and 40 turns. The schedule shuffles blocks of task, host and
repetition, then the four arms within each block, from the manifest's seed.

| Arm | kapi surface in the cell | Project |
| --- | --- | --- |
| `baseline` | none: no skill, no MCP server, no kapi name on PATH | none |
| `skill-cli` | the shipped kapi skill; `kapi` on PATH | the fixture's recipe, bound through `KAPI_PROJECT` |
| `mcp` | the kapi MCP server (its default writing set); `kapi` on PATH; no skill | the fixture's recipe, bound with `-p` |
| `project-free` | `kapi-files`, the same binary under a multi-call name (`cli.BusyboxRoot`) exposing `inspect`, `apply`, `formats` and the toolbox with discovery off, and a skill with the shipped skill's edit and toolbox guidance | none: the recipe sits in the cell and is never read |

Every arm keeps the host's ordinary shell and file tools. Every cell holds the
same files, the task's content, its `kapi.yaml` and a `STYLE.md`, and every
project's store holds the same context, imported before the session starts: a
voice and one term rule, which forbids `portal` and names `overview page` in
its place. `STYLE.md` states that rule for an agent that reads files. No skill
an arm installs and no MCP tool description uses the word, which a test
asserts, so no arm is primed for or against it.

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
path into the checkout, and Claude's sandbox denies reading the checkout and
the attempt's own records.

Section 15.2 item 1 asks for the alias against kapi with and without a project.
The 168 sessions hold four arms, so the comparison maps onto them:
`skill-cli` and `mcp` are kapi with a project, and `project-free` is the binary
without one. kapi's own CLI with discovery off is not a fifth arm: its edit
commands are the code paths the alias runs, and what differs is the extra
commands and the skill text. Measuring it would add 42 sessions (two hosts,
seven tasks, three repetitions).

The seven tasks, one per WP5 family, follow. Every task fails when nothing
changes, every fixture file outside the edit must stay byte-identical, and no
file may appear in the scoped directories except one the task creates.

| Task | Family | Files | Graded by |
| --- | --- | --- | --- |
| `edit-link-html-md` | wording and link | `site/help.html`, `docs/help.md` | byte diff against the reference; a FAQ link shares the old address as a prefix and a code block holds it verbatim |
| `edit-plural-branch` | plural branch | Flutter ARB `lib/l10n/app_en.arb` | byte diff; the edited `one` branch, the `=0` and `other` branches with the plural's syntax around them, and the `@inboxCount` metadata each checked byte for byte; the other branches also contain the words being replaced |
| `add-edition-markup` | new edition | `docs/en/welcome.md` to a new `docs/nb/welcome.md` | structure: headings, list items, bold spans, inline code and link addresses in order; every block translated; Norwegian Bokmål by its function words; byte equality with the reference reported only |
| `edit-po-context` | bilingual PO | `locales/nb/messages.po` | byte diff; two entries share `msgid "Book"` and differ by `msgctxt` |
| `add-json-key` | key added (`insert_block`) | `locales/en.json` | the JSON leaves in document order, so the key lands in `settings` after `importData`; layout byte equality reported only |
| `recover-stale-read` | stale recovery | `docs/en/upgrade.md` | byte diff of the agent's sentence and another editor's change to the same paragraph |
| `recover-gate-refusal` | gate refusal recovery | `docs/en/reports.md` | the first paragraph keeps its text and gains a sentence that names CSV and does not say `portal`; every other block unchanged; no `portal` anywhere; no override attempt in the transcript; naming the overview page reported only |

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
shows the `other` branch, is refused as a flattening guard. The task's reference
route is that `replace_text`, and `TestPairedPluralRouteOnEverySurface` sends it
through `kapi apply` in the project, `apply_edits` on the MCP server and
`kapi-files apply` without a project; each result passes the task's graders.
The other plural in the catalog and the `@inboxCount` placeholder declaration
are there to be left alone.

Two tasks act on the session while it runs. In `recover-stale-read` the runner
watches the host's stream and, once a tool result has shown the agent the text
"it takes about five minutes", changes it to "about ten minutes" in the
paragraph the agent edits. The prompt asks for the agent's sentence and says
nothing of another editor, so an agent learns of the change only from what its
tools report. A search that printed only the agent's own sentence
does not land it, and neither does a command that names the text without
printing it. A kapi arm's write against the old revision is refused `stale`.
Claude Code 2.1's Edit tool applies an edit to a file changed since it was read
and adds a note saying so, which the record counts as `host:stale`; Codex's
patch fails when its context lines changed (`host:patch_failed`). The record
says whether the change landed before the agent's write, after it, or never: an
agent that writes with `ksed` before reading never sees it, and the file is
then graded against the reference without the other editor's change. The
score report counts recovery over the attempts whose change landed before the
agent wrote, and separately over those that met a conflict signal.
`recover-gate-refusal` asks for a sentence "from the portal". In the two arms
with a project, `kapi apply` and `apply_edits` refuse it `gate_failed` and name
the replacement, and an agent's `--gate report` is refused `not_permitted`. The
baseline and project-free arms have no gate, so for them the task measures
whether they follow `STYLE.md` unprompted. The grader asks for a sentence about
CSV downloads without the forbidden word; a sentence that names the overview
page is reported, and one that refers back to the overview page the paragraph
already names also passes.

Beside the graders, each attempt records its status, duration, input and output
tokens (cache reads and writes kept apart), Claude's turn count, tool calls, the
refusal codes its tool results carried (kapi's own, `host:stale` or
`host:patch_failed` for a host tool's, and `invalid:<pointer>` for a change set
that did not decode, with each array position as `*`), override attempts
(`--gate report`, a change set with `"gate": "report"`, `KAPI_ACTOR=person`,
`if_match: "*"`), kapi names and skills it tried that its cell does not hold,
and paths it named outside its cell. The score report lists the decode errors
per task, host and arm: they are the names and shapes agents reach for that the
contract does not take. The Bokmål check of `add-edition-markup` asks for a
form only Bokmål writes (`deg`, `inn`, `etter` and the like) and none only
Danish or Swedish writes, since both share most of its function words.

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
the study alone. The main checkout is shared with other work and rebuilt during
the day; the study's worktree is touched by nothing else until the study is
scored. From the main checkout, once this change has merged:

```bash
git fetch origin
git worktree add .claude/worktrees/wp5-study origin/main
cd .claude/worktrees/wp5-study
make i18n-catalogs && vp install && make build
PAIRED_TEST_KAPI="$PWD/bin/kapi" go test -tags fts5 ./scripts/skilleval \
  -run 'PairedSolutions|PluralRoute|ProjectFreeAlias|WithBuiltKapi'
export HOMEBREW_NO_AUTO_UPDATE=1
make paired-eval-preflight PAIRED_EVAL_DIR="$HOME/kapi-wp5-study"
# The first stage: eight sessions.
caffeinate -i make paired-eval-pilot PAIRED_EVAL_DIR="$HOME/kapi-wp5-study" \
  PAIRED_EVAL_MAX_ATTEMPTS=8 PAIRED_EVAL_CONCURRENCY=2
# Read the eight transcripts. The second stage: the other 160, and 12 reruns.
caffeinate -i make paired-eval-pilot PAIRED_EVAL_DIR="$HOME/kapi-wp5-study" \
  PAIRED_EVAL_MAX_ATTEMPTS=180 PAIRED_EVAL_CONCURRENCY=2
make paired-eval-score PAIRED_EVAL_DIR="$HOME/kapi-wp5-study"
```

The test line sends each task's reference route through the worktree's build
before any session is spent, and the plural task's through each arm's surface.
The preflight prints the checkout and host versions a study started then would
pin. For a long Claude run, export a long-lived `CLAUDE_CODE_OAUTH_TOKEN` from
`claude setup-token` first: a keychain token must stay valid for a session's
limit and fifteen minutes more, or the session is not started.

The ceiling counts every attempt the study has started, the first stage's
included. The study is 168 sessions, so after the first eight a ceiling of 168
runs the other 160; 180 adds the twelve reruns `PAIRED_EVAL_RETRY=1` may need,
and every rerun counts against it. A second-stage ceiling of 160 would stop
eight sessions short of the grid.

The cells live in the study directory, so it sits outside the checkout: the
runner refuses a directory with a checkout, an instruction file, host
configuration or a kapi recipe above it, which an agent in a cell would find.
A directory in the home folder survives a restart, which `/tmp` does not.

When the study starts it copies `bin/kapi` and the shipped skill into
`inputs/` in the study directory, and every session runs those copies. Its
`study.json` records, and its fingerprint covers, the checkout it runs from,
the commit, the hash of its copy of kapi and each host's `--version`. To resume
after an interrupt, a rate limit or a paused host, run only the
`make paired-eval-pilot` line again from the same worktree; it needs no build.
The runner refuses a run from another checkout, naming the one the study runs
from, and a run with `claude` or `codex` at another version than the study
started with, naming that version to reinstall: a session on another version
would measure another agent. Each session's own version check uses the same
pin, so a host upgraded before its first session is refused too. A new
directory would run every session again.

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

To be filled after the run: per host, task and arm, the objective pass rate
over the three repetitions, median duration, tokens and tool calls, the
refusals met and recovered, override attempts, and what the human review of
the attempts found. Then the decision for the v1 freeze and for D14.

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
