# Agent comparison runbook

The comparison asks one question: does a project's context, held in kapi, make
coding agents write better than the same rules held without kapi? The same
writing tasks run in four arms on Claude Code and on Codex, with the same
prompt and model:

| Arm | What the cell holds |
| --- | --- |
| `kapi` | `kapi init --agents all --no-rules-files` (the MCP entry and the skill for each host), the recipe, the voice read in with `kapi store import`, and each held rename noted and kept by a person with `kapi context review --keep`. The voice file is removed afterwards, because the context lives in the store. |
| `kapifiles` | The same kapi project with the rules files kapi writes: a section of `AGENTS.md` and `CLAUDE.md` at the root and in each folder whose rules differ, refreshed with `kapi context sync --files-only` once the rules are held. |
| `rulesfile` | The same rules as a writing guide in `CLAUDE.md` and `AGENTS.md`, which each host loads into every session by itself. |
| `bare` | The content and nothing else. |

The source is `scripts/skilleval/compare*.go`; the projects, tasks and graders
are under `scripts/skilleval/testdata/compare/`, and the study manifest is
`scripts/skilleval/testdata/compare-study.json`.

## The projects and the rules

Three sample projects (Teamboard, a planning app; Ledgerly, invoicing for
freelancers; Harbor, a deploy tool). Each holds a rename, the house voice, a
competitor's name, banned claims and a few house words. Teamboard and Harbor
hold their rename in one part of the project only (customer help, not the API
reference; the docs, not the legal terms), noted at a file in that part and
kept with `--widen-to channel`, so it holds across that profile and nowhere
else. `context/rules.md` is the
same set of rules written as a style guide, and a test
(`TestCompareArmsHoldTheSameRules`) fails when the two drift apart.

Prompts name no rule and no kapi surface. Several use the old or forbidden word
themselves, the way a person asking would.

## Phases

```bash
make build
make compare-preflight                 # no model calls: cells, wiring, gate probe
make compare-pilot                     # live: the manifest's pilot tasks
make compare-grade                     # no model calls
make compare-judge                     # live: two judges per attempt
make compare-report COMPARE_SCOPE=pilot
make compare-run                       # live: the full grid
make compare-grade compare-judge
make compare-report COMPARE_SCOPE=run COMPARE_ARGS="-compare-out /path/report.md"
```

A live phase skips every attempt that already has a result, so an interrupted
run resumes where it stopped. `COMPARE_ARGS=-compare-retry` runs again the
attempts a rate limit, an outage or an interruption cut short.
`-compare-max-attempts N` caps one invocation.

The cells are generated outside the checkout (the system temporary directory,
or `COMPARE_CELLS_DIR`), each with its own HOME, private PATH and kapi roots.
Claude reads only the project's own settings; Codex gets a CODEX_HOME of its
own that marks the cell as trusted. Every cell is sandboxed. Claude's sandbox is on, with writes limited to the
cell and its temporary directory, reads of the developer's home and the shared
temporary directories denied, no network, and no unsandboxed fallback; the
kapi binary is copied into the cell so nothing needs the checkout. Codex runs
in `workspace-write` with the same writable roots and no network. The cells
directory must sit outside the developer's home and outside Claude Code's
shared temporary directories (`/tmp/claude-<uid>`), which the sandbox denies;
a cell there is refused at preparation.
Claude needs a keychain token that stays
valid for the attempt timeout plus 15 minutes; refresh the login, or export a
`CLAUDE_CODE_OAUTH_TOKEN` from `claude setup-token`, before a live phase.

## Scoring

- **Rule violations** (`compare_grade.go`): each project's house checks forbid
  a pattern in the prose an attempt added (Markdown lines it added, JSON values
  it added or changed; code and link targets set aside), outside the files the
  check exempts. A task's own checks read the final file: a rename update must
  leave no old name in scope and must leave the out-of-scope file alone.
- **kapi check**: every attempt's files, from every arm, are laid over the
  baseline in a grader cell that holds the rules and checked with
  `kapi check --diff-against`. It is reported beside the graders. It reports
  a scoped rename applied where the old name stays correct (the new name in a
  file under a "Keep as it is" rule), and a capitalised rename matches
  case-sensitively.
- **Voice fit** (`compare_judge.go`): two judges from different model families
  answer five yes/no questions about what each attempt wrote (reader
  addressed as "you", opens with the answer or the change, no promotional
  wording, no facts beyond the task's, short sentences), each with a passing
  and a failing example, without being told the arm or the host. A judge
  quotes the words behind every "no". Changing a question means changing
  `compareRubricVersion`, which makes the judge phase ask again. The score is the share of yes answers, averaged
  over both judges. It is reported as validated only when the judges agree at
  Cohen's kappa of at least 0.6 over at least 30 paired answers.
- **Intervals**: Wilson for shares, and a percentile bootstrap that resamples
  attempts within each task for means and differences.

## Changing the verbs

The harness runs three kapi verbs itself, as the person who sets the project
up: `compareVerbImport`, `compareVerbRecord` and `compareVerbKeep` in
`compare.go`. A change to the CLI is an edit there. A verb retired from
`kapi context` exits 2 and names its replacement; a verb removed elsewhere may
not fail, so preflight's gate probe and the context read are what catch a
project that did not load.
