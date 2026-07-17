---
id: 022-convergence-as-a-service
sidebar_position: 22
title: "AD-022: Convergence as a Service"
---

# AD-022: Convergence as a Service

## Summary

Convergence — the loop that reconciles a project's translations toward its ship
gates ([AD-framework-033: Project State Model](https://neokapi.github.io/web/neokapi/contribute/architecture/033-project-state-model)) —
is a single verb, `kapi up`, that runs in one of two **venues**: the developer's
machine or the Bowrain server. The server venue is *convergence as a service*:
`kapi up` on a connected project dispatches the whole loop to bowrain-server,
which converges on the organization's AI keys, its shared TM and terminology, and
its gate policy, records the run as a first-class **run** entity, streams progress
back to the caller, and routes whatever parks into the team's review and
assignment queue.

The server executes the **same** convergence engine the CLI runs locally
(`core/convergence`), driven by server-backed dependencies — the block store, the
translation job queue as its produce step, and the server's checks and gates.
There is no second, parallel translation pipeline: the reactive
translate-on-push automations collapse into the one goal-seeking loop.

## Context

Two things were true before this decision, and they conflicted:

1. `kapi up` was a purely local loop. It re-extracted drift, recycled from TM,
   called AI providers with the developer's keys, ran checks, and parked the
   remainder — all on the laptop, silent until done.
2. The server already translated content, but through a *different* machine:
   push landed a `push.completed` event, which fired an `auto-translate-on-push`
   automation, which enqueued per-item translation jobs. This fired once — nothing
   re-derived coverage, re-ran checks, demoted failing units, looped to the gates,
   or parked. And it was an implicit side effect of `push`, a transport verb.

So the platform carried two convergence loops under two names — the local `up`
and the push-triggered server automation — that did the same job with unrelated
code and produced unrelated progress stories. The friction surfaced as the
retired `sync` verb (push → wait for server translation → pull): a remote
convergence loop wearing a transport name.

The resolution separates three orthogonal concerns (see
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

### 2. Runs are first-class

A server convergence run is a `convergence_runs` entity (id, project, stream,
trigger, pass count, per-locale standing, cost, state), persisted in both SQLite
and PostgreSQL like the rest of the store. It exposes:

- an **SSE endpoint** that streams the convergence event protocol — the same
  `convergence.Event` types the CLI renders locally (plan, pass, per-locale
  progress, checks, parked/converged), so nothing about a run's rendering knows
  which venue produced it;
- **REST** list and cancel, and a **Runs page** in the Bowrain web app — the org
  dashboard of past and in-flight runs.

One event protocol runs end to end: engine → in-process CLI renderer (local
venue), and engine → SSE → the `kapi-bowrain` plugin → the same CLI renderer
(server venue), and engine → SSE → the web Runs page, and engine → the desktop
passes view. This single decision is what lets one verb span two venues without
the caller being able to tell them apart.

### 3. Trigger policy replaces reactive automations

When the server converges is an explicit per-project policy, `server.converge`
([AD-010](010-bowrain-cli-and-project-model.md)):

- `on-push` (default for connected projects) — every push starts a run. This is
  the old translate-on-push behavior, but running the full engine (checks, gates,
  parking) and recorded as a run anyone can watch. `kapi push` from CI just
  pushes; the server converges on its own clock; `kapi up` is push + *watch the
  run* + pull. The analogy is `git push` → CI → `gh pr checks --watch`.
- `manual` — the server converges only when `kapi up` is invoked (or a run is
  started from the Runs surface).

The reactive translation automations collapse into the engine. Drift detection in
the loop subsumes `auto-translate-on-push`, `auto-extract-on-push`, and
`auto-translate-new-locale` (they were three triggers for the same "produce the
missing targets" work); the engine's park step subsumes the review-task fan-out.
User-defined automations that start a run, or react to run outcomes (a
`run-parked` notification, a webhook `run_flow`), survive as first-class triggers.

### 4. Relation to transport and to the review queue

- **Transport stays pure.** `push` no longer translates as a side effect;
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

- [AD-framework-033: Project State Model](https://neokapi.github.io/web/neokapi/contribute/architecture/033-project-state-model) — the derived state, ladders, gates, and parking the loop reconciles toward
- [AD-010: Bowrain CLI and Project Model](010-bowrain-cli-and-project-model.md) — transport vs convergence vs venue; the `server.converge` policy; plugin dispatch
- [AD-009: Sync Protocol](009-sync-protocol.md) — the wire contract that `push`/`pull` use for transport
- [AD-013: Automation Engine](013-automation-engine.md) — the rule/run/SSE machinery the convergence run builds on
- [AD-015: Server-Side AI Operations](015-server-ai-operations.md) — the translation job queue the engine uses as its produce step
- [AD-014: Translator Workflow](014-translator-workflow.md) — the assignment and review queue that parked units enter
