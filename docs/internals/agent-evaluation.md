# Agent evaluation runbook

How to run the agent evaluation and read what it says. What it measures, and
the rules it scores by, are on the contributor page
[Agent evaluation](../../web/docs/contribute/implementation/repo/agent-evaluation.md).

The live phases drive Claude Code and Codex on the signed-in subscriptions and
consume plan allowance. Nothing here runs in CI.

## Before a run

1. Build the binary under test once: `make build`. Every build stamps a new
   binary, and the fingerprint covers the binary, so a rebuild between phases
   starts a new study. Build again only when you mean to.
2. Sign in to both hosts: `claude` with a claude.ai subscription, and `codex
   login` with ChatGPT. Preparation copies nothing else from your configuration.
3. Choose where the cells live. The default is the system temporary directory,
   which macOS sweeps after a few days. When the review comes later than that,
   name a lasting directory outside the checkout with nothing discoverable above
   it:

   ```bash
   export EVAL_CELLS_DIR=~/Library/Caches/kapi-eval
   ```

## The phases

```bash
make eval-preflight   # fixture, every cell, wiring checks; no model call
make eval-smoke       # one Measure 2 run per host
make eval-apply       # Measure 1: three runs per host
make eval-grow        # Measure 2: three runs per host
make eval-report      # report and review sheet from saved attempts; no model call
```

Preflight prepares all fourteen cells the live phases use and prints, per cell,
the server that answers, the rules held on a Measure 1 cell, and what Codex
sees. A blocker line stops the live phases before any session starts. Read the
notes too: a server tool the transcript reader cannot place, or a habit the
server offers no tool for, is reported there.

The live phases share one ceiling, `EVAL_MAX_ATTEMPTS` (fourteen by default:
two smoke runs and three runs per host on each measure). It counts started
attempts, failed ones included, across every phase in the evidence directory,
and nothing resets it or retries on its own. A rate limit pauses the batch.
Running a phase again resumes it: attempts already started are kept, and only
sessions that never started are run. `EVAL_ARGS=-eval-sessions <id>,<id>` runs
chosen sessions within the ceiling.

Evidence goes to `harness/out/eval` (`EVAL_DIR`), which is ignored. Changing
the manifest, the fixture, the runner's code, the shipped skill or the binary
changes the fingerprint, and a live phase then refuses the directory. Choose a
fresh `EVAL_DIR` for the new study and keep the old one.

## Reading an attempt

Each session leaves `harness/out/eval/<phase>/<session>/` with the prompt, the
preparation record, the transcript, `result.json`, the first and final version
of every file it changed under `first/` and `final/`, and the context log after
the session. The report is recomputed from these, so a change to the scoring
reaches runs already made: run `make eval-report` again.

Before reading a score, read three things:

- Did the session finish? A timeout, a turn cap or a rate limit shows in the
  status column and leaves the run out of the host's verdict.
- Did the server start and was it the build under test? The preparation record
  holds the server's name, version and tools.
- Did the agent reach kapi at all? The report lists the kapi tools each run
  called, over MCP or from a shell.

A host's verdict reads `unmeasured` until every task has a completed run.

## The review

`make eval-report` writes a review sheet beside the report once a grow run
exists: `report-<time>-review.md`. Read it as the owner of the Loomwise help
centre would, then write `review-answers.yaml` in the evidence directory:

```yaml
minutes: 6
keep:
  grow-feature-page-claude: ["4", "7"]
  grow-release-note-codex: ["2"]
learned: |
  The release notes always lead with the headline change.
```

`keep` names entries by run and by the id the sheet shows. `learned` is anything
the entries showed about the fixture that the key does not plant. Run `make
eval-report` again and the report records the answers under Measure 4.
`EVAL_ARGS=-eval-answers <path>` reads the answers from elsewhere.

## When the product changes

The evaluation measures kapi as it stands when it runs. When a product change
renames what the evaluation reads, the change belongs in one place:

- how a person puts a rule in force, a held rule's status, which severities fail
  and the operation kinds: `scripts/skilleval/eval_product.go`;
- how an MCP tool counts: the table in `scripts/skilleval/eval_transcript.go`;
- the shape of `kapi context log --json` and `kapi check --json`:
  `scripts/skilleval/eval_store.go`, whose tests read recordings in
  `scripts/skilleval/testdata/agenteval/`. Re-record them from the current
  binary when the shape moves, and the tests say what the scoring now reads.
