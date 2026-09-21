---
sidebar_position: 6
title: Cold-start drill
description: Measure whether an agent given an ordinary writing task grows an empty project context in a way a person would confirm.
---

# Cold-start drill

The cold-start mode in `scripts/skilleval` answers one question. An agent is
given an ordinary writing task in a project whose context holds nothing, with
the shipped skill and MCP server and no other guidance. Does it grow that
context by itself, and does a person keep what it recorded?

The premise fails when agents on two hosts record nothing a person would
confirm. That outcome is a row in the report with zeros in it, and the drill is
built so that row can be produced.

The [paired agent evaluation](paired-agent-evaluation.md) compares integrations
across the same task. This drill holds the integration fixed and measures what
one session leaves behind for the next.

## What one cell is

A cell is one host working one first-session task in its own copy of a
generated repository, with its own kapi workspace. Its second session runs in
the same cell, so it reads whatever the first session recorded and the person
confirmed. Each cell starts from a repository of its own, because an agent
changes the tree it reads.

The fixture is a product team's repository: a README, documentation pages, a UI
strings file and an email template. Its prose keeps conventions nobody wrote
down, among them a product name with one casing, one verb for entering the
product, a feature name that keeps its spelling, and a second-person address.
Nothing in it mentions kapi.

It is generated in a temporary directory outside this repository. An agent host
walks up from its working directory looking for `CLAUDE.md`, and kapi walks up
looking for `kapi.yaml`, so a fixture inside the tree would bind to neokapi's
own project. Preparation walks up from the fixture and reports every
discoverable file above it; anything found blocks the batch.

## Phases

```sh
make build
make coldstart-preflight
```

Preflight makes no model call. It generates the fixture, puts it under git, runs
the real `kapi init --agents all` over it with the binary under test, starts the
MCP server the wiring names, and records what answers.

```sh
make coldstart-smoke          # one first session per host
make coldstart-review         # the sheet, and the readback afterwards
make coldstart-session-two
make coldstart-report
```

`make coldstart-session-one` runs every cell's first session rather than the
smoke task alone. All three live phases share one persistent attempt ceiling
(`COLDSTART_MAX_ATTEMPTS`, two by default), which counts failed attempts as well
as completed ones and is never reset by re-running a command. There are no
automatic retries. `COLDSTART_MANIFEST` and `COLDSTART_DIR` select the manifest
and the evidence directory, which defaults to the ignored `harness/out/coldstart`.

`COLDSTART_CELLS_DIR` says where the cells themselves are generated. The default
is the system temporary directory, which macOS sweeps after a few days, and a
person's review can come later than that, so a batch whose review is not
immediate names a directory that outlives it. A path inside this checkout is
refused, and whatever sits above the chosen directory is measured per cell the
same way.

## The task set

The manifest names at least three first-session tasks, and exactly one of them
carries a person's wording correction in its prompt ("I changed your X to Y; we
always say Y"). That prompt is the only cue for `context_correct`. With none,
nothing cues the third habit; with two, a session that recorded one correction
reads the same as a session that recorded both.

No prompt mentions kapi, context, terms, recording or checking. A prompt that
named any of them would be asking for the behaviour rather than measuring it.
`TestColdStartPromptsAskForWritingOnly` holds that line.

## The three measures

**What the first session recorded.** The store read back through `kapi context
log --json`, counted by operation kind, with evidence present or absent on each
entry. The report also lists which kapi tools the session called, over either
surface: MCP tool names and kapi commands run from a shell are classified the
same way, because the question is what the agent did rather than which door it
used.

**What a person confirmed.** `make coldstart-review` prints each cell's
candidates with the exact commands to confirm or discard them, and a command that
opens Kapi Desktop on the cell's workspace. The harness confirms nothing itself
and no judge model stands in for the person. Running the review phase again reads
back what the store holds.

macOS `open` hands an application the login session's environment rather than the
calling shell's, so the desktop command carries the cell's roots in `--env`
arguments and starts an instance of its own with `-n` to receive them.
`COLDSTART_DESKTOP_APP` points that command at a locally built bundle; unset, it
names the application the shipped bundle registers.

