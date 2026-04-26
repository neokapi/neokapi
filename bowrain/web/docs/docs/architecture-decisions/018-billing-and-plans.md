---
id: 018-billing-and-plans
sidebar_position: 18
title: "AD-018: Billing and Plans"
---

# AD-018: Billing and Plans

## Summary

Bowrain uses a four-tier plan model (Free, Pro, Team, Enterprise) with
weekly AI usage credits. Stripe Subscriptions and the Meters API are
the source of truth; webhooks sync plan state and credit allocations
into bowrain-server. `PlanGuard` and `QuotaGuard` Echo middleware
enforce feature gates and quota limits on the hot path. Per-workspace
feature overrides — set from the Admin Control Plane
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

| Plan           | Price         | AI credits / week   | @bravo                      | Seats     | Billing cycle |
| -------------- | ------------- | ------------------- | --------------------------- | --------- | ------------- |
| **Free**       | $0            | 50 K tokens         | 5 messages/day              | 1         | —             |
| **Pro**        | $25 / mo      | 500 K tokens (10×)  | Unlimited messages          | 3         | Monthly       |
| **Team**       | $20 / seat/mo | 2 M tokens (40×)    | Unlimited + code exec       | Unlimited | Monthly       |
| **Enterprise** | Custom        | Custom              | Custom                      | Unlimited | Annual        |

One credit equals one AI token (input or output). Operations cost:

| Operation                    | Credit cost      |
| ---------------------------- | ---------------- |
| AI translation (per token)   | 1                |
| AI quality check (per token) | 1                |
| @bravo message (per token)   | 1                |
| @bravo container time        | 10 credits / sec |

Weekly allocations reset Monday 00:00 UTC. Weekly (rather than monthly)
smooths spending, aligns with AI provider billing cycles, matches the
short-horizon feedback users expect from AI products (Claude's 5-hour
window is the nearest precedent), and avoids the month-one/month-four
binge-drought pattern.

**Overage handling.**

| Plan       | Overage behavior                                                        |
| ---------- | ----------------------------------------------------------------------- |
| Free       | Hard block until Monday reset                                           |
| Pro        | Soft block + option to buy a one-time credit pack ($5 = 200 K tokens)   |
| Team       | Configurable: soft block or auto-purchase credit packs                  |
| Enterprise | No limits (custom agreement)                                            |

### Feature matrix

Features are gated by plan using a compile-time matrix in
`billing/plans.go`:

| Feature               | Free | Pro | Team     | Enterprise |
| --------------------- | ---- | --- | -------- | ---------- |
| @bravo chat           | yes  | yes | yes      | yes        |
| @bravo code execution | –    | –   | yes      | yes        |
| Git connectors        | –    | yes | yes      | yes        |
| Custom connectors     | –    | –   | yes      | yes        |
| API access            | –    | yes | yes      | yes        |
| SSO / SAML            | –    | –   | –        | yes        |
| Custom MT providers   | –    | yes | yes      | yes        |
| Max projects          | 1    | 10  | unlimited| unlimited  |
| Max seats             | 1    | 3   | unlimited| unlimited  |

The matrix is the default authorization path: zero latency, no external
calls, deployed with the binary.

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
connectors := ws.Group("/connectors")
connectors.Use(billing.PlanGuard(billing.FeatureConnectorsGit))

agentExec := ws.Group("/agent/exec")
agentExec.Use(billing.PlanGuard(billing.FeatureBravoCodeExec))
```

**`QuotaGuard()`** rejects requests when weekly credits are exhausted.
Returns `429 Too Many Requests` with `Retry-After` set to the next
Monday 00:00 UTC. Applied to every AI-touching route.

Seat and project limits are enforced at mutation time (add member,
create project) — no middleware needed because the check depends on
the target of the operation.

### Stripe integration

**Products and prices.**

```
bowrain-pro          $25/mo flat subscription
bowrain-team-seat    $20/mo per seat, quantity-based subscription
bowrain-credits      $5 per 200 K pack, one-time metered via Stripe Meters
bowrain-enterprise   custom, manual invoicing
```

**Stripe Meters API** (v2) tracks real-time AI consumption:

```
Meter: bowrain_ai_tokens
  - event_name: ai_token_usage
  - aggregation: sum
  - dimensions: [workspace_id, operation_type]
