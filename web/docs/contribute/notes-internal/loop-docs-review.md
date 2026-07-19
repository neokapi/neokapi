---
title: The kapi loop — documentation accuracy review
description: A per-page review of the convergence / kapi-loop / source-first documentation across the kapi and bowrain docs sites, checked against the code and the source-first model (epic 019), with a prioritised update list for when the source-first gate lands.
---

# The kapi loop — documentation accuracy review

Internal note. This reviews the existing documentation about **convergence** —
the kapi loop, `kapi up`, source readiness, and the translation flow — across
both docs sites, and checks each page against (a) the code and (b) the
**source-first** model in roadmap epic **019** (settle the source → source
ship-gate → translate the approved source per locale, TM-first → target
ship-gate → converged; a gate that is not met *holds*, it does not ship).

The verb is `kapi up`; the model is *Shape → Ship*, and specifically
source-first convergence — the ship-gate is the seam where governed review
meets the loop.

This note is the review plus one explainer (the source-first section added to
[The kapi loop](/kapi/convergence#source-first) with the `GatedLoopDiagram`
visual ship-gate). It deliberately does **not** rewrite the docs; the fuller
rewrites are listed under [Follow-ups](#follow-ups).

## What the code actually does today (verification baseline)

The claims below were checked against source. Confirmed unless noted.

- **`kapi up`** exists with flags `--plan`, `--passes`, `--materialize`,
  `--local`, `--server`, `--jobs`, `--no-extract`, `--no-checks`, `--json`
  (`cli/up.go`, `host/up.go`). Default pass cap is `ConvergeMaxPassesDefault`
  (5); `--jobs` falls back to `defaults.jobs`.
- **Default flow** when `defaults.flow` is absent is the built-in `translate`
  flow: `recycle → translate → qa` (`host/flowdef/builtin.go`). So `kapi up` is
  TM-first by construction on the CLI.
- **`kapi status` / `--review`**, **`kapi check --ship`**, **`kapi apply`**
  (`kind: "review"` and `kind: "tm"`) all present
  (`cli/status.go`, `cli/check.go`, `cli/apply.go`).
- **Ladders:** source `authored → checked → approved`
  (`core/model/sourcestatus.go`); target `draft → translated → reviewed →
  signed-off` (`core/model/target.go`).
- **Recipe keys** `defaults.flow`, `defaults.jobs`, `defaults.tm_source`,
  `defaults.termbase_source`, `defaults.state` (default `.kapi-state.json`),
  `defaults.locales.<lang>.tools`, `ship_gate`/`ship_gates`/`gates`,
  `source_gate`, `server:` — all present (`core/project/project.go`).

**Update (2026-07-18): epic 019 build phases are all merged to main
(#1311/#1312/#1317/#1319).** The items below were the "to build" gaps at review
time; they now describe **shipped** behaviour and are retained for history. What
actually shipped, verified against code:

- **The source gate now gates the server fan-out.** `defaults.source_gate`
  (`core/project` `Defaults.SourceGate`, a level string `checked` default /
  `approved` / `none` opt-out / `authored`) is enforced by server convergence:
  `bowrain/server/source_settle.go` settles the source (source-QA + readiness
  stamp; terminology/brand extensible) and `convergence_orchestrator.go` holds
  any below-gate block. **Nuance for the docs:** enforcement is **server-side**.
  The *local* CLI `kapi up` (`host/converge.go`) is still `recycle → translate`
  with no source-settle stage, and `computeSourceReadiness` (`host/coverage.go`)
  remains report-only there (feeds `kapi status` and `kapi check --ship`'s
  `verifySourceGate`, not the local fan-out). So kapi-OSS pages describe the
  source-hold conceptually + via `check --ship`; the *hold on push* is a Bowrain
  (server) behaviour.
- **Server convergence is TM-first with a truthful split.** `recycleBlocks`
  recycles before AI and `reconcileSplit` (`convergence_orchestrator.go`) makes
  `ViaTM + ViaAI = Done`, so the `TM N · AI M` report is truthful server-side.
- **Stall reasons** now include `source_not_ready` (`core/convergence/events.go`
  `StallSourceNotReady`); the run row carries a `blocked_on_source` count
  (`bowrain/store/convergence_run.go`). (`needs_source_review` was never added as
  a constant — the hold reason is `source_not_ready`; the automation it triggers
  is `create_source_review` → `TaskSourceReview`.)
- **Protected/DNT terms are enforced (mask-then-restore).**
  `core/ai/tools/translate.go` masks each DNT span with a sentinel before the
  model and restores it verbatim after (`dntMask`/`dntRestore`, "epic 019 item
  4"); the server worker wires it via `projectDNTTerms`
  (`bowrain/jobs/worker.go`, `Properties["dnt_terms"]`).
- **Estimate endpoint** `GET /projects/:id/convergence/estimate`
  (`bowrain/server/server.go`, `handlers_convergence_estimate.go`) returns source
  readiness first, then per-locale TM/AI split + credit estimate; the
  `ConvergenceRunNowDialog` (`bowrain/packages/ui`) is the source-readiness-first
  consent flow.

<details>
<summary>Original review-time gaps (pre-#1317, now closed)</summary>

- **The source gate does not gate the fan-out yet.** `SourceStatus` is
  **report-only**; `defaults.source_gate` is evaluated by
  `computeSourceReadiness` for `kapi status` **reporting only**
  (`host/coverage.go`), never by the derive/produce step. Convergence does not
  hold the fan-out on an under-ready source today.
- **Server convergence is not yet source-first, and its TM split is not
  truthful.** PR #1312 added a recycle stage server-side (`recycleBlocks`,
  `bowrain/jobs/worker.go`), but `produceFunc` still attributes produced units
  to AI (`ViaTM` unknown server-side, `convergence_orchestrator.go`). The
  source-review automation exists (`create_source_review`, title *"Review source
  content before translation"*, `bowrain/server/automation.go`) and the
  `EventSourceReviewCompleted` event exists, but convergence-on-push does not
  hold the fan-out on it.
- **Stall reasons** in `core/convergence/events.go` today are `needs_credits`,
  `needs_ai_key`, `rate_limited`, `no_progress`, `checks_failing`.
  `source_not_ready` / `needs_source_review` are **not** present yet (epic 019
  phase E).
- **Protected/DNT terms are prompted, not enforced.** The glossary is rendered
  into the translate prompt (`core/ai/prompt/translate.go`); `termcheck` is an
  annotation tool (`core/tools/termcheck.go`); DNT spans are not enforced in the
  AI path.

</details>

## Per-page review — kapi docs (`web/docs/**`)

The kapi site must not sell or mention Bowrain (standing decision). The loop is
a shared concept; the conceptual explainer belongs here, described without a
Bowrain funnel.

| Page | Status | Issue | Recommended action |
|---|---|---|---|
| `kapi/convergence.mdx` (The kapi loop) | Updated | Had both ladders and named `source_gate`, but framed the source as merely "the first half" — it did **not** show the source gate *holding* the fan-out, i.e. the source-first shape. | **Done in this PR:** added the [Source first](/kapi/convergence#source-first) section with `GatedLoopDiagram` (visual ship-gates + held branches). Kept at model level; no Bowrain funnel. |
| `kapi/convergence-in-ci.mdx` (The kapi loop in CI) | **Done** | Stated the ship gate "includes the source gates" but had no source-first *hold* framing. | **Done in the follow-up PR:** added a "Source first: settle before you fan out" note — `kapi check --ship` enforces `source_gate`, and the loop holds under it (`source_not_ready`). Bowrain-free; links to the model. |
| `kapi/recipes/gate-localization-in-ci.mdx` (Ship gates & CI) | Correct | `kapi check --ship`, exit-non-zero, `ship_gates:` all verified. Source gate mentioned in passing. | Cross-link the source-first section for the "why settle first" rationale. Low priority. |
| `kapi/recipes/keep-source-on-brand.mdx` (Keep source on brand) | Correct | The source-settle recipe (brand check/rewrite, source QA) — the phase-1 tools. Does not connect to the gate/hold. | Add a closing cross-link: settling source is *phase 1* of the loop → the source ship-gate. Low priority. |
| `kapi/recipes/translate-content.mdx` (Translate with your AI) | **Done** | No mention that the source must be settled first. | **Done in the follow-up PR:** added one line + link to source-first (source below `source_gate` holds `source_not_ready`, fixed once not once per locale). |
| `kapi/recipes/review-and-approve.mdx` (Review & approve) | Correct | Target-language review only (`kapi status --review`, `kapi apply kind:review`). | **Done (neokapi#1325):** CLI source-gate parity landed; the page documents the local source-settle loop (hold on `source_not_ready` → settle → re-run). |
| `kapi/recipes/pre-translate-with-tm.mdx` (Reuse translations) | Correct | TM recycle = phase-2 engine. Accurate. | No change. Optionally note recycle only ever runs on approved source. |
| `kapi/recipes/machine-ship-strategy.mdx` (Tier gates per market) | Correct | Per-market target tiers, approver classes. Verified against `gates:` registry. | No change. |
| `kapi/recipes/tm-termbase-storage.mdx` (Where translations & terms live) | Correct | `kind:tm` vs `kind:review`, `defaults.tm_source`/`state`. Accurate. | No change. |
| `kapi/project-store.mdx` (The project store) | Correct | `.kapi-state.json`, derived state vs decisions. Accurate. | No change. |
| `kapi/projects.mdx` (Projects) | Correct | Uses `PipelineDiagram`; loop mechanics accurate. | No change. |
| `kapi/direct-execution-layer.mdx` (CLI layers) | Correct | Direct / flow / convergence layers; `kapi up` vs `kapi run` vs `kapi exec`. Accurate. | No change. |
| `kapi/overview.mdx` (Overview) | Correct | "Keep your source right" then "every language caught up" — already source-first in spirit. | No change. |
| `reference/project-file.mdx` (Project file) | **Done** | Documented only the top-level `source_gate` coverage bar; `defaults.source_gate` (the convergence level) was missing. | **Done in the follow-up PR:** documented `defaults.source_gate` (`checked`/`approved`/`none`) as the source-first convergence gate, distinct from the coverage bar; added `defaults.flow`/`defaults.jobs` rows. |
| `contribute/architecture/033-project-state-model.md` (AD-033) | **Done** | Named the symmetric source ladder as (implicitly) report-only. | **Done in the follow-up PR:** flipped it — the source ladder gates the loop symmetrically with the target ladder (server convergence settles + gates before the fan-out); source rungs are load-bearing. |

**ASCII diagrams:** none found on the kapi docs. All flow/relationship visuals
already use the `@neokapi/docs-shared` diagram kit. No conversions needed.

## Per-page review — bowrain docs (`bowrain/web/docs/docs/**`)

The governance / venue framing correctly lives here. The source-first *gate as
governance* story is present in the ADs but **not surfaced** on the user-facing
loop pages.

| Page | Status | Issue | Recommended action |
|---|---|---|---|
| `getting-started/the-loop.mdx` (The loop on Bowrain) | **Done** | Produce → promote → release was right, but there was **no source-first phase**. | **Done in the follow-up PR:** added the "Source first" section with `GatedLoopDiagram` — settle → source gate (hold → source review) → translate approved source → target gate. |
| `cli/commands/up.mdx` (`kapi up`) | Mostly correct | Flags and venue accurate. `--timeout` (15m server default) documented — verify default in code. Shows push → catch up → stream → pull; no source hold. | Add the source-hold outcome and `source_not_ready` once wired. Verify `--timeout` default. |
| `server/review.mdx` (Review) | **Done** | Target review only (draft → translated → reviewed); no **source review** surface. | **Done in the follow-up PR:** added the source-review worklist (phase-1 gate) as a sibling to target review, tied to the `source_review` task queue and the `source_not_ready` hold. |
| `server/automation.md` (Automation) | Correct | `server.converge` (on-push default / manual / schedule); quality gates. Accurate. | Note the `fan-out-after-source-review` default rule once source-first is on by default. |
| `architecture-decisions/022-convergence-as-a-service.md` (AD-022) | **Done** | Predated source-first: no source-settle phase, no `source_not_ready` stall. | **Done in the follow-up PR:** added decision *1a* (settle → gate → translate approved source), `source_not_ready` + `blocked_on_source` on the run entity, and the estimate endpoint. |
| `architecture-decisions/014-translator-workflow.md` (AD-014) | **Done** | Documented the source-review gate as an *optional*/bypassed automation option. | **Done in the follow-up PR:** reconciled with AD-022 — the source gate is convergence-enforced (one story); `TaskSourceReview` is the human half of the `source_not_ready` hold. |
| `architecture-decisions/013-automation-engine.md` (AD-013) | **Done** | `fan-out-after-source-review` default rule; defers to AD-014. | **Done in the follow-up PR:** reframed the rule as "resume a held run"; the on-push note now describes the source-first hold. |
| `architecture-decisions/015-server-ai-operations.md` (AD-015) | Correct | Translation jobs, extraction, brand voice. | **Done (neokapi#1323):** the produce/worker description now includes the TM-first recycle step and truthful `ViaTM`/`ViaAI` split. |
| `notes/translator-workflow.md` | Accurate | Implementation detail for AD-014 (events, tasks, tracker). | No change; update alongside AD-022 reconciliation. |
| `notes/translation-job-queue.md` | Correct | Job model, quotas, worker algorithm. | **Done (neokapi#1323):** the TM-first split section reflects `recycleBlocks` + `reconcileSplit`. |

**ASCII diagrams:** none found on the bowrain docs either — flow visuals use the
diagram kit (`PhaseFlow`, `LanesDiagram`, `PipelineDiagram`, `SwimlaneDiagram`).
No conversions needed.

## Biggest inaccuracies / gaps (prioritised)

> **Resolved (2026-07-18, follow-up PR):** items 1 and 3 are done — the bowrain
> venue framing (the-loop.mdx) and the source-review worklist (review.mdx) now
> exist, and the AD/reference source-gate story is reconciled. Item 2 (server
> TM-split) is now truthful in code (`reconcileSplit`) — the remaining doc caveat
> is a deferred AD-015 / job-queue note. Item 4 is closed: `source_not_ready`
> exists in `core/convergence/events.go`. Retained below for history.

1. **The source-first shape was invisible.** Both sites had the source *ladder*
   and the `source_gate` *key*, but neither showed the source ship-gate
   **holding** the fan-out — the whole point of source-first. Fixed on the kapi
   site in this PR (the explainer + `GatedLoopDiagram`); the bowrain venue
   framing is a follow-up gated on phase E.
2. **Server TM-first is over-claimed.** Docs imply a truthful `TM N · AI M`
   server-side; `produceFunc` still attributes to AI (`ViaTM` unknown). Soften
   until #1312's split is real, or scope the truthful-split claim to the CLI.
3. **Source-review as a first-class surface is undocumented on the user side.**
   The machinery exists (`create_source_review`, `TaskSourceReview`) but no
   user-facing page shows a source-review worklist. Add once the surface ships.
4. **Stall vocabulary is incomplete.** No page should promise `source_not_ready`
   / `needs_source_review` until they exist in `core/convergence/events.go`.

## Follow-ups {#follow-ups}

Epic 019 phase E (source-first) landed (#1317), so the source gate now holds the
server fan-out. The documentation follow-ups below are **done** except the
walkthrough-video re-records, which remain open.

**Done (docs: source-first loop follow-ups PR):**

- ✅ **`bowrain/getting-started/the-loop.mdx`** — added the "Source first"
  section (the source-gate / governed-review framing the kapi site omits),
  reusing `GatedLoopDiagram`: settle source → source gate (hold → source
  review) → translate approved source TM-first → target ship-gate.
- ✅ **`bowrain/.../022-convergence-as-a-service.md`** — added decision *1a.
  Settle the source, then translate*: the source-settle pass, the
  `source_not_ready` hold, `blocked_on_source`, and the estimate endpoint;
  reconciled with **AD-014** (the source gate is convergence-enforced, not a
  bypassable automation option) and **AD-013** (`fan-out-after-source-review`
  resumes a held run). One source-gate story across AD-022/014/013.
- ✅ **`bowrain/server/review.mdx`** — documented the source-review worklist as
  the phase-1 counterpart to target review, tied to the existing `source_review`
  task queue (no new UI invented).
- ✅ **`kapi/convergence-in-ci.mdx` + `kapi/recipes/translate-content.mdx`** —
  added the source-hold outcome, Bowrain-free, cross-linking the source-first
  section. (kapi-OSS framing keeps the *hold on push* conceptual; the local CLI
  `kapi up` does not yet source-hold — that is a Bowrain/server behaviour.)
- ✅ **AD-033 + `reference/project-file.mdx`** — flipped the "source status is
  report-only" framing: AD-033 now states the source ladder gates the loop
  symmetrically with the target ladder; the reference documents
  `defaults.source_gate` (the convergence level `checked`/`approved`/`none`) as
  distinct from the top-level `source_gate` coverage bar.

**Resolved (2026-07-18):**

- ✅ **`kapi/recipes/review-and-approve.mdx`** — CLI source-gate parity shipped
  (neokapi#1325): local `kapi up` now settles + holds on un-ready source
  (`source_not_ready`) via the shared `check.SettleSourceStatus`, default
  `checked` (owner decision), `none` to draft freely. The page documents the
  local source-settle loop (hold → `kapi check --ship` / fix terms·brand·source
  → `kapi apply` → re-run).
- ✅ **AD-015 / `notes/translation-job-queue.md`** — TM-split caveats dropped
  (neokapi#1323): both now describe the truthful server `TM N · AI M`
  (`reconcileSplit` + the job-pipeline recycle step).

**Open — walkthrough videos to re-record** (out of scope for the docs PR; per
CLAUDE.md, CLI/UI surface changes):

- the `kapi up` demo (once it prints the source-readiness summary and can hold on
  source);
- the review-and-approve demo (if a source-review worklist is added CLI-side);
- any bowrain Runs-view demo that would newly show a `settle-source` loop
  position or a `source_not_ready` stall / *Run now* consent dialog.
