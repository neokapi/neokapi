---
title: Analytics event reference
description: The taxonomy of product-analytics events emitted to PostHog, their properties, and the drift gate that keeps this reference in sync with the code.
---

# Analytics event reference

This is the reference for every product-analytics event the platform emits to
PostHog (roadmap epic 018). Event names are `snake_case` `domain_action`. The
server-side constants live in `bowrain/analytics/events.go`; a drift test
(`bowrain/analytics/events_test.go`) fails when a constant defined there is
missing from this document, so adding an event without documenting it does not
pass CI. The client-side (web app) constants live in
`bowrain/packages/ui/src/analytics-events.ts` with the same gate
(`bowrain/packages/ui/src/__tests__/analytics-events.test.ts`).

## Invariants

- **Keyless-silent**: a deployment with no PostHog key configured emits
  nothing. Captures are fire-and-forget and can never block or fail a user
  operation.
- **No content**: events never carry document content, file paths, source
  text, or command arguments. Sizes are bucketed
  (`analytics.CountBucket`), timings are bucketed
  (`analytics.DurationBucket`), credit amounts are bucketed
  (`analytics.CreditBucket`), and shares are banded
  (`analytics.PercentBucket` / `SharePercentBucket`). An exact credit amount,
  token count, or unit count never leaves the process. The client mirrors the
  same tables in `bowrain/packages/ui/src/analytics-buckets.ts`, label for
  label, so a property is comparable across the `server` and `web-app`
  surfaces.
- **EU ingestion**: the default host is `https://eu.i.posthog.com`
  (`analytics.DefaultHost`).

## Mandatory properties (server surface)

Every event captured through `analytics.PostHogClient.CaptureEvent`
automatically carries:

| Property | Value |
|---|---|
| `surface` | `"server"` |
| `app_version` | the server build version |

When a `workspace_id` property is present, the event is also associated with
the PostHog **workspace** group (`$groups: {workspace: <id>}`).

## Identity and workspace lifecycle

| Event | Fired when | Properties |
|---|---|---|
| `user_signup` | first OIDC login creates the user | `email` |
| `user_login` | returning OIDC login | `email` |
| `workspace_created` | `AuthService.CreateWorkspaceWithOwner` succeeds (covers onboarding and explicit creation) | `workspace_id`, `workspace_type` |
| `project_created` | project create handler succeeds | `project_id`, `workspace_id`, `project_name`, `source_language`, `target_count`, `workspace_slug` |
| `project_claimed` | `AuthService.ClaimProject` moves an anonymous project into a personal workspace | `workspace_id`, `project_id` |
| `project_deleted` | project delete handler succeeds | `project_id`, `workspace_id` (when known) |
| `member_invited` | `AuthService.CreateInvite` persists an invite | `workspace_id`, `role` |
| `member_joined` | `AuthService.AcceptInvite` adds the member | `workspace_id`, `role` |

## Domain events

| Event | Fired when | Properties |
|---|---|---|
| `flow_run_completed` | a flow execution finishes — `service.FlowService.ExecuteFlow` and the MCP `run_flow` tool | `flow`, `duration_bucket`, `outcome` (`completed` / `failed` / `persist_failed`), `part_count`, `project_id` |
| `content_pushed` | the sync worker finishes processing a push (`processSyncPushJob`) | `project_id`, `item_count`, `block_count_bucket`, `workspace_slug` |
| `content_pulled` | a sync pull returns changed content (no-change polls stay silent) | `project_id`, `workspace_id` (when known), `block_count_bucket`, `has_more` |
| `review_approved` | a review decision (single or batch) persists with `approve` | `batch_size`, `locale` (single decisions), `workspace_id` (when known) |
| `review_rejected` | a review decision (single or batch) persists with `reject` | `batch_size`, `locale` (single decisions), `workspace_id` (when known) |
| `connector_published` | `service.ConnectorService.Publish` resolves | `workspace_id`, `project_id`, `connector_type`, `outcome` (`completed` / `failed`), `block_count_bucket` |
| `convergence_estimate_computed` | the server computes a pre-flight estimate (`GET …/convergence/estimate`), whichever surface asked — the web run-now dialog, `kapi up`'s confirm, or any API client | `workspace_id` (when known), `project_id`, `source_held` (bool), `estimated_credits_bucket` (credit bucket), `ai_units_bucket` (count bucket), `tm_leverage_pct_bucket` (percent band), `covers_all_ai` (bool — omitted where billing is unconfigured), `first_run` (bool) |
| `convergence_run_started` | a server convergence run starts (`convergenceOrchestrator.StartRun`) | `workspace_id` (when known), `project_id`, `trigger` (`cli` / `push` / `manual`), `first_run` (bool) |
| `convergence_run_completed` | a server convergence run reaches a terminal state (`convergenceOrchestrator.driveWith`) | `workspace_id` (when known), `project_id`, `outcome` (`converged` / `parked` / `failed` / `canceled`), `stall_reason` (`needs_credits` / `needs_ai_key` / `rate_limited` / `no_progress` / `checks_failing` / `source_not_ready`, empty on converge), `passes`, `via_tm` / `via_ai` (count buckets), `blocked_on_source` (count bucket — source blocks held below the source-first gate), `duration_bucket`, `consumed_credits_bucket` (credit bucket — what the run ACTUALLY spent), `tm_leverage_pct_bucket` (percent band), `first_run` (bool) |