Both surfaces name the actor the store holds on each entry, with the agent's name
and the session that groups one run of it. The report counts a session's entries
by actor kind, and a session is one agent working a task, so a count under a
person is an entry whose attribution the store lost. That is what
[#2912](https://github.com/neokapi/neokapi/issues/2912) was about, and why the
report puts those cells in a list of their own.

**What the second session did.** Whether a context read came before its first
change to a file, and what `kapi check --diff-against HEAD --json` reported over
its output, run through the shipped command against the cell's own store.

Reports are rendered from saved attempts with no model call, so a scoring change
can be applied to sessions already run.

## Isolation, established rather than assumed

Each cell keeps its own `KAPI_DATA_DIR`, `KAPI_CONFIG_DIR`, `XDG_DATA_HOME`,
`XDG_CACHE_HOME` and plugin root, a fresh `HOME`, and a private PATH holding
ordinary editing tools and this checkout's kapi under each name the skill drives.
`KAPI_NO_PROJECT` is absent on purpose: the fixture's own recipe has to be
discoverable, and that discovery is part of what the drill measures.

The `.mcp.json` that `kapi init` writes names a bare `kapi`. Preparation resolves
that name on the cell's PATH, records what it resolves to, and blocks the batch
unless it is the build under test. A stale release answers with the same shape
as a current one, which is how a live feature once looked unwired (#2642).

Preparation also records a digest of the person's own data root, taken before and
after every session, and the workspace file that appeared under the cell's data
root instead. Process-level confinement is not claimed for this drill; the
controls are the fresh HOME, the private PATH, the cell's own kapi roots, and
evidence that stays in ignored local output.

## What the harness completes, and why the report says so

Two things `kapi init` leaves for a person, the trust a person grants Codex, and
one thing the cell's isolation asks for:

- The scaffolded recipe holds an empty collection list, and its own comments say
  to point collections at the files to keep in voice. The harness writes that
  mapping.
- The scaffold binds a starter voice pack, whose tone, style and vocabulary are
  its own. The harness removes it, because a drill run over a pack would measure
  the pack rather than a cold start.
- `kapi init` writes the kapi server into the repository's own
  `.codex/config.toml`, and Codex reads that file for a repository the person has
  trusted. The harness marks the fixture as a trusted project in the cell's own
  `CODEX_HOME`, which stands for the trust prompt, and leaves the server entry as
  the product wrote it. Preflight then asks Codex what it sees, with `codex mcp
  list --json` under the cell environment and no model call, and blocks the batch
  when the kapi server is missing or resolves to anything but the build under
  test.
- Codex starts a stdio MCP server with a filtered environment: `HOME`, `PATH` and
  a few locale variables reach it and the rest are dropped. The cell's kapi roots
  are therefore named on the launch itself, through the `env_vars` list Codex
  forwards from its own environment, and preflight refuses a launch that would
  leave them out.

Each of these appears under "What the harness wired by hand" in the report, so a
reader knows which part of the first run was shipped and which part was
completed here.

## Reading a negative result

A cell whose first session called no kapi tool reports `none` in the tools
column and `nothing` under by-kind. Keep the attempt: an empty row is the
measurement. Read it against three other columns before concluding anything.

- Did the skill load at all? `skill_loaded` in the attempt record answers for
  Claude's skill loader and for a host that meets the guidance by reading
  `SKILL.md`.
- Did the server start? The preparation record holds the server's own name,
  version and tool inventory.
- Did the session finish? A timeout, a turn cap or a rate limit produces an
  empty store for reasons that have nothing to do with the premise.

With the wiring established and the session completed, a run of empty rows on
both hosts is the result that ends the premise. One host's empty rows say
something about that host.

## Evidence handling

Transcripts, generated fixtures and review sheets stay in ignored local output.
Every attempt is bound to a fingerprint over the manifest, the fixture and its
prompts, the runner's own source, the shipped skill and the kapi binary, so a
changed task or a rebuilt binary calls for a fresh evidence directory rather
than a silently mixed study. The study record carries the kapi version and
commit, and each attempt records the host version it ran against.
