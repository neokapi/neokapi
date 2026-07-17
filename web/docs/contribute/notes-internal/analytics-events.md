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
pass CI.

## Invariants

- **Keyless-silent**: a deployment with no PostHog key configured emits
  nothing. Captures are fire-and-forget and can never block or fail a user
  operation.
- **No content**: events never carry document content, file paths, source
  text, or command arguments. Sizes are bucketed
  (`analytics.CountBucket`), timings are bucketed
  (`analytics.DurationBucket`).
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

## MCP surface

| Event | Fired when | Properties |
|---|---|---|
| `mcp_session_start` | MCP `initialize` succeeds | `transport`, `workspace_id` (when known) |
| `mcp_tool_call` | MCP `tools/call` succeeds | `tool_name`, `workspace_id` / `project_id` (when present in arguments) |
| `mcp_resource_read` | MCP `resources/read` succeeds | `resource_uri`, `workspace_id` (when known) |

## Billing funnel

The distinct ID for webhook-driven events is the workspace ID (the epic-007
anonymous-to-identified join happens on the workspace group). These names are
normalized from the earlier `billing.*` prefixed events
(`billing.checkout_started`, `billing.checkout_completed`,
`billing.feature_gate_hit`, `billing.credits_exhausted`); the old names are
retired rather than dual-fired.

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

## Adding an event

1. Add the constant to `bowrain/analytics/events.go` (snake_case
   `domain_action`).
2. Capture it at the service seam (preferred) or, where no service layer
   exists, at the handler success point — always fire-and-forget, after the
   operation succeeded, never carrying content.
3. Document it in this file (the `events_test.go` drift gate enforces this).