```

**Webhook events** — subscription lifecycle flows inbound through
`POST /api/webhooks/stripe` with signature verification:

| Event                                  | Action                                       |
| -------------------------------------- | -------------------------------------------- |
| `checkout.session.completed`           | Activate subscription, set plan on workspace |
| `customer.subscription.updated`        | Update plan, adjust quotas                   |
| `customer.subscription.deleted`        | Downgrade to Free                            |
| `invoice.paid`                         | Confirm payment, grant weekly credits        |
| `invoice.payment_failed`               | Grace period (3 days), then downgrade        |
| `customer.subscription.trial_will_end` | Send reminder email                          |

### Data model

```sql
CREATE TABLE subscriptions (
    id                      TEXT PRIMARY KEY,
    workspace_id            TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    stripe_customer_id      TEXT NOT NULL,
    stripe_subscription_id  TEXT,
    plan                    TEXT NOT NULL DEFAULT 'free',
    status                  TEXT NOT NULL DEFAULT 'active',
    seat_count              INTEGER NOT NULL DEFAULT 1,
    current_period_start    TIMESTAMPTZ,
    current_period_end      TIMESTAMPTZ,
    cancel_at               TIMESTAMPTZ,
    created_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at              TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(workspace_id),
    UNIQUE(stripe_customer_id)
);

CREATE TABLE credit_allocations (
    id            TEXT PRIMARY KEY,
    workspace_id  TEXT NOT NULL REFERENCES workspaces(id) ON DELETE CASCADE,
    credits_total BIGINT NOT NULL,
    credits_used  BIGINT NOT NULL DEFAULT 0,
    week_start    TIMESTAMPTZ NOT NULL,
    week_end      TIMESTAMPTZ NOT NULL,
    source        TEXT NOT NULL DEFAULT 'plan',    -- 'plan' | 'purchased'
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
    reference_id  TEXT,                     -- job_id, conversation_id, etc.
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

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

    GetCurrentAllocation(ctx context.Context, workspaceID string) (*CreditAllocation, error)
    DeductCredits(ctx context.Context, workspaceID string, amount int64, op string, refID string) error
    CheckCredits(ctx context.Context, workspaceID string) (remaining int64, err error)
    GrantCredits(ctx context.Context, workspaceID string, amount int64, source string) error

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
- **Credit exhaustion** — "Weekly credits used. Resets Monday. [Buy credits →] or [Upgrade →]"
- **Seat limit** — "Your plan includes 3 seats. [Upgrade to Team →]"
- **Project limit** — "Free plan allows 1 project. [Upgrade to Pro →]"

The `UpgradePrompt` component lives in `packages/ui/` so the customer
app and the control plane render the same prompt.

### Email notifications

Triggered by billing lifecycle events:

| Trigger             | Email                                                                 |
| ------------------- | --------------------------------------------------------------------- |
| 80% credits used    | Warning with usage breakdown and reset date                           |
| Credits exhausted   | Blocked notice with upgrade CTA (Pro/Team) or reset countdown (Free)  |
| Weekly credit reset | Summary of last week's usage                                          |
| Payment failed      | Grace period notice (3 days), then downgrade warning                  |
| Subscription change | Confirmation of upgrade/downgrade with new limits                     |

### Self-hosted: graceful billing-disabled mode

Self-hosted deployments run without Stripe credentials. When
`STRIPE_SECRET_KEY` is empty, `BillingStore` returns a synthetic
"unlimited" subscription per workspace, `PlanGuard` becomes a no-op,
and `QuotaGuard` never rejects. The admin control plane is optional —
the endpoints register but the ctrl app is not deployed. The code path
remains the same; only the values change. This keeps the open-source
deployment experience uncompromised while letting the managed cloud
rely on the full billing pipeline.

## Consequences

- Stripe owns money; bowrain owns enforcement. The hot path reads a
  cached `plan` field — no external call on every request.
- Weekly credit windows give users frequent fresh starts and align
  spend with the AI provider's billing surface.
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
