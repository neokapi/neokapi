---
title: Billing and credits
description: Bowrain's plans, what each one meters (markets, brands, custodian seats, AI credits), the one-time trial grant, top-up packs, trials, and how to manage a subscription.
sidebar_position: 13
---

# Billing and credits

Bowrain bills a workspace on one of four plans. A plan sets three things: the
features the workspace can use, the scope of custody it holds (markets, brands,
and custodian seats), and its monthly allowance of AI credits. Billing is
managed by the workspace owner from **Workspace Settings > Billing**.

<!--
Plan facts come from bowrain/billing/plans.go (PlanLimits, MonthlyCredits,
FreeTrialGrantCredits, CreditPackCredits, PlanFeatures, DefaultTrialDays). The
landing renders the same facts from the generated
bowrain/web/landing/src/generated/plans.json. When plans.go changes, update
this table in the same change.
-->

## Plans

| | **Free** | **Pro** | **Team** | **Enterprise** |
| --- | --- | --- | --- | --- |
| Price | $0 | $25 / month | $20 / seat / month | Custom |
| Monthly AI credits | none (one-time 200K trial credits) | 2M | 8M | Unlimited |
| Markets | 2 | 5 | 25 | Unlimited |
| Brands | 1 | 1 | 3 | Unlimited |
| Custodian seats | none | 1 | 5 | Unlimited |
| Git connectors | no | Yes | Yes | Yes |
| Custom connectors | no | no | Yes | Yes |
| API access | Yes | Yes | Yes | Yes |
| SSO / SAML | no | no | no | Yes |
| Bring your own AI key | Yes | Yes | Yes | Yes |

Free is free forever. Pro and Team are self-serve: a workspace owner subscribes
to them from the app. Enterprise is arranged directly and adds SSO/SAML,
dedicated support, custom agreements, and SLA terms.

Three things are never metered, on any plan:

- **Members.** Viewers, contributors, reviewers and machine tokens are free and
  uncapped. The people most worth having in the workspace are the ones who
  notice what no rule caught, and a seat cap is a standing reason not to invite
  them.
- **Checks.** Every check runs on every plan without limit, including Free.
- **Coordinates.** A scan proposes axes and a person approves them; approving
  one never costs anything. Markets and brands are tier boundaries, never
  per-point charges.

### Markets and brands