### Sizing the new-workspace grant

Every new workspace gets one non-recurring grant of
`billing.FreeTrialGrantCredits` credits, and Free carries no recurring
allowance — so that grant is the entire budget a first project has. The
properties above exist to answer the sizing question the coverage flag alone
cannot: **how large must the grant be to cover N% of first projects?** Three
pieces make that answerable:

- **Magnitude, not a yes/no.** `covers_all_ai` says whether the balance was
  enough; `estimated_credits_bucket` says *how much was asked for*, so the
  distribution of first-project demand can be read against a candidate grant
  size. `ai_units_bucket` and `tm_leverage_pct_bucket` explain the shape of
  that demand — a corpus that recycles well costs nothing to converge, and
  content-memory leverage is the strongest lever on how far a fixed grant
  stretches.
- **The cold-start cohort.** `first_run` is true when no convergence run
  precedes this one anywhere in the workspace — the workspace's very first
  project run. It is derived from a bounded existence probe over the
  workspace's projects in the run store (`ConvergenceRunStore.HasRunBefore`),
  not a stored column, and it is workspace-scoped on purpose: a second
  project's first run is no longer a cold start. Filtering to `first_run: true`
  is what turns fleet-wide spend into the grant-sizing cohort.
- **Demand vs. realized spend.** `estimated_credits_bucket` (on the estimate
  events) is uncensored demand — what a run *would* cost, computed before any
  credit is spent. `consumed_credits_bucket` (on `convergence_run_completed`)
  is what a run actually spent, from the token usage its translation jobs
  reported; it is *censored by the balance*, because a run that exhausts the
  grant parks with `stall_reason: needs_credits`. Size the grant on demand and
  validate it against realized spend; reading realized spend alone would
  measure the current grant rather than the need. Two caveats on realized
  spend: a workspace using its own AI key burns tokens but no credits, and a
  run driven by a replica that restarted mid-flight reports only what it saw.

Because the estimate is computed server-side for every pre-flight, the CLI
(`kapi up`) and the web dialog land in the same population; the mandatory
`surface` property separates them. `convergence_estimate_viewed` remains the
web-only impression event (a human actually saw the estimate) and is not a
substitute for it.

All of these are bucketed for privacy: the events carry a band label, never an
exact credit amount, token count, unit count, locale list, or anything derived
from content. The credit bands are `0`, `1_1k`, `1k_10k`, `10k_50k`,
`50k_100k`, `100k_200k`, `200k_500k`, `500k_1m`, `gt_1m` — the upper edge of
`100k_200k` is exactly the grant (and the top-up pack), so "fits inside the
grant" is a boundary read rather than an interpolation. Percent bands are `0`,
`1_25`, `26_50`, `51_75`, `76_99`, `100`, with the saturated bands reserved for
exactly none and exactly all.

## MCP surface

| Event | Fired when | Properties |
|---|---|---|
| `mcp_session_start` | MCP `initialize` succeeds | `transport`, `workspace_id` (when known) |
| `mcp_tool_call` | MCP `tools/call` succeeds | `tool_name`, `workspace_id` / `project_id` (when present in arguments) |
| `mcp_resource_read` | MCP `resources/read` succeeds | `resource_uri`, `workspace_id` (when known) |

## Billing funnel

The distinct ID for webhook-driven events is the workspace ID (the epic-007
anonymous-to-identified join happens on the workspace group). The names are
unprefixed, and each event is fired under exactly one name.

| Event | Fired when | Properties |
|---|---|---|
| `checkout_started` | checkout session created for a workspace | `workspace_id`, `plan`, `seats` |
| `checkout_completed` | Stripe `checkout.session.completed` webhook | `workspace_id`, `customer_id`, `plan`, `seats` (subscriptions) or `type: credit_pack` |
| `trial_started` | workspace creation sets up the local card-free trial (not a Stripe webhook — no Stripe object exists for the local trial) | `workspace_id`, `plan`, `trial_days` |
| `trial_converted` | Stripe `checkout.session.completed` while the workspace subscription was `trialing` | `workspace_id`, `plan` |
| `payment_failed` | Stripe `invoice.payment_failed` webhook | `workspace_id` |
| `subscription_cancelled` | Stripe `customer.subscription.deleted` webhook | `workspace_id`, `plan` |
| `feature_gate_hit` | a plan guard blocks a request needing a paid feature | `workspace_id`, `feature`, `plan`, `minimum_plan` |
| `credits_exhausted` | the quota guard blocks a request with no credits left | `workspace_id`, `plan` |

