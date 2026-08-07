---
id: 018-billing-and-plans
sidebar_position: 18
title: "AD-018: Billing and Plans"
---

# AD-018: Billing and Plans

## Summary

Bowrain uses a four-tier plan model (Free, Pro, Team, Enterprise). AI usage
credits are part of the paid offering: paid plans include a **monthly** credit
allowance, and the Free tier has no recurring allowance — every new workspace
instead receives a **one-time trial grant** of credits at creation, which
bounds free-tier cost exposure to one grant per workspace. Stripe
Subscriptions and the Meters API are the source of truth; webhooks sync plan
state and credit allocations into bowrain-server. `PlanGuard` and `QuotaGuard`
Echo middleware enforce feature gates and quota limits on the hot path.
Per-workspace feature overrides — set from the Admin Control Plane
([AD-017](017-bowrain-apps.md)) — sit above the plan matrix for betas,
partner deals, and support remediation. Self-hosted deployments run in
a graceful billing-disabled mode.

## Context

The platform needs a billing model that aligns revenue with AI spend,
caps unbounded usage, and supports both self-service checkout and
enterprise sales. Workspaces are the natural billing unit — they
already own usage accounting, seat management, and the permission
model — and they can mix free and paid tiers across the same user.

## Decision

### Four-tier plan model

| Plan           | Price         | AI credits / month           | @bravo                      | Seats     | Billing cycle |
| -------------- | ------------- | ---------------------------- | --------------------------- | --------- | ------------- |
| **Free**       | $0            | — (one-time 200 K trial grant) | 5 messages/day            | 1         | —             |
| **Pro**        | $25 / mo      | 2 M tokens                   | Unlimited messages          | 3         | Monthly       |
| **Team**       | $20 / seat/mo | 8 M tokens                   | Unlimited + code exec       | Unlimited | Monthly       |
| **Enterprise** | Custom        | Custom                       | Custom                      | Unlimited | Annual        |