A **market** is a workspace-defined scope, a name plus the locales it covers
(see [Context](/server/context#markets)). A **brand** is the coarsest
coordinate a point sits under: a workspace has brands, a brand has products, a
product ships channels. Together they bound the scope of custody a workspace
holds, and markets is the number a workspace expands into most often.

### Custodians

A **custodian** is someone who may author what governs content, the voice and
the terms, over a bounded region of the workspace's points. Custodians own rules
and escalations; they hold no review queue of their own. A reviewer bounded to a
point is a contributor with a narrow beat, and contributors are never metered.

- The seat is **derived from a grant**, never declared. Holding `manage_voice`
  or `manage_terms` over a region makes a member a custodian, so the billable
  role and what a person can do are one fact and cannot drift apart.
- A grant that would exceed the plan's custodian allowance is **refused at
  grant time**, with the count and the limit, so the answer is actionable.
- When a plan carries no custodian seats, which is what a lapsed trial leaves
  behind, existing custodial authority stops resolving. Nothing is deleted: the
  voice, the terms, the rules and the coordinates stay exactly as they are, and
  the authority returns the moment a plan does.
- The workspace **owner keeps an implicit, unbillable custodianship at the root
  point**, so approval is always possible even on Free.
- A point nobody holds is reported on its profile card in the
  [Context hub](/server/context#profiles). It never blocks a build or a run.

See [Members and roles](/server/members-and-roles#custodians-and-points) for
how a grant is bound to a point.

## AI credits

AI work in Bowrain is metered in credits. One credit equals one AI token,
counting both the tokens sent to the model and the tokens it returns.

Paid plans come with a monthly credit allowance. The balance resets on the 1st
of each month at 00:00 UTC to the plan's full allowance, 2M on Pro and 8M on
Team. Enterprise credits are unlimited. Unused monthly credits do not roll
over.

The Free plan has no recurring allowance. Instead, every new workspace receives
a **one-time grant of 200K trial credits** when it is created. Trial credits
never expire, but they also never renew. Once they are spent, a free workspace
restores AI access by upgrading to a paid plan, buying a top-up pack, or
bringing its own AI key.

### What consumes credits

Credits are spent by the operations that call an AI model on your behalf:

- **Drafting** in a run, whether the run started from a push, from `kapi up`,
  from a connector, or from the Runs view.
- **Inline AI actions** in the editor.
- **Context scans**, which read a corpus and draft a profile, axes and term
  candidates.

Editing, reviewing, running checks, and reading content memory or terms do not
consume credits.

### Bring your own AI key

On any plan, you can configure Bowrain to run against your own AI provider key
under **Workspace Settings > Providers**. Work that runs on your own key uses no
Bowrain credits at all: you pay your provider directly, and the workspace's
credit balance is left untouched.

### Top-up packs

If a workspace needs more credits, the owner can buy a credit pack: 200K
credits for $5. Packs are one-time purchases, never charged automatically on
your behalf. They do not expire, and they are drawn from last, after the
monthly plan allowance and any remaining trial credits, so a pack you buy now
stays available across monthly resets until it is spent.

### When credits run out

When the spendable balance (monthly allowance, plus remaining trial credits,
plus any purchased packs) reaches zero, AI operations pause. A paused request
is rejected rather than run, and the workspace owner is emailed. Everything
that does not call an AI model keeps working: editing, review, checks, and
browsing content.

On a paid plan, AI access returns at the next monthly reset; at any time the
owner can also buy a credit pack or upgrade the plan. Enterprise workspaces
have unlimited credits and are never paused.

The owner of a paid workspace also receives an email warning when the monthly
allowance reaches 80% usage, before it is exhausted.

## Per-seat pricing on Team

The Team plan is priced per seat per month. The seat is the custodian seat
described above: the subscription quantity is the number of custodians the
workspace may hold, up to the plan's allowance of five. Members who are not
custodians are free. Monthly credits are shared across the whole workspace
rather than allocated per seat.

At checkout the quantity defaults to the custodians the workspace already has
and cannot be set below it. Self-serve checkout covers up to 50 seats; larger
teams are handled as an Enterprise conversation.

## Trials

Every new workspace starts on a 14-day Pro trial. No card is required to start
it. During the trial the workspace has Pro limits and features, including one
custodian seat, and its AI work draws on the one-time 200K trial credits every
workspace receives at creation. The Pro monthly allowance starts with a paid
subscription.

The trial is card-free and has no Stripe subscription behind it, so nothing
charges you when it ends. The workspace moves to the Free plan, which is free
forever, keeps whatever trial credits remain, and keeps everything it governed:
custodial authority is suspended until a plan returns, and nothing is deleted.
To stay on Pro (or move to Team), subscribe before or after the trial ends.

## Managing your subscription

Subscription management is owner-only and runs through Stripe.

- **Subscribe.** From **Workspace Settings > Billing**, the owner starts
  checkout for Pro or Team. Checkout opens a hosted Stripe page; on completion
  the workspace moves to the new plan.
- **Change plan.** Upgrades take effect immediately; downgrades apply at the end
  of the current billing period.
- **Billing portal.** The owner can open the Stripe customer portal to update
  the payment method, change the plan, view invoices, or cancel. Past invoices
  are also listed in the billing settings.
- **Buy credits.** The owner can purchase a top-up pack at any time from the
  same billing settings.

### If a payment fails

When a recurring payment fails, the subscription is marked **past due** and the
owner is emailed to update the payment method. Update it from the billing portal
to keep the workspace on its plan. If the subscription is ultimately canceled,
by you or by Stripe after repeated failed attempts, the workspace is downgraded
to Free.

## Next steps

- [Workspaces](/server/workspaces): the unit a plan and its credits apply to.
- [Members and roles](/server/members-and-roles): invite teammates and bind
  custodians to points.
- [Context](/server/context): the points, markets and profiles a plan bounds.