## Web app (client)

Client-side events from the browser SPA (`bowrain/apps/web` +
`bowrain/packages/app` + `@neokapi/ui`), epic-018 workstream B. They flow
through the platform seam (`platform.analytics.capture`, wired to `posthog-js`
in `bowrain/apps/web/src/posthog.ts`; the desktop shell wires the same seam
gated on its telemetry setting — see Client surfaces) and the shared-component capture
seam (`useAnalytics()` in `@neokapi/ui`). Event names are defined once in
`bowrain/packages/ui/src/analytics-events.ts`.

Super-properties registered at init and attached to **every** client event
(including `$pageview` and autocapture):

| Property | Value |
|---|---|
| `surface` | `"web-app"` |
| `environment` | the Vite build mode (`production` / `development`) |

Client events carry ids, locales, and enum-ish values only — never free-text
names, document content, or file paths. Group association: on workspace
switch/load the web shell calls `posthog.group("workspace", workspaceId)`
(platform seam `analytics.group`), scoping subsequent events to the active
workspace group.

### Navigation

| Event | Fired when | Properties |
|---|---|---|
| `$pageview` | every TanStack Router navigation (including the initial load — `capture_pageview` is off; the router subscription in `BowrainApp` owns all pageviews) | `path_pattern` — the matched route PATTERN with params unresolved (e.g. `/$workspace/p/$projectId/s/$stream/$itemId/translate`), so slugs/ids never appear |
| `feature_entered` | the route-derived feature changes on navigation (deduped against the previous feature) | `feature` — snake_case surface name derived from the route pattern (`dashboard`, `translate`, `review`, `brand_concepts`, `settings_members`, `locale_demand`, …) |

### Form / action events

