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
source-first convergence. "kapi drafts, Bowrain governs" — the ship-gate is the
governance seam.

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

Gaps that matter for the docs (these are the epic-019 *to build* items — the
model is documented ahead of the wiring, which is fine for a concepts page but
must not be stated as *shipped behaviour*):

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

## Per-page review — kapi docs (`web/docs/**`)

The kapi site must not sell or mention Bowrain (standing decision). The loop is
a shared concept; the conceptual explainer belongs here, described without a
Bowrain funnel.

| Page | Status | Issue | Recommended action |
|---|---|---|---|
| `kapi/convergence.mdx` (The kapi loop) | Updated | Had both ladders and named `source_gate`, but framed the source as merely "the first half" — it did **not** show the source gate *holding* the fan-out, i.e. the source-first shape. | **Done in this PR:** added the [Source first](/kapi/convergence#source-first) section with `GatedLoopDiagram` (visual ship-gates + held branches). Kept at model level; no Bowrain funnel. |
| `kapi/convergence-in-ci.mdx` (The kapi loop in CI) | Mostly correct | States the ship gate "includes the source gates (brand, terminology)". True as a *model* claim, but `--ship` source-gate enforcement rides `computeSourceReadiness`; fine today. No source-first *hold* framing. | When phase E lands: add a short note that a pre-merge `kapi up` holds on an unsettled source (`source_not_ready`). Link to the new section. |
| `kapi/recipes/gate-localization-in-ci.mdx` (Ship gates & CI) | Correct | `kapi check --ship`, exit-non-zero, `ship_gates:` all verified. Source gate mentioned in passing. | Cross-link the source-first section for the "why settle first" rationale. Low priority. |
| `kapi/recipes/keep-source-on-brand.mdx` (Keep source on brand) | Correct | The source-settle recipe (brand check/rewrite, source QA) — the phase-1 tools. Does not connect to the gate/hold. | Add a closing cross-link: settling source is *phase 1* of the loop → the source ship-gate. Low priority. |
| `kapi/recipes/translate-content.mdx` (Translate with your AI) | Correct | `kapi up` catches locales up to the ship gate; TM-first. No mention that the source must be settled first. | Add one line + link to source-first once the gate holds the fan-out. |
| `kapi/recipes/review-and-approve.mdx` (Review & approve) | Correct | Target-language review only (`kapi status --review`, `kapi apply kind:review`). | When source review lands CLI-side, add the source-review worklist as a sibling. Follow-up. |
| `kapi/recipes/pre-translate-with-tm.mdx` (Reuse translations) | Correct | TM recycle = phase-2 engine. Accurate. | No change. Optionally note recycle only ever runs on approved source. |
| `kapi/recipes/machine-ship-strategy.mdx` (Tier gates per market) | Correct | Per-market target tiers, approver classes. Verified against `gates:` registry. | No change. |
| `kapi/recipes/tm-termbase-storage.mdx` (Where translations & terms live) | Correct | `kind:tm` vs `kind:review`, `defaults.tm_source`/`state`. Accurate. | No change. |
| `kapi/project-store.mdx` (The project store) | Correct | `.kapi-state.json`, derived state vs decisions. Accurate. | No change. |
| `kapi/projects.mdx` (Projects) | Correct | Uses `PipelineDiagram`; loop mechanics accurate. | No change. |
| `kapi/direct-execution-layer.mdx` (CLI layers) | Correct | Direct / flow / convergence layers; `kapi up` vs `kapi run` vs `kapi exec`. Accurate. | No change. |
| `kapi/overview.mdx` (Overview) | Correct | "Keep your source right" then "every language caught up" — already source-first in spirit. | No change. |
| `reference/project-file.mdx` (Project file) | Correct | `ship_gate(s)`, `gates`, `source_gate`, `defaults.*` documented. Verified against `core/project/project.go`. | Confirm `source_gate` schema text once enforcement lands. |
| `contribute/architecture/033-project-state-model.md` (AD-033) | Correct | Names the symmetric source ladder. Report-only source status matches code. | Update when `SourceStatus` becomes gating (no longer report-only). |

**ASCII diagrams:** none found on the kapi docs. All flow/relationship visuals
already use the `@neokapi/docs-shared` diagram kit. No conversions needed.

## Per-page review — bowrain docs (`bowrain/web/docs/docs/**`)

The governance / venue framing correctly lives here. The source-first *gate as
governance* story is present in the ADs but **not surfaced** on the user-facing
loop pages.

| Page | Status | Issue | Recommended action |
|---|---|---|---|
| `getting-started/the-loop.mdx` (The loop on Bowrain) | Stale-ish | Produce → promote → release is right, but there is **no source-first phase**: the loop is shown fanning out on push with no source ship-gate. | When phase E lands: add the source ship-gate as the first governance seam (the venue framing that the kapi site can't carry). Reuse `GatedLoopDiagram`. |
| `cli/commands/up.mdx` (`kapi up`) | Mostly correct | Flags and venue accurate. `--timeout` (15m server default) documented — verify default in code. Shows push → catch up → stream → pull; no source hold. | Add the source-hold outcome and `source_not_ready` once wired. Verify `--timeout` default. |
| `server/review.mdx` (Review) | Correct but incomplete | Target review (draft → translated → reviewed). No **source review** surface, though `TaskSourceReview` / `create_source_review` exist in code. | Add the source-review queue (phase-1 gate) as a sibling to target review. Follow-up (needs the surface). |
| `server/automation.md` (Automation) | Correct | `server.converge` (on-push default / manual / schedule); quality gates. Accurate. | Note the `fan-out-after-source-review` default rule once source-first is on by default. |
| `architecture-decisions/022-convergence-as-a-service.md` (AD-022) | Stale on source-first | Describes convergence-as-service and `convergence_runs` but predates source-first: no source-settle phase, no `source_not_ready` stall. | Update to add the source phase + source ship-gate as the first pass, and the new stall reasons, when phase E merges. |
| `architecture-decisions/014-translator-workflow.md` (AD-014) | Accurate (ahead of wiring) | Documents the source-review gate (`create_source_review`, `EventSourceReviewCompleted`, `fan-out-after-source-review`). Matches code that exists but is **bypassed** by convergence-on-push. | Reconcile with AD-022 so the source gate is one story, not two, when it becomes gating. |
| `architecture-decisions/013-automation-engine.md` (AD-013) | Accurate | `fan-out-after-source-review` default rule; defers to AD-014. | No change; keep in sync with AD-022/014. |
| `architecture-decisions/015-server-ai-operations.md` (AD-015) | Correct | Translation jobs, extraction, brand voice. TM split not yet truthful server-side (see baseline). | Update the "produce" description once `ViaTM` is real server-side. |
| `notes/translator-workflow.md` | Accurate | Implementation detail for AD-014 (events, tasks, tracker). | No change; update alongside AD-022 reconciliation. |
| `notes/translation-job-queue.md` | Correct | Job model, quotas, worker algorithm. | Update the TM-split note once server `ViaTM` is real. |

**ASCII diagrams:** none found on the bowrain docs either — flow visuals use the
diagram kit (`PhaseFlow`, `LanesDiagram`, `PipelineDiagram`, `SwimlaneDiagram`).
No conversions needed.

## Biggest inaccuracies / gaps (prioritised)

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

Fuller rewrites, to do when epic 019 phase E (source-first) lands and the source
gate actually holds the fan-out:

- **`bowrain/getting-started/the-loop.mdx`** — add the source ship-gate as the
  first governance seam; this is where the "kapi drafts, Bowrain governs" venue
  framing belongs (the kapi site can only describe it conceptually). Reuse
  `GatedLoopDiagram`.
- **`bowrain/.../022-convergence-as-a-service.md`** — add the source-settle pass
  + the `source_not_ready` stall; reconcile with AD-014/AD-013 so the source
  gate is one story.
- **`bowrain/server/review.mdx`** — document the source-review worklist as the
  phase-1 counterpart to target review (needs the surface).
- **`kapi/convergence-in-ci.mdx` + `kapi/recipes/translate-content.mdx`** — add
  the source-hold outcome and cross-link the source-first section.
- **`kapi/recipes/review-and-approve.mdx`** — add source review as a sibling
  worklist once it exists CLI-side.
- **AD-033 + `reference/project-file.mdx`** — flip the "source status is
  report-only" language when `SourceStatus` becomes gating.
- **Walkthrough videos to re-record** (per CLAUDE.md — CLI/UI surface changes):
  the `kapi up` demo (once it prints the source-readiness summary and can hold
  on source), the review-and-approve demo (if a source-review worklist is
  added), and any bowrain Runs-view demo that would newly show a `settle-source`
  loop position or a `source_not_ready` stall.