The @bravo column records the intended per-plan design; @bravo is currently
dark on every plan (see the [feature matrix](#feature-matrix)) and is not
surfaced as a customer feature.

One credit equals one AI token (input or output). Operations cost:

| Operation                    | Credit cost      |
| ---------------------------- | ---------------- |
| AI translation (per token)   | 1                |
| AI quality check (per token) | 1                |
| @bravo message (per token)   | 1                |
| @bravo container time        | 10 credits / sec |

Monthly plan allocations reset on the 1st of each calendar month, 00:00 UTC —
the same cadence as the subscription itself, so the credits a paid plan
includes are a predictable part of what the customer buys each month. The Free
tier deliberately has **no recurring allowance**: its credits are the one-time
trial grant every workspace receives at creation. This is what bounds the
platform's free-tier AI cost exposure — one grant per workspace, ever, instead
of an open-ended recurring drip. All allowance numbers are named constants in
`billing/plans.go` (`MonthlyCredits`, `ProMonthlyCredits`,
`TeamMonthlyCredits`, `FreeTrialGrantCredits`); the current values are
provisional pending cold-start estimator data, and tuning them is an edit to
those constants plus the pricing surfaces, nowhere else.

**Overage handling.** Every paid plan behaves the same way: when the spendable
balance runs out, AI operations are blocked (`QuotaGuard`, HTTP 429) until
either the monthly reset or the workspace buys a credit pack. There is no
auto-purchase: a workspace can only be charged by a human choosing to be.

| Plan       | Overage behavior                                                      |
| ---------- | --------------------------------------------------------------------- |
| Free       | Blocked; the trial grant does not renew — upgrade or buy a pack        |
| Pro        | Blocked; may buy a credit pack ($5 = 200 K credits, does not expire)  |
| Team       | Blocked; may buy a credit pack (same pack)                            |
| Enterprise | No limits (custom agreement)                                          |

The pack size is one constant, `billing.CreditPackCredits`; the $5 lives in
Stripe. They are set together by the provisioning tool
(`bowrain/cmd/stripe-provision`), which is also what writes the
`bowrain_plan` price metadata the webhook reads.

### AI providers and credits

AI operations run on one of two provider sources, chosen per workspace
(`ProviderSource` in `jobs/resolved_provider.go`):

- **Platform** — the shared, platform-managed provider. Work metered against
  the workspace's credits. This is the **default**: a workspace with no
  provider configured (or one set to `platform`) runs here.
- **Bring-your-own (BYO)** — a per-workspace provider key the workspace saves.
  BYO work runs on the workspace's own key and **spends no credits** (usage is
  still recorded for the abuse cap). A configured BYO key overrides the
  platform default.

Selection is a single predicate (`TranslationJob.IsPlatformProvider`): an empty
or `platform` provider config routes to the platform provider; any other saved
config routes to BYO. Credits are deducted only when the resolved source is the
platform provider, so BYO is free of credit cost while platform work draws
down the allocation. The same distinction gates the synchronous editor path,
which treats a request carrying a BYO provider config or an inline API key as
BYO and skips the credit guard.

Credits come from three buckets, all recorded in `credit_allocations` with a
`source` column:

- **Plan** (`source = 'plan'`) — the monthly allocation for a PAID tier (the
  *AI credits / month* column above), keyed on the calendar month. Unspent
  plan credits expire at the monthly reset. Free has no plan bucket.
- **Trial** (`source = 'trial'`) — the one-time grant every workspace receives
  at creation (`FreeTrialGrantCredits`). Non-expiring, granted at most once
  per workspace, ever (enforced by the allocation table's unique key). It is
  granted at the workspace-creation sites (onboarding and explicit creation)
  and lazily backfilled by the allocation middleware for workspaces that
  predate it.
- **Purchased** (`source = 'purchased'`) — one-time credit packs bought through
  Stripe. Non-expiring; repeated purchases accumulate in one row.

When credits are spent, the cascade order is **plan → trial → purchased**:
the expiring bucket is drained first (it evaporates at month end anyway), the
promotional trial grant next, and pack credits the customer paid real money
for are preserved the longest. A workspace's spendable balance — the number
`QuotaGuard` and the job-enqueue pre-check enforce — is this month's remaining
plan credits plus all remaining trial and purchased credits. Admin-granted
bonus credits land in the purchased bucket, so they are spendable through the
same cascade.

### Feature matrix

Features are gated by plan using a compile-time matrix in
`billing/plans.go`:

| Feature               | Free | Pro | Team     | Enterprise | Enforced at                    |
| --------------------- | ---- | --- | -------- | ---------- | ------------------------------ |
| @bravo chat           | –    | –   | –        | –          | `PlanGuard` on the bravo routes |
| @bravo code execution | –    | –   | yes      | yes        | (moot while @bravo is dark)    |
| Git connectors        | –    | yes | yes      | yes        | `RequireFeature` in `HandleAddConnector` |
| Custom connectors     | –    | –   | yes      | yes        | reserved — no such feature yet |
| API access            | –    | yes | yes      | yes        | `PlanGuard` on the token group |
| SSO / SAML            | –    | –   | –        | yes        | reserved — no such feature yet |
| Max projects          | 1    | 10  | unlimited| unlimited  | workspace limits               |
| Max seats             | 1    | 3   | unlimited| unlimited  | workspace limits               |

The **Enforced at** column is part of the decision, not documentation colour: a
feature in this matrix with nothing enforcing it is a plan boundary the product
does not actually hold — sold on one plan, available on every plan. Rows marked
*reserved* gate a capability that does not exist yet; they are inert by
construction (there is no route to guard), and the pricing surfaces must not
advertise them until there is.

A flag that gates nothing real does not belong here at all: when a capability
leaves the product, its flag leaves the matrix rather than staying behind to
promise something the code cannot deliver.

The matrix is the default authorization path: zero latency, no external
calls, deployed with the binary.

`FeatureBravo` gates the entire @bravo surface (chat panel, settings, routes)
and is **dark on every plan** — its self-hostable runtime is not launch-ready
(see [AD-016](016-bravo-agent.md)). The only way to enable it is a
per-workspace [feature override](#per-workspace-feature-overrides) through the
control plane, used for internal dogfooding, so @bravo is not surfaced as a
customer feature. The `@bravo code execution` sub-gate reflects the intended
per-plan split for when the surface is enabled, but it has no effect while
`FeatureBravo` is off.

### Per-workspace feature overrides

Overrides sit above the plan matrix. They live in a DB table managed
through the Admin Control Plane ([AD-017](017-bowrain-apps.md)):

```sql
CREATE TABLE feature_overrides (
    id           TEXT PRIMARY KEY,
    workspace_id TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    feature      TEXT NOT NULL,
    enabled      BOOLEAN NOT NULL,
    reason       TEXT,
    created_by   TEXT NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    expires_at   TIMESTAMPTZ,
    UNIQUE(workspace_id, feature)
);
```

Use cases include beta programs, support compensation after outages,
partner deals, and gradual rollouts. Overrides with an `expires_at`
stop applying once expired and a periodic job cleans them up. The
`HasFeature` function checks the override first, then the plan matrix:

```go
func HasFeature(plan Plan, feature Feature, overrides map[Feature]bool) bool {
    if enabled, ok := overrides[feature]; ok {
        return enabled
    }
    if features, ok := PlanFeatures[plan]; ok {
        return features[feature]
    }
    return false
}
```

### Middleware

**`PlanGuard(feature)`** rejects requests when the workspace plan
doesn't include the required feature. It reads the workspace's `plan`
field from request context (already loaded by
`WorkspaceAccessMiddleware`) and returns `403 upgrade_required` with
the minimum qualifying plan so the frontend renders a contextual
upgrade prompt instead of a generic error.

```go
bravo := ws.Group("/bravo")
bravo.Use(billing.PlanGuard(billing.FeatureBravo))

tokens := ws.Group("/tokens")
tokens.Use(billing.PlanGuard(billing.FeatureAPIAccess))
```

**`billing.RequireFeature(c, feature)`** is the same gate for a decision that
can only be made after the body is read — a connector's *type* decides whether
the request needs a paid feature, and every connector type posts to one route.
It returns the identical `403 upgrade_required` payload, so the client's upgrade
prompt does not care which one blocked it.

**`QuotaGuard()`** rejects requests when the spendable balance (monthly plan
allowance + trial grant + purchased packs) is exhausted. Returns
`429 Too Many Requests` with `Retry-After` set to the next month start
(00:00 UTC on the 1st — when a paid allowance refreshes; a Free workspace's
way forward is an upgrade or a pack, which the client copy explains). Applied
to every AI-touching route. A workspace with **no credit rows at all** is
allowed through (the billing-disabled degrade path); a workspace whose buckets
exist but are spent is blocked — with no recurring Free allowance, "trial
grant fully spent" is the steady state of a free workspace and must read as
out-of-credits, not as unmetered.

Seat and project limits are enforced at mutation time (add member,
create project) — no middleware needed because the check depends on
the target of the operation.

### Trials

A new workspace gets two distinct trial artifacts at creation:

- The **one-time trial credit grant** (`billing.EnsureTrialGrant`, called at
  both creation sites — onboarding's personal workspace and explicit workspace
  creation). This is the workspace's AI budget until it converts to a paid
  plan; it never renews and never expires.
- A **14-day Pro features trial with no card** (`billing.SetupTrial`): plan
  `pro`, status `trialing`, and a `trial_ends_at` deadline. The trial grants
  Pro *features and limits* — **not** the Pro monthly credit allowance.
  `EnsureMonthlyAllocation` skips a `trialing` subscription precisely so a
  signup cannot mint a paid-size allocation; the paid allowance starts with
  the first touch after conversion. This is what keeps per-signup cost
  exposure bounded to `FreeTrialGrantCredits`.

When the deadline passes, the **trial sweeper** in `bowrain-worker`
(`billing.TrialSweeper`, hourly) moves the workspace to Free and records a
`trial_expired` event.

The sweeper is the *only* thing that can end this trial, which is why it is not
optional and not gated on Stripe: no Stripe subscription exists for a card-free
trial, so no Stripe event will ever downgrade the workspace. A deployment that
grants trials without running the sweeper grants Pro forever.

The downgrade updates the subscription *and* the denormalized `workspaces.plan`
cache in one statement (`ExpireTrials`). This is deliberate: the cache is what
the hot path reads — `PlanGuard` and the monthly-credit grant — so a downgrade
that updated the subscription but failed to update the cache as a separate step
would leave the workspace off `trialing` (never re-swept) yet still cached as
Pro, keeping Pro limits and, once no longer `trialing`, drawing the Pro monthly
allowance for nothing. One statement means a failure rolls back both and the
next tick retries; a checkout converting the trial mid-sweep serializes on the
same row lock and the paid plan wins.

The rejected alternative is a Stripe-side trial (`CheckoutOptions.TrialDays`):
it would end itself, but it requires collecting a card at signup, which is the
wrong trade for self-serve. The trial credit grant is untouched by the
downgrade — it is the workspace's one-time grant regardless of plan, and the
workspace keeps whatever remains of it on Free.

### Stripe integration

**Products and prices** are created by `bowrain/cmd/stripe-provision` (idempotent;
run once per Stripe account, test mode then live). Dollar amounts live in Stripe,
not in the code:

```
bowrain_pro_monthly        $25/mo flat subscription       metadata bowrain_plan=pro
bowrain_team_monthly_seat  $20/mo per seat (licensed)     metadata bowrain_plan=team
bowrain_credit_pack        $5 one-time, 200 K credits
(enterprise)               custom, manual invoicing
```

The `bowrain_plan` **price metadata is a contract**, not a label: it is what the
webhook reads to decide which plan a subscription is
(`billing.planFromSubscription`). A price created without it makes plan detection
fall back to guessing from the quantity.

**Stripe Meters API** (v2) records AI consumption for observability:

```
Meter event_name: ai_token_usage   (must match billing.UsageHooks exactly)
  - aggregation: sum over the `value` payload key
  - customer mapping: the `stripe_customer_id` payload key
  - payload also carries: workspace_id, operation_type
```

Meters **price nothing** — credits are the billing unit, and a meter event that
fails is logged and dropped. They exist so token consumption is visible in Stripe
next to the revenue. The event name is load-bearing: a meter created under any
other name silently discards every event, because meter reporting is
fire-and-forget by design (a billing telemetry failure must never fail a
translation).

**Why the credit ledger is ours and not Stripe's.** Stripe's billing credits
(credit grants) are *monetary* balances applied when an invoice finalizes. Our
credits are AI tokens, and they must be enforced **before** the call runs — the
whole point is to not spend money with Anthropic or Gemini on behalf of a
workspace that has none left. No invoice-time construct can do that, so the
ledger stays in Postgres, in the same transaction as the work. (Stripe's
token-billing product can hard-stop a call, but only by proxying every LLM request
through Stripe's AI Gateway, which is incompatible with bring-your-own keys — those
calls never touch Stripe.) Metronome, now Stripe-owned and Stripe's recommended
path for new metering integrations, does not enforce at request time either; it
would sit *under* a gate we would still have to keep. The option value is cheap:
all token accounting funnels through one seam, `billing.UsageHooks`, so swapping
what happens downstream of a deduction is a change to one file.

**Webhook events** — subscription lifecycle flows inbound through
`POST /api/webhooks/stripe` with signature verification:

| Event                            | Action                                                        |
| -------------------------------- | ------------------------------------------------------------- |
| `checkout.session.completed`     | Activate subscription; plan + seats from the session metadata  |
| `customer.subscription.created`  | Reconcile plan + seats from the price metadata                 |
| `customer.subscription.updated`  | Same handler — plan, seats, status, period                     |
| `customer.subscription.deleted`  | Downgrade to Free                                              |
| `invoice.paid`                   | Record the payment                                             |
| `invoice.payment_failed`         | Mark the subscription `past_due` and email the owner           |

Both `created` and `updated` are handled, and this is load-bearing: Stripe
emits `created` for a new subscription and `updated` only when something
subsequently *changes*, so a fresh Team subscription may never produce an
`updated` at all. The checkout session therefore carries the plan it was
started for, and the subscription events re-derive the same plan from the
price's `bowrain_plan` metadata.

Three delivery rules follow, and all three protect money rather than tidiness:

- **The pack grant is idempotent on the checkout session id.** Stripe delivers
  webhooks at-least-once, and this handler rolls its processed-event marker back
  on any dispatch error so a genuine failure is retried. That combination would
  let a $5 pack credit twice if the grant committed and a *later* step failed:
  the retry re-runs the grant. So the grant, its ledger row, and its
  `credits_purchased` event are one transaction keyed on the session id
  (`GrantPurchasedCredits`), with a unique index on the purchase ledger row —
  a duplicate delivery is a no-op, a concurrent one collides rather than
  double-credits.
- **A referenced deduction is idempotent.** The queues are at-least-once
  everywhere, not only at Stripe's edge: a translation job requeued after a
  provider error restarts from its first chunk, and a redelivered queue message
  re-runs the whole worker. Every metered call therefore carries a reference the
  caller can reproduce — `<job id>:<chunk offset>` for a translation, `<job
  id>:infer` / `:terms` for a brand scan, the message id for an agent turn — and
  `DeductCredits` applies one reference once. It checks inside the transaction
  that already holds the allocation buckets locked, and each ledger insert is
  `ON CONFLICT DO NOTHING` against the unique index above, with the
  `credits_used` bump skipped on a conflict. Both are needed: the index cannot
  see a replay that cascades into a *different* allocation, which a month
  rollover between two attempts produces.
- **`canceled` is terminal.** Stripe does not guarantee event ordering, so a
  stale `subscription.updated` (status active) can be delivered *after* the
  `subscription.deleted` that canceled it. A blind upsert would resurrect the
  workspace to its paid plan with no live subscription behind it. An `updated`
  for a locally-canceled subscription (status `canceled`, empty
  `stripe_subscription_id`) is therefore ignored — reactivation always arrives
  as a fresh checkout with a new subscription id, never as an update to the dead
  one.

**`past_due` keeps access.** A failed payment marks the subscription `past_due`
and notifies the owner, but does not downgrade or block: access ends only when
Stripe's dunning gives up and cancels, which arrives as
`customer.subscription.deleted`. There is no grace-period timer in Bowrain —
the retry schedule is configured in Stripe, where the payment state actually
lives, and duplicating it here would mean two clocks that can disagree. The
exposure is bounded (a workspace using a plan it has stopped paying for, for as
long as Stripe keeps retrying) and the billing page shows the customer a banner
throughout.

### Data model

```sql
CREATE TABLE subscriptions (
    id                      TEXT PRIMARY KEY,
    workspace_id            TEXT NOT NULL UNIQUE,
    -- Nullable, and unique only among non-empty values: a local trial and an
    -- admin plan override are subscriptions with no Stripe customer, and a
    -- NOT NULL UNIQUE column would let only the FIRST such workspace exist.
    stripe_customer_id      TEXT DEFAULT '',
    stripe_subscription_id  TEXT,
    plan                    TEXT NOT NULL DEFAULT 'free',
    status                  TEXT NOT NULL DEFAULT 'active',
    seat_count              INTEGER NOT NULL DEFAULT 1,
    current_period_start    TIMESTAMPTZ,
    current_period_end      TIMESTAMPTZ,
    cancel_at               TIMESTAMPTZ,
    trial_ends_at           TIMESTAMPTZ,   -- local card-free trial; NULL once a subscription exists
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE UNIQUE INDEX subscriptions_stripe_customer_id_key
    ON subscriptions (stripe_customer_id)
    WHERE stripe_customer_id IS NOT NULL AND stripe_customer_id != '';

CREATE TABLE credit_allocations (
    id            TEXT PRIMARY KEY,
    workspace_id  TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    credits_total BIGINT NOT NULL,
    credits_used  BIGINT NOT NULL DEFAULT 0,
    -- Allocation period bounds. plan rows: the calendar month (UTC);
    -- trial/purchased rows: a fixed non-expiring sentinel (1970..9999), which
    -- collapses each non-expiring source into one row per workspace via the
    -- UNIQUE constraint. The week_* column names bound whatever period the
    -- row's source defines; renaming them is not a zero-downtime change.
    week_start    TIMESTAMPTZ NOT NULL,
    week_end      TIMESTAMPTZ NOT NULL,
    source        TEXT NOT NULL DEFAULT 'plan',    -- 'plan' | 'trial' | 'purchased'
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id, week_start, source)
);

CREATE TABLE credit_ledger (
    id            BIGSERIAL PRIMARY KEY,
    workspace_id  TEXT NOT NULL,
    allocation_id TEXT REFERENCES credit_allocations(id),
    amount        BIGINT NOT NULL,         -- negative = debit, positive = credit
    balance_after BIGINT NOT NULL,
    operation     TEXT NOT NULL,            -- 'ai_translation' | 'bravo_message' | ...
    reference_id  TEXT,                     -- job_id:chunk, conversation_id, etc.
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- A referenced charge is applied at most once, whichever buckets it cascades
-- into. Every metered path is at-least-once.
CREATE UNIQUE INDEX credit_ledger_usage_ref
    ON credit_ledger (workspace_id, operation, reference_id, allocation_id)
    WHERE reference_id <> '';

ALTER TABLE workspaces ADD COLUMN plan TEXT NOT NULL DEFAULT 'free';
ALTER TABLE workspaces ADD COLUMN stripe_customer_id TEXT;
```

The subscription table caches Stripe state — Stripe remains the source
of truth, webhooks keep the cache current, and the cached `plan` on
`workspaces` makes `PlanGuard` a zero-query middleware.

### BillingStore interface

```go
type BillingStore interface {
    GetSubscription(ctx context.Context, workspaceID string) (*Subscription, error)
    UpsertSubscription(ctx context.Context, sub *Subscription) error

    // The current month's plan allocation (nil semantics for Free/trialing).
    GetCurrentAllocation(ctx context.Context, workspaceID string) (*CreditAllocation, error)
    // Cascades plan → trial → purchased; one ledger entry per bucket touched.
    DeductCredits(ctx context.Context, workspaceID string, amount int64, op string, refID string) error
    // The spendable balance across all three buckets.
    CheckCredits(ctx context.Context, workspaceID string) (remaining int64, err error)
    // Plan grants dedupe on the month key; non-expiring sources accumulate.
    GrantCredits(ctx context.Context, workspaceID string, amount int64, source string) error
    // Idempotent on the Stripe checkout session id.
    GrantPurchasedCredits(ctx context.Context, workspaceID string, amount int64, referenceID string) (granted bool, err error)
    // At most one trial grant per workspace, ever.
    GrantTrialCredits(ctx context.Context, workspaceID string, amount int64) (granted bool, err error)

    GetLedger(ctx context.Context, workspaceID string, from, to time.Time) ([]LedgerEntry, error)
}
```

### Credit deduction path

AI operations record detailed usage (for debugging, cost tracking, and
quota enforcement) alongside credit deduction:

```
AI tool / @bravo
    │
    ├─▶ jobs.QuotaStore.RecordUsage()       # detailed token tracking (AD-015)
    ├─▶ agent.AgentStore.RecordUsage()       # @bravo per-conversation usage (AD-016)
    └─▶ billing.BillingStore.DeductCredits() # credit deduction
              │
              ├─▶ PostgreSQL credit_ledger
              └─▶ Stripe Meter Event (async)
```

Stripe Meter events flow asynchronously so the hot path never blocks
on an external API call.

### PostHog for analytics

PostHog's role is analytics and experiments, not billing or gating:

- **Usage patterns** — which AI features are used, by whom, how often.
- **Conversion funnel** — viewed pricing → started checkout →
  completed checkout, viewed feature gate → upgraded.
- **Churn prediction** — declining usage patterns before cancellation.
- **Experiments** — trial length, starting-credits multipliers,
  upgrade-prompt placement.

PostHog is loaded on both `app.bowrain.cloud` and `ctrl.bowrain.cloud`,
identifies users by user ID + workspace context, and tracks
conversion events emitted by `PlanGuard` on `403 upgrade_required`.

### Keycloak admin realm

The billing subsystem depends on the `bowrain-admin` Keycloak realm
([AD-017](017-bowrain-apps.md)) to gate admin endpoints
(`/api/admin/workspaces`, `/api/admin/workspaces/:id/credits`,
`/api/admin/workspaces/:id/feature-overrides`, …). The admin realm
hosts operator accounts; Stripe customer mapping lives on the
subscription table keyed by workspace.

### API surface

```
# Customer self-service
GET    /api/v1/:ws/billing                  # current plan, usage, credits
GET    /api/v1/:ws/billing/usage             # credit usage breakdown
POST   /api/v1/:ws/billing/checkout          # create Stripe Checkout session
POST   /api/v1/:ws/billing/portal            # create Stripe Customer Portal session
GET    /api/v1/:ws/billing/invoices           # invoice history
POST   /api/v1/:ws/billing/buy-credits        # one-time credit pack purchase

# Admin (control plane)
PUT    /api/admin/workspaces/:id/plan                # override plan
POST   /api/admin/workspaces/:id/credits             # grant bonus credits
GET    /api/admin/workspaces/:id/feature-overrides   # list overrides
PUT    /api/admin/workspaces/:id/feature-overrides   # set overrides
GET    /api/admin/events                             # billing event feed
GET    /api/admin/upsells                            # ranked upsell opportunities

# Webhooks
POST   /api/webhooks/stripe                          # Stripe webhook, signature-verified
```

### Upgrade prompts

When a request hits a plan gate, the `403` body includes
`feature` and `minimum_plan` so the frontend renders a contextual
prompt instead of a generic error:

- **Feature gate** — "Git connectors require a Pro plan. [Upgrade →]"
- **Credit exhaustion** — "Credits used. Paid plans reset on the 1st. [Buy credits →] or [Upgrade →]"
- **Seat limit** — "Your plan includes 3 seats. [Upgrade to Team →]"
- **Project limit** — "Free plan allows 1 project. [Upgrade to Pro →]"

The `UpgradePrompt` component lives in `packages/ui/` so the customer
app and the control plane render the same prompt.

### Email notifications

Triggered by billing lifecycle events:

| Trigger             | Email                                                                 |
| ------------------- | --------------------------------------------------------------------- |
| 80% credits used    | Warning with usage against the monthly plan allowance and reset date (paid plans — Free has no plan allocation to warn on) |
| Credits exhausted   | Blocked notice with upgrade / credit-pack CTA; the monthly reset date applies to paid plans |
| Payment failed      | Notice that Stripe will retry, and that cancellation drops to Free     |
| Subscription change | Confirmation of upgrade/downgrade with new limits                     |

There is deliberately no trial-ending reminder email: the trial is card-free, so
its end is not a charge — it is a quiet drop back to Free limits, which the
billing page states from the day the workspace is created.

### Self-hosted: graceful billing-disabled mode

Self-hosted deployments run without Stripe credentials. With no
`STRIPE_SECRET_KEY`, no Stripe client is built, no plan lands on the request
context, `PlanGuard` and `RequireFeature` become no-ops, and `QuotaGuard` never
rejects. `GET /billing/plans` reports every plan as not purchasable, so the UI
shows no upgrade buttons rather than buttons that fail. The admin control plane
is optional — the endpoints register but the ctrl app is not deployed. This keeps
the open-source deployment experience uncompromised while letting the managed
cloud rely on the full billing pipeline.

**A placeholder is not a configuration.** The Stripe settings are accepted only
when they look like Stripe identifiers (`sk_`/`rk_`, `whsec_`, `price_`).
Terraform creates the Stripe SSM parameters as literal `CHANGEME` (epic 002), and
`STRIPE_SECRET_KEY` being non-empty is what the platform reads as *this is a
billed deployment* — it enables checkout, turns on worker metering, and makes a
missing `BOWRAIN_SECRETS_KEY` a hard startup failure. A placeholder that passed
for a key would give the worst available state: billing advertised, every Stripe
call rejected. An unprovisioned production therefore behaves exactly like a
self-hosted install until the real values are written.

## Consequences

- Stripe owns money; bowrain owns enforcement. The hot path reads a
  cached `plan` field — no external call on every request.
- Monthly credit windows make the included credits a predictable part of the
  paid subscription, on the same cadence the customer is billed.
- Free-tier AI cost exposure is bounded: one trial grant per workspace, ever,
  instead of an open-ended recurring allowance.
- Feature overrides provide the escape hatch needed for real customer
  situations (betas, outages, partner deals) without polluting the
  plan matrix.
- Admin operators can grant credits, change plans, and toggle features
  from a single screen, all audited.
- Self-hosted users are not billed, not gated, and not surprised.

## Related

- [AD-011: REST API](011-rest-api.md) — admin, billing, and webhook route families
- [AD-015: Server-Side AI Operations](015-server-ai-operations.md) — translation quota system that feeds credit deductions
- [AD-016: Bravo Agent](016-bravo-agent.md) — @bravo usage drains the same pool
- [AD-017: Bowrain Apps](017-bowrain-apps.md) — Admin Control Plane managing plans and overrides
