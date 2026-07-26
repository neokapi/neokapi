---
id: 022-convergence-as-a-service
sidebar_position: 22
title: "AD-022: Convergence as a Service"
---

# AD-022: Convergence as a Service

## Summary

Convergence — the loop that reconciles a project's translations toward its ship
gates ([AD-framework-033: Project State Model](https://neokapi.github.io/contribute/architecture/033-project-state-model)) —
is a single verb, `kapi up`, that runs in one of two **venues**: the developer's
machine or the Bowrain server. The server venue is *convergence as a service*:
`kapi up` on a connected project dispatches the whole loop to bowrain-server,
which converges on the organization's AI keys, its shared content memory and terminology, and
its gate policy, records the run as a first-class **run** entity, streams progress
back to the caller, and routes whatever parks into the team's review and
assignment queue.

The server executes the **same** convergence engine the CLI runs locally
(`core/convergence`), driven by server-backed dependencies — the block store, the
translation job queue as its produce step, and the server's checks and gates.
There is no second, parallel translation pipeline: producing the missing
targets is the one goal-seeking loop's own work.

A server run is **source-first**: before it translates a single locale it
settles the source and holds at a source ship-gate. It never AI-translates a
block whose source is below the project's `source_gate` — under-ready source
**holds** the fan-out (`stall_reason = source_not_ready`) and opens a
source-review task, rather than paying to translate an unsettled source into
every language.

## Context

A project converges in two very different settings. On a developer's machine
the loop re-extracts drift, recycles from content memory, calls AI providers
with that developer's keys, runs checks, and parks the remainder. On the
server it must do the same job against the organization's keys, its shared
content memory and terminology, its gate policy, and its review queue — and
report progress to everyone watching, not just the person who started it.

The temptation is to build the server side as a set of reactive rules: a push
lands, an automation fires, translation jobs enqueue. That produces a second
machine with none of the loop's properties — it fires once rather than seeking
a goal, so nothing re-derives coverage, re-runs checks, demotes failing units,
loops back to the gates, or parks. It also makes translation an implicit side
effect of `push`, a transport verb, and leaves two code paths that do the same
job and tell unrelated progress stories.

So the three concerns are kept orthogonal (see
[AD-010](010-bowrain-cli-and-project-model.md)): **transport** (`push`/`pull`,
pure data movement), **convergence** (`up`, the one loop), and **venue** (where
the loop's compute runs). This AD specifies the server venue.

## Decision

### 1. The server runs the same engine

The convergence loop is a framework engine (`core/convergence`), parameterized
over the interfaces it needs: a content store, a flow runner, checks, and gate
policy. The CLI drives it with file-backed dependencies; the server drives it
with the block store, the existing translation **job queue as its flow runner**,
and server-side checks and gates. One loop, two harnesses.

This makes the fable-era invariant concrete — *no server-side reimplementation of
convergence*. The per-item translation jobs that the server already runs become
the engine's produce step; the engine adds the derive → check → demote → park
bracket around them that the reactive automation never had.

### 1a. Settle the source, then translate — the source ship-gate

A run has two phases with a gate between them, not one fan-out. The **first
pass settles the source** over the source locale only — source checks, source
QA, and marking do-not-translate/protected terms so translation can never alter
them — and stamps each unit's source status up the authoring ladder (*authored →
checked → approved*, [AD-framework-033](https://neokapi.github.io/contribute/architecture/033-project-state-model)).
Only then does the produce step translate, and it translates only the *approved*
source, memory-first (recycle before paid AI), so the run's `content memory N · AI M` split is
truthful.

The gate between the phases is the project's **`source_gate`**
(`defaults.source_gate`; default `checked` — the automated bar — with `approved`
for human sign-off and `none` to opt out). A block whose source is below the gate
is **not** translated: the run **holds** rather than fanning it out.

Holding is a first-class outcome, symmetric with parking:

- the run parks with `stall_reason = source_not_ready` and records a
  `blocked_on_source` count on the run entity, so a surface renders *"N blocks
  need source review"* without a round-trip;
- it opens a **source-review task** (`source_review`,
  [AD-014](014-translator-workflow.md)) in the team's queue instead of enqueuing
  translation jobs. Clearing that task (`source.review.completed`) lifts the
  source to the gate and lets the held fan-out proceed on the next pass
  ([AD-013](013-automation-engine.md)).

This is the venue's differentiator made mechanical: source content is shared
across every locale, so a defect left in the source would be paid for once per
language and again on the fix; the gate settles it once, before the fan-out.
A source edit re-opens the gate for only the changed segments, so a fix
re-settles and re-translates just those.

### 2. Runs are first-class

A server convergence run is a `convergence_runs` entity (id, project, stream,
trigger, pass count, per-locale standing, cost, state, and — for a source hold —
a `stall_reason` and a `blocked_on_source` count), persisted in both SQLite
and PostgreSQL like the rest of the store. It exposes:

- an **SSE endpoint** that streams the convergence event protocol — the same
  `convergence.Event` types the CLI renders locally (plan, pass, per-locale
  progress, checks, parked/converged), so nothing about a run's rendering knows
  which venue produced it;
- **REST** list and cancel, and a **Runs page** in the Bowrain web app — the org
  dashboard of past and in-flight runs;
- a provider-free **estimate** (`GET /projects/:id/convergence/estimate`) that a
  *Run now* consent flow reads before spending anything: it reports source
  readiness first (ready vs. held blocks against the gate), then the per-locale
  `content memory N · AI M` split and the credit/cost estimate for the ready source only.

One event protocol runs end to end: engine → in-process CLI renderer (local
venue), and engine → SSE → the `kapi-bowrain` plugin → the same CLI renderer
(server venue), and engine → SSE → the web Runs page, and engine → the desktop
passes view. This single decision is what lets one verb span two venues without
the caller being able to tell them apart.

### 3. Trigger policy

When the server converges is an explicit per-project policy, `server.converge`
([AD-010](010-bowrain-cli-and-project-model.md)):

- `on-push` (default for connected projects) — every push starts a run of the
  full engine (settle source, gate, checks, target gates, parking), recorded as
  a run anyone can watch. Being source-first, a push over an unsettled corpus
  holds on source rather than fanning out. `kapi push` from CI just pushes; the
  server converges on its own clock; `kapi up` is push + *watch the run* + pull.
  The analogy is `git push` → CI → `gh pr checks --watch`.
- `manual` — the server converges only when `kapi up` is invoked (or a run is
  started from the Runs surface).

Producing missing targets is the engine's own work, not an automation:
drift detection in the loop decides what to produce, and the park step fans out
review tasks. Automations are for what a workspace adds on top — starting a run,
or reacting to a run outcome (a `run-parked` notification, a webhook
`run_flow`).

### 4. Relation to transport and to the review queue

- **Transport stays pure.** `push` moves content and nothing else;
  server-side convergence is the `server.converge` policy, not a property of
  `push`. `kapi up` on a connected project is, mechanically, push (transport) →
  start/attach a run → stream → pull (transport), with `--local` inverting the
  order (converge on the machine, then push results, so the server is never left
  stale).
- **Parked work enters the team queue.** Locally, parked units are the
  developer's `kapi status --review` worklist. On the server they enter Bowrain's
  assignment and review machinery ([AD-014](014-translator-workflow.md)) —
  assigned, tracked, audited. This is the single-player → multiplayer seam: "I
  want someone else to review this" is expressed by connecting the project, not
  by learning a new verb.

### 5. Licensing boundary

The open-source `kapi` binary carries zero server code. The server venue arrives
only through the installed `kapi-bowrain` plugin, which declares the `up` verb in
its manifest — so kapi dispatches `kapi up` to the plugin (a manifest command
overrides the built-in of the same name). The plugin's `up` detects the `server:`
block, starts (or attaches to) the server run, subscribes to its SSE stream, and
re-emits the convergence event protocol onto the shared renderer. No
new plugin transport is needed beyond the established subprocess dispatch
([AD-010](010-bowrain-cli-and-project-model.md)).

## Consequences

**Positive:**

- One convergence engine, two venues — no divergent server translation pipeline
  to keep in step with the CLI loop.
- Server-side translation becomes a visible, gated, auditable run instead of an
  implicit push side effect.
- The paid product has a clean justification: the venue — always-on, org-keyed,
  shared-asset, governed convergence — rather than the loop itself, which is
  open-source and runs anywhere.
- Keys leave laptops: one org-configured provider replaces N developers each
  wiring their own AI credentials.

**Negative:**

- The convergence engine must be extracted from the CLI into a framework package
  with store/flow-runner/checks/gates interfaces clean enough for two harnesses —
  more up-front factoring than a server-local reimplementation.
- Cost visibility and rate limiting move server-side for the default venue; the
  server owns a shared provider limiter across concurrent per-locale workers.

## Related

- [AD-framework-033: Project State Model](https://neokapi.github.io/contribute/architecture/033-project-state-model) — the derived state, ladders, gates, and parking the loop reconciles toward
- [AD-010: Bowrain CLI and Project Model](010-bowrain-cli-and-project-model.md) — transport vs convergence vs venue; the `server.converge` policy; plugin dispatch
- [AD-009: Sync Protocol](009-sync-protocol.md) — the wire contract that `push`/`pull` use for transport
- [AD-013: Automation Engine](013-automation-engine.md) — the rule/run/SSE machinery the convergence run builds on
- [AD-015: Server-Side AI Operations](015-server-ai-operations.md) — the translation job queue the engine uses as its produce step
- [AD-014: Translator Workflow](014-translator-workflow.md) — the assignment and review queue that parked units enter
