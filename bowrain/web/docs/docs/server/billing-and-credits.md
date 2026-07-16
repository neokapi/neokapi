---
title: Billing and credits
description: Bowrain's plans, per-seat pricing, weekly AI credits, top-up packs, trials, and how to manage a subscription — what each plan includes and what happens when credits run out or a payment fails.
sidebar_position: 13
---

# Billing and credits

Bowrain bills a workspace on one of four plans. A plan sets three things: the
features the workspace can use, its numeric limits (projects and seats), and its
weekly allowance of AI credits. Billing is managed by the workspace owner from
**Workspace Settings > Billing**.

## Plans

| | **Free** | **Pro** | **Team** | **Enterprise** |
| --- | --- | --- | --- | --- |
| Price | $0 | $25 / month | $20 / seat / month | Custom |
| Weekly AI credits | 50K | 500K | 2M | Custom (unlimited) |
| Projects | 1 | 10 | Unlimited | Unlimited |
| Seats | 1 | 3 | Unlimited | Unlimited |
| Git connectors | — | Yes | Yes | Yes |
| Custom connectors | — | — | Yes | Yes |
| API access | — | Yes | Yes | Yes |
| SSO / SAML | — | — | — | Yes |
| Bring your own AI key | Yes | Yes | Yes | Yes |

Free is free forever. Pro and Team are self-serve — a workspace owner subscribes
to them from the app. Enterprise is arranged directly and adds SSO/SAML,
dedicated support, custom agreements, and SLA terms.

Every plan can [bring its own AI provider key](#bring-your-own-ai-key), including
Free.

## AI credits

AI work in Bowrain is metered in credits. One credit equals one AI token,
counting both the tokens sent to the model and the tokens it returns.

Each plan comes with a weekly credit allowance. The balance resets every Monday
at 00:00 UTC to the plan's full allowance — 50K on Free, 500K on Pro, 2M on
Team. Enterprise credits are unlimited. Unused weekly credits do not roll over;
they reset each Monday, which keeps costs predictable.

### What consumes credits

Credits are spent by the AI operations Bowrain runs on your behalf:

- **AI translation** — translating content, whether run as a batch flow or
  requested inline in the translation editor.
- **Brand scans** — analyzing content to build or update a brand-voice profile.

Editing, reviewing, running non-AI checks, and reading translation memory or
terminology do not consume credits. Only the operations that call an AI model
draw down the balance.

### Bring your own AI key

On any plan, you can configure Bowrain to run against your own AI provider key.
Work that runs on your own key uses no Bowrain credits at all — you pay your
provider directly, and the workspace's weekly allowance is left untouched.

### Top-up packs

If a workspace needs more than its weekly allowance, the owner can buy a credit
pack: 200K credits for $5. Packs are one-time purchases, never charged
automatically on your behalf. They do not expire, and they are only drawn from
after the weekly plan allowance runs out — so a pack you buy now stays available
across weekly resets until it is spent.

### When credits run out

When the spendable balance (weekly allowance plus any purchased packs) reaches
zero, AI operations pause until the next weekly reset. A paused request is
rejected rather than run, and the workspace owner is emailed. Everything that
does not call an AI model — editing, review, and browsing content — keeps
working normally.

To restore AI access before the Monday reset, the owner can buy a credit pack or
upgrade the plan. Enterprise workspaces have unlimited credits and are never
paused.

The owner also receives an email warning when the weekly allowance reaches 80%
usage, before it is exhausted.

## Per-seat pricing on Team

The Team plan is priced per seat per month: the subscription quantity is the
number of seats, so you pay only for the seats you use. Weekly credits are shared
across all workspace members rather than allocated per seat.

At checkout the seat count defaults to the workspace's current member count and
cannot be set below it. Self-serve checkout covers up to 50 seats; larger teams
are handled as an Enterprise conversation.

## Trials

Every new workspace starts on a 14-day Pro trial. No card is required to start
it. During the trial the workspace has Pro limits and Pro weekly credits.

The trial is card-free and has no Stripe subscription behind it, so nothing
charges you when it ends — the workspace simply moves to the Free plan, which is
free forever and keeps 50K weekly credits and full access to the translation
editor. To stay on Pro (or move to Team), subscribe before or after the trial
ends.

## Managing your subscription

Subscription management is owner-only and runs through Stripe.

- **Subscribe** — from **Workspace Settings > Billing**, the owner starts
  checkout for Pro or Team. Checkout opens a hosted Stripe page; on completion
  the workspace moves to the new plan.
- **Change plan** — upgrades take effect immediately; downgrades apply at the end
  of the current billing period.
- **Billing portal** — the owner can open the Stripe customer portal to update
  the payment method, change the plan, view invoices, or cancel. Past invoices
  are also listed in the billing settings.
- **Buy credits** — the owner can purchase a top-up pack at any time from the
  same billing settings.

### If a payment fails

When a recurring payment fails, the subscription is marked **past due** and the
owner is emailed to update the payment method. Update it from the billing portal
to keep the workspace on its plan. If the subscription is ultimately canceled —
by you, or by Stripe after repeated failed attempts — the workspace is
downgraded to Free.

## Next steps

- [Workspaces](/server/workspaces) — the unit a plan and its credits apply to.
- [Members and roles](/server/members-and-roles) — invite teammates and manage
  seats.
- [Brand voice](/server/brand-voice) — brand scans and the corrections that feed
  them.
