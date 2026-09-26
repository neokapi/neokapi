---
sidebar_position: 6
title: Agent evaluation
description: Measure whether coding agents apply the rules a project holds and record its conventions, on a generated repository scored against an answer key.
---

# Agent evaluation

The agent evaluation in `scripts/skilleval` measures whether coding agents find,
apply and grow a project's context through kapi. Agents do ordinary writing
tasks in a documentation repository whose conventions are written down as an
answer key, and the harness scores what they wrote and what they recorded
against that key. The [paired agent evaluation](paired-agent-evaluation.md)
compares integrations across one task; this evaluation holds the integration
fixed and asks what an agent does with a project's context.

The commands, the order to run them in and how to read a report are in the
runbook, `docs/internals/agent-evaluation.md`.

## The fixture and its key

The fixture is the help centre of Loomwise, an invented rota app for cafés and
small shops: a README and eight pages of varied prose. Its answer key names
twelve entries:

| Kind | Entries |
|---|---|
| Names | the product, a feature and two plan names, each written one way |
| Spellings | British spelling throughout, "sign in" rather than "log in", "email" rather than "e-mail" |
| Avoided words | "roster" and "employee", with "rota" and "team member" used instead |
| Habits | the reader is addressed as "you"; no exclamation marks |
| Decoy | "timesheet" and "time sheet", used both ways about equally |

The first eleven are conventions. The decoy is something the project is
inconsistent about, and it must never become a rule.

Code generates the fixture, and generation is deterministic, so every run sees
the same bytes. The key is the one source of truth: names enter the prose only
through the key's own tokens, and generation checks the rendered pages against
the key before a cell is built. Every name and spelling appears several times
across several files, no avoided word or misspelt name appears anywhere, every
page addresses the reader as "you", and the decoy's two forms stay roughly even.
A page that broke any of these fails generation, which is how the fixture is
kept from contradicting its own key. The prompts are checked the same way: none
may mention kapi, context, rules or checking, or name a form of any convention.

Three writing tasks run on the fixture: a new help page for a feature, a monthly
release note, and a troubleshooting section.

## What one cell is

A cell is one host working one task, in its own copy of the fixture with its own
kapi roots. Every cell is prepared the same way: the fixture under git, the real
`kapi init --agents all` run over it by the binary under test, and the content
mapping its scaffold leaves for a person. A Measure 1 cell also has the planted
conventions put in force by a person before the agent starts. The commit created after preparation is the cell's baseline.

Cells are generated outside this repository. An agent host walks up from its
working directory looking for `CLAUDE.md`, and kapi walks up looking for
`kapi.yaml`, so a cell inside the tree would bind to neokapi's own project.
Preparation reports every discoverable file above a cell, and anything found
blocks the batch.

Each cell keeps its own `KAPI_DATA_DIR`, `KAPI_CONFIG_DIR`, `XDG_DATA_HOME`,
`XDG_CACHE_HOME` and plugin root, with `KAPI_PLUGINS_DIR_ONLY` set, a fresh
`HOME`, and a private PATH holding ordinary editing tools and this checkout's
kapi under each name the skill drives. The `.mcp.json` that `kapi init` writes
names a bare `kapi`; preparation resolves that name on the cell's PATH and
blocks the batch unless it is the build under test. Codex cells get the trust
entry a person would accept and the environment forwarding Codex needs, and
preflight asks Codex itself what it sees. A digest of the person's own data root
is taken before and after every session. Process-level confinement is not
claimed.

## Measure 1: agents apply context

The eleven conventions are established as rules with `KAPI_ACTOR=person`
before the session. Preparation reads the context log back and refuses a cell in
which any rule did not reach the held status.

While the session runs, the harness polls the working tree and keeps the first
saved version of every file the agent changed. Afterwards it runs `kapi check
--diff-against <baseline> --json` twice: over the final state, and over the
first versions laid on the baseline. A finding fails when its severity does,
which leaves out minor, neutral and unconfirmed advice. Each failing vocabulary
finding is traced to the convention whose rule it names.

A convention applies to a run when the text the agent added uses one of its
forms, correct or avoided, or when a finding names it. The two habits apply to
any added prose. The transcript says whether the agent ran a check after its
last write.

Acceptance criteria: 90% of applicable conventions followed in the first saved version, and
no failing finding at the end.

## Measure 2: agents grow context

These cells start without established rules. The agent's entries in
`kapi context log --json` are scored against the answer key. Entries by a person and
decisions such as confirmations are left out.

- A rule matches a convention when its replacement is a form the convention
  writes, when its term is a form the convention avoids, or, for British
  spelling, when the two sides are one word's American and British spellings.
- An observation matches when it names an avoided form or one of the
  convention's cues, and, for a name, when it names the name and says how it is
  written.
- The decoy is proposed when a rule names either of its forms on either side, or
  when an observation names a form and tells a writer what to do with it.

An entry outside the key is judged by a stated rule rather than by a model. A
rule is harmless when the project already obeys it and it points at wording the
project uses: its term appears nowhere in the fixture and its replacement does.
An observation is harmless when it carries evidence and every piece of that
evidence names a fixture file or quotes the fixture's own words. Anything else
is noise.

Recall counts the four names and three spellings a run recorded. Precision is
the share of entries that match the key or are harmless. Acceptance criteria: on average
half the names and spellings recorded per run, 80% precision over every entry,
and the decoy never proposed.

## Measure 3: settling

Measure 3 needs no live run. `eval_settle.go` scripts a story across four
machines that start from one agent session's suggestions: a second session
that repeats one rule and suggests a rival for another, CI recording a merge to
the default branch, a person's corrections, and a person's digest that keeps one
suggestion and drops another. Each machine settles its own log. The measure
then merges the four logs into a fresh workspace in all 24 orders, settling
after each merge, and is met when every order establishes exactly the expected
rules and every rule ends at the same status. The scenario lives in
`eval_product.go` with the rest of the product's vocabulary.

`TestEvalSettle_MeasureIsMet` runs it with the framework tests in CI, and every
report measures it afresh, with a table of each rule's status.

## Measure 4: review usefulness {#measure-4-review-is-worth-it}

After the grow runs, the report writes a review sheet: every entry each run
recorded, grouped under the convention it matches, then the decoy, then entries
outside the key, each with its evidence. It ends with three questions: how long
reading it took, which entries are worth keeping, and whether the entries showed
anything about the fixture that was not planted. A person answers in a small
YAML file, and the next report records the answers.

## Where the product's vocabulary lives

The evaluation measures kapi as it stands when it runs. How a person puts a rule
in force, what a held rule's status is called and which severities fail are in
`eval_product.go`. How each MCP tool counts (asking, recording, checking or
writing) is one table in `eval_transcript.go`. Preparation reports a server tool
that table cannot place, so a renamed tool shows up before a run rather than as
a silent zero in a report.

## Evidence handling

Transcripts, both versions of every changed file, the context log, review sheets
and the answer key stay in ignored local output under `harness/out/eval`. Every
attempt is bound to a fingerprint over the manifest, the fixture with its key and
prompts, the runner's source, the shipped skill and the kapi binary, so a
changed input calls for a fresh evidence directory rather than a mixed study.
The live phases share one persistent attempt ceiling, which counts failed
attempts too and never resets, and nothing retries automatically. Reports are
rendered from saved attempts with no model call, so a change to the scoring
reaches runs already made.