| Event | Fired when | Properties |
|---|---|---|
| `project_create_submitted` | the create-project (or sample-project) call succeeds on the dashboard — client complement of the server's `project_created` | `source_language`, `target_count`, `sample` (sample project only) |
| `connector_added` | the add-connector dialog's create call succeeds | `connector_type` (`wordpress` / `figma` / `hubspot`) |
| `connector_publish_clicked` | the publish confirmation is confirmed (before the server round-trip; the server's `connector_published` reports the outcome) | `connector_category` |
| `review_decision_clicked` | a per-block Approve/Reject (translate editor or review surface) or the bulk mark-reviewed action — client complement of the server's `review_approved` / `review_rejected` | `decision` (`approve` / `reject` / `clear`), `locale`, `bulk` (bulk action only) |
| `translation_saved` | a block target save persists (editor save or memory-match apply) | `locale`, `method` (`editor` / `tm`) |
| `settings_saved` | a workspace settings section persists a change (general/pulse visibility, governance SoD mode, role overrides) | `section` (`general` / `governance`) |
| `member_invite_sent` | an invite is created in the members settings | `role` |
| `locale_added` | a language is added in the workspace language settings | `locale` |
| `brand_voice_saved` | a brand-voice profile create/update persists | `mode` (`created` / `updated`) |
| `glossary_saved` | a concept term status change persists in the Brand · Concepts edit dialog | `status`, `locale` |
| `locale_demand_connect_clicked` | the "Connect PostHog" / "Fix connection" affordance is clicked on the locale-demand page (AD-018 demand path) | `reconnect` |
| `convergence_estimate_viewed` | the run-now consent dialog opens and the source-readiness-first pre-flight estimate is shown, before any run starts (epic 019) — the web-only impression complement of the server's `convergence_estimate_computed` | `source_held` (bool — any source blocks held on the gate), `covers_all_ai` (bool — balance covers the AI remainder), `estimated_credits_bucket` (credit bucket — what the AI remainder would cost), `ai_units_bucket` (count bucket — units left for paid AI after content-memory reuse), `tm_leverage_pct_bucket` (percent band — the content memory's share of the pending work). The three buckets size the new-workspace grant — see [Sizing the new-workspace grant](#sizing-the-new-workspace-grant) |
| `convergence_run_started` | a convergence run is started from the run-now consent dialog after the user picks a scope and confirms (epic 019) — the web/client complement of the server's same-named event, distinguished by `surface` | `scope` (`all` / `ready-only` / `none`), `source_held` (bool) |
| `github_setup_installation_missing` | the GitHub App setup page (`/github/setup`) loads from a GitHub redirect (`setup_action` present) but without an `installation_id` — the post-install/update handoff lost the id and the user sees the recovery card instead of the repo list | `setup_action` (`install` / `update`) |

## Client surfaces

Client-side captures go through `posthog-js` and are key-gated the same way
as the server: a build without `VITE_POSTHOG_KEY` emits nothing. Each surface
registers its name as a super-property on every event:

| Surface | App | Consent class | Behavior |
|---|---|---|---|
| `kapi-docs`, `bowrain-docs`, `landing` | docs sites + landing (`@neokapi/docs-shared` cookieless init) | docs/marketing | explicit `$pageview` per route; memory persistence, DNT respected, no cookies/storage |
| `ctrl` | admin panel (`bowrain/apps/ctrl/src/analytics.ts`) | cloud app | route-pattern `$pageview` + the admin actions below; no PII beyond ids |
| `pulse` | real-time dashboard (`bowrain/apps/pulse/src/analytics.ts`) | cloud app | route-pattern `$pageview` only |
| `keycloak` | auth theme (`bowrain/apps/keycloak-theme`, cookieless `@neokapi/docs-shared` init) | cloud app, cookieless | the two page-view events below only; auth *outcome* events fire post-auth in the app, keeping the theme dumb |
| `bowrain-desktop` | Bowrain desktop shell (`bowrain/apps/bowrain/frontend/src/analytics.ts`, wired through the platform seam) | local client (opt-out, D1) | route-pattern `$pageview`; identifies by user id only; URL default properties scrubbed |
| `kapi-desktop` | Kapi Desktop (`apps/kapi-desktop/frontend/src/analytics.ts`) | local client (opt-out, D1) | `app_opened`, panel `$pageview`, flow-run events below; URL default properties scrubbed |

Local clients (both desktops) follow decision D1: telemetry defaults ON with a
one-time first-run notice (OK / Disable), a persistent settings toggle, Do Not
Track honored, and keyless builds silent. They never carry file paths, project
names, or content — pageviews report the matched route pattern or a static
view id only, and the URL-bearing default properties (`$current_url`,
`$pathname`, referrers, …) are stripped from every event.

### Pageviews

| Event | Fired when | Properties |
|---|---|---|
| `$pageview` | route/panel change on ctrl, pulse, both desktops, and the docs sites | `route` (matched route pattern or static view id; docs sites attach the URL instead) |

### Ctrl admin actions

| Event | Fired when | Properties |
|---|---|---|
| `admin_plan_changed` | a workspace's plan is updated (ChangePlanDialog) | `workspace_id`, `plan` |
| `admin_feature_override_set` | a feature override is applied, bypassing the plan matrix — the L8 OSS-grant flow (FeatureOverrideDialog) | `workspace_id`, `feature`, `enabled` |
| `admin_credits_granted` | credits are granted to a workspace (GrantCreditsDialog) | `workspace_id`, `amount` |
| `admin_member_added` | a user is added to a workspace (AddMemberDialog) | `workspace_id`, `user_id`, `role` |
| `admin_workspace_impersonated` | an admin impersonates a workspace | `workspace_id` |

### Keycloak theme

| Event | Fired when | Properties |
|---|---|---|
| `login_page_viewed` | a login template renders (`login.ftl`, `login-username.ftl`) | `template` |
| `register_page_viewed` | the register template renders (`register.ftl`) | `template` |

### Kapi Desktop

| Event | Fired when | Properties |
|---|---|---|
| `app_opened` | app start with telemetry enabled | — |
| `flow_run_started` | a flow run is triggered (JobFeed `startJob` — covers the runner and convergence) | `flow`, `file_count` |
| `flow_run_completed` | the run's terminal event arrives (also emitted server-side; the `surface` property discriminates the local variant) | `flow`, `outcome` (`completed` / `failed` / `canceled`), `duration_bucket` |

## Adding an event

1. Add the constant to `bowrain/analytics/events.go` (server events) or
   `bowrain/packages/ui/src/analytics-events.ts` (web app client events) —
   snake_case `domain_action`.
2. Capture it at the service seam (preferred) or, where no service layer
   exists, at the handler success point — always fire-and-forget, after the
   operation succeeded, never carrying content. Web-app client events go
   through the platform/`useAnalytics` capture seam, never a direct
   `posthog-js` import.
3. Document it in this file. Server constants are gated by `events_test.go`
   and web-app client names by `analytics-events.test.ts`; client events on
   the other surfaces (ctrl, pulse, keycloak theme, desktops, docs) have no
   automated gate — document them in the client-surfaces section in the same
   change.
