---
title: Billing and credits
description: Bowrain's plans, per-seat pricing, monthly AI credits, the one-time trial grant, top-up packs, trials, and how to manage a subscription — what each plan includes and what happens when credits run out or a payment fails.
sidebar_position: 13
---

# Billing and credits

Bowrain bills a workspace on one of four plans. A plan sets three things: the
features the workspace can use, its numeric limits (projects and seats), and its
monthly allowance of AI credits. Billing is managed by the workspace owner from
**Workspace Settings > Billing**.

## Plans

| | **Free** | **Pro** | **Team** | **Enterprise** |
| --- | --- | --- | --- | --- |
| Price | $0 | $25 / month | $20 / seat / month | Custom |
| Monthly AI credits | — (one-time 200K trial credits) | 2M | 8M | Custom (unlimited) |
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

Paid plans come with a monthly credit allowance. The balance resets on the 1st
of each month at 00:00 UTC to the plan's full allowance — 2M on Pro, 8M on
Team. Enterprise credits are unlimited. Unused monthly credits do not roll
over; they reset each month, which keeps costs predictable.

The Free plan has no recurring allowance. Instead, every new workspace receives
a **one-time grant of 200K trial credits** when it is created. Trial credits
never expire, but they also never renew — once they are spent, a free workspace
restores AI access by upgrading to a paid plan, buying a top-up pack, or
bringing its own AI key.

### What consumes credits

Credits are spent by the AI operations Bowrain runs on your behalf:

- **AI translation** — translating content, whether run as a batch flow or
  requested inline in the translation editor.
- **Context scans** — analyzing content to build or update a voice profile.

Editing, reviewing, running non-AI checks, and reading content memory or
terminology do not consume credits. Only the operations that call an AI model
draw down the balance.

### Bring your own AI key

On any plan, you can configure Bowrain to run against your own AI provider key.
Work that runs on your own key uses no Bowrain credits at all — you pay your
provider directly, and the workspace's credit balance is left untouched.

### Top-up packs

If a workspace needs more credits, the owner can buy a credit pack: 200K
credits for $5. Packs are one-time purchases, never charged automatically on
your behalf. They do not expire, and they are drawn from last — after the
monthly plan allowance and any remaining trial credits — so a pack you buy now
stays available across monthly resets until it is spent.

### When credits run out

When the spendable balance (monthly allowance, plus remaining trial credits,
plus any purchased packs) reaches zero, AI operations pause. A paused request
is rejected rather than run, and the workspace owner is emailed. Everything
that does not call an AI model — editing, review, and browsing content — keeps
working normally.

On a paid plan, AI access returns at the next monthly reset; at any time the
owner can also buy a credit pack or upgrade the plan. Enterprise workspaces
have unlimited credits and are never paused.

The owner of a paid workspace also receives an email warning when the monthly
allowance reaches 80% usage, before it is exhausted.

## Per-seat pricing on Team

The Team plan is priced per seat per month: the subscription quantity is the
number of seats, so you pay only for the seats you use. Monthly credits are
shared across all workspace members rather than allocated per seat.

At checkout the seat count defaults to the workspace's current member count and
cannot be set below it. Self-serve checkout covers up to 50 seats; larger teams
are handled as an Enterprise conversation.

## Trials

Every new workspace starts on a 14-day Pro trial. No card is required to start
it. During the trial the workspace has Pro limits and features, and its AI work
draws on the one-time 200K trial credits every workspace receives at creation.
The Pro monthly allowance starts with a paid subscription.

The trial is card-free and has no Stripe subscription behind it, so nothing
charges you when it ends — the workspace moves to the Free plan, which
is free forever, keeps whatever trial credits remain, and retains full access
to the translation editor. To stay on Pro (or move to Team), subscribe before
or after the trial ends.

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
- [Voice & corrections](/server/context-voice) — context scans and the corrections that feed
  them.
